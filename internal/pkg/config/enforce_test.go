package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
)

const enforceConfig = `
update-defaults:
  schedule:
    interval: weekly
  open-pull-requests-limit: 5
  cooldown:
    default-days: 3
enforce:
  fields: [schedule, cooldown, open-pull-requests-limit]
  exceptions-file: exceptions.yml
  repos-file: rollout.txt
`

func TestParseToolConfigReadsEnforce(t *testing.T) {
	toolConfig, err := ParseToolConfig([]byte(enforceConfig))
	if err != nil {
		t.Fatalf("ParseToolConfig() failed: %v", err)
	}

	expected := &Enforce{
		Fields:         []string{"schedule", "cooldown", "open-pull-requests-limit"},
		ExceptionsFile: "exceptions.yml",
		ReposFile:      "rollout.txt",
	}

	if !reflect.DeepEqual(toolConfig.Enforce, expected) {
		t.Errorf("Enforce\nExpected: %+v\nGot:      %+v", expected, toolConfig.Enforce)
	}
}

func TestParseToolConfigRejectsInvalidEnforce(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config string
		error  string
	}{
		{"no fields", "enforce:\n  fields: []\n", "at least one field"},
		{"unknown field", "enforce:\n  fields: [labels]\n", `"labels"`},
		{"repeated field", "enforce:\n  fields: [cooldown, cooldown]\n", `"cooldown" more than once`},
		{"schedule without a schedule to enforce", "enforce:\n  fields: [schedule]\n", "schedule"},
		{"cooldown without a cooldown to enforce", "enforce:\n  fields: [cooldown]\n", "cooldown"},
		{"limit without a limit to enforce", "enforce:\n  fields: [open-pull-requests-limit]\n", "open-pull-requests-limit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseToolConfig([]byte(tt.config))

			if err == nil || !strings.Contains(err.Error(), tt.error) {
				t.Errorf("ParseToolConfig() error = %v, expected it to contain %q", err, tt.error)
			}
		})
	}
}

func TestParseToolConfigEnforcesScheduleFromSlots(t *testing.T) {
	config := "enforce:\n  fields: [schedule]\nschedule-slots:\n  windows: [{name: early, hours: [2], timezone: UTC}]\n"

	if _, err := ParseToolConfig([]byte(config)); err != nil {
		t.Errorf("ParseToolConfig() failed: %v", err)
	}
}

func writeEnforceFiles(t *testing.T, exceptions string, rollout string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "exceptions.yml"), []byte(exceptions), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rollout.txt"), []byte(rollout), 0o600); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestFieldsForSkipsExceptionsAndReposOutsideTheRollout(t *testing.T) {
	dir := writeEnforceFiles(t, `
- repo: Monorepo
  fields: [cooldown]
  reason: needs a longer cooldown
  owner: team-a
`, "monorepo\nplain-repo\n")
	enforce := Enforce{
		Fields:         []string{"schedule", "cooldown", "open-pull-requests-limit"},
		ExceptionsFile: "exceptions.yml",
		ReposFile:      "rollout.txt",
	}

	if err := enforce.LoadFiles(dir); err != nil {
		t.Fatalf("LoadFiles() failed: %v", err)
	}

	for _, tt := range []struct {
		repo   string
		fields []string
	}{
		{"monorepo", []string{"schedule", "open-pull-requests-limit"}},
		{"Plain-Repo", []string{"schedule", "cooldown", "open-pull-requests-limit"}},
		{"later-wave", nil},
	} {
		if got := enforce.FieldsFor(tt.repo); !reflect.DeepEqual(got, tt.fields) {
			t.Errorf("FieldsFor(%q) = %v, expected %v", tt.repo, got, tt.fields)
		}
	}
}

func TestFieldsForWithoutRolloutListEnforcesEveryRepo(t *testing.T) {
	enforce := Enforce{Fields: []string{"cooldown"}}

	if err := enforce.LoadFiles(t.TempDir()); err != nil {
		t.Fatalf("LoadFiles() failed: %v", err)
	}

	if got := enforce.FieldsFor("any-repo"); !reflect.DeepEqual(got, []string{"cooldown"}) {
		t.Errorf("FieldsFor() = %v, expected [cooldown]", got)
	}
}

func TestFieldsForWithoutEnforce(t *testing.T) {
	var enforce *Enforce

	if got := enforce.FieldsFor("any-repo"); got != nil {
		t.Errorf("FieldsFor() = %v, expected nil", got)
	}
}

func TestLoadFilesRejectsInvalidExceptions(t *testing.T) {
	for _, tt := range []struct {
		name       string
		exceptions string
		error      string
	}{
		{"no repo", "- {fields: [cooldown], reason: r, owner: o}", "repo"},
		{"no fields", "- {repo: a, fields: [], reason: r, owner: o}", "fields"},
		{"unknown field", "- {repo: a, fields: [labels], reason: r, owner: o}", `"labels"`},
		{"no reason", "- {repo: a, fields: [cooldown], owner: o}", "reason"},
		{"no owner", "- {repo: a, fields: [cooldown], reason: r}", "owner"},
		{"not a list", "repo: a", "exceptions.yml"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeEnforceFiles(t, tt.exceptions, "")
			enforce := Enforce{Fields: []string{"cooldown"}, ExceptionsFile: "exceptions.yml"}

			err := enforce.LoadFiles(dir)

			if err == nil || !strings.Contains(err.Error(), tt.error) {
				t.Errorf("LoadFiles() error = %v, expected it to contain %q", err, tt.error)
			}
		})
	}
}

func TestLoadFilesFailsOnMissingFiles(t *testing.T) {
	for _, enforce := range []Enforce{
		{Fields: []string{"cooldown"}, ExceptionsFile: "missing.yml"},
		{Fields: []string{"cooldown"}, ReposFile: "missing.txt"},
	} {
		err := enforce.LoadFiles(t.TempDir())

		if err == nil || !strings.Contains(err.Error(), "missing") {
			t.Errorf("LoadFiles(%+v) error = %v, expected it to name the missing file", enforce, err)
		}
	}
}

var allEnforcedFields = []string{"schedule", "cooldown", "open-pull-requests-limit"}

func enforceToolConfig() ToolConfig {
	return ToolConfig{
		UpdateDefaults: UpdateDefaults{
			Schedule:              Schedule{Interval: "weekly", Day: "sunday"},
			OpenPullRequestsLimit: util.Ptr(5),
			Cooldown:              Cooldown{DefaultDays: 3, SemverMajorDays: 21},
		},
		UpdateOverrides: map[string]UpdateDefaults{
			"npm": {Cooldown: Cooldown{DefaultDays: 3, SemverMajorDays: 21, Exclude: []string{"@getyourguide*"}}},
		},
	}
}

func updateConfigWith(t *testing.T, dependabotYaml string, toolConfig ToolConfig, enforcedFields []string) (*DependabotConfig, ChangeInfo) {
	t.Helper()

	dependabotConfig, err := ParseDependabotConfig([]byte(dependabotYaml))
	if err != nil {
		t.Fatal(err)
	}

	slotSchedule := &Schedule{Interval: "cron", Cronjob: "0 3 * * 1,4", Timezone: "UTC"}
	directoryExists := func(string, CheckDirectoryExistsParameters) bool { return true }
	changeInfo := dependabotConfig.UpdateConfig(map[string]string{}, toolConfig, slotSchedule, enforcedFields,
		LoadFileContentDummy, LoadFileContentParameters{}, directoryExists, CheckDirectoryExistsParameters{})

	return dependabotConfig, changeInfo
}

const customNpmEntry = `
version: 2
updates:
  - package-ecosystem: npm
    directory: /
    target-branch: develop
    schedule:
      interval: daily
      time: "05:00"
    open-pull-requests-limit: 0
    cooldown:
      default-days: 7
      exclude: ["*getyourguide*"]
    commit-message:
      prefix: "[deps]"
    labels: [dependencies]
    assignees: [someone]
    allow:
      - dependency-type: direct
    ignore:
      - dependency-name: left-pad
`

func TestUpdateConfigEnforcesFieldsOnExistingEntries(t *testing.T) {
	dependabotConfig, changeInfo := updateConfigWith(t, customNpmEntry, enforceToolConfig(), allEnforcedFields)
	update := dependabotConfig.Updates[0]

	expectedSchedule := Schedule{Interval: "cron", Cronjob: "0 3 * * 1,4", Timezone: "UTC"}
	expectedCooldown := Cooldown{DefaultDays: 3, SemverMajorDays: 21, Exclude: []string{"@getyourguide*"}}

	if !reflect.DeepEqual(update.Schedule, expectedSchedule) {
		t.Errorf("schedule = %+v, expected %+v", update.Schedule, expectedSchedule)
	}
	if !reflect.DeepEqual(update.Cooldown, expectedCooldown) {
		t.Errorf("cooldown = %+v, expected %+v", update.Cooldown, expectedCooldown)
	}
	if update.OpenPullRequestsLimit == nil || *update.OpenPullRequestsLimit != 5 {
		t.Errorf("open-pull-requests-limit = %v, expected 5", update.OpenPullRequestsLimit)
	}

	expectedEnforced := []EnforcedUpdateInfo{{Type: "npm", Directory: "/", Fields: allEnforcedFields}}
	if !reflect.DeepEqual(changeInfo.EnforcedUpdates, expectedEnforced) {
		t.Errorf("EnforcedUpdates = %+v, expected %+v", changeInfo.EnforcedUpdates, expectedEnforced)
	}
}

func TestUpdateConfigEnforcementKeepsOtherKeys(t *testing.T) {
	expected, _ := updateConfigWith(t, customNpmEntry, enforceToolConfig(), nil)
	enforced, _ := updateConfigWith(t, customNpmEntry, enforceToolConfig(), allEnforcedFields)

	kept := enforced.Updates[0]
	kept.Schedule = expected.Updates[0].Schedule
	kept.Cooldown = expected.Updates[0].Cooldown
	kept.OpenPullRequestsLimit = expected.Updates[0].OpenPullRequestsLimit

	if !reflect.DeepEqual(kept, expected.Updates[0]) {
		t.Errorf("enforcement changed other keys\nExpected: %+v\nGot:      %+v", expected.Updates[0], kept)
	}
}

func TestUpdateConfigEnforcesOnlyTheListedFields(t *testing.T) {
	dependabotConfig, changeInfo := updateConfigWith(t, customNpmEntry, enforceToolConfig(), []string{"cooldown"})
	update := dependabotConfig.Updates[0]

	if update.Schedule.Interval != "daily" || *update.OpenPullRequestsLimit != 0 {
		t.Errorf("fields outside the list changed: schedule %+v, limit %v", update.Schedule, *update.OpenPullRequestsLimit)
	}
	if len(changeInfo.EnforcedUpdates) != 1 || !reflect.DeepEqual(changeInfo.EnforcedUpdates[0].Fields, []string{"cooldown"}) {
		t.Errorf("EnforcedUpdates = %+v, expected only cooldown", changeInfo.EnforcedUpdates)
	}
}

func TestUpdateConfigEnforcesNothingOnMatchingEntries(t *testing.T) {
	_, changeInfo := updateConfigWith(t, `
version: 2
updates:
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: cron
      cronjob: "0 3 * * 1,4"
      timezone: UTC
    open-pull-requests-limit: 5
    cooldown:
      semver-major-days: 21
      default-days: 3
`, enforceToolConfig(), allEnforcedFields)

	if len(changeInfo.EnforcedUpdates) != 0 || len(changeInfo.FixedUpdates) != 0 {
		t.Errorf("changes on a matching entry: enforced %+v, fixed %+v", changeInfo.EnforcedUpdates, changeInfo.FixedUpdates)
	}
}

func TestUpdateConfigEnforcementIsStableOnceWritten(t *testing.T) {
	toolConfig := enforceToolConfig()
	toolConfig.UpdateOverrides = nil
	toolConfig.UpdateDefaults.Cooldown.Include = []string{}
	toolConfig.UpdateDefaults.Cooldown.Exclude = []string{}

	enforced, _ := updateConfigWith(t, customNpmEntry, toolConfig, allEnforcedFields)
	_, changeInfo := updateConfigWith(t, string(enforced.ToYaml()), toolConfig, allEnforcedFields)

	if len(changeInfo.EnforcedUpdates) != 0 {
		t.Errorf("EnforcedUpdates = %+v on a file that was already enforced", changeInfo.EnforcedUpdates)
	}
}

func TestUpdateConfigListsTheDirectoriesOfEnforcedEntries(t *testing.T) {
	_, changeInfo := updateConfigWith(t, `
version: 2
updates:
  - package-ecosystem: docker
    directories: [/a, /b]
    schedule:
      interval: daily
`, enforceToolConfig(), []string{"schedule"})

	if len(changeInfo.EnforcedUpdates) != 1 || changeInfo.EnforcedUpdates[0].Directory != "/a, /b" {
		t.Errorf("EnforcedUpdates = %+v, expected the directories /a, /b", changeInfo.EnforcedUpdates)
	}
}

func TestUpdateConfigEnforcedCooldownIsNotFilledIn(t *testing.T) {
	toolConfig := enforceToolConfig()
	toolConfig.UpdateMissingCooldownSettings = util.Ptr(true)

	_, changeInfo := updateConfigWith(t, customNpmEntry, toolConfig, []string{"cooldown"})

	if len(changeInfo.FixedUpdates) != 0 {
		t.Errorf("FixedUpdates = %+v, expected the enforced cooldown to replace filling in missing fields", changeInfo.FixedUpdates)
	}
}

func TestOnlyEnforced(t *testing.T) {
	enforced := []EnforcedUpdateInfo{{Type: "npm", Directory: "/", Fields: []string{"cooldown"}}}

	for _, tt := range []struct {
		name       string
		changeInfo ChangeInfo
		expected   bool
	}{
		{"only enforcement", ChangeInfo{EnforcedUpdates: enforced}, true},
		{"enforcement and a new update", ChangeInfo{EnforcedUpdates: enforced, NewUpdates: []UpdateInfo{{Type: "gomod"}}}, false},
		{"enforcement and a fixed update", ChangeInfo{EnforcedUpdates: enforced, FixedUpdates: []UpdateInfo{{Type: "npm"}}}, false},
		{"no enforcement", ChangeInfo{NewUpdates: []UpdateInfo{{Type: "gomod"}}}, false},
	} {
		if got := tt.changeInfo.OnlyEnforced(); got != tt.expected {
			t.Errorf("%v: OnlyEnforced() = %v, expected %v", tt.name, got, tt.expected)
		}
	}
}

func TestForEnforcementUsesTheEnforceTitleAndCommitMessage(t *testing.T) {
	params := PullRequestParameters{
		PRTitle:              "update",
		CommitMessage:        "update",
		EnforcePRTitle:       "enforce",
		EnforceCommitMessage: "enforce commit",
		PRLabels:             []string{"automerge"},
	}

	got := params.ForEnforcement()

	if got.PRTitle != "enforce" || got.CommitMessage != "enforce commit" || !reflect.DeepEqual(got.PRLabels, []string{"automerge"}) {
		t.Errorf("ForEnforcement() = %+v, expected the enforce title and commit message with the same labels", got)
	}
}

func TestForEnforcementKeepsTheDefaultsWhenNotSet(t *testing.T) {
	params := PullRequestParameters{PRTitle: "update", CommitMessage: "update commit"}

	got := params.ForEnforcement()

	if got.PRTitle != "update" || got.CommitMessage != "update commit" {
		t.Errorf("ForEnforcement() = %+v, expected the default title and commit message", got)
	}
}
