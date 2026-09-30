package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

// Row is one line of the list view: a group header or a card.
type Row struct {
	Group *model.EpicGroup
	Card  *model.Card
}

// ListRows flattens groups into rows, skipping cards of collapsed groups.
// collapsed is keyed by epic key ("" for No epic).
func ListRows(groups []model.EpicGroup, collapsed map[string]bool) []Row {
	var rows []Row
	for gi := range groups {
		g := &groups[gi]
		rows = append(rows, Row{Group: g})
		if collapsed[g.Key] {
			continue
		}
		for ci := range g.Cards {
			rows = append(rows, Row{Card: &g.Cards[ci]})
		}
	}
	return rows
}

// RenderList draws the epic tree. sel indexes into ListRows.
func RenderList(groups []model.EpicGroup, collapsed map[string]bool, sel, width int) string {
	var sb strings.Builder
	for i, r := range ListRows(groups, collapsed) {
		var line string
		if r.Group != nil {
			line = groupLine(*r.Group, collapsed[r.Group.Key])
		} else {
			line = cardLine(*r.Card, width)
		}
		line = pad(marker(i == sel)+line, width)
		if i == sel {
			line = selStyle.Render(line)
		}
		sb.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	return sb.String()
}

func groupLine(g model.EpicGroup, collapsed bool) string {
	arrow := "▼"
	if collapsed {
		arrow = "▶"
	}
	mine, waiting := 0, 0
	for _, c := range g.Cards {
		switch c.Lane {
		case model.LaneMine:
			mine++
		case model.LaneWaiting:
			waiting++
		}
	}
	return fmt.Sprintf("%s %s %s Mine %d  Waiting %d", arrow, g.Light.Emoji(), pad(g.Name, 28), mine, waiting)
}

func cardLine(c model.Card, width int) string {
	who := ""
	if c.Lane != model.LaneMine && c.AssigneeName != "" {
		who = " @" + firstWord(c.AssigneeName)
	}
	prefix := fmt.Sprintf("    %s %s %s %s ", c.Light.Emoji(), pad(c.Key, 9), pad(c.Column, 12), pad(ageFlag(c), 5))
	return prefix + truncate(c.Summary+who, max(10, width-1-lipgloss.Width(prefix)))
}
