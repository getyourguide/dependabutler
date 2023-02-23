package util

import (
	"os"
	"testing"
)

func TestCompileRePattern(t *testing.T) {
	for _, tt := range []struct {
		pattern     string
		expectedNil bool
		matchSample string
	}{
		{"", false, ""},
		{"invalid(pattern", true, ""},
		{"^a(b|c)d*e*f+g$", false, "acdddddddfffg"},
	} {
		got := CompileRePattern(tt.pattern)
		if tt.expectedNil != (got == nil) {
			t.Errorf("CompileRePattern() failed; expected %t, got %t", tt.expectedNil, (got == nil))
		}
		if got != nil && !got.MatchString(tt.matchSample) {
			t.Errorf("CompileRePattern() failed; sample %v not matching", tt.matchSample)
		}
	}
}

func TestGetEnvParameter(t *testing.T) {
	os.Setenv("TEST_ENV_VAR_NAME_1337_NONEMPTY", "value")
	os.Setenv("TEST_ENV_VAR_NAME_1337_EMPTY", "")
	for _, tt := range []struct {
		name      string
		mandatory bool
		expected  string
	}{
		{"TEST_ENV_VAR_NAME_1337_NONEMPTY", true, "value"},
		{"TEST_ENV_VAR_NAME_1337_NONEMPTY", false, "value"},
		{"TEST_ENV_VAR_NAME_1337_EMPTY", false, ""},
		{"TEST_ENV_VAR_NAME_1337_UNKNOWN", false, ""},
	} {
		got := GetEnvParameter(tt.name, tt.mandatory)
		if got != tt.expected {
			t.Errorf("GetEnvParameter() failed; expected %v, got %v", tt.expected, got)
		}
	}
	os.Unsetenv("TEST_ENV_VAR_NAME_1337_NONEMPTY")
	os.Unsetenv("TEST_ENV_VAR_NAME_1337_EMPTY")
}
