package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	code := run([]string{"--version"}, &out, &out)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "jira-green") {
		t.Fatalf("got %q", out.String())
	}
}

func TestRunHelp(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"--help"}, &out, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"jira-green init", "--version"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help missing %q", want)
		}
	}
}
