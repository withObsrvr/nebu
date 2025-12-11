# Shape: Basic CLI - Run & New Commands

**Appetite:** 3 days
**Status:** Ready to bet
**Depends on:** Token Transfer Origin Processor

---

## Problem

Right now you have to write Go programs to use nebu. Developers want:
- "Just run this processor on these ledgers"
- "Scaffold me a new processor"
- Simple commands, not coding

We need the bare minimum CLI that makes nebu feel like a real tool, not a library.

---

## Solution (Fat-Marker Sketch)

Build `cmd/nebu/main.go` with **two** commands only:

**`nebu run origin <name>`**
- Hardcoded to run `token_transfer` origin (only one we have)
- Flags: `--rpc-url`, `--start-ledger`, `--end-ledger`
- Output: Prints events as JSON to stdout
- That's it. No config files, no pipeline specs yet.

**`nebu new processor <name>`**
- Creates a directory: `processors/examples/<name>/`
- Generates:
  - `processor.go` skeleton (implements Origin or Transform)
  - `proto/<name>.proto` template
  - `manifest.yaml` with placeholders
  - Simple README
- Uses Go templates for code gen
- Interactive prompt: "Origin, Transform, or Sink?" → generates appropriate skeleton

---

## Rabbit Holes

**Don't build a registry yet** - Hardcode `token_transfer` for `nebu run`. When we have more processors, we can add discovery.

**Don't build pipeline YAML support** - One processor at a time is enough. Chaining comes later.

**Don't make `new` too smart** - Simple templates that developers edit. No interactive wizard beyond processor type.

**Don't add configuration management** - Flags only. No config files, no environment-based profiles.

---

## No-Gos

- ❌ No `nebu run pipeline` command
- ❌ No registry/install/search commands
- ❌ No processor discovery (hardcoded list is fine)
- ❌ No config file support
- ❌ No interactive mode beyond processor type selection
- ❌ No validation of generated code (they'll see compile errors)

---

## Done Looks Like

**Running the token transfer origin:**
```bash
nebu run origin token_transfer \
  --rpc-url https://mainnet.sorobanrpc.com \
  --start-ledger 58155263 \
  --end-ledger 58155280
```

Output:
```json
{"event_type":"transfer","from":"GABC...","to":"GDEF...","amount":"100.00","asset":"USDC:GA5Z..."}
{"event_type":"fee","from":"GABC...","amount":"0.0001","asset":"native"}
...
```

**Creating a new processor:**
```bash
nebu new processor usdc-filter
# What type of processor? [origin/transform/sink]: transform
# Created processors/examples/usdc-filter/
# Next steps:
#   1. Edit processors/examples/usdc-filter/proto/usdc_filter.proto
#   2. Run: make proto
#   3. Implement processors/examples/usdc-filter/processor.go
```

Directory created:
```
processors/examples/usdc-filter/
├── processor.go          (skeleton with TODOs)
├── proto/
│   └── usdc_filter.proto (template service definition)
├── manifest.yaml         (filled with placeholders)
└── README.md             (simple usage guide)
```

**Help output:**
```bash
$ nebu --help
nebu - modular streaming runtime for Stellar

Commands:
  run       Run processors against Stellar RPC
  new       Scaffold new processors
  help      Show help

$ nebu run --help
nebu run origin <name> - Run an origin processor

Flags:
  --rpc-url string       Stellar RPC endpoint
  --start-ledger uint    Start ledger (required)
  --end-ledger uint      End ledger (required)
```

---

## Scope Line

### MUST HAVE ════════════════
- `nebu run origin token_transfer` command
- JSON output to stdout
- Required flags: rpc-url, start-ledger, end-ledger
- `nebu new processor` command
- Templates for origin/transform/sink skeletons
- Generated files compile (even if TODOs remain)
- Help text for all commands

### NICE TO HAVE ─────────────
- Pretty colored output option (vs raw JSON)
- Validation of ledger ranges before running
- Progress indicator (e.g., "Processing ledger 58155270/58155280")
- More detailed help/examples in CLI output

### COULD HAVE ───────────────
- Shell completion
- Verbose/debug logging flag
- Output format options (JSON, CSV, etc.)
- Dry-run mode

---

## Notes

This CLI is intentionally limited. It does **two things well**:
1. Makes it trivial to run the token transfer processor
2. Makes it trivial to start building a new processor

Everything else (pipelines, registry, discovery) comes later after we validate these primitives work.

The `new` command is especially important - it's how the community will contribute processors. The templates need to be clean and well-commented.

---

## Success Metric

A developer who's never seen nebu before can:
1. Install it: `go install github.com/withObsrvr/nebu/cmd/nebu@latest`
2. Run it: `nebu run origin token_transfer --start-ledger 58155263 --end-ledger 58155280 --rpc-url https://mainnet.sorobanrpc.com`
3. See events stream in <30 seconds
4. Scaffold a new processor: `nebu new processor my-thing`
5. Understand what to edit next from the generated code comments

If we nail this, we've got the foundation for a real developer tool.
