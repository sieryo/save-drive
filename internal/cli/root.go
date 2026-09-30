// Package cli wires up the save-drive cobra command tree.
package cli

import (
	"context"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"save-drive/internal/buildinfo"
)

// Execute builds the root command and runs it. Ctrl+C cancels the context
// passed to every command, which aborts in-flight Drive requests.
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return newRootCmd().ExecuteContext(ctx)
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "save-drive",
		Short:         "Simple Google Drive CLI for release artifact management",
		Version:       buildinfo.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Manage files in Google Drive from scripts.

Env vars:
  GOOGLE_OAUTH_CLIENT_CREDENTIALS  path to OAuth client credentials JSON (recommended)
  GOOGLE_OAUTH_TOKEN_FILE          token file path (default: .save-drive/oauth-token.json)
  GOOGLE_APPLICATION_CREDENTIALS   service account credentials JSON (optional)
  SAVE_DRIVE_LOG_FILE              override audit log file location
  DRIVE_ROOT_ID                    root folder ID (default: root)`,
	}

	cmd.AddCommand(
		newAuthorizeCmd(),
		newWhoamiCmd(),
		newLogoutCmd(),
		newPushCmd(),
		newListCmd(),
		newFetchCmd(),
		newDeleteCmd(),
		newPruneCmd(),
		newShareCmd(),
		newLogsCmd(),
		newVersionCmd(),
	)
	return cmd
}
