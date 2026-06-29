package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"save-drive/internal/buildinfo"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Printf("save-drive %s\n", buildinfo.Version)
			fmt.Printf("  commit: %s\n", buildinfo.Commit)
			fmt.Printf("  built:  %s\n", buildinfo.Date)
			fmt.Printf("  go:     %s\n", runtime.Version())
			return nil
		},
	}
}
