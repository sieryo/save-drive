package gdrive

import (
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type staticSource struct{ tok *oauth2.Token }

func (s staticSource) Token() (*oauth2.Token, error) { return s.tok, nil }

func TestSaveTokenRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "token.json")
	want := &oauth2.Token{AccessToken: "a", RefreshToken: "r", Expiry: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	if err := saveToken(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := tokenFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken || !got.Expiry.Equal(want.Expiry) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPersistingTokenSourceSavesRefreshedToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	src := &persistingTokenSource{src: staticSource{&oauth2.Token{AccessToken: "new", RefreshToken: "r"}}, path: path, last: "old"}

	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	got, err := tokenFromFile(path)
	if err != nil {
		t.Fatalf("refreshed token not saved: %v", err)
	}
	if got.AccessToken != "new" {
		t.Fatalf("saved %q, want new", got.AccessToken)
	}
}

func TestPersistingTokenSourceSkipsUnchangedToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	src := &persistingTokenSource{src: staticSource{&oauth2.Token{AccessToken: "same"}}, path: path, last: "same"}

	if _, err := src.Token(); err != nil {
		t.Fatal(err)
	}
	if _, err := tokenFromFile(path); err == nil {
		t.Fatal("unchanged token should not be written")
	}
}

func TestRandomStateIsUnique(t *testing.T) {
	a, _ := randomState()
	b, _ := randomState()
	if a == b || len(a) != 32 {
		t.Fatalf("states %q %q", a, b)
	}
}
