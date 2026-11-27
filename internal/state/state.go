// Package state keeps the card claimed in the current working tree.
//
// The state lives inside the git directory (per worktree), so parallel
// agents working in separate worktrees each track their own card and
// nothing is ever committed.
package state

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Claim is the card an agent is currently working on.
type Claim struct {
	Repo      string    `json:"repo"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url,omitempty"`
	ItemID    string    `json:"item_id"`
	ClaimedBy string    `json:"claimed_by,omitempty"`
	ClaimedAt time.Time `json:"claimed_at"`
	// ActualsAt is set once actuals have been recorded for the claim.
	ActualsAt *time.Time `json:"actuals_at,omitempty"`
}

// Open reports whether the claim still needs actuals.
func (c *Claim) Open() bool { return c != nil && c.ActualsAt == nil }

// State is the persisted content of the state file.
type State struct {
	Current *Claim `json:"current,omitempty"`
}

// Store reads and writes the state file.
type Store struct {
	Path string
}

// DefaultStore places the state in the git directory of dir, or in
// dir/.pinakas when dir is not inside a git repository.
func DefaultStore(dir string) Store {
	cmd := exec.Command("git", "rev-parse", "--absolute-git-dir")
	cmd.Dir = dir
	if out, err := cmd.Output(); err == nil {
		gitDir := strings.TrimSpace(string(out))
		return Store{Path: filepath.Join(gitDir, "pinakas", "state.json")}
	}
	return Store{Path: filepath.Join(dir, ".pinakas", "state.json")}
}

// Load returns the stored state, or an empty state when none exists.
func (s Store) Load() (State, error) {
	raw, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{}, err
	}
	return st, nil
}

// Save writes the state atomically.
func (s Store) Save(st State) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}
