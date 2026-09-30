package alert_test

import (
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/alert"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

var t0 = time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)

func card(key string, l model.Stoplight) []model.Card {
	return []model.Card{{
		Issue: model.Issue{Key: key, Summary: "Fix the widget", StatusName: "In Progress", URL: "https://example.atlassian.net/browse/" + key},
		Light: l, Reasons: []string{"flagged"},
	}}
}

func TestTrackerFiresOncePerIncident(t *testing.T) {
	tr := alert.NewTracker(2 * time.Hour)
	red := card("ABC-1", model.Red)

	if ev := tr.Observe(red, t0); len(ev) != 0 {
		t.Fatalf("first red sighting fired: %+v", ev)
	}
	if ev := tr.Observe(red, t0.Add(time.Hour)); len(ev) != 0 {
		t.Fatalf("fired before the threshold: %+v", ev)
	}
	ev := tr.Observe(red, t0.Add(2*time.Hour))
	want := alert.Event{
		Type: alert.TypeTicketStuck, Key: "ABC-1", Summary: "Fix the widget", Status: "In Progress",
		URL: "https://example.atlassian.net/browse/ABC-1", RedFor: "2h", At: t0.Add(2 * time.Hour),
	}
	if len(ev) != 1 || ev[0].Key != want.Key || ev[0].Type != want.Type || ev[0].Summary != want.Summary ||
		ev[0].Status != want.Status || ev[0].URL != want.URL || ev[0].RedFor != want.RedFor ||
		!ev[0].At.Equal(want.At) || len(ev[0].Reasons) != 1 || ev[0].Reasons[0] != "flagged" {
		t.Fatalf("at the threshold: got %+v, want one %+v", ev, want)
	}
	if ev := tr.Observe(red, t0.Add(3*time.Hour)); len(ev) != 0 {
		t.Fatalf("re-fired within one incident: %+v", ev)
	}
}

func TestTrackerRecoveryStartsNewIncident(t *testing.T) {
	tr := alert.NewTracker(2 * time.Hour)
	red := card("ABC-1", model.Red)
	tr.Observe(red, t0)
	if ev := tr.Observe(red, t0.Add(2*time.Hour)); len(ev) != 1 {
		t.Fatalf("first incident: want 1 event, got %d", len(ev))
	}

	tr.Observe(card("ABC-1", model.Yellow), t0.Add(3*time.Hour))
	tr.Observe(red, t0.Add(4*time.Hour))
	if ev := tr.Observe(red, t0.Add(5*time.Hour)); len(ev) != 0 {
		t.Fatalf("new incident timed from the old red start: %+v", ev)
	}
	if ev := tr.Observe(red, t0.Add(6*time.Hour)); len(ev) != 1 {
		t.Fatalf("new incident after recovery: want 1 event, got %d", len(ev))
	}
}

func TestTrackerCardGoneStartsNewIncident(t *testing.T) {
	tr := alert.NewTracker(2 * time.Hour)
	red := card("ABC-1", model.Red)
	tr.Observe(red, t0)
	tr.Observe(red, t0.Add(2*time.Hour))

	tr.Observe(nil, t0.Add(3*time.Hour)) // muted, or left every lane
	tr.Observe(red, t0.Add(4*time.Hour))
	if ev := tr.Observe(red, t0.Add(6*time.Hour)); len(ev) != 1 {
		t.Fatalf("card back after leaving: want 1 event, got %d", len(ev))
	}
}

func TestTrackerStaleRedCardKeepsIncident(t *testing.T) {
	tr := alert.NewTracker(2 * time.Hour)
	iss := model.Issue{Key: "ABC-1", Flagged: true}
	fresh := []model.Card{model.Evaluate(iss, "In Progress", model.LaneMine, model.Rules{}, t0, false)}
	stale := []model.Card{model.Evaluate(iss, "In Progress", model.LaneMine, model.Rules{}, t0, true)}

	tr.Observe(fresh, t0)
	tr.Observe(stale, t0.Add(time.Hour)) // a failed poll re-marks the last cards stale
	if ev := tr.Observe(fresh, t0.Add(2*time.Hour)); len(ev) != 1 {
		t.Fatalf("a stale poll reset the incident: want 1 event at 2h, got %d", len(ev))
	}
}
