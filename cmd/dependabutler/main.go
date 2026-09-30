package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getyourguide/dependabutler/internal/pkg/config"
	"github.com/getyourguide/dependabutler/internal/pkg/githubapi"
	"github.com/getyourguide/dependabutler/internal/pkg/util"
)

// LoadRemoteFileContent is the implementation of LoadFileContent, for remote files (GitHub).
func LoadRemoteFileContent(file string, params config.LoadFileContentParameters) string {
	content, err := params.Client.GetFileContent(params.Org, params.Repo, file, "")
	if err != nil {
		return ""
	}
	return string(content)
}

// LoadLocalFileContent is the implementation of LoadFileContent, for local files (file system).
func LoadLocalFileContent(file string, params config.LoadFileContentParameters) string {
	fullPath := filepath.Join(params.Directory, file)
	content, err := util.ReadFile(fullPath)
	if err != nil {
		return ""
	}
	return string(content)
}

// CheckRemoteDirectoryExists is the implementation of CheckFolderExists, for remote directories (GitHub).
func CheckRemoteDirectoryExists(directory string, params config.CheckDirectoryExistsParameters) bool {
	exists, err := params.Client.CheckDirectoryExists(params.Org, params.Repo, directory, "")
	if err != nil {
		return false
	}
	return exists
}

// CheckLocalDirectoryExists is the implementation of CheckFolderExists, for local directories (file system).
func CheckLocalDirectoryExists(directory string, params config.CheckDirectoryExistsParameters) bool {
	fullPath := filepath.Join(params.Directory, directory)
	info, err := os.Stat(fullPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func showUsageAndExit() {
	flag.Usage()
	os.Exit(1)
}

// sanitizeRepoName strips line breaks from a user-provided repository name.
// Line breaks are not valid in GitHub repository names, and removing them makes
// sure a crafted name cannot forge additional log entries when it is logged
// (CodeQL: go/log-injection).
func sanitizeRepoName(repo string) string {
	repo = strings.ReplaceAll(repo, "\n", "")
	return strings.ReplaceAll(repo, "\r", "")
}

func getParameters() (string, string, bool, string, string, string, string) {
	var mode, dir, repo, repoFile, org, configFile string
	var execute bool
	flag.StringVar(&mode, "mode", "local", "local or remote")
	flag.StringVar(&configFile, "configFile", "dependabutler.yml", "location of tool config file")
	flag.BoolVar(&execute, "execute", false, "true: write file/create PR; false: log-only mode")
	flag.StringVar(&dir, "dir", "./", "local directory containing the project, for mode=local")
	flag.StringVar(&org, "org", "", "org/owner name, required for mode=remote")
	flag.StringVar(&repo, "repo", "", "repository name, for mode=remote")
	flag.StringVar(&repoFile, "repoFile", "", "file containing repo list (one per line), for mode=remote")
	// Deprecated since v0.9.5, kept so existing callers do not break on an unknown flag.
	var rateLimitBuffer int
	flag.IntVar(&rateLimitBuffer, "rateLimitBuffer", 0, "deprecated, has no effect: rate limits are handled automatically")
	flag.Parse()
	if rateLimitBuffer != 0 {
		log.Printf("WARN  The -rateLimitBuffer flag is deprecated and has no effect: rate limits are handled automatically.")
	}
	switch mode {
	case "local":
		break
	case "remote":
		if (repo == "" && repoFile == "") || org == "" {
			showUsageAndExit()
		}
	default:
		showUsageAndExit()
	}
	return mode, configFile, execute, dir, org, sanitizeRepoName(repo), repoFile
}

// remoteRepoNames returns the repositories to process in remote mode: repo if set, the lines of repoFile otherwise.
func remoteRepoNames(repo string, repoFile string) []string {
	if repo != "" {
		return []string{repo}
	}

	var repos []string
	for _, line := range util.ReadLinesFromFile(repoFile) {
		repos = append(repos, sanitizeRepoName(line))
	}

	return repos
}

func getGitHubClient() *githubapi.Client {
	gitHubToken := util.GetEnvParameter("GITHUB_TOKEN", true)
	if gitHubToken == "" {
		log.Printf("ERROR Missing GITHUB_TOKEN environment variable, quitting.")
		os.Exit(1)
	}
	client, err := githubapi.NewClient(gitHubToken)
	if err != nil {
		log.Printf("ERROR Could not create GitHub client: %v", err)
		os.Exit(1)
	}
	return client
}

// waitForRateLimit pauses until the GitHub API rate limit allows further
// requests, based on the limit reported by the API responses seen so far. A
// single pause is enough: the wait lasts until the reported reset (WaitFor caps
// it against bogus reset timestamps), and the state cannot change in between,
// because no API calls are made while waiting.
func waitForRateLimit(gitHubClient *githubapi.Client) {
	state := gitHubClient.RateLimit()
	wait := state.WaitFor(time.Now())
	if wait <= 0 {
		return
	}
	log.Printf("WARN  Rate limit reached, waiting %v for the reset at %v...",
		wait.Round(time.Second), state.Reset.UTC().Format(time.RFC3339))
	time.Sleep(wait)
}

// processRemoteRepoWithRateLimit processes one repo, waiting for the rate limit
// to reset beforehand if it is currently exhausted, and retrying the repo once
// if it ran into the limit while being processed.
func processRemoteRepoWithRateLimit(gitHubClient *githubapi.Client, repo string, process func() bool) bool {
	waitForRateLimit(gitHubClient)
	if process() {
		return true
	}
	if !gitHubClient.RateLimit().Exhausted {
		// failed for another reason, a retry would not help
		return false
	}
	log.Printf("WARN  Repo %v ran into the rate limit, retrying after the reset.", repo)
	waitForRateLimit(gitHubClient)
	return process()
}

// remoteRepo holds what is read from GitHub to process a repository.
type remoteRepo struct {
	defaultBranch string
	config        []byte // nil if .github/dependabot.yml does not exist
	files         []string
	treeTruncated bool
}

// loadRemoteRepo reads the dependabot config and the file list of a repository. It returns a skip reason
// instead if there is nothing to process.
func loadRemoteRepo(gitHubClient *githubapi.Client, org string, repo string) (loaded remoteRepo, skipReason string, err error) {
	gitHubRepo, err := gitHubClient.GetRepository(org, repo)
	if err != nil {
		return remoteRepo{}, "", err
	}

	if *gitHubRepo.Archived {
		log.Printf("INFO  Repository %v is archived. Nothing to do.", repo)
		return remoteRepo{}, "archived", nil
	}

	currentConfig, err := gitHubClient.GetFileContent(org, repo, ".github/dependabot.yml", "")
	if err != nil {
		if strings.Contains(err.Error(), "This repository is empty") {
			log.Printf("INFO  Repository %v is empty. Nothing to do.", repo)
			return remoteRepo{}, "empty", nil
		}

		log.Printf("ERROR Could not read config of repo %v: %v", repo, err)
		return remoteRepo{}, "", err
	}

	baseBranch := *gitHubRepo.DefaultBranch
	fileList, treeTruncated, err := gitHubClient.GetRepoFileList(org, repo, baseBranch)
	if err != nil {
		log.Printf("ERROR Could not read the file tree of repo %v: %v", repo, err)
		return remoteRepo{}, "", err
	}

	if treeTruncated {
		log.Printf("WARN  The file tree of repo %v is truncated by GitHub, some manifests may be missed.", repo)
	}

	return remoteRepo{defaultBranch: baseBranch, config: currentConfig, files: fileList, treeTruncated: treeTruncated}, "", nil
}

func processRemoteRepo(toolConfig config.ToolConfig, gitHubClient *githubapi.Client, execute bool, org string, repo string) (success bool) {
	loaded, skipReason, err := loadRemoteRepo(gitHubClient, org, repo)
	if err != nil {
		return false
	}

	if skipReason != "" {
		return true // not an error, just skip
	}

	// find manifests
	manifests := map[string]string{}
	config.ScanFileList(loaded.files, manifests)
	// update the configuration and create a PR
	loadFileParameters := config.LoadFileContentParameters{Client: gitHubClient, Org: org, Repo: repo}
	checkDirectoryExistsParameters := config.CheckDirectoryExistsParameters{Client: gitHubClient, Org: org, Repo: repo}
	yamlContent, changeInfo := GetUpdatedConfigYaml(loaded.config, manifests, toolConfig, repo, LoadRemoteFileContent, loadFileParameters, CheckRemoteDirectoryExists, checkDirectoryExistsParameters)
	if yamlContent != nil {
		prDesc := githubapi.CreatePRDescription(changeInfo)
		if execute {
			if err := gitHubClient.CreateOrUpdatePullRequest(org, repo, loaded.defaultBranch, prDesc, string(yamlContent), toolConfig); err != nil {
				if strings.Contains(err.Error(), "pull request already exists") {
					log.Printf("WARN  There's an open pull request already on repo %v. Close or merge it first.", repo)
				} else if strings.Contains(err.Error(), "Resource not accessible") {
					log.Printf("ERROR Could not create PR for repo %v, permission problem: %v", repo, err)
					return false
				} else {
					log.Printf("ERROR Could not create PR for repo %v: %v", repo, err)
					return false
				}
			}
		} else {
			log.Printf("INFO  log-only mode, would create PR for %v:\n----------\n%v\n----------\n%v\n----------\nuse -execute=true to apply", repo, prDesc, string(yamlContent))
		}
	}
	return true
}

func processLocalRepo(toolConfig config.ToolConfig, execute bool, dir string) (success bool) {
	// find manifests
	manifests := map[string]string{}

	// get the current config and file list, from local file system
	dirPath := filepath.Join(dir, ".github/")
	fullPath := filepath.Join(dirPath, "dependabot.yml")
	currentConfig, err := util.ReadFile(fullPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			// file not found -> use empty config
			currentConfig = []byte("version: 2")
		} else {
			log.Printf("ERROR Could not read config from file %v: %v", fullPath, err)
			return false
		}
	}
	config.ScanLocalDirectory(dir, "", manifests)
	// update the configuration and save it back
	loadFileParameters := config.LoadFileContentParameters{Directory: dir}
	checkDirectoryExistsParameters := config.CheckDirectoryExistsParameters{Directory: dir}
	yamlContent, _ := GetUpdatedConfigYaml(currentConfig, manifests, toolConfig, dir, LoadLocalFileContent, loadFileParameters, CheckLocalDirectoryExists, checkDirectoryExistsParameters)
	if yamlContent != nil {
		if execute {
			if err := util.MakeDirIfNotExists(dirPath); err != nil {
				log.Printf("ERROR Could not create directory %v : %v\n", dirPath, err)
				return false
			}
			if err := util.SaveFile(fullPath, yamlContent); err != nil {
				log.Printf("ERROR Could not save file %v : %v\n", fullPath, err)
				return false
			}
			log.Printf("INFO  File %v written.", fullPath)
		} else {
			log.Printf("INFO  log-only mode, would write file %v:\n----------\n%v\n----------\nuse -execute=true to apply", fullPath, string(yamlContent))
		}
	}
	return true
}

func main() {
	// get parameters
	mode, configFile, execute, dir, org, repo, repoFile := getParameters()

	// read and parse config file, and initialize the patterns
	fileContent, err := util.ReadFile(configFile)
	if err != nil {
		log.Printf("ERROR Could not read tool config file for repo %s: %v.", repo, configFile)
		os.Exit(1)
	}
	toolConfig, err := config.ParseToolConfig(fileContent)
	if err != nil {
		log.Printf("ERROR Could not parse tool config for repo %s: %v", repo, err)
		os.Exit(1)
	}

	// initialize / precompile the patterns
	toolConfig.InitializePatterns()

	// track number of failed repositories
	failureCount := 0
	// process
	if mode == "local" {
		if !processLocalRepo(*toolConfig, execute, dir) {
			failureCount++
		}
	} else if mode == "remote" {
		gitHubClient := getGitHubClient()

		for _, repo := range remoteRepoNames(repo, repoFile) {
			process := func() bool { return processRemoteRepo(*toolConfig, gitHubClient, execute, org, repo) }
			if !processRemoteRepoWithRateLimit(gitHubClient, repo, process) {
				failureCount++
			}
		}
	}
	// Exit with error code if any processing failed
	if failureCount > 0 {
		log.Printf("ERROR %d repositories could not be processed successfully.", failureCount)
		os.Exit(1)
	}
}

// GetUpdatedConfigYaml returns the new .dependabot.yml file content, based on the current content and the manifests found.
func GetUpdatedConfigYaml(currentConfig []byte, manifests map[string]string, toolConfig config.ToolConfig, repo string,
	loadFileFn config.LoadFileContent, loadFileParams config.LoadFileContentParameters, checkDirectoryExistsFn config.CheckDirectoryExists, checkDirectoryExistsParams config.CheckDirectoryExistsParameters,
) ([]byte, config.ChangeInfo) {
	dependabotConfig, err := config.ParseDependabotConfig(currentConfig)
	if err != nil {
		log.Printf("ERROR Could not parse current config for %v: %v", repo, err)
		return nil, config.ChangeInfo{}
	}
	changeInfo := dependabotConfig.UpdateConfig(manifests, toolConfig, loadFileFn, loadFileParams, checkDirectoryExistsFn, checkDirectoryExistsParams)
	if len(changeInfo.NewRegistries) > 0 || len(changeInfo.NewUpdates) > 0 || len(changeInfo.FixedUpdates) > 0 || len(changeInfo.RemovedUpdates) > 0 || len(changeInfo.RemovedRegistries) > 0 {
		// at least one item in the update block is needed
		return dependabotConfig.ToYaml(), changeInfo
	}
	log.Printf("INFO  No update needed for repo %s.", repo)
	return nil, config.ChangeInfo{}
}
