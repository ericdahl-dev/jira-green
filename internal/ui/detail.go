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
	add(strings.Split(ansi.Wrap(c.DisplaySummary(), width, ""), "\n")...)
	add("")

	field := func(name, v string) { add(fmt.Sprintf("%-9s %s", name, v)) }
	if c.StatusName != "" {
		field("Status", c.StatusName)
	}
	switch {
	case c.EpicSummary != "":
		field("Epic", c.EpicSummary)
	case c.EpicKey != "":
		field("Epic", c.EpicKey) // as model.ByEpic names it
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
	bullets("Why", whyReasons(c))
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

// dataIncomplete starts the reason model.Evaluate gives for DecodeErrors.
const dataIncomplete = "data incomplete: "

// whyReasons are c's reasons less the DecodeErrors one: those errors have
// their own Data incomplete section.
func whyReasons(c model.Card) []string {
	var out []string
	for _, r := range c.Reasons {
		if len(c.DecodeErrors) == 0 || !strings.HasPrefix(r, dataIncomplete) {
			out = append(out, r)
		}
	}
	return out
}

// commentSummary is "N comments, latest by X on DATE". X is the author's
// display name, or their account ID when Jira hid the name.
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
	who := latest.AuthorName
	if who == "" {
		who = latest.AuthorID
	}
	return fmt.Sprintf("%d %s, latest by %s on %s", len(cs), noun, who, latest.Created.Format("2006-01-02"))
}
