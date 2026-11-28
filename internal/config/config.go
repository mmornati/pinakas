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

// Board providers.
const (
	ProviderGitHub = "github"
	ProviderGitLab = "gitlab"
)

// Board describes where the cards live.
type Board struct {
	// Provider is the board backend: "github" (Projects v2) or "gitlab"
	// (issues with one label per column).
	Provider string `yaml:"provider"`
	// URL is the GitLab instance; https://gitlab.com when empty. Unused
	// on GitHub.
	URL string `yaml:"url"`
	// Owner is the user or organization that owns the GitHub Project. On
	// GitLab it is an optional group path: the board then spans the group.
	Owner string `yaml:"owner"`
	// Project is the GitHub Project (v2) number. Unused on GitLab.
	Project int `yaml:"project"`
	// Repo is the default repository for issues, as owner/name (on GitLab
	// group/subgroup/name). On GitLab without owner, it is the board.
	Repo string `yaml:"repo"`
	// StatusField is the single-select field holding the column (GitHub).
	StatusField string `yaml:"status_field"`
	// Columns maps workflow keys to the option names of StatusField on
	// GitHub, or to labels on GitLab.
	Columns map[string]string `yaml:"columns"`
}

// ColumnKeys lists the workflow keys in order.
var ColumnKeys = []string{ColumnReady, ColumnInProgress, ColumnReview, ColumnDone}

// Rules toggles the guardrails enforced by the hooks.
type Rules struct {
	// RequireClaimBeforePR blocks `gh pr create` unless a card is claimed.
	RequireClaimBeforePR bool `yaml:"require_claim_before_pr"`
	// RequireActualsBeforeStop blocks the agent from stopping while a
	// claimed card has no actuals recorded.
	RequireActualsBeforeStop bool `yaml:"require_actuals_before_stop"`
	// RequireLinkedIssue fails the CI check when a pull request closes no
	// card of the board.
	RequireLinkedIssue bool `yaml:"require_linked_issue"`
	// RequireActualsOnPR fails the CI check while a linked card has no
	// actuals comment.
	RequireActualsOnPR bool `yaml:"require_actuals_on_pr"`
	// ApprovalLabels lists pull request labels (risk classes) that need an
	// approving review from a human other than the author.
	ApprovalLabels []string `yaml:"approval_labels"`
	// ExemptAuthors skips the CI check for these pull request authors,
	// such as dependency bots.
	ExemptAuthors []string `yaml:"exempt_authors"`
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
			RequireLinkedIssue:       true,
			RequireActualsOnPR:       true,
			ApprovalLabels:           []string{"risk:high"},
			ExemptAuthors:            []string{"dependabot[bot]", "renovate[bot]"},
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
	switch c.Board.Provider {
	case ProviderGitHub:
		if c.Board.Owner == "" {
			problems = append(problems, "board.owner is required")
		}
		if c.Board.Project <= 0 {
			problems = append(problems, "board.project must be a positive project number")
		}
		if c.Board.Repo != "" && strings.Count(c.Board.Repo, "/") != 1 {
			problems = append(problems, "board.repo must look like owner/name")
		}
	case ProviderGitLab:
		if c.Board.Repo == "" && c.Board.Owner == "" {
			problems = append(problems, "board.repo (project path) or board.owner (group path) is required on GitLab")
		}
		if c.Board.Repo != "" && !strings.Contains(c.Board.Repo, "/") {
			problems = append(problems, "board.repo must look like group/name")
		}
	default:
		problems = append(problems, fmt.Sprintf("board.provider %q is not supported (use \"github\" or \"gitlab\")", c.Board.Provider))
	}
	for _, key := range ColumnKeys {
		if c.Board.Columns[key] == "" {
			problems = append(problems, "board.columns."+key+" is required")
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid %s: %s", FileName, strings.Join(problems, "; "))
	}
	return nil
}

// Name describes the board for people: "acme project 3", or the GitLab
// group or project path.
func (c Config) Name() string {
	if c.Board.Provider == ProviderGitLab {
		if c.Board.Owner != "" {
			return "GitLab group " + c.Board.Owner
		}
		return "GitLab project " + c.Board.Repo
	}
	return fmt.Sprintf("%s project %d", c.Board.Owner, c.Board.Project)
}

// Column returns the board option name for a workflow key, or the input
// unchanged when it is already an option name.
func (c Config) Column(key string) string {
	if name, ok := c.Board.Columns[key]; ok {
		return name
	}
	return key
}
