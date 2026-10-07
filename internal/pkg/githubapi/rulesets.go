package githubapi

import (
	"context"

	"github.com/google/go-github/v90/github"
)

// GetActiveRulesetNames returns the names of the active rulesets that apply to a repo, including the
// ones configured on its organization.
func (client *Client) GetActiveRulesetNames(org string, repo string) ([]string, error) {
	ctx := context.Background()
	opts := &github.RepositoryListRulesetsOptions{
		IncludesParents: github.Ptr(true),
		ListOptions:     github.ListOptions{PerPage: 100},
	}

	var names []string
	for {
		rulesets, resp, err := client.gh.Repositories.GetAllRulesets(ctx, org, repo, opts)
		client.observe(resp, err)
		if err != nil {
			return nil, err
		}

		names = append(names, activeRulesetNames(rulesets)...)
		if resp.NextPage == 0 {
			return names, nil
		}

		opts.Page = resp.NextPage
	}
}

func activeRulesetNames(rulesets []*github.RepositoryRuleset) []string {
	var names []string
	for _, ruleset := range rulesets {
		if ruleset.Enforcement == github.RulesetEnforcementActive {
			names = append(names, ruleset.Name)
		}
	}

	return names
}
