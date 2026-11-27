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
	formatCodex   = "codex"
)

type hookEvent int

const (
	preToolUse hookEvent = iota
	stopEvent
)

func addFormatFlag(cmd *cobra.Command, format *string) {
	cmd.Flags().StringVar(format, "format", formatClaude, "hook output format: claude, copilot or codex")
}

func checkFormat(format string) error {
	switch format {
	case formatClaude, formatCopilot, formatCodex:
		return nil
	}
	return fmt.Errorf("unknown --format %q (use claude, copilot or codex)", format)
}

// verdict turns a decision into the tool's blocking convention.
//
// Claude Code: exit code 2, reason on stderr.
// Copilot: one JSON object on stdout, exit code 0. preToolUse takes
// {"permissionDecision":"deny","permissionDecisionReason":...}; agentStop
// takes {"decision":"block","reason":...}, the reason becoming the next prompt.
// Codex: one JSON object on stdout, exit code 0. PreToolUse takes
// {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",...}};
// Stop takes {"decision":"block","reason":...}. Allowing prints nothing,
// which Codex reads as success.
func verdict(out io.Writer, format string, event hookEvent, d hooks.Decision) error {
	if !d.Block {
		return nil
	}
	var payload any
	switch {
	case format == formatClaude:
		return &exitError{code: blockedExitCode, msg: d.Reason}
	case event == stopEvent:
		payload = map[string]string{"decision": "block", "reason": d.Reason}
	case format == formatCodex:
		payload = map[string]any{"hookSpecificOutput": map[string]string{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": d.Reason,
		}}
	default:
		payload = map[string]string{"permissionDecision": "deny", "permissionDecisionReason": d.Reason}
	}
	return encode(out, payload)
}

// encode writes one JSON object on one line, without HTML escaping.
func encode(out io.Writer, v any) error {
	return newEncoder(out).Encode(v)
}

func newEncoder(out io.Writer) *json.Encoder {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return enc
}

// writeContext prints SessionStart context: plain text for Claude Code,
// {"additionalContext": ...} for Copilot, hookSpecificOutput for Codex.
func writeContext(out io.Writer, format, text string) error {
	if text == "" {
		return nil
	}
	switch format {
	case formatCopilot:
		return encode(out, map[string]string{"additionalContext": text})
	case formatCodex:
		return encode(out, map[string]any{"hookSpecificOutput": map[string]string{
			"hookEventName":     "SessionStart",
			"additionalContext": text,
		}})
	}
	_, err := io.WriteString(out, text)
	return err
}
