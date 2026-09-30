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

func TestEvaluateOneReasonPerLabel(t *testing.T) {
	r := rules()
	r.BlockedLabels = []string{"blocked", "BLOCKED"}
	c := Evaluate(Issue{Labels: []string{"Blocked"}}, "To Do", LaneMine, r, t0, false)
	if len(c.Reasons) != 1 {
		t.Errorf("reasons %v, want one per label", c.Reasons)
	}
}

func TestEvaluateFutureSinceClampsAge(t *testing.T) {
	c := Evaluate(Issue{StatusSince: t0.Add(time.Hour)}, "In Progress", LaneMine, rules(), t0, false)
	if c.Age != 0 || c.Light != Green {
		t.Errorf("age %v light %v, want 0 green for a StatusSince in the future", c.Age, c.Light)
	}
}

func TestEvaluateThresholdEdges(t *testing.T) {
	now := t0.Add(10 * 24 * time.Hour)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	r := rules()
	r.Thresholds["QA"] = Threshold{Yellow: 24 * time.Hour} // yellow only
	r.Thresholds["UA"] = Threshold{Red: 48 * time.Hour}    // red only
	cases := []struct {
		name   string
		since  time.Duration
		column string
		want   Stoplight
	}{
		{"exactly at yellow", 72 * time.Hour, "In Progress", Yellow},
		{"exactly at red", 120 * time.Hour, "In Progress", Red},
		{"just under yellow", 72*time.Hour - time.Second, "In Progress", Green},
		{"yellow-only under", 23 * time.Hour, "QA", Green},
		{"yellow-only over", 25 * time.Hour, "QA", Yellow},
		{"yellow-only never red", 500 * time.Hour, "QA", Yellow},
		{"red-only under", 47 * time.Hour, "UA", Green},
		{"red-only over", 49 * time.Hour, "UA", Red},
	}
	for _, c := range cases {
		got := Evaluate(Issue{StatusSince: ago(c.since)}, c.column, LaneMine, r, now, false)
		if got.Light != c.want {
			t.Errorf("%s: light %v want %v (reasons %v)", c.name, got.Light, c.want, got.Reasons)
		}
	}
}
