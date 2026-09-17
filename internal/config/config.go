// Package config is the single source of truth for authentication and defaults.
// Everything is read from the environment so the tool works identically in a
// shell, a cron job, or a coding agent — no interactive login, no state files.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the resolved credentials and site defaults.
type Config struct {
	Email   string // Atlassian account email
	Token   string // Atlassian API token (id.atlassian.com > Security > API tokens)
	Site    string // default host, e.g. "example.atlassian.net" (no scheme)
	CloudID string // optional; used by Goals. Auto-resolved if empty.
}

// firstEnv returns the first non-empty value among the given env vars.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// NormalizeHost strips scheme and trailing slash from a site value so callers
// can safely do "https://" + Site. Exported so callers building a Config
// directly (e.g. the `auth` wizard) get the same normalization as Load.
func NormalizeHost(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, "/")
	return s
}

// FilePath returns the credentials file written by `magpie auth`:
// ~/.config/magpie/.env. It is only ever a fallback for values not already set
// by real environment variables, so cron/CI/agents can still override it with
// a plain export.
func FilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "magpie", ".env"), nil
}

// loadFile reads KEY=VALUE lines from a dotenv-style file. A missing file is
// not an error — most users rely on plain environment variables instead.
func loadFile(path string) map[string]string {
	vals := map[string]string{}
	if path == "" {
		return vals
	}
	f, err := os.Open(path)
	if err != nil {
		return vals
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if n := len(val); n >= 2 && (val[0] == '"' && val[n-1] == '"' || val[0] == '\'' && val[n-1] == '\'') {
			val = val[1 : n-1]
		}
		vals[key] = val
	}
	return vals
}

// Load builds a Config from the environment. It accepts MAGPIE_* names first
// and falls back to the more generic ATLASSIAN_* names so the same token can be
// shared with other tooling. Anything still unset is filled in from the
// credentials file written by `magpie auth`, if present.
func Load() (Config, error) {
	path, _ := FilePath()
	file := loadFile(path)

	c := Config{
		Email:   firstEnv("MAGPIE_EMAIL", "ATLASSIAN_EMAIL"),
		Token:   firstEnv("MAGPIE_TOKEN", "ATLASSIAN_API_TOKEN", "ATLASSIAN_TOKEN"),
		Site:    NormalizeHost(firstEnv("MAGPIE_SITE", "ATLASSIAN_SITE")),
		CloudID: firstEnv("MAGPIE_CLOUD_ID", "ATLASSIAN_CLOUD_ID"),
	}
	if c.Email == "" {
		c.Email = file["MAGPIE_EMAIL"]
	}
	if c.Token == "" {
		c.Token = file["MAGPIE_TOKEN"]
	}
	if c.Site == "" {
		c.Site = NormalizeHost(file["MAGPIE_SITE"])
	}
	if c.CloudID == "" {
		c.CloudID = file["MAGPIE_CLOUD_ID"]
	}
	if c.Email == "" || c.Token == "" {
		return c, fmt.Errorf("missing credentials: set MAGPIE_EMAIL and MAGPIE_TOKEN " +
			"(or ATLASSIAN_EMAIL / ATLASSIAN_API_TOKEN), or run `magpie auth`")
	}
	return c, nil
}

// Save writes credentials to the fallback file (~/.config/magpie/.env),
// creating its directory if needed and restricting it to owner-only
// read/write since it holds a live API token.
func Save(c Config) (string, error) {
	path, err := FilePath()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "MAGPIE_EMAIL=%q\n", c.Email)
	fmt.Fprintf(&b, "MAGPIE_TOKEN=%q\n", c.Token)
	fmt.Fprintf(&b, "MAGPIE_SITE=%q\n", c.Site)
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		return "", err
	}
	// WriteFile only applies the mode on create; force it on overwrite too.
	if err := os.Chmod(path, 0600); err != nil {
		return "", err
	}
	return path, nil
}

// ResolveSite returns the host to use for a request: the one parsed from a URL
// when present, otherwise the configured default. It is an error to have neither.
func (c Config) ResolveSite(fromURL string) (string, error) {
	if h := NormalizeHost(fromURL); h != "" {
		return h, nil
	}
	if c.Site != "" {
		return c.Site, nil
	}
	return "", fmt.Errorf("no site given: pass a full URL or set MAGPIE_SITE " +
		"(e.g. MAGPIE_SITE=example.atlassian.net)")
}
