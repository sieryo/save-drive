package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const folderMimeType = "application/vnd.google-apps.folder"

type app struct {
	svc    *drive.Service
	logger *auditLogger
}

type auditLogger struct {
	path string
}

type auditEvent struct {
	Time       string `json:"time"`
	Action     string `json:"action"`
	Result     string `json:"result"`
	FileName   string `json:"file_name,omitempty"`
	FolderPath string `json:"folder_path,omitempty"`
	RemoteID   string `json:"remote_id,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type credsMeta struct {
	Type      string          `json:"type"`
	Installed json.RawMessage `json:"installed"`
	Web       json.RawMessage `json:"web"`
}

type pushOptions struct {
	filePath string
	to       string
	mkdir    bool
	replace  bool
	creds    string
	rootID   string
}

type listOptions struct {
	from   string
	creds  string
	rootID string
}

type fetchOptions struct {
	id     string
	name   string
	from   string
	to     string
	creds  string
	rootID string
}

type deleteOptions struct {
	id     string
	name   string
	from   string
	creds  string
	rootID string
}

type authorizeOptions struct {
	creds string
}

type logsOptions struct {
	limit int
	file  string
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "save-drive",
		Short:         "Simple Google Drive CLI for release artifact management",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Manage files in Google Drive from scripts.

Env vars:
  GOOGLE_OAUTH_CLIENT_CREDENTIALS  path to OAuth client credentials JSON (recommended)
  GOOGLE_OAUTH_TOKEN_FILE          token file path (default: .save-drive/oauth-token.json)
  GOOGLE_APPLICATION_CREDENTIALS   service account credentials JSON (optional)
  SAVE_DRIVE_LOG_FILE              override audit log file location
  DRIVE_ROOT_ID                    root folder ID (default: root)`,
	}

	cmd.AddCommand(newAuthorizeCmd())
	cmd.AddCommand(newPushCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newFetchCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newLogsCmd())
	return cmd
}

func newAuthorizeCmd() *cobra.Command {
	opts := authorizeOptions{}
	cmd := &cobra.Command{
		Use:   "authorize",
		Short: "Run OAuth authorization and cache token locally",
		Long:  "Starts the Google OAuth flow so the local token is created before running push/list/fetch/delete.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runAuthorize(opts)
		},
	}
	cmd.Flags().StringVar(&opts.creds, "creds", defaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	return cmd
}

func newPushCmd() *cobra.Command {
	opts := pushOptions{}
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Upload file to Drive folder",
		Example: "  save-drive push --file .\\build\\release.zip --to Releases/Prod --mkdir --replace\n" +
			"  save-drive push --file .\\README.md --to Test --mkdir",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runPush(opts)
		},
	}
	cmd.Flags().StringVar(&opts.filePath, "file", "", "Local file path")
	cmd.Flags().StringVar(&opts.to, "to", "", "Target folder path in Drive")
	cmd.Flags().BoolVar(&opts.mkdir, "mkdir", false, "Create folder path if it does not exist")
	cmd.Flags().BoolVar(&opts.replace, "replace", false, "Delete existing same-name file before upload")
	cmd.Flags().StringVar(&opts.creds, "creds", defaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", getenvOr("DRIVE_ROOT_ID", "root"), "Root folder ID")
	_ = cmd.MarkFlagRequired("file")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func newListCmd() *cobra.Command {
	opts := listOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List folder contents",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runList(opts)
		},
	}
	cmd.Flags().StringVar(&opts.from, "from", "", "Folder path in Drive")
	cmd.Flags().StringVar(&opts.creds, "creds", defaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", getenvOr("DRIVE_ROOT_ID", "root"), "Root folder ID")
	_ = cmd.MarkFlagRequired("from")
	return cmd
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
	cmd.Flags().StringVar(&opts.creds, "creds", defaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", getenvOr("DRIVE_ROOT_ID", "root"), "Root folder ID")
	return cmd
}

func newDeleteCmd() *cobra.Command {
	opts := deleteOptions{}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete file in Drive",
		Example: "  save-drive delete --id <FILE_ID>\n" +
			"  save-drive delete --name release.zip --from Releases/Prod",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDelete(opts)
		},
	}
	cmd.Flags().StringVar(&opts.id, "id", "", "Remote file ID")
	cmd.Flags().StringVar(&opts.name, "name", "", "Remote file name (requires --from)")
	cmd.Flags().StringVar(&opts.from, "from", "", "Folder path in Drive")
	cmd.Flags().StringVar(&opts.creds, "creds", defaultCredsPath(), "Credentials JSON path (OAuth client or service account)")
	cmd.Flags().StringVar(&opts.rootID, "root-id", getenvOr("DRIVE_ROOT_ID", "root"), "Root folder ID")
	return cmd
}

func newLogsCmd() *cobra.Command {
	opts := logsOptions{}
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show latest audit log entries",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runLogs(opts)
		},
	}
	cmd.Flags().IntVar(&opts.limit, "limit", 10, "Number of latest log entries")
	cmd.Flags().StringVar(&opts.file, "file", defaultLogPath(), "Audit log file path")
	return cmd
}

func runPush(opts pushOptions) error {
	a, err := newApp(opts.creds)
	if err != nil {
		return err
	}

	fileName := filepath.Base(opts.filePath)
	folderID, err := resolveFolderPath(context.Background(), a.svc, opts.rootID, opts.to, opts.mkdir)
	if err != nil {
		a.log(auditEvent{Time: nowUTC(), Action: "push", Result: "failed", FileName: fileName, FolderPath: opts.to, Detail: err.Error()})
		return err
	}

	existing, err := listFilesByName(context.Background(), a.svc, folderID, fileName)
	if err != nil {
		return err
	}
	if len(existing) > 0 && !opts.replace {
		return fmt.Errorf("file %q already exists, use --replace", fileName)
	}

	if opts.replace {
		for _, f := range existing {
			if err := a.svc.Files.Delete(f.Id).SupportsAllDrives(true).Context(context.Background()).Do(); err != nil {
				return fmt.Errorf("delete old file %s: %w", f.Id, err)
			}
		}
	}

	fd, err := os.Open(opts.filePath)
	if err != nil {
		return err
	}
	defer fd.Close()

	meta := &drive.File{Name: fileName, Parents: []string{folderID}}
	created, err := a.svc.Files.Create(meta).
		Media(fd).
		SupportsAllDrives(true).
		Fields("id,name,size,md5Checksum").
		Context(context.Background()).
		Do()
	if err != nil {
		a.log(auditEvent{Time: nowUTC(), Action: "push", Result: "failed", FileName: fileName, FolderPath: opts.to, Detail: err.Error()})
		return err
	}

	a.log(auditEvent{Time: nowUTC(), Action: "push", Result: "success", FileName: fileName, FolderPath: opts.to, RemoteID: created.Id, Detail: fmt.Sprintf("size=%d replace=%t", created.Size, opts.replace)})
	fmt.Printf("uploaded: %s (id=%s)\n", created.Name, created.Id)
	return nil
}

func runAuthorize(opts authorizeOptions) error {
	svc, err := newDriveService(context.Background(), opts.creds)
	if err != nil {
		return err
	}

	if _, err := svc.About.Get().Fields("user(displayName,emailAddress)").Context(context.Background()).Do(); err != nil {
		return fmt.Errorf("verify drive access: %w", err)
	}

	tokenFile := getenvOr("GOOGLE_OAUTH_TOKEN_FILE", filepath.Join(".save-drive", "oauth-token.json"))
	fmt.Printf("authorization ready; token file: %s\n", tokenFile)
	return nil
}

func runList(opts listOptions) error {
	a, err := newApp(opts.creds)
	if err != nil {
		return err
	}

	folderID, err := resolveFolderPath(context.Background(), a.svc, opts.rootID, opts.from, false)
	if err != nil {
		return err
	}
	q := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
	res, err := a.svc.Files.List().
		Q(q).
		Fields("files(id,name,mimeType,size,modifiedTime)").
		PageSize(1000).
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true).
		Context(context.Background()).
		Do()
	if err != nil {
		return err
	}

	for _, f := range res.Files {
		typeName := "file"
		if f.MimeType == folderMimeType {
			typeName = "folder"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", f.Id, typeName, f.Name, f.ModifiedTime)
	}
	a.log(auditEvent{Time: nowUTC(), Action: "list", Result: "success", FolderPath: opts.from, Detail: fmt.Sprintf("count=%d", len(res.Files))})
	return nil
}

func runFetch(opts fetchOptions) error {
	if opts.id == "" && opts.name == "" {
		return errors.New("fetch requires --id or --name")
	}

	a, err := newApp(opts.creds)
	if err != nil {
		return err
	}

	fileID := opts.id
	fileName := opts.name
	if fileID == "" {
		if opts.from == "" {
			return errors.New("fetch --name requires --from")
		}
		folderID, err := resolveFolderPath(context.Background(), a.svc, opts.rootID, opts.from, false)
		if err != nil {
			return err
		}
		files, err := listFilesByName(context.Background(), a.svc, folderID, opts.name)
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

	resp, err := a.svc.Files.Get(fileID).
		SupportsAllDrives(true).
		Context(context.Background()).
		Download()
	if err != nil {
		a.log(auditEvent{Time: nowUTC(), Action: "fetch", Result: "failed", RemoteID: fileID, Detail: err.Error()})
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

	a.log(auditEvent{Time: nowUTC(), Action: "fetch", Result: "success", RemoteID: fileID, FileName: fileName, Detail: fmt.Sprintf("saved_to=%s", to)})
	fmt.Printf("downloaded: %s\n", to)
	return nil
}

func runDelete(opts deleteOptions) error {
	if opts.id == "" && opts.name == "" {
		return errors.New("delete requires --id or --name")
	}

	a, err := newApp(opts.creds)
	if err != nil {
		return err
	}

	if opts.id != "" {
		if err := a.svc.Files.Delete(opts.id).SupportsAllDrives(true).Context(context.Background()).Do(); err != nil {
			a.log(auditEvent{Time: nowUTC(), Action: "delete", Result: "failed", RemoteID: opts.id, Detail: err.Error()})
			return err
		}
		a.log(auditEvent{Time: nowUTC(), Action: "delete", Result: "success", RemoteID: opts.id})
		fmt.Printf("deleted: %s\n", opts.id)
		return nil
	}

	if opts.from == "" {
		return errors.New("delete --name requires --from")
	}
	folderID, err := resolveFolderPath(context.Background(), a.svc, opts.rootID, opts.from, false)
	if err != nil {
		return err
	}
	files, err := listFilesByName(context.Background(), a.svc, folderID, opts.name)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("file %q not found", opts.name)
	}

	for _, f := range files {
		if err := a.svc.Files.Delete(f.Id).SupportsAllDrives(true).Context(context.Background()).Do(); err != nil {
			return err
		}
	}
	a.log(auditEvent{Time: nowUTC(), Action: "delete", Result: "success", FileName: opts.name, FolderPath: opts.from, Detail: fmt.Sprintf("count=%d", len(files))})
	fmt.Printf("deleted %d file(s) named %s\n", len(files), opts.name)
	return nil
}

func runLogs(opts logsOptions) error {
	if opts.limit <= 0 {
		return errors.New("--limit must be > 0")
	}

	path := opts.file
	if strings.TrimSpace(path) == "" {
		path = defaultLogPath()
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("no logs yet: %s\n", path)
			return nil
		}
		return err
	}
	defer f.Close()

	events := make([]auditEvent, 0, opts.limit)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		e := auditEvent{}
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(events) == 0 {
		fmt.Printf("no logs yet: %s\n", path)
		return nil
	}

	start := 0
	if len(events) > opts.limit {
		start = len(events) - opts.limit
	}
	fmt.Printf("log file: %s\n", path)
	for i := len(events) - 1; i >= start; i-- {
		e := events[i]
		target := e.FileName
		if target == "" {
			target = e.RemoteID
		}
		if target == "" {
			target = "-"
		}
		folder := e.FolderPath
		if folder == "" {
			folder = "-"
		}
		detail := e.Detail
		if detail == "" {
			detail = "-"
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", e.Time, e.Action, e.Result, target, folder, detail)
	}
	return nil
}

func newApp(credsPath string) (*app, error) {
	svc, err := newDriveService(context.Background(), credsPath)
	if err != nil {
		return nil, err
	}
	return &app{svc: svc, logger: &auditLogger{path: defaultLogPath()}}, nil
}

func newDriveService(ctx context.Context, credsPath string) (*drive.Service, error) {
	if strings.TrimSpace(credsPath) == "" {
		return nil, errors.New("credentials file not set; use --creds or set GOOGLE_OAUTH_CLIENT_CREDENTIALS")
	}

	b, err := os.ReadFile(credsPath)
	if err != nil {
		return nil, fmt.Errorf("read credentials file: %w", err)
	}

	var meta credsMeta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, fmt.Errorf("parse credentials file: %w", err)
	}

	if meta.Type == "service_account" {
		svc, err := drive.NewService(ctx, option.WithScopes(drive.DriveScope), option.WithCredentialsFile(credsPath))
		if err != nil {
			return nil, fmt.Errorf("init drive service (service account): %w", err)
		}
		return svc, nil
	}

	if len(meta.Installed) == 0 && len(meta.Web) == 0 {
		return nil, errors.New("unsupported credentials json; use OAuth client json (Desktop app) or service account json")
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("load oauth config: %w", err)
	}
	tokenFile := getenvOr("GOOGLE_OAUTH_TOKEN_FILE", filepath.Join(".save-drive", "oauth-token.json"))
	client, err := oauthClientFromToken(ctx, config, tokenFile)
	if err != nil {
		return nil, err
	}

	svc, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("init drive service (oauth): %w", err)
	}
	return svc, nil
}

func oauthClientFromToken(ctx context.Context, config *oauth2.Config, tokenPath string) (*http.Client, error) {
	tok, err := tokenFromFile(tokenPath)
	if err != nil {
		tok, err = tokenFromWeb(ctx, config)
		if err != nil {
			return nil, err
		}
		if err := saveToken(tokenPath, tok); err != nil {
			return nil, err
		}
		fmt.Printf("saved oauth token: %s\n", tokenPath)
	}
	return config.Client(ctx, tok), nil
}

func tokenFromWeb(ctx context.Context, config *oauth2.Config) (*oauth2.Token, error) {
	cfg := *config
	cfg.RedirectURL = "http://127.0.0.1:8085/oauth2callback"
	state := fmt.Sprintf("state-%d", time.Now().UnixNano())

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- errors.New("oauth state mismatch")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			errCh <- fmt.Errorf("oauth error: %s", e)
			http.Error(w, e, http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- errors.New("oauth code not found")
			http.Error(w, "code missing", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, "Authorization success. You can close this tab and return to terminal.")
		codeCh <- code
	})

	srv := &http.Server{Addr: "127.0.0.1:8085", Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	defer srv.Shutdown(context.Background())

	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
	fmt.Println("Google OAuth authorize URL:")
	fmt.Println(authURL)
	fmt.Println("If the browser does not open or the callback fails, open the URL above manually.")
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("warning: failed to open browser automatically: %v\n", err)
	}

	select {
	case code := <-codeCh:
		tok, err := cfg.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("oauth token exchange: %w", err)
		}
		return tok, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(3 * time.Minute):
		return nil, errors.New("oauth timeout; authorization not completed within 3 minutes")
	}
}

func openBrowser(url string) error {
	// Use rundll32 instead of `cmd /c start`: cmd.exe treats `&` in the URL as a
	// command separator and would truncate the OAuth URL at the first `&`,
	// dropping params like response_type and causing a Google 400 error.
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	return cmd.Start()
}

func tokenFromFile(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tok := &oauth2.Token{}
	if err := json.NewDecoder(f).Decode(tok); err != nil {
		return nil, err
	}
	return tok, nil
}

func saveToken(path string, token *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}

func resolveFolderPath(ctx context.Context, svc *drive.Service, rootID, folderPath string, create bool) (string, error) {
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
		found, err := findFolderByName(ctx, svc, current, part)
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

		meta := &drive.File{Name: part, MimeType: folderMimeType, Parents: []string{current}}
		created, err := svc.Files.Create(meta).
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

func findFolderByName(ctx context.Context, svc *drive.Service, parentID, name string) (*drive.File, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = '%s' and trashed = false", escapeQuery(name), parentID, folderMimeType)
	res, err := svc.Files.List().
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

func listFilesByName(ctx context.Context, svc *drive.Service, parentID, name string) ([]*drive.File, error) {
	q := fmt.Sprintf("name = '%s' and '%s' in parents and trashed = false", escapeQuery(name), parentID)
	res, err := svc.Files.List().
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

func (l *auditLogger) write(e auditEvent) {
	if l == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	_ = enc.Encode(e)
}

func (a *app) log(e auditEvent) {
	a.logger.write(e)
}

func escapeQuery(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "'", "\\'")
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func defaultCredsPath() string {
	if v := os.Getenv("GOOGLE_OAUTH_CLIENT_CREDENTIALS"); v != "" {
		return v
	}
	return os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
}

func defaultLogPath() string {
	if v := os.Getenv("SAVE_DRIVE_LOG_FILE"); v != "" {
		return v
	}
	exePath, err := os.Executable()
	if err != nil {
		return filepath.Join("logs", "audit.jsonl")
	}
	return filepath.Join(filepath.Dir(exePath), "logs", "audit.jsonl")
}

func getenvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
