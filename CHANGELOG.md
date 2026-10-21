# Changelog

All notable changes to nebu are documented here. For full release artifacts, see [GitHub Releases](https://github.com/withObsrvr/nebu/releases).

This changelog follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Changes are grouped under Added, Changed, Deprecated, Removed, Fixed and Security. Entries before v0.6.15 predate that convention and are left as written.

## Unreleased

### Added

- **`processor.LedgerCloseTimeMs(ledger)`** returns a ledger's close time in milliseconds since the Unix epoch. Added to the stable surface, because every origin processor needs it and the obvious implementation is wrong. Protocol 30's CAP-88 puts the millisecond value in the `StellarValueExt` union, and **two** arms carry it — `SIGNED_MS` (3) via `SignedMsValue` and `EMPTY_TX_SET_MS` (4) via `ProposedMsValue`. Code that reads only `GetSignedMsValue()`, the natural first guess, silently falls back to second resolution on every *empty* Protocol 30 ledger — a case no fixture taken from a busy ledger exercises. Pre-Protocol-30 ledgers (arms 0-2) have their whole-second close time promoted, losing nothing, since those close times are seconds apart. Covered by a table test over all five discriminants.
  - Prefer it over the SDK's `LedgerCloseMeta.ClosedAt()` and `LedgerCloseTime()` whenever a close time is emitted, stored, ordered, or used to bucket events: those return whole seconds, and from Protocol 30 onward consecutive ledgers can close within the same second, so second-resolution windowing produces wrong answers rather than errors.
  - For the RFC 3339 UTC form that OBSRVR data contracts require (DATA-3): `time.UnixMilli(processor.LedgerCloseTimeMs(ledger)).UTC().Format(time.RFC3339Nano)`.
- `LICENSE` (Apache-2.0), `SECURITY.md` and `CONTRIBUTING.md`. nebu previously shipped no license file at all, and the two claims in the tree disagreed — `README.md` said MIT while `flake.nix` declared Apache-2.0.
- `docs/engineering-profile.md`: nebu's project profile under the OBSRVR Engineering Standard v2.0 — applicable rules, the contracts nebu owns, its product-specific rules, its exceptions, and its conformance status.
- `docs/adr/`: architecture decision records in MADR form, with an index. ADR 0001 resolves whether the CLI surface is a contract; ADR 0002 schedules the exit-code remap; ADR 0003 records the narrowed CLI-12 exception and ADR 0004 the task-runner exception — both accepted, so those gaps are live exceptions under CONF-1 expiring 2027-04-20 rather than unrecorded non-conformance; ADR 0004 adds the lifecycle vocabulary below.
- `nebu.status.v1` records carry a required `lifecycle` field holding the CLI-12 operation lifecycle (`running`, `succeeded`, `failed`, `cancelled`) alongside the existing Program Status Protocol `state` token. One consumer state machine now reads nebu, flowctl and the `obsrvr` CLI. Additive — consumers are already required to ignore unknown fields, so the schema version does not move. The mapping is derived on write and cannot be set independently, so the two fields can never disagree. Note that `state: idle` maps to `lifecycle: cancelled`: CLI-17 specifies `idle` for interruption, which is the only case nebu emits it. See ADR 0004 and `docs/STATUS_STREAM.md`.
- CI merge gate (`.github/workflows/ci.yml`): build, `gofmt`, `go vet`, `go test -race -short`, the stable-API snapshot check, and `govulncheck`. No workflow previously built or tested nebu on a pull request.
- A `justfile` as the task runner (BUILD-1), with `just ci` running exactly what the merge gate runs, plus `just test-short` and `just fmt-check`. `just` with no arguments lists every recipe.

### Changed

- **Protocol 30 ("Bell"): upgraded `github.com/stellar/go-stellar-sdk` to v0.8.0** in nebu (from v0.7.3) and the hooks example (from v0.7.1). This is required, not optional. CAP-88 adds two arms to the `StellarValueExt` union — `STELLAR_VALUE_SIGNED_MS` (3) and `STELLAR_VALUE_EMPTY_TX_SET_MS` (4) — and an SDK that does not know those discriminants fails to decode any Protocol 30 ledger. Testnet votes 2026-11-03, pubnet 2026-11-23.
  - No nebu source changes were needed, and the committed stable-API snapshots are unchanged.
  - `nebu fetch` never decoded ledgers (it streams raw bytes and derives sequences arithmetically), so raw archival was never exposed to this; the five `xdr.Unmarshal` decode paths were.
  - Verified on mainnet ledgers 60200000-60200050: `fetch` output is byte-identical to the previous release, and a 39,652-event pipeline through a processor still pinned to v0.7.3 is unchanged.
  - **Not yet addressed:** CAP-88 gives close times millisecond resolution, so consecutive ledgers can close within the same whole second. Any consumer using whole-second close times for ordering or windowing becomes ambiguous — including the `closed_at_unix` field defined in the processor registry's protos, which `time-window` and `aggregator` filter on. That is a registry-side schema decision, tracked separately. The SDK's own `LedgerCloseMeta.ClosedAt()` also truncates to seconds; the millisecond value lives in the new union arms.
  - **Not yet addressed:** CAP-84 muxed contract addresses add an `SCAddress` variant and the `W` strkey prefix. nebu passes SDK-produced address strings through unmodified, but processors that parse, filter, or re-encode addresses need review.
- **The task runner is now a `justfile`, not the Makefile** (BUILD-1). `Makefile` remains as a shim that forwards every target to `just` and prints a deprecation note; it is scheduled for removal one release after this one. Do not add task logic to it. The dev shell and CI both provide `just` 1.51.0.
- `blocked` is now a documented domain state in `docs/STATUS_STREAM.md`, as CLI-12 requires for any state outside its five-value lifecycle: the operation is running but cannot progress until it receives input. It maps to `lifecycle: running` because it is not terminal. nebu does not emit it yet; it is retained so that adding an interactive confirmation (CLI-8 `--yes`) needs no schema change.

- **Go 1.26 is now required** (`go.mod` declares `go 1.26.0`, `toolchain go1.26.10`). The module previously declared Go 1.25.4 while the Nix dev shell already provided Go 1.26.6. The toolchain is pinned to 1.26.10 rather than the dev shell's 1.26.6 because 1.26.6's standard library carries 11 reachable vulnerabilities fixed in 1.26.9 — the pinned toolchain governs, and Go fetches it automatically. `go.work` and `examples/hooks/go.mod` move with it.
- CLI flags, subcommands, machine-readable output documents and exit codes are now part of nebu's stable surface and follow the deprecation process. `docs/STABILITY.md` previously declared them non-contractual, which contradicted the engineering standard. Human-readable table output remains explicitly non-contractual. See ADR 0001.
- The deprecation window in `docs/STABILITY.md` is now "at least 90 days and two minor releases, whichever is longer". The previous wording omitted the 90-day floor, so a fast run of minor releases could close the window in days.
- `just test` runs with the race detector.
- All GitHub workflows resolve the Go version from `go.mod` instead of hardcoding it. They had drifted apart — `1.25` in the API-stability workflow, `1.25.4` in the release workflows.
- Release builds are reproducible: `-trimpath` and a commit-derived `mod_timestamp` are set, so the same tag rebuilds to the same bytes.
- Release archives now include `LICENSE`, `SECURITY.md` and `CHANGELOG.md`.
- Container images carry `org.opencontainers.image.source` and `.licenses` labels. `.source` is required and was missing.

### Fixed

- The dev shell's greeting, the integration suite's error message, and the `Makefile` shim's own comment still referred to `make` targets or to an unrelated ADR after the justfile conversion. The CI workflows also pinned `just` 1.51.0 while this repo's `flake.lock` resolves to 1.43.1 — the opposite of the BUILD-2 alignment the pin exists for. CI now pins 1.43.1, and the comment records that the version must be read with `nix develop --command just --version` rather than `nix eval nixpkgs#just.version`, which reads the live registry instead of this repo's lock.

- `flake.nix` declared `version = "0.6.3"` on a v0.6.15 tree.
- `docs/STABILITY.md` listed a `pkg/logging` package that no longer exists.
- Nine files were not `gofmt`-clean.

### Security

- Updated dependencies to clear **10 reachable vulnerabilities** reported by `govulncheck`: `golang.org/x/net` v0.53.0 → v0.60.0 (five HTTP/2 issues plus an IDNA Punycode issue), `golang.org/x/text` v0.36.0 → v0.42.0 (infinite loop on invalid input), `google.golang.org/grpc` v1.80.0 → v1.83.1 (HTTP/2 DATA-frame memory exhaustion, xDS RBAC) and `go.opentelemetry.io/otel/sdk` v1.43.0 → v1.45.0 (exporter endpoint URLs in logs). `govulncheck` now reports no reachable vulnerabilities, and runs on every pull request.
- `SECURITY.md` documents how to report a vulnerability, what is in scope, and two known gaps that are not yet fixed: secrets can still be passed as command-line arguments (`--rpc-header`, `--dsn`), and `nebu` still defaults to the Stellar public network.

## v0.6.15

- Correctness: bounded `nebu fetch` runs now fail instead of silently succeeding when the requested range is truncated. Stable exit codes distinguish usage errors, missing processors, invalid ranges, unreachable data sources, general failures, and interruptions.
- Resume: added opt-in, sink-independent fetch checkpoints with atomic persistence, durable output ordering, range validation, append-on-resume behavior, and protection against checkpoint/output path aliases.
- Status: added OSC 7501 Program Status Protocol reporting for interactive terminals and an opt-in `nebu.status.v1` NDJSON stream for orchestrators. Fetch and processor helpers report lifecycle, ledger progress, event progress, cancellation, and failures without contaminating stdout data.
- Processor contract: documented discovery, launch, structured status, cancellation, exit-code, and flowctl integration boundaries while keeping orchestration outside nebu.
- Registry: external `description.yml` parsing now preserves metadata, examples, schemas, repository refs, and deprecation details; validation is strict across the complete YAML stream.
- Dependencies: upgraded `github.com/stellar/go-stellar-sdk` to v0.7.3.

## v0.6.14

- Added OSC 7501 Program Status Protocol support to nebu and processor CLI helpers, including `auto`, `always`, and `never` modes.

## v0.6.13

- Upgraded `github.com/stellar/go-stellar-sdk` to v0.7.1.

## v0.6.12

- Corrected GoReleaser configuration and release packaging.

## v0.6.11

- Protocol 28: upgraded `github.com/stellar/go-stellar-sdk` from v0.6.0 to v0.7.0 in nebu, the hooks example, and all 34 Go processor modules in `nebu-processor-registry`.
- Raw fetch: `nebu fetch` now uses the SDK's `LedgerStream.RawLedgers` path for both RPC and archive mode. Ledger XDR is written directly to stdout or `--output` without decoding into `LedgerCloseMeta` and marshaling it again; the existing concatenated-XDR wire format is unchanged.
- Processor ownership: removed duplicated published processor implementations from nebu. The external processor registry is now their canonical source; the two in-repo educational processors remain.
- Build: `make build-processors` now installs published processors from `github.com/withObsrvr/nebu-processor-registry` and builds the educational processors locally.
- Release packaging: GoReleaser archives and the `withobsrvr/nebu` container now contain the nebu CLI only. Published processors release independently from `nebu-processor-registry`.
- Testing: added byte-for-byte XDR compatibility, no-decode, stream-error, short-write, range, and RPC-header tests for raw fetch. Verified RPC and public-S3 fetch pipelines through `token-transfer`; all nebu and registry processor tests and vet checks pass.

## v0.6.10

- `make build` now emits the canonical `bin/nebu` executable before compiling the remaining packages.
- Build versions are derived from `git describe --tags --always --dirty` instead of a stale hardcoded value, with `VERSION` override and `dev` fallback preserved.

## v0.6.9

- Added checksum-verified installation of prebuilt processor binaries, enabling processors written outside Go. Binary installs are atomic and gated by a successful `--describe-json` check.
- Added the normative, language-agnostic [`docs/PROCESSOR_CONTRACT.md`](docs/PROCESSOR_CONTRACT.md).
- Extended registry v1 entries with an optional `install` block while preserving compatibility with existing Go-module processors.

## v0.6.8

- Dependencies: `github.com/stellar/go-stellar-sdk` v0.5.0 → v0.6.0 (with the matching `go-xdr` bump) and duckdb 1.5.1 → 1.5.4 in `flake.nix`.
- Testing: rewrote the `pkg/source/rpc` integration tests to derive ledger ranges from the RPC health endpoint instead of hardcoded sequences that age out of the retention window, and to assert `Stream` errors instead of logging them. Added fully offline unit coverage for auth-header injection, `Stream` error paths (including channel-close-on-error), bounded-range boundaries, and cancellation; network-dependent tests now skip under `go test -short`.
- Docs: added `AGENTS.md` (agent/contributor guide) and `docs/SMOKE_TESTING.md` (bounded live-RPC end-to-end smoke test). `scripts/sync-skill-docs.sh` now syncs every `SKILL.md` copy, fixing drift in `docs/SKILL.md`.
- Skill: rewrote the nebu skill (`SKILL.md`) and added trigger evals (`skills/nebu/evals/`, `scripts/eval-nebu-triggers.sh`).
- Reference hooks: added `examples/hooks/` as a new module with three drop-in `runtime.Hooks` implementations covering the most requested patterns — `rate_limit.go` (token-bucket throttle, stdlib only), `metrics.go` (Prometheus counters and histograms), and `tracing.go` (OpenTelemetry parent-pipeline + per-ledger child spans). Each file follows the same "copy into your processor's cmd/ directory" pattern as the existing `progress.go`. The smoke-test binary `examples/hooks/cmd/hooks-demo` wires all three hooks into a real RPC run; verified end-to-end against ledgers 62080000-62080010 (11 spans, full Prometheus scrape on :9090, rate limit active). `docs/build-processors.html` §08_HOOKS updated to link each file; `docs/processors.html` token-transfer card gained a `progress-bar hook` link.
- Documentation: archive-mode examples now default to the public `aws-public-blockchain/v1.1/stellar/ledgers/pubnet` S3 bucket (region `us-east-2`). No AWS account is required — the Stellar SDK's S3 datastore falls back to `AnonymousCredentials` automatically when no credentials are present. Verified end-to-end: fetching ledgers 62080000-62080010 anonymously and piping through `token-transfer` produces 8,713 well-formed events. `docs/ARCHIVE_MODE.md`, `docs/BACKFILL_STRATEGIES.md`, and the website cookbook/man/quickstart pages updated. No code changes.
- Website: full redesign rolled out to `docs/` (GitHub Pages). New pages — `man.html`, `processors.html`, `schemas.html`, `changelog.html` — plus shared `assets/nebu.css` and `assets/nebu.js`. Install script moved to `docs/install.sh` so `curl -sSL https://nebu.withobsrvr.com/install.sh | sh` resolves.

## v0.6.7

- Updated AWS SDK v2 dependencies across the root module and all reference processor modules to address the EventStream decoder DoS advisory:
  - `github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream` → `v1.7.8`
  - `github.com/aws/aws-sdk-go-v2/service/s3` → `v1.99.0`
- Pulled related AWS SDK internals forward (`aws-sdk-go-v2`, `smithy-go`) via `go mod tidy`.
- Verified that `google.golang.org/grpc` is already at `v1.80.0`, newer than the fix version for the `:path` authorization-bypass advisory (`v1.79.3`).

## v0.6.6

- Removed an unused command and cleaned up documentation (#24).

## v0.6.5

- Updated library dependencies.

## v0.6.4

- Documentation updates (#19).

## v0.6.3

- Fixed `_nebu_version` in event envelopes for processors installed via `go install`.
- Pinned all reference processors to `github.com/withObsrvr/nebu v0.6.2`, so runtime build-info fallback now reports the real version instead of `dev`.
- No other code changes in this release.

## v0.6.2

- Unblocked `go install` for in-tree processors.
- `nebu --version` now reads runtime build info when ldflags are absent.
- `nebu install` now includes captured `go install` output in failures.
- Fixed `usdc-filter` to read the correct `transfer.assetCode` field.
- Added Claude Code skill docs and worked pipeline examples.

## v0.6.0

- Added runtime hooks via `Runtime.Use(Hooks{...})`.
- Added deterministic fatal-priority preemption.
- Added CI enforcement for stable API snapshots.

## v0.5.0

- Declared the stable contract surface: `pkg/processor`, `pkg/source`, `registry.yaml` v1, and `--describe-json`.
- Formalized transform and sink interfaces.
- Split `pkg/source` into a dep-free interface package with concrete implementations elsewhere.
- Added the registry specification.
