package config

import (
	"fmt"
	"slices"

	"github.com/getyourguide/dependabutler/internal/pkg/util"
	"go.yaml.in/yaml/v4"
)

// Ecosystems whose groups can use dependency-type.
var dependencyTypeEcosystems = []string{"bundler", "composer", "maven", "mix", "npm", "pip"}

// Groups holds the groups of an update in the order they are written. Dependabot puts a dependency in the first group
// it matches, so the order must survive a rewrite.
type Groups []NamedGroup

// NamedGroup is a group and its name.
type NamedGroup struct {
	Name  string
	Group Group
}

// UnmarshalYAML reads the groups mapping in its written order.
func (groups *Groups) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: groups must be a mapping", node.Line)
	}

	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		name := node.Content[i].Value
		if name == "<<" {
			return fmt.Errorf("line %d: merge keys (<<) are not supported in groups", node.Content[i].Line)
		}
		if seen[name] {
			return fmt.Errorf("line %d: group %q is defined more than once", node.Content[i].Line, name)
		}

		var group Group
		if err := node.Content[i+1].Decode(&group); err != nil {
			return err
		}

		seen[name] = true
		*groups = append(*groups, NamedGroup{Name: name, Group: group})
	}

	return nil
}

// MarshalYAML writes the groups as a mapping in their order.
func (groups Groups) MarshalYAML() (any, error) {
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	for _, named := range groups {
		value := &yaml.Node{}
		if err := value.Encode(named.Group); err != nil {
			return nil, err
		}

		key := &yaml.Node{}
		if err := key.Encode(named.Name); err != nil {
			return nil, err
		}

		mapping.Content = append(mapping.Content, key, value)
	}

	return mapping, nil
}

// Get returns the group with the given name.
func (groups Groups) Get(name string) (Group, bool) {
	for _, named := range groups {
		if named.Name == name {
			return named.Group, true
		}
	}

	return Group{}, false
}

func (config *ToolConfig) stableGroupPrefixes() bool {
	return config.StableGroupPrefixes == nil || *config.StableGroupPrefixes
}

func (config *ToolConfig) validateGroups() error {
	if err := validateGroups("update-defaults", "", config.UpdateDefaults.Groups); err != nil {
		return err
	}

	ecosystems := make([]string, 0, len(config.UpdateOverrides))
	for ecosystem := range config.UpdateOverrides {
		ecosystems = append(ecosystems, ecosystem)
	}

	slices.Sort(ecosystems)
	for _, ecosystem := range ecosystems {
		if err := validateGroups("update-overrides."+ecosystem, ecosystem, config.UpdateOverrides[ecosystem].Groups); err != nil {
			return err
		}
	}

	return nil
}

// validateGroups rejects dependency-type where Dependabot does not support it, and a catch-all group before other
// groups of the same kind: Dependabot puts a dependency in the first group it matches, so those would stay empty.
func validateGroups(section string, ecosystem string, groups Groups) error {
	for i, named := range groups {
		if named.Group.DependencyType != "" && !util.Contains(dependencyTypeEcosystems, ecosystem) {
			return fmt.Errorf("group %q in %v has a dependency-type, which only %v support", named.Name, section, dependencyTypeEcosystems)
		}

		if !named.Group.catchesAll() {
			continue
		}

		for _, later := range groups[i+1:] {
			if later.Group.appliesTo() == named.Group.appliesTo() {
				return fmt.Errorf("group %q in %v matches every dependency, so group %q after it stays empty: put %q last",
					named.Name, section, later.Name, named.Name)
			}
		}
	}

	return nil
}

func (group Group) catchesAll() bool {
	return slices.Contains(group.Patterns, "*") && group.DependencyType == "" && len(group.UpdateTypes) == 0 &&
		len(group.ExcludePatterns) == 0
}

func (group Group) appliesTo() string {
	if group.AppliesTo == "" {
		return "version-updates"
	}

	return group.AppliesTo
}
