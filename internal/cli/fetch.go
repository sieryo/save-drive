package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"save-drive/internal/audit"
	"save-drive/internal/config"
)

type fetchOptions struct {
	id     string
	name   string
	from   string
	to     string
	creds  string
	rootID string
}

func newFetchCmd() *cobra.Command {
	opts := fetchOptions{}
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Download file from Drive",
		Example: "  save-drive fetch --id <FILE_ID> --to .\\downloads\\release.zip\n" +
			"  save-drive fetch --name release.zip --from Releases/Prod --to .\\release.zip",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runFetch(opts)
		},
	}
	cmd.Flags().StringVar(&opts.id, "id", "", "Remote file ID")
	cmd.Flags().StringVar(&opts.name, "name", "", "Remote file name (requires --from)")
	cmd.Flags().StringVar(&opts.from, "from", "", "Folder path in Drive")
	cmd.Flags().StringVar(&opts.to, "to", "", "Local output file path")
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", config.DefaultRootID(), "Root folder ID")
	return cmd
}

func runFetch(opts fetchOptions) error {
	if opts.id == "" && opts.name == "" {
		return errors.New("fetch requires --id or --name")
	}

	ctx := context.Background()
	a, err := newApp(opts.creds)
	if err != nil {
		return err
	}
	svc := a.client.Service()

	fileID := opts.id
	fileName := opts.name
	if fileID == "" {
		if opts.from == "" {
			return errors.New("fetch --name requires --from")
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
		fileID = files[0].Id
		fileName = files[0].Name
	}

	to := opts.to
	if to == "" {
		if fileName == "" {
			fileName = fileID
		}
		to = fileName
	}

	resp, err := svc.Files.Get(fileID).
		SupportsAllDrives(true).
		Context(ctx).
		Download()
	if err != nil {
		a.log(audit.Event{Action: "fetch", Result: "failed", RemoteID: fileID, Detail: err.Error()})
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.Create(to)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}

	a.log(audit.Event{Action: "fetch", Result: "success", RemoteID: fileID, FileName: fileName, Detail: fmt.Sprintf("saved_to=%s", to)})
	fmt.Printf("downloaded: %s\n", to)
	return nil
}
