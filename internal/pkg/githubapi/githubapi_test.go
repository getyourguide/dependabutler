package githubapi

import (
	"net/http"
	"net/http/httptest"
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
