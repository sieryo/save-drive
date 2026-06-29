package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"save-drive/internal/audit"
	"save-drive/internal/config"
)

type logsOptions struct {
	limit int
	file  string
}

func newLogsCmd() *cobra.Command {
	opts := logsOptions{}
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show latest audit log entries",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runLogs(opts)
		},
	}
	cmd.Flags().IntVar(&opts.limit, "limit", 10, "Number of latest log entries")
	cmd.Flags().StringVar(&opts.file, "file", config.DefaultLogPath(), "Audit log file path")
	return cmd
}

func runLogs(opts logsOptions) error {
	if opts.limit <= 0 {
		return errors.New("--limit must be > 0")
	}

	path := opts.file
	if strings.TrimSpace(path) == "" {
		path = config.DefaultLogPath()
	}

	events, err := audit.ReadAll(path)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		fmt.Printf("no logs yet: %s\n", path)
		return nil
	}

	start := 0
	if len(events) > opts.limit {
		start = len(events) - opts.limit
	}
	fmt.Printf("log file: %s\n", path)
	for i := len(events) - 1; i >= start; i-- {
		e := events[i]
		target := e.FileName
		if target == "" {
			target = e.RemoteID
		}
		if target == "" {
			target = "-"
		}
		folder := e.FolderPath
		if folder == "" {
			folder = "-"
		}
		detail := e.Detail
		if detail == "" {
			detail = "-"
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", e.Time, e.Action, e.Result, target, folder, detail)
	}
	return nil
}
