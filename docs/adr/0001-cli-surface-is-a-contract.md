# 0001. The nebu CLI surface is a contract

- **Status:** Accepted
- **Date:** 2026-10-20
- **Owner:** Tillman Mosley III
- **Affected interfaces:** `nebu` CLI flags and subcommands, machine-readable
  output documents, exit codes
- **Supersedes:** the *Unstable surfaces* entry in
  [STABILITY.md](../STABILITY.md) that declared CLI flags non-contractual

## Context and problem statement

[STABILITY.md](../STABILITY.md) listed `cmd/nebu` flags and subcommands under
*Unstable surfaces*, with the rationale "the CLI is a product UI, not a
contract. Flags can be renamed, subcommands can be reshaped, output formats can
change."

The OBSRVR Engineering Standard v2.0 says the opposite in two places. §3 opens:
"A CLI is a programmable interface to the core (P-2). Its JSON output and exit
codes are public contracts, as binding as an API." §6, under *What counts as a
contract*, lists "CLI flags, `--output json` and NDJSON shapes, exit and error
codes, config files".

Both cannot stand. §1 of the standard settles precedence — "Where one of them
conflicts with this standard, this standard wins inside OBSRVR and the conflict
gets an ADR" — so this is that ADR.

The conflict is not academic. nebu's own documentation already treats the CLI as
a contract: [EXIT_CODES.md](../EXIT_CODES.md) exists precisely so that "shell
scripts, CI systems, agents, and orchestrators can distinguish failures without
parsing stderr", and [PROCESSOR_CONTRACT.md](../PROCESSOR_CONTRACT.md)
specifies a launch and exit contract that flowctl depends on. A document saying
flags may be renamed at will undercuts both.

## Decision

The following are promoted to nebu's stable surface, governed by the deprecation
process in STABILITY.md:

1. **Flag names, their meanings, and subcommand names.**
2. **Machine-readable output documents** — currently `nebu list --json`,
   `nebu describe --json`, `nebu describe --schema`, and the `nebu.status.v1`
   stream — plus error envelopes.
3. **Exit codes**, as documented in EXIT_CODES.md.

**Human-readable output stays explicitly non-contractual.** Tables, headings,
column order, colour, and wording in the default output can be reshaped at any
time. CLI-6 permits this directly: "Human tables are the default and are not a
contract." Scripts must use the machine-readable output.

## Consequences

**Good:**

- The promise matches what consumers already rely on, and what the standard
  requires.
- Flag renames now need a deprecation window instead of a release note, which is
  the behaviour an agent or CI pipeline driving nebu needs (P-7).
- Human output stays free to improve, so the CLI can still be made nicer.

**Bad / costs:**

- CLI ergonomics work gets slower. The gaps in
  [CLI_QUALITY_ROADMAP.md](../CLI_QUALITY_ROADMAP.md) — `--output json`
  replacing `--json`, `--config`, `--timeout`, `--no-color` — now have to land
  as additive changes with deprecation windows for anything they supersede,
  rather than as straight renames.
- Two known non-conformances become visible debt rather than free changes: the
  `--output` flag on `nebu fetch` means *output file* rather than output format,
  which CLI-3 forbids, and `--json` is not the CLI-6 spelling. Both now need
  migration windows.

**Neutral:**

- No code changes. This ADR amends STABILITY.md only; the CLI-6 and CLI-3 fixes
  are separate work.

## Compliance

Enforced by review against STABILITY.md. There is no mechanical check for CLI
drift equivalent to the `.api/` snapshots that guard `pkg/processor` and
`pkg/source`; adding one (a golden snapshot of `--help` output and the JSON
document shapes) is follow-up work and is listed in the
[engineering profile](../engineering-profile.md).
