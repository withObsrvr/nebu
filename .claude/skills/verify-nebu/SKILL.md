---
name: verify-nebu
description: >
  Drive the real nebu CLI — the shipped one, installed from the module proxy
  into a throwaway GOBIN — and capture evidence that a user-facing surface
  works. Use before claiming any nebu command, processor, pipeline, flag, help
  text, or documented example works; after changing cmd/nebu, pkg/processor,
  pkg/source, registry.yaml, or the README quickstart; and whenever a check
  that only read the tree needs to become a check that ran the binary.
user-invocable: true
---

# Verify nebu

nebu's user-facing surface is a **CLI**: `nebu` plus a set of standalone
processor binaries that stream newline-delimited JSON between stdin and
stdout. There is no server, no UI, and nothing to keep alive — each drive is
a short-lived process in its own isolated environment.

`just ci` and `just docs-smoke` check the tree and the locally built binaries.
This skill checks **what a user receives**: `go install …@latest` into an empty
`GOBIN`, then the documented commands run verbatim. That gap is where this
repo's real defects have lived (`AGENTS.md`, "Verify against the shipped
artifact"), so prefer the published mode unless you are specifically
validating an uncommitted change.

Everything runs through the helper, which is executable and invoked directly:

```bash
.claude/skills/verify-nebu/nebu-verify help
```

It keeps three directories apart:

| Directory | Lifetime | Holds |
|---|---|---|
| `${TMPDIR:-/tmp}/nebu-verify/<run-id>/` | deleted by `down` | `bin/` (the installation), `cache/` (registry cache), `work/` (cwd for every step) |
| `.verify-artifacts/<run-id>/` | **survives `down`** | `steps/NNN-<name>.{cmd,out,err,exit}`, plus anything you write under `out/` |
| `.verify-artifacts/fixtures/` | **survives `down`**, shared across runs | ledger XDR fetched once from the public archive |

`.verify-artifacts/` is gitignored. Two runs never collide: pass
`NEBU_VERIFY_RUN=<id>` to address a specific one, otherwise the newest `up`
wins through `.verify-artifacts/current`.

## Launch

```bash
V=.claude/skills/verify-nebu/nebu-verify

# The shipped artifact. Installs go install github.com/withObsrvr/nebu/cmd/nebu@latest
# into an empty GOBIN, then adds processors through `nebu install`.
$V up --published

# A specific release instead of @latest:
$V up --published --version v0.7.11

# Extra processors (default: token-transfer usdc-filter json-file-sink):
$V up --published --processors "token-transfer contract-events json-file-sink"

# A local build. NOT what users get — only for an uncommitted change, and
# the proof must say so.
$V up --local
```

`up` is ready when it prints `up complete`, having installed `nebu`, printed
the version it actually resolved, and confirmed each processor binary exists
on disk. **Ready is not "the command exited 0"** — see the first two gotchas
below. Confirm readiness with `doctor`, never by the absence of an error.

Teardown is `$V down`. There is no daemon to stop; `down` kills only PIDs
this run started (see `bg`), removes the run root, and leaves the evidence and
the fixtures in place.

## Doctor

```bash
$V doctor
```

Read-only. Exits nonzero if this instance is not worth driving. It answers:

- Does `nebu` on `PATH` resolve **inside this run's root**? A `nebu` found
  anywhere else belongs to someone else and must not be driven.
- Does `nebu --version` exit 0 and print a real version, and which one?
- Did `nebu list` reach the external registry with **nothing on stderr**, and
  does its output contain `[community]` entries?
- Is the registry cache populated under the run's `XDG_CACHE_HOME`?
- Is each installed processor in the run root, resolving to the run root copy,
  and does `--describe-json` name itself, exit 0, and carry a `schema.id`
  (sinks excepted, `docs/PROCESSOR_CONTRACT.md:113`)?
- Are `jq` and `duckdb` on `PATH`? (Both come from the dev shell.)
- Are ledger fixtures present? Missing fixtures are a `WARN`, not a failure.

Run it first whenever anything looks off, and again after any step that
reinstalls a binary.

## Drive

Fetch the ledger fixture once, then drive from the file. The default RPC
endpoint refuses the third `getLedgers` call in a burst with 429
(`scripts/docs-smoke.sh`), so RPC is for assertions that are *about* RPC.

```bash
$V fixture                       # defaults to 60200000..60200004
$V fixture 62080000 62080009     # any range; idempotent, skipped if present
```

`fixture` prints progress to stderr and the absolute fixture path to stdout,
so it composes. Name the two paths every recipe needs up front:

```bash
F="$($V fixture)"
E="$PWD/.verify-artifacts/$(cat .verify-artifacts/current)"; mkdir -p "$E/out"
```

Three ways to drive. `run` and `sh` capture `.cmd`, `.out`, `.err`, and
`.exit`; `bg` captures `.cmd`, `.out`, and `.err` and returns the PID instead
of an exit code:

```bash
# One command, no shell. Use for anything that does not need a pipe.
$V run quickstart -- token-transfer --start-ledger 60200000 --end-ledger 60200001

# A pipeline, through bash -o pipefail -c. Use for pipes and redirects.
$V sh usdc-pipeline -- "token-transfer --quiet < '$F' | usdc-filter | json-file-sink --out '$E/out/usdc.jsonl'"

# A long-running command (--follow). Records the PID for teardown.
PID=$($V bg follow-tip -- nebu fetch --last 1 --follow --output /dev/null)
```

`run` and `sh` exit with the command's own exit code, so a step that
*should* fail (exit 2 on bad usage, 3 on an unknown processor, 4 on an
unreachable source, 8 on incomplete output — `docs/EXIT_CODES.md`) makes the
helper exit nonzero too. That is expected: read the `.exit` file, do not treat
it as a harness error.

Use the stable handles the CLI itself documents, not positions or guesses:
subcommand names, long flags from `--help`, `_schema` ids, `jq` field paths
from `--describe-json`, exit codes from `docs/EXIT_CODES.md`. Confirm a flag
exists on the **installed** binary (`<processor> --help`) before using it;
registry processors ship on their own cadence and the version you installed
may be ahead of or behind the docs.

## Evidence

Everything `run`/`sh`/`bg` touches is already under
`.verify-artifacts/<run-id>/steps/`. Write sink output and derived files to
`.verify-artifacts/<run-id>/out/` so they survive teardown:

```bash
E="$PWD/.verify-artifacts/$(cat .verify-artifacts/current)"
mkdir -p "$E/out"
```

Proof standards for this repo:

- **Exercise the real user path.** The documented command, verbatim, against
  the published binary. Not `go run`, not a Go test, not `bin/` from the
  working tree unless the claim is specifically about an uncommitted change.
- **Capture the action and the resulting state.** The command and its exit
  code *and* the data it produced — not a line count alone, and not a "ran
  without error".
- **Assert against independently known truth, never the binary's own output.**
  Count rows, check `_schema` and `_nebu_version` on every record, check a
  filter actually removed something, check a ledger range has no holes.
- **Verify side effects separately.** A sink reports nothing on stdout by
  contract (`docs/PROCESSOR_CONTRACT.md:66`), so read the file, the table, or
  the subject it wrote and assert on that.
- **Treat silence as failure, because every real defect here was
  silence-shaped** — empty output, a `null`, a `.000`, a hang. Never accept a
  passing step whose stdout is zero bytes unless emptiness is the claim.
- **Mock nothing that the pipeline crosses.** Ledgers come from the real
  public archive or real RPC. The one place a stand-in is right is a sink
  whose backend is already an isolated boundary (the NATS container in
  `scripts/docs-smoke.sh`), and a `--describe-json` probe, which by contract
  starts no processing.
- **Observe what a safe mode actually skips.** `--quiet` only silences
  stderr; it does not reduce fetching. `nebu fetch` *infers* quiet when stdout
  is not a terminal, which is why a resolved `--last` range appears only on
  stderr — capture stderr, do not assume silence means nothing happened.
- **Name the artifact you drove.** Record `nebu --version` and each
  processor's `--describe-json` version alongside the result, and say
  "published v0.7.11" or "local build, dirty tree" in the proof.

A worked proof, start to finish:

```bash
V=.claude/skills/verify-nebu/nebu-verify
$V up --published
$V doctor
F="$($V fixture)"
E="$PWD/.verify-artifacts/$(cat .verify-artifacts/current)"; mkdir -p "$E/out"
$V sh all-events  -- "token-transfer --quiet < '$F' | json-file-sink --out '$E/out/all.jsonl'"
$V sh usdc-events -- "token-transfer --quiet < '$F' | usdc-filter | json-file-sink --out '$E/out/usdc.jsonl'"
$V sh assert -- "jq -se 'length > 0 and all(.[]; ._schema == \"nebu.token_transfer.v1\" and .transfer.assetCode == \"USDC\")' '$E/out/usdc.jsonl' \
  && echo usdc=\$(wc -l < '$E/out/usdc.jsonl') all=\$(wc -l < '$E/out/all.jsonl')"
$V down
```

## Cleanup

```bash
$V down
```

`down` kills the PIDs recorded by `bg` — **only those**; never match nebu or a
processor by process name, because the developer's own pipelines look
identical — then removes the run root. It keeps
`.verify-artifacts/<run-id>/` and `.verify-artifacts/fixtures/` on purpose.
After tearing down, confirm the evidence is still there before reporting:

```bash
ls .verify-artifacts/<run-id>/steps .verify-artifacts/<run-id>/out
```

Run `down` after a failed attempt too, so a broken run does not leave a stale
`current` pointer or a half-installed `bin/` for the next one to inherit.

## Feature map

[`features/README.md`](./features/README.md) is the maintained list of
user-facing features, with one file each: how a user reaches it, how to drive
it here, and what end state proves it. **A proof that drives one convenient
entry point is incomplete when the map lists others** — report the entry
points you skipped rather than reporting the feature verified.

## Gotchas

- **`nebu install` run inside this checkout installs local source, not the
  shipped artifact.** It prefers a local build whenever the registry entry's
  `Location.Path` exists relative to the current directory
  (`cmd/nebu/install.go:99-104`). Every step therefore runs with cwd set to
  the run root, never the repo. If you drive `nebu install` by hand, `cd`
  out of the repo first or you will verify your own tree and call it published.
- **`nebu install` does not pass `--path` to `go install`.** For a community
  processor it runs `go install <pkg>@latest` with no `GOBIN` override, then
  prints `Installed: <--path>/<name>` regardless
  (`cmd/nebu/install.go:158-186`). `GOBIN` is what actually decides. Always
  assert the binary exists at the path `command -v` resolves — never trust the
  `Installed:` line.
- **`nebu list` exits 0 when the external registry is unreachable.** It prints
  a warning to stderr and returns the embedded catalog only
  (`pkg/registry/registry.go:236-239`). Exit 0 is not readiness; `doctor`
  requires an empty stderr plus `[community]` entries.
- **`| head` inside `$V sh` exits 141.** The helper runs pipelines with
  `-o pipefail`, so `head` closing the pipe surfaces SIGPIPE. Bound the data
  instead — a smaller fixture range, `--last N`, `jq -n 'limit(...)'` — or
  append `|| true` and state in the proof why the exit code was discarded.
- **`@latest` is not a date.** Go resolves pseudo-versions for untagged
  modules against the local module cache, so a warm cache can keep installing
  stale registry processors with no error at all (see the `registry_version`
  comment in `justfile`). Record what you actually got from
  `--describe-json`, and use `scripts/check-registry-pin.py` as the
  independent check on the pin.
- **Published registry processors can be ahead of the repo's pinned ones.**
  `just build-processors` installs the pin; this skill installs `@latest`. A
  flag or field present in one may be missing from the other, so a result from
  `bin/` does not transfer to the published artifact or back.
- **The archive needs `AWS_EC2_METADATA_DISABLED=true` off EC2**, or every
  process waits out the instance metadata service (~10s) before the anonymous
  fallback a public bucket needs (`docs/ARCHIVE_MODE.md`). The helper exports
  it; a hand-run command outside the helper will not have it.
- **DuckDB 2.0.0 behaves differently for agents.** With `AI_AGENT` or
  `CLAUDECODE` set and stdout not a terminal it collapses `read_json`'s
  inferred schema to a single `json` column, turning a documented
  `SELECT transfer …` into a binder error. The helper exports
  `DUCKDB_AGENT_MODE=0` so a check means the same thing whoever runs it.
- **RPC is rate-limited in bursts.** Measured at two `getLedgers` calls, then
  429s, recovering about a second later (`scripts/docs-smoke.sh`). Use the
  archive fixture for anything that is not an assertion about RPC, and space
  the RPC steps that remain.
- **A ledger state that does not exist yet cannot be run.** Protocol-version
  work ahead of the network can only be reasoned about and constructed; see
  `docs/protocol-30-impact-review.md`. Say "unverified — no such ledger
  exists" rather than producing a proof.
