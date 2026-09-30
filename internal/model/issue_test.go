package model

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)

func TestStatusSinceNoTransitionsUsesCreated(t *testing.T) {
	if got := StatusSince(t0, "3", nil); !got.Equal(t0) {
		t.Fatalf("got %v", got)
	}
}

func TestStatusSinceLatestEntryIntoCurrentStatus(t *testing.T) {
	changes := []StatusChange{
		{At: t0.Add(1 * time.Hour), ToID: "3"},  // into In Progress
		{At: t0.Add(2 * time.Hour), ToID: "10"}, // into Code Review
		{At: t0.Add(5 * time.Hour), ToID: "3"},  // bounced back
		{At: t0.Add(9 * time.Hour), ToID: "10"}, // review again (current)
	}
	want := t0.Add(9 * time.Hour)
	if got := StatusSince(t0, "10", changes); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestStatusSinceIgnoresOrder(t *testing.T) {
	changes := []StatusChange{
		{At: t0.Add(9 * time.Hour), ToID: "10"},
		{At: t0.Add(2 * time.Hour), ToID: "10"},
	}
	if got := StatusSince(t0, "10", changes); !got.Equal(t0.Add(9 * time.Hour)) {
		t.Fatalf("got %v", got)
	}
}
