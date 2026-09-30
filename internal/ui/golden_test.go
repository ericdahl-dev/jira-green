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
