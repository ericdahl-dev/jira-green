package ui_test

import (
	"testing"

	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/ui"
)

func TestKanbanGolden(t *testing.T) {
	b := model.Layout(fxCols, append(fxCards(), fxBacklog()...), false)
	for _, w := range []int{80, 160} {
		got := ui.RenderKanban(b, ui.Cursor{Lane: model.LaneMine, Col: 2, Row: 0}, w, ui.KanbanOptions{})
		golden(t, "kanban_"+itoa(w), got)
	}
	got := ui.RenderKanban(b, ui.Cursor{Lane: model.LaneMine, Col: 2, Row: 0}, 80, ui.KanbanOptions{BacklogOpen: true})
	golden(t, "kanban_80_backlog", got)
}

func TestListGolden(t *testing.T) {
	// A long summary, so the 80 and 160 goldens differ.
	cards := append(fxCards(), card("ABC-2030", "Announce the result count and active filters to screen readers after every search", "In Progress", model.LaneMine, model.Green, 2*day, "Accessibility", "Me"))
	groups := model.ByEpic(cards)
	collapsed := map[string]bool{"": true} // No epic
	progress := map[string]model.Progress{"ABC-E-Auth": {Done: 4, Total: 7}, "ABC-E-Search": {Done: 0, Total: 3}}
	for _, w := range []int{80, 160} {
		got := ui.RenderList(groups, collapsed, 1, w, ui.ListOptions{HiddenBacklog: len(fxBacklog()), Progress: progress})
		golden(t, "list_"+itoa(w), got)
	}
}
