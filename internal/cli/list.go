package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"save-drive/internal/audit"
	"save-drive/internal/config"
	"save-drive/internal/gdrive"
)

type listOptions struct {
	from   string
	creds  string
	rootID string
}

func newListCmd() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List folder contents",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runList(opts)
		},
	}
	cmd.Flags().StringVar(&opts.from, "from", "", "Folder path in Drive")
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", config.DefaultRootID(), "Root folder ID")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

func runList(opts listOptions) error {
	ctx := context.Background()
	a, err := newApp(opts.creds)
	if err != nil {
		return err
	}

	folderID, err := a.client.ResolveFolderPath(ctx, opts.rootID, opts.from, false)
	if err != nil {
		return err
	}
	q := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
	res, err := a.client.Service().Files.List().
		Q(q).
		Fields("files(id,name,mimeType,size,modifiedTime)").
		PageSize(1000).
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	for _, f := range res.Files {
		typeName := "file"
		if f.MimeType == gdrive.FolderMimeType {
			typeName = "folder"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", f.Id, typeName, f.Name, f.ModifiedTime)
	}
	a.log(audit.Event{Action: "list", Result: "success", FolderPath: opts.from, Detail: fmt.Sprintf("count=%d", len(res.Files))})
	return nil
}
