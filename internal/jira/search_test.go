package jira_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/jira"
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

const commentJSON = `{"author":{"accountId":%q},"created":"2026-09-2%dT09:00:00.000-0400","body":null}`

func TestSearchMarksTruncatedCommentsWithoutFetching(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("Search fetched %s; the poller fetches truncated comments", r.URL.Path)
		}
		_, _ = fmt.Fprintf(w, `{"isLast":true,"issues":[
			{"key":"ABC-7","fields":{"comment":{"total":3,"comments":[`+commentJSON+`]}}},
			{"key":"ABC-8","fields":{"comment":{"total":1,"comments":[`+commentJSON+`]}}}]}`,
			"acct-a", 1, "acct-z", 1)
	})
	issues, err := c.Search(context.Background(), "project = ABC", "")
	if err != nil || len(issues) != 2 {
		t.Fatalf("%+v %v", issues, err)
	}
	if a := issues[0]; !a.CommentsTruncated || len(a.Comments) != 1 || len(a.DecodeErrors) != 0 {
		t.Errorf("truncated issue: %+v", a)
	}
	if b := issues[1]; b.CommentsTruncated || len(b.Comments) != 1 {
		t.Errorf("untruncated issue: %+v", b)
	}
}

func TestCommentsFetchesNewest100(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rest/api/3/issue/ABC-7/comment" ||
			r.URL.Query().Get("orderBy") != "-created" || r.URL.Query().Get("maxResults") != "100" {
			t.Errorf("%s %s", r.Method, r.URL)
		}
		_, _ = fmt.Fprintf(w, `{"total":3,"comments":[`+commentJSON+`,`+commentJSON+`,`+commentJSON+`]}`,
			"acct-c", 3, "acct-b", 2, "acct-a", 1)
	})
	cms, err := c.Comments(context.Background(), "ABC-7")
	if err != nil {
		t.Fatal(err)
	}
	var authors []string
	for _, cm := range cms {
		authors = append(authors, cm.AuthorID)
	}
	if !slices.Equal(authors, []string{"acct-c", "acct-b", "acct-a"}) {
		t.Errorf("authors %v", authors)
	}
}

func TestCommentsRateLimitIsAPIError(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := c.Comments(context.Background(), "ABC-7")
	var ae *jira.APIError
	if !errors.As(err, &ae) || ae.Status != http.StatusTooManyRequests || ae.RetryAfter != 30*time.Second {
		t.Fatalf("got %v, want a 429 APIError with RetryAfter 30s", err)
	}
}

func TestSearchRepeatedPageTokenIsError(t *testing.T) {
	calls := 0
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 10 {
			t.Errorf("still paginating after %d calls", calls)
			_, _ = w.Write([]byte(`{"isLast":true,"issues":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"isLast":false,"nextPageToken":"same","issues":[{"key":"ABC-1","fields":{}}]}`))
	})
	if _, err := c.Search(context.Background(), "project = ABC", ""); err == nil {
		t.Fatal("want an error for a repeated nextPageToken")
	}
}

func TestSearchEndlessEmptyPagesHitPageCap(t *testing.T) {
	calls := 0
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 60 {
			t.Errorf("still paginating after %d calls", calls)
			_, _ = w.Write([]byte(`{"isLast":true,"issues":[]}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"isLast":false,"nextPageToken":"t%d","issues":[]}`, calls)
	})
	if _, err := c.Search(context.Background(), "project = ABC", ""); err == nil {
		t.Fatal("want an error once the page cap is exceeded, not a truncated result")
	}
	if calls != 50 {
		t.Errorf("calls %d, want the 50-page cap", calls)
	}
}

func TestSearchSendsFieldsAndMaxResults(t *testing.T) {
	var body map[string]any
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{"isLast":true,"issues":[]}`))
	})
	if _, err := c.Search(context.Background(), "project = ABC", "customfield_10021"); err != nil {
		t.Fatal(err)
	}
	var fields []string
	raw, _ := body["fields"].([]any)
	for _, f := range raw {
		s, _ := f.(string)
		fields = append(fields, s)
	}
	want := []string{"summary", "status", "assignee", "parent", "issuetype", "labels", "created", "updated", "comment", "customfield_10021"}
	if !slices.Equal(fields, want) {
		t.Errorf("fields %q, want %q", fields, want)
	}
	if body["maxResults"] != float64(100) || body["jql"] != "project = ABC" {
		t.Errorf("body %v", body)
	}
}

func TestSearchAPIErrorOnSecondPage(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["nextPageToken"] == "p2" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errorMessages":["token expired"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"isLast":false,"nextPageToken":"p2","issues":[{"key":"ABC-1","fields":{}}]}`))
	})
	issues, err := c.Search(context.Background(), "project = ABC", "")
	var ae *jira.APIError
	if !errors.As(err, &ae) || ae.Status != http.StatusBadRequest || issues != nil {
		t.Fatalf("issues %+v err %v, want a 400 APIError and no partial result", issues, err)
	}
}

func TestSearchSubtaskKeepsItsStoryAsParent(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if fields, _ := body["fields"].([]any); !slices.Contains(fields, any("issuetype")) {
			t.Errorf("fields %v missing issuetype", fields)
		}
		_, _ = w.Write([]byte(`{"isLast":true,"issues":[
			{"key":"ABC-13","fields":{"summary":"Write tests","issuetype":{"subtask":true},
			 "parent":{"key":"ABC-12","fields":{"summary":"Login story"}}}},
			{"key":"ABC-12","fields":{"summary":"Login story","issuetype":{"subtask":false},
			 "parent":{"key":"ABC-100","fields":{"summary":"Auth"}}}}]}`))
	})
	issues, err := c.Search(context.Background(), "project = ABC", "")
	if err != nil || len(issues) != 2 {
		t.Fatalf("%+v %v", issues, err)
	}
	sub, story := issues[0], issues[1]
	if sub.ParentKey != "ABC-12" || sub.ParentSummary != "Login story" {
		t.Errorf("subtask parent %q/%q, want ABC-12/Login story", sub.ParentKey, sub.ParentSummary)
	}
	// Until the poller resolves the real epic, a subtask groups under its story.
	if sub.EpicKey != "ABC-12" || sub.EpicSummary != "Login story" {
		t.Errorf("subtask epic %q/%q, want the story as fallback", sub.EpicKey, sub.EpicSummary)
	}
	if story.ParentKey != "" || story.EpicKey != "ABC-100" || story.EpicSummary != "Auth" {
		t.Errorf("story parent %q epic %q/%q", story.ParentKey, story.EpicKey, story.EpicSummary)
	}
}

func TestParentOf(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rest/api/3/issue/ABC-12" || r.URL.Query().Get("fields") != "parent,summary" {
			t.Errorf("%s %s", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(`{"key":"ABC-12","fields":{"summary":"Login story",
			"parent":{"key":"ABC-100","fields":{"summary":"Auth"}}}}`))
	})
	p, err := c.ParentOf(context.Background(), "ABC-12")
	if err != nil || p.Key != "ABC-100" || p.Summary != "Auth" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestParentOfNoParent(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"key":"ABC-12","fields":{"summary":"Login story","parent":null}}`))
	})
	p, err := c.ParentOf(context.Background(), "ABC-12")
	if err != nil || p != (jira.Parent{}) {
		t.Fatalf("%+v %v, want zero Parent", p, err)
	}
}
