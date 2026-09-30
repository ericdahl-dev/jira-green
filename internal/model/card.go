package model

import (
	"fmt"
	"strings"
	"time"
)

// Lane is a kanban swimlane.
type Lane int

const (
	LaneMine Lane = iota
	LaneWaiting
	LaneDone
)

func (l Lane) String() string {
	return [...]string{"Mine", "Waiting on others", "Done this sprint"}[l]
}

// Rules configures Evaluate.
type Rules struct {
	Me            string               // my Jira account ID
	BlockedLabels []string             // matched case-insensitively
	Thresholds    map[string]Threshold // keyed by board column name
}

// Card is an issue placed on the board with its health.
type Card struct {
	Issue
	Column  string
	Lane    Lane
	Light   Stoplight
	Age     time.Duration // time in current status; 0 when unknown
	Reasons []string      // human-readable, shown in the detail pane
}

// Evaluate computes a card's stoplight. Red: flagged, blocked label, or over
// the column's red threshold. Yellow: over yellow, or an unanswered mention.
// Stale only shows through when nothing is yellow or red.
func Evaluate(iss Issue, column string, lane Lane, r Rules, now time.Time, stale bool) Card {
	c := Card{Issue: iss, Column: column, Lane: lane, Light: Green}
	raise := func(l Stoplight, why string) {
		c.Light = Worst(c.Light, l)
		c.Reasons = append(c.Reasons, why)
	}

	if iss.Flagged {
		raise(Red, "flagged")
	}
	for _, l := range iss.Labels {
		for _, b := range r.BlockedLabels {
			if strings.EqualFold(l, b) {
				raise(Red, "label "+l)
			}
		}
	}
	if !iss.StatusSince.IsZero() {
		c.Age = now.Sub(iss.StatusSince)
		if th, ok := r.Thresholds[column]; ok {
			switch {
			case th.Red > 0 && c.Age >= th.Red:
				raise(Red, fmt.Sprintf("in %s %s (red at %s)", column, FormatAge(c.Age), FormatAge(th.Red)))
			case th.Yellow > 0 && c.Age >= th.Yellow:
				raise(Yellow, fmt.Sprintf("in %s %s (yellow at %s)", column, FormatAge(c.Age), FormatAge(th.Yellow)))
			}
		}
	}
	if UnansweredMention(r.Me, iss.Comments) {
		raise(Yellow, "unanswered mention")
	}
	if stale {
		c.Light = Worst(c.Light, Stale)
	}
	return c
}
