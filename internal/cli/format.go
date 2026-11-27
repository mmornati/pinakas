package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mmornati/pinakas/internal/hooks"
)

// Hook output formats. Each agent tool reads hook results differently;
// the decisions are the same.
const (
	formatClaude  = "claude"
	formatCopilot = "copilot"
)

type hookEvent int

const (
	preToolUse hookEvent = iota
	stopEvent
)

func addFormatFlag(cmd *cobra.Command, format *string) {
	cmd.Flags().StringVar(format, "format", formatClaude, "hook output format: claude or copilot")
}

func checkFormat(format string) error {
	if format != formatClaude && format != formatCopilot {
		return fmt.Errorf("unknown --format %q (use claude or copilot)", format)
	}
	return nil
}

// verdict turns a decision into the tool's blocking convention.
//
// Claude Code: exit code 2, reason on stderr.
// Copilot: one JSON object on stdout, exit code 0. preToolUse takes
// {"permissionDecision":"deny","permissionDecisionReason":...}; agentStop
// takes {"decision":"block","reason":...}, the reason becoming the next prompt.
func verdict(out io.Writer, format string, event hookEvent, d hooks.Decision) error {
	if !d.Block {
		return nil
	}
	if format != formatCopilot {
		return &exitError{code: blockedExitCode, msg: d.Reason}
	}
	var payload map[string]string
	switch event {
	case preToolUse:
		payload = map[string]string{"permissionDecision": "deny", "permissionDecisionReason": d.Reason}
	case stopEvent:
		payload = map[string]string{"decision": "block", "reason": d.Reason}
	}
	return encode(out, payload)
}

// encode writes one JSON object on one line, without HTML escaping.
func encode(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// writeContext prints SessionStart context: plain text for Claude Code,
// {"additionalContext": ...} for Copilot.
func writeContext(out io.Writer, format, text string) error {
	if text == "" {
		return nil
	}
	if format == formatCopilot {
		return encode(out, map[string]string{"additionalContext": text})
	}
	_, err := io.WriteString(out, text)
	return err
}
