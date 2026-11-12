# 0005. The status stream carries a run id adopted from the orchestrator

- **Status:** Accepted
- **Date:** 2026-11-12
- **Owner:** Tillman Mosley III
- **Affected interfaces:** `nebu.status.v1` NDJSON status stream
- **Change class (VER-3):** Additive
- **Relates to:** [ADR 0003](0003-exception-cli-12-operation-lifecycle.md), [ADR 0004](0004-status-stream-carries-cli-12-lifecycle.md)

## Context and problem statement

Nothing in nebu's output identified a run.

[ADR 0003](0003-exception-cli-12-operation-lifecycle.md) went through CLI-12
clause by clause and recorded the operation id as the one clause that is a
plain gap rather than an exception: "Not emitted. A per-invocation run ID is a
label on the current run, not durable job state. **Gap.** Backlog, no
architectural obstacle." This ADR closes it.

The gap is not theoretical. A consumer following several status streams at once
cannot tell two concurrent runs of the same processor apart — `app` is the
processor's name, not the run's — and cannot tell that the three stages of
`nebu fetch | token-transfer | postgres-sink` belong to one pipeline. Each
stage emits a well-formed, complete, entirely unrelatable stream. Anything
trying to present a pipeline as one thing has to infer the topology from
process ancestry or timing.

Two other rules of the standard want the same identifier for other reasons:
DATA-11 requires a run id in partition lineage, and OBS-1 wants a correlation
id on every structured log line.

## Decision

Carry a required `run_id` on every status record, and an optional
`component_id` naming the process's position in a pipeline. **Adopt them from
the environment when something is running nebu; mint only when nothing is.**

| Source | Precedence | Set by |
|---|---:|---|
| `FLOWCTL_RUN_ID` | 1 | flowctl, on every child process it starts |
| `NEBU_RUN_ID` | 2 | any other caller composing a pipeline |
| minted UUIDv4 | 3 | nebu itself |

`component_id` resolves the same way through `FLOWCTL_COMPONENT_ID` and
`NEBU_COMPONENT_ID`, and is absent when neither is set.

Adopting before minting is the whole point. flowctl already mints a run id per
pipeline run and exports it to every child it starts, so adopting that value
makes nebu's stream line up with flowctl's record of the same run. Minting
first — or minting unconditionally — would produce a second identity for a run
that already had one, which is worse than no identity at all: two ids that
both look authoritative.

A minted id is a UUIDv4, the same shape flowctl produces, so a consumer never
has to care which case it got.

Identity belongs to the writer, not to the record. It is stamped on write
alongside `app` and `ts`, and a value set by a caller is overwritten, so one
run cannot split itself across several ids.

## Consequences

A consumer can group records by run, and group runs by pipeline. A UI
composing its own pipeline sets `NEBU_RUN_ID` once and gets one correlated
view across processes that have no other way to know they are related.

This is additive. `nebu.status.v1` requires consumers to ignore unknown
fields, so no schema version changes and existing consumers are unaffected.

What this does **not** do is make a run addressable from outside the process
executing it. `inspect <id>` and `cancel <id>` still require a durable
registry of runs nebu is not executing, which remains excepted by
[ADR 0003](0003-exception-cli-12-operation-lifecycle.md) until 2027-04-20. A
run id is a label on a run in flight; it is not job state.

## Alternatives considered

**Mint unconditionally and ignore the environment.** Simpler, and wrong: it
guarantees that a nebu stage inside a flowctl pipeline reports an id that
matches nothing flowctl knows about, which is the exact problem the id is
meant to solve.

**Derive the id from the invocation** — a hash of the command line, range and
network. Appealing because it is reproducible, and useless for the actual
requirement: two concurrent runs of the same command would hash identically,
and telling those apart is the motivating case.

**Carry W3C `traceparent` instead** (OBS-2). It is the right thing for
cross-service tracing and a poor fit for identifying a run: a trace spans
source → processor → sink → API, so the trace id is broader than a run, and
nebu would still need a run-scoped id underneath it. Not mutually exclusive;
`traceparent` propagation remains separate backlog work.
