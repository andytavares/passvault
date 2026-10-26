// Package syncer copies secrets from 1Password into the pass store and
// records what it did. Values live only in local buffers, never in state,
// reports, errors or argv.
package syncer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// EntryState records the last sync of one pass entry.
type EntryState struct {
	SyncedAt time.Time `json:"synced_at"`
	Tag      string    `json:"tag"`
}

// State is the on-disk sync state. It holds names, times and tags only.
type State struct {
	Entries map[string]EntryState `json:"entries"`
	LastTag string                `json:"last_tag"`
}

// LoadState reads the state file at path; a missing file yields empty state.
func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{Entries: map[string]EntryState{}}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	if state.Entries == nil {
		state.Entries = map[string]EntryState{}
	}
	return state, nil
}

// SaveState writes state to path as JSON (0600, parent dir 0700).
func SaveState(path string, state State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
