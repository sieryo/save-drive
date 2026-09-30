package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendReadAllRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "audit.jsonl")
	l := NewLogger(path)
	events := []Event{
		{Action: "push", Result: "success", FileName: "a.zip", FolderPath: "Releases", RemoteID: "f1"},
		{Action: "delete", Result: "failed", Detail: "boom"},
	}
	for _, e := range events {
		if err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ReadAll(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	for i := range events {
		if got[i].Time == "" {
			t.Errorf("event %d: time not stamped", i)
		}
		got[i].Time = ""
		if got[i] != events[i] {
			t.Errorf("event %d = %+v, want %+v", i, got[i], events[i])
		}
	}
}

func TestReadAllSkipsBadLinesAndReadsOldFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	content := `{"time":"2026-01-01T00:00:00Z","action":"push","result":"success","file_name":"a.zip"}
not json
{"time":"2026-01-02T00:00:00Z","action":"list","result":"success","folder_path":"Test","detail":"count=3"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAll(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].FileName != "a.zip" || got[1].Detail != "count=3" {
		t.Fatalf("got %+v", got)
	}
}

func TestReadAllMissingFile(t *testing.T) {
	got, err := ReadAll(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil || got != nil {
		t.Fatalf("got %v, %v; want nil, nil", got, err)
	}
}

func TestAppendReportsUnwritablePath(t *testing.T) {
	dir := t.TempDir()
	// A regular file where the log directory should be makes MkdirAll fail.
	blocker := filepath.Join(dir, "logs")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewLogger(filepath.Join(blocker, "audit.jsonl")).Append(Event{Action: "push"}); err == nil {
		t.Fatal("expected error")
	}
}
