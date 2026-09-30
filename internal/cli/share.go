package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"save-drive/internal/audit"
	"save-drive/internal/config"
)

type shareOptions struct {
	id     string
	name   string
	from   string
	latest bool
	revoke bool
	json   bool
	creds  string
	rootID string
}

func newShareCmd() *cobra.Command {
	opts := shareOptions{}
	cmd := &cobra.Command{
		Use:   "share",
		Short: "Make file readable by anyone with the link, and print the link",
		Example: "  save-drive share --name release.zip --from Releases/Prod\n" +
			"  save-drive share --id <FILE_ID> --revoke",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runShare(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.id, "id", "", "Remote file ID")
	cmd.Flags().StringVar(&opts.name, "name", "", "Remote file name (requires --from)")
	cmd.Flags().StringVar(&opts.from, "from", "", "Folder path in Drive")
	cmd.Flags().BoolVar(&opts.latest, "latest", false, "With --name, pick the most recently modified file when several share the name")
	cmd.Flags().BoolVar(&opts.revoke, "revoke", false, "Remove link sharing instead of adding it")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print the shared file as JSON instead of a status line")
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", config.DefaultRootID(), "Root folder ID")
	return cmd
}

func runShare(ctx context.Context, opts shareOptions) (err error) {
	if opts.id == "" && opts.name == "" {
		return errors.New("share requires --id or --name")
	}
	if opts.id == "" && opts.from == "" {
		return errors.New("share --name requires --from")
	}

	a, err := newApp(ctx, opts.creds)
	if err != nil {
		return err
	}
	e := audit.Event{Action: "share", FileName: opts.name, FolderPath: opts.from, RemoteID: opts.id}
	defer a.record(&e, &err)

	fileID := opts.id
	if fileID == "" {
		fileID, err = findOne(ctx, a.client, opts.rootID, opts.from, opts.name, opts.latest)
		if err != nil {
			return err
		}
		e.RemoteID = fileID
	}

	if opts.revoke {
		e.Detail = "revoke=true"
		if err := a.client.Unshare(ctx, fileID); err != nil {
			return err
		}
		fmt.Printf("unshared: %s\n", fileID)
		return nil
	}

	f, err := a.client.Share(ctx, fileID)
	if err != nil {
		return err
	}
	e.FileName = f.Name
	e.Detail = "link=" + f.WebViewLink
	if opts.json {
		return printJSON(fileJSON{ID: f.Id, Name: f.Name, Type: typeName(f.MimeType), Size: f.Size, ModifiedTime: f.ModifiedTime, Link: f.WebViewLink})
	}
	fmt.Printf("shared: %s\n", f.WebViewLink)
	return nil
}
