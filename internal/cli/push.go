package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"save-drive/internal/audit"
	"save-drive/internal/config"
	"save-drive/internal/gdrive"
)

type pushOptions struct {
	files   []string
	to      string
	mkdir   bool
	replace bool
	dryRun  bool
	json    bool
	creds   string
	rootID  string
}

func newPushCmd() *cobra.Command {
	opts := pushOptions{}
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Upload files to Drive folder",
		Example: "  save-drive push --file .\\build\\release.zip --to Releases/Prod --mkdir --replace\n" +
			"  save-drive push --file .\\dist\\*.zip --file .\\dist\\SHA256SUMS --to Releases/1.2.0 --mkdir\n" +
			"  save-drive push --file .\\README.md --to Test --dry-run",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPush(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringArrayVar(&opts.files, "file", nil, "Local file path or glob pattern (repeatable)")
	cmd.Flags().StringVar(&opts.to, "to", "", "Target folder path in Drive")
	cmd.Flags().BoolVar(&opts.mkdir, "mkdir", false, "Create folder path if it does not exist")
	cmd.Flags().BoolVar(&opts.replace, "replace", false, "Move existing same-name file to trash after the new upload succeeds")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show what would be uploaded without changing Drive")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print uploaded files as JSON instead of status lines")
	cmd.Flags().StringVar(&opts.creds, "creds", config.DefaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", config.DefaultRootID(), "Root folder ID")
	_ = cmd.MarkFlagRequired("file")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func runPush(ctx context.Context, opts pushOptions) (err error) {
	// Validate local files before touching Drive: a typo in --file must never
	// get as far as replacing the remote copy.
	paths, err := expandFiles(opts.files)
	if err != nil {
		return err
	}
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
	}

	a, err := newApp(ctx, opts.creds)
	if err != nil {
		return err
	}
	e := audit.Event{Action: "push", FileName: strings.Join(names, ","), FolderPath: opts.to}
	defer a.record(&e, &err)

	folderID, err := a.client.ResolveFolderPath(ctx, opts.rootID, opts.to, opts.mkdir && !opts.dryRun)
	if err != nil {
		if opts.dryRun && opts.mkdir && errors.Is(err, gdrive.ErrNotFound) {
			e.Detail = fmt.Sprintf("dry_run=true files=%d", len(paths))
			fmt.Printf("would create folder: %s\n", opts.to)
			for _, n := range names {
				fmt.Printf("would upload: %s\n", n)
			}
			return nil
		}
		return err
	}

	// Check every name up front so a multi-file push fails before uploading
	// anything, instead of stopping halfway.
	existing := make([]int, len(names))
	for i, n := range names {
		found, err := a.client.FindFiles(ctx, folderID, n)
		if err != nil {
			return err
		}
		if len(found) > 0 && !opts.replace {
			return fmt.Errorf("file %q %w, use --replace", n, gdrive.ErrExists)
		}
		existing[i] = len(found)
	}

	if opts.dryRun {
		e.Detail = fmt.Sprintf("dry_run=true files=%d replace=%t", len(paths), opts.replace)
		for i, n := range names {
			if existing[i] > 0 {
				fmt.Printf("would upload: %s (replace %d existing)\n", n, existing[i])
			} else {
				fmt.Printf("would upload: %s\n", n)
			}
		}
		return nil
	}

	var total int64
	var ids []string
	var results []fileJSON
	for i, p := range paths {
		uo := gdrive.UploadOptions{Replace: opts.replace}
		var pr *progress
		if stderrIsTerminal() {
			pr = &progress{name: names[i]}
			uo.Progress = pr.update
		}
		created, err := a.client.Upload(ctx, folderID, p, uo)
		pr.finish()
		if created != nil {
			ids = append(ids, created.Id)
			e.RemoteID = strings.Join(ids, ",")
		}
		if err != nil {
			return err
		}
		total += created.Size
		if opts.json {
			results = append(results, fileJSON{
				ID: created.Id, Name: created.Name, Type: "file", Size: created.Size,
				ModifiedTime: created.ModifiedTime, MD5: created.Md5Checksum, Link: created.WebViewLink,
			})
		} else {
			fmt.Printf("uploaded: %s (id=%s)\n", created.Name, created.Id)
		}
	}

	e.Detail = fmt.Sprintf("size=%d replace=%t", total, opts.replace)
	if len(paths) > 1 {
		e.Detail += fmt.Sprintf(" files=%d", len(paths))
	}
	if opts.json {
		return printJSON(results)
	}
	return nil
}

// expandFiles resolves --file values into regular file paths. Values with
// glob characters are expanded (PowerShell does not expand them for native
// commands); plain paths must exist. Duplicate base names are rejected
// because they would land on the same Drive name.
func expandFiles(patterns []string) ([]string, error) {
	var out []string
	seen := map[string]string{}
	add := func(p string) error {
		key := strings.ToLower(filepath.Base(p))
		if prev, ok := seen[key]; ok {
			return fmt.Errorf("--file %s and %s have the same file name", prev, p)
		}
		seen[key] = p
		out = append(out, p)
		return nil
	}

	for _, pat := range patterns {
		if !strings.ContainsAny(pat, "*?[") {
			fi, err := os.Stat(pat)
			if err != nil {
				return nil, fmt.Errorf("local file: %w", err)
			}
			if !fi.Mode().IsRegular() {
				return nil, fmt.Errorf("local file %s is not a regular file", pat)
			}
			if err := add(pat); err != nil {
				return nil, err
			}
			continue
		}

		matches, err := filepath.Glob(pat)
		if err != nil {
			return nil, fmt.Errorf("--file %q: %w", pat, err)
		}
		n := 0
		for _, m := range matches {
			if fi, err := os.Stat(m); err != nil || !fi.Mode().IsRegular() {
				continue
			}
			if err := add(m); err != nil {
				return nil, err
			}
			n++
		}
		if n == 0 {
			return nil, fmt.Errorf("--file %q matches no files", pat)
		}
	}
	return out, nil
}

// progress renders a single, self-overwriting upload progress line on stderr.
type progress struct {
	name  string
	shown bool
}

func (p *progress) update(sent, total int64) {
	p.shown = true
	pct := int64(100)
	if total > 0 {
		pct = 100 * sent / total
	}
	fmt.Fprintf(os.Stderr, "\rupload %s: %3d%% (%.1f/%.1f MB)", p.name, pct, mb(sent), mb(total))
}

func (p *progress) finish() {
	if p != nil && p.shown {
		fmt.Fprintln(os.Stderr)
	}
}

func mb(n int64) float64 { return float64(n) / (1 << 20) }
