package config

import (
	"fmt"
	"log"

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
