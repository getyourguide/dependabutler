package githubapi

import (
	"strings"
	"testing"

	"github.com/getyourguide/dependabutler/internal/pkg/config"
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
