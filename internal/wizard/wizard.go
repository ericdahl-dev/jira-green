// Package wizard runs the interactive `jira-green init` form.
package wizard

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/jira"
)

// API is the part of the Jira client Finish needs.
type API interface {
	Myself(ctx context.Context) (jira.User, error)
	FindFieldID(ctx context.Context, name string) (string, error)
}

// Answers is what the form collected. Exactly one of TokenCommand and
// TokenEnv is set.
type Answers struct {
	Site         string
	Email        string
	TokenCommand string
	TokenEnv     string
	BoardID      int
}

// Finish is the non-interactive end of the wizard: it verifies the account,
// looks up the Flagged field, and writes the config to path.
func Finish(ctx context.Context, api API, a Answers, path string, force bool) (*config.Config, error) {
	if (a.TokenCommand == "") == (a.TokenEnv == "") {
		return nil, errors.New("set exactly one of token command or token env")
	}
	c, err := config.New(path, config.Jira{
		Site:         a.Site,
		Email:        a.Email,
		TokenCommand: a.TokenCommand,
		TokenEnv:     a.TokenEnv,
		BoardID:      a.BoardID,
	})
	if err != nil {
		return nil, err
	}
	if _, err := api.Myself(ctx); err != nil {
		return nil, fmt.Errorf("verify account: %w", err)
	}
	if c.Jira.FlaggedField, err = api.FindFieldID(ctx, "Flagged"); err != nil {
		return nil, err
	}
	if force {
		return c, c.Save() // atomic: an existing file is replaced only now
	}
	return c, c.Create() // fails if a file appeared since RunInteractive checked
}

// validSite checks the site as it is typed; config.New checks it
// again.
func validSite(s string) error {
	if u, err := url.Parse(s); err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("site must be an https URL such as https://example.atlassian.net, got %q", s)
	}
	return nil
}
