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
	// EpicKey and EpicSummary name the epic the issue rolls up to. For a
	// subtask this is its story's epic, which the poller resolves; until
	// then (or if that lookup fails) it is the story itself.
	EpicKey     string
	EpicSummary string
	// ParentKey and ParentSummary are a subtask's story. Both are empty for
	// an issue that is not a subtask.
	ParentKey     string
	ParentSummary string
	Labels        []string
	Flagged       bool
	Created       time.Time
	Updated       time.Time
	// StatusSince is when the issue entered its current status. The poller
	// fills it from the changelog; zero means "not yet known".
	StatusSince time.Time
	Comments    []Comment
	// CommentsTruncated is set when Search embedded only the first page of
	// comments. The poller then fetches the newest ones.
	CommentsTruncated bool
	// DecodeErrors lists fields the jira package could not decode, as
	// "field: error". Non-empty marks the issue's data as incomplete.
	DecodeErrors []string
}

// DisplaySummary is the summary a view shows: "ABC-12 › Write tests" for a
// subtask, naming its story, else the plain summary.
func (i Issue) DisplaySummary() string {
	if i.ParentKey == "" {
		return i.Summary
	}
	return i.ParentKey + " › " + i.Summary
}

// Comment is one issue comment, with the account IDs it @-mentions.
type Comment struct {
	AuthorID   string
	AuthorName string // display name; empty when Jira hides it
	Created    time.Time
	Mentions   []string
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
