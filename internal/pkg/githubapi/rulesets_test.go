package githubapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/go-github/v90/github"
)

func TestActiveRulesetNames(t *testing.T) {
	rulesets := []*github.RepositoryRuleset{
		{Name: "audited", Enforcement: github.RulesetEnforcementActive},
		{Name: "trial", Enforcement: github.RulesetEnforcementEvaluate},
		{Name: "switched-off", Enforcement: github.RulesetEnforcementDisabled},
		{Name: "default", Enforcement: github.RulesetEnforcementActive},
	}

	got := activeRulesetNames(rulesets)

	expected := []string{"audited", "default"}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("activeRulesetNames() = %v, expected %v", got, expected)
	}
}

func TestGetActiveRulesetNamesReadsEveryPage(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("includes_parents") != "true" {
			t.Errorf("rulesets requested without includes_parents=true: %v", r.URL)
		}

		w.Header().Set("Content-Type", "application/json")

		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"name": "audited", "enforcement": "active"}]`))
			return
		}

		w.Header().Set("Link", fmt.Sprintf(`<%v/repos/acme/app/rulesets?includes_parents=true&page=2>; rel="next"`, server.URL))
		_, _ = w.Write([]byte(`[{"name": "default", "enforcement": "active"}]`))
	}))
	defer server.Close()

	baseURL := server.URL + "/"
	gh, err := github.NewClient(github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatal(err)
	}

	client := &Client{gh: gh}

	got, err := client.GetActiveRulesetNames("acme", "app")
	if err != nil {
		t.Fatalf("GetActiveRulesetNames() failed: %v", err)
	}

	expected := []string{"default", "audited"}

	if !reflect.DeepEqual(got, expected) {
		t.Errorf("GetActiveRulesetNames() = %v, expected %v", got, expected)
	}
}

func TestGetCustomPropertyValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"property_name": "owner", "value": "team-a"}, {"property_name": "dependabot-grouping", "value": "per-app"}]`))
	}))
	defer server.Close()

	baseURL := server.URL + "/"
	gh, err := github.NewClient(github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{gh: gh}

	for _, tt := range []struct {
		property string
		value    string
	}{
		{"dependabot-grouping", "per-app"},
		{"not-set", ""},
	} {
		got, err := client.GetCustomPropertyValue("acme", "app", tt.property)
		if err != nil {
			t.Fatalf("GetCustomPropertyValue() failed: %v", err)
		}

		if got != tt.value {
			t.Errorf("GetCustomPropertyValue(%q) = %q, expected %q", tt.property, got, tt.value)
		}
	}
}
