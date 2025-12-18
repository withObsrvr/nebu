# Unix Philosophy Review: Avoiding Magic in nebu

This document analyzes nebu against the Unix philosophy principle of "avoid magic" - behavior should be explicit, predictable, and inspectable rather than implicit or heuristic-based.

## What is "Magic"?

Based on Unix philosophy, "magic" includes:
- **Implicit behavior**: Things happening without explicit user action
- **Hidden state**: Configuration or state that isn't visible
- **Heuristics**: Automatic detection or guessing of intent
- **Schema inference**: Automatically detecting data formats
- **Smart defaults**: Reasonable but hidden assumptions
- **Auto-detection**: Figuring out what the user wants

The Unix approach prefers **explicit over implicit** and **visible over hidden**.

---

## Current "Magic" in nebu

### 1. Stdin Auto-Detection

**Location**: `cmd/nebu/run.go:84-88`

```go
// Auto-detect stdin
stat, _ := os.Stdin.Stat()
if (stat.Mode() & os.ModeCharDevice) == 0 {
    // stdin is a pipe
    useStdin = true
}
```

**Analysis**:
- ❌ **Implicit behavior** - Automatically switches to stdin mode without user specifying
- The mode is chosen via OS-level heuristics (checking if stdin is a character device)
- User doesn't explicitly say "read from stdin", the program guesses

**Unix Philosophy Violation**: Medium
- Users might be surprised when behavior changes based on whether input is piped
- Hard to predict in scripts without understanding the heuristic

**Recommendation**:
Make input mode explicit:
```bash
# Current (magic):
cat ledgers.xdr | nebu run origin token-transfer

# Better (explicit):
nebu run origin token-transfer --input stdin < ledgers.xdr
# or
nebu run origin token-transfer --input -
```

**Mitigation**: The `-` (dash) argument does work explicitly (`cmd/nebu/run.go:77`), so users can opt out of magic. The auto-detection is a convenience, not the only way.

---

### 2. Default RPC URLs

**Location**: `cmd/nebu/run.go:157`, `cmd/nebu/fetch.go:83`

```go
cmd.Flags().StringVar(&rpcURL, "rpc-url", "https://archive-rpc.lightsail.network", "Stellar RPC endpoint")
cmd.Flags().StringVar(&networkPass, "network", network.PublicNetworkPassphrase, "Network passphrase")
```

**Analysis**:
- ⚠️ **Smart defaults** - Assumes mainnet when user doesn't specify
- The default is reasonable (most users want mainnet)
- BUT: Makes it easy to accidentally hit mainnet when testing

**Unix Philosophy Violation**: Low
- Default is documented in `--help`
- Users can override with flags
- Not really "hidden" - it's explicit in the interface

**Recommendation**:
Current approach is acceptable, but consider for production deployments:
```bash
# Could require explicit network:
nebu run origin token-transfer --network mainnet --start 100 --end 200

# Instead of allowing default:
nebu run origin token-transfer --start 100 --end 200  # implies mainnet
```

**Decision**: Keep as-is. This is a reasonable default that's documented and overridable. Requiring `--network` every time would be tedious without providing much safety benefit.

---

### 3. Hardcoded Buffer Sizes

**Location**: Multiple locations with `1024`

```go
// pkg/runtime/runner.go:47
ledgerCh := make(chan xdr.LedgerCloseMeta, 128)

// cmd/nebu/fetch.go:114
ledgerCh := make(chan xdr.LedgerCloseMeta, 128)

// examples/processors/token-transfer/processor.go:26
emitter: processor.NewEmitter[*token_transfer.TokenTransferEvent](1024),
```

**Analysis**:
- ⚠️ **Hidden behavior** - Buffer sizes affect backpressure and memory usage
- Users cannot control these values
- Different parts of the system use different buffer sizes (128 vs 1024)

**Unix Philosophy Violation**: Low-Medium
- Buffer sizes are implementation details that users rarely need to control
- BUT: For high-volume production use, users might want to tune these
- Not discoverable via `--help` or environment variables

**Recommendation**:
For runtime buffer sizes, consider making them configurable:
```go
// Future: Allow tuning via environment or config
bufSize := getEnvInt("NEBU_BUFFER_SIZE", 128)
ledgerCh := make(chan xdr.LedgerCloseMeta, bufSize)
```

**Decision**: Low priority. Buffer sizes are reasonable defaults and most users won't need to change them. If performance tuning becomes important, add environment variables for advanced users.

---

### 4. Schema Simplification (Protobuf → JSON)

**Location**: `cmd/nebu/run.go:302-363`, `pkg/processor/cli/origin.go:229-289`

```go
func simplifyTokenTransferEvent(ev *token_transfer.TokenTransferEvent) map[string]interface{} {
    meta := ev.GetMeta()
    event := map[string]interface{}{
        "ledger_sequence": meta.LedgerSequence,
        "tx_hash":         meta.TxHash,
    }

    // Handle asset
    if asset := ev.GetAsset(); asset != nil {
        assetInfo := make(map[string]string)
        if asset.GetNative() {
            assetInfo["code"] = "native"
        } else if issued := asset.GetIssuedAsset(); issued != nil {
            assetInfo["code"] = issued.AssetCode
            assetInfo["issuer"] = issued.Issuer
        }
        event["asset"] = assetInfo
    }

    // Handle different event types via switch...
}
```

**Analysis**:
- ✅ **Acceptable transformation** - Converting protobuf to JSON is necessary for CLI output
- ❌ **Schema loss** - Original protobuf structure is flattened
- ⚠️ **No schema versioning** - JSON output doesn't indicate format version
- Field selection is opinionated (omits some protobuf fields)

**Unix Philosophy Violation**: Medium
- Output schema is implicit, not declared
- Users don't know if fields might be added/removed in future versions
- Downstream consumers (jq, DuckDB) rely on undocumented schema

**Recommendation**:
1. Add schema version to output:
```json
{
  "_schema": "nebu.token_transfer.v1",
  "type": "transfer",
  "ledger_sequence": 60200000,
  ...
}
```

2. Document the JSON schema in processor README
3. Consider supporting `--format protobuf` for lossless output

**Example**:
```bash
# Current (implicit JSON):
nebu run origin token-transfer --start 100 --end 200

# Better (explicit format):
nebu run origin token-transfer --start 100 --end 200 --format json
nebu run origin token-transfer --start 100 --end 200 --format protobuf
```

---

### 5. JSON Output Default

**Location**: `cmd/nebu/run.go:161`

```go
cmd.Flags().BoolVar(&jsonOutput, "json", true, "Output events as JSON (default true)")
```

**Analysis**:
- ⚠️ **Implicit output format** - Defaults to JSON without user specifying
- The alternative (`--json=false`) just prints event summaries, not raw protobuf
- No way to get raw protobuf output from CLI

**Unix Philosophy Violation**: Low
- JSON is a reasonable default for CLI tools
- BUT: Doesn't provide lossless output option
- Flag naming is slightly misleading (it's not "json vs protobuf", it's "json vs summary")

**Recommendation**:
```bash
# Current:
--json=true   # JSON output
--json=false  # Summary output

# Better:
--format json      # JSON output (default)
--format summary   # Human-readable summary
--format protobuf  # Raw protobuf (future)
```

---

### 6. Registry Auto-Discovery

**Location**: `pkg/registry/registry.go:72-88`

```go
func LoadDefault() (*Registry, error) {
    // Look for registry.yaml in current directory or parent directories
    paths := []string{
        "registry.yaml",
        "../registry.yaml",
        "../../registry.yaml",
    }

    for _, path := range paths {
        if _, err := os.Stat(path); err == nil {
            return Load(path)
        }
    }

    return nil, fmt.Errorf("registry.yaml not found")
}
```

**Analysis**:
- ⚠️ **Implicit search** - Walks up directory tree looking for registry.yaml
- User doesn't specify registry location
- Behavior depends on current working directory

**Unix Philosophy Violation**: Low-Medium
- Similar to how git searches for `.git` directory
- Common pattern in dev tools
- BUT: Could be surprising if wrong registry is found

**Recommendation**:
Add explicit override:
```bash
# Current (magic):
cd /some/dir && nebu list  # finds registry.yaml via search

# Better (explicit):
nebu list --registry /path/to/registry.yaml
nebu list --registry $NEBU_REGISTRY
```

**Decision**: Low priority. The search pattern is conventional (like git, npm, etc.) and fails fast with a clear error. Adding `--registry` flag for explicit control is a good enhancement but not critical.

---

### 7. Network Passphrase Inference

**Location**: `cmd/nebu/run.go:160`

```go
cmd.Flags().StringVar(&networkPass, "network", network.PublicNetworkPassphrase, "Network passphrase")
```

**Analysis**:
- ⚠️ **Hidden coupling** - Network passphrase default must match RPC URL network
- If user provides `--rpc-url https://testnet.sorobanrpc.com` but doesn't change `--network`, they get mainnet passphrase with testnet RPC
- Easy to create misconfiguration

**Unix Philosophy Violation**: Medium
- The relationship between RPC URL and network passphrase is implicit
- User must remember to set both consistently
- No validation that they match

**Recommendation**:
```go
// Detect network from RPC URL and validate consistency
func validateNetworkConfig(rpcURL, networkPass string) error {
    if strings.Contains(rpcURL, "testnet") && !strings.Contains(networkPass, "Test") {
        return fmt.Errorf("RPC URL appears to be testnet but network passphrase is for mainnet")
    }
    return nil
}
```

Or better: infer network from RPC URL if not specified:
```bash
# Explicit is better:
nebu run origin token-transfer --rpc-url https://testnet.sorobanrpc.com --network testnet ...

# But if we must have magic, make it safe:
# Auto-detect "testnet" in URL → use test network passphrase
```

---

## Non-Magic Patterns (Good Examples)

### 1. Explicit Ledger Ranges

```bash
nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200100
```

✅ User explicitly specifies the data range, no guessing or defaults

### 2. Explicit Processor Types

```go
cmd.Flags().StringVar(&processorName, "processor", "", "Processor name (required)")
```

✅ User must specify which processor to run, no auto-detection

### 3. Explicit Registry Format

```yaml
version: 1
processors:
  - name: token-transfer
    type: origin
    location:
      type: local
      path: ./examples/processors/token-transfer
```

✅ All processor metadata is explicit in YAML, no convention-based discovery

### 4. Explicit Protobuf Definitions

```protobuf
message TransformRequest {
  bytes event_json = 1;
}
```

✅ All data contracts are defined in `.proto` files, no schema inference

### 5. Explicit Channel Buffer Sizes

```go
func NewEmitter[T proto.Message](bufSize int) *Emitter[T] {
    return &Emitter[T]{
        ch: make(chan T, bufSize),
    }
}
```

✅ Buffer size is a required parameter, not a hidden default (though call sites use magic number 1024)

---

## Summary Table

| Feature | Magic Level | Severity | Recommendation |
|---------|------------|----------|----------------|
| Stdin auto-detection | High | Medium | Keep `-` explicit option, document auto-detect behavior |
| Default RPC URLs | Low | Low | Keep as-is, documented in --help |
| Hardcoded buffer sizes | Medium | Low | Add env vars for tuning if needed |
| Schema simplification | Medium | Medium | Add `_schema` version field, document JSON format |
| JSON output default | Low | Low | Consider `--format` flag for clarity |
| Registry auto-discovery | Medium | Low-Medium | Add `--registry` flag for explicit override |
| Network passphrase inference | Medium | Medium | Validate consistency or infer from RPC URL |

---

## Recommendations

### High Priority (Reduce Significant Magic)

1. **Add schema versioning to JSON output**
   - Include `_schema` field in all JSON events
   - Document JSON schema in processor READMEs
   - This prevents silent breakage when output format changes

2. **Validate network configuration**
   - Check that RPC URL and network passphrase are consistent
   - Fail fast with clear error message if misconfigured

### Medium Priority (Improve Explicitness)

3. **Add `--registry` flag**
   - Allow explicit registry path override
   - Support `NEBU_REGISTRY` environment variable
   - Keep auto-discovery as fallback

4. **Document auto-detection behavior**
   - Clearly document stdin auto-detection in `--help` and README
   - Emphasize `-` for explicit stdin specification

### Low Priority (Polish)

5. **Make buffer sizes configurable**
   - Add `NEBU_BUFFER_SIZE` environment variable
   - Only needed for advanced performance tuning

6. **Replace `--json` with `--format`**
   - More extensible for future output formats
   - Clearer semantics

---

## Conclusion

Overall, nebu follows Unix philosophy fairly well:

**Strengths**:
- ✅ Explicit ledger ranges (no "smart" defaults)
- ✅ Explicit processor selection (no auto-detection)
- ✅ Explicit protobuf schemas (no schema inference)
- ✅ Clear separation of concerns (source, processor, runtime)
- ✅ Composable via Unix pipes

**Areas for Improvement**:
- ⚠️ Stdin auto-detection adds implicit behavior (but `-` opt-out exists)
- ⚠️ JSON schema is undocumented and unversioned
- ⚠️ Network configuration can be inconsistent
- ⚠️ Some defaults are reasonable but could be more explicit

**Philosophy Alignment**: 7/10

The magic that exists is mostly "training wheels" magic (convenience defaults) rather than "surprising" magic (hidden heuristics). Users can generally understand and override the defaults. The main improvements should focus on:
1. Making output schemas explicit and versioned
2. Validating configuration consistency
3. Providing explicit overrides for convenience features
