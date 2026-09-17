# magpie

A small, read-only command-line tool that gathers context from **Jira**, **Confluence**, and **Atlassian Goals** into one place — built for a shell, a cron job, or a coding agent. Like the bird, it collects the useful things and brings them back. One static Go binary, no runtime dependencies, one API token for all three.

> **Unofficial.** magpie is a third-party tool and is not affiliated with, endorsed by, or sponsored by Atlassian. "Jira", "Confluence", and "Atlassian" are trademarks of Atlassian, used here only to describe what the tool works with.

```
magpie <domain> <command> <location> [flags]
```

Output is **JSON by default** (a stable contract for an agent); add `--markdown` for a human-readable version.

## Install

**Homebrew** (once the tap is set up — see [Publishing](#publishing-this-repo)):

```sh
brew install cgast/tap/magpie
```

**Go:**

```sh
go install github.com/cgast/magpie@latest
```

**From source:**

```sh
git clone https://github.com/cgast/magpie && cd magpie
make build          # -> ./bin/magpie
make macos          # -> ./bin/magpie  (universal Intel + Apple Silicon)
```

## Credentials

Need credentials for your own use of magpie (a script, a cron job, a coding agent)? Here's how to get set up without pinging anyone.

1. **Generate an API token.** Go to [id.atlassian.com/manage-profile/security/api-tokens](https://id.atlassian.com/manage-profile/security/api-tokens), sign in with your usual Atlassian account, and click **Create API token**. Give it a name that tells you where it's used (e.g. `magpie-cli`) so you can find and revoke it later. Copy the token — you won't be able to see it again.
2. **Run the wizard:**

   ```sh
   magpie auth
   ```

   It asks for your email, your site (the `*.atlassian.net` hostname you see in the address bar in Jira/Confluence), and the token from step 1 (typed hidden, not echoed to the terminal). It verifies the credentials against Atlassian before saving anything, then stores them in `~/.config/magpie/.env` (owner-only permissions — `chmod 600`). Run it again any time to rotate a token; it shows your current values as defaults so you only need to retype what's changing.

That's it — every other `magpie` command now picks the credentials up automatically. Under the hood this file is just a fallback: a real `MAGPIE_EMAIL`/`MAGPIE_TOKEN`/`MAGPIE_SITE` already set in the environment always wins over it, so cron jobs, CI, and agent sandboxes that export their own credentials are unaffected by what `magpie auth` wrote. That's also what keeps the tool predictable outside an interactive shell: no login flow to script around, just environment variables with one optional file underneath as a convenience.

**Prefer plain environment variables** (no file at all — useful for cron, CI, or an agent's env)? Skip the wizard and export them yourself instead:

```sh
export MAGPIE_EMAIL="you@example.com"
export MAGPIE_TOKEN="the-token-from-step-1"
export MAGPIE_SITE="example.atlassian.net"   # default site for search/list
# optional; auto-resolved from the site if omitted:
# export MAGPIE_CLOUD_ID="00000000-0000-0000-0000-000000000000"
```

`ATLASSIAN_EMAIL` / `ATLASSIAN_API_TOKEN` / `ATLASSIAN_SITE` work as fallback names too, so the token can be shared with other tooling.

**Either way, check it worked:**

```sh
magpie whoami
```

A successful run prints your account details back to you. A 401 means the email/token pair is wrong; double-check for stray whitespace or an expired/revoked token. A 403 on a specific command usually means your account can see the site but lacks permission for that particular issue/page/board — that's a permissions problem in Jira/Confluence, not a magpie or credentials problem.

Tokens are scoped to your account and inherit your existing Jira/Confluence/Goals permissions — magpie can't see anything you couldn't already see in the browser. Revoke a token any time from the same API tokens page without affecting anything else.

## Usage

Every command accepts either a **pasted URL** or a **bare id/key** as its `location` — the tool parses the ids out of the URL for you.

```sh
# Are my credentials working?
magpie whoami

# Jira
magpie jira read https://example.atlassian.net/browse/PROJ-123
magpie jira read PROJ-123 --markdown
magpie jira read https://example.atlassian.net/jira/software/projects/PROJ/boards/42
magpie jira search "project = PROJ AND status = 'In Progress'" --limit 25

# Confluence
magpie confluence read https://example.atlassian.net/wiki/spaces/ENG/pages/98765/Runbook
magpie confluence read 98765 --depth 2          # page + 2 levels of children
magpie confluence search "space = ENG AND title ~ 'onboarding'"

# Goals
magpie goals list --tql "(archived = false)" --markdown
magpie goals read GOAL-4
```

### Flags

| Flag            | Applies to                  | Meaning                                    |
| --------------- | --------------------------- | ------------------------------------------ |
| `--markdown`  | all                         | human-readable output instead of JSON      |
| `--limit N`   | searches, board, goals list | cap the number of results                  |
| `--depth N`   | `confluence read`         | include N levels of child pages            |
| `--tql "..."` | `goals list`              | TQL filter (default`(archived = false)`) |

## Using it from a coding agent

Point the agent at the binary and let it shell out. Because output is JSON by default, the agent can parse results directly; because it's read-only, there's nothing to guard against. A typical agent instruction:

> To read a Jira ticket, run `magpie jira read <KEY>` and parse the JSON. To find tickets, run `magpie jira search "<JQL>"`. To read a Confluence page and its children, run `magpie confluence read <url> --depth 1`.

**Using Claude Code?** [.claude/skills/magpie/](.claude/skills/magpie/) has a ready-made skill covering the same ground (commands, credential troubleshooting, what's out of scope) so you don't have to paste instructions like the above into every session. Copy that folder into `.claude/skills/magpie/` in whichever project repo you want it available — a skill has to live in the repo the agent is working in, so it doesn't do anything sitting only in this one unless you're working directly in this repo.

## Notes on Goals

Atlassian Goals has no REST API or official CLI — only a GraphQL gateway — and its schema is the least stable of the three. The two queries live in `internal/goals/goals.go` as editable constants. If a field name differs on your tenant, open the GraphiQL explorer linked from the [Goals GraphQL docs](https://developer.atlassian.com/platform/goals/goals-graphql-api/using-graphql-api/), confirm the field, and adjust the constant. The gateway host is your `*.atlassian.net` tenant (not `home.atlassian.com`), so `MAGPIE_SITE` must be set for Goals commands.

## Publishing this repo

The module path is already set to `github.com/cgast/magpie`. If you ever move it to a different owner, change the path first (it's baked into imports and build flags):

```sh
NEW=github.com/<your-user>/magpie
grep -rl 'github.com/cgast/magpie' . \
  | xargs sed -i '' -e "s#github.com/cgast/magpie#$NEW#g"   # macOS sed
go build ./...   # confirm it still compiles
```

Then create and push the repo:

```sh
git init
git add .
git commit -m "Initial commit: magpie"
git branch -M main
git remote add origin git@github.com:<your-user>/magpie.git
git push -u origin main
```

Cut the first release (binaries only):

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow (`.github/workflows/release.yml`) builds macOS + Linux binaries for Intel and Apple Silicon and attaches them to a GitHub Release. To add `brew install`, follow the 4 steps in `.goreleaser.yaml`, then uncomment the `brews:` block and the token line in the workflow.

### Distributing to macOS colleagues

- **Homebrew tap** is the smoothest path: `brew install` handles the architecture and avoids Gatekeeper prompts.
- **Handing over a raw binary** (Slack/AirDrop) works too, but macOS quarantines unsigned downloads. The recipient runs once: `xattr -dr com.apple.quarantine ./magpie`.
- **Notarization** removes that step entirely but needs an Apple Developer account; it can be added to the release pipeline later.

## Layout

```
main.go                     entrypoint
cmd/dispatch.go             arg parsing + routing
cmd/auth.go                 interactive credentials wizard (`magpie auth`)
internal/config             credentials + defaults (env vars, then ~/.config/magpie/.env)
internal/httpclient         basic auth, retries, GraphQL — every call goes here
internal/urlparse           URL/id -> typed target (shared "location" handling)
internal/adf                Atlassian Document Format -> Markdown
internal/output             JSON-default / Markdown emitter
internal/jira               read issue, read board, JQL search
internal/confluence         read page (+depth), CQL search
internal/goals              read goal, list goals (GraphQL)
.claude/skills/magpie        Claude Code skill template — copy into another repo to use
```

## License

MIT — see [LICENSE](LICENSE). Change the copyright holder to your name or org.
