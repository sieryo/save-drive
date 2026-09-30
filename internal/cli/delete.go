package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"save-drive/internal/audit"
	"save-drive/internal/config"
	"save-drive/internal/gdrive"
)

type deleteOptions struct {
	id        string
	name      string
	from      string
	permanent bool
	dryRun    bool
	creds     string
	rootID    string
}

func newDeleteCmd() *cobra.Command {
	opts := deleteOptions{}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Move file in Drive to trash (or delete permanently)",
		Long: "Moves the file to the Drive trash, where it can be restored for 30 days.\n" +
			"--name matches files only, never folders, and removes every file with that name.",
		Example: "  save-drive delete --id <FILE_ID>\n" +
			"  save-drive delete --name release.zip --from Releases/Prod\n" +
			"  save-drive delete --name release.zip --from Releases/Prod --permanent",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDelete(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.id, "id", "", "Remote file ID")
	cmd.Flags().StringVar(&opts.name, "name", "", "Remote file name (requires --from)")
	cmd.Flags().StringVar(&opts.from, "from", "", "Folder path in Drive")
	cmd.Flags().BoolVar(&opts.permanent, "permanent", false, "Delete permanently instead of moving to trash")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show what would be deleted without changing Drive")
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", config.DefaultRootID(), "Root folder ID")
	return cmd
}

func runDelete(ctx context.Context, opts deleteOptions) (err error) {
	if opts.id == "" && opts.name == "" {
		return errors.New("delete requires --id or --name")
	}
	if opts.id == "" && opts.from == "" {
		return errors.New("delete --name requires --from")
	}

	a, err := newApp(ctx, opts.creds)
	if err != nil {
		return err
	}
	e := audit.Event{Action: "delete", FileName: opts.name, FolderPath: opts.from, RemoteID: opts.id}
	defer a.record(&e, &err)

	verb, dryVerb := removeVerbs(opts.permanent)
	remove := func(ids ...string) error {
		if opts.permanent {
			return a.client.Delete(ctx, ids...)
		}
		return a.client.Trash(ctx, ids...)
	}

	if opts.id != "" {
		f, err := a.client.Get(ctx, opts.id)
		if err != nil {
			return err
		}
		e.FileName = f.Name
		if f.MimeType == gdrive.FolderMimeType && opts.permanent {
			return fmt.Errorf("%s (id=%s) is a folder; refusing to delete it and its contents permanently, drop --permanent to move it to trash", f.Name, f.Id)
		}
		e.Detail = fmt.Sprintf("permanent=%t dry_run=%t", opts.permanent, opts.dryRun)
		if opts.dryRun {
			fmt.Printf("%s: %s (id=%s)\n", dryVerb, f.Name, f.Id)
			return nil
		}
		if err := remove(opts.id); err != nil {
			return err
		}
		fmt.Printf("%s: %s\n", verb, opts.id)
		return nil
	}

	folderID, err := a.client.ResolveFolderPath(ctx, opts.rootID, opts.from, false)
	if err != nil {
		return err
	}
	files, err := a.client.FindFiles(ctx, folderID, opts.name)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("file %q in %q: %w", opts.name, opts.from, gdrive.ErrNotFound)
	}
	e.Detail = fmt.Sprintf("count=%d permanent=%t dry_run=%t", len(files), opts.permanent, opts.dryRun)

	if opts.dryRun {
		for _, f := range files {
			fmt.Printf("%s: %s (id=%s)\n", dryVerb, f.Name, f.Id)
		}
		return nil
	}
	ids := make([]string, len(files))
	for i, f := range files {
		ids[i] = f.Id
	}
	if err := remove(ids...); err != nil {
		return err
	}
	fmt.Printf("%s %d file(s) named %s\n", verb, len(files), opts.name)
	return nil
}

// removeVerbs returns the status words for a removal: past tense for the
// result line and the dry-run prefix.
func removeVerbs(permanent bool) (done, dry string) {
	if permanent {
		return "deleted", "would delete"
	}
	return "trashed", "would trash"
}
