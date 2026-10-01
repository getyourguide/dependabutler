package config

import (
	"encoding/json"
	"testing"
)

func TestNewRepoReport(t *testing.T) {
	toolConfig := ToolConfig{
		ManifestPatterns: map[string]string{
			"npm":    "^(.*/)?package\\.json$",
			"docker": "^(.*/)?Dockerfile$",
			"gomod":  "^(.*/)?go\\.mod$",
		},
	}
	toolConfig.InitializePatterns()

	currentConfig := []byte(`version: 2

registries:
  npm-reg:
    type: npm-registry
    url: https://npm.example.com
    password: "${{secrets.NPM_PASSWORD}}"

updates:
  - package-ecosystem: npm
    directory: /
    registries:
      - npm-reg
    schedule:
      interval: weekly
      day: sunday
      time: "06:00"
      timezone: Europe/Berlin

  - package-ecosystem: docker
    directories:
      - /
      - /tools
    schedule:
      interval: cron
      cronjob: 0 3 * * 1,4
`)

	for _, tt := range []struct {
		name          string
		currentConfig []byte
		files         []string
		treeTruncated bool
		expected      string
	}{
		{
			name:          "covered and uncovered manifests",
			currentConfig: currentConfig,
			files:         []string{"web/package.json", "README.md", "Dockerfile", "tools/lint/Dockerfile", "go.mod"},
			expected: `{"repo":"acme","default_branch":"main","dependabot_yml":true,"tree_truncated":false,` +
				`"manifests":[` +
				`{"path":"Dockerfile","ecosystem":"docker","covered":true},` +
				`{"path":"go.mod","ecosystem":"gomod","covered":false},` +
				`{"path":"tools/lint/Dockerfile","ecosystem":"docker","covered":false},` +
				`{"path":"web/package.json","ecosystem":"npm","covered":true}],` +
				`"updates":[` +
				`{"ecosystem":"npm","directory":"/","schedule":{"interval":"weekly","day":"sunday","time":"06:00","timezone":"Europe/Berlin"}},` +
				`{"ecosystem":"docker","directories":["/","/tools"],"schedule":{"interval":"cron","cronjob":"0 3 * * 1,4"}}]}`,
		},
		{
			name:          "no manifests",
			currentConfig: currentConfig,
			files:         []string{"README.md"},
			expected: `{"repo":"acme","default_branch":"main","dependabot_yml":true,"tree_truncated":false,"manifests":[],` +
				`"updates":[` +
				`{"ecosystem":"npm","directory":"/","schedule":{"interval":"weekly","day":"sunday","time":"06:00","timezone":"Europe/Berlin"}},` +
				`{"ecosystem":"docker","directories":["/","/tools"],"schedule":{"interval":"cron","cronjob":"0 3 * * 1,4"}}]}`,
		},
		{
			name:          "no dependabot.yml",
			currentConfig: nil,
			files:         []string{"go.mod"},
			expected: `{"repo":"acme","default_branch":"main","dependabot_yml":false,"tree_truncated":false,` +
				`"manifests":[{"path":"go.mod","ecosystem":"gomod","covered":false}],"updates":[]}`,
		},
		{
			name:          "truncated file tree",
			currentConfig: nil,
			files:         []string{"go.mod"},
			treeTruncated: true,
			expected: `{"repo":"acme","default_branch":"main","dependabot_yml":false,"tree_truncated":true,` +
				`"manifests":[{"path":"go.mod","ecosystem":"gomod","covered":false}],"updates":[]}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			report, err := NewRepoReport("acme", "main", tt.currentConfig, tt.files, tt.treeTruncated)
			if err != nil {
				t.Fatalf("NewRepoReport() failed: %v", err)
			}

			got, err := json.Marshal(report)
			if err != nil {
				t.Fatalf("json.Marshal() failed: %v", err)
			}

			if string(got) != tt.expected {
				t.Errorf("NewRepoReport() JSON mismatch.\nExpected: %v\nGot:      %v", tt.expected, string(got))
			}
		})
	}
}

func TestNewRepoReportInvalidConfig(t *testing.T) {
	if _, err := NewRepoReport("acme", "main", []byte("updates: ["), nil, false); err == nil {
		t.Errorf("NewRepoReport() expected an error for an invalid dependabot.yml")
	}
}

func TestSkippedRepoReport(t *testing.T) {
	for _, tt := range []struct {
		report   SkippedRepoReport
		expected string
	}{
		{SkippedRepoReport{Repo: "acme", Skipped: "archived"}, `{"repo":"acme","skipped":"archived"}`},
		{SkippedRepoReport{Repo: "acme", Error: "404 Not Found"}, `{"repo":"acme","error":"404 Not Found"}`},
	} {
		got, err := json.Marshal(tt.report)
		if err != nil {
			t.Fatalf("json.Marshal() failed: %v", err)
		}

		if string(got) != tt.expected {
			t.Errorf("SkippedRepoReport JSON mismatch.\nExpected: %v\nGot:      %v", tt.expected, string(got))
		}
	}
}
