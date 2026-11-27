---
name: pinakas-implementer
description: Implement one Kanban card end to end with pinakas. Use when asked to work on, implement or fix a story/issue tracked on the pinakas board.
---

# Implementing a card with pinakas

pinakas hooks enforce this workflow: a pull request is refused unless the
card is claimed and in progress, and you cannot finish while the card has
no actuals. Follow the steps in order.

## 1. Claim

```sh
pinakas status            # confirm the card is in Ready
pinakas claim 42          # assigns you, moves it to In Progress, starts the clock
```

If the claim is refused, read the reason: the card is not ready, someone
else holds it, or this worktree still has an open card.

## 2. Build

- Write the tests first, then the code.
- Keep the change to the card's scope. New work becomes a new issue, not
  part of this pull request.

## 3. Open the pull request

```sh
gh pr create --fill --body "Closes #42"
```

Link the issue with `Closes #42`: the pinakas GitHub Action moves the card
to review when the pull request opens, and the required check fails
without that link. If the repository does not run the Action, move it
yourself: `pinakas move 42 review`.

## 4. Record actuals before stopping

```sh
pinakas actuals 42 --tokens 120000 --first-pass yes --findings major=0,minor=2
```

- `--tokens`: tokens used for this card, if your tool reports them.
- `--first-pass`: `yes` if the first review needed no changes.
- `--findings`: review findings by severity.

Duration is computed from the claim timestamp; do not estimate it. The
required check `pinakas / check` stays red until the actuals are posted.
At merge, the Action adds a timing comment measured from timestamps.

If you abandon the card, run `pinakas release --to ready` and say why in an
issue comment.
