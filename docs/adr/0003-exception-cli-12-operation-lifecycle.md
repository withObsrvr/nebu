# 0003. Exception: CLI-12 out-of-band operation addressing

- **Status:** Accepted (2026-10-20)
- **Date:** 2026-10-20
- **Owner:** Tillman Mosley III
- **Rule partially excepted:** CLI-12, limited to addressing a run from outside
  the process executing it (`inspect <id>`, `cancel <id>`) and to the `queued`
  lifecycle state
- **Expiry:** 2027-04-20
- **Relates to:** [ADR 0005](0005-status-stream-carries-cli-12-lifecycle.md)

## Context

CLI-12 requires that "Long-running operations MUST return an operation ID and
support `inspect`, `events --follow`, `wait --for <state> --timeout <d>` and,
where the engine allows, `cancel`. Lifecycle states are `queued → running →
succeeded | failed | cancelled`. Domain states (`lagging`, `degraded`) need a
written definition. Accepting a job is not completing it."

`nebu fetch --follow` is long-running, so the rule is triggered.

The rule is not one requirement but seven clauses, and nebu's position differs
on each. Treating it as all-or-nothing would except far more than is actually
in conflict, so this ADR excepts only what is genuinely irreducible and routes
the rest to the backlog as ordinary work.

| CLI-12 clause | nebu | Disposition |
|---|---|---|
| "Accepting a job is not completing it" | Work runs in the caller's process; the exit code is the real outcome | **Satisfied** |
| Lifecycle state vocabulary | `lifecycle` field carries the CLI-12 values | **Satisfied** — [ADR 0005](0005-status-stream-carries-cli-12-lifecycle.md) |
| Domain states carry a definition | `blocked` is defined in [STATUS_STREAM.md](../STATUS_STREAM.md) | **Satisfied** — ADR 0005 |
| `events --follow` | `nebu.status.v1` is a followable NDJSON event stream carrying state, ledger progress, lag and counts | **Substantially satisfied**; only the subcommand spelling is missing. Backlog |
| `wait --for <state> --timeout <d>` | A foreground process *is* the wait and the exit code *is* the terminal state; `--timeout` and exit code 6 are missing | **Partial.** Backlog, no architectural obstacle |
| Operation ID | Not emitted. A per-invocation run ID is a label on the current run, not durable job state | **Gap.** Backlog, no architectural obstacle |
| `inspect <id>` / `cancel <id>` out-of-band | Requires a durable registry of runs nebu is not executing | **Excepted by this ADR** |
| `queued` state | nebu has no queue | **Excepted by this ADR** |

## Reason for the exception

The excepted clauses share one requirement: nebu would have to persist run
identity somewhere queryable and answer questions about work it no longer owns.
That is job-state ownership —
[ARCHITECTURE_DECISIONS.md](../ARCHITECTURE_DECISIONS.md) rules it out, rule
N-1 of the [engineering profile](../engineering-profile.md) restates it, §13 of
the standard assigns the control-plane role to flowctl, and P-2 forbids nebu
re-implementing another tool's operations.

`queued` is excepted for a different and simpler reason: nebu runs work in the
caller's own process, so no state exists between accepting a request and
running it. There is nothing to report, rather than something nebu declines to
report.

Nothing else in CLI-12 conflicts with nebu's architecture. The clauses marked
backlog above are missing work, not excepted work, and the profile tracks them.

## Risk

An orchestrator cannot query or cancel a nebu run from a process other than the
one that started it. It must hold the process handle, or read the status
stream.

Severity: low. nebu's callers are a shell, a pipeline, or flowctl — all of
which hold the handle. The residual case is an operator who loses the terminal
mid-run and has not configured `--status-file`.

## Compensating controls

These compensate for the excepted clauses specifically — the out-of-band
half. They are not offered as evidence for the clauses marked satisfied or
backlog above, which stand on their own.

- `--status-file` (or `NEBU_STATUS_FILE`) writes `nebu.status.v1` to a path the
  caller chooses. Anything holding that path can follow a run it did not start,
  which covers the observability half of `inspect` without nebu owning a
  registry. Failure to open or write a configured status file fails the
  command, so an orchestrator can rely on the stream rather than silently
  losing visibility.
- `--checkpoint` makes an interrupted run resumable, which is the practical
  need behind `cancel` followed by a retry.
- CLI-11 shutdown is implemented: SIGINT and SIGTERM stop new work, write the
  checkpoint, and exit. In-process cancellation is therefore available to
  anything holding the handle.
- [PROCESSOR_CONTRACT.md](../PROCESSOR_CONTRACT.md) documents the launch,
  status, cancellation and exit contract an orchestrator drives nebu through.

## Review at expiry

Confirm that the excepted clauses are still the only ones in conflict, and that
the backlog clauses have moved. This exception narrows as that work lands; it
should not grow.

If flowctl gains a run registry that addresses nebu invocations by ID, the
remaining gap is closed at the layer that owns it, and this exception can be
retired rather than renewed. That is the intended path. The exception is
renewed only while that is still outstanding — it is not a standing claim that
CLI-12 does not apply to nebu.
