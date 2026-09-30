package ui_test

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/poller"
	"github.com/ericdahl-dev/jira-green/internal/ui"
)

// sized is a dashboard of 40 cards at 120 x h.
func sized(t *testing.T, view string, h int, keys ...string) ui.Dashboard {
	t.Helper()
	d := ui.NewDashboard(view, noOpen(t))
	d, _ = d.Update(tea.WindowSizeMsg{Width: 120, Height: h})
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: many(40), At: fxAt})
	return press(d, keys...)
}

func TestViewsFitTinyHeights(t *testing.T) {
	views := map[string]func(t *testing.T, h int) string{
		"kanban": func(t *testing.T, h int) string { return sized(t, "kanban", h, "down", "down").View() },
		"list":   func(t *testing.T, h int) string { return sized(t, "list", h, "down", "down").View() },
		"detail": func(t *testing.T, h int) string { return sized(t, "kanban", h, "enter").View() },
		"help":   func(t *testing.T, h int) string { return sized(t, "kanban", h, "?").View() },
		"manage": func(t *testing.T, h int) string {
			m := ui.NewManage(model.ByEpic(many(40)), nil, nil).WithSize(80, h)
			return pressManage(m, "down", "down").View()
		},
	}
	for name, view := range views {
		for h := 1; h <= 6; h++ {
			if ls := lines(view(t, h)); len(ls) > h {
				t.Errorf("%s at height %d: %d lines:\n%s", name, h, len(ls), strings.Join(ls, "\n"))
			}
		}
	}
}

func TestHeightOneShowsOnlyTheStatusLine(t *testing.T) {
	ls := lines(sized(t, "kanban", 1).View())
	if len(ls) != 1 || !strings.HasPrefix(ls[0], "updated") {
		t.Errorf("height 1: %q", ls)
	}
}

func TestSmallHeightKeepsTheCursorWithoutMarkers(t *testing.T) {
	d := sized(t, "list", 3, "down", "down", "down", "down", "down")
	ls := lines(d.View())
	if lineWith(ls, selKey(d)) < 0 {
		t.Errorf("the selected %s is off screen:\n%s", selKey(d), strings.Join(ls, "\n"))
	}
	if v := strings.Join(ls, "\n"); strings.Contains(v, "lines above") || strings.Contains(v, "lines below") {
		t.Errorf("no room for markers:\n%s", v)
	}
}

func TestUnknownHeightShowsEverything(t *testing.T) {
	d := ui.NewDashboard("list", noOpen(t))
	d, _ = d.Update(poller.Snapshot{Columns: fxCols, Cards: many(40), At: fxAt})
	if ls := lines(d.View()); len(ls) < 40 {
		t.Errorf("height 0 (not yet known) cut the view to %d lines", len(ls))
	}
}
