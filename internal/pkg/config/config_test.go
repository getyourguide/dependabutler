package config

import (
	"reflect"
	"testing"
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
					OpenPullRequestsLimit:         10,
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
  registry: "docker-registry"
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
		{"docker", "sub/dir/Dockerfile", true},
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
			{PackageEcosystem: "github-actions", Directories: []string{"/"}}},
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
		{"docker", "sub/dir/Dockerfile", true},
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
			OpenPullRequestsLimit: 9,
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
