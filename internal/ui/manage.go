package ui

import (
	"fmt"
	"maps"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

// BackMsg is sent when the user leaves the manage screen.
type BackMsg struct{}

// manageRow is one line of the manage screen. A row with no key is a
// heading and cannot be muted.
type manageRow struct {
	key    string
	label  string
	indent bool
	epic   string // a card row's epic key
}

// Manage is the screen for muting and unmuting epics and cards.
type Manage struct {
	rows     []manageRow
	muted    map[string]bool
	cursor   int
	err      error // the last failed save
	changed  bool  // a mute was saved; the board needs a refresh
	width    int
	height   int
	setMuted func(key string, muted bool) error
}

// NewManage lists groups' epics with their cards. setMuted persists a
// change (main passes cfg.SetMuted).
func NewManage(groups []model.EpicGroup, muted []string, setMuted func(key string, muted bool) error) Manage {
	m := Manage{muted: map[string]bool{}, setMuted: setMuted, width: 80}
	for _, k := range muted {
		m.muted[k] = true
	}
	for _, g := range groups {
		m.rows = append(m.rows, manageRow{key: g.Key, label: g.Name})
		for _, c := range g.Cards {
			m.rows = append(m.rows, manageRow{key: c.Key, label: fmt.Sprintf("%s  %s", c.Key, c.DisplaySummary()), indent: true, epic: g.Key})
		}
	}
	// The poller drops muted issues and epics, so they are not in groups:
	// list them from the config so they can be unmuted.
	shown := map[string]bool{}
	for _, r := range m.rows {
		shown[r.key] = true
	}
	var gone []manageRow
	for _, k := range muted {
		if !shown[k] {
			gone = append(gone, manageRow{key: k, label: k, indent: true})
		}
	}
	if len(gone) > 0 {
		m.rows = append(append(m.rows, manageRow{label: "Muted"}), gone...)
	}
	return m
}

// WithSize sets the terminal size. main builds Manage after the
// WindowSizeMsg was delivered, so it passes OpenManageMsg's size here. A
// zero width keeps the 80-column default.
func (m Manage) WithSize(width, height int) Manage {
	if width > 0 {
		m.width = width
	}
	m.height = height
	return m
}

// Update handles one message.
func (m Manage) Update(msg tea.Msg) (Manage, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		return m, nil
	}
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "up", "k":
		m.cursor = max(m.cursor-1, 0)
	case "down", "j":
		m.cursor = min(m.cursor+1, max(len(m.rows)-1, 0))
	case " ":
		return m.toggle(), nil
	case "esc", "m", "q":
		back := func() tea.Msg { return BackMsg{} }
		if m.changed {
			return m, tea.Batch(back, func() tea.Msg { return RefreshMsg{} })
		}
		return m, back
	}
	return m, nil
}

func (m Manage) toggle() Manage {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].key == "" {
		return m
	}
	r := m.rows[m.cursor]
	on := !m.muted[r.key]
	if m.err = m.setMuted(r.key, on); m.err != nil {
		return m
	}
	m.muted = maps.Clone(m.muted) // earlier copies keep their state
	m.muted[r.key] = on
	m.changed = true
	return m
}

// View renders the manage screen. The rows scroll to fit the terminal
// above the error and key hint, keeping the cursor in view.
func (m Manage) View() string {
	var rows []string
	if len(m.rows) == 0 {
		rows = append(rows, "  nothing to mute yet - wait for the first poll")
	}
	for i, r := range m.rows {
		mark := "✓"
		if m.muted[r.key] {
			mark = "✗"
		}
		line := mark + " " + r.label
		if r.epic != "" && m.muted[r.epic] {
			line += "  (epic muted)"
		}
		if r.key == "" {
			line = "  " + r.label
		}
		if r.indent {
			line = "    " + line
		}
		line = truncate(marker(i == m.cursor)+" "+line, m.width)
		if i == m.cursor {
			line = selStyle.Render(line)
		}
		rows = append(rows, line)
	}
	var tail []string
	if m.err != nil {
		tail = append(tail, "", truncate("  ⚠ "+m.err.Error(), m.width))
	}
	tail = append(tail, "", dimStyle.Render(truncate("↑↓ move  space mute/unmute  esc back", m.width)))
	rows = window(rows, m.cursor, m.cursor+1, m.height-len(tail))
	return strings.Join(append(rows, tail...), "\n") + "\n"
}
