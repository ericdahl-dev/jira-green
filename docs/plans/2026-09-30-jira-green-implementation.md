# jira-green Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build `jira-green`, a single-binary terminal dashboard that shows Jira ticket flow health
as a kanban board (with a list toggle), lets you transition tickets, and alerts on stuck work.

**Architecture:** A thin Jira REST client (`internal/jira`) feeds a poller (`internal/poller`) that
converts API issues into pure domain values and runs them through a side-effect-free health model
(`internal/model`). The Bubble Tea UI (`internal/ui`) renders the model's `Board` as kanban or as
an epic tree. Layout, conventions, config shape, and release pipeline copy the sibling
`ericdahl-dev/coolify-green` repo. Read its code when a task says "port from coolify-green".

**Tech Stack:** Go 1.25+, Bubble Tea / Bubbles / Lipgloss / Huh (charmbracelet), BurntSushi/toml,
goreleaser v2, golangci-lint v2. Jira Cloud REST v3 + Agile v1.0.

**Design doc:** `docs/plans/2026-09-30-jira-green-design.md`. Read it first.

**Reference repo:** `gh repo clone ericdahl-dev/coolify-green ../coolify-green -- --depth 1`

---

## Ground rules

- TDD: every task writes the failing test first, runs it, then implements.
- Run `go test ./...` and `go vet ./...` before each commit. The final gate also runs
  `golangci-lint run`.
- Conventional commits (`feat:`, `fix:`, `test:`, `chore:`, `docs:`).
- **Never commit employer data.** No real Jira hostnames, board IDs, project keys, account IDs,
  or names in code, fixtures, or golden files. Fixtures use `example.atlassian.net`, project
  `ABC`, and account IDs like `acct-me` / `acct-jsmith`.
- Jira timestamps are **not** RFC 3339. They look like `2026-09-30T10:00:00.000-0400`. Always
  parse them with `jira.Time` (Task 8).

---

### Task 0: Scaffold the repo

**Files:**
- Create: `go.mod`, `main.go`, `main_test.go`, `.gitignore`, `.golangci.yml`, `.goreleaser.yaml`,
  `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `CLAUDE.md`, `AGENTS.md`

**Step 1: Init module and copy tooling**

```bash
go mod init github.com/ericdahl-dev/jira-green
cp -f ../coolify-green/.golangci.yml ../coolify-green/.gitignore .
mkdir -p .github/workflows
cp -f ../coolify-green/.github/workflows/ci.yml ../coolify-green/.github/workflows/release.yml .github/workflows/
cp -f ../coolify-green/.goreleaser.yaml .
sed -i '' 's/coolify-green/jira-green/g; s/live Coolify deploy and resource health/Jira ticket flow health/' .goreleaser.yaml .github/workflows/release.yml
```

Open `.goreleaser.yaml` and check that every `coolify` reference is gone:
`grep -i coolify .goreleaser.yaml .github/workflows/*` → no output.

**Step 2: Write the failing test**

`main_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"--version"}, &out, &out)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "jira-green") {
		t.Fatalf("got %q", out.String())
	}
}

func TestRunHelp(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"--help"}, &out, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"jira-green init", "--version"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help missing %q", want)
		}
	}
}
```

**Step 3: Run it to confirm it fails**

Run: `go test ./...`
Expected: FAIL, `undefined: run`

**Step 4: Minimal implementation**

`main.go`:

```go
// Command jira-green is a terminal dashboard for Jira ticket flow health.
package main

import (
	"fmt"
	"io"
	"os"
)

var version = "dev"

const usage = `jira-green — terminal dashboard for Jira ticket flow health

Usage:
  jira-green            launch the dashboard
  jira-green init       create a starter config
  jira-green --version  print the version
  jira-green --help     show this help
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-v", "version":
			fmt.Fprintf(stdout, "jira-green %s\n", version)
			return 0
		case "--help", "-h", "help":
			fmt.Fprint(stdout, usage)
			return 0
		}
	}
	fmt.Fprintln(stderr, "dashboard not implemented yet")
	return 1
}
```

**Step 5: Run the tests to confirm they pass**

Run: `go test ./...` → PASS

**Step 6: Agent docs**

`CLAUDE.md`: copy the structure of coolify-green's CLAUDE.md, then:
- Replace the "Talking to a real Coolify instance" section with: "Never hardcode a Jira token,
  site, board ID, or project key. Real values live only in `~/.config/jira-green/config.toml`.
  Fixtures and golden files use `example.atlassian.net` and project `ABC`."
- Keep the non-interactive shell section.

`AGENTS.md`: `See CLAUDE.md.`

**Step 7: Commit**

```bash
git add -A && git commit -m "chore: scaffold jira-green"
```

---

### Task 1: Stoplight

**Files:**
- Create: `internal/model/stoplight.go`
- Test: `internal/model/stoplight_test.go`

**Step 1: Write the failing test**

```go
package model

import "testing"

func TestWorst(t *testing.T) {
	cases := []struct {
		in   []Stoplight
		want Stoplight
	}{
		{nil, Green},
		{[]Stoplight{Green, Green}, Green},
		{[]Stoplight{Green, Stale}, Stale},
		{[]Stoplight{Stale, Yellow}, Yellow},
		{[]Stoplight{Yellow, Red, Green}, Red},
	}
	for _, c := range cases {
		if got := Worst(c.in...); got != c.want {
			t.Errorf("Worst(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEmoji(t *testing.T) {
	for s, want := range map[Stoplight]string{Green: "🟢", Yellow: "🟡", Red: "🔴", Stale: "⚪"} {
		if s.Emoji() != want {
			t.Errorf("%v.Emoji() = %q", s, s.Emoji())
		}
	}
}
```

**Step 2: Run it to confirm it fails**

Run: `go test ./internal/model/` → FAIL, `undefined: Stoplight`

**Step 3: Implement**

```go
// Package model holds jira-green's pure flow-health logic. It performs no I/O.
package model

// Stoplight is a card or group health. Values are ordered by severity, so the
// worst of several is simply the maximum.
type Stoplight int

const (
	Green Stoplight = iota
	Stale
	Yellow
	Red
)

func (s Stoplight) Emoji() string {
	switch s {
	case Red:
		return "🔴"
	case Yellow:
		return "🟡"
	case Stale:
		return "⚪"
	default:
		return "🟢"
	}
}

func (s Stoplight) String() string {
	return [...]string{"green", "stale", "yellow", "red"}[s]
}

// Worst returns the most severe stoplight, or Green for none.
func Worst(ls ...Stoplight) Stoplight {
	w := Green
	for _, l := range ls {
		if l > w {
			w = l
		}
	}
	return w
}
```

**Step 4: Run the tests to confirm they pass** → `go test ./internal/model/` PASS

**Step 5: Commit** → `git add -A && git commit -m "feat(model): stoplight"`

---

### Task 2: Threshold durations

Config writes thresholds as `"3d"`, `"12h"`, or `"90m"`. `time.ParseDuration` has no `d`.

**Files:**
- Create: `internal/model/threshold.go`
- Test: `internal/model/threshold_test.go`

**Step 1: Write the failing test**

```go
package model

import (
	"testing"
	"time"
)

func TestParseAge(t *testing.T) {
	ok := map[string]time.Duration{
		"3d":   72 * time.Hour,
		"1d":   24 * time.Hour,
		"12h":  12 * time.Hour,
		"90m":  90 * time.Minute,
		"1d6h": 30 * time.Hour,
	}
	for in, want := range ok {
		got, err := ParseAge(in)
		if err != nil || got != want {
			t.Errorf("ParseAge(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "d", "3x", "-1d"} {
		if _, err := ParseAge(bad); err == nil {
			t.Errorf("ParseAge(%q) should fail", bad)
		}
	}
}

func TestFormatAge(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Minute: "30m",
		5 * time.Hour:    "5h",
		26 * time.Hour:   "1d",
		6 * 24 * time.Hour: "6d",
	}
	for in, want := range cases {
		if got := FormatAge(in); got != want {
			t.Errorf("FormatAge(%v) = %q, want %q", in, got, want)
		}
	}
}
```

**Step 2: Run it to confirm it fails** → `undefined: ParseAge`

**Step 3: Implement**

```go
package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Threshold is the age-in-status at which a card turns yellow and red.
// A zero value disables that level.
type Threshold struct {
	Yellow time.Duration
	Red    time.Duration
}

var dayPart = regexp.MustCompile(`^(\d+)d`)

// ParseAge parses a duration that may start with a whole number of days,
// e.g. "3d", "1d6h", "12h", "90m".
func ParseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	var total time.Duration
	if m := dayPart.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		total = time.Duration(n) * 24 * time.Hour
		s = s[len(m[0]):]
	}
	if s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("bad duration: %w", err)
		}
		if d < 0 {
			return 0, fmt.Errorf("negative duration %q", s)
		}
		total += d
	} else if total == 0 {
		return 0, fmt.Errorf("bad duration %q", s)
	}
	return total, nil
}

// FormatAge renders an age compactly for a card: "30m", "5h", "6d".
func FormatAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
```

Note: `"d"` fails because the regexp needs a digit, and `time.ParseDuration("d")` errors. `"-1d"`
fails in `time.ParseDuration("-1d")`. Check that both fail when the test runs.

**Step 4: Run the tests to confirm they pass** → PASS

**Step 5: Commit** → `git commit -am "feat(model): age thresholds"` (use `git add -A` first)

---

### Task 3: Domain types and time-in-status

**Files:**
- Create: `internal/model/issue.go`
- Test: `internal/model/issue_test.go`

**Step 1: Write the failing test**

```go
package model

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)

func TestStatusSinceNoTransitionsUsesCreated(t *testing.T) {
	if got := StatusSince(t0, "3", nil); !got.Equal(t0) {
		t.Fatalf("got %v", got)
	}
}

func TestStatusSinceLatestEntryIntoCurrentStatus(t *testing.T) {
	changes := []StatusChange{
		{At: t0.Add(1 * time.Hour), ToID: "3"},  // into In Progress
		{At: t0.Add(2 * time.Hour), ToID: "10"}, // into Code Review
		{At: t0.Add(5 * time.Hour), ToID: "3"},  // bounced back
		{At: t0.Add(9 * time.Hour), ToID: "10"}, // review again (current)
	}
	want := t0.Add(9 * time.Hour)
	if got := StatusSince(t0, "10", changes); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestStatusSinceIgnoresOrder(t *testing.T) {
	changes := []StatusChange{
		{At: t0.Add(9 * time.Hour), ToID: "10"},
		{At: t0.Add(2 * time.Hour), ToID: "10"},
	}
	if got := StatusSince(t0, "10", changes); !got.Equal(t0.Add(9 * time.Hour)) {
		t.Fatalf("got %v", got)
	}
}
```

**Step 2: Run it to confirm it fails** → `undefined: StatusSince`

**Step 3: Implement**

```go
package model

import "time"

// Issue is a Jira issue reduced to what the health model needs. The jira
// package builds these; model never sees API JSON.
type Issue struct {
	Key          string
	Summary      string
	URL          string
	StatusID     string
	StatusName   string
	AssigneeID   string
	AssigneeName string
	EpicKey      string
	EpicSummary  string
	Labels       []string
	Flagged      bool
	Created      time.Time
	Updated      time.Time
	// StatusSince is when the issue entered its current status. The poller
	// fills it from the changelog; zero means "not yet known".
	StatusSince time.Time
	Comments    []Comment
}

// Comment is one issue comment, with the account IDs it @-mentions.
type Comment struct {
	AuthorID string
	Created  time.Time
	Mentions []string
}

// StatusChange is one status transition from the changelog.
type StatusChange struct {
	At   time.Time
	ToID string
}

// StatusSince returns when the issue last entered currentID: the latest
// transition into it, or created when there was none.
func StatusSince(created time.Time, currentID string, changes []StatusChange) time.Time {
	since := created
	for _, c := range changes {
		if c.ToID == currentID && c.At.After(since) {
			since = c.At
		}
	}
	return since
}
```

**Step 4: Run the tests to confirm they pass** → PASS

**Step 5: Commit** → `feat(model): issue types and time in status`

---

### Task 4: Unanswered mentions

A mention is "unanswered" when someone else @-mentioned me after my own latest comment.

**Files:**
- Create: `internal/model/mention.go`
- Test: `internal/model/mention_test.go`

**Step 1: Write the failing test**

```go
package model

import (
	"testing"
	"time"
)

func TestUnansweredMention(t *testing.T) {
	me := "acct-me"
	at := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	cases := []struct {
		name string
		cs   []Comment
		want bool
	}{
		{"no comments", nil, false},
		{"mention, no reply", []Comment{{AuthorID: "acct-x", Created: at(1), Mentions: []string{me}}}, true},
		{"mention then my reply", []Comment{
			{AuthorID: "acct-x", Created: at(1), Mentions: []string{me}},
			{AuthorID: me, Created: at(2)},
		}, false},
		{"my comment then mention", []Comment{
			{AuthorID: me, Created: at(1)},
			{AuthorID: "acct-x", Created: at(2), Mentions: []string{me}},
		}, true},
		{"mentions someone else", []Comment{{AuthorID: "acct-x", Created: at(1), Mentions: []string{"acct-y"}}}, false},
		{"I mention myself", []Comment{{AuthorID: me, Created: at(1), Mentions: []string{me}}}, false},
	}
	for _, c := range cases {
		if got := UnansweredMention(me, c.cs); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
```

**Step 2: Run it to confirm it fails** → `undefined: UnansweredMention`

**Step 3: Implement**

```go
package model

import (
	"slices"
	"time"
)

// UnansweredMention reports whether someone else @-mentioned me after my
// latest comment on the issue.
func UnansweredMention(me string, comments []Comment) bool {
	if me == "" {
		return false
	}
	var lastMention, lastMine time.Time
	for _, c := range comments {
		if c.AuthorID == me {
			if c.Created.After(lastMine) {
				lastMine = c.Created
			}
			continue
		}
		if slices.Contains(c.Mentions, me) && c.Created.After(lastMention) {
			lastMention = c.Created
		}
	}
	return !lastMention.IsZero() && lastMention.After(lastMine)
}
```

**Step 4: Run the tests to confirm they pass** → PASS

**Step 5: Commit** → `feat(model): unanswered mention detection`

---

### Task 5: Evaluate a card

**Files:**
- Create: `internal/model/card.go`
- Test: `internal/model/card_test.go`

**Step 1: Write the failing test**

```go
package model

import (
	"testing"
	"time"
)

func rules() Rules {
	return Rules{
		Me:            "acct-me",
		BlockedLabels: []string{"blocked"},
		Thresholds: map[string]Threshold{
			"In Progress": {Yellow: 72 * time.Hour, Red: 120 * time.Hour},
			"Code Review": {Yellow: 24 * time.Hour, Red: 48 * time.Hour},
		},
	}
}

func TestEvaluate(t *testing.T) {
	now := t0.Add(10 * 24 * time.Hour)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	cases := []struct {
		name   string
		iss    Issue
		column string
		stale  bool
		want   Stoplight
	}{
		{"fresh in progress", Issue{StatusSince: ago(time.Hour)}, "In Progress", false, Green},
		{"aging in progress", Issue{StatusSince: ago(80 * time.Hour)}, "In Progress", false, Yellow},
		{"stuck in progress", Issue{StatusSince: ago(130 * time.Hour)}, "In Progress", false, Red},
		{"review over red", Issue{StatusSince: ago(49 * time.Hour)}, "Code Review", false, Red},
		{"no threshold column", Issue{StatusSince: ago(500 * time.Hour)}, "To Do", false, Green},
		{"flagged", Issue{Flagged: true, StatusSince: ago(time.Hour)}, "In Progress", false, Red},
		{"blocked label", Issue{Labels: []string{"Blocked"}, StatusSince: ago(time.Hour)}, "To Do", false, Red},
		{"mention", Issue{StatusSince: ago(time.Hour), Comments: []Comment{
			{AuthorID: "acct-x", Created: ago(time.Minute), Mentions: []string{"acct-me"}}}}, "To Do", false, Yellow},
		{"stale beats green", Issue{StatusSince: ago(time.Hour)}, "In Progress", true, Stale},
		{"red beats stale", Issue{Flagged: true}, "In Progress", true, Red},
		{"unknown since is green", Issue{}, "In Progress", false, Green},
	}
	for _, c := range cases {
		got := Evaluate(c.iss, c.column, LaneMine, rules(), now, c.stale)
		if got.Light != c.want {
			t.Errorf("%s: light %v want %v (reasons %v)", c.name, got.Light, c.want, got.Reasons)
		}
	}
}

func TestEvaluateReasonsAndAge(t *testing.T) {
	now := t0.Add(10 * 24 * time.Hour)
	c := Evaluate(Issue{Flagged: true, StatusSince: now.Add(-130 * time.Hour)}, "In Progress", LaneMine, rules(), now, false)
	if c.Age != 130*time.Hour {
		t.Errorf("age %v", c.Age)
	}
	if len(c.Reasons) != 2 {
		t.Errorf("want flagged + over-red reasons, got %v", c.Reasons)
	}
}
```

**Step 2: Run it to confirm it fails** → `undefined: Rules`

**Step 3: Implement**

```go
package model

import (
	"fmt"
	"strings"
	"time"
)

// Lane is a kanban swimlane.
type Lane int

const (
	LaneMine Lane = iota
	LaneWaiting
	LaneDone
)

func (l Lane) String() string {
	return [...]string{"Mine", "Waiting on others", "Done this sprint"}[l]
}

// Rules configures Evaluate.
type Rules struct {
	Me            string               // my Jira account ID
	BlockedLabels []string             // matched case-insensitively
	Thresholds    map[string]Threshold // keyed by board column name
}

// Card is an issue placed on the board with its health.
type Card struct {
	Issue
	Column  string
	Lane    Lane
	Light   Stoplight
	Age     time.Duration // time in current status; 0 when unknown
	Reasons []string      // human-readable, shown in the detail pane
}

// Evaluate computes a card's stoplight. Red: flagged, blocked label, or over
// the column's red threshold. Yellow: over yellow, or an unanswered mention.
// Stale only shows through when nothing is yellow or red.
func Evaluate(iss Issue, column string, lane Lane, r Rules, now time.Time, stale bool) Card {
	c := Card{Issue: iss, Column: column, Lane: lane, Light: Green}
	raise := func(l Stoplight, why string) {
		c.Light = Worst(c.Light, l)
		c.Reasons = append(c.Reasons, why)
	}

	if iss.Flagged {
		raise(Red, "flagged")
	}
	for _, l := range iss.Labels {
		for _, b := range r.BlockedLabels {
			if strings.EqualFold(l, b) {
				raise(Red, "label "+l)
			}
		}
	}
	if !iss.StatusSince.IsZero() {
		c.Age = now.Sub(iss.StatusSince)
		if th, ok := r.Thresholds[column]; ok {
			switch {
			case th.Red > 0 && c.Age >= th.Red:
				raise(Red, fmt.Sprintf("in %s %s (red at %s)", column, FormatAge(c.Age), FormatAge(th.Red)))
			case th.Yellow > 0 && c.Age >= th.Yellow:
				raise(Yellow, fmt.Sprintf("in %s %s (yellow at %s)", column, FormatAge(c.Age), FormatAge(th.Yellow)))
			}
		}
	}
	if UnansweredMention(r.Me, iss.Comments) {
		raise(Yellow, "unanswered mention")
	}
	if stale {
		c.Light = Worst(c.Light, Stale)
	}
	return c
}
```

**Step 4: Run the tests to confirm they pass** → PASS

**Step 5: Commit** → `feat(model): evaluate card health`

---

### Task 6: Board layout and epic grouping

Statuses not on the board (common for "Waiting on others" tickets in other projects) go into a
trailing `Other` column.

**Files:**
- Create: `internal/model/board.go`
- Test: `internal/model/board_test.go`

**Step 1: Write the failing test**

```go
package model

import (
	"reflect"
	"testing"
	"time"
)

var cols = []Column{
	{Name: "To Do", StatusIDs: []string{"1"}},
	{Name: "In Progress", StatusIDs: []string{"3"}},
	{Name: "Code Review", StatusIDs: []string{"10", "11"}},
	{Name: "Done", StatusIDs: []string{"5"}},
}

func TestColumnFor(t *testing.T) {
	if got := ColumnFor(cols, "11"); got != "Code Review" {
		t.Errorf("got %q", got)
	}
	if got := ColumnFor(cols, "999"); got != OtherColumn {
		t.Errorf("got %q", got)
	}
}

func TestLayoutSortsWorstThenOldest(t *testing.T) {
	cards := []Card{
		{Issue: Issue{Key: "ABC-1"}, Column: "In Progress", Lane: LaneMine, Light: Green, Age: 5 * time.Hour},
		{Issue: Issue{Key: "ABC-2"}, Column: "In Progress", Lane: LaneMine, Light: Red, Age: 1 * time.Hour},
		{Issue: Issue{Key: "ABC-3"}, Column: "In Progress", Lane: LaneMine, Light: Green, Age: 9 * time.Hour},
		{Issue: Issue{Key: "ABC-4"}, Column: OtherColumn, Lane: LaneWaiting, Light: Yellow},
	}
	b := Layout(cols, cards, false)

	wantCols := []string{"To Do", "In Progress", "Code Review", OtherColumn}
	if !reflect.DeepEqual(b.Columns, wantCols) {
		t.Fatalf("columns %v (Done hidden, Other added because used)", b.Columns)
	}
	var keys []string
	for _, c := range b.Cell(LaneMine, "In Progress") {
		keys = append(keys, c.Key)
	}
	if !reflect.DeepEqual(keys, []string{"ABC-2", "ABC-3", "ABC-1"}) {
		t.Errorf("order %v", keys)
	}
	if b.LaneLight(LaneMine) != Red || b.LaneLight(LaneWaiting) != Yellow {
		t.Errorf("lane lights %v %v", b.LaneLight(LaneMine), b.LaneLight(LaneWaiting))
	}
}

func TestLayoutShowDone(t *testing.T) {
	b := Layout(cols, nil, true)
	if b.Columns[len(b.Columns)-1] != "Done" {
		t.Errorf("columns %v", b.Columns)
	}
}

func TestByEpic(t *testing.T) {
	cards := []Card{
		{Issue: Issue{Key: "ABC-1", EpicKey: "ABC-100", EpicSummary: "Search"}, Light: Green},
		{Issue: Issue{Key: "ABC-2", EpicKey: "ABC-200", EpicSummary: "Auth"}, Light: Red},
		{Issue: Issue{Key: "ABC-3"}, Light: Yellow},
		{Issue: Issue{Key: "ABC-4", EpicKey: "ABC-100", EpicSummary: "Search"}, Light: Yellow},
	}
	gs := ByEpic(cards)
	var names []string
	for _, g := range gs {
		names = append(names, g.Name)
	}
	if !reflect.DeepEqual(names, []string{"Auth", "Search", NoEpic}) {
		t.Fatalf("groups %v", names)
	}
	if gs[1].Light != Yellow || len(gs[1].Cards) != 2 {
		t.Errorf("Search group %+v", gs[1])
	}
}
```

**Step 2: Run it to confirm it fails** → `undefined: Column`

**Step 3: Implement**

```go
package model

import (
	"slices"
	"sort"
)

const (
	OtherColumn = "Other"
	NoEpic      = "No epic"
)

// Column is a board column and the status IDs mapped to it.
type Column struct {
	Name      string
	StatusIDs []string
}

// ColumnFor returns the board column for a status ID, or OtherColumn.
func ColumnFor(cols []Column, statusID string) string {
	for _, c := range cols {
		if slices.Contains(c.StatusIDs, statusID) {
			return c.Name
		}
	}
	return OtherColumn
}

type cellKey struct {
	lane   Lane
	column string
}

// Board is the kanban layout: ordered columns and cards per (lane, column).
type Board struct {
	Columns []string
	cells   map[cellKey][]Card
}

func (b Board) Cell(l Lane, column string) []Card { return b.cells[cellKey{l, column}] }

// LaneLight is the worst card in a lane.
func (b Board) LaneLight(l Lane) Stoplight {
	w := Green
	for k, cs := range b.cells {
		if k.lane != l {
			continue
		}
		for _, c := range cs {
			w = Worst(w, c.Light)
		}
	}
	return w
}

// LaneCount is the number of cards in a lane.
func (b Board) LaneCount(l Lane) int {
	n := 0
	for k, cs := range b.cells {
		if k.lane == l {
			n += len(cs)
		}
	}
	return n
}

// Layout places cards into cells. The last board column is treated as Done
// and hidden unless showDone. OtherColumn is appended only when used.
func Layout(cols []Column, cards []Card, showDone bool) Board {
	b := Board{cells: map[cellKey][]Card{}}
	for i, c := range cols {
		if i == len(cols)-1 && !showDone {
			continue
		}
		b.Columns = append(b.Columns, c.Name)
	}
	usedOther := false
	for _, c := range cards {
		if c.Column == OtherColumn {
			usedOther = true
		}
		k := cellKey{c.Lane, c.Column}
		b.cells[k] = append(b.cells[k], c)
	}
	if usedOther {
		b.Columns = append(b.Columns, OtherColumn)
	}
	for k := range b.cells {
		sortCards(b.cells[k])
	}
	return b
}

func sortCards(cs []Card) {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Light != cs[j].Light {
			return cs[i].Light > cs[j].Light
		}
		return cs[i].Age > cs[j].Age
	})
}

// EpicGroup is one row group in the list view.
type EpicGroup struct {
	Key   string
	Name  string
	Light Stoplight
	Cards []Card
}

// ByEpic groups cards by epic, worst group first, then by name. Cards with
// no epic go last in NoEpic.
func ByEpic(cards []Card) []EpicGroup {
	idx := map[string]int{}
	var gs []EpicGroup
	for _, c := range cards {
		key, name := c.EpicKey, c.EpicSummary
		if key == "" {
			key, name = "", NoEpic
		}
		i, ok := idx[key]
		if !ok {
			i = len(gs)
			idx[key] = i
			gs = append(gs, EpicGroup{Key: key, Name: name})
		}
		gs[i].Cards = append(gs[i].Cards, c)
		gs[i].Light = Worst(gs[i].Light, c.Light)
	}
	for i := range gs {
		sortCards(gs[i].Cards)
	}
	sort.SliceStable(gs, func(i, j int) bool {
		if (gs[i].Key == "") != (gs[j].Key == "") {
			return gs[j].Key == ""
		}
		if gs[i].Light != gs[j].Light {
			return gs[i].Light > gs[j].Light
		}
		return gs[i].Name < gs[j].Name
	})
	return gs
}
```

**Step 4: Run the tests to confirm they pass** → PASS

**Step 5: Commit** → `feat(model): board layout and epic grouping`

---

### Task 7: Config

Port the structure from `../coolify-green/internal/config/config.go`, keeping `Load`,
`applyDefaultsAndValidate`, `Save`, `ResolveToken`, and `WriteStarter`. The schema changes to a
single Jira site.

> **Note (decided after implementation):** Threshold defaults merge per column. The defaults
> always apply, a user `[thresholds.X]` table replaces only column X, and a table with neither
> `yellow` nor `red` disables that column. `Load` also rejects unknown keys through
> `md.Undecoded()` (for example `unknown key(s): jira.token_cmd`), which catches typos and a
> top-level key such as `muted` written below the `[jira]` table.
>
> **Note (code review):** The config keeps only what the user wrote. `validate()` checks values
> and writes nothing back (except trimming a trailing slash from `jira.site`), and `Save` omits
> absent keys, so no default is ever frozen into the file. Defaults are resolved when read, so
> callers (the poller, main, the UI) must use the accessors `MineJQL()`, `WaitingJQL()`,
> `DoneJQL()`, `PollInterval()`, `BoardRefreshInterval()`, `StuckAlertAfter()`,
> `DefaultView()`, and `Rules(me)`, never the raw fields such as `c.JQL.Mine` or
> `c.Settings.PollIntervalSeconds`. `blocked_labels` defaults to `["blocked"]` only when the key
> is absent; `blocked_labels = []` disables label blocking.

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const minimal = `
[jira]
  site = "https://example.atlassian.net"
  email = "me@example.com"
  token_env = "JG_TEST_TOKEN"
  board_id = 7
`

func TestLoadDefaults(t *testing.T) {
	c, err := Load(write(t, minimal))
	if err != nil {
		t.Fatal(err)
	}
	if c.Settings.PollIntervalSeconds != 60 || c.Settings.BoardRefreshIntervalSeconds != 600 {
		t.Errorf("settings %+v", c.Settings)
	}
	if c.Settings.DefaultView != "kanban" {
		t.Errorf("view %q", c.Settings.DefaultView)
	}
	if c.JQL.Mine == "" || c.JQL.Waiting == "" || c.JQL.Done == "" {
		t.Errorf("default JQL missing: %+v", c.JQL)
	}
	r, err := c.Rules("acct-me")
	if err != nil {
		t.Fatal(err)
	}
	if r.Thresholds["In Progress"].Red != 5*24*time.Hour || r.Thresholds["Code Review"].Yellow != 24*time.Hour {
		t.Errorf("default thresholds %+v", r.Thresholds)
	}
}

func TestLoadRejects(t *testing.T) {
	for name, body := range map[string]string{
		"no site":  `[jira]` + "\n" + `board_id = 1`,
		"no board": `[jira]` + "\n" + `site = "https://example.atlassian.net"`,
		"bad view": minimal + "\n[settings]\n  default_view = \"grid\"\n",
		"bad age":  minimal + "\n[thresholds.\"UA\"]\n  yellow = \"soon\"\n",
	} {
		if _, err := Load(write(t, body)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestResolveTokenOrder(t *testing.T) {
	t.Setenv("JG_TEST_TOKEN", "from-env")
	t.Setenv("JIRA_API_TOKEN", "fallback")
	if tok, _ := ResolveToken(Jira{TokenCommand: "printf from-cmd", TokenEnv: "JG_TEST_TOKEN"}); tok != "from-cmd" {
		t.Errorf("cmd first, got %q", tok)
	}
	if tok, _ := ResolveToken(Jira{TokenEnv: "JG_TEST_TOKEN"}); tok != "from-env" {
		t.Errorf("env second, got %q", tok)
	}
	if tok, _ := ResolveToken(Jira{}); tok != "fallback" {
		t.Errorf("JIRA_API_TOKEN last, got %q", tok)
	}
	t.Setenv("JIRA_API_TOKEN", "")
	if _, err := ResolveToken(Jira{}); err == nil {
		t.Error("want error with no token source")
	}
}

func TestMuteRoundTrip(t *testing.T) {
	c, _ := Load(write(t, minimal))
	if err := c.SetMuted("ABC-9", true); err != nil {
		t.Fatal(err)
	}
	c2, _ := Load(c.Path())
	if !c2.IsMuted("ABC-9") || c2.IsMuted("ABC-1") {
		t.Errorf("mutes %v", c2.Muted)
	}
}
```

**Step 2: Run it to confirm it fails** → `undefined: Load`

**Step 3: Implement**

```go
// Package config loads and saves ~/.config/jira-green/config.toml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

type Settings struct {
	PollIntervalSeconds         int    `toml:"poll_interval_seconds"`
	BoardRefreshIntervalSeconds int    `toml:"board_refresh_interval_seconds"`
	StuckAlertAfter             string `toml:"stuck_alert_after"`
	DefaultView                 string `toml:"default_view"` // "kanban" | "list"
}

type Jira struct {
	Site         string `toml:"site"`
	Email        string `toml:"email"`
	Token        string `toml:"token,omitempty"`
	TokenEnv     string `toml:"token_env,omitempty"`
	TokenCommand string `toml:"token_command,omitempty"`
	BoardID      int    `toml:"board_id"`
	FlaggedField string `toml:"flagged_field,omitempty"` // e.g. customfield_10021; found by init
}

type JQL struct {
	Mine    string `toml:"mine"`
	Waiting string `toml:"waiting"`
	Done    string `toml:"done"`
}

type Age struct {
	Yellow string `toml:"yellow,omitempty"`
	Red    string `toml:"red,omitempty"`
}

type Webhook struct {
	URL    string `toml:"url"`
	Secret string `toml:"secret,omitempty"`
}

type Config struct {
	Settings      Settings       `toml:"settings"`
	Jira          Jira           `toml:"jira"`
	JQL           JQL            `toml:"jql"`
	Thresholds    map[string]Age `toml:"thresholds"`
	BlockedLabels []string       `toml:"blocked_labels"`
	Muted         []string       `toml:"muted"` // issue or epic keys
	Webhooks      []Webhook      `toml:"webhooks"`

	path string
}

const (
	DefaultMineJQL    = `assignee = currentUser() AND sprint IN openSprints() AND statusCategory != Done`
	DefaultWaitingJQL = `(reporter = currentUser() OR watcher = currentUser()) AND assignee != currentUser() AND statusCategory != Done`
	DefaultDoneJQL    = `assignee = currentUser() AND sprint IN openSprints() AND statusCategory = Done`
)

func defaultThresholds() map[string]Age {
	return map[string]Age{
		"In Progress": {Yellow: "3d", Red: "5d"},
		"Code Review": {Yellow: "1d", Red: "2d"},
		"UA":          {Yellow: "1d", Red: "2d"},
	}
}

// DefaultPath is ~/.config/jira-green/config.toml (XDG_CONFIG_HOME honored).
func DefaultPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "jira-green", "config.toml")
}

func Load(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	c.path = path
	if err := c.applyDefaultsAndValidate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) applyDefaultsAndValidate() error {
	s := &c.Settings
	if s.PollIntervalSeconds <= 0 {
		s.PollIntervalSeconds = 60
	}
	if s.BoardRefreshIntervalSeconds <= 0 {
		s.BoardRefreshIntervalSeconds = 600
	}
	if s.StuckAlertAfter == "" {
		s.StuckAlertAfter = "2h"
	}
	if s.DefaultView == "" {
		s.DefaultView = "kanban"
	}
	if s.DefaultView != "kanban" && s.DefaultView != "list" {
		return fmt.Errorf("settings.default_view must be kanban or list, got %q", s.DefaultView)
	}
	if _, err := model.ParseAge(s.StuckAlertAfter); err != nil {
		return fmt.Errorf("settings.stuck_alert_after: %w", err)
	}
	if strings.TrimSpace(c.Jira.Site) == "" {
		return errors.New("jira.site is required")
	}
	c.Jira.Site = strings.TrimRight(c.Jira.Site, "/")
	if c.Jira.BoardID <= 0 {
		return errors.New("jira.board_id is required")
	}
	if c.JQL.Mine == "" {
		c.JQL.Mine = DefaultMineJQL
	}
	if c.JQL.Waiting == "" {
		c.JQL.Waiting = DefaultWaitingJQL
	}
	if c.JQL.Done == "" {
		c.JQL.Done = DefaultDoneJQL
	}
	if c.Thresholds == nil {
		c.Thresholds = defaultThresholds()
	}
	if c.BlockedLabels == nil {
		c.BlockedLabels = []string{"blocked"}
	}
	_, err := c.Rules("")
	return err
}

// Rules builds the health rules for the given account ID.
func (c *Config) Rules(me string) (model.Rules, error) {
	r := model.Rules{Me: me, BlockedLabels: c.BlockedLabels, Thresholds: map[string]model.Threshold{}}
	for col, a := range c.Thresholds {
		var th model.Threshold
		var err error
		if a.Yellow != "" {
			if th.Yellow, err = model.ParseAge(a.Yellow); err != nil {
				return r, fmt.Errorf("thresholds.%q.yellow: %w", col, err)
			}
		}
		if a.Red != "" {
			if th.Red, err = model.ParseAge(a.Red); err != nil {
				return r, fmt.Errorf("thresholds.%q.red: %w", col, err)
			}
		}
		r.Thresholds[col] = th
	}
	return r, nil
}

func (c *Config) Path() string { return c.path }

func (c *Config) IsMuted(key string) bool { return slices.Contains(c.Muted, key) }

// SetMuted adds or removes a key from the mute list and saves.
func (c *Config) SetMuted(key string, muted bool) error {
	has := c.IsMuted(key)
	switch {
	case muted && !has:
		c.Muted = append(c.Muted, key)
	case !muted && has:
		c.Muted = slices.DeleteFunc(c.Muted, func(k string) bool { return k == key })
	default:
		return nil
	}
	return c.Save()
}

func (c *Config) Save() error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(c.path, buf.Bytes(), 0o600)
}

// ResolveToken returns the API token: token_command, then token_env, then the
// literal token, then $JIRA_API_TOKEN (the variable go-jira-cli also reads).
func ResolveToken(j Jira) (string, error) {
	if j.TokenCommand != "" {
		cmd := exec.Command("sh", "-c", j.TokenCommand)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("token_command failed: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		tok := strings.TrimSpace(string(out))
		if tok == "" {
			return "", errors.New("token_command produced no output")
		}
		return tok, nil
	}
	if j.TokenEnv != "" {
		if tok := strings.TrimSpace(os.Getenv(j.TokenEnv)); tok != "" {
			return tok, nil
		}
	}
	if tok := strings.TrimSpace(j.Token); tok != "" {
		return tok, nil
	}
	if tok := strings.TrimSpace(os.Getenv("JIRA_API_TOKEN")); tok != "" {
		return tok, nil
	}
	return "", errors.New("no token — set token_command, token_env, token, or JIRA_API_TOKEN")
}

// WriteStarter writes a new config for the wizard. It refuses to overwrite.
func WriteStarter(path string, j Jira) (*Config, error) {
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("%s already exists (use --force)", path)
	}
	c := &Config{Jira: j, path: path}
	if err := c.applyDefaultsAndValidate(); err != nil {
		return nil, err
	}
	return c, c.Save()
}
```

Then run `go get github.com/BurntSushi/toml` and `go mod tidy`.

Note: `WriteStarter` refuses to overwrite. The wizard's `--force` path deletes the old file
first.

**Step 4: Run the tests to confirm they pass** → `go test ./internal/config/` PASS

**Step 5: Commit** → `feat(config): load, validate, token resolution, mutes`

---

### Task 8: Jira client: transport, time, errors, myself

**Files:**
- Create: `internal/jira/client.go`, `internal/jira/time.go`, `internal/jira/errors.go`
- Test: `internal/jira/client_test.go`

**Step 1: Write the failing test**

```go
package jira

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTest(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, "me@example.com", "tok")
}

func TestTimeParsesJiraFormat(t *testing.T) {
	var jt Time
	if err := jt.UnmarshalJSON([]byte(`"2026-09-30T10:00:00.000-0400"`)); err != nil {
		t.Fatal(err)
	}
	if !jt.Time.Equal(time.Date(2026, 9, 30, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("got %v", jt.Time)
	}
	if err := jt.UnmarshalJSON([]byte(`null`)); err != nil || !jt.IsZero() {
		t.Errorf("null should be zero")
	}
}

func TestMyselfSendsBasicAuth(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "me@example.com" || p != "tok" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path != "/rest/api/3/myself" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write([]byte(`{"accountId":"acct-me","displayName":"Me"}`))
	})
	me, err := c.Myself(context.Background())
	if err != nil || me.AccountID != "acct-me" {
		t.Fatalf("%+v %v", me, err)
	}
}

func TestAuthErrorIsDetectable(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) })
	_, err := c.Myself(context.Background())
	if !IsAuth(err) {
		t.Fatalf("want auth error, got %v", err)
	}
}

func TestRateLimitCarriesRetryAfter(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(429)
	})
	_, err := c.Myself(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) || ae.RetryAfter != 30*time.Second {
		t.Fatalf("got %v", err)
	}
}
```

**Step 2: Run it to confirm it fails** → `undefined: New`

**Step 3: Implement**

`time.go`:

```go
package jira

import (
	"bytes"
	"time"
)

// Time parses Jira's timestamp format, which is not RFC 3339
// ("2026-09-30T10:00:00.000-0400").
type Time struct{ time.Time }

const jiraLayout = "2006-01-02T15:04:05.000-0700"

func (t *Time) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) || len(b) < 2 {
		return nil
	}
	s := string(b[1 : len(b)-1])
	p, err := time.Parse(jiraLayout, s)
	if err != nil {
		p, err = time.Parse(time.RFC3339, s)
	}
	t.Time = p
	return err
}
```

`errors.go`:

```go
package jira

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// APIError is a non-2xx response.
type APIError struct {
	Status     int
	Body       string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jira: HTTP %d: %s", e.Status, e.Body)
}

// IsAuth reports whether err is a 401/403 — polling should stop, not retry.
func IsAuth(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && (ae.Status == 401 || ae.Status == 403)
}

func parseRetryAfter(h string) time.Duration {
	if n, err := strconv.Atoi(h); err == nil {
		return time.Duration(n) * time.Second
	}
	return 0
}
```

`client.go`:

```go
// Package jira is a thin Jira Cloud REST client covering only what
// jira-green needs.
package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	site, email, token string
	http               *http.Client
}

func New(site, email, token string) *Client {
	return &Client{
		site:  strings.TrimRight(site, "/"),
		email: email,
		token: token,
		http:  &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) Site() string { return c.site }

// BrowseURL is the web URL for an issue key.
func (c *Client) BrowseURL(key string) string { return c.site + "/browse/" + key }

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.site+path, rdr)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return &APIError{
			Status:     resp.StatusCode,
			Body:       strings.TrimSpace(string(b)),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("jira: decode %s: %w", path, err)
	}
	return nil
}

type User struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
}

func (c *Client) Myself(ctx context.Context) (User, error) {
	var u User
	err := c.do(ctx, http.MethodGet, "/rest/api/3/myself", nil, &u)
	return u, err
}
```

**Step 4: Run the tests to confirm they pass** → PASS

**Step 5: Commit** → `feat(jira): client transport, time, errors, myself`

---

### Task 9: Jira client: search and issue conversion

**Files:**
- Create: `internal/jira/search.go`, `internal/jira/adf.go`
- Create: `internal/jira/testdata/search_page1.json`, `internal/jira/testdata/search_page2.json`
- Test: `internal/jira/search_test.go`

**Step 1: Write the fixtures** (synthetic, no employer data)

`testdata/search_page1.json`:

```json
{
  "issues": [
    {
      "key": "ABC-1",
      "fields": {
        "summary": "Fix auth redirect loop",
        "status": {"id": "3", "name": "In Progress"},
        "assignee": {"accountId": "acct-me", "displayName": "Me"},
        "parent": {"key": "ABC-100", "fields": {"summary": "Auth"}},
        "labels": ["blocked"],
        "customfield_10021": [{"value": "Impediment"}],
        "created": "2026-09-20T09:00:00.000-0400",
        "updated": "2026-09-29T09:00:00.000-0400",
        "comment": {"comments": [
          {"author": {"accountId": "acct-jsmith"}, "created": "2026-09-28T09:00:00.000-0400",
           "body": {"type": "doc", "content": [{"type": "paragraph", "content": [
             {"type": "mention", "attrs": {"id": "acct-me", "text": "@Me"}},
             {"type": "text", "text": " can you look?"}]}]}}
        ]}
      }
    }
  ],
  "nextPageToken": "p2",
  "isLast": false
}
```

`testdata/search_page2.json`:

```json
{
  "issues": [
    {
      "key": "ABC-2",
      "fields": {
        "summary": "Solr pagination",
        "status": {"id": "10", "name": "Code Review"},
        "assignee": null,
        "labels": [],
        "customfield_10021": null,
        "created": "2026-09-21T09:00:00.000-0400",
        "updated": "2026-09-21T09:00:00.000-0400",
        "comment": {"comments": []}
      }
    }
  ],
  "isLast": true
}
```

**Step 2: Write the failing test**

```go
package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

func TestSearchPaginatesAndConverts(t *testing.T) {
	calls := 0
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		calls++
		file := "testdata/search_page1.json"
		if body["nextPageToken"] == "p2" {
			file = "testdata/search_page2.json"
		}
		b, _ := os.ReadFile(file)
		w.Write(b)
	})
	issues, err := c.Search(context.Background(), "assignee = currentUser()", "customfield_10021")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(issues) != 2 {
		t.Fatalf("calls %d issues %d", calls, len(issues))
	}
	a := issues[0]
	if a.Key != "ABC-1" || a.StatusID != "3" || a.EpicKey != "ABC-100" || a.EpicSummary != "Auth" {
		t.Errorf("%+v", a)
	}
	if !a.Flagged || a.AssigneeID != "acct-me" || a.URL != c.Site()+"/browse/ABC-1" {
		t.Errorf("%+v", a)
	}
	if len(a.Comments) != 1 || a.Comments[0].Mentions[0] != "acct-me" {
		t.Errorf("comments %+v", a.Comments)
	}
	if issues[1].Flagged || issues[1].AssigneeID != "" {
		t.Errorf("%+v", issues[1])
	}
}
```

**Step 3: Run it to confirm it fails** → `c.Search undefined`

**Step 4: Implement**

`adf.go`:

```go
package jira

// mentions walks an Atlassian Document Format tree and returns every
// mentioned account ID.
func mentions(node any) []string {
	var out []string
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["type"] == "mention" {
				if attrs, ok := v["attrs"].(map[string]any); ok {
					if id, ok := attrs["id"].(string); ok {
						out = append(out, id)
					}
				}
			}
			walk(v["content"])
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	walk(node)
	return out
}
```

`search.go`:

```go
package jira

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

var baseFields = []string{"summary", "status", "assignee", "parent", "labels", "created", "updated", "comment"}

type apiIssue struct {
	Key    string                     `json:"key"`
	Fields map[string]json.RawMessage `json:"fields"`
}

type searchResp struct {
	Issues        []apiIssue `json:"issues"`
	NextPageToken string     `json:"nextPageToken"`
	IsLast        bool       `json:"isLast"`
}

// Search runs JQL and returns every matching issue as a model.Issue.
// flaggedField is the custom field ID for "Flagged" ("" to skip).
func (c *Client) Search(ctx context.Context, jql, flaggedField string) ([]model.Issue, error) {
	fields := append([]string{}, baseFields...)
	if flaggedField != "" {
		fields = append(fields, flaggedField)
	}
	var out []model.Issue
	token := ""
	for {
		body := map[string]any{"jql": jql, "fields": fields, "maxResults": 100}
		if token != "" {
			body["nextPageToken"] = token
		}
		var r searchResp
		if err := c.do(ctx, http.MethodPost, "/rest/api/3/search/jql", body, &r); err != nil {
			return nil, err
		}
		for _, ai := range r.Issues {
			out = append(out, c.convert(ai, flaggedField))
		}
		if r.IsLast || r.NextPageToken == "" {
			return out, nil
		}
		token = r.NextPageToken
	}
}

func (c *Client) convert(ai apiIssue, flaggedField string) model.Issue {
	var f struct {
		Summary string `json:"summary"`
		Status  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"status"`
		Assignee *User `json:"assignee"`
		Parent   *struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
			} `json:"fields"`
		} `json:"parent"`
		Labels  []string `json:"labels"`
		Created Time     `json:"created"`
		Updated Time     `json:"updated"`
		Comment struct {
			Comments []struct {
				Author  User `json:"author"`
				Created Time `json:"created"`
				Body    any  `json:"body"`
			} `json:"comments"`
		} `json:"comment"`
	}
	raw, _ := json.Marshal(ai.Fields)
	_ = json.Unmarshal(raw, &f)

	iss := model.Issue{
		Key:        ai.Key,
		Summary:    f.Summary,
		URL:        c.BrowseURL(ai.Key),
		StatusID:   f.Status.ID,
		StatusName: f.Status.Name,
		Labels:     f.Labels,
		Created:    f.Created.Time,
		Updated:    f.Updated.Time,
	}
	if f.Assignee != nil {
		iss.AssigneeID, iss.AssigneeName = f.Assignee.AccountID, f.Assignee.DisplayName
	}
	if f.Parent != nil {
		iss.EpicKey, iss.EpicSummary = f.Parent.Key, f.Parent.Fields.Summary
	}
	if flaggedField != "" {
		var flags []any
		if json.Unmarshal(ai.Fields[flaggedField], &flags) == nil && len(flags) > 0 {
			iss.Flagged = true
		}
	}
	for _, cm := range f.Comment.Comments {
		iss.Comments = append(iss.Comments, model.Comment{
			AuthorID: cm.Author.AccountID,
			Created:  cm.Created.Time,
			Mentions: mentions(cm.Body),
		})
	}
	return iss
}
```

**Step 5: Run the tests to confirm they pass** → PASS

**Step 6: Commit** → `feat(jira): search with pagination and issue conversion`

---

### Task 10: Jira client: board config, boards, changelog, transitions, fields

**Files:**
- Create: `internal/jira/board.go`, `internal/jira/changelog.go`, `internal/jira/transitions.go`, `internal/jira/fields.go`
- Test: `internal/jira/board_test.go` (one test file covers all four; keep it table-light)

**Step 1: Write the failing test**

```go
package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestBoardColumns(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board/7/configuration" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write([]byte(`{"columnConfig":{"columns":[
			{"name":"To Do","statuses":[{"id":"1"}]},
			{"name":"Code Review","statuses":[{"id":"10"},{"id":"11"}]}]}}`))
	})
	cols, err := c.BoardColumns(context.Background(), 7)
	if err != nil || len(cols) != 2 || cols[1].StatusIDs[1] != "11" {
		t.Fatalf("%+v %v", cols, err)
	}
}

func TestStatusChangesPaginates(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("startAt") == "0" {
			w.Write([]byte(`{"startAt":0,"maxResults":1,"isLast":false,"values":[
				{"created":"2026-09-21T09:00:00.000-0400","items":[{"field":"status","to":"3"},{"field":"labels","to":null}]}]}`))
			return
		}
		w.Write([]byte(`{"startAt":1,"maxResults":1,"isLast":true,"values":[
			{"created":"2026-09-22T09:00:00.000-0400","items":[{"field":"status","to":"10"}]}]}`))
	})
	ch, err := c.StatusChanges(context.Background(), "ABC-1")
	if err != nil || len(ch) != 2 || ch[1].ToID != "10" {
		t.Fatalf("%+v %v", ch, err)
	}
}

func TestTransitions(t *testing.T) {
	var posted map[string]any
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte(`{"transitions":[{"id":"21","name":"Start review","to":{"id":"10","name":"Code Review"}}]}`))
			return
		}
		json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(204)
	})
	ts, err := c.Transitions(context.Background(), "ABC-1")
	if err != nil || len(ts) != 1 || ts[0].ToName != "Code Review" {
		t.Fatalf("%+v %v", ts, err)
	}
	if err := c.DoTransition(context.Background(), "ABC-1", "21"); err != nil {
		t.Fatal(err)
	}
	if posted["transition"].(map[string]any)["id"] != "21" {
		t.Errorf("posted %v", posted)
	}
}

func TestFindFieldID(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":"summary","name":"Summary"},{"id":"customfield_10021","name":"Flagged"}]`))
	})
	id, err := c.FindFieldID(context.Background(), "flagged")
	if err != nil || id != "customfield_10021" {
		t.Fatalf("%q %v", id, err)
	}
}

func TestBoards(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"isLast":true,"values":[{"id":7,"name":"ABC board","location":{"projectKey":"ABC"}}]}`))
	})
	bs, err := c.Boards(context.Background())
	if err != nil || len(bs) != 1 || bs[0].ID != 7 || bs[0].ProjectKey != "ABC" {
		t.Fatalf("%+v %v", bs, err)
	}
}
```

**Step 2: Run it to confirm it fails** → undefined methods

**Step 3: Implement**

`board.go`:

```go
package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

// BoardColumns returns the board's columns in display order.
func (c *Client) BoardColumns(ctx context.Context, boardID int) ([]model.Column, error) {
	var r struct {
		ColumnConfig struct {
			Columns []struct {
				Name     string `json:"name"`
				Statuses []struct {
					ID string `json:"id"`
				} `json:"statuses"`
			} `json:"columns"`
		} `json:"columnConfig"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/rest/agile/1.0/board/%d/configuration", boardID), nil, &r); err != nil {
		return nil, err
	}
	var cols []model.Column
	for _, col := range r.ColumnConfig.Columns {
		mc := model.Column{Name: col.Name}
		for _, s := range col.Statuses {
			mc.StatusIDs = append(mc.StatusIDs, s.ID)
		}
		cols = append(cols, mc)
	}
	return cols, nil
}

type Board struct {
	ID         int
	Name       string
	ProjectKey string
}

// Boards lists boards visible to the user (for the init wizard).
func (c *Client) Boards(ctx context.Context) ([]Board, error) {
	var out []Board
	for start := 0; ; {
		var r struct {
			IsLast bool `json:"isLast"`
			Values []struct {
				ID       int    `json:"id"`
				Name     string `json:"name"`
				Location struct {
					ProjectKey string `json:"projectKey"`
				} `json:"location"`
			} `json:"values"`
		}
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/rest/agile/1.0/board?startAt=%d&maxResults=50", start), nil, &r); err != nil {
			return nil, err
		}
		for _, v := range r.Values {
			out = append(out, Board{ID: v.ID, Name: v.Name, ProjectKey: v.Location.ProjectKey})
		}
		if r.IsLast || len(r.Values) == 0 {
			return out, nil
		}
		start += len(r.Values)
	}
}
```

`changelog.go`:

```go
package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

// StatusChanges returns every status transition in the issue's changelog.
func (c *Client) StatusChanges(ctx context.Context, key string) ([]model.StatusChange, error) {
	var out []model.StatusChange
	for start := 0; ; {
		var r struct {
			IsLast bool `json:"isLast"`
			Values []struct {
				Created Time `json:"created"`
				Items   []struct {
					Field string  `json:"field"`
					To    *string `json:"to"`
				} `json:"items"`
			} `json:"values"`
		}
		path := fmt.Sprintf("/rest/api/3/issue/%s/changelog?startAt=%d&maxResults=100", key, start)
		if err := c.do(ctx, http.MethodGet, path, nil, &r); err != nil {
			return nil, err
		}
		for _, v := range r.Values {
			for _, it := range v.Items {
				if it.Field == "status" && it.To != nil {
					out = append(out, model.StatusChange{At: v.Created.Time, ToID: *it.To})
				}
			}
		}
		if r.IsLast || len(r.Values) == 0 {
			return out, nil
		}
		start += len(r.Values)
	}
}
```

`transitions.go`:

```go
package jira

import (
	"context"
	"net/http"
)

type Transition struct {
	ID     string
	Name   string
	ToName string
}

// Transitions lists the transitions Jira allows from the issue's current status.
func (c *Client) Transitions(ctx context.Context, key string) ([]Transition, error) {
	var r struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := c.do(ctx, http.MethodGet, "/rest/api/3/issue/"+key+"/transitions", nil, &r); err != nil {
		return nil, err
	}
	out := make([]Transition, 0, len(r.Transitions))
	for _, t := range r.Transitions {
		out = append(out, Transition{ID: t.ID, Name: t.Name, ToName: t.To.Name})
	}
	return out, nil
}

func (c *Client) DoTransition(ctx context.Context, key, transitionID string) error {
	body := map[string]any{"transition": map[string]string{"id": transitionID}}
	return c.do(ctx, http.MethodPost, "/rest/api/3/issue/"+key+"/transitions", body, nil)
}
```

`fields.go`:

```go
package jira

import (
	"context"
	"net/http"
	"strings"
)

// FindFieldID returns the ID of the field with the given display name
// (case-insensitive), or "" if none.
func (c *Client) FindFieldID(ctx context.Context, name string) (string, error) {
	var fs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/rest/api/3/field", nil, &fs); err != nil {
		return "", err
	}
	for _, f := range fs {
		if strings.EqualFold(f.Name, name) {
			return f.ID, nil
		}
	}
	return "", nil
}
```

Also add the interface the poller and UI depend on, at the bottom of `client.go`:

```go
// API is the subset of Client the poller and UI use; tests supply fakes.
type API interface {
	Myself(ctx context.Context) (User, error)
	Search(ctx context.Context, jql, flaggedField string) ([]model.Issue, error)
	BoardColumns(ctx context.Context, boardID int) ([]model.Column, error)
	StatusChanges(ctx context.Context, key string) ([]model.StatusChange, error)
	Transitions(ctx context.Context, key string) ([]Transition, error)
	DoTransition(ctx context.Context, key, transitionID string) error
}

var _ API = (*Client)(nil)
```

(Add the `model` import to `client.go`.)

**Step 4: Run the tests to confirm they pass** → `go test ./internal/jira/` PASS

**Step 5: Commit** → `feat(jira): board config, boards, changelog, transitions, fields`

---

### Task 11: Poller

The poller produces immutable `Snapshot`s on a channel, following coolify-green's poller
(`../coolify-green/internal/poller/poller.go`). Read that file first; keep the same
`Start` / `ForceRefresh` / channel shape.

Behavior:
- Each poll runs the three lane JQL queries (Mine, Waiting, Done), fetches the changelog only
  for issues whose `Updated` differs from the cache, and evaluates cards.
- An issue that matches both Mine and Waiting stays in Mine. Dedupe by key, in the order
  Mine → Waiting → Done.
- Muted keys, and issues whose epic key is muted, are dropped.
- Board columns are cached and refetched after `BoardRefreshIntervalSeconds`.
- If a poll errors, the previous snapshot's cards are re-evaluated with `stale=true` and
  `Snapshot.Err` is set.
- On `jira.IsAuth(err)`: set `Snapshot.AuthFailed = true` and stop the ticker loop.
- On a 429 (`APIError.RetryAfter > 0`): the next poll waits `max(interval, RetryAfter)`.

**Files:**
- Create: `internal/poller/poller.go`
- Test: `internal/poller/poller_test.go`

**Step 1: Write the failing test**

```go
package poller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/jira"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

type fake struct {
	byJQL        map[string][]model.Issue
	changelogs   map[string][]model.StatusChange
	changelogHit map[string]int
	err          error
}

func (f *fake) Myself(context.Context) (jira.User, error) { return jira.User{AccountID: "acct-me"}, nil }
func (f *fake) Search(_ context.Context, jql, _ string) ([]model.Issue, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byJQL[jql], nil
}
func (f *fake) BoardColumns(context.Context, int) ([]model.Column, error) {
	return []model.Column{{Name: "To Do", StatusIDs: []string{"1"}}, {Name: "In Progress", StatusIDs: []string{"3"}}, {Name: "Done", StatusIDs: []string{"5"}}}, nil
}
func (f *fake) StatusChanges(_ context.Context, key string) ([]model.StatusChange, error) {
	f.changelogHit[key]++
	return f.changelogs[key], nil
}
func (f *fake) Transitions(context.Context, string) ([]jira.Transition, error) { return nil, nil }
func (f *fake) DoTransition(context.Context, string, string) error          { return nil }

func cfg(t *testing.T) *config.Config {
	t.Helper()
	c := &config.Config{Jira: config.Jira{Site: "https://example.atlassian.net", BoardID: 7}}
	if err := config.ApplyDefaults(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPollBuildsLanesAndDedupes(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	iss := model.Issue{Key: "ABC-1", StatusID: "3", Created: now.Add(-200 * time.Hour), Updated: now}
	c := cfg(t)
	f := &fake{
		byJQL: map[string][]model.Issue{
			c.JQL.Mine:    {iss},
			c.JQL.Waiting: {iss, {Key: "ABC-2", StatusID: "999", Created: now, Updated: now}},
		},
		changelogs:   map[string][]model.StatusChange{"ABC-1": {{At: now.Add(-130 * time.Hour), ToID: "3"}}},
		changelogHit: map[string]int{},
	}
	p := New(c, f)
	p.now = func() time.Time { return now }

	snap := p.PollOnce(context.Background())
	if snap.Err != nil {
		t.Fatal(snap.Err)
	}
	if len(snap.Cards) != 2 {
		t.Fatalf("want 2 cards (deduped), got %d", len(snap.Cards))
	}
	a := snap.Cards[0]
	if a.Key != "ABC-1" || a.Lane != model.LaneMine || a.Column != "In Progress" || a.Light != model.Red {
		t.Errorf("ABC-1 %+v", a)
	}
	if snap.Cards[1].Column != model.OtherColumn || snap.Cards[1].Lane != model.LaneWaiting {
		t.Errorf("ABC-2 %+v", snap.Cards[1])
	}

	p.PollOnce(context.Background())
	if f.changelogHit["ABC-1"] != 1 {
		t.Errorf("changelog should be cached when Updated is unchanged, hits=%d", f.changelogHit["ABC-1"])
	}
}

func TestPollErrorMarksStale(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	c := cfg(t)
	f := &fake{byJQL: map[string][]model.Issue{c.JQL.Mine: {{Key: "ABC-1", StatusID: "1", Created: now, Updated: now}}},
		changelogHit: map[string]int{}}
	p := New(c, f)
	p.now = func() time.Time { return now }
	p.PollOnce(context.Background())

	f.err = errors.New("boom")
	snap := p.PollOnce(context.Background())
	if snap.Err == nil || len(snap.Cards) != 1 || snap.Cards[0].Light != model.Stale {
		t.Fatalf("%+v", snap)
	}
}

func TestPollAuthFailure(t *testing.T) {
	c := cfg(t)
	f := &fake{err: &jira.APIError{Status: 401}, changelogHit: map[string]int{}}
	snap := New(c, f).PollOnce(context.Background())
	if !snap.AuthFailed {
		t.Fatal("want AuthFailed")
	}
}

func TestMutedDropped(t *testing.T) {
	now := time.Now()
	c := cfg(t)
	c.Muted = []string{"ABC-100"}
	f := &fake{byJQL: map[string][]model.Issue{c.JQL.Mine: {
		{Key: "ABC-1", EpicKey: "ABC-100", Created: now, Updated: now},
		{Key: "ABC-2", Created: now, Updated: now},
	}}, changelogHit: map[string]int{}}
	snap := New(c, f).PollOnce(context.Background())
	if len(snap.Cards) != 1 || snap.Cards[0].Key != "ABC-2" {
		t.Fatalf("%+v", snap.Cards)
	}
}
```

This test needs `config.ApplyDefaults(c *Config) error` exported. Add it in `config.go` as a thin
wrapper: `func ApplyDefaults(c *Config) error { return c.applyDefaultsAndValidate() }`.

**Step 2: Run it to confirm it fails** → `undefined: New`

**Step 3: Implement**

```go
// Package poller fetches Jira state on an interval and emits snapshots.
package poller

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/jira"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

// Snapshot is one immutable poll result.
type Snapshot struct {
	Columns    []model.Column
	Cards      []model.Card
	At         time.Time
	Err        error
	AuthFailed bool
	RetryAfter time.Duration
}

type clEntry struct {
	updated time.Time
	since   time.Time
}

type Poller struct {
	cfg *config.Config
	api jira.API
	now func() time.Time

	mu         sync.Mutex
	me         string
	cols       []model.Column
	colsAt     time.Time
	changelogs map[string]clEntry
	last       Snapshot
}

func New(cfg *config.Config, api jira.API) *Poller {
	return &Poller{cfg: cfg, api: api, now: time.Now, changelogs: map[string]clEntry{}}
}

// PollOnce performs one poll synchronously. Safe to call from any goroutine.
func (p *Poller) PollOnce(ctx context.Context) Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()

	snap, err := p.fetch(ctx, now)
	if err != nil {
		snap = p.staleFrom(err)
		snap.AuthFailed = jira.IsAuth(err)
		var ae *jira.APIError
		if errors.As(err, &ae) {
			snap.RetryAfter = ae.RetryAfter
		}
	}
	p.last = snap
	return snap
}

func (p *Poller) fetch(ctx context.Context, now time.Time) (Snapshot, error) {
	if p.me == "" {
		u, err := p.api.Myself(ctx)
		if err != nil {
			return Snapshot{}, err
		}
		p.me = u.AccountID
	}
	refresh := time.Duration(p.cfg.Settings.BoardRefreshIntervalSeconds) * time.Second
	if p.cols == nil || now.Sub(p.colsAt) >= refresh {
		cols, err := p.api.BoardColumns(ctx, p.cfg.Jira.BoardID)
		if err != nil {
			return Snapshot{}, err
		}
		p.cols, p.colsAt = cols, now
	}
	rules, err := p.cfg.Rules(p.me)
	if err != nil {
		return Snapshot{}, err
	}

	lanes := []struct {
		lane model.Lane
		jql  string
	}{
		{model.LaneMine, p.cfg.JQL.Mine},
		{model.LaneWaiting, p.cfg.JQL.Waiting},
		{model.LaneDone, p.cfg.JQL.Done},
	}
	seen := map[string]bool{}
	var cards []model.Card
	for _, l := range lanes {
		issues, err := p.api.Search(ctx, l.jql, p.cfg.Jira.FlaggedField)
		if err != nil {
			return Snapshot{}, err
		}
		for _, iss := range issues {
			if seen[iss.Key] || p.cfg.IsMuted(iss.Key) || (iss.EpicKey != "" && p.cfg.IsMuted(iss.EpicKey)) {
				continue
			}
			seen[iss.Key] = true
			if l.lane != model.LaneDone {
				since, err := p.statusSince(ctx, iss)
				if err != nil {
					return Snapshot{}, err
				}
				iss.StatusSince = since
			}
			col := model.ColumnFor(p.cols, iss.StatusID)
			cards = append(cards, model.Evaluate(iss, col, l.lane, rules, now, false))
		}
	}
	return Snapshot{Columns: p.cols, Cards: cards, At: now}, nil
}

func (p *Poller) statusSince(ctx context.Context, iss model.Issue) (time.Time, error) {
	if e, ok := p.changelogs[iss.Key]; ok && e.updated.Equal(iss.Updated) {
		return e.since, nil
	}
	ch, err := p.api.StatusChanges(ctx, iss.Key)
	if err != nil {
		return time.Time{}, err
	}
	since := model.StatusSince(iss.Created, iss.StatusID, ch)
	p.changelogs[iss.Key] = clEntry{updated: iss.Updated, since: since}
	return since, nil
}

// staleFrom re-marks the last good cards as stale, keeping red/yellow.
func (p *Poller) staleFrom(err error) Snapshot {
	s := Snapshot{Columns: p.last.Columns, At: p.last.At, Err: err}
	for _, c := range p.last.Cards {
		c.Light = model.Worst(c.Light, model.Stale)
		s.Cards = append(s.Cards, c)
	}
	return s
}

// Start polls on the configured interval until ctx is done or auth fails.
// A refresh request on the returned channel triggers an immediate poll.
func (p *Poller) Start(ctx context.Context) (<-chan Snapshot, chan<- struct{}) {
	out := make(chan Snapshot, 1)
	refresh := make(chan struct{}, 1)
	go func() {
		defer close(out)
		interval := time.Duration(p.cfg.Settings.PollIntervalSeconds) * time.Second
		for {
			snap := p.PollOnce(ctx)
			select {
			case out <- snap:
			case <-ctx.Done():
				return
			}
			if snap.AuthFailed {
				return
			}
			wait := max(interval, snap.RetryAfter)
			select {
			case <-time.After(wait):
			case <-refresh:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, refresh
}
```

Note for the stale test: the stale card in `TestPollErrorMarksStale` is `To Do` with no
threshold, so its light is Green before, and `Worst(Green, Stale)` = Stale. That matches.

**Step 4: Run the tests to confirm they pass** → `go test -race ./internal/poller/` PASS

**Step 5: Commit** → `feat(poller): lanes, changelog cache, stale, auth stop, backoff`

---

### Task 12: Stuck-alert webhooks

Port `../coolify-green/internal/webhooks/` (signing, dispatch, tests), then adapt it.

**Files:**
- Create: `internal/alert/alert.go` (copied from `webhooks.go`, package renamed `alert`)
- Create: `internal/alert/stuck.go`
- Test: `internal/alert/alert_test.go` (port the coolify-green tests), `internal/alert/stuck_test.go`

**Step 1: Port**

```bash
mkdir -p internal/alert
cp -f ../coolify-green/internal/webhooks/webhooks.go internal/alert/alert.go
cp -f ../coolify-green/internal/webhooks/webhooks_test.go internal/alert/alert_test.go
sed -i '' 's/package webhooks/package alert/; s#coolify-green/internal#jira-green/internal#' internal/alert/*.go
```

Change the `Event` struct to:

```go
type Event struct {
	Type    string    `json:"type"` // "ticket.stuck"
	Key     string    `json:"key"`
	Summary string    `json:"summary"`
	Status  string    `json:"status"`
	URL     string    `json:"url"`
	Reasons []string  `json:"reasons"`
	RedFor  string    `json:"red_for"`
	At      time.Time `json:"at"`
}
```

Update the ported tests to use this Event, then run `go test ./internal/alert/` and fix until it
passes. Keep the signing header name and algorithm the same as coolify-green, so any receiver
works for both tools.

**Step 2: Write the failing stuck test**

```go
package alert

import (
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

func TestTrackerFiresOncePerIncident(t *testing.T) {
	tr := NewTracker(2 * time.Hour)
	t0 := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	red := []model.Card{{Issue: model.Issue{Key: "ABC-1"}, Light: model.Red}}

	if ev := tr.Observe(red, t0); len(ev) != 0 {
		t.Fatal("not yet past threshold")
	}
	if ev := tr.Observe(red, t0.Add(2*time.Hour)); len(ev) != 1 {
		t.Fatal("should fire at threshold")
	}
	if ev := tr.Observe(red, t0.Add(3*time.Hour)); len(ev) != 0 {
		t.Fatal("should not re-fire")
	}
	green := []model.Card{{Issue: model.Issue{Key: "ABC-1"}, Light: model.Green}}
	tr.Observe(green, t0.Add(4*time.Hour))
	tr.Observe(red, t0.Add(5*time.Hour))
	if ev := tr.Observe(red, t0.Add(7*time.Hour)); len(ev) != 1 {
		t.Fatal("new incident after recovery should fire again")
	}
}
```

**Step 3: Run it to confirm it fails** → `undefined: NewTracker`

**Step 4: Implement `stuck.go`**

```go
package alert

import (
	"time"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

// Tracker turns a stream of snapshots into one stuck event per red incident.
type Tracker struct {
	after   time.Duration
	redFrom map[string]time.Time
	fired   map[string]bool
}

func NewTracker(after time.Duration) *Tracker {
	return &Tracker{after: after, redFrom: map[string]time.Time{}, fired: map[string]bool{}}
}

// Observe records the cards' lights at now and returns events to send.
func (t *Tracker) Observe(cards []model.Card, now time.Time) []Event {
	var out []Event
	present := map[string]bool{}
	for _, c := range cards {
		present[c.Key] = true
		if c.Light != model.Red {
			delete(t.redFrom, c.Key)
			delete(t.fired, c.Key)
			continue
		}
		from, ok := t.redFrom[c.Key]
		if !ok {
			t.redFrom[c.Key] = now
			continue
		}
		if !t.fired[c.Key] && now.Sub(from) >= t.after {
			t.fired[c.Key] = true
			out = append(out, Event{
				Type: "ticket.stuck", Key: c.Key, Summary: c.Summary, Status: c.StatusName,
				URL: c.URL, Reasons: c.Reasons, RedFor: model.FormatAge(now.Sub(from)), At: now,
			})
		}
	}
	for k := range t.redFrom {
		if !present[k] {
			delete(t.redFrom, k)
			delete(t.fired, k)
		}
	}
	return out
}
```

Stale cards keep their red light (`Worst`), so a network blip doesn't reset an incident.

**Step 5: Run the tests to confirm they pass** → PASS

**Step 6: Commit** → `feat(alert): stuck webhooks ported from coolify-green`

---

### Task 13: UI: kanban renderer (pure function + golden tests)

Rendering is a pure function of `(Board, cursor, width)`. That makes golden tests deterministic.

**Files:**
- Create: `internal/ui/kanban.go`, `internal/ui/golden_test.go`, `internal/ui/fixtures_test.go`
- Create: `internal/ui/testdata/kanban_80.golden`, `internal/ui/testdata/kanban_160.golden` (generated)

**Step 1: Shared fixtures and golden helper**

`fixtures_test.go`:

```go
package ui

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/muesli/termenv"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name+".golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(p, []byte(got), 0o644)
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update): %v", p, err)
	}
	if string(want) != got {
		t.Errorf("%s mismatch:\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

var fxCols = []model.Column{
	{Name: "To Do", StatusIDs: []string{"1"}},
	{Name: "In Progress", StatusIDs: []string{"3"}},
	{Name: "Code Review", StatusIDs: []string{"10"}},
	{Name: "UA", StatusIDs: []string{"12"}},
	{Name: "Done", StatusIDs: []string{"5"}},
}

func card(key, summary, col string, lane model.Lane, light model.Stoplight, age time.Duration, epic, assignee string) model.Card {
	return model.Card{
		Issue: model.Issue{Key: key, Summary: summary, EpicSummary: epic, EpicKey: epicKey(epic), AssigneeName: assignee, Flagged: light == model.Red},
		Column: col, Lane: lane, Light: light, Age: age,
	}
}

func epicKey(e string) string {
	if e == "" {
		return ""
	}
	return "ABC-E-" + e
}

func fxCards() []model.Card {
	d := 24 * time.Hour
	return []model.Card{
		card("ABC-2011", "Add alt text to search results", "To Do", model.LaneMine, model.Green, 1*d, "Accessibility", "Me"),
		card("ABC-2020", "Update footer links", "To Do", model.LaneMine, model.Green, 0, "", "Me"),
		card("ABC-1974", "Fix auth redirect loop", "In Progress", model.LaneMine, model.Yellow, 4*d, "Auth", "Me"),
		card("ABC-1836", "Solr pagination breaks on page 11", "Code Review", model.LaneMine, model.Red, 6*d, "Search", "Me"),
		card("ABC-1990", "Harden session cookie", "In Progress", model.LaneWaiting, model.Yellow, 3*d, "Auth", "J Smith"),
		card("ABC-1950", "Verify catalog export", "UA", model.LaneWaiting, model.Green, 1*d, "", "QA Team"),
	}
}
```

Run `go get github.com/charmbracelet/lipgloss github.com/muesli/termenv`.

**Step 2: Write the failing test**

`golden_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

func TestKanbanGolden(t *testing.T) {
	b := model.Layout(fxCols, fxCards(), false)
	for _, w := range []int{80, 160} {
		got := RenderKanban(b, Cursor{Lane: model.LaneMine, Col: 2, Row: 0}, w)
		golden(t, "kanban_"+itoa(w), got)
	}
}
```

(Add `func itoa(i int) string { return strconv.Itoa(i) }` in `fixtures_test.go`.)

**Step 3: Run it to confirm it fails** → `undefined: RenderKanban`

**Step 4: Implement `kanban.go`**

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

// Cursor addresses one card on the kanban board.
type Cursor struct {
	Lane model.Lane
	Col  int
	Row  int
}

var (
	selStyle  = lipgloss.NewStyle().Reverse(true)
	dimStyle  = lipgloss.NewStyle().Faint(true)
	headStyle = lipgloss.NewStyle().Bold(true)
)

// visibleLanes are the lanes the kanban draws, in order. Done shows as a
// lane only when the board is laid out with showDone.
func visibleLanes(b model.Board) []model.Lane {
	ls := []model.Lane{model.LaneMine, model.LaneWaiting}
	if b.LaneCount(model.LaneDone) > 0 {
		ls = append(ls, model.LaneDone)
	}
	return ls
}

// RenderKanban draws the board to fit width.
func RenderKanban(b model.Board, cur Cursor, width int) string {
	n := len(b.Columns)
	if n == 0 {
		return "no columns"
	}
	colW := max(12, (width-1)/n)
	var sb strings.Builder

	for _, name := range b.Columns {
		sb.WriteString(headStyle.Render(pad(" "+name, colW)))
	}
	sb.WriteString("\n")

	for _, lane := range visibleLanes(b) {
		title := fmt.Sprintf("─ %s %s (%d) ", b.LaneLight(lane).Emoji(), lane, b.LaneCount(lane))
		sb.WriteString(title + strings.Repeat("─", max(0, width-lipgloss.Width(title))) + "\n")

		depth := 0
		for _, name := range b.Columns {
			depth = max(depth, len(b.Cell(lane, name)))
		}
		for row := 0; row < depth; row++ {
			// Each card is three lines: light+key, summary, age (+assignee in Waiting).
			lines := [3]strings.Builder{}
			for ci, name := range b.Columns {
				cell := b.Cell(lane, name)
				var l [3]string
				if row < len(cell) {
					c := cell[row]
					l[0] = fmt.Sprintf(" %s %s", c.Light.Emoji(), c.Key)
					l[1] = "  " + truncate(c.Summary, colW-3)
					meta := model.FormatAge(c.Age)
					if c.Flagged {
						meta += " ⚑"
					}
					if lane != model.LaneMine && c.AssigneeName != "" {
						meta = "@" + firstWord(c.AssigneeName) + " " + meta
					}
					l[2] = "  " + truncate(meta, colW-3)
				}
				selected := cur.Lane == lane && cur.Col == ci && cur.Row == row && row < len(cell)
				for i := range l {
					s := pad(l[i], colW)
					if selected {
						s = selStyle.Render(s)
					}
					lines[i].WriteString(s)
				}
			}
			for i := range lines {
				sb.WriteString(strings.TrimRight(lines[i].String(), " ") + "\n")
			}
		}
		if depth == 0 {
			sb.WriteString(dimStyle.Render("  nothing here") + "\n")
		}
	}
	return sb.String()
}

func pad(s string, w int) string {
	s = truncate(s, w)
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+2 > w {
		r = r[:len(r)-1]
	}
	return string(r) + ".."
}

func firstWord(s string) string {
	if i := strings.IndexByte(s, ' '); i > 0 {
		return strings.ToLower(s[:i])
	}
	return strings.ToLower(s)
}
```

**Step 5: Generate and inspect goldens**

Run: `go test ./internal/ui/ -run TestKanbanGolden -update`, then `cat internal/ui/testdata/kanban_80.golden`.

Check by eye: four columns (Done hidden), a Mine lane and a Waiting lane, ABC-1836 red with ⚑,
and ABC-1990 showing `@j 3d`. No line in the 80 file is wider than 80. Check with:
`awk '{ if (length($0) > 80) print NR": "length($0) }' internal/ui/testdata/kanban_80.golden`.
Emoji are double-width, so `length` under-counts. That's fine as a smoke check.

**Step 6: Run the tests to confirm they pass** → `go test ./internal/ui/` PASS

**Step 7: Commit** → `feat(ui): kanban renderer with golden tests`

---

### Task 14: UI: list renderer

**Files:**
- Create: `internal/ui/list.go`
- Modify: `internal/ui/golden_test.go`

**Step 1: Write the failing test** (append to `golden_test.go`)

```go
func TestListGolden(t *testing.T) {
	groups := model.ByEpic(fxCards())
	collapsed := map[string]bool{"ABC-E-Accessibility": true}
	for _, w := range []int{80, 160} {
		got := RenderList(groups, collapsed, 1, w)
		golden(t, "list_"+itoa(w), got)
	}
}

func TestListRows(t *testing.T) {
	groups := model.ByEpic(fxCards())
	rows := ListRows(groups, map[string]bool{})
	// 4 groups (Search, Auth, Accessibility, No epic) + 6 cards
	if len(rows) != 10 {
		t.Fatalf("rows %d", len(rows))
	}
	if rows[0].Group == nil || rows[1].Card == nil {
		t.Errorf("row 0 is a group header, row 1 is its first card")
	}
}
```

**Step 2: Run it to confirm it fails** → `undefined: RenderList`

**Step 3: Implement**

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

// Row is one line of the list view: a group header or a card.
type Row struct {
	Group *model.EpicGroup
	Card  *model.Card
}

// ListRows flattens groups into rows, skipping cards of collapsed groups.
// collapsed is keyed by epic key ("" for No epic).
func ListRows(groups []model.EpicGroup, collapsed map[string]bool) []Row {
	var rows []Row
	for gi := range groups {
		g := &groups[gi]
		rows = append(rows, Row{Group: g})
		if collapsed[g.Key] {
			continue
		}
		for ci := range g.Cards {
			rows = append(rows, Row{Card: &g.Cards[ci]})
		}
	}
	return rows
}

// RenderList draws the epic tree. sel indexes into ListRows.
func RenderList(groups []model.EpicGroup, collapsed map[string]bool, sel, width int) string {
	var sb strings.Builder
	for i, r := range ListRows(groups, collapsed) {
		var line string
		if r.Group != nil {
			arrow := "▼"
			if collapsed[r.Group.Key] {
				arrow = "▶"
			}
			mine, waiting := 0, 0
			for _, c := range r.Group.Cards {
				if c.Lane == model.LaneMine {
					mine++
				} else {
					waiting++
				}
			}
			line = fmt.Sprintf("%s %s %-28s Mine %d  Waiting %d", arrow, r.Group.Light.Emoji(), truncate(r.Group.Name, 28), mine, waiting)
		} else {
			c := r.Card
			meta := model.FormatAge(c.Age)
			if c.Flagged {
				meta += " ⚑"
			}
			who := ""
			if c.Lane != model.LaneMine && c.AssigneeName != "" {
				who = " @" + firstWord(c.AssigneeName)
			}
			prefix := fmt.Sprintf("    %s %-9s %-12s %-5s", c.Light.Emoji(), c.Key, truncate(c.Column, 12), meta)
			line = prefix + " " + truncate(c.Summary+who, max(10, width-lipglossWidth(prefix)-1))
		}
		line = pad(line, width)
		if i == sel {
			line = selStyle.Render(line)
		}
		sb.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return sb.String()
}

func lipglossWidth(s string) int { return len([]rune(s)) }
```

(If `golangci-lint` flags `lipglossWidth` as redundant, replace it with `lipgloss.Width`.)

**Step 4: Generate the goldens, inspect them, and run the tests**

`go test ./internal/ui/ -run TestList -update && cat internal/ui/testdata/list_80.golden`, then run
`go test ./internal/ui/` → PASS.

**Step 5: Commit** → `feat(ui): list renderer grouped by epic`

---

### Task 15: UI: dashboard model (navigation, view toggle, done, refresh, open)

The Bubble Tea model owns state and delegates to the pure renderers. Look at
`../coolify-green/internal/ui/dashboard.go` and `../coolify-green/main.go` for the
spinner, window-size, and help-overlay patterns, and copy them.

**Files:**
- Create: `internal/ui/dashboard.go`, `internal/ui/keys.go`, `internal/ui/help.go`
- Test: `internal/ui/dashboard_test.go`

**Step 1: Write the failing test**

```go
package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/poller"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func loaded(t *testing.T) Dashboard {
	d := NewDashboard("kanban")
	d, _ = d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: fxCards(), At: time.Now()})
	return d
}

func TestToggleView(t *testing.T) {
	d := loaded(t)
	if d.View_ != ViewKanban {
		t.Fatal("starts kanban")
	}
	d, _ = d.Update(key("v"))
	if d.View_ != ViewList {
		t.Fatal("v toggles to list")
	}
}

func TestKanbanNavigationSkipsEmptyCells(t *testing.T) {
	d := loaded(t)
	// Mine lane, To Do column has 2 cards; right moves to In Progress.
	d, _ = d.Update(key("right"))
	if c := d.Selected(); c == nil || c.Key != "ABC-1974" {
		t.Fatalf("selected %+v", c)
	}
	// Down past the last Mine card moves into the Waiting lane.
	d, _ = d.Update(key("down"))
	if c := d.Selected(); c == nil || c.Key != "ABC-1990" {
		t.Fatalf("selected %+v", c)
	}
}

func TestDoneToggleRelayouts(t *testing.T) {
	d := loaded(t)
	n := len(d.board.Columns)
	d, _ = d.Update(key("d"))
	if len(d.board.Columns) != n+1 {
		t.Fatalf("d should reveal Done column")
	}
}

func TestOpenEmitsCommand(t *testing.T) {
	d := loaded(t)
	var opened string
	d.openURL = func(u string) error { opened = u; return nil }
	d.board.Cell(model.LaneMine, "To Do")[0].URL = "https://example.atlassian.net/browse/ABC-2011"
	_, cmd := d.Update(key("o"))
	if cmd == nil {
		t.Fatal("o should return a command")
	}
	cmd()
	if opened == "" {
		t.Fatal("URL not opened")
	}
}
```

Note: `View_` is used because `View()` is the Bubble Tea method. If you prefer, name the field
`mode` and adjust the test.

**Step 2: Run it to confirm it fails** → `undefined: NewDashboard`

**Step 3: Implement `dashboard.go`**

```go
package ui

import (
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/poller"
)

type ViewMode int

const (
	ViewKanban ViewMode = iota
	ViewList
)

// RefreshMsg asks main to trigger an immediate poll.
type RefreshMsg struct{}

// Dashboard is the main screen.
type Dashboard struct {
	View_     ViewMode
	snap      poller.Snapshot
	board     model.Board
	groups    []model.EpicGroup
	showDone  bool
	cur       Cursor
	listSel   int
	collapsed map[string]bool
	width     int
	height    int
	openURL   func(string) error
}

func NewDashboard(defaultView string) Dashboard {
	d := Dashboard{collapsed: map[string]bool{}, openURL: openBrowser}
	if defaultView == "list" {
		d.View_ = ViewList
	}
	return d
}

func (d Dashboard) Init() tea.Cmd { return nil }

func (d *Dashboard) relayout() {
	d.board = model.Layout(d.snap.Columns, d.visibleCards(), d.showDone)
	d.groups = model.ByEpic(d.visibleCards())
	d.clamp()
}

func (d Dashboard) visibleCards() []model.Card {
	if d.showDone {
		return d.snap.Cards
	}
	var out []model.Card
	for _, c := range d.snap.Cards {
		if c.Lane != model.LaneDone {
			out = append(out, c)
		}
	}
	return out
}

// Selected returns the card under the cursor, or nil.
func (d Dashboard) Selected() *model.Card {
	if d.View_ == ViewList {
		rows := ListRows(d.groups, d.collapsed)
		if d.listSel < len(rows) {
			return rows[d.listSel].Card
		}
		return nil
	}
	if d.cur.Col >= len(d.board.Columns) {
		return nil
	}
	cell := d.board.Cell(d.cur.Lane, d.board.Columns[d.cur.Col])
	if d.cur.Row < len(cell) {
		return &cell[d.cur.Row]
	}
	return nil
}

func (d Dashboard) Update(msg tea.Msg) (Dashboard, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = msg.Width, msg.Height
	case poller.Snapshot:
		d.snap = msg
		d.relayout()
	case tea.KeyMsg:
		return d.handleKey(msg)
	}
	return d, nil
}

func (d Dashboard) handleKey(k tea.KeyMsg) (Dashboard, tea.Cmd) {
	switch k.String() {
	case "v":
		d.View_ = 1 - d.View_
	case "d":
		d.showDone = !d.showDone
		d.relayout()
	case "r":
		return d, func() tea.Msg { return RefreshMsg{} }
	case "o":
		if c := d.Selected(); c != nil && c.URL != "" {
			u := c.URL
			return d, func() tea.Msg { _ = d.openURL(u); return nil }
		}
	case "enter", " ":
		if d.View_ == ViewList {
			rows := ListRows(d.groups, d.collapsed)
			if d.listSel < len(rows) && rows[d.listSel].Group != nil {
				g := rows[d.listSel].Group.Key
				d.collapsed[g] = !d.collapsed[g]
			}
		}
	case "up", "k":
		d.move(0, -1)
	case "down", "j":
		d.move(0, 1)
	case "left", "h":
		d.move(-1, 0)
	case "right", "l":
		d.move(1, 0)
	}
	return d, nil
}

func (d *Dashboard) move(dx, dy int) {
	if d.View_ == ViewList {
		d.listSel += dy
		d.clamp()
		return
	}
	lanes := visibleLanes(d.board)
	if dx != 0 {
		d.cur.Col += dx
		d.cur.Row = 0
		d.clamp()
		return
	}
	cellLen := func(l model.Lane) int {
		if d.cur.Col >= len(d.board.Columns) {
			return 0
		}
		return len(d.board.Cell(l, d.board.Columns[d.cur.Col]))
	}
	d.cur.Row += dy
	li := indexOf(lanes, d.cur.Lane)
	for d.cur.Row >= cellLen(d.cur.Lane) && li < len(lanes)-1 && dy > 0 {
		li++
		d.cur.Lane, d.cur.Row = lanes[li], 0
	}
	for d.cur.Row < 0 && li > 0 {
		li--
		d.cur.Lane = lanes[li]
		d.cur.Row = cellLen(d.cur.Lane) - 1
	}
	d.clamp()
}

func (d *Dashboard) clamp() {
	d.cur.Col = min(max(d.cur.Col, 0), max(len(d.board.Columns)-1, 0))
	if n := len(d.board.Cell(d.cur.Lane, colName(d.board, d.cur.Col))); d.cur.Row >= n {
		d.cur.Row = max(n-1, 0)
	}
	d.cur.Row = max(d.cur.Row, 0)
	rows := len(ListRows(d.groups, d.collapsed))
	d.listSel = min(max(d.listSel, 0), max(rows-1, 0))
}

func colName(b model.Board, i int) string {
	if i < len(b.Columns) {
		return b.Columns[i]
	}
	return ""
}

func indexOf(ls []model.Lane, l model.Lane) int {
	for i, x := range ls {
		if x == l {
			return i
		}
	}
	return 0
}

func (d Dashboard) View() string {
	var body string
	if d.View_ == ViewList {
		body = RenderList(d.groups, d.collapsed, d.listSel, d.width)
	} else {
		body = RenderKanban(d.board, d.cur, d.width)
	}
	return body + "\n" + d.statusLine()
}

func (d Dashboard) statusLine() string {
	var parts []string
	if d.snap.AuthFailed {
		parts = append(parts, "token rejected - check token_command")
	} else if d.snap.Err != nil {
		parts = append(parts, "⚪ stale: "+d.snap.Err.Error())
	} else if !d.snap.At.IsZero() {
		parts = append(parts, "updated "+d.snap.At.Format("15:04:05"))
	}
	parts = append(parts, "←→↑↓ move  enter detail  t transition  o open  v view  d done  r refresh  m manage  q quit  ? help")
	return dimStyle.Render(strings.Join(parts, "  ·  "))
}

func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}

```

In `TestKanbanNavigationSkipsEmptyCells`, the In Progress column's Mine cell has one card
(ABC-1974). Pressing down moves to the Waiting lane's In Progress cell (ABC-1990). The lane-hop
loop above handles that. If the test fails, fix `move`, not the test.

In list view, `enter` toggles collapse on a group row. On a card row it opens the detail pane
(Task 16).

**Step 4: Run the tests to confirm they pass** → PASS

**Step 5: Commit** → `feat(ui): dashboard navigation, view toggle, done, open`

---

### Task 16: UI: detail pane and transition picker

**Files:**
- Create: `internal/ui/detail.go`, `internal/ui/transition.go`
- Modify: `internal/ui/dashboard.go` (the `enter` key on a card opens the detail pane; `t` opens the picker)
- Test: `internal/ui/transition_test.go`

**Behavior:**
- **Detail** (`enter` on a card in either view; `esc` closes): key, stoplight, status, epic, age,
  assignee, the full summary wrapped to width, `Reasons` as bullets, the last 3 comments (author
  ID + date only; comment bodies aren't in `model.Comment` for v1, so show "N comments, latest by X
  on DATE"), and the URL.
- **Transition picker** (`t`): the dashboard returns a `LoadTransitionsMsg{Key}`, and main runs
  `api.Transitions` in a `tea.Cmd`. When the result arrives, a vertical list of
  `Name → ToName` appears. `enter` asks "Move ABC-1 to Code Review? y/n", and `y` returns
  `DoTransitionMsg{Key, ID}`. Main runs `api.DoTransition`. Success returns `RefreshMsg`. Failure
  shows Jira's error text in the picker, and the card doesn't move.

**Step 1: Write the failing test**

```go
package ui

import (
	"errors"
	"testing"

	"github.com/ericdahl-dev/jira-green/internal/jira"
)

func TestPickerConfirmFlow(t *testing.T) {
	p := NewPicker("ABC-1")
	p, _ = p.Update(TransitionsLoadedMsg{Key: "ABC-1", Transitions: []jira.Transition{
		{ID: "21", Name: "Start review", ToName: "Code Review"},
		{ID: "31", Name: "Done", ToName: "Done"},
	}})
	p, _ = p.Update(key("down"))
	p, _ = p.Update(key("enter"))
	if !p.confirming {
		t.Fatal("enter should ask for confirmation")
	}
	_, cmd := p.Update(key("y"))
	msg := cmd()
	dm, ok := msg.(DoTransitionMsg)
	if !ok || dm.Key != "ABC-1" || dm.TransitionID != "31" {
		t.Fatalf("got %#v", msg)
	}
}

func TestPickerNoCancels(t *testing.T) {
	p := NewPicker("ABC-1")
	p, _ = p.Update(TransitionsLoadedMsg{Key: "ABC-1", Transitions: []jira.Transition{{ID: "21", ToName: "Code Review"}}})
	p, _ = p.Update(key("enter"))
	p, cmd := p.Update(key("n"))
	if p.confirming || cmd != nil {
		t.Fatal("n should cancel confirmation without a command")
	}
}

func TestPickerShowsError(t *testing.T) {
	p := NewPicker("ABC-1")
	p, _ = p.Update(TransitionResultMsg{Key: "ABC-1", Err: errors.New("Field 'resolution' is required")})
	if got := p.View(); !strings.Contains(got, "resolution") {
		t.Fatalf("view %q", got)
	}
}

```

(Import `strings` in the test file.)

**Step 2: Run it to confirm it fails** → `undefined: NewPicker`

**Step 3: Implement `transition.go`**

```go
package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ericdahl-dev/jira-green/internal/jira"
)

type LoadTransitionsMsg struct{ Key string }
type TransitionsLoadedMsg struct {
	Key         string
	Transitions []jira.Transition
	Err         error
}
type DoTransitionMsg struct{ Key, TransitionID string }
type TransitionResultMsg struct {
	Key string
	Err error
}

// ClosePickerMsg tells the dashboard to dismiss the picker.
type ClosePickerMsg struct{}

type Picker struct {
	key        string
	items      []jira.Transition
	sel        int
	confirming bool
	loading    bool
	err        error
}

func NewPicker(key string) Picker { return Picker{key: key, loading: true} }

func (p Picker) Update(msg tea.Msg) (Picker, tea.Cmd) {
	switch msg := msg.(type) {
	case TransitionsLoadedMsg:
		p.loading, p.items, p.err = false, msg.Transitions, msg.Err
	case TransitionResultMsg:
		if msg.Err != nil {
			p.err, p.confirming = msg.Err, false
			return p, nil
		}
		return p, tea.Batch(
			func() tea.Msg { return ClosePickerMsg{} },
			func() tea.Msg { return RefreshMsg{} },
		)
	case tea.KeyMsg:
		if p.confirming {
			switch msg.String() {
			case "y":
				t := p.items[p.sel]
				k := p.key
				return p, func() tea.Msg { return DoTransitionMsg{Key: k, TransitionID: t.ID} }
			case "n", "esc":
				p.confirming = false
			}
			return p, nil
		}
		switch msg.String() {
		case "up", "k":
			p.sel = max(p.sel-1, 0)
		case "down", "j":
			p.sel = min(p.sel+1, max(len(p.items)-1, 0))
		case "enter":
			if len(p.items) > 0 {
				p.confirming = true
			}
		case "esc", "q":
			return p, func() tea.Msg { return ClosePickerMsg{} }
		}
	}
	return p, nil
}

func (p Picker) View() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Transition %s\n\n", p.key)
	if p.loading {
		sb.WriteString("  loading transitions…\n")
	}
	for i, t := range p.items {
		line := fmt.Sprintf("  %s → %s", t.Name, t.ToName)
		if i == p.sel {
			line = selStyle.Render(line)
		}
		sb.WriteString(line + "\n")
	}
	if p.confirming {
		fmt.Fprintf(&sb, "\nMove %s to %s? y/n\n", p.key, p.items[p.sel].ToName)
	}
	if p.err != nil {
		sb.WriteString("\n" + p.err.Error() + "\n")
	}
	sb.WriteString(dimStyle.Render("\n↑↓ choose  enter select  esc cancel"))
	return sb.String()
}
```

**Step 4: Implement `detail.go`** as a `RenderDetail(c model.Card, width int) string` pure
function, and add a golden test `detail_80` for `fxCards()[3]` with
`Reasons: []string{"flagged", "in Code Review 6d (red at 2d)"}`.

**Step 5: Wire into `Dashboard`**: add fields `picker *Picker` and `detail *model.Card`. When
either is set, route keys to it and render it instead of the board. `t` on a selected card sets
`picker` and returns `func() tea.Msg { return LoadTransitionsMsg{Key: c.Key} }`. `ClosePickerMsg`
clears it.

**Step 6: Run the tests to confirm they pass** → `go test ./internal/ui/` PASS

**Step 7: Commit** → `feat(ui): detail pane and transition picker`

---

### Task 17: UI: manage screen (mute)

Port `../coolify-green/internal/ui/manage.go` and its test. It lists epics, and cards under
them, from the latest snapshot. `space` toggles mute and calls `cfg.SetMuted(key, …)`. `esc`
returns to the dashboard. After closing, main triggers a refresh so muted items disappear.

Muted items that are no longer in the snapshot are still listed, from `cfg.Muted`, so you can
unmute them.

**Files:**
- Create: `internal/ui/manage.go`
- Test: `internal/ui/manage_test.go`

**Step 1: Write the failing test**

```go
func TestManageTogglesMute(t *testing.T) {
	var got []string
	m := NewManage(model.ByEpic(fxCards()), []string{"ABC-OLD"}, func(k string, on bool) error {
		if on {
			got = append(got, k)
		}
		return nil
	})
	m, _ = m.Update(key(" ")) // first row = first epic
	if len(got) != 1 {
		t.Fatalf("mute callback not called: %v", got)
	}
	if !strings.Contains(m.View(), "ABC-OLD") {
		t.Error("previously muted key should be listed for unmuting")
	}
}
```

**Step 2 → 5:** Run it and confirm it fails, implement by adapting the coolify-green file, run
it and confirm it passes, then commit `feat(ui): manage screen for muting`.

---

### Task 18: Init wizard

Port `../coolify-green/internal/wizard/wizard.go` (Huh forms). Steps:
1. Site URL (validate that it starts with `https://`).
2. Email.
3. Token source: select `command` / `env var` / `literal`. The default is `command`, with the
   placeholder `security find-generic-password -s jira-green -w`.
4. Verify: `ResolveToken`, then `jira.New(...).Myself`. On failure, show the error and loop back
   to step 3.
5. Board: `Boards()` → Huh select showing `Name (PROJECT)`.
6. `FindFieldID("Flagged")` → store it in `flagged_field` (it may come back empty; that's fine).
7. `config.WriteStarter`. With `--force`, remove the existing file first.

**Files:**
- Create: `internal/wizard/wizard.go`
- Test: `internal/wizard/wizard_test.go`. Test only the non-interactive core: extract
  `func Finish(ctx, api interface{ Myself; FindFieldID }, answers Answers, path string) (*config.Config, error)`
  and test it with a fake API. The Huh forms themselves aren't unit-tested (same as
  coolify-green).

**Steps:** write the `Finish` test with a fake that returns `acct-me` and `customfield_10021`,
and assert that the written file loads with `board_id` and `flagged_field` set. Then run it and
confirm it fails, implement, and run it again to confirm it passes. Commit
`feat(wizard): interactive init`.

---

### Task 19: Wire main

**Files:**
- Modify: `main.go`
- Modify: `main_test.go`

Follow `../coolify-green/main.go`'s top-level `model`: a screen enum (dashboard / manage), a
`waitForSnapshot` loop, and a spinner while the first poll runs.

- `run`: `init [--force]` → wizard. Otherwise load `config.DefaultPath()`. If the file is
  missing, print `no config — run: jira-green init` and exit 1.
- Build `jira.New(site, email, token)`, `poller.New(cfg, api)`, and
  `alert.New(cfg.Webhooks)` + `alert.NewTracker(stuckAfter)`.
- On each `poller.Snapshot`: pass it to the dashboard, call `tracker.Observe`, then dispatch the
  events.
- `RefreshMsg` → non-blocking send on the poller's refresh channel.
- `LoadTransitionsMsg` / `DoTransitionMsg` → run the API call in a `tea.Cmd`, and reply with
  `TransitionsLoadedMsg` / `TransitionResultMsg`.
- `m` → manage screen. `q` / `ctrl+c` → quit. `?` → help overlay.

**Step 1: Write the failing test**

```go
func TestRunMissingConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	if code := run(nil, &out, &out); code != 1 || !strings.Contains(out.String(), "jira-green init") {
		t.Fatalf("code %d out %q", code, out.String())
	}
}
```

**Step 2 → 4:** Run it and confirm it fails, implement, then run `go test ./... && go vet ./...`
and confirm everything passes.

**Step 5: Manual smoke test against real Jira** (the only step that touches employer data; it
stays local):

```bash
go build -o /tmp/jira-green . && /tmp/jira-green init && /tmp/jira-green
```

Check:
- The kanban shows the real board columns.
- Your sprint tickets appear in Mine.
- `v` toggles the view.
- `o` opens a ticket.
- `t` lists real transitions (press `esc` without confirming, unless a ticket really needs to
  move).

**Step 6: Commit** → `feat: wire dashboard, poller, alerts, transitions`

---

### Task 20: README, lint, release

**Files:**
- Create: `README.md` (follow coolify-green's README structure: tagline, ASCII screenshot built
  from the kanban golden, features, install, usage, first-time config, config reference, keys)
- Create: `CONTEXT.md` (glossary: Lane, Column, Card, Stoplight, Stale, Threshold, Muted)

**Steps:**
1. Write the README and CONTEXT.md. The screenshot must use `ABC-` keys only.
2. `golangci-lint run` → fix everything it flags.
3. `go test -race ./...` → PASS.
4. Commit: `docs: README and CONTEXT`.
5. Release (requires the GitHub repo and the `HOMEBREW_TAP_GITHUB_TOKEN` secret, copied from the
   coolify-green repo settings): `git tag v0.1.0 && git push origin v0.1.0`. Then confirm the
   release workflow went green and that `brew install --cask ericdahl-dev/tap/jira-green` works.

---

## Out of scope for v1

- A comment/nudge action, creating issues, editing fields
- git-green cross-linking (a Jira key found in a branch name + PR CI status on the card)
- Multi-site or multi-board configs
- Showing comment bodies in the detail pane
