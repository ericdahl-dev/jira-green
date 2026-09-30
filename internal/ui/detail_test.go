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
