# Machine-readable status stream

Nebu exposes two complementary status mechanisms:

- OSC 7501 is the terminal-native lifecycle signal. Use
  `--program-status=always` to emit it when stderr is not a TTY.
- The opt-in status stream is versioned NDJSON intended for orchestrators and
  other non-terminal consumers.

Enable the structured stream with a dedicated path:

```bash
nebu fetch --status-file ./run.status.ndjson 60200000 60200100 > ledgers.xdr
```

`NEBU_STATUS_FILE` supplies the default. Status never uses stdout, so raw XDR
and processor JSONL remain clean. Diagnostics and OSC 7501 remain on stderr.

## `nebu.status.v1`

Each line is one JSON object:

```json
{"_schema":"nebu.status.v1","state":"working","app":"nebu","ts":"2026-10-11T14:00:00Z","current_ledger":60200050,"target_ledger":60200100,"ledger_lag":50,"message":"Fetching ledgers"}
```

| Field | Type | Required | Meaning |
|---|---|---:|---|
| `_schema` | string | yes | Always `nebu.status.v1` for this contract. |
| `state` | string | yes | `idle`, `working`, `blocked`, `done`, or `error`. |
| `app` | string | yes | Stable emitting application name. |
| `ts` | RFC 3339 timestamp | yes | Time the record was emitted, in UTC. |
| `current_ledger` | uint32 | no | Most recently written ledger. |
| `target_ledger` | uint32 | no | Requested bounded-range end ledger. |
| `ledger_lag` | uint64 | no | `max(target_ledger - current_ledger, 0)`. |
| `events_processed` | uint64 | no | Events consumed successfully by a transform or sink. |
| `message` | string | no | One-line human-readable context. |

For an unbounded fetch, `current_ledger` is present while `target_ledger` and
`ledger_lag` are omitted because there is no fixed target. Consumers must
ignore unknown fields so compatible fields can be added in future versions.

The stream begins with `working` and emits fetch progress records. Its terminal
state is `done` after successful completion, `idle` after cancellation or
interruption, or `error` after failure. Failure to open or write an explicitly
configured status file fails the command; it is therefore safe for an
orchestrator to rely on the configured stream rather than silently losing
observability.

The `nebu` command and processor binaries built with `pkg/processor/cli` both
support `--status-file` and `NEBU_STATUS_FILE`. Origin processors report ledger
progress. Transforms and sinks report `events_processed`. These records use the
same schema and lifecycle semantics as `nebu fetch`.
