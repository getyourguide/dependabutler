package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseToolConfigReadsDirectoryGrouping(t *testing.T) {
	toolConfig, err := ParseToolConfig([]byte("directory-grouping:\n  default: global\n  property: dependabot-grouping\n"))
	if err != nil {
		t.Fatalf("ParseToolConfig() failed: %v", err)
	}

	expected := &DirectoryGrouping{Default: "global", Property: "dependabot-grouping"}

	if !reflect.DeepEqual(toolConfig.DirectoryGrouping, expected) {
		t.Errorf("DirectoryGrouping = %+v, expected %+v", toolConfig.DirectoryGrouping, expected)
	}
}

func TestParseToolConfigRejectsInvalidDirectoryGrouping(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config string
		error  string
	}{
		{"no default", "directory-grouping:\n  property: dependabot-grouping\n", "default"},
		{"unknown default", "directory-grouping:\n  default: both\n", `"both"`},
		{
			"enforcing directory-grouping without its block",
			"update-defaults:\n  schedule: {interval: weekly}\nenforce:\n  fields: [directory-grouping]\n",
			"directory-grouping",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseToolConfig([]byte(tt.config))

			if err == nil || !strings.Contains(err.Error(), tt.error) {
				t.Errorf("ParseToolConfig() error = %v, expected it to contain %q", err, tt.error)
			}
		})
	}
}

func TestModeForPrefersAValidPropertyValue(t *testing.T) {
	grouping := &DirectoryGrouping{Default: "global", Property: "dependabot-grouping"}

	for _, tt := range []struct {
		property string
		mode     string
	}{
		{"per-app", "per-app"},
		{"global", "global"},
		{"", "global"},
		{"everything", "global"},
	} {
		if got := grouping.ModeFor(tt.property); got != tt.mode {
			t.Errorf("ModeFor(%q) = %q, expected %q", tt.property, got, tt.mode)
		}
	}
}

func TestModeForWithoutDirectoryGrouping(t *testing.T) {
	var grouping *DirectoryGrouping

	if got := grouping.ModeFor("per-app"); got != "" {
		t.Errorf("ModeFor() = %q, expected no mode", got)
	}
}

func TestLoadFilesAcceptsDirectoriesExceptions(t *testing.T) {
	dir := writeEnforceFiles(t, "- {repo: monorepo, fields: [directory-grouping], reason: apps deploy alone, owner: team-a}", "")
	enforce := Enforce{Fields: []string{"directory-grouping", "cooldown"}, ExceptionsFile: "exceptions.yml"}

	if err := enforce.LoadFiles(dir); err != nil {
		t.Fatalf("LoadFiles() failed: %v", err)
	}

	if got := enforce.FieldsFor("monorepo"); !reflect.DeepEqual(got, []string{"cooldown"}) {
		t.Errorf("FieldsFor() = %v, expected only cooldown", got)
	}
}
