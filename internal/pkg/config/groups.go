package config

import (
	"fmt"

	"go.yaml.in/yaml/v4"
)

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
