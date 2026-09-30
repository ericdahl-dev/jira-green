package jira_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ericdahl-dev/jira-green/internal/jira"
)

func newTest(t *testing.T, h http.HandlerFunc) *jira.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return jira.New(srv.URL, "me@example.com", "tok")
}

func TestTimeParsesJiraFormat(t *testing.T) {
	var jt jira.Time
	if err := jt.UnmarshalJSON([]byte(`"2026-09-30T10:00:00.000-0400"`)); err != nil {
		t.Fatal(err)
	}
	if !jt.Equal(time.Date(2026, 9, 30, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("got %v", jt.Time)
	}
	if err := jt.UnmarshalJSON([]byte(`null`)); err != nil || !jt.IsZero() {
		t.Errorf("null should be zero")
	}
}

func TestMyselfSendsBasicAuth(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "me@example.com" || p != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/rest/api/3/myself" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"accountId":"acct-me","displayName":"Me"}`))
	})
	me, err := c.Myself(context.Background())
	if err != nil || me.AccountID != "acct-me" {
		t.Fatalf("%+v %v", me, err)
	}
}

func TestAuthErrorIsDetectable(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	_, err := c.Myself(context.Background())
	if !jira.IsAuth(err) {
		t.Fatalf("want auth error, got %v", err)
	}
}

func TestRateLimitCarriesRetryAfter(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := c.Myself(context.Background())
	var ae *jira.APIError
	if !errors.As(err, &ae) || ae.RetryAfter != 30*time.Second {
		t.Fatalf("got %v", err)
	}
}

func TestForbiddenIsNotAuth(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	_, err := c.Myself(context.Background())
	if err == nil || jira.IsAuth(err) {
		t.Fatalf("403 should be an error but not an auth error, got %v", err)
	}
}

func TestAPIErrorSummarizesJiraEnvelope(t *testing.T) {
	body := `{"errorMessages":["The JQL is bad."],"errors":{"summary":"required","assignee":"unknown user"}}`
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	})
	_, err := c.Myself(context.Background())
	var ae *jira.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("got %v", err)
	}
	want := []string{"The JQL is bad.", "assignee: unknown user", "summary: required"}
	if !slices.Equal(ae.Messages, want) {
		t.Errorf("messages %q, want %q", ae.Messages, want)
	}
	if got := err.Error(); got != "jira: HTTP 400: The JQL is bad.; assignee: unknown user; summary: required" {
		t.Errorf("Error() = %q", got)
	}
	if ae.Body != body {
		t.Errorf("raw body %q", ae.Body)
	}
}

func TestAPIErrorHTMLBodyUsesStatusText(t *testing.T) {
	body := "<html><body><h1>502 Bad Gateway</h1></body></html>"
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(body))
	})
	_, err := c.Myself(context.Background())
	var ae *jira.APIError
	if !errors.As(err, &ae) || ae.Body != body || len(ae.Messages) != 0 {
		t.Fatalf("got %#v", err)
	}
	if got := err.Error(); got != "jira: HTTP 502 Bad Gateway" {
		t.Errorf("Error() = %q", got)
	}
}

func TestAPIErrorTruncatesLongMessages(t *testing.T) {
	long := strings.Repeat("é", 500)
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"errorMessages":[%q]}`, long)
	})
	_, err := c.Myself(context.Background())
	got := err.Error()
	if n := utf8.RuneCountInString(got); n > 205 || !strings.HasPrefix(got, "jira: HTTP 400: éé") {
		t.Errorf("Error() is %d runes: %q", n, got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncation split a rune: %q", got)
	}
}
