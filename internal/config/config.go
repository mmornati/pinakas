// Package config loads and validates the .pinakas.yml file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// FileName is the name of the configuration file looked up from the
// current directory upwards.
const FileName = ".pinakas.yml"

// Column keys used by the workflow. Each maps to a Status option on the board.
const (
	ColumnReady      = "ready"
	ColumnInProgress = "in_progress"
	ColumnReview     = "review"
	ColumnDone       = "done"
)

// Config is the content of .pinakas.yml.
type Config struct {
	Board Board `yaml:"board"`
	Rules Rules `yaml:"rules"`
}

// Board describes where the cards live.
type Board struct {
	// Provider is the board backend. Only "github" is supported in v0.1.
	Provider string `yaml:"provider"`
	// Owner is the user or organization that owns the GitHub Project.
	Owner string `yaml:"owner"`
	// Project is the GitHub Project (v2) number.
	Project int `yaml:"project"`
	// Repo is the default repository for issues, as owner/name.
	Repo string `yaml:"repo"`
	// StatusField is the single-select field holding the column.
	StatusField string `yaml:"status_field"`
	// Columns maps workflow keys to the option names of StatusField.
	Columns map[string]string `yaml:"columns"`
}

// Rules toggles the guardrails enforced by the hooks.
type Rules struct {
	// RequireClaimBeforePR blocks `gh pr create` unless a card is claimed.
	RequireClaimBeforePR bool `yaml:"require_claim_before_pr"`
	// RequireActualsBeforeStop blocks the agent from stopping while a
	// claimed card has no actuals recorded.
	RequireActualsBeforeStop bool `yaml:"require_actuals_before_stop"`
}

// Default returns a configuration with sensible defaults.
func Default() Config {
	return Config{
		Board: Board{
			Provider:    "github",
			StatusField: "Status",
			Columns: map[string]string{
				ColumnReady:      "Ready",
				ColumnInProgress: "In Progress",
				ColumnReview:     "In Review",
				ColumnDone:       "Done",
			},
		},
		Rules: Rules{
			RequireClaimBeforePR:     true,
			RequireActualsBeforeStop: true,
		},
	}
}

// ErrNotFound is returned when no configuration file exists.
var ErrNotFound = errors.New("no " + FileName + " found (run `pinakas init`)")

// Find walks from dir up to the filesystem root looking for FileName.
func Find(dir string) (string, error) {
	for {
		p := filepath.Join(dir, FileName)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}

// Load reads and validates the configuration at path.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return Parse(raw)
}

// Parse decodes raw YAML on top of the defaults and validates it.
func Parse(raw []byte) (Config, error) {
	cfg := Default()
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", FileName, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate reports the first problem found in the configuration.
func (c Config) Validate() error {
	var problems []string
	if c.Board.Provider != "github" {
		problems = append(problems, fmt.Sprintf("board.provider %q is not supported (only \"github\")", c.Board.Provider))
	}
	if c.Board.Owner == "" {
		problems = append(problems, "board.owner is required")
	}
	if c.Board.Project <= 0 {
		problems = append(problems, "board.project must be a positive project number")
	}
	if c.Board.Repo != "" && strings.Count(c.Board.Repo, "/") != 1 {
		problems = append(problems, "board.repo must look like owner/name")
	}
	for _, key := range []string{ColumnReady, ColumnInProgress, ColumnReview, ColumnDone} {
		if c.Board.Columns[key] == "" {
			problems = append(problems, "board.columns."+key+" is required")
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid %s: %s", FileName, strings.Join(problems, "; "))
	}
	return nil
}

// Column returns the board option name for a workflow key, or the input
// unchanged when it is already an option name.
func (c Config) Column(key string) string {
	if name, ok := c.Board.Columns[key]; ok {
		return name
	}
	return key
}
