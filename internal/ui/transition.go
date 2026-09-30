package ui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ericdahl-dev/jira-green/internal/jira"
)

// LoadTransitionsMsg asks main to fetch Key's transitions (api.Transitions)
// and reply with TransitionsLoadedMsg.
type LoadTransitionsMsg struct{ Key string }

// TransitionsLoadedMsg is main's reply to LoadTransitionsMsg.
type TransitionsLoadedMsg struct {
	Key         string
	Transitions []jira.Transition
	Err         error
}

// DoTransitionMsg asks main to run api.DoTransition and reply with
// TransitionResultMsg.
type DoTransitionMsg struct{ Key, TransitionID string }

// TransitionResultMsg is main's reply to DoTransitionMsg.
type TransitionResultMsg struct {
	Key string
	Err error
}

// ClosePickerMsg tells the dashboard to dismiss the picker.
type ClosePickerMsg struct{}

// Picker chooses and confirms a transition for one issue.
type Picker struct {
	key        string
	items      []jira.Transition
	sel        int
	loading    bool
	confirming bool
	moving     bool // DoTransitionMsg sent, waiting for its result
	err        error
}

// NewPicker is a picker for key, waiting for its transitions.
func NewPicker(key string) Picker { return Picker{key: key, loading: true} }

// Update handles one message.
func (p Picker) Update(msg tea.Msg) (Picker, tea.Cmd) {
	switch msg := msg.(type) {
	case TransitionsLoadedMsg:
		if msg.Key == p.key {
			p.loading, p.items, p.err = false, msg.Transitions, msg.Err
		}
	case TransitionResultMsg:
		if msg.Key != p.key {
			return p, nil
		}
		if msg.Err == nil {
			return p, tea.Batch(
				func() tea.Msg { return ClosePickerMsg{} },
				func() tea.Msg { return RefreshMsg{} },
			)
		}
		p.moving, p.err = false, msg.Err
	case tea.KeyMsg:
		if p.moving {
			return p, nil
		}
		if p.confirming {
			switch msg.String() {
			case "y":
				p.confirming, p.moving = false, true
				k, id := p.key, p.items[p.sel].ID
				return p, func() tea.Msg { return DoTransitionMsg{Key: k, TransitionID: id} }
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
			p.confirming = len(p.items) > 0
		case "esc", "q":
			return p, func() tea.Msg { return ClosePickerMsg{} }
		}
	}
	return p, nil
}

// View renders the picker.
func (p Picker) View() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Transition %s\n\n", p.key)
	switch {
	case p.loading:
		sb.WriteString("  loading transitions…\n")
	case len(p.items) == 0 && p.err == nil:
		sb.WriteString("  no transitions available\n")
	}
	for i, t := range p.items {
		line := fmt.Sprintf("%s  %s → %s", marker(i == p.sel), t.Name, t.ToName)
		if i == p.sel {
			line = selStyle.Render(line)
		}
		sb.WriteString(line + "\n")
	}
	switch {
	case p.confirming:
		fmt.Fprintf(&sb, "\nMove %s to %s? y/n\n", p.key, p.items[p.sel].ToName)
	case p.moving:
		fmt.Fprintf(&sb, "\nmoving %s to %s…\n", p.key, p.items[p.sel].ToName)
	}
	for _, m := range errorLines(p.err) {
		sb.WriteString("\n" + m)
	}
	if p.err != nil {
		sb.WriteString("\n")
	}
	sb.WriteString("\n" + dimStyle.Render("↑↓ choose  enter select  esc cancel") + "\n")
	return sb.String()
}

// errorLines are Jira's own messages for an APIError that has them (they
// say which field a transition needs), else the error text.
func errorLines(err error) []string {
	var ae *jira.APIError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ae) && len(ae.Messages) > 0:
		return ae.Messages
	default:
		return []string{err.Error()}
	}
}
