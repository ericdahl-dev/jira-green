package model_test

import (
	"slices"
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

func rules() model.Rules {
	return model.Rules{
		Me:            "acct-me",
		BlockedLabels: []string{"blocked"},
		Thresholds: map[string]model.Threshold{
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
		iss    model.Issue
		column string
		stale  bool
		want   model.Stoplight
	}{
		{"fresh in progress", model.Issue{StatusSince: ago(time.Hour)}, "In Progress", false, model.Green},
		{"aging in progress", model.Issue{StatusSince: ago(80 * time.Hour)}, "In Progress", false, model.Yellow},
		{"stuck in progress", model.Issue{StatusSince: ago(130 * time.Hour)}, "In Progress", false, model.Red},
		{"review over red", model.Issue{StatusSince: ago(49 * time.Hour)}, "Code Review", false, model.Red},
		{"no threshold column", model.Issue{StatusSince: ago(500 * time.Hour)}, "To Do", false, model.Green},
		{"flagged", model.Issue{Flagged: true, StatusSince: ago(time.Hour)}, "In Progress", false, model.Red},
		{"blocked label", model.Issue{Labels: []string{"Blocked"}, StatusSince: ago(time.Hour)}, "To Do", false, model.Red},
		{"mention", model.Issue{StatusSince: ago(time.Hour), Comments: []model.Comment{
			{AuthorID: "acct-x", Created: ago(time.Minute), Mentions: []string{"acct-me"}}}}, "To Do", false, model.Yellow},
		{"stale beats green", model.Issue{StatusSince: ago(time.Hour)}, "In Progress", true, model.Stale},
		{"red beats stale", model.Issue{Flagged: true}, "In Progress", true, model.Red},
		{"unknown since is green", model.Issue{}, "In Progress", false, model.Green},
	}
	for _, c := range cases {
		got := model.Evaluate(c.iss, c.column, model.LaneMine, rules(), now, c.stale)
		if got.Light != c.want {
			t.Errorf("%s: light %v want %v (reasons %v)", c.name, got.Light, c.want, got.Reasons)
		}
	}
}

func TestEvaluateReasonsAndAge(t *testing.T) {
	now := t0.Add(10 * 24 * time.Hour)
	c := model.Evaluate(model.Issue{Flagged: true, StatusSince: now.Add(-130 * time.Hour)}, "In Progress", model.LaneMine, rules(), now, false)
	if c.Age != 130*time.Hour {
		t.Errorf("age %v", c.Age)
	}
	if len(c.Reasons) != 2 {
		t.Errorf("want flagged + over-red reasons, got %v", c.Reasons)
	}
}

func TestLaneString(t *testing.T) {
	for l, want := range map[model.Lane]string{model.LaneMine: "Mine", model.LaneWaiting: "Waiting on others", model.LaneBacklog: "Backlog", model.LaneDone: "Done this sprint"} {
		if got := l.String(); got != want {
			t.Errorf("Lane(%d).String() = %q, want %q", int(l), got, want)
		}
	}
}

func TestLaneOrderPutsBacklogBetweenWaitingAndDone(t *testing.T) {
	lanes := []model.Lane{model.LaneMine, model.LaneWaiting, model.LaneBacklog, model.LaneDone}
	if !slices.IsSorted(lanes) || len(slices.Compact(slices.Clone(lanes))) != len(lanes) {
		t.Errorf("lane order Mine %d Waiting %d Backlog %d Done %d", model.LaneMine, model.LaneWaiting, model.LaneBacklog, model.LaneDone)
	}
}

func TestEvaluateAppliesThresholdsInTheBacklog(t *testing.T) {
	iss := model.Issue{StatusSince: t0.Add(-3 * 24 * time.Hour)}
	if c := model.Evaluate(iss, "Code Review", model.LaneBacklog, rules(), t0, false); c.Light != model.Red || c.Lane != model.LaneBacklog {
		t.Errorf("3d in Code Review in the backlog: light %v lane %v, want red", c.Light, c.Lane)
	}
}

func TestEvaluateOneReasonPerLabel(t *testing.T) {
	r := rules()
	r.BlockedLabels = []string{"blocked", "BLOCKED"}
	c := model.Evaluate(model.Issue{Labels: []string{"Blocked"}}, "To Do", model.LaneMine, r, t0, false)
	if len(c.Reasons) != 1 {
		t.Errorf("reasons %v, want one per label", c.Reasons)
	}
}

func TestEvaluateFutureSinceClampsAge(t *testing.T) {
	c := model.Evaluate(model.Issue{StatusSince: t0.Add(time.Hour)}, "In Progress", model.LaneMine, rules(), t0, false)
	if c.Age != 0 || c.Light != model.Green {
		t.Errorf("age %v light %v, want 0 green for a StatusSince in the future", c.Age, c.Light)
	}
}

func TestEvaluateThresholdEdges(t *testing.T) {
	now := t0.Add(10 * 24 * time.Hour)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	r := rules()
	r.Thresholds["QA"] = model.Threshold{Yellow: 24 * time.Hour} // yellow only
	r.Thresholds["UA"] = model.Threshold{Red: 48 * time.Hour}    // red only
	cases := []struct {
		name   string
		since  time.Duration
		column string
		want   model.Stoplight
	}{
		{"exactly at yellow", 72 * time.Hour, "In Progress", model.Yellow},
		{"exactly at red", 120 * time.Hour, "In Progress", model.Red},
		{"just under yellow", 72*time.Hour - time.Second, "In Progress", model.Green},
		{"yellow-only under", 23 * time.Hour, "QA", model.Green},
		{"yellow-only over", 25 * time.Hour, "QA", model.Yellow},
		{"yellow-only never red", 500 * time.Hour, "QA", model.Yellow},
		{"red-only under", 47 * time.Hour, "UA", model.Green},
		{"red-only over", 49 * time.Hour, "UA", model.Red},
	}
	for _, c := range cases {
		got := model.Evaluate(model.Issue{StatusSince: ago(c.since)}, c.column, model.LaneMine, r, now, false)
		if got.Light != c.want {
			t.Errorf("%s: light %v want %v (reasons %v)", c.name, got.Light, c.want, got.Reasons)
		}
	}
}

func TestEvaluateDecodeErrorsAreStale(t *testing.T) {
	iss := model.Issue{DecodeErrors: []string{"created: bad time", "flagged: not an array"}}
	c := model.Evaluate(iss, "To Do", model.LaneMine, rules(), t0, false)
	if c.Light != model.Stale {
		t.Errorf("light %v, want stale", c.Light)
	}
	want := "data incomplete: created: bad time; flagged: not an array"
	if !slices.Contains(c.Reasons, want) {
		t.Errorf("reasons %q, want %q", c.Reasons, want)
	}
	iss.Flagged = true
	if c := model.Evaluate(iss, "To Do", model.LaneMine, rules(), t0, false); c.Light != model.Red {
		t.Errorf("flagged + decode errors: light %v, want red", c.Light)
	}
}
