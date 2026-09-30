package jira

import (
	"bytes"
	"time"
)

// Time parses Jira's timestamp format, which is not RFC 3339
// ("2026-09-30T10:00:00.000-0400").
type Time struct{ time.Time }

const jiraLayout = "2006-01-02T15:04:05.000-0700"

// UnmarshalJSON accepts Jira's layout, RFC 3339 as a fallback, and null.
func (t *Time) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) || len(b) < 2 {
		t.Time = time.Time{}
		return nil
	}
	s := string(b[1 : len(b)-1])
	p, err := time.Parse(jiraLayout, s)
	if err != nil {
		p, err = time.Parse(time.RFC3339, s)
	}
	t.Time = p
	return err
}
