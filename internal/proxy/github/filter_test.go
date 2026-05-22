package github

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kobus-v-schoor/dcx/internal/config"
)

func makeConfig(perms []config.GitHubPermission) *config.Config {
	return &config.Config{
		Proxy: config.ProxyConfig{
			GitHub: config.GitHubProxyConfig{
				Permissions: perms,
			},
		},
	}
}

// TestFilterRequestNoPermissions tests that all requests are allowed when
// no permissions are configured.
func TestFilterRequestNoPermissions(t *testing.T) {
	cfg := makeConfig(nil)
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/foo/bar/issues", nil)
	resp, err := filterRequest(req, cfg)
	if err != nil {
		t.Fatalf("filterRequest error: %v", err)
	}
	if resp != nil {
		t.Fatal("expected nil response when no permissions configured")
	}
}

// TestFilterRequestAllowedRepoAction tests that a matching repo and action
// is permitted.
func TestFilterRequestAllowedRepoAction(t *testing.T) {
	cfg := makeConfig([]config.GitHubPermission{
		{Repo: "foo/bar", Actions: []string{"issue_read"}},
	})
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/foo/bar/issues/1", nil)
	resp, err := filterRequest(req, cfg)
	if err != nil {
		t.Fatalf("filterRequest error: %v", err)
	}
	if resp != nil {
		t.Fatal("expected nil response for allowed action")
	}
}

// TestFilterRequestBlockedRepoAction tests that a non-matching action is
// blocked.
func TestFilterRequestBlockedRepoAction(t *testing.T) {
	cfg := makeConfig([]config.GitHubPermission{
		{Repo: "foo/bar", Actions: []string{"issue_read"}},
	})
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/foo/bar/pulls/1", nil)
	resp, err := filterRequest(req, cfg)
	if err != nil {
		t.Fatalf("filterRequest error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected blocked response for disallowed action")
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

// TestFilterRequestWildcardRepo tests that "*" matches all repos.
func TestFilterRequestWildcardRepo(t *testing.T) {
	cfg := makeConfig([]config.GitHubPermission{
		{Repo: "*", Actions: []string{"get_me"}},
	})
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	resp, err := filterRequest(req, cfg)
	if err != nil {
		t.Fatalf("filterRequest error: %v", err)
	}
	if resp != nil {
		t.Fatal("expected nil response for wildcard-allowed action")
	}
}

// TestFilterRequestWildcardRepoBlockedAction tests that a wildcard repo
// still requires the action to be in the allowed list.
func TestFilterRequestWildcardRepoBlockedAction(t *testing.T) {
	cfg := makeConfig([]config.GitHubPermission{
		{Repo: "*", Actions: []string{"get_me"}},
	})
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/foo/bar/issues", nil)
	resp, err := filterRequest(req, cfg)
	if err != nil {
		t.Fatalf("filterRequest error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected blocked response for disallowed action on wildcard repo")
	}
}

// TestFilterRequestEmptyActionsAllowsAll tests that an empty Actions list
// allows all actions for the matched repo.
func TestFilterRequestEmptyActionsAllowsAll(t *testing.T) {
	cfg := makeConfig([]config.GitHubPermission{
		{Repo: "foo/bar", Actions: nil},
	})
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/foo/bar/pulls/1", nil)
	resp, err := filterRequest(req, cfg)
	if err != nil {
		t.Fatalf("filterRequest error: %v", err)
	}
	if resp != nil {
		t.Fatal("expected nil response when actions list is empty")
	}
}

// TestFilterRequestUnknownRoute tests that unknown routes are blocked when
// permissions are configured.
func TestFilterRequestUnknownRoute(t *testing.T) {
	cfg := makeConfig([]config.GitHubPermission{
		{Repo: "foo/bar", Actions: []string{"issue_read"}},
	})
	req, _ := http.NewRequest("GET", "https://api.github.com/unknown/endpoint", nil)
	resp, err := filterRequest(req, cfg)
	if err != nil {
		t.Fatalf("filterRequest error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected blocked response for unknown route")
	}
}

// TestFilterRequestGraphQLAllowed tests that an allowed GraphQL query is
// permitted.
func TestFilterRequestGraphQLAllowed(t *testing.T) {
	cfg := makeConfig([]config.GitHubPermission{
		{Repo: "foo/bar", Actions: []string{"issue_read"}},
	})
	body := `{"query":"query { repository(owner:\"foo\",name:\"bar\") { issue(number:1) { title } } }"}`
	req, _ := http.NewRequest("POST", "https://api.github.com/graphql", strings.NewReader(body))
	resp, err := filterRequest(req, cfg)
	if err != nil {
		t.Fatalf("filterRequest error: %v", err)
	}
	if resp != nil {
		t.Fatal("expected nil response for allowed GraphQL query")
	}
	// Verify the body was restored so downstream can read it.
	restored, _ := io.ReadAll(req.Body)
	if string(restored) != body {
		t.Errorf("request body was not restored, got %q", string(restored))
	}
}

// TestFilterRequestGraphQLBlocked tests that a disallowed GraphQL query is
// blocked.
func TestFilterRequestGraphQLBlocked(t *testing.T) {
	cfg := makeConfig([]config.GitHubPermission{
		{Repo: "foo/bar", Actions: []string{"issue_read"}},
	})
	body := `{"query":"query { repository(owner:\"foo\",name:\"bar\") { pullRequest(number:1) { title } } }"}`
	req, _ := http.NewRequest("POST", "https://api.github.com/graphql", strings.NewReader(body))
	resp, err := filterRequest(req, cfg)
	if err != nil {
		t.Fatalf("filterRequest error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected blocked response for disallowed GraphQL query")
	}
}

// TestFilterRequestGHHostPrefix tests that /api/v3 prefix is stripped before
// route matching.
func TestFilterRequestGHHostPrefix(t *testing.T) {
	cfg := makeConfig([]config.GitHubPermission{
		{Repo: "foo/bar", Actions: []string{"issue_read"}},
	})
	req, _ := http.NewRequest("GET", "https://github.com/api/v3/repos/foo/bar/issues/1", nil)
	resp, err := filterRequest(req, cfg)
	if err != nil {
		t.Fatalf("filterRequest error: %v", err)
	}
	if resp != nil {
		t.Fatal("expected nil response after stripping GH_HOST prefix")
	}
}

// TestMatchRESTRepoScoped tests REST route matching for a repo-scoped path.
func TestMatchRESTRepoScoped(t *testing.T) {
	m := matchREST("/repos/acme/corp/issues/42")
	if m == nil {
		t.Fatal("expected match")
	}
	if m.toolName != "issue_read" {
		t.Errorf("toolName = %q, want issue_read", m.toolName)
	}
	if m.owner != "acme" {
		t.Errorf("owner = %q, want acme", m.owner)
	}
	if m.repo != "corp" {
		t.Errorf("repo = %q, want corp", m.repo)
	}
}

// TestMatchRESTUserEndpoint tests REST route matching for the /user endpoint.
func TestMatchRESTUserEndpoint(t *testing.T) {
	m := matchREST("/user")
	if m == nil {
		t.Fatal("expected match")
	}
	if m.toolName != "get_me" {
		t.Errorf("toolName = %q, want get_me", m.toolName)
	}
}

// TestMatchRESTNoMatch tests that unmatched paths return nil.
func TestMatchRESTNoMatch(t *testing.T) {
	m := matchREST("/nonexistent/path")
	if m != nil {
		t.Fatal("expected no match")
	}
}

// TestMatchGraphQLWithVariables tests GraphQL matching using variables.
func TestMatchGraphQLWithVariables(t *testing.T) {
	body := []byte(`{"query":"query GetIssue($owner:String!,$name:String!){repository(owner:$owner,name:$name){issue(number:1){title}}}","variables":{"owner":"foo","name":"bar"}}`)
	m := matchGraphQL(body)
	if m == nil {
		t.Fatal("expected match")
	}
	if m.toolName != "issue_read" {
		t.Errorf("toolName = %q, want issue_read", m.toolName)
	}
	if m.owner != "foo" {
		t.Errorf("owner = %q, want foo", m.owner)
	}
	if m.repo != "bar" {
		t.Errorf("repo = %q, want bar", m.repo)
	}
}

// TestMatchGraphQLInlineRepo tests GraphQL matching with inline owner/repo.
func TestMatchGraphQLInlineRepo(t *testing.T) {
	body := []byte(`{"query":"query { repository(owner:\"acme\", name: \"corp\") { pullRequest(number: 1) { title } } }"}`)
	m := matchGraphQL(body)
	if m == nil {
		t.Fatal("expected match")
	}
	if m.toolName != "pull_request_read" {
		t.Errorf("toolName = %q, want pull_request_read", m.toolName)
	}
	if m.owner != "acme" {
		t.Errorf("owner = %q, want acme", m.owner)
	}
	if m.repo != "corp" {
		t.Errorf("repo = %q, want corp", m.repo)
	}
}

// TestIsGraphQLPath tests GraphQL endpoint detection.
func TestIsGraphQLPath(t *testing.T) {
	for _, path := range []string{"/graphql", "/api/v3/graphql", "/api/graphql"} {
		if !isGraphQLPath(path) {
			t.Errorf("isGraphQLPath(%q) = false, want true", path)
		}
	}
	if isGraphQLPath("/repos/foo/bar/issues") {
		t.Error("isGraphQLPath(/repos/foo/bar/issues) = true, want false")
	}
}
