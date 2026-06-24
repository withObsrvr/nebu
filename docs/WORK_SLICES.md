# nebu Work Slices

Prioritized, independently shippable work items for nebu's next phase. Each slice is scoped to be completable by one developer in 1-5 days. Slices are grouped into tiers; within each tier, items are ordered by leverage (do the top ones first).

**Context**: nebu v0.6.x shipped the stable contract (`pkg/processor`, `pkg/source`), streams-never-throw error handling, the `--describe-json` introspection protocol, runtime hooks, the registry spec, and hand-written SKILL.md files for AI agents. This document covers what comes next.

**Design principles** (from [Mitchell Hashimoto's Building Block Economy](https://mitchellh.com/writing/building-block-economy)):
- Optimize for strangers building things we didn't imagine, without asking us.
- Derive everything possible from schemas; hand-write as little as possible.
- Agents and human strangers need the same things: introspectable schemas, deterministic CLIs, structured errors.

---

## Tier 1: Foundation

These slices improve the core CLI and contract surface. No external dependencies, no new binaries. Ship these before anything in Tier 2+.

---

### Slice 1: Structured exit codes

**Why**: Processors currently exit 0 or 1. Agents and scripts can't distinguish "bad flags" from "Horizon is down" from "internal bug" without parsing stderr.

**Scope**:
- Define exit code constants in `pkg/processor` or `pkg/errors`:

  | Code | Name | Meaning |
  |------|------|---------|
  | 0 | `ExitOK` | Success |
  | 1 | `ExitProcessingError` | Processing error (malformed ledger, bad event) |
  | 2 | `ExitNetworkError` | Network/RPC failure (Horizon unreachable, timeout) |
  | 3 | `ExitValidationError` | Validation failure (bad flags, invalid ledger range) |
  | 4 | `ExitDiscoveryError` | Registry or processor discovery failure |
  | 5 | `ExitInternalError` | Internal bug (panic recovery, unexpected state) |

- Update `pkg/processor/cli/*.go` to map `NebuError` types and Reporter fatals to the appropriate exit code.
- Update `cmd/nebu/*.go` commands to use the codes.
- Document the codes in `docs/STABILITY.md` as part of the stable contract (they should be permanent once shipped).
- Add a `--exit-codes` or `nebu help exit-codes` reference.

**Definition of done**: Every nebu binary and processor exits with a semantic code. An agent can `if [ $? -eq 2 ]; then retry_with_backoff; fi` without parsing text.

**Key files**: `pkg/errors/errors.go`, `pkg/processor/cli/origin.go`, `pkg/processor/cli/transform.go`, `pkg/processor/cli/sink.go`, `cmd/nebu/main.go`, `docs/STABILITY.md`

**Depends on**: Nothing.

**Effort**: S (1-2 days)

---

### Slice 2: `--dry-run` for `nebu run`

**Why**: When an AI agent composes a pipeline, it should be able to preview what will execute before committing. This is a safety net for agent-authored pipelines and a debugging tool for humans.

**Scope**:
- Add a `--dry-run` flag to `nebu run` (and consider processor CLIs too).
- In dry-run mode, resolve the full pipeline: which processors, which flags, which ledger range, which sink. Print the resolved plan as structured JSON to stdout, then exit 0.
- The plan should include:
  - Each stage (origin, transforms, sink) with its binary name and resolved flags
  - The shell command that would be executed
  - The ledger range
  - Schema IDs of each stage (from `--describe-json`)
  - Any warnings (e.g., "sink binary not found in PATH")
- Do NOT fetch any ledgers or process any data.

**Definition of done**: `nebu run --dry-run <pipeline-args>` prints a JSON plan and exits. An agent can inspect the plan, then re-run without `--dry-run` to execute.

**Key files**: `cmd/nebu/run.go`, potentially a new `pkg/runtime/plan.go`

**Depends on**: Nothing (but pairs well with Slice 6: IndexDescriptor).

**Effort**: S (1-2 days)

---

### Slice 3: Auto-generated SKILL.md from registry + describe-json

**Why**: The current SKILL.md files are hand-written. As the processor registry grows (especially with external processors), manually maintaining skills won't scale. The Google Workspace CLI generates 100+ skill files from API schemas automatically.

**Scope**:
- Write a `scripts/generate-skills.sh` (or a Go tool in `cmd/`) that:
  1. Reads `registry.yaml` (all processors)
  2. For each installed processor, runs `<processor> --describe-json` to get the envelope
  3. Generates a SKILL.md per processor from a template (name, type, description, flags, schema, examples, works_with)
  4. Generates an index SKILL.md that lists all processors grouped by type
- Template should include: when to use, flags reference (from envelope), example commands (from envelope + registry), input/output schema summary, common pitfalls placeholder.
- Add a `make generate-skills` target.
- Keep the hand-written `skills/pipeline-composer/SKILL.md` as-is (it's a composition skill, not a processor skill). The auto-generated skills complement it.

**Definition of done**: `make generate-skills` produces a SKILL.md for every installed processor. Adding a new processor to the registry and running the generator is all that's needed to create its agent skill.

**Key files**: New `scripts/generate-skills.sh` or `cmd/nebu/generate_skills.go`, `skills/` directory, `Makefile`

**Depends on**: Nothing.

**Effort**: M (2-3 days)

---

### Slice 4: CONTRIBUTING.md for processor authors

**Why**: The building-block thesis says success = "strangers write processors without asking us." `docs/BUILDING_PROCESSORS.md` exists but is a reference doc, not a contributor onboarding guide. A root-level `CONTRIBUTING.md` is where GitHub contributors look first.

**Scope**:
- Create `CONTRIBUTING.md` at repo root, aimed at someone who wants to write and publish a processor (not contribute to nebu core).
- Structure:
  1. **Quick start**: `nebu new <name>`, implement the interface, `go build`, test with a fixture ledger
  2. **The contract**: link to `pkg/processor` godoc and `docs/STABILITY.md`
  3. **Testing your processor**: how to use fixture XDR ledgers, how to write unit tests with `processor.WithReporter`, how to verify `--describe-json` output
  4. **Publishing**: how to write `description.yml`, how to submit to `nebu-processor-registry`, how `nebu install` resolves external processors
  5. **Style guide**: naming conventions (kebab-case binaries), error reporting (use `ReportWarning`/`ReportFatal`, never panic), schema versioning (`nebu.<name>.v1`)
- Include a sample fixture ledger (or document how to capture one with `nebu fetch`) for testing without network access.

**Definition of done**: A developer who has never seen nebu can follow CONTRIBUTING.md from zero to a published processor in an external registry.

**Key files**: New `CONTRIBUTING.md`, updates to `docs/BUILDING_PROCESSORS.md` (cross-link), potentially a test fixture in `testdata/`

**Depends on**: Nothing.

**Effort**: M (2-3 days)

---

## Tier 2: Agent & Developer Experience

These slices make nebu more powerful for both AI agents and human developers. They build on the Tier 1 foundation but can be started in parallel.

---

### Slice 5: Dynamic CLI flags from --describe-json

**Why**: Currently, `nebu run` has its own flag definitions that must be kept in sync with processor flags. The Google Workspace CLI generates its entire command surface from API schemas at runtime. nebu can do the same with `--describe-json`.

**Scope**:
- When `nebu run <processor> [flags]` is invoked:
  1. Shell out to `<processor> --describe-json` to get the flag set
  2. Build a dynamic cobra flag set from the `flags[]` array in the envelope
  3. Parse remaining args against the dynamic flags
  4. Forward the resolved flags to the processor binary
- Handle edge cases: processor binary not found, `--describe-json` returns error, flag type mapping (the envelope has `type: "uint32"`, `type: "string"`, etc.)
- `nebu run <processor> --help` should show the processor's dynamically discovered flags, not generic help.

**Definition of done**: `nebu run token-transfer --start-ledger 60200000` works by discovering the flags from the processor itself. Adding a new flag to a processor requires zero changes in `cmd/nebu/`.

**Key files**: `cmd/nebu/run.go`, potentially `pkg/processor/cli/dynamic.go`

**Depends on**: Nothing, but benefits from Slice 2 (dry-run uses the same flag resolution).

**Effort**: M (2-3 days)

---

### Slice 6: IndexDescriptor — serializable pipeline format

**Why**: The canonical hand-off format between nebu and AI agents. An agent should be able to write a JSON/YAML file that fully describes a pipeline, and `nebu run descriptor.yaml` should execute it. This replaces ad-hoc flag combinations with a portable, versionable artifact.

**Scope**:
- Define the `IndexDescriptor` schema (YAML and JSON):

  ```yaml
  version: 1
  name: usdc-large-transfers
  description: USDC transfers over 10 USDC from mainnet

  origin:
    processor: token-transfer
    flags:
      start-ledger: 60200000
      end-ledger: 60200100
      network: mainnet

  transforms:
    - processor: usdc-filter
    - processor: amount-filter
      flags:
        min: 100000000

  sink:
    processor: json-file-sink
    flags:
      out: /tmp/usdc-big.jsonl
  ```

- Implement `nebu run --descriptor <file>` that reads the file and executes the pipeline.
- Implement `nebu run --dry-run --descriptor <file>` that prints the resolved plan without executing (ties into Slice 2).
- Add JSON Schema for the descriptor format (so agents can validate before submitting).
- Document in `docs/PIPELINE.md` or a new `docs/DESCRIPTORS.md`.

**Definition of done**: `cat descriptor.yaml | nebu run --descriptor -` executes a full pipeline. An agent can write a descriptor, validate it against the JSON Schema, dry-run it, then execute it.

**Key files**: New `pkg/descriptor/` package, `cmd/nebu/run.go`, new `docs/DESCRIPTORS.md`

**Depends on**: Benefits from Slice 2 (dry-run) and Slice 5 (dynamic flags), but can be built independently.

**Effort**: L (3-5 days)

---

### Slice 7: `--format` flag for orchestrator commands

**Why**: Processors emit JSONL via pipes (machine-readable by default). But orchestrator commands like `nebu list`, `nebu describe`, and any future `nebu query` should support multiple output formats for different consumers.

**Scope**:
- Add a global `--format` flag to the nebu CLI: `json` (default for piped output), `table` (default for TTY), `yaml`, `csv`.
- Implement a shared formatter in `pkg/output/` or `cmd/nebu/format.go`:
  - `json`: compact JSON, one object per line
  - `table`: aligned columns with truncation, dot-notation for nested fields
  - `yaml`: full YAML dump
  - `csv`: proper escaping, headers on first row
- Apply to `nebu list` (processor catalog as table/JSON) and `nebu describe` (processor detail as structured output).
- TTY detection: default to `table` when stdout is a terminal, `json` when piped. `--format` always overrides.

**Definition of done**: `nebu list --format json | jq` and `nebu list --format table` both work. Piped output defaults to JSON; interactive output defaults to table.

**Key files**: New `pkg/output/format.go` or `cmd/nebu/format.go`, `cmd/nebu/list.go`, `cmd/nebu/describe.go`, `cmd/nebu/main.go` (global flag)

**Depends on**: Nothing.

**Effort**: M (2-3 days)

---

### Slice 8: Presets catalog

**Why**: Reduce the degrees of freedom for agents and new users. Instead of hand-wiring flags for common scenarios, ship a catalog of well-known configurations.

**Scope**:
- Define a `presets.yaml` format:

  ```yaml
  presets:
    - name: usdc-transfers-mainnet
      description: All USDC transfers on mainnet
      pipeline:
        origin: token-transfer
        transforms: [usdc-filter]
        flags:
          network: mainnet

    - name: all-contract-events
      description: Raw contract events from any Soroban contract
      pipeline:
        origin: contract-events
        flags:
          network: mainnet
  ```

- Implement `nebu run --preset <name> --start-ledger N --end-ledger M`.
- Implement `nebu presets` to list available presets.
- Ship 5-10 presets covering common use cases (USDC transfers, all token transfers, contract events, DEX activity).
- Presets are composable with additional flags (the preset provides defaults, explicit flags override).

**Definition of done**: `nebu run --preset usdc-transfers-mainnet --start-ledger 60200000 --end-ledger 60200100` works. `nebu presets` lists all available presets with descriptions.

**Key files**: New `presets.yaml`, `cmd/nebu/run.go`, `cmd/nebu/presets.go`

**Depends on**: Benefits from Slice 6 (IndexDescriptor — presets are just named descriptors).

**Effort**: S-M (2-3 days)

---

## Tier 3: DuckDB Integration

These slices connect nebu to DuckDB, opening nebu to the analytical SQL ecosystem. They can be developed independently of each other.

---

### Slice 9: DuckDB-native sink processor

**Why**: Instead of nebu → JSONL → DuckDB import (two steps), write directly to a `.duckdb` file (one step). DuckDB is the most natural analytical backend for nebu's event streams.

**Scope**:
- New processor: `examples/processors/duckdb-sink/`
- Reads JSONL from stdin (standard sink interface).
- Writes to a `.duckdb` file specified by `--out`.
- Auto-creates the table schema from the first event's shape (or from a `--schema` flag pointing to a JSON Schema file from `--describe-json`).
- Supports TOID-based idempotent upserts (like `postgres-sink`): if the same TOID is seen again, skip or overwrite.
- Supports append mode and checkpoint tracking (last-processed ledger stored in a metadata table).
- Uses the DuckDB Go driver (`github.com/marcboeker/go-duckdb` or the official `github.com/duckdb/duckdb-go`).

**Definition of done**: `token-transfer --start-ledger N --end-ledger M | duckdb-sink --out stellar.duckdb` produces a queryable DuckDB database. `duckdb stellar.duckdb "SELECT count(*) FROM events"` works.

**Key files**: New `examples/processors/duckdb-sink/` directory (go.mod, main.go, sink.go, README.md)

**Depends on**: Nothing (standalone processor).

**Effort**: M-L (3-5 days)

---

### Slice 10: `duckdb-nebu` extension — processors as SQL table functions

**Why**: The highest-leverage DuckDB integration. Every nebu processor becomes SQL-addressable: `SELECT * FROM nebu('token-transfer', start => 60200000, end => 60200100)`. DuckDB users discover nebu through SQL, not the CLI.

**Scope**:
- DuckDB extension (C++ or Rust, following [query.farm](https://query.farm) conventions).
- Registers a `nebu()` table function that:
  1. Takes processor name + flags as arguments
  2. Discovers the processor binary via PATH
  3. Shells out to `<processor> --describe-json` to get the output schema
  4. Runs the processor with the given flags, reads JSONL from its stdout
  5. Streams rows into DuckDB using the discovered schema
- The extension should handle:
  - Schema inference from `--describe-json` envelope's `schema.output`
  - Streaming (don't buffer the entire output)
  - Error propagation (processor exit code → SQL error)
- Optional: `nebu_list()` table function that returns the processor catalog.

**Definition of done**: `INSTALL nebu FROM community; LOAD nebu; SELECT * FROM nebu('token-transfer', start => 60200000, end => 60200100) LIMIT 10;` returns rows.

**Key files**: New repo or `extensions/duckdb-nebu/` directory. Separate build system (CMake for DuckDB extensions).

**Depends on**: Nothing technically, but having Slice 9 (DuckDB sink) first gives familiarity with DuckDB's Go/C++ interfaces.

**Effort**: XL (1-2 weeks). This is the most complex slice and could be its own project.

---

## Tier 4: Ecosystem

Longer-term slices that build on the foundation. These are larger initiatives, not single-sprint items.

---

### Slice 11: Agent-authored processors

**Why**: The building-block endgame. An AI agent reads the contract, scaffolds a processor, tests it, and publishes it. The registry fills itself.

**Scope**:
- Create a `nebu-processor-builder` SKILL.md (like `pipeline-composer` but for authoring new processors).
- The skill should teach an agent to:
  1. Read `docs/BUILDING_PROCESSORS.md` and a reference exemplar
  2. Run `nebu new <name>` to scaffold
  3. Implement the Origin/Transform/Sink interface
  4. Test against a fixture ledger (captured with `nebu fetch`)
  5. Verify `--describe-json` output matches expectations
  6. Write `description.yml` for registry submission
- Include fixture ledger data in `testdata/` (a few real ledgers captured from mainnet) so the agent can test without network access.
- Consider a `nebu test <processor>` command that runs the processor against fixtures and validates output shape against its declared schema.

**Definition of done**: An AI agent (Claude, GPT-4, etc.) can follow the skill to create a working processor from a natural-language description, with tests passing, in a single session.

**Key files**: New `skills/processor-builder/SKILL.md`, `testdata/`, potentially `cmd/nebu/test.go`

**Depends on**: Slice 4 (CONTRIBUTING.md), Slice 3 (auto-generated skills for reference).

**Effort**: L (1 week)

---

### Slice 12: "Ask the chain" natural-language REPL

**Why**: The killer demo for nebu. A REPL that mixes natural language, SQL, and nebu commands. An agent translates NL queries into processor chains and DuckDB SQL.

**Scope**:
- A new binary or mode: `nebu repl` or `nebu chat`.
- Integrates an LLM (via API) with tools:
  - `nebu list` — discover processors
  - `<processor> --describe-json` — introspect schemas
  - `nebu run` — execute pipelines
  - `duckdb` — query results
- The LLM sees the full processor catalog and schemas, then translates natural language into pipelines + SQL.
- Example session:
  ```
  > what were the largest USDC transfers last week?
  [agent installs token-transfer, usdc-filter, runs pipeline, queries with DuckDB]
  Top 5 USDC transfers in ledgers 62000000-62100000:
  1. GA...7X → GD...3K  250,000 USDC  (ledger 62045123)
  ...
  ```

**Definition of done**: A working demo that can answer basic Stellar data questions end-to-end.

**Key files**: New `cmd/nebu-repl/` or `cmd/nebu/repl.go`, integration with an LLM API

**Depends on**: Slice 9 or 10 (DuckDB integration), Slice 3 (auto-generated skills for the agent's context).

**Effort**: XL (2+ weeks)

---

### Slice 13: Composable skill tiers

**Why**: Following the Google Workspace CLI pattern of atomic → persona → recipe skills. As the processor and skill catalog grows, agents need higher-level entry points.

**Scope**:
- **Processor skills** (Tier 1, already exists): one SKILL.md per processor.
- **Pipeline skills** (Tier 2, `pipeline-composer` exists): how to compose processors.
- **Workflow skills** (Tier 3, new): end-to-end scenarios:
  - `skills/workflows/backfill-to-postgres.md` — backfill a date range to a Postgres table
  - `skills/workflows/live-monitoring.md` — continuous stream with alerting
  - `skills/workflows/historical-analysis.md` — archive mode + DuckDB analysis
  - `skills/workflows/contract-audit.md` — audit a specific Soroban contract's activity
- Each workflow skill references the processor and pipeline skills it uses.
- Add YAML frontmatter to all skills for machine parsing (name, version, dependencies, required processors).

**Definition of done**: An agent can read a workflow skill and execute a multi-step scenario (backfill + query + summarize) without human guidance.

**Key files**: New `skills/workflows/` directory, updates to existing skills for cross-linking

**Depends on**: Slice 3 (auto-generated processor skills), Slice 6 (IndexDescriptor for pipeline definitions).

**Effort**: M (2-3 days per workflow skill)

---

### Slice 14: Contract module extraction

**Why**: Extract `pkg/processor` and `pkg/source` into a separate Go module (`github.com/withObsrvr/nebu-api` or similar) so external processors depend on a tiny, independently-versioned module instead of all of nebu.

**Scope**:
- Create a new module with just the stable-surface types: `Processor`, `Origin`, `Transform`, `Sink`, `Emitter[T]`, `Reporter`, `LedgerSource`, `DescribeEnvelope`, and related types.
- Update all internal nebu code to import from the new module.
- Update all example processors to import from the new module.
- The new module gets its own semver and release cycle.
- Update `docs/STABILITY.md` to reference the new module.

**Definition of done**: External processors `go get github.com/withObsrvr/nebu-api@v1.0.0` and depend only on that module. The main nebu repo can churn freely without breaking external processors.

**Key files**: New module repo or directory, all `go.mod` files, `docs/STABILITY.md`

**Depends on**: Sufficient external demand. The `.api/` snapshot CI check is the current substitute. Defer until at least 2-3 external processors exist.

**Effort**: L (3-5 days, mostly mechanical but high-risk for breaking changes)

---

## Slice dependency graph

```
Tier 1 (no dependencies, start immediately):
  [1] Structured exit codes
  [2] --dry-run
  [3] Auto-generated SKILL.md
  [4] CONTRIBUTING.md

Tier 2 (can start in parallel with Tier 1):
  [5] Dynamic CLI flags          ← benefits from [2]
  [6] IndexDescriptor            ← benefits from [2], [5]
  [7] --format flag
  [8] Presets catalog             ← benefits from [6]

Tier 3 (independent):
  [9]  DuckDB sink
  [10] duckdb-nebu extension     ← benefits from [9]

Tier 4 (build on earlier tiers):
  [11] Agent-authored processors  ← [3], [4]
  [12] "Ask the chain" REPL       ← [9] or [10], [3]
  [13] Composable skill tiers     ← [3], [6]
  [14] Contract module extraction ← defer until external demand
```

## Effort key

| Size | Days | Description |
|------|------|-------------|
| S | 1-2 | Focused change, mostly in one package |
| M | 2-3 | Touches 2-3 packages, some design decisions |
| L | 3-5 | New package or significant feature, needs design review |
| XL | 1-2 weeks | Multi-package or cross-repo, may need external research |
