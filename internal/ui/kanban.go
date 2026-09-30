// Package ui renders jira-green's board views.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

// Cursor addresses one card on the kanban board. Col indexes b.Columns,
// which includes a trailing Other column when one is in use.
type Cursor struct {
	Lane model.Lane
	Col  int
	Row  int
}

var (
	selStyle  = lipgloss.NewStyle().Reverse(true)
	dimStyle  = lipgloss.NewStyle().Faint(true)
	headStyle = lipgloss.NewStyle().Bold(true)
	errStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
)

// visibleLanes are the lanes the kanban draws, in order. Backlog and Done
// show only when they have cards, and Backlog only when backlog is set.
func visibleLanes(b model.Board, backlog bool) []model.Lane {
	ls := []model.Lane{model.LaneMine, model.LaneWaiting}
	if backlog && b.LaneCount(model.LaneBacklog) > 0 {
		ls = append(ls, model.LaneBacklog)
	}
	if b.LaneCount(model.LaneDone) > 0 {
		ls = append(ls, model.LaneDone)
	}
	return ls
}

// KanbanOptions are the dashboard's view state that RenderKanban draws.
type KanbanOptions struct {
	// BacklogOpen draws the Backlog as a lane of cards. Closed, it is a
	// one-line rule with its light and count.
	BacklogOpen bool
}

// RenderKanban draws the board to fit width.
func RenderKanban(b model.Board, cur Cursor, width int, opt KanbanOptions) string {
	ls, _ := kanbanLines(b, cur, width, opt)
	return strings.Join(ls, "\n") + "\n"
}

// kanbanLines is RenderKanban's lines, and the index of the selected card's
// first line (-1 when no card is selected). Line 0 is the column header.
func kanbanLines(b model.Board, cur Cursor, width int, opt KanbanOptions) ([]string, int) {
	n := len(b.Columns)
	if n == 0 {
		return []string{"no columns"}, -1
	}
	colW := max(minColWidth, (width-1)/n)
	var sb strings.Builder
	cursorLine, line := -1, 0

	var head strings.Builder
	for _, name := range b.Columns {
		head.WriteString(pad(" "+name, colW))
	}
	// Trim before styling: the escape codes would hide the padding.
	sb.WriteString(headStyle.Render(strings.TrimRight(head.String(), " ")) + "\n")
	line++

	for _, lane := range visibleLanes(b, true) {
		if lane == model.LaneBacklog && !opt.BacklogOpen {
			sb.WriteString(backlogRule(b, width) + "\n")
			line++
			continue
		}
		name := lane.String()
		if lane == model.LaneBacklog {
			name = "▼ " + name
		}
		title := fmt.Sprintf("─ %s %s (%d) ", b.LaneLight(lane).Emoji(), name, b.LaneCount(lane))
		sb.WriteString(title + strings.Repeat("─", max(0, width-lipgloss.Width(title))) + "\n")
		line++

		depth := 0
		for _, name := range b.Columns {
			depth = max(depth, len(b.Cell(lane, name)))
		}
		for row := range depth {
			// Each card is three lines: light+key, summary, meta.
			var lines [3]strings.Builder
			for ci, name := range b.Columns {
				cell := b.Cell(lane, name)
				selected := cur.Lane == lane && cur.Col == ci && cur.Row == row && row < len(cell)
				if selected {
					cursorLine = line
				}
				var l [3]string
				if row < len(cell) {
					c := cell[row]
					l[0] = fmt.Sprintf("%s%s %s", marker(selected), c.Light.Emoji(), c.Key)
					l[1] = "  " + truncate(c.DisplaySummary(), colW-3)
					l[2] = "  " + truncate(cardMeta(c), colW-3)
				}
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
			line += len(lines)
		}
		if depth == 0 {
			sb.WriteString(dimStyle.Render("  nothing here") + "\n")
			line++
		}
	}
	return strings.Split(strings.TrimSuffix(sb.String(), "\n"), "\n"), cursorLine
}

// backlogRule is the collapsed Backlog: "─ 🟡 ▶ Backlog (N) ─── b to expand".
func backlogRule(b model.Board, width int) string {
	title := fmt.Sprintf("─ %s ▶ %s (%d) ", b.LaneLight(model.LaneBacklog).Emoji(), model.LaneBacklog, b.LaneCount(model.LaneBacklog))
	const hint = " b to expand"
	fill := max(1, width-lipgloss.Width(title)-lipgloss.Width(hint))
	return truncate(title+strings.Repeat("─", fill)+hint, width)
}

// marker is the selection gutter. It shows the cursor where color is off
// (NO_COLOR, dumb terminals) and reverse video would not.
func marker(selected bool) string {
	if selected {
		return ">"
	}
	return " "
}

// namesAssignee reports whether a view names c's assignee: Mine and Backlog
// cards are mine.
func namesAssignee(c model.Card) bool {
	return c.Lane == model.LaneWaiting && c.AssigneeName != ""
}

// cardMeta is the assignee when the card is not mine, then ageFlag.
func cardMeta(c model.Card) string {
	if namesAssignee(c) {
		return strings.TrimSpace("@" + firstWord(c.AssigneeName) + " " + ageFlag(c))
	}
	return ageFlag(c)
}

// ageFlag is the card's age in status, with a flag mark when flagged. A Done
// card has no age: the poller skips its changelog, so Age would read 0m.
func ageFlag(c model.Card) string {
	var parts []string
	if c.Lane != model.LaneDone {
		parts = append(parts, model.FormatAge(c.Age))
	}
	if c.Flagged {
		parts = append(parts, "⚑")
	}
	return strings.Join(parts, " ")
}

func firstWord(s string) string {
	if i := strings.IndexByte(s, ' '); i > 0 {
		return strings.ToLower(s[:i])
	}
	return strings.ToLower(s)
}

func pad(s string, w int) string {
	s = truncate(s, w)
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

// truncate cuts s to w display columns, ending in ".." when cut. It never
// splits a grapheme.
func truncate(s string, w int) string { return ansi.Truncate(s, w, "..") }
