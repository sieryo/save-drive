package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"google.golang.org/api/drive/v3"

	"save-drive/internal/audit"
	"save-drive/internal/config"
)

type pushOptions struct {
	filePath string
	to       string
	mkdir    bool
	replace  bool
	creds    string
	rootID   string
}

func newPushCmd() *cobra.Command {
	opts := pushOptions{}
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Upload file to Drive folder",
		Example: "  save-drive push --file .\\build\\release.zip --to Releases/Prod --mkdir --replace\n" +
			"  save-drive push --file .\\README.md --to Test --mkdir",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runPush(opts)
		},
	}
	cmd.Flags().StringVar(&opts.filePath, "file", "", "Local file path")
	cmd.Flags().StringVar(&opts.to, "to", "", "Target folder path in Drive")
	cmd.Flags().BoolVar(&opts.mkdir, "mkdir", false, "Create folder path if it does not exist")
	cmd.Flags().BoolVar(&opts.replace, "replace", false, "Delete existing same-name file before upload")
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", config.DefaultRootID(), "Root folder ID")
	_ = cmd.MarkFlagRequired("file")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func runPush(opts pushOptions) error {
	ctx := context.Background()
	a, err := newApp(opts.creds)
	if err != nil {
		return err
	}
	svc := a.client.Service()

	fileName := filepath.Base(opts.filePath)
	folderID, err := a.client.ResolveFolderPath(ctx, opts.rootID, opts.to, opts.mkdir)
	if err != nil {
		a.log(audit.Event{Action: "push", Result: "failed", FileName: fileName, FolderPath: opts.to, Detail: err.Error()})
		return err
	}

	existing, err := a.client.ListFilesByName(ctx, folderID, fileName)
	if err != nil {
		return err
	}
	if len(existing) > 0 && !opts.replace {
		return fmt.Errorf("file %q already exists, use --replace", fileName)
	}

	if opts.replace {
		for _, f := range existing {
			if err := svc.Files.Delete(f.Id).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
				return fmt.Errorf("delete old file %s: %w", f.Id, err)
			}
		}
	}

	fd, err := os.Open(opts.filePath)
	if err != nil {
		return err
	}
	defer fd.Close()

	meta := &drive.File{Name: fileName, Parents: []string{folderID}}
	created, err := svc.Files.Create(meta).
		Media(fd).
		SupportsAllDrives(true).
		Fields("id,name,size,md5Checksum").
		Context(ctx).
		Do()
	if err != nil {
		a.log(audit.Event{Action: "push", Result: "failed", FileName: fileName, FolderPath: opts.to, Detail: err.Error()})
		return err
	}

	a.log(audit.Event{Action: "push", Result: "success", FileName: fileName, FolderPath: opts.to, RemoteID: created.Id, Detail: fmt.Sprintf("size=%d replace=%t", created.Size, opts.replace)})
	fmt.Printf("uploaded: %s (id=%s)\n", created.Name, created.Id)
	return nil
}
