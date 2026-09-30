package ui_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/ui"
)

type muteCall struct {
	key string
	on  bool
}

// recorder is a setMuted fake that records its calls.
func recorder(calls *[]muteCall) func(string, bool) error {
	return func(k string, on bool) error { *calls = append(*calls, muteCall{k, on}); return nil }
}

func pressManage(m ui.Manage, keys ...string) ui.Manage {
	for _, k := range keys {
		m, _ = m.Update(key(k))
	}
	return m
}

func TestManageSpaceMutesEpic(t *testing.T) {
	var calls []muteCall
	m := ui.NewManage(model.ByEpic(fxCards()), nil, recorder(&calls))
	if v := m.View(); !strings.HasPrefix(v, "> ✓ Search") {
		t.Fatalf("first row is the first epic, watched and selected:\n%s", v)
	}
	m = pressManage(m, " ")
	if len(calls) != 1 || calls[0] != (muteCall{"ABC-E-Search", true}) {
		t.Fatalf("calls %v", calls)
	}
	if v := m.View(); !strings.HasPrefix(v, "> ✗ Search") {
		t.Errorf("the epic shows as muted:\n%s", v)
	}
}

func TestManageMutesCardsNotHeadings(t *testing.T) {
	var calls []muteCall
	m := ui.NewManage(model.ByEpic(fxCards()), nil, recorder(&calls))
	m = pressManage(m, "down", " ") // ABC-1836 under Search
	if len(calls) != 1 || calls[0] != (muteCall{"ABC-1836", true}) {
		t.Fatalf("calls %v", calls)
	}
	ls := lines(m.View())
	if !strings.HasPrefix(ls[1], ">     ✗ ABC-1836  Solr") {
		t.Errorf("card row: %q", ls[1])
	}
	noEpic := lineWith(ls, "No epic")
	m = pressManage(m, "up", "k", "j", "down")
	for range noEpic - 2 {
		m = pressManage(m, "down")
	}
	if ls := lines(m.View()); !strings.HasPrefix(ls[noEpic], ">   No epic") {
		t.Fatalf("on the No epic heading, which has no mark:\n%s", m.View())
	}
	if m = pressManage(m, " "); len(calls) != 1 {
		t.Errorf("space on a heading mutes nothing: %v", calls)
	}
	m = pressManage(m, "down", "down", "down", "down")
	if !strings.HasPrefix(lines(m.View())[noEpic+2], ">") {
		t.Errorf("down stops on the last row:\n%s", m.View())
	}
}

func TestManageListsMutedKeysForUnmuting(t *testing.T) {
	var calls []muteCall
	m := ui.NewManage(model.ByEpic(fxCards()), []string{"ABC-OLD", "ABC-E-Gone"}, recorder(&calls))
	ls := lines(m.View())
	head := lineWith(ls, "Muted")
	if head < 0 || ls[head+1] != "      ✗ ABC-OLD" || ls[head+2] != "      ✗ ABC-E-Gone" || ls[head+3] != "" {
		t.Fatalf("muted keys follow a Muted heading, last:\n%s", m.View())
	}
	for range head + 1 {
		m = pressManage(m, "down")
	}
	m = pressManage(m, " ")
	if len(calls) != 1 || calls[0] != (muteCall{"ABC-OLD", false}) {
		t.Fatalf("space unmutes: %v", calls)
	}
	if !strings.HasPrefix(lines(m.View())[head+1], ">     ✓ ABC-OLD") {
		t.Errorf("an unmuted key stays listed, so it can be muted again:\n%s", m.View())
	}
}

func TestManageSaveError(t *testing.T) {
	fail := func(string, bool) error { return errors.New("write config.toml: permission denied") }
	m := ui.NewManage(model.ByEpic(fxCards()), nil, fail)
	before := m
	m = pressManage(m, " ")
	v := m.View()
	if !strings.HasPrefix(v, "> ✓ Search") || !strings.Contains(v, "⚠ write config.toml: permission denied") {
		t.Errorf("the mark is unchanged and the error shown:\n%s", v)
	}
	if strings.Contains(before.View(), "⚠") || !strings.HasPrefix(before.View(), "> ✓ Search") {
		t.Errorf("an earlier copy is untouched:\n%s", before.View())
	}
}

func TestManageEscGoesBack(t *testing.T) {
	var calls []muteCall
	m := ui.NewManage(model.ByEpic(fxCards()), nil, recorder(&calls))
	for _, k := range []string{"esc", "m", "q"} {
		_, cmd := m.Update(key(k))
		if got := msgs(cmd); len(got) != 1 || got[0] != (ui.BackMsg{}) {
			t.Errorf("%s with nothing changed: %#v, want just BackMsg", k, got)
		}
	}
	_, cmd := pressManage(m, " ").Update(key("esc"))
	got := msgs(cmd)
	if len(got) != 2 || got[0] != (ui.BackMsg{}) || got[1] != (ui.RefreshMsg{}) {
		t.Errorf("after a mute: %#v, want BackMsg then RefreshMsg", got)
	}
}

func TestManageEmpty(t *testing.T) {
	var calls []muteCall
	m := pressManage(ui.NewManage(nil, nil, recorder(&calls)), "down", " ")
	if v := m.View(); !strings.Contains(v, "nothing to mute yet") || len(calls) != 0 {
		t.Errorf("empty manage screen: %q, calls %v", v, calls)
	}
}

func TestManageFitsWidth(t *testing.T) {
	cards := append(fxCards(), card("ABC-3000", strings.Repeat("very long summary ", 10), "To Do", model.LaneMine, model.Green, day, "Search", "Me"))
	m := ui.NewManage(model.ByEpic(cards), nil, nil)
	for _, w := range []int{40, 80} {
		m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		for i, l := range lines(m.View()) {
			if lipgloss.Width(l) > w {
				t.Errorf("width %d: line %d is %d wide: %q", w, i, lipgloss.Width(l), l)
			}
		}
	}
}

func TestManageScrollsToCursor(t *testing.T) {
	m := ui.NewManage(model.ByEpic(many(100)), nil, nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = pressManage(m, slices.Repeat([]string{"down"}, 50)...)
	ls := lines(m.View())
	if len(ls) > 20 {
		t.Errorf("%d lines for a 20-line terminal", len(ls))
	}
	// Row 0 is the Big epic, so 50 downs land on the 50th card.
	if at := lineWith(ls, "ABC-5049"); at < 0 || !strings.HasPrefix(ls[at], ">") {
		t.Errorf("the cursor row is not on screen:\n%s", strings.Join(ls, "\n"))
	}
	if !strings.Contains(ls[len(ls)-1], "esc back") {
		t.Errorf("the key hint stays:\n%s", strings.Join(ls, "\n"))
	}
}
