// Package config resolves environment variables and default file paths used
// across the save-drive CLI.
package config

import (
	"os"
	"path/filepath"
)

// Environment variable names recognized by save-drive.
const (
	EnvOAuthCreds = "GOOGLE_OAUTH_CLIENT_CREDENTIALS"
	EnvAppCreds   = "GOOGLE_APPLICATION_CREDENTIALS"
	EnvTokenFile  = "GOOGLE_OAUTH_TOKEN_FILE"
	EnvLogFile    = "SAVE_DRIVE_LOG_FILE"
	EnvRootID     = "DRIVE_ROOT_ID"
)

// GetenvOr returns the value of key, or fallback when the variable is unset or empty.
func GetenvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// DefaultCredsPath prefers the OAuth client credentials, falling back to the
// service account credentials variable.
func DefaultCredsPath() string {
	if v := os.Getenv(EnvOAuthCreds); v != "" {
		return v
	}
	return os.Getenv(EnvAppCreds)
}

// DefaultTokenFile is where the cached OAuth token lives unless overridden.
func DefaultTokenFile() string {
	return GetenvOr(EnvTokenFile, filepath.Join(".save-drive", "oauth-token.json"))
}

// DefaultRootID is the Drive folder used as the root for path resolution.
func DefaultRootID() string {
	return GetenvOr(EnvRootID, "root")
}

// DefaultLogPath resolves the audit log location. It honors SAVE_DRIVE_LOG_FILE,
// otherwise it sits next to the executable under logs/audit.jsonl.
func DefaultLogPath() string {
	if v := os.Getenv(EnvLogFile); v != "" {
		return v
	}
	exePath, err := os.Executable()
	if err != nil {
		return filepath.Join("logs", "audit.jsonl")
	}
	return filepath.Join(filepath.Dir(exePath), "logs", "audit.jsonl")
}
