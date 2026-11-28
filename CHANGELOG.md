# Changelog

Notable changes to pinakas. Release notes with binaries live on the [releases page](https://github.com/mmornati/pinakas/releases).

## [0.4.0] - 2026-11-28

- GitLab backend: `board.provider: gitlab`, one label per column (`workflow::…` by default), moving to done closes the issue, optional group-wide board; token from `GITLAB_TOKEN`. `pinakas init --provider gitlab`.
- The PR hook also guards `glab mr create`.
- MCP server: `pinakas mcp` serves `board_status`, `claim_card`, `move_card`, `record_actuals`, `release_card`, `story_report` and `link_defect` over stdio (official Go MCP SDK); each tool runs the matching command with the same rules.
- `report`: the median and total agent time read `< 1 min` instead of `0 min` for stories under a minute, like the story rows.

## [0.3.1] - 2026-11-27

Fixes from the end-to-end run of v0.3.0 on a real GitHub Project.

- `move <card> done` ends the claim once its actuals are recorded; without actuals it keeps the claim and says so.
- `status` shows "Last card here … (actuals recorded; nothing claimed)" instead of "Claimed here" for a finished card, like the session hook.
- `claim` says which finished card it stops tracking instead of replacing it silently.
- Durations under a minute read `< 1 min`; the report computes medians and totals from seconds, so two quick stories no longer show a 1 min median.
- `report`: Agent column in the table, Markdown and CSV, and `duration_sec` in the CSV.
- Docs: GitHub's built-in project workflows can auto-close issues moved to Done.

## [0.3.0] - 2026-11-27

- `pinakas report`: per-story metrics and a summary from the actuals, timing and defect markers (agent time, tokens, first-pass rate, findings, lead and review time, defects within 30 days), as a table, JSON, CSV or Markdown; `--since`, `--status`/`--all`, `--measured`.
- `pinakas defect <story> <bug>` links a bug back to the story that introduced it.
- GitHub Action: `command: report` with an `args` input writes the report to the job summary.
- Codex: `--format codex` for the hooks, `pinakas init --codex` writes `.codex/hooks.json` and the skills to `.agents/skills/`.
- Plugin 0.3.0; the coordinator skill covers `report` and `defect`.

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

[0.4.0]: https://github.com/mmornati/pinakas/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/mmornati/pinakas/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/mmornati/pinakas/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/mmornati/pinakas/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/mmornati/pinakas/releases/tag/v0.2.0
[0.1.0]: https://github.com/mmornati/pinakas/commit/6987e356a6a50d1cfd226a8369d4de4aeb8fc09b
