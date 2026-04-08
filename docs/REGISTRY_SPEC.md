# nebu Registry Specification (v1)

**Status:** v1 is stable. See [STABILITY.md](STABILITY.md) for the full stability commitment.

This document formally defines the two YAML formats that describe nebu processors: `registry.yaml` (the built-in catalog shipped with nebu) and `description.yml` (the entry format used by external registries). Both are stable v1 surfaces; parsers must ignore unknown fields so the format can grow in minor releases without breaking consumers.

## Why two formats?

| Format | Lives in | Purpose |
|---|---|---|
| **`registry.yaml` v1** | Nebu repo root, embedded into the `nebu` binary, authored by nebu maintainers | A single catalog of all built-in processors with rich curated metadata (events, output fields, works_with, examples). One file, many processors. |
| **`description.yml` v1** | External registry repos (like [nebu-processor-registry](https://github.com/withObsrvr/nebu-processor-registry)), one directory per processor | Lightweight pointer entry for a community-authored processor. References a GitHub repo where the processor code lives; the processor binary itself is the source of truth for schemas and flags via `--describe-json`. |

The two formats are intentionally different. `registry.yaml` is a curated catalog; `description.yml` is a pointer. Neither duplicates the other, and `nebu describe` merges them at runtime.

## `registry.yaml` v1

Lives at the root of a nebu installation (or is embedded into the nebu binary). Contains one or more processor entries, each describing a local or community processor.

### Top-level structure

```yaml
version: 1
processors:
  - name: token-transfer
    type: origin
    # ... see Processor Entry below
```

| Field | Type | Required | Description |
|---|---|---|---|
| `version` | int | **yes** | Schema version. Must be `1` for this spec. |
| `processors` | list of Processor Entry | yes | Ordered list of processor entries. Order is preserved and honored by `nebu list`. |

### Processor Entry

```yaml
- name: token-transfer
  type: origin
  description: Stream token transfer events from Stellar ledgers
  long_description: |
    Extracts Stellar Asset Contract (SAC) token events from ledger data.
    Produces events for transfers, mints, burns, clawbacks, and network fees.

  location:
    type: local
    path: ./examples/processors/token-transfer
    package: github.com/withObsrvr/nebu/examples/processors/token-transfer
    module_package: github.com/withObsrvr/nebu/examples/processors/token-transfer/cmd/token-transfer

  proto:
    source: github.com/stellar/go-stellar-sdk/protos/processors/token_transfer
    package: token_transfer

  schema:
    version: v1
    identifier: nebu.token_transfer.v1
    documentation: ./examples/processors/token-transfer/SCHEMA.md

  events:
    - name: transfer
      description: Token sent between accounts
    - name: mint
      description: New tokens created

  output_fields:
    - path: meta.ledgerSequence
      description: Ledger number

  examples:
    - comment: Basic usage
      command: token-transfer --start-ledger 60200000 --end-ledger 60200100

  works_with:
    input: [nebu fetch]
    transforms: [usdc-filter, amount-filter, dedup]
    sinks: [json-file-sink, postgres-sink]

  maintainer:
    name: OBSRVR
    url: https://withobsrvr.com
```

**Required fields:**

| Field | Type | Description |
|---|---|---|
| `name` | string | Unique processor name. Must match the binary name. Kebab-case. |
| `type` | enum: `origin`, `transform`, `sink` | Processor role in a pipeline. |
| `description` | string | One-line human-readable summary. |
| `location` | Location | Where the processor source or binary lives. See below. |

**Optional fields:**

| Field | Type | Description |
|---|---|---|
| `long_description` | string | Multi-line prose for the `nebu describe` DESCRIPTION section. |
| `proto` | ProtoConfig | Protocol buffer descriptor, for processors that emit typed events. |
| `schema` | SchemaConfig | Schema versioning metadata. Prefer the processor's own `--describe-json` output at runtime; this is for static catalogs. |
| `events` | list of EventInfo | Event-type enumeration (for origins emitting oneof events). |
| `output_fields` | list of FieldInfo | Top-level field paths the processor emits, for discoverability. |
| `examples` | list of ExampleInfo | Curated command-line examples with comments. |
| `works_with` | WorksWithInfo | Compatibility hints for the pipeline-builder UI. |
| `manifest` | string | Path to a `manifest.yaml` file with additional metadata. |
| `maintainer` | MaintainerInfo | Who maintains the processor. |

### Location

Describes where a processor's code or binary can be found. Three `type` values are recognized:

```yaml
# Local: source lives in the nebu repo.
location:
  type: local
  path: ./examples/processors/token-transfer
  package: github.com/withObsrvr/nebu/examples/processors/token-transfer
  module_package: github.com/withObsrvr/nebu/examples/processors/token-transfer/cmd/token-transfer
```

```yaml
# Module: go-installable from an external repo.
location:
  type: module
  module_package: github.com/someuser/nebu-processor-foo/cmd/foo
```

```yaml
# Git: raw repository reference (less common; prefer module).
location:
  type: git
  url: https://github.com/someuser/nebu-processor-foo
  path: cmd/foo
```

| Field | Type | Notes |
|---|---|---|
| `type` | enum: `local`, `module`, `git` | How nebu should resolve the source. |
| `path` | string | For `local` or `git`: relative path to the package directory. |
| `package` | string | Go package path (documentation only). |
| `module_package` | string | Go module path used by `nebu install` to run `go install`. |
| `url` | string | For `git`: repository URL. |

### SchemaConfig

```yaml
schema:
  version: v1
  identifier: nebu.token_transfer.v1
  documentation: ./examples/processors/token-transfer/SCHEMA.md
```

| Field | Type | Notes |
|---|---|---|
| `version` | string | Semantic schema version (e.g., `v1`, `v2`). Bumped on breaking output changes. |
| `identifier` | string | Canonical schema ID. Usually matches the `SchemaID` field the processor declares in its CLI config and the `_schema` field in emitted events. |
| `documentation` | string | Path or URL to human-readable schema docs. |

At runtime, the processor's own `--describe-json` output is the source of truth for its schema. This registry entry is a static pointer used when the binary isn't installed.

### Other sub-types

**`ProtoConfig`** — optional, documents the proto file origin:
```yaml
proto:
  source: github.com/stellar/go-stellar-sdk/protos/processors/token_transfer
  package: token_transfer
```

**`EventInfo`** — for origin processors with oneof event types:
```yaml
events:
  - name: transfer
    description: Token sent between accounts
```

**`FieldInfo`** — top-level output field documentation:
```yaml
output_fields:
  - path: meta.ledgerSequence
    description: Ledger number
```

**`ExampleInfo`** — curated command examples:
```yaml
examples:
  - comment: Basic usage
    command: token-transfer --start-ledger 60200000 --end-ledger 60200100
```

**`WorksWithInfo`** — compatibility hints:
```yaml
works_with:
  input: [nebu fetch]
  transforms: [usdc-filter, amount-filter]
  sinks: [json-file-sink, postgres-sink]
```

**`MaintainerInfo`** — ownership:
```yaml
maintainer:
  name: OBSRVR
  url: https://withobsrvr.com
```

## `description.yml` v1

Lives one per directory in an external registry repo (e.g. `processors/my-processor/description.yml` in `nebu-processor-registry`). Lighter than `registry.yaml` because the rich information (schema, flags, examples) is fetched live from the processor binary via `--describe-json`.

### Structure

```yaml
processor:
  name: my-processor
  type: origin
  description: Extract foo events from Stellar
  version: 1.0.0
  language: Go
  license: MIT
  maintainers:
    - username

repo:
  github: username/nebu-processor-my-thing
  ref: v1.0.0

proto:
  source: github.com/username/nebu-processor-my-thing/proto
  package: my_processor

schema:
  version: v1
  identifier: nebu.my_processor.v1
  documentation: https://github.com/username/nebu-processor-my-thing/blob/main/SCHEMA.md

docs:
  quick_start: |
    nebu install my-processor
    my-processor --start-ledger 60200000 --end-ledger 60200100

  examples: |
    my-processor --start-ledger 60200000 --end-ledger 60200100 | jq

  extended_description: |
    Longer prose describing what the processor does...
```

| Section | Required | Description |
|---|---|---|
| `processor` | **yes** | Identity and metadata. |
| `repo` | **yes** | Where the source lives. |
| `proto` | no | Proto descriptor pointer. |
| `schema` | no | Schema ID and docs pointer. |
| `docs` | no | Quick-start, examples, and extended prose (rendered by `nebu describe`). |

**`processor` block:**

| Field | Type | Required |
|---|---|---|
| `name` | string | yes |
| `type` | enum: `origin`, `transform`, `sink` | yes |
| `description` | string | yes |
| `version` | string | yes |
| `language` | string | no (for humans; external-language support is future work) |
| `license` | string | no |
| `maintainers` | list of string | no |
| `deprecated` | bool | no — set to `true` to mark for removal |
| `deprecation_notice` | string | no — migration guidance |

**`repo` block:**

| Field | Type | Required |
|---|---|---|
| `github` | string | yes — in `owner/repo` form |
| `ref` | string | yes — git tag (preferred), branch, or commit SHA |

### How external registries are merged

When a user runs `nebu list` or `nebu describe`, nebu loads the external registry URL from `NEBU_REGISTRY` (or the default, `github.com/withObsrvr/nebu-processor-registry`), fetches every `description.yml` under `processors/`, and merges them into the local `registry.yaml` — embedded entries win on name collisions. Each external processor becomes a `ProcessorEntry` of `type: module` with its `module_package` synthesized from `repo.github` and the processor name.

This merge is cached on disk at `~/.cache/nebu/registries/<hash>/` with a default 1-hour TTL.

## Fetching live metadata with `--describe-json`

Every nebu-compatible processor binary implements a `--describe-json` flag that prints a JSON envelope describing the processor and exits 0. This is how `nebu describe <name>` gets the authoritative picture — registry metadata is a fallback for when the binary isn't installed.

See [STABILITY.md](STABILITY.md) for the full envelope specification.

```json
{
  "name":        "token-transfer",
  "type":        "origin",
  "version":     "0.3.0",
  "description": "Stream token transfer events from Stellar ledgers",
  "schema": {
    "id":     "nebu.token_transfer.v1",
    "output": { "$schema": "https://json-schema.org/draft/2020-12/schema", ... }
  },
  "flags": [
    { "name": "start-ledger", "type": "uint32", "required": true, "description": "..." }
  ],
  "examples": [ { "comment": "...", "command": "..." } ]
}
```

The envelope and the registry metadata complement each other:

- **Envelope wins** for things the processor is the source of truth about: flags, actual JSON Schema, version string, schema ID.
- **Registry wins** for curated catalog metadata: `long_description`, `events` enumeration, `output_fields`, `works_with`, `maintainer`.

Processor authors writing Go processors get `--describe-json` for free from `pkg/processor/cli`. Non-Go authors must implement it themselves — see "Authoring in another language" in [STABILITY.md](STABILITY.md).

## Parser compatibility

Both `registry.yaml` v1 and `description.yml` v1 follow the same forward-compatibility rules:

- **Adding fields is minor-version safe.** Parsers must ignore unknown fields. Nebu may add fields at any time within the v1 line.
- **Renaming, removing, or retyping fields is a breaking change** and requires a new version number (`version: 2`). A parser that reads v1 must refuse v2 loudly rather than silently misinterpret it.
- **Field ordering is not significant.** YAML maps are unordered; parsers must not depend on the order of keys.
- **Lists preserve order.** `processors`, `events`, `output_fields`, `examples`, `works_with.*`, and `maintainers` all preserve authored order and consumers display them in that order.

## Authoring an external registry

Externally-hosted registries use the `description.yml` format. The minimum viable external registry is a Git repository with:

```
my-registry/
├── README.md
└── processors/
    ├── foo/
    │   └── description.yml
    └── bar/
        └── description.yml
```

Users point nebu at it via the `NEBU_REGISTRY` environment variable:

```bash
NEBU_REGISTRY=github.com/myorg/nebu-registry nebu list
```

The spec repo for the flagship community registry is [withObsrvr/nebu-processor-registry](https://github.com/withObsrvr/nebu-processor-registry), which also hosts the comprehensive [`BUILDING_PROTO_PROCESSORS.md`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/BUILDING_PROTO_PROCESSORS.md) guide for authoring processors.

## See also

- [STABILITY.md](STABILITY.md) — The stability policy covering `pkg/processor`, `pkg/source`, the `--describe-json` protocol, and both registry formats.
- [ARCHITECTURE_DECISIONS.md](ARCHITECTURE_DECISIONS.md) — Why nebu stays CLI-only and composes via Unix pipes.
- `pkg/registry/registry.go` — The Go types that define the `registry.yaml` v1 schema.
- `pkg/registry/external.go` — The Go types that define the `description.yml` v1 schema and the merge logic.
