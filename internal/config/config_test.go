package config

import (
	"path/filepath"
	"testing"
)

func TestGetenvOr(t *testing.T) {
	t.Setenv("SAVE_DRIVE_TEST_VAR", "")
	if got := GetenvOr("SAVE_DRIVE_TEST_VAR", "fallback"); got != "fallback" {
		t.Errorf("empty: got %q", got)
	}
	t.Setenv("SAVE_DRIVE_TEST_VAR", "set")
	if got := GetenvOr("SAVE_DRIVE_TEST_VAR", "fallback"); got != "set" {
		t.Errorf("set: got %q", got)
	}
}

func TestDefaultCredsPath(t *testing.T) {
	tests := []struct{ oauth, app, want string }{
		{"", "", ""},
		{"", "sa.json", "sa.json"},
		{"oauth.json", "sa.json", "oauth.json"},
	}
	for _, tt := range tests {
		t.Setenv(EnvOAuthCreds, tt.oauth)
		t.Setenv(EnvAppCreds, tt.app)
		if got := DefaultCredsPath(); got != tt.want {
			t.Errorf("oauth=%q app=%q: got %q, want %q", tt.oauth, tt.app, got, tt.want)
		}
	}
}

func TestDefaults(t *testing.T) {
	t.Setenv(EnvTokenFile, "")
	t.Setenv(EnvRootID, "")
	t.Setenv(EnvLogFile, "")
	if got, want := DefaultTokenFile(), filepath.Join(".save-drive", "oauth-token.json"); got != want {
		t.Errorf("DefaultTokenFile = %q, want %q", got, want)
	}
	if got := DefaultRootID(); got != "root" {
		t.Errorf("DefaultRootID = %q", got)
	}
	if got := DefaultLogPath(); filepath.Base(got) != "audit.jsonl" || filepath.Base(filepath.Dir(got)) != "logs" {
		t.Errorf("DefaultLogPath = %q", got)
	}

	t.Setenv(EnvLogFile, "custom.jsonl")
	if got := DefaultLogPath(); got != "custom.jsonl" {
		t.Errorf("DefaultLogPath override = %q", got)
	}
}
