package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/getyourguide/dependabutler/internal/pkg/config"
	"github.com/getyourguide/dependabutler/internal/pkg/githubapi"
	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"github.com/google/go-github/v50/github"
)

// LoadRemoteFileContent is the implementation of LoadFileContent, for remote files (GitHub).
func LoadRemoteFileContent(file string, params config.LoadFileContentParameters) string {
	content, err := githubapi.GetFileContent(params.GitHubClient, params.Org, params.Repo, file)
	if err != nil {
		log.Printf("WARN  Could not content of file %v: %v", file, err)
		return ""
	}
	return string(content)
}

// LoadLocalFileContent is the implementation of LoadFileContent, for local files (file system).
func LoadLocalFileContent(file string, params config.LoadFileContentParameters) string {
	fullPath := filepath.Join(params.Directory, file)
	content, err := util.ReadFile(fullPath)
	if err != nil {
		log.Printf("WARN  Could not content of file %v: %v", fullPath, err)
		return ""
	}
	return string(content)
}

func showUsageAndExit() {
	flag.Usage()
	os.Exit(1)
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
	return mode, configFile, execute, dir, org, repo, repoFile
}

func getGitHubClient() *github.Client {
	gitHubToken := util.GetEnvParameter("GITHUB_TOKEN", true)
	if gitHubToken == "" {
		log.Printf("ERROR Missing GITHUB_TOKEN environment variable, quitting.")
		os.Exit(1)
	}
	return githubapi.GetGitHubClient(gitHubToken)
}

func processRemoteRepo(toolConfig config.ToolConfig, execute bool, org string, repo string) {
	// find manifests
	manifests := map[string]string{}

	// get the current config and file list, from GitHub, via API
	gitHubClient := getGitHubClient()
	gitHubRepo, err := githubapi.GetRepository(gitHubClient, org, repo)
	if err != nil {
		return
	}
	if *gitHubRepo.Archived {
		log.Printf("INFO  Repository %v is archived. Nothing to do.", repo)
		return
	}
	currentConfig, err := githubapi.GetFileContent(gitHubClient, org, repo, ".github/dependabot.yml")
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
	yamlContent, changeInfo := GetUpdatedConfigYaml(currentConfig, manifests, toolConfig, repo, LoadRemoteFileContent, loadFileParameters)
	if yamlContent != nil {
		prDesc := githubapi.CreatePRDescription(changeInfo)
		if execute {
			if err := githubapi.CreatePullRequest(gitHubClient, org, repo, baseBranch, prDesc, string(yamlContent), toolConfig); err != nil {
				if strings.Contains(err.Error(), "pull request already exists") {
					log.Printf("WARN  There's an open pull request already on repo %v. Close or merge it first.", repo)
				} else {
					log.Printf("ERROR Could not create PR: %v", err)
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
	yamlContent, _ := GetUpdatedConfigYaml(currentConfig, manifests, toolConfig, dir, LoadLocalFileContent, loadFileParameters)
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
	mode, configFile, execute, dir, org, repo, repoFile := getParameters()

	// read and parse config file, and initialize the patterns
	fileContent, err := util.ReadFile(configFile)
	if err != nil {
		log.Printf("ERROR Could not read tool config file %v.", configFile)
		return
	}
	toolConfig, err := config.ParseToolConfig(fileContent)
	if err != nil {
		log.Printf("ERROR Could not parse tool config: %v", err)
		return
	}

	// initialize / precompile the patterns
	config.InitializePatterns(toolConfig.ManifestPatterns)

	// process
	if mode == "local" {
		processLocalRepo(*toolConfig, execute, dir)
	} else if mode == "remote" {
		if repo != "" {
			processRemoteRepo(*toolConfig, execute, org, repo)
		} else if repoFile != "" {
			for _, repo := range util.ReadLinesFromFile(repoFile) {
				processRemoteRepo(*toolConfig, execute, org, repo)
			}
		}
	}
}

// GetUpdatedConfigYaml returns the new .dependabot.yml file content, based on the current content and the manifests found.
func GetUpdatedConfigYaml(currentConfig []byte, manifests map[string]string, toolConfig config.ToolConfig, repo string,
	loadFileFn config.LoadFileContent, loadFileParams config.LoadFileContentParameters) ([]byte, config.ChangeInfo) {
	dependabotConfig, err := config.ParseDependabotConfig(currentConfig)
	if err != nil {
		log.Printf("ERROR Could not parse current config for %v: %v", repo, err)
		return nil, config.ChangeInfo{}
	}
	changeInfo := dependabotConfig.UpdateConfig(manifests, toolConfig, loadFileFn, loadFileParams)
	if len(changeInfo.NewRegistries) > 0 || len(changeInfo.NewUpdates) > 0 {
		// at least one item in the update block is needed
		return dependabotConfig.ToYaml(), changeInfo
	}
	log.Printf("INFO  No update needed.")
	return nil, config.ChangeInfo{}
}
