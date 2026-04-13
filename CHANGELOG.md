# Changelog

All notable changes to nebu are documented here. For full release artifacts, see [GitHub Releases](https://github.com/withObsrvr/nebu/releases).

## Unreleased

- Updated AWS SDK v2 dependencies across the root module and all reference processor modules to address the EventStream decoder DoS advisory:
  - `github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream` → `v1.7.8`
  - `github.com/aws/aws-sdk-go-v2/service/s3` → `v1.99.0`
- This also pulled related AWS SDK internals forward (`aws-sdk-go-v2`, `smithy-go`, and related service internals) via `go mod tidy`.
- Verified that `google.golang.org/grpc` is already at `v1.80.0`, which is newer than the fix version for the `:path` authorization-bypass advisory (`v1.79.3`).

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
