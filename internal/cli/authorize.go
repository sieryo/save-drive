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
		Long:  "Starts the Google OAuth flow so the local token is created before running push/list/fetch/delete.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runAuthorize(opts)
		},
	}
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	return cmd
}

func runAuthorize(opts authorizeOptions) error {
	ctx := context.Background()
	client, err := gdrive.New(ctx, opts.creds)
	if err != nil {
		return err
	}

	if _, err := client.Service().About.Get().Fields("user(displayName,emailAddress)").Context(ctx).Do(); err != nil {
		return fmt.Errorf("verify drive access: %w", err)
	}

	fmt.Printf("authorization ready; token file: %s\n", config.DefaultTokenFile())
	return nil
}
