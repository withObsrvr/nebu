# Status stream

Orchestrators and agents need to know what a nebu run is doing without parsing
human stderr. `--status-file` writes versioned `nebu.status.v1` NDJSON to a
dedicated path, and every stage of a pipeline stamps the same `run_id` when the
caller supplies one — which is what makes two concurrent runs, and the stages
of one pipeline, tellable apart (`docs/STATUS_STREAM.md`).

## Sub-features

- `status-file` writes NDJSON to the path `--status-file` names, never to
  stdout.
- `status-lifecycle` begins at `working` and reaches `done` on success.
- `status-progress` reports `current_ledger`, `target_ledger`, and
  `ledger_lag` for a bounded range.
- `status-run-id` adopts `NEBU_RUN_ID` instead of minting one.
- `status-component-id` stamps `NEBU_COMPONENT_ID` so stages are
  distinguishable.
- `status-correlate` lets one query join both stages of a pipeline by
  `run_id`.
- `status-env` takes its default path from `NEBU_STATUS_FILE`.

## How to get to it (user POV)

- `nebu fetch --status-file ./run.status.ndjson 60200000 60200100 > ledgers.xdr`
- `export NEBU_STATUS_FILE=./run.status.ndjson` then any nebu command.
- `NEBU_RUN_ID=<uuid> NEBU_COMPONENT_ID=fetch nebu fetch … | NEBU_COMPONENT_ID=transfers token-transfer --status-file …`
- `--program-status=always` for the terminal-native OSC 7501 signal (a
  separate mechanism; not covered here).

## Driving it with nebu-verify

Preconditions:

- `nebu-verify up --published` completed; `doctor` passes.
- `E="$PWD/.verify-artifacts/$(cat .verify-artifacts/current)"`,
  `mkdir -p "$E/out"`.
- A fixed run id to assert against:
  `RID=11111111-2222-4333-8444-555555555555`.

- **Run a two-stage pipeline with both status files.** Run
  `nebu-verify sh status-stream -- "export NEBU_RUN_ID=11111111-2222-4333-8444-555555555555; NEBU_COMPONENT_ID=fetch nebu --status-file '$E/out/s-fetch.ndjson' fetch --quiet 60200000 60200001 2>/dev/null | NEBU_COMPONENT_ID=transfers token-transfer --quiet --status-file '$E/out/s-tt.ndjson' - >/dev/null"`.
  Exit 0 and both NDJSON files exist.
- **Assert the schema and the adopted run id.** Run
  `nebu-verify sh status-schema -- "jq -se 'length > 0 and all(.[]; ._schema == \"nebu.status.v1\" and .run_id == \"11111111-2222-4333-8444-555555555555\" and .app != null and .ts != null)' '$E/out/s-fetch.ndjson'"`.
  Stdout is `true` — the run id was *adopted*, not minted, so a minted UUID
  here means `NEBU_RUN_ID` was ignored.
- **Assert the lifecycle reached a terminal state.** Run
  `nebu-verify sh status-lifecycle -- "jq -r '.state' '$E/out/s-fetch.ndjson' | tail -1"`.
  Stdout is `done`. A stream whose last state is `working` means the run never
  finished — exactly the hang this check exists to catch.
- **Assert bounded progress fields.** Run
  `nebu-verify sh status-progress -- "jq -se 'any(.[]; .target_ledger == 60200001 and .current_ledger != null and .ledger_lag != null)' '$E/out/s-fetch.ndjson'"`.
  Stdout is `true`.
- **Assert both stages are distinguishable.** Run
  `nebu-verify sh status-components -- "jq -r '.component_id' '$E/out/'s-*.ndjson | sort -u"`.
  Stdout is exactly `fetch` and `transfers`.
- **Correlate the pipeline with one query.** Run
  `nebu-verify sh status-correlate -- "duckdb -c \"COPY (SELECT COUNT(DISTINCT component_id) c FROM read_json('$E/out/s-*.ndjson', union_by_name = true) WHERE run_id = '11111111-2222-4333-8444-555555555555') TO '/dev/stdout' (FORMAT CSV, HEADER FALSE)\""`.
  Stdout is `2` (observed with published nebu v0.7.11 and the current
  published `token-transfer`).
- **Status never reaches stdout.** Check `status-stream.out` is zero bytes and
  that neither NDJSON file's content appears in it — raw XDR and processor
  JSONL stay clean.
- **Proof.** `s-fetch.ndjson`, `s-tt.ndjson`, and the `status-*` step outputs.
  Record the nebu and processor versions: whether the second stage stamps
  `run_id` at all depends on which nebu the processor was built against.

## Gotchas

- **A processor only emits `run_id` when it was built against nebu v0.7.1 or
  later.** `scripts/docs-smoke.sh` deliberately asserts one stage only,
  because the pinned registry binaries `just build-processors` installs are
  older and write `null`. The *published* processors this skill installs do
  stamp it (observed: both stages, run id adopted), so a two-stage
  correlation result from here does **not** transfer to a `bin/`-based run,
  or vice versa. State which binaries you drove.
- **`FLOWCTL_RUN_ID` outranks `NEBU_RUN_ID`** (`docs/STATUS_STREAM.md`, "Run
  identity"). If it happens to be set in the environment, your fixed run id is
  silently ignored and the correlation query returns zero rows. The helper's
  environment does not set it; check if an assertion on the run id fails.
- **An unbounded fetch omits `target_ledger` and `ledger_lag` by design**, so
  the progress assertion only applies to a bounded range. Asserting them on
  `--follow` output fails for the wrong reason.
- **Failing to open or write a configured status file fails the command.**
  That is deliberate, so an orchestrator can rely on the stream — but it also
  means a status path inside a directory you did not create takes the whole
  run down. `mkdir -p "$E/out"` first.
- **The terminal state is not always `done`.** It is `idle` after cancellation
  or interruption and `error` after failure. Assert the state you expect for
  the scenario, not `done` unconditionally.
- Status records are NDJSON, one object per line, and consumers must ignore
  unknown fields. Do not assert an exact object shape or a field count — new
  fields are a compatible change.
