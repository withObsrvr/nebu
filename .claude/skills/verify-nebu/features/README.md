# nebu verification map

This directory is the maintained source for verifying nebu's user-facing
behavior. Read this index before driving the CLI, then use the matching
feature file as the recipe. Keep it honest as the CLI changes:
`/pstack:maintain-verification-skill`.

## Baseline preconditions

- Install the shipped artifact into an empty `GOBIN` with
  `.claude/skills/verify-nebu/nebu-verify up --published`. A local build is
  not what users get; use `--local` only for an uncommitted change and say so
  in the proof.
- Run `nebu-verify doctor` and require every check to pass before driving.
- Every step runs with cwd set to the run root, never the repo checkout:
  `nebu install` prefers local source whenever it can see it
  (`cmd/nebu/install.go:99-104`).
- Fetch ledgers once with `nebu-verify fixture` and drive from the file. The
  default RPC endpoint 429s on the third `getLedgers` call in a burst.
- Never drive a `nebu` or processor binary that this run did not install, and
  never kill one by process name.

## Driving conventions

- Start from the baseline unless a feature's preconditions say otherwise.
- `nebu-verify run <step> -- <cmd>` for a single command,
  `nebu-verify sh <step> -- '<pipeline>'` for pipes, `nebu-verify bg` for
  `--follow`.
- Treat every command as literal. Keep quoted flags, ledger numbers, and
  `jq` filters unchanged — a paraphrased quickstart proves nothing about the
  printed one.
- Prefer stable handles: subcommand names, long flags listed by `--help` on
  the installed binary, `_schema` ids, field paths from `--describe-json`,
  and the exit codes in `docs/EXIT_CODES.md`.
- Use the archive fixture for data; use RPC only when the claim is about RPC.
- `nebu-verify sh` runs with `-o pipefail`, so `| head` exits 141. Bound the
  data instead of truncating the stream.

## Proof and skip reporting

- Capture the command, its exit code, stdout, and stderr — the helper writes
  all four per step under `.verify-artifacts/<run-id>/steps/`.
- Write sink output and derived files to `.verify-artifacts/<run-id>/out/`;
  the run root is deleted at teardown and the evidence directory is not.
- Assert on content, not on success: non-zero record count, `_schema` and
  `_nebu_version` on every record, a filter that measurably removed rows, a
  range with no missing ledgers.
- Prove a side effect by reading what was written, not by a clean exit — a
  sink's stdout is silent by contract (`docs/PROCESSOR_CONTRACT.md:66`).
- Record the identity of what you drove: `nebu --version` plus each
  processor's `--describe-json` version.
- Report an unreachable path with the attempted command, its exit code, and
  the unmet precondition. Do not report an entry point as verified through a
  different one.
- When seed data or a network state a journey needs is absent, the journey
  reports a failure and the reason goes in that feature's `Gotchas`. Never
  weaken the journey to make it pass.

## Feature entry contract

Each file opens with an H1 and one paragraph of user-visible behavior, then
exactly four H2 sections in order: `Sub-features`, `How to get to it (user
POV)`, `Driving it with nebu-verify`, `Gotchas`.

## Features

- [Install the CLI and a processor](./install.md) — the published quickstart:
  `go install`, `nebu install`, first events out.
- [Discover processors](./discover.md) — `nebu list`, `nebu describe`,
  `--describe-json`.
- [Fetch ledgers](./fetch.md) — `nebu fetch` over RPC and the public archive,
  `--last`, `--output`, bounded-range exit codes.
- [Build a pipeline](./pipeline.md) — origin → transform → sink, plus `jq`
  and DuckDB over the result.
- [Status stream](./status-stream.md) — `--status-file` NDJSON and run
  correlation across pipeline stages.

## Not yet mapped

Real user surfaces with no file here yet. A proof must not imply these were
covered:

- `nebu new processor` scaffolding.
- `nebu fetch --checkpoint` / `nebu resume` resumption.
- `nebu fetch --follow` live streaming (drive with `nebu-verify bg`).
- Sinks other than `json-file-sink`: postgres, nats, kafka, s3, sqlite,
  webhook.
- `checkpoint-entries` and the history-archive bucket-list path.
- The published Docker image (`withobsrvr/nebu:latest`).
- nebu as a Go library (`examples/go-library/`, `examples/simple_origin/`).
