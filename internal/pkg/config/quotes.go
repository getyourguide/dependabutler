package config

import (
	"io"
	"strings"

	"go.yaml.in/yaml/v4"
)

const quotedStyles = yaml.SingleQuotedStyle | yaml.DoubleQuotedStyle

// quoteStyles maps the quoted values of a written dependabot.yml to their quote style.
type quoteStyles map[string]yaml.Style

// writtenQuoteStyles reads the quote style of every quoted value in a dependabot.yml. Invalid YAML has none.
func writtenQuoteStyles(data []byte) quoteStyles {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil
	}

	styles := quoteStyles{}
	styles.collect(&node)

	return styles
}

func (styles quoteStyles) collect(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode && node.Style&quotedStyles != 0 {
		styles[node.Value] = node.Style & quotedStyles
	}
	for _, child := range node.Content {
		styles.collect(child)
	}
}

// encodeKeepingQuotes writes YAML in which every value that needs quotes keeps the quotes it had in written, and
// otherwise gets the quotes Prettier would give it. Quote changes would fail the checks of repositories that format
// their dependabot.yml.
func encodeKeepingQuotes(w io.Writer, value any, written quoteStyles) error {
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return err
	}

	written.apply(&node)

	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)

	return encoder.Encode(&node)
}

// apply sets the quote style of every value that the YAML library quoted. The library single-quotes strings that YAML
// 1.1 would read as another type, like 07:00 or on, whatever quote style is preferred.
func (styles quoteStyles) apply(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode && node.Style&quotedStyles != 0 {
		node.Style = styles.styleFor(node.Value)
	}
	for _, child := range node.Content {
		styles.apply(child)
	}
}

// styleFor returns the written style of a value, or the style Prettier picks: double quotes unless the value has more
// double quotes than single quotes. The encoder still uses double quotes where single quotes cannot hold the value.
func (styles quoteStyles) styleFor(value string) yaml.Style {
	if style, ok := styles[value]; ok {
		return style
	}
	if strings.Count(value, `"`) > strings.Count(value, "'") {
		return yaml.SingleQuotedStyle
	}

	return yaml.DoubleQuotedStyle
}
