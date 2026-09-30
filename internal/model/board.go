package model

import (
	"slices"
	"sort"
)

const (
	OtherColumn = "Other"
	NoEpic      = "No epic"
)

// Column is a board column and the status IDs mapped to it.
type Column struct {
	Name      string
	StatusIDs []string
}

// ColumnFor returns the board column for a status ID, or OtherColumn.
func ColumnFor(cols []Column, statusID string) string {
	for _, c := range cols {
		if slices.Contains(c.StatusIDs, statusID) {
			return c.Name
		}
	}
	return OtherColumn
}

type cellKey struct {
	lane   Lane
	column string
}

// Board is the kanban layout: ordered columns and cards per (lane, column).
type Board struct {
	Columns []string
	cells   map[cellKey][]Card
}

func (b Board) Cell(l Lane, column string) []Card { return b.cells[cellKey{l, column}] }

// LaneLight is the worst card in a lane.
func (b Board) LaneLight(l Lane) Stoplight {
	w := Green
	for k, cs := range b.cells {
		if k.lane != l {
			continue
		}
		for _, c := range cs {
			w = Worst(w, c.Light)
		}
	}
	return w
}

// LaneCount is the number of cards in a lane.
func (b Board) LaneCount(l Lane) int {
	n := 0
	for k, cs := range b.cells {
		if k.lane == l {
			n += len(cs)
		}
	}
	return n
}

// Layout places cards into cells. The last board column is treated as Done
// and hidden unless showDone. OtherColumn is appended only when used.
func Layout(cols []Column, cards []Card, showDone bool) Board {
	b := Board{cells: map[cellKey][]Card{}}
	for i, c := range cols {
		if i == len(cols)-1 && !showDone {
			continue
		}
		b.Columns = append(b.Columns, c.Name)
	}
	usedOther := false
	for _, c := range cards {
		if c.Column == OtherColumn {
			usedOther = true
		}
		k := cellKey{c.Lane, c.Column}
		b.cells[k] = append(b.cells[k], c)
	}
	if usedOther {
		b.Columns = append(b.Columns, OtherColumn)
	}
	for k := range b.cells {
		sortCards(b.cells[k])
	}
	return b
}

func sortCards(cs []Card) {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Light != cs[j].Light {
			return cs[i].Light > cs[j].Light
		}
		return cs[i].Age > cs[j].Age
	})
}

// EpicGroup is one row group in the list view.
type EpicGroup struct {
	Key   string
	Name  string
	Light Stoplight
	Cards []Card
}

// ByEpic groups cards by epic, worst group first, then by name. Cards with
// no epic go last in NoEpic.
func ByEpic(cards []Card) []EpicGroup {
	idx := map[string]int{}
	var gs []EpicGroup
	for _, c := range cards {
		key, name := c.EpicKey, c.EpicSummary
		if key == "" {
			key, name = "", NoEpic
		}
		i, ok := idx[key]
		if !ok {
			i = len(gs)
			idx[key] = i
			gs = append(gs, EpicGroup{Key: key, Name: name})
		}
		gs[i].Cards = append(gs[i].Cards, c)
		gs[i].Light = Worst(gs[i].Light, c.Light)
	}
	for i := range gs {
		sortCards(gs[i].Cards)
	}
	sort.SliceStable(gs, func(i, j int) bool {
		if (gs[i].Key == "") != (gs[j].Key == "") {
			return gs[j].Key == ""
		}
		if gs[i].Light != gs[j].Light {
			return gs[i].Light > gs[j].Light
		}
		return gs[i].Name < gs[j].Name
	})
	return gs
}
