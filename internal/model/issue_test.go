package model_test

import (
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

var t0 = time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)

func TestStatusSinceNoTransitionsUsesCreated(t *testing.T) {
	if got := model.StatusSince(t0, "3", nil); !got.Equal(t0) {
		t.Fatalf("got %v", got)
	}
}

func TestStatusSinceLatestEntryIntoCurrentStatus(t *testing.T) {
	changes := []model.StatusChange{
		{At: t0.Add(1 * time.Hour), ToID: "3"},  // into In Progress
		{At: t0.Add(2 * time.Hour), ToID: "10"}, // into Code Review
		{At: t0.Add(5 * time.Hour), ToID: "3"},  // bounced back
		{At: t0.Add(9 * time.Hour), ToID: "10"}, // review again (current)
	}
	want := t0.Add(9 * time.Hour)
	if got := model.StatusSince(t0, "10", changes); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestStatusSinceIgnoresOrder(t *testing.T) {
	changes := []model.StatusChange{
		{At: t0.Add(9 * time.Hour), ToID: "10"},
		{At: t0.Add(2 * time.Hour), ToID: "10"},
	}
	if got := model.StatusSince(t0, "10", changes); !got.Equal(t0.Add(9 * time.Hour)) {
		t.Fatalf("got %v", got)
	}
}

func TestDisplaySummaryPrefixesASubtasksStory(t *testing.T) {
	sub := model.Issue{Key: "ABC-13", Summary: "Write tests", ParentKey: "ABC-12"}
	if got := sub.DisplaySummary(); got != "ABC-12 › Write tests" {
		t.Errorf("subtask: %q", got)
	}
	story := model.Issue{Key: "ABC-12", Summary: "Login story", EpicKey: "ABC-100"}
	if got := story.DisplaySummary(); got != "Login story" {
		t.Errorf("story: %q", got)
	}
}
