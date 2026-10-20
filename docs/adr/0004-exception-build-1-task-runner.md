# 0004. Exception: BUILD-1 `just` task runner

- **Status:** Proposed — needs the owner's sign-off on the expiry date before it
  counts as a live exception under CONF-1
- **Date:** 2026-10-20
- **Owner:** Tillman Mosley III
- **Rule excepted:** BUILD-1 (`just` as the task runner, with `build`, `test`,
  `lint` and `ci` recipes)
- **Expiry:** 2027-04-20

## Context

BUILD-1 requires that "Every repo MUST go from clone to passing tests with two
documented commands: `nix develop` (or `direnv allow`), then `just test`."

nebu's task runner is a Makefile. The clone-to-tests path works — `direnv allow`
then `make test` — but spells the second command differently from other OBSRVR
repos.

## Reason for the exception

BUILD-1 grandfathers this directly: "Existing Makefiles may stay until the repo
is next reworked, but new task logic goes in the justfile, not in shell
scripts." nebu's Makefile predates the standard and carries non-trivial logic
worth preserving rather than retyping — the `api-check` target's
`diff -u file <(cmd)` process substitution and the explicit `SHELL` resolution
that makes it work in both Nix dev shells and CI.

Converting it now would mean re-verifying every target for no behavioural gain,
in the same change that adds the CI merge gate. Those are better kept separate:
the gate is the higher-value change and should not be entangled with a task-runner
migration.

## Risk

Someone moving between OBSRVR repos types `just test` and gets
`command not found`. Low severity, immediately self-correcting.

## Compensating controls

- `AGENTS.md` documents every target, so both people and agents have the
  command list at the root of the repo.
- `CONTRIBUTING.md` documents the two-command clone-to-tests path explicitly.
- `make ci` exists and runs exactly what the CI merge gate runs, which is the
  substantive requirement behind BUILD-1's recipe list.
- CI invokes the Makefile through the repo's own flake, so developers and CI run
  the same commands against the same toolchain (BUILD-2).

## Review at expiry

Convert to a justfile the next time the build is reworked — most naturally when
`pkg/processor` is extracted into its own module, which is already anticipated
in [STABILITY.md](../STABILITY.md) and will restructure the build anyway. Keep
the Makefile as a thin forwarding shim for one release so no one's muscle memory
breaks mid-migration.
