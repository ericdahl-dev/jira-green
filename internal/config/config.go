// Package config loads and saves ~/.config/jira-green/config.toml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

type Settings struct {
	PollIntervalSeconds         int    `toml:"poll_interval_seconds"`
	BoardRefreshIntervalSeconds int    `toml:"board_refresh_interval_seconds"`
	StuckAlertAfter             string `toml:"stuck_alert_after"`
	DefaultView                 string `toml:"default_view"` // "kanban" | "list"
}

type Jira struct {
	Site         string `toml:"site"`
	Email        string `toml:"email"`
	Token        string `toml:"token,omitempty"`
	TokenEnv     string `toml:"token_env,omitempty"`
	TokenCommand string `toml:"token_command,omitempty"`
	BoardID      int    `toml:"board_id"`
	FlaggedField string `toml:"flagged_field,omitempty"` // e.g. customfield_10021; found by init
}

type JQL struct {
	Mine    string `toml:"mine"`
	Waiting string `toml:"waiting"`
	Done    string `toml:"done"`
}

type Age struct {
	Yellow string `toml:"yellow,omitempty"`
	Red    string `toml:"red,omitempty"`
}

type Webhook struct {
	URL    string `toml:"url"`
	Secret string `toml:"secret,omitempty"`
}

type Config struct {
	Settings      Settings       `toml:"settings"`
	Jira          Jira           `toml:"jira"`
	JQL           JQL            `toml:"jql"`
	Thresholds    map[string]Age `toml:"thresholds"`
	BlockedLabels []string       `toml:"blocked_labels"`
	Muted         []string       `toml:"muted"` // issue or epic keys
	Webhooks      []Webhook      `toml:"webhooks"`

	path string
}

const (
	DefaultMineJQL    = `assignee = currentUser() AND sprint IN openSprints() AND statusCategory != Done`
	DefaultWaitingJQL = `(reporter = currentUser() OR watcher = currentUser()) AND assignee != currentUser() AND statusCategory != Done`
	DefaultDoneJQL    = `assignee = currentUser() AND sprint IN openSprints() AND statusCategory = Done`
)

func defaultThresholds() map[string]Age {
	return map[string]Age{
		"In Progress": {Yellow: "3d", Red: "5d"},
		"Code Review": {Yellow: "1d", Red: "2d"},
		"UA":          {Yellow: "1d", Red: "2d"},
	}
}

// DefaultPath is ~/.config/jira-green/config.toml (XDG_CONFIG_HOME honored).
func DefaultPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "jira-green", "config.toml")
}

func Load(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	c.path = path
	if err := c.applyDefaultsAndValidate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) applyDefaultsAndValidate() error {
	s := &c.Settings
	if s.PollIntervalSeconds <= 0 {
		s.PollIntervalSeconds = 60
	}
	if s.BoardRefreshIntervalSeconds <= 0 {
		s.BoardRefreshIntervalSeconds = 600
	}
	if s.StuckAlertAfter == "" {
		s.StuckAlertAfter = "2h"
	}
	if s.DefaultView == "" {
		s.DefaultView = "kanban"
	}
	if s.DefaultView != "kanban" && s.DefaultView != "list" {
		return fmt.Errorf("settings.default_view must be kanban or list, got %q", s.DefaultView)
	}
	if _, err := model.ParseAge(s.StuckAlertAfter); err != nil {
		return fmt.Errorf("settings.stuck_alert_after: %w", err)
	}
	if strings.TrimSpace(c.Jira.Site) == "" {
		return errors.New("jira.site is required")
	}
	c.Jira.Site = strings.TrimRight(c.Jira.Site, "/")
	if c.Jira.BoardID <= 0 {
		return errors.New("jira.board_id is required")
	}
	if c.JQL.Mine == "" {
		c.JQL.Mine = DefaultMineJQL
	}
	if c.JQL.Waiting == "" {
		c.JQL.Waiting = DefaultWaitingJQL
	}
	if c.JQL.Done == "" {
		c.JQL.Done = DefaultDoneJQL
	}
	// Defaults apply per column; a user entry replaces only its own column,
	// and an entry with neither yellow nor red disables that column.
	merged := defaultThresholds()
	for col, a := range c.Thresholds {
		merged[col] = a
	}
	c.Thresholds = merged
	if c.BlockedLabels == nil {
		c.BlockedLabels = []string{"blocked"}
	}
	_, err := c.Rules("")
	return err
}

// Rules builds the health rules for the given account ID.
func (c *Config) Rules(me string) (model.Rules, error) {
	r := model.Rules{Me: me, BlockedLabels: c.BlockedLabels, Thresholds: map[string]model.Threshold{}}
	for col, a := range c.Thresholds {
		var th model.Threshold
		var err error
		if a.Yellow != "" {
			if th.Yellow, err = model.ParseAge(a.Yellow); err != nil {
				return r, fmt.Errorf("thresholds.%q.yellow: %w", col, err)
			}
		}
		if a.Red != "" {
			if th.Red, err = model.ParseAge(a.Red); err != nil {
				return r, fmt.Errorf("thresholds.%q.red: %w", col, err)
			}
		}
		r.Thresholds[col] = th
	}
	return r, nil
}

func (c *Config) Path() string { return c.path }

func (c *Config) IsMuted(key string) bool { return slices.Contains(c.Muted, key) }

// SetMuted adds or removes a key from the mute list and saves.
func (c *Config) SetMuted(key string, muted bool) error {
	has := c.IsMuted(key)
	switch {
	case muted && !has:
		c.Muted = append(c.Muted, key)
	case !muted && has:
		c.Muted = slices.DeleteFunc(c.Muted, func(k string) bool { return k == key })
	default:
		return nil
	}
	return c.Save()
}

func (c *Config) Save() error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(c.path, buf.Bytes(), 0o600)
}

// ResolveToken returns the API token: token_command, then token_env, then the
// literal token, then $JIRA_API_TOKEN (the variable go-jira-cli also reads).
func ResolveToken(j Jira) (string, error) {
	if j.TokenCommand != "" {
		cmd := exec.Command("sh", "-c", j.TokenCommand)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("token_command failed: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		tok := strings.TrimSpace(string(out))
		if tok == "" {
			return "", errors.New("token_command produced no output")
		}
		return tok, nil
	}
	if j.TokenEnv != "" {
		if tok := strings.TrimSpace(os.Getenv(j.TokenEnv)); tok != "" {
			return tok, nil
		}
	}
	if tok := strings.TrimSpace(j.Token); tok != "" {
		return tok, nil
	}
	if tok := strings.TrimSpace(os.Getenv("JIRA_API_TOKEN")); tok != "" {
		return tok, nil
	}
	return "", errors.New("no token — set token_command, token_env, token, or JIRA_API_TOKEN")
}

// WriteStarter writes a new config for the wizard. It refuses to overwrite.
func WriteStarter(path string, j Jira) (*Config, error) {
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("%s already exists (use --force)", path)
	}
	c := &Config{Jira: j, path: path}
	if err := c.applyDefaultsAndValidate(); err != nil {
		return nil, err
	}
	return c, c.Save()
}
