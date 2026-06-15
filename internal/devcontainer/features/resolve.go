package features

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SourceType indicates how a feature is distributed.
type SourceType int

const (
	SourceOCI SourceType = iota
	SourceLocal
	SourceDirectTarball
)

// FeatureRef is a parsed feature identifier plus user options.
type FeatureRef struct {
	RawID     string                 // original string from devcontainer.json (normalized to lowercase)
	Registry  string                 // e.g. "ghcr.io"
	Namespace string                 // e.g. "devcontainers/features"
	ID        string                 // feature name, e.g. "github-cli"
	Version   string                 // e.g. "1", "latest"
	Options   map[string]interface{} // merged user opts + defaults
	Source    SourceType
}

// String returns the canonical fully-qualified feature ID.
func (r FeatureRef) String() string {
	var b strings.Builder
	if r.Registry != "" {
		b.WriteString(r.Registry)
		b.WriteString("/")
	}
	if r.Namespace != "" {
		b.WriteString(r.Namespace)
		b.WriteString("/")
	}
	b.WriteString(r.ID)
	if r.Version != "" {
		b.WriteString(":")
		b.WriteString(r.Version)
	}
	return b.String()
}

// CacheKey returns a string suitable for use as a cache directory name.
func (r FeatureRef) CacheKey() string {
	key := strings.ReplaceAll(r.String(), "/", "_")
	key = strings.ReplaceAll(key, ":", "_")
	return key
}

// Parse parses a raw feature key/value pair from devcontainer.json.
// If rawOptions is a JSON string, it is treated as a version override
// for the feature identifier. If it is a JSON object, it is parsed as
// feature options.
func Parse(rawID string, rawOptions json.RawMessage) (*FeatureRef, error) {
	ref := &FeatureRef{
		RawID:   strings.ToLower(rawID),
		Version: "latest",
		Options: make(map[string]interface{}),
	}

	// Detect source type.
	if strings.HasPrefix(ref.RawID, "./") || strings.HasPrefix(ref.RawID, "../") {
		ref.Source = SourceLocal
	} else if strings.HasPrefix(ref.RawID, "http://") || strings.HasPrefix(ref.RawID, "https://") {
		ref.Source = SourceDirectTarball
		ref.Version = ""
		// Parse version from URL fragment if any.
		if idx := strings.LastIndex(ref.RawID, ":"); idx > strings.LastIndex(ref.RawID, "/") {
			ref.Version = ref.RawID[idx+1:]
			ref.RawID = ref.RawID[:idx]
		}
	} else {
		ref.Source = SourceOCI
	}

	// Parse options.
	if len(rawOptions) > 0 && string(rawOptions) != "null" {
		var s string
		if err := json.Unmarshal(rawOptions, &s); err == nil {
			// String value means feature version override.
			ref.Version = s
		} else {
			var opts map[string]interface{}
			if err := json.Unmarshal(rawOptions, &opts); err != nil {
				return nil, fmt.Errorf("parsing feature options for %s: %w", rawID, err)
			}
			ref.Options = opts
		}
	}

	if ref.Source == SourceOCI {
		if err := parseOCIRef(ref); err != nil {
			return nil, err
		}
	}

	return ref, nil
}

// parseOCIRef splits an OCI feature identifier into registry, namespace, id,
// and version components.
func parseOCIRef(ref *FeatureRef) error {
	s := ref.RawID

	// Extract version from the last colon that is not part of a registry port.
	lastColon := strings.LastIndex(s, ":")
	lastSlash := strings.LastIndex(s, "/")
	if lastColon > lastSlash && lastColon != -1 {
		ref.Version = s[lastColon+1:]
		s = s[:lastColon]
	}

	// Split registry from the rest. The first segment before the first /
	// is the registry.
	firstSlash := strings.Index(s, "/")
	if firstSlash == -1 {
		return fmt.Errorf("invalid OCI feature reference %q: missing namespace", ref.RawID)
	}
	ref.Registry = s[:firstSlash]
	rest := s[firstSlash+1:]

	// The last segment is the feature ID; everything before it is the namespace.
	lastSlash = strings.LastIndex(rest, "/")
	if lastSlash == -1 {
		return fmt.Errorf("invalid OCI feature reference %q: missing feature ID", ref.RawID)
	}
	ref.Namespace = rest[:lastSlash]
	ref.ID = rest[lastSlash+1:]

	return nil
}
