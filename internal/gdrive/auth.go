package gdrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
)

// oauthCallbackAddr is the loopback address the local OAuth callback server
// listens on during the interactive authorization flow.
const oauthCallbackAddr = "127.0.0.1:8085"

// oauthClient returns an HTTP client authorized with a cached token, obtaining
// and persisting a new token via the web flow when none exists yet.
func oauthClient(ctx context.Context, conf *oauth2.Config, tokenPath string) (*http.Client, error) {
	tok, err := tokenFromFile(tokenPath)
	if err != nil {
		tok, err = tokenFromWeb(ctx, conf)
		if err != nil {
			return nil, err
		}
		if err := saveToken(tokenPath, tok); err != nil {
			return nil, err
		}
		fmt.Printf("saved oauth token: %s\n", tokenPath)
	}
	return conf.Client(ctx, tok), nil
}

// tokenFromWeb runs the interactive OAuth flow: it spins up a local callback
// server, opens the consent page, and exchanges the returned code for a token.
func tokenFromWeb(ctx context.Context, conf *oauth2.Config) (*oauth2.Token, error) {
	cfg := *conf
	cfg.RedirectURL = "http://" + oauthCallbackAddr + "/oauth2callback"
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

	srv := &http.Server{Addr: oauthCallbackAddr, Handler: mux}
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

// openBrowser opens url in the default browser. It uses rundll32 rather than
// `cmd /c start`, because cmd.exe treats `&` in the URL as a command separator
// and would truncate the OAuth URL at the first `&`, dropping params such as
// response_type and triggering a Google 400 error.
func openBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
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
