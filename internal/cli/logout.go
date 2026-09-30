package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"save-drive/internal/config"
)

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the cached OAuth token",
		Long: "Deletes the local OAuth token file. The grant stays listed in your Google account;\n" +
			"revoke it at https://myaccount.google.com/permissions if needed.",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := config.DefaultTokenFile()
			err := os.Remove(path)
			if errors.Is(err, os.ErrNotExist) {
				fmt.Printf("no token file: %s\n", path)
				return nil
			}
			if err != nil {
				return fmt.Errorf("remove token file: %w", err)
			}
			fmt.Printf("removed token: %s\n", path)
			return nil
		},
	}
}
