package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/getyourguide/dependabutler/internal/pkg/config"
)

const scheduleSlotsConfig = `
schedule-slots:
  salt: test
  windows:
    - name: office-hours
      hours: [10, 11, 13, 14, 15]
      timezone: Europe/Berlin
      rulesets: [audited]
      repos-file: office-hours.txt
    - name: early
      hours: [2, 3, 4, 5, 6, 7, 8]
      timezone: UTC
`

func loadTestScheduleSlots(t *testing.T) *config.ScheduleSlots {
	t.Helper()

	dir := t.TempDir()
	configFile := filepath.Join(dir, "dependabutler.yml")
	if err := os.WriteFile(configFile, []byte(scheduleSlotsConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "office-hours.txt"), []byte("opted-in\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	toolConfig, err := config.ParseToolConfig([]byte(scheduleSlotsConfig))
	if err != nil {
		t.Fatal(err)
	}
	if err := loadScheduleSlots(toolConfig, configFile); err != nil {
		t.Fatalf("loadScheduleSlots() failed: %v", err)
	}

	return toolConfig.ScheduleSlots
}

func TestLoadScheduleSlotsReadsReposFilesNextToTheConfig(t *testing.T) {
	slots := loadTestScheduleSlots(t)

	if got := slots.WindowFor("opted-in", nil).Name; got != "office-hours" {
		t.Errorf("WindowFor(opted-in) = %q, expected office-hours", got)
	}
}

func TestLoadScheduleSlotsWithoutSlots(t *testing.T) {
	if err := loadScheduleSlots(&config.ToolConfig{}, "dependabutler.yml"); err != nil {
		t.Errorf("loadScheduleSlots() failed without schedule-slots: %v", err)
	}
}

func TestSlotSchedule(t *testing.T) {
	slots := loadTestScheduleSlots(t)
	rulesets := func(names ...string) func() ([]string, error) {
		return func() ([]string, error) { return names, nil }
	}

	for _, tt := range []struct {
		name     string
		repo     string
		rulesets func() ([]string, error)
		timezone string
	}{
		{"audited by ruleset", "audited-repo", rulesets("default", "audited"), "Europe/Berlin"},
		{"opted in by file", "opted-in", rulesets("default"), "Europe/Berlin"},
		{"everything else", "plain-repo", rulesets("default"), "UTC"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			schedule, err := slotSchedule(slots, tt.repo, tt.rulesets)
			if err != nil {
				t.Fatalf("slotSchedule() failed: %v", err)
			}

			if schedule.Interval != "cron" || schedule.Cronjob == "" || schedule.Timezone != tt.timezone {
				t.Errorf("slotSchedule() = %+v, expected a cron schedule in %v", schedule, tt.timezone)
			}
		})
	}
}

func TestSlotScheduleFailsWhenRulesetsCannotBeRead(t *testing.T) {
	slots := loadTestScheduleSlots(t)

	_, err := slotSchedule(slots, "some-repo", func() ([]string, error) { return nil, errors.New("forbidden") })

	if err == nil {
		t.Errorf("slotSchedule() succeeded, expected the ruleset error")
	}
}

func TestSlotScheduleWithoutSlotsReadsNoRulesets(t *testing.T) {
	schedule, err := slotSchedule(nil, "some-repo", func() ([]string, error) {
		t.Errorf("rulesets read without schedule-slots")
		return nil, nil
	})

	if schedule != nil || err != nil {
		t.Errorf("slotSchedule() = %v, %v, expected nil, nil", schedule, err)
	}
}

func TestSlotScheduleReadsRulesetsOnlyWhenAWindowUsesThem(t *testing.T) {
	slots := &config.ScheduleSlots{Windows: []config.SlotWindow{{Name: "early", Hours: []int{2}, Timezone: "UTC"}}}

	schedule, err := slotSchedule(slots, "some-repo", func() ([]string, error) {
		t.Errorf("rulesets read although no window uses them")
		return nil, nil
	})

	if err != nil || schedule == nil || schedule.Cronjob == "" {
		t.Errorf("slotSchedule() = %v, %v, expected a schedule", schedule, err)
	}
}

func TestLoadEnforceReadsFilesNextToTheConfig(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "dependabutler.yml")
	if err := os.WriteFile(filepath.Join(dir, "rollout.txt"), []byte("in-wave\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	toolConfig := &config.ToolConfig{Enforce: &config.Enforce{Fields: []string{"cooldown"}, ReposFile: "rollout.txt"}}
	if err := loadEnforce(toolConfig, configFile); err != nil {
		t.Fatalf("loadEnforce() failed: %v", err)
	}

	if toolConfig.Enforce.FieldsFor("in-wave") == nil || toolConfig.Enforce.FieldsFor("later-wave") != nil {
		t.Errorf("FieldsFor() does not follow the repos-file next to the config")
	}
}

func TestLoadEnforceWithoutEnforce(t *testing.T) {
	if err := loadEnforce(&config.ToolConfig{}, "dependabutler.yml"); err != nil {
		t.Errorf("loadEnforce() failed without enforce: %v", err)
	}
}

func TestPullRequestConfigForOnlyEnforcedChanges(t *testing.T) {
	toolConfig := config.ToolConfig{PullRequestParameters: config.PullRequestParameters{PRTitle: "update", EnforcePRTitle: "enforce"}}
	enforced := []config.EnforcedUpdateInfo{{Type: "npm", Directory: "/", Fields: []string{"cooldown"}}}

	onlyEnforced := pullRequestConfig(toolConfig, config.ChangeInfo{EnforcedUpdates: enforced})
	mixed := pullRequestConfig(toolConfig, config.ChangeInfo{EnforcedUpdates: enforced, NewUpdates: []config.UpdateInfo{{Type: "gomod"}}})

	if onlyEnforced.PullRequestParameters.PRTitle != "enforce" || mixed.PullRequestParameters.PRTitle != "update" {
		t.Errorf("titles: only enforced %q, mixed %q; expected enforce and update",
			onlyEnforced.PullRequestParameters.PRTitle, mixed.PullRequestParameters.PRTitle)
	}
}

func TestLocalRepoName(t *testing.T) {
	if got := localRepoName("given", "/some/dir"); got != "given" {
		t.Errorf("localRepoName() = %q, expected the -repo value", got)
	}
	if got := localRepoName("", "/some/my-repo/"); got != "my-repo" {
		t.Errorf("localRepoName() = %q, expected the directory name", got)
	}
}

func TestRemoteRepoNames(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile := func(name string, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile() failed: %v", err)
		}
		return path
	}

	for _, tt := range []struct {
		name     string
		repo     string
		repoFile string
		expected []string
		wantErr  bool
	}{
		{name: "repo takes precedence", repo: "single", repoFile: filepath.Join(dir, "missing.txt"), expected: []string{"single"}},
		{name: "blank lines skipped", repoFile: writeRepoFile("list.txt", "one\n\n  \r\ntwo\r\n three \n"), expected: []string{"one", "two", "three"}},
		{name: "missing file", repoFile: filepath.Join(dir, "missing.txt"), wantErr: true},
		{name: "no repos", repoFile: writeRepoFile("empty.txt", "\n \n"), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := remoteRepoNames(tt.repo, tt.repoFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("remoteRepoNames() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("remoteRepoNames() = %q, expected %q", got, tt.expected)
			}
		})
	}
}
