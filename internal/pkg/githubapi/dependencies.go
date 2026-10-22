package githubapi

import (
	"context"

	"github.com/getyourguide/dependabutler/internal/pkg/config"
	"github.com/google/go-github/v90/github"
)

// GetDependencies returns the dependencies of a repo by package ecosystem, read from its dependency graph.
func (client *Client) GetDependencies(org string, repo string) (config.Dependencies, error) {
	sbom, resp, err := client.gh.DependencyGraph.GetSBOM(context.Background(), org, repo)
	client.observe(resp, err)
	if err != nil {
		return nil, err
	}

	return dependenciesOf(sbom), nil
}

// dependenciesOf takes the ecosystem of each package from its package URL. Packages without one, like the repository
// itself, are skipped.
func dependenciesOf(sbom *github.SBOM) config.Dependencies {
	dependencies := config.Dependencies{}
	if sbom.SBOM == nil {
		return dependencies
	}

	for _, pkg := range sbom.SBOM.Packages {
		for _, ref := range pkg.ExternalRefs {
			if ecosystems := config.EcosystemsOfPackageURL(ref.ReferenceLocator); len(ecosystems) > 0 {
				dependencies.Add(pkg.GetName(), ecosystems)
				break
			}
		}
	}

	return dependencies
}
