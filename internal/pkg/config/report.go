package config

import (
	"fmt"
	"sort"
)

// RepoReport is the coverage report of a repository, as printed by report mode.
type RepoReport struct {
	Repo          string           `json:"repo"`
	DefaultBranch string           `json:"default_branch"`
	DependabotYml bool             `json:"dependabot_yml"`
	TreeTruncated bool             `json:"tree_truncated"`
	Manifests     []ManifestReport `json:"manifests"`
	Updates       []UpdateReport   `json:"updates"`
}

// ManifestReport is a manifest file found in a repository, and whether an update entry covers it.
type ManifestReport struct {
	Path      string `json:"path"`
	Ecosystem string `json:"ecosystem"`
	Covered   bool   `json:"covered"`
}

// UpdateReport is an update entry of a dependabot.yml file.
type UpdateReport struct {
	Ecosystem   string         `json:"ecosystem"`
	Directory   string         `json:"directory,omitempty"`
	Directories []string       `json:"directories,omitempty"`
	Schedule    ScheduleReport `json:"schedule"`
}

// ScheduleReport is the schedule of an update entry.
type ScheduleReport struct {
	Interval string `json:"interval"`
	Cronjob  string `json:"cronjob,omitempty"`
	Day      string `json:"day,omitempty"`
	Time     string `json:"time,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// SkippedRepoReport is printed by report mode instead of a RepoReport, if a repository was skipped or could not be read.
type SkippedRepoReport struct {
	Repo    string `json:"repo"`
	Skipped string `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

// NewRepoReport builds the coverage report of a repository from its current dependabot.yml content (nil if the
// file does not exist) and its file list.
func NewRepoReport(repo string, defaultBranch string, currentConfig []byte, files []string, treeTruncated bool) (RepoReport, error) {
	dependabotConfig, err := ParseDependabotConfig(currentConfig)
	if err != nil {
		return RepoReport{}, fmt.Errorf("could not parse .github/dependabot.yml: %w", err)
	}

	manifests := map[string]string{}
	ScanFileList(files, manifests)

	manifestReports := make([]ManifestReport, 0, len(manifests))
	for path, ecosystem := range manifests {
		manifestReports = append(manifestReports, ManifestReport{
			Path:      path,
			Ecosystem: ecosystem,
			Covered:   dependabotConfig.IsManifestCovered(path, ecosystem, nil),
		})
	}

	sort.Slice(manifestReports, func(i, j int) bool { return manifestReports[i].Path < manifestReports[j].Path })

	updateReports := make([]UpdateReport, 0, len(dependabotConfig.Updates))
	for _, update := range dependabotConfig.Updates {
		updateReports = append(updateReports, UpdateReport{
			Ecosystem:   update.PackageEcosystem,
			Directory:   update.Directory,
			Directories: update.Directories,
			Schedule: ScheduleReport{
				Interval: update.Schedule.Interval,
				Cronjob:  update.Schedule.Cronjob,
				Day:      update.Schedule.Day,
				Time:     update.Schedule.Time,
				Timezone: update.Schedule.Timezone,
			},
		})
	}

	return RepoReport{
		Repo:          repo,
		DefaultBranch: defaultBranch,
		DependabotYml: currentConfig != nil,
		TreeTruncated: treeTruncated,
		Manifests:     manifestReports,
		Updates:       updateReports,
	}, nil
}
