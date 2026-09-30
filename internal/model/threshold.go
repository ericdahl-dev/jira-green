package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var dayPart = regexp.MustCompile(`^(\d+)d`)

// ParseAge parses a duration that may start with a whole number of days,
// e.g. "3d", "1d6h", "12h", "90m".
func ParseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	in := s
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	var total time.Duration
	if m := dayPart.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		total = time.Duration(n) * 24 * time.Hour
		s = s[len(m[0]):]
	}
	if s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("bad duration: %w", err)
		}
		if d < 0 {
			return 0, fmt.Errorf("negative duration %q", s)
		}
		total += d
	} else if total == 0 {
		return 0, fmt.Errorf("bad duration %q", in)
	}
	return total, nil
}

// FormatAge renders an age compactly for a card: "30m", "5h", "6d".
func FormatAge(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
