package config

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"go.yaml.in/yaml/v4"
)

// Fields of an existing update entry that can be enforced.
const (
	EnforceSchedule              = "schedule"
	EnforceCooldown              = "cooldown"
	EnforceOpenPullRequestsLimit = "open-pull-requests-limit"
)

var enforceableFields = []string{EnforceSchedule, EnforceCooldown, EnforceOpenPullRequestsLimit}

// Enforce lists the fields of existing update entries that get the values a new entry would get.
type Enforce struct {
	Fields         []string `yaml:"fields"`
	ExceptionsFile string   `yaml:"exceptions-file,omitempty"`
	ReposFile      string   `yaml:"repos-file,omitempty"`
	exceptions     map[string][]string
	repos          map[string]bool
}

// EnforceException lets a repository keep its own values for some fields.
type EnforceException struct {
	Repo   string   `yaml:"repo"`
	Fields []string `yaml:"fields"`
	Reason string   `yaml:"reason"`
	Owner  string   `yaml:"owner"`
}

func (enforce *Enforce) validate() error {
	if len(enforce.Fields) == 0 {
		return errors.New("enforce needs at least one field")
	}

	return validateEnforceableFields("enforce", enforce.Fields)
}

func validateEnforceableFields(section string, fields []string) error {
	seen := map[string]bool{}
	for _, field := range fields {
		if !util.Contains(enforceableFields, field) {
			return fmt.Errorf("%v has field %q, expected one of %v", section, field, enforceableFields)
		}
		if seen[field] {
			return fmt.Errorf("%v has field %q more than once", section, field)
		}

		seen[field] = true
	}

	return nil
}

// LoadFiles reads the exceptions-file and the repos-file, relative to the directory of the tool config.
func (enforce *Enforce) LoadFiles(configDir string) error {
	if enforce.ExceptionsFile != "" {
		exceptions, err := readExceptions(filepath.Join(configDir, enforce.ExceptionsFile))
		if err != nil {
			return err
		}

		enforce.exceptions = exceptions
	}

	if enforce.ReposFile != "" {
		repos, err := readRepoNames(filepath.Join(configDir, enforce.ReposFile))
		if err != nil {
			return fmt.Errorf("could not read the enforce repos-file: %w", err)
		}

		enforce.repos = repos
	}

	return nil
}

func readExceptions(path string) (map[string][]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read the enforce exceptions-file: %w", err)
	}

	var exceptions []EnforceException
	if err := yaml.Unmarshal(content, &exceptions); err != nil {
		return nil, fmt.Errorf("could not parse the enforce exceptions-file %v: %w", path, err)
	}

	fieldsByRepo := map[string][]string{}
	for i, exception := range exceptions {
		section := fmt.Sprintf("exception %d of %v", i+1, path)
		if exception.Repo == "" {
			return nil, fmt.Errorf("%v needs a repo", section)
		}
		if len(exception.Fields) == 0 {
			return nil, fmt.Errorf("%v needs fields", section)
		}
		if err := validateEnforceableFields(section, exception.Fields); err != nil {
			return nil, err
		}
		if exception.Reason == "" || exception.Owner == "" {
			return nil, fmt.Errorf("%v needs a reason and an owner", section)
		}

		repo := strings.ToLower(exception.Repo)
		fieldsByRepo[repo] = append(fieldsByRepo[repo], exception.Fields...)
	}

	return fieldsByRepo, nil
}

// FieldsFor returns the fields to enforce for a repository: none if it is not in the repos-file, and not those its
// exceptions list. Repository names match regardless of case, as on GitHub.
func (enforce *Enforce) FieldsFor(repo string) []string {
	if enforce == nil {
		return nil
	}

	repo = strings.ToLower(repo)
	if enforce.repos != nil && !enforce.repos[repo] {
		return nil
	}

	var fields []string
	for _, field := range enforce.Fields {
		if !util.Contains(enforce.exceptions[repo], field) {
			fields = append(fields, field)
		}
	}

	return fields
}

// enforceUpdateConfig sets the enforced fields of an existing update to the values a new update of its ecosystem would
// get, and returns the fields that changed.
func enforceUpdateConfig(update *Update, toolConfig ToolConfig, slotSchedule *Schedule, fields []string) []string {
	if len(fields) == 0 {
		return nil
	}

	expected := createUpdateEntry(update.PackageEcosystem, update.Directory, toolConfig, slotSchedule)

	var changed []string
	for _, field := range fields {
		switch field {
		case EnforceSchedule:
			if !sameYaml(update.Schedule, expected.Schedule) {
				update.Schedule = expected.Schedule
				changed = append(changed, field)
			}
		case EnforceCooldown:
			if !sameYaml(update.Cooldown, expected.Cooldown) {
				update.Cooldown = expected.Cooldown
				changed = append(changed, field)
			}
		case EnforceOpenPullRequestsLimit:
			if !sameYaml(update.OpenPullRequestsLimit, expected.OpenPullRequestsLimit) {
				update.OpenPullRequestsLimit = expected.OpenPullRequestsLimit
				changed = append(changed, field)
			}
		}
	}

	if len(changed) > 0 {
		log.Printf("INFO  Enforced %v on %v %v.", strings.Join(changed, ", "), update.PackageEcosystem, update.Directory)
	}

	return changed
}

// sameYaml returns whether two values are written the same way to dependabot.yml. Comparing the values themselves would
// tell an empty list from a missing one, although neither is written.
func sameYaml(a any, b any) bool {
	aYaml, aErr := yaml.Marshal(a)
	bYaml, bErr := yaml.Marshal(b)

	return aErr == nil && bErr == nil && bytes.Equal(aYaml, bYaml)
}

// OnlyEnforced returns whether enforced fields of existing updates are the only changes.
func (changeInfo ChangeInfo) OnlyEnforced() bool {
	otherChanges := len(changeInfo.NewRegistries) + len(changeInfo.RemovedRegistries) + len(changeInfo.NewUpdates) +
		len(changeInfo.FixedUpdates) + len(changeInfo.RemovedUpdates)

	return len(changeInfo.EnforcedUpdates) > 0 && otherChanges == 0
}

// ForEnforcement returns the parameters of a PR that only enforces fields: the enforce title and commit message, if set.
func (params PullRequestParameters) ForEnforcement() PullRequestParameters {
	if params.EnforcePRTitle != "" {
		params.PRTitle = params.EnforcePRTitle
	}
	if params.EnforceCommitMessage != "" {
		params.CommitMessage = params.EnforceCommitMessage
	}

	return params
}
