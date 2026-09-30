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
)

// visibleLanes are the lanes the kanban draws, in order. Done shows as a
// lane only when it has cards.
func visibleLanes(b model.Board) []model.Lane {
	ls := []model.Lane{model.LaneMine, model.LaneWaiting}
	if b.LaneCount(model.LaneDone) > 0 {
		ls = append(ls, model.LaneDone)
	}
	return ls
}

// RenderKanban draws the board to fit width.
func RenderKanban(b model.Board, cur Cursor, width int) string {
	n := len(b.Columns)
	if n == 0 {
		return "no columns"
	}
	colW := max(12, (width-1)/n)
	var sb strings.Builder

	var head strings.Builder
	for _, name := range b.Columns {
		head.WriteString(pad(" "+name, colW))
	}
	// Trim before styling: the escape codes would hide the padding.
	sb.WriteString(headStyle.Render(strings.TrimRight(head.String(), " ")) + "\n")

	for _, lane := range visibleLanes(b) {
		title := fmt.Sprintf("─ %s %s (%d) ", b.LaneLight(lane).Emoji(), lane, b.LaneCount(lane))
		sb.WriteString(title + strings.Repeat("─", max(0, width-lipgloss.Width(title))) + "\n")

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
				var l [3]string
				if row < len(cell) {
					c := cell[row]
					l[0] = fmt.Sprintf("%s%s %s", marker(selected), c.Light.Emoji(), c.Key)
					l[1] = "  " + truncate(c.Summary, colW-3)
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
		}
		if depth == 0 {
			sb.WriteString(dimStyle.Render("  nothing here") + "\n")
		}
	}
	return sb.String()
}

// marker is the selection gutter. It shows the cursor where color is off
// (NO_COLOR, dumb terminals) and reverse video would not.
func marker(selected bool) string {
	if selected {
		return ">"
	}
	return " "
}

// cardMeta is the assignee when the card is not mine, then ageFlag.
func cardMeta(c model.Card) string {
	if c.Lane != model.LaneMine && c.AssigneeName != "" {
		return "@" + firstWord(c.AssigneeName) + " " + ageFlag(c)
	}
	return ageFlag(c)
}

// ageFlag is the card's age in status, with a flag mark when flagged.
func ageFlag(c model.Card) string {
	if c.Flagged {
		return model.FormatAge(c.Age) + " ⚑"
	}
	return model.FormatAge(c.Age)
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
