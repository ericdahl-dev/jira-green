package model

import "time"

// Issue is a Jira issue reduced to what the health model needs. The jira
// package builds these; model never sees API JSON.
type Issue struct {
	Key          string
	Summary      string
	URL          string
	StatusID     string
	StatusName   string
	AssigneeID   string
	AssigneeName string
	EpicKey      string
	EpicSummary  string
	Labels       []string
	Flagged      bool
	Created      time.Time
	Updated      time.Time
	// StatusSince is when the issue entered its current status. The poller
	// fills it from the changelog; zero means "not yet known".
	StatusSince time.Time
	Comments    []Comment
	// DecodeErrors lists fields the jira package could not decode, as
	// "field: error". Non-empty marks the issue's data as incomplete.
	DecodeErrors []string
}

// Comment is one issue comment, with the account IDs it @-mentions.
type Comment struct {
	AuthorID string
	Created  time.Time
	Mentions []string
}

// StatusChange is one status transition from the changelog.
type StatusChange struct {
	At   time.Time
	ToID string
}

// StatusSince returns when the issue last entered currentID: the latest
// transition into it, or created when there was none.
func StatusSince(created time.Time, currentID string, changes []StatusChange) time.Time {
	since := created
	for _, c := range changes {
		if c.ToID == currentID && c.At.After(since) {
			since = c.At
		}
	}
	return since
}
