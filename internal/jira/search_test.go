package jira_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSearchPaginatesAndConverts(t *testing.T) {
	calls := 0
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		calls++
		fields, _ := body["fields"].([]any)
		if !slices.Contains(fields, any("customfield_10021")) {
			t.Errorf("fields %v missing flagged field", fields)
		}
		file := "testdata/search_page1.json"
		if body["nextPageToken"] == "p2" {
			file = "testdata/search_page2.json"
		}
		b, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("read fixture: %v", err)
		}
		_, _ = w.Write(b)
	})
	issues, err := c.Search(context.Background(), "assignee = currentUser()", "customfield_10021")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(issues) != 2 {
		t.Fatalf("calls %d issues %d", calls, len(issues))
	}
	a := issues[0]
	if a.Key != "ABC-1" || a.StatusID != "3" || a.EpicKey != "ABC-100" || a.EpicSummary != "Auth" {
		t.Errorf("%+v", a)
	}
	if !a.Flagged || a.AssigneeID != "acct-me" || a.URL != c.Site()+"/browse/ABC-1" {
		t.Errorf("%+v", a)
	}
	wantCreated := time.Date(2026, 9, 20, 13, 0, 0, 0, time.UTC)
	wantUpdated := time.Date(2026, 9, 29, 13, 0, 0, 0, time.UTC)
	if !slices.Equal(a.Labels, []string{"blocked"}) || !a.Created.Equal(wantCreated) || !a.Updated.Equal(wantUpdated) {
		t.Errorf("labels %v created %v updated %v", a.Labels, a.Created, a.Updated)
	}
	if len(a.Comments) != 1 || a.Comments[0].AuthorID != "acct-jsmith" ||
		!slices.Equal(a.Comments[0].Mentions, []string{"acct-me"}) {
		t.Errorf("comments %+v", a.Comments)
	}
	if issues[1].Flagged || issues[1].AssigneeID != "" {
		t.Errorf("%+v", issues[1])
	}
}

func TestSearchSkipsMentionsWithoutID(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"ABC-3","fields":{"comment":{"comments":[
			{"author":{"accountId":"acct-jsmith"},"created":"2026-09-28T09:00:00.000-0400",
			 "body":{"type":"doc","content":[{"type":"paragraph","content":[
				{"type":"mention","attrs":{"text":"@nobody"}},
				{"type":"mention","attrs":{"id":"","text":"@empty"}},
				{"type":"mention"},
				{"type":"mention","attrs":{"id":"acct-jsmith","text":"@J"}}]}]}}]}}}]}`))
	})
	issues, err := c.Search(context.Background(), "project = ABC", "")
	if err != nil || len(issues) != 1 || len(issues[0].Comments) != 1 {
		t.Fatalf("%+v %v", issues, err)
	}
	if got := issues[0].Comments[0].Mentions; !slices.Equal(got, []string{"acct-jsmith"}) {
		t.Errorf("mentions %q", got)
	}
}

func TestSearchKeepsGoodFieldsWhenOneIsMalformed(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"ABC-4","fields":{
			"summary":"Still here","status":{"id":"3","name":"In Progress"},
			"labels":["blocked"],"created":"not a time","updated":null}}]}`))
	})
	issues, err := c.Search(context.Background(), "project = ABC", "")
	if err != nil || len(issues) != 1 {
		t.Fatalf("%+v %v", issues, err)
	}
	a := issues[0]
	if a.Summary != "Still here" || a.StatusID != "3" || !slices.Equal(a.Labels, []string{"blocked"}) {
		t.Errorf("good fields lost: %+v", a)
	}
	if len(a.DecodeErrors) != 1 || !strings.HasPrefix(a.DecodeErrors[0], "created: ") {
		t.Errorf("decode errors %q, want one for created", a.DecodeErrors)
	}
}

func TestSearchFlaggedNotArrayIsDecodeError(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"ABC-5","fields":{
			"summary":"s","customfield_10021":{"value":"Impediment"}}}]}`))
	})
	issues, err := c.Search(context.Background(), "project = ABC", "customfield_10021")
	if err != nil || len(issues) != 1 {
		t.Fatalf("%+v %v", issues, err)
	}
	if a := issues[0]; a.Flagged || len(a.DecodeErrors) != 1 || !strings.HasPrefix(a.DecodeErrors[0], "customfield_10021: ") {
		t.Errorf("flagged %v decode errors %q", a.Flagged, a.DecodeErrors)
	}
}

func TestSearchMalformedStatusLeavesStatusEmpty(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"ABC-6","fields":{
			"summary":"s","status":{"id":3,"name":"In Progress"}}}]}`))
	})
	issues, err := c.Search(context.Background(), "project = ABC", "")
	if err != nil || len(issues) != 1 {
		t.Fatalf("%+v %v", issues, err)
	}
	if a := issues[0]; a.StatusID != "" || a.StatusName != "" || len(a.DecodeErrors) != 1 {
		t.Errorf("status %q/%q decode errors %q", a.StatusID, a.StatusName, a.DecodeErrors)
	}
}
