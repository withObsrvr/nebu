# 0003. Exception: CLI-12 operation lifecycle

- **Status:** Proposed — needs the owner's sign-off on the expiry date before it
  counts as a live exception under CONF-1
- **Date:** 2026-10-20
- **Owner:** Tillman Mosley III
- **Rule excepted:** CLI-12 (operation IDs, `inspect`, `events --follow`,
  `wait --for`, `cancel`)
- **Expiry:** 2027-04-20

## Context

CLI-12 requires that "Long-running operations MUST return an operation ID and
support `inspect`, `events --follow`, `wait --for <state> --timeout <d>` and,
where the engine allows, `cancel`."

`nebu fetch --follow` is long-running, so the rule is triggered. nebu satisfies
none of it.

## Reason for the exception

Satisfying CLI-12 means owning operation identity and state: nebu would have to
mint IDs, persist their lifecycle somewhere durable, and answer queries about
runs it is no longer executing. That is the job-state ownership that
[ARCHITECTURE_DECISIONS.md](../ARCHITECTURE_DECISIONS.md) rules out, and that
rule N-1 of the [engineering profile](../engineering-profile.md) restates: nebu
must not become a service manager, orchestrator, or desired-state reconciler.
flowctl owns operation identity; §13 of the standard assigns it the control-plane
role, and P-2 forbids nebu re-implementing it.

nebu also runs synchronously in the caller's own process. `inspect` and `wait`
presuppose an operation that outlives the invoking shell, which a foreground
pipeline stage does not have.

## Risk

An orchestrator cannot query a running nebu invocation out-of-band. It must
either watch the process it started or read the status stream. An operator who
loses the terminal loses visibility into that run.

Severity: low. nebu's intended callers are a shell, a pipeline, or flowctl —
all of which hold the process handle.

## Compensating controls

- The opt-in `nebu.status.v1` NDJSON stream (`--status-file`, or
  `NEBU_STATUS_FILE`) carries lifecycle state, current and target ledger, ledger
  lag, and event counts. It covers the `inspect` and `events --follow` use cases
  for anything holding the file path. Specified in
  [STATUS_STREAM.md](../STATUS_STREAM.md).
- OSC 7501 program status reports the same lifecycle to an attached terminal or
  a Rex session (CLI-17).
- `--checkpoint` makes an interrupted run resumable, which is the practical need
  behind `cancel` plus `wait`.
- CLI-11 shutdown behaviour is implemented: SIGINT stops new work, writes the
  checkpoint, and exits.
- [PROCESSOR_CONTRACT.md](../PROCESSOR_CONTRACT.md) documents the launch,
  status, cancellation, and exit contract that an orchestrator drives nebu
  through.

## Review at expiry

The question at expiry is not "has nebu implemented CLI-12" — it should not.
It is whether CLI-12 should be recorded as structurally inapplicable to
foreground CLI stages, which would make this a §1 scope carve-out in the
standard (by CONF-3 ADR against the standard itself) rather than a nebu
exception renewed every six months. That is the preferred outcome; renewing is
the fallback.
