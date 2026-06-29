package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"save-drive/internal/audit"
	"save-drive/internal/config"
)

type deleteOptions struct {
	id     string
	name   string
	from   string
	creds  string
	rootID string
}

func newDeleteCmd() *cobra.Command {
	opts := deleteOptions{}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete file in Drive",
		Example: "  save-drive delete --id <FILE_ID>\n" +
			"  save-drive delete --name release.zip --from Releases/Prod",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDelete(opts)
		},
	}
	cmd.Flags().StringVar(&opts.id, "id", "", "Remote file ID")
	cmd.Flags().StringVar(&opts.name, "name", "", "Remote file name (requires --from)")
	cmd.Flags().StringVar(&opts.from, "from", "", "Folder path in Drive")
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", config.DefaultRootID(), "Root folder ID")
	return cmd
}

func runDelete(opts deleteOptions) error {
	if opts.id == "" && opts.name == "" {
		return errors.New("delete requires --id or --name")
	}

	ctx := context.Background()
	a, err := newApp(opts.creds)
	if err != nil {
		return err
	}
	svc := a.client.Service()

	if opts.id != "" {
		if err := svc.Files.Delete(opts.id).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
			a.log(audit.Event{Action: "delete", Result: "failed", RemoteID: opts.id, Detail: err.Error()})
			return err
		}
		a.log(audit.Event{Action: "delete", Result: "success", RemoteID: opts.id})
		fmt.Printf("deleted: %s\n", opts.id)
		return nil
	}

	if opts.from == "" {
		return errors.New("delete --name requires --from")
	}
	folderID, err := a.client.ResolveFolderPath(ctx, opts.rootID, opts.from, false)
	if err != nil {
		return err
	}
	files, err := a.client.ListFilesByName(ctx, folderID, opts.name)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("file %q not found", opts.name)
	}

	for _, f := range files {
		if err := svc.Files.Delete(f.Id).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
			return err
		}
	}
	a.log(audit.Event{Action: "delete", Result: "success", FileName: opts.name, FolderPath: opts.from, Detail: fmt.Sprintf("count=%d", len(files))})
	fmt.Printf("deleted %d file(s) named %s\n", len(files), opts.name)
	return nil
}
