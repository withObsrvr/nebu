# Architecture decision records

Decisions that change a contract, the architecture, or the security posture of
nebu — or that except a rule of the
[OBSRVR Engineering Standard](../engineering-profile.md) — are recorded here as
ADRs, per DOC-4 of that standard.

ADRs use [MADR](https://adr.github.io/madr/). **An accepted ADR is never edited.**
To change a decision, write a new ADR that supersedes it and update the old one's
status line to point at the replacement.

Exception ADRs additionally carry the CONF-1 fields: the rule ID being excepted,
the risk, the compensating control, an owner, and an expiry date no more than six
months out. An expired exception counts as non-conformance, so they are reviewed
quarterly alongside the profile (CONF-2).

| ADR | Title | Status |
|---|---|---|
| [0001](0001-cli-surface-is-a-contract.md) | The nebu CLI surface is a contract | Accepted |
| [0002](0002-adopt-standard-exit-codes.md) | Adopt the standard exit-code table | Accepted |
| [0003](0003-exception-cli-12-operation-lifecycle.md) | Exception: CLI-12 out-of-band operation addressing | Accepted |
| [0004](0004-exception-build-1-task-runner.md) | Exception: BUILD-1 `just` task runner | Accepted |
| [0005](0005-status-stream-carries-cli-12-lifecycle.md) | The status stream carries the CLI-12 lifecycle vocabulary | Accepted |

Next number: 0006.
