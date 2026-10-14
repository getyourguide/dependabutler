package config

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
)

// Directory grouping modes.
const (
	DirectoriesGlobal = "global"
	DirectoriesPerApp = "per-app"
)

var directoryGroupingModes = []string{DirectoriesGlobal, DirectoriesPerApp}

// DirectoryGrouping chooses between one update entry per ecosystem for all directories of a repository (global), whose
// groups then cover all directories, and one entry per directory (per-app).
type DirectoryGrouping struct {
	Default  string `yaml:"default"`
	Property string `yaml:"property,omitempty"`
}

func (grouping *DirectoryGrouping) validate() error {
	if !util.Contains(directoryGroupingModes, grouping.Default) {
		return fmt.Errorf("directory-grouping has default %q, expected one of %v", grouping.Default, directoryGroupingModes)
	}

	return nil
}

// ModeFor returns the mode of a repository: the value of its custom property if that is a mode, the default otherwise.
func (grouping *DirectoryGrouping) ModeFor(propertyValue string) string {
	if grouping == nil {
		return ""
	}

	if util.Contains(directoryGroupingModes, propertyValue) {
		return propertyValue
	}
	if propertyValue != "" {
		log.Printf("WARN  Custom property %v has value %q, expected one of %v, using %v.", grouping.Property, propertyValue,
			directoryGroupingModes, grouping.Default)
	}

	return grouping.Default
}

// groupDirectories merges the compatible updates of an ecosystem into one (global), or splits updates with several
// directories into one update per directory (per-app).
func (config *DependabotConfig) groupDirectories(mode string, changeInfo *ChangeInfo) {
	switch mode {
	case DirectoriesGlobal:
		config.mergeCompatibleUpdates(changeInfo)
	case DirectoriesPerApp:
		config.splitUpdates(changeInfo)
	}
}

func (config *DependabotConfig) mergeCompatibleUpdates(changeInfo *ChangeInfo) {
	var merged []Update
	mergedInto := map[int]bool{}
	for _, update := range config.Updates {
		if i := compatibleUpdate(merged, update); i >= 0 {
			merged[i].addDirectories(update.directories(), update.Registries)
			mergedInto[i] = true
			continue
		}

		merged = append(merged, update)
	}

	for i := range merged {
		if mergedInto[i] {
			changeInfo.EnforcedUpdates = append(changeInfo.EnforcedUpdates, EnforcedUpdateInfo{
				Type: merged[i].PackageEcosystem, Directory: updateDirectories(&merged[i]), Fields: []string{EnforceDirectoryGrouping},
			})
		}
	}

	config.Updates = merged
}

func (config *DependabotConfig) splitUpdates(changeInfo *ChangeInfo) {
	var split []Update
	for i, update := range config.Updates {
		if len(update.Directories) < 2 || slices.ContainsFunc(update.Directories, isGlob) {
			split = append(split, update)
			continue
		}
		if config.overlapsOtherUpdate(i) {
			log.Printf("WARN  Not splitting %v %v: another update has one of its directories.", update.PackageEcosystem, updateDirectories(&update))
			split = append(split, update)
			continue
		}

		for _, directory := range update.Directories {
			single := update
			single.Directory = directory
			single.Directories = nil
			single.Registries = slices.Clone(update.Registries)
			split = append(split, single)
		}

		changeInfo.EnforcedUpdates = append(changeInfo.EnforcedUpdates, EnforcedUpdateInfo{
			Type: update.PackageEcosystem, Directory: updateDirectories(&update), Fields: []string{EnforceDirectoryGrouping},
		})
	}

	config.Updates = split
}

// overlapsOtherUpdate returns whether another update of the same ecosystem and target branch has one of the directories
// of the update at index i.
func (config *DependabotConfig) overlapsOtherUpdate(i int) bool {
	update := config.Updates[i]
	for j := range config.Updates {
		other := &config.Updates[j]
		if j == i || other.PackageEcosystem != update.PackageEcosystem || other.TargetBranch != update.TargetBranch {
			continue
		}

		if slices.ContainsFunc(other.directories(), func(directory string) bool { return slices.Contains(update.Directories, directory) }) {
			return true
		}
	}

	return false
}

// removeMissingDirectories removes the update entries and the directories of entries that no longer exist. A check
// that fails is not a missing directory: removing it would drop the settings of its entry when it is added back.
func (config *DependabotConfig) removeMissingDirectories(exists CheckDirectoryExists, params CheckDirectoryExistsParameters, changeInfo *ChangeInfo) error {
	var kept []Update
	for _, update := range config.Updates {
		if len(update.Directories) > 0 {
			directories, err := existingDirectories(update, exists, params, changeInfo)
			if err != nil {
				return err
			}
			if len(directories) > 0 {
				update.Directories = directories
				kept = append(kept, update)
			}

			continue
		}

		found, err := exists(update.Directory, params)
		if err != nil {
			return fmt.Errorf("could not check directory %v of %v: %w", update.Directory, update.PackageEcosystem, err)
		}
		if found {
			kept = append(kept, update)
		} else {
			changeInfo.RemovedUpdates = append(changeInfo.RemovedUpdates, UpdateInfo{Type: update.PackageEcosystem, Directory: update.Directory})
		}
	}

	config.Updates = kept

	return nil
}

// existingDirectories returns the directories of an update that exist, and records the others as removed. Glob
// patterns are kept.
func existingDirectories(update Update, exists CheckDirectoryExists, params CheckDirectoryExistsParameters, changeInfo *ChangeInfo) ([]string, error) {
	var kept []string
	for _, directory := range update.Directories {
		if isGlob(directory) {
			kept = append(kept, directory)
			continue
		}

		found, err := exists(directory, params)
		if err != nil {
			return nil, fmt.Errorf("could not check directory %v of %v: %w", directory, update.PackageEcosystem, err)
		}
		if found {
			kept = append(kept, directory)
			continue
		}

		changeInfo.RemovedUpdates = append(changeInfo.RemovedUpdates, UpdateInfo{Type: update.PackageEcosystem, Directory: directory})
	}

	return kept, nil
}

// compatibleUpdate returns the index of the update that can take the directories of update, or -1: one of the same
// ecosystem whose settings are the same apart from its directories and registries.
func compatibleUpdate(updates []Update, update Update) int {
	for i := range updates {
		if updates[i].PackageEcosystem == update.PackageEcosystem && sameYaml(mergeKey(updates[i]), mergeKey(update)) {
			return i
		}
	}

	return -1
}

func mergeKey(update Update) Update {
	update.Directory = ""
	update.Directories = nil
	update.Registries = nil

	return update
}

func (update *Update) directories() []string {
	if update.Directory != "" {
		return []string{update.Directory}
	}

	return update.Directories
}

// addDirectories adds directories and registries to an update, keeping its directories sorted and without repeats.
func (update *Update) addDirectories(directories []string, registries []string) {
	all := append(slices.Clone(update.directories()), directories...)
	slices.Sort(all)
	all = slices.Compact(all)

	update.Directory = ""
	update.Directories = all
	if len(all) == 1 {
		update.Directory = all[0]
		update.Directories = nil
	}

	mergeStringLists(&update.Registries, registries)
}

func isGlob(directory string) bool {
	return strings.ContainsAny(directory, "*?[")
}
