package githubapi

import (
	"reflect"
	"testing"

	"github.com/getyourguide/dependabutler/internal/pkg/config"
	"github.com/google/go-github/v90/github"
)

func sbomPackage(name string, packageURL string) *github.RepoDependencies {
	return &github.RepoDependencies{
		Name:         github.Ptr(name),
		ExternalRefs: []*github.PackageExternalRef{{ReferenceCategory: "PACKAGE-MANAGER", ReferenceType: "purl", ReferenceLocator: packageURL}},
	}
}

func TestDependenciesOf(t *testing.T) {
	sbom := &github.SBOM{SBOM: &github.SBOMInfo{Packages: []*github.RepoDependencies{
		sbomPackage("com.github.org/repo", "pkg:github/org/repo@main"),
		sbomPackage("org/actions/setup", "pkg:githubactions/org/actions/setup@v1"),
		sbomPackage("org/actions/setup", "pkg:githubactions/org/actions/setup@v2"),
		sbomPackage("@types/node", "pkg:npm/%40types/node@20.0.0"),
		{Name: github.Ptr("no-package-url")},
	}}}

	want := config.Dependencies{
		"github-actions": {"org/actions/setup"},
		"npm":            {"@types/node"},
		"bun":            {"@types/node"},
	}

	if got := dependenciesOf(sbom); !reflect.DeepEqual(got, want) {
		t.Errorf("dependenciesOf() = %v, expected %v", got, want)
	}
}

func TestDependenciesOfAnEmptySBOM(t *testing.T) {
	if got := dependenciesOf(&github.SBOM{}); len(got) != 0 || got == nil {
		t.Errorf("dependenciesOf() = %#v, expected an empty, non-nil value", got)
	}
}
