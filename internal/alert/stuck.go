package alert

import (
	"time"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

// Tracker turns a stream of snapshots into one stuck event per red incident.
// It is not safe for concurrent use: feed it from one goroutine.
type Tracker struct {
	after   time.Duration
	redFrom map[string]time.Time
	fired   map[string]bool
}

// NewTracker returns a Tracker that fires once a card has been red for after.
func NewTracker(after time.Duration) *Tracker {
	return &Tracker{after: after, redFrom: map[string]time.Time{}, fired: map[string]bool{}}
}

// Observe records the cards' lights at now and returns the events to send.
func (t *Tracker) Observe(cards []model.Card, now time.Time) []Event {
	var out []Event
	present := map[string]bool{}
	for _, c := range cards {
		present[c.Key] = true
		if c.Light != model.Red {
			delete(t.redFrom, c.Key)
			delete(t.fired, c.Key)
			continue
		}
		from, ok := t.redFrom[c.Key]
		if !ok {
			t.redFrom[c.Key] = now
			continue
		}
		if !t.fired[c.Key] && now.Sub(from) >= t.after {
			t.fired[c.Key] = true
			out = append(out, Event{
				Type: TypeTicketStuck, Key: c.Key, Summary: c.Summary, Status: c.StatusName,
				URL: c.URL, Reasons: c.Reasons, RedFor: model.FormatAge(now.Sub(from)), At: now,
			})
		}
	}
	// A card that left the board (muted, or out of every lane) ends its
	// incident.
	for k := range t.redFrom {
		if !present[k] {
			delete(t.redFrom, k)
			delete(t.fired, k)
		}
	}
	return out
}

// ObserveSnapshot is Observe for one poll result. When the poll failed
// (pollErr != nil) its cards are only the last good ones re-marked stale, so
// it does nothing: a stale snapshot neither advances an incident toward
// firing nor resets one. Stuck alerts resume with the next good poll.
func (t *Tracker) ObserveSnapshot(cards []model.Card, pollErr error, now time.Time) []Event {
	if pollErr != nil {
		return nil
	}
	return t.Observe(cards, now)
}
