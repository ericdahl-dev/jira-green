package model_test

import (
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

func TestUnansweredMention(t *testing.T) {
	me := "acct-me"
	at := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	cases := []struct {
		name string
		cs   []model.Comment
		want bool
	}{
		{"no comments", nil, false},
		{"mention, no reply", []model.Comment{{AuthorID: "acct-x", Created: at(1), Mentions: []string{me}}}, true},
		{"mention then my reply", []model.Comment{
			{AuthorID: "acct-x", Created: at(1), Mentions: []string{me}},
			{AuthorID: me, Created: at(2)},
		}, false},
		{"my comment then mention", []model.Comment{
			{AuthorID: me, Created: at(1)},
			{AuthorID: "acct-x", Created: at(2), Mentions: []string{me}},
		}, true},
		{"mentions someone else", []model.Comment{{AuthorID: "acct-x", Created: at(1), Mentions: []string{"acct-y"}}}, false},
		{"I mention myself", []model.Comment{{AuthorID: me, Created: at(1), Mentions: []string{me}}}, false},
	}
	for _, c := range cases {
		if got := model.UnansweredMention(me, c.cs); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestUnansweredMentionUnknownMe(t *testing.T) {
	at := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	cases := []struct {
		name string
		cs   []model.Comment
	}{
		{"empty mention ID does not match", []model.Comment{{AuthorID: "acct-x", Created: at(1), Mentions: []string{""}}}},
		{"empty author mentions empty ID", []model.Comment{{AuthorID: "", Created: at(1), Mentions: []string{""}}}},
		{"empty author ID is not my reply", []model.Comment{
			{AuthorID: "acct-x", Created: at(1), Mentions: []string{"acct-y"}},
			{AuthorID: "", Created: at(2)},
		}},
	}
	for _, c := range cases {
		if model.UnansweredMention("", c.cs) {
			t.Errorf("%s: got true want false", c.name)
		}
	}
}
