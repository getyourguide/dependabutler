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
