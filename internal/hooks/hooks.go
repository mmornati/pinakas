// Package hooks holds the guardrail decisions shared by every agent tool.
//
// Each tool (Claude Code, Copilot, Codex) wires a one-line hook that calls
// `pinakas check-pr` or `pinakas check-stop`; the logic lives here.
package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	"github.com/mmornati/pinakas/internal/config"
	"github.com/mmornati/pinakas/internal/state"
)

// Input is the subset of the hook payload pinakas reads. Field names follow
// Claude Code; other tools are mapped onto it by their adapters.
type Input struct {
	HookEventName  string    `json:"hook_event_name"`
	ToolName       string    `json:"tool_name"`
	ToolInput      ToolInput `json:"tool_input"`
	StopHookActive bool      `json:"stop_hook_active"`
	Cwd            string    `json:"cwd"`
}

// ToolInput carries the shell command for Bash-like tools.
type ToolInput struct {
	Command string `json:"command"`
}

// ParseInput decodes a hook payload. An empty payload is valid.
func ParseInput(r io.Reader) (Input, error) {
	var in Input
	raw, err := io.ReadAll(r)
	if err != nil {
		return in, err
	}
	if len(raw) == 0 {
		return in, nil
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return in, fmt.Errorf("decode hook input: %w", err)
	}
	return in, nil
}

// Decision is the outcome of a guardrail check.
type Decision struct {
	Block  bool
	Reason string
}

// Allow is the decision that lets the agent continue.
var Allow = Decision{}

func block(format string, args ...any) Decision {
	return Decision{Block: true, Reason: fmt.Sprintf(format, args...)}
}

var prCreate = regexp.MustCompile(`(^|[\s;&|(])gh\s+pr\s+create($|[\s;&|)])`)

// IsPRCreate reports whether a shell command opens a pull request.
func IsPRCreate(command string) bool {
	return prCreate.MatchString(command)
}

// CardStatus is what the board says about the claimed card, when known.
type CardStatus struct {
	Status    string
	Assignees []string
}

// CheckPR decides whether a command may run. board may be nil when the
// board could not be reached; the local claim is then the only check.
func CheckPR(cfg config.Config, in Input, st state.State, board *CardStatus) Decision {
	if !cfg.Rules.RequireClaimBeforePR || !IsPRCreate(in.ToolInput.Command) {
		return Allow
	}
	c := st.Current
	if c == nil {
		return block("pinakas: no card is claimed in this working tree. Run `pinakas status` to pick an unblocked card, then `pinakas claim <issue>` before opening a pull request.")
	}
	if board != nil {
		want := cfg.Column(config.ColumnInProgress)
		if board.Status != want {
			return block("pinakas: card #%d is in %q on the board, not %q. Run `pinakas move %d in_progress` (or claim it again) before opening the pull request.", c.Number, board.Status, want, c.Number)
		}
		if len(board.Assignees) == 0 {
			return block("pinakas: card #%d has no assignee. Run `pinakas claim %d` before opening the pull request.", c.Number, c.Number)
		}
	}
	return Allow
}

// CheckStop decides whether the agent may end its turn.
func CheckStop(cfg config.Config, in Input, st state.State) Decision {
	// Never block twice in a row: the agent already got the reminder.
	if !cfg.Rules.RequireActualsBeforeStop || in.StopHookActive {
		return Allow
	}
	if st.Current.Open() {
		c := st.Current
		return block("pinakas: card #%d (%s) has no actuals yet. Record them with `pinakas actuals %d --tokens <n> --first-pass=<yes|no>` before stopping, or `pinakas release` if the card is abandoned.", c.Number, c.Title, c.Number)
	}
	return Allow
}
