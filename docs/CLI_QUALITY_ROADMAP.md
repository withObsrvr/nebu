# Nebu CLI quality roadmap

**Status:** Proposed implementation roadmap  
**Scope:** `nebu`, its processor CLI helpers, and their public command-line contracts  
**Audience:** Maintainers and contributors planning post-`v0.6.15` improvements

## Purpose

Nebu should become an exceptionally predictable streaming CLI and processor
platform. It should not become a service manager, persistent daemon, pipeline
control plane, or graphical application backend. Those responsibilities belong
to flowctl.

The guiding principle is:

> Make the CLI feel simple while treating its streams, schemas, errors, status,
> installation behavior, and interruption semantics as carefully engineered
> interfaces.

This document translates general CLI-design advice into work that fits nebu's
actual role. It defines the intended architecture, prioritized work packages,
compatibility constraints, acceptance criteria, and ideas that are explicitly
out of scope.

## Product boundary

Nebu is:

- a local-first command-line toolkit for Stellar data extraction;
- a stable Go contract for origin, transform, and sink processors;
- a registry and installer for independently distributed processors;
- a Unix pipeline participant using XDR and JSONL streams;
- a fast path from a ledger range to useful data in shell tools, DuckDB, or a
  file sink.

Nebu is not:

- an authoritative store of pipeline or job state;
- a process or container orchestrator;
- a long-running local daemon;
- a remote multi-user API;
- a pipeline YAML runtime;
- a desired-state reconciler;
- an audit or authorization system;
- a replacement for flowctl.

The existing separation in [ARCHITECTURE_DECISIONS.md](ARCHITECTURE_DECISIONS.md)
remains authoritative. New work must preserve the shell-native pipeline:

```text
ledger source -> origin -> transform(s) -> sink
```

Data belongs on stdout. Diagnostics belong on stderr. OSC 7501 belongs on the
terminal diagnostic stream. `nebu.status.v1` belongs in its explicitly
configured status file.

## Existing strengths to preserve

Nebu already implements many of the most valuable modern CLI practices:

- Unix-pipe composition and streaming input/output;
- clean separation of stdout data and stderr diagnostics;
- schema-versioned processor events;
- stable, documented exit codes;
- context cancellation and interrupt handling;
- OSC 7501 terminal status;
- a stable `nebu.status.v1` machine-readable progress stream;
- durable fetch checkpoints and resume behavior;
- processor discovery with `list` and `describe`;
- capability and schema discovery through `--describe-json`;
- independently distributed processors and a registry specification;
- compatibility-sensitive Go contracts with API snapshots;
- checksummed binary installation and post-install conformance checks;
- reproducible release automation through Go, Nix, containers, and GoReleaser.

Future work must strengthen these foundations rather than introduce a parallel
application model.

## Design rules

All work in this roadmap should follow these rules.

1. **Cobra is an adapter.** A `RunE` function parses input, invokes an
   operation, and renders its result. It should not contain the implementation
   of the operation.
2. **Operations are transport-independent Go code.** They accept typed input,
   `context.Context`, and explicit dependencies. They do not assume a terminal,
   HTTP server, or daemon.
3. **Writers are explicit.** Code should accept `io.Writer` values or use
   `cmd.OutOrStdout()` and `cmd.ErrOrStderr()` instead of reaching directly for
   process-global streams.
4. **Machine contracts are versioned.** Stable JSON and NDJSON output includes
   a schema identifier and additive compatibility rules.
5. **Human output may evolve.** Tables, prose, spacing, and presentation are not
   machine contracts.
6. **Streaming formats remain purpose-specific.** A global presentation flag
   must not reinterpret XDR or processor JSONL as ordinary command output.
7. **No secrets in output.** Human logs, JSON errors, plans, status records, and
   diagnostics redact authentication material.
8. **Cancellation is ordinary control flow.** Long-running paths honor context
   cancellation and return the documented interrupted status.
9. **No orchestration creep.** Persistent jobs, resource lifecycles, event
   histories, and remote control remain flowctl concerns.
10. **Compatibility changes are deliberate.** Stable surfaces follow
    [STABILITY.md](STABILITY.md), with tests and API snapshots where required.

## Target internal architecture

The target structure is intentionally smaller than a daemon/client design:

```text
Cobra command adapter
        |
        v
typed request -> operation -> typed result/error
                       |
                       +-- source factory
                       +-- registry/installer
                       +-- filesystem
                       +-- clock
                       +-- progress observer
                       +-- explicit writers
```

Illustrative types:

```go
type FetchRequest struct {
    StartLedger uint32
    EndLedger   uint32
    Source      SourceConfig
    Output      OutputConfig
    Checkpoint  CheckpointConfig
}

type FetchResult struct {
    LedgersWritten uint64
    LastLedger     uint32
    Resumed        bool
}

type Dependencies struct {
    DataOutput       io.Writer
    DiagnosticOutput io.Writer
    Status           StatusReporter
    SourceFactory    SourceFactory
    Clock            Clock
}

func Fetch(ctx context.Context, deps Dependencies, req FetchRequest) (FetchResult, error)
```

These are internal design examples, not proposed additions to the stable public
API. Do not export a new framework prematurely.

## Prioritized implementation plan

### Priority 1: Extract operations from Cobra handlers

#### Problem

Several command handlers combine argument parsing, configuration resolution,
validation, filesystem setup, signal handling, source construction, execution,
and rendering. This makes behavior harder to test and reuse without a process
or terminal.

`fetch` should be the first refactor because it contains the most important
reliability behavior: binary output, two source modes, cancellation,
checkpoints, status reporting, and resume semantics.

#### Work

- Introduce internal request/result types for fetch.
- Move source execution, ordered output, checkpoint advancement, and the final
  result into a terminal-independent operation.
- Keep flag parsing, environment resolution, signal-aware context creation,
  and human rendering in `cmd/nebu`.
- Inject source creation so tests can exercise success, gaps, cancellation,
  write failures, and upstream failure without network access.
- Repeat the pattern for `install`, `list`, and `describe` after fetch proves
  the boundary.
- Avoid a generic command framework. Prefer small operation-specific types.

#### Acceptance criteria

- The fetch operation can be tested with buffers and fake sources without
  executing Cobra or replacing global process streams.
- Existing CLI syntax, exit codes, XDR output, checkpoint semantics, OSC
  reports, and structured status remain unchanged.
- Cancellation and final output/checkpoint ordering retain regression tests.
- `make test`, `make vet`, `make docs-smoke`, and `make api-check` pass.

### Priority 2: Make all command I/O injectable

#### Problem

Direct `os.Stdout` and `os.Stderr` writes make unit tests process-global and can
ignore Cobra's configured writers. They also blur the distinction among data,
human diagnostics, and machine status.

#### Work

- Use `cmd.OutOrStdout()` and `cmd.ErrOrStderr()` in Cobra adapters.
- Pass explicit writers into human and JSON renderers.
- Pass an explicit data writer into fetch and processor execution.
- Give processor CLI helpers an internal I/O configuration while preserving
  current public helper signatures. Additive options may be considered only if
  external processor authors need them.
- Replace tests that temporarily assign `os.Stdout` or `os.Stderr` with buffer
  injection.
- Preserve direct child-process inheritance where installation intentionally
  streams `go install` output, but make the selected destinations explicit.

#### Acceptance criteria

- Command renderers have focused tests using `bytes.Buffer`.
- Parallel tests do not mutate global stdout or stderr.
- Data never enters the diagnostic writer, including error and interrupt paths.
- Status-file alias protection and redirected-stdout tests continue to pass.

### Priority 3: Version discovery-command JSON

#### Problem

Processor events and structured status are versioned, but `nebu list --json`
and `nebu describe --json` do not identify a stable schema. Agents and scripts
therefore cannot know whether they are consuming a committed contract.

#### Proposed contracts

```json
{
  "_schema": "nebu.processor_list.v1",
  "processors": []
}
```

```json
{
  "_schema": "nebu.processor_description.v1",
  "processor": {}
}
```

The exact names should be settled before implementation and then recorded in
`STABILITY.md`.

#### Work

- Define Go envelope types for list and describe output.
- Document required fields, optional fields, and forward-compatibility rules.
- Require consumers to ignore unknown fields.
- Add JSON Schema documents or schema-generating tests.
- Add golden or structural contract tests.
- Keep `--json` working.
- Consider adding `--output=human|json` to discovery commands as an additive
  spelling. Do not remove `--json` during the 0.x transition without a
  documented deprecation window.
- Do not add a global `--output` that changes fetch XDR or processor JSONL.

#### Compatibility decision

Changing the top-level `list --json` value from an array to an envelope is a
breaking scripting change. Choose one of these before implementation:

1. introduce a new explicit format such as `--output=json-v1` and retain the
   legacy array under `--json`; or
2. announce and execute a pre-1.0 migration with a deprecation period.

The first option is safer.

#### Acceptance criteria

- A schema identifier unambiguously selects the output contract.
- Unknown-field compatibility is tested.
- Human output remains outside the stable machine contract.
- The website and repository docs use the stable format in scripting examples.

### Priority 4: Add structured errors

#### Problem

Exit codes identify broad failure classes, but automation must still parse
English diagnostics to identify the exact condition and relevant fields.

#### Proposed interface

```bash
nebu --error-format=json install missing-processor
```

```json
{
  "_schema": "nebu.error.v1",
  "code": "processor_not_found",
  "exit_code": 3,
  "operation": "install",
  "message": "Processor was not found",
  "details": {"name": "missing-processor"}
}
```

#### Work

- Define stable string error codes independently from Go type names.
- Map current typed error kinds to codes and documented exit codes.
- Emit exactly one terminal JSON error record to stderr when JSON error mode is
  selected.
- Preserve human-readable stderr as the default.
- Redact headers, credentials, signed URLs, tokens, and sensitive paths where
  appropriate.
- Keep exit codes authoritative even if rendering fails.
- Define whether child-process diagnostics precede the final structured record;
  document the answer so consumers can parse reliably. A dedicated error file
  may be preferable if stderr must also contain child output.

#### Acceptance criteria

- Every documented exit-code class has structured-error coverage.
- Error codes are stable and tested in a table.
- No secret-bearing configuration is serialized.
- stdout remains untouched on all failures.

### Priority 5: Add `nebu doctor`

#### Goal

Diagnose installation, version, configuration, and processor-contract problems
without requiring users to infer them from a failed backfill.

#### Default offline checks

- nebu version, build information, and executable path;
- duplicate `nebu` executables on `PATH` and which one wins;
- `GOBIN` and `GOPATH/bin` discoverability;
- installed processor paths and versions;
- registry version versus installed processor version;
- successful, JSON-only `--describe-json` execution;
- required processor description fields and schema validity;
- write access for explicitly supplied output/checkpoint/status directories;
- path-alias conflicts among output, checkpoint, and status targets;
- relevant optional tools such as `jq` and DuckDB, clearly marked optional;
- resolved platform (`GOOS`, `GOARCH`) for binary installation.

#### Explicit connectivity checks

Network access should be opt-in:

```bash
nebu doctor --connectivity
nebu doctor --processor token-transfer
nebu doctor --json
```

Connectivity mode may verify:

- RPC endpoint reachability and network identity;
- archive datastore configuration and access to a harmless known object or
  requested ledger;
- authentication presence without printing credentials.

It must use bounded timeouts and make no writes.

#### Output

Human output should identify `pass`, `warning`, `failure`, and a concrete next
action. JSON output should use a versioned schema, for example
`nebu.doctor.v1`.

#### Acceptance criteria

- The old-binary/multiple-PATH-entry scenario is detected.
- Missing processor status/introspection support is explained directly.
- Default execution performs no network calls and no mutations.
- Connectivity checks cannot print authentication values.
- Exit status distinguishes healthy, warnings-only, and failed checks, with the
  exact policy documented.

### Priority 6: Add processor conformance checking

#### Proposed command

```bash
nebu processor check ./my-processor
```

If introducing a `processor` noun would unnecessarily reshape the current CLI,
an initial spelling such as `nebu check-processor` is acceptable. Choose the
long-term command organization before publishing it.

#### Checks

- the executable starts and supports `--describe-json`;
- describe succeeds without runtime-required flags or network access;
- stdout contains one valid JSON envelope and no diagnostic prefix;
- name, type, version, schemas, flags, and examples satisfy the contract;
- executable and declared registry names agree;
- declared JSON Schemas are valid;
- help and describe flag declarations agree where mechanically comparable;
- a configured status stream opens and reaches an appropriate terminal state;
- transform/sink malformed-input behavior follows the streams-never-throw rule;
- stdout remains valid NDJSON during a fixture run;
- SIGINT is honored promptly and returns the documented status.

Some runtime checks require fixtures or command-specific flags. Support a small
manifest rather than guessing valid invocation arguments.

#### Acceptance criteria

- Registry CI can run the checker against every published processor.
- Failures name the violated section of `PROCESSOR_CONTRACT.md`.
- Human and versioned JSON reports are available.
- The checker itself does not install or publish anything.

### Priority 7: Add scoped dry runs

Dry-run behavior must be command-specific and must state exactly what is and is
not verified.

#### `nebu install --dry-run`

Resolve and display:

- selected registry entry;
- repository/module and ref;
- installation kind;
- platform artifact URL and checksum source;
- intended target path;
- post-install conformance step;
- deprecation status.

It must not download, compile, install, or replace a binary unless a separate
option explicitly requests download verification.

#### `nebu fetch --dry-run`

Resolve and validate:

- RPC or archive mode;
- network;
- bounded or unbounded range;
- datastore, bucket, region, buffer, and worker settings;
- output, checkpoint, and status paths;
- checkpoint resume position and already-complete state;
- path alias conflicts.

It must not fetch ledgers, create output files, or advance checkpoints.

#### Acceptance criteria

- Plans are deterministic and redact secrets.
- No filesystem mutation occurs in dry-run mode.
- A versioned JSON plan is available for automation.
- Documentation does not imply that offline dry run proves remote access.

### Priority 8: Add configuration explanation

Nebu should improve transparency before adding a general configuration-file
framework.

#### Proposed interface

```bash
nebu config explain fetch --mode archive
nebu config explain fetch --mode archive --json
```

Output should show the effective value and its source:

```text
mode             archive    flag
datastore-type   S3         NEBU_DATASTORE_TYPE
bucket-path      ...        NEBU_BUCKET_PATH
region           us-east-2  NEBU_REGION
workers          10         default
```

#### Work

- Reuse the existing configuration-source tracking.
- Centralize precedence as `flag > environment > default` unless a command
  explicitly documents otherwise.
- Redact authentication values while still reporting their source and presence.
- Ensure the same resolver powers execution, startup banners, dry runs, and
  explain output. Do not duplicate resolution logic.

#### Explicit deferral

Do not add Koanf, profiles, or a nebu config file until repeated real-world use
shows that flags and environment variables are insufficient.

### Priority 9: Add correlation identifiers

#### Goal

Allow one invocation to be correlated across human logs, structured status,
processor status, checkpoints, and an enclosing flowctl process without
creating a persistent run resource.

#### Proposed behavior

- Generate a random invocation ID at command start.
- Accept `NEBU_RUN_ID` or a more narrowly named correlation variable from an
  orchestrator so related processors can share a value.
- Add an optional `run_id` to `nebu.status.v1` under its additive-field rules.
- Include the ID in structured errors and JSON plans.
- Make human display concise; print it at startup or only in verbose mode.
- Never use the identifier as a durability or idempotency key by accident.

#### Acceptance criteria

- Status consumers that ignore unknown fields remain compatible.
- IDs are bounded in length and validated before being repeated in output.
- No high-cardinality run ID is added as a Prometheus label.

### Priority 10: Mechanically verify help and examples

#### Problem

CLI documentation is part of the user experience, but handwritten examples can
drift from actual commands, flags, processor versions, and schemas.

#### Work

- Extend `make docs-smoke` to validate commands and flags appearing in root and
  subcommand help.
- Validate processor example flags against `--describe-json` envelopes.
- Syntax-check shell snippets where practical.
- Validate JSON examples against their declared JSON Schemas.
- Ensure every documented processor exists in the intended registry or is
  clearly labeled illustrative.
- Generate or centrally source release-version statements instead of manually
  claiming that an already tagged release is still being prepared.
- Test shell completion generation for Bash, Zsh, Fish, and PowerShell.
- Keep examples bounded by default. Use unbounded streaming only when the
  example explicitly teaches it.

#### Immediate documentation corrections

- Audit root help for abbreviated processor flags such as `--start`/`--end`
  when the processor actually exposes `--start-ledger`/`--end-ledger`.
- Update stale release language referring to `v0.6.15` as unreleased.
- Reconcile any roadmap language suggesting pipeline descriptors with the rule
  that nebu does not become a pipeline-YAML orchestrator. A descriptor may be a
  handoff artifact, but nebu must not own deployment lifecycle.

## Secondary improvements

These are valuable after the ten priorities above.

### Terminal accessibility

Nebu currently has limited styling, so a color framework is not urgent. Still:

- avoid requiring color or emoji to understand status;
- respect `NO_COLOR` if color is added;
- provide an ASCII-safe mode if symbols cause compatibility problems;
- avoid unbounded-width tables;
- keep noninteractive output free of cursor movement and spinners;
- retain `--quiet` for scripting.

### Timeouts

Avoid an ambiguous universal timeout. Prefer explicit meanings:

- connection/request timeout for RPC setup;
- describe/introspection timeout;
- install/download timeout;
- overall bounded-operation deadline only if users need it.

Cancellation must continue to flow through `context.Context`. A timeout error
must remain distinguishable from user interruption.

### Performance budgets

Track a few user-visible budgets rather than optimizing blindly:

- `nebu --help`, `nebu list`, and local `nebu describe` startup latency;
- memory bounded by configured fetch buffers;
- no full-stream buffering for XDR or JSONL;
- no network access for local help, describe metadata, doctor default mode, or
  dry-run configuration validation;
- bounded shutdown time after cancellation.

Add benchmarks only where they protect a meaningful regression.

### Upgrade visibility

Nebu may report its own version and processor version differences through
`doctor`, but it should not silently phone home on every command. Upgrade checks
must be explicit or infrequent, respect offline operation, and never block core
commands.

### Cross-platform behavior

Continue release testing across Linux, macOS, and Windows. Specifically test:

- executable suffix and binary-install artifact selection;
- path alias and symlink behavior where supported;
- signals and cancellation semantics appropriate to each platform;
- status-file atomicity and directory synchronization fallbacks;
- PowerShell completion and pipeline examples;
- terminal detection with redirected streams.

## Agent-friendly behavior without an agent subsystem

Nebu should be easy for an agent to use without embedding an agent runtime or
MCP server. The required foundations are:

- stable discovery JSON;
- stable error JSON;
- stable exit codes;
- schemas for data and command results;
- a processor conformance checker;
- explicit side-effect descriptions in help and dry-run plans;
- noninteractive defaults;
- no prompts in CI unless explicitly requested;
- deterministic output and bounded operations;
- correlation IDs for multi-process diagnostics.

An external MCP server or agent can then wrap nebu using these contracts. That
adapter belongs outside the stable core unless concrete usage demonstrates a
need for an official integration.

## Ideas that belong in flowctl

Do not implement the following in nebu:

- a persistent local daemon;
- stored pipeline, job, or session resources;
- resource `create`, `start`, `stop`, `wait`, `events`, or `call` lifecycles;
- durable event history and resumable event subscriptions;
- multi-user authentication, authorization, or audit trails;
- remote team contexts;
- declarative pipeline reconciliation;
- container or process supervision;
- GUI state synchronization;
- service health aggregation;
- distributed checkpoints and recovery coordination.

Nebu may emit the status, schema, and process signals that flowctl consumes. It
must not duplicate flowctl's ownership of the larger lifecycle.

## Ideas to defer until demonstrated demand

- Bubble Tea or another full-screen TUI;
- a local HTTP, gRPC, or Connect server;
- a general plugin transport such as `go-plugin`;
- config profiles and configuration files;
- a built-in MCP server;
- dynamic resource methods;
- desired-state `plan` and `apply`;
- background job persistence;
- extracting the stable processor contract into a separate module.

Deferral is not rejection. Each item should begin with a concrete user problem,
an owner, and evidence that existing shell composition or flowctl cannot solve
it cleanly.

## Suggested pull-request sequence

Keep changes reviewable and preserve stable behavior after every merge.

1. **Internal fetch operation and injected source factory**
2. **Injected command/rendering writers**
3. **Versioned discovery JSON design and additive implementation**
4. **Structured error schema and renderer**
5. **Offline `nebu doctor`**
6. **Optional doctor connectivity checks**
7. **Processor conformance checker**
8. **Install dry run**
9. **Fetch dry run and shared configuration resolver**
10. **Configuration explanation**
11. **Optional correlation ID fields**
12. **Help/example/release-documentation verification**

Do not combine the operation refactor with JSON contract changes. The former
should demonstrate behavioral equivalence; the latter requires an explicit
compatibility decision.

## Verification matrix

Every implementation PR should select the relevant checks below.

| Area | Required verification |
|---|---|
| Go implementation | `make build`, `make test`, `make vet` |
| Stable packages | `make api-check` |
| Help or examples | `make docs-smoke` |
| Processor helpers | helper unit tests plus representative origin, transform, and sink tests |
| JSON contract | schema/golden tests and unknown-field compatibility test |
| Exit/error behavior | table-driven command exit-code tests |
| Streaming output | parse/framing test proving stdout contains only the declared data format |
| Cancellation | bounded test proving prompt exit and code 130 where applicable |
| Checkpoints | interruption, sync ordering, replay bound, and completed-range tests |
| Cross-platform paths | clean/absolute, symlinked parent, existing alias, and redirected stdout tests |
| Documentation | repository docs plus website docs updated together |

Network-dependent failures must be reported exactly; they must not be hidden or
converted into unconditional skips.

## Definition of success

This roadmap is successful when:

- command handlers are thin adapters rather than operation implementations;
- core operations are testable without a terminal or global stream mutation;
- agents and scripts can consume versioned discovery and error contracts;
- users can diagnose old binaries, PATH conflicts, processor incompatibility,
  and configuration mistakes with `nebu doctor`;
- processor authors can run one conformance command before publishing;
- install and fetch behavior can be previewed without mutation;
- examples are mechanically checked against the actual CLI;
- all existing XDR, JSONL, checkpoint, status, exit-code, and processor
  compatibility guarantees remain intact;
- nebu remains a small Unix-composable tool and flowctl remains the owner of
  persistent orchestration.

