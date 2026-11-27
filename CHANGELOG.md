# Changelog

Notable changes to pinakas. Release notes with binaries live on the [releases page](https://github.com/mmornati/pinakas/releases).

## [0.2.1] - 2026-11-27

Fixes found on a real GitHub Project, and prebuilt binaries.

- `check-ci`: an API error no longer aborts the check; it becomes a failed line and the other results are kept.
- Timing: a card claimed after its PR was opened no longer shows a 0 min claim-to-PR step; the comment says why it is left out.
- `release` unassigns the claimer (unless `--keep-assignee`) and says what it did.
- `status`: assignees `[]` instead of `null` in JSON, claim time with its time zone.
- `actuals`: `< 1 min` instead of `0 min`, `duration_sec` in the marker.
- GoReleaser config and a tag-triggered release workflow; `pinakas --version` reports the module version with `go install`.
- Website, documentation and intro film on GitHub Pages.

## [0.2.0] - 2026-11-27

- GitHub Action (`sync-pr`) moves cards when a PR opens, goes back to draft or merges, and posts a timing comment.
- Required check (`check-ci`): the PR closes a card, linked cards have actuals, risky labels have a human approval.
- GitHub Copilot hooks (`pinakas init --copilot`) and `--format copilot` output.
- `pinakas init --workflow` writes the workflow.

## [0.1.0] - 2026-11-27

- `pinakas` CLI on GitHub Projects (v2): `status`, `claim`, `move`, `actuals`, `release`, `init`.
- Claude Code plugin with the coordinator and implementer skills and the SessionStart, PreToolUse and Stop hooks.
- Per-worktree claims stored in the git directory.

[0.2.1]: https://github.com/mmornati/pinakas/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/mmornati/pinakas/releases/tag/v0.2.0
[0.1.0]: https://github.com/mmornati/pinakas/commit/6987e356a6a50d1cfd226a8369d4de4aeb8fc09b
