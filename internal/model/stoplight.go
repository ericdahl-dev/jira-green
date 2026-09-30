// Package model holds jira-green's pure flow-health logic. It performs no I/O.
package model

// Stoplight is a card or group health. Values are ordered by severity, so the
// worst of several is simply the maximum.
type Stoplight int

const (
	Green Stoplight = iota
	Stale
	Yellow
	Red
)

func (s Stoplight) Emoji() string {
	switch s {
	case Red:
		return "🔴"
	case Yellow:
		return "🟡"
	case Stale:
		return "⚪"
	default:
		return "🟢"
	}
}

func (s Stoplight) String() string {
	return [...]string{"green", "stale", "yellow", "red"}[s]
}

// Worst returns the most severe stoplight, or Green for none.
func Worst(ls ...Stoplight) Stoplight {
	w := Green
	for _, l := range ls {
		if l > w {
			w = l
		}
	}
	return w
}
