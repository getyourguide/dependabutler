// Package config contains functions related to config files
package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"gopkg.in/yaml.v3"
)

var manifestFilePatterns map[string]*regexp.Regexp

// InitializePatterns precompiles manifest file name patterns
func InitializePatterns(patterns map[string]string) {
	manifestFilePatterns = map[string]*regexp.Regexp{}
	for key, pattern := range patterns {
		manifestFilePatterns[key] = util.CompileRePattern(pattern)
	}
}

// ToolConfig holds the tool's configuration defined in config.yml
type ToolConfig struct {
	UpdateDefaults        UpdateDefaults               `yaml:"update-defaults"`
	Registries            map[string]DefaultRegistries `yaml:"registries"`
	ManifestPatterns      map[string]string            `yaml:"manifest-patterns"`
	PullRequestParameters PullRequestParameters        `yaml:"pull-request-parameters"`
}

// DefaultRegistries holds the default registries for new update definitions
type DefaultRegistries map[string]Registry

// PullRequestParameters holds the parameters for PRs created by dependabutler
type PullRequestParameters struct {
	AuthorName    string `yaml:"author-name"`
	AuthorEmail   string `yaml:"author-email"`
	CommitMessage string `yaml:"commit-message"`
	PRTitle       string `yaml:"pr-title"`
	BranchName    string `yaml:"branch-name"`
}

// UpdateDefaults holds the default config for new update definitions
type UpdateDefaults struct {
	Schedule                      Schedule      `yaml:"schedule"`
	CommitMessage                 CommitMessage `yaml:"commit-message"`
	OpenPullRequestsLimit         int           `yaml:"open-pull-requests-limit"`
	InsecureExternalCodeExecution string        `yaml:"insecure-external-code-execution"`
	RebaseStrategy                string        `yaml:"rebase-strategy"`
}

// DependabotConfig holds the configuration defined in dependabot.yml
type DependabotConfig struct {
	Version    int                 `yaml:"version"`
	Registries map[string]Registry `yaml:"registries,omitempty"`
	Updates    []Update            `yaml:"updates"`
}

// Update holds the config items of an update definition
type Update struct {
	PackageEcosystem              string        `yaml:"package-ecosystem"`
	Directory                     string        `yaml:"directory"`
	InsecureExternalCodeExecution string        `yaml:"insecure-external-code-execution,omitempty"`
	Registries                    []string      `yaml:"registries,omitempty"`
	Schedule                      Schedule      `yaml:"schedule,omitempty"`
	CommitMessage                 CommitMessage `yaml:"commit-message,omitempty"`
	OpenPullRequestsLimit         int           `yaml:"open-pull-requests-limit,omitempty"`
	RebaseStrategy                string        `yaml:"rebase-strategy,omitempty"`
}

// Registry holds the config items of a registry definition
type Registry struct {
	Type     string `yaml:"type"`
	URL      string `yaml:"url"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// Schedule holds the config items of a schedule
type Schedule struct {
	Interval string `yaml:"interval"`
	Time     string `yaml:"time,omitempty"`
	Timezone string `yaml:"timezone,omitempty"`
}

// CommitMessage holds the config items for the commit message
type CommitMessage struct {
	Prefix            string `yaml:"prefix,omitempty"`
	PrefixDevelopment string `yaml:"prefix-development,omitempty"`
	Include           string `yaml:"include,omitempty"`
}

// ChangeInfo holds the changes applied to a config.
type ChangeInfo struct {
	NewRegistries []RegistryInfo
	NewUpdates    []UpdateInfo
}

// RegistryInfo holds the properties of a registry, for the change message.
type RegistryInfo struct {
	Type string
	Name string
}

// UpdateInfo holds the properties of an update, for the change message.
type UpdateInfo struct {
	Type      string
	Directory string
	File      string
}

// Parse parses the config.yml format
func (config *ToolConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, config)
}

// Parse parses the dependabot.yml format
func (config *DependabotConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, config)
}

// ParseToolConfig parses the config file
func ParseToolConfig(fileContent []byte) (*ToolConfig, error) {
	if fileContent == nil {
		return nil, nil
	}
	var config ToolConfig
	err := config.Parse(fileContent)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// ParseDependabotConfig parses the config file
func ParseDependabotConfig(fileContent []byte) (*DependabotConfig, error) {
	config := DependabotConfig{Version: 2}
	err := config.Parse(fileContent)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// IsManifestCovered returns if a manifest file is covered within a dependabot.yml config
func (config *DependabotConfig) IsManifestCovered(manifestFile string, manifestType string) bool {
	if config.Updates == nil || len(config.Updates) == 0 {
		return false
	}
	for _, update := range config.Updates {
		ecosystem := update.PackageEcosystem
		directory := update.Directory
		if ecosystem == "" || directory == "" {
			log.Printf("WARN  Invalid dependabot config: %v", update)
			return false
		}
		if ecosystem == manifestType && strings.HasPrefix("/"+manifestFile, directory) {
			return true
		}
	}
	return false
}

// AddManifest adds config for a new manifest file to dependabot.yml
func (config *DependabotConfig) AddManifest(manifestFile string, manifestType string, toolConfig ToolConfig, changeInfo *ChangeInfo) {
	if manifestFile == "" || manifestType == "" {
		return
	}
	if config.Updates == nil {
		config.Updates = []Update{}
	}
	if config.Registries == nil {
		config.Registries = map[string]Registry{}
	}
	manifestPath, _ := filepath.Split("/" + manifestFile)
	if manifestPath != "/" {
		manifestPath = strings.TrimSuffix(manifestPath, "/")
	}
	updateRegistries := []string{}

	// check if one or more (default) registries are defined for this manifest type
	if defaultRegistries, containsRegistry := toolConfig.Registries[manifestType]; containsRegistry {
		for name, defaultRegistry := range defaultRegistries {
			updateRegistries = append(updateRegistries, name)
			if _, contains := config.Registries[name]; !contains {
				// registry not yet in config -> add it
				config.Registries[name] = defaultRegistry
				changeInfo.NewRegistries = append(changeInfo.NewRegistries, RegistryInfo{Type: defaultRegistry.Type, Name: name})
			}
		}
	}
	// create the new update section and add it
	update := Update{
		PackageEcosystem:      manifestType,
		Directory:             manifestPath,
		Schedule:              toolConfig.UpdateDefaults.Schedule,
		CommitMessage:         toolConfig.UpdateDefaults.CommitMessage,
		OpenPullRequestsLimit: toolConfig.UpdateDefaults.OpenPullRequestsLimit,
		RebaseStrategy:        toolConfig.UpdateDefaults.RebaseStrategy,
	}
	if manifestType == "pip" {
		update.InsecureExternalCodeExecution = toolConfig.UpdateDefaults.InsecureExternalCodeExecution
	}
	if len(updateRegistries) > 0 {
		update.Registries = updateRegistries
	}
	config.Updates = append(config.Updates, update)
	changeInfo.NewUpdates = append(changeInfo.NewUpdates, UpdateInfo{Type: manifestType, Directory: manifestPath, File: manifestFile})
}

// GetManifestType returns the type of a manifest file, if any.
func GetManifestType(fullPath string) string {
	for manifestType, re := range manifestFilePatterns {
		if re.MatchString(fullPath) {
			return manifestType
		}
	}
	return ""
}

// ScanFileList looks for manifest files, in a list of file names (incl. path)
func ScanFileList(files []string, manifests map[string]string) {
	for _, fullPath := range files {
		manifestType := GetManifestType(fullPath)
		if manifestType != "" {
			manifests[fullPath] = manifestType
		}
	}
}

// ScanLocalDirectory lists all files in a directory, recursively
func ScanLocalDirectory(baseDirectory string, directory string, manifests map[string]string) {
	files, err := os.ReadDir(filepath.Join(baseDirectory, directory))
	if err != nil {
		log.Printf("ERROR Could not read directory %v: %v\n", directory, err)
		return
	}
	for _, file := range files {
		fullPath := filepath.Join(directory, file.Name())
		if file.IsDir() {
			ScanLocalDirectory(baseDirectory, fullPath, manifests)
		} else {
			manifestType := GetManifestType(fullPath)
			if manifestType != "" {
				manifests[file.Name()] = manifestType
			}
		}
	}
}

// ToYaml returns a YAML representation of a dependabot config.
func (config *DependabotConfig) ToYaml() []byte {
	buf := new(bytes.Buffer)
	encoder := yaml.NewEncoder(buf)
	encoder.SetIndent(2)
	err := encoder.Encode(config)
	if err != nil {
		log.Printf("ERROR Could not encode yml: %v", err)
	}
	// quote expressions like ${{secrets.MY_SECRET}} - after GitHub replaces variables, there might be quotes needed
	re := regexp.MustCompile(`(\$\{\{[^}]+\}\})`)
	rawString := buf.String()
	rawString = re.ReplaceAllString(rawString, `"$1"`)
	return []byte(rawString)
}

// UpdateConfig updates a dependabot config with a list of manifests found and the tool's config.
func (config *DependabotConfig) UpdateConfig(manifests map[string]string, toolConfig ToolConfig) ChangeInfo {
	changeInfo := ChangeInfo{
		NewRegistries: []RegistryInfo{},
		NewUpdates:    []UpdateInfo{},
	}

	// Iterate manifest files and check if they are covered by the current config file
	for manifestFile, manifestType := range manifests {
		if !config.IsManifestCovered(manifestFile, manifestType) {
			config.AddManifest(manifestFile, manifestType, toolConfig, &changeInfo)
		}
	}
	return changeInfo
}
