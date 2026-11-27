// Package plugin embeds the agent skills shipped in the Claude Code plugin,
// so that `pinakas init --codex` can write the same skills for Codex.
package plugin

import "embed"

// Skills holds skills/<name>/SKILL.md.
//
//go:embed skills
var Skills embed.FS
