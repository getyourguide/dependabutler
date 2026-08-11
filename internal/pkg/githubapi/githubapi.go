// Package githubapi contains functionality related to the GitHub API
package githubapi

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/getyourguide/dependabutler/internal/pkg/config"
	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"github.com/google/go-github/v50/github"
	"golang.org/x/oauth2"
)

// Client is a GitHub API client that keeps track of the rate limit reported by
// the responses it receives. All API calls go through it, so the rate limit
// state is always derived from real traffic and is scoped to this client rather
// than shared through package level state.
type Client struct {
	gh *github.Client

	rateMutex sync.Mutex
	rate      RateLimitState
}

// NewClient returns a GitHub API client authenticated with the given token.
func NewClient(accessToken string) *Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: accessToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	return &Client{gh: github.NewClient(tc)}
}

// GetRepository gets a repository object.
func (client *Client) GetRepository(org string, repo string) (*github.Repository, error) {
	ctx := context.Background()
	repository, resp, err := client.gh.Repositories.Get(ctx, org, repo)
	client.observe(resp, err)
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
func (client *Client) GetRepoFileList(org string, repo string, defaultBranch string) []string {
	// get the file tree
	ctx := context.Background()
	tree, resp, err := client.gh.Git.GetTree(ctx, org, repo, defaultBranch, true)
	client.observe(resp, err)
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
func (client *Client) GetFileContent(org string, repo string, path string, branchName string) ([]byte, error) {
	ctx := context.Background()
	opts := &github.RepositoryContentGetOptions{}
	if branchName != "" {
		opts.Ref = branchName
	}
	content, _, resp, err := client.gh.Repositories.GetContents(ctx, org, repo, path, opts)
	client.observe(resp, err)
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
func (client *Client) CheckDirectoryExists(org string, repo string, directory string, branchName string) (bool, error) {
	ctx := context.Background()
	opts := &github.RepositoryContentGetOptions{}
	if branchName != "" {
		opts.Ref = branchName
	}
	_, dirContents, resp, err := client.gh.Repositories.GetContents(ctx, org, repo, directory, opts)
	client.observe(resp, err)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			return false, nil
		}
		return false, err
	}
	return dirContents != nil, nil
}

// CreateOrUpdatePullRequest creates or updates a PR for changes in dependabot.yml
func (client *Client) CreateOrUpdatePullRequest(org string, repo string, baseBranch string, prDesc string, content string, toolConfig config.ToolConfig) error {
	prParams := toolConfig.PullRequestParameters

	// Check if there already is a PR open, from dependabutler. If so, re-use its branch.
	existingPr, err := client.getExistingPr(org, repo)
	if err != nil {
		return err
	}
	var branchName string
	if existingPr != nil {
		branchName = *existingPr.Head.Ref
		// In case a PR exists, check if the file content has changed meanwhile.
		prContent, err := client.GetFileContent(org, repo, ".github/dependabot.yml", branchName)
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
	ref, err := client.getReference(org, repo, baseBranch, branchName)
	if err != nil {
		return err
	}

	// Create a tree with one entry, for the commit.
	tree, err := client.getTree(ref, org, repo, ".github/dependabot.yml", content)
	if err != nil {
		return err
	}

	// Push the commit.
	err = client.pushCommit(ref, tree, org, repo, prParams.CommitMessage, prParams.AuthorName, prParams.AuthorEmail)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if existingPr != nil {
		existingPr.Body = &prDesc
		_, resp, err := client.gh.PullRequests.Edit(ctx, org, repo, *existingPr.Number, existingPr)
		client.observe(resp, err)
		if err != nil {
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
		pr, resp, err := client.gh.PullRequests.Create(ctx, org, repo, newPR)
		client.observe(resp, err)
		if err != nil {
			return err
		}
		labels := prParams.PRLabels
		if len(labels) == 0 {
			labels = []string{"dependabutler"}
		}
		_, resp, err = client.gh.Issues.AddLabelsToIssue(ctx, org, repo, *pr.Number, labels)
		client.observe(resp, err)
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
	lines := []string{"### dependabutler has created this PR to update `.github/dependabot.yml`"}
	if len(changeInfo.NewRegistries) > 0 {
		lines = append(lines, "")
		lines = append(lines, "#### 🏛 registries added")
		lines = append(lines, "| type | name |")
		lines = append(lines, "| - | - |")
		for _, registry := range changeInfo.NewRegistries {
			lines = append(lines, fmt.Sprintf("| %v | `%v` |", registry.Type, registry.Name))
		}
	}
	if len(changeInfo.RemovedRegistries) > 0 {
		lines = append(lines, "")
		lines = append(lines, "#### 🧹 registries removed")
		lines = append(lines, "| type | name |")
		lines = append(lines, "| - | - |")
		for _, registry := range changeInfo.RemovedRegistries {
			lines = append(lines, fmt.Sprintf("| %v | `%v` |", registry.Type, registry.Name))
		}
	}
	if len(changeInfo.NewUpdates) > 0 {
		lines = append(lines, "")
		lines = append(lines, "#### ♻ updates added")
		lines = append(lines, "| type | directory | file |")
		lines = append(lines, "| - | - | - |")
		for _, update := range changeInfo.NewUpdates {
			lines = append(lines, fmt.Sprintf("| %v | `%v` | `%v` |", update.Type, update.Directory, update.File))
		}
	}
	if len(changeInfo.FixedUpdates) > 0 {
		lines = append(lines, "")
		lines = append(lines, "#### 🔨 updates fixed")
		lines = append(lines, "| type | directory | ")
		lines = append(lines, "| - | - |")
		for _, update := range changeInfo.FixedUpdates {
			lines = append(lines, fmt.Sprintf("| %v | `%v` |", update.Type, update.Directory))
		}
	}
	if len(changeInfo.RemovedUpdates) > 0 {
		lines = append(lines, "")
		lines = append(lines, "#### 🧹 unused updates removed")
		lines = append(lines, "| type | directory | ")
		lines = append(lines, "| - | - |")
		for _, update := range changeInfo.RemovedUpdates {
			lines = append(lines, fmt.Sprintf("| %v | `%v` |", update.Type, update.Directory))
		}
	}
	lines = append(lines, "")
	lines = append(lines, "#### note")
	lines = append(lines, "* Check the default settings applied (schedule, open-pull-requests-limit, etc.) and change if required.")
	return strings.Join(lines, "\n")
}

func (client *Client) getTree(ref *github.Reference, org string, repo string, file string, content string) (*github.Tree, error) {
	ctx := context.Background()
	entries := []*github.TreeEntry{
		{Path: github.String(file), Type: github.String("blob"), Content: github.String(content), Mode: github.String("100644")},
	}
	tree, resp, err := client.gh.Git.CreateTree(ctx, org, repo, *ref.Object.SHA, entries)
	client.observe(resp, err)
	if err != nil {
		return nil, err
	}
	return tree, nil
}

func (client *Client) getReference(org string, repo string, baseBranch string, commitBranch string) (*github.Reference, error) {
	ctx := context.Background()
	baseRefName := "refs/heads/" + baseBranch
	commitRefName := "refs/heads/" + commitBranch
	existingRef, resp, err := client.gh.Git.GetRef(ctx, org, repo, commitRefName)
	client.observe(resp, err)
	if err == nil {
		// branch for commit already exists -> return it
		return existingRef, nil
	}
	// create commit branch
	var baseRef *github.Reference
	baseRef, resp, err = client.gh.Git.GetRef(ctx, org, repo, baseRefName)
	client.observe(resp, err)
	if err != nil {
		log.Printf("ERROR Could not get base branch %v of repo %v : %v\n", baseBranch, repo, err)
		return nil, err
	}
	newRef := &github.Reference{Ref: github.String(commitRefName), Object: &github.GitObject{SHA: baseRef.Object.SHA}}
	ref, _, err := client.gh.Git.CreateRef(ctx, org, repo, newRef)
	if err != nil {
		log.Printf("ERROR Could not create commit branch %v for repo %v : %v\n", commitBranch, repo, err)
		return nil, err
	}
	return ref, nil
}

func (client *Client) pushCommit(ref *github.Reference, tree *github.Tree, org string, repo string, commitMessage string, authorName string, authorEmail string) error {
	ctx := context.Background()
	parent, resp, err := client.gh.Repositories.GetCommit(ctx, org, repo, *ref.Object.SHA, nil)
	client.observe(resp, err)
	if err != nil {
		return err
	}
	parent.Commit.SHA = parent.SHA
	now := time.Now()
	author := &github.CommitAuthor{Date: &github.Timestamp{Time: now}, Name: &authorName, Email: &authorEmail}
	commit := &github.Commit{Author: author, Message: &commitMessage, Tree: tree, Parents: []*github.Commit{parent.Commit}}
	newCommit, resp, err := client.gh.Git.CreateCommit(ctx, org, repo, commit)
	client.observe(resp, err)
	if err != nil {
		return err
	}
	ref.Object.SHA = newCommit.SHA
	_, resp, err = client.gh.Git.UpdateRef(ctx, org, repo, ref, false)
	client.observe(resp, err)
	if err != nil {
		return err
	}
	return nil
}

func (client *Client) getExistingPr(org string, repo string) (*github.PullRequest, error) {
	ctx := context.Background()
	opts := github.IssueListByRepoOptions{
		State:  "open",
		Labels: []string{"dependabutler"},
	}
	issues, resp, err := client.gh.Issues.ListByRepo(ctx, org, repo, &opts)
	client.observe(resp, err)
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
		existingPr, resp, err := client.gh.PullRequests.Get(ctx, org, repo, *existingPrIssue.Number)
		client.observe(resp, err)
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
