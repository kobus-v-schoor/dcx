// Package proxy provides a transparent MITM proxy infrastructure for injecting
// secrets into API requests from the devcontainer. This file implements
// SetupAllProxies, which is the main entry point for the CLI layer. It
// iterates over registered providers, starts a single MITM proxy configured
// with all enabled provider domains, handles CA certificate injection into
// the container's system trust store, and returns the combined results so
// the caller only needs to pass the final flags to the devcontainer command.
package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"net/http"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/docker"
)

// ProxySetupResult holds the results of setting up the proxy.
// Returned by SetupAllProxies for use by the CLI layer.
type ProxySetupResult struct {
	// RemoteEnv contains the --remote-env flags for the devcontainer exec
	// command. These configure the container to route traffic through the
	// proxy (HTTP_PROXY, HTTPS_PROXY).
	RemoteEnv []string

	// Cleanup must be called when the devcontainer session ends to stop the
	// proxy and remove any temporary files on the host.
	Cleanup func()
}

// SetupAllProxies sets up the MITM proxy for all enabled providers based on
// the given config. It detects the Docker gateway IP, starts a single proxy
// that intercepts traffic for all enabled provider domains, injects the
// temporary CA certificate into the container's system CA bundle, and returns
// the combined remote env vars and a cleanup function.
//
// This is the single entry point that the CLI layer (e.g. dcx exec) should
// call — it does not need to interact with service-specific sub-packages
// directly. If no providers are enabled, it returns an empty result without
// error. If proxy setup fails, an error is returned.
func SetupAllProxies(ctx context.Context, cfg *config.Config, containerID string) (*ProxySetupResult, error) {
	// Create a Docker client for gateway IP detection and container
	// operations (copying CA cert, appending to CA bundle).
	dockerCLI, err := docker.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating Docker client: %w", err)
	}
	defer func() { _ = dockerCLI.Close() }()

	// Determine the host IP that the container can reach. The proxy listens
	// on the gateway IP by default (more secure) so it is reachable from
	// the container.
	gatewayIP, err := docker.GatewayIP(ctx, dockerCLI, containerID)
	if err != nil {
		return nil, fmt.Errorf("detecting host gateway IP: %w", err)
	}

	// Collect enabled providers and their domains.
	var domains []string
	var injectors []func(*http.Request) error

	for _, p := range providers {
		if !p.Enabled(cfg) {
			continue
		}
		pDomains := p.Domains(cfg)
		if len(pDomains) == 0 {
			continue
		}
		domains = append(domains, pDomains...)
		provider := p // capture for closure
		injectors = append(injectors, func(req *http.Request) error {
			return provider.PrepareRequest(req, cfg)
		})
	}

	if len(domains) == 0 {
		return &ProxySetupResult{
			RemoteEnv: nil,
			Cleanup:   func() {},
		}, nil
	}

	// Combine all injectors into a single function. Errors from individual
	// injectors are logged; the combined injector always returns nil so
	// requests are never blocked at the proxy layer.
	combinedInjector := func(req *http.Request) error {
		for _, inj := range injectors {
			if err := inj(req); err != nil {
				slog.Debug("credential injection failed", "error", err)
			}
		}
		return nil
	}

	// Determine cert expiry and bind address from config.
	certExpiry := cfg.Proxy.GitHub.CertExpiry
	if certExpiry == 0 {
		certExpiry = 24 * time.Hour
	}
	bindAddr := cfg.Proxy.GitHub.BindAddr

	// Create and start the proxy server.
	srv, err := NewServer(certExpiry)
	if err != nil {
		return nil, fmt.Errorf("creating proxy server: %w", err)
	}

	port, err := srv.Start(gatewayIP, bindAddr, domains, combinedInjector)
	if err != nil {
		return nil, fmt.Errorf("starting proxy: %w", err)
	}

	// Inject the CA certificate into the container's system trust store.
	if err := injectCACert(ctx, dockerCLI, containerID, srv.CACertPEM(), port); err != nil {
		srv.Shutdown()
		return nil, fmt.Errorf("injecting CA certificate: %w", err)
	}

	// Build the proxy URL that the container should use. On non-Linux hosts
	// containers reach the host via host.docker.internal.
	proxyHost := gatewayIP
	if runtime.GOOS != "linux" {
		proxyHost = "host.docker.internal"
	}
	proxyURL := fmt.Sprintf("http://%s:%d", proxyHost, port)

	remoteEnv := []string{
		fmt.Sprintf("--remote-env=HTTP_PROXY=%s", proxyURL),
		fmt.Sprintf("--remote-env=http_proxy=%s", proxyURL),
		fmt.Sprintf("--remote-env=HTTPS_PROXY=%s", proxyURL),
		fmt.Sprintf("--remote-env=https_proxy=%s", proxyURL),
	}

	// Add provider-specific env vars (e.g. GH_TOKEN=dummy for GitHub).
	for _, p := range providers {
		if !p.Enabled(cfg) {
			continue
		}
		for _, env := range p.EnvVars(cfg) {
			remoteEnv = append(remoteEnv, fmt.Sprintf("--remote-env=%s", env))
		}
	}

	return &ProxySetupResult{
		RemoteEnv: remoteEnv,
		Cleanup: func() {
			srv.Shutdown()
		},
	}, nil
}

// injectCACert copies the CA certificate into the container and appends it to
// the system CA bundles. This makes all TLS clients inside the container trust
// the proxy's dynamically-generated per-host certificates. The CA cert is
// copied to a session-unique path (based on the proxy port) and then appended
// to every existing system CA bundle found in the container. Concurrent
// appends are safe because each session appends from a distinct file and OS
// append writes are atomic for small writes.
func injectCACert(ctx context.Context, dockerCLI docker.DockerClient, containerID string, caPEM []byte, port int) error {
	caDir := fmt.Sprintf("/opt/dcx/proxy/%d", port)
	caPath := caDir + "/ca.crt"

	if err := docker.MkdirInContainer(ctx, dockerCLI, containerID, caDir); err != nil {
		return fmt.Errorf("creating CA cert directory in container: %w", err)
	}

	if err := docker.CopyBytesToContainer(ctx, dockerCLI, containerID, "ca.crt", caPEM, caDir); err != nil {
		return fmt.Errorf("copying CA cert into container: %w", err)
	}

	// Append the CA cert to every known system CA bundle path. Not all paths
	// exist in every distro; we try them all and ignore missing ones.
	bundles := []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
		"/etc/ssl/ca-bundle.pem",
	}

	// Build a single shell command that appends the cert to every existing bundle.
	var sb strings.Builder
	sb.WriteString("set -e\n")
	for _, bundle := range bundles {
		sb.WriteString(fmt.Sprintf("if [ -f %q ]; then cat %q >> %q; fi\n", bundle, caPath, bundle))
	}

	if err := docker.ExecInContainer(ctx, dockerCLI, containerID, "sh", "-c", sb.String()); err != nil {
		return fmt.Errorf("appending CA cert to system bundles: %w", err)
	}

	return nil
}
