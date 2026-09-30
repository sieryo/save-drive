package gdrive

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/drive/v3"
)

// ErrNotFound is returned (wrapped) when a folder or file cannot be found.
var ErrNotFound = errors.New("not found")

// ResolveFolderPath walks a slash-separated folder path under rootID and returns
// the ID of the final folder. When create is true, missing folders are created
// along the way. A missing folder without create yields an error wrapping
// ErrNotFound.
//
// Drive allows several folders with the same name, so lookups always pick the
// oldest one. That keeps resolution stable across runs, and lets two
// concurrent `push --mkdir` runs converge on the same folder.
func (c *Client) ResolveFolderPath(ctx context.Context, rootID, folderPath string, create bool) (string, error) {
	cleaned := strings.Trim(folderPath, "/")
	if cleaned == "" {
		return rootID, nil
	}

	current := rootID
	for _, part := range strings.Split(cleaned, "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		found, err := c.FindFolderByName(ctx, current, part)
		if err != nil {
			return "", err
		}
		if found != nil {
			current = found.Id
			continue
		}
		if !create {
			return "", fmt.Errorf("folder %q in path %q: %w", part, folderPath, ErrNotFound)
		}

		id, err := c.createFolder(ctx, current, part)
		if err != nil {
			return "", err
		}
		current = id
	}
	return current, nil
}

// createFolder creates name under parentID and returns the ID to use. If a
// concurrent run created the same folder first, the older folder wins and the
// one just created is moved to trash.
func (c *Client) createFolder(ctx context.Context, parentID, name string) (string, error) {
	meta := &drive.File{Name: name, MimeType: FolderMimeType, Parents: []string{parentID}}
	created, err := c.svc.Files.Create(meta).
		SupportsAllDrives(true).
		Fields("id").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("create folder %q: %w", name, err)
	}

	oldest, err := c.FindFolderByName(ctx, parentID, name)
	if err != nil {
		return "", err
	}
	if oldest == nil || oldest.Id == created.Id {
		return created.Id, nil
	}
	if err := c.Trash(ctx, created.Id); err != nil {
		return "", err
	}
	return oldest.Id, nil
}

// FindFolderByName returns the oldest folder named name directly under
// parentID, or nil when none exists.
func (c *Client) FindFolderByName(ctx context.Context, parentID, name string) (*drive.File, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = '%s' and trashed = false",
		escapeQuery(name), escapeQuery(parentID), FolderMimeType)
	res, err := c.svc.Files.List().
		Q(q).
		OrderBy("createdTime").
		PageSize(1).
		Fields("files(id,name)").
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("find folder %q: %w", name, err)
	}
	if len(res.Files) == 0 {
		return nil, nil
	}
	return res.Files[0], nil
}

// FindFiles returns the files (never folders) named name directly under
// parentID, most recently modified first.
func (c *Client) FindFiles(ctx context.Context, parentID, name string) ([]*drive.File, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and mimeType != '%s' and trashed = false",
		escapeQuery(name), escapeQuery(parentID), FolderMimeType)
	files, err := c.list(ctx, q, "modifiedTime desc")
	if err != nil {
		return nil, fmt.Errorf("find file %q: %w", name, err)
	}
	return files, nil
}

// escapeQuery escapes a value for use inside a Drive query string literal.
func escapeQuery(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "'", "\\'")
}
