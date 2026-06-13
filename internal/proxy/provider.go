// Package proxy provides a transparent MITM proxy infrastructure for injecting
// secrets into API requests from the devcontainer. This file defines the
// Provider interface and registration mechanism. Each proxy service (e.g.
// GitHub) implements Provider and registers it via RegisterProvider in an
// init() function. The proxy package uses registered providers to discover
// which domains to intercept and how to inject credentials for each domain.
package proxy

import (
	"net/http"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

// Provider describes a proxy service that can be registered and used by the
// MITM proxy infrastructure. Each service (e.g. GitHub) implements this
// interface and registers via RegisterProvider in an init() function.
// SetupAllProxies iterates over all registered providers, collects their
// domains, and configures a single proxy server that intercepts traffic for
// all registered domains and injects the appropriate credentials.
type Provider interface {
	// Name returns the human-readable name of the provider (e.g. “github”),
	// used in log messages.
	Name() string

	// Enabled returns true if this proxy service is enabled in the given
	// config. The proxy infrastructure only sets up providers that report
	// enabled.
	Enabled(cfg *config.Config) bool

	// Domains returns the list of domains this provider handles. The proxy
	// will MITM intercept HTTPS traffic to these domains and call
	// PrepareRequest for matching requests. The returned domains must not
	// include ports (e.g. "github.com", not "github.com:443").
	Domains(cfg *config.Config) []string

	// PrepareRequest injects service-specific credentials or headers into
	// the intercepted request. Called by the proxy for each request whose
	// Host matches one of the provider's domains. The request object may be
	// modified in place (e.g. setting Authorization header). Returns an
	// error if injection fails; the error is logged and the request is
	// still forwarded.
	PrepareRequest(req *http.Request, cfg *config.Config) error

	// EnvVars returns additional container environment variables that this
	// provider needs to function correctly. These are injected alongside
	// HTTP_PROXY/HTTPS_PROXY. For example, the GitHub provider returns
	// {GH_TOKEN: dummy} so that the gh CLI inside the container makes API
	// requests (the proxy replaces the dummy token with the real host
	// token at the network layer).
	EnvVars(cfg *config.Config) map[string]string
}

// providers holds all registered proxy providers. Populated by sub-packages
// calling RegisterProvider in their init() functions.
var providers []Provider

// RegisterProvider registers a proxy service provider. Called by sub-packages
// (e.g. github) in their init() functions so that SetupAllProxies can
// discover and set up all available proxy services without the caller needing
// to import service-specific packages.
func RegisterProvider(p Provider) {
	providers = append(providers, p)
}
