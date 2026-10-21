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
		{
			name: "comment after an inline list",
			input: `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
    ignore:
      - dependency-name: tsx
        update-types: ["version-update:semver-major"] # breaks build
      - dependency-name: node
`,
			expected: `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
    ignore:
      - dependency-name: tsx
        update-types: # breaks build
          - "version-update:semver-major"
      - dependency-name: node
`,
		},
		{
			name: "rules for the same dependency",
			input: `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
    ignore:
      - dependency-name: "@types/node"
        update-types:
          - version-update:semver-major
      # 22.3 breaks the build
      - dependency-name: "@types/node"
        versions:
          - 22.3.x
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
	tests := []struct {
		name    string
		ignore  string
		change  func(ignore []Ignore) []Ignore
		warning string
	}{
		{
			name: "removed item",
			ignore: `      # waiting for a fix
      - dependency-name: tsx
`,
			change:  func([]Ignore) []Ignore { return nil },
			warning: `WARN  Dropped the comment "# waiting for a fix" of $.updates[npm /].ignore[tsx]`,
		},
		{
			name: "changed item",
			ignore: `      # waiting for a fix
      - dependency-name: tsx
        versions:
          - 4.23.x
`,
			change: func(ignore []Ignore) []Ignore {
				ignore[0].Versions = []string{"4.24.x"}
				return ignore
			},
			warning: `WARN  Dropped the comment "# waiting for a fix" of $.updates[npm /].ignore[tsx 4.23.x]`,
		},
		{
			name: "item with an identical twin",
			ignore: `      - dependency-name: tsx
      # waiting for a fix
      - dependency-name: tsx
`,
			change:  func(ignore []Ignore) []Ignore { return ignore },
			warning: `WARN  Dropped the comment "# waiting for a fix" of $.updates[npm /].ignore[tsx]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := `version: 2
updates:
  - package-ecosystem: npm
    directory: /
    schedule:
      interval: daily
    ignore:
` + tt.ignore

			parsedConfig, err := ParseDependabotConfig([]byte(original))
			if err != nil {
				t.Fatalf("ParseDependabotConfig() failed: %v", err)
			}

			parsedConfig.Updates[0].Ignore = tt.change(parsedConfig.Updates[0].Ignore)

			var logs bytes.Buffer
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(os.Stderr) })

			got := string(parsedConfig.ToYamlKeepingQuotes([]byte(original)))

			if strings.Contains(got, "waiting for a fix") {
				t.Errorf("ToYamlKeepingQuotes() kept a comment it cannot place:\n%v", got)
			}
			if !strings.Contains(logs.String(), tt.warning) {
				t.Errorf("ToYamlKeepingQuotes() did not warn about the dropped comment, logged:\n%v", logs.String())
			}
		})
	}
}

func TestReadCommentsOfInvalidYaml(t *testing.T) {
	if comments := readComments([]byte("updates: [ # comment")); len(comments.byPath) != 0 {
		t.Errorf("readComments() = %v, expected no comments for invalid YAML", comments)
	}
}
