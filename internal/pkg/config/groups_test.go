package config

import (
	"reflect"
	"strings"
	"testing"
)

const unsortedGroups = `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: weekly
    groups:
      minor-patch:
        patterns:
          - '*'
        update-types:
          - minor
          - patch
      development-major:
        dependency-type: development
        update-types:
          - major
      across-apps:
        patterns:
          - react*
        group-by: dependency-name
`

func TestToYamlKeepsTheWrittenGroupOrder(t *testing.T) {
	dependabotConfig, err := ParseDependabotConfig([]byte(unsortedGroups))
	if err != nil {
		t.Fatalf("ParseDependabotConfig() failed: %v", err)
	}

	got := string(dependabotConfig.ToYaml())

	if got != unsortedGroups {
		t.Errorf("ToYaml() changed the groups\nExpected:\n%v\nGot:\n%v", unsortedGroups, got)
	}
}

func TestParseDependabotConfigRejectsRepeatedGroupNames(t *testing.T) {
	repeated := strings.Replace(unsortedGroups, "across-apps:", "minor-patch:", 1)

	_, err := ParseDependabotConfig([]byte(repeated))

	if err == nil || !strings.Contains(err.Error(), `"minor-patch"`) {
		t.Errorf("ParseDependabotConfig() error = %v, expected it to name the repeated group", err)
	}
}

func TestGroupsByName(t *testing.T) {
	groups := Groups{{Name: "first", Group: Group{Patterns: []string{"a*"}}}, {Name: "second"}}

	group, found := groups.Get("first")
	_, missing := groups.Get("third")

	if !found || group.Patterns[0] != "a*" || missing {
		t.Errorf("Get() = %+v, %v and %v, expected the first group, true and false", group, found, missing)
	}
}

func groupNames(groups Groups) []string {
	var names []string
	for _, named := range groups {
		names = append(names, named.Name)
	}

	return names
}

func TestToYamlQuotesGroupNamesThatLookLikeOtherValues(t *testing.T) {
	dependabotConfig, err := ParseDependabotConfig([]byte("version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    groups:\n      'yes':\n        patterns: ['*']\n      '1':\n        patterns: ['a*']\n"))
	if err != nil {
		t.Fatalf("ParseDependabotConfig() failed: %v", err)
	}

	got := string(dependabotConfig.ToYaml())

	if !strings.Contains(got, "'yes':") || !strings.Contains(got, `"1":`) {
		t.Errorf("ToYaml() did not quote the group names:\n%v", got)
	}
}

func TestParseDependabotConfigRejectsMergeKeysInGroups(t *testing.T) {
	_, err := ParseDependabotConfig([]byte("version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    groups:\n      <<: {all: {patterns: ['*']}}\n"))

	if err == nil || !strings.Contains(err.Error(), "<<") {
		t.Errorf("ParseDependabotConfig() error = %v, expected it to reject the merge key", err)
	}
}

func TestEnsureStableGroupPrefixesNumbersInTheWrittenOrder(t *testing.T) {
	update := Update{Groups: Groups{
		{Name: "security", Group: Group{Patterns: []string{"*"}, UpdateTypes: []string{"patch"}}},
		{Name: "all", Group: Group{Patterns: []string{"*"}}},
	}}

	ensureStableGroupPrefixes(&update)

	if got := groupNames(update.Groups); !reflect.DeepEqual(got, []string{"01_security", "02_all"}) {
		t.Errorf("groups = %v, expected the written order kept", got)
	}
}

const groupsToolConfig = `
stable-group-prefixes: false
update-defaults:
  groups:
    minor-patch:
      patterns: ["*"]
      update-types: [minor, patch]
update-overrides:
  npm:
    groups:
      minor-patch:
        patterns: ["*"]
        update-types: [minor, patch]
      development-major:
        dependency-type: development
        update-types: [major]
`

func TestCreateUpdateEntryGetsTheConfiguredGroups(t *testing.T) {
	toolConfig, err := ParseToolConfig([]byte(groupsToolConfig))
	if err != nil {
		t.Fatalf("ParseToolConfig() failed: %v", err)
	}

	npm := createUpdateEntry("npm", "/", *toolConfig, nil)
	gomod := createUpdateEntry("gomod", "/", *toolConfig, nil)

	if got := groupNames(npm.Groups); !reflect.DeepEqual(got, []string{"minor-patch", "development-major"}) {
		t.Errorf("npm groups = %v, expected the override groups in their order", got)
	}
	if got := groupNames(gomod.Groups); !reflect.DeepEqual(got, []string{"minor-patch"}) {
		t.Errorf("gomod groups = %v, expected the default groups", got)
	}
	if group, _ := npm.Groups.Get("development-major"); group.DependencyType != "development" {
		t.Errorf("development-major = %+v, expected dependency-type development", group)
	}
}

func TestCreateUpdateEntryPrefixesGroupsLikeExistingEntries(t *testing.T) {
	toolConfig, err := ParseToolConfig([]byte(strings.Replace(groupsToolConfig, "stable-group-prefixes: false", "", 1)))
	if err != nil {
		t.Fatalf("ParseToolConfig() failed: %v", err)
	}

	npm := createUpdateEntry("npm", "/", *toolConfig, nil)

	if got := groupNames(npm.Groups); !reflect.DeepEqual(got, []string{"01_minor-patch", "02_development-major"}) {
		t.Errorf("npm groups = %v, expected the prefixed names stable-group-prefixes writes", got)
	}
}

func TestParseToolConfigRejectsInvalidGroups(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config string
		error  string
	}{
		{
			"dependency-type in the defaults",
			"update-defaults:\n  groups:\n    dev: {dependency-type: development}\n",
			"update-defaults",
		},
		{
			"dependency-type on an ecosystem without it",
			"update-overrides:\n  gomod:\n    groups:\n      dev: {dependency-type: development}\n",
			"gomod",
		},
		{
			"catch-all before a narrower group",
			"update-defaults:\n  groups:\n    everything: {patterns: ['*']}\n    react: {patterns: ['react*']}\n",
			`"everything"`,
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

func TestParseToolConfigAcceptsACatchAllPerUpdateType(t *testing.T) {
	config := `
update-defaults:
  groups:
    react: {patterns: ['react*']}
    everything: {patterns: ['*']}
    security: {patterns: ['*'], applies-to: security-updates}
`

	if _, err := ParseToolConfig([]byte(config)); err != nil {
		t.Errorf("ParseToolConfig() failed: %v", err)
	}
}

func TestParseToolConfigDropsUnknownGroupKeys(t *testing.T) {
	toolConfig, err := ParseToolConfig([]byte(`
stable-group-prefixes: false
update-defaults:
  groups:
    all: {patterns: ["*"], exclude-pattern: [x], group-by: dependency-name}
update-overrides:
  npm:
    groups:
      all: {patterns: ["*"], update-type: [minor]}
`))
	if err != nil {
		t.Fatalf("ParseToolConfig() failed: %v", err)
	}

	gomod, _ := createUpdateEntry("gomod", "/", *toolConfig, nil).Groups.Get("all")
	npm, _ := createUpdateEntry("npm", "/", *toolConfig, nil).Groups.Get("all")

	if gomod.Unknown != nil || npm.Unknown != nil {
		t.Errorf("unknown group keys kept: gomod %v, npm %v", gomod.Unknown, npm.Unknown)
	}
	if gomod.GroupBy != "dependency-name" {
		t.Errorf("group-by = %q, expected dependency-name", gomod.GroupBy)
	}
}
