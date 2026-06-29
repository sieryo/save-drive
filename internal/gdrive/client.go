// Package gdrive wraps the Google Drive API with the small set of operations
// save-drive needs: building an authenticated client and resolving folder paths.
package gdrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"save-drive/internal/config"
)

// FolderMimeType is the Drive MIME type that identifies a folder.
const FolderMimeType = "application/vnd.google-apps.folder"

// Client is an authenticated Google Drive client.
type Client struct {
	svc *drive.Service
}

// credsMeta is the minimal shape used to detect the credentials JSON variant.
type credsMeta struct {
	Type      string          `json:"type"`
	Installed json.RawMessage `json:"installed"`
	Web       json.RawMessage `json:"web"`
}

// New builds an authenticated Drive client from a credentials JSON file. It
// supports both service-account credentials and OAuth client credentials; for
// OAuth it loads (or interactively obtains) a token cached on disk.
func New(ctx context.Context, credsPath string) (*Client, error) {
	if strings.TrimSpace(credsPath) == "" {
		return nil, errors.New("credentials file not set; use --creds or set GOOGLE_OAUTH_CLIENT_CREDENTIALS")
	}

	b, err := os.ReadFile(credsPath)
	if err != nil {
		return nil, fmt.Errorf("read credentials file: %w", err)
	}

	var meta credsMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, fmt.Errorf("parse credentials file: %w", err)
	}

	if meta.Type == "service_account" {
		svc, err := drive.NewService(ctx, option.WithScopes(drive.DriveScope), option.WithCredentialsFile(credsPath))
		if err != nil {
			return nil, fmt.Errorf("init drive service (service account): %w", err)
		}
		return &Client{svc: svc}, nil
	}

	if len(meta.Installed) == 0 && len(meta.Web) == 0 {
		return nil, errors.New("unsupported credentials json; use OAuth client json (Desktop app) or service account json")
	}

	conf, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("load oauth config: %w", err)
	}
	httpClient, err := oauthClient(ctx, conf, config.DefaultTokenFile())
	if err != nil {
		return nil, err
	}

	svc, err := drive.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("init drive service (oauth): %w", err)
	}
	return &Client{svc: svc}, nil
}

// Service exposes the underlying Drive service for direct API calls.
func (c *Client) Service() *drive.Service { return c.svc }
