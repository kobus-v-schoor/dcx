package docker

import (
	"regexp"
	"strings"
)

var slugRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// WorkspaceNameSlug sanitises a workspace folder name so it can be used as
// part of a Docker image tag. Non-alphanumeric characters are replaced
// with hyphens, the result is lowercased, and truncated to 20 characters.
func WorkspaceNameSlug(name string) string {
	slug := slugRegex.ReplaceAllString(name, "-")
	slug = strings.ToLower(strings.Trim(slug, "-"))
	if slug == "" {
		slug = "workspace"
	}
	if len(slug) > 20 {
		slug = slug[:20]
	}
	return slug
}
