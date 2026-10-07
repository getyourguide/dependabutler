package config

import (
	"errors"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strings"
	"time"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
)

const weekdays = 5

// ScheduleSlots spreads the schedules of new update entries over fixed slots, derived from the repository name.
type ScheduleSlots struct {
	Salt    string       `yaml:"salt"`
	Windows []SlotWindow `yaml:"windows"`
}

// SlotWindow is a set of hours a repository can be scheduled in, and which repositories use it.
// The last window has no rulesets and no repos-file, and holds every repository no other window matches.
type SlotWindow struct {
	Name      string   `yaml:"name"`
	Hours     []int    `yaml:"hours"`
	Timezone  string   `yaml:"timezone"`
	Rulesets  []string `yaml:"rulesets,omitempty"`
	ReposFile string   `yaml:"repos-file,omitempty"`
	repos     map[string]bool
}

func (slots *ScheduleSlots) validate() error {
	if len(slots.Windows) == 0 {
		return errors.New("schedule-slots needs at least one window")
	}

	for i, window := range slots.Windows {
		if err := window.validate(); err != nil {
			return err
		}

		isLast := i == len(slots.Windows)-1
		if isLast && window.matchesRepos() {
			return fmt.Errorf("the last window %q must have no rulesets and no repos-file, it holds every other repo", window.Name)
		}
		if !isLast && !window.matchesRepos() {
			return fmt.Errorf("window %q has no rulesets and no repos-file, only the last window may have neither", window.Name)
		}
	}

	return nil
}

func (window SlotWindow) validate() error {
	if window.Name == "" {
		return errors.New("every schedule-slots window needs a name")
	}
	if len(window.Hours) == 0 {
		return fmt.Errorf("window %q has no hours", window.Name)
	}

	seen := map[int]bool{}
	for _, hour := range window.Hours {
		if hour < 0 || hour > 23 {
			return fmt.Errorf("window %q has hour %d, expected 0 to 23", window.Name, hour)
		}
		if seen[hour] {
			return fmt.Errorf("window %q has hour %d more than once", window.Name, hour)
		}

		seen[hour] = true
	}

	if window.Timezone == "" || window.Timezone == "Local" {
		return fmt.Errorf("window %q needs a timezone, %q is not one", window.Name, window.Timezone)
	}
	if _, err := time.LoadLocation(window.Timezone); err != nil {
		return fmt.Errorf("window %q has an unknown timezone %q: %w", window.Name, window.Timezone, err)
	}

	return nil
}

func (window SlotWindow) matchesRepos() bool {
	return len(window.Rulesets) > 0 || window.ReposFile != ""
}

// LoadReposFiles reads the repos-file of every window, relative to the directory of the tool config.
func (slots *ScheduleSlots) LoadReposFiles(configDir string) error {
	for i := range slots.Windows {
		window := &slots.Windows[i]
		if window.ReposFile == "" {
			continue
		}

		repos, err := readRepoNames(filepath.Join(configDir, window.ReposFile))
		if err != nil {
			return fmt.Errorf("could not read the repos-file of window %q: %w", window.Name, err)
		}

		window.repos = repos
	}

	return nil
}

// readRepoNames reads a file with one repository name per line, in lower case. Blank lines are skipped.
func readRepoNames(path string) (map[string]bool, error) {
	lines, err := util.ReadLinesFromFile(path)
	if err != nil {
		return nil, err
	}

	repos := map[string]bool{}
	for _, line := range lines {
		if name := strings.ToLower(strings.TrimSpace(line)); name != "" {
			repos[name] = true
		}
	}

	return repos, nil
}

// NeedsRulesets returns whether any window selects repositories by ruleset.
func (slots *ScheduleSlots) NeedsRulesets() bool {
	for _, window := range slots.Windows {
		if len(window.Rulesets) > 0 {
			return true
		}
	}

	return false
}

// WindowFor returns the first window that one of the repository's rulesets or its repos-file selects,
// or the last window when none does. Repository names match regardless of case, as on GitHub.
func (slots *ScheduleSlots) WindowFor(repo string, rulesets []string) SlotWindow {
	for _, window := range slots.Windows {
		if window.repos[strings.ToLower(repo)] {
			return window
		}
		for _, ruleset := range rulesets {
			if util.Contains(window.Rulesets, ruleset) {
				return window
			}
		}
	}

	return slots.Windows[len(slots.Windows)-1]
}

// Schedule returns the cron schedule of a repository in this window.
func (window SlotWindow) Schedule(salt string, repo string) Schedule {
	slot := SlotFor(salt, repo, weekdays*len(window.Hours))
	return Schedule{Interval: "cron", Cronjob: slotCronjob(slot, window.Hours), Timezone: window.Timezone}
}

// SlotFor maps a repository name to one of slotCount slots, using FNV-1a over "salt:repo" with the name in lower case.
func SlotFor(salt string, repo string, slotCount int) int {
	hash := fnv.New32a()
	hash.Write([]byte(salt + ":" + strings.ToLower(repo)))

	return int(hash.Sum32() % uint32(slotCount))
}

// slotCronjob runs on the slot's weekday and three weekdays later, so every weekday is part of exactly two pairs.
func slotCronjob(slot int, hours []int) string {
	hour := hours[slot%len(hours)]
	first := slot / len(hours)

	second := (first + 3) % weekdays
	if second < first {
		first, second = second, first
	}

	return fmt.Sprintf("0 %d * * %d,%d", hour, first+1, second+1)
}
