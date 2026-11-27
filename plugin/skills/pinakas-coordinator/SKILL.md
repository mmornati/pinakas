---
name: pinakas-coordinator
description: Coordinate a human + agent Kanban board with pinakas. Use when asked to pick the next story, plan work from the board, dispatch implementers, or report board status.
---

# Coordinating the board with pinakas

The board is the source of truth. Never rely on notes, memory or earlier
messages about which card is where: read it with `pinakas status`.

## Picking work

1. Run `pinakas status` (or `pinakas status --json` to parse it).
2. Choose only cards in the **Ready** column. A card is ready when its
   dependencies are done; if a card lists blockers that are not in Done,
   skip it.
3. Prefer the smallest ready card that unblocks others.
4. Hand one card to one implementer, each in its own git worktree. The
   implementer claims it; do not claim on its behalf.

## While implementers work

- One card per worktree. pinakas refuses a second claim while the first has
  no actuals.
- If a card turns out to be blocked, the implementer runs
  `pinakas release --to ready` and explains why in an issue comment.

## Reviewing and closing

- A human reviews and merges every pull request. Do not merge.
- Pull requests labelled with a risk class from `rules.approval_labels`
  (default `risk:high`) need a human approval before the required check
  passes. Label risky changes; do not remove the label to get green.
- After merge, the GitHub Action moves the card to Done and posts a timing
  comment. Without the Action, run `pinakas move <issue> done`.

## Reporting

Report from the board, not from memory: cards per column, what each
implementer holds, and the actuals recorded on finished cards.

- `pinakas report` measures the done column: agent time, tokens, first-pass
  rate, review findings, lead time and defects within 30 days. Use
  `--since 30d`, `--all`, or `--format markdown|json|csv` as needed.
- When a bug traces back to a delivered story, link it with
  `pinakas defect <story> <bug>` so that the defect rate stays honest.
