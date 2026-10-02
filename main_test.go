package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ericdahl-dev/jira-green/internal/alert"
	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/jira"
	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/poller"
	"github.com/ericdahl-dev/jira-green/internal/ui"
	"github.com/ericdahl-dev/jira-green/internal/wizard"
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

func TestRunMissingConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	code := run(nil, &out, &out)
	if code != 1 || !strings.Contains(out.String(), "no config - run: jira-green init") {
		t.Fatalf("code %d out %q", code, out.String())
	}
}

// writeConfig writes body as the config under a fresh XDG_CONFIG_HOME.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "jira-green"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "jira-green", "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunInvalidConfig(t *testing.T) {
	writeConfig(t, "[jira]\nsite = \"https://example.atlassian.net\"\n")
	var out bytes.Buffer
	code := run(nil, &out, &out)
	if code != 1 || !strings.Contains(out.String(), "jira.email is required") {
		t.Fatalf("code %d out %q", code, out.String())
	}
}

const validConfig = `[jira]
site = "https://example.atlassian.net"
email = "me@example.com"
token_env = "JIRA_GREEN_TEST_TOKEN"
board_id = 1
`

func TestRunTokenUnresolved(t *testing.T) {
	writeConfig(t, validConfig)
	t.Setenv("JIRA_GREEN_TEST_TOKEN", "")
	var out bytes.Buffer
	code := run(nil, &out, &out)
	if code != 1 || !strings.Contains(out.String(), `token_env "JIRA_GREEN_TEST_TOKEN" is unset`) {
		t.Fatalf("code %d out %q", code, out.String())
	}
}

func TestRunPrintsConfigWarnings(t *testing.T) {
	writeConfig(t, validConfig)
	p := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "jira-green", "config.toml")
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JIRA_GREEN_TEST_TOKEN", "") // stop before the dashboard
	var out bytes.Buffer
	run(nil, &out, &out)
	if !strings.Contains(out.String(), "jira-green: warning: "+p+" is readable by others") {
		t.Errorf("out %q", out.String())
	}
}

// fakeWizard replaces runWizard for one test and records its calls.
type fakeWizard struct {
	calls int
	path  string
	force bool
	err   error
}

func stubWizard(t *testing.T, err error) *fakeWizard {
	t.Helper()
	f := &fakeWizard{err: err}
	old := runWizard
	runWizard = func(_ context.Context, path string, force bool) error {
		f.calls++
		f.path, f.force = path, force
		return f.err
	}
	t.Cleanup(func() { runWizard = old })
	return f
}

func TestRunInitRunsWizard(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	w := stubWizard(t, nil)
	var out bytes.Buffer
	if code := run([]string{"init"}, &out, &out); code != 0 {
		t.Fatalf("exit %d: %q", code, out.String())
	}
	want := filepath.Join(dir, "jira-green", "config.toml")
	if w.calls != 1 || w.path != want || w.force {
		t.Fatalf("wizard %+v, want one call at %s without force", w, want)
	}
}

func TestRunInitForce(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	w := stubWizard(t, nil)
	var out bytes.Buffer
	if code := run([]string{"init", "--force"}, &out, &out); code != 0 || !w.force {
		t.Fatalf("exit %d, wizard %+v", code, w)
	}
}

func TestRunInitAbortedIsQuiet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stubWizard(t, fmt.Errorf("form: %w", wizard.ErrUserAborted))
	var out bytes.Buffer
	if code := run([]string{"init"}, &out, &out); code != 0 || out.Len() != 0 {
		t.Fatalf("exit %d out %q", code, out.String())
	}
}

func TestRunInitFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stubWizard(t, errors.New("list boards: boom"))
	var out bytes.Buffer
	if code := run([]string{"init"}, &out, &out); code != 1 || !strings.Contains(out.String(), "boom") {
		t.Fatalf("exit %d out %q", code, out.String())
	}
}

// fakeAPI is the Jira transitions API with canned replies.
type fakeAPI struct {
	transitions []jira.Transition
	err         error
	moved       []string // "KEY:transitionID"
}

func (f *fakeAPI) Transitions(_ context.Context, key string) ([]jira.Transition, error) {
	return f.transitions, f.err
}

func (f *fakeAPI) DoTransition(_ context.Context, key, id string) error {
	f.moved = append(f.moved, key+":"+id)
	return f.err
}

// harness is an app wired to fakes.
type harness struct {
	m         app
	cfg       *config.Config
	snaps     chan poller.Snapshot
	refreshes int
	canceled  bool
	api       *fakeAPI
	now       time.Time
	mu        sync.Mutex
	events    []alert.Event
	// dispatchErr is what the fake dispatch returns.
	dispatchErr error
	savedViews  []string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	cfg, err := config.New(filepath.Join(t.TempDir(), "config.toml"), config.Jira{
		Site: "https://example.atlassian.net", Email: "me@example.com", TokenEnv: "JIRA_API_TOKEN", BoardID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{cfg: cfg, snaps: make(chan poller.Snapshot, 4), api: &fakeAPI{}, now: time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)}
	h.m = newApp(deps{
		ctx:     context.Background(),
		cancel:  func() { h.canceled = true },
		cfg:     cfg,
		api:     h.api,
		snaps:   h.snaps,
		refresh: func() { h.refreshes++ },
		dispatch: func(_ context.Context, e alert.Event) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.events = append(h.events, e)
			return h.dispatchErr
		},
		now:     func() time.Time { return h.now },
		openURL: func(string) error { return nil },
		saveView: func(v string) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.savedViews = append(h.savedViews, v)
			return nil
		},
	})
	return h
}

// send feeds msg to the model and returns the command it produced.
func (h *harness) send(msg tea.Msg) tea.Cmd {
	next, cmd := h.m.Update(msg)
	h.m = next.(app)
	return cmd
}

// msgs runs cmd, flattening batches, and returns the messages that arrive
// within a short wait. A command that blocks (a tick, an empty channel) is
// left behind.
func msgs(cmd tea.Cmd) []tea.Msg {
	out := make(chan tea.Msg, 16)
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		go func() {
			msg := c()
			if b, ok := msg.(tea.BatchMsg); ok {
				for _, c := range b {
					run(c)
				}
				return
			}
			out <- msg
		}()
	}
	run(cmd)
	var got []tea.Msg
	timeout := time.After(50 * time.Millisecond)
	for {
		select {
		case m := <-out:
			got = append(got, m)
		case <-timeout:
			return got
		}
	}
}

// has reports whether cmd yields a message of type T.
func has[T any](cmd tea.Cmd) (T, bool) {
	for _, m := range msgs(cmd) {
		if v, ok := m.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

func ctrlC() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlC} }

func TestCtrlCQuitsAndStopsPolling(t *testing.T) {
	h := newHarness(t)
	cmd := h.send(ctrlC())
	if _, ok := has[tea.QuitMsg](cmd); !ok || !h.canceled {
		t.Fatalf("quit %v canceled %v", ok, h.canceled)
	}
}

var testCols = []model.Column{{Name: "In Progress", StatusIDs: []string{"3"}}, {Name: "Code Review", StatusIDs: []string{"4"}}, {Name: "Done", StatusIDs: []string{"5"}}}

// redCard is a synthetic Mine card stuck in Code Review.
func redCard() model.Card {
	return model.Card{
		Issue:  model.Issue{Key: "ABC-1836", Summary: "Solr pagination breaks on page 11", StatusID: "4", StatusName: "Code Review", URL: "https://example.atlassian.net/browse/ABC-1836"},
		Column: "Code Review", Lane: model.LaneMine, Light: model.Red, Reasons: []string{"in Code Review 6d"},
	}
}

func TestSnapshotReachesDashboard(t *testing.T) {
	h := newHarness(t)
	h.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{redCard()}, At: h.now})
	if v := h.m.View(); !strings.Contains(v, "ABC-1836") {
		t.Fatalf("view missing the card:\n%s", v)
	}
}

// dispatched waits briefly for n dispatched events and returns them.
func (h *harness) dispatched(n int) []alert.Event {
	deadline := time.Now().Add(time.Second)
	for {
		h.mu.Lock()
		got := append([]alert.Event(nil), h.events...)
		h.mu.Unlock()
		if len(got) >= n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(time.Millisecond)
	}
}

func TestStuckCardIsDispatched(t *testing.T) {
	h := newHarness(t)
	h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{redCard()}, At: h.now})
	h.now = h.now.Add(2 * time.Hour) // the default stuck_alert_after
	msgs(h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{redCard()}, At: h.now}))
	got := h.dispatched(1)
	if len(got) != 1 || got[0].Key != "ABC-1836" || got[0].Type != alert.TypeTicketStuck {
		t.Fatalf("dispatched %+v", got)
	}
}

func TestWebhookFailureReachesDashboard(t *testing.T) {
	h := newHarness(t)
	h.dispatchErr = errors.New("hooks.example.com: 500")
	h.send(tea.WindowSizeMsg{Width: 200, Height: 40})
	h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{redCard()}, At: h.now})
	h.now = h.now.Add(2 * time.Hour)
	failed, ok := has[ui.WebhookFailedMsg](h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{redCard()}, At: h.now}))
	if !ok {
		t.Fatal("a failed dispatch sent no WebhookFailedMsg")
	}
	h.send(failed)
	if v := h.m.View(); !strings.Contains(v, "webhook failed: hooks.example.com: 500") {
		t.Errorf("status line lacks the failure:\n%s", v)
	}
}

func TestStaleSnapshotPausesStuckAlerts(t *testing.T) {
	h := newHarness(t)
	h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{redCard()}, At: h.now})
	h.now = h.now.Add(2 * time.Hour)
	stale := redCard()
	stale.Light = model.Worst(stale.Light, model.Stale)
	msgs(h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{stale}, At: h.now.Add(-2 * time.Hour), Err: errors.New("jira: HTTP 502")}))
	if got := h.dispatched(0); len(got) != 0 {
		t.Fatalf("a stale poll fired %+v", got)
	}
}

func TestSnapshotsKeepFlowing(t *testing.T) {
	h := newHarness(t)
	first := poller.Snapshot{Columns: testCols, At: h.now}
	h.snaps <- first
	snap, ok := has[poller.Snapshot](h.m.Init())
	if !ok {
		t.Fatal("Init does not wait for a snapshot")
	}
	second := poller.Snapshot{Columns: testCols, At: h.now.Add(time.Minute)}
	h.snaps <- second
	if got, ok := has[poller.Snapshot](h.send(snap)); !ok || !got.At.Equal(second.At) {
		t.Fatalf("a snapshot does not wait for the next one: %v %v", ok, got.At)
	}
}

func TestPollerStopsAfter401(t *testing.T) {
	h := newHarness(t)
	h.send(tea.WindowSizeMsg{Width: 200, Height: 40})
	h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{redCard()}, At: h.now})
	close(h.snaps)
	cmd := h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{redCard()}, At: h.now, Err: errors.New("jira: HTTP 401"), AuthFailed: true})
	for _, msg := range msgs(cmd) {
		if msg != nil {
			h.send(msg)
		}
	}
	v := h.m.View()
	if !strings.Contains(v, "token rejected") || !strings.Contains(v, "ABC-1836") {
		t.Fatalf("view after 401:\n%s", v)
	}
}

func TestSpinnerUntilFirstSnapshot(t *testing.T) {
	h := newHarness(t)
	if v := h.m.View(); !strings.Contains(v, "loading Jira") {
		t.Fatalf("before the first poll:\n%s", v)
	}
	if _, ok := has[spinner.TickMsg](h.m.Init()); !ok {
		t.Fatal("Init does not start the spinner")
	}
	h.send(poller.Snapshot{Columns: testCols, At: h.now})
	if v := h.m.View(); strings.Contains(v, "loading Jira") {
		t.Fatalf("after the first poll:\n%s", v)
	}
}

func TestRefreshMsgPolls(t *testing.T) {
	h := newHarness(t)
	h.send(ui.RefreshMsg{})
	if h.refreshes != 1 {
		t.Fatalf("refreshes %d", h.refreshes)
	}
}

func TestLoadTransitionsRoundTrip(t *testing.T) {
	h := newHarness(t)
	h.api.transitions = []jira.Transition{{ID: "31", Name: "Start review"}}
	got, ok := has[ui.TransitionsLoadedMsg](h.send(ui.LoadTransitionsMsg{Key: "ABC-1836"}))
	if !ok || got.Key != "ABC-1836" || len(got.Transitions) != 1 || got.Transitions[0].ID != "31" || got.Err != nil {
		t.Fatalf("reply %+v ok %v", got, ok)
	}
}

func TestDoTransitionRoundTrip(t *testing.T) {
	h := newHarness(t)
	h.api.err = errors.New("jira: HTTP 400 transition not allowed")
	got, ok := has[ui.TransitionResultMsg](h.send(ui.DoTransitionMsg{Key: "ABC-1836", TransitionID: "31"}))
	if !ok || got.Key != "ABC-1836" || got.Err == nil || len(h.api.moved) != 1 || h.api.moved[0] != "ABC-1836:31" {
		t.Fatalf("reply %+v ok %v moved %v", got, ok, h.api.moved)
	}
}

func key(s string) tea.KeyMsg {
	if s == " " {
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	}
	if s == "esc" {
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestManageMutesAndGoesBack(t *testing.T) {
	h := newHarness(t)
	card := redCard()
	card.EpicKey, card.EpicSummary = "ABC-1800", "Search"
	h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{card}, At: h.now})
	h.send(ui.OpenManageMsg{Groups: model.ByEpic([]model.Card{card}), Width: 100, Height: 30})
	if v := h.m.View(); !strings.Contains(v, "space mute/unmute") {
		t.Fatalf("manage not shown:\n%s", v)
	}
	h.send(key(" ")) // the epic row
	if got := h.cfg.MutedKeys(); len(got) != 1 || got[0] != "ABC-1800" {
		t.Fatalf("muted %v", got)
	}
	cmd := h.send(key("esc"))
	back, ok := has[ui.BackMsg](cmd)
	if !ok {
		t.Fatal("esc does not leave manage")
	}
	if _, ok := has[ui.RefreshMsg](cmd); !ok {
		t.Fatal("leaving after a mute does not refresh")
	}
	h.send(back)
	if v := h.m.View(); strings.Contains(v, "space mute/unmute") || !strings.Contains(v, "ABC-1836") {
		t.Fatalf("dashboard not back:\n%s", v)
	}
}

func TestCtrlCQuitsFromManage(t *testing.T) {
	h := newHarness(t)
	h.send(poller.Snapshot{Columns: testCols, At: h.now})
	h.send(ui.OpenManageMsg{Width: 100, Height: 30})
	if _, ok := has[tea.QuitMsg](h.send(key("q"))); ok {
		t.Fatal("q on manage quits the app; it should go back")
	}
	h.send(ui.OpenManageMsg{Width: 100, Height: 30})
	if _, ok := has[tea.QuitMsg](h.send(ctrlC())); !ok || !h.canceled {
		t.Fatalf("ctrl+c on manage: canceled %v", h.canceled)
	}
}

func TestVSavesTheView(t *testing.T) {
	h := newHarness(t)
	h.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	h.send(poller.Snapshot{Columns: testCols, Cards: []model.Card{redCard()}, At: h.now})
	changed, ok := has[ui.ViewChangedMsg](h.send(key("v")))
	if !ok {
		t.Fatal("v sent no ViewChangedMsg")
	}
	msgs(h.send(changed))
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.savedViews) != 1 || h.savedViews[0] != "list" {
		t.Errorf("saved %q, want [list]", h.savedViews)
	}
}

func TestStartViewPrefersRememberedView(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.New(filepath.Join(dir, "config.toml"), config.Jira{
		Site: "https://example.atlassian.net", Email: "me@example.com", TokenEnv: "JIRA_API_TOKEN", BoardID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := startView(cfg); got != "kanban" {
		t.Errorf("no state: %q, want default_view kanban", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.toml"), []byte("view = \"list\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := startView(cfg); got != "list" {
		t.Errorf("remembered list: got %q", got)
	}
}
