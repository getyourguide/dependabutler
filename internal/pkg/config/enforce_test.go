package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
