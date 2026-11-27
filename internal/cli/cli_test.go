package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmornati/pinakas/internal/board"
	"github.com/mmornati/pinakas/internal/board/boardtest"
	"github.com/mmornati/pinakas/internal/config"
	"github.com/mmornati/pinakas/internal/state"
)

type harness struct {
	t      *testing.T
	app    *App
	board  *boardtest.Fake
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
		board:  &boardtest.Fake{User: "agent-bot"},
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		now:    time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC),
	}
	h.board.CardList = []board.Card{
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
	if got := strings.Join(h.board.Moves, ";"); got != "acme/app#1->In Progress" {
		t.Errorf("moves = %q", got)
	}
	if got := strings.Join(h.board.Assigned, ";"); got != "acme/app#1=agent-bot" {
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
	if len(h.board.Posted) != 1 || !strings.Contains(h.board.Posted[0], "| Duration | 26 min |") {
		t.Fatalf("comments = %q", h.board.Posted)
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
	h.board.CardList[0].Status = "Ready" // someone moved it back
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

func TestStatusText(t *testing.T) {
	h := newHarness(t, true)
	if code := h.run("", "status"); code != 0 {
		t.Fatal(h.stderr)
	}
	for _, line := range strings.Split(h.stdout.String(), "\n") {
		if strings.HasSuffix(line, " ") {
			t.Errorf("trailing space in %q", line)
		}
	}
	if !strings.Contains(h.stdout.String(), "acme/app#2  Logout  someone") {
		t.Errorf("status = %q", h.stdout)
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
	if code := h.run("", "move", "acme/app#1", "review"); code != 0 || h.board.Moves[0] != "acme/app#1->In Review" {
		t.Fatalf("move: code %d, moves %v, stderr %q", code, h.board.Moves, h.stderr)
	}
	if code := h.run("", "claim", "3", "--force"); code != 0 {
		t.Fatal(h.stderr)
	}
	if code := h.run("", "release", "--to", "ready"); code != 0 || !strings.Contains(h.stdout.String(), `Released acme/app#3: unassigned agent-bot, moved to "Ready".`) {
		t.Fatalf("release: code %d, stdout %q, stderr %q", code, h.stdout, h.stderr)
	}
	if len(h.board.CardList[2].Assignees) != 0 || h.board.CardList[2].Status != "Ready" {
		t.Errorf("released card = %+v", h.board.CardList[2])
	}
	if code := h.run("", "claim", "3", "--force"); code != 0 {
		t.Fatal(h.stderr)
	}
	if code := h.run("", "release", "--keep-assignee"); code != 0 || strings.TrimSpace(h.stdout.String()) != "Released acme/app#3." {
		t.Errorf("release --keep-assignee: %q", h.stdout)
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
	if code := h.run("", "init", "--copilot", "--workflow"); code != 0 {
		t.Fatalf("init extras: %q", h.stderr)
	}
	hooksFile, err := os.ReadFile(filepath.Join(h.app.Dir, ".github", "hooks", "pinakas.json"))
	if err != nil || !json.Valid(hooksFile) || !strings.Contains(string(hooksFile), "check-pr --format copilot") {
		t.Errorf("copilot hooks = %s, %v", hooksFile, err)
	}
	if _, err := os.Stat(filepath.Join(h.app.Dir, ".github", "workflows", "pinakas.yml")); err != nil {
		t.Errorf("workflow not written: %v", err)
	}
	if code := h.run("", "init", "--copilot"); code != 0 || !strings.Contains(h.stdout.String(), "Kept the existing .github/hooks/pinakas.json") {
		t.Errorf("init must keep existing files: %q", h.stdout)
	}
}

const copilotPR = `{"timestamp":1704614400000,"cwd":"/tmp","toolName":"bash","toolArgs":"{\"command\":\"gh pr create --fill\"}"}`

func TestCopilotFormat(t *testing.T) {
	h := newHarness(t, true)
	if code := h.run(copilotPR, "check-pr", "--format", "copilot"); code != 0 {
		t.Fatalf("copilot deny must exit 0, got %d (%q)", code, h.stderr)
	}
	var deny map[string]string
	if err := json.Unmarshal(h.stdout.Bytes(), &deny); err != nil || deny["permissionDecision"] != "deny" || !strings.Contains(deny["permissionDecisionReason"], "pinakas claim") {
		t.Fatalf("deny output = %q, %v", h.stdout, err)
	}
	if code := h.run("", "claim", "1"); code != 0 {
		t.Fatal(h.stderr)
	}
	if code := h.run(copilotPR, "check-pr", "--format", "copilot"); code != 0 || h.stdout.Len() != 0 {
		t.Errorf("allowed PR must print nothing: code %d, stdout %q", code, h.stdout)
	}
	if code := h.run(`{"stopReason":"end_turn"}`, "check-stop", "--format", "copilot"); code != 0 {
		t.Fatalf("copilot block must exit 0, got %d", code)
	}
	var stop map[string]string
	if err := json.Unmarshal(h.stdout.Bytes(), &stop); err != nil || stop["decision"] != "block" || !strings.Contains(stop["reason"], "pinakas actuals 1") {
		t.Fatalf("stop output = %q, %v", h.stdout, err)
	}
	if code := h.run("", "status", "--hook", "--format", "copilot"); code != 0 {
		t.Fatal(h.stderr)
	}
	var ctx map[string]string
	if err := json.Unmarshal(h.stdout.Bytes(), &ctx); err != nil || !strings.Contains(ctx["additionalContext"], "Claimed in this working tree") {
		t.Fatalf("context output = %q, %v", h.stdout, err)
	}
	if code := h.run("", "check-pr", "--format", "cursor"); code == 0 {
		t.Error("unknown format should fail")
	}
}

func writeEvent(t *testing.T, payload string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "event.json")
	if err := os.WriteFile(p, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSyncPRAndCheckCI(t *testing.T) {
	h := newHarness(t, true)
	t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(t.TempDir(), "summary.md"))
	opened := writeEvent(t, `{"action":"opened","repository":{"full_name":"acme/app"},"pull_request":{"number":5,"body":"Closes #1","user":{"login":"agent-bot"},"created_at":"2026-11-27T10:00:00Z"}}`)

	if code := h.run("", "sync-pr", "--event", opened); code != 0 || h.board.CardList[0].Status != "In Review" {
		t.Fatalf("sync-pr: code %d, status %q, stderr %q", code, h.board.CardList[0].Status, h.stderr)
	}
	if code := h.run("", "check-ci", "--event", opened); code != 1 || !strings.Contains(h.stderr.String(), "no actuals") {
		t.Fatalf("check-ci without actuals: code %d, stderr %q", code, h.stderr)
	}
	summary, _ := os.ReadFile(os.Getenv("GITHUB_STEP_SUMMARY"))
	if !strings.Contains(string(summary), "pinakas workflow check") {
		t.Errorf("job summary = %q", summary)
	}

	h.board.IssueNotes = map[string][]string{"acme/app#1": {`<!-- pinakas:actuals {"v":1,"repo":"acme/app","number":1} -->`}}
	if code := h.run("", "check-ci", "--event", opened); code != 0 {
		t.Fatalf("check-ci with actuals: code %d, stderr %q", code, h.stderr)
	}

	t.Setenv("GITHUB_EVENT_PATH", "")
	if code := h.run("", "sync-pr"); code == 0 || !strings.Contains(h.stderr.String(), "no event") {
		t.Errorf("missing event: code %d, stderr %q", code, h.stderr)
	}
}
