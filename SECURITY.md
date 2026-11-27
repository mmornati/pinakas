# Security policy

pinakas handles tokens that can read and write GitHub Projects and issues, and runs inside agent hooks and CI. We take security reports seriously.

## Supported versions

| Version | Supported |
| --- | --- |
| Latest release (0.x) | ✅ |
| Older releases | ❌, please upgrade |

## Reporting a vulnerability

**Do not open a public issue.** Report it privately through [GitHub private vulnerability reporting](https://github.com/mmornati/pinakas/security/advisories/new).

Please include:

- the pinakas version and how it was installed,
- what an attacker can do and the steps to reproduce,
- any logs or proof of concept (redact tokens).

You should get a first answer within a week. Once a fix is released, we publish a GitHub security advisory and credit you unless you prefer otherwise.

## Scope notes

- Tokens: pinakas reads `GH_TOKEN` / the `gh` CLI credentials and the `PINAKAS_TOKEN` secret in CI. It never writes them to disk or to comments.
- Guardrails fail open by design: a hook that cannot reach the board does not block work. That is intended behaviour, not a bypass.
- The workflow is not meant for pull requests from forks, which receive no secrets.
