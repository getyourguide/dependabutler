package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const scheduleSlotsConfig = `
schedule-slots:
  salt: some-salt
  windows:
    - name: office-hours
      hours: [10, 11, 13, 14, 15]
      timezone: Europe/Berlin
      rulesets: [audited]
      repos-file: office-hours.txt
    - name: early
      hours: [2, 3, 4, 5, 6, 7, 8]
      timezone: Europe/Berlin
`

func TestParseToolConfigReadsScheduleSlots(t *testing.T) {
	toolConfig, err := ParseToolConfig([]byte(scheduleSlotsConfig))
	if err != nil {
		t.Fatalf("ParseToolConfig() failed: %v", err)
	}

	expected := &ScheduleSlots{
		Salt: "some-salt",
		Windows: []SlotWindow{
			{
				Name:      "office-hours",
				Hours:     []int{10, 11, 13, 14, 15},
				Timezone:  "Europe/Berlin",
				Rulesets:  []string{"audited"},
				ReposFile: "office-hours.txt",
			},
			{
				Name:     "early",
				Hours:    []int{2, 3, 4, 5, 6, 7, 8},
				Timezone: "Europe/Berlin",
			},
		},
	}

	if !reflect.DeepEqual(toolConfig.ScheduleSlots, expected) {
		t.Errorf("ScheduleSlots\nExpected: %+v\nGot:      %+v", expected, toolConfig.ScheduleSlots)
	}
}

func TestParseToolConfigWithoutScheduleSlots(t *testing.T) {
	toolConfig, err := ParseToolConfig([]byte("update-defaults:\n  open-pull-requests-limit: 5\n"))
	if err != nil {
		t.Fatalf("ParseToolConfig() failed: %v", err)
	}

	if toolConfig.ScheduleSlots != nil {
		t.Errorf("ScheduleSlots = %+v, expected nil", toolConfig.ScheduleSlots)
	}
}

func TestParseToolConfigRejectsInvalidScheduleSlots(t *testing.T) {
	for _, tt := range []struct {
		name    string
		windows string
		error   string
	}{
		{"no window", `[]`, "at least one window"},
		{"no name", `[{hours: [2], timezone: UTC}]`, "name"},
		{"no hours", `[{name: early, timezone: UTC}]`, "no hours"},
		{"hour out of range", `[{name: early, hours: [24], timezone: UTC}]`, "hour 24"},
		{"negative hour", `[{name: early, hours: [-1], timezone: UTC}]`, "hour -1"},
		{"repeated hour", `[{name: early, hours: [2, 2], timezone: UTC}]`, "hour 2 more than once"},
		{"no timezone", `[{name: early, hours: [2]}]`, "timezone"},
		{"unknown timezone", `[{name: early, hours: [2], timezone: Mars/Olympus}]`, "Mars/Olympus"},
		{"local timezone", `[{name: early, hours: [2], timezone: Local}]`, "Local"},
		{"no default window", `[{name: office, hours: [10], timezone: UTC, repos-file: x.txt}]`, "last window"},
		{
			"default window not last",
			`[{name: early, hours: [2], timezone: UTC}, {name: office, hours: [10], timezone: UTC, rulesets: [a]}]`,
			"last window",
		},
		{
			"two default windows",
			`[{name: early, hours: [2], timezone: UTC}, {name: late, hours: [20], timezone: UTC}]`,
			"only the last window",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseToolConfig([]byte("schedule-slots:\n  windows: " + tt.windows + "\n"))

			if err == nil || !strings.Contains(err.Error(), tt.error) {
				t.Errorf("ParseToolConfig() error = %v, expected it to contain %q", err, tt.error)
			}
		})
	}
}

func TestSlotCronjobPairsEachWeekdayWithTheOneThreeWeekdaysLater(t *testing.T) {
	hours := []int{2, 3}

	for _, tt := range []struct {
		slot    int
		cronjob string
	}{
		{0, "0 2 * * 1,4"},
		{1, "0 3 * * 1,4"},
		{2, "0 2 * * 2,5"},
		{4, "0 2 * * 1,3"},
		{6, "0 2 * * 2,4"},
		{8, "0 2 * * 3,5"},
		{9, "0 3 * * 3,5"},
	} {
		if got := slotCronjob(tt.slot, hours); got != tt.cronjob {
			t.Errorf("slotCronjob(%d) = %q, expected %q", tt.slot, got, tt.cronjob)
		}
	}
}

func TestSlotWindowScheduleIsStable(t *testing.T) {
	window := SlotWindow{Name: "early", Hours: []int{2, 3, 4, 5, 6}, Timezone: "Europe/Berlin"}

	first := window.Schedule("salt", "my-repo")
	second := window.Schedule("salt", "my-repo")

	if !reflect.DeepEqual(first, second) {
		t.Errorf("Schedule() is not stable: %+v != %+v", first, second)
	}
	if first.Interval != "cron" || first.Timezone != "Europe/Berlin" || first.Day != "" || first.Time != "" {
		t.Errorf("Schedule() = %+v, expected a cron schedule in Europe/Berlin without day and time", first)
	}
}

func TestSlotWindowScheduleKeepsItsSlot(t *testing.T) {
	window := SlotWindow{Name: "early", Hours: []int{2, 3, 4, 5, 6, 7, 8}, Timezone: "Europe/Berlin"}

	got := window.Schedule("salt", "my-repo").Cronjob

	if got != "0 7 * * 3,5" {
		t.Errorf("Schedule() = %q, a changed slot moves every repo", got)
	}
}

func TestSlotWindowScheduleIgnoresRepoNameCase(t *testing.T) {
	window := SlotWindow{Name: "early", Hours: []int{2, 3, 4, 5, 6}, Timezone: "UTC"}

	if window.Schedule("salt", "My-Repo").Cronjob != window.Schedule("salt", "my-repo").Cronjob {
		t.Errorf("Schedule() differs by the case of the repo name")
	}
}

func TestSlotWindowScheduleMovesWithTheSalt(t *testing.T) {
	window := SlotWindow{Name: "early", Hours: []int{2, 3, 4, 5, 6}, Timezone: "UTC"}

	moved := 0
	for i := range 100 {
		repo := fmt.Sprintf("repo-%03d", i)
		if window.Schedule("a", repo).Cronjob != window.Schedule("b", repo).Cronjob {
			moved++
		}
	}

	if moved < 80 {
		t.Errorf("changing the salt moved %d of 100 repos, expected most of them", moved)
	}
}

func TestSlotForSpreadsRepoNamesEvenly(t *testing.T) {
	const (
		slotCount = 25
		repoCount = 600
	)

	counts := make([]int, slotCount)
	for i := range repoCount {
		counts[SlotFor("", fmt.Sprintf("repo-%03d", i), slotCount)]++
	}

	limit := 1.5 * repoCount / slotCount
	for slot, count := range counts {
		if float64(count) > limit {
			t.Errorf("slot %d holds %d repos, more than 1.5x the mean (%.0f)", slot, count, limit)
		}
	}
}

func TestWindowForPicksTheFirstMatchingWindow(t *testing.T) {
	slots := ScheduleSlots{Windows: []SlotWindow{
		{Name: "office-hours", Rulesets: []string{"audited"}, repos: map[string]bool{"opted-in": true}},
		{Name: "late", repos: map[string]bool{"opted-in": true, "late-only": true}},
		{Name: "early"},
	}}

	for _, tt := range []struct {
		repo     string
		rulesets []string
		window   string
	}{
		{"audited-repo", []string{"other", "audited"}, "office-hours"},
		{"opted-in", nil, "office-hours"},
		{"late-only", nil, "late"},
		{"plain-repo", []string{"other"}, "early"},
		{"plain-repo", nil, "early"},
	} {
		if got := slots.WindowFor(tt.repo, tt.rulesets).Name; got != tt.window {
			t.Errorf("WindowFor(%q, %v) = %q, expected %q", tt.repo, tt.rulesets, got, tt.window)
		}
	}
}

func TestNeedsRulesets(t *testing.T) {
	withRulesets := ScheduleSlots{Windows: []SlotWindow{{Name: "office", Rulesets: []string{"audited"}}, {Name: "early"}}}
	withoutRulesets := ScheduleSlots{Windows: []SlotWindow{{Name: "office", ReposFile: "x.txt"}, {Name: "early"}}}

	if !withRulesets.NeedsRulesets() {
		t.Errorf("NeedsRulesets() = false for a window with rulesets")
	}
	if withoutRulesets.NeedsRulesets() {
		t.Errorf("NeedsRulesets() = true without any window with rulesets")
	}
}

func TestLoadReposFilesReadsNamesRelativeToTheConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "office-hours.txt"), []byte("first-repo\n\n  Second-Repo  \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	slots := ScheduleSlots{Windows: []SlotWindow{{Name: "office", ReposFile: "office-hours.txt"}, {Name: "early"}}}

	if err := slots.LoadReposFiles(dir); err != nil {
		t.Fatalf("LoadReposFiles() failed: %v", err)
	}

	expected := map[string]bool{"first-repo": true, "second-repo": true}

	if !reflect.DeepEqual(slots.Windows[0].repos, expected) {
		t.Errorf("repos = %v, expected %v", slots.Windows[0].repos, expected)
	}
	for _, repo := range []string{"second-repo", "SECOND-repo"} {
		if got := slots.WindowFor(repo, nil).Name; got != "office" {
			t.Errorf("WindowFor(%v) = %q, expected office", repo, got)
		}
	}
}

func TestLoadReposFilesFailsOnMissingFile(t *testing.T) {
	slots := ScheduleSlots{Windows: []SlotWindow{{Name: "office", ReposFile: "missing.txt"}, {Name: "early"}}}

	err := slots.LoadReposFiles(t.TempDir())

	if err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Errorf("LoadReposFiles() error = %v, expected it to name missing.txt", err)
	}
}
