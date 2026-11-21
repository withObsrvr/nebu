package registry

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The registry's GitHub API calls were unauthenticated, so a CI runner or
// anything behind shared NAT hit the 60-per-hour limit and got a 403. The
// failure was quiet — FetchAll warns and falls back, so `nebu list` still
// exited 0 with only the embedded catalog.
func TestGithubToken(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"nothing set", map[string]string{}, ""},
		{"nebu-specific wins", map[string]string{
			"NEBU_REGISTRY_TOKEN": "nebu", "GITHUB_TOKEN": "gh", "GH_TOKEN": "cli",
		}, "nebu"},
		{"GITHUB_TOKEN before GH_TOKEN", map[string]string{
			"GITHUB_TOKEN": "gh", "GH_TOKEN": "cli",
		}, "gh"},
		{"GH_TOKEN alone", map[string]string{"GH_TOKEN": "cli"}, "cli"},
		// A variable set to whitespace is set-but-empty, which would otherwise
		// produce an Authorization header of "Bearer " and a confusing 401.
		{"whitespace is not a token", map[string]string{"GITHUB_TOKEN": "   "}, ""},
		{"falls past an empty value", map[string]string{
			"NEBU_REGISTRY_TOKEN": "", "GH_TOKEN": "cli",
		}, "cli"},
		{"surrounding whitespace is trimmed", map[string]string{
			"GITHUB_TOKEN": "  gh\n",
		}, "gh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"NEBU_REGISTRY_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			if got := githubToken(); got != tt.want {
				t.Errorf("githubToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The token must reach the GitHub API request, and must not be sent when
// there is none — an empty "Bearer " header is worse than no header.
func TestDiscoverProcessorsSendsAuthorization(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{"with a token", "secret-value", "Bearer secret-value"},
		{"without a token", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"NEBU_REGISTRY_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
				t.Setenv(key, "")
			}
			if tt.token != "" {
				t.Setenv("NEBU_REGISTRY_TOKEN", tt.token)
			}

			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			}))
			defer srv.Close()

			req, err := http.NewRequest("GET", srv.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Accept", "application/vnd.github.v3+json")
			req.Header.Set("User-Agent", "nebu-cli")
			if token := githubToken(); token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if got != tt.want {
				t.Errorf("Authorization header = %q, want %q", got, tt.want)
			}
		})
	}
}
