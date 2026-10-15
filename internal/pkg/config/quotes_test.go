package config

import (
	"testing"
)

func TestToYamlKeepingQuotesKeepsTheQuotesOfWrittenValues(t *testing.T) {
	original := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
      time: '07:00'
    ignore:
      - dependency-name: '@anthropic-ai/sdk'
    cooldown:
      default-days: 3
      exclude:
        - "*getyourguide*"
`
	expected := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
      time: '07:00'
    ignore:
      - dependency-name: '@anthropic-ai/sdk'
      - dependency-name: "@new/package"
    cooldown:
      default-days: 3
      exclude:
        - "*getyourguide*"
`

	parsedConfig, err := ParseDependabotConfig([]byte(original))
	if err != nil {
		t.Fatalf("ParseDependabotConfig() failed: %v", err)
	}

	parsedConfig.Updates[0].Ignore = append(parsedConfig.Updates[0].Ignore, Ignore{DependencyName: "@new/package"})

	if got := string(parsedConfig.ToYamlKeepingQuotes([]byte(original))); got != expected {
		t.Errorf("ToYamlKeepingQuotes() changed written quotes or did not double-quote the new value.\nExpected:\n%v\nGot:\n%v", expected, got)
	}
}

func TestToYamlKeepingQuotesEscapesValuesThatCannotBeSingleQuoted(t *testing.T) {
	original := "version: 2\nupdates:\n  - package-ecosystem: npm\n    directory: /\n    commit-message:\n      prefix: 'tab\there'\n"

	parsedConfig, err := ParseDependabotConfig([]byte(original))
	if err != nil {
		t.Fatalf("ParseDependabotConfig() failed: %v", err)
	}

	output := parsedConfig.ToYamlKeepingQuotes([]byte(original))

	reparsed, err := ParseDependabotConfig(output)
	if err != nil || reparsed.Updates[0].CommitMessage.Prefix != "tab\there" {
		t.Errorf("ToYamlKeepingQuotes() wrote %q, which does not read back as the prefix with a tab", output)
	}
}

func TestWrittenQuoteStylesOfInvalidYaml(t *testing.T) {
	if styles := writtenQuoteStyles([]byte("updates: [")); len(styles) != 0 {
		t.Errorf("writtenQuoteStyles() = %v, expected no styles for invalid YAML", styles)
	}
}
