# 0002. Adopt the standard exit-code table

- **Status:** Accepted
- **Date:** 2026-10-20
- **Owner:** Tillman Mosley III
- **Affected interfaces:** `nebu` CLI exit codes, `pkg/errors.Kind`
- **Change class (VER-3):** Breaking

## Context and problem statement

nebu's exit codes ([EXIT_CODES.md](../EXIT_CODES.md)) and CLI-15 of the OBSRVR
Engineering Standard disagree on the meaning of code `4`:

| Code | nebu today | Standard (CLI-15) |
|---|---|---|
| 3 | Not found or ambiguous | Resource not found |
| 4 | **Data source unavailable** | **Not authenticated or not permitted** |
| 5 | unused | Conflict or failed precondition |
| 6 | unused | Timed out waiting |
| 7 | unused | **Dependency unavailable (network, RPC, storage)** |
| 8 | unused | Data validation failed |
| 143 | unused (SIGTERM exits 130) | Interrupted (SIGTERM) |

`pkg/errors.KindUnreachable` maps to `4` today, so RPC connection failures,
HTTP 429 rate limits, and HTTP 401 auth failures all collapse into a single code
that means something different company-wide. A caller that retries on `4`
expecting "dependency unavailable" — the correct behaviour per the standard's
retry column — would also retry a permission failure that can never succeed.

## Decision

Adopt the CLI-15 table. Specifically:

- `4` is reserved for authentication and permission failures. A new
  `errors.KindUnauthorized` carries it, and `AuthRequired` (HTTP 401) moves to it.
- `7` becomes "dependency unavailable". `KindUnreachable` maps here —
  connection failures, rate limits, archive and storage errors.
- `6` is adopted for timeouts once nebu has a `--timeout` flag (CLI-3).
- `8` is adopted for data validation failures — gap, duplicate, checksum, and
  schema errors, including the truncated-range detection added in v0.6.15.
- SIGTERM exits `143`; SIGINT keeps `130`. Both still write a checkpoint first
  (CLI-11).
- Codes `10`–`63` stay available for nebu-specific additions and must be
  documented in EXIT_CODES.md, per CLI-15.

## Consequences

**Good:** a caller can treat exit codes from nebu, flowctl, stellar-ingest and
the `obsrvr` umbrella identically, which is the entire point of the rule.

**Bad:** this breaks any script that branches on `4`. nebu is pre-1.0, so VER-2
permits a minor bump to break, but the change still needs the migration handling
below — the codes were introduced in v0.6.15 and documented as stable, so
silently changing them is not acceptable.

## Migration

- The remap lands in a single minor release, called out under **Changed** in the
  changelog and listed as breaking in the release notes (VER-11).
- Because `4` keeps a meaning rather than disappearing, a script branching on it
  fails quietly rather than loudly. To make the change visible, the release that
  carries the remap also prints a one-line warning to stderr on the first exit
  with a remapped code when `NEBU_EXIT_CODE_V1=1` is set, so operators can opt
  into detection during their migration. The variable is removed one minor
  release later.
- `pkg/errors` is not part of the stable surface (STABILITY.md § Unstable
  surfaces), so adding `KindUnauthorized` needs no window.

## Rollback

Revert the `ExitCode` mapping. The codes are produced in exactly one function,
`pkg/errors.ExitCode`, and asserted in `pkg/errors/errors_test.go`, so rollback
is a single-function change plus its table test.

## Tests

- `pkg/errors/errors_test.go` extends its table with one case per code.
- A CLI subprocess test (TEST-2.4) asserts the real process exit status for each
  class: usage error, unknown processor, 401, connection failure, truncated
  range, SIGINT, SIGTERM.
