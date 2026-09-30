package model

import (
	"testing"
	"time"
)

func rules() Rules {
	return Rules{
		Me:            "acct-me",
		BlockedLabels: []string{"blocked"},
		Thresholds: map[string]Threshold{
			"In Progress": {Yellow: 72 * time.Hour, Red: 120 * time.Hour},
			"Code Review": {Yellow: 24 * time.Hour, Red: 48 * time.Hour},
		},
	}
}

func TestEvaluate(t *testing.T) {
	now := t0.Add(10 * 24 * time.Hour)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	cases := []struct {
		name   string
		iss    Issue
		column string
		stale  bool
		want   Stoplight
	}{
		{"fresh in progress", Issue{StatusSince: ago(time.Hour)}, "In Progress", false, Green},
		{"aging in progress", Issue{StatusSince: ago(80 * time.Hour)}, "In Progress", false, Yellow},
		{"stuck in progress", Issue{StatusSince: ago(130 * time.Hour)}, "In Progress", false, Red},
		{"review over red", Issue{StatusSince: ago(49 * time.Hour)}, "Code Review", false, Red},
		{"no threshold column", Issue{StatusSince: ago(500 * time.Hour)}, "To Do", false, Green},
		{"flagged", Issue{Flagged: true, StatusSince: ago(time.Hour)}, "In Progress", false, Red},
		{"blocked label", Issue{Labels: []string{"Blocked"}, StatusSince: ago(time.Hour)}, "To Do", false, Red},
		{"mention", Issue{StatusSince: ago(time.Hour), Comments: []Comment{
			{AuthorID: "acct-x", Created: ago(time.Minute), Mentions: []string{"acct-me"}}}}, "To Do", false, Yellow},
		{"stale beats green", Issue{StatusSince: ago(time.Hour)}, "In Progress", true, Stale},
		{"red beats stale", Issue{Flagged: true}, "In Progress", true, Red},
		{"unknown since is green", Issue{}, "In Progress", false, Green},
	}
	for _, c := range cases {
		got := Evaluate(c.iss, c.column, LaneMine, rules(), now, c.stale)
		if got.Light != c.want {
			t.Errorf("%s: light %v want %v (reasons %v)", c.name, got.Light, c.want, got.Reasons)
		}
	}
}

func TestEvaluateReasonsAndAge(t *testing.T) {
	now := t0.Add(10 * 24 * time.Hour)
	c := Evaluate(Issue{Flagged: true, StatusSince: now.Add(-130 * time.Hour)}, "In Progress", LaneMine, rules(), now, false)
	if c.Age != 130*time.Hour {
		t.Errorf("age %v", c.Age)
	}
	if len(c.Reasons) != 2 {
		t.Errorf("want flagged + over-red reasons, got %v", c.Reasons)
	}
}

func TestLaneString(t *testing.T) {
	for l, want := range map[Lane]string{LaneMine: "Mine", LaneWaiting: "Waiting on others", LaneDone: "Done this sprint"} {
		if got := l.String(); got != want {
			t.Errorf("Lane(%d).String() = %q, want %q", int(l), got, want)
		}
	}
}
