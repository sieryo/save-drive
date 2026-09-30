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

type fetchOptions struct {
	id     string
	name   string
	from   string
	to     string
	latest bool
	json   bool
	creds  string
	rootID string
}

func newFetchCmd() *cobra.Command {
	opts := fetchOptions{}
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Download file from Drive",
		Example: "  save-drive fetch --id <FILE_ID> --to .\\downloads\\\n" +
			"  save-drive fetch --name release.zip --from Releases/Prod --to .\\release.zip\n" +
			"  save-drive fetch --name release.zip --from Releases/Prod --latest",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFetch(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.id, "id", "", "Remote file ID")
	cmd.Flags().StringVar(&opts.name, "name", "", "Remote file name (requires --from)")
	cmd.Flags().StringVar(&opts.from, "from", "", "Folder path in Drive")
	cmd.Flags().StringVar(&opts.to, "to", "", "Local output file or directory (default: Drive name in current directory)")
	cmd.Flags().BoolVar(&opts.latest, "latest", false, "With --name, pick the most recently modified file when several share the name")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print the downloaded file as JSON instead of a status line")
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", config.DefaultRootID(), "Root folder ID")
	return cmd
}

func runFetch(ctx context.Context, opts fetchOptions) (err error) {
	if opts.id == "" && opts.name == "" {
		return errors.New("fetch requires --id or --name")
	}
	if opts.id == "" && opts.from == "" {
		return errors.New("fetch --name requires --from")
	}

	a, err := newApp(ctx, opts.creds)
	if err != nil {
		return err
	}
	e := audit.Event{Action: "fetch", FileName: opts.name, FolderPath: opts.from, RemoteID: opts.id}
	defer a.record(&e, &err)

	fileID := opts.id
	if fileID == "" {
		fileID, err = findOne(ctx, a.client, opts.rootID, opts.from, opts.name, opts.latest)
		if err != nil {
			return err
		}
		e.RemoteID = fileID
	}

	meta, path, err := a.client.Download(ctx, fileID, opts.to)
	if err != nil {
		return err
	}
	e.FileName = meta.Name
	e.Detail = fmt.Sprintf("saved_to=%s", path)

	if opts.json {
		return printJSON(fileJSON{
			ID: meta.Id, Name: meta.Name, Type: "file", Size: meta.Size,
			ModifiedTime: meta.ModifiedTime, MD5: meta.Md5Checksum, LocalPath: path,
		})
	}
	fmt.Printf("downloaded: %s\n", path)
	return nil
}

// findOne resolves --name/--from to a single file ID. Several files with the
// same name are an error unless latest is set, so a script never silently
// gets an arbitrary one.
func findOne(ctx context.Context, c *gdrive.Client, rootID, from, name string, latest bool) (string, error) {
	folderID, err := c.ResolveFolderPath(ctx, rootID, from, false)
	if err != nil {
		return "", err
	}
	files, err := c.FindFiles(ctx, folderID, name)
	if err != nil {
		return "", err
	}
	switch {
	case len(files) == 0:
		return "", fmt.Errorf("file %q in %q: %w", name, from, gdrive.ErrNotFound)
	case len(files) > 1 && !latest:
		return "", fmt.Errorf("%d files named %q in %q; use --latest or --id", len(files), name, from)
	}
	// FindFiles returns the most recently modified first.
	return files[0].Id, nil
}
