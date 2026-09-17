// Package cmd parses the command line (domain / command / location / flags) and
// routes to the read-only service for each domain. Auth and HTTP live in their
// own packages; this file only wires arguments to calls and picks the output.
package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cgast/magpie/internal/config"
	"github.com/cgast/magpie/internal/confluence"
	"github.com/cgast/magpie/internal/goals"
	"github.com/cgast/magpie/internal/httpclient"
	"github.com/cgast/magpie/internal/jira"
	"github.com/cgast/magpie/internal/output"
	"github.com/cgast/magpie/internal/urlparse"
)

// Version is set at build time via -ldflags "-X .../cmd.Version=...".
var Version = "dev"

const usageText = `magpie — read-only CLI for Jira, Confluence, and Atlassian Goals

Usage:
  magpie <domain> <command> <location> [flags]

Jira:
  magpie jira read   <issue-url | KEY-123 | board-url | board-id> [--markdown] [--limit N]
  magpie jira search "<JQL>" [--limit N] [--markdown]

Confluence:
  magpie confluence read   <page-url | page-id> [--depth N] [--markdown]
  magpie confluence search "<CQL>" [--limit N] [--markdown]

Goals:
  magpie goals read <goal-url | goal-key> [--markdown]
  magpie goals list [--tql "<TQL>"] [--limit N] [--markdown]

Global:
  --markdown   human-readable output (default is JSON)
  version      print version

Auth (environment):
  MAGPIE_EMAIL      Atlassian account email     (or ATLASSIAN_EMAIL)
  MAGPIE_TOKEN      Atlassian API token         (or ATLASSIAN_API_TOKEN)
  MAGPIE_SITE       default site host           e.g. example.atlassian.net
  MAGPIE_CLOUD_ID   tenant cloud id (Goals)     optional; auto-resolved
`

// Run is the entry point. It returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
	switch args[0] {
	case "version", "-v", "--version":
		fmt.Println("magpie " + Version)
		return 0
	case "help", "-h", "--help":
		fmt.Print(usageText)
		return 0
	case "jira":
		return runJira(args[1:])
	case "confluence", "conf":
		return runConfluence(args[1:])
	case "goals", "goal":
		return runGoals(args[1:])
	default:
		errln("unknown domain %q", args[0])
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
}

func services() (config.Config, *httpclient.Client, bool) {
	cfg, err := config.Load()
	if err != nil {
		errln("%v", err)
		return cfg, nil, false
	}
	return cfg, httpclient.New(cfg), true
}

// ---------- Jira ----------

func runJira(args []string) int {
	if len(args) == 0 {
		errln("jira: expected 'read' or 'search'")
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "read":
		fs := newFlags("jira read")
		md := fs.Bool("markdown", false, "render as Markdown")
		limit := fs.Int("limit", 100, "max issues for a board")
		if !parse(fs, rest) {
			return 2
		}
		loc := fs.Arg(0)
		if loc == "" {
			errln("jira read: need an issue key/URL or a board id/URL")
			return 2
		}
		cfg, client, ok := services()
		if !ok {
			return 1
		}
		t := urlparse.Parse(loc, urlparse.Board) // bare key -> issue, bare number -> board
		site, err := cfg.ResolveSite(t.Site)
		if err != nil {
			errln("%v", err)
			return 1
		}
		svc := jira.Service{Client: client, Site: site}
		c, cancel := ctx()
		defer cancel()
		switch t.Kind {
		case urlparse.Issue:
			issue, err := svc.ReadIssue(c, t.ID)
			return emit(issue, *md, err)
		case urlparse.Board:
			board, err := svc.ReadBoard(c, t.ID, *limit)
			return emit(board, *md, err)
		default:
			errln("jira read: could not tell if %q is an issue or a board", loc)
			return 2
		}
	case "search":
		fs := newFlags("jira search")
		md := fs.Bool("markdown", false, "render as Markdown")
		limit := fs.Int("limit", 50, "max results")
		if !parse(fs, rest) {
			return 2
		}
		jql := strings.TrimSpace(strings.Join(fs.Args(), " "))
		if jql == "" {
			errln("jira search: need a JQL string (quote it)")
			return 2
		}
		cfg, client, ok := services()
		if !ok {
			return 1
		}
		site, err := cfg.ResolveSite("")
		if err != nil {
			errln("%v", err)
			return 1
		}
		svc := jira.Service{Client: client, Site: site}
		c, cancel := ctx()
		defer cancel()
		res, err := svc.Search(c, jql, *limit)
		return emit(res, *md, err)
	default:
		errln("jira: unknown command %q (want 'read' or 'search')", cmd)
		return 2
	}
}

// ---------- Confluence ----------

func runConfluence(args []string) int {
	if len(args) == 0 {
		errln("confluence: expected 'read' or 'search'")
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "read":
		fs := newFlags("confluence read")
		md := fs.Bool("markdown", false, "render as Markdown")
		depth := fs.Int("depth", 0, "levels of child pages to include")
		if !parse(fs, rest) {
			return 2
		}
		loc := fs.Arg(0)
		if loc == "" {
			errln("confluence read: need a page URL or id")
			return 2
		}
		cfg, client, ok := services()
		if !ok {
			return 1
		}
		t := urlparse.Parse(loc, urlparse.Page)
		if t.Kind != urlparse.Page {
			errln("confluence read: %q is not a Confluence page URL or id", loc)
			return 2
		}
		site, err := cfg.ResolveSite(t.Site)
		if err != nil {
			errln("%v", err)
			return 1
		}
		svc := confluence.Service{Client: client, Site: site}
		c, cancel := ctx()
		defer cancel()
		page, err := svc.ReadPage(c, t.ID, *depth)
		return emit(page, *md, err)
	case "search":
		fs := newFlags("confluence search")
		md := fs.Bool("markdown", false, "render as Markdown")
		limit := fs.Int("limit", 25, "max results")
		if !parse(fs, rest) {
			return 2
		}
		cql := strings.TrimSpace(strings.Join(fs.Args(), " "))
		if cql == "" {
			errln("confluence search: need a CQL string (quote it)")
			return 2
		}
		cfg, client, ok := services()
		if !ok {
			return 1
		}
		site, err := cfg.ResolveSite("")
		if err != nil {
			errln("%v", err)
			return 1
		}
		svc := confluence.Service{Client: client, Site: site}
		c, cancel := ctx()
		defer cancel()
		res, err := svc.Search(c, cql, *limit)
		return emit(res, *md, err)
	default:
		errln("confluence: unknown command %q (want 'read' or 'search')", cmd)
		return 2
	}
}

// ---------- Goals ----------

func runGoals(args []string) int {
	if len(args) == 0 {
		errln("goals: expected 'read' or 'list'")
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "read":
		fs := newFlags("goals read")
		md := fs.Bool("markdown", false, "render as Markdown")
		if !parse(fs, rest) {
			return 2
		}
		loc := fs.Arg(0)
		if loc == "" {
			errln("goals read: need a goal URL or key")
			return 2
		}
		cfg, client, ok := services()
		if !ok {
			return 1
		}
		t := urlparse.Parse(loc, urlparse.Goal)
		site, err := goalGateway(cfg, t)
		if err != nil {
			errln("%v", err)
			return 1
		}
		cloud := firstNonEmpty(t.CloudID, cfg.CloudID)
		svc := goals.Service{Client: client, Site: site, CloudID: cloud}
		c, cancel := ctx()
		defer cancel()
		g, err := svc.Read(c, t.ID)
		return emit(g, *md, err)
	case "list":
		fs := newFlags("goals list")
		md := fs.Bool("markdown", false, "render as Markdown")
		limit := fs.Int("limit", 50, "max goals")
		tql := fs.String("tql", "", "TQL filter (default: (archived = false))")
		if !parse(fs, rest) {
			return 2
		}
		cfg, client, ok := services()
		if !ok {
			return 1
		}
		site, err := goalGateway(cfg, urlparse.Target{})
		if err != nil {
			errln("%v", err)
			return 1
		}
		svc := goals.Service{Client: client, Site: site, CloudID: cfg.CloudID}
		c, cancel := ctx()
		defer cancel()
		res, err := svc.List(c, *tql, *limit)
		return emit(res, *md, err)
	default:
		errln("goals: unknown command %q (want 'read' or 'list')", cmd)
		return 2
	}
}

// goalGateway picks the atlassian.net host that serves the GraphQL gateway.
// Goal URLs are on home.atlassian.com, which is NOT the gateway, so we prefer
// the configured tenant site and only fall back to a parsed *.atlassian.net host.
func goalGateway(cfg config.Config, t urlparse.Target) (string, error) {
	if cfg.Site != "" {
		return cfg.Site, nil
	}
	if strings.HasSuffix(t.Site, "atlassian.net") {
		return t.Site, nil
	}
	return "", fmt.Errorf("goals: set MAGPIE_SITE to your tenant, e.g. example.atlassian.net")
}

// ---------- helpers ----------

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func parse(fs *flag.FlagSet, args []string) bool {
	return fs.Parse(args) == nil
}

func emit(v output.Renderable, markdown bool, err error) int {
	if err != nil {
		errln("%v", err)
		return 1
	}
	if err := output.Emit(os.Stdout, v, markdown); err != nil {
		errln("output: %v", err)
		return 1
	}
	return 0
}

func errln(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "magpie: "+format+"\n", a...)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
