package poller_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/jira"
	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/poller"
)

// fake is a jira.API whose answers the test sets. It is safe for the
// poller's goroutine.
type fake struct {
	mu            sync.Mutex
	byJQL         map[string][]model.Issue
	changelogs    map[string][]model.StatusChange
	changelogHits map[string]int
	boardCalls    int
	err           error // returned by Search when set
}

func newFake() *fake {
	return &fake{byJQL: map[string][]model.Issue{}, changelogs: map[string][]model.StatusChange{}, changelogHits: map[string]int{}}
}

func (f *fake) Myself(context.Context) (jira.User, error) {
	return jira.User{AccountID: "acct-me"}, nil
}

func (f *fake) Search(_ context.Context, jql, _ string) ([]model.Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return f.byJQL[jql], nil
}

func (f *fake) BoardColumns(context.Context, int) ([]model.Column, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.boardCalls++
	return []model.Column{
		{Name: "To Do", StatusIDs: []string{"1"}},
		{Name: "In Progress", StatusIDs: []string{"3"}},
		{Name: "Done", StatusIDs: []string{"5"}},
	}, nil
}

func (f *fake) StatusChanges(_ context.Context, key string) ([]model.StatusChange, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changelogHits[key]++
	return f.changelogs[key], nil
}

func (f *fake) Transitions(context.Context, string) ([]jira.Transition, error) { return nil, nil }
func (f *fake) DoTransition(context.Context, string, string) error             { return nil }

func (f *fake) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// cfg loads a config the way main does, from a temp file. extra is TOML
// placed before the [jira] table.
func cfg(t *testing.T, extra string) *config.Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	body := extra + "\n[jira]\n  site = \"https://example.atlassian.net\"\n  email = \"me@example.com\"\n  board_id = 7\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var t0 = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func newPoller(c *config.Config, f *fake, now *time.Time) *poller.Poller {
	p := poller.New(c, f)
	poller.SetNow(p, func() time.Time { return *now })
	return p
}

func TestPollBuildsLanesAndDedupesMineFirst(t *testing.T) {
	c := cfg(t, "")
	f := newFake()
	both := model.Issue{Key: "ABC-1", StatusID: "3", Created: t0.Add(-200 * time.Hour), Updated: t0}
	f.byJQL[c.MineJQL()] = []model.Issue{both}
	f.byJQL[c.WaitingJQL()] = []model.Issue{both, {Key: "ABC-2", StatusID: "999", Created: t0, Updated: t0}}
	f.byJQL[c.DoneJQL()] = []model.Issue{{Key: "ABC-3", StatusID: "5", Created: t0, Updated: t0}}
	f.changelogs["ABC-1"] = []model.StatusChange{{At: t0.Add(-130 * time.Hour), ToID: "3"}}
	now := t0

	snap := newPoller(c, f, &now).PollOnce(context.Background())
	if snap.Err != nil {
		t.Fatal(snap.Err)
	}
	if len(snap.Cards) != 3 {
		t.Fatalf("want 3 cards (ABC-1 deduped), got %d: %+v", len(snap.Cards), snap.Cards)
	}
	a, b, d := snap.Cards[0], snap.Cards[1], snap.Cards[2]
	if a.Key != "ABC-1" || a.Lane != model.LaneMine || a.Column != "In Progress" || a.Light != model.Red {
		t.Errorf("ABC-1 = %+v", a)
	}
	if b.Key != "ABC-2" || b.Lane != model.LaneWaiting || b.Column != model.OtherColumn {
		t.Errorf("ABC-2 = %+v", b)
	}
	if d.Key != "ABC-3" || d.Lane != model.LaneDone || d.Column != "Done" {
		t.Errorf("ABC-3 = %+v", d)
	}
	if len(snap.Columns) != 3 || !snap.At.Equal(t0) {
		t.Errorf("columns %v at %v", snap.Columns, snap.At)
	}
}

func TestPollCachesChangelogUntilUpdatedChanges(t *testing.T) {
	c := cfg(t, "")
	f := newFake()
	f.byJQL[c.MineJQL()] = []model.Issue{{Key: "ABC-1", StatusID: "3", Created: t0, Updated: t0}}
	now := t0
	p := newPoller(c, f, &now)

	p.PollOnce(context.Background())
	p.PollOnce(context.Background())
	if f.changelogHits["ABC-1"] != 1 {
		t.Fatalf("Updated unchanged: want 1 changelog fetch, got %d", f.changelogHits["ABC-1"])
	}

	f.byJQL[c.MineJQL()] = []model.Issue{{Key: "ABC-1", StatusID: "3", Created: t0, Updated: t0.Add(time.Minute)}}
	p.PollOnce(context.Background())
	if f.changelogHits["ABC-1"] != 2 {
		t.Fatalf("Updated changed: want 2 changelog fetches, got %d", f.changelogHits["ABC-1"])
	}
}

func TestPollErrorKeepsLastCardsAsStale(t *testing.T) {
	c := cfg(t, "")
	f := newFake()
	f.byJQL[c.MineJQL()] = []model.Issue{
		{Key: "ABC-1", StatusID: "1", Created: t0, Updated: t0},
		{Key: "ABC-2", StatusID: "1", Flagged: true, Created: t0, Updated: t0},
	}
	now := t0
	p := newPoller(c, f, &now)
	p.PollOnce(context.Background())

	boom := errors.New("boom")
	f.setErr(boom)
	now = t0.Add(time.Minute)
	snap := p.PollOnce(context.Background())
	if !errors.Is(snap.Err, boom) {
		t.Fatalf("Err = %v, want boom", snap.Err)
	}
	if len(snap.Cards) != 2 || len(snap.Columns) != 3 || !snap.At.Equal(t0) {
		t.Fatalf("want last good cards, columns, and time kept: %+v", snap)
	}
	if snap.Cards[0].Light != model.Stale {
		t.Errorf("green card light = %v, want stale", snap.Cards[0].Light)
	}
	if snap.Cards[1].Light != model.Red {
		t.Errorf("red card light = %v, want red kept", snap.Cards[1].Light)
	}

	snap = p.PollOnce(context.Background())
	if len(snap.Cards) != 2 {
		t.Fatalf("a second failed poll must still keep the cards, got %d", len(snap.Cards))
	}
}

func TestPoll401SetsAuthFailed(t *testing.T) {
	c := cfg(t, "")
	f := newFake()
	f.setErr(&jira.APIError{Status: 401})
	now := t0
	snap := newPoller(c, f, &now).PollOnce(context.Background())
	if !snap.AuthFailed || snap.Err == nil {
		t.Fatalf("want AuthFailed and Err, got %+v", snap)
	}
}

// recv reads one snapshot, failing the test if none arrives promptly.
func recv(t *testing.T, ch <-chan poller.Snapshot) (poller.Snapshot, bool) {
	t.Helper()
	select {
	case s, ok := <-ch:
		return s, ok
	case <-time.After(time.Second):
		t.Fatal("no snapshot within 1s")
		return poller.Snapshot{}, false
	}
}

func TestStartStopsOn401(t *testing.T) {
	c := cfg(t, "")
	f := newFake()
	f.setErr(&jira.APIError{Status: 401})
	now := t0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snaps, _ := newPoller(c, f, &now).Start(ctx)
	if s, ok := recv(t, snaps); !ok || !s.AuthFailed {
		t.Fatalf("first snapshot = %+v (open %v), want AuthFailed", s, ok)
	}
	if _, ok := recv(t, snaps); ok {
		t.Fatal("channel should close after a 401")
	}
}

func TestStartKeepsPollingThrough403(t *testing.T) {
	c := cfg(t, "")
	f := newFake()
	f.byJQL[c.MineJQL()] = []model.Issue{{Key: "ABC-1", StatusID: "1", Created: t0, Updated: t0}}
	now := t0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snaps, refresh := newPoller(c, f, &now).Start(ctx)
	if s, _ := recv(t, snaps); s.Err != nil {
		t.Fatalf("first poll: %v", s.Err)
	}

	f.setErr(&jira.APIError{Status: 403})
	refresh <- struct{}{}
	s, ok := recv(t, snaps)
	var ae *jira.APIError
	if !ok || s.AuthFailed || !errors.As(s.Err, &ae) || ae.Status != 403 {
		t.Fatalf("403 snapshot = %+v (open %v), want Err 403 without AuthFailed", s, ok)
	}
	if len(s.Cards) != 1 || s.Cards[0].Light != model.Stale {
		t.Fatalf("403 should keep the card as stale: %+v", s.Cards)
	}

	refresh <- struct{}{}
	if _, ok := recv(t, snaps); !ok {
		t.Fatal("polling stopped after a 403")
	}
}

// recordWaits makes Start report each wait it asks for instead of sleeping.
// The returned timers never fire; tests advance with the refresh channel.
func recordWaits(p *poller.Poller) <-chan time.Duration {
	waits := make(chan time.Duration, 8)
	poller.SetAfter(p, func(d time.Duration) <-chan time.Time {
		waits <- d
		return nil
	})
	return waits
}

func nextWait(t *testing.T, waits <-chan time.Duration) time.Duration {
	t.Helper()
	select {
	case d := <-waits:
		return d
	case <-time.After(time.Second):
		t.Fatal("Start did not wait within 1s")
		return 0
	}
}

func TestStartWaitsForRetryAfter(t *testing.T) {
	c := cfg(t, "[settings]\n  poll_interval_seconds = 30\n")
	f := newFake()
	now := t0
	p := newPoller(c, f, &now)
	waits := recordWaits(p)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f.setErr(&jira.APIError{Status: 429, RetryAfter: 90 * time.Second})
	snaps, refresh := p.Start(ctx)
	recv(t, snaps)
	if d := nextWait(t, waits); d != 90*time.Second {
		t.Errorf("after a 429 with Retry-After 90s: wait %v, want 90s", d)
	}

	f.setErr(&jira.APIError{Status: 429, RetryAfter: 5 * time.Second})
	refresh <- struct{}{}
	recv(t, snaps)
	if d := nextWait(t, waits); d != 30*time.Second {
		t.Errorf("Retry-After shorter than the interval: wait %v, want 30s", d)
	}
}

func TestPollDropsMutedIssuesAndEpics(t *testing.T) {
	c := cfg(t, `muted = ["ABC-9", "ABC-100"]`)
	f := newFake()
	f.byJQL[c.MineJQL()] = []model.Issue{
		{Key: "ABC-1", EpicKey: "ABC-100", Created: t0, Updated: t0},
		{Key: "ABC-2", Created: t0, Updated: t0},
		{Key: "ABC-9", Created: t0, Updated: t0},
	}
	now := t0
	snap := newPoller(c, f, &now).PollOnce(context.Background())
	if len(snap.Cards) != 1 || snap.Cards[0].Key != "ABC-2" {
		t.Fatalf("want only ABC-2, got %+v", snap.Cards)
	}
}

func TestPollRefreshesBoardColumnsAfterInterval(t *testing.T) {
	c := cfg(t, "[settings]\n  board_refresh_interval_seconds = 600\n")
	f := newFake()
	now := t0
	p := newPoller(c, f, &now)

	p.PollOnce(context.Background())
	now = t0.Add(9 * time.Minute)
	p.PollOnce(context.Background())
	if f.boardCalls != 1 {
		t.Fatalf("before the refresh interval: want 1 board fetch, got %d", f.boardCalls)
	}
	now = t0.Add(10 * time.Minute)
	p.PollOnce(context.Background())
	if f.boardCalls != 2 {
		t.Fatalf("at the refresh interval: want 2 board fetches, got %d", f.boardCalls)
	}
}
