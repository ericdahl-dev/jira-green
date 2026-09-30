# jira-green glossary

Terms used in the code, the docs, and the UI. The code is authoritative.

- **Lane**: a horizontal band of the dashboard, set by which JQL query found the issue. Four
  lanes, in order: **Mine** (assigned to me in an open sprint), **Waiting** (I reported or
  watch it, someone else has it), **Backlog** (assigned to me, not in an open sprint), and
  **Done** (done this sprint). An issue found by several queries lands in the first, in that
  order. Each query can be overridden under `[jql]`. Compare lanes by name, never by number.
- **Column**: a board column from the Jira board's configuration. Each maps a set of status IDs.
- **Other**: the synthetic column for a status no board column maps. It appears only when a
  card uses it.
- **Card**: one issue placed on the dashboard: its lane, column, stoplight, age, and reasons.
- **Stoplight**: a card's health. Red (flagged, a blocked label, or over the column's red
  threshold), Yellow (over the yellow threshold, or an unanswered mention), Stale, or Green.
  A lane's light is its worst card.
- **Stale**: the data may be out of date or partial: the last poll failed, or the card is
  Incomplete. It ranks below Yellow, so it shows only when nothing is yellow or red.
- **Incomplete**: a card with `DecodeErrors`, fields the jira package could not decode. It is
  Stale, and the errors are listed on their own, not as reasons. Stuck-ticket webhooks carry
  `incomplete` and `decode_errors`.
- **Threshold**: per-column yellow and red durations for time in the current status.
- **Muted**: issue or epic keys in `muted`; those issues, and the children of muted epics, are
  dropped from the board. Managed from the manage screen.
- **Epic progress**: an epic's Done of Total child issues, counted by the poller and shown in
  the list view's epic headers. Missing when the count fails, never 0/0.
- **Board scope**: the board's saved filter, ANDed onto the Mine, Backlog, and Done queries so
  they show only this board's issues. Waiting stays global. With no filter the status line
  says `unscoped`.
- **Stuck**: a card red for `stuck_alert_after` (default 2h), which fires one webhook event
  per incident. The clock starts at the app's first observation of the card as red, not when
  it turned red in Jira, and restarts with each launch. The webhook's HMAC signature covers
  the body only: there is no replay protection.
