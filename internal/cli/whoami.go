package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"save-drive/internal/config"
	"save-drive/internal/gdrive"
)

type whoamiOptions struct {
	creds string
	json  bool
}

func newWhoamiCmd() *cobra.Command {
	opts := whoamiOptions{}
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the Google account the CLI acts as",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWhoami(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print JSON instead of text")
	return cmd
}

func runWhoami(ctx context.Context, opts whoamiOptions) error {
	client, err := gdrive.New(ctx, opts.creds)
	if err != nil {
		return err
	}
	user, err := client.WhoAmI(ctx)
	if err != nil {
		return err
	}
	if opts.json {
		return printJSON(map[string]string{"email": user.EmailAddress, "name": user.DisplayName})
	}
	fmt.Printf("%s\t%s\n", user.EmailAddress, tsvField(user.DisplayName))
	return nil
}
