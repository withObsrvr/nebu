# 0005. The status stream carries the CLI-12 lifecycle vocabulary

- **Status:** Accepted
- **Date:** 2026-10-20
- **Owner:** Tillman Mosley III
- **Affected interfaces:** `nebu.status.v1` NDJSON status stream
- **Change class (VER-3):** Additive
- **Relates to:** [ADR 0003](0003-exception-cli-12-operation-lifecycle.md)

## Context and problem statement

nebu reports operation state in two places, and until now in only one
vocabulary.

The `nebu.status.v1` stream and the OSC 7501 escape sequence both carry a
`state` drawn from the Program Status Protocol: `idle`, `working`, `blocked`,
`done`, `error`. CLI-12 of the OBSRVR Engineering Standard defines a different
set for operation lifecycle — `queued → running → succeeded | failed |
cancelled` — and flowctl and the `obsrvr` CLI use that one.

So an orchestrator reading state across OBSRVR tools needs two state machines,
and the overlap between them is not obvious. `idle` is the trap: it reads as
"waiting for work" but CLI-17 specifies it for interruption, so in nebu it
means *cancelled*. A consumer mapping the vocabularies by name would get that
backwards.

Renaming is not available. CLI-17 requires the 7501 tokens to be emitted
verbatim in the escape sequence, because that is what a terminal or a Rex
session parses, and it requires them to come from one shared package pinning
the protocol revision. The tokens are the wire format.

## Decision

Carry both vocabularies in the status record. Add a required `lifecycle` field
holding the CLI-12 value, keeping `state` for the 7501 token:

| `state` | `lifecycle` |
|---|---|
| `working` | `running` |
| `blocked` | `running` |
| `done` | `succeeded` |
| `error` | `failed` |
| `idle` | `cancelled` |

Three supporting decisions:

1. **`lifecycle` is derived, not supplied.** `status.Writer.Write` computes it
   from `state` and overwrites anything the caller set, so the two fields
   cannot disagree. A record with an unmappable state is rejected rather than
   written without a lifecycle.
2. **`blocked` is retained and defined.** Nothing in nebu emits it today, and
   deleting it was considered. It is kept because CLI-17 specifies it for a
   tool waiting on a confirmation, an answer, or a login, and nebu will need it
   when the `--yes` confirmation CLI-8 requires for destructive operations
   lands — at which point having it already in the schema avoids a schema
   change. CLI-12 requires domain states to carry a written definition, so it
   now has one in [STATUS_STREAM.md](../STATUS_STREAM.md): running but unable
   to progress until it receives input. It is not terminal, hence `running`.
3. **`queued` is never emitted.** nebu runs work in the caller's process and
   has no queue, so no state exists between accepting a request and running it.
   The constant is declared for vocabulary completeness.

## Consequences

**Good:** one state machine reads every OBSRVR tool. The `idle`-means-cancelled
trap is removed for anyone reading `lifecycle`. nebu stays conformant to the
Program Status Protocol on the wire. And this closes the vocabulary clause of
CLI-12 without nebu taking on any operation state — the part of the rule that
ADR 0003 excepts is unaffected either way.

**Bad:** every record is slightly larger, and the same information appears
twice, which is a mild smell. The alternative — one vocabulary — is unavailable
while CLI-17 requires the 7501 tokens on the wire.

**Neutral:** the change is additive. `nebu.status.v1` already instructs
consumers to ignore unknown fields, so no existing consumer breaks and the
schema version does not move. A minor release carries it.

## Rollback

Remove the field and `lifecycleFor`. Consumers that adopted `lifecycle` would
break, so after one release this is forward-only; within the release that
introduces it, rollback is safe because nothing can yet depend on it.

## Tests

`pkg/status/status_test.go` covers the mapping for all five states, that the
7501 token is preserved on the wire alongside it, that a caller-supplied
lifecycle is overwritten, that the field is always present, and that an
unmappable state is rejected without being written.
