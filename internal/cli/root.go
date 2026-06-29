// Package cli wires up the save-drive cobra command tree.
package cli

import (
	"github.com/spf13/cobra"

	"save-drive/internal/buildinfo"
)

// Execute builds the root command and runs it.
func Execute() error {
	return newRootCmd().Execute()
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
		newPushCmd(),
		newListCmd(),
		newFetchCmd(),
		newDeleteCmd(),
		newLogsCmd(),
		newVersionCmd(),
	)
	return cmd
}
