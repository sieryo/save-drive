package gdrive

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

// ErrExists is returned (wrapped) when an upload would collide with an
// existing file and replacing was not requested.
var ErrExists = errors.New("already exists")

const (
	// fileFields is the metadata requested for every file handed to callers.
	fileFields = "id,name,mimeType,size,md5Checksum,modifiedTime,createdTime,webViewLink"
	listFields = "nextPageToken,files(" + fileFields + ")"

	// uploadChunkSize is the resumable upload chunk size. Files smaller than
	// this go up in a single multipart request.
	uploadChunkSize = 8 << 20

	// anyoneWithLinkPermissionID is the fixed Drive permission ID for
	// "anyone with the link".
	anyoneWithLinkPermissionID = "anyoneWithLink"

	workspaceMimePrefix = "application/vnd.google-apps."
)

// exportFormats maps Google Workspace file types, which have no binary
// content, to the Office format fetch exports them as.
var exportFormats = map[string]struct{ mime, ext string }{
	"application/vnd.google-apps.document":     {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
	"application/vnd.google-apps.spreadsheet":  {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
	"application/vnd.google-apps.presentation": {"application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"},
	"application/vnd.google-apps.drawing":      {"image/png", ".png"},
}

// UploadOptions tunes Upload.
type UploadOptions struct {
	// Replace moves existing files with the same name to trash, after the new
	// file has been uploaded and verified.
	Replace bool
	// Progress, when set, is called as bytes are sent. It only fires for
	// files large enough to use resumable upload.
	Progress func(sent, total int64)
}

// Upload uploads localPath into folderID under its base name and verifies the
// MD5 checksum Drive reports. The upload happens before any existing file is
// touched, so a failed upload never loses the previous copy.
func (c *Client) Upload(ctx context.Context, folderID, localPath string, opts UploadOptions) (*drive.File, error) {
	fi, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("local file: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("local file %s is not a regular file", localPath)
	}
	name := filepath.Base(localPath)

	existing, err := c.FindFiles(ctx, folderID, name)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 && !opts.Replace {
		return nil, fmt.Errorf("file %q %w, use --replace", name, ErrExists)
	}

	sum, err := fileMD5(localPath)
	if err != nil {
		return nil, err
	}
	fd, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("local file: %w", err)
	}
	defer fd.Close()

	call := c.svc.Files.Create(&drive.File{Name: name, Parents: []string{folderID}}).
		Media(fd, googleapi.ChunkSize(uploadChunkSize)).
		SupportsAllDrives(true).
		Fields(fileFields).
		Context(ctx)
	if opts.Progress != nil {
		size := fi.Size()
		call = call.ProgressUpdater(func(sent, _ int64) { opts.Progress(sent, size) })
	}
	created, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("upload %s: %w", name, err)
	}

	if created.Md5Checksum != "" && created.Md5Checksum != sum {
		trashErr := c.Trash(ctx, created.Id)
		err := fmt.Errorf("upload %s: checksum mismatch (local %s, drive %s); the uploaded copy was moved to trash", name, sum, created.Md5Checksum)
		return nil, errors.Join(err, trashErr)
	}

	if opts.Replace && len(existing) > 0 {
		if err := c.Trash(ctx, ids(existing)...); err != nil {
			return created, fmt.Errorf("uploaded %s (id=%s) but could not trash the old copy: %w", name, created.Id, err)
		}
	}
	return created, nil
}

// Download saves the content of fileID to disk and returns the file metadata
// and the local path written. dst may be empty (use the Drive name in the
// current directory), an existing directory, or a file path. Google Workspace
// files are exported (see exportFormats). Data is written to a ".part" file
// first and renamed on success, so a failed download never leaves a truncated
// file under the final name.
func (c *Client) Download(ctx context.Context, fileID, dst string) (*drive.File, string, error) {
	meta, err := c.Get(ctx, fileID)
	if err != nil {
		return nil, "", err
	}
	if meta.MimeType == FolderMimeType {
		return nil, "", fmt.Errorf("%s (id=%s) is a folder, not a file", meta.Name, meta.Id)
	}

	name := localName(meta.Name, meta.Id)
	var body io.ReadCloser
	wantMD5 := meta.Md5Checksum
	if strings.HasPrefix(meta.MimeType, workspaceMimePrefix) {
		f, ok := exportFormats[meta.MimeType]
		if !ok {
			return nil, "", fmt.Errorf("%s has type %s, which cannot be downloaded or exported", meta.Name, meta.MimeType)
		}
		if !strings.EqualFold(filepath.Ext(name), f.ext) {
			name += f.ext
		}
		resp, err := c.svc.Files.Export(fileID, f.mime).Context(ctx).Download()
		if err != nil {
			return nil, "", fmt.Errorf("export %s: %w", meta.Name, err)
		}
		body = resp.Body
		wantMD5 = ""
	} else {
		resp, err := c.svc.Files.Get(fileID).SupportsAllDrives(true).Context(ctx).Download()
		if err != nil {
			return nil, "", fmt.Errorf("download %s: %w", meta.Name, err)
		}
		body = resp.Body
	}
	defer body.Close()

	target := downloadTarget(dst, name)
	if err := writeVerified(target, body, wantMD5); err != nil {
		return nil, "", err
	}
	return meta, target, nil
}

// downloadTarget resolves where Download writes: dst itself, or name inside
// dst when dst is empty, an existing directory, or ends with a separator.
func downloadTarget(dst, name string) string {
	if dst == "" {
		return name
	}
	if strings.HasSuffix(dst, "/") || strings.HasSuffix(dst, string(filepath.Separator)) {
		return filepath.Join(dst, name)
	}
	if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
		return filepath.Join(dst, name)
	}
	return dst
}

func writeVerified(target string, r io.Reader, wantMD5 string) error {
	if dir := filepath.Dir(target); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	part := target + ".part"
	out, err := os.Create(part)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	h := md5.New()
	_, copyErr := io.Copy(io.MultiWriter(out, h), r)
	closeErr := out.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		os.Remove(part)
		return fmt.Errorf("write %s: %w", target, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); wantMD5 != "" && got != wantMD5 {
		os.Remove(part)
		return fmt.Errorf("download %s: checksum mismatch (drive %s, local %s)", target, wantMD5, got)
	}
	if err := os.Rename(part, target); err != nil {
		os.Remove(part)
		return fmt.Errorf("move %s into place: %w", target, err)
	}
	return nil
}

// Get returns the metadata of fileID.
func (c *Client) Get(ctx context.Context, fileID string) (*drive.File, error) {
	f, err := c.svc.Files.Get(fileID).SupportsAllDrives(true).Fields(fileFields).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get file %s: %w", fileID, err)
	}
	return f, nil
}

// Trash moves the given files or folders to the Drive trash, where they can
// be restored for 30 days. It stops at the first failure.
func (c *Client) Trash(ctx context.Context, ids ...string) error {
	for i, id := range ids {
		_, err := c.svc.Files.Update(id, &drive.File{Trashed: true}).
			SupportsAllDrives(true).
			Fields("id").
			Context(ctx).
			Do()
		if err != nil {
			return fmt.Errorf("trash %s (%d of %d done): %w", id, i, len(ids), err)
		}
	}
	return nil
}

// Delete permanently deletes the given files, bypassing the trash. Deleting a
// folder also deletes everything in it. It stops at the first failure.
func (c *Client) Delete(ctx context.Context, ids ...string) error {
	for i, id := range ids {
		if err := c.svc.Files.Delete(id).SupportsAllDrives(true).Context(ctx).Do(); err != nil {
			return fmt.Errorf("delete %s (%d of %d done): %w", id, i, len(ids), err)
		}
	}
	return nil
}

// List returns every item (files and folders) directly under folderID,
// folders first, then by name. It follows pagination to the end.
func (c *Client) List(ctx context.Context, folderID string) ([]*drive.File, error) {
	q := fmt.Sprintf("'%s' in parents and trashed = false", escapeQuery(folderID))
	files, err := c.list(ctx, q, "folder,name")
	if err != nil {
		return nil, fmt.Errorf("list folder %s: %w", folderID, err)
	}
	return files, nil
}

// Entry is an item found by ListTree, with its path relative to the listed
// folder ("sub/dir/file.zip").
type Entry struct {
	Path string
	File *drive.File
}

// ListTree returns every item under folderID, recursing into subfolders. A
// folder is listed before its contents.
func (c *Client) ListTree(ctx context.Context, folderID string) ([]Entry, error) {
	var out []Entry
	var walk func(id, prefix string) error
	walk = func(id, prefix string) error {
		files, err := c.List(ctx, id)
		if err != nil {
			return err
		}
		for _, f := range files {
			p := prefix + f.Name
			out = append(out, Entry{Path: p, File: f})
			if f.MimeType == FolderMimeType {
				if err := walk(f.Id, p+"/"); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(folderID, ""); err != nil {
		return nil, err
	}
	return out, nil
}

// Share makes fileID readable by anyone with the link and returns its
// metadata, including WebViewLink.
func (c *Client) Share(ctx context.Context, fileID string) (*drive.File, error) {
	_, err := c.svc.Permissions.Create(fileID, &drive.Permission{Type: "anyone", Role: "reader"}).
		SupportsAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("share %s: %w", fileID, err)
	}
	return c.Get(ctx, fileID)
}

// Unshare removes the "anyone with the link" permission from fileID. Other
// permissions are left alone.
func (c *Client) Unshare(ctx context.Context, fileID string) error {
	err := c.svc.Permissions.Delete(fileID, anyoneWithLinkPermissionID).
		SupportsAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("unshare %s: %w", fileID, err)
	}
	return nil
}

// list runs a Drive query and follows pagination to the end.
func (c *Client) list(ctx context.Context, q, orderBy string) ([]*drive.File, error) {
	var out []*drive.File
	call := c.svc.Files.List().
		Q(q).
		OrderBy(orderBy).
		PageSize(1000).
		Fields(listFields).
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true)
	err := call.Pages(ctx, func(res *drive.FileList) error {
		out = append(out, res.Files...)
		return nil
	})
	return out, err
}

func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("local file: %w", err)
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// localName turns a Drive file name into a safe local file name. Drive names
// may contain characters Windows rejects, or path separators that would write
// outside the target directory.
func localName(name, fallback string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, " .")
	if name == "" {
		return fallback
	}
	return name
}

func ids(files []*drive.File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Id
	}
	return out
}
