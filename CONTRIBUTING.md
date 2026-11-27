# Contributing to pinakas

Thanks for helping keep AI coding agents on the board. This guide covers how to report problems, propose changes and get a pull request merged.

## Ways to help

- **Report a bug** with the [bug report form](https://github.com/mmornati/pinakas/issues/new?template=bug_report.yml). Include the pinakas version (`pinakas --version`), your agent tool and what the board looked like.
- **Propose a feature** with the [feature request form](https://github.com/mmornati/pinakas/issues/new?template=feature_request.yml). Say which step of the workflow it helps and how you would measure it.
- **Add an adapter** for another agent tool or board. Open an issue first so we can agree on where it plugs in.
- **Improve the docs or the website**, under `site/`.

## Development setup

You need the Go version in [`go.mod`](go.mod) (pinakas tracks the latest stable Go) and [golangci-lint](https://golangci-lint.run) v2.

```sh
git clone https://github.com/mmornati/pinakas
cd pinakas
go build ./cmd/pinakas
go test -race ./...
golangci-lint run
```

Tests run against an in-memory board (`internal/board/boardtest`), so they need no token and no network.

### Layout

| Path | What lives there |
| --- | --- |
| `cmd/pinakas` | The CLI entry point |
| `internal/cli` | Commands, flags and output formats |
| `internal/board` | Board interface; `github/` talks to GitHub Projects (v2) |
| `internal/hooks` | Hook input parsing and the PR / stop guardrails |
| `internal/pr` | GitHub Action logic: card sync, timing, required check |
| `internal/actuals`, `internal/state` | Actuals comments and the per-worktree claim |
| `plugin/` | Claude Code plugin: skills and hooks |
| `templates/`, `internal/templates/` | Files written by `pinakas init` |
| `site/` | Website and documentation, published to GitHub Pages |

## Design rules

These keep pinakas small and trustworthy. Pull requests that break them are hard to merge.

1. **All logic lives in the CLI.** Agent wiring files only call `pinakas` commands.
2. **Guardrails fail open.** A hook that can't reach the board warns and checks only what it can verify locally. It never blocks all work.
3. **The board is the source of truth.** Never cache card state in files that get committed.
4. **Measure from timestamps.** Durations come from recorded times, not from what an agent reports.
5. **Latest stable dependencies.** Keep Go and modules up to date.

## Pull requests

1. Create a branch from `main`.
2. Write tests first, then the change. Keep the PR to one topic.
3. Run `go test -race ./...` and `golangci-lint run`.
4. Update the README and `site/docs/` when behaviour or flags change.
5. Open the PR with the template filled in, and link the issue (`Closes #42`).

CI must be green before review. A maintainer reviews and merges every pull request.

### Commit messages

Write the subject in the imperative (`Add GitLab board backend`), under about 70 characters, and explain the why in the body when it isn't obvious.

## Releases

Maintainers create a GitHub release with a `vX.Y.Z` tag. The `release` workflow runs GoReleaser and attaches binaries for Linux, macOS and Windows.

## Code of Conduct

Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

By contributing, you agree that your contributions are licensed under the [Apache License 2.0](LICENSE).
