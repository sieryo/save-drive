package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"save-drive/internal/config"
	"save-drive/internal/gdrive"
)

type authorizeOptions struct {
	creds string
}

func newAuthorizeCmd() *cobra.Command {
	opts := authorizeOptions{}
	cmd := &cobra.Command{
		Use:   "authorize",
		Short: "Run OAuth authorization and cache token locally",
		Long: "Starts the Google OAuth flow so the local token is created before running push/list/fetch/delete.\n" +
			"Always runs a fresh flow and overwrites the cached token, so it also fixes a revoked or expired token.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthorize(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	return cmd
}

func runAuthorize(ctx context.Context, opts authorizeOptions) error {
	client, err := gdrive.Authorize(ctx, opts.creds)
	if err != nil {
		return err
	}
	user, err := client.WhoAmI(ctx)
	if err != nil {
		return fmt.Errorf("verify drive access: %w", err)
	}

	fmt.Printf("authorization ready; token file: %s\n", config.DefaultTokenFile())
	fmt.Printf("account: %s\n", user.EmailAddress)
	return nil
}
