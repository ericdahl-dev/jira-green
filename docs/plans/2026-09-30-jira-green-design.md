# jira-green design

Date: 2026-09-30
Status: validated, pre-implementation

A terminal dashboard for live Jira ticket flow health, in the same family as `aws-green`,
`git-green`, and `coolify-green`. Single Go binary, TOML config, `init` wizard, stoplight
rollups, auto-polling, shipped through the `ericdahl-dev/tap` Homebrew cask.

## Goals

- See at a glance which of my tickets are moving and which are stuck.
- See tickets I am waiting on that sit with someone else.
- Move a ticket to its next status without opening a browser.

## Non-goals (v1)

- Creating tickets or editing fields
- Comment / nudge actions
- Cross-linking with git-green CI status (natural v2: Jira keys already appear in branch names)
- Whole-team sprint views

## Data source and auth

- Jira Cloud REST API, called directly (no shelling out to go-jira-cli).
- Auth: email + API token. The token never lives in the config file; the config names an env
  var (`token_env`) or a command (`token_command`), matching coolify-green. A literal `token =`
  key is rejected as unknown. Resolution order: `token_command` (run with a 10s timeout), then
  `token_env` (an error if the variable is unset or empty), then `$JIRA_API_TOKEN` only when
  neither is configured. A configured source that fails is an error, never a fall-through.
- No site hostnames, board IDs, or project keys are committed to the repo. They live only in the
  user's local config.

## Columns

Columns are discovered, not configured. `init` asks for the board, then reads
`/rest/agile/1.0/board/{id}/configuration` for column order and status-to-column mapping.
Board config is refreshed every 10 minutes. The Done column is hidden by default; `d` toggles
"done this sprint".

## Swimlanes

Two JQL queries, overridable in config:

- **Mine**: `assignee = currentUser() AND sprint IN openSprints() AND statusCategory != Done`
- **Waiting on others**: `(reporter = currentUser() OR watcher = currentUser()) AND assignee != currentUser() AND statusCategory != Done`

## Flow health (card stoplight)

| Light | Rule |
|---|---|
| 🔴 | Flagged / impediment, a `blocked` label, or time in current status over the column's red threshold |
| 🟡 | Time in status over the yellow threshold, or an unanswered comment mentioning me |
| 🟢 | Moving |
| ⚪ | Stale: last poll failed, showing last-known state |

- Thresholds are per column. Defaults: In Progress 3d yellow / 5d red; Code Review and UA
  1d yellow / 2d red. Configurable.
- Time in status comes from the changelog (`expand=changelog`), fetched only for issues whose
  `updated` timestamp changed, and cached.
- Swimlane headers and epic rows roll up to the worst card. Red cards sort to the top of a column.

## Views

`v` toggles between two views over the same state. The last-used view is remembered.

**Kanban** (default): columns = board statuses, swimlanes = Mine / Waiting on others. A card
shows stoplight, key, truncated summary, age in status, and assignee in the Waiting lane.

```
  To Do        In Progress   Code Review   UA
─ Mine ──────────────────────────────────────────
 🟢 ABC-2011   🟡 ABC-1974   🔴 ABC-1836
  Add alt..    Fix auth..   Solr pag..
  1d           4d           6d ⚑
─ Waiting on others ─────────────────────────────
              🟡 ABC-1990                 🟢 ABC-1950
               @jsmith                    @qa
               3d                         1d
```

**List**: a collapsible tree grouped by epic, like coolify-green, with full summaries. Better on
narrow terminals.

```
▼ 🔴 Discovery epic          Mine 2  Waiting 1
    🔴 ABC-1836  Code Review  6d ⚑  Solr pagination breaks on..
    🟡 ABC-1974  In Progress  4d    Fix auth redirect loop
    🟡 ABC-1990  Code Review  3d    @jsmith
▶ 🟢 Accessibility           Mine 1
```

## Keys

| Key | Action |
|---|---|
| arrows | Move between cards / columns / rows |
| enter | Detail pane: full summary, epic, last 3 comments, linked PRs if the dev panel has them |
| t | Transition picker: only transitions Jira allows from the current status, with confirmation |
| o | Open ticket in browser |
| v | Toggle kanban / list |
| d | Show / hide Done |
| r | Refresh now |
| m | Manage: mute tickets or epics, written to config |
| q | Quit |
| ? | Help |

## Stuck alerts

Carried over from coolify-green: POST a signed JSON event to configured webhooks when a card
stays red past a threshold, once per incident.

## Architecture

```
main.go              CLI entry: launch, init, --version, --help
internal/config      TOML load/save, init wizard, token env/cmd resolution
internal/jira        Thin REST client behind an interface: search, board config,
                     changelog, transitions
internal/model       Pure health logic: issues + changelog + columns + thresholds -> cards
internal/ui          Bubble Tea app: shared state, kanban.go, list.go, detail pane,
                     transition picker, manage screen
internal/alert       Webhook signing and dedupe
```

`internal/model` performs no I/O, so the health rules are fully unit-testable.

## Polling

- Swimlane JQL every 60s by default (Jira rate limits are tighter than Coolify's).
- Changelog only for issues whose `updated` changed.
- Board configuration every 10 minutes.
- `r` for a manual refresh.

## Error handling

- Poll failure: keep last-known cards, mark them ⚪ stale, show a one-line error bar.
- 401: stop polling (a 403 is usually per-issue permission: show the error, keep polling) and show "token rejected - check token_command" instead of retrying.
- 429: back off according to `Retry-After`.
- Failed transition: show Jira's error text in the picker; the card stays where it was.

## Testing

- Table-driven tests for every health rule in `internal/model`.
- `httptest` fixtures for `internal/jira`, recorded from real responses and scrubbed of employer data.
- Golden-file snapshots of both views at 80 and 160 columns.

## Release

goreleaser -> `ericdahl-dev/tap` cask, the same way as coolify-green:
`brew install --cask ericdahl-dev/tap/jira-green`.

## Example config

```toml
[settings]
  poll_interval_seconds = 60
  board_refresh_interval_seconds = 600
  default_view = "kanban"

[jira]
  site = "https://example.atlassian.net"
  email = "me@example.com"
  token_command = "security find-generic-password -s jira-green -w"
  board_id = 123

[thresholds."In Progress"]
  yellow = "3d"
  red = "5d"

[thresholds."Code Review"]
  yellow = "1d"
  red = "2d"
```
