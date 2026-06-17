package features

import (
	"fmt"
	"sort"
	"strings"
)

// canonicalFeatureID returns the normalized fully-qualified feature identifier
// (without version) that is used as the uniform node key in the dependency
// graph. When RawID is present it is used (it is the original user-provided
// string); otherwise the reconstructed String() form is used.
func canonicalFeatureID(ref FeatureRef) string {
	if ref.RawID != "" {
		return stripVersion(ref.RawID)
	}
	return stripVersion(ref.String())
}

// Ordered resolves dependencies and returns features in installation order.
// It respects installsAfter soft dependencies via topological sort. If a
// circular dependency is detected, or if a feature declares dependsOn a
// feature not in the provided set, an error is returned.
func Ordered(features []ResolvedFeature, overrideOrder []string) ([]ResolvedFeature, error) {
	if len(overrideOrder) > 0 {
		return nil, fmt.Errorf("overrideFeatureInstallOrder is not yet supported")
	}

	// Map from canonical feature ID (without version) to resolved feature.
	byID := make(map[string]*ResolvedFeature, len(features))
	// Secondary map from short Meta.ID to resolved feature for backward compatibility.
	byShortID := make(map[string]*ResolvedFeature, len(features))
	for i := range features {
		canonicalID := canonicalFeatureID(features[i].Ref)
		byID[canonicalID] = &features[i]
		if features[i].Meta.ID != "" {
			byShortID[features[i].Meta.ID] = &features[i]
		}
	}

	// lookupFeature finds a feature in the set by canonical or short ID.
	lookupFeature := func(id string) *ResolvedFeature {
		stripped := stripVersion(id)
		if f, ok := byID[stripped]; ok {
			return f
		}
		if f, ok := byShortID[stripped]; ok {
			return f
		}
		return nil
	}

	// Check for dependsOn referencing features outside the set.
	for i := range features {
		for depID := range features[i].Meta.DependsOn {
			if lookupFeature(depID) == nil {
				return nil, fmt.Errorf("feature %q dependsOn %q which is not in the feature set", features[i].Meta.ID, depID)
			}
		}
	}

	// Build graph from installsAfter.
	// Edge A -> B means A must be installed before B because B is in A's installsAfter.
	adj := make(map[string][]string) // canonical ID -> list of canonical IDs that come AFTER it
	inDegree := make(map[string]int)
	for i := range features {
		id := canonicalFeatureID(features[i].Ref)
		inDegree[id] = 0
	}
	for i := range features {
		id := canonicalFeatureID(features[i].Ref)
		for _, after := range features[i].Meta.InstallsAfter {
			f := lookupFeature(after)
			if f == nil {
				continue // soft dep not in set, ignore
			}
			afterID := canonicalFeatureID(f.Ref)
			adj[afterID] = append(adj[afterID], id)
			inDegree[id]++
		}
	}

	// Kahn's algorithm.
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	// Ensure deterministic output by sorting the initial queue.
	sort.Strings(queue)

	var result []ResolvedFeature
	for len(queue) > 0 {
		// Pop from front.
		id := queue[0]
		queue = queue[1:]

		result = append(result, *byID[id])

		next := adj[id]
		sort.Strings(next)
		for _, neighbor := range next {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
		sort.Strings(queue)
	}

	if len(result) != len(features) {
		return nil, fmt.Errorf("circular installsAfter dependency detected among features")
	}

	return result, nil
}

// stripVersion removes a trailing ":version" segment from a feature ID.
func stripVersion(id string) string {
	if idx := strings.LastIndex(id, ":"); idx > strings.LastIndex(id, "/") {
		return id[:idx]
	}
	return id
}
