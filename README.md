# pinakas

> Keep AI coding agents on the board.

**pinakas** (Greek πίνακας, "board") makes a Kanban board the single source of truth for human + agent teams. Agents claim cards, move them and record what each story really cost. Hooks and CI checks make sure they can't skip a step.

## Why

Running coding agents from a Kanban board works: stories ship 3 to 6 times faster than estimated. But left alone, agents drift. They forget to move cards, open PRs for stories nobody claimed and finish without logging actuals. A skill can describe the workflow; it can't enforce it. pinakas does.

## How it works

| Layer | What it does | Status |
| --- | --- | --- |
| `pinakas` CLI | Gives agents write access to the board: `claim`, `move`, `actuals`, `status`, `release` | v0.1 |
| Agent hooks | `SessionStart` injects the live board, `PreToolUse` blocks `gh pr create` for an unclaimed card, `Stop` blocks finishing without actuals | v0.1 |
| GitHub Action | Moves the card when a PR opens or merges, computes duration from timestamps | v0.2 |
| Required check | PR linked to an issue, actuals filled, human approval by risk class | v0.2 |

All logic lives in the CLI. Each agent tool gets a one-line hook that calls it, so there is one implementation to maintain and a few small wiring files.

## Quick start

Requirements: Go 1.27+, the [GitHub CLI](https://cli.github.com) signed in with the `project` scope, and a GitHub Project (v2) with a single-select `Status` field.

```sh
# 1. Install the CLI
go install github.com/mmornati/pinakas/cmd/pinakas@latest
gh auth refresh -s project

# 2. Configure your repository
pinakas init --owner <user-or-org> --project <number> --repo <owner/name>
pinakas status
```

```text
# 3. Install the Claude Code plugin (skills + hooks), inside Claude Code
/plugin marketplace add mmornati/pinakas
/plugin install pinakas@pinakas
```

Then ask your agent to pick the next ready story.

## Commands

| Command | What it does |
| --- | --- |
| `pinakas status [--json\|--mine\|--hook]` | Board grouped by column, plus the card claimed in this working tree |
| `pinakas claim <issue>` | Assign a **Ready** card to you, move it to **In Progress**, start the clock |
| `pinakas move <issue> <column>` | Move a card (`ready`, `in_progress`, `review`, `done` or a column name) |
| `pinakas actuals [issue] --tokens N --first-pass yes --findings major=1,minor=3` | Post the story's actuals as an issue comment |
| `pinakas release [--to ready]` | Drop the claim without actuals |
| `pinakas check-pr` / `pinakas check-stop` | Hook entry points: exit 2 with a reason to block |

Issues can be written `42`, `#42` or `owner/name#42`. The claim is stored inside the git directory, so each worktree (and each parallel agent) tracks its own card and nothing gets committed.

## Configuration

`.pinakas.yml` at the repository root:

```yaml
board:
  provider: github
  owner: acme
  project: 3
  repo: acme/app
  status_field: Status
  columns:
    ready: Ready
    in_progress: In Progress
    review: In Review
    done: Done
rules:
  require_claim_before_pr: true
  require_actuals_before_stop: true
```

## Measuring

Every story records the same fields so that experiments can be compared: timestamped duration (from claim to actuals), tokens, first-pass success and review findings by severity. Defects found within 30 days and `pinakas report` come in v0.3. Each actuals comment carries a machine-readable marker that the report and CI checks parse back.

## Guardrails are fail-open

A broken guardrail must never stop all work. If the hook input is malformed or the board can't be reached, pinakas prints a warning and checks only what it can verify locally. Outside a repository with `.pinakas.yml`, the hooks do nothing.

## Supported tools

| Tool | Status |
| --- | --- |
| Claude Code | v0.1 |
| GitHub Copilot | v0.2 |
| Codex | v0.3 |

Boards: GitHub Projects (v0.1), GitLab (v0.4).

## Roadmap

- **v0.1** CLI on GitHub Projects, Claude Code plugin with two skills and three hooks.
- **v0.2** GitHub Action for card moves and durations, required CI check, Copilot wiring.
- **v0.3** Per-story metrics schema, `pinakas report`, Codex adapter.
- **v0.4** GitLab backend, MCP server.
- **v1.0** Stable config schema, complete docs, example project, signed releases.

## Background

pinakas grew out of running real projects with a human + agent Kanban workflow; see [blog.mornati.net](https://blog.mornati.net).

## Development

```sh
go test -race ./...
golangci-lint run
```

## License

[Apache 2.0](LICENSE)
