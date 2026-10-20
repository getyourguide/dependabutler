package config

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
)

// identityKeys are the keys that tell the items of a list apart. Items are matched by them instead of by position,
// because a rewrite sorts the update entries and adds or removes items. versions, update-types and dependency-type tell
// apart the rules of an ignore or allow list for the same dependency.
var identityKeys = []string{
	"package-ecosystem", "directory", "directories", "target-branch",
	"dependency-name", "dependency-type", "versions", "update-types",
}

type comment struct {
	head, line, foot string
}

func (c comment) String() string {
	parts := []string{}
	for _, part := range []string{c.head, c.line, c.foot} {
		if part != "" {
			parts = append(parts, part)
		}
	}

	return strings.Join(parts, "\n")
}

// writtenComments holds the comments of a dependabot.yml by the path of their node.
type writtenComments struct {
	byPath map[string][]comment

	// shared are the paths of several nodes, like identical items of a list, whose comments cannot be matched
	shared map[string]bool
}

// readComments reads the comments of a dependabot.yml. Invalid YAML has none.
func readComments(data []byte) writtenComments {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return writtenComments{}
	}

	moveInlineCollectionComments(&node)

	comments := writtenComments{byPath: map[string][]comment{}, shared: sharedPaths(&node)}
	walkPaths(&node, "", func(path string, n *yaml.Node) {
		if n.HeadComment != "" || n.LineComment != "" || n.FootComment != "" {
			comments.byPath[path] = append(comments.byPath[path], comment{n.HeadComment, n.LineComment, n.FootComment})
		}
	})

	return comments
}

// moveInlineCollectionComments moves the comment after an inline list or mapping, like `versions: [1.x] # broken`, to
// its key. The rewrite writes the collection as a block, which would put the comment on the next line, often another
// item of the list.
func moveInlineCollectionComments(node *yaml.Node) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if value.Kind != yaml.ScalarNode && value.LineComment != "" && key.LineComment == "" {
				key.LineComment, value.LineComment = value.LineComment, ""
			}
		}
	}

	for _, child := range node.Content {
		moveInlineCollectionComments(child)
	}
}

// apply puts every comment on the node with its path. A comment whose node is gone, or whose path is shared by several
// nodes in the file or the rewrite, is dropped with a warning, so the loss shows in the log instead of only in the pull
// request.
func (comments writtenComments) apply(document *yaml.Node) {
	shared := sharedPaths(document)
	placed := map[string]bool{}
	walkPaths(document, "", func(path string, n *yaml.Node) {
		if comments.shared[path] || shared[path] {
			return
		}
		if c, ok := comments.byPath[path]; ok {
			n.HeadComment, n.LineComment, n.FootComment = c[0].head, c[0].line, c[0].foot
			placed[path] = true
		}
	})

	dropped := []string{}
	for path := range comments.byPath {
		if !placed[path] {
			dropped = append(dropped, path)
		}
	}

	slices.Sort(dropped)
	for _, path := range dropped {
		for _, c := range comments.byPath[path] {
			log.Printf("WARN  Dropped the comment %q of %v, which is no longer in dependabot.yml.", c, path)
		}
	}
}

func sharedPaths(node *yaml.Node) map[string]bool {
	seen := map[string]bool{}
	shared := map[string]bool{}
	walkPaths(node, "", func(path string, _ *yaml.Node) {
		shared[path] = seen[path]
		seen[path] = true
	})

	return shared
}

// walkPaths calls visit with every node and its path, like $.updates[npm /].ignore[tsx] for the item of an ignore
// list, or $.updates[npm /].schedule(key) for the key of a mapping.
func walkPaths(node *yaml.Node, path string, visit func(string, *yaml.Node)) {
	visit(path, node)

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			walkPaths(child, path+"$", visit)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyPath := path + "." + node.Content[i].Value
			walkPaths(node.Content[i], keyPath+"(key)", visit)
			walkPaths(node.Content[i+1], keyPath, visit)
		}
	case yaml.SequenceNode:
		for i, identity := range itemIdentities(node.Content) {
			walkPaths(node.Content[i], path+"["+identity+"]", visit)
		}
	}
}

// itemIdentities names the items of a list. Items without a name are named by their position.
func itemIdentities(items []*yaml.Node) []string {
	identities := make([]string, len(items))
	for i, item := range items {
		identities[i] = itemIdentity(item)
		if identities[i] == "" {
			identities[i] = fmt.Sprintf("#%d", i)
		}
	}

	return identities
}

// itemIdentity is the value of a scalar, or the values of the identity keys of a mapping. The values are taken in the
// order of identityKeys, since the file can have the keys in another order than the rewrite.
func itemIdentity(item *yaml.Node) string {
	if item.Kind == yaml.ScalarNode {
		return item.Value
	}

	values := []string{}
	for _, key := range identityKeys {
		for i := 0; i+1 < len(item.Content); i += 2 {
			if item.Content[i].Value == key {
				values = append(values, scalarValues(item.Content[i+1]))
			}
		}
	}

	return strings.Join(values, " ")
}

func scalarValues(node *yaml.Node) string {
	if node.Kind == yaml.ScalarNode {
		return node.Value
	}

	values := []string{}
	for _, child := range node.Content {
		values = append(values, scalarValues(child))
	}

	return strings.Join(values, ",")
}
