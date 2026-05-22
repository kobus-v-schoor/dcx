package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

// routeMatch contains the result of matching a REST API path or GraphQL query
// to a semantic action name and repository.
type routeMatch struct {
	toolName string
	owner    string
	repo     string
}

// route defines a pattern → tool name mapping.
type route struct {
	pattern  *regexp.Regexp
	toolName string
	// extract extracts owner/repo from regex submatches.
	extract func(matches []string) (owner, repo string)
}

// repoExtract builds the standard owner/repo from the first two submatches.
func repoExtract(matches []string) (string, string) {
	return matches[1], matches[2]
}

// Routes is the ordered list of REST URL patterns mapped to tool names.
// Patterns are tried in order; first match wins. Logic is ported from the
// gh-aw-mcpg gateway router.
var routes = []route{
	// Issues
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues/\d+/comments$`), toolName: "issue_read", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues/\d+/labels$`), toolName: "issue_read", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues/\d+$`), toolName: "issue_read", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues$`), toolName: "list_issues", extract: repoExtract},

	// Pull Requests
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/pulls/\d+/files$`), toolName: "pull_request_read", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/pulls/\d+/reviews$`), toolName: "pull_request_read", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/pulls/\d+/comments$`), toolName: "pull_request_read", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/pulls/\d+$`), toolName: "pull_request_read", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/pulls$`), toolName: "list_pull_requests", extract: repoExtract},

	// Commits
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/commits/[^/]+$`), toolName: "get_commit", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/commits$`), toolName: "list_commits", extract: repoExtract},

	// Branches and Tags
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/branches$`), toolName: "list_branches", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/ref/tags/.+$`), toolName: "get_tag", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/tags$`), toolName: "list_tags", extract: repoExtract},

	// Releases
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/releases/latest$`), toolName: "get_latest_release", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/releases/tags/.+$`), toolName: "get_release_by_tag", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/releases$`), toolName: "list_releases", extract: repoExtract},

	// Contents
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/contents/.+$`), toolName: "get_file_contents", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/trees/.+$`), toolName: "get_file_contents", extract: repoExtract},

	// Labels
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/labels/.+$`), toolName: "get_label", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/labels$`), toolName: "list_labels", extract: repoExtract},

	// Actions (Workflows)
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/workflows$`), toolName: "actions_list", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/workflows/[^/]+/runs$`), toolName: "actions_list", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/workflows/[^/]+$`), toolName: "actions_get", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/runs/\d+/attempts/\d+/jobs$`), toolName: "actions_list", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/runs/\d+/attempts/\d+/logs$`), toolName: "get_job_logs", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/runs/\d+/logs$`), toolName: "get_job_logs", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/runs/\d+/artifacts$`), toolName: "actions_list", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/runs/\d+$`), toolName: "actions_get", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/runs$`), toolName: "actions_list", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/jobs/\d+$`), toolName: "actions_get", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/artifacts$`), toolName: "actions_list", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/caches$`), toolName: "actions_list", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/secrets$`), toolName: "actions_list", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/variables(?:/[^/]+)?$`), toolName: "actions_list", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/environments/[^/]+/(?:secrets|variables)$`), toolName: "actions_list", extract: repoExtract},

	// Notifications
	{pattern: regexp.MustCompile(`^/notifications$`), toolName: "list_notifications", extract: func(_ []string) (string, string) { return "", "" }},

	// User API
	{pattern: regexp.MustCompile(`^/user$`), toolName: "get_me", extract: func(_ []string) (string, string) { return "", "" }},
	{pattern: regexp.MustCompile(`^/user/(?:keys|ssh_signing_keys|gpg_keys)$`), toolName: "get_me", extract: func(_ []string) (string, string) { return "", "" }},

	// Org-scoped Actions
	{pattern: regexp.MustCompile(`^/orgs/([^/]+)/actions/(?:secrets|variables)(?:/[^/]+)?$`), toolName: "actions_list", extract: func(m []string) (string, string) { return m[1], "" }},

	// Discussions
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/discussions$`), toolName: "list_discussions", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/discussions/\d+/comments$`), toolName: "get_discussion_comments", extract: repoExtract},
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/discussions/\d+$`), toolName: "list_discussions", extract: repoExtract},

	// Check runs/suites
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/commits/[^/]+/check-(?:runs|suites)$`), toolName: "pull_request_read", extract: repoExtract},

	// Search APIs
	{pattern: regexp.MustCompile(`^/search/code$`), toolName: "search_code", extract: func(_ []string) (string, string) { return "", "" }},
	{pattern: regexp.MustCompile(`^/search/issues$`), toolName: "search_issues", extract: func(_ []string) (string, string) { return "", "" }},
	{pattern: regexp.MustCompile(`^/search/repositories$`), toolName: "search_repositories", extract: func(_ []string) (string, string) { return "", "" }},

	// Generic repo-scoped fallback (must be last)
	{pattern: regexp.MustCompile(`^/repos/([^/]+)/([^/]+)(?:/.*)?$`), toolName: "get_file_contents", extract: repoExtract},
}

// graphqlPattern maps GraphQL query content to tool names.
type graphqlPattern struct {
	queryPattern *regexp.Regexp
	toolName     string
}

var graphqlPatterns = []graphqlPattern{
	{queryPattern: regexp.MustCompile(`(?i)__type\s*\(`), toolName: "graphql_introspection"},
	{queryPattern: regexp.MustCompile(`(?i)__schema\b`), toolName: "graphql_introspection"},
	{queryPattern: regexp.MustCompile(`(?i)repository\s*\([^)]*\)\s*\{[^}]*\bissue\s*\(`), toolName: "issue_read"},
	{queryPattern: regexp.MustCompile(`(?i)repository\s*\([^)]*\)\s*\{[^}]*\bissues\s*[\({]`), toolName: "list_issues"},
	{queryPattern: regexp.MustCompile(`(?i)repository\s*\([^)]*\)\s*\{[^}]*\bpullRequest\s*\(`), toolName: "pull_request_read"},
	{queryPattern: regexp.MustCompile(`(?i)repository\s*\([^)]*\)\s*\{[^}]*\bpullRequests\s*[\({]`), toolName: "list_pull_requests"},
	{queryPattern: regexp.MustCompile(`(?i)\bhistory\s*[\({]`), toolName: "list_commits"},
	{queryPattern: regexp.MustCompile(`(?i)repository\s*\([^)]*\)\s*\{[^}]*\bdiscussion\s*\(`), toolName: "list_discussions"},
	{queryPattern: regexp.MustCompile(`(?i)repository\s*\([^)]*\)\s*\{[^}]*\bdiscussions\s*[\({]`), toolName: "list_discussions"},
	{queryPattern: regexp.MustCompile(`(?i)repository\s*\([^)]*\)\s*\{[^}]*\bdiscussionCategories\s*[\({]`), toolName: "list_discussion_categories"},
	{queryPattern: regexp.MustCompile(`(?i)\bsearch\s*\(`), toolName: "search_issues"},
	{queryPattern: regexp.MustCompile(`(?i)projectV2`), toolName: "list_projects"},
	{queryPattern: regexp.MustCompile(`(?i)\bviewer\s*\{`), toolName: "get_me"},
	{queryPattern: regexp.MustCompile(`(?i)\borganization\s*\(`), toolName: "search_orgs"},
	{queryPattern: regexp.MustCompile(`(?i)\brepository\s*\(`), toolName: "get_file_contents"},
}

var (
	varOwnerPattern  = regexp.MustCompile(`(?i)"owner"\s*:\s*"([^"]+)"`)
	varRepoPattern   = regexp.MustCompile(`(?i)"(?:name|repo)"\s*:\s*"([^"]+)"`)
	queryRepoPattern = regexp.MustCompile(`(?i)repository\s*\(\s*owner\s*:\s*(?:"([^"]+)"|\$\w+)\s*,?\s*name\s*:\s*(?:"([^"]+)"|\$\w+)`)
)

// graphqlRequest represents a parsed GraphQL request body.
type graphqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// stripGHHostPrefix removes the /api/v3 prefix that gh adds when using GH_HOST.
func stripGHHostPrefix(path string) string {
	const prefix = "/api/v3"
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}

// isGraphQLPath returns true if the request path targets the GraphQL endpoint.
func isGraphQLPath(path string) bool {
	cleaned := strings.TrimSuffix(path, "/")
	return cleaned == "/graphql" || cleaned == "/api/v3/graphql" || cleaned == "/api/graphql"
}

// matchREST matches a REST API path to a semantic tool name and repository.
func matchREST(path string) *routeMatch {
	path = stripGHHostPrefix(path)
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}

	for _, r := range routes {
		matches := r.pattern.FindStringSubmatch(path)
		if matches != nil {
			owner, repo := r.extract(matches)
			return &routeMatch{toolName: r.toolName, owner: owner, repo: repo}
		}
	}
	return nil
}

// matchGraphQL matches a GraphQL request body to a tool name and repository.
func matchGraphQL(body []byte) *routeMatch {
	var gql graphqlRequest
	if err := json.Unmarshal(body, &gql); err != nil {
		return nil
	}
	if gql.Query == "" {
		return nil
	}

	var toolName string
	for _, p := range graphqlPatterns {
		if p.queryPattern.MatchString(gql.Query) {
			toolName = p.toolName
			break
		}
	}
	if toolName == "" {
		return nil
	}

	owner, repo := extractOwnerRepo(gql.Variables, gql.Query)
	return &routeMatch{toolName: toolName, owner: owner, repo: repo}
}

// extractOwnerRepo extracts owner and repo from GraphQL variables and query text.
func extractOwnerRepo(variables map[string]interface{}, query string) (string, string) {
	var owner, repo string

	if variables != nil {
		if v, ok := variables["owner"].(string); ok {
			owner = v
		}
		if v, ok := variables["name"].(string); ok {
			repo = v
		}
		if v, ok := variables["repo"].(string); ok && repo == "" {
			repo = v
		}
	}

	if owner == "" || repo == "" {
		if m := queryRepoPattern.FindStringSubmatch(query); m != nil {
			if m[1] != "" && owner == "" {
				owner = m[1]
			}
			if m[2] != "" && repo == "" {
				repo = m[2]
			}
		}
	}

	if owner == "" {
		if m := varOwnerPattern.FindStringSubmatch(query); m != nil {
			owner = m[1]
		}
	}
	if repo == "" {
		if m := varRepoPattern.FindStringSubmatch(query); m != nil {
			repo = m[1]
		}
	}

	return owner, repo
}

// filterRequest checks whether the intercepted request is permitted by the
// configured GitHub proxy permissions. It returns a non-nil *http.Response
// when the request should be blocked at the proxy level.
func filterRequest(req *http.Request, cfg *config.Config) (*http.Response, error) {
	perms := cfg.Proxy.GitHub.Permissions
	if len(perms) == 0 {
		// No permissions configured — allow all (backward-compatible).
		return nil, nil
	}

	var match *routeMatch
	path := req.URL.Path

	if isGraphQLPath(path) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("reading GraphQL body: %w", err)
		}
		// Restore body so downstream handlers can re-read it.
		req.Body = io.NopCloser(bytes.NewReader(body))
		match = matchGraphQL(body)
	} else {
		match = matchREST(path)
	}

	if match == nil {
		// Unknown route — deny when permissions are configured (fail closed).
		return blockedResponse(req, "unknown GitHub API endpoint"), nil
	}

	repoKey := match.owner + "/" + match.repo
	if repoKey == "/" {
		repoKey = ""
	}

	if !isActionAllowed(match.toolName, repoKey, perms) {
		return blockedResponse(req, fmt.Sprintf("action %q on %q is not permitted", match.toolName, repoKey)), nil
	}

	return nil, nil
}

// isActionAllowed returns true if any permission entry matching repo allows
// the given action.
func isActionAllowed(action, repoKey string, perms []config.GitHubPermission) bool {
	for _, p := range perms {
		if !repoMatches(p.Repo, repoKey) {
			continue
		}
		if len(p.Actions) == 0 {
			// Empty actions list means all actions allowed for this repo.
			return true
		}
		for _, a := range p.Actions {
			if a == action {
				return true
			}
		}
	}
	return false
}

// repoMatches returns true if the permission repo pattern matches the
// request repo key. A pattern of "*" matches everything; otherwise an
// exact match is required.
func repoMatches(pattern, repoKey string) bool {
	if pattern == "*" {
		return true
	}
	return pattern == repoKey
}

// blockedResponse returns a synthetic 403 Forbidden response.
func blockedResponse(req *http.Request, reason string) *http.Response {
	body := []byte(fmt.Sprintf(`{"message":"dcx: GitHub API request blocked by permission policy: %s"}`, reason))
	return &http.Response{
		StatusCode: http.StatusForbidden,
		Status:     http.StatusText(http.StatusForbidden),
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}
