# Contributing to nebu

nebu is a Unix-pipe-composable CLI for Stellar data pipelines. Ledger sources
and processors stream newline-delimited JSON between stdin and stdout. Keeping
that composability intact matters more than any individual feature.

Before a substantial change, please open an issue. nebu has a deliberately
narrow scope (see [What nebu is not](#what-nebu-is-not)) and it is better to find
out early that an idea belongs in flowctl.

## Getting set up

Two commands from a fresh clone:

```bash
direnv allow     # or: nix develop
just test
```

`just` with no arguments lists every recipe. A `Makefile` still forwards to
`just` for one release, so old muscle memory keeps working — but it warns, and
it is going away.

The flake pins the dev-shell tooling. The Go compiler itself is pinned by the
`toolchain` line in `go.mod`, which Go fetches automatically — it is ahead of
the version in the flake, because the flake's Go has known standard-library
vulnerabilities. CI resolves Go the same way, so it builds with exactly the
compiler you do.

## The commands you need

| Command | What it does |
|---|---|
| `just build` | Build the `nebu` binary into `bin/` |
| `just test` | Full test suite with the race detector |
| `just ci` | Exactly what the merge gate runs — use this before pushing |
| `just vet` | `go vet` |
| `just fmt` | `gofmt` the tree |
| `just api-check` | Verify the stable Go API matches the committed snapshots |
| `just api-snapshot` | Regenerate those snapshots after an intentional change |
| `just build-processors` | Install published processors and build the in-repo examples |
| `just test-integration` | Run pipelines end to end against installed processors |
| `just docs-smoke` | Verify the CLI examples in the docs still work |
| `just gen-protos` | Regenerate protobuf code |

Network-dependent tests in `pkg/source/rpc` skip under `-short`, which is what
`just ci` uses. `just test` runs them, so it needs network and can fail on a
rate-limited RPC endpoint. That is expected; report the exact failure rather
than working around it.

## House rules

These are the things reviewers will send back, so they are worth knowing up
front. The short form for AI agents lives in [AGENTS.md](AGENTS.md).

**Streams.** Data goes to stdout. Logs, progress, warnings, diagnostics, and
terminal escape sequences go to stderr. The wire format is newline-delimited
JSON — `nebu … | jq` must always work. `nebu fetch` is the exception: it writes
concatenated ledger XDR.

**Streams never throw.** `Origin.ProcessLedger`, `Transform.ProcessEvent` and
`Sink.WriteEvent` do not return `error`. Report per-event problems with
`processor.ReportWarning(ctx, name, err)` and unrecoverable ones with
`processor.ReportFatal(ctx, name, err)`. One malformed event must not kill a
run that has hours of ledgers left.

**Context.** Every long-running and streaming path honours `ctx.Done()`.

**Errors are never ignored.** If an error is deliberately discarded, assign it
to `_` with a comment saying why.

**No secrets anywhere.** Not in code, tests, docs, examples, or commit
messages. Credentials come from the environment — `NEBU_RPC_AUTH` and friends.

**`--describe-json` stays inert.** It must work with no other flags set, print
one valid JSON envelope to stdout and nothing else, exit 0, and start no
processing logic.

**Event schemas** carry `_schema` and `_nebu_version`. Bump the schema ID for a
breaking output change.

**Bounded ranges by default.** `--follow` only when the user asks for it.

## Testing expectations

- New exported behaviour gets table-driven tests.
- A bug fix adds a test that fails without the fix. For a data bug, that
  includes the ledger that exposed it.
- Functional tests should use real chain data captured from testnet or pubnet,
  with the source ledger range recorded. Invented data is for injecting failures
  — timeouts, malformed input, write errors — not for showing that a feature
  works.
- Concurrent code must pass `go test -race`.

## Changing a contract

Some surfaces are compatibility-sensitive: `pkg/processor`, `pkg/source`,
`registry.yaml` v1, the `--describe-json` protocol, the `nebu.status.v1` stream,
CLI flags and subcommands, machine-readable output shapes, and exit codes.
[docs/STABILITY.md](docs/STABILITY.md) is the authority on what is promised.

If your change touches one:

1. **Classify it** in the pull-request description — additive, deprecating,
   breaking, or semantic. "Semantic" means the wire format is unchanged but the
   meaning is not: a decoding fix, a changed unit, a new protocol rule. Semantic
   changes are treated as breaking.
2. **Run `just api-check`.** If `pkg/processor` or `pkg/source` drifted and the
   drift is intended, run `just api-snapshot` and commit the `.api/` diff in the
   same pull request. Reviewers use that diff as the signal that the contract
   moved.
3. **Write an ADR** for anything breaking or semantic —
   [docs/adr/](docs/adr/README.md), MADR format, next number in the index.
   Accepted ADRs are never edited; supersede them instead.
4. **Deprecate rather than remove.** A deprecated public interface keeps working
   for at least 90 days and two minor releases, whichever is longer.

## Commits and changelog

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)
— `feat:`, `fix:`, `docs:`, `build:`, `chore:`, `refactor:`, `test:`, with a
scope where it helps (`feat(registry):`). Breaking changes get a `!` or a
`BREAKING CHANGE:` footer.

User-visible changes get a [CHANGELOG.md](CHANGELOG.md) entry under
`## Unreleased`, in the right group: Added, Changed, Deprecated, Removed, Fixed,
or Security.

## What nebu is not

nebu will not become a service manager, orchestrator, gRPC runtime,
pipeline-YAML system, desired-state reconciler, or long-running daemon. Those
belong to [flowctl](https://github.com/withObsrvr/flowctl). The reasoning is in
[docs/ARCHITECTURE_DECISIONS.md](docs/ARCHITECTURE_DECISIONS.md) and it is still
current.

nebu also stays fully usable with no OBSRVR-hosted service reachable — no
account, no key, no phone-home. The hosted platform is a data source, never a
dependency.

Published processors live in
[nebu-processor-registry](https://github.com/withObsrvr/nebu-processor-registry).
New processors go there, not here. The two processors under
`examples/processors/` are teaching material.

## Security

Do not report vulnerabilities through issues or pull requests. See
[SECURITY.md](SECURITY.md).

## License

Contributions are licensed under the [Apache License 2.0](LICENSE), the same as
the project.
