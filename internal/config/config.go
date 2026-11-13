// Package config keeps the API key that `photos login` saves.
//
// One small JSON file, `credentials`, readable by its owner only, in the
// user's configuration directory: ~/.config/photos-sh on Linux,
// ~/Library/Application Support/photos-sh on macOS, %AppData%\photos-sh on
// Windows. PHOTOS_CONFIG_DIR names another directory.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// EnvDir is the environment variable that overrides the directory.
	EnvDir = "PHOTOS_CONFIG_DIR"

	dirName  = "photos-sh"
	fileName = "credentials"
)

// Credentials is what `photos login` saves.
type Credentials struct {
	// APIKey is the key.
	APIKey string `json:"api_key"`
	// BaseURL is the API the key was verified against, when that was not
	// the default: the key is used with it, so that it is not sent elsewhere.
	BaseURL string `json:"base_url,omitempty"`
	// Account is the account's name as the API gave it at login.
	Account string `json:"account,omitempty"`
}

// Dir is the directory the credentials live in.
func Dir() (string, error) {
	if dir := os.Getenv(EnvDir); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot find your configuration directory (set %s): %w", EnvDir, err)
	}
	return filepath.Join(base, dirName), nil
}

// Path is the credentials file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// Load reads the saved credentials. With none saved it returns nil and no
// error.
func Load() (*Credentials, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	c := &Credentials{}
	if err := json.Unmarshal(data, c); err != nil {
		// Not the decoder's message: it can quote the file, which holds a key.
		return nil, fmt.Errorf("%s is not valid: run `photos login` again", path)
	}
	if c.APIKey == "" {
		return nil, nil
	}
	return c, nil
}

// Save writes the credentials, readable by the owner only (0600, in a 0700
// directory), through a temporary file so that a crash leaves the old file
// or the new one. It returns the file's path.
func Save(c *Credentials) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "."+fileName+"-*") // created 0600
	if err != nil {
		return "", fmt.Errorf("cannot write in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name()) // a no-op once renamed
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}

// Remove deletes the saved credentials and reports whether there were any.
func Remove() (bool, error) {
	path, err := Path()
	if err != nil {
		return false, err
	}
	err = os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
