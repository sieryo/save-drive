package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func touch(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExpandFiles(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "a.zip", "b.zip", "notes.txt")
	if err := os.Mkdir(filepath.Join(dir, "sub.zip"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := expandFiles([]string{filepath.Join(dir, "*.zip"), filepath.Join(dir, "notes.txt")})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, filepath.Base(p))
	}
	// The directory matched by the glob is skipped.
	if want := []string{"a.zip", "b.zip", "notes.txt"}; !slices.Equal(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
}

func TestExpandFilesErrors(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	touch(t, dir, "a.zip")
	touch(t, other, "A.ZIP")

	tests := []struct {
		name     string
		patterns []string
		want     string
	}{
		{"missing file", []string{filepath.Join(dir, "typo.zip")}, "local file"},
		{"directory", []string{dir}, "not a regular file"},
		{"glob without match", []string{filepath.Join(dir, "*.exe")}, "matches no files"},
		{"duplicate name", []string{filepath.Join(dir, "a.zip"), filepath.Join(other, "A.ZIP")}, "same file name"},
	}
	for _, tt := range tests {
		_, err := expandFiles(tt.patterns)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestTSVField(t *testing.T) {
	if got := tsvField("a\tb\nc\rd"); got != "a b c d" {
		t.Fatalf("got %q", got)
	}
}
