// Package audit records and reads structured audit events for save-drive.
//
// Events are stored as JSON Lines (one JSON object per line) so the log is both
// append-friendly and trivially parseable.
package audit

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Event is a single audit log record.
type Event struct {
	Time       string `json:"time"`
	Action     string `json:"action"`
	Result     string `json:"result"`
	FileName   string `json:"file_name,omitempty"`
	FolderPath string `json:"folder_path,omitempty"`
	RemoteID   string `json:"remote_id,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// Logger appends audit events to a JSONL file.
type Logger struct {
	Path string
}

// NewLogger returns a Logger writing to path.
func NewLogger(path string) *Logger {
	return &Logger{Path: path}
}

// Append writes e to the log file, stamping e.Time with the current UTC time
// when it is empty. Callers should report a returned error as a warning
// rather than fail the command, so auditing never blocks the primary
// operation.
func (l *Logger) Append(e Event) error {
	if l == nil {
		return nil
	}
	if e.Time == "" {
		e.Time = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(e); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ReadAll loads every parseable event from path in chronological order.
// A missing file returns (nil, nil) so callers can treat "no log yet" as a
// non-error. Unparseable lines are skipped.
func ReadAll(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}
