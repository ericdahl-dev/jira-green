package wizard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/jira"
)

// ErrUserAborted is returned when the user cancels the form.
var ErrUserAborted = huh.ErrUserAborted

const (
	sourceCommand = "command"
	sourceEnv     = "env"

	tokenCommandPlaceholder = "security find-generic-password -s jira-green -w"
)

// RunInteractive asks for the site, account, token source, and board, then
// writes the config to path. With force, an existing file is replaced.
func RunInteractive(ctx context.Context, path string, force bool) error {
	// Early warning only, so nobody answers every question first; Create's atomic link is the binding rule.
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("config already exists at %s (use --force to overwrite)", path)
	}

	var a Answers
	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Jira site").
			Placeholder("https://example.atlassian.net").
			Value(&a.Site).
			Validate(func(s string) error { return validSite(strings.TrimSpace(s)) }),
		huh.NewInput().
			Title("Email").
			Description("The Atlassian account the API token belongs to.").
			Value(&a.Email).
			Validate(required("email")),
	).Title("jira-green init · Account")).Run()
	if err != nil {
		return err
	}
	a.Site = strings.TrimRight(strings.TrimSpace(a.Site), "/")
	a.Email = strings.TrimSpace(a.Email)

	client, err := askToken(ctx, &a)
	if err != nil {
		return err
	}

	boards, err := client.Boards(ctx)
	if err != nil {
		return fmt.Errorf("list boards: %w", err)
	}
	if len(boards) == 0 {
		return errors.New("no boards visible to this account")
	}
	opts := make([]huh.Option[int], len(boards))
	for i, b := range boards {
		opts[i] = huh.NewOption(boardLabel(b), b.ID)
	}
	err = huh.NewForm(huh.NewGroup(
		huh.NewSelect[int]().Title("Board").Options(opts...).Value(&a.BoardID),
	).Title("jira-green init · Board")).Run()
	if err != nil {
		return err
	}

	if _, err := Finish(ctx, client, a, path, force); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Wrote %s\n", path)
	return nil
}

// askToken asks for the token source until the token resolves and Myself
// accepts it, showing the last failure above the form.
func askToken(ctx context.Context, a *Answers) (*jira.Client, error) {
	source := sourceCommand
	command := ""
	envName := "JIRA_API_TOKEN"
	var lastErr error
	for {
		desc := "Create one at id.atlassian.com → Security → API tokens. It is never written to the config."
		if lastErr != nil {
			desc = "Check failed: " + lastErr.Error()
		}
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Where should the API token come from?").
					Options(
						huh.NewOption("A command (e.g. the macOS keychain)", sourceCommand),
						huh.NewOption("An environment variable", sourceEnv),
					).
					Value(&source),
			).Title("jira-green init · Token").Description(desc),
			huh.NewGroup(
				huh.NewInput().
					Title("Token command").
					Description("Shell command whose output is the token.").
					Placeholder(tokenCommandPlaceholder).
					Value(&command).
					Validate(required("token command")),
			).WithHideFunc(func() bool { return source != sourceCommand }),
			huh.NewGroup(
				huh.NewInput().
					Title("Environment variable name").
					Value(&envName).
					Validate(required("variable name")),
			).WithHideFunc(func() bool { return source != sourceEnv }),
		).Run()
		if err != nil {
			return nil, err
		}

		a.TokenCommand, a.TokenEnv = "", ""
		if source == sourceCommand {
			a.TokenCommand = strings.TrimSpace(command)
		} else {
			a.TokenEnv = strings.TrimSpace(envName)
		}
		client, err := verify(ctx, *a)
		if err == nil {
			return client, nil
		}
		lastErr = err
	}
}

func verify(ctx context.Context, a Answers) (*jira.Client, error) {
	token, err := config.ResolveToken(config.Jira{TokenCommand: a.TokenCommand, TokenEnv: a.TokenEnv})
	if err != nil {
		return nil, err
	}
	client := jira.New(a.Site, a.Email, token)
	if _, err := client.Myself(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

func boardLabel(b jira.Board) string {
	if b.ProjectKey == "" {
		return b.Name
	}
	return fmt.Sprintf("%s (%s)", b.Name, b.ProjectKey)
}

func required(what string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New(what + " is required")
		}
		return nil
	}
}
