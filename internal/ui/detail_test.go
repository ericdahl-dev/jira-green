package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/ui"
)

func detailCard() model.Card {
	c := fxCards()[3] // ABC-1836, red, Code Review, epic Search
	c.Summary = "Solr pagination breaks on page 11 when the result set has more than ten thousand hits"
	c.StatusName = "In Review"
	c.URL = "https://example.atlassian.net/browse/ABC-1836"
	c.Reasons = []string{"flagged", "in Code Review 6d (red at 2d)"}
	c.DecodeErrors = []string{"changelog: jira: HTTP 500 Internal Server Error"}
	c.Comments = []model.Comment{
		{AuthorID: "acct-me", Created: time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)},
		{AuthorID: "acct-jsmith", Created: time.Date(2026, 9, 28, 16, 30, 0, 0, time.UTC)},
		{AuthorID: "acct-qa", Created: time.Date(2026, 9, 27, 11, 0, 0, 0, time.UTC)},
	}
	return c
}

func TestDetailGolden(t *testing.T) {
	golden(t, "detail_80", ui.RenderDetail(detailCard(), 80))
}

func TestDetailFitsWidth(t *testing.T) {
	for _, w := range []int{40, 80} {
		for i, l := range lines(ui.RenderDetail(detailCard(), w)) {
			if lipgloss.Width(l) > w {
				t.Errorf("width %d: line %d is %d wide: %q", w, i, lipgloss.Width(l), l)
			}
		}
	}
}

func TestDetailSparseCard(t *testing.T) {
	c := card("ABC-7", "Bare", "To Do", model.LaneWaiting, model.Green, day, "", "")
	v := ui.RenderDetail(c, 80)
	for _, want := range []string{"Assignee  unassigned", "Comments  none"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q:\n%s", want, v)
		}
	}
	for _, not := range []string{"Epic", "Why", "Data incomplete"} {
		if strings.Contains(v, not) {
			t.Errorf("empty section %q shown:\n%s", not, v)
		}
	}
}

func TestSubtaskShowsItsStory(t *testing.T) {
	c := card("ABC-13", "Write tests", "To Do", model.LaneMine, model.Green, day, "", "Me")
	c.ParentKey, c.ParentSummary = "ABC-12", "Checkout flow"
	cs := []model.Card{c}
	views := map[string]string{
		"kanban": ui.RenderKanban(model.Layout(fxCols, cs, false), ui.Cursor{}, 160, ui.KanbanOptions{}),
		"list":   ui.RenderList(model.ByEpic(cs), map[string]bool{}, -1, 160, ui.ListOptions{}),
		"detail": ui.RenderDetail(c, 160),
		"manage": ui.NewManage(model.ByEpic(cs), nil, nil).View(),
	}
	for name, v := range views {
		if !strings.Contains(v, "ABC-12 › Write tests") {
			t.Errorf("%s:\n%s", name, v)
		}
	}
}

func TestDetailShowsDecodeErrorsOnce(t *testing.T) {
	iss := model.Issue{Key: "ABC-9", Summary: "Half decoded", Flagged: true, DecodeErrors: []string{"labels: not an array"}}
	c := model.Evaluate(iss, "To Do", model.LaneMine, model.Rules{}, time.Now(), false)
	v := ui.RenderDetail(c, 80)
	if n := strings.Count(v, "labels: not an array"); n != 1 {
		t.Errorf("decode error shown %d times:\n%s", n, v)
	}
	if !strings.Contains(v, "Data incomplete\n  • labels: not an array") || !strings.Contains(v, "Why\n  • flagged\n\n") {
		t.Errorf("it stays under Data incomplete, and Why keeps the other reasons:\n%s", v)
	}
}

func TestDetailNamesCommentAuthor(t *testing.T) {
	c := detailCard()
	c.Comments[1].AuthorName = "Jane Smith" // the latest
	if v := ui.RenderDetail(c, 80); !strings.Contains(v, "latest by Jane Smith on 2026-09-28") {
		t.Errorf("display name:\n%s", v)
	}
	if v := ui.RenderDetail(detailCard(), 80); !strings.Contains(v, "latest by acct-jsmith on") {
		t.Errorf("no display name falls back to the account ID:\n%s", v)
	}
}

func TestDetailEpicWithoutSummary(t *testing.T) {
	c := detailCard()
	c.EpicSummary = ""
	if v := ui.RenderDetail(c, 80); !strings.Contains(v, "Epic      ABC-E-Search\n") {
		t.Errorf("an epic with no summary is named by its key:\n%s", v)
	}
}
