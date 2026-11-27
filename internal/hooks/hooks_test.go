package hooks

import (
	"strings"
	"testing"
	"time"

	"github.com/mmornati/pinakas/internal/config"
	"github.com/mmornati/pinakas/internal/state"
)

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Board.Owner = "acme"
	cfg.Board.Project = 1
	return cfg
}

func TestIsPRCreate(t *testing.T) {
	cases := map[string]bool{
		"gh pr create --fill":           true,
		"git push && gh pr create -t x": true,
		"cd app; gh  pr   create":       true,
		"(gh pr create)":                true,
		"gh pr view 3":                  false,
		"echo 'nogh pr create'":         false,
		"gh pr createfoo":               false,
		"gh issue create":               false,
	}
	for cmd, want := range cases {
		if got := IsPRCreate(cmd); got != want {
			t.Errorf("IsPRCreate(%q) = %v, want %v", cmd, got, want)
		}
	}
}

func TestParseInput(t *testing.T) {
	in, err := ParseInput(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"gh pr create"}}`))
	if err != nil || in.ToolInput.Command != "gh pr create" || in.ToolName != "Bash" {
		t.Fatalf("ParseInput = %+v, %v", in, err)
	}
	if _, err := ParseInput(strings.NewReader("")); err != nil {
		t.Errorf("empty input should be valid: %v", err)
	}
	if _, err := ParseInput(strings.NewReader("{")); err == nil {
		t.Error("malformed input should fail")
	}
}

func TestCheckPR(t *testing.T) {
	cfg := testConfig()
	pr := Input{ToolInput: ToolInput{Command: "gh pr create --fill"}}
	claimed := state.State{Current: &state.Claim{Number: 7, ClaimedAt: time.Now()}}

	if d := CheckPR(cfg, Input{ToolInput: ToolInput{Command: "go test ./..."}}, state.State{}, nil); d.Block {
		t.Error("non-PR commands must pass")
	}
	if d := CheckPR(cfg, pr, state.State{}, nil); !d.Block || !strings.Contains(d.Reason, "pinakas claim") {
		t.Errorf("PR without claim must be blocked, got %+v", d)
	}
	if d := CheckPR(cfg, pr, claimed, nil); d.Block {
		t.Error("claimed card with unreachable board must pass")
	}
	if d := CheckPR(cfg, pr, claimed, &CardStatus{Status: "Ready", Assignees: []string{"bot"}}); !d.Block || !strings.Contains(d.Reason, "Ready") {
		t.Errorf("card not in progress must be blocked, got %+v", d)
	}
	if d := CheckPR(cfg, pr, claimed, &CardStatus{Status: "In Progress"}); !d.Block || !strings.Contains(d.Reason, "assignee") {
		t.Errorf("unassigned card must be blocked, got %+v", d)
	}
	if d := CheckPR(cfg, pr, claimed, &CardStatus{Status: "In Progress", Assignees: []string{"bot"}}); d.Block {
		t.Errorf("assigned in-progress card must pass, got %+v", d)
	}

	cfg.Rules.RequireClaimBeforePR = false
	if d := CheckPR(cfg, pr, state.State{}, nil); d.Block {
		t.Error("disabled rule must not block")
	}
}

func TestCheckStop(t *testing.T) {
	cfg := testConfig()
	now := time.Now()
	open := state.State{Current: &state.Claim{Number: 7, Title: "Login", ClaimedAt: now}}
	done := state.State{Current: &state.Claim{Number: 7, ClaimedAt: now, ActualsAt: &now}}

	if d := CheckStop(cfg, Input{}, state.State{}); d.Block {
		t.Error("no claim must pass")
	}
	if d := CheckStop(cfg, Input{}, open); !d.Block || !strings.Contains(d.Reason, "pinakas actuals 7") {
		t.Errorf("open claim must be blocked, got %+v", d)
	}
	if d := CheckStop(cfg, Input{StopHookActive: true}, open); d.Block {
		t.Error("second stop attempt must pass to avoid loops")
	}
	if d := CheckStop(cfg, Input{}, done); d.Block {
		t.Error("claim with actuals must pass")
	}
	cfg.Rules.RequireActualsBeforeStop = false
	if d := CheckStop(cfg, Input{}, open); d.Block {
		t.Error("disabled rule must not block")
	}
}
