package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
func newApp(ctx context.Context, credsPath string) (*app, error) {
	client, err := gdrive.New(ctx, credsPath)
	if err != nil {
		return nil, err
	}
	return &app{client: client, logger: audit.NewLogger(config.DefaultLogPath())}, nil
}

// record finalizes e from the command's result and appends it to the audit
// log. Call it once per run via defer, with a pointer to the named error.
func (a *app) record(e *audit.Event, err *error) {
	e.Result = "success"
	if *err != nil {
		e.Result = "failed"
		e.Detail = (*err).Error()
	}
	if werr := a.logger.Append(*e); werr != nil {
		fmt.Fprintf(os.Stderr, "warning: write audit log %s: %v\n", a.logger.Path, werr)
	}
}

// fileJSON is the --json representation of a Drive item.
type fileJSON struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Size         int64  `json:"size"`
	ModifiedTime string `json:"modified_time,omitempty"`
	MD5          string `json:"md5,omitempty"`
	Link         string `json:"link,omitempty"`
	Path         string `json:"path,omitempty"`
	LocalPath    string `json:"local_path,omitempty"`
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func typeName(mimeType string) string {
	if mimeType == gdrive.FolderMimeType {
		return "folder"
	}
	return "file"
}

// tsvField keeps a value on one TSV cell: Drive names may contain tabs or
// newlines, which would otherwise break the columns scripts rely on.
func tsvField(s string) string {
	return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(s)
}

// stderrIsTerminal reports whether stderr is an interactive console, so
// progress output is only shown to people and not captured by scripts.
func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
