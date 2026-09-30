package wizard_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/jira"
	"github.com/ericdahl-dev/jira-green/internal/wizard"
)

type fakeAPI struct {
	myselfErr error
	findErr   error
	flagged   string
}

func (f fakeAPI) Myself(context.Context) (jira.User, error) {
	return jira.User{AccountID: "acct-me"}, f.myselfErr
}

func (f fakeAPI) FindFieldID(context.Context, string) (string, error) {
	return f.flagged, f.findErr
}

func answers() wizard.Answers {
	return wizard.Answers{
		Site:         "https://example.atlassian.net",
		Email:        "me@example.com",
		TokenCommand: "security find-generic-password -s jira-green -w",
		BoardID:      42,
	}
}

func TestFinishWritesLoadableConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if _, err := wizard.Finish(context.Background(), fakeAPI{flagged: "customfield_10021"}, answers(), path, false); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Jira.BoardID != 42 || c.Jira.FlaggedField != "customfield_10021" {
		t.Fatalf("jira = %+v", c.Jira)
	}
}

func TestFinishMyselfFailureWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	api := fakeAPI{myselfErr: errors.New("401 Unauthorized")}
	if _, err := wizard.Finish(context.Background(), api, answers(), path, false); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config written: %v", err)
	}
}

func TestFinishExistingFileWithoutForceErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wizard.Finish(context.Background(), fakeAPI{}, answers(), path, false); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(path); string(b) != "old" {
		t.Fatalf("file changed: %q", b)
	}
}

func TestFinishForceReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wizard.Finish(context.Background(), fakeAPI{}, answers(), path, true); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err != nil {
		t.Fatal(err)
	}
}

func TestFinishEmptyFlaggedFieldIsOK(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if _, err := wizard.Finish(context.Background(), fakeAPI{flagged: ""}, answers(), path, false); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Jira.FlaggedField != "" {
		t.Fatalf("flagged_field = %q", c.Jira.FlaggedField)
	}
}

func TestFinishRejectsNonHTTPSSiteBeforeTouchingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := answers()
	a.Site = "http://example.atlassian.net"
	if _, err := wizard.Finish(context.Background(), fakeAPI{}, a, path, true); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(path); string(b) != "old" {
		t.Fatalf("file changed: %q", b)
	}
}

func TestFinishNeverWritesLiteralToken(t *testing.T) {
	for name, a := range map[string]wizard.Answers{
		"command": answers(),
		"env":     {Site: "https://example.atlassian.net", Email: "me@example.com", TokenEnv: "JIRA_API_TOKEN", BoardID: 42},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if _, err := wizard.Finish(context.Background(), fakeAPI{}, a, path, false); err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if _, err := toml.DecodeFile(path, &doc); err != nil {
				t.Fatal(err)
			}
			jiraTable, _ := doc["jira"].(map[string]any)
			for where, table := range map[string]map[string]any{"top level": doc, "[jira]": jiraTable} {
				if _, ok := table["token"]; ok {
					t.Errorf("%s holds a literal token: %v", where, table)
				}
			}
		})
	}
}

func TestFinishForceKeepsOldFileWhenAnswersInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	old := []byte("[jira]\nsite = \"https://example.atlassian.net\"\n")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	a := answers()
	a.Email = ""
	if _, err := wizard.Finish(context.Background(), fakeAPI{}, a, path, true); err == nil {
		t.Fatal("want error")
	}
	if b, _ := os.ReadFile(path); string(b) != string(old) {
		t.Fatalf("file changed: %q", b)
	}
}

func TestFinishRequiresExactlyOneTokenSource(t *testing.T) {
	both := answers()
	both.TokenEnv = "JIRA_API_TOKEN"
	neither := answers()
	neither.TokenCommand = ""
	for name, a := range map[string]wizard.Answers{"both": both, "neither": neither} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if _, err := wizard.Finish(context.Background(), fakeAPI{}, a, path, false); err == nil {
				t.Fatal("want error")
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("config written: %v", err)
			}
		})
	}
}

func TestFinishForceKeepsOldFileWhenJiraFails(t *testing.T) {
	for name, api := range map[string]fakeAPI{
		"Myself":      {myselfErr: errors.New("401 Unauthorized")},
		"FindFieldID": {findErr: errors.New("500 Internal Server Error")},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			old := []byte("[jira]\nsite = \"https://old.example.net\"\n")
			if err := os.WriteFile(path, old, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := wizard.Finish(context.Background(), api, answers(), path, true); err == nil {
				t.Fatal("want error")
			}
			if b, _ := os.ReadFile(path); string(b) != string(old) {
				t.Fatalf("file changed: %q", b)
			}
		})
	}
}

func TestFinishReturnsNoConfigOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, err := wizard.Finish(context.Background(), fakeAPI{}, answers(), path, false); err == nil || c != nil {
		t.Errorf("onto an existing file: config %v, err %v; want nil and an error", c, err)
	}
}
