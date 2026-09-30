package gdrive

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/drive/v3"
)

// fakeDrive is an in-memory stand-in for the subset of the Drive v3 API that
// save-drive uses. It is deliberately simple: queries support only the
// clauses gdrive builds.
type fakeDrive struct {
	t *testing.T

	mu      sync.Mutex
	files   map[string]*fakeFile
	nextID  int
	clock   time.Time
	ops     []string // mutating calls in order, e.g. "create:a.zip", "trash:f3"
	perms   map[string][]string
	maxPage int // server-side page size cap, to exercise pagination

	badMD5     bool // report a wrong md5Checksum for uploads
	failStatus int  // status returned for the next failCount requests
	failCount  int
	requests   int
}

type fakeFile struct {
	id, name, mimeType string
	parents            []string
	trashed            bool
	content            []byte
	created, modified  time.Time
}

func newFakeDrive(t *testing.T) (*fakeDrive, *Client) {
	t.Helper()
	fd := &fakeDrive{
		t:       t,
		files:   map[string]*fakeFile{"root": {id: "root", name: "My Drive", mimeType: FolderMimeType}},
		clock:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		perms:   map[string][]string{},
		maxPage: 1000,
	}
	srv := httptest.NewServer(fd.handler())
	t.Cleanup(srv.Close)
	c, err := newWithHTTP(context.Background(), withRetry(srv.Client(), time.Millisecond), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	return fd, c
}

// add stores a file directly, bypassing the API, and returns its ID.
func (fd *fakeDrive) add(parent, name, mimeType string, content []byte) string {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	return fd.addLocked(parent, name, mimeType, content)
}

func (fd *fakeDrive) addLocked(parent, name, mimeType string, content []byte) string {
	fd.nextID++
	fd.clock = fd.clock.Add(time.Second)
	id := fmt.Sprintf("f%d", fd.nextID)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	fd.files[id] = &fakeFile{id: id, name: name, mimeType: mimeType, parents: []string{parent},
		content: content, created: fd.clock, modified: fd.clock}
	return id
}

func (fd *fakeDrive) opLog() []string {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	return append([]string(nil), fd.ops...)
}

func (fd *fakeDrive) file(id string) *fakeFile {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	return fd.files[id]
}

func (fd *fakeDrive) toAPI(f *fakeFile) *drive.File {
	out := &drive.File{
		Id: f.id, Name: f.name, MimeType: f.mimeType, Parents: f.parents, Trashed: f.trashed,
		CreatedTime: f.created.Format(time.RFC3339), ModifiedTime: f.modified.Format(time.RFC3339),
		WebViewLink: "https://drive.example/" + f.id,
	}
	if f.mimeType != FolderMimeType && !strings.HasPrefix(f.mimeType, workspaceMimePrefix) {
		sum := md5.Sum(f.content)
		out.Md5Checksum = hex.EncodeToString(sum[:])
		out.Size = int64(len(f.content))
	}
	return out
}

func (fd *fakeDrive) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /files", fd.list)
	mux.HandleFunc("POST /files", fd.createMeta)
	mux.HandleFunc("POST /upload/drive/v3/files", fd.upload)
	mux.HandleFunc("GET /files/{id}", fd.get)
	mux.HandleFunc("PATCH /files/{id}", fd.update)
	mux.HandleFunc("DELETE /files/{id}", fd.delete)
	mux.HandleFunc("GET /files/{id}/export", fd.export)
	mux.HandleFunc("POST /files/{id}/permissions", fd.createPerm)
	mux.HandleFunc("DELETE /files/{id}/permissions/{pid}", fd.deletePerm)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fd.mu.Lock()
		fd.requests++
		fail := fd.failCount > 0
		if fail {
			fd.failCount--
		}
		status := fd.failStatus
		fd.mu.Unlock()
		if fail {
			http.Error(w, `{"error":{"code":`+strconv.Itoa(status)+`,"message":"injected"}}`, status)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func notFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, `{"error":{"code":404,"message":"File not found"}}`)
}

var (
	reName     = regexp.MustCompile(`^name = '((?:[^'\\]|\\.)*)'$`)
	reParent   = regexp.MustCompile(`^'((?:[^'\\]|\\.)*)' in parents$`)
	reMime     = regexp.MustCompile(`^mimeType = '(.*)'$`)
	reNotMime  = regexp.MustCompile(`^mimeType != '(.*)'$`)
	unescapeQ  = strings.NewReplacer(`\'`, `'`, `\\`, `\`)
	reTrashedF = regexp.MustCompile(`^trashed = false$`)
)

func (fd *fakeDrive) matches(f *fakeFile, q string) bool {
	for _, clause := range strings.Split(q, " and ") {
		switch {
		case reName.MatchString(clause):
			if f.name != unescapeQ.Replace(reName.FindStringSubmatch(clause)[1]) {
				return false
			}
		case reParent.MatchString(clause):
			p := unescapeQ.Replace(reParent.FindStringSubmatch(clause)[1])
			if len(f.parents) == 0 || f.parents[0] != p {
				return false
			}
		case reMime.MatchString(clause):
			if f.mimeType != reMime.FindStringSubmatch(clause)[1] {
				return false
			}
		case reNotMime.MatchString(clause):
			if f.mimeType == reNotMime.FindStringSubmatch(clause)[1] {
				return false
			}
		case reTrashedF.MatchString(clause):
			if f.trashed {
				return false
			}
		default:
			fd.t.Errorf("fake drive: unsupported query clause %q", clause)
			return false
		}
	}
	return true
}

func (fd *fakeDrive) list(w http.ResponseWriter, r *http.Request) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	q := r.URL.Query()
	var hits []*fakeFile
	for _, f := range fd.files {
		if f.id != "root" && fd.matches(f, q.Get("q")) {
			hits = append(hits, f)
		}
	}
	switch q.Get("orderBy") {
	case "createdTime":
		sort.Slice(hits, func(i, j int) bool { return hits[i].created.Before(hits[j].created) })
	case "modifiedTime desc":
		sort.Slice(hits, func(i, j int) bool { return hits[i].modified.After(hits[j].modified) })
	default:
		sort.Slice(hits, func(i, j int) bool {
			fi, fj := hits[i].mimeType == FolderMimeType, hits[j].mimeType == FolderMimeType
			if fi != fj {
				return fi
			}
			return hits[i].name < hits[j].name
		})
	}

	size, _ := strconv.Atoi(q.Get("pageSize"))
	if size <= 0 || size > fd.maxPage {
		size = fd.maxPage
	}
	start, _ := strconv.Atoi(q.Get("pageToken"))
	end := min(start+size, len(hits))
	res := &drive.FileList{}
	for _, f := range hits[start:end] {
		res.Files = append(res.Files, fd.toAPI(f))
	}
	if end < len(hits) {
		res.NextPageToken = strconv.Itoa(end)
	}
	writeJSON(w, res)
}

func (fd *fakeDrive) createMeta(w http.ResponseWriter, r *http.Request) {
	var meta drive.File
	if err := json.NewDecoder(r.Body).Decode(&meta); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fd.mu.Lock()
	defer fd.mu.Unlock()
	id := fd.addLocked(meta.Parents[0], meta.Name, meta.MimeType, nil)
	fd.ops = append(fd.ops, "create:"+meta.Name)
	writeJSON(w, fd.toAPI(fd.files[id]))
}

func (fd *fakeDrive) upload(w http.ResponseWriter, r *http.Request) {
	if ut := r.URL.Query().Get("uploadType"); ut != "multipart" {
		http.Error(w, "fake drive supports multipart uploads only, got "+ut, http.StatusBadRequest)
		return
	}
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mr := multipart.NewReader(r.Body, params["boundary"])
	metaPart, err := mr.NextPart()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var meta drive.File
	if err := json.NewDecoder(metaPart).Decode(&meta); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mediaPart, err := mr.NextPart()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	content, _ := io.ReadAll(mediaPart)

	fd.mu.Lock()
	defer fd.mu.Unlock()
	id := fd.addLocked(meta.Parents[0], meta.Name, "", content)
	fd.ops = append(fd.ops, "create:"+meta.Name)
	out := fd.toAPI(fd.files[id])
	if fd.badMD5 {
		out.Md5Checksum = "00000000000000000000000000000000"
	}
	writeJSON(w, out)
}

func (fd *fakeDrive) get(w http.ResponseWriter, r *http.Request) {
	fd.mu.Lock()
	f, ok := fd.files[r.PathValue("id")]
	var out *drive.File
	var content []byte
	if ok {
		out = fd.toAPI(f)
		content = f.content
	}
	fd.mu.Unlock()
	if !ok {
		notFound(w)
		return
	}
	if r.URL.Query().Get("alt") == "media" {
		_, _ = w.Write(content)
		return
	}
	writeJSON(w, out)
}

func (fd *fakeDrive) update(w http.ResponseWriter, r *http.Request) {
	var patch map[string]any
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fd.mu.Lock()
	defer fd.mu.Unlock()
	f, ok := fd.files[r.PathValue("id")]
	if !ok {
		notFound(w)
		return
	}
	if patch["trashed"] == true {
		f.trashed = true
		fd.ops = append(fd.ops, "trash:"+f.id)
	}
	writeJSON(w, fd.toAPI(f))
}

func (fd *fakeDrive) delete(w http.ResponseWriter, r *http.Request) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	id := r.PathValue("id")
	if _, ok := fd.files[id]; !ok {
		notFound(w)
		return
	}
	delete(fd.files, id)
	fd.ops = append(fd.ops, "delete:"+id)
	w.WriteHeader(http.StatusNoContent)
}

func (fd *fakeDrive) export(w http.ResponseWriter, r *http.Request) {
	fd.mu.Lock()
	f, ok := fd.files[r.PathValue("id")]
	fd.mu.Unlock()
	if !ok {
		notFound(w)
		return
	}
	_, _ = io.WriteString(w, "exported:"+f.name+":"+r.URL.Query().Get("mimeType"))
}

func (fd *fakeDrive) createPerm(w http.ResponseWriter, r *http.Request) {
	var p drive.Permission
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fd.mu.Lock()
	defer fd.mu.Unlock()
	id := r.PathValue("id")
	fd.perms[id] = append(fd.perms[id], p.Type+":"+p.Role)
	fd.ops = append(fd.ops, "share:"+id)
	writeJSON(w, &drive.Permission{Id: anyoneWithLinkPermissionID, Type: p.Type, Role: p.Role})
}

func (fd *fakeDrive) deletePerm(w http.ResponseWriter, r *http.Request) {
	fd.mu.Lock()
	defer fd.mu.Unlock()
	id := r.PathValue("id")
	if r.PathValue("pid") != anyoneWithLinkPermissionID || len(fd.perms[id]) == 0 {
		notFound(w)
		return
	}
	delete(fd.perms, id)
	fd.ops = append(fd.ops, "unshare:"+id)
	w.WriteHeader(http.StatusNoContent)
}
