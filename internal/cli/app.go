package cli

import (
	"context"

	"save-drive/internal/audit"
	"save-drive/internal/config"
	"save-drive/internal/gdrive"
)

// app bundles the dependencies shared by the data commands: an authenticated
// Drive client and the audit logger.
type app struct {
	client *gdrive.Client
	logger *audit.Logger
}

// newApp builds a Drive client from credsPath and attaches the default logger.
func newApp(credsPath string) (*app, error) {
	client, err := gdrive.New(context.Background(), credsPath)
	if err != nil {
		return nil, err
	}
	return &app{client: client, logger: audit.NewLogger(config.DefaultLogPath())}, nil
}

func (a *app) log(e audit.Event) { a.logger.Append(e) }
