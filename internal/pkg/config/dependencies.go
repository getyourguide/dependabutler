package config

import (
	"slices"
	"strings"
)

// Dependencies holds the names of the dependencies of a repository by package ecosystem, as the dependency graph lists
// them. A nil value means they were not read.
type Dependencies map[string][]string

// Package URL types of the dependency graph and the package ecosystems their dependencies belong to. Ecosystems that are
// missing here, like docker, are not covered by the dependency graph.
var packageTypeEcosystems = map[string][]string{
	"cargo":         {"cargo"},
	"composer":      {"composer"},
	"gem":           {"bundler"},
	"githubactions": {"github-actions"},
	"golang":        {"gomod"},
	"hex":           {"mix"},
	"maven":         {"gradle", "maven"},
	"npm":           {"bun", "npm"},
	"nuget":         {"nuget"},
	"pub":           {"pub"},
	"pypi":          {"pip", "uv"},
	"swift":         {"swift"},
}

// EcosystemsOfPackageURL returns the package ecosystems of a package URL like "pkg:npm/lodash@4.17.21", or nil when
// its type belongs to none.
func EcosystemsOfPackageURL(packageURL string) []string {
	rest, found := strings.CutPrefix(packageURL, "pkg:")
	if !found {
		return nil
	}

	packageType, _, _ := strings.Cut(rest, "/")
	return packageTypeEcosystems[strings.ToLower(packageType)]
}

// Add records a dependency for each of the ecosystems, once.
func (dependencies Dependencies) Add(name string, ecosystems []string) {
	for _, ecosystem := range ecosystems {
		if !slices.Contains(dependencies[ecosystem], name) {
			dependencies[ecosystem] = append(dependencies[ecosystem], name)
		}
	}
}

func dependencyGraphCovers(ecosystem string) bool {
	for _, ecosystems := range packageTypeEcosystems {
		if slices.Contains(ecosystems, ecosystem) {
			return true
		}
	}

	return false
}

// usedGroups returns the groups without the only-if-used groups that no dependency of the ecosystem matches. Kept groups
// lose the flag, as Dependabot does not know it.
func usedGroups(groups Groups, ecosystem string, dependencies Dependencies) Groups {
	if !slices.ContainsFunc(groups, func(named NamedGroup) bool { return named.Group.OnlyIfUsed }) {
		return groups
	}

	used := make(Groups, 0, len(groups))
	for _, named := range groups {
		if !named.Group.OnlyIfUsed {
			used = append(used, named)
			continue
		}

		if named.Group.matchesAny(dependencies[ecosystem]) {
			named.Group.OnlyIfUsed = false
			used = append(used, named)
		}
	}

	return used
}

func (group Group) matchesAny(names []string) bool {
	return slices.ContainsFunc(names, group.matches)
}

func (group Group) matches(name string) bool {
	matchesName := func(pattern string) bool { return matchesPattern(pattern, name) }

	return slices.ContainsFunc(group.Patterns, matchesName) && !slices.ContainsFunc(group.ExcludePatterns, matchesName)
}

// matchesPattern reports whether a dependency name matches a group pattern. As in Dependabot, "*" matches any
// characters, including "/", and case is ignored.
func matchesPattern(pattern string, name string) bool {
	parts := strings.Split(strings.ToLower(pattern), "*")
	name = strings.ToLower(name)
	if len(parts) == 1 {
		return name == parts[0]
	}

	rest, found := strings.CutPrefix(name, parts[0])
	if !found {
		return false
	}

	for _, part := range parts[1 : len(parts)-1] {
		index := strings.Index(rest, part)
		if index < 0 {
			return false
		}

		rest = rest[index+len(part):]
	}

	return strings.HasSuffix(rest, parts[len(parts)-1])
}
