package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

// RenderDetail is the detail pane for one card, fit to width.
func RenderDetail(c model.Card, width int) string {
	var ls []string
	add := func(s ...string) { ls = append(ls, s...) }

	add(headStyle.Render(fmt.Sprintf("%s %s  %s", c.Light.Emoji(), c.Key, c.Column)), "")
	add(strings.Split(ansi.Wrap(c.Summary, width, ""), "\n")...)
	add("")

	field := func(name, v string) { add(fmt.Sprintf("%-9s %s", name, v)) }
	if c.StatusName != "" {
		field("Status", c.StatusName)
	}
	if c.EpicKey != "" {
		field("Epic", c.EpicSummary)
	}
	field("Age", ageFlag(c))
	assignee := c.AssigneeName
	if assignee == "" {
		assignee = "unassigned"
	}
	field("Assignee", assignee)
	field("Comments", commentSummary(c.Comments))

	bullets := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		add("", title)
		for _, it := range items {
			add("  • " + it)
		}
	}
	bullets("Why", c.Reasons)
	bullets("Data incomplete", c.DecodeErrors)

	if c.URL != "" {
		add("", c.URL)
	}
	add("", dimStyle.Render("esc back  t transition  o open"))

	for i := range ls {
		ls[i] = truncate(ls[i], width)
	}
	return strings.Join(ls, "\n") + "\n"
}

// commentSummary is "N comments, latest by X on DATE". model.Comment has no
// body or display name in v1, so X is the author's account ID.
func commentSummary(cs []model.Comment) string {
	if len(cs) == 0 {
		return "none"
	}
	latest := cs[0]
	for _, c := range cs[1:] {
		if c.Created.After(latest.Created) {
			latest = c
		}
	}
	noun := "comments"
	if len(cs) == 1 {
		noun = "comment"
	}
	return fmt.Sprintf("%d %s, latest by %s on %s", len(cs), noun, latest.AuthorID, latest.Created.Format("2006-01-02"))
}
