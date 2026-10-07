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

func groupDirectoriesWith(t *testing.T, dependabotYaml string, mode string, enforced []string, manifests map[string]string) (*DependabotConfig, ChangeInfo) {
	t.Helper()

	dependabotConfig, err := ParseDependabotConfig([]byte(dependabotYaml))
	if err != nil {
		t.Fatal(err)
	}

	toolConfig := ToolConfig{UpdateDefaults: UpdateDefaults{Schedule: Schedule{Interval: "weekly"}}}
	repoSettings := RepoSettings{EnforcedFields: enforced, DirectoryGrouping: mode}
	directoryExists := func(string, CheckDirectoryExistsParameters) bool { return true }
	changeInfo := dependabotConfig.UpdateConfig(manifests, toolConfig, repoSettings,
		LoadFileContentDummy, LoadFileContentParameters{}, directoryExists, CheckDirectoryExistsParameters{})

	return dependabotConfig, changeInfo
}

func updateLocations(updates []Update) []string {
	var locations []string
	for i := range updates {
		locations = append(locations, updates[i].PackageEcosystem+" "+updateDirectories(&updates[i]))
	}

	return locations
}

const dockerApps = `
version: 2
updates:
  - package-ecosystem: docker
    directory: /a
    registries: [first]
    schedule:
      interval: weekly
  - package-ecosystem: docker
    directory: /b
    registries: [first, second]
    schedule:
      interval: weekly
  - package-ecosystem: docker
    directory: /c
    schedule:
      interval: weekly
    ignore:
      - dependency-name: nginx
  - package-ecosystem: docker
    directory: /d
    target-branch: develop
    schedule:
      interval: weekly
`

func TestGlobalMergesCompatibleEntries(t *testing.T) {
	dependabotConfig, changeInfo := groupDirectoriesWith(t, dockerApps, "global", []string{"directory-grouping"}, nil)

	expected := []string{"docker /a, /b", "docker /c", "docker /d"}
	if got := updateLocations(dependabotConfig.Updates); !reflect.DeepEqual(got, expected) {
		t.Errorf("updates = %v, expected %v", got, expected)
	}

	merged := dependabotConfig.Updates[0]
	if !reflect.DeepEqual(merged.Registries, []string{"first", "second"}) {
		t.Errorf("registries = %v, expected both", merged.Registries)
	}
	if len(changeInfo.EnforcedUpdates) != 1 || changeInfo.EnforcedUpdates[0].Directory != "/a, /b" {
		t.Errorf("EnforcedUpdates = %+v, expected the merged entry", changeInfo.EnforcedUpdates)
	}
}

func TestGlobalMergeIsStableOnceWritten(t *testing.T) {
	merged, _ := groupDirectoriesWith(t, dockerApps, "global", []string{"directory-grouping"}, nil)

	_, changeInfo := groupDirectoriesWith(t, string(merged.ToYaml()), "global", []string{"directory-grouping"}, nil)

	if len(changeInfo.EnforcedUpdates) != 0 {
		t.Errorf("EnforcedUpdates = %+v on a file that was already merged", changeInfo.EnforcedUpdates)
	}
}

func TestPerAppSplitsDirectories(t *testing.T) {
	dependabotConfig, changeInfo := groupDirectoriesWith(t, `
version: 2
updates:
  - package-ecosystem: npm
    directories: [/a, /b]
    registries: [first]
    schedule:
      interval: weekly
  - package-ecosystem: gomod
    directories: ["/lib-*", /tools]
    schedule:
      interval: weekly
`, "per-app", []string{"directory-grouping"}, nil)

	expected := []string{"npm /a", "npm /b", "gomod /lib-*, /tools"}
	if got := updateLocations(dependabotConfig.Updates); !reflect.DeepEqual(got, expected) {
		t.Errorf("updates = %v, expected %v", got, expected)
	}

	dependabotConfig.Updates[0].Registries[0] = "changed"
	if !reflect.DeepEqual(dependabotConfig.Updates[1].Registries, []string{"first"}) {
		t.Errorf("split entries share their registries: %v", dependabotConfig.Updates[1].Registries)
	}
	if len(changeInfo.EnforcedUpdates) != 1 || changeInfo.EnforcedUpdates[0].Directory != "/a, /b" {
		t.Errorf("EnforcedUpdates = %+v, expected the split entry", changeInfo.EnforcedUpdates)
	}
}

func TestDirectoriesNotEnforcedLeavesExistingEntries(t *testing.T) {
	dependabotConfig, _ := groupDirectoriesWith(t, dockerApps, "global", nil, nil)

	if len(dependabotConfig.Updates) != 4 {
		t.Errorf("updates = %v, expected the four entries unchanged", updateLocations(dependabotConfig.Updates))
	}
}

const dockerApp = `
version: 2
updates:
  - package-ecosystem: docker
    directory: /a
    schedule:
      interval: weekly
`

func TestGlobalAddsNewManifestsToACompatibleEntry(t *testing.T) {
	dependabotConfig, changeInfo := groupDirectoriesWith(t, dockerApp, "global", nil, map[string]string{"b/Dockerfile": "docker"})

	if got := updateLocations(dependabotConfig.Updates); !reflect.DeepEqual(got, []string{"docker /a, /b"}) {
		t.Errorf("updates = %v, expected /b added to the existing entry", got)
	}
	if len(changeInfo.NewUpdates) != 1 || changeInfo.NewUpdates[0].Directory != "/b" {
		t.Errorf("NewUpdates = %+v, expected /b", changeInfo.NewUpdates)
	}
}

func TestGlobalAddsNewManifestsAsEntriesWhenNoneIsCompatible(t *testing.T) {
	incompatible := strings.Replace(dockerApp, "    schedule:", "    commit-message: {prefix: deps}\n    schedule:", 1)

	dependabotConfig, _ := groupDirectoriesWith(t, incompatible, "global", nil, map[string]string{"b/Dockerfile": "docker"})

	if got := updateLocations(dependabotConfig.Updates); !reflect.DeepEqual(got, []string{"docker /a", "docker /b"}) {
		t.Errorf("updates = %v, expected a new entry for /b", got)
	}
}

func TestPerAppAddsNewManifestsAsEntries(t *testing.T) {
	dependabotConfig, _ := groupDirectoriesWith(t, dockerApp, "per-app", nil, map[string]string{"b/Dockerfile": "docker"})

	if got := updateLocations(dependabotConfig.Updates); !reflect.DeepEqual(got, []string{"docker /a", "docker /b"}) {
		t.Errorf("updates = %v, expected a new entry for /b", got)
	}
}

func TestUpdateConfigRemovesMissingDirectoriesOfAnEntry(t *testing.T) {
	dependabotConfig, err := ParseDependabotConfig([]byte(`
version: 2
updates:
  - package-ecosystem: docker
    directories: [/a, /gone, "/lib-*"]
    schedule:
      interval: weekly
  - package-ecosystem: npm
    directories: [/gone, /also-gone]
    schedule:
      interval: weekly
`))
	if err != nil {
		t.Fatal(err)
	}
	exists := func(directory string, _ CheckDirectoryExistsParameters) bool {
		return !strings.Contains(directory, "gone")
	}

	changeInfo := dependabotConfig.UpdateConfig(nil, ToolConfig{}, RepoSettings{},
		LoadFileContentDummy, LoadFileContentParameters{}, exists, CheckDirectoryExistsParameters{})

	if got := updateLocations(dependabotConfig.Updates); !reflect.DeepEqual(got, []string{"docker /a, /lib-*"}) {
		t.Errorf("updates = %v, expected only the existing directories", got)
	}
	if len(changeInfo.RemovedUpdates) != 3 {
		t.Errorf("RemovedUpdates = %+v, expected the three missing directories", changeInfo.RemovedUpdates)
	}
}

func TestPerAppDoesNotSplitIntoDirectoriesOfOtherEntries(t *testing.T) {
	overlapping := `
version: 2
updates:
  - package-ecosystem: docker
    directories: [/a, /b]
    schedule:
      interval: weekly
  - package-ecosystem: docker
    directory: /a
    schedule:
      interval: daily
`

	dependabotConfig, _ := groupDirectoriesWith(t, overlapping, "per-app", []string{"directory-grouping"}, nil)

	if got := updateLocations(dependabotConfig.Updates); !reflect.DeepEqual(got, []string{"docker /a, /b", "docker /a"}) {
		t.Errorf("updates = %v, expected the entries left as they are", got)
	}
}

func TestToYamlOrdersEntriesWithSeveralDirectories(t *testing.T) {
	dependabotConfig, err := ParseDependabotConfig([]byte(`
version: 2
updates:
  - package-ecosystem: npm
    directories: [/x, /y]
  - package-ecosystem: npm
    directories: [/a, /b]
`))
	if err != nil {
		t.Fatal(err)
	}

	got := string(dependabotConfig.ToYaml())

	if strings.Index(got, "- /a") > strings.Index(got, "- /x") {
		t.Errorf("ToYaml() did not order the entries by their directories:\n%v", got)
	}
}

func TestGlobalAddsNewManifestsToEntriesWithUnprefixedGroups(t *testing.T) {
	dependabotConfig, err := ParseDependabotConfig([]byte(`
version: 2
updates:
  - package-ecosystem: docker
    directory: /a
    schedule:
      interval: weekly
    groups:
      minor-patch:
        patterns: ["*"]
`))
	if err != nil {
		t.Fatal(err)
	}
	toolConfig := ToolConfig{UpdateDefaults: UpdateDefaults{
		Schedule: Schedule{Interval: "weekly"},
		Groups:   Groups{{Name: "minor-patch", Group: Group{Patterns: []string{"*"}}}},
	}}
	exists := func(string, CheckDirectoryExistsParameters) bool { return true }

	dependabotConfig.UpdateConfig(map[string]string{"b/Dockerfile": "docker"}, toolConfig, RepoSettings{DirectoryGrouping: "global"},
		LoadFileContentDummy, LoadFileContentParameters{}, exists, CheckDirectoryExistsParameters{})

	if got := updateLocations(dependabotConfig.Updates); !reflect.DeepEqual(got, []string{"docker /a, /b"}) {
		t.Errorf("updates = %v, expected /b added to the existing entry", got)
	}
}
