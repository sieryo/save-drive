package gdrive

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const (
	// fixedCallbackAddr is used for "web" OAuth clients, whose redirect URI
	// must match the one registered in Google Cloud exactly.
	fixedCallbackAddr = "127.0.0.1:8085"
	// dynamicCallbackAddr lets the OS pick a free port; Desktop app clients
	// accept any loopback port.
	dynamicCallbackAddr = "127.0.0.1:0"

	oauthTimeout = 3 * time.Minute
)

// oauthTokenSource returns a token source backed by the cached token at
// tokenPath. The interactive flow runs when forceLogin is set or no usable
// token file exists. Refreshed tokens are written back to tokenPath.
func oauthTokenSource(ctx context.Context, conf *oauth2.Config, tokenPath string, forceLogin, fixedPort bool) (oauth2.TokenSource, error) {
	var tok *oauth2.Token
	var err error
	if !forceLogin {
		tok, err = tokenFromFile(tokenPath)
	}
	if forceLogin || err != nil {
		tok, err = tokenFromWeb(ctx, conf, fixedPort)
		if err != nil {
			return nil, err
		}
		if err := saveToken(tokenPath, tok); err != nil {
			return nil, fmt.Errorf("save oauth token: %w", err)
		}
		fmt.Fprintf(os.Stderr, "saved oauth token: %s\n", tokenPath)
	}
	return &persistingTokenSource{src: conf.TokenSource(ctx, tok), path: tokenPath, last: tok.AccessToken}, nil
}

// persistingTokenSource saves every newly refreshed token so the next run can
// reuse the access token instead of refreshing again.
type persistingTokenSource struct {
	src  oauth2.TokenSource
	path string

	mu   sync.Mutex
	last string
}

func (s *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := s.src.Token()
	if err != nil {
		return nil, fmt.Errorf("oauth token is invalid or expired; run `save-drive authorize`: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tok.AccessToken != s.last {
		s.last = tok.AccessToken
		if err := saveToken(s.path, tok); err != nil {
			return nil, fmt.Errorf("save refreshed oauth token: %w", err)
		}
	}
	return tok, nil
}

// tokenFromWeb runs the interactive OAuth flow: it spins up a local callback
// server, opens the consent page, and exchanges the returned code for a token.
// Messages go to stderr so they never mix with output a script parses.
func tokenFromWeb(ctx context.Context, conf *oauth2.Config, fixedPort bool) (*oauth2.Token, error) {
	addr := dynamicCallbackAddr
	if fixedPort {
		addr = fixedCallbackAddr
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen for oauth callback on %s: %w", addr, err)
	}

	cfg := *conf
	cfg.RedirectURL = "http://" + ln.Addr().String() + "/oauth2callback"
	state, err := randomState()
	if err != nil {
		ln.Close()
		return nil, err
	}
	verifier := oauth2.GenerateVerifier()

	type result struct {
		code string
		err  error
	}
	// Buffered and written with a non-blocking send: a repeated callback (for
	// example a browser refresh) must never block a handler, or Shutdown
	// below would wait for it forever.
	resCh := make(chan result, 1)
	deliver := func(r result) {
		select {
		case resCh <- r:
		default:
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		// A request with the wrong state did not come from our consent page;
		// reject it without aborting the flow.
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			deliver(result{err: fmt.Errorf("oauth error: %s", e)})
			http.Error(w, e, http.StatusBadRequest)
			return
		}
		code := q.Get("code")
		if code == "" {
			deliver(result{err: errors.New("oauth code not found")})
			http.Error(w, "code missing", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, "Authorization success. You can close this tab and return to terminal.")
		deliver(result{code: code})
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			deliver(result{err: fmt.Errorf("oauth callback server: %w", err)})
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	// ApprovalForce makes Google return a refresh token even when this account
	// already granted access before; without it the saved token dies after an
	// hour.
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))
	fmt.Fprintln(os.Stderr, "Google OAuth authorize URL:")
	fmt.Fprintln(os.Stderr, authURL)
	fmt.Fprintln(os.Stderr, "If the browser does not open or the callback fails, open the URL above manually.")
	if err := openBrowser(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to open browser automatically: %v\n", err)
	}

	timer := time.NewTimer(oauthTimeout)
	defer timer.Stop()
	select {
	case r := <-resCh:
		if r.err != nil {
			return nil, r.err
		}
		tok, err := cfg.Exchange(ctx, r.code, oauth2.VerifierOption(verifier))
		if err != nil {
			return nil, fmt.Errorf("oauth token exchange: %w", err)
		}
		return tok, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, errors.New("oauth timeout; authorization not completed within 3 minutes")
	}
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// openBrowser opens url in the default browser. On Windows it uses rundll32
// rather than `cmd /c start`, because cmd.exe treats `&` in the URL as a
// command separator and would truncate the OAuth URL at the first `&`,
// dropping params such as response_type and triggering a Google 400 error.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
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

// saveToken writes token atomically so a crash mid-write never leaves a
// truncated token file behind.
func saveToken(path string, token *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(token)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
