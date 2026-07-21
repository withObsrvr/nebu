# nebu Stability Policy

**Status:** Pre-1.0. Most of nebu changes freely. A small, deliberately minimal _contract surface_ is committed to long-term stability so that external processors and registries can depend on it without churning.

This document says what is and is not covered by that commitment. If you're writing an external processor, external registry, or depending on nebu as a library, read this before you start.

## The stable surface

| Surface | Commitment |
|---|---|
| **`pkg/processor`** | The types `Processor`, `Origin`, `Transform`, `Sink`, `Emitter[T]`, the `Type` enum (`TypeOrigin`, `TypeTransform`, `TypeSink`), and the describe-protocol types (`DescribeEnvelope`, `DescribeSchema`, `DescribeFlag`, `DescribeExample`) are permanent. They exist at this path with these names. |
| **`pkg/source`** | The `LedgerSource` interface is permanent. It exists at this path with this name. |
| **`registry.yaml` v1** | The field set and types defined by the v1 schema are permanent. External registries may author YAML against this schema. |
| **`--describe-json` protocol** | Every processor binary must implement a `--describe-json` flag that prints a [`DescribeEnvelope`](../pkg/processor/describe.go) to stdout and exits 0. The envelope shape is permanent; nebu may add new optional fields in minor releases but will not rename or remove existing ones without a major version bump. |

**"Permanent"** means: these symbols will never be renamed, moved, or removed without a major version bump. At 1.0, their _signatures_ also become frozen under semver. As of the streams-never-throw landing (see below), **no further breaking changes to the stable surface are planned before 1.0**.

### Enforcement

The promises above are mechanically enforced by a CI check ([`.github/workflows/api-stability.yml`](../.github/workflows/api-stability.yml)) that runs on every pull request touching `pkg/processor` or `pkg/source`. The check compares the live `go doc -all` output of each stable package against committed snapshots in [`.api/`](../.api/) and fails if they differ.

When the check fails, the PR diff shows exactly which symbol or signature changed. The expected workflow on intentional contract changes is:

1. Verify the change is acceptable per this document.
2. Run `make api-snapshot` locally to regenerate the snapshots.
3. Commit the updated `.api/` files alongside the code change.

The reviewer then sees both the code change and the snapshot diff in the PR, giving a clear "this PR is moving the contract surface" signal that's hard to miss in code review.

This is the lightweight enforcement layer until a future release where `pkg/processor` may be extracted into its own Go module (which would let semver itself enforce contract stability). Until then, the snapshot check is the substitute.

## Error handling: streams-never-throw

`Origin.ProcessLedger`, `Transform.ProcessEvent`, and `Sink.WriteEvent` do not return `error`. The previous behavior — halt the pipeline on the first error — was the wrong default for indexing jobs that run for hours over millions of ledgers: one malformed event shouldn't kill the whole run.

Instead, processors report errors through a `Reporter` attached to the context. Two convenience helpers are provided:

- [`processor.ReportWarning(ctx, name, err)`](../pkg/processor/reporter.go) — per-event error. The pipeline continues.
- [`processor.ReportFatal(ctx, name, err)`](../pkg/processor/reporter.go) — unrecoverable error. The runtime halts the pipeline.

Inside a runtime, the attached reporter logs warnings to stderr, counts them for metrics, and halts on the first fatal. Outside a runtime (standalone CLI, or no reporter attached), a default reporter logs to stderr and exits the process on fatal reports. Tests attach a capturing reporter via `processor.WithReporter(ctx, ...)`.

The `Reporter` interface, `ErrorReport` struct, `Severity` enum, and the two convenience helpers are part of the stable surface.

## Introspection: the `--describe-json` protocol

Every processor binary nebu ships implements a `--describe-json` flag. When invoked, the processor prints a JSON envelope describing itself (name, type, version, schema, flags, examples) to stdout and exits 0 — without running any of its actual processing logic.

```bash
$ token-transfer --describe-json | jq .name
"token-transfer"
```

This is the introspection protocol: `nebu describe <name>` shells out to `<name> --describe-json` to fetch the binary's self-description, and tools outside nebu can do the same. Because the envelope is the stable artifact, external processors written in any language can participate in the protocol by emitting a conforming JSON document.

The envelope shape is defined by [`processor.DescribeEnvelope`](../pkg/processor/describe.go):

```json
{
  "name":        "token-transfer",
  "type":        "origin",
  "version":     "0.3.0",
  "description": "Stream token transfer events from Stellar ledgers",
  "schema": {
    "id":     "nebu.token_transfer.v1",
    "output": { /* JSON Schema Draft 2020-12 */ }
  },
  "flags": [
    {"name": "start-ledger", "type": "uint32", "required": true, "description": "..."}
  ],
  "examples": [
    {"comment": "...", "command": "..."}
  ]
}
```

**Required invariants of the protocol:**

- `--describe-json` must succeed without any other flags set, even if the processor normally requires flags for real runs. (Describe is introspection, not execution.)
- The output must be valid JSON — a complete envelope on stdout, nothing else.
- The process must exit 0 after printing.
- Unknown fields added by nebu in later releases must be ignored by parsers — the envelope is forward-compatible.

**Processor authors using `pkg/processor/cli`** get the protocol for free: the helper registers the flag and emits the envelope automatically from the processor's proto type and cobra flag set. Set `SchemaID` on your config (and optionally `InputType`/`OutputType` for transforms and sinks) to populate the schema section.

**Authoring in another language:** implement the flag yourself. Parse `os.Args` for `--describe-json` before any flag validation, build a JSON object matching the envelope shape above, print it, and exit 0. The full language-agnostic contract — process model, wire format, error reporting, and distribution — is specified in [PROCESSOR_CONTRACT.md](PROCESSOR_CONTRACT.md).

## Unstable surfaces

Explicit non-commitments. These can and will change without notice in the 0.x line:

- **`pkg/source/rpc`, `pkg/source/storage`** — concrete `LedgerSource` implementations. Constructor signatures, option types, and defaults can change.
- **`pkg/runtime`** — the execution engine is a stub. Expect `Runtime` to grow methods, fields, hooks, and configuration.
- **`pkg/registry` (Go API)** — the `registry.yaml` protocol is stable; the Go parser that reads it isn't. Import `pkg/registry` at your own risk.
- **`cmd/nebu` CLI flags and subcommands** — the CLI is a product UI, not a contract. Flags can be renamed, subcommands can be reshaped, output formats can change.
- **`examples/processors/*`** — reference implementations, not API. They exist to show contributors what good processors look like.
- **`pkg/errors`, `pkg/logging`, `pkg/metrics`, `pkg/toid`, `pkg/version`** — internal utilities. Not intended for external use.

## Deprecation process

When we need to remove something from the stable surface (rare, and only at major version boundaries):

1. Add a `// Deprecated:` godoc comment pointing to the replacement, in the same release the replacement ships.
2. The deprecated symbol keeps working for **at least two minor releases**, or until the next major version, whichever comes later.
3. Removal only happens in a major version.

```go
// Deprecated: Use NewLedgerSource instead. This constructor will be removed in v2.
func NewRPCLedgerSource(url string) (*LedgerSource, error) { ... }
```

## How to depend on nebu

### Implementing an external processor

Import only `pkg/processor` and `pkg/source`. Do not import `pkg/runtime`, `pkg/registry`, `pkg/source/rpc`, or `pkg/source/storage` — none of those are part of the committed surface.

```go
import (
    "github.com/withObsrvr/nebu/pkg/processor"
    "github.com/withObsrvr/nebu/pkg/source"
)
```

Binary size stays tight: `pkg/processor` + `pkg/source` pull in only `context`, `google.golang.org/protobuf`, and `github.com/stellar/go-stellar-sdk/xdr`. Your processor picks its own concrete `LedgerSource` (RPC, storage, or a custom implementation) and only pays the dependency cost of what it uses.

### Authoring an external registry

Emit valid `registry.yaml` v1. Do not depend on nebu's Go types — the YAML _is_ the contract. nebu's `pkg/registry` parser is the consumer, not the protocol. You should be able to author a registry in any language that can write YAML.

### Consuming nebu as a Go library

If you only use the stable surface, you're covered by this policy. If you import anything else (`pkg/runtime`, `pkg/registry`, CLI internals), pin to an exact version — those can change without notice.

## The path to 1.0

1.0 is the point at which the committed surface locks under full semver. It ships when all of the following are true:

- The contract interfaces have been exercised by at least one external processor written by someone who didn't write nebu.
- `registry.yaml` v1 has been exercised by at least one external registry.
- The streams-never-throw migration is complete.
- A CI check exists that fails the build on breaking changes to any stable surface.

Until then, the 0.x line is the laboratory. The stable surface is the part of the laboratory we've promised not to rearrange.

## Reporting stability violations

If you find a stable-surface symbol that was renamed, removed, or had its signature changed outside the streams-never-throw exception, file an issue. It's a bug and we'll revert it.
