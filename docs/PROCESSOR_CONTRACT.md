# nebu Processor Contract (v1)

**Status:** normative. This document defines what a binary must do to be a
nebu processor — in any implementation language. It consolidates the
behavioral rules scattered across [STABILITY.md](STABILITY.md) and
[REGISTRY_SPEC.md](REGISTRY_SPEC.md) into a single language-agnostic spec.

The key words MUST, MUST NOT, SHOULD, and MAY are used as described in
RFC 2119.

A processor that satisfies this contract composes into any nebu pipeline:

```
ledger source -> origin -> transform(s) -> sink
```

Go authors get most of this for free from `pkg/processor` and
`pkg/processor/cli`. Authors in other languages implement it directly —
it is deliberately small. The reference non-Go implementation is
[`webhook-sink`](https://github.com/withObsrvr/nebu-processor-registry/tree/main/processors/webhook-sink)
(TypeScript, compiled with Bun).

## 1. Process model

- A processor is a single self-contained executable. Its file name MUST be
  kebab-case and MUST match the `name` in its registry entry and describe
  envelope.
- **stdout is the data plane.** Only NDJSON events (§2) may be written to
  stdout — plus the describe envelope when invoked with `--describe-json`
  (§3). Banners, logs, progress, warnings, and diagnostics MUST go to
  stderr.
- Exit code 0 means the stream completed cleanly (input EOF reached, or a
  bounded range finished). Any unrecoverable error MUST produce a nonzero
  exit code and a human-readable message on stderr. Processors MAY use
  exit code 2 for usage errors (bad flags).
- On SIGINT or SIGTERM the processor MUST stop promptly; sinks and
  transforms SHOULD flush buffered events first, then exit 0 if the flush
  succeeded. When stdout closes (downstream pipe exits), the processor
  MUST exit quietly rather than log an error storm — the default SIGPIPE
  behavior is acceptable.
- Long-running loops (ledger tailing, batch flushing, HTTP retries) MUST
  remain responsive to termination; do not block signals behind
  uninterruptible waits.

## 2. Wire format

The pipe carries newline-delimited JSON (NDJSON):

- One complete JSON object per line, terminated by `\n`, encoded UTF-8.
- No pretty-printing on the pipe. A multi-line JSON document is a framing
  violation.

**Origins** (produce events from ledger data) MUST stamp every event with:

| Field | Meaning |
|---|---|
| `_schema` | Versioned schema identifier, e.g. `nebu.token_transfer.v1`. Bumped on breaking output changes. |
| `_nebu_version` | Version of the emitting processor. |
| `meta` | Object with provenance: `ledgerSequence`, `txHash`, `closedAtUnix`, … |

**Transforms** (events in, events out) MUST preserve `_schema`,
`_nebu_version`, and `meta` on events they pass through. A transform that
reshapes events into a new structure MUST emit its own `_schema`
identifier instead.

**Sinks** (events in, side effects out) MUST NOT echo events to stdout —
fan-out is the shell's job (`tee`). A sink's stdout stays silent.

**Malformed input** follows the "streams never throw" rule: a line that
is not valid JSON MUST NOT crash the processor and MUST NOT be silently
swallowed — report a warning on stderr (at minimum a count at exit) and
continue with the next line.

## 3. Introspection: `--describe-json`

Every processor MUST implement the `--describe-json` flag. Its invariants
(normative, from [STABILITY.md](STABILITY.md)):

- It MUST succeed with no other flags set, even if real runs require
  flags. Describe is introspection, not execution: no network calls, no
  processing side effects.
- The output MUST be a single valid JSON envelope on stdout, nothing
  else, followed by exit code 0.
- Parse for the flag before general flag validation (in Go,
  `pkg/processor/cli` does this; elsewhere, scan argv first).

Envelope shape:

```json
{
  "name":        "webhook-sink",
  "type":        "sink",
  "version":     "0.1.0",
  "description": "POST nebu events to an HTTP endpoint",
  "schema": {
    "input": { "type": "object" }
  },
  "flags": [
    {"name": "url", "type": "string", "required": true, "description": "..."}
  ],
  "examples": [
    {"comment": "...", "command": "..."}
  ]
}
```

| Field | Required | Notes |
|---|---|---|
| `name` | MUST | Matches the binary and registry names. |
| `type` | MUST | `origin`, `transform`, or `sink`. |
| `version` | MUST | Processor version string. |
| `description` | MUST | One-line summary. |
| `schema` | SHOULD | `id` + `output` (JSON Schema) for origins/transforms; `input` for transforms/sinks. A generic sink MAY declare `{"input": {"type": "object"}}`. |
| `flags` | SHOULD | One entry per CLI flag: `name`, `type`, `required`, `description`. |
| `examples` | MAY | Curated `comment` + `command` pairs. |

Consumers MUST ignore unknown envelope fields; nebu may add fields in
minor releases.

## 4. Flags and environment

- Flag names are kebab-case (`--start-ledger`, `--batch-size`).
- `--help` MUST exist and print usage to stdout or stderr, exit 0.
- Processors that print a startup banner MUST send it to stderr and
  SHOULD offer `-q` / `--quiet` to suppress it.
- Secrets MUST come from environment variables, never flags in examples
  (e.g. `NEBU_RPC_AUTH` for origins). Document every env var the
  processor reads.

## 5. Distribution and registry

A community processor is published by adding a directory with a
`description.yml` to a registry repo (see
[REGISTRY_SPEC.md](REGISTRY_SPEC.md)).

- Go processors are installed via `go install` from the entry's derived
  module path.
- Processors in any other language publish prebuilt per-platform binaries
  and declare an `install` block:

```yaml
install:
  kind: binary
  url: https://github.com/OWNER/REPO/releases/download/NAME-v{version}/NAME-{os}-{arch}{exe}
  checksums: https://github.com/OWNER/REPO/releases/download/NAME-v{version}/checksums.txt
```

- `{os}` and `{arch}` expand to Go's GOOS/GOARCH names (`linux`,
  `darwin`, `windows` × `amd64`, `arm64`) and `{exe}` to `.exe` on
  Windows, empty elsewhere; name release artifacts accordingly.
- A `checksums.txt` in `sha256sum` format (`<hex>  <filename>`) MUST be
  published with the artifacts. `nebu install` refuses to install a
  binary whose digest does not match.
- After download, `nebu install` runs `<binary> --describe-json` as a
  conformance gate — a binary that fails §3 fails installation.

## 6. Conformance checklist

Run these against a candidate binary before publishing:

```bash
# 1. Describe works bare, emits one JSON doc, exits 0
./my-proc --describe-json | jq -e .name && echo OK

# 2. stdout stays clean: a bounded run emits only parseable NDJSON
./my-proc <flags> 2>/dev/null | while read -r line; do
  printf '%s\n' "$line" | jq -e . >/dev/null || { echo "FRAMING VIOLATION: $line"; exit 1; }
done

# 3. Malformed input does not kill a transform/sink (expect warning on
#    stderr, exit 0 at EOF)
printf 'not json\n{"_schema":"nebu.test.v1","x":1}\n' | ./my-proc <flags>

# 4. SIGINT exits promptly and cleanly
./my-proc <flags> & sleep 2; kill -INT $!; wait $!
```

## See also

- [STABILITY.md](STABILITY.md) — stability commitments, the full envelope
  specification, and the Go API surface.
- [REGISTRY_SPEC.md](REGISTRY_SPEC.md) — `registry.yaml` and
  `description.yml` formats, including the `install` block.
- [BUILDING_PROCESSORS.md](BUILDING_PROCESSORS.md) — the Go authoring
  guide.
