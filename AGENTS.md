# nebu — Agent Guide

nebu is a Go CLI for Stellar data pipelines. It is intentionally Unix-pipe
composable: ledger sources and processors stream JSONL between stdin/stdout.
Cobra powers the CLI; configuration comes from flags and `NEBU_*` env vars.

Keep this file lean. Add rules only when agents repeatedly get something wrong.

## Build & verify

- Build: `make build`
- Test: `make test`
  - Current note: `pkg/source/rpc` has network/RPC-dependent tests that may fail.
    Report the exact failure; do not hide it.
- Vet: `make vet`
- Stable API check: `make api-check`
- Processor binaries: `make build-processors`
- Proto changes: `make gen-protos`
- Docs/CLI examples: `make docs-smoke`
- Lint, if installed: `golangci-lint run ./...`
  - Do not rely on `make lint` until its missing-binary behavior is fixed.

## Project model

- nebu stays CLI-first. Do not turn it into a service manager, orchestrator,
  gRPC runtime, or pipeline-YAML system. Use flowctl for orchestration.
- Pipeline shape: `ledger source -> origin -> transform(s) -> sink/fan-out`.
- Wire format is newline-delimited JSON. Data goes to stdout. Logs, progress,
  warnings, and diagnostics go to stderr.

## Stable surfaces

Treat these as compatibility-sensitive:

- `pkg/processor`
- `pkg/source`
- `registry.yaml` v1
- processor `--describe-json` protocol
- committed API snapshots under `.api/`

If changing `pkg/processor` or `pkg/source`, run `make api-check`.
If API drift is intentional and acceptable per `docs/STABILITY.md`, run
`make api-snapshot` and commit the `.api/` diff with the code change.

## Processor conventions

- Processor methods follow “streams never throw”: report per-event issues with
  `processor.ReportWarning(ctx, name, err)` and unrecoverable issues with
  `processor.ReportFatal(ctx, name, err)`.
- Long-running and streaming paths must honor `ctx.Done()`.
- `--describe-json` must:
  - work without required runtime flags,
  - print valid JSON only to stdout,
  - exit 0,
  - avoid starting processing logic.
- Event schemas should include `_schema` and `_nebu_version`. Bump schema IDs for
  breaking output changes.
- `registry.yaml` is the built-in catalog. External/community entries use
  `description.yml`. Processor binary names should be kebab-case and match their
  registry names.

## Stellar / operations rules

- Prefer bounded ledger ranges. Use `--follow` only when explicitly requested.
- Prefer archive mode for broad historical backfills. Use RPC for recent ledgers,
  live streams, and targeted lookups.
- Use auth from env, e.g. `NEBU_RPC_AUTH`. Never paste secrets into code,
  scripts, docs, or examples.
- For destructive or operational work — resets, migrations, credential rotation,
  large backfills, `--follow`, prod sinks, or anything touching mainnet
  infrastructure — present the sequence, rollback point, and verification
  command, then stop for confirmation.

## Review/output contracts

- For review tasks, cite file path and line range for every finding.
- No speculative findings. If evidence is not in the repo, say so.
- Deduplicate findings with the same root cause.
- Treat missing or weak tests as a finding when it affects confidence.
- Do not create plan files, markdown summaries, patches, or extra files unless
  explicitly asked.

## General rules

- Never ignore returned errors. If intentionally discarded, assign to `_` with a
  short comment explaining why.
- Never hardcode secrets, keys, tokens, or credentials.
- New exported behavior should have table-driven tests where practical.
