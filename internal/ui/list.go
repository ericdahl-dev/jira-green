package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

// ListRow is one line of the list view: a group header or a card. Both
// point into the groups passed to ListRows, so a ListRow is valid only until
// the next ByEpic. A row's index (the sel of RenderList) shifts when a group
// above it is collapsed or expanded.
type ListRow struct {
	Group *model.EpicGroup
	Card  *model.Card
}

// ListRows flattens groups into rows, skipping cards of collapsed groups.
// collapsed is keyed by epic key ("" for No epic).
func ListRows(groups []model.EpicGroup, collapsed map[string]bool) []ListRow {
	var rows []ListRow
	for gi := range groups {
		g := &groups[gi]
		rows = append(rows, ListRow{Group: g})
		if collapsed[g.Key] {
			continue
		}
		for ci := range g.Cards {
			rows = append(rows, ListRow{Card: &g.Cards[ci]})
		}
	}
	return rows
}

// ListOptions are the dashboard's view state that RenderList draws.
type ListOptions struct {
	// HiddenBacklog is how many Backlog cards the dashboard left out of
	// groups while Backlog is collapsed. Non-zero adds a hint row at the end.
	HiddenBacklog int
}

// RenderList draws the epic tree. sel indexes into ListRows.
func RenderList(groups []model.EpicGroup, collapsed map[string]bool, sel, width int, opt ListOptions) string {
	cw := listWidths(groups)
	var sb strings.Builder
	for i, r := range ListRows(groups, collapsed) {
		var line string
		if r.Group != nil {
			line = groupLine(*r.Group, collapsed[r.Group.Key])
		} else {
			line = cardLine(*r.Card, cw, width)
		}
		line = pad(marker(i == sel)+line, width)
		if i == sel {
			line = selStyle.Render(line)
		}
		sb.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	if opt.HiddenBacklog > 0 {
		hint := fmt.Sprintf(" ▶ %s (%d hidden) - b to show", model.LaneBacklog, opt.HiddenBacklog)
		sb.WriteString(dimStyle.Render(truncate(hint, width)) + "\n")
	}
	return sb.String()
}

func groupLine(g model.EpicGroup, collapsed bool) string {
	arrow := "▼"
	if collapsed {
		arrow = "▶"
	}
	return fmt.Sprintf("%s %s %s %s", arrow, g.Light.Emoji(), pad(g.Name, 28), laneCounts(g.Cards))
}

// laneCounts is "Mine N  Waiting N  Backlog N", leaving out a zero count.
// Done cards count as none of them.
func laneCounts(cards []model.Card) string {
	n := map[model.Lane]int{}
	for _, c := range cards {
		n[c.Lane]++
	}
	var parts []string
	for _, l := range []struct {
		lane model.Lane
		name string
	}{{model.LaneMine, "Mine"}, {model.LaneWaiting, "Waiting"}, {model.LaneBacklog, "Backlog"}} {
		if n[l.lane] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", l.name, n[l.lane]))
		}
	}
	return strings.Join(parts, "  ")
}

// colWidths are the key and age column widths of the list view.
type colWidths struct{ key, age int }

// listWidths fits the key and age columns to the widest value in any group,
// collapsed or not, so a key or a flagged age is never cut and the columns
// do not shift when a group is toggled.
func listWidths(groups []model.EpicGroup) colWidths {
	w := colWidths{key: 9, age: 5}
	for _, g := range groups {
		for _, c := range g.Cards {
			w.key = max(w.key, lipgloss.Width(c.Key))
			w.age = max(w.age, lipgloss.Width(ageFlag(c)))
		}
	}
	return w
}

func cardLine(c model.Card, cw colWidths, width int) string {
	who := ""
	if namesAssignee(c) {
		who = " @" + firstWord(c.AssigneeName)
	}
	prefix := fmt.Sprintf("    %s %s %s %s ", c.Light.Emoji(), pad(c.Key, cw.key), pad(c.Column, 12), pad(ageFlag(c), cw.age))
	return prefix + truncate(c.Summary+who, max(10, width-1-lipgloss.Width(prefix)))
}
