// Command jira-green is a terminal dashboard for Jira ticket flow health.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/wizard"
)

var version = "dev"

// runWizard is the init wizard; tests replace it.
var runWizard = wizard.RunInteractive

const usage = `jira-green — terminal dashboard for Jira ticket flow health

Usage:
  jira-green            launch the dashboard
  jira-green init       create a starter config
  jira-green --version  print the version
  jira-green --help     show this help
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-v", "version":
			_, _ = fmt.Fprintf(stdout, "jira-green %s\n", version)
			return 0
		case "--help", "-h", "help":
			_, _ = fmt.Fprint(stdout, usage)
			return 0
		}
	}
	path, err := config.DefaultPath()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "jira-green: %v\n", err)
		return 1
	}
	if len(args) > 0 && args[0] == "init" {
		return runInit(args[1:], path, stderr)
	}
	cfg, err := config.Load(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			_, _ = fmt.Fprintln(stderr, "no config - run: jira-green init")
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "jira-green: %v\n", err)
		return 1
	}
	if _, err := config.ResolveToken(cfg.Jira); err != nil {
		_, _ = fmt.Fprintf(stderr, "jira-green: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(stderr, "dashboard not implemented yet")
	return 1
}

func runInit(args []string, path string, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	force := flags.Bool("force", false, "replace an existing config")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	err := runWizard(context.Background(), path, *force)
	switch {
	case errors.Is(err, wizard.ErrUserAborted):
		return 0
	case err != nil:
		_, _ = fmt.Fprintf(stderr, "jira-green init: %v\n", err)
		return 1
	}
	return 0
}
