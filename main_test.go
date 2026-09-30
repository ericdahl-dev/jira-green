package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ericdahl-dev/jira-green/internal/wizard"
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

func TestRunMissingConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	code := run(nil, &out, &out)
	if code != 1 || !strings.Contains(out.String(), "no config - run: jira-green init") {
		t.Fatalf("code %d out %q", code, out.String())
	}
}

// writeConfig writes body as the config under a fresh XDG_CONFIG_HOME.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "jira-green"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "jira-green", "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunInvalidConfig(t *testing.T) {
	writeConfig(t, "[jira]\nsite = \"https://example.atlassian.net\"\n")
	var out bytes.Buffer
	code := run(nil, &out, &out)
	if code != 1 || !strings.Contains(out.String(), "jira.email is required") {
		t.Fatalf("code %d out %q", code, out.String())
	}
}

const validConfig = `[jira]
site = "https://example.atlassian.net"
email = "me@example.com"
token_env = "JIRA_GREEN_TEST_TOKEN"
board_id = 1
`

func TestRunTokenUnresolved(t *testing.T) {
	writeConfig(t, validConfig)
	t.Setenv("JIRA_GREEN_TEST_TOKEN", "")
	var out bytes.Buffer
	code := run(nil, &out, &out)
	if code != 1 || !strings.Contains(out.String(), `token_env "JIRA_GREEN_TEST_TOKEN" is unset`) {
		t.Fatalf("code %d out %q", code, out.String())
	}
}

// fakeWizard replaces runWizard for one test and records its calls.
type fakeWizard struct {
	calls int
	path  string
	force bool
	err   error
}

func stubWizard(t *testing.T, err error) *fakeWizard {
	t.Helper()
	f := &fakeWizard{err: err}
	old := runWizard
	runWizard = func(_ context.Context, path string, force bool) error {
		f.calls++
		f.path, f.force = path, force
		return f.err
	}
	t.Cleanup(func() { runWizard = old })
	return f
}

func TestRunInitRunsWizard(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	w := stubWizard(t, nil)
	var out bytes.Buffer
	if code := run([]string{"init"}, &out, &out); code != 0 {
		t.Fatalf("exit %d: %q", code, out.String())
	}
	want := filepath.Join(dir, "jira-green", "config.toml")
	if w.calls != 1 || w.path != want || w.force {
		t.Fatalf("wizard %+v, want one call at %s without force", w, want)
	}
}

func TestRunInitForce(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	w := stubWizard(t, nil)
	var out bytes.Buffer
	if code := run([]string{"init", "--force"}, &out, &out); code != 0 || !w.force {
		t.Fatalf("exit %d, wizard %+v", code, w)
	}
}

func TestRunInitAbortedIsQuiet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stubWizard(t, fmt.Errorf("form: %w", wizard.ErrUserAborted))
	var out bytes.Buffer
	if code := run([]string{"init"}, &out, &out); code != 0 || out.Len() != 0 {
		t.Fatalf("exit %d out %q", code, out.String())
	}
}

func TestRunInitFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	stubWizard(t, errors.New("list boards: boom"))
	var out bytes.Buffer
	if code := run([]string{"init"}, &out, &out); code != 1 || !strings.Contains(out.String(), "boom") {
		t.Fatalf("exit %d out %q", code, out.String())
	}
}
