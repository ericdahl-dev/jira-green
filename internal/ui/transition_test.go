package ui_test

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ericdahl-dev/jira-green/internal/jira"
	"github.com/ericdahl-dev/jira-green/internal/ui"
)

var fxTransitions = []jira.Transition{
	{ID: "21", Name: "Start review", ToName: "Code Review"},
	{ID: "31", Name: "Finish", ToName: "Done"},
}

func loadedPicker() ui.Picker {
	p := ui.NewPicker("ABC-1", 80)
	p, _ = p.Update(ui.TransitionsLoadedMsg{Key: "ABC-1", Transitions: fxTransitions})
	return p
}

func TestPickerConfirmFlow(t *testing.T) {
	p := ui.NewPicker("ABC-1", 80)
	if v := p.View(); !strings.Contains(v, "loading") {
		t.Errorf("before the transitions arrive: %q", v)
	}
	p, _ = p.Update(ui.TransitionsLoadedMsg{Key: "ABC-1", Transitions: fxTransitions})
	if v := p.View(); !strings.Contains(v, "Start review → Code Review") || !strings.Contains(v, ">  Start review") {
		t.Errorf("lists transitions, first selected: %q", v)
	}
	p, _ = p.Update(key("down"))
	p, cmd := p.Update(key("enter"))
	if cmd != nil || !strings.Contains(p.View(), "Move ABC-1 to Done? y/n") {
		t.Fatalf("enter asks first: %q", p.View())
	}
	_, cmd = p.Update(key("y"))
	if dm, ok := run(cmd).(ui.DoTransitionMsg); !ok || dm.Key != "ABC-1" || dm.TransitionID != "31" {
		t.Fatalf("y returns %#v", run(cmd))
	}
}

func TestPickerCancel(t *testing.T) {
	for _, k := range []string{"n", "esc"} {
		p, _ := loadedPicker().Update(key("enter"))
		p, cmd := p.Update(key(k))
		if cmd != nil || strings.Contains(p.View(), "? y/n") {
			t.Errorf("%s cancels the confirmation without a command: %q", k, p.View())
		}
		if _, cmd := p.Update(key("y")); cmd != nil {
			t.Errorf("after %s, y does nothing until enter asks again", k)
		}
	}
	for _, k := range []string{"esc", "q"} {
		_, cmd := loadedPicker().Update(key(k))
		if _, ok := run(cmd).(ui.ClosePickerMsg); !ok {
			t.Errorf("%s closes the picker: %#v", k, run(cmd))
		}
	}
}

func TestPickerSendsOnce(t *testing.T) {
	p, _ := loadedPicker().Update(key("enter"))
	p, _ = p.Update(key("y"))
	if !strings.Contains(p.View(), "moving ABC-1 to Code Review…") {
		t.Errorf("waits for Jira: %q", p.View())
	}
	for _, k := range []string{"y", "enter", "down"} {
		if _, cmd := p.Update(key(k)); cmd != nil {
			t.Errorf("%s while moving: %#v", k, run(cmd))
		}
	}
}

func TestPickerShowsJiraError(t *testing.T) {
	p, _ := loadedPicker().Update(key("enter"))
	p, _ = p.Update(key("y"))
	p, cmd := p.Update(ui.TransitionResultMsg{Key: "ABC-1", Err: &jira.APIError{
		Status: 400, Messages: []string{"Field 'resolution' is required", "fixVersions: pick one"},
	}})
	v := p.View()
	if cmd != nil || !strings.Contains(v, "Field 'resolution' is required\n") || !strings.Contains(v, "fixVersions: pick one") {
		t.Errorf("shows each of Jira's messages and stays open: %q", v)
	}
	if strings.Contains(v, "HTTP 400") || strings.Contains(v, "moving") {
		t.Errorf("no status code, no longer moving: %q", v)
	}
	if _, cmd := p.Update(key("enter")); cmd != nil {
		t.Error("enter asks again before retrying")
	}

	// A transport error has no Jira messages: show the error itself.
	p, _ = ui.NewPicker("ABC-1", 80).Update(ui.TransitionsLoadedMsg{Key: "ABC-1", Err: errors.New("dial tcp: connection refused")})
	if v := p.View(); !strings.Contains(v, "dial tcp: connection refused") || strings.Contains(v, "loading") {
		t.Errorf("load error: %q", v)
	}
}

// msgs runs cmd, flattening a tea.Batch, and returns the messages.
func msgs(cmd tea.Cmd) []tea.Msg {
	m := run(cmd)
	if b, ok := m.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, msgs(c)...)
		}
		return out
	}
	if m == nil {
		return nil
	}
	return []tea.Msg{m}
}

func TestPickerSuccessClosesAndRefreshes(t *testing.T) {
	p, _ := loadedPicker().Update(key("enter"))
	p, _ = p.Update(key("y"))
	_, cmd := p.Update(ui.TransitionResultMsg{Key: "ABC-1"})
	got := msgs(cmd)
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
	if _, ok := got[0].(ui.ClosePickerMsg); !ok {
		t.Errorf("first ClosePickerMsg, got %#v", got[0])
	}
	if _, ok := got[1].(ui.RefreshMsg); !ok {
		t.Errorf("then RefreshMsg, got %#v", got[1])
	}
}

func TestPickerIgnoresOtherKeys(t *testing.T) {
	p, _ := ui.NewPicker("ABC-1", 80).Update(ui.TransitionsLoadedMsg{Key: "ABC-2", Transitions: fxTransitions})
	if v := p.View(); !strings.Contains(v, "loading") || strings.Contains(v, "Start review") {
		t.Errorf("transitions for another issue are dropped: %q", v)
	}
	p, _ = loadedPicker().Update(key("enter"))
	p, _ = p.Update(key("y"))
	if _, cmd := p.Update(ui.TransitionResultMsg{Key: "ABC-2"}); cmd != nil {
		t.Errorf("a result for another issue does nothing: %#v", msgs(cmd))
	}
}

func TestPickerNoTransitions(t *testing.T) {
	p, _ := ui.NewPicker("ABC-1", 80).Update(ui.TransitionsLoadedMsg{Key: "ABC-1"})
	if v := p.View(); !strings.Contains(v, "no transitions available") {
		t.Errorf("view %q", v)
	}
	if _, cmd := p.Update(key("enter")); cmd != nil {
		t.Error("enter on an empty list does nothing")
	}
}

func TestPickerIgnoresALateLoad(t *testing.T) {
	// t, esc, t again: the first request's reply arrives after the second's.
	p := pressPicker(loadedPicker(), "down", "enter") // confirming Finish, sel 1
	p, _ = p.Update(ui.TransitionsLoadedMsg{Key: "ABC-1", Transitions: fxTransitions[:1]})
	if v := p.View(); !strings.Contains(v, "Move ABC-1 to Done? y/n") {
		t.Errorf("the late reply changes nothing:\n%s", v)
	}
	_, cmd := p.Update(key("y"))
	if dm, ok := run(cmd).(ui.DoTransitionMsg); !ok || dm.TransitionID != "31" {
		t.Errorf("y moves the confirmed transition: %#v", run(cmd))
	}
}

func pressPicker(p ui.Picker, keys ...string) ui.Picker {
	for _, k := range keys {
		p, _ = p.Update(key(k))
	}
	return p
}

func TestPickerFitsWidth(t *testing.T) {
	long := []jira.Transition{{ID: "41", Name: "Send back to the product owner for another look", ToName: "Needs Clarification From Product"}}
	p := ui.NewPicker("ABC-1", 30)
	p, _ = p.Update(ui.TransitionsLoadedMsg{Key: "ABC-1", Transitions: long})
	p = pressPicker(p, "enter", "y")
	p, _ = p.Update(ui.TransitionResultMsg{Key: "ABC-1", Err: &jira.APIError{Status: 400,
		Messages: []string{"resolution: Field 'resolution' is required when moving to Needs Clarification From Product"}}})
	v := p.View()
	for i, l := range lines(v) {
		if lipgloss.Width(l) > 30 {
			t.Errorf("line %d is %d wide: %q", i, lipgloss.Width(l), l)
		}
	}
	if !strings.Contains(strings.ReplaceAll(v, "\n", " "), "is required when moving") {
		t.Errorf("Jira's message wraps rather than being cut:\n%s", v)
	}
}

func TestPickerClearsErrorOnNewConfirm(t *testing.T) {
	p := pressPicker(loadedPicker(), "enter", "y")
	p, _ = p.Update(ui.TransitionResultMsg{Key: "ABC-1", Err: errors.New("jira: HTTP 409 Conflict")})
	p = pressPicker(p, "down", "enter")
	if v := p.View(); strings.Contains(v, "409") || !strings.Contains(v, "Move ABC-1 to Done? y/n") {
		t.Errorf("the old error goes when a new confirmation starts:\n%s", v)
	}
}

func TestPickerSaysEscIsOffWhileMoving(t *testing.T) {
	p := pressPicker(loadedPicker(), "enter", "y")
	if v := p.View(); !strings.Contains(v, "esc disabled while moving") || strings.Contains(v, "esc cancel") {
		t.Errorf("moving:\n%s", v)
	}
	if v := loadedPicker().View(); strings.Contains(v, "disabled") {
		t.Errorf("not moving:\n%s", v)
	}
}
