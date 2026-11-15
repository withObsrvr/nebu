# nebu — Agent Guide

nebu is a Go CLI for Stellar data pipelines. It is intentionally Unix-pipe
composable: ledger sources and processors stream JSONL between stdin/stdout.
Cobra powers the CLI; configuration comes from flags and `NEBU_*` env vars.

Keep this file lean. Add rules only when agents repeatedly get something wrong.

## Build & verify

- Task runner is the justfile; `just` alone lists recipes. The `Makefile`
  is a deprecated shim that forwards to `just`. Do not add task logic to it.
- Everything the CI merge gate enforces: `just ci`
  (gofmt, vet, build, `go test -race -short`, stable-API snapshot check).
  Run this before claiming a change is done.
- Build: `just build`
- Test: `just test` (race detector on)
  - `pkg/source/rpc` has network/RPC-dependent tests. They skip under
    `-short`, which is what `just ci` uses; `just test` runs them and can
    fail on an unreachable or rate-limited endpoint. Report the exact
    failure; do not hide it.
- Vet: `just vet`
- Stable API check: `just api-check`
- Processor binaries: `just build-processors`
- Proto changes: `just gen-protos`
- Docs/CLI examples: `just docs-smoke`
- Lint: `just lint`. Provided by the dev shell. Not in `just ci` yet —
  its findings are untriaged, so do not treat them as regressions from
  your change without checking `main`.
- Vulnerabilities: `govulncheck ./...` must report none reachable.

### Verify against the shipped artifact

`just ci` and `just docs-smoke` check the tree. They do not check what a user
receives, and that gap is where this repo's real defects have lived. Before
claiming a user-facing surface works:

- Install the published version into an empty `GOBIN` and run the printed
  quickstart **verbatim**, not a paraphrase of it.
- Run the real binary against real data and compare it to independently known
  truth, never to its own output.
- Ask what a check reads. A local build, a cache, or a pinned copy is what it
  validates — a check never run against what users get is a claim, not
  evidence.
- If a number looks too good, confirm the exit code and that output was
  produced before believing it.

This is here because reading missed all of it: a `jq` filter in `nebu --help`
that selected nothing, the same filter generated into 30-odd processors, flags
that did not exist, `RunGenericOriginCLI` hanging after emitting every event
(a `defer` that reviews as *more* careful than an explicit close), a
millisecond field carrying whole seconds, and a one-ledger read downloading
ten because of a default inside the SDK. Each failure was silence-shaped —
empty output, a null, `.000`, a hang — and absence reads as a plausible
answer. See `docs/protocol-30-impact-review.md` for the one counter-case:
ledger states that do not exist yet can only be reasoned about and
constructed, never run.

## Governing docs

- `docs/engineering-profile.md` — which OBSRVR Engineering Standard rules
  apply to nebu, the contracts nebu owns, and its recorded exceptions.
- `docs/STABILITY.md` — what is promised to stay stable, and the
  deprecation window.
- `docs/adr/` — decisions that changed a contract. Accepted ADRs are never
  edited; supersede them instead.
- `CONTRIBUTING.md` — the human-facing version of these rules.

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

If changing `pkg/processor` or `pkg/source`, run `just api-check`.
If API drift is intentional and acceptable per `docs/STABILITY.md`, run
`just api-snapshot` and commit the `.api/` diff with the code change.

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
