package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAppliesDefaults(t *testing.T) {
	cfg, err := Parse([]byte("board:\n  owner: acme\n  project: 3\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Board.StatusField != "Status" {
		t.Errorf("StatusField = %q, want Status", cfg.Board.StatusField)
	}
	if got := cfg.Column(ColumnInProgress); got != "In Progress" {
		t.Errorf("Column(in_progress) = %q", got)
	}
	if !cfg.Rules.RequireClaimBeforePR || !cfg.Rules.RequireActualsBeforeStop {
		t.Error("rules should default to enabled")
	}
}

func TestParseOverridesColumns(t *testing.T) {
	cfg, err := Parse([]byte("board:\n  owner: acme\n  project: 3\n  columns:\n    in_progress: Doing\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.Column(ColumnInProgress); got != "Doing" {
		t.Errorf("Column(in_progress) = %q, want Doing", got)
	}
	if got := cfg.Column(ColumnDone); got != "Done" {
		t.Errorf("Column(done) = %q, want Done (default kept)", got)
	}
	if got := cfg.Column("Blocked"); got != "Blocked" {
		t.Errorf("unknown key should pass through, got %q", got)
	}
}

func TestValidateReportsProblems(t *testing.T) {
	_, err := Parse([]byte("board:\n  provider: jira\n  repo: nope\n"))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"provider", "owner", "project", "owner/name"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
}

func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, FileName)
	if err := os.WriteFile(want, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Find(nested)
	if err != nil || got != want {
		t.Fatalf("Find = %q, %v; want %q", got, err, want)
	}
	if _, err := Find(t.TempDir()); err == nil {
		t.Error("expected ErrNotFound in an empty tree")
	}
}
