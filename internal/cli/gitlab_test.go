package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmornati/pinakas/internal/board/gitlab"
	"github.com/mmornati/pinakas/internal/config"
)

func TestInitGitLab(t *testing.T) {
	h := newHarness(t, false)
	if code := h.run("", "init", "--provider", "gitlab"); code == 0 || !strings.Contains(h.stderr.String(), "--repo group/project") {
		t.Errorf("gitlab init without repo: code %d, %q", code, h.stderr)
	}
	if code := h.run("", "init", "--provider", "gitlab", "--url", "https://gitlab.example.com", "--repo", "acme/platform/app"); code != 0 {
		t.Fatalf("init: %q", h.stderr)
	}
	cfg, err := config.Load(filepath.Join(h.app.Dir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Board.Provider != "gitlab" || cfg.Board.URL != "https://gitlab.example.com" || cfg.Column(config.ColumnReview) != "workflow::review" || cfg.Name() != "GitLab project acme/platform/app" {
		t.Errorf("cfg = %+v", cfg.Board)
	}

	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GL_TOKEN", "")
	if _, err := newBoard(cfg); err == nil || !strings.Contains(err.Error(), "GITLAB_TOKEN") {
		t.Errorf("newBoard without token: %v", err)
	}
	t.Setenv("GL_TOKEN", "x")
	if b, err := newBoard(cfg); err != nil {
		t.Errorf("newBoard: %v", err)
	} else if _, ok := b.(*gitlab.Board); !ok {
		t.Errorf("newBoard returned %T", b)
	}
	if code := h.run("", "init", "--provider", "jira", "--repo", "a/b"); code == 0 {
		t.Error("unknown provider must fail")
	}
}

func TestStatusHookOnGitLab(t *testing.T) {
	h := newHarness(t, false)
	cfg := "board:\n  provider: gitlab\n  owner: acme\n  repo: acme/app\n  columns:\n    ready: Ready\n"
	if err := os.WriteFile(filepath.Join(h.app.Dir, config.FileName), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := h.run("", "status", "--hook"); code != 0 || !strings.Contains(h.stdout.String(), "live board state (GitLab group acme)") {
		t.Errorf("status --hook: %q %q", h.stdout, h.stderr)
	}
	glab := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"glab mr create --fill"}}`
	if code := h.run(glab, "check-pr"); code != blockedExitCode {
		t.Errorf("glab mr create without a claim must be blocked, got %d", code)
	}
}
