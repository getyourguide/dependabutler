package githubapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/getyourguide/dependabutler/internal/pkg/config"
	"github.com/google/go-github/v90/github"
)

func TestCreatePRDescriptionListsEnforcedSettings(t *testing.T) {
	changeInfo := config.ChangeInfo{EnforcedUpdates: []config.EnforcedUpdateInfo{
		{Type: "npm", Directory: "/web", Fields: []string{"schedule", "cooldown"}},
	}}

	description := CreatePRDescription(changeInfo, "Ask for an exception in the exceptions file.")

	for _, expected := range []string{"| npm | `/web` | schedule, cooldown |", "Ask for an exception in the exceptions file."} {
		if !strings.Contains(description, expected) {
			t.Errorf("description does not contain %q:\n%v", expected, description)
		}
	}
}

func TestCreatePRDescriptionWithoutEnforcementHasNoNote(t *testing.T) {
	changeInfo := config.ChangeInfo{NewUpdates: []config.UpdateInfo{{Type: "gomod", Directory: "/", File: "go.mod"}}}

	description := CreatePRDescription(changeInfo, "Ask for an exception in the exceptions file.")

	if strings.Contains(description, "exception") {
		t.Errorf("description of a PR without enforcement contains the enforce note:\n%v", description)
	}
}

func TestCreatePRDescriptionWithEnforcementDoesNotInviteChanges(t *testing.T) {
	changeInfo := config.ChangeInfo{EnforcedUpdates: []config.EnforcedUpdateInfo{{Type: "npm", Directory: "/", Fields: []string{"schedule"}}}}

	description := CreatePRDescription(changeInfo, "")

	if strings.Contains(description, "change if required") {
		t.Errorf("description of an enforcement PR invites changing the settings:\n%v", description)
	}
}

func TestCheckDirectoryExistsTellsMissingFromFailed(t *testing.T) {
	for _, tt := range []struct {
		name    string
		status  int
		body    string
		exists  bool
		failure bool
	}{
		{"directory", http.StatusOK, `[{"type": "file", "name": "Dockerfile", "path": "app/Dockerfile"}]`, true, false},
		{"missing", http.StatusNotFound, `{"message": "Not Found"}`, false, false},
		{"server error", http.StatusBadGateway, `{"message": "Bad Gateway"}`, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			baseURL := server.URL + "/"
			gh, err := github.NewClient(github.WithURLs(&baseURL, &baseURL))
			if err != nil {
				t.Fatal(err)
			}

			exists, err := (&Client{gh: gh}).CheckDirectoryExists("acme", "app", "app", "")

			if exists != tt.exists || (err != nil) != tt.failure {
				t.Errorf("CheckDirectoryExists() = %v, %v, expected %v and an error: %v", exists, err, tt.exists, tt.failure)
			}
		})
	}
}

// stalePRServer serves an open dependabutler PR number 7, which reports total commits and lists those authored by the
// given emails. With no emails, the repo has no open dependabutler PR. It records the requests that change the PR.
func stalePRServer(t *testing.T, emails []string, total int, changes *[]string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/issues":
			if len(emails) == 0 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"number": 7, "pull_request": {"url": "x"}}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/pulls/7":
			_, _ = w.Write([]byte(`{"number": 7, "html_url": "https://github.com/acme/app/pull/7", "commits": ` + strconv.Itoa(total) + `}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/app/pulls/7/commits":
			commits := make([]string, 0, len(emails))
			for _, email := range emails {
				commits = append(commits, `{"commit": {"author": {"email": "`+email+`"}}}`)
			}
			_, _ = w.Write([]byte("[" + strings.Join(commits, ",") + "]"))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/acme/app/issues/7/comments":
			*changes = append(*changes, "comment")
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/acme/app/pulls/7":
			body, _ := io.ReadAll(r.Body)
			*changes = append(*changes, "edit "+strings.TrimSpace(string(body)))
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request %v %v", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	baseURL := server.URL + "/"
	gh, err := github.NewClient(github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatal(err)
	}
	return &Client{gh: gh}
}

func TestGetStalePullRequest(t *testing.T) {
	for _, tt := range []struct {
		name   string
		emails []string
		total  int
		stale  bool
	}{
		{"no open PR", nil, 0, false},
		{"only dependabutler commits", []string{"dependabutler@example.com", "Dependabutler@Example.com"}, 2, true},
		{"commit by someone else", []string{"dependabutler@example.com", "dev@example.com"}, 2, false},
		{"more commits than listed", []string{"dependabutler@example.com"}, 251, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var changes []string
			client := stalePRServer(t, tt.emails, tt.total, &changes)

			pr, err := client.GetStalePullRequest("acme", "app", "dependabutler@example.com")

			if err != nil || (pr != nil) != tt.stale {
				t.Errorf("GetStalePullRequest() = %v, %v, expected a stale PR: %v", pr, err, tt.stale)
			}
			if len(changes) > 0 {
				t.Errorf("GetStalePullRequest() changed the PR: %v", changes)
			}
		})
	}
}

func TestCloseStalePullRequestCommentsThenCloses(t *testing.T) {
	var changes []string
	client := stalePRServer(t, []string{"dependabutler@example.com"}, 1, &changes)

	err := client.CloseStalePullRequest("acme", "app", &github.PullRequest{Number: github.Ptr(7)}, 0)

	expected := []string{"comment", `edit {"state":"closed"}`}
	if err != nil || !reflect.DeepEqual(changes, expected) {
		t.Errorf("CloseStalePullRequest() = %v with changes %q, expected no error and %q", err, changes, expected)
	}
}
