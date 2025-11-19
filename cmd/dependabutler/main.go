package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getyourguide/dependabutler/internal/pkg/config"
	"github.com/getyourguide/dependabutler/internal/pkg/githubapi"
	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"github.com/google/go-github/v50/github"
)

// LoadRemoteFileContent is the implementation of LoadFileContent, for remote files (GitHub).
func LoadRemoteFileContent(file string, params config.LoadFileContentParameters) string {
	content, err := githubapi.GetFileContent(params.GitHubClient, params.Org, params.Repo, file, "")
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
	exists, err := githubapi.CheckDirectoryExists(params.GitHubClient, params.Org, params.Repo, directory, "")
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

func getParameters() (string, string, bool, string, string, string, string, int) {
	var mode, dir, repo, repoFile, org, configFile string
	var execute bool
	var rateLimitBuffer int
	flag.StringVar(&mode, "mode", "local", "local or remote")
	flag.StringVar(&configFile, "configFile", "dependabutler.yml", "location of tool config file")
	flag.BoolVar(&execute, "execute", false, "true: write file/create PR; false: log-only mode")
	flag.StringVar(&dir, "dir", "./", "local directory containing the project, for mode=local")
	flag.StringVar(&org, "org", "", "org/owner name, required for mode=remote")
	flag.StringVar(&repo, "repo", "", "repository name, for mode=remote")
	flag.StringVar(&repoFile, "repoFile", "", "file containing repo list (one per line), for mode=remote")
	flag.IntVar(&rateLimitBuffer, "rateLimitBuffer", 0, "safety buffer for GitHub API rate limits. Pauses when remaining requests drop below this number. 0=disabled.")
	flag.Parse()
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
	return mode, configFile, execute, dir, org, repo, repoFile, rateLimitBuffer
}

func getGitHubClient() *github.Client {
	gitHubToken := util.GetEnvParameter("GITHUB_TOKEN", true)
	if gitHubToken == "" {
		log.Printf("ERROR Missing GITHUB_TOKEN environment variable, quitting.")
		os.Exit(1)
	}
	return githubapi.GetGitHubClient(gitHubToken)
}

// checkRateLimit checks if there are enough GitHub API requests remaining
func checkRateLimit(client *github.Client, minRemaining int) (bool, int, error) {
	ctx := context.Background()
	rateLimits, _, err := client.RateLimits(ctx)
	if err != nil {
		return false, 0, err
	}

	remaining := rateLimits.Core.Remaining
	return remaining >= minRemaining, remaining, nil
}

// ensureRateLimit ensures there are enough remaining GitHub API requests by waiting if necessary
// Returns true if rate limit is sufficient, false if max retries exceeded
func ensureRateLimit(client *github.Client, minRemaining int) bool {
	const maxRetries = 20
	const waitDuration = 5 * time.Minute

	for attempt := 1; attempt <= maxRetries; attempt++ {
		hasEnough, remaining, err := checkRateLimit(client, minRemaining)
		if err != nil {
			log.Printf("ERROR Failed to check rate limit: %v", err)
			return false
		}

		if hasEnough {
			return true
		}

		log.Printf("WARN  Rate limit too low (%d remaining, need %d). Waiting 5 minutes (attempt %d/%d)...",
			remaining, minRemaining, attempt, maxRetries)
		time.Sleep(waitDuration)
	}

	log.Printf("ERROR Rate limit still too low after %d attempts", maxRetries)
	return false
}

func processRemoteRepo(toolConfig config.ToolConfig, gitHubClient *github.Client, execute bool, org string, repo string) {
	// find manifests
	manifests := map[string]string{}

	// get the current config and file list, from GitHub, via API
	gitHubRepo, err := githubapi.GetRepository(gitHubClient, org, repo)
	if err != nil {
		return
	}
	if *gitHubRepo.Archived {
		log.Printf("INFO  Repository %v is archived. Nothing to do.", repo)
		return
	}
	currentConfig, err := githubapi.GetFileContent(gitHubClient, org, repo, ".github/dependabot.yml", "")
	if err != nil {
		if strings.Contains(err.Error(), "This repository is empty") {
			log.Printf("INFO  Repository %v is empty. Nothing to do.", repo)
		} else {
			log.Printf("ERROR Could not read config of repo %v: %v", repo, err)
		}
		return
	}
	baseBranch := *gitHubRepo.DefaultBranch
	fileList := githubapi.GetRepoFileList(gitHubClient, org, repo, baseBranch)
	config.ScanFileList(fileList, manifests)
	// update the configuration and create a PR
	loadFileParameters := config.LoadFileContentParameters{GitHubClient: gitHubClient, Org: org, Repo: repo}
	checkDirectoryExistsParameters := config.CheckDirectoryExistsParameters{GitHubClient: gitHubClient, Org: org, Repo: repo}
	yamlContent, changeInfo := GetUpdatedConfigYaml(currentConfig, manifests, toolConfig, repo, LoadRemoteFileContent, loadFileParameters, CheckRemoteDirectoryExists, checkDirectoryExistsParameters)
	if yamlContent != nil {
		prDesc := githubapi.CreatePRDescription(changeInfo)
		if execute {
			if err := githubapi.CreateOrUpdatePullRequest(gitHubClient, org, repo, baseBranch, prDesc, string(yamlContent), toolConfig); err != nil {
				if strings.Contains(err.Error(), "pull request already exists") {
					log.Printf("WARN  There's an open pull request already on repo %v. Close or merge it first.", repo)
				} else if strings.Contains(err.Error(), "Resource not accessible") {
					// Fail with error.
					log.Fatalf("ERROR Could not create PR for repo %v, permission problem. Stopping. %v", repo, err)
				} else {
					log.Printf("ERROR Could not create PR for repo %v: %v", repo, err)
				}
			}
		} else {
			log.Printf("INFO  log-only mode, would create PR for %v:\n----------\n%v\n----------\n%v\n----------\nuse -execute=true to apply", repo, prDesc, string(yamlContent))
		}
	}
}

func processLocalRepo(toolConfig config.ToolConfig, execute bool, dir string) {
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
			return
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
				return
			}
			if err := util.SaveFile(fullPath, yamlContent); err != nil {
				log.Printf("ERROR Could not save file %v : %v\n", fullPath, err)
				return
			}
			log.Printf("INFO  File %v written.", fullPath)
		} else {
			log.Printf("INFO  log-only mode, would write file %v:\n----------\n%v\n----------\nuse -execute=true to apply", fullPath, string(yamlContent))
		}
	}
}

func main() {
	// get parameters
	mode, configFile, execute, dir, org, repo, repoFile, rateLimitBuffer := getParameters()

	// read and parse config file, and initialize the patterns
	fileContent, err := util.ReadFile(configFile)
	if err != nil {
		log.Printf("ERROR Could not read tool config file for repo %s: %v.", repo, configFile)
		return
	}
	toolConfig, err := config.ParseToolConfig(fileContent)
	if err != nil {
		log.Printf("ERROR Could not parse tool config for repo %s: %v", repo, err)
		return
	}

	// initialize / precompile the patterns
	toolConfig.InitializePatterns()

	// process
	if mode == "local" {
		processLocalRepo(*toolConfig, execute, dir)
	} else if mode == "remote" {
		gitHubClient := getGitHubClient()

		if repo != "" {
			processRemoteRepo(*toolConfig, gitHubClient, execute, org, repo)
		} else if repoFile != "" {
			for _, repo := range util.ReadLinesFromFile(repoFile) {
				// Check rate limit before processing each repo if enabled
				if rateLimitBuffer > 0 {
					if !ensureRateLimit(gitHubClient, rateLimitBuffer) {
						log.Printf("ERROR Rate limit check failed, exiting")
						os.Exit(1)
					}
				}
				processRemoteRepo(*toolConfig, gitHubClient, execute, org, repo)
			}
		}
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
