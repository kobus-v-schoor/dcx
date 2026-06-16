package features

import (
	"fmt"
	"sort"
	"strings"
)

// Ordered resolves dependencies and returns features in installation order.
// It respects installsAfter soft dependencies via topological sort. If a
// circular dependency is detected, or if a feature declares dependsOn a
// feature not in the provided set, an error is returned.
func Ordered(features []ResolvedFeature, overrideOrder []string) ([]ResolvedFeature, error) {
	if len(overrideOrder) > 0 {
		return nil, fmt.Errorf("overrideFeatureInstallOrder is not yet supported")
	}

	// Map from feature ID (without version) to resolved feature.
	byID := make(map[string]*ResolvedFeature, len(features))
	for i := range features {
		byID[features[i].Meta.ID] = &features[i]
	}

	// Check for dependsOn referencing features outside the set.
	for i := range features {
		for depID := range features[i].Meta.DependsOn {
			if _, ok := byID[depID]; !ok {
				return nil, fmt.Errorf("feature %q dependsOn %q which is not in the feature set", features[i].Meta.ID, depID)
			}
		}
	}

	// Build graph from installsAfter.
	// Edge A -> B means A must be installed before B because B is in A's installsAfter.
	adj := make(map[string][]string) // feature ID -> list of IDs that come AFTER it
	inDegree := make(map[string]int)
	for id := range byID {
		inDegree[id] = 0
	}
	for i := range features {
		id := features[i].Meta.ID
		for _, after := range features[i].Meta.InstallsAfter {
			// Normalize the after ID: strip version suffix if present.
			afterBase := stripVersion(after)
			if _, ok := byID[afterBase]; !ok {
				continue // soft dep not in set, ignore
			}
			adj[afterBase] = append(adj[afterBase], id)
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
