# Build a pipeline

The whole point of nebu is composition: a ledger source feeds an origin, the
origin's JSONL feeds transforms, and a sink or a standard tool consumes the
result — `ledger source -> origin -> transform(s) -> sink/fan-out`. A user
builds this with ordinary shell pipes, so the proof must show data surviving
every stage, a transform measurably changing the stream, and a sink leaving
something behind on disk.

## Sub-features

- `pipe-origin` turns ledger XDR into schema-stamped JSONL.
- `pipe-transform` filters the stream and demonstrably removes records.
- `pipe-sink` writes the stream to a file and keeps its own stdout silent.
- `pipe-jq` supports the documented `jq` filters on real field paths.
- `pipe-duckdb` supports the documented DuckDB queries over the JSONL.
- `pipe-fanout` splits one stream to two consumers with `tee >(…) >(…)`.
- `pipe-failure` fails the whole pipeline when a stage fails, under
  `set -o pipefail`.

## How to get to it (user POV)

- `nebu fetch --last 100 | token-transfer | jq 'select(.transfer != null)'`
- `token-transfer --start-ledger 60200000 --end-ledger 60200100 | usdc-filter | amount-filter --min 1000000 | json-file-sink --out large-usdc.jsonl`
- `cat ledgers.xdr | token-transfer | jq -c 'select(.transfer != null)'`
- `token-transfer --last 100 | duckdb -c "SELECT COUNT(*) FROM read_json('/dev/stdin') WHERE transfer IS NOT NULL"`
- `token-transfer --last 100 | tee >(usdc-filter > usdc.jsonl) >(json-file-sink --out all.jsonl) > /dev/null`

## Driving it with nebu-verify

Preconditions:

- `nebu-verify up --published` completed with `token-transfer`, `usdc-filter`,
  and `json-file-sink`; `doctor` passes.
- `F="$(nebu-verify fixture)"` — ledgers 60200000..60200004.
- `E="$PWD/.verify-artifacts/$(cat .verify-artifacts/current)"`,
  `mkdir -p "$E/out"`.

- **Origin stage.** Run
  `nebu-verify sh pipe-all -- "token-transfer --quiet < '$F' | json-file-sink --out '$E/out/all.jsonl'"`.
  Exit 0, and `all.jsonl` holds thousands of records for five mainnet ledgers
  (about 4,000 at 60200000..60200004).
- **Transform stage.** Run
  `nebu-verify sh pipe-usdc -- "token-transfer --quiet < '$F' | usdc-filter | json-file-sink --out '$E/out/usdc.jsonl'"`.
  Exit 0.
- **Assert the transform did something.** Run
  `nebu-verify sh pipe-assert -- "jq -se 'length > 0 and all(.[]; ._schema == \"nebu.token_transfer.v1\" and .transfer.assetCode == \"USDC\")' '$E/out/usdc.jsonl' && echo usdc=\$(wc -l < '$E/out/usdc.jsonl') all=\$(wc -l < '$E/out/all.jsonl')"`.
  Stdout is `true` followed by two counts where `usdc` is non-zero and
  strictly smaller than `all` (observed: 110 of 4041). Equal counts mean the
  filter passed everything through — a pass-shaped failure.
- **Sink silence.** Check `pipe-usdc.out` is zero bytes. A sink must not echo
  events to stdout (`docs/PROCESSOR_CONTRACT.md:66`); fan-out is the shell's
  job.
- **Documented `jq` filter.** Run
  `nebu-verify sh pipe-jq -- "token-transfer --quiet < '$F' | jq -c 'select(.transfer != null)' | jq -se 'length > 0 and all(.[]; .transfer.amount != null)'"`.
  Stdout is `true` — the filter selects records *and* the field the README
  example reads is populated.
- **Count with `jq` for the cross-check.** Run
  `nebu-verify sh pipe-jq-count -- "token-transfer --quiet < '$F' | jq -c 'select(.transfer != null)' | wc -l"`.
  Stdout is a non-zero count (observed: 1908).
- **Documented DuckDB query.** Run
  `nebu-verify sh pipe-duckdb -- "token-transfer --quiet < '$F' | duckdb -c \"COPY (SELECT COUNT(*) c FROM read_json('/dev/stdin') WHERE transfer IS NOT NULL) TO '/dev/stdout' (FORMAT CSV, HEADER FALSE)\""`.
  Stdout is a non-zero integer and **equals** the `jq` count from the previous
  step (observed: 1908 from both). Two different engines reading the same
  stream is the independent truth check here; a mismatch means one of them is
  misreading the JSONL.
- **Fan-out.** Run
  `nebu-verify sh pipe-fanout -- "token-transfer --quiet < '$F' | tee >(usdc-filter > '$E/out/fanout-usdc.jsonl') >(json-file-sink --out '$E/out/fanout-all.jsonl') > /dev/null; sleep 1; wc -l '$E/out/fanout-usdc.jsonl' '$E/out/fanout-all.jsonl'"`.
  Both files are non-empty and their counts match the single-path runs above.
- **A failing stage fails the pipeline.** Run
  `nebu-verify sh pipe-failure -- "token-transfer --quiet < '$F' | usdc-filter --no-such-flag | json-file-sink --out '$E/out/never.jsonl'"`.
  The `.exit` file is non-zero and `never.jsonl` is absent or empty.
- **Proof.** `all.jsonl`, `usdc.jsonl`, the two fan-out files,
  `pipe-assert.out` (counts and the `true`), `pipe-duckdb.out`, and
  `pipe-failure.exit`. Name the `nebu --version` and processor versions the
  run installed.

## Gotchas

- **`| head` exits 141 under `pipefail`**, which `nebu-verify sh` sets. Bound
  the input (a smaller fixture range, `--last N`) or use `jq -n 'limit(…)'`
  rather than truncating a live pipe; if you must, append `|| true` and say in
  the proof why the exit code was discarded.
- **DuckDB 2.0.0 reads differently for agents.** With `AI_AGENT` or
  `CLAUDECODE` set and stdout not a terminal it collapses `read_json`'s
  inferred schema to one `json` column, so `WHERE transfer IS NOT NULL`
  becomes a binder error. The helper exports `DUCKDB_AGENT_MODE=0`; a DuckDB
  query run outside the helper may fail for this reason alone.
- **Equal input and output counts are the transform failure mode here**, not
  an error. Always compare filtered against unfiltered.
- **Field paths differ per processor**, and the README's `jq` examples have
  been wrong before — a filter on a field that does not exist selects nothing
  and still exits 0. Read `<processor> --describe-json` for the real paths and
  assert a field inside the selected record, never just that `jq` ran.
- **`tee >(…)` backgrounds its consumers**, so the files can still be short
  when the pipeline's last command returns. The recipe sleeps before counting;
  without that, a fan-out proof can read a truncated file.
- **A sink's output path is inside the run root unless you say otherwise**, and
  the run root is deleted at teardown. Write sink output under
  `.verify-artifacts/<run-id>/out/` or the proof disappears with the
  instance.
- Event counts vary with ledger activity. Assert "non-empty, correctly shaped,
  and strictly fewer after filtering" — never a fixed total, unless the
  fixture range is pinned, in which case record the number you observed so a
  later drift is visible.
