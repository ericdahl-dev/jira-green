package model_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

var cols = []model.Column{
	{Name: "To Do", StatusIDs: []string{"1"}},
	{Name: "In Progress", StatusIDs: []string{"3"}},
	{Name: "Code Review", StatusIDs: []string{"10", "11"}},
	{Name: "Done", StatusIDs: []string{"5"}},
}

func TestColumnFor(t *testing.T) {
	if got := model.ColumnFor(cols, "11"); got != "Code Review" {
		t.Errorf("got %q", got)
	}
	if got := model.ColumnFor(cols, "999"); got != model.OtherColumn {
		t.Errorf("got %q", got)
	}
}

func TestLayoutSortsWorstThenOldest(t *testing.T) {
	cards := []model.Card{
		{Issue: model.Issue{Key: "ABC-1"}, Column: "In Progress", Lane: model.LaneMine, Light: model.Green, Age: 5 * time.Hour},
		{Issue: model.Issue{Key: "ABC-2"}, Column: "In Progress", Lane: model.LaneMine, Light: model.Red, Age: 1 * time.Hour},
		{Issue: model.Issue{Key: "ABC-3"}, Column: "In Progress", Lane: model.LaneMine, Light: model.Green, Age: 9 * time.Hour},
		{Issue: model.Issue{Key: "ABC-4"}, Column: model.OtherColumn, Lane: model.LaneWaiting, Light: model.Yellow},
	}
	b := model.Layout(cols, cards, false)

	wantCols := []string{"To Do", "In Progress", "Code Review", model.OtherColumn}
	if !reflect.DeepEqual(b.Columns, wantCols) {
		t.Fatalf("columns %v (Done hidden, Other added because used)", b.Columns)
	}
	var keys []string
	for _, c := range b.Cell(model.LaneMine, "In Progress") {
		keys = append(keys, c.Key)
	}
	if !reflect.DeepEqual(keys, []string{"ABC-2", "ABC-3", "ABC-1"}) {
		t.Errorf("order %v", keys)
	}
	if b.LaneLight(model.LaneMine) != model.Red || b.LaneLight(model.LaneWaiting) != model.Yellow {
		t.Errorf("lane lights %v %v", b.LaneLight(model.LaneMine), b.LaneLight(model.LaneWaiting))
	}
	if b.LaneCount(model.LaneMine) != 3 || b.LaneCount(model.LaneWaiting) != 1 {
		t.Errorf("lane counts %d %d", b.LaneCount(model.LaneMine), b.LaneCount(model.LaneWaiting))
	}
}

func TestLayoutShowDone(t *testing.T) {
	cards := []model.Card{
		{Issue: model.Issue{Key: "ABC-1"}, Column: "Done", Lane: model.LaneMine, Light: model.Red},
		{Issue: model.Issue{Key: "ABC-2"}, Column: "In Progress", Lane: model.LaneMine, Light: model.Yellow},
	}
	b := model.Layout(cols, cards, true)
	if b.Columns[len(b.Columns)-1] != "Done" {
		t.Errorf("columns %v", b.Columns)
	}
	if b.LaneCount(model.LaneMine) != 2 || b.LaneLight(model.LaneMine) != model.Red {
		t.Errorf("mine lane %d %v, want 2 red with Done shown", b.LaneCount(model.LaneMine), b.LaneLight(model.LaneMine))
	}
	if len(b.Cell(model.LaneMine, "Done")) != 1 {
		t.Errorf("done cell %v", b.Cell(model.LaneMine, "Done"))
	}
}

func TestLayoutHiddenDoneNotCounted(t *testing.T) {
	cards := []model.Card{{Issue: model.Issue{Key: "ABC-1"}, Column: "Done", Lane: model.LaneMine, Light: model.Red}}
	b := model.Layout(cols, cards, false)
	if b.LaneLight(model.LaneMine) != model.Green || b.LaneCount(model.LaneMine) != 0 {
		t.Errorf("mine lane %v %d, want green 0 with Done hidden", b.LaneLight(model.LaneMine), b.LaneCount(model.LaneMine))
	}
	if len(b.Cell(model.LaneMine, "Done")) != 0 {
		t.Errorf("hidden done cell still holds %v", b.Cell(model.LaneMine, "Done"))
	}
}

func TestLayoutUnknownColumnGoesToOther(t *testing.T) {
	cards := []model.Card{{Issue: model.Issue{Key: "ABC-1"}, Column: "Backlog", Lane: model.LaneWaiting, Light: model.Yellow}}
	b := model.Layout(cols, cards, false)
	if got := b.Cell(model.LaneWaiting, model.OtherColumn); len(got) != 1 || got[0].Key != "ABC-1" {
		t.Errorf("other cell %v", got)
	}
	if b.Columns[len(b.Columns)-1] != model.OtherColumn {
		t.Errorf("columns %v, want Other appended", b.Columns)
	}
	if b.LaneCount(model.LaneWaiting) != 1 || b.LaneLight(model.LaneWaiting) != model.Yellow {
		t.Errorf("waiting lane %d %v", b.LaneCount(model.LaneWaiting), b.LaneLight(model.LaneWaiting))
	}
}

func TestByEpic(t *testing.T) {
	cards := []model.Card{
		{Issue: model.Issue{Key: "ABC-1", EpicKey: "ABC-100", EpicSummary: "Search"}, Light: model.Green},
		{Issue: model.Issue{Key: "ABC-2", EpicKey: "ABC-200", EpicSummary: "Auth"}, Light: model.Red},
		{Issue: model.Issue{Key: "ABC-3"}, Light: model.Yellow},
		{Issue: model.Issue{Key: "ABC-4", EpicKey: "ABC-100", EpicSummary: "Search"}, Light: model.Yellow},
	}
	gs := model.ByEpic(cards)
	var names []string
	for _, g := range gs {
		names = append(names, g.Name)
	}
	if !reflect.DeepEqual(names, []string{"Auth", "Search", model.NoEpic}) {
		t.Fatalf("groups %v", names)
	}
	if gs[1].Light != model.Yellow || len(gs[1].Cards) != 2 {
		t.Errorf("Search group %+v", gs[1])
	}
	if gs[1].Cards[0].Key != "ABC-4" {
		t.Errorf("Search cards not worst-first: %v, %v", gs[1].Cards[0].Key, gs[1].Cards[1].Key)
	}
}

func TestByEpicEmptySummaryUsesKey(t *testing.T) {
	gs := model.ByEpic([]model.Card{{Issue: model.Issue{Key: "ABC-1", EpicKey: "ABC-100"}}})
	if len(gs) != 1 || gs[0].Name != "ABC-100" {
		t.Errorf("groups %+v, want name ABC-100", gs)
	}
}

func TestLayoutIncludesTheBacklogLane(t *testing.T) {
	cards := []model.Card{
		{Issue: model.Issue{Key: "ABC-1"}, Column: "To Do", Lane: model.LaneBacklog, Light: model.Green},
		{Issue: model.Issue{Key: "ABC-2"}, Column: "Code Review", Lane: model.LaneBacklog, Light: model.Yellow},
		{Issue: model.Issue{Key: "ABC-3"}, Column: "To Do", Lane: model.LaneMine, Light: model.Red},
	}
	b := model.Layout(cols, cards, false)
	if got := b.Cell(model.LaneBacklog, "Code Review"); len(got) != 1 || got[0].Key != "ABC-2" {
		t.Errorf("backlog Code Review cell %+v", got)
	}
	if b.LaneCount(model.LaneBacklog) != 2 || b.LaneLight(model.LaneBacklog) != model.Yellow {
		t.Errorf("backlog count %d light %v, want 2 yellow", b.LaneCount(model.LaneBacklog), b.LaneLight(model.LaneBacklog))
	}
}
