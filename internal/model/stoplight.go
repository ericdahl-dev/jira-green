// Package model holds jira-green's pure flow-health logic. It performs no I/O.
package model

import "fmt"

// Stoplight is a card or group health. Values are ordered by severity, so the
// worst of several is simply the maximum.
type Stoplight int

// Stoplight values, least to most severe.
const (
	Green Stoplight = iota
	Stale
	Yellow
	Red
)

// Emoji is the colored circle shown on a card; unknown values render green.
func (s Stoplight) Emoji() string {
	switch s {
	case Red:
		return "🔴"
	case Yellow:
		return "🟡"
	case Stale:
		return "⚪"
	default:
		return "🟢"
	}
}

// String is the lowercase name, or "Stoplight(N)" for an unknown value.
func (s Stoplight) String() string {
	names := [...]string{"green", "stale", "yellow", "red"}
	if s < 0 || int(s) >= len(names) {
		return fmt.Sprintf("Stoplight(%d)", int(s))
	}
	return names[s]
}

// Worst returns the most severe stoplight, or Green for none.
func Worst(ls ...Stoplight) Stoplight {
	w := Green
	for _, l := range ls {
		if l > w {
			w = l
		}
	}
	return w
}
