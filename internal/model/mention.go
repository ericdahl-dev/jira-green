package model

import (
	"slices"
	"time"
)

// UnansweredMention reports whether someone else @-mentioned me after my
// latest comment on the issue.
func UnansweredMention(me string, comments []Comment) bool {
	var lastMention, lastMine time.Time
	for _, c := range comments {
		if c.AuthorID == me {
			if c.Created.After(lastMine) {
				lastMine = c.Created
			}
			continue
		}
		if slices.Contains(c.Mentions, me) && c.Created.After(lastMention) {
			lastMention = c.Created
		}
	}
	return !lastMention.IsZero() && lastMention.After(lastMine)
}
