package config

import (
	"errors"
	"fmt"
	"hash/fnv"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"go.yaml.in/yaml/v4"
)

const defaultRunsPerWeek = 2

var (
	defaultDays = []string{"monday", "tuesday", "wednesday", "thursday", "friday"}
	cronWeekday = map[string]int{
		"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3, "thursday": 4, "friday": 5, "saturday": 6,
	}
)

// ScheduleSlots spreads the schedules of new update entries over fixed slots, derived from the repository name.
type ScheduleSlots struct {
	Salt    string       `yaml:"salt"`
	Windows []SlotWindow `yaml:"windows"`
}

// SlotWindow is a set of hours a repository can be scheduled in, and which repositories use it.
// The last window has no rulesets and no repos-file, and holds every repository no other window matches.
type SlotWindow struct {
	Name        string   `yaml:"name"`
	Hours       []int    `yaml:"hours"`
	Days        []string `yaml:"days,omitempty"`
	RunsPerWeek int      `yaml:"runs-per-week,omitempty"`
	Timezone    string   `yaml:"timezone"`
	Rulesets    []string `yaml:"rulesets,omitempty"`
	ReposFile   string   `yaml:"repos-file,omitempty"`
	repos       map[string]bool
}

// UnmarshalYAML rejects unknown keys: a typo would otherwise be dropped without a word.
func (slots *ScheduleSlots) UnmarshalYAML(node *yaml.Node) error {
	type plain ScheduleSlots
	if err := rejectUnknownKeys(node, "schedule-slots", plain{}); err != nil {
		return err
	}

	return node.Decode((*plain)(slots))
}

// UnmarshalYAML rejects unknown keys: a typo like "ruleset" would otherwise put the repos the window selects in the
// last window.
func (window *SlotWindow) UnmarshalYAML(node *yaml.Node) error {
	type plain SlotWindow
	if err := rejectUnknownKeys(node, "schedule-slots window", plain{}); err != nil {
		return err
	}

	return node.Decode((*plain)(window))
}

// rejectUnknownKeys checks the keys of the mapping itself: keys brought in by a merge key would go unchecked, so merge
// keys are rejected.
func rejectUnknownKeys(node *yaml.Node, section string, value any) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%v must be a mapping", section)
	}

	known := yamlKeys(reflect.TypeOf(value))
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Value == "<<" {
			return fmt.Errorf("merge keys (<<) are not supported in %v", section)
		}
		if !slices.Contains(known, key.Value) {
			return fmt.Errorf("unknown key %q in %v, expected one of %v", key.Value, section, known)
		}
	}

	return nil
}

func yamlKeys(structType reflect.Type) []string {
	var keys []string
	for i := range structType.NumField() {
		name, _, _ := strings.Cut(structType.Field(i).Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}

	return keys
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

	seenDays := map[string]bool{}
	for _, day := range window.Days {
		if _, known := cronWeekday[day]; !known {
			return fmt.Errorf("window %q has day %q, expected a weekday in lower case, like monday", window.Name, day)
		}
		if seenDays[day] {
			return fmt.Errorf("window %q has day %q more than once", window.Name, day)
		}

		seenDays[day] = true
	}

	if runs, days := window.Runs(), len(window.Weekdays()); window.RunsPerWeek < 0 || runs > days {
		return fmt.Errorf("window %q runs %d times a week, expected 1 to %d, the number of its days", window.Name, runs, days)
	}

	if window.Timezone == "" || window.Timezone == "Local" {
		return fmt.Errorf("window %q needs a timezone, %q is not one", window.Name, window.Timezone)
	}
	if _, err := time.LoadLocation(window.Timezone); err != nil {
		return fmt.Errorf("window %q has an unknown timezone %q: %w", window.Name, window.Timezone, err)
	}

	return nil
}

// Weekdays returns the days the window runs on, Monday to Friday unless days is set.
func (window SlotWindow) Weekdays() []string {
	if len(window.Days) == 0 {
		return defaultDays
	}

	return window.Days
}

// Runs returns how many times a week a repository in the window runs, twice unless runs-per-week is set.
func (window SlotWindow) Runs() int {
	if window.RunsPerWeek == 0 {
		return defaultRunsPerWeek
	}

	return window.RunsPerWeek
}

func (window SlotWindow) cronWeekdays() []int {
	var weekdays []int
	for _, day := range window.Weekdays() {
		weekdays = append(weekdays, cronWeekday[day])
	}

	slices.Sort(weekdays)

	return weekdays
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

		path := filepath.Join(configDir, window.ReposFile)
		lines, err := util.ReadLinesFromFile(path)
		if err != nil {
			return fmt.Errorf("could not read repos-file %v of window %q: %w", path, window.Name, err)
		}

		window.repos = map[string]bool{}
		for _, line := range lines {
			if name := strings.ToLower(strings.TrimSpace(line)); name != "" {
				window.repos[name] = true
			}
		}
	}

	return nil
}

// NeedsRulesets returns whether the rulesets of a repository can change its window: not when the repos-file of a
// window before the first ruleset window, or of that window itself, lists it. Reading them costs an API call, and a
// failed call skips the repository.
func (slots *ScheduleSlots) NeedsRulesets(repo string) bool {
	for _, window := range slots.Windows {
		if window.repos[strings.ToLower(repo)] {
			return false
		}
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
	weekdays := window.cronWeekdays()
	slot := SlotFor(salt, repo, len(weekdays)*len(window.Hours))

	return Schedule{Interval: "cron", Cronjob: slotCronjob(slot, window.Hours, weekdays, window.Runs()), Timezone: window.Timezone}
}

// SlotFor maps a repository name to one of slotCount slots, using FNV-1a over "salt:repo" with the name in lower case.
func SlotFor(salt string, repo string, slotCount int) int {
	hash := fnv.New32a()
	hash.Write([]byte(salt + ":" + strings.ToLower(repo)))

	return int(hash.Sum32() % uint32(slotCount))
}

// slotCronjob spreads the runs over the weekdays as evenly as they divide. As every weekday starts the same number of
// slots, every weekday also runs in the same number of them.
func slotCronjob(slot int, hours []int, weekdays []int, runs int) string {
	hour := hours[slot%len(hours)]
	first := slot / len(hours)

	var runDays []int
	for run := range runs {
		offset := (run*len(weekdays) + runs - 1) / runs
		runDays = append(runDays, weekdays[(first+offset)%len(weekdays)])
	}

	slices.Sort(runDays)

	var fields []string
	for _, day := range runDays {
		fields = append(fields, strconv.Itoa(day))
	}

	return fmt.Sprintf("0 %d * * %s", hour, strings.Join(fields, ","))
}
