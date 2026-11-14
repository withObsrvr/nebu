# Changelog

All notable changes to nebu are documented here. For full release artifacts, see [GitHub Releases](https://github.com/withObsrvr/nebu/releases).

This changelog follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Changes are grouped under Added, Changed, Deprecated, Removed, Fixed and Security. Entries before v0.6.15 predate that convention and are left as written.

## Unreleased

## v0.7.5

### Fixed

- **`RunGenericOriginCLI` hung after processing every ledger, and had no `--describe-json`.** Both faults are the kind that only exist in code nothing runs: no processor in the registry uses this helper, and the one that was about to — `trade-extractor`, migrating off a hand-rolled cobra command — hit them immediately.
  - **The hang.** The helper closed the origin in a `defer`, so the writer goroutine ranging over `Out()` waited for a close that could not happen until `RunE` returned, and `RunE` was blocked waiting for the writer. A processor on this helper read its ledgers, wrote every event, and then waited forever: over a three-ledger range it emitted all 341 trades and never exited. `RunProtoOriginCLI` and `RunOriginCLI` always closed the origin explicitly before waiting; this one was the odd one out, and now matches them.
  - **The missing flag.** `describe.go` calls `--describe-json` "the canonical flag every CLI helper registers", which was true of sinks, transforms and proto origins and false of `RunGenericOriginCLI` and `RunOriginCLI`. A processor built on either answered `Error: unknown flag: --describe-json`. `docs/STABILITY.md` lists the describe protocol as a stable surface and `nebu install` shells out to it, so a processor without it cannot be installed. Neither helper has an output schema to publish — one emits any Go struct, the other a simplified map rather than the protobuf it reads — so both report schema id, flags and examples, the same shape a transform over arbitrary JSON reports.
  - Three tests. Two run the real helper by re-execing the test binary as a processor, since a `Run*CLI` builds its own cobra command and calls `os.Exit`: one asserts it exits on an empty ledger stream (the shutdown path is where the deadlock lived, so it needs no input to reproduce — with the `defer` restored the test times out at 20s), the other that `--describe-json` prints a valid envelope to stdout, writes nothing to stderr and exits 0. The third parses the package and requires every `Run*CLI` to both register the flag and call `emitDescribeIfRequested` before cobra validates required flags, which is what makes the describe path work on a processor with a required flag.
- **Nothing told anyone the pinned registry version had gone stale.** `scripts/check-registry-pin.py` asks the Go module proxy for the newest published version of each processor `build-processors` installs, and fails when the pin is behind any of them. Pinning is the right call — `@latest` is not a contract, and it had already installed pre-Protocol-30 processors from a warm module cache — but its failure mode is quieter: the pin sat two weeks and four merged registry PRs behind while every documentation check passed against processors built before `meta.closedAt` existed. The checks were right; they were reading an old product.
  - It reads the processor list out of the `build-processors` recipe instead of keeping its own copy, so adding a processor there cannot leave the check behind. It queries the proxy's `/@latest` endpoint directly — these modules carry no tags, and going to the proxy also sidesteps the warm-cache resolution that made `@latest` unreliable in the first place. A pin *ahead* of the proxy passes, since that is a commit the proxy has not indexed yet; only a pin behind fails. An unreachable proxy says it learned nothing rather than passing quietly.
  - It lives in `docs-smoke`, not `just ci`: a stale pin should not block an unrelated PR, and `docs-smoke` is where the documentation-versus-output checks already run. Negative-tested against the pin this repo carried yesterday, which it reports along with the exact version to repin to.

- **The documented samples were missing a field the product emits.** They move to `_nebu_version: v0.7.4`, now that the registry has rebuilt against it and published processors actually report that — the field names the nebu a processor was built against, so moving it earlier would have documented a version no binary emitted. Bumping it meant repinning `registry_version` in the justfile, which had sat at an October pseudo-version since the `@latest` fix, and that is what exposed the real drift: `meta.closedAt` has been emitted since the Protocol 30 work and none of the four samples marked `complete` showed it. `docs-smoke` had been comparing them against processors built before the field existed, so it passed. All four now carry `closedAt`, each consistent with its own `closedAtUnix`.

## v0.7.4

### Fixed

- **Every origin processor's help advertised a jq filter that selects nothing.** `pkg/processor/cli/help.go` generates each processor's EXAMPLES from its name, so one wrong line there ships in every processor built against the release. The line was `jq 'select(.type == "transfer")'` — the same filter that was wrong in `nebu --help`, and wrong here for a second reason: no origin emits a top-level `type` field, and for `contract-events` or `ttl-tracker` a transfer filter is not even the right idea. A generator that knows only the processor's name cannot know its field names, so the examples now show how to look them up (`--last 1 -q | head -1 | jq 'keys'`) and how to read the one field every nebu event does carry (`jq -r '._schema' | sort | uniq -c`). Both verified against real output.
  - The sink examples had the same failure twice over: `jq 'select(.amount > "1000000")'` reads `.amount`, but token-transfer puts it at `.transfer.amount`, and amounts are JSON strings, so even at the right path `>` compares them lexicographically. The replacement guards on the event kind before converting — a ledger holds fees, mints and burns beside transfers, and an unguarded `tonumber` errors once per non-transfer event — and it is written only for the origin whose shape this package knows; any other origin gets a passthrough rather than a guess.
  - The transform and sink examples no longer contain a literal `...`, which was not a command a reader could run, and every generated example now leads with `--last` instead of a hardcoded ledger sequence.
  - Two checks, because the old unit test only covered the one flag that had already gone wrong. `help_test.go` is table-driven over all three generators and rejects a `select(` on any top-level field except `_schema`, a `tonumber` without a guard, a literal `...`, and an example invoking the processor with `--follow`; it fails on the exact line that shipped. `check-help-examples.py --self` reads a processor's own EXAMPLES and its own Flags block and reports any flag it advertises but does not define — the flags have to come from the Flags block, since scraping the whole help text would let an example's own flags count as available and pass by construction. Wired into `docs-smoke` across seven processors.

## v0.7.3

### Added

- **`--last N`** on `nebu fetch` and every origin processor (`NEBU_LAST`): use the most recent N ledgers the source holds instead of an explicit range. Until now the first thing a newcomer had to do was go find a ledger sequence number, which is the one piece of friction standing between installing nebu and getting data out of it.
  - **It means something different per source, so the resolved range is always reported.** On RPC it counts back from the network tip; in archive mode from the newest *exported* ledger, which lags the network. `Resolved --last 100 to ledgers 65457987-65458086 (source reports 65458023-65458086)` goes to stderr so a script or a UI can see what "the last 100" meant.
  - **It never returns fewer ledgers than asked.** The full range is requested, so a source that cannot serve it fails the run rather than handing back a short dataset that looks complete — the same principle as exit code 8, one layer earlier. Asking for more ledgers than the chain has is exit 3, not a clamp.
  - **The retention floor an endpoint reports is a note, not a gate.** `archive-rpc.lightsail.network` — nebu's default — reports a 64-ledger window and serves ledger 60200000 without complaint, 5.2 million ledgers below its own reported floor. Refusing on that basis would have rejected `--last 1000` on the default endpoint, so a request below the reported floor warns and proceeds, and the stream decides.
  - Mutually exclusive with an explicit range, `--start-ledger`, stdin input, and `--checkpoint` — all usage errors rather than silent precedence. The checkpoint case is structural: a checkpoint resumes a fixed range and `--last` resolves a different one as the chain advances, so combining them could only ever work if the tip had not moved.
  - `pkg/source` gains `LedgerWindow`, `WindowedSource` and `ResolveLast`. `WindowedSource` is a separate interface rather than a method on `LedgerSource`, because adding a method to the frozen interface would break every external implementation; callers type-assert for it. The committed snapshot diff is purely additive and `LedgerSource` is unchanged.

### Changed

- **DuckDB pinned to 2.0.0** in the dev shell, up from 1.5.4 (nixpkgs itself still carries 1.4.1, so the overlay stays). Two things had to be handled rather than assumed:
  - The nixpkgs derivation runs DuckDB's own suite with an explicit list of tests that cannot pass in its sandbox, and 2.0.0 adds loadable-extension tests that are not on it — 25 of 7832 cases fail, every one under `test/extension/`. The overlay sets `doInstallCheck = false`, which is the attribute that actually gates them; it also feeds `BUILD_UNITTESTS`, so they are no longer compiled either. `doCheck` is inert here and does not even change the derivation hash.
  - **DuckDB 2.0.0 enables an "agent mode" when `AI_AGENT` or `CLAUDECODE` is set and stdout is not a terminal.** Everything it documents is presentational, but it also collapses `read_json`'s inferred schema to a single `json` column — turning `SELECT contractId ...` into a binder error on data that plainly contains the column. `DUCKDB_AGENT_MODE=0` is pinned in both the dev shell and `docs-smoke` so the checks mean the same thing whether a person or an agent runs them, and the cookbook documents the hazard for anyone running a recipe by hand. DuckDB 1.x ignores the variable, so both versions pass.
  - `docs-smoke`'s SQL used double-quoted paths like `read_json_auto("/dev/stdin")`, which 2.0.0 deprecates — double quotes are identifiers, single quotes are string literals. Rewritten; the cookbook already used single quotes.

- **The DuckDB cookbook gained coverage verification, status-stream queries, and a section on the three things that silently produce wrong answers.** "Getting the numbers right" covers exact amount arithmetic; that 39% of `fee` events in a sampled range came from *failed* transactions, while `transfer`, `mint` and `burn` are only derived from successful operations, so it is easy not to know you need `meta.inSuccessfulTx`; and that `closedAtUnix` is a whole-second string which stops being a deterministic sort key once CAP-88 lets two ledgers close in the same second. "Verifying coverage" finds missing ledger sequences in a dataset at rest, stating the two limits that make it a check rather than a false reassurance. "Querying the status stream" reads `nebu.status.v1` with DuckDB and correlates every stage of one pipeline by `run_id`. Every snippet was executed against real pubnet data, and four new `docs-smoke` assertions check values rather than row counts.

- **Documentation and the site catch up with v0.7.2.** `--on-ledger-error`, coverage reporting and exit code 8 are documented in the README and the man page; `pkg/processor/ledgerread` is documented for processor authors in BUILDING_PROCESSORS.md, including which of `Transactions` and `AllTransactions` to reach for and why; `man.html` gained the eleven `fetch` flags it had never listed, including the archive layout overrides; and `changelog.html` gained the five releases it was behind.
- **The documented-sample check covers every sample, not just the landing page.** `scripts/check-sample-event.py` was written after the landing page's `SAMPLE_EVENT` drifted from reality, and checking only that page let the same drift happen everywhere else: by v0.7.2 the README and `docs/PIPELINE.md` were each missing `network`, `meta.operationIndex` and `meta.inSuccessfulTx`, and `docs/quickstart.html` — the page a new reader lands on — was missing seven fields including the `transactionIndex`/`operationIndex` pair that is the whole argument for nebu giving you a resumable cursor. The landing page was the only sample that was right, because it was the only one checked. All four are now checked, in both directions for samples that claim to be complete.
  - `docs/quickstart.html` previously used a literal `...` inside its JSON, so it could never be parsed or verified. It is now a real, complete event.

### Fixed

- **The packaged skill described a nebu that does not exist in three places.** `skills/nebu/references/reference.md` listed thirteen "built-in processors", but nebu's embedded `registry.yaml` holds exactly two — `transaction-stats` and `ledger-change-stats`. The other eleven, `token-transfer` included, come from the external registry and `nebu list` tags them `[community]`, which the skill's own rules say not to promise before `nebu list` shows them. The table now carries a source column read off `nebu list`. Its Docker example ran `docker run withobsrvr/nebu:latest token-transfer …`, but `Dockerfile.goreleaser` copies one binary onto distroless and processors ship separately, so a processor name is not a command that image can run; it now shows the pipe-on-the-host form. And the development install said `make install`, which `AGENTS.md` deprecates in favour of `just`.
- The skill gained `--last` — including the fact that published processors reject it until the registry rebuilds against this release, so an agent reaches recent ledgers through `nebu fetch --last N | <origin>` — and `NEBU_LAST` joined the documented environment variables.

- **A piped `nebu fetch --last N` never said which ledgers it chose.** `fetch` infers quiet mode when stdout is not a terminal, so a stderr merged into stdout cannot corrupt the binary XDR stream — and that inference also silenced the resolved range. `nebu fetch --last 100 | token-transfer`, the form the quickstart documents, reported nothing about which 100 ledgers it had picked, and nothing downstream can recover that from the stream. The origin processors print it unconditionally; fetch now matches them. An explicit `--quiet` still silences everything, because asking for quiet and having it inferred are not the same instruction — `logResolved` tells them apart, with the gating under test.
- **The quickstart in `nebu --help` now reaches recent ledgers through `nebu fetch --last`,** so the first command a reader runs no longer requires looking a ledger sequence up. It deliberately does *not* use `token-transfer --last`: the help ships inside the nebu binary, so `nebu fetch --last` is always true of the nebu in hand, while a processor's `--last` only exists once the registry rebuilds it against a nebu release carrying the flag — the published processors are built on v0.7.2 and reject it. Verified end to end against the installed v0.7.2 processors: `nebu fetch --last 10 | token-transfer` yields 342 events, and the four-stage pipeline the help documents (`fetch | token-transfer | usdc-filter | amount-filter --min 1000000 | json-file-sink`) writes 125 rows.
  - `check-help-examples.py` attributes flags per pipeline stage rather than per line, since `nebu fetch --last 100 | token-transfer` puts a fetch flag on a processor's line, and it now checks the transform and sink stages too. Negative-tested: it reports `--start`/`--end` on token-transfer and `--minimum` on a later `amount-filter` stage.

- **Two of the four quickstart commands in `nebu --help` did not work.** The root help is the first thing anyone runs, and nothing checked it. `jq 'select(.type == "transfer")'` selected **nothing** — token-transfer events have no `.type` field, the event kind is a protobuf oneof, and the correct filter returns 1,044 events over the same three ledgers where the documented one returns 0. And `token-transfer --start N --end M` exits with `unknown flag: --start`, because the processor exposes `--start-ledger`/`--end-ledger` (an abbreviation `docs/CLI_QUALITY_ROADMAP.md` had already flagged). Found by installing the released binary from scratch and running the printed quickstart verbatim.
  - `scripts/check-help-examples.py` now verifies that every flag the root help uses on a processor is one that processor actually accepts, wired into `docs-smoke`. Negative-tested against a binary built from the old help text: it names both `--start` and `--end`. A second assertion runs the documented jq filter and requires it to select something, since a filter that silently matches nothing is the failure that was there.
- **`nebu install` blamed the wrong things when the module proxy was cold.** The Go proxy returns a 404 with `fetch timed out` the first time anyone asks for a module it has not cached, and the registry's processor modules are large enough for that to happen — `nebu install json-file-sink` failed that way on a clean machine and succeeded on retry. The suggestion offered write permissions, module path and network access, none of which was the cause. It now says the proxy has not cached the processor yet, that a retry usually works, and offers `GOPROXY=direct`.
- **The DuckDB cookbook's volume queries summed stroops into a `DOUBLE`.** Amounts are integers that reach into `int64`, and `DOUBLE` holds integers exactly only to 2^53 — a single ordinary XLM transfer of 4.06 × 10^16 stroops already exceeds that. Over 51 ledgers of pubnet traffic the documented query and an exact `::HUGEINT` sum disagree by 648 stroops, and the gap grows with the range without ever announcing itself. Both volume queries now cast to `HUGEINT`. The existing smoke assertions did not catch it because they all count rows: they check that a query runs, not that its answer is right.

## v0.7.2

### Added

- **`ledgerread.AllTransactions`** reads a ledger's transactions and reports whether it reached the end. `Transactions` skips an unreadable transaction and carries on, which is right when each transaction is emitted on its own and wrong when they become an index something later consults by key: the skip is invisible at lookup time, and a missing key usually resolves to a zero value or an explicit default, so a partial read produces confidently mislabelled output rather than merely short output. A transaction-hash to success map is the shape that gets this wrong, and there is one in the registry. `Transactions` now says in its own documentation that it is not for that shape.

## v0.7.1

### Added

- **Every `nebu.status.v1` record carries a `run_id`**, and a `component_id` when something upstream knows one. `app` names the program; neither named the run, so a consumer following several streams could not tell two concurrent runs of the same processor apart, and could not tell that the stages of `nebu fetch | token-transfer | postgres-sink` belonged to one pipeline — each emitted a well-formed, complete, entirely unrelatable stream. Closes the one CLI-12 clause [ADR 0003](docs/adr/0003-exception-cli-12-operation-lifecycle.md) recorded as a plain gap rather than an exception; decided in [ADR 0005](docs/adr/0005-status-stream-carries-the-run-id.md).
  - **Adopted from the orchestrator, minted only when there is none.** flowctl already mints a run id per pipeline run and exports it to every child it starts, so `FLOWCTL_RUN_ID` and `FLOWCTL_COMPONENT_ID` are read first and nebu's stream lines up with flowctl's record of the same run. `NEBU_RUN_ID` and `NEBU_COMPONENT_ID` serve callers that are not flowctl — a UI composing its own pipeline sets the run id once and labels each stage. Only a genuinely standalone run mints, and it mints the same UUIDv4 shape, so a consumer never has to care which case it got.
  - Additive: `nebu.status.v1` requires consumers to ignore unknown fields, so the schema version is unchanged. This does not make a run addressable from outside the process executing it; `inspect <id>` and `cancel <id>` remain excepted by ADR 0003.

### Fixed

- **Cancelling `nebu fetch` early left its status stream claiming the run was still live.** The signal handler force-exits with `os.Exit(130)` two seconds after cancelling, and `os.Exit` runs no deferred code — so the terminal record written on the normal finish path never happened. A fetch interrupted before its first RPC response exited 130 with `working`/`running` as the last word on its `nebu.status.v1` stream, and anything following that stream showed the run as live forever. Reproduced across the window: cancelling at 0.3s, 1s and 2s produced no terminal record; 3s and later did. Cancellation is now reported the moment the signal arrives, before any force-exit timer can fire, and the record is terminal: every later write is dropped, including progress the fetch loop was already about to emit between its context check and its next status write. A consumer can rely on reading nothing after it. The origin, transform and sink CLIs already reported cancellation immediately; `nebu fetch` was the outlier.

## v0.7.0

### Added

- **`--on-ledger-error best-effort|strict`** (`NEBU_ON_LEDGER_ERROR`) on every origin processor, and `runtime.Policy` behind it. "Streams never throw" settles who *reports* a problem; it never settled whether a run that hit problems produced usable output. Three bad ledgers in ten million is noise for an exploratory query and disqualifying for a reconciliation dataset, and the processor author cannot know which one the operator is doing — so that decision now belongs to the invocation. `best-effort` is the default and is exactly the previous behavior.
  - `strict` stops at the first ledger that cannot be processed completely and exits **8**, a new code meaning "ran, but the output is incomplete". It is deliberately distinct from `4` (source unavailable): `4` is worth retrying, `8` will fail the same way, and a consumer that cannot tell them apart either retries forever or accepts holes.
  - `runtime.PipelineSummary` now carries `Coverage` — ledgers processed, how many were incomplete, and the first such sequence. A warning count cannot answer "is this dataset whole?"; coverage can. Incomplete coverage is reported on stderr at the end of a run regardless of `--quiet`.
- **`pkg/processor/ledgerread`** iterates a ledger's transactions (`Transactions`) and changes (`Changes`), reporting read failures through the context's `Reporter`. Every origin needs this loop and it has one failure mode that is easy to get wrong: `if err != nil { break }` treats a mid-ledger decode failure as end-of-ledger, truncating the ledger and emitting no warning, so the run looks clean. Kept in a subpackage so importing the processor interfaces does not pull in the SDK's ingest and cloud-storage dependencies.
- **`nebu fetch --mode archive` accepts `--ledgers-per-file` and `--files-per-partition`** (`NEBU_LEDGERS_PER_FILE`, `NEBU_FILES_PER_PARTITION`) for buckets with no manifest, or to have a layout you supply checked against the manifest instead of assumed.

### Changed

- **Archive mode takes the datastore layout from the bucket.** `nebu fetch --mode archive` asserted a hardcoded 1 ledger per file and 64,000 files per partition against every bucket, so any other export failed as a config mismatch with no flag available to correct it. The layout now comes from the manifest the exporter published, which is where it has always been recorded.
- **Archive buffer tuning comes from the SDK.** `--buffer-size` and `--num-workers` default to `0`, meaning `ingest.DefaultBufferedStorageBackendConfig` picks them from the datastore layout. nebu's fixed `BufferSize: 100` throttled bounded backfills on the one-ledger-per-file layout, where the SDK asks for 10,000 so workers see the whole queue up front; the buffer is capped to the range size, so that is an upper bound, not an allocation. Explicit values still win.
- **Archive mode passes the network passphrase to the datastore**, so the manifest check catches a pubnet-configured fetch pointed at a testnet export instead of returning ledgers from the wrong network.
- **`pkg/source/rpc` and `pkg/source/storage` stream through the SDK's `ledgerbackend.LedgerStream`** instead of each carrying its own `PrepareRange`/`GetLedger` loop. For storage this is a correctness change, not a cleanup: `NewBufferedStorageStream` reads the datastore manifest, which the hand-rolled loop never did, so that path took the schema from its caller and never checked it. The `LedgerSource` interface is unchanged and the committed API snapshots are unchanged; `Close` is now a no-op because the stream owns its backend's lifetime, and the storage constructor no longer reaches the bucket (an unreachable bucket is reported when streaming starts).

### Fixed

- **Processors reading from stdin ran with no error reporter attached.** `processor.ReporterFrom` fell through to the package default, which counts nothing and calls `os.Exit` on a fatal report — so warnings were invisible and unhookable in exactly the mode Unix composition uses (`nebu fetch ... | processor`). All three origin CLI variants now attach a reporter on the stdin and file paths.
- **The three origin CLI variants each had their own copy of the stdin decode loop, and the copies had drifted.** Two reported decode and file-open failures with a bare `fmt.Errorf`, losing both the actionable message and the exit-code classification the third produced. There is now one loop.

### Security

- **Upgraded `github.com/klauspost/compress` v1.18.5 → v1.20.2** in nebu and the hooks example, clearing GO-2026-6865 (panic on two-block frames), the one vulnerability `govulncheck` reported as reachable from nebu's own code. It arrives indirectly through the SDK's `xdr` package, which uses zstd. Both modules now report no reachable vulnerabilities.
- **Pinned the Go toolchain to 1.26.11** for GO-2026-6859, a `net/http` vulnerability in the standard library reachable from `pkg/registry`, `pkg/programstatus` and `pkg/errors`. Binaries built from this release are not affected; anyone building nebu themselves gets the fixed toolchain from the `toolchain` directive.

- `just build-processors` installed the registry's processors with `@latest`, which is not a version. Go resolves `@latest` against the pseudo-version list in the local module cache, so a machine with a warm cache kept installing **pre-Protocol-30 processors** after the registry had been upgraded — silently, with no error, producing binaries that cannot decode a Protocol 30 ledger. The registry version is now pinned in one place in the justfile and echoed on every build, so the command is reproducible (P-6, BUILD-2) and you can see which registry build you have. `@latest` was also plainly against P-4: "'Latest' is never a contract."
- The sample-event check no longer excuses the `network` field. The registry now builds against nebu v0.6.17 and emits it, so it is enforced in both directions like any other field.

## v0.6.17

### Added

- **Event envelopes carry a `network` field** (DATA-6), so records from different networks are no longer indistinguishable in one stream or table. The value is `pubnet`, `testnet`, or an unrecognised passphrase verbatim — never a placeholder, so a standalone network is identified rather than mislabelled as a known one.
  - **The label is derived from the ledger, not from `--network`.** A transaction's hash commits to the network passphrase, and the ledger records that hash, so hashing any envelope under a candidate passphrase and finding it among the stored hashes identifies the network conclusively — no signature verification, no network access. Derived once per run and cached.
  - This matters because a processor reading from stdin cannot know where its bytes came from. Labelling from the flag would have made `nebu fetch --network testnet 1 2 | token-transfer` report `pubnet`, which is worse than no label: confidently wrong, and durable once written to a sink.
  - When the derived network disagrees with `--network`, a warning is reported once naming both. The mismatch means the processor is decoding with the wrong passphrase, so anything derived from it — Stellar Asset Contract IDs above all — is wrong. Refusing to start on mismatch is BUILD-7, which is breaking and tracked separately.
  - Appears in output from processors once they are rebuilt against this release.
- **`processor.LedgerCloseTimeMs(ledger)`** returns a ledger's close time in milliseconds since the Unix epoch. Added to the stable surface, because every origin processor needs it and the obvious implementation is wrong. Protocol 30's CAP-88 puts the millisecond value in the `StellarValueExt` union, and **two** arms carry it — `SIGNED_MS` (3) via `SignedMsValue` and `EMPTY_TX_SET_MS` (4) via `ProposedMsValue`. Code that reads only `GetSignedMsValue()`, the natural first guess, silently falls back to second resolution on every *empty* Protocol 30 ledger — a case no fixture taken from a busy ledger exercises. Pre-Protocol-30 ledgers (arms 0-2) have their whole-second close time promoted, losing nothing, since those close times are seconds apart. Covered by a table test over all five discriminants.
  - Prefer it over the SDK's `LedgerCloseMeta.ClosedAt()` and `LedgerCloseTime()` whenever a close time is emitted, stored, ordered, or used to bucket events: those return whole seconds, and from Protocol 30 onward consecutive ledgers can close within the same second, so second-resolution windowing produces wrong answers rather than errors.
  - For the RFC 3339 UTC form that OBSRVR data contracts require (DATA-3): `time.UnixMilli(processor.LedgerCloseTimeMs(ledger)).UTC().Format(time.RFC3339Nano)`.

### Changed

- **Protocol 30 ("Bell"): upgraded `github.com/stellar/go-stellar-sdk` to v0.8.0** in nebu (from v0.7.3) and the hooks example (from v0.7.1). This is required, not optional. CAP-88 adds two arms to the `StellarValueExt` union — `STELLAR_VALUE_SIGNED_MS` (3) and `STELLAR_VALUE_EMPTY_TX_SET_MS` (4) — and an SDK that does not know those discriminants fails to decode any Protocol 30 ledger. Testnet votes 2026-11-03, pubnet 2026-11-23.
  - No nebu source changes were needed, and the committed stable-API snapshots are unchanged.
  - `nebu fetch` never decoded ledgers (it streams raw bytes and derives sequences arithmetically), so raw archival was never exposed to this; the five `xdr.Unmarshal` decode paths were.
  - Verified on mainnet ledgers 60200000-60200050: `fetch` output is byte-identical to the previous release, and a 39,652-event pipeline through a processor still pinned to v0.7.3 is unchanged.
  - **Not yet addressed:** CAP-88 gives close times millisecond resolution, so consecutive ledgers can close within the same whole second. Any consumer using whole-second close times for ordering or windowing becomes ambiguous — including the `closed_at_unix` field defined in the processor registry's protos, which `time-window` and `aggregator` filter on. That is a registry-side schema decision, tracked separately. The SDK's own `LedgerCloseMeta.ClosedAt()` also truncates to seconds; the millisecond value lives in the new union arms.
  - **Not yet addressed:** CAP-84 muxed contract addresses add an `SCAddress` variant and the `W` strkey prefix. nebu passes SDK-produced address strings through unmodified, but processors that parse, filter, or re-encode addresses need review.

### Fixed

- The engineering profile claimed `v0.6.12` and `v0.6.13` were signed tags and that signing had regressed. Neither is true: those are lightweight tags with no tag object to sign, and no nebu tag has ever been signed. The "Verified" badges belong to the commits, which GitHub signs with its own key on web-UI merges — not the maintainer's key, and not covering tags. The backlog item now says what actually has to be set up.
- `RunProtoOriginCLI` emitted invalid JSON for an event with an empty payload: the envelope splice left a trailing comma, producing `{"_schema":"…","_nebu_version":"…",}`. Interpolated values are now JSON-escaped too, so a schema ID or network label containing a quote cannot break a line.
- The `SAMPLE_EVENT` block on the landing page advertised four fewer fields than `token-transfer` actually emits — `meta.transactionIndex`, `meta.operationIndex`, `meta.inSuccessfulTx` and `transfer.assetIssuer`. The first two are what a reader needs in order to believe nebu gives them a resumable cursor. `just docs-smoke` now compares the documented sample against a real event on three axes — field names in both directions, JSON types, and the values that are contractual rather than illustrative (`_schema` must match exactly, `_nebu_version` must be version-shaped) — so it cannot drift again. The type check is the one that matters most: `closedAtUnix` is a *string* of digits, and documenting it as a number would send anyone writing a consumer down the wrong path invisibly.
- The dev shell's greeting, the integration suite's error message, and the `Makefile` shim's own comment still referred to `make` targets or to an unrelated ADR after the justfile conversion. The CI workflows also pinned `just` 1.51.0 while this repo's `flake.lock` resolves to 1.43.1 — the opposite of the BUILD-2 alignment the pin exists for. CI now pins 1.43.1, and the comment records that the version must be read with `nix develop --command just --version` rather than `nix eval nixpkgs#just.version`, which reads the live registry instead of this repo's lock.

## v0.6.16

### Added

- `LICENSE` (Apache-2.0), `SECURITY.md` and `CONTRIBUTING.md`. nebu previously shipped no license file at all, and the two claims in the tree disagreed — `README.md` said MIT while `flake.nix` declared Apache-2.0.
- `docs/engineering-profile.md`: nebu's project profile under the OBSRVR Engineering Standard v2.0 — applicable rules, the contracts nebu owns, its product-specific rules, its exceptions, and its conformance status.
- `docs/adr/`: architecture decision records in MADR form, with an index. ADR 0001 resolves whether the CLI surface is a contract; ADR 0002 schedules the exit-code remap; ADR 0003 records the narrowed CLI-12 exception and ADR 0004 the task-runner exception — both accepted, so those gaps are live exceptions under CONF-1 expiring 2027-04-20 rather than unrecorded non-conformance; ADR 0004 adds the lifecycle vocabulary below.
- `nebu.status.v1` records carry a required `lifecycle` field holding the CLI-12 operation lifecycle (`running`, `succeeded`, `failed`, `cancelled`) alongside the existing Program Status Protocol `state` token. One consumer state machine now reads nebu, flowctl and the `obsrvr` CLI. Additive — consumers are already required to ignore unknown fields, so the schema version does not move. The mapping is derived on write and cannot be set independently, so the two fields can never disagree. Note that `state: idle` maps to `lifecycle: cancelled`: CLI-17 specifies `idle` for interruption, which is the only case nebu emits it. See ADR 0004 and `docs/STATUS_STREAM.md`.
- CI merge gate (`.github/workflows/ci.yml`): build, `gofmt`, `go vet`, `go test -race -short`, the stable-API snapshot check, and `govulncheck`. No workflow previously built or tested nebu on a pull request.
- A `justfile` as the task runner (BUILD-1), with `just ci` running exactly what the merge gate runs, plus `just test-short` and `just fmt-check`. `just` with no arguments lists every recipe.

### Changed

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
