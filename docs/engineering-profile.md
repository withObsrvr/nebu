# Engineering profile: nebu

**Repo:** [withObsrvr/nebu](https://github.com/withObsrvr/nebu) ·
**Standard:** OBSRVR Engineering Standard v2.0 (20 October 2026) ·
**Owner:** Tillman Mosley III ·
**Profile reviewed:** 20 October 2026 · **Next review:** 20 January 2027 (CONF-2)

This is nebu's project profile under §13 of the standard. It records which rules
apply, which contracts nebu owns, where nebu tightens the standard, and which
rules are excepted. The standard itself is company-wide and is not restated here.

## 1. System class and applicable sections

nebu is two system classes at once:

| Class | What in nebu | Sections that apply |
|---|---|---|
| CLI tool | `cmd/nebu` (`fetch`, `install`, `list`, `describe`, `new`, `resume`) | 2, 3, 4, 6, 7, 8, 9, 10, 11, 12 |
| Processor or plugin | `pkg/processor`, `pkg/processor/cli`, the two in-repo educational processors | 2, 4, 6, 7, 8, 9, 10, 11, 12 |

Union of applicable sections: **§2, 3, 4, 6, 7, 8, 9, 10, 11, 12.**

**§5 (Network APIs and services) does not apply.** nebu exposes no HTTP or gRPC
surface, serves no requests, and holds no listener. Its only network behaviour is
as a *client* of Stellar RPC and object storage. If nebu ever gains a listener,
this line is wrong and the profile needs an ADR.

Published processors (`token-transfer`, `contract-events`, sinks, filters) live in
[nebu-processor-registry](https://github.com/withObsrvr/nebu-processor-registry)
and are covered by **that** repo's profile, not this one. nebu owns the *contract*
they implement; it does not own their code.

## 2. Contracts owned (VER-1)

Every contract below has one named owner — Tillman Mosley III — until nebu has
more than one maintainer. Compatibility promises are specified in
[STABILITY.md](STABILITY.md); this table is the index, not the policy.

| Contract | Version | Source of truth | Consumers |
|---|---|---|---|
| `pkg/processor` Go API | nebu v0.6.15, pre-1.0 | `.api/processor.txt` snapshot, enforced by `api-stability.yml` | External processor authors |
| `pkg/source` Go API (`LedgerSource`) | nebu v0.6.15, pre-1.0 | `.api/source.txt` snapshot | External processor authors |
| `--describe-json` protocol (`DescribeEnvelope`) | v1, forward-compatible | `pkg/processor/describe.go`; spec in [PROCESSOR_CONTRACT.md](PROCESSOR_CONTRACT.md) | `nebu describe`, flowctl, any language |
| `registry.yaml` schema | v1 | `registry.yaml` (`version: 1`); spec in [REGISTRY_SPEC.md](REGISTRY_SPEC.md) | External registries, `description.yml` authors |
| `nebu.status.v1` NDJSON status stream | v1 | `pkg/status/status.go`; spec in [STATUS_STREAM.md](STATUS_STREAM.md) | Orchestrators, flowctl, agents |
| OSC 7501 program status emission | Program Status Protocol draft rev 0.4 | `pkg/programstatus` (single shared package, pins the revision per CLI-17) | Terminals, Rex sessions |
| `nebu` CLI exit codes | v0.6.15 | [EXIT_CODES.md](EXIT_CODES.md) | Shell scripts, CI, agents |
| `nebu fetch` stdout wire format | Concatenated ledger XDR, unchanged since v0.6.11 | `cmd/nebu/fetch.go` | Every downstream origin processor |
| Event envelope fields `_schema`, `_nebu_version` | v1 | `pkg/version/version.go` (`GetSchemaVersion`) | Every consumer of processor JSONL |
| Schema IDs `nebu.transaction_stats.v1`, `nebu.ledger_change_stats.v1` | v1 | `registry.yaml` | Users of the in-repo educational processors |

**Not contracts, by explicit non-commitment:** `pkg/source/rpc`,
`pkg/source/storage`, `pkg/runtime`, `pkg/registry` Go API, `pkg/errors`,
`pkg/metrics`, `pkg/toid`, `pkg/version`, `examples/processors/*`. See
STABILITY.md § Unstable surfaces, which also still lists a `pkg/logging` that no
longer exists — a stale reference to drop on the next edit.

### Resolved: the CLI surface is a contract

STABILITY.md used to declare `cmd/nebu` flags and subcommands non-contractual
("the CLI is a product UI, not a contract"), which contradicted §3 ("Its JSON
output and exit codes are public contracts, as binding as an API") and §6, which
lists CLI flags, JSON and NDJSON shapes, and exit codes under *What counts as a
contract*. §1 gives the standard precedence, so the conflict was resolved in the
standard's favour by [ADR 0001](adr/0001-cli-surface-is-a-contract.md): flags,
subcommands, machine-readable output documents and exit codes are now on the
stable surface; human table output stays explicitly non-contractual, which CLI-6
permits.

Two known non-conformances became visible debt as a result, both now needing
migration windows rather than free renames: `--output` on `nebu fetch` means
*output file* rather than output format (CLI-3 forbids reusing the name), and
`--json` is not the CLI-6 spelling.

There is no mechanical drift check for the CLI equivalent to the `.api/`
snapshots. Adding one — a golden snapshot of `--help` and of the JSON document
shapes — is open work.

### Resolved: deprecation window

STABILITY.md promised "at least two minor releases, or until the next major
version". VER-6 requires **90 days *and* two minor releases, whichever is
longer**. The 90-day floor has been added.

## 3. Product-specific rules that tighten the standard

These are nebu's own rules. They are additive to the standard and are enforced
in review; `AGENTS.md` carries the agent-facing short form.

- **N-1 Architectural boundary.** nebu MUST NOT become a service manager,
  orchestrator, gRPC runtime, pipeline-YAML system, desired-state reconciler, or
  long-running daemon. Orchestration belongs to flowctl. This predates the
  standard and remains authoritative:
  [ARCHITECTURE_DECISIONS.md](ARCHITECTURE_DECISIONS.md).
- **N-2 Self-hostable in full.** Beyond P-8, nebu MUST stay completely usable
  with no OBSRVR-hosted service reachable — no account, no key, no phone-home.
  The hosted platform is a data source, never a dependency.
- **N-3 Stream discipline.** Tighter than CLI-5: the pipeline shape is
  `ledger source -> origin -> transform(s) -> sink/fan-out`, data on stdout is
  newline-delimited JSON (or ledger XDR for `fetch`), and *everything* else —
  logs, progress, warnings, diagnostics, OSC sequences — goes to stderr.
- **N-4 Streams never throw.** `Origin.ProcessLedger`, `Transform.ProcessEvent`
  and `Sink.WriteEvent` MUST NOT return `error`. Per-event problems are reported
  with `processor.ReportWarning`, unrecoverable ones with
  `processor.ReportFatal`. One malformed event must not kill a multi-hour run.
- **N-5 Describe is inert.** `--describe-json` MUST work with no other flags set,
  print one valid JSON envelope to stdout and nothing else, exit 0, and start no
  processing logic. This is stricter than CLI-13 and is what makes registry
  discovery safe to run against untrusted binaries.
- **N-6 Snapshot the stable surface.** Any PR touching `pkg/processor` or
  `pkg/source` MUST either leave `.api/` unchanged or commit a regenerated
  snapshot in the same PR. This is nebu's substitute for semver enforcement
  until `pkg/processor` becomes its own module, and it is a concrete
  implementation of VER-1/VER-3 that the standard does not require.
- **N-7 Bounded by default.** Ledger ranges MUST be bounded unless `--follow` is
  explicitly requested. Archive mode for historical backfills, RPC for recent
  ledgers and live streams.
- **N-8 Agent docs are versioned with the tool.** `SKILL.md` and `skills/` ship
  and version with nebu, and `just docs-smoke` mechanically verifies the CLI
  examples in docs. This is DOC-7 plus an executable check.

## 4. Rules not triggered, and exceptions

### 4a. Not triggered — no applicable surface

These are recorded so conformance reviews stop re-flagging them. They are *not*
exceptions and need no ADR or expiry; they lapse the moment nebu grows the
corresponding surface.

| Rule(s) | Why not triggered |
|---|---|
| §5 entirely (API-1–13) | No HTTP or gRPC surface; nebu is a client only |
| SEC-5 | No browser-facing surface |
| SEC-7 | nebu never holds a signing key |
| SEC-2, SEC-3, SEC-4 | No authorization surface; nebu runs as the invoking user with that user's credentials |
| OBS-4, OBS-5, OBS-6, OBS-9 | SLOs, reconciliation, alerting and incident grading attach to hosted products, not to a local CLI |
| OBS-7, OBS-8 | No production deployment and no stateful store owned by nebu |
| DATA-9, DATA-10, DATA-12 | nebu publishes no datasets; it is a transport |
| BUILD-5 | No infrastructure owned by nebu |
| STL-5 | Validator fleet rule, owned by obsrvr-ops |
| VER-9, VER-10 | No rolling multi-component deployment; nebu is a single binary |

OBS-3's pipeline signals, DATA-11 lineage, and STL-1–STL-4 **do** apply: nebu
produces the records those rules govern.

### 4b. Exceptions and scheduled non-conformances

| Rule | Record | Reason | Compensating control | Expiry |
|---|---|---|---|---|
| **CLI-12**, limited to out-of-band addressing (`inspect <id>`, `cancel <id>`) and the `queued` state | [ADR 0003](adr/0003-exception-cli-12-operation-lifecycle.md) — Accepted | Addressing a run from outside the executing process needs a durable run registry — the orchestrator role N-1 forbids. nebu has no queue, so `queued` has nothing to report | `--status-file` lets a third party follow a run it did not start; `--checkpoint` resumability; CLI-11 in-process cancellation | 2027-04-20 |
| **CLI-15** (exit-code table) | [ADR 0002](adr/0002-adopt-standard-exit-codes.md) — Accepted | Not an exception: a scheduled breaking fix. nebu's `4` means "data source unavailable"; the standard reserves `4` for auth and `7` for dependency-unavailable | Codes documented in EXIT_CODES.md and covered by table tests; migration and rollback specified in the ADR | Fix due before the next minor release |

nebu carries **one** exception. [ADR 0003](adr/0003-exception-cli-12-operation-lifecycle.md)
is Accepted as of 2026-10-20 and expires **2027-04-20**, within CONF-1's
six-month ceiling. It is reviewed at the quarterly profile review on 2027-01-20
(CONF-2), which falls before expiry; an expired exception counts as
non-conformance, so neither date should pass unexamined.

A **BUILD-1** exception for the Makefile was drafted and then discarded in
favour of conforming: nebu's task runner is now a justfile. The grandfather
clause would have permitted the Makefile to stay, but it also requires new task
logic to go in the justfile — and the same change added three new targets, so
the exception would have been incoherent from the day it was written.
`Makefile` survives as a shim that forwards to `just` and warns, scheduled for
removal one release later.

CLI-12 is **partially excepted only.** Its lifecycle-vocabulary and
domain-state clauses are now satisfied (ADR 0004); its "accepting a job is not
completing it" clause always was. The operation ID, the `--timeout` form of
`wait`, and the `events --follow` spelling are missing work with no
architectural obstacle, tracked in the backlog below — not excepted. ADR 0003
has the clause-by-clause disposition.

The **TEST-2.1** exception previously anticipated here is not needed: the
network-dependent tests in `pkg/source/rpc` already guard with
`testing.Short()`, and the CI gate runs `go test -race -short`, so a green gate
never depends on a reachable Stellar RPC endpoint.

## 5. Conformance status

Audited against v2.0 on 20 October 2026; updated after the conformance pull
request that introduced this profile. **nebu does not yet conform**, but the
merge gate that holds everything else in place now exists.

| § | Area | Status | Moved this cycle |
|---|---|---|---|
| 3 | Command-line interfaces | Partial | — |
| 4 | Data contracts | Partial | — |
| 6 | Versioning and releases | Partial | Reproducible builds, archive contents, changelog form |
| 7 | Security and supply chain | Partial | `SECURITY.md`, `govulncheck` in CI, 10 reachable vulns cleared |
| 8 | Observability | **Fails** | — |
| 9 | Testing and quality gates | Partial | CI merge gate added; `-race` on by default |
| 10 | Builds and reproducibility | Partial | `just` task runner (BUILD-1), `-trimpath`, `mod_timestamp`, OCI `.source`, toolchain pinned |
| 11 | Stellar domain | **Fails** | — |
| 12 | Documentation and ADRs | Partial | `LICENSE`, `CONTRIBUTING.md`, `docs/adr/`, this profile |

**Ahead of the standard:** stream discipline (CLI-5, N-3); OSC 7501 through one
revision-pinning package (CLI-17, CLI-20, CLI-21); mechanically enforced
contract-drift detection (`.api/` + `api-stability.yml`, beyond VER-1);
versioned agent docs with an executable example check (DOC-7, N-8); a published
stability policy and processor contract.

### Closed in this cycle

CI merge gate (build, `gofmt`, `go vet`, `go test -race -short`, API snapshot,
`govulncheck`) · 10 reachable vulnerabilities cleared · Go toolchain pinned and
aligned with the flake (BUILD-2) · `LICENSE` (Apache-2.0, resolving the
MIT/Apache contradiction) · `SECURITY.md` (SEC-12) · `CONTRIBUTING.md` ·
`docs/adr/` with four records (DOC-4), including the accepted exception that
makes the CLI-12 out-of-band gap live under CONF-1 rather than unrecorded ·
this profile (§13) · CLI-surface
contract conflict resolved (ADR 0001) · VER-6 deprecation floor · `-trimpath`
and `mod_timestamp` (BUILD-3) · OCI `image.source` (BUILD-4) · Keep a Changelog
form (VER-11) · `gofmt` clean · lint no longer silently passes · BUILD-1 `just`
task runner, with the Makefile reduced to a deprecated forwarding shim.

### The next gap to close

**BUILD-7 — nebu defaults to the Stellar public network.** `--network` defaults
to `network.PublicNetworkPassphrase` in four places (`cmd/nebu/fetch.go:263`,
`pkg/processor/cli/origin.go:228`, `generic_origin.go:235`,
`proto_origin.go:214`), and the default `--rpc-url` is a mainnet archive
endpoint. The rule is unambiguous: "The Stellar network MUST be explicit
configuration, and no tool may default to pubnet." The passphrase/endpoint
agreement check is also only a URL-substring heuristic
(`cmd/nebu/log.go:71-101`), so the "MUST refuse to start on mismatch" half is
unmet too. This is a breaking change and needs its own ADR.

### Ordered backlog

1. **BUILD-7**, as above.
2. **Fix the event envelope's network and timestamp fields.** Two separate
   changes with different change classes, in the two live emission paths —
   `pkg/processor/cli/proto_origin.go:153-162` (`RunProtoOriginCLI`, used by 26
   registry processors) and `generic_origin.go:155-170` (`RunGenericOriginCLI`,
   used by 4):
   - **DATA-6, additive.** Records carry `_schema` and `_nebu_version` but no
     `network`. Verified against 39,652 real token-transfer events: none
     carries one. The passphrase is in scope at the emission point. A minor
     bump covers it.
   - **DATA-3, breaking.** Ledger close time is present but wrongly typed:
     `meta.closedAtUnix` is a Unix epoch *string* (`"1765158311"`), where
     DATA-3 requires RFC 3339 in UTC with a `Z` suffix. This is a format
     change to an existing field, not an addition, so it is breaking and needs
     an ADR, a schema-ID bump, and a migration window — not the minor bump an
     added field would get.
3. **Remap exit codes** per [ADR 0002](adr/0002-adopt-standard-exit-codes.md),
   including `143` on SIGTERM (CLI-11).
4. **Close the non-excepted CLI-12 clauses**, none of which need operation
   state: mint a per-invocation run ID and carry it in the status stream and
   diagnostics; add `--timeout` (CLI-3) returning exit code 6 on expiry
   (CLI-15), which completes `wait --for --timeout` for a foreground process;
   and give the status stream a documented `events --follow` spelling. Overlaps
   Priority 9 of [CLI_QUALITY_ROADMAP.md](CLI_QUALITY_ROADMAP.md).
5. **Real-ledger fixtures** (TEST-1) captured from testnet and pubnet with the
   ledger range recorded, golden tests for every machine-readable output
   (TEST-2.3) and for exit codes, and fuzz seeds for the XDR, JSONL and
   registry-YAML decoders (TEST-3). The largest item here.
6. **Structured logging** — `log/slog` to stderr with `service`, `version`,
   `network` and `ledger` (OBS-1) — and the missing pipeline signals: lag in
   ledgers *and* seconds, last checkpointed ledger, gap count (OBS-3).
7. **Declare CAP and SEP compliance** in schema metadata (STL-1) and mark
   pre-protocol-23 reconstructed events as derived (STL-3).
8. **Restore tag signing.** `v0.6.14` and `v0.6.15` are unsigned; `v0.6.12` and
   `v0.6.13` were signed. Then add Sigstore signing, SBOM attachment and SLSA
   provenance to the release path (SEC-10, VER-12).
9. **Remove argument-borne secrets** (CLI-10): `--rpc-header` and `--dsn`
   should read from the environment, a file path, or stdin only.
10. **CLI-3 and CLI-6 conventions** — `--output json|ndjson` superseding
    `--json`, freeing `--output` from its current file meaning, plus
    `--config`, `--timeout`, `--no-color`, `--yes` and `NO_COLOR` (CLI-7).
    Overlaps [CLI_QUALITY_ROADMAP.md](CLI_QUALITY_ROADMAP.md); plan it there.

### Verified satisfied — do not re-raise

Checked against a real pipeline run (39,652 token-transfer events over mainnet
ledgers 60200000-60200050) rather than by reading the code, after an earlier
audit pass misread which code path actually runs:

- **DATA-7 cursor.** Events carry `meta.ledgerSequence`,
  `meta.transactionIndex` and `meta.operationIndex` — the ledger sequence plus
  an ordinal within the ledger that the rule requires. The SEP-35 1-indexed
  convention is preserved from the SDK.
- **DATA-7 delivery semantics.** Stated in
  [CHECKPOINTS.md](CHECKPOINTS.md#delivery-semantics): at-least-once, with
  TOIDs named as the stable key for deduplication, and an explicit statement
  that nebu does not claim exactly-once across a Unix pipe.
- **DATA-2 amounts.** Carried as integer strings in the asset's smallest unit
  (`"amount":"100"`), never floating point.

`RunOriginCLI` and its `simplifyEvent` helper
(`pkg/processor/cli/origin.go:80,310`) **do** drop close time and the
intra-ledger indices, but nothing calls them: there are no in-repo callers, and
the processor registry's own authoring guide marks `RunOriginCLI` as "legacy:
token_transfer.TokenTransferEvent only". It is exported, so an unknown external
processor could still use it. Confirm that before deleting it; auditing it as
though it were the live path is what produced the earlier error.

Three emission paths now each build the envelope separately
(`origin.go`, `generic_origin.go`, `proto_origin.go`), which is a latent
**DATA-5** problem — one record type, three places that decide its shape. Worth
collapsing when the DATA-6 field lands, since that change has to touch all of
them anyway.

### Known gaps in the gate itself

Recorded so they are not mistaken for conformance:

- **The CI gate does not literally use the flake, and the flake's Go is
  behind.** BUILD-2 says "CI MUST use the same flake as developers." CI
  resolves Go from `go.mod`'s `toolchain` line, so CI and developers build with
  the same compiler — but that line pins **go1.26.10**, not the 1.26.6 the
  flake's nixpkgs provides, because 1.26.6's standard library carries 11
  reachable vulnerabilities fixed in 1.26.9. Go's toolchain switching fetches
  1.26.10 in both places, so the dev shell's `go` is effectively a bootstrap.
  Closing this properly needs a nixpkgs bump to a Go ≥ 1.26.9 and then moving
  the gate onto `nix develop`. Until then SEC-9 is being prioritised over a
  literal reading of BUILD-2, which is the correct order under P-1 and §1.
- **Secret scanning is not in the gate** (SEC-6, and part of the §9 merge gate).
  The intended route is GitHub's native secret scanning with push protection,
  which is free for public repositories and is a repository-settings change
  rather than a workflow. It has not been enabled yet.
- **`golangci-lint` is not in the gate** (§9 names `staticcheck` or
  `golangci-lint`). It is now provided by the dev shell and `just lint` runs it,
  but its findings are untriaged and wiring it in today would block every PR.
  Triage, then add.
- **No change-classification gate** (VER-3). `CONTRIBUTING.md` asks for a
  classification on contract-touching PRs; nothing enforces it. A pull-request
  template is the cheap next step.
- **`just docs-smoke` and `just test-integration` are not in the gate.** Both
  need network and installed processor binaries.
