// magpie auth is an interactive wizard that prompts for credentials, verifies
// them against the myself endpoint, and stores them in a fallback file so a
// colleague doesn't need to hand-edit shell profiles. Real environment
// variables still take priority (see internal/config), so cron/CI/agents are
// unaffected by whatever is stored here.
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/cgast/magpie/internal/config"
	"github.com/cgast/magpie/internal/httpclient"
	"github.com/cgast/magpie/internal/jira"
)

func runAuth(args []string) int {
	if len(args) > 0 {
		errln("auth: takes no arguments")
		return 2
	}

	current, _ := config.Load() // ignore error: fields we need may still be usable as defaults

	fmt.Println("magpie auth — set up credentials for Jira, Confluence, and Goals")
	fmt.Println("Create a token first at https://id.atlassian.com/manage-profile/security/api-tokens")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	email, err := promptLine(reader, "Atlassian email", current.Email)
	if err != nil {
		errln("%v", err)
		return 1
	}
	site, err := promptLine(reader, "Site (e.g. example.atlassian.net)", current.Site)
	if err != nil {
		errln("%v", err)
		return 1
	}
	token, err := promptSecret(reader, "API token", current.Token)
	if err != nil {
		errln("%v", err)
		return 1
	}

	if email == "" || site == "" || token == "" {
		errln("auth: email, site, and token are all required")
		return 1
	}

	cfg := config.Config{Email: email, Token: token, Site: config.NormalizeHost(site)}

	fmt.Print("Checking credentials... ")
	svc := jira.Service{Client: httpclient.New(cfg), Site: cfg.Site}
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	me, err := svc.Whoami(c)
	if err != nil {
		fmt.Println("failed")
		errln("%v", err)
		return 1
	}
	fmt.Printf("ok, signed in as %s\n", me.DisplayName)

	path, err := config.Save(cfg)
	if err != nil {
		errln("could not save credentials: %v", err)
		return 1
	}
	fmt.Printf("Saved to %s (owner-only permissions).\n", path)
	fmt.Println("Run `magpie whoami` any time to re-check.")
	return 0
}

// promptLine reads one line of visible input, showing def as the default when
// the user presses enter without typing anything.
func promptLine(r *bufio.Reader, label, def string) (string, error) {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

// promptSecret reads one line of hidden input (no terminal echo). Falls back
// to r (the same reader used for the other prompts) when stdin isn't a
// terminal — e.g. piped input in a test — since term.ReadPassword requires a
// real terminal and a fresh reader on the same fd would lose data r already
// buffered.
func promptSecret(r *bufio.Reader, label, def string) (string, error) {
	hint := ""
	if def != "" {
		hint = " [keep existing]"
	}
	fmt.Printf("%s%s: ", label, hint)

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return def, nil
		}
		return line, nil
	}

	b, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(b))
	if line == "" {
		return def, nil
	}
	return line, nil
}
