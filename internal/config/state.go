package config

import (
	"bytes"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// State is state.toml: what the app remembers between runs. Unlike
// config.toml it is written by the app, never by hand, so a missing or
// unreadable file is not an error and unknown keys are ignored.
type State struct {
	View string `toml:"view,omitempty"` // "kanban" | "list"; "" = default_view
}

// StatePath is state.toml in the directory of the config at configPath.
func StatePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "state.toml")
}

// LoadState reads the state at path. A missing or corrupt file, or an
// unknown view, reads as the zero State.
func LoadState(path string) State {
	var s State
	if _, err := toml.DecodeFile(path, &s); err != nil {
		return State{}
	}
	if s.View != "kanban" && s.View != "list" {
		s.View = ""
	}
	return s
}

// SaveState writes s to path atomically, mode 0600, like Config.Save.
func SaveState(path string, s State) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(s); err != nil {
		return err
	}
	return writeAtomic(path, buf.Bytes())
}
