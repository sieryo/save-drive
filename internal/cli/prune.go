package cli

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"

	"github.com/spf13/cobra"

	"save-drive/internal/audit"
	"save-drive/internal/config"
	"save-drive/internal/gdrive"
)

type pruneOptions struct {
	from      string
	keep      int
	match     string
	permanent bool
	dryRun    bool
	creds     string
	rootID    string
}

func newPruneCmd() *cobra.Command {
	opts := pruneOptions{}
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Keep only the newest N files in a folder",
		Long: "Moves all but the --keep most recently modified files in --from to trash.\n" +
			"Only files directly in the folder count; subfolders are never touched.",
		Example: "  save-drive prune --from Releases/Prod --keep 5 --dry-run\n" +
			"  save-drive prune --from Releases/Prod --keep 3 --match \"*.zip\"",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPrune(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.from, "from", "", "Folder path in Drive")
	cmd.Flags().IntVar(&opts.keep, "keep", 0, "Number of newest files to keep (at least 1)")
	cmd.Flags().StringVar(&opts.match, "match", "*", "Only consider file names matching this glob pattern")
	cmd.Flags().BoolVar(&opts.permanent, "permanent", false, "Delete permanently instead of moving to trash")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show what would be removed without changing Drive")
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", config.DefaultRootID(), "Root folder ID")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("keep")
	return cmd
}

func runPrune(ctx context.Context, opts pruneOptions) (err error) {
	if opts.keep < 1 {
		return errors.New("prune --keep must be at least 1")
	}
	if _, err := path.Match(opts.match, ""); err != nil {
		return fmt.Errorf("prune --match %q: %w", opts.match, err)
	}

	a, err := newApp(ctx, opts.creds)
	if err != nil {
		return err
	}
	e := audit.Event{Action: "prune", FolderPath: opts.from}
	defer a.record(&e, &err)

	folderID, err := a.client.ResolveFolderPath(ctx, opts.rootID, opts.from, false)
	if err != nil {
		return err
	}
	items, err := a.client.List(ctx, folderID)
	if err != nil {
		return err
	}

	type candidate struct{ id, name, modified string }
	var files []candidate
	for _, f := range items {
		if f.MimeType == gdrive.FolderMimeType {
			continue
		}
		if ok, _ := path.Match(opts.match, f.Name); ok {
			files = append(files, candidate{f.Id, f.Name, f.ModifiedTime})
		}
	}
	// Drive timestamps are RFC 3339 in UTC, so string order is time order.
	sort.SliceStable(files, func(i, j int) bool { return files[i].modified > files[j].modified })

	var drop []candidate
	if len(files) > opts.keep {
		drop = files[opts.keep:]
	}
	e.Detail = fmt.Sprintf("keep=%d match=%s removed=%d permanent=%t dry_run=%t", opts.keep, opts.match, len(drop), opts.permanent, opts.dryRun)

	verb, dryVerb := removeVerbs(opts.permanent)
	for _, f := range drop {
		if opts.dryRun {
			fmt.Printf("%s: %s (id=%s)\n", dryVerb, f.name, f.id)
			continue
		}
		if opts.permanent {
			err = a.client.Delete(ctx, f.id)
		} else {
			err = a.client.Trash(ctx, f.id)
		}
		if err != nil {
			return err
		}
		fmt.Printf("%s: %s (id=%s)\n", verb, f.name, f.id)
	}
	if len(drop) == 0 {
		fmt.Printf("nothing to prune: %d file(s), keeping %d\n", len(files), opts.keep)
	}
	return nil
}
