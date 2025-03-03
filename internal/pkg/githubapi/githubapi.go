// Package githubapi contains functionality related to the GitHub API
package githubapi

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/getyourguide/dependabutler/internal/pkg/config"
	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"github.com/google/go-github/v50/github"
	"golang.org/x/oauth2"
)

// GetGitHubClient returns a GitHub client for API calls
func GetGitHubClient(accessToken string) *github.Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

// GetRepository gets a repository object.
func GetRepository(client *github.Client, org string, repo string) (*github.Repository, error) {
	ctx := context.Background()
	repository, _, err := client.Repositories.Get(ctx, org, repo)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			log.Printf("WARN  GitHub repo %v/%v not found.", org, repo)
		} else {
			log.Printf("ERROR Got error when requesting GitHub repo.\n%v", err)
		}
		return nil, err
	}
	return repository, nil
}

// GetRepoFileList returns a list (strings) of all files in a repo, including their path.
func GetRepoFileList(client *github.Client, org string, repo string, defaultBranch string) []string {
	// get the file tree
	ctx := context.Background()
	tree, _, err := client.Git.GetTree(ctx, org, repo, defaultBranch, true)
	if err != nil {
		log.Printf("ERROR Got error when requesting GitHub repo tree.\n%v", err)
		return nil
	}
	result := make([]string, 0)
	for _, entry := range tree.Entries {
		result = append(result, *entry.Path)
	}
	return result
}

// GetFileContent returns the content of a file
func GetFileContent(client *github.Client, org string, repo string, path string, branchName string) ([]byte, error) {
	ctx := context.Background()
	opts := &github.RepositoryContentGetOptions{}
	if branchName != "" {
		opts.Ref = branchName
	}
	content, _, _, err := client.Repositories.GetContents(ctx, org, repo, path, opts)
	if err != nil && !strings.Contains(err.Error(), "404 Not Found") {
		return nil, err
	}
	if content == nil {
		return nil, nil
	}
	fileContent, err := content.GetContent()
	if err != nil {
		return nil, err
	}
	return bytes.NewBufferString(fileContent).Bytes(), nil
}

// CheckDirectoryExists checks if a directory exists in the remote GitHub repository.
func CheckDirectoryExists(client *github.Client, org string, repo string, directory string, branchName string) (bool, error) {
	ctx := context.Background()
	opts := &github.RepositoryContentGetOptions{}
	if branchName != "" {
		opts.Ref = branchName
	}
	_, dirContents, _, err := client.Repositories.GetContents(ctx, org, repo, directory, opts)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			return false, nil
		}
		return false, err
	}
	return dirContents != nil, nil
}

// CreateOrUpdatePullRequest creates or updates a PR for changes in dependabot.yml
func CreateOrUpdatePullRequest(client *github.Client, org string, repo string, baseBranch string, prDesc string, content string, toolConfig config.ToolConfig) error {
	prParams := toolConfig.PullRequestParameters

	// Check if there already is a PR open, from dependabutler. If so, re-use its branch.
	existingPr, err := getExistingPr(client, org, repo)
	if err != nil {
		return err
	}
	var branchName string
	if existingPr != nil {
		branchName = *existingPr.Head.Ref
		// In case a PR exists, check if the file content has changed meanwhile.
		prContent, err := GetFileContent(client, org, repo, ".github/dependabot.yml", branchName)
		if err != nil {
			return err
		}
		if string(prContent) == content {
			log.Printf("INFO  Found open PR, no update required: %v", *existingPr.HTMLURL)
			return nil
		}
	} else {
		branchName, err = getNewBranchName(prParams)
		if err != nil {
			return err
		}
	}

	// Get the reference (existing or new).
	ref, err := getReference(client, org, repo, baseBranch, branchName)
	if err != nil {
		return err
	}

	// Create a tree with one entry, for the commit.
	tree, err := getTree(client, ref, org, repo, ".github/dependabot.yml", content)
	if err != nil {
		return err
	}

	// Push the commit.
	err = pushCommit(client, ref, tree, org, repo, prParams.CommitMessage, prParams.AuthorName, prParams.AuthorEmail)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if existingPr != nil {
		existingPr.Body = &prDesc
		if _, _, err := client.PullRequests.Edit(ctx, org, repo, *existingPr.Number, existingPr); err != nil {
			return err
		}
		log.Printf("INFO  PR successfully updated: %s\n", existingPr.GetHTMLURL())
	} else {
		// Create a new PR for the branch. In case of an existing PR, no further action is needed.
		newPR := &github.NewPullRequest{}
		newPR.Title = &prParams.PRTitle
		newPR.Body = &prDesc
		newPR.Head = &branchName
		newPR.Base = &baseBranch
		pr, _, err := client.PullRequests.Create(ctx, org, repo, newPR)
		if err != nil {
			return err
		}
		labels := []string{"dependabutler"}
		_, _, err = client.Issues.AddLabelsToIssue(ctx, org, repo, *pr.Number, labels)
		if err != nil {
			return err
		}
		log.Printf("INFO  PR successfully created: %s\n", pr.GetHTMLURL())
	}
	sleepSeconds := toolConfig.PullRequestParameters.SleepAfterPRAction
	if sleepSeconds > 0 {
		// Sleep - can help to avoid issues with second rate limit.
		time.Sleep(time.Duration(sleepSeconds) * time.Second)
	}
	return nil
}

// CreatePRDescription renders the body of the PR to be created.
func CreatePRDescription(changeInfo config.ChangeInfo) string {
	lines := []string{"### dependabutler has created this PR to update .github/dependabot.yml"}
	if len(changeInfo.NewRegistries) > 0 {
		lines = append(lines, "")
		lines = append(lines, "#### 🏛 registries added")
		lines = append(lines, "| type | name |")
		lines = append(lines, "| - | - |")
		for _, registry := range changeInfo.NewRegistries {
			lines = append(lines, fmt.Sprintf("| %v | %v |", registry.Type, registry.Name))
		}
	}
	if len(changeInfo.NewUpdates) > 0 {
		lines = append(lines, "")
		lines = append(lines, "#### ♻ updates added")
		lines = append(lines, "| type | directory | file |")
		lines = append(lines, "| - | - | - |")
		for _, update := range changeInfo.NewUpdates {
			lines = append(lines, fmt.Sprintf("| %v | %v | %v |", update.Type, update.Directory, update.File))
		}
	}
	if len(changeInfo.FixedUpdates) > 0 {
		lines = append(lines, "")
		lines = append(lines, "#### 🔨 updates fixed")
		lines = append(lines, "| type | directory | ")
		lines = append(lines, "| - | - |")
		for _, update := range changeInfo.FixedUpdates {
			lines = append(lines, fmt.Sprintf("| %v | %v |", update.Type, update.Directory))
		}
	}
	lines = append(lines, "")
	lines = append(lines, "#### note")
	lines = append(lines, "* Check the default settings applied (schedule, open-pull-requests-limit, etc.) and change if required.")
	return strings.Join(lines, "\n")
}

func getTree(client *github.Client, ref *github.Reference, org string, repo string, file string, content string) (*github.Tree, error) {
	ctx := context.Background()
	entries := []*github.TreeEntry{
		{Path: github.String(file), Type: github.String("blob"), Content: github.String(content), Mode: github.String("100644")},
	}
	tree, _, err := client.Git.CreateTree(ctx, org, repo, *ref.Object.SHA, entries)
	if err != nil {
		return nil, err
	}
	return tree, nil
}

func getReference(client *github.Client, org string, repo string, baseBranch string, commitBranch string) (*github.Reference, error) {
	ctx := context.Background()
	baseRefName := "refs/heads/" + baseBranch
	commitRefName := "refs/heads/" + commitBranch
	if ref, _, err := client.Git.GetRef(ctx, org, repo, commitRefName); err == nil {
		// branch for commit already exists -> return it
		return ref, nil
	}
	// create commit branch
	var baseRef *github.Reference
	baseRef, _, err := client.Git.GetRef(ctx, org, repo, baseRefName)
	if err != nil {
		log.Printf("ERROR Could not get base branch %v of repo %v : %v\n", baseBranch, repo, err)
		return nil, err
	}
	newRef := &github.Reference{Ref: github.String(commitRefName), Object: &github.GitObject{SHA: baseRef.Object.SHA}}
	ref, _, err := client.Git.CreateRef(ctx, org, repo, newRef)
	if err != nil {
		log.Printf("ERROR Could not create commit branch %v for repo %v : %v\n", commitBranch, repo, err)
		return nil, err
	}
	return ref, nil
}

func pushCommit(client *github.Client, ref *github.Reference, tree *github.Tree, org string, repo string, commitMessage string, authorName string, authorEmail string) error {
	ctx := context.Background()
	parent, _, err := client.Repositories.GetCommit(ctx, org, repo, *ref.Object.SHA, nil)
	if err != nil {
		return err
	}
	parent.Commit.SHA = parent.SHA
	now := time.Now()
	author := &github.CommitAuthor{Date: &github.Timestamp{Time: now}, Name: &authorName, Email: &authorEmail}
	commit := &github.Commit{Author: author, Message: &commitMessage, Tree: tree, Parents: []*github.Commit{parent.Commit}}
	newCommit, _, err := client.Git.CreateCommit(ctx, org, repo, commit)
	if err != nil {
		return err
	}
	ref.Object.SHA = newCommit.SHA
	_, _, err = client.Git.UpdateRef(ctx, org, repo, ref, false)
	if err != nil {
		return err
	}
	return nil
}

func getExistingPr(client *github.Client, org string, repo string) (*github.PullRequest, error) {
	ctx := context.Background()
	opts := github.IssueListByRepoOptions{
		State:  "open",
		Labels: []string{"dependabutler"},
	}
	issues, _, err := client.Issues.ListByRepo(ctx, org, repo, &opts)
	if err != nil {
		return nil, err
	}
	existingPrIssue := (*github.Issue)(nil)
	for _, issue := range issues {
		if issue.IsPullRequest() {
			existingPrIssue = issue
			break
		}
	}
	if existingPrIssue != nil {
		existingPr, _, err := client.PullRequests.Get(ctx, org, repo, *existingPrIssue.Number)
		if err != nil {
			return nil, err
		}
		return existingPr, nil
	}
	return nil, nil
}

func getNewBranchName(prParams config.PullRequestParameters) (string, error) {
	branchName := prParams.BranchName
	if prParams.BranchNameRandomSuffix {
		randToken, err := util.RandToken(16)
		if err != nil {
			return "", err
		}
		branchName = fmt.Sprintf("%v-%v", prParams.BranchName, randToken)
	}
	return branchName, nil
}
