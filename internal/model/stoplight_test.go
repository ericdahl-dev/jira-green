package model

import "testing"

func TestWorst(t *testing.T) {
	cases := []struct {
		in   []Stoplight
		want Stoplight
	}{
		{nil, Green},
		{[]Stoplight{Green, Green}, Green},
		{[]Stoplight{Green, Stale}, Stale},
		{[]Stoplight{Stale, Yellow}, Yellow},
		{[]Stoplight{Yellow, Red, Green}, Red},
	}
	for _, c := range cases {
		if got := Worst(c.in...); got != c.want {
			t.Errorf("Worst(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEmoji(t *testing.T) {
	for s, want := range map[Stoplight]string{Green: "🟢", Yellow: "🟡", Red: "🔴", Stale: "⚪"} {
		if s.Emoji() != want {
			t.Errorf("%v.Emoji() = %q", s, s.Emoji())
		}
	}
}
