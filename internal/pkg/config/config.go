// Package config contains functions related to config files
package config

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"github.com/google/go-github/v50/github"
	"gopkg.in/yaml.v3"
)

var (
	manifestFilePatterns      map[string]*regexp.Regexp
	manifestIgnoreFilePattern *regexp.Regexp
)

// InitializePatterns pre-compiles manifest file name patterns
func (config *ToolConfig) InitializePatterns() {
	manifestFilePatterns = map[string]*regexp.Regexp{}
	for key, pattern := range config.ManifestPatterns {
		manifestFilePatterns[key] = util.CompileRePattern(pattern)
	}
	manifestIgnoreFilePattern = nil
	if config.ManifestIgnorePattern != "" {
		manifestIgnoreFilePattern = util.CompileRePattern(config.ManifestIgnorePattern)
	}
}

// ToolConfig holds the tool's configuration defined in config.yml
type ToolConfig struct {
	UpdateDefaults        UpdateDefaults               `yaml:"update-defaults"`
	UpdateOverrides       map[string]UpdateDefaults    `yaml:"update-overrides"`
	Registries            map[string]DefaultRegistries `yaml:"registries"`
	ManifestPatterns      map[string]string            `yaml:"manifest-patterns"`
	ManifestIgnorePattern string                       `yaml:"manifest-ignore-pattern"`
	PullRequestParameters PullRequestParameters        `yaml:"pull-request-parameters"`
	StableGroupPrefixes   *bool                        `yaml:"stable-group-prefixes,omitempty"`
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
	Cooldown                      Cooldown      `yaml:"cooldown"`
}

// DependabotConfig holds the configuration defined in dependabot.yml
type DependabotConfig struct {
	Version              int                 `yaml:"version"`
	Registries           map[string]Registry `yaml:"registries,omitempty"`
	Updates              []Update            `yaml:"updates"`
	EnableBetaEcoSystems bool                `yaml:"enable-beta-ecosystems,omitempty"`
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
	PackageEcosystem              string           `yaml:"package-ecosystem"`
	Directory                     string           `yaml:"directory,omitempty"`
	Directories                   []string         `yaml:"directories,omitempty"`
	Schedule                      Schedule         `yaml:"schedule,omitempty"`
	Registries                    []string         `yaml:"registries,omitempty"`
	CommitMessage                 CommitMessage    `yaml:"commit-message,omitempty"`
	OpenPullRequestsLimit         int              `yaml:"open-pull-requests-limit,omitempty"`
	Assignees                     []string         `yaml:"assignees,omitempty"`
	Allow                         []Allow          `yaml:"allow,omitempty"`
	Ignore                        []Ignore         `yaml:"ignore,omitempty"`
	Groups                        map[string]Group `yaml:"groups,omitempty"`
	InsecureExternalCodeExecution string           `yaml:"insecure-external-code-execution,omitempty"`
	Labels                        []string         `yaml:"labels,omitempty"`
	Milestone                     int              `yaml:"milestone,omitempty"`
	PullRequestBranchName         struct {
		Separator string `yaml:"separator"`
	} `yaml:"pull-request-branch-name,omitempty"`
	RebaseStrategy     string   `yaml:"rebase-strategy,omitempty"`
	Reviewers          []string `yaml:"reviewers,omitempty"`
	TargetBranch       string   `yaml:"target-branch,omitempty"`
	Vendor             bool     `yaml:"vendor,omitempty"`
	VersioningStrategy string   `yaml:"versioning-strategy,omitempty"`
	Cooldown           Cooldown `yaml:"cooldown,omitempty"`
}

// Group holds the config items of a group definition
type Group struct {
	Separator       string   `yaml:"dependency-type,omitempty"`
	Patterns        []string `yaml:"patterns,omitempty"`
	ExcludePatterns []string `yaml:"exclude-patterns,omitempty"`
	UpdateTypes     []string `yaml:"update-types,omitempty"`
	AppliesTo       string   `yaml:"applies-to,omitempty"`
}

// Registry holds the config items of a registry definition
type Registry struct {
	Type         string `yaml:"type"`
	URL          string `yaml:"url"`
	Username     string `yaml:"username,omitempty"`
	Password     string `yaml:"password,omitempty"`
	Key          string `yaml:"key,omitempty"`
	Token        string `yaml:"token,omitempty"`
	ReplacesBase string `yaml:"replaces-base,omitempty"`
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

// Cooldown holds the cooldown configuration for different semver update types
type Cooldown struct {
	SemverMajorDays int      `yaml:"semver-major-days,omitempty"`
	SemverMinorDays int      `yaml:"semver-minor-days,omitempty"`
	SemverPatchDays int      `yaml:"semver-patch-days,omitempty"`
	DefaultDays     int      `yaml:"default-days,omitempty"`
	Include         []string `yaml:"include,omitempty"`
	Exclude         []string `yaml:"exclude,omitempty"`
}

// ChangeInfo holds the changes applied to a config.
type ChangeInfo struct {
	NewRegistries     []RegistryInfo
	RemovedRegistries []RegistryInfo
	NewUpdates        []UpdateInfo
	FixedUpdates      []UpdateInfo
	RemovedUpdates    []UpdateInfo
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

// CheckDirectoryExistsParameters holds all parameters needed for the CheckDirectoryExists function implementations.
type CheckDirectoryExistsParameters struct {
	GitHubClient *github.Client
	Org          string
	Repo         string
	Directory    string
}

// KeyValue holds a key/value pair of strings. Used as a sortable key/value map.
type KeyValue struct {
	Key   string
	Value string
}

// LoadFileContent is a function type for loading the content of a file.
type LoadFileContent func(file string, params LoadFileContentParameters) string

// CheckDirectoryExists is a function type for checking if a folder exists.
type CheckDirectoryExists func(directory string, params CheckDirectoryExistsParameters) bool

// Parse parses the config.yml format
func (config *ToolConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, config)
}

// Parse parses the dependabot.yml format
func (config *DependabotConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, config); err != nil {
		return err
	}
	for i, update := range config.Updates {
		if update.Directory != "/" && strings.HasSuffix(update.Directory, "/") {
			config.Updates[i].Directory = strings.TrimSuffix(update.Directory, "/")
		}
		if update.Directories != nil {
			for j, directory := range update.Directories {
				if directory != "/" && strings.HasSuffix(directory, "/") {
					config.Updates[i].Directories[j] = strings.TrimSuffix(directory, "/")
				}
			}
		}
	}
	return nil
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
func (config *DependabotConfig) IsManifestCovered(manifestFile string, manifestType string, updateRegistries []string) bool {
	if len(config.Updates) == 0 {
		return false
	}
	for i, update := range config.Updates {
		ecosystem := update.PackageEcosystem
		if ecosystem == "" || !hasDirectorySet(&update) {
			log.Printf("WARN  Invalid dependabot config: %v", update)
			continue
		}
		if ecosystem != manifestType {
			continue
		}
		manifestPath := PathWithEndingSlash(GetManifestPath(manifestFile, manifestType))
		if isPathCovered(manifestPath, manifestType, update.Directory, update.Directories) {
			// update entry is covering the one being checked
			// in case the latter is using registries, these must be referenced by this entry
			for _, name := range updateRegistries {
				// check if name in update -> []registries
				if !util.Contains(update.Registries, name) {
					config.Updates[i].Registries = append(config.Updates[i].Registries, name)
				}
			}
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

// PathWithEndingSlash returns a path with an added slash, if needed
func PathWithEndingSlash(path string) string {
	if path == "/" || path == "" {
		return "/"
	}
	if strings.HasSuffix(path, "/") {
		return path
	}
	return path + "/"
}

// GetManifestPath returns the path of the absolute path of a manifest file
func GetManifestPath(manifestFile string, manifestType string) string {
	if manifestType == "github-actions" {
		// special case for GitHub Actions
		return "/"
	}
	manifestPath, _ := filepath.Split("/" + manifestFile)
	if manifestPath == "/" {
		return "/"
	}
	return strings.TrimSuffix(manifestPath, "/")
}

// ProcessManifest adds config for a new manifest file to dependabot.yml if necessary
func (config *DependabotConfig) ProcessManifest(manifestFile string, manifestType string, toolConfig ToolConfig,
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
	manifestPath := GetManifestPath(manifestFile, manifestType)
	updateRegistries := make([]string, 0)

	// check if the default registries of the manifest's type are covered, and add them if necessary
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

	// check if the manifest itself is covered, and add it if necessary
	if !config.IsManifestCovered(manifestFile, manifestType, updateRegistries) {
		// create the new update section using the default properties
		update := createUpdateEntry(manifestType, manifestPath, toolConfig)
		// add new registries if required
		if len(updateRegistries) > 0 {
			update.Registries = updateRegistries
		}
		// add the update block, to the config
		config.Updates = append(config.Updates, update)
		changeInfo.NewUpdates = append(changeInfo.NewUpdates, UpdateInfo{Type: manifestType, Directory: manifestPath, File: manifestFile})
	}
}

// createUpdateEntry creates a new update entry for a manifest file
func createUpdateEntry(manifestType string, manifestPath string, toolConfig ToolConfig) Update {
	// Use cooldown configuration from config file
	cooldown := toolConfig.UpdateDefaults.Cooldown

	update := Update{
		PackageEcosystem:              manifestType,
		Directory:                     manifestPath,
		Schedule:                      toolConfig.UpdateDefaults.Schedule,
		CommitMessage:                 toolConfig.UpdateDefaults.CommitMessage,
		OpenPullRequestsLimit:         toolConfig.UpdateDefaults.OpenPullRequestsLimit,
		RebaseStrategy:                toolConfig.UpdateDefaults.RebaseStrategy,
		InsecureExternalCodeExecution: toolConfig.UpdateDefaults.InsecureExternalCodeExecution,
		Cooldown:                      cooldown,
	}
	// apply override properties, if defined
	if overrides, hasOverrides := toolConfig.UpdateOverrides[manifestType]; hasOverrides {
		applyOverrides(&update, overrides)
	}
	fixNewUpdateConfig(&update, manifestType)
	return update
}

// GetManifestType returns the type of manifest file, if any.
func GetManifestType(fullPath string) string {
	if manifestIgnoreFilePattern != nil && manifestIgnoreFilePattern.MatchString(fullPath) {
		return ""
	}
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
				manifests[fullPath] = manifestType
			}
		}
	}
}

// ToYaml returns a YAML representation of a dependabot config.
func (config *DependabotConfig) ToYaml() []byte {
	// sort entries in update list, to avoid commits due to changed order only
	// nothing to be done for registries, as yaml v3 marshals maps sorted by key
	if len(config.Updates) > 1 {
		sort.Slice(config.Updates, func(i, j int) bool {
			a := config.Updates[i]
			b := config.Updates[j]
			return (a.PackageEcosystem < b.PackageEcosystem) ||
				(a.PackageEcosystem == b.PackageEcosystem && a.Directory < b.Directory)
		})
	}
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
	loadFileFn LoadFileContent, loadFileParams LoadFileContentParameters, checkDirectoryExists CheckDirectoryExists,
	checkDirectoryExistsParams CheckDirectoryExistsParameters,
) ChangeInfo {
	changeInfo := ChangeInfo{
		NewRegistries:     []RegistryInfo{},
		RemovedRegistries: []RegistryInfo{},
		NewUpdates:        []UpdateInfo{},
		FixedUpdates:      []UpdateInfo{},
		RemovedUpdates:    []UpdateInfo{},
	}

	// Base directories must be processed before subdirectories (/ before /app).
	// Sort by length of path we must.
	manifestsSorted := make([]KeyValue, 0, len(manifests))
	for k, v := range manifests {
		manifestsSorted = append(manifestsSorted, KeyValue{k, v})
	}
	sort.SliceStable(manifestsSorted, func(i, j int) bool {
		path1, _ := filepath.Split("/" + manifestsSorted[i].Key)
		path2, _ := filepath.Split("/" + manifestsSorted[j].Key)
		return len(path1) < len(path2) || len(path1) == len(path2) && path1 < path2
	})

	// Remove updates with non-existing directories
	var existingUpdates []Update
	for _, update := range config.Updates {
		if checkDirectoryExists(update.Directory, checkDirectoryExistsParams) {
			existingUpdates = append(existingUpdates, update)
		} else {
			changeInfo.RemovedUpdates = append(changeInfo.RemovedUpdates, UpdateInfo{Type: update.PackageEcosystem, Directory: update.Directory, File: ""})
		}
	}
	config.Updates = existingUpdates

	// Fix existing updates, if necessary
	for i := range config.Updates {
		update := &config.Updates[i]
		fixed := false
		if fixExistingUpdateConfig(update) {
			fixed = true
		}
		if addCooldownToExistingUpdate(update, toolConfig) {
			fixed = true
		}
		if fixed {
			changeInfo.FixedUpdates = append(changeInfo.FixedUpdates, UpdateInfo{Type: update.PackageEcosystem, Directory: update.Directory, File: ""})
		}
	}

	// Iterate manifest files and check if they are covered by the current config file
	for _, manifest := range manifestsSorted {
		config.ProcessManifest(manifest.Key, manifest.Value, toolConfig, &changeInfo, loadFileFn, loadFileParams)
	}

	// Handle stable group prefixes if enabled
	if toolConfig.StableGroupPrefixes == nil || *toolConfig.StableGroupPrefixes {
		for i := range config.Updates {
			if len(config.Updates[i].Groups) > 0 {
				ensureStableGroupPrefixes(&config.Updates[i])
			}
		}
	}

	// Check if there are unused registries to be removed
	for name, registry := range config.Registries {
		found := false
		for _, update := range config.Updates {
			if util.Contains(update.Registries, name) {
				found = true
				break
			}
		}
		if !found {
			delete(config.Registries, name)
			changeInfo.RemovedRegistries = append(changeInfo.RemovedRegistries, RegistryInfo{Type: registry.Type, Name: name})
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
	if overrides.Cooldown.SemverMajorDays != 0 || overrides.Cooldown.SemverMinorDays != 0 || 
		overrides.Cooldown.SemverPatchDays != 0 || overrides.Cooldown.DefaultDays != 0 ||
		len(overrides.Cooldown.Include) > 0 || len(overrides.Cooldown.Exclude) > 0 {
		update.Cooldown = overrides.Cooldown
	}
}

// fixNewUpdateConfig fixes the config for a newly created Update, if necessary
func fixNewUpdateConfig(update *Update, manifestType string) {
	// remove "insecure-external-code-execution" if it is not allowed
	if update.InsecureExternalCodeExecution != "" && manifestType != "bundler" && manifestType != "mix" && manifestType != "pip" {
		update.InsecureExternalCodeExecution = ""
	}
}

// isPathCovered returns if a manifest file is covered within a dependabot.yml config
// special case for Dockerfiles, every subdirectory needs to be listed individually
func isPathCovered(manifestPath string, manifestType string, directory string, directories []string) bool {
	cmpFunc := strings.HasPrefix
	if manifestType == "docker" {
		// special case for Dockerfiles, every subdirectory needs to be listed individually
		cmpFunc = func(a, b string) bool { return a == b }
	}
	if directory != "" {
		return cmpFunc(PathWithEndingSlash(manifestPath), PathWithEndingSlash(directory))
	}
	if len(directories) > 0 {
		for _, directory := range directories {
			if cmpFunc(PathWithEndingSlash(manifestPath), PathWithEndingSlash(directory)) {
				return true
			}
		}
	}
	return false
}

// hasDirectorySet checks if a directory is set in an Update
func hasDirectorySet(update *Update) bool {
	if update.Directory != "" {
		return true
	}
	if len(update.Directories) > 0 {
		for _, directory := range update.Directories {
			if directory != "" {
				return true
			}
		}
	}
	return false
}

// fixNewUpdateConfig fixes the config for an existing Update, if necessary
func fixExistingUpdateConfig(update *Update) bool {
	// change path "" to "/"
	if !hasDirectorySet(update) {
		log.Printf("INFO  fixed empty directory in config for %v", update.PackageEcosystem)
		update.Directory = "/"
		update.Directories = nil
		return true
	}
	return false
}

// ensureStableGroupPrefixes ensures all group names have a unique numeric prefix (01_, 02_, 03_, etc.)
// If a group doesn't have a prefix, it adds one.
func ensureStableGroupPrefixes(update *Update) {
	if len(update.Groups) == 0 {
		return
	}

	// First collect all group names and check if they follow the pattern
	prefixRegex := regexp.MustCompile(`^(\d{2})_(.+)$`)

	// First check if we need to rename any groups
	needsRenaming := false
	existingPrefixes := make(map[string]bool)
	baseNameToOrigName := make(map[string]string)
	origNames := make([]string, 0, len(update.Groups))

	for name := range update.Groups {
		// Check if name already has a numeric prefix
		matches := prefixRegex.FindStringSubmatch(name)
		var baseName string

		if matches != nil {
			// Has a prefix, extract the base name and prefix
			prefix := matches[1]
			baseName = matches[2]

			if existingPrefixes[prefix] {
				// Duplicate prefix found, need to rename
				needsRenaming = true
			}
			existingPrefixes[prefix] = true
		} else {
			// No prefix found, need to rename
			baseName = name
			needsRenaming = true
		}

		baseNameToOrigName[baseName] = name
		origNames = append(origNames, name)
	}

	// If all groups already have unique prefixes, no need to change
	if !needsRenaming {
		return
	}

	// Sort original names for stable ordering
	sort.Strings(origNames)

	// Create a new map with properly prefixed groups
	newGroups := make(map[string]Group)
	for i, origName := range origNames {
		baseName := origName
		// If it has a prefix, extract the base name
		matches := prefixRegex.FindStringSubmatch(origName)
		if matches != nil {
			baseName = matches[2]
		}
		newName := fmt.Sprintf("%02d_%s", i+1, baseName)
		newGroups[newName] = update.Groups[origName]
	}

	// Replace the groups with the new prefixed map
	update.Groups = newGroups
}

// addCooldownToExistingUpdate adds cooldown configuration to existing updates that don't have it
func addCooldownToExistingUpdate(update *Update, toolConfig ToolConfig) bool {
	hasCooldonwConfig := update.Cooldown.SemverMajorDays != 0 && 
	                   update.Cooldown.SemverMinorDays != 0 && 
	                   update.Cooldown.SemverPatchDays != 0 && 
	                   update.Cooldown.DefaultDays != 0

	if hasCooldonwConfig {
		return false
	}

	// Preserve existing exclude/include lists, add default timing values
	existingExclude := update.Cooldown.Exclude
	existingInclude := update.Cooldown.Include
	if len(existingExclude) == 0 {
		existingExclude = []string{"@getyourguide*"}
	}

	// Add timing configuration from config file while preserving user's exclude/include
	update.Cooldown = Cooldown{
		SemverMajorDays: toolConfig.UpdateDefaults.Cooldown.SemverMajorDays,
		SemverMinorDays: toolConfig.UpdateDefaults.Cooldown.SemverMinorDays,
		SemverPatchDays: toolConfig.UpdateDefaults.Cooldown.SemverPatchDays,
		DefaultDays:     toolConfig.UpdateDefaults.Cooldown.DefaultDays,
		Include:         existingInclude,
		Exclude:         existingExclude,
	}

	return true
}
