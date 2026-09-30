package gdrive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUploadNew(t *testing.T) {
	fd, c := newFakeDrive(t)
	folder := fd.add("root", "Releases", FolderMimeType, nil)
	p := writeTemp(t, "app.zip", "v1")

	f, err := c.Upload(context.Background(), folder, p, UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "app.zip" || f.Size != 2 {
		t.Fatalf("got %+v", f)
	}
	if got := string(fd.file(f.Id).content); got != "v1" {
		t.Fatalf("content = %q", got)
	}
}

func TestUploadExistingWithoutReplace(t *testing.T) {
	fd, c := newFakeDrive(t)
	folder := fd.add("root", "Releases", FolderMimeType, nil)
	fd.add(folder, "app.zip", "", []byte("old"))
	p := writeTemp(t, "app.zip", "new")

	_, err := c.Upload(context.Background(), folder, p, UploadOptions{})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if ops := fd.opLog(); len(ops) != 0 {
		t.Fatalf("unexpected mutations: %v", ops)
	}
}

func TestUploadReplaceUploadsBeforeTrashing(t *testing.T) {
	fd, c := newFakeDrive(t)
	folder := fd.add("root", "Releases", FolderMimeType, nil)
	old := fd.add(folder, "app.zip", "", []byte("old"))
	p := writeTemp(t, "app.zip", "new")

	f, err := c.Upload(context.Background(), folder, p, UploadOptions{Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"create:app.zip", "trash:" + old}
	if ops := fd.opLog(); !slices.Equal(ops, want) {
		t.Fatalf("ops = %v, want %v", ops, want)
	}
	if !fd.file(old).trashed || fd.file(f.Id).trashed {
		t.Fatal("old copy should be trashed and new copy kept")
	}
}

func TestUploadReplaceMissingLocalFileTouchesNothing(t *testing.T) {
	fd, c := newFakeDrive(t)
	folder := fd.add("root", "Releases", FolderMimeType, nil)
	fd.add(folder, "app.zip", "", []byte("old"))

	_, err := c.Upload(context.Background(), folder, filepath.Join(t.TempDir(), "app.zip"), UploadOptions{Replace: true})
	if err == nil {
		t.Fatal("expected error for missing local file")
	}
	if ops := fd.opLog(); len(ops) != 0 {
		t.Fatalf("unexpected mutations: %v", ops)
	}
}

func TestUploadReplaceIgnoresSameNameFolder(t *testing.T) {
	fd, c := newFakeDrive(t)
	folder := fd.add("root", "Releases", FolderMimeType, nil)
	sub := fd.add(folder, "app.zip", FolderMimeType, nil)
	p := writeTemp(t, "app.zip", "new")

	if _, err := c.Upload(context.Background(), folder, p, UploadOptions{Replace: true}); err != nil {
		t.Fatal(err)
	}
	if fd.file(sub).trashed {
		t.Fatal("a folder with the same name must never be replaced")
	}
}

func TestUploadChecksumMismatchKeepsOldCopy(t *testing.T) {
	fd, c := newFakeDrive(t)
	fd.badMD5 = true
	folder := fd.add("root", "Releases", FolderMimeType, nil)
	old := fd.add(folder, "app.zip", "", []byte("old"))
	p := writeTemp(t, "app.zip", "new")

	_, err := c.Upload(context.Background(), folder, p, UploadOptions{Replace: true})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
	if fd.file(old).trashed {
		t.Fatal("old copy must survive a failed upload")
	}
	ops := fd.opLog()
	if len(ops) != 2 || ops[0] != "create:app.zip" || !strings.HasPrefix(ops[1], "trash:") || ops[1] == "trash:"+old {
		t.Fatalf("ops = %v, want create then trash of the new copy", ops)
	}
}

func TestDownload(t *testing.T) {
	fd, c := newFakeDrive(t)
	id := fd.add("root", "app.zip", "", []byte("payload"))
	dir := t.TempDir()

	_, path, err := c.Download(context.Background(), id, dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "app.zip") {
		t.Fatalf("path = %s", path)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "payload" {
		t.Fatalf("content = %q", b)
	}
	if _, err := os.Stat(path + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file left behind")
	}
}

func TestDownloadToNewNestedPath(t *testing.T) {
	fd, c := newFakeDrive(t)
	id := fd.add("root", "app.zip", "", []byte("payload"))
	dst := filepath.Join(t.TempDir(), "a", "b", "renamed.zip")

	_, path, err := c.Download(context.Background(), id, dst)
	if err != nil {
		t.Fatal(err)
	}
	if path != dst {
		t.Fatalf("path = %s, want %s", path, dst)
	}
}

func TestWriteVerifiedChecksumMismatchKeepsExistingFile(t *testing.T) {
	dst := writeTemp(t, "app.zip", "previous")

	err := writeVerified(dst, strings.NewReader("tampered"), "deadbeefdeadbeefdeadbeefdeadbeef")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "previous" {
		t.Fatalf("existing file overwritten: %q", b)
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file left behind")
	}
}

func TestDownloadExportsWorkspaceFile(t *testing.T) {
	fd, c := newFakeDrive(t)
	id := fd.add("root", "Notes", "application/vnd.google-apps.document", nil)
	dir := t.TempDir()

	_, path, err := c.Download(context.Background(), id, dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "Notes.docx" {
		t.Fatalf("path = %s, want Notes.docx", path)
	}
}

func TestDownloadRejectsFolder(t *testing.T) {
	fd, c := newFakeDrive(t)
	id := fd.add("root", "Releases", FolderMimeType, nil)
	if _, _, err := c.Download(context.Background(), id, t.TempDir()); err == nil {
		t.Fatal("expected error for folder")
	}
}

func TestListFollowsPagination(t *testing.T) {
	fd, c := newFakeDrive(t)
	fd.maxPage = 2
	folder := fd.add("root", "Releases", FolderMimeType, nil)
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		fd.add(folder, n, "", nil)
	}
	files, err := c.List(context.Background(), folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatalf("got %d files, want 5", len(files))
	}
}

func TestListTree(t *testing.T) {
	fd, c := newFakeDrive(t)
	top := fd.add("root", "Releases", FolderMimeType, nil)
	sub := fd.add(top, "1.0", FolderMimeType, nil)
	fd.add(sub, "app.zip", "", nil)
	fd.add(top, "README", "", nil)

	entries, err := c.ListTree(context.Background(), top)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	want := []string{"1.0", "1.0/app.zip", "README"}
	if !slices.Equal(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestFindFilesSkipsFoldersAndTrash(t *testing.T) {
	fd, c := newFakeDrive(t)
	fd.add("root", "x", FolderMimeType, nil)
	gone := fd.add("root", "x", "", nil)
	keep := fd.add("root", "x", "", nil)
	fd.files[gone].trashed = true

	files, err := c.FindFiles(context.Background(), "root", "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Id != keep {
		t.Fatalf("got %v, want only %s", files, keep)
	}
}

func TestShareAndUnshare(t *testing.T) {
	fd, c := newFakeDrive(t)
	id := fd.add("root", "app.zip", "", nil)

	f, err := c.Share(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if f.WebViewLink == "" {
		t.Fatal("missing link")
	}
	if err := c.Unshare(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	want := []string{"share:" + id, "unshare:" + id}
	if ops := fd.opLog(); !slices.Equal(ops, want) {
		t.Fatalf("ops = %v, want %v", ops, want)
	}
}

func TestLocalName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"app.zip", "app.zip"},
		{`a/b\c`, "a_b_c"},
		{"what?.txt", "what_.txt"},
		{"..", "fallback"},
		{"trailing. ", "trailing"},
		{"", "fallback"},
	}
	for _, tt := range tests {
		if got := localName(tt.in, "fallback"); got != tt.want {
			t.Errorf("localName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
