package cli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"save-drive/internal/audit"
	"save-drive/internal/config"
	"save-drive/internal/gdrive"
)

type listOptions struct {
	from      string
	recursive bool
	json      bool
	creds     string
	rootID    string
}

func newListCmd() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List folder contents",
		Long: "Prints one TSV line per item: id, type, name, modified time, size, path.\n" +
			"size is - for folders; path is relative to --from.",
		Example: "  save-drive list --from Releases\n" +
			"  save-drive list --from Releases --recursive --json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.from, "from", "", "Folder path in Drive (use / for the root folder)")
	cmd.Flags().BoolVarP(&opts.recursive, "recursive", "r", false, "Include the contents of subfolders")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print JSON instead of TSV")
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", config.DefaultRootID(), "Root folder ID")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

func runList(ctx context.Context, opts listOptions) (err error) {
	a, err := newApp(ctx, opts.creds)
	if err != nil {
		return err
	}
	e := audit.Event{Action: "list", FolderPath: opts.from}
	defer a.record(&e, &err)

	folderID, err := a.client.ResolveFolderPath(ctx, opts.rootID, opts.from, false)
	if err != nil {
		return err
	}

	var entries []gdrive.Entry
	if opts.recursive {
		entries, err = a.client.ListTree(ctx, folderID)
	} else {
		files, lerr := a.client.List(ctx, folderID)
		err = lerr
		for _, f := range files {
			entries = append(entries, gdrive.Entry{Path: f.Name, File: f})
		}
	}
	if err != nil {
		return err
	}
	e.Detail = fmt.Sprintf("count=%d recursive=%t", len(entries), opts.recursive)

	if opts.json {
		out := make([]fileJSON, 0, len(entries))
		for _, en := range entries {
			f := en.File
			out = append(out, fileJSON{
				ID: f.Id, Name: f.Name, Type: typeName(f.MimeType), Size: f.Size,
				ModifiedTime: f.ModifiedTime, MD5: f.Md5Checksum, Link: f.WebViewLink, Path: en.Path,
			})
		}
		return printJSON(out)
	}

	for _, en := range entries {
		f := en.File
		size := "-"
		if f.MimeType != gdrive.FolderMimeType {
			size = strconv.FormatInt(f.Size, 10)
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", f.Id, typeName(f.MimeType), tsvField(f.Name), f.ModifiedTime, size, tsvField(en.Path))
	}
	return nil
}
