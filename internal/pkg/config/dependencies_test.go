package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
)

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"*", "anything", true},
		{"lodash", "lodash", true},
		{"lodash", "lodash-es", false},
		{"Lodash", "lodash", true},
		{"org/actions*", "org/actions", true},
		{"org/actions*", "org/actions/setup", true},
		{"org/actions/*", "org/actions", false},
		{"*/setup", "org/actions/setup", true},
		{"@types/*", "@types/node", true},
		{"com.example:*-core", "com.example:billing-core", true},
		{"com.example:*-core", "com.example:billing-api", false},
		{"a*b*c", "abc", true},
		{"a*b*c", "acb", false},
		{"a*a", "a", false},
		{"*/checkout", "actions/checkout", true},
		{"**", "anything", true},
		{"", "", true},
		{"", "lodash", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+" "+tt.name, func(t *testing.T) {
			if got := matchesPattern(tt.pattern, tt.name); got != tt.want {
				t.Errorf("matchesPattern(%q, %q) = %v, expected %v", tt.pattern, tt.name, got, tt.want)
			}
		})
	}
}

func TestEcosystemsOfPackageURL(t *testing.T) {
	tests := []struct {
		packageURL string
		want       []string
	}{
		{"pkg:githubactions/org/actions/setup@v1", []string{"github-actions"}},
		{"pkg:npm/%40types/node@20.0.0", []string{"bun", "npm"}},
		{"pkg:maven/com.example/core@1.0", []string{"gradle", "maven"}},
		{"pkg:github/org/repo@main", nil},
		{"https://example.com", nil},
	}

	for _, tt := range tests {
		t.Run(tt.packageURL, func(t *testing.T) {
			if got := EcosystemsOfPackageURL(tt.packageURL); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EcosystemsOfPackageURL(%q) = %v, expected %v", tt.packageURL, got, tt.want)
			}
		})
	}
}

func onlyIfUsedToolConfig() ToolConfig {
	return ToolConfig{
		StableGroupPrefixes: util.Ptr(false),
		UpdateDefaults: UpdateDefaults{
			Schedule: Schedule{Interval: "weekly"},
			Groups:   Groups{{Name: "minor-patch", Group: Group{Patterns: []string{"*"}, UpdateTypes: []string{"minor", "patch"}}}},
		},
		UpdateOverrides: map[string]UpdateDefaults{
			"github-actions": {Groups: Groups{
				{Name: "shared-actions", Group: Group{Patterns: []string{"org/actions*"}, ExcludePatterns: []string{"org/actions/legacy"}, OnlyIfUsed: true}},
				{Name: "minor-patch", Group: Group{Patterns: []string{"*"}, UpdateTypes: []string{"minor", "patch"}}},
			}},
		},
	}
}

func TestCreateUpdateEntryKeepsOnlyIfUsedGroupsThatMatch(t *testing.T) {
	tests := []struct {
		name         string
		dependencies Dependencies
		want         []string
	}{
		{"matching dependency", Dependencies{"github-actions": {"actions/checkout", "org/actions/setup"}}, []string{"shared-actions", "minor-patch"}},
		{"no matching dependency", Dependencies{"github-actions": {"actions/checkout"}}, []string{"minor-patch"}},
		{"matching dependency of another ecosystem", Dependencies{"npm": {"org/actions/setup"}}, []string{"minor-patch"}},
		{"only an excluded dependency", Dependencies{"github-actions": {"org/actions/legacy"}}, []string{"minor-patch"}},
		{"dependencies not read", nil, []string{"minor-patch"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update := createUpdateEntry("github-actions", "/", onlyIfUsedToolConfig(), RepoSettings{Dependencies: tt.dependencies})

			if got := groupNames(update.Groups); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("groups = %v, expected %v", got, tt.want)
			}
			for _, named := range update.Groups {
				if named.Group.OnlyIfUsed {
					t.Errorf("group %q keeps only-if-used, which Dependabot does not know", named.Name)
				}
			}
		})
	}
}

func TestCreateUpdateEntryLeavesOtherEcosystemsAlone(t *testing.T) {
	update := createUpdateEntry("npm", "/", onlyIfUsedToolConfig(), RepoSettings{Dependencies: Dependencies{}})

	if got := groupNames(update.Groups); !reflect.DeepEqual(got, []string{"minor-patch"}) {
		t.Errorf("groups = %v, expected the default groups", got)
	}
}

func TestStableGroupPrefixesDoNotDependOnTheUsedGroups(t *testing.T) {
	toolConfig := onlyIfUsedToolConfig()
	toolConfig.StableGroupPrefixes = util.Ptr(true)

	unused := createUpdateEntry("github-actions", "/", toolConfig, RepoSettings{Dependencies: Dependencies{}})
	used := createUpdateEntry("github-actions", "/", toolConfig, RepoSettings{Dependencies: Dependencies{"github-actions": {"org/actions/setup"}}})

	if got := groupNames(unused.Groups); !reflect.DeepEqual(got, []string{"02_minor-patch"}) {
		t.Errorf("groups without the dependency = %v, expected [02_minor-patch]", got)
	}
	if got := groupNames(used.Groups); !reflect.DeepEqual(got, []string{"01_shared-actions", "02_minor-patch"}) {
		t.Errorf("groups with the dependency = %v, expected [01_shared-actions 02_minor-patch]", got)
	}
}

func TestWrittenConfigHasNoOnlyIfUsed(t *testing.T) {
	dependabotConfig := DependabotConfig{Version: 2, Updates: []Update{
		createUpdateEntry("github-actions", "/", onlyIfUsedToolConfig(), RepoSettings{Dependencies: Dependencies{"github-actions": {"org/actions/setup"}}}),
	}}

	if written := string(dependabotConfig.ToYaml()); strings.Contains(written, "only-if-used") {
		t.Errorf("ToYaml() wrote only-if-used:\n%v", written)
	}
}

const actionsEntryWithSharedGroup = `
version: 2
updates:
  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: weekly
    groups:
      shared-actions:
        patterns:
          - org/actions*
        exclude-patterns:
          - org/actions/legacy
      minor-patch:
        patterns:
          - "*"
        update-types:
          - minor
          - patch
`

func TestEnforcedGroupsFollowTheDependencies(t *testing.T) {
	tests := []struct {
		name         string
		dependencies Dependencies
		wantGroups   []string
		wantEnforced bool
	}{
		{"used group is kept", Dependencies{"github-actions": {"org/actions/setup"}}, []string{"shared-actions", "minor-patch"}, false},
		{"unused group is removed", Dependencies{"github-actions": {"actions/checkout"}}, []string{"minor-patch"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dependabotConfig, err := ParseDependabotConfig([]byte(actionsEntryWithSharedGroup))
			if err != nil {
				t.Fatal(err)
			}

			repoSettings := RepoSettings{EnforcedFields: []string{EnforceGroups}, Dependencies: tt.dependencies}
			directoryExists := func(string, CheckDirectoryExistsParameters) (bool, error) { return true, nil }
			changeInfo, err := dependabotConfig.UpdateConfig(map[string]string{}, onlyIfUsedToolConfig(), repoSettings,
				LoadFileContentDummy, LoadFileContentParameters{}, directoryExists, CheckDirectoryExistsParameters{})
			if err != nil {
				t.Fatal(err)
			}

			if got := groupNames(dependabotConfig.Updates[0].Groups); !reflect.DeepEqual(got, tt.wantGroups) {
				t.Errorf("groups = %v, expected %v", got, tt.wantGroups)
			}
			if enforced := len(changeInfo.EnforcedUpdates) > 0; enforced != tt.wantEnforced {
				t.Errorf("enforced = %v, expected %v", enforced, tt.wantEnforced)
			}
		})
	}
}
