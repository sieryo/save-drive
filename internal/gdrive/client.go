// Package gdrive wraps the Google Drive API with the operations save-drive
// needs: authentication, folder path resolution, and file transfer. It is the
// only package that talks to the Drive API.
package gdrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
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
// OAuth it loads a token cached on disk, running the interactive flow only
// when no usable token file exists.
func New(ctx context.Context, credsPath string) (*Client, error) {
	return newClient(ctx, credsPath, false)
}

// Authorize is like New, but for OAuth credentials it always runs the
// interactive flow and overwrites the cached token. Use it to recover from a
// revoked or expired refresh token.
func Authorize(ctx context.Context, credsPath string) (*Client, error) {
	return newClient(ctx, credsPath, true)
}

func newClient(ctx context.Context, credsPath string, forceLogin bool) (*Client, error) {
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

	var ts oauth2.TokenSource
	switch {
	case meta.Type == "service_account":
		creds, err := google.CredentialsFromJSONWithType(ctx, b, google.ServiceAccount, drive.DriveScope)
		if err != nil {
			return nil, fmt.Errorf("load service account credentials: %w", err)
		}
		ts = creds.TokenSource
	case len(meta.Installed) > 0 || len(meta.Web) > 0:
		conf, err := google.ConfigFromJSON(b, drive.DriveScope)
		if err != nil {
			return nil, fmt.Errorf("load oauth config: %w", err)
		}
		// Only "installed" (Desktop app) clients accept any loopback port; "web"
		// clients need the exact redirect URI registered in Google Cloud.
		fixedPort := len(meta.Installed) == 0
		ts, err = oauthTokenSource(ctx, conf, config.DefaultTokenFile(), forceLogin, fixedPort)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported credentials json; use OAuth client json (Desktop app) or service account json")
	}

	hc := withRetry(oauth2.NewClient(ctx, ts), defaultRetryBackoff)
	return newWithHTTP(ctx, hc, "")
}

// newWithHTTP builds a Client on top of an already-authorized HTTP client.
// Tests use it to point the client at a fake Drive server via endpoint; an
// empty endpoint means the real Drive API.
func newWithHTTP(ctx context.Context, hc *http.Client, endpoint string) (*Client, error) {
	opts := []option.ClientOption{option.WithHTTPClient(hc)}
	if endpoint != "" {
		opts = append(opts, option.WithEndpoint(endpoint))
	}
	svc, err := drive.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("init drive service: %w", err)
	}
	return &Client{svc: svc}, nil
}

// WhoAmI returns the Google account the client is acting as.
func (c *Client) WhoAmI(ctx context.Context) (*drive.User, error) {
	about, err := c.svc.About.Get().Fields("user(displayName,emailAddress)").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get drive account: %w", err)
	}
	return about.User, nil
}
