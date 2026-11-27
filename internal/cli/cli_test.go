package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmornati/pinakas/internal/board"
	"github.com/mmornati/pinakas/internal/config"
	"github.com/mmornati/pinakas/internal/state"
)

type fakeBoard struct {
	cards    []board.Card
	user     string
	moves    []string
	assigned []string
	comments []string
}

func (f *fakeBoard) Cards(context.Context) ([]board.Card, error) { return f.cards, nil }
func (f *fakeBoard) CurrentUser(context.Context) (string, error) { return f.user, nil }
func (f *fakeBoard) Move(_ context.Context, c board.Card, status string) error {
	f.moves = append(f.moves, c.Ref()+"->"+status)
	for i := range f.cards {
		if f.cards[i].Number == c.Number && f.cards[i].Repo == c.Repo {
			f.cards[i].Status = status
		}
	}
	return nil
}
func (f *fakeBoard) Assign(_ context.Context, c board.Card, login string) error {
	f.assigned = append(f.assigned, c.Ref()+"="+login)
	for i := range f.cards {
		if f.cards[i].Number == c.Number && f.cards[i].Repo == c.Repo {
			f.cards[i].Assignees = append(f.cards[i].Assignees, login)
		}
	}
	return nil
}
func (f *fakeBoard) Comment(_ context.Context, _ board.Card, body string) error {
	f.comments = append(f.comments, body)
	return nil
}

type harness struct {
	t      *testing.T
	app    *App
	board  *fakeBoard
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	now    time.Time
}

func newHarness(t *testing.T, withConfig bool) *harness {
	t.Helper()
	dir := t.TempDir()
	if withConfig {
		cfg := "board:\n  owner: acme\n  project: 1\n  repo: acme/app\n"
		if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := &harness{
		t:      t,
		board:  &fakeBoard{user: "agent-bot"},
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		now:    time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC),
	}
	h.board.cards = []board.Card{
		{ItemID: "I1", Repo: "acme/app", Number: 1, Title: "Login", Status: "Ready"},
		{ItemID: "I2", Repo: "acme/app", Number: 2, Title: "Logout", Status: "Ready", Assignees: []string{"someone"}},
		{ItemID: "I3", Repo: "acme/app", Number: 3, Title: "Profile", Status: "Done"},
	}
	h.app = &App{
		Stdin:    strings.NewReader(""),
		Stdout:   h.stdout,
		Stderr:   h.stderr,
		Dir:      dir,
		Now:      func() time.Time { return h.now },
		NewBoard: func(config.Config) (board.Board, error) { return h.board, nil },
		NewStore: func(root string) state.Store { return state.Store{Path: filepath.Join(root, "state.json")} },
	}
	return h
}

func (h *harness) run(stdin string, args ...string) int {
	h.stdout.Reset()
	h.stderr.Reset()
	h.app.Stdin = strings.NewReader(stdin)
	return Execute(h.app, args)
}

const prPayload = `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"gh pr create --fill"}}`

func TestFullStoryFlow(t *testing.T) {
	h := newHarness(t, true)

	if code := h.run(prPayload, "check-pr"); code != blockedExitCode || !strings.Contains(h.stderr.String(), "no card is claimed") {
		t.Fatalf("check-pr before claim: code %d, stderr %q", code, h.stderr)
	}
	if code := h.run("", "claim", "1"); code != 0 {
		t.Fatalf("claim: code %d, stderr %q", code, h.stderr)
	}
	if got := strings.Join(h.board.moves, ";"); got != "acme/app#1->In Progress" {
		t.Errorf("moves = %q", got)
	}
	if got := strings.Join(h.board.assigned, ";"); got != "acme/app#1=agent-bot" {
		t.Errorf("assigned = %q", got)
	}
	if code := h.run(prPayload, "check-pr"); code != 0 {
		t.Fatalf("check-pr after claim: code %d, stderr %q", code, h.stderr)
	}
	if code := h.run(`{"hook_event_name":"Stop"}`, "check-stop"); code != blockedExitCode || !strings.Contains(h.stderr.String(), "pinakas actuals 1") {
		t.Fatalf("check-stop before actuals: code %d, stderr %q", code, h.stderr)
	}
	if code := h.run(`{"stop_hook_active":true}`, "check-stop"); code != 0 {
		t.Fatalf("check-stop on second attempt must pass, code %d", code)
	}
	if code := h.run("", "claim", "3"); code == 0 || !strings.Contains(h.stderr.String(), "still claimed") {
		t.Fatalf("second claim with open card: code %d, stderr %q", code, h.stderr)
	}

	h.now = h.now.Add(26 * time.Minute)
	if code := h.run("", "actuals", "--tokens", "120000", "--first-pass", "yes", "--findings", "minor=2"); code != 0 {
		t.Fatalf("actuals: code %d, stderr %q", code, h.stderr)
	}
	if len(h.board.comments) != 1 || !strings.Contains(h.board.comments[0], "| Duration | 26 min |") {
		t.Fatalf("comments = %q", h.board.comments)
	}
	if code := h.run(`{}`, "check-stop"); code != 0 {
		t.Fatalf("check-stop after actuals: code %d, stderr %q", code, h.stderr)
	}
}

func TestClaimGuards(t *testing.T) {
	h := newHarness(t, true)
	if code := h.run("", "claim", "2"); code == 0 || !strings.Contains(h.stderr.String(), "already assigned to someone") {
		t.Errorf("claim assigned card: code %d, stderr %q", code, h.stderr)
	}
	if code := h.run("", "claim", "3"); code == 0 || !strings.Contains(h.stderr.String(), "only ready cards") {
		t.Errorf("claim done card: code %d, stderr %q", code, h.stderr)
	}
	if code := h.run("", "claim", "9"); code == 0 || !strings.Contains(h.stderr.String(), "not found") {
		t.Errorf("claim missing card: code %d, stderr %q", code, h.stderr)
	}
	if code := h.run("", "claim", "3", "--force"); code != 0 {
		t.Errorf("forced claim: code %d, stderr %q", code, h.stderr)
	}
}

func TestCheckPRBlocksWhenBoardDisagrees(t *testing.T) {
	h := newHarness(t, true)
	if code := h.run("", "claim", "1"); code != 0 {
		t.Fatal(h.stderr)
	}
	h.board.cards[0].Status = "Ready" // someone moved it back
	if code := h.run(prPayload, "check-pr"); code != blockedExitCode || !strings.Contains(h.stderr.String(), `"Ready"`) {
		t.Errorf("code %d, stderr %q", code, h.stderr)
	}
}

func TestHooksAreSilentOutsidePinakasRepos(t *testing.T) {
	h := newHarness(t, false)
	for _, args := range [][]string{{"check-pr"}, {"check-stop"}, {"status", "--hook"}} {
		if code := h.run(prPayload, args...); code != 0 || h.stderr.Len() != 0 || h.stdout.Len() != 0 {
			t.Errorf("%v: code %d, stdout %q, stderr %q", args, code, h.stdout, h.stderr)
		}
	}
}

func TestHooksNeverBlockOnBadInput(t *testing.T) {
	h := newHarness(t, true)
	if code := h.run("{not json", "check-pr"); code != 0 || !strings.Contains(h.stderr.String(), "hook skipped") {
		t.Errorf("code %d, stderr %q", code, h.stderr)
	}
}

func TestStatusHookContext(t *testing.T) {
	h := newHarness(t, true)
	if code := h.run("", "status", "--hook"); code != 0 {
		t.Fatal(h.stderr)
	}
	out := h.stdout.String()
	for _, want := range []string{"source of truth", "No card claimed", "- Ready: 2", "acme/app#1 Login", "pinakas claim"} {
		if !strings.Contains(out, want) {
			t.Errorf("hook context missing %q:\n%s", want, out)
		}
	}
}

func TestMoveAndRelease(t *testing.T) {
	h := newHarness(t, true)
	if code := h.run("", "move", "acme/app#1", "review"); code != 0 || h.board.moves[0] != "acme/app#1->In Review" {
		t.Fatalf("move: code %d, moves %v, stderr %q", code, h.board.moves, h.stderr)
	}
	if code := h.run("", "claim", "3", "--force"); code != 0 {
		t.Fatal(h.stderr)
	}
	if code := h.run("", "release", "--to", "ready"); code != 0 || !strings.Contains(h.stdout.String(), "Released acme/app#3") {
		t.Fatalf("release: code %d, stdout %q, stderr %q", code, h.stdout, h.stderr)
	}
	if code := h.run(`{}`, "check-stop"); code != 0 {
		t.Error("released card must not block stop")
	}
}

func TestInit(t *testing.T) {
	h := newHarness(t, false)
	if code := h.run("", "init"); code == 0 {
		t.Error("init without owner/project should fail")
	}
	if code := h.run("", "init", "--owner", "acme", "--project", "4", "--repo", "acme/app"); code != 0 {
		t.Fatalf("init: %q", h.stderr)
	}
	if _, err := config.Load(filepath.Join(h.app.Dir, config.FileName)); err != nil {
		t.Errorf("generated config invalid: %v", err)
	}
	if code := h.run("", "init", "--owner", "acme", "--project", "4"); code == 0 {
		t.Error("init must not overwrite")
	}
}
