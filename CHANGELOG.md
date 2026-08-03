# Changelog

All notable changes to nebu are documented here. For full release artifacts, see [GitHub Releases](https://github.com/withObsrvr/nebu/releases).

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
