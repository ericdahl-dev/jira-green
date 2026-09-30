package jira_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
