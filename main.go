// Command magpie is a read-only CLI over Jira, Confluence, and Atlassian Goals,
// designed to be driven by a shell, a cron job, or a coding agent. All behavior
// lives in internal packages; this file only hands off to the dispatcher.
package main

import (
	"os"

	"github.com/cgast/magpie/cmd"
)

func main() {
	os.Exit(cmd.Run(os.Args[1:]))
}
