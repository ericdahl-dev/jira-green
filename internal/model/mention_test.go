package model

import (
	"testing"
	"time"
)

func TestUnansweredMention(t *testing.T) {
	me := "acct-me"
	at := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	cases := []struct {
		name string
		cs   []Comment
		want bool
	}{
		{"no comments", nil, false},
		{"mention, no reply", []Comment{{AuthorID: "acct-x", Created: at(1), Mentions: []string{me}}}, true},
		{"mention then my reply", []Comment{
			{AuthorID: "acct-x", Created: at(1), Mentions: []string{me}},
			{AuthorID: me, Created: at(2)},
		}, false},
		{"my comment then mention", []Comment{
			{AuthorID: me, Created: at(1)},
			{AuthorID: "acct-x", Created: at(2), Mentions: []string{me}},
		}, true},
		{"mentions someone else", []Comment{{AuthorID: "acct-x", Created: at(1), Mentions: []string{"acct-y"}}}, false},
		{"I mention myself", []Comment{{AuthorID: me, Created: at(1), Mentions: []string{me}}}, false},
	}
	for _, c := range cases {
		if got := UnansweredMention(me, c.cs); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
