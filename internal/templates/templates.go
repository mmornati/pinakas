// Package templates embeds the files `pinakas init` can write into a
// repository.
package templates

import _ "embed"

// CopilotHooks is written to .github/hooks/pinakas.json.
//
//go:embed copilot-hooks.json
var CopilotHooks []byte

// Workflow is written to .github/workflows/pinakas.yml.
//
//go:embed workflow.yml
var Workflow []byte

// CodexHooks is written to .codex/hooks.json.
//
//go:embed codex-hooks.json
var CodexHooks []byte
