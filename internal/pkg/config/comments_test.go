package config

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

func TestToYamlKeepingQuotesKeepsComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "comment above an item of a list",
			input: `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
    ignore:
      - dependency-name: node
      # Temporary: tsx 4.23.15 causes an Apollo HttpLink local startup regression.
      # Remove after verifying a compatible fixed tsx release.
      - dependency-name: tsx
`,
		},
		{
			name: "comment at the top of the file",
			input: `# Managed by the platform team.

version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
`,
		},
		{
			name: "comment above a key and after a value",
			input: `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    # weekly is too slow for security fixes
    schedule:
      interval: daily # every weekday
    open-pull-requests-limit: 5
`,
		},
		{
			name: "comment above an update entry and in a list of directories",
			input: `version: 2
updates:
  # the apps
  - package-ecosystem: docker
    directories:
      - /apps/a
      # b is deployed by hand
      - /apps/b
    schedule:
      interval: weekly
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
`,
		},
		{
			name: "secret expression in a comment",
			input: `version: 2
registries:
  npm-reg:
    type: npm-registry
    url: https://npm.example.com
    # rotated by ${{secrets.NPM_TOKEN}} owners
    token: "${{secrets.NPM_TOKEN}}"
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
`,
		},
		{
			name: "entry with the keys in another order",
			input: `version: 2
updates:
  # the root
  - directory: /
    package-ecosystem: npm
    schedule:
      interval: daily # nightly
`,
			expected: `version: 2
updates:
  # the root
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily # nightly
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedConfig, err := ParseDependabotConfig([]byte(tt.input))
			if err != nil {
				t.Fatalf("ParseDependabotConfig() failed: %v", err)
			}

			expected := tt.expected
			if expected == "" {
				expected = tt.input
			}

			if got := string(parsedConfig.ToYamlKeepingQuotes([]byte(tt.input))); got != expected {
				t.Errorf("ToYamlKeepingQuotes() did not keep the comments.\nExpected:\n%v\nGot:\n%v", expected, got)
			}
		})
	}
}

func TestToYamlKeepingQuotesMovesCommentsWithTheirEntry(t *testing.T) {
	original := `version: 2
updates:
  # JavaScript packages
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
  # Go modules
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: weekly
`
	expected := `version: 2
updates:
  # Go modules
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: weekly
  # JavaScript packages
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
`

	parsedConfig, err := ParseDependabotConfig([]byte(original))
	if err != nil {
		t.Fatalf("ParseDependabotConfig() failed: %v", err)
	}

	if got := string(parsedConfig.ToYamlKeepingQuotes([]byte(original))); got != expected {
		t.Errorf("ToYamlKeepingQuotes() did not move the comments with the sorted entries.\nExpected:\n%v\nGot:\n%v", expected, got)
	}
}

func TestToYamlKeepingQuotesWarnsAboutDroppedComments(t *testing.T) {
	original := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
    ignore:
      # waiting for a fix
      - dependency-name: tsx
`

	parsedConfig, err := ParseDependabotConfig([]byte(original))
	if err != nil {
		t.Fatalf("ParseDependabotConfig() failed: %v", err)
	}

	parsedConfig.Updates[0].Ignore = nil

	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	got := string(parsedConfig.ToYamlKeepingQuotes([]byte(original)))

	if strings.Contains(got, "waiting for a fix") {
		t.Errorf("ToYamlKeepingQuotes() kept the comment of a removed item:\n%v", got)
	}
	if !strings.Contains(logs.String(), `WARN  Dropped the comment "# waiting for a fix" of $.updates[npm /].ignore[tsx]`) {
		t.Errorf("ToYamlKeepingQuotes() did not warn about the dropped comment, logged:\n%v", logs.String())
	}
}

func TestReadCommentsOfInvalidYaml(t *testing.T) {
	if comments := readComments([]byte("updates: [ # comment")); len(comments) != 0 {
		t.Errorf("readComments() = %v, expected no comments for invalid YAML", comments)
	}
}
