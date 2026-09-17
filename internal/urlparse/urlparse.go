// Package urlparse turns a pasted Atlassian URL — or a bare id/key — into a
// typed Target. This is the shared "location" handling behind every command, so
// an agent can hand the tool whatever link it found without pre-processing.
package urlparse

import (
	"net/url"
	"regexp"
	"strings"
)

// Kind classifies what a location points at.
type Kind int

const (
	Unknown Kind = iota
	Issue        // Jira issue
	Board        // Jira board
	Page         // Confluence page
	Goal         // Atlassian Home goal
	GoalList     // Atlassian Home goals list view (no single id)
)

// Target is the parsed result of a location argument.
type Target struct {
	Kind    Kind
	Site    string // host, e.g. "example.atlassian.net" (empty for bare ids)
	ID      string // issue key / board id / page id / goal key
	CloudID string // from a ?cloudId= query param, when present
	TQL     string // from a ?tql= query param (goals list views)
}

var (
	issueKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]+-\d+$`)
	digitsRe   = regexp.MustCompile(`^\d+$`)
	// path segment ".../boards/123..." or ".../pages/123/..."
	boardsSegRe = regexp.MustCompile(`/boards/(\d+)`)
	pagesSegRe  = regexp.MustCompile(`/pages/(\d+)`)
	goalSegRe   = regexp.MustCompile(`/goal/([^/?#]+)`)
)

// isURL reports whether s looks like an http(s) URL.
func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// Parse interprets a location string. When it is not a URL, hint disambiguates
// bare ids (a numeric board id vs a numeric page id look identical).
//
// hint should be one of the target Kinds the calling domain expects for a bare
// id: Issue/Board for jira, Page for confluence, Goal for goals.
func Parse(s string, hint Kind) Target {
	s = strings.TrimSpace(s)
	if !isURL(s) {
		return parseBare(s, hint)
	}

	u, err := url.Parse(s)
	if err != nil {
		return Target{Kind: Unknown}
	}
	t := Target{Site: u.Host}
	q := u.Query()
	t.CloudID = q.Get("cloudId")
	t.TQL = q.Get("tql")
	path := u.Path

	// Jira issue via /browse/KEY-123
	if strings.HasPrefix(path, "/browse/") {
		key := strings.TrimPrefix(path, "/browse/")
		key = strings.SplitN(key, "/", 2)[0]
		if issueKeyRe.MatchString(key) {
			t.Kind, t.ID = Issue, key
			return t
		}
	}
	// Jira issue via ?selectedIssue=KEY-123
	if sel := q.Get("selectedIssue"); issueKeyRe.MatchString(sel) {
		t.Kind, t.ID = Issue, sel
		return t
	}
	// Jira board via /.../boards/123  or classic ?rapidView=123
	if m := boardsSegRe.FindStringSubmatch(path); m != nil {
		t.Kind, t.ID = Board, m[1]
		return t
	}
	if rv := q.Get("rapidView"); digitsRe.MatchString(rv) {
		t.Kind, t.ID = Board, rv
		return t
	}
	// Confluence page via /wiki/spaces/SPACE/pages/12345/Title
	if m := pagesSegRe.FindStringSubmatch(path); m != nil {
		t.Kind, t.ID = Page, m[1]
		return t
	}
	if pid := q.Get("pageId"); digitsRe.MatchString(pid) {
		t.Kind, t.ID = Page, pid
		return t
	}
	// Goal detail via /goal/KEY (home.atlassian.com or a site)
	if m := goalSegRe.FindStringSubmatch(path); m != nil {
		t.Kind, t.ID = Goal, m[1]
		return t
	}
	// Goals list view via /goals
	if strings.Contains(path, "/goals") {
		t.Kind = GoalList
		return t
	}
	return t // Unknown kind, but Site/CloudID may still be useful
}

func parseBare(s string, hint Kind) Target {
	// The calling domain sets hint, so we trust it. The only in-domain
	// ambiguity is Jira (hint Board), where a bare token may be an issue key
	// (ABC-1) or a numeric board id.
	switch hint {
	case Board:
		if issueKeyRe.MatchString(s) {
			return Target{Kind: Issue, ID: s}
		}
		if digitsRe.MatchString(s) {
			return Target{Kind: Board, ID: s}
		}
		return Target{Kind: Unknown, ID: s}
	default:
		return Target{Kind: hint, ID: s}
	}
}
