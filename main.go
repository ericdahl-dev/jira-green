// Command jira-green is a terminal dashboard for Jira ticket flow health.
package main

import (
	"fmt"
	"io"
	"os"
)

var version = "dev"

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
	_, _ = fmt.Fprintln(stderr, "dashboard not implemented yet")
	return 1
}
