package ui_test

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/muesli/termenv"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update): %v", p, err)
	}
	if string(want) != got {
		t.Errorf("%s mismatch:\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

// lines splits rendered output, dropping the final empty line.
func lines(s string) []string { return strings.Split(strings.TrimSuffix(s, "\n"), "\n") }

// lineWith returns the index of the first line containing sub, or -1.
func lineWith(ls []string, sub string) int {
	for i, l := range ls {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

var fxCols = []model.Column{
	{Name: "To Do", StatusIDs: []string{"1"}},
	{Name: "In Progress", StatusIDs: []string{"3"}},
	{Name: "Code Review", StatusIDs: []string{"10"}},
	{Name: "UA", StatusIDs: []string{"12"}},
	{Name: "Done", StatusIDs: []string{"5"}},
}

func card(key, summary, col string, lane model.Lane, light model.Stoplight, age time.Duration, epic, assignee string) model.Card {
	return model.Card{
		Issue:  model.Issue{Key: key, Summary: summary, EpicSummary: epic, EpicKey: epicKey(epic), AssigneeName: assignee, Flagged: light == model.Red},
		Column: col, Lane: lane, Light: light, Age: age,
	}
}

func epicKey(e string) string {
	if e == "" {
		return ""
	}
	return "ABC-E-" + e
}

const day = 24 * time.Hour

func fxCards() []model.Card {
	return []model.Card{
		card("ABC-2011", "Add alt text to search results", "To Do", model.LaneMine, model.Green, 1*day, "Accessibility", "Me"),
		card("ABC-2020", "Update footer links", "To Do", model.LaneMine, model.Green, 0, "", "Me"),
		card("ABC-1974", "Fix auth redirect loop", "In Progress", model.LaneMine, model.Yellow, 4*day, "Auth", "Me"),
		card("ABC-1836", "Solr pagination breaks on page 11", "Code Review", model.LaneMine, model.Red, 6*day, "Search", "Me"),
		card("ABC-1990", "Harden session cookie", "In Progress", model.LaneWaiting, model.Yellow, 3*day, "Auth", "Jane Smith"),
		card("ABC-1950", "Verify catalog export", "UA", model.LaneWaiting, model.Green, 1*day, "", "QA Team"),
	}
}

// colOf is the display column where sub starts in line, or -1.
func colOf(line, sub string) int {
	i := strings.Index(line, sub)
	if i < 0 {
		return -1
	}
	return lipgloss.Width(line[:i])
}
