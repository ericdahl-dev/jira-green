# jira-green

### get your tickets green

A terminal dashboard for Jira ticket flow health. It shows which of your tickets are moving,
which are stuck, and which sit with someone else, without leaving the terminal.

jira-green is part of a family of terminal dashboards: [aws-green](https://github.com/ericdahl-dev/aws-green),
[git-green](https://github.com/ericdahl-dev/git-green), and
[coolify-green](https://github.com/ericdahl-dev/coolify-green).

```
 To Do              In Progress        Code Review        UA
─ 🔴 Mine (4) ──────────────────────────────────────────────────────────────────
 🟢 ABC-2011        🟡 ABC-1974       >🔴 ABC-1836
  Add alt text t..   Fix auth redir..   Solr paginatio..
  1d                 4d                 6d ⚑
 🟢 ABC-2020
  Update footer ..
  0m
─ 🟡 Waiting on others (2) ─────────────────────────────────────────────────────
                    🟡 ABC-1990                           🟢 ABC-1950
                     Harden session..                      Verify catalog..
                     @jane 3d                              @qa 1d
─ 🟡 ▶ Backlog (2) ───────────────────────────────────────────────── b to expand
```

Press `v` for the list view, grouped by epic:

```
 ▼ 🔴 Search                       Mine 1  0/3 done
>    🔴 ABC-1836  Code Review  6d ⚑  Solr pagination breaks on page 11
 ▼ 🟡 Auth                         Mine 1  Waiting 1  4/7 done
     🟡 ABC-1974  In Progress  4d    Fix auth redirect loop
     🟡 ABC-1990  In Progress  3d    Harden session cookie @jane
 ▼ 🟢 Accessibility                Mine 2
     🟢 ABC-2030  In Progress  2d    Announce the result count and active filt..
     🟢 ABC-2011  To Do        1d    Add alt text to search results
 ▶ 🟢 No epic                      Mine 1  Waiting 1
 ▶ 🟡 Backlog (2 hidden) - b to show
```

## Features

- **Columns come from your board** - `init` asks which board to use, and the column order and
  status mapping are read from its configuration. There is no column list to keep in sync.
- **Four lanes** - Mine, Waiting on others, Backlog, and Done this sprint, each a JQL query you
  can override
- **Stoplight per card** - 🔴 🟡 ⚪ 🟢 from flags, blocked labels, time in status, and unanswered
  mentions. A lane or epic takes the light of its worst card.
- **Kanban and list views** - `v` switches between them, and the last one you used is
  remembered
- **Transitions from the terminal** - `t` offers only the transitions Jira allows from the
  card's current status, and asks before it moves anything
- **Epic progress** - the list view shows `N/M done` for each epic
- **Auto-polling** - every 60 seconds by default. The last good cards stay on screen, marked
  stale, when a poll fails.
- **Stuck alerts** - POSTs a signed JSON event to your webhooks when a card stays red too long,
  once per incident
- **Mute what you don't care about** - the manage screen (`m`) hides tickets or whole epics
- **Tokens stay out of the config** - read from a command or an environment variable
- **Single binary** - no runtime, no dependencies

## Install

Builds are published for macOS and Linux, on amd64 and arm64. There is no Windows build.

### Homebrew (macOS)

```bash
brew install --cask ericdahl-dev/tap/jira-green
```

The binaries are neither signed nor notarized. The cask removes the quarantine attribute after
install so Gatekeeper lets it run.

### Go

Requires Go 1.25 or later.

```bash
go install github.com/ericdahl-dev/jira-green@latest
```

### Release tarballs

Each [GitHub release](https://github.com/ericdahl-dev/jira-green/releases) has a
`jira-green_<version>_<os>_<arch>.tar.gz` for `darwin` and `linux` on `amd64` and `arm64`, plus
`checksums.txt`. Unpack it and put `jira-green` on your `PATH`. On macOS, a downloaded binary may
be quarantined; clear it with:

```bash
xattr -d com.apple.quarantine ./jira-green
```

## Usage

```
jira-green — terminal dashboard for Jira ticket flow health

Usage:
  jira-green            launch the dashboard
  jira-green init       create a config (--force replaces one)
  jira-green --version  print the version
  jira-green --help     show this help
```

## First-time config

### 1. Create an API token

Sign in at [id.atlassian.com](https://id.atlassian.com), open **Security → API tokens**, and
create a token. jira-green authenticates with your Atlassian email and this token.

### 2. Run the wizard

```bash
jira-green init
```

The wizard asks for:

1. your Jira site (`https://example.atlassian.net`) and email
2. where the token comes from: a command, such as the macOS Keychain, or an environment
   variable. It checks the token against Jira and asks again until it works.
3. which board to show, from the boards your account can see

It then looks up your site's Flagged field and writes the config. The token itself is never
written.

The config lives at `$XDG_CONFIG_HOME/jira-green/config.toml`, or
`~/.config/jira-green/config.toml` when `XDG_CONFIG_HOME` is unset. `init` refuses to overwrite
an existing file. `init --force` replaces it, atomically and only as the last step: if you cancel
the wizard or any check fails, the old file is left as it was.

### Where the token comes from

jira-green tries these sources in order:

```toml
[jira]
# 1. a command, run with `sh -c` and a 10-second timeout. Its output is the token.
token_command = "security find-generic-password -s jira-green -w"

# 2. an environment variable you name
token_env = "MY_JIRA_TOKEN"
```

3. With neither key set, `$JIRA_API_TOKEN` is read from the environment. If you already use
   go-jira-cli, you have this exported and jira-green reuses it with no token config at all.

A source you configure has to work. If `token_command` fails, times out, or prints nothing, or
the `token_env` variable is unset, jira-green stops with an error. It never falls through to the
next source. There is no `token = "..."` key: the config file never holds the token, and a
`token` key is rejected as unknown.

The token is read once, at startup. After you rotate it, restart jira-green.

To keep the token in the macOS Keychain:

```bash
security add-generic-password -a "$USER" -s jira-green -w
# paste the token at the prompt
```

then set `token_command = "security find-generic-password -s jira-green -w"`.

## Config reference

Every key below is optional unless marked required. The values shown are the defaults; leave a
key out to get its default. An unknown key is an error, so a typo stops the app instead of being
ignored.

```toml
blocked_labels = ["blocked"]
muted          = ["ABC-7", "ABC-1502"]  # issue or epic keys; written by the manage screen

[settings]
poll_interval_seconds          = 60        # how often the lanes are polled
board_refresh_interval_seconds = 600       # how often the board's columns, epic lookups, and epic progress are refreshed
stuck_alert_after              = "2h"      # how long a card stays red before webhooks fire
default_view                   = "kanban"  # "kanban" or "list"

[jira]
site          = "https://example.atlassian.net"  # required; must be https
email         = "you@example.com"                # required; the account the token belongs to
board_id      = 42                               # required; picked by init
token_command = "security find-generic-password -s jira-green -w"
# token_env   = "MY_JIRA_TOKEN"
flagged_field = "customfield_10021"              # your site's Flagged field; found by init

[jql]
mine    = 'assignee = currentUser() AND sprint IN openSprints() AND statusCategory != Done'
waiting = '(reporter = currentUser() OR watcher = currentUser()) AND (assignee != currentUser() OR assignee IS EMPTY) AND statusCategory != Done AND updated >= -90d'
backlog = 'assignee = currentUser() AND statusCategory != Done AND (sprint IS EMPTY OR sprint NOT IN openSprints())'
done    = 'assignee = currentUser() AND sprint IN openSprints() AND statusCategory = Done'

[thresholds."In Progress"]
yellow = "3d"
red    = "5d"

[thresholds."Code Review"]
yellow = "3d"
red    = "7d"

[thresholds."UA"]
yellow = "3d"
red    = "7d"

[[webhooks]]
url    = "https://hooks.example.com/jira-green"  # http or https
secret = "shared-secret"                         # optional; signs each request
```

`blocked_labels` and `muted` are top-level keys, so they must come before the first `[table]`
header. Anywhere later, TOML reads them as part of that table, and they are rejected as unknown.

### `[settings]`

The two interval keys must be zero or positive; `0` means the default. `stuck_alert_after` is a
duration (see below). `default_view` is used until you press `v`: from then on, the last view is
remembered in `state.toml`, next to `config.toml`. The app writes `state.toml` itself, and a
missing or unreadable one falls back to `default_view`.

### `[jira]`

`site`, `email`, and `board_id` are required. `site` must be an `https://` URL; a trailing slash
is dropped. `init` writes exactly one of `token_command` and `token_env`; if you set both by
hand, `token_command` wins (see [Where the token comes from](#where-the-token-comes-from)).

### `[jql]`

Each key replaces one lane's query. A key you leave out keeps its default, shown above. The
board filter is still ANDed onto `mine`, `backlog`, and `done` overrides (see [Lanes](#lanes)),
so an override can narrow a lane within the board but cannot widen it past the board.

### `[thresholds."Column"]`

Time in the current status that turns a card yellow and red, keyed by board column name.
Durations accept whole days followed by Go duration units: `"3d"`, `"1d6h"`, `"12h"`, `"90m"`.
`yellow` must be less than `red`, and a zero duration is an error.

Defaults apply per column. A table you write replaces that one column's default and leaves the
others alone. Either level can be left out. A table with neither disables that column:

```toml
[thresholds."UA"]   # no yellow, no red: age in UA never colors a card
```

### `blocked_labels`

Labels that turn a card red, matched case-insensitively. A card whose board column has one of
these names is red too, so a "Blocked" column works with the default. Leave the key out for
`["blocked"]`. Set `blocked_labels = []` for none: that turns off both the label and the column
match.

### `muted`

Issue or epic keys to hide. Muting an epic hides its children; muting a story hides its
subtasks. You normally edit this through the manage screen (`m`).

### `[[webhooks]]`

Zero or more receivers for [stuck-ticket alerts](#stuck-ticket-webhooks). `url` is required and
must be an `http://` or `https://` URL. `secret` is optional; with it, each request is signed.

### File permissions

The config names your token command and webhook secrets, so only you should be able to read it.
If it is readable by group or others, jira-green starts anyway and prints:

```
jira-green: warning: /home/you/.config/jira-green/config.toml is readable by others (mode 0644) - run: chmod 600 /home/you/.config/jira-green/config.toml
```

Every file jira-green writes is created with mode `0600`.

## Keys

### Dashboard

| Key | Action |
|---|---|
| `←` `→` `↑` `↓` / `h` `j` `k` `l` | Move between cards |
| `enter` | Open a card's detail; in the list view, collapse or expand an epic group |
| `t` | Transition the selected card: `t`, then `enter` on a transition, then `y` to confirm |
| `o` | Open the selected card on your Jira site |
| `v` | Switch between kanban and list |
| `d` | Show or hide Done |
| `b` | Show or hide Backlog |
| `r` | Refresh now |
| `m` | Mute epics and cards |
| `?` | Toggle help |
| `esc` | Close help, detail, or the transition picker |
| `q` / `ctrl+c` | Quit |

### Manage (`m`)

| Key | Action |
|---|---|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `space` | Mute or unmute the epic or card |
| `esc` / `m` / `q` | Back to the dashboard |

The screen lists every epic with its cards, plus anything already muted so you can unmute it. A
change is saved at once, and the dashboard refreshes when you go back.

> **The manage screen rewrites `config.toml`.** Each mute or unmute re-encodes the whole file
> from what was loaded: **comments are dropped and the file is reformatted.** Your settings and
> values are kept. The write is atomic, a symlinked config stays a symlink (the file it points to
> is replaced), and the result is mode `0600`. If you keep comments in your config, keep a copy
> or manage `muted` by hand.

## Lanes

Each lane is one JQL query. Top to bottom:

| Lane | Default query finds | Shown |
|---|---|---|
| **Mine** | issues assigned to you in an open sprint | always |
| **Waiting on others** | open issues you reported or watch, updated in the last 90 days, that are assigned to someone else or to no one | always |
| **Backlog** | issues assigned to you outside the open sprints | collapsed; `b` expands it |
| **Done this sprint** | your issues in an open sprint that are done | hidden; `d` shows it |

An issue that matches several queries appears once, in the first matching lane in the order
Mine, Waiting, Backlog, Done.

**Board scoping.** jira-green reads the board's saved filter and ANDs it onto the Mine, Backlog,
and Done queries, so they show only this board's issues. Waiting stays global, because work you
wait on often lives on other boards. If the board has no usable saved filter, nothing is scoped
and the status line says `unscoped`.

**Columns.** Columns and their order come from the board. A card whose status no board column
maps goes to an **Other** column, which appears only when a card uses it. The board's last
column (Done, by convention) is hidden while Done is hidden, unless an open card sits in it.

## Stoplight

| Light | Meaning |
|---|---|
| 🔴 Red | Flagged, has a blocked label, sits in a column named like a blocked label, or has been in its column past the red threshold |
| 🟡 Yellow | Past the column's yellow threshold, or a comment mentions you and you have not replied |
| ⚪ Stale | The last poll failed, or the card's data is incomplete |
| 🟢 Green | Moving |

A lane header and an epic group take the light of their worst card. Within a cell, red cards
sort first, then the oldest.

**Stale** ranks below yellow, so it shows only when nothing is yellow or red: a network blip
never hides a problem. When a poll fails, the last good cards stay on screen and turn stale.

**Incomplete.** When jira-green cannot decode some of a card's fields, or a per-card request
such as its changelog fails, the card is marked data-incomplete. It shows ⚪ (unless it is
already red or yellow), and the detail pane lists what went wrong under **Data incomplete**,
separate from the reasons for its light.

**Age** is time in the current status, read from the changelog. The changelog is fetched again
only when an issue changes. A Backlog card in a column with no threshold (To Do, by default)
skips the changelog and uses its created date instead, since age there colors nothing.

**Epic progress.** The list view's epic headers show `N/M done`: the epic's child issues in the
Done status category, out of all of them. The counts come from Jira's approximate-count search,
so treat them as approximate. They are cached per epic for `board_refresh_interval_seconds`
(10 minutes by default). If a count fails, the header shows nothing rather than a wrong number.

**Subtasks** roll up under their story's epic (or No epic), shown as `ABC-12 › summary`, where
`ABC-12` is the story.

## Views

**Kanban** (the default) shows board columns across and lanes down. A card shows its light, key,
summary, and age; Waiting cards also name the assignee. On a terminal too narrow for the board
(12 characters per column), the dashboard shows the list instead, with a note saying so, and
returns to the kanban when the terminal is wide enough. Your chosen view is not changed.

**List** (`v`) groups cards by epic with full summaries. `enter` on an epic collapses or expands
it.

Both views keep the header pinned and scroll the rest to fit the terminal, keeping the selection
in view. The manage screen scrolls the same way.

## When things go wrong

- **A poll fails**: the last good cards stay on screen, marked stale, and the status line shows
  `⚪ stale:` with the error and the time of the last good poll. Polling continues.
- **401 Unauthorized**: polling stops, and the status line says
  `token rejected - fix the token and restart`. `r` cannot recover from this, because the token
  is only read at startup.
- **403 Forbidden**: usually one board or issue is off limits while the token still works. The
  error is shown and polling keeps going.
- **429 Too Many Requests**: the next poll waits for `Retry-After`, capped at 15 minutes, or 60
  seconds if the header is missing. Pressing `r` during that window waits it out.
- **A board refresh fails**: the cached columns stay in use and the refresh is retried on the
  next poll.
- **A transition fails**: the picker shows Jira's error and the card stays where it was.

## Stuck-ticket webhooks

A dashboard only helps when someone is looking at it. Configure one or more `[[webhooks]]` and
jira-green POSTs a JSON event when a card has stayed red for `stuck_alert_after` (default `2h`):

```json
{
  "type": "ticket.stuck",
  "key": "ABC-1836",
  "summary": "Solr pagination breaks on page 11",
  "status": "In Review",
  "url": "<the issue's link on your Jira site>",
  "reasons": ["flagged", "in Code Review 8d (red at 7d)"],
  "red_for": "2h",
  "at": "2026-09-30T14:05:00Z",
  "incomplete": true,
  "decode_errors": ["changelog: jira: HTTP 500 Internal Server Error"]
}
```

`incomplete` and `decode_errors` appear only when the card's data is incomplete, in which case
`reasons` may be missing a cause.

- **Once per incident.** A card fires one event per red spell. When it leaves red, or leaves every
  lane, it is re-armed and the next incident alerts again.
- **The clock is jira-green's.** It starts the first time jira-green sees the card red, not when
  the card turned red in Jira, and it restarts each time you launch the app.
- **Alerts pause while polls fail.** A failed poll neither starts, fires, nor resets an incident,
  so an outage is not reported as a wall of stuck tickets.
- **Failures are shown, not retried.** A delivery that fails flashes `⚠ webhook failed:` on the
  status line, naming only the webhook's host, never its path, query, or secret.

### Verifying the signature

With a `secret`, each request carries

```
X-Coolify-Green-Signature: sha256=<hex>
```

where `<hex>` is the HMAC-SHA256 of the raw request body, keyed with the secret. The header name
is coolify-green's on purpose, so one receiver can verify both tools the same way. The signature
covers the body only: there is no timestamp or nonce, so it gives no replay protection.

```python
import hashlib, hmac

def verify(body: bytes, header: str, secret: str) -> bool:
    want = "sha256=" + hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(want, header)
```

## Privacy

jira-green has no telemetry. It talks only to your Jira site and to the webhooks you configure.

## Development

```bash
go test -race ./...
```

The kanban, list, and detail views are covered by golden files in `internal/ui/testdata/`. After
an intended rendering change, regenerate them and review the diff:

```bash
go test ./internal/ui -update
```

Fixtures and golden files use `example.atlassian.net` and project `ABC`. Keep real sites, board
IDs, project keys, and names out of the repo; they belong only in your local config.

## License

MIT
