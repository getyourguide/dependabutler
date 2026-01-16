package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"go.yaml.in/yaml/v4"
)

func TestParseToolConfig(t *testing.T) {
	for _, tt := range []struct {
		configString string
		expected     *ToolConfig
	}{
		{``, &ToolConfig{}},
		{
			`
update-defaults:
  schedule:
    interval: daily
    time: 06:00
    timezone: Europe/Berlin
  open-pull-requests-limit: 10
  insecure-external-code-execution: allow

update-overrides:
  docker:
    schedule:
      interval: weekly
      time: 08:00

registries:
  npm:
    npm-reg:
      type: npm-registry
      url: https://npm.foo.bar
      username: usr
      password: "${{secrets.PASSWORD}}"
  docker:
    docker-1:
      type: docker-registry-1
      url: https://docker.bar.foo
      username: dockeruser
      password: dockerpass
    docker-2:
      type: docker-registry-2
      url: https://docker.foo.bar
      username: dockeruser2
      password: dockerpass2
      url-match-required: true
`,
			&ToolConfig{
				UpdateDefaults: UpdateDefaults{
					OpenPullRequestsLimit:         util.Ptr(10),
					InsecureExternalCodeExecution: "allow",
					Schedule: Schedule{
						Interval: "daily",
						Time:     "06:00",
						Timezone: "Europe/Berlin",
					},
				},
				UpdateOverrides: map[string]UpdateDefaults{
					"docker": {
						Schedule: Schedule{
							Interval: "weekly",
							Time:     "08:00",
						},
					},
				},
				Registries: map[string]DefaultRegistries{
					"npm": map[string]DefaultRegistry{
						"npm-reg": {Type: "npm-registry", URL: "https://npm.foo.bar", Username: "usr", Password: "${{secrets.PASSWORD}}"},
					},
					"docker": map[string]DefaultRegistry{
						"docker-1": {Type: "docker-registry-1", URL: "https://docker.bar.foo", Username: "dockeruser", Password: "dockerpass"},
						"docker-2": {Type: "docker-registry-2", URL: "https://docker.foo.bar", Username: "dockeruser2", Password: "dockerpass2", URLMatchRequired: true},
					},
				},
			},
		},
		{
			`
update-defaults:
  schedule:
    interval: daily
  open-pull-requests-limit: 0
`,
			&ToolConfig{
				UpdateDefaults: UpdateDefaults{
					OpenPullRequestsLimit: util.Ptr(0),
					Schedule: Schedule{
						Interval: "daily",
					},
				},
			},
		},
		{
			`
update-defaults:
  schedule:
    interval: daily
`,
			&ToolConfig{
				UpdateDefaults: UpdateDefaults{
					OpenPullRequestsLimit: nil,
					Schedule: Schedule{
						Interval: "daily",
					},
				},
			},
		},
	} {
		got, err := ParseToolConfig([]byte(tt.configString))
		if err != nil {
			t.Errorf("ParseDependabotConfig() failed;\n  parsing error %v", err)
		} else if !reflect.DeepEqual(tt.expected, got) {
			t.Errorf("ParseToolConfig() failed;\n  expected %v\n  got      %v", tt.expected, got)
		}
	}
}

func TestParseDependabotConfig(t *testing.T) {
	for _, tt := range []struct {
		configString string
		expected     *DependabotConfig
	}{
		{``, &DependabotConfig{Version: 2}},
		{
			`version: 2
registries:
  docker-registry:
    type: docker
    url: https://docker.foo.bar
    username: "usr"
    password: "${{secrets.DOCKER_PASS}}"
updates:
- package-ecosystem: docker
  directory: "/"
- package-ecosystem: npm
  directory: "/npm/stuff/here"
- package-ecosystem: npm
  directory: "/npm/other"
- package-ecosystem: github-actions
  directory: "/"
`,
			&DependabotConfig{
				Version: 2,
				Updates: []Update{
					{PackageEcosystem: "docker", Directory: "/"},
					{PackageEcosystem: "npm", Directory: "/npm/stuff/here"},
					{PackageEcosystem: "npm", Directory: "/npm/other"},
					{PackageEcosystem: "github-actions", Directory: "/"},
				},
				Registries: map[string]Registry{
					"docker-registry": {
						Type:     "docker",
						URL:      "https://docker.foo.bar",
						Username: "usr",
						Password: "${{secrets.DOCKER_PASS}}",
					},
				},
			},
		},
	} {
		got, err := ParseDependabotConfig([]byte(tt.configString))
		if err != nil {
			t.Errorf("ParseDependabotConfig() failed;\n  parsing error %v", err)
		} else if !reflect.DeepEqual(tt.expected, got) {
			t.Errorf("ParseDependabotConfig() failed;\n  expected %v\n  got      %v", tt.expected, got)
		}
	}
}

func TestParseDependabotConfigWithDirectories(t *testing.T) {
	for _, tt := range []struct {
		configString string
		expected     string
	}{
		{
			`version: 2
updates:
  - package-ecosystem: docker
    directories:
      - "/one"
      - "/two"
      - "/three"
`,
			`version: 2
updates:
- package-ecosystem: docker
  directories:
  - /one
  - /two
  - /three
`,
		},
	} {
		parsedConfig, err := ParseDependabotConfig([]byte(tt.configString))
		if err != nil {
			t.Errorf("TestParseDependabotConfigWithDirectories() failed;\n  parsing error %v", err)
		}
		got := (string)(parsedConfig.ToYaml())
		if tt.expected != got {
			t.Errorf("TestParseDependabotConfigWithDirectories() failed;\n  expected \n%v\n  got      \n%v\n", tt.expected, got)
		}
	}
}

func TestIsManifestCoveredWithDirectory(t *testing.T) {
	config := DependabotConfig{
		Updates: []Update{
			{PackageEcosystem: "docker", Directory: "/"},
			{PackageEcosystem: "npm", Directory: "/npm/stuff/here"},
			{PackageEcosystem: "pip", Directory: "/pip1"},
			{PackageEcosystem: "pip", Directory: "/pip2/"},
			{PackageEcosystem: "composer", Directory: "/app"},
			{PackageEcosystem: "github-actions", Directory: "/"},
		},
	}
	for _, tt := range []struct {
		manifestType string
		manifestFile string
		expected     bool
	}{
		{"", "", false},
		{"", "dummy.txt", false},
		{"dummy", "", false},
		{"dummy", "dummy.txt", false},
		{"composer", "composer.json", false},
		{"docker", "Dockerfile", true},
		{"docker", "sub/dir/Dockerfile", false},
		{"pip", "pip1/requirements.txt", true},
		{"pip", "pip1/sub/requirements.txt", true},
		{"pip", "pip2/requirements.txt", true},
		{"pip", "pip2/sub/requirements.txt", true},
		{"pip", "pip123/requirements.txt", false},
		{"pip", "pip123/sub/requirements.txt", false},
		{"pip", "requirements.txt", false},
		{"composer", "app2/requirements.txt", false},
		{"npm", "npm/stuff/here/package.json", true},
		{"npm", "npm/stuff/here/sub/dir/package.json", true},
		{"npm", "npm/stuff/not_here/package.json", false},
		{"github-actions", ".github/workflows/action.yml", true},
	} {
		got := config.IsManifestCovered(tt.manifestFile, tt.manifestType, []string{})
		if tt.expected != got {
			t.Errorf("IsManifestCovered(%v, %v) failed; expected %t got %t", tt.manifestType, tt.manifestFile, tt.expected, got)
		}
	}
}

func TestIsManifestCoveredWithDirectories(t *testing.T) {
	config := DependabotConfig{
		Updates: []Update{
			{PackageEcosystem: "docker", Directories: []string{"/", "/something-else"}},
			{PackageEcosystem: "npm", Directories: []string{"/npm/stuff/here", "/npm/other"}},
			{PackageEcosystem: "pip", Directories: []string{"/pip1", "/pip2/"}},
			{PackageEcosystem: "composer", Directories: []string{"/app"}},
			{PackageEcosystem: "github-actions", Directories: []string{"/"}},
		},
	}

	for _, tt := range []struct {
		manifestType string
		manifestFile string
		expected     bool
	}{
		{"", "", false},
		{"", "dummy.txt", false},
		{"dummy", "", false},
		{"dummy", "dummy.txt", false},
		{"composer", "composer.json", false},
		{"docker", "Dockerfile", true},
		{"docker", "sub/dir/Dockerfile", false},
		{"pip", "pip1/requirements.txt", true},
		{"pip", "pip1/sub/requirements.txt", true},
		{"pip", "pip2/requirements.txt", true},
		{"pip", "pip2/sub/requirements.txt", true},
		{"pip", "pip123/requirements.txt", false},
		{"pip", "pip123/sub/requirements.txt", false},
		{"pip", "requirements.txt", false},
		{"composer", "app2/requirements.txt", false},
		{"npm", "npm/stuff/here/package.json", true},
		{"npm", "npm/stuff/here/sub/dir/package.json", true},
		{"npm", "npm/stuff/not_here/package.json", false},
		{"github-actions", ".github/workflows/action.yml", true},
	} {
		got := config.IsManifestCovered(tt.manifestFile, tt.manifestType, []string{})
		if tt.expected != got {
			t.Errorf("IsManifestCovered(%v, %v) failed; expected %t got %t", tt.manifestType, tt.manifestFile, tt.expected, got)
		}
	}
}

func LoadFileContentDummy(_ string, _ LoadFileContentParameters) string {
	return "dummy"
}

func TestAddManifest(t *testing.T) {
	config := DependabotConfig{}
	toolConfig := ToolConfig{
		UpdateDefaults: UpdateDefaults{
			Schedule: Schedule{
				Interval: "daily",
				Time:     "18:15",
				Timezone: "Europe/Berlin",
			},
			OpenPullRequestsLimit: util.Ptr(9),
		},
		UpdateOverrides: map[string]UpdateDefaults{
			"docker": {
				Schedule: Schedule{
					Interval: "weekly",
				},
			},
		},
		Registries: map[string]DefaultRegistries{},
	}

	for _, tt := range []struct {
		manifestType     string
		manifestFile     string
		expectedCount    int
		expectedPath     string
		expectedInterval string
	}{
		{"", "", 0, "", ""},
		{"pip", "", 0, "", ""},
		{"", "requirements.txt", 0, "", ""},
		{"pip", "requirements.txt", 1, "/", "daily"},
		{"docker", "app/Dockerfile", 2, "/app", "weekly"},
		{"docker", "other_app/sub/folder/Dockerfile", 3, "/other_app/sub/folder", "weekly"},
	} {
		changeInfo := ChangeInfo{}
		config.ProcessManifest(tt.manifestFile, tt.manifestType, toolConfig, &changeInfo, LoadFileContentDummy, LoadFileContentParameters{})
		// check the number of expected elements
		gotCount := len(config.Updates)
		if gotCount != tt.expectedCount {
			t.Errorf("AddManifest(%v, %v) failed; expected count %v got %v", tt.manifestType, tt.manifestFile, tt.expectedCount, gotCount)
		}
		if tt.expectedPath != "" {
			// check if the expected path has been added, for the manifest type
			foundPath := false
			for _, update := range config.Updates {
				if update.PackageEcosystem == tt.manifestType && update.Directory == tt.expectedPath {
					foundPath = true
				}
			}
			if !foundPath {
				t.Errorf("AddManifest(%v, %v) failed; couldn't find path %v after adding", tt.manifestType, tt.manifestFile, tt.expectedPath)
			}
		}
		if tt.expectedInterval != "" {
			// check if the expected path has been added, for the manifest type
			for _, update := range config.Updates {
				if update.PackageEcosystem == tt.manifestType && update.Directory == tt.expectedPath {
					if tt.expectedInterval != update.Schedule.Interval {
						t.Errorf("AddManifest(%v, %v) failed; expected interval %v got %v", tt.manifestType, tt.manifestFile, tt.expectedInterval, update.Schedule.Interval)
					}
				}
			}
		}
	}
}

func TestGetManifestType(t *testing.T) {
	// initialize patterns
	manifestFilePatterns := map[string]string{
		"npm":            "(.*/)?package\\.json",
		"maven":          "(.*/)?pom\\.xml",
		"pip":            "(.*/)?requirements\\.txt",
		"docker":         "(.*/)?Dockerfile",
		"gomod":          "(.*/)?go\\.mod",
		"composer":       "(.*/)?composer\\.json",
		"gradle":         "(.*/)?build\\.gradle(\\.kts)?",
		"github-actions": "\\.github/workflows/.*\\.yml",
	}

	config := ToolConfig{
		ManifestPatterns: manifestFilePatterns,
	}
	config.InitializePatterns()

	for _, tt := range []struct {
		fullPath string
		expected string
	}{
		{"", ""},
		{"README.md", ""},
		{"Dockerfile", "docker"},
		{"foo/bar/package.json", "npm"},
		{"foo/bar/requirements.txt", "pip"},
		{"foo/pom.xml", "maven"},
		{"foo/build.gradle", "gradle"},
		{"foo/composer.json", "composer"},
		{"module/package/go.mod", "gomod"},
		{".github/workflows/action.yml", "github-actions"},
	} {
		got := GetManifestType(tt.fullPath)
		if tt.expected != got {
			t.Errorf("GetManifestType() failed for %v : expected '%v', got '%v'", tt.fullPath, tt.expected, got)
		}
	}
}

func TestHasDirectorySet(t *testing.T) {
	for _, tt := range []struct {
		update   Update
		expected bool
	}{
		{Update{Directory: ""}, false},
		{Update{Directory: "/"}, true},
		{Update{Directory: "/etc"}, true},
		{Update{Directory: "/etc/shadow"}, true},
		{Update{Directories: []string{"", ""}}, false},
		{Update{Directories: []string{"", "/", ""}}, true},
		{Update{Directories: []string{"/"}}, true},
		{Update{Directories: []string{"/var/lib/kubelet"}}, true},
		{Update{Directories: []string{"/etc/kubernetes/manifests", "/opt"}}, true},
		{Update{Directory: "", Directories: []string{""}}, false},
		{Update{Directory: "", Directories: []string{"/"}}, true},
		{Update{Directory: "", Directories: []string{"/applicationsupport/internetexplorer"}}, true},
		{Update{Directory: "/", Directories: []string{""}}, true},
		{Update{Directory: "/home/joe", Directories: []string{""}}, true},
		{Update{Directory: "/", Directories: []string{"/"}}, true},
		{Update{Directory: "/", Directories: []string{"/home/joe"}}, true},
	} {
		got := hasDirectorySet(&tt.update)
		if tt.expected != got {
			t.Errorf("hasDirectorySet(%v) failed; expected %t got %t", tt.update, tt.expected, got)
		}
	}
}

func TestGroupAppliesTo(t *testing.T) {
	// Test unmarshaling YAML with applies-to field
	yamlContent := `
patterns:
  - "lodash*"
exclude-patterns:
  - "lodash-es"
update-types:
  - minor
  - patch
applies-to: security-updates
`

	var group Group
	err := yaml.Unmarshal([]byte(yamlContent), &group)
	if err != nil {
		t.Errorf("Failed to unmarshal YAML: %v", err)
	}

	// Check that the AppliesTo field was correctly unmarshaled
	if group.AppliesTo != "security-updates" {
		t.Errorf("Expected AppliesTo to be 'security-updates', got '%s'", group.AppliesTo)
	}

	// Test unmarshaling YAML without applies-to field
	yamlWithoutAppliesTo := `
separator: ":"
patterns:
  - "react*"
update-types:
  - "major"
`
	var groupWithoutAppliesTo Group
	err = yaml.Unmarshal([]byte(yamlWithoutAppliesTo), &groupWithoutAppliesTo)
	if err != nil {
		t.Errorf("Failed to unmarshal YAML: %v", err)
	}

	// Check that the AppliesTo field is empty when not in YAML
	if groupWithoutAppliesTo.AppliesTo != "" {
		t.Errorf("Expected AppliesTo to be empty, got '%s'", groupWithoutAppliesTo.AppliesTo)
	}

	// Test marshaling Group with AppliesTo field
	groupWithAppliesTo := Group{
		Separator:       ",",
		Patterns:        []string{"axios", "fetch-mock"},
		ExcludePatterns: []string{"@axios/types"},
		UpdateTypes:     []string{"major", "minor"},
		AppliesTo:       "version-updates",
	}

	marshaled, err := yaml.Marshal(groupWithAppliesTo)
	if err != nil {
		t.Errorf("Failed to marshal Group: %v", err)
	}

	// Unmarshal to verify the applies-to field is included
	var unmarshaledGroup Group
	err = yaml.Unmarshal(marshaled, &unmarshaledGroup)
	if err != nil {
		t.Errorf("Failed to unmarshal marshaled YAML: %v", err)
	}

	if unmarshaledGroup.AppliesTo != "version-updates" {
		t.Errorf("Expected AppliesTo to be 'version-updates', got '%s'", unmarshaledGroup.AppliesTo)
	}

	// Test marshaling Group with empty AppliesTo field - should be omitted in output
	groupWithEmptyAppliesTo := Group{
		Separator:   "/",
		Patterns:    []string{"webpack*"},
		UpdateTypes: []string{"patch"},
	}

	marshaledEmpty, err := yaml.Marshal(groupWithEmptyAppliesTo)
	if err != nil {
		t.Errorf("Failed to marshal Group: %v", err)
	}

	// Convert to string to check if applies-to is present
	marshaledStr := string(marshaledEmpty)
	if strings.Contains(marshaledStr, "applies-to") {
		t.Errorf("Expected 'applies-to' to be omitted, but it was found in the marshaled YAML: %s", marshaledStr)
	}
}

func TestEnsureStableGroupPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		update   Update
		expected map[string]Group
	}{
		{
			name: "No groups",
			update: Update{
				PackageEcosystem: "npm",
				Directory:        "/",
				Groups:           nil,
			},
			expected: nil,
		},
		{
			name: "Empty groups",
			update: Update{
				PackageEcosystem: "npm",
				Directory:        "/",
				Groups:           map[string]Group{},
			},
			expected: map[string]Group{},
		},
		{
			name: "Already prefixed groups",
			update: Update{
				PackageEcosystem: "npm",
				Directory:        "/",
				Groups: map[string]Group{
					"01_frontend": {
						Patterns: []string{"react*", "vue*"},
					},
					"02_backend": {
						Patterns: []string{"express*", "fastify*"},
					},
				},
			},
			expected: map[string]Group{
				"01_frontend": {
					Patterns: []string{"react*", "vue*"},
				},
				"02_backend": {
					Patterns: []string{"express*", "fastify*"},
				},
			},
		},
		{
			name: "Mixed prefixed and non-prefixed groups",
			update: Update{
				PackageEcosystem: "npm",
				Directory:        "/",
				Groups: map[string]Group{
					"01_frontend": {
						Patterns: []string{"react*", "vue*"},
					},
					"backend": {
						Patterns: []string{"express*", "fastify*"},
					},
				},
			},
			expected: map[string]Group{
				"01_frontend": {
					Patterns: []string{"react*", "vue*"},
				},
				"02_backend": {
					Patterns: []string{"express*", "fastify*"},
				},
			},
		},
		{
			name: "All non-prefixed groups",
			update: Update{
				PackageEcosystem: "npm",
				Directory:        "/",
				Groups: map[string]Group{
					"frontend": {
						Patterns: []string{"react*", "vue*"},
					},
					"backend": {
						Patterns: []string{"express*", "fastify*"},
					},
					"tooling": {
						Patterns: []string{"webpack*", "babel*"},
					},
				},
			},
			expected: map[string]Group{
				"01_backend": {
					Patterns: []string{"express*", "fastify*"},
				},
				"02_frontend": {
					Patterns: []string{"react*", "vue*"},
				},
				"03_tooling": {
					Patterns: []string{"webpack*", "babel*"},
				},
			},
		},
		{
			name: "Duplicate prefixes",
			update: Update{
				PackageEcosystem: "npm",
				Directory:        "/",
				Groups: map[string]Group{
					"01_frontend": {
						Patterns: []string{"react*", "vue*"},
					},
					"01_backend": {
						Patterns: []string{"express*", "fastify*"},
					},
				},
			},
			expected: map[string]Group{
				"01_backend": {
					Patterns: []string{"express*", "fastify*"},
				},
				"02_frontend": {
					Patterns: []string{"react*", "vue*"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of the update to avoid modifying the test case
			update := tt.update

			ensureStableGroupPrefixes(&update)

			// Check if the groups match the expected values
			if !reflect.DeepEqual(update.Groups, tt.expected) {
				t.Errorf("ensureStableGroupPrefixes() failed\nExpected: %v\nGot:      %v", tt.expected, update.Groups)
			}
		})
	}
}

// Test behavior when update-missing-cooldown-settings is false
func TestCoolDownWithUpdateFlagFalse(t *testing.T) {
	t.Run("Existing cooldown - no changes", func(t *testing.T) {
		// dependabutler.yml config
		toolConfig := ToolConfig{
			UpdateMissingCooldownSettings: func() *bool { b := false; return &b }(),
			UpdateDefaults: UpdateDefaults{
				Cooldown: Cooldown{
					SemverMajorDays: 21,
					SemverMinorDays: 7,
					SemverPatchDays: 3,
					DefaultDays:     20,
				},
			},
		}

		// repo dependabot.yml
		update := Update{
			PackageEcosystem: "npm",
			Directory:        "/",
			Cooldown: Cooldown{
				DefaultDays: 10,
			},
		}

		addCooldownToExistingUpdate(&update, toolConfig)

		expected := Cooldown{
			DefaultDays: 10, // preserved existing value
		}

		if !reflect.DeepEqual(update.Cooldown, expected) {
			t.Errorf("Expected cooldown %+v, got %+v", expected, update.Cooldown)
		}
	})

	t.Run("flag nil - should act like false", func(t *testing.T) {
		toolConfig := ToolConfig{
			UpdateMissingCooldownSettings: nil,
			UpdateDefaults: UpdateDefaults{
				Cooldown: Cooldown{
					DefaultDays: 10,
				},
			},
		}

		// repo dependabot.yml
		update := Update{
			PackageEcosystem: "npm",
			Directory:        "/",
			Cooldown: Cooldown{
				SemverPatchDays: 3,
			},
		}

		addCooldownToExistingUpdate(&update, toolConfig)

		expected := Cooldown{
			SemverPatchDays: 3,
		}

		if !reflect.DeepEqual(update.Cooldown, expected) {
			t.Errorf("Expected cooldown %+v, got %+v", expected, update.Cooldown)
		}
	})

	t.Run("No cooldown - no changes", func(t *testing.T) {
		// dependabutler.yml config
		toolConfig := ToolConfig{
			UpdateMissingCooldownSettings: func() *bool { b := false; return &b }(),
			UpdateDefaults: UpdateDefaults{
				Cooldown: Cooldown{
					SemverMajorDays: 21,
					SemverMinorDays: 7,
					SemverPatchDays: 3,
					DefaultDays:     20,
				},
			},
		}

		// repo dependabot.yml with no cooldown
		update := Update{
			PackageEcosystem: "npm",
			Directory:        "/",
			Cooldown:         Cooldown{}, // empty cooldown
		}

		addCooldownToExistingUpdate(&update, toolConfig)

		// Expected: no changes, cooldown remains empty
		expected := Cooldown{}

		if !reflect.DeepEqual(update.Cooldown, expected) {
			t.Errorf("Expected cooldown %+v, got %+v", expected, update.Cooldown)
		}
	})

	// New manifests should still add cooldown (flag only affects existing entries)
	t.Run("New manifest gets cooldown", func(t *testing.T) {
		// dependabutler.yml config
		toolConfig := ToolConfig{
			UpdateMissingCooldownSettings: func() *bool { b := false; return &b }(),
			UpdateDefaults: UpdateDefaults{
				Cooldown: Cooldown{
					SemverMajorDays: 21,
					SemverMinorDays: 7,
					SemverPatchDays: 3,
					DefaultDays:     20,
				},
			},
		}

		update := createUpdateEntry("npm", "/", toolConfig)

		expected := Cooldown{
			SemverMajorDays: 21,
			SemverMinorDays: 7,
			SemverPatchDays: 3,
			DefaultDays:     20,
		}

		if !reflect.DeepEqual(update.Cooldown, expected) {
			t.Errorf("Expected cooldown %+v, got %+v", expected, update.Cooldown)
		}
	})
}

// Test behavior when update-missing-cooldown-settings is true
func TestCoolDownWithUpdateFlagTrue(t *testing.T) {
	t.Run("Partial cooldown gets missing values", func(t *testing.T) {
		// dependabutler.yml config
		toolConfig := ToolConfig{
			UpdateMissingCooldownSettings: func() *bool { b := true; return &b }(),
			UpdateDefaults: UpdateDefaults{
				Cooldown: Cooldown{
					DefaultDays: 10,
				},
			},
		}

		// repo dependabot.yml
		update := Update{
			PackageEcosystem: "npm",
			Directory:        "/",
			Cooldown: Cooldown{
				SemverPatchDays: 3,
			},
		}

		addCooldownToExistingUpdate(&update, toolConfig)

		expected := Cooldown{
			SemverPatchDays: 3,  // preserved existing value
			DefaultDays:     10, // added from config
		}

		if !reflect.DeepEqual(update.Cooldown, expected) {
			t.Errorf("Expected cooldown %+v, got %+v", expected, update.Cooldown)
		}
	})

	t.Run("No cooldown gets all values", func(t *testing.T) {
		// dependabutler.yml config
		toolConfig := ToolConfig{
			UpdateMissingCooldownSettings: func() *bool { b := true; return &b }(),
			UpdateDefaults: UpdateDefaults{
				Cooldown: Cooldown{
					SemverMajorDays: 21,
					SemverMinorDays: 7,
					SemverPatchDays: 3,
					DefaultDays:     20,
				},
			},
		}

		// repo dependabot.yml
		update := Update{
			PackageEcosystem: "npm",
			Directory:        "/",
			Cooldown:         Cooldown{}, // empty cooldown
		}

		addCooldownToExistingUpdate(&update, toolConfig)

		expected := Cooldown{
			SemverMajorDays: 21, // added from config
			SemverMinorDays: 7,  // added from config
			SemverPatchDays: 3,  // added from config
			DefaultDays:     20, // added from config
		}

		if !reflect.DeepEqual(update.Cooldown, expected) {
			t.Errorf("Expected cooldown %+v, got %+v", expected, update.Cooldown)
		}
	})
}

func TestFixExistingUpdateConfigWithReviewers(t *testing.T) {
	tests := []struct {
		name     string
		update   Update
		expected Update
		modified bool
	}{
		{
			name: "No reviewers to remove",
			update: Update{
				PackageEcosystem: "npm",
				Directory:        "/",
			},
			expected: Update{
				PackageEcosystem: "npm",
				Directory:        "/",
			},
			modified: false,
		},
		{
			name: "Remove reviewers from update",
			update: Update{
				PackageEcosystem: "npm",
				Directory:        "/",
				Reviewers:        []string{"user1", "user2"},
			},
			expected: Update{
				PackageEcosystem: "npm",
				Directory:        "/",
				Reviewers:        nil,
			},
			modified: true,
		},
		{
			name: "Fix empty directory and remove reviewers",
			update: Update{
				PackageEcosystem: "docker",
				Directory:        "",
				Reviewers:        []string{"user3"},
			},
			expected: Update{
				PackageEcosystem: "docker",
				Directory:        "/",
				Reviewers:        nil,
			},
			modified: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of the update to avoid modifying the test case
			update := tt.update

			modified := fixExistingUpdateConfig(&update)

			// Check if the update matches the expected values
			if !reflect.DeepEqual(update, tt.expected) {
				t.Errorf("fixExistingUpdateConfig() failed\nExpected: %+v\nGot:      %+v", tt.expected, update)
			}

			// Check if the modified flag is correct
			if modified != tt.modified {
				t.Errorf("fixExistingUpdateConfig() modified flag failed\nExpected: %t\nGot:      %t", tt.modified, modified)
			}
		})
	}
}

func TestToYamlWithEmojis(t *testing.T) {
	// Test that emojis and other Unicode characters are preserved correctly
	config := DependabotConfig{
		Version: 2,
		Registries: map[string]Registry{
			"npm-registry": {
				Type:     "npm-registry",
				URL:      "https://npm.example.com",
				Username: "user",
				Password: "${{secrets.NPM_TOKEN}}",
			},
		},
		Updates: []Update{
			{
				PackageEcosystem:      "npm",
				Directory:             "/",
				OpenPullRequestsLimit: intPtr(5),
				Schedule: Schedule{
					Interval: "daily",
				},
				CommitMessage: CommitMessage{
					Prefix: "🔧 deps:",
				},
			},
			{
				PackageEcosystem: "docker",
				Directory:        "/app",
				Schedule: Schedule{
					Interval: "weekly",
				},
				CommitMessage: CommitMessage{
					Prefix: "🐳 docker:",
				},
			},
		},
	}

	yamlBytes := config.ToYaml()
	yamlString := string(yamlBytes)

	// Verify emojis are preserved
	if !contains(yamlString, "🔧 deps:") {
		t.Errorf("ToYaml() failed to preserve emoji '🔧' in commit message prefix")
	}
	if !contains(yamlString, "🐳 docker:") {
		t.Errorf("ToYaml() failed to preserve emoji '🐳' in commit message prefix")
	}

	// Verify the YAML can be parsed back correctly
	parsedConfig, err := ParseDependabotConfig(yamlBytes)
	if err != nil {
		t.Errorf("ToYaml() produced invalid YAML that cannot be parsed: %v", err)
	}

	// Verify emojis survive round-trip
	// Note: Updates are sorted by package-ecosystem, so docker comes before npm
	if len(parsedConfig.Updates) >= 1 && parsedConfig.Updates[0].CommitMessage.Prefix != "🐳 docker:" {
		t.Errorf("ToYaml() round-trip failed to preserve emoji in first update. Expected '🐳 docker:', got '%v'", parsedConfig.Updates[0].CommitMessage.Prefix)
	}
	if len(parsedConfig.Updates) >= 2 && parsedConfig.Updates[1].CommitMessage.Prefix != "🔧 deps:" {
		t.Errorf("ToYaml() round-trip failed to preserve emoji in second update. Expected '🔧 deps:', got '%v'", parsedConfig.Updates[1].CommitMessage.Prefix)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func intPtr(i int) *int {
	return &i
}

func TestIsEnvVarReference(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"Empty string", "", false},
		{"Regular URL", "https://example.com", false},
		{"Secret reference", "${{secrets.MY_SECRET}}", true},
		{"Secret with underscores", "${{secrets.JFROG_ARTIFACTORY_URL}}", true},
		{"Secret in middle of string", "prefix ${{secrets.TOKEN}} suffix", true},
		{"Multiple secrets", "${{secrets.USER}} ${{secrets.PASS}}", true},
		{"Not a secret - missing closing braces", "${{secrets.INCOMPLETE", false},
		{"Not a secret - missing opening braces", "secrets.INCOMPLETE}}", false},
		{"Environment variable", "${{env.MY_VAR}}", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEnvVarReference(tt.value)
			if got != tt.expected {
				t.Errorf("isEnvVarReference(%q) = %v, expected %v", tt.value, got, tt.expected)
			}
		})
	}
}

func TestIsRegistryUsed(t *testing.T) {
	loadFileWithHostname := func(hostname string) LoadFileContent {
		return func(_ string, _ LoadFileContentParameters) string {
			return "some content with " + hostname + " in it"
		}
	}

	tests := []struct {
		name         string
		manifestFile string
		manifestPath string
		registry     DefaultRegistry
		loadFileFn   LoadFileContent
		expected     bool
		description  string
	}{
		{
			name:         "Environment variable URL should return true",
			manifestFile: "Dockerfile",
			manifestPath: "/",
			registry: DefaultRegistry{
				Type:             "docker-registry",
				URL:              "${{secrets.JFROG_ARTIFACTORY_URL}}",
				URLMatchRequired: true,
			},
			loadFileFn:  LoadFileContentDummy,
			expected:    true,
			description: "When URL is an environment variable reference, registry should be included",
		},
		{
			name:         "Valid URL with hostname match",
			manifestFile: "Dockerfile",
			manifestPath: "/",
			registry: DefaultRegistry{
				Type:             "docker-registry",
				URL:              "https://docker.example.com",
				URLMatchRequired: true,
			},
			loadFileFn:  loadFileWithHostname("docker.example.com"),
			expected:    true,
			description: "When hostname is found in file, should return true",
		},
		{
			name:         "Valid URL without hostname match",
			manifestFile: "Dockerfile",
			manifestPath: "/",
			registry: DefaultRegistry{
				Type:             "docker-registry",
				URL:              "https://docker.example.com",
				URLMatchRequired: true,
			},
			loadFileFn:  LoadFileContentDummy,
			expected:    false,
			description: "When hostname is not found in file, should return false",
		},
		{
			name:         "Invalid URL",
			manifestFile: "Dockerfile",
			manifestPath: "/",
			registry: DefaultRegistry{
				Type:             "docker-registry",
				URL:              "not a valid url",
				URLMatchRequired: true,
			},
			loadFileFn:  LoadFileContentDummy,
			expected:    false,
			description: "Invalid URL should return false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRegistryUsed(tt.manifestFile, tt.manifestPath, tt.registry, tt.loadFileFn, LoadFileContentParameters{})
			if got != tt.expected {
				t.Errorf("IsRegistryUsed() = %v, expected %v. %s", got, tt.expected, tt.description)
			}
		})
	}
}
