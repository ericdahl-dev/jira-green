package model

import "time"

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
