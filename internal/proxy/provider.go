// Package proxy provides a generic reverse proxy infrastructure for injecting
// secrets into API requests from the devcontainer. This file defines the
// Provider interface and registration mechanism. Each proxy service (e.g.
// GitHub, OpenAI) implements Provider and registers it via RegisterProvider
// in an init() function. The proxy package uses registered providers to
// discover and set up all enabled proxy services when SetupAllProxies is
// called, so that callers (e.g. dcx exec) do not need to interact with
// service-specific sub-packages directly.
package proxy

import (
	"net/http"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

// Provider describes a proxy service that can be registered and set up by the
// proxy infrastructure. Each service (e.g. GitHub, OpenAI) implements this
// interface and registers via RegisterProvider in an init() function.
// SetupAllProxies iterates over all registered providers and sets up each
// enabled one, handling all generic infrastructure (TLS, CA certs, container
// injection, remote env vars) so that the caller only needs to pass the
// config and collect the results.
type Provider interface {
	// Name returns the human-readable name of the provider (e.g. "github"),
	// used in log messages to identify which proxy instance is acting.
	Name() string

	// Enabled returns true if this proxy service is enabled in the given
	// config. The proxy infrastructure only sets up providers that report
	// enabled.
	Enabled(cfg *config.Config) bool

	// ServiceOptions returns the proxy.Options for this service based on the
	// given config and gateway IP. The options include network settings, TLS
	// configuration, and container paths. The proxy infrastructure uses these
	// options to create and start the Service.
	ServiceOptions(cfg *config.Config, gatewayIP string) Options

	// CreateHandler creates the HTTP handler for the proxy service. The
	// handler implements service-specific request rewriting and header
	// injection (e.g. the GitHub provider creates a reverse proxy that
	// rewrites requests to the GitHub API and injects the auth token).
	CreateHandler(opts Options, cfg *config.Config) (http.Handler, error)

	// RemoteEnvVars returns service-specific remote environment variables for
	// the container (e.g. GH_HOST for the GitHub proxy). These are combined
	// with generic TLS env vars (SSL_CERT_FILE, NODE_EXTRA_CA_CERTS) by the
	// proxy infrastructure. The port parameter is the port the proxy is
	// listening on.
	RemoteEnvVars(port int, opts Options, cfg *config.Config) []string
}

// providers holds all registered proxy providers. Populated by sub-packages
// calling RegisterProvider in their init() functions.
var providers []Provider

// RegisterProvider registers a proxy service provider. Called by sub-packages
// (e.g. github, openai) in their init() functions so that SetupAllProxies can
// discover and set up all available proxy services without the caller needing
// to import service-specific packages.
func RegisterProvider(p Provider) {
	providers = append(providers, p)
}
