package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Scopes contains the Google Chat API OAuth2 scopes requested during user
// authentication. These are the scopes that work with the standard OAuth2
// consent flow for desktop applications.
var Scopes = []string{
	"https://www.googleapis.com/auth/chat.spaces",
	"https://www.googleapis.com/auth/chat.spaces.readonly",
	"https://www.googleapis.com/auth/chat.spaces.create",
	"https://www.googleapis.com/auth/chat.messages",
	"https://www.googleapis.com/auth/chat.messages.readonly",
	"https://www.googleapis.com/auth/chat.messages.create",
	"https://www.googleapis.com/auth/chat.messages.reactions",
	"https://www.googleapis.com/auth/chat.messages.reactions.readonly",
	"https://www.googleapis.com/auth/chat.messages.reactions.create",
	"https://www.googleapis.com/auth/chat.memberships",
	"https://www.googleapis.com/auth/chat.memberships.readonly",
	"https://www.googleapis.com/auth/chat.customemojis",
	"https://www.googleapis.com/auth/chat.customemojis.readonly",
	"https://www.googleapis.com/auth/chat.users.readstate",
	"https://www.googleapis.com/auth/chat.users.readstate.readonly",
	"https://www.googleapis.com/auth/chat.users.spacesettings",
}

// RestrictedScopes contains scopes that require special access such as
// Workspace admin privileges, domain-wide delegation, or Google approval.
// These are NOT requested during normal user login.
var RestrictedScopes = []string{
	"https://www.googleapis.com/auth/chat.admin.spaces",
	"https://www.googleapis.com/auth/chat.admin.spaces.readonly",
	"https://www.googleapis.com/auth/chat.admin.memberships",
	"https://www.googleapis.com/auth/chat.admin.memberships.readonly",
	"https://www.googleapis.com/auth/chat.admin.delete",
	"https://www.googleapis.com/auth/chat.delete",
	"https://www.googleapis.com/auth/chat.memberships.app",
	"https://www.googleapis.com/auth/chat.import",
}

// DefaultClientID is the OAuth2 client ID for the gogchat CLI.
// This is set at build time via -ldflags:
//
//	go build -ldflags "-X 'github.com/cipher-shad0w/gogchat/internal/auth.DefaultClientID=YOUR_ID'"
//
// Users can also override this via the --client-id flag, config file, or
// GOGCHAT_CLIENT_ID environment variable.
var DefaultClientID string

// DefaultClientSecret is the OAuth2 client secret for the gogchat CLI.
// This is set at build time via -ldflags:
//
//	go build -ldflags "-X 'github.com/cipher-shad0w/gogchat/internal/auth.DefaultClientSecret=YOUR_SECRET'"
//
// Users can also override this via the --client-secret flag, config file, or
// GOGCHAT_CLIENT_SECRET environment variable.
var DefaultClientSecret string

// ErrMissingCredentials is returned when OAuth2 credentials are not configured.
var ErrMissingCredentials = errors.New(`OAuth2 credentials are not configured.

If you installed gogchat via Homebrew or a release binary, this is a bug — please report it.

If you built from source, you need to supply your own Google OAuth2 credentials:

  Option 1: Build with credentials baked in:
    go build -ldflags "-X 'github.com/cipher-shad0w/gogchat/internal/auth.DefaultClientID=YOUR_ID' \
                        -X 'github.com/cipher-shad0w/gogchat/internal/auth.DefaultClientSecret=YOUR_SECRET'" \
      ./cmd/gogchat

  Option 2: Pass credentials at runtime:
    gogchat auth login --client-id YOUR_ID --client-secret YOUR_SECRET

  Option 3: Set environment variables:
    export GOGCHAT_CLIENT_ID=YOUR_ID
    export GOGCHAT_CLIENT_SECRET=YOUR_SECRET

To create OAuth2 credentials, visit:
  https://console.cloud.google.com/apis/credentials`)

// ValidateCredentials checks that both a client ID and client secret are
// available. It returns ErrMissingCredentials if either is empty.
func ValidateCredentials(clientID, clientSecret string) error {
	if clientID == "" || clientSecret == "" {
		return ErrMissingCredentials
	}
	return nil
}

// redirectURI is the local callback address used during the OAuth2 flow.
const redirectURI = "http://localhost:8085"

// GetOAuthConfig creates an OAuth2 configuration for the Google Chat API
// using the provided client credentials.
func GetOAuthConfig(clientID, clientSecret string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURI,
		Scopes:       Scopes,
	}
}

// Login performs the full interactive OAuth2 authorization-code flow.
// It starts a local HTTP server on localhost:8085 to receive the callback,
// opens the user's browser to the consent screen, waits for the authorization
// code, exchanges it for a token, and returns the resulting token.
func Login(clientID, clientSecret string) (*oauth2.Token, error) {
	cfg := GetOAuthConfig(clientID, clientSecret)

	// Generate the authorization URL requesting offline access so that a
	// refresh token is included in the response.
	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	// Channel to receive the authorization code (or an error) from the
	// callback handler.
	type callbackResult struct {
		code string
		err  error
	}
	resultCh := make(chan callbackResult, 1)

	// Set up a temporary HTTP server to handle the OAuth2 redirect.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			if errMsg == "" {
				errMsg = "no authorization code received"
			}
			http.Error(w, "Authentication failed: "+errMsg, http.StatusBadRequest)
			resultCh <- callbackResult{err: fmt.Errorf("OAuth callback error: %s", errMsg)}
			return
		}

		// Show a success page in the browser.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><h1>Authentication successful!</h1><p>You may close this window and return to the terminal.</p></body></html>")
		resultCh <- callbackResult{code: code}
	})

	// Bind the listener before opening the browser so we know the port is
	// available.
	listener, err := net.Listen("tcp", "localhost:8085")
	if err != nil {
		return nil, fmt.Errorf("starting local HTTP server: %w", err)
	}

	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(listener)
	}()

	// Open the authorization URL in the user's default browser.
	fmt.Println("Opening browser for authentication...")
	fmt.Printf("If the browser does not open automatically, visit:\n%s\n", authURL)
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Warning: could not open browser automatically: %v\n", err)
	}

	// Block until the callback delivers a result.
	res := <-resultCh

	// Shut down the temporary server; ignore errors since we only care about
	// the token exchange at this point.
	_ = server.Shutdown(context.Background())

	if res.err != nil {
		return nil, res.err
	}

	// Exchange the authorization code for a token.
	token, err := cfg.Exchange(context.Background(), res.code)
	if err != nil {
		return nil, fmt.Errorf("exchanging authorization code: %w", err)
	}

	return token, nil
}

// RefreshToken uses the refresh token embedded in the provided token to obtain
// a new access token from Google's token endpoint.
func RefreshToken(clientID, clientSecret string, token *oauth2.Token) (*oauth2.Token, error) {
	cfg := GetOAuthConfig(clientID, clientSecret)
	src := cfg.TokenSource(context.Background(), token)

	newToken, err := src.Token()
	if err != nil {
		return nil, fmt.Errorf("refreshing token: %w", err)
	}
	return newToken, nil
}

// TokenSource returns a reusable oauth2.TokenSource that automatically
// refreshes the access token when it expires.
func TokenSource(clientID, clientSecret string, token *oauth2.Token) oauth2.TokenSource {
	cfg := GetOAuthConfig(clientID, clientSecret)
	return cfg.TokenSource(context.Background(), token)
}

// HTTPClient returns an *http.Client that automatically attaches OAuth2
// credentials to every outgoing request and refreshes the token as needed.
func HTTPClient(clientID, clientSecret string, token *oauth2.Token) *http.Client {
	cfg := GetOAuthConfig(clientID, clientSecret)
	return cfg.Client(context.Background(), token)
}

// openBrowser attempts to open the given URL in the user's default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
