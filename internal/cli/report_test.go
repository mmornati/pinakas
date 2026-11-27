package cli

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmornati/pinakas/internal/actuals"
	"github.com/mmornati/pinakas/internal/metrics"
)

// codexPR is a Codex PreToolUse payload: same field names as Claude Code.
const codexPR = `{"session_id":"s","cwd":"/tmp","hook_event_name":"PreToolUse","model":"gpt","turn_id":"t","tool_name":"Bash","tool_use_id":"u","tool_input":{"command":"gh pr create --fill"}}`

func TestCodexFormat(t *testing.T) {
	h := newHarness(t, true)
	if code := h.run(codexPR, "check-pr", "--format", "codex"); code != 0 {
		t.Fatalf("codex deny must exit 0, got %d (%q)", code, h.stderr)
	}
	var deny struct {
		Out map[string]string `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &deny); err != nil || deny.Out["hookEventName"] != "PreToolUse" ||
		deny.Out["permissionDecision"] != "deny" || !strings.Contains(deny.Out["permissionDecisionReason"], "pinakas claim") {
		t.Fatalf("deny output = %q, %v", h.stdout, err)
	}
	if code := h.run("", "claim", "1"); code != 0 {
		t.Fatal(h.stderr)
	}
	if code := h.run(codexPR, "check-pr", "--format", "codex"); code != 0 || h.stdout.Len() != 0 {
		t.Errorf("allowed PR must print nothing: code %d, stdout %q", code, h.stdout)
	}
	stopPayload := `{"hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"done"}`
	if code := h.run(stopPayload, "check-stop", "--format", "codex"); code != 0 {
		t.Fatalf("codex block must exit 0, got %d", code)
	}
	var stop map[string]string
	if err := json.Unmarshal(h.stdout.Bytes(), &stop); err != nil || stop["decision"] != "block" || !strings.Contains(stop["reason"], "pinakas actuals 1") {
		t.Fatalf("stop output = %q, %v", h.stdout, err)
	}
	if code := h.run(`{"hook_event_name":"Stop","stop_hook_active":true}`, "check-stop", "--format", "codex"); code != 0 || h.stdout.Len() != 0 {
		t.Errorf("stop_hook_active must allow silently: code %d, stdout %q", code, h.stdout)
	}
	if code := h.run("", "status", "--hook", "--format", "codex"); code != 0 {
		t.Fatal(h.stderr)
	}
	var ctx struct {
		Out map[string]string `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &ctx); err != nil || ctx.Out["hookEventName"] != "SessionStart" ||
		!strings.Contains(ctx.Out["additionalContext"], "Claimed in this working tree") {
		t.Fatalf("context output = %q, %v", h.stdout, err)
	}
}

func TestInitCodex(t *testing.T) {
	h := newHarness(t, true)
	if code := h.run("", "init", "--codex"); code != 0 {
		t.Fatalf("init --codex: %q", h.stderr)
	}
	hooksFile, err := os.ReadFile(filepath.Join(h.app.Dir, ".codex", "hooks.json"))
	if err != nil || !json.Valid(hooksFile) || !strings.Contains(string(hooksFile), "check-stop --format codex") {
		t.Errorf("codex hooks = %s, %v", hooksFile, err)
	}
	for _, name := range []string{"pinakas-coordinator", "pinakas-implementer"} {
		skill, err := os.ReadFile(filepath.Join(h.app.Dir, ".agents", "skills", name, "SKILL.md"))
		if err != nil || !strings.Contains(string(skill), "name: "+name) {
			t.Errorf("skill %s = %v", name, err)
		}
	}
	if !strings.Contains(h.stdout.String(), "/hooks") {
		t.Errorf("init should explain hook trust: %q", h.stdout)
	}
}

func TestDefect(t *testing.T) {
	h := newHarness(t, true)
	if code := h.run("", "defect", "3", "acme/api#9", "--at", "2026-11-20", "--note", "null session"); code != 0 {
		t.Fatalf("defect: %q", h.stderr)
	}
	if !strings.Contains(h.stdout.String(), "Linked acme/api#9 to acme/app#3") {
		t.Errorf("stdout = %q", h.stdout)
	}
	d, ok := metrics.ParseDefect(h.board.IssueNotes["acme/app#3"][0])
	if !ok || d.DefectRepo != "acme/api" || d.DefectNumber != 9 || d.FoundAt.Local().Format(time.DateOnly) != "2026-11-20" || d.Note != "null session" {
		t.Errorf("defect = %+v, %v", d, ok)
	}
	if code := h.run("", "defect", "3", "#3"); code == 0 {
		t.Error("a story cannot be its own defect")
	}
	if code := h.run("", "defect", "3", "4", "--at", "yesterday"); code == 0 {
		t.Error("bad --at should fail")
	}
}

func TestReport(t *testing.T) {
	h := newHarness(t, true)
	claimed := h.now.Add(-50 * 24 * time.Hour)
	tokens, pass := 1200, true
	a := actuals.New("acme/app", 3, claimed, claimed.Add(45*time.Minute))
	a.Tokens, a.FirstPass, a.Findings = &tokens, &pass, map[string]int{"minor": 2}
	body, err := a.Comment()
	if err != nil {
		t.Fatal(err)
	}
	h.board.IssueNotes = map[string][]string{"acme/app#3": {body}}
	if code := h.run("", "defect", "3", "12", "--at", claimed.Add(5*24*time.Hour).Format(time.RFC3339)); code != 0 {
		t.Fatal(h.stderr)
	}

	if code := h.run("", "report"); code != 0 {
		t.Fatalf("report: %q", h.stderr)
	}
	out := h.stdout.String()
	for _, want := range []string{"acme/app#3", "Profile", "45 min", "1200", "minor=2", "Stories: 1 (1 with actuals)", "First-pass rate: 100%", "Stories with a defect: 100% of 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("table misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Login") {
		t.Errorf("default report must only show done cards:\n%s", out)
	}

	if code := h.run("", "report", "--format", "json", "--all"); code != 0 {
		t.Fatal(h.stderr)
	}
	var js struct {
		Summary metrics.Summary `json:"summary"`
		Stories []metrics.Story `json:"stories"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &js); err != nil || len(js.Stories) != 3 || js.Summary.Defects30 != 1 {
		t.Errorf("json = %q, %v", h.stdout, err)
	}

	if code := h.run("", "report", "--format", "csv", "--status", "done,ready"); code != 0 {
		t.Fatal(h.stderr)
	}
	recs, err := csv.NewReader(strings.NewReader(h.stdout.String())).ReadAll()
	if err != nil || len(recs) != 4 || recs[0][0] != "repo" {
		t.Errorf("csv = %q, %v", h.stdout, err)
	}

	if code := h.run("", "report", "--format", "markdown", "--measured"); code != 0 {
		t.Fatal(h.stderr)
	}
	if md := h.stdout.String(); !strings.Contains(md, "## pinakas report") || !strings.Contains(md, "| acme/app#3 | Profile |") {
		t.Errorf("markdown = %s", md)
	}

	if code := h.run("", "report", "--since", "7d"); code != 0 || !strings.Contains(h.stdout.String(), "No stories match") {
		t.Errorf("since 7d: code %d, %q", code, h.stdout)
	}
	if code := h.run("", "report", "--format", "xml"); code == 0 {
		t.Error("unknown format should fail")
	}
}

func TestMinutesText(t *testing.T) {
	for in, want := range map[float64]string{0: "0 min", 0.4: "< 1 min", 45: "45 min", 125: "2 h 05", 3000: "2 d 2 h"} {
		if got := minutesText(in); got != want {
			t.Errorf("minutesText(%v) = %q, want %q", in, got, want)
		}
	}
}
