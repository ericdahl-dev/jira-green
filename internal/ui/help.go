package ui

const helpText = `jira-green keys

  ←→↑↓ / hjkl   move between cards
  enter         open a card's detail; collapse a list group
  t             transition the selected card: t then enter, y to confirm
  o             open the selected card in the browser
  v             switch between kanban and list
  d             show or hide Done
  b             show or hide Backlog
  r             refresh now
  m             mute epics and cards: space to mute or unmute
  ?             toggle this help
  esc           close help, detail, or picker
  q / ctrl+c    quit`

// RenderHelp is the help overlay.
func RenderHelp() string { return helpText + "\n" }
