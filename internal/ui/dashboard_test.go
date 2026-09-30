package ui_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/poller"
	"github.com/ericdahl-dev/jira-green/internal/ui"
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
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

var fxAt = time.Date(2026, 9, 30, 14, 5, 9, 0, time.UTC)

// noOpen fails the test if the dashboard tries to launch a browser.
func noOpen(t *testing.T) func(string) error {
	return func(u string) error { t.Errorf("unexpected browser open %q", u); return nil }
}

func loaded(t *testing.T, view string) ui.Dashboard {
	t.Helper()
	d := ui.NewDashboard(view, noOpen(t))
	d, _ = d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: fxCards(), At: fxAt})
	return d
}

func press(d ui.Dashboard, keys ...string) ui.Dashboard {
	for _, k := range keys {
		d, _ = d.Update(key(k))
	}
	return d
}

func TestVTogglesView(t *testing.T) {
	d := loaded(t, "kanban")
	if d.Mode() != ui.ViewKanban {
		t.Fatal("starts in the configured kanban view")
	}
	if d = press(d, "v"); d.Mode() != ui.ViewList {
		t.Fatal("v switches to list")
	}
	if d = press(d, "v"); d.Mode() != ui.ViewKanban {
		t.Fatal("v switches back to kanban")
	}
	if ui.NewDashboard("list", noOpen(t)).Mode() != ui.ViewList {
		t.Error("default_view list starts in list")
	}
}

func TestViewRendersChosenBoard(t *testing.T) {
	d := loaded(t, "kanban")
	if v := d.View(); !strings.Contains(v, "─ 🔴 Mine (4) ─") || strings.Contains(v, "▼") {
		t.Errorf("kanban view:\n%s", v)
	}
	if v := press(d, "v").View(); !strings.Contains(v, "▼ 🔴 Search") || strings.Contains(v, "Mine (4)") {
		t.Errorf("list view:\n%s", v)
	}
}

// status is the dashboard's last line.
func status(d ui.Dashboard) string {
	ls := lines(d.View())
	return ls[len(ls)-1]
}

func TestStatusLineShowsSyncTime(t *testing.T) {
	if got := status(loaded(t, "kanban")); !strings.HasPrefix(got, "updated 14:05:09") {
		t.Errorf("status %q", got)
	}
	d := ui.NewDashboard("kanban", noOpen(t))
	d, _ = d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if got := status(d); !strings.HasPrefix(got, "never synced") {
		t.Errorf("before the first poll: %q", got)
	}
}

func TestStatusLineShowsStaleAndAuth(t *testing.T) {
	d := loaded(t, "kanban")
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: fxCards(), At: fxAt, Err: errors.New("jira: HTTP 502 Bad Gateway")})
	if got := status(d); !strings.HasPrefix(got, "⚪ stale: jira: HTTP 502 Bad Gateway  ·  updated 14:05:09") {
		t.Errorf("stale status %q", got)
	}
	d, _ = d.Update(poller.Snapshot{Err: errors.New("jira: HTTP 401"), AuthFailed: true})
	if got := status(d); !strings.HasPrefix(got, "token rejected - check token_command  ·  never synced") {
		t.Errorf("auth status %q", got)
	}
}

func TestDTogglesDone(t *testing.T) {
	d := ui.NewDashboard("kanban", noOpen(t))
	d, _ = d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	cards := append(fxCards(), card("ABC-1900", "Shipped thing", "Done", model.LaneDone, model.Green, day, "Auth", "Me"))
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: cards, At: fxAt})

	hidden := func(d ui.Dashboard) bool {
		v := d.View()
		return !strings.Contains(lines(v)[0], "Done") && !strings.Contains(v, "ABC-1900")
	}
	if !hidden(d) {
		t.Fatalf("Done starts hidden:\n%s", d.View())
	}
	d = press(d, "d")
	if v := d.View(); !strings.Contains(lines(v)[0], "Done") || !strings.Contains(v, "Done this sprint (1)") {
		t.Errorf("d shows the Done column and lane:\n%s", v)
	}
	if v := press(d, "v").View(); !strings.Contains(v, "ABC-1900") {
		t.Errorf("the list shows Done cards too:\n%s", v)
	}
	if d = press(d, "d"); !hidden(d) || strings.Contains(press(d, "v").View(), "ABC-1900") {
		t.Errorf("d hides Done again")
	}
}

// run executes cmd and returns its message, or nil for no command.
func run(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestRAsksForRefresh(t *testing.T) {
	_, cmd := loaded(t, "kanban").Update(key("r"))
	if _, ok := run(cmd).(ui.RefreshMsg); !ok {
		t.Fatalf("r returns %#v, want RefreshMsg", run(cmd))
	}
}

func selKey(d ui.Dashboard) string {
	if c := d.Selected(); c != nil {
		return c.Key
	}
	return ""
}

func TestSelectedStartsOnFirstCard(t *testing.T) {
	if got := selKey(loaded(t, "kanban")); got != "ABC-2011" {
		t.Errorf("kanban starts on %q, want ABC-2011 (Mine, To Do, worst first)", got)
	}
	// Mine is empty: the cursor lands on the first card of the next lane.
	d := ui.NewDashboard("kanban", noOpen(t))
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: fxCards()[4:], At: fxAt})
	if got := selKey(d); got != "ABC-1990" {
		t.Errorf("empty Mine lane: starts on %q, want ABC-1990", got)
	}
	if got := selKey(ui.NewDashboard("kanban", noOpen(t))); got != "" {
		t.Errorf("no snapshot selects nothing, got %q", got)
	}
}

// walk presses keys in order and checks the selection after each one.
func walk(t *testing.T, d ui.Dashboard, steps [][2]string) ui.Dashboard {
	t.Helper()
	for i, s := range steps {
		d = press(d, s[0])
		if got := selKey(d); got != s[1] {
			t.Fatalf("step %d (%s): selected %q, want %q", i, s[0], got, s[1])
		}
	}
	return d
}

func TestLeftRightSkipEmptyCells(t *testing.T) {
	walk(t, loaded(t, "kanban"), [][2]string{
		{"right", "ABC-1974"}, {"right", "ABC-1836"},
		{"right", "ABC-1836"}, // Mine has nothing in UA: stay
		{"left", "ABC-1974"}, {"l", "ABC-1836"}, {"h", "ABC-1974"},
	})
	d := ui.NewDashboard("kanban", noOpen(t))
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: fxCards()[4:], At: fxAt}) // Waiting only
	walk(t, d, [][2]string{
		{"right", "ABC-1950"}, // over the empty Code Review cell
		{"left", "ABC-1990"},
		{"left", "ABC-1990"}, // nothing in To Do: stay
	})
}

func TestRightKeepsRowWhereItCan(t *testing.T) {
	walk(t, loaded(t, "kanban"), [][2]string{{"down", "ABC-2020"}, {"right", "ABC-1974"}})
}

func TestUpDownHopLanes(t *testing.T) {
	walk(t, loaded(t, "kanban"), [][2]string{
		{"down", "ABC-2020"},
		{"down", "ABC-2020"}, // Waiting has nothing in To Do: stay
		{"up", "ABC-2011"}, {"up", "ABC-2011"},
		{"right", "ABC-1974"},
		{"down", "ABC-1990"}, // past the last Mine card into Waiting
		{"up", "ABC-1974"},
		{"j", "ABC-1990"}, {"k", "ABC-1974"},
	})
}

func TestDownSkipsAnEmptyLane(t *testing.T) {
	d := ui.NewDashboard("kanban", noOpen(t))
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, At: fxAt, Cards: []model.Card{
		card("ABC-1", "Mine", "To Do", model.LaneMine, model.Green, day, "", "Me"),
		card("ABC-3", "Done lane", "To Do", model.LaneDone, model.Green, day, "", "Me"),
	}})
	walk(t, d, [][2]string{{"d", "ABC-1"}, {"down", "ABC-3"}, {"up", "ABC-1"}})
}

func TestListUpDownMovesRows(t *testing.T) {
	d := walk(t, loaded(t, "list"), [][2]string{
		{"up", ""}, // on the Search header; nothing above it
		{"down", "ABC-1836"}, {"down", ""}, {"down", "ABC-1974"},
		{"right", "ABC-1974"}, {"left", "ABC-1974"}, // no columns in the list
		{"up", ""}, {"k", "ABC-1836"}, {"j", ""},
	})
	d = press(d, "down", "down", "down", "down", "down", "down", "down", "down", "down")
	if got := selKey(d); got != "ABC-2020" {
		t.Errorf("down stops on the last row, got %q", got)
	}
	if !strings.HasPrefix(lines(d.View())[9], ">") {
		t.Errorf("the marker follows the selection:\n%s", d.View())
	}
}

func TestEnterCollapsesListGroup(t *testing.T) {
	d := press(loaded(t, "list"), "down", "down") // Auth header
	d = press(d, "enter")
	v := d.View()
	if !strings.Contains(v, "▶ 🟡 Auth") || strings.Contains(v, "ABC-1974") || strings.Contains(v, "ABC-1990") {
		t.Fatalf("enter collapses Auth:\n%s", v)
	}
	if got := selKey(press(d, "down")); got != "" {
		t.Errorf("the next row is the Accessibility header, got %q", got)
	}
	if v := press(d, "enter").View(); !strings.Contains(v, "▼ 🟡 Auth") || !strings.Contains(v, "ABC-1974") {
		t.Errorf("enter again expands Auth:\n%s", v)
	}
}

func TestOOpensSelectedCardInBrowser(t *testing.T) {
	cards := fxCards()
	cards[0].URL = "https://example.atlassian.net/browse/ABC-2011"
	var opened []string
	d := ui.NewDashboard("kanban", func(u string) error { opened = append(opened, u); return nil })
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: cards, At: fxAt})

	_, cmd := d.Update(key("o"))
	if len(opened) != 0 {
		t.Fatal("Update itself must not launch the browser")
	}
	run(cmd)
	if len(opened) != 1 || opened[0] != cards[0].URL {
		t.Fatalf("opened %v", opened)
	}
	if _, cmd := press(d, "right").Update(key("o")); cmd != nil {
		t.Error("a card without a URL opens nothing")
	}
	if _, cmd := press(d, "v").Update(key("o")); cmd != nil {
		t.Error("a list group header opens nothing")
	}
}

func TestMOpensManage(t *testing.T) {
	_, cmd := loaded(t, "kanban").Update(key("m"))
	msg, ok := run(cmd).(ui.OpenManageMsg)
	if !ok {
		t.Fatalf("m returns %#v, want OpenManageMsg", run(cmd))
	}
	if len(msg.Groups) != 4 || msg.Groups[0].Name != "Search" {
		t.Errorf("OpenManageMsg carries the snapshot's epic groups: %+v", msg.Groups)
	}
}

func TestQQuits(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		_, cmd := loaded(t, "kanban").Update(key(k))
		if _, ok := run(cmd).(tea.QuitMsg); !ok {
			t.Errorf("%s returns %#v, want tea.QuitMsg", k, run(cmd))
		}
	}
}

func TestHelpToggles(t *testing.T) {
	d := press(loaded(t, "kanban"), "?")
	if v := d.View(); !strings.Contains(v, "jira-green keys") || strings.Contains(v, "ABC-2011") {
		t.Fatalf("? shows help instead of the board:\n%s", v)
	}
	if press(d, "v").Mode() != ui.ViewKanban {
		t.Error("board keys do nothing under help")
	}
	for _, k := range []string{"?", "esc"} {
		if v := press(d, k).View(); strings.Contains(v, "jira-green keys") {
			t.Errorf("%s closes help", k)
		}
	}
}

func TestNarrowKanbanFallsBackToList(t *testing.T) {
	d := loaded(t, "kanban")
	// 4 visible columns need 48 terminal columns.
	d, _ = d.Update(tea.WindowSizeMsg{Width: 47, Height: 40})
	ls := lines(d.View())
	if ls[0] != "kanban needs 48 cols - showing list" || !strings.Contains(ls[1], "▼ 🔴 Search") {
		t.Fatalf("narrow kanban shows the list with a hint:\n%s", d.View())
	}
	if d.Mode() != ui.ViewKanban {
		t.Error("the chosen mode stays kanban")
	}
	if got := selKey(press(d, "down")); got != "ABC-1836" {
		t.Errorf("keys drive the list that is shown: selected %q", got)
	}
	for _, l := range ls {
		if lipgloss.Width(l) > 47 {
			t.Errorf("line wider than the terminal: %q", l)
		}
	}

	d, _ = d.Update(tea.WindowSizeMsg{Width: 48, Height: 40})
	if v := d.View(); strings.Contains(v, "showing list") || !strings.Contains(v, "Mine (4)") {
		t.Errorf("48 columns fit kanban again:\n%s", v)
	}
	if got := selKey(d); got != "ABC-2011" {
		t.Errorf("the kanban cursor is where it was: %q", got)
	}
	d, _ = d.Update(tea.WindowSizeMsg{Width: 20, Height: 40})
	if v := press(d, "v").View(); strings.Contains(v, "showing list") {
		t.Errorf("the list view itself needs no hint:\n%s", v)
	}
}

func TestSelectionFollowsCardAcrossPolls(t *testing.T) {
	moved := fxCards()
	moved[2].Column, moved[2].Light = "Code Review", model.Red // ABC-1974 moves on
	moved = append(moved, card("ABC-1000", "New and urgent", "To Do", model.LaneMine, model.Red, 9*day, "Search", "Me"))
	next := poller.Snapshot{Columns: fxCols, Cards: moved, At: fxAt}

	d := press(loaded(t, "kanban"), "right") // ABC-1974
	if d, _ = d.Update(next); selKey(d) != "ABC-1974" {
		t.Errorf("kanban: selected %q after the poll, want ABC-1974", selKey(d))
	}
	d = press(loaded(t, "list"), "down", "down", "down", "down") // ABC-1990
	if d, _ = d.Update(next); selKey(d) != "ABC-1990" {
		t.Errorf("list: selected %q after the poll, want ABC-1990", selKey(d))
	}

	// The selected card is gone: the selection is pulled back onto the rows.
	d = press(loaded(t, "list"), "down", "down", "down", "down", "down", "down", "down", "down") // ABC-1950
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: fxCards()[:2], At: fxAt})
	if got := selKey(d); got != "ABC-2020" {
		t.Errorf("list: selected %q, want the last row, ABC-2020", got)
	}
}

func TestEnterOpensDetailEscCloses(t *testing.T) {
	d := press(loaded(t, "kanban"), "right", "enter") // ABC-1974
	if v := d.View(); !strings.Contains(v, "🟡 ABC-1974  In Progress") || !strings.Contains(v, "Assignee") || strings.Contains(v, "Mine (4)") {
		t.Fatalf("enter shows the card's detail instead of the board:\n%s", v)
	}
	if got := selKey(press(d, "right", "down")); got != "ABC-1974" {
		t.Errorf("board keys do not move the cursor under the detail: %q", got)
	}
	for _, k := range []string{"esc", "q"} {
		if v := press(d, k).View(); !strings.Contains(v, "Mine (4)") {
			t.Errorf("%s returns to the board:\n%s", k, v)
		}
	}
	if _, cmd := d.Update(key("q")); cmd != nil {
		t.Error("q closes the detail rather than quitting")
	}

	d = press(loaded(t, "list"), "down", "enter") // ABC-1836
	if v := d.View(); !strings.Contains(v, "🔴 ABC-1836  Code Review") {
		t.Errorf("enter on a list card shows its detail:\n%s", v)
	}
}

func TestRendersAt80ColumnsBeforeWindowSize(t *testing.T) {
	d := ui.NewDashboard("list", noOpen(t))
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: fxCards(), At: fxAt})
	if v := d.View(); !strings.Contains(v, "ABC-1836") {
		t.Errorf("list before WindowSizeMsg:\n%s", v)
	}
	if v := press(d, "down", "enter").View(); !strings.Contains(v, "Assignee") {
		t.Errorf("detail before WindowSizeMsg:\n%s", v)
	}
}

func TestTOpensPickerAndRoutesIt(t *testing.T) {
	d, cmd := loaded(t, "kanban").Update(key("t"))
	if lm, ok := run(cmd).(ui.LoadTransitionsMsg); !ok || lm.Key != "ABC-2011" {
		t.Fatalf("t returns %#v, want LoadTransitionsMsg for ABC-2011", run(cmd))
	}
	if v := d.View(); !strings.Contains(v, "Transition ABC-2011") || strings.Contains(v, "Mine (4)") {
		t.Fatalf("the picker replaces the board:\n%s", v)
	}
	d, _ = d.Update(ui.TransitionsLoadedMsg{Key: "ABC-2011", Transitions: fxTransitions})
	d = press(d, "down", "enter")
	if !strings.Contains(d.View(), "Move ABC-2011 to Done? y/n") {
		t.Fatalf("keys reach the picker:\n%s", d.View())
	}
	d, cmd = d.Update(key("y"))
	if dm, ok := run(cmd).(ui.DoTransitionMsg); !ok || dm.Key != "ABC-2011" || dm.TransitionID != "31" {
		t.Fatalf("y returns %#v", run(cmd))
	}
	d = feed(d.Update(ui.TransitionResultMsg{Key: "ABC-2011"}))
	if v := d.View(); !strings.Contains(v, "Mine (4)") {
		t.Errorf("success closes the picker:\n%s", v)
	}
	if got := selKey(d); got != "ABC-2011" {
		t.Errorf("the cursor did not move: %q", got)
	}
}

func TestTWithNothingSelected(t *testing.T) {
	if _, cmd := loaded(t, "list").Update(key("t")); cmd != nil {
		t.Errorf("t on a group header returns %#v", run(cmd))
	}
}

func TestTFromDetailReturnsToDetail(t *testing.T) {
	d := press(loaded(t, "kanban"), "enter", "t")
	if !strings.Contains(d.View(), "Transition ABC-2011") {
		t.Fatalf("t in the detail opens the picker:\n%s", d.View())
	}
	if v := feed(d.Update(key("esc"))).View(); !strings.Contains(v, "Assignee") {
		t.Errorf("closing the picker goes back to the detail:\n%s", v)
	}
}

// feed delivers cmd's messages back to d, as Bubble Tea would.
func feed(d ui.Dashboard, cmd tea.Cmd) ui.Dashboard {
	for _, m := range msgs(cmd) {
		d, _ = d.Update(m)
	}
	return d
}

func TestDetailFollowsSnapshot(t *testing.T) {
	d := press(loaded(t, "kanban"), "right", "enter") // ABC-1974
	moved := fxCards()
	moved[2].Column = "Code Review"
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: moved, At: fxAt})
	if v := d.View(); !strings.Contains(v, "🟡 ABC-1974  Code Review") {
		t.Errorf("the detail shows the card's new column:\n%s", v)
	}
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: fxCards()[:2], At: fxAt})
	if v := d.View(); !strings.Contains(v, "ABC-1974") {
		t.Errorf("a card that left the board keeps its last detail:\n%s", v)
	}
}

func TestOInDetailOpensTheCard(t *testing.T) {
	cards := fxCards()
	cards[0].URL = "https://example.atlassian.net/browse/ABC-2011"
	var opened string
	d := ui.NewDashboard("kanban", func(u string) error { opened = u; return nil })
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: cards, At: fxAt})
	_, cmd := press(d, "enter").Update(key("o"))
	if run(cmd); opened != cards[0].URL {
		t.Errorf("opened %q", opened)
	}
}
