package gdrive

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/drive/v3"
)

// ResolveFolderPath walks a slash-separated folder path under rootID and returns
// the ID of the final folder. When create is true, missing folders are created
// along the way.
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
			return "", fmt.Errorf("folder %q not found in path %q", part, folderPath)
		}

		meta := &drive.File{Name: part, MimeType: FolderMimeType, Parents: []string{current}}
		created, err := c.svc.Files.Create(meta).
			SupportsAllDrives(true).
			Fields("id,name").
			Context(ctx).
			Do()
		if err != nil {
			return "", err
		}
		current = created.Id
	}
	return current, nil
}

// FindFolderByName returns the first folder named name directly under parentID,
// or nil when none exists.
func (c *Client) FindFolderByName(ctx context.Context, parentID, name string) (*drive.File, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = '%s' and trashed = false", escapeQuery(name), parentID, FolderMimeType)
	res, err := c.svc.Files.List().
		Q(q).
		PageSize(1).
		Fields("files(id,name)").
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	if len(res.Files) == 0 {
		return nil, nil
	}
	return res.Files[0], nil
}

// ListFilesByName returns all files named name directly under parentID.
func (c *Client) ListFilesByName(ctx context.Context, parentID, name string) ([]*drive.File, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", escapeQuery(name), parentID)
	res, err := c.svc.Files.List().
		Q(q).
		Fields("files(id,name)").
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	return res.Files, nil
}

// escapeQuery escapes a value for use inside a Drive query string literal.
func escapeQuery(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "'", "\\'")
}
