package model

import (
	"reflect"
	"testing"
	"time"
)

var cols = []Column{
	{Name: "To Do", StatusIDs: []string{"1"}},
	{Name: "In Progress", StatusIDs: []string{"3"}},
	{Name: "Code Review", StatusIDs: []string{"10", "11"}},
	{Name: "Done", StatusIDs: []string{"5"}},
}

func TestColumnFor(t *testing.T) {
	if got := ColumnFor(cols, "11"); got != "Code Review" {
		t.Errorf("got %q", got)
	}
	if got := ColumnFor(cols, "999"); got != OtherColumn {
		t.Errorf("got %q", got)
	}
}

func TestLayoutSortsWorstThenOldest(t *testing.T) {
	cards := []Card{
		{Issue: Issue{Key: "ABC-1"}, Column: "In Progress", Lane: LaneMine, Light: Green, Age: 5 * time.Hour},
		{Issue: Issue{Key: "ABC-2"}, Column: "In Progress", Lane: LaneMine, Light: Red, Age: 1 * time.Hour},
		{Issue: Issue{Key: "ABC-3"}, Column: "In Progress", Lane: LaneMine, Light: Green, Age: 9 * time.Hour},
		{Issue: Issue{Key: "ABC-4"}, Column: OtherColumn, Lane: LaneWaiting, Light: Yellow},
	}
	b := Layout(cols, cards, false)

	wantCols := []string{"To Do", "In Progress", "Code Review", OtherColumn}
	if !reflect.DeepEqual(b.Columns, wantCols) {
		t.Fatalf("columns %v (Done hidden, Other added because used)", b.Columns)
	}
	var keys []string
	for _, c := range b.Cell(LaneMine, "In Progress") {
		keys = append(keys, c.Key)
	}
	if !reflect.DeepEqual(keys, []string{"ABC-2", "ABC-3", "ABC-1"}) {
		t.Errorf("order %v", keys)
	}
	if b.LaneLight(LaneMine) != Red || b.LaneLight(LaneWaiting) != Yellow {
		t.Errorf("lane lights %v %v", b.LaneLight(LaneMine), b.LaneLight(LaneWaiting))
	}
	if b.LaneCount(LaneMine) != 3 || b.LaneCount(LaneWaiting) != 1 {
		t.Errorf("lane counts %d %d", b.LaneCount(LaneMine), b.LaneCount(LaneWaiting))
	}
}

func TestLayoutShowDone(t *testing.T) {
	cards := []Card{
		{Issue: Issue{Key: "ABC-1"}, Column: "Done", Lane: LaneMine, Light: Red},
		{Issue: Issue{Key: "ABC-2"}, Column: "In Progress", Lane: LaneMine, Light: Yellow},
	}
	b := Layout(cols, cards, true)
	if b.Columns[len(b.Columns)-1] != "Done" {
		t.Errorf("columns %v", b.Columns)
	}
	if b.LaneCount(LaneMine) != 2 || b.LaneLight(LaneMine) != Red {
		t.Errorf("mine lane %d %v, want 2 red with Done shown", b.LaneCount(LaneMine), b.LaneLight(LaneMine))
	}
	if len(b.Cell(LaneMine, "Done")) != 1 {
		t.Errorf("done cell %v", b.Cell(LaneMine, "Done"))
	}
}

func TestLayoutHiddenDoneNotCounted(t *testing.T) {
	cards := []Card{{Issue: Issue{Key: "ABC-1"}, Column: "Done", Lane: LaneMine, Light: Red}}
	b := Layout(cols, cards, false)
	if b.LaneLight(LaneMine) != Green || b.LaneCount(LaneMine) != 0 {
		t.Errorf("mine lane %v %d, want green 0 with Done hidden", b.LaneLight(LaneMine), b.LaneCount(LaneMine))
	}
	if len(b.Cell(LaneMine, "Done")) != 0 {
		t.Errorf("hidden done cell still holds %v", b.Cell(LaneMine, "Done"))
	}
}

func TestLayoutUnknownColumnGoesToOther(t *testing.T) {
	cards := []Card{{Issue: Issue{Key: "ABC-1"}, Column: "Backlog", Lane: LaneWaiting, Light: Yellow}}
	b := Layout(cols, cards, false)
	if got := b.Cell(LaneWaiting, OtherColumn); len(got) != 1 || got[0].Key != "ABC-1" {
		t.Errorf("other cell %v", got)
	}
	if b.Columns[len(b.Columns)-1] != OtherColumn {
		t.Errorf("columns %v, want Other appended", b.Columns)
	}
	if b.LaneCount(LaneWaiting) != 1 || b.LaneLight(LaneWaiting) != Yellow {
		t.Errorf("waiting lane %d %v", b.LaneCount(LaneWaiting), b.LaneLight(LaneWaiting))
	}
}

func TestByEpic(t *testing.T) {
	cards := []Card{
		{Issue: Issue{Key: "ABC-1", EpicKey: "ABC-100", EpicSummary: "Search"}, Light: Green},
		{Issue: Issue{Key: "ABC-2", EpicKey: "ABC-200", EpicSummary: "Auth"}, Light: Red},
		{Issue: Issue{Key: "ABC-3"}, Light: Yellow},
		{Issue: Issue{Key: "ABC-4", EpicKey: "ABC-100", EpicSummary: "Search"}, Light: Yellow},
	}
	gs := ByEpic(cards)
	var names []string
	for _, g := range gs {
		names = append(names, g.Name)
	}
	if !reflect.DeepEqual(names, []string{"Auth", "Search", NoEpic}) {
		t.Fatalf("groups %v", names)
	}
	if gs[1].Light != Yellow || len(gs[1].Cards) != 2 {
		t.Errorf("Search group %+v", gs[1])
	}
	if gs[1].Cards[0].Key != "ABC-4" {
		t.Errorf("Search cards not worst-first: %v, %v", gs[1].Cards[0].Key, gs[1].Cards[1].Key)
	}
}
