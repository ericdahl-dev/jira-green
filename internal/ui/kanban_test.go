package ui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/ui"
	"github.com/muesli/termenv"
)

func TestKanbanOneCard(t *testing.T) {
	cards := []model.Card{card("ABC-1", "Short summary", "To Do", model.LaneMine, model.Green, 2*day, "", "Me")}
	b := model.Layout(fxCols, cards, false)
	ls := lines(ui.RenderKanban(b, ui.Cursor{Lane: model.LaneWaiting}, 80, ui.KanbanOptions{}))

	if !strings.Contains(ls[0], "To Do") || !strings.Contains(ls[0], "UA") || strings.Contains(ls[0], "Done") {
		t.Errorf("header should list visible columns without Done: %q", ls[0])
	}
	if !strings.HasPrefix(ls[1], "─ 🟢 Mine (1) ─") {
		t.Errorf("lane title: %q", ls[1])
	}
	want := []string{" 🟢 ABC-1", "  Short summary", "  2d"}
	for i, w := range want {
		if ls[2+i] != w {
			t.Errorf("card line %d = %q, want %q", i, ls[2+i], w)
		}
	}
	if !strings.HasPrefix(ls[5], "─ 🟢 Waiting on others (0) ─") || strings.TrimSpace(ls[6]) != "nothing here" {
		t.Errorf("empty Waiting lane: %q / %q", ls[5], ls[6])
	}
}

func TestKanbanCardOrderAndColumns(t *testing.T) {
	cards := []model.Card{
		card("ABC-1", "Green one", "To Do", model.LaneMine, model.Green, 5*day, "", "Me"),
		card("ABC-2", "Red one", "To Do", model.LaneMine, model.Red, 1*day, "", "Me"),
		card("ABC-3", "Review one", "Code Review", model.LaneMine, model.Yellow, 1*day, "", "Me"),
	}
	b := model.Layout(fxCols, cards, false)
	ls := lines(ui.RenderKanban(b, ui.Cursor{Lane: model.LaneWaiting}, 80, ui.KanbanOptions{}))

	red, green := lineWith(ls, "ABC-2"), lineWith(ls, "ABC-1")
	if red < 0 || green < 0 || red >= green {
		t.Errorf("worst card first: red at %d, green at %d", red, green)
	}
	if red != lineWith(ls, "ABC-3") {
		t.Errorf("first row of each column shares a line")
	}
	// 80 columns over 4 visible board columns: 19 cells each; Code Review is the third.
	row := ls[red]
	if got := lipgloss.Width(row[:strings.Index(row, "🟡")]); got != 2*19+1 {
		t.Errorf("Code Review card starts at column %d, want %d: %q", got, 2*19+1, row)
	}
}

func TestKanbanMetaLine(t *testing.T) {
	cards := []model.Card{
		card("ABC-1", "Mine and flagged", "To Do", model.LaneMine, model.Red, 6*day, "", "Me"),
		card("ABC-2", "Someone else", "To Do", model.LaneWaiting, model.Yellow, 3*day, "", "Jane Smith"),
		card("ABC-3", "Nobody", "UA", model.LaneWaiting, model.Green, 1*day, "", ""),
	}
	b := model.Layout(fxCols, cards, false)
	ls := lines(ui.RenderKanban(b, ui.Cursor{Lane: model.LaneDone}, 80, ui.KanbanOptions{}))

	if got := strings.TrimSpace(ls[lineWith(ls, "ABC-1")+2]); got != "6d ⚑" {
		t.Errorf("Mine meta = %q, want flag and no assignee", got)
	}
	meta := ls[lineWith(ls, "ABC-2")+2]
	if !strings.HasPrefix(meta, "  @jane 3d") {
		t.Errorf("Waiting meta = %q, want lowercased first name then age", meta)
	}
	// ABC-3's own cell: its key sits 4 columns into the cell (marker, light, space).
	at := lineWith(ls, "ABC-3")
	start := colOf(ls[at], "ABC-3") - 4
	if got := strings.TrimSpace(ansi.Cut(ls[at+2], start, start+19)); got != "1d" {
		t.Errorf("unassigned Waiting card shows only age: %q", got)
	}
}

func TestKanbanTruncatesSummary(t *testing.T) {
	cards := []model.Card{card("ABC-1", "Solr pagination breaks on page 11", "To Do", model.LaneMine, model.Green, day, "", "Me")}
	b := model.Layout(fxCols, cards, false)
	ls := lines(ui.RenderKanban(b, ui.Cursor{Lane: model.LaneWaiting}, 80, ui.KanbanOptions{}))
	if got := ls[lineWith(ls, "ABC-1")+1]; got != "  Solr paginatio.." {
		t.Errorf("summary line = %q", got)
	}
}

func TestKanbanFitsWidth(t *testing.T) {
	cards := append(fxCards(),
		card("ABC-3000", strings.Repeat("very long summary ", 20), "UA", model.LaneMine, model.Red, 400*day, "", "Me"),
		card("ABC-3001", "Unmapped status", "Blocked Upstream Forever", model.LaneWaiting, model.Stale, day, "", "Someonewithaverylongname Last"),
	)
	for _, w := range []int{80, 160} {
		b := model.Layout(fxCols, cards, false)
		for i, l := range lines(ui.RenderKanban(b, ui.Cursor{Lane: model.LaneMine, Col: 2}, w, ui.KanbanOptions{})) {
			if lipgloss.Width(l) > w {
				t.Errorf("width %d: line %d is %d wide: %q", w, i, lipgloss.Width(l), l)
			}
		}
	}
}

func TestKanbanSelection(t *testing.T) {
	b := model.Layout(fxCols, fxCards(), false)
	cur := ui.Cursor{Lane: model.LaneWaiting, Col: 1, Row: 0} // ABC-1990

	ls := lines(ui.RenderKanban(b, cur, 80, ui.KanbanOptions{}))
	if got := strings.Count(strings.Join(ls, "\n"), ">"); got != 1 {
		t.Errorf("want exactly one selection marker, got %d", got)
	}
	if l := ls[lineWith(ls, "ABC-1990")]; !strings.Contains(l, ">🟡 ABC-1990") {
		t.Errorf("marker on the selected card: %q", l)
	}
	if l := ls[lineWith(ls, "ABC-1974")]; strings.Contains(l, ">") {
		t.Errorf("same column in the Mine lane is not selected: %q", l)
	}

	// With color on, the selected card's three lines are reversed, and nothing else.
	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	ls = lines(ui.RenderKanban(b, cur, 80, ui.KanbanOptions{}))
	const rev = "\x1b[7m"
	at := lineWith(ls, "ABC-1990")
	for i, l := range ls {
		want := i >= at && i < at+3
		if strings.Contains(l, rev) != want {
			t.Errorf("line %d reversed=%v, want %v: %q", i, !want, want, l)
		}
	}
}

func TestKanbanTruncateKeepsGraphemes(t *testing.T) {
	// 👩‍💻 is one grapheme of three code points (woman, ZWJ, laptop).
	for _, prefix := range []string{"a", "ab", "abc", "abcd"} {
		sum := prefix + " " + strings.Repeat("👩‍💻", 10)
		cards := []model.Card{card("ABC-1", sum, "To Do", model.LaneMine, model.Green, day, "", "Me")}
		ls := lines(ui.RenderKanban(model.Layout(fxCols, cards, false), ui.Cursor{Lane: model.LaneWaiting}, 80, ui.KanbanOptions{}))
		got := strings.TrimSpace(ls[lineWith(ls, "ABC-1")+1])
		body := strings.TrimSuffix(got, "..")
		if body == got || strings.Count(body, "👩") != strings.Count(body, "💻") {
			t.Errorf("prefix %q: summary cut inside a grapheme: %q", prefix, got)
		}
	}
}

func TestKanbanHeaderHasNoTrailingPadInColor(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	head := lines(ui.RenderKanban(model.Layout(fxCols, fxCards(), false), ui.Cursor{}, 80, ui.KanbanOptions{}))[0]
	if plain := ansi.Strip(head); plain != strings.TrimRight(plain, " ") {
		t.Errorf("header ends in padding: %q", plain)
	}
}

// fxBacklog are Backlog cards: mine, outside the open sprint.
func fxBacklog() []model.Card {
	return []model.Card{
		card("ABC-1700", "Retire the legacy export", "To Do", model.LaneBacklog, model.Yellow, 30*day, "Search", "Me"),
		card("ABC-1710", "Document the reindex job", "To Do", model.LaneBacklog, model.Green, 12*day, "", "Me"),
	}
}

func TestKanbanBacklogCollapsed(t *testing.T) {
	b := model.Layout(fxCols, append(fxCards(), fxBacklog()...), false)
	ls := lines(ui.RenderKanban(b, ui.Cursor{}, 80, ui.KanbanOptions{}))

	at := lineWith(ls, "Backlog")
	if at < 0 || at != len(ls)-1 {
		t.Fatalf("collapsed Backlog is one rule after Waiting:\n%s", strings.Join(ls, "\n"))
	}
	rule := ls[at]
	if !strings.HasPrefix(rule, "─ 🟡 ▶ Backlog (2) ─") || !strings.HasSuffix(rule, "─ b to expand") || lipgloss.Width(rule) != 80 {
		t.Errorf("rule %q", rule)
	}
	if lineWith(ls, "ABC-1700") >= 0 {
		t.Errorf("collapsed Backlog hides its cards")
	}
}

func TestKanbanBacklogOpen(t *testing.T) {
	b := model.Layout(fxCols, append(fxCards(), fxBacklog()...), false)
	ls := lines(ui.RenderKanban(b, ui.Cursor{Lane: model.LaneBacklog}, 80, ui.KanbanOptions{BacklogOpen: true}))

	at := lineWith(ls, "Backlog")
	if at < 0 || !strings.HasPrefix(ls[at], "─ 🟡 ▼ Backlog (2) ─") || strings.Contains(ls[at], "b to expand") {
		t.Fatalf("open Backlog is a lane title with ▼:\n%s", strings.Join(ls, "\n"))
	}
	if at > lineWith(ls, "Waiting") && lineWith(ls, ">🟡 ABC-1700") != at+1 {
		t.Errorf("open Backlog draws its cards, worst first, and can be selected:\n%s", strings.Join(ls, "\n"))
	}
	if strings.Contains(strings.Join(ls, "\n"), "@me") {
		t.Errorf("Backlog cards are mine and name no assignee:\n%s", strings.Join(ls, "\n"))
	}
}

func TestKanbanDoneCardShowsNoAge(t *testing.T) {
	// Done cards skip the changelog, so their age is a meaningless 0.
	cards := []model.Card{card("ABC-1900", "Shipped thing", "Done", model.LaneDone, model.Green, 0, "", "Jane Smith")}
	b := model.Layout(fxCols, cards, true)
	ls := lines(ui.RenderKanban(b, ui.Cursor{Lane: model.LaneMine}, 80, ui.KanbanOptions{}))
	at := lineWith(ls, "ABC-1900")
	if at < 0 {
		t.Fatalf("Done card missing:\n%s", strings.Join(ls, "\n"))
	}
	if got := strings.TrimSpace(ls[at+2]); got != "@jane" {
		t.Errorf("Done meta = %q, want the assignee and no age", got)
	}
}
