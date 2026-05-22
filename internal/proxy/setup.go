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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"runtime"
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

	// Collect enabled providers and their domains, filters, and injectors.
	var domains []string
	var filters []func(*http.Request) (*http.Response, error)
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
		filters = append(filters, func(req *http.Request) (*http.Response, error) {
			return provider.FilterRequest(req, cfg)
		})
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

	// Build the onRequest callback that runs filters first, then injectors.
	// If any filter returns a non-nil response, the request is blocked.
	onRequest := func(req *http.Request) (*http.Request, *http.Response) {
		for _, filter := range filters {
			resp, err := filter(req)
			if resp != nil {
				return req, resp
			}
			if err != nil {
				slog.Debug("filter failed", "error", err)
			}
		}
		for _, inj := range injectors {
			if err := inj(req); err != nil {
				slog.Debug("credential injection failed", "error", err)
			}
		}
		return req, nil
	}

	// Determine cert expiry and bind address from config.
	certExpiry := cfg.Proxy.CertExpiry
	if certExpiry == 0 {
		certExpiry = 24 * time.Hour
	}
	bindAddr := cfg.Proxy.BindAddr

	// Create and start the proxy server.
	srv, err := NewServer(certExpiry)
	if err != nil {
		return nil, fmt.Errorf("creating proxy server: %w", err)
	}

	port, err := srv.Start(gatewayIP, bindAddr, domains, onRequest)
	if err != nil {
		return nil, fmt.Errorf("starting proxy: %w", err)
	}

	// Inject the CA certificate into the container's system trust store.
	certPath, err := injectCACert(ctx, dockerCLI, containerID, srv.CACertPEM())
	if err != nil {
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

			// Best-effort cleanup of the injected CA certificate.
			cleanupCtx := context.Background()
			dockerCLI, err := docker.NewClient(cleanupCtx)
			if err != nil {
				slog.Error("failed to create docker client for CA cert cleanup", "error", err)
				return
			}
			defer func() { _ = dockerCLI.Close() }()
			removeCACert(cleanupCtx, dockerCLI, containerID, certPath)
		},
	}, nil
}

// randomCertName generates a random filename for the proxy CA certificate.
// Using a unique name per exec session allows multiple concurrent dcx exec
// instances against the same container without colliding on the cert path.
func randomCertName() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails (extremely unlikely).
		return fmt.Sprintf("dcx-proxy-ca-%d.crt", time.Now().UnixNano())
	}
	return fmt.Sprintf("dcx-proxy-ca-%s.crt", hex.EncodeToString(b))
}

// injectCACert installs the CA certificate into the container's system trust
// store by copying it to /usr/local/share/ca-certificates/ and running
// update-ca-certificates. This is the standard approach on Debian-derived
// and Alpine images. It returns the full path to the installed certificate
// so cleanup can remove the exact file.
func injectCACert(ctx context.Context, dockerCLI docker.DockerClient, containerID string, caPEM []byte) (string, error) {
	certDir := "/usr/local/share/ca-certificates"

	if err := docker.MkdirInContainer(ctx, dockerCLI, containerID, certDir); err != nil {
		return "", fmt.Errorf("creating CA certificates directory in container: %w", err)
	}

	certName := randomCertName()
	if err := docker.CopyBytesToContainer(ctx, dockerCLI, containerID, certName, caPEM, certDir); err != nil {
		return "", fmt.Errorf("copying CA cert into container: %w", err)
	}

	if err := docker.ExecInContainer(ctx, dockerCLI, containerID, "update-ca-certificates"); err != nil {
		return "", fmt.Errorf("running update-ca-certificates in container: %w", err)
	}

	return certDir + "/" + certName, nil
}

// removeCACert removes the CA certificate from the container's system trust
// store. It deletes the cert file at the given path and runs
// update-ca-certificates to regenerate the CA bundle. Errors are logged but
// not returned because cleanup is best-effort (the container may already be
// stopped).
func removeCACert(ctx context.Context, dockerCLI docker.DockerClient, containerID, certPath string) {
	if err := docker.ExecInContainer(ctx, dockerCLI, containerID, "rm", "-f", certPath); err != nil {
		slog.Debug("failed to remove CA cert from container", "error", err)
	}

	if err := docker.ExecInContainer(ctx, dockerCLI, containerID, "update-ca-certificates", "--fresh"); err != nil {
		slog.Debug("failed to update CA certificates after removal", "error", err)
	}
}
