package model

import (
	"testing"
	"time"
)

func TestParseAge(t *testing.T) {
	ok := map[string]time.Duration{
		"3d":   72 * time.Hour,
		"1d":   24 * time.Hour,
		"90m":  90 * time.Minute,
		"1d6h": 30 * time.Hour,
		"12h":  12 * time.Hour,
	}
	for in, want := range ok {
		got, err := ParseAge(in)
		if err != nil || got != want {
			t.Errorf("ParseAge(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "d", "3x", "-1d", "-1h", "0d"} {
		if _, err := ParseAge(bad); err == nil {
			t.Errorf("ParseAge(%q) should fail", bad)
		}
	}
}

func TestFormatAge(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Minute:   "30m",
		5 * time.Hour:      "5h",
		26 * time.Hour:     "1d",
		6 * 24 * time.Hour: "6d",
	}
	for in, want := range cases {
		if got := FormatAge(in); got != want {
			t.Errorf("FormatAge(%v) = %q, want %q", in, got, want)
		}
	}
}
