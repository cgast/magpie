---
name: magpie
description: Read Jira issues/boards, Confluence pages, and Atlassian Goals via the magpie CLI. Use whenever the user references a Jira ticket/board (key like PROJ-123, or a URL), a Confluence page (URL or id), an Atlassian Goal, or asks to search/list any of these — instead of asking the user to paste the content in.
---

# magpie

A read-only CLI that reads Jira, Confluence, and Atlassian Goals and prints
JSON to stdout. Prefer it over asking the user to paste ticket/page content.

```
magpie <domain> <command> <location> [flags]
```

Every `location` accepts either a full URL pasted from the browser or a bare
id/key — no need to parse the URL yourself.

## Before first use

Check credentials work:

```sh
magpie whoami
```

- Exits 0 and prints the authenticated user → good to go.
- `command not found` → magpie isn't installed or isn't on PATH; tell the
  user, don't try to install it yourself.
- Non-zero exit with a 401 → credentials are missing or wrong; tell the user
  to run `magpie auth` (interactive) or check `MAGPIE_EMAIL`/`MAGPIE_TOKEN`.
- Non-zero exit with a 403 on a *specific* command later → the account can't
  see that issue/page/board; this is a Jira/Confluence permissions issue, not
  a magpie problem — say so rather than retrying.

## Commands

```sh
# Jira
magpie jira read <issue-url | KEY-123 | board-url | board-id> [--limit N]
magpie jira search "<JQL>" [--limit N]

# Confluence
magpie confluence read <page-url | page-id> [--depth N]
magpie confluence search "<CQL>" [--limit N]

# Goals
magpie goals read <goal-url | goal-key>
magpie goals list [--tql "<TQL>"] [--limit N]
```

- `jira read` on a bare numeric id is ambiguous between an issue and a board;
  pass the full URL when you have it to avoid guessing wrong.
- `confluence read --depth N` pulls N levels of child pages in one call —
  use this instead of issuing separate reads for a page's children.
- Quote JQL/CQL/TQL strings; they're shell arguments, not files.
- `--limit` defaults are generous (see `magpie help`) but cap them lower when
  you only need a handful of results, to keep the output small.

## Output

JSON by default — parse it directly, don't shell out with `--markdown` unless
you're printing the result for a human to read as-is. Every entity has a
stable, documented shape (see the main project README) so field names won't
surprise you across calls.

Errors go to stderr with a non-zero exit code and a one-line message that
already includes the HTTP status and Atlassian's own error text — surface
that message to the user rather than re-deriving your own explanation.

## Not in scope

magpie is read-only: there is no create/update/comment/transition command.
If the user wants to *change* something in Jira/Confluence, say that magpie
can't do it and suggest the Atlassian web UI (or an API call, if the user has
their own tooling for that).
