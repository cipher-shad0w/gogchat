package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
)

// DefaultTokenPath returns the default filesystem path where the OAuth2 token
// is stored: ~/.config/gogchat/token.json.
func DefaultTokenPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "gogchat", "token.json")
}

// SaveToken serialises the given OAuth2 token as JSON and writes it to the
// specified path. Parent directories are created automatically. The file is
// written with 0600 permissions so that only the current user can read it.
func SaveToken(path string, token *oauth2.Token) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating token directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling token: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing token file %s: %w", path, err)
	}

	return nil
}

// LoadToken reads an OAuth2 token from the JSON file at the given path.
func LoadToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading token file %s: %w", path, err)
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("parsing token file %s: %w", path, err)
	}

	return &token, nil
}

// DeleteToken removes the token file at the given path.
func DeleteToken(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing token file %s: %w", path, err)
	}
	return nil
}

// TokenExists reports whether a token file exists at the given path.
func TokenExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
