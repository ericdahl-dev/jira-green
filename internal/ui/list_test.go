package ui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/ui"
)

func TestListRows(t *testing.T) {
	groups := model.ByEpic(fxCards())
	rows := ui.ListRows(groups, map[string]bool{})
	// 4 groups (Search, Auth, Accessibility, No epic) + 6 cards
	if len(rows) != 10 {
		t.Fatalf("rows %d, want 10", len(rows))
	}
	if rows[0].Group == nil || rows[0].Group.Name != "Search" || rows[1].Card == nil || rows[1].Card.Key != "ABC-1836" {
		t.Errorf("row 0 is the Search header, row 1 its first card: %+v %+v", rows[0], rows[1])
	}

	rows = ui.ListRows(groups, map[string]bool{"ABC-E-Auth": true, "": true})
	var keys []string
	for _, r := range rows {
		if r.Card != nil {
			keys = append(keys, r.Card.Key)
		}
	}
	if len(rows) != 6 || len(keys) != 2 || keys[0] != "ABC-1836" || keys[1] != "ABC-2011" {
		t.Errorf("collapsed Auth and No epic keep their headers and hide their cards: %d rows, cards %v", len(rows), keys)
	}
}

func TestListGroupHeader(t *testing.T) {
	cards := append(fxCards(),
		card("ABC-1900", "Shipped", "Done", model.LaneDone, model.Green, day, "Auth", "Me"),
		card("ABC-1901", "Invoice totals", "UA", model.LaneWaiting, model.Green, day, "Billing", "Jane Smith"),
		card("ABC-1902", "Invoice dates", "UA", model.LaneWaiting, model.Green, day, "Billing", "Jane Smith"),
	)
	groups := model.ByEpic(cards)
	ls := lines(ui.RenderList(groups, map[string]bool{"ABC-E-Accessibility": true}, -1, 80, ui.ListOptions{}))

	auth := ls[lineWith(ls, "Auth")]
	if !strings.HasPrefix(auth, " ▼ 🟡 Auth ") || !strings.HasSuffix(auth, " Mine 1  Waiting 1") {
		t.Errorf("Auth header (Done card counts as neither): %q", auth)
	}
	if s := ls[lineWith(ls, "Search")]; !strings.HasSuffix(s, " Mine 1") || strings.Contains(s, "Waiting") {
		t.Errorf("zero Waiting count is hidden: %q", s)
	}
	if acc := ls[lineWith(ls, "Accessibility")]; !strings.HasPrefix(acc, " ▶ 🟢 Accessibility ") {
		t.Errorf("collapsed header: %q", acc)
	}
	if lineWith(ls, "ABC-2011") >= 0 {
		t.Errorf("collapsed group hides its cards")
	}
	waitOnly := ls[lineWith(ls, "Billing")]
	if !strings.HasSuffix(waitOnly, " Waiting 2") || strings.Contains(waitOnly, "Mine") {
		t.Errorf("zero Mine count is hidden: %q", waitOnly)
	}
	if colOf(waitOnly, "Waiting") != colOf(auth, "Mine") {
		t.Errorf("a lone Waiting count starts where Mine would:\n%s", strings.Join(ls, "\n"))
	}
	// Counts line up whatever the epic name's length.
	if colOf(auth, "Mine") != colOf(ls[lineWith(ls, "No epic")], "Mine") {
		t.Errorf("counts are not aligned:\n%s", strings.Join(ls, "\n"))
	}
}

func TestListCardRow(t *testing.T) {
	ls := lines(ui.RenderList(model.ByEpic(fxCards()), map[string]bool{}, -1, 160, ui.ListOptions{}))

	want := "     🔴 ABC-1836  Code Review  6d ⚑  Solr pagination breaks on page 11"
	if got := ls[lineWith(ls, "ABC-1836")]; got != want {
		t.Errorf("card row\n got %q\nwant %q", got, want)
	}
	if got := ls[lineWith(ls, "ABC-1990")]; !strings.HasSuffix(got, "Harden session cookie @jane") {
		t.Errorf("Waiting card names its assignee: %q", got)
	}
	if got := ls[lineWith(ls, "ABC-1974")]; strings.Contains(got, "@") {
		t.Errorf("Mine card has no assignee: %q", got)
	}
	// Every card's summary starts in the same column.
	at := colOf(ls[lineWith(ls, "ABC-1836")], "Solr")
	if at < 0 {
		t.Fatal("no summary on the ABC-1836 row")
	}
	for key, word := range map[string]string{"ABC-1974": "Fix", "ABC-2020": "Update", "ABC-1950": "Verify"} {
		if got := colOf(ls[lineWith(ls, key)], word); got != at {
			t.Errorf("%s summary at column %d, want %d", key, got, at)
		}
	}
}

func TestListSelection(t *testing.T) {
	groups := model.ByEpic(fxCards())
	sel := -1
	for i, r := range ui.ListRows(groups, map[string]bool{}) {
		if r.Card != nil && r.Card.Key == "ABC-1990" {
			sel = i
		}
	}
	ls := lines(ui.RenderList(groups, map[string]bool{}, sel, 80, ui.ListOptions{}))
	if got := strings.Count(strings.Join(ls, "\n"), ">"); got != 1 || !strings.HasPrefix(ls[sel], ">") || lineWith(ls, "ABC-1990") != sel {
		t.Errorf("one marker, on row %d (ABC-1990):\n%s", sel, strings.Join(ls, "\n"))
	}

	lipgloss.SetColorProfile(termenv.ANSI)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	for i, l := range lines(ui.RenderList(groups, map[string]bool{}, sel, 80, ui.ListOptions{})) {
		if strings.Contains(l, "\x1b[7m") != (i == sel) {
			t.Errorf("row %d reversed=%v, want only row %d: %q", i, i != sel, sel, l)
		}
	}
}

func TestListFitsWidth(t *testing.T) {
	cards := append(fxCards(),
		card("ABC-3000", strings.Repeat("very long summary ", 20), "Blocked Upstream Forever", model.LaneWaiting, model.Stale, 400*day, strings.Repeat("Epic ", 20), "Someonewithaverylongname Last"),
	)
	groups := model.ByEpic(cards)
	for _, w := range []int{80, 160} {
		for i, l := range lines(ui.RenderList(groups, map[string]bool{}, 1, w, ui.ListOptions{})) {
			if lipgloss.Width(l) > w {
				t.Errorf("width %d: line %d is %d wide: %q", w, i, lipgloss.Width(l), l)
			}
		}
	}
	ls := lines(ui.RenderList(groups, map[string]bool{}, -1, 80, ui.ListOptions{}))
	if got := ls[lineWith(ls, "ABC-3000")]; lipgloss.Width(got) != 80 || !strings.HasSuffix(got, "..") {
		t.Errorf("long summary is cut to 80 columns with ..: %d %q", lipgloss.Width(got), got)
	}
}

func TestListWideKeyAndAge(t *testing.T) {
	cards := append(fxCards(),
		card("ABC-1234567", "Long key", "To Do", model.LaneMine, model.Red, 400*day, "Auth", "Me"),
	)
	ls := lines(ui.RenderList(model.ByEpic(cards), map[string]bool{}, -1, 160, ui.ListOptions{}))

	i := lineWith(ls, "ABC-1234567")
	if i < 0 {
		t.Fatalf("key was cut:\n%s", strings.Join(ls, "\n"))
	}
	row := ls[i]
	if !strings.Contains(row, " ABC-1234567 ") || !strings.Contains(row, " 400d ⚑ ") {
		t.Errorf("key and flagged age are never cut: %q", row)
	}
	at := colOf(row, "Long key")
	for key, word := range map[string]string{"ABC-1836": "Solr", "ABC-2020": "Update"} {
		if got := colOf(ls[lineWith(ls, key)], word); got != at {
			t.Errorf("%s summary at column %d, want %d (widths come from the widest row)", key, got, at)
		}
	}
}

func TestListBacklog(t *testing.T) {
	// Open: Backlog cards sit in their epics and count in the header.
	groups := model.ByEpic(append(fxCards(), fxBacklog()...))
	ls := lines(ui.RenderList(groups, map[string]bool{}, -1, 80, ui.ListOptions{}))
	if s := ls[lineWith(ls, "Search")]; !strings.HasSuffix(s, " Mine 1  Backlog 1") {
		t.Errorf("Search header counts its Backlog card: %q", s)
	}
	if got := ls[lineWith(ls, "ABC-1700")]; strings.Contains(got, "@") {
		t.Errorf("a Backlog card is mine and names no assignee: %q", got)
	}
	if lineWith(ls, "hidden") >= 0 {
		t.Errorf("nothing hidden, no hint row:\n%s", strings.Join(ls, "\n"))
	}

	// Collapsed: the dashboard leaves the cards out and says how many.
	ls = lines(ui.RenderList(model.ByEpic(fxCards()), map[string]bool{}, -1, 80, ui.ListOptions{HiddenBacklog: 2, HiddenBacklogLight: model.Yellow}))
	if got := ls[len(ls)-1]; got != " ▶ 🟡 Backlog (2 hidden) - b to show" {
		t.Errorf("last row %q", got)
	}
}

func TestListEpicProgress(t *testing.T) {
	progress := map[string]model.Progress{
		"ABC-E-Auth":          {Done: 4, Total: 7},
		"ABC-E-Accessibility": {Done: 0, Total: 0}, // never 0/0
	}
	ls := lines(ui.RenderList(model.ByEpic(fxCards()), map[string]bool{}, -1, 80, ui.ListOptions{Progress: progress}))
	if got := ls[lineWith(ls, "Auth")]; !strings.HasSuffix(got, " Mine 1  Waiting 1  4/7 done") {
		t.Errorf("Auth header: %q", got)
	}
	for _, g := range []string{"Search", "Accessibility", "No epic"} {
		if got := ls[lineWith(ls, g)]; strings.Contains(got, "done") {
			t.Errorf("%s has no count to show: %q", g, got)
		}
	}
}

func TestListDoneCardShowsNoAge(t *testing.T) {
	cards := []model.Card{card("ABC-1900", "Shipped thing", "Done", model.LaneDone, model.Green, 0, "", "Me")}
	ls := lines(ui.RenderList(model.ByEpic(cards), nil, -1, 80, ui.ListOptions{}))
	got := ls[lineWith(ls, "ABC-1900")]
	if strings.Contains(got, "0m") || !strings.Contains(got, "Done         ") {
		t.Errorf("Done card line %q, want a blank age", got)
	}
}
