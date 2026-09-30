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
	"time"

	"github.com/BurntSushi/toml"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

type Settings struct {
	PollIntervalSeconds         int    `toml:"poll_interval_seconds,omitempty"`
	BoardRefreshIntervalSeconds int    `toml:"board_refresh_interval_seconds,omitempty"`
	StuckAlertAfter             string `toml:"stuck_alert_after,omitempty"`
	DefaultView                 string `toml:"default_view,omitempty"` // "kanban" | "list"
}

type Jira struct {
	Site         string `toml:"site"`
	Email        string `toml:"email,omitempty"`
	Token        string `toml:"token,omitempty"`
	TokenEnv     string `toml:"token_env,omitempty"`
	TokenCommand string `toml:"token_command,omitempty"`
	BoardID      int    `toml:"board_id,omitempty"`
	FlaggedField string `toml:"flagged_field,omitempty"` // e.g. customfield_10021; found by init
}

type JQL struct {
	Mine    string `toml:"mine,omitempty"`
	Waiting string `toml:"waiting,omitempty"`
	Done    string `toml:"done,omitempty"`
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
	Settings      Settings       `toml:"settings,omitempty"`
	Jira          Jira           `toml:"jira"`
	JQL           JQL            `toml:"jql,omitempty"`
	Thresholds    map[string]Age `toml:"thresholds,omitempty"`
	BlockedLabels []string       `toml:"blocked_labels"` // nil = default; [] = none
	Muted         []string       `toml:"muted,omitempty"` // issue or epic keys
	Webhooks      []Webhook      `toml:"webhooks,omitempty"`

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
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("%s: unknown key(s): %s", path, strings.Join(keys, ", "))
	}
	c.path = path
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// validate checks the values the user wrote. It writes nothing back except
// normalising jira.site, so Save persists only what the user wrote; defaults
// are resolved by the accessors.
func (c *Config) validate() error {
	if v := c.DefaultView(); v != "kanban" && v != "list" {
		return fmt.Errorf("settings.default_view must be kanban or list, got %q", v)
	}
	if c.Settings.StuckAlertAfter != "" {
		if _, err := model.ParseAge(c.Settings.StuckAlertAfter); err != nil {
			return fmt.Errorf("settings.stuck_alert_after: %w", err)
		}
	}
	if strings.TrimSpace(c.Jira.Site) == "" {
		return errors.New("jira.site is required")
	}
	c.Jira.Site = strings.TrimRight(c.Jira.Site, "/")
	if c.Jira.BoardID <= 0 {
		return errors.New("jira.board_id is required")
	}
	_, err := c.Rules("")
	return err
}

// MineJQL is the query for the Mine lane.
func (c *Config) MineJQL() string { return orDefault(c.JQL.Mine, DefaultMineJQL) }

// WaitingJQL is the query for the Waiting on others lane.
func (c *Config) WaitingJQL() string { return orDefault(c.JQL.Waiting, DefaultWaitingJQL) }

// DoneJQL is the query for the Done this sprint lane.
func (c *Config) DoneJQL() string { return orDefault(c.JQL.Done, DefaultDoneJQL) }

// PollInterval is how often issues are polled. Default 60s.
func (c *Config) PollInterval() time.Duration {
	return secondsOr(c.Settings.PollIntervalSeconds, 60)
}

// BoardRefreshInterval is how often the board configuration is re-read.
// Default 10m.
func (c *Config) BoardRefreshInterval() time.Duration {
	return secondsOr(c.Settings.BoardRefreshIntervalSeconds, 600)
}

// StuckAlertAfter is how long a card must stay red before webhooks fire.
// Default 2h. Load has already validated the value.
func (c *Config) StuckAlertAfter() time.Duration {
	d, err := model.ParseAge(orDefault(c.Settings.StuckAlertAfter, "2h"))
	if err != nil {
		return 2 * time.Hour
	}
	return d
}

// DefaultView is "kanban" or "list". Default kanban.
func (c *Config) DefaultView() string { return orDefault(c.Settings.DefaultView, "kanban") }

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func secondsOr(n, def int) time.Duration {
	if n <= 0 {
		n = def
	}
	return time.Duration(n) * time.Second
}

// Rules builds the health rules for the given account ID. Threshold defaults
// apply per column: a user [thresholds.X] table replaces only column X, and a
// table with neither yellow nor red disables that column. BlockedLabels
// defaults to ["blocked"] only when the key is absent.
func (c *Config) Rules(me string) (model.Rules, error) {
	labels := c.BlockedLabels
	if labels == nil {
		labels = []string{"blocked"}
	}
	merged := defaultThresholds()
	for col, a := range c.Thresholds {
		merged[col] = a
	}
	r := model.Rules{Me: me, BlockedLabels: labels, Thresholds: map[string]model.Threshold{}}
	for col, a := range merged {
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
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, c.Save()
}
