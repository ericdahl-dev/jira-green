package ui_test

import (
	"testing"

	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/ui"
)

func TestKanbanGolden(t *testing.T) {
	b := model.Layout(fxCols, fxCards(), false)
	for _, w := range []int{80, 160} {
		got := ui.RenderKanban(b, ui.Cursor{Lane: model.LaneMine, Col: 2, Row: 0}, w)
		golden(t, "kanban_"+itoa(w), got)
	}
}

func TestListGolden(t *testing.T) {
	groups := model.ByEpic(fxCards())
	collapsed := map[string]bool{"ABC-E-Accessibility": true}
	for _, w := range []int{80, 160} {
		got := ui.RenderList(groups, collapsed, 1, w)
		golden(t, "list_"+itoa(w), got)
	}
}
