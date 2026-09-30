# Agent Instructions

## Issue tracker

Issues live in GitHub Issues (`ericdahl-dev/jira-green`).

## Domain docs

Design and implementation plans live in `docs/plans/`.

## Talking to a real Jira instance

Never hardcode a Jira token, site, board ID, or project key. Real values live only in
`~/.config/jira-green/config.toml`. Fixtures and golden files use `example.atlassian.net` and
project `ABC`.

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems.

```bash
cp -f source dest
mv -f source dest
rm -f file
rm -rf directory
```
