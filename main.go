// Command jira-green is a terminal dashboard for Jira ticket flow health.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ericdahl-dev/jira-green/internal/alert"
	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/jira"
	"github.com/ericdahl-dev/jira-green/internal/poller"
	"github.com/ericdahl-dev/jira-green/internal/ui"
	"github.com/ericdahl-dev/jira-green/internal/wizard"
)

var version = "dev"

// runWizard is the init wizard; tests replace it.
var runWizard = wizard.RunInteractive

const usage = `jira-green — terminal dashboard for Jira ticket flow health

Usage:
  jira-green            launch the dashboard
  jira-green init       create a config (--force replaces one)
  jira-green --version  print the version
  jira-green --help     show this help
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-v", "version":
			_, _ = fmt.Fprintf(stdout, "jira-green %s\n", version)
			return 0
		case "--help", "-h", "help":
			_, _ = fmt.Fprint(stdout, usage)
			return 0
		}
	}
	path, err := config.DefaultPath()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jira-green: %v\n", err)
		return 1
	}
	if len(args) > 0 && args[0] == "init" {
		return runInit(args[1:], path, stderr)
	}
	cfg, err := config.Load(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			_, _ = fmt.Fprintln(stderr, "no config - run: jira-green init")
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "jira-green: %v\n", err)
		return 1
	}
	for _, w := range cfg.Warnings() {
		_, _ = fmt.Fprintf(stderr, "jira-green: warning: %s\n", w)
	}
	token, err := config.ResolveToken(cfg.Jira)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jira-green: %v\n", err)
		return 1
	}
	if err := runDashboard(cfg, token); err != nil {
		_, _ = fmt.Fprintf(stderr, "jira-green: %v\n", err)
		return 1
	}
	return 0
}

// runDashboard polls Jira and runs the TUI until the user quits. Every quit
// path (ctrl+c, q, an error) cancels the poller through the deferred cancel.
func runDashboard(cfg *config.Config, token string) error {
	api := jira.New(cfg.Jira.Site, cfg.Jira.Email, token)
	p := poller.New(cfg, api)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d := alert.New(cfg.Webhooks)
	statePath := config.StatePath(cfg.Path())
	a := newApp(deps{
		ctx: ctx, cancel: cancel, cfg: cfg, api: api,
		snaps: p.Start(ctx), refresh: p.Refresh, dispatch: d.Dispatch, now: time.Now,
		openURL: ui.OpenBrowser, view: startView(cfg),
		saveView: func(v string) error { return config.SaveState(statePath, config.State{View: v}) },
	})
	_, err := tea.NewProgram(a, tea.WithAltScreen()).Run()
	return err
}

// startView is the view the dashboard opens in: the one remembered in
// state.toml, else default_view.
func startView(cfg *config.Config) string {
	if s := config.LoadState(config.StatePath(cfg.Path())); s.View != "" {
		return s.View
	}
	return cfg.DefaultView()
}

func runInit(args []string, path string, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	force := flags.Bool("force", false, "replace an existing config")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	err := runWizard(context.Background(), path, *force)
	switch {
	case errors.Is(err, wizard.ErrUserAborted):
		return 0
	case err != nil:
		_, _ = fmt.Fprintf(stderr, "jira-green init: %v\n", err)
		return 1
	}
	return 0
}

// transitionAPI is the part of jira.API the model calls directly.
type transitionAPI interface {
	Transitions(ctx context.Context, key string) ([]jira.Transition, error)
	DoTransition(ctx context.Context, key, transitionID string) error
}

// deps is everything the model needs from the outside, so tests can pass
// fakes.
type deps struct {
	ctx      context.Context // the poller's; cancel stops it
	cancel   context.CancelFunc
	cfg      *config.Config
	api      transitionAPI
	snaps    <-chan poller.Snapshot
	refresh  func() // poller.Refresh
	dispatch func(context.Context, alert.Event) error
	now      func() time.Time
	openURL  func(string) error // ui.OpenBrowser
	view     string             // the starting view; "" = cfg.DefaultView()
	saveView func(string) error // remembers the view in state.toml
}

type screen int

const (
	screenDashboard screen = iota
	screenManage
)

// app is the top-level tea.Model: it routes messages to the current
// screen and runs everything slow as a tea.Cmd.
type app struct {
	deps
	screen    screen
	dashboard ui.Dashboard
	manage    ui.Manage
	tracker   *alert.Tracker
	loaded    bool // the first snapshot has arrived
	spinner   spinner.Model
}

func newApp(d deps) app {
	return app{
		deps:      d,
		dashboard: ui.NewDashboard(cmp.Or(d.view, d.cfg.DefaultView()), d.openURL),
		tracker:   alert.NewTracker(d.cfg.StuckAlertAfter()),
		spinner:   spinner.New(spinner.WithSpinner(spinner.MiniDot)),
	}
}

// waitForSnapshot delivers the poller's next snapshot. When the poller has
// stopped (a 401, or quit) the channel is closed and it delivers nothing, so
// the dashboard keeps its last state.
func waitForSnapshot(ch <-chan poller.Snapshot) tea.Cmd {
	return func() tea.Msg {
		snap, ok := <-ch
		if !ok {
			return nil
		}
		return snap
	}
}

func (m app) Init() tea.Cmd { return tea.Batch(waitForSnapshot(m.snaps), m.spinner.Tick) }

func (m app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+c" {
		m.cancel()
		return m, tea.Quit
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Both screens take the size, so neither draws at a stale width.
		var dc, mc tea.Cmd
		m.dashboard, dc = m.dashboard.Update(msg)
		m.manage, mc = m.manage.Update(msg)
		return m, tea.Batch(dc, mc)
	case ui.OpenManageMsg:
		m.screen = screenManage
		m.manage = ui.NewManage(msg.Groups, m.cfg.MutedKeys(), m.cfg.SetMuted).WithSize(msg.Width, msg.Height)
		return m, nil
	case ui.BackMsg:
		m.screen = screenDashboard
		return m, nil
	case tea.KeyMsg:
		if m.screen == screenManage {
			var cmd tea.Cmd
			m.manage, cmd = m.manage.Update(msg)
			return m, cmd
		}
	case spinner.TickMsg:
		if m.loaded {
			return m, nil // stop ticking
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case ui.LoadTransitionsMsg:
		api, ctx, key := m.api, m.ctx, msg.Key
		return m, func() tea.Msg {
			ts, err := api.Transitions(ctx, key)
			return ui.TransitionsLoadedMsg{Key: key, Transitions: ts, Err: err}
		}
	case ui.DoTransitionMsg:
		api, ctx, key, id := m.api, m.ctx, msg.Key, msg.TransitionID
		return m, func() tea.Msg {
			return ui.TransitionResultMsg{Key: key, Err: api.DoTransition(ctx, key, id)}
		}
	case ui.RefreshMsg:
		m.refresh()
		return m, nil
	case ui.ViewChangedMsg:
		save, v := m.saveView, msg.View
		return m, func() tea.Msg {
			// Best effort: a view that is not remembered costs one v press.
			if err := save(v); err != nil {
				slog.Debug("save view failed", "err", err)
			}
			return nil
		}
	case poller.Snapshot:
		snap := msg
		m.loaded = true
		cmds := []tea.Cmd{waitForSnapshot(m.snaps)}
		for _, evt := range m.tracker.ObserveSnapshot(snap.Cards, snap.Err, m.now()) {
			cmds = append(cmds, m.sendAlert(evt))
		}
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(snap)
		return m, tea.Batch(append(cmds, cmd)...)
	}
	var cmd tea.Cmd
	m.dashboard, cmd = m.dashboard.Update(msg)
	return m, cmd
}

// sendAlert posts evt to the webhooks off the UI goroutine. A failure comes
// back as ui.WebhookFailedMsg, which flashes on the status line.
func (m app) sendAlert(evt alert.Event) tea.Cmd {
	dispatch, ctx := m.dispatch, m.ctx
	return func() tea.Msg {
		if err := dispatch(ctx, evt); err != nil {
			return ui.WebhookFailedMsg{Err: err}
		}
		return nil
	}
}

func (m app) View() string {
	if !m.loaded {
		return m.spinner.View() + " loading Jira..."
	}
	if m.screen == screenManage {
		return m.manage.View()
	}
	return m.dashboard.View()
}
