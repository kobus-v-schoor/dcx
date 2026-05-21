// Package proxy provides a generic reverse proxy infrastructure for injecting
// secrets into API requests from the devcontainer. This file implements
// SetupAllProxies, which is the main entry point for the CLI layer. It
// iterates over registered providers, starts each enabled proxy, handles all
// generic setup (TLS certificates, CA bundle injection, remote env vars), and
// returns the combined results so the caller only needs to pass the final
// flags to the devcontainer command.
package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/docker"
)

// ProxySetupResult holds the results of setting up all proxy services.
// Returned by SetupAllProxies for use by the CLI layer.
type ProxySetupResult struct {
	// RemoteEnv contains the --remote-env flags for the devcontainer exec
	// command. These configure the container to route traffic through the
	// proxy services (e.g. GH_HOST, SSL_CERT_FILE, NODE_EXTRA_CA_CERTS).
	RemoteEnv []string

	// Cleanup must be called when the devcontainer session ends to stop all
	// proxy services and remove any temporary files on the host.
	Cleanup func()
}

// SetupAllProxies sets up all enabled proxy services based on the given config.
// It detects the Docker gateway IP, starts each enabled proxy provider, copies
// CA certificates into the container (for TLS-enabled proxies), creates CA
// bundles, and returns the combined remote env vars and a cleanup function.
//
// This is the single entry point that the CLI layer (e.g. dcx exec) should
// call — it does not need to interact with service-specific sub-packages or
// handle CA cert/bundle details directly. If a provider fails to set up, a
// warning is logged and the remaining providers continue without it.
func SetupAllProxies(ctx context.Context, cfg *config.Config, containerID string) (*ProxySetupResult, error) {
	// Create a Docker client for gateway IP detection and container
	// operations (copying CA certs, creating CA bundles).
	dockerCLI, err := docker.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating Docker client: %w", err)
	}
	defer func() { _ = dockerCLI.Close() }()

	// Determine the host IP that the container can reach. The proxy listens
	// on the gateway IP by default (more secure) so it is reachable from
	// the container. We inspect the container's network to find the gateway
	// IP, which is the host's IP on the Docker bridge network.
	gatewayIP, err := docker.GatewayIP(ctx, dockerCLI, containerID)
	if err != nil {
		return nil, fmt.Errorf("detecting host gateway IP: %w", err)
	}

	var remoteEnv []string
	var cleanups []func()

	// Iterate over all registered providers and set up each enabled one.
	// If a provider fails, log a warning and continue without it — the
	// user gets a shell but without that proxy service.
	for _, p := range providers {
		if !p.Enabled(cfg) {
			continue
		}

		env, cleanup, err := setupProvider(ctx, p, cfg, gatewayIP, dockerCLI, containerID)
		if err != nil {
			slog.Warn("proxy setup failed, proceeding without proxy",
				"provider", p.Name(), "error", err)
			continue
		}

		remoteEnv = append(remoteEnv, env...)
		cleanups = append(cleanups, cleanup)
	}

	result := &ProxySetupResult{
		RemoteEnv: remoteEnv,
		Cleanup: func() {
			for _, c := range cleanups {
				c()
			}
		},
	}

	return result, nil
}

// setupProvider sets up a single proxy provider. It creates and starts the
// proxy service, handles TLS cert injection if applicable, builds remote env
// vars (both generic and service-specific), and returns the env vars and a
// cleanup function. Returns an error if any step fails; the caller decides
// whether to abort or continue without the proxy.
func setupProvider(
	ctx context.Context,
	p Provider,
	cfg *config.Config,
	gatewayIP string,
	dockerCLI docker.DockerClient,
	containerID string,
) ([]string, func(), error) {
	opts := p.ServiceOptions(cfg, gatewayIP)

	handler, err := p.CreateHandler(opts, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("creating %s proxy handler: %w", p.Name(), err)
	}

	// Create and start the proxy service.
	svc := NewService(p.Name(), opts)
	port, err := svc.Start(handler)
	if err != nil {
		return nil, nil, fmt.Errorf("starting %s proxy: %w", p.Name(), err)
	}

	var hostCleanupFiles []string

	// If TLS is enabled, set up CA certificates in the container: write the
	// CA cert to a temp file on the host, copy it into the container, and
	// create a combined CA bundle so Go programs trust both the system CAs
	// and the proxy's self-signed CA.
	if opts.TLSEnabled {
		if err := setupTLSCerts(ctx, svc, opts, dockerCLI, containerID); err != nil {
			svc.Shutdown()
			cleanupHostFiles(hostCleanupFiles)
			return nil, nil, fmt.Errorf("setting up TLS certs for %s proxy: %w", p.Name(), err)
		}

		// Write the CA cert to a temp file on the host for potential
		// debugging use. The file is cleaned up when the proxy shuts down.
		caCertPath, err := WriteCACertToFile(svc.CACertPEM())
		if err != nil {
			svc.Shutdown()
			cleanupHostFiles(hostCleanupFiles)
			return nil, nil, fmt.Errorf("writing CA cert for %s proxy: %w", p.Name(), err)
		}
		hostCleanupFiles = append(hostCleanupFiles, caCertPath)
	}

	// Build remote env vars: service-specific (e.g. GH_HOST for the GitHub
	// proxy) plus generic TLS env vars (SSL_CERT_FILE, NODE_EXTRA_CA_CERTS).
	envVars := p.RemoteEnvVars(port, opts, cfg)
	if opts.TLSEnabled {
		envVars = append(envVars, tlsRemoteEnvVars(opts)...)
	}

	cleanup := func() {
		svc.Shutdown()
		cleanupHostFiles(hostCleanupFiles)
	}

	return envVars, cleanup, nil
}

// setupTLSCerts handles all TLS-related container setup for a proxy service.
// It copies the proxy's CA certificate into the container and creates a
// combined CA bundle that includes both the system CA certificates and the
// proxy's self-signed CA. This is necessary because Go's SSL_CERT_FILE
// replaces the system CA pool entirely (rather than appending to it), so a
// bundle containing only the proxy CA would break HTTPS for all other Go
// programs in the container. NODE_EXTRA_CA_CERTS does not need this — Node.js
// appends to the system trust store.
func setupTLSCerts(
	ctx context.Context,
	svc *Service,
	opts Options,
	dockerCLI docker.DockerClient,
	containerID string,
) error {
	// Create the target directory inside the container and copy the CA cert.
	// The directory must exist before CopyToContainer can place files into it.
	caDir := filepath.Dir(opts.CACertPathResolved())
	if err := docker.MkdirInContainer(ctx, dockerCLI, containerID, caDir); err != nil {
		return fmt.Errorf("creating CA cert directory in container: %w", err)
	}

	if err := docker.CopyBytesToContainer(
		ctx,
		dockerCLI,
		containerID,
		"ca.crt",
		svc.CACertPEM(),
		caDir,
	); err != nil {
		// Log the error and proceed — the client may still work without
		// the CA cert if the proxy is not used for HTTPS verification.
		slog.Warn("could not copy CA cert into container, client may not trust the proxy",
			"service", svc.name, "error", err)
	}

	// Create a combined CA bundle inside the container that merges the
	// system CA certificates with the proxy's self-signed CA certificate.
	if err := createCABundleInContainer(ctx, dockerCLI, containerID, opts); err != nil {
		slog.Warn("could not create combined CA bundle in container, Go programs may have HTTPS issues",
			"service", svc.name, "error", err)
	}

	return nil
}

// tlsRemoteEnvVars returns the generic TLS-related remote env vars that apply
// to all TLS-enabled proxies: SSL_CERT_FILE (pointing to the combined CA
// bundle) and NODE_EXTRA_CA_CERTS (pointing to the proxy's CA cert alone).
// These are combined with service-specific env vars from the provider.
func tlsRemoteEnvVars(opts Options) []string {
	return []string{
		fmt.Sprintf("--remote-env=SSL_CERT_FILE=%s", opts.CABundlePathResolved()),
		fmt.Sprintf("--remote-env=NODE_EXTRA_CA_CERTS=%s", opts.CACertPathResolved()),
	}
}

// createCABundleInContainer creates a combined CA bundle file inside the
// container that includes both the system CA certificates and the proxy's
// self-signed CA certificate. This is necessary because Go's SSL_CERT_FILE
// environment variable replaces the system CA pool entirely (it does not
// append), so setting it to only the proxy CA would break all HTTPS
// connectivity for Go programs in the container. The combined bundle is
// referenced by SSL_CERT_FILE, while NODE_EXTRA_CA_CERTS points to the
// proxy CA alone (Node.js appends rather than replaces).
func createCABundleInContainer(
	ctx context.Context,
	dockerCLI docker.DockerClient,
	containerID string,
	opts Options,
) error {
	// Build a multi-line script that concatenates the system CA bundle with the
	// proxy's CA cert. The system CA bundle location varies by distro; we
	// check the most common paths in order.
	//
	// Debian/Ubuntu: /etc/ssl/certs/ca-certificates.crt
	// Alpine:        /etc/ssl/certs/ca-certificates.crt (same)
	// RHEL/Fedora:   /etc/pki/tls/certs/ca-bundle.crt
	// OpenSUSE:      /etc/ssl/ca-bundle.pem
	script := fmt.Sprintf(`
		sys_ca=""
		for f in /etc/ssl/certs/ca-certificates.crt /etc/pki/tls/certs/ca-bundle.crt /etc/ssl/ca-bundle.pem; do
			if [ -f "$f" ]; then
				sys_ca="$f"
				break
			fi
		done
		if [ -n "$sys_ca" ]; then
			cat "$sys_ca" %s > %s
		else
			cp %s %s
		fi`,
		opts.CACertPathResolved(),
		opts.CABundlePathResolved(),
		opts.CACertPathResolved(),
		opts.CABundlePathResolved(),
	)

	if err := docker.ExecInContainer(ctx, dockerCLI, containerID, "sh", "-c", script); err != nil {
		return fmt.Errorf("creating combined CA bundle in container: %w", err)
	}

	return nil
}

// cleanupHostFiles removes the given temporary files from the host filesystem.
// Used as part of proxy cleanup to remove CA cert temp files.
func cleanupHostFiles(files []string) {
	for _, f := range files {
		_ = os.Remove(f)
	}
}
