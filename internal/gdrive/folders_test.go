package gdrive

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResolveFolderPathExisting(t *testing.T) {
	fd, c := newFakeDrive(t)
	a := fd.add("root", "Releases", FolderMimeType, nil)
	b := fd.add(a, "Prod", FolderMimeType, nil)

	got, err := c.ResolveFolderPath(context.Background(), "root", "/Releases/Prod/", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != b {
		t.Fatalf("got %s, want %s", got, b)
	}
}

func TestResolveFolderPathEmptyIsRoot(t *testing.T) {
	_, c := newFakeDrive(t)
	got, err := c.ResolveFolderPath(context.Background(), "root", "/", false)
	if err != nil || got != "root" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestResolveFolderPathMissing(t *testing.T) {
	fd, c := newFakeDrive(t)
	fd.add("root", "Releases", FolderMimeType, nil)

	_, err := c.ResolveFolderPath(context.Background(), "root", "Releases/Prod", false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if ops := fd.opLog(); len(ops) != 0 {
		t.Fatalf("unexpected mutations: %v", ops)
	}
}

func TestResolveFolderPathCreate(t *testing.T) {
	fd, c := newFakeDrive(t)
	fd.add("root", "Releases", FolderMimeType, nil)

	id, err := c.ResolveFolderPath(context.Background(), "root", "Releases/Prod/1.0", true)
	if err != nil {
		t.Fatal(err)
	}
	if f := fd.file(id); f == nil || f.name != "1.0" || f.mimeType != FolderMimeType {
		t.Fatalf("got %+v", f)
	}
	again, err := c.ResolveFolderPath(context.Background(), "root", "Releases/Prod/1.0", true)
	if err != nil || again != id {
		t.Fatalf("second resolve = %q, %v; want %q", again, err, id)
	}
}

func TestResolveFolderPathPicksOldestDuplicate(t *testing.T) {
	fd, c := newFakeDrive(t)
	oldest := fd.add("root", "Releases", FolderMimeType, nil)
	fd.add("root", "Releases", FolderMimeType, nil)

	got, err := c.ResolveFolderPath(context.Background(), "root", "Releases", false)
	if err != nil || got != oldest {
		t.Fatalf("got %q, %v; want %q", got, err, oldest)
	}
}

func TestCreateFolderLosesRaceToOlderFolder(t *testing.T) {
	fd, c := newFakeDrive(t)
	// Simulate a concurrent run: an identical folder already exists but was
	// created before ours, and the lookup that led us to create missed it.
	winner := fd.add("root", "Releases", FolderMimeType, nil)

	got, err := c.createFolder(context.Background(), "root", "Releases")
	if err != nil {
		t.Fatal(err)
	}
	if got != winner {
		t.Fatalf("got %s, want older folder %s", got, winner)
	}
	ops := fd.opLog()
	if len(ops) != 2 || ops[0] != "create:Releases" || !strings.HasPrefix(ops[1], "trash:") || ops[1] == "trash:"+winner {
		t.Fatalf("ops = %v, want create then trash", ops)
	}
}

func TestEscapeQuery(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", "plain"},
		{"it's", `it\'s`},
		{`back\slash`, `back\\slash`},
		{`\'`, `\\\'`},
	}
	for _, tt := range tests {
		if got := escapeQuery(tt.in); got != tt.want {
			t.Errorf("escapeQuery(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveFolderPathQuotedName(t *testing.T) {
	fd, c := newFakeDrive(t)
	want := fd.add("root", "Bob's builds", FolderMimeType, nil)
	got, err := c.ResolveFolderPath(context.Background(), "root", "Bob's builds", false)
	if err != nil || got != want {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
}
