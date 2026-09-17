// Package config is the single source of truth for authentication and defaults.
// Everything is read from the environment so the tool works identically in a
// shell, a cron job, or a coding agent — no interactive login, no state files.
package config

import (
	"fmt"
	"os"
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

// normalizeHost strips scheme and trailing slash from a site value so callers
// can safely do "https://" + Site.
func normalizeHost(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, "/")
	return s
}

// Load builds a Config from the environment. It accepts MAGPIE_* names first
// and falls back to the more generic ATLASSIAN_* names so the same token can be
// shared with other tooling.
func Load() (Config, error) {
	c := Config{
		Email:   firstEnv("MAGPIE_EMAIL", "ATLASSIAN_EMAIL"),
		Token:   firstEnv("MAGPIE_TOKEN", "ATLASSIAN_API_TOKEN", "ATLASSIAN_TOKEN"),
		Site:    normalizeHost(firstEnv("MAGPIE_SITE", "ATLASSIAN_SITE")),
		CloudID: firstEnv("MAGPIE_CLOUD_ID", "ATLASSIAN_CLOUD_ID"),
	}
	if c.Email == "" || c.Token == "" {
		return c, fmt.Errorf("missing credentials: set MAGPIE_EMAIL and MAGPIE_TOKEN " +
			"(or ATLASSIAN_EMAIL / ATLASSIAN_API_TOKEN)")
	}
	return c, nil
}

// ResolveSite returns the host to use for a request: the one parsed from a URL
// when present, otherwise the configured default. It is an error to have neither.
func (c Config) ResolveSite(fromURL string) (string, error) {
	if h := normalizeHost(fromURL); h != "" {
		return h, nil
	}
	if c.Site != "" {
		return c.Site, nil
	}
	return "", fmt.Errorf("no site given: pass a full URL or set MAGPIE_SITE " +
		"(e.g. MAGPIE_SITE=example.atlassian.net)")
}
