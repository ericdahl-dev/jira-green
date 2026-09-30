package model_test

import (
	"testing"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

func TestWorst(t *testing.T) {
	cases := []struct {
		in   []model.Stoplight
		want model.Stoplight
	}{
		{nil, model.Green},
		{[]model.Stoplight{model.Green, model.Green}, model.Green},
		{[]model.Stoplight{model.Green, model.Stale}, model.Stale},
		{[]model.Stoplight{model.Stale, model.Yellow}, model.Yellow},
		{[]model.Stoplight{model.Yellow, model.Red, model.Green}, model.Red},
	}
	for _, c := range cases {
		if got := model.Worst(c.in...); got != c.want {
			t.Errorf("Worst(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEmoji(t *testing.T) {
	for s, want := range map[model.Stoplight]string{model.Green: "🟢", model.Yellow: "🟡", model.Red: "🔴", model.Stale: "⚪"} {
		if s.Emoji() != want {
			t.Errorf("%v.Emoji() = %q", s, s.Emoji())
		}
	}
}

func TestStringOutOfRange(t *testing.T) {
	if got := model.Stoplight(99).String(); got != "Stoplight(99)" {
		t.Errorf("Stoplight(99).String() = %q", got)
	}
	if got := model.Lane(-1).String(); got != "Lane(-1)" {
		t.Errorf("Lane(-1).String() = %q", got)
	}
}
