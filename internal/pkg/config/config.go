// Package config contains functions related to config files
package config

import (
	"bytes"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"github.com/google/go-github/v50/github"
	"gopkg.in/yaml.v3"
)

var manifestFilePatterns map[string]*regexp.Regexp

// InitializePatterns pre-compiles manifest file name patterns
func InitializePatterns(patterns map[string]string) {
	manifestFilePatterns = map[string]*regexp.Regexp{}
	for key, pattern := range patterns {
		manifestFilePatterns[key] = util.CompileRePattern(pattern)
	}
}

// ToolConfig holds the tool's configuration defined in config.yml
type ToolConfig struct {
	UpdateDefaults        UpdateDefaults               `yaml:"update-defaults"`
	UpdateOverrides       map[string]UpdateDefaults    `yaml:"update-overrides"`
	Registries            map[string]DefaultRegistries `yaml:"registries"`
	ManifestPatterns      map[string]string            `yaml:"manifest-patterns"`
	PullRequestParameters PullRequestParameters        `yaml:"pull-request-parameters"`
}

// DefaultRegistries holds the default registries for new update definitions
type DefaultRegistries map[string]DefaultRegistry

// PullRequestParameters holds the parameters for PRs created by dependabutler
type PullRequestParameters struct {
	AuthorName             string `yaml:"author-name"`
	AuthorEmail            string `yaml:"author-email"`
	CommitMessage          string `yaml:"commit-message"`
	PRTitle                string `yaml:"pr-title"`
	BranchName             string `yaml:"branch-name"`
	BranchNameRandomSuffix bool   `yaml:"branch-name-random-suffix"`
	SleepAfterPRAction     int    `yaml:"sleep-after-pr-action"`
}

// DefaultRegistry holds the config items of a default registry
type DefaultRegistry struct {
	Type                    string   `yaml:"type"`
	URL                     string   `yaml:"url"`
	Username                string   `yaml:"username,omitempty"`
	Password                string   `yaml:"password,omitempty"`
	URLMatchRequired        bool     `yaml:"url-match-required,omitempty"`
	URLMatchAdditionalFiles []string `yaml:"url-match-additional-files,omitempty"`
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

// Allow holds the config items of an allow definition
type Allow struct {
	DependencyName string `yaml:"dependency-name,omitempty"`
	DependencyType string `yaml:"dependency-type,omitempty"`
}

// Ignore holds the config items of an ignore definition
type Ignore struct {
	DependencyName string   `yaml:"dependency-name"`
	Versions       []string `yaml:"versions,omitempty"`
	UpdateTypes    []string `yaml:"update-types,omitempty"`
}

// Update holds the config items of an update definition
type Update struct {
	PackageEcosystem              string        `yaml:"package-ecosystem"`
	Directory                     string        `yaml:"directory"`
	Schedule                      Schedule      `yaml:"schedule,omitempty"`
	Registries                    []string      `yaml:"registries,omitempty"`
	CommitMessage                 CommitMessage `yaml:"commit-message,omitempty"`
	OpenPullRequestsLimit         int           `yaml:"open-pull-requests-limit,omitempty"`
	Assignees                     []string      `yaml:"assignees,omitempty"`
	Allow                         []Allow       `yaml:"allow,omitempty"`
	Ignore                        []Ignore      `yaml:"ignore,omitempty"`
	InsecureExternalCodeExecution string        `yaml:"insecure-external-code-execution,omitempty"`
	Labels                        []string      `yaml:"labels,omitempty"`
	Milestone                     int           `yaml:"milestone,omitempty"`
	PullRequestBranchName         struct {
		Separator string `yaml:"separator"`
	} `yaml:"pull-request-branch-name,omitempty"`
	RebaseStrategy     string   `yaml:"rebase-strategy,omitempty"`
	Reviewers          []string `yaml:"reviewers,omitempty"`
	TargetBranch       string   `yaml:"target-branch,omitempty"`
	Vendor             bool     `yaml:"vendor,omitempty"`
	VersioningStrategy string   `yaml:"versioning-strategy,omitempty"`
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
	Day      string `yaml:"day,omitempty"`
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

// LoadFileContentParameters holds all parameters needed for the LoadFileContent function implementations.
type LoadFileContentParameters struct {
	GitHubClient *github.Client
	Org          string
	Repo         string
	Directory    string
}

// LoadFileContent is a function type for loading the content of a file.
type LoadFileContent func(file string, params LoadFileContentParameters) string

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

// IsRegistryUsed returns if a registry is used by a manifest file
func IsRegistryUsed(manifestFile string, manifestPath string, defaultRegistry DefaultRegistry,
	loadFileFn LoadFileContent, loadFileParams LoadFileContentParameters,
) bool {
	// check if registry is used for this manifest file - only add it if so
	registryURL, err := url.Parse(defaultRegistry.URL)
	if err != nil || registryURL.Hostname() == "" {
		log.Printf("ERROR default registry has invalid URL %v", defaultRegistry.URL)
		return false
	}
	// search the manifest file itself and - if defined - additional files
	searchFiles := []string{manifestFile}
	for _, additionalFile := range defaultRegistry.URLMatchAdditionalFiles {
		searchFiles = append(searchFiles, filepath.Join(manifestPath, additionalFile))
	}
	for _, searchFile := range searchFiles {
		fileContent := loadFileFn(searchFile, loadFileParams)
		if strings.Contains(fileContent, registryURL.Hostname()) {
			return true
		}
	}
	return false
}

// AddManifest adds config for a new manifest file to dependabot.yml
func (config *DependabotConfig) AddManifest(manifestFile string, manifestType string, toolConfig ToolConfig,
	changeInfo *ChangeInfo, loadFileFn LoadFileContent, loadFileParams LoadFileContentParameters,
) {
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
	if manifestType == "github-actions" {
		// special case for GitHub Actions
		manifestPath = "/"
	}
	updateRegistries := make([]string, 0)

	// check if one or more (default) registries are defined for this manifest type
	if defaultRegistries, containsRegistry := toolConfig.Registries[manifestType]; containsRegistry {
		for name, defaultRegistry := range defaultRegistries {
			if defaultRegistry.URLMatchRequired {
				// check if registry is used for this manifest file - only add it if so
				found := IsRegistryUsed(manifestFile, manifestPath, defaultRegistry, loadFileFn, loadFileParams)
				if !found {
					continue
				}
			}
			updateRegistries = append(updateRegistries, name)
			if _, contains := config.Registries[name]; !contains {
				// registry not yet in config -> add it
				config.Registries[name] = Registry{
					Type:     defaultRegistry.Type,
					URL:      defaultRegistry.URL,
					Username: defaultRegistry.Username,
					Password: defaultRegistry.Password,
				}
				changeInfo.NewRegistries = append(changeInfo.NewRegistries, RegistryInfo{Type: defaultRegistry.Type, Name: name})
			}
		}
	}
	// create the new update section using the default properties
	update := Update{
		PackageEcosystem:              manifestType,
		Directory:                     manifestPath,
		Schedule:                      toolConfig.UpdateDefaults.Schedule,
		CommitMessage:                 toolConfig.UpdateDefaults.CommitMessage,
		OpenPullRequestsLimit:         toolConfig.UpdateDefaults.OpenPullRequestsLimit,
		RebaseStrategy:                toolConfig.UpdateDefaults.RebaseStrategy,
		InsecureExternalCodeExecution: toolConfig.UpdateDefaults.InsecureExternalCodeExecution,
	}
	// apply override properties, if defined
	if overrides, hasOverrides := toolConfig.UpdateOverrides[manifestType]; hasOverrides {
		applyOverrides(&update, overrides)
	}
	fixUpdateConfig(&update, manifestType)

	// add new registries if required
	if len(updateRegistries) > 0 {
		update.Registries = updateRegistries
	}
	// add the update block, to the config
	config.Updates = append(config.Updates, update)
	changeInfo.NewUpdates = append(changeInfo.NewUpdates, UpdateInfo{Type: manifestType, Directory: manifestPath, File: manifestFile})
}

// GetManifestType returns the type of manifest file, if any.
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
func (config *DependabotConfig) UpdateConfig(manifests map[string]string, toolConfig ToolConfig,
	loadFileFn LoadFileContent, loadFileParams LoadFileContentParameters,
) ChangeInfo {
	changeInfo := ChangeInfo{
		NewRegistries: []RegistryInfo{},
		NewUpdates:    []UpdateInfo{},
	}

	// Iterate manifest files and check if they are covered by the current config file
	for manifestFile, manifestType := range manifests {
		if !config.IsManifestCovered(manifestFile, manifestType) {
			config.AddManifest(manifestFile, manifestType, toolConfig, &changeInfo, loadFileFn, loadFileParams)
		}
	}
	return changeInfo
}

// applyOverrides updates a config for an Update, using overridden values
func applyOverrides(update *Update, overrides UpdateDefaults) {
	if overrides.Schedule != (Schedule{}) {
		update.Schedule = overrides.Schedule
	}
	if overrides.CommitMessage != (CommitMessage{}) {
		update.CommitMessage = overrides.CommitMessage
	}
	if overrides.OpenPullRequestsLimit != 0 {
		update.OpenPullRequestsLimit = overrides.OpenPullRequestsLimit
	}
	if overrides.RebaseStrategy != "" {
		update.RebaseStrategy = overrides.RebaseStrategy
	}
	if overrides.InsecureExternalCodeExecution != "" {
		update.InsecureExternalCodeExecution = overrides.InsecureExternalCodeExecution
	}
}

// fixUpdateConfig fixes the config for an Update, if necessary
func fixUpdateConfig(update *Update, manifestType string) {
	// remove "insecure-external-code-execution" if it is not allowed
	if update.InsecureExternalCodeExecution != "" && manifestType != "bundler" && manifestType != "mix" && manifestType != "pip" {
		update.InsecureExternalCodeExecution = ""
	}
}
