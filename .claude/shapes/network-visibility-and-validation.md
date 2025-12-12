# Shape: Network Visibility and Validation

**Status**: Ready for Betting
**Appetite**: 1 day
**Created**: 2025-12-12

---

## Problem

When running nebu, users don't know what network they're connected to or what RPC they're using until after the command runs (or fails). This leads to:

1. **Accidental mainnet usage** - Developers testing on testnet data accidentally hit mainnet RPC
2. **Network mismatch** - RPC URL points to testnet but `--network` flag says mainnet
3. **No visibility** - Can't tell from logs what configuration was used
4. **Silent failures** - Wrong network passphrase causes cryptic errors deep in processing

**Current behavior:**
```bash
nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200100

# No output about RPC or network
# Starts processing...
# Error 5 minutes later: "invalid signature" (because network passphrase was wrong)
```

**What users need:**
- See RPC URL, network, and auth status BEFORE processing starts
- Validate that RPC URL and network passphrase are consistent
- Fail fast with clear errors if configuration is wrong

---

## Appetite

**1 day** - This is mostly logging and validation, minimal new functionality.

---

## Solution

### Fat-Marker Sketch

**1. Startup Banner:**

Show configuration before starting work:

```bash
nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200100

# Output:
🚀 nebu run origin token-transfer
   RPC:     https://mainnet.sorobanrpc.com
   Network: Public Global Stellar Network ; September 2015
   Auth:    None
   Range:   60200000 → 60200100

Processing ledgers 60200000 to 60200100...
```

**2. Network Validation:**

Detect common misconfigurations:

```go
func validateNetworkConfig(rpcURL, networkPass string) error {
    // Check for obvious mismatches
    if strings.Contains(rpcURL, "testnet") && networkPass == network.PublicNetworkPassphrase {
        return fmt.Errorf(`
Network configuration mismatch detected:
  RPC URL:  %s (appears to be TESTNET)
  Network:  %s (MAINNET passphrase)

Did you mean to use --network testnet?

To fix:
  nebu run origin token-transfer --rpc-url %s --network testnet ...
Or:
  export NEBU_NETWORK=testnet
`, rpcURL, networkPass, rpcURL)
    }

    if strings.Contains(rpcURL, "mainnet") && networkPass == network.TestNetworkPassphrase {
        return fmt.Errorf(`
Network configuration mismatch detected:
  RPC URL:  %s (appears to be MAINNET)
  Network:  %s (TESTNET passphrase)

Did you mean to use --network mainnet?
`, rpcURL, networkPass)
    }

    return nil
}
```

**3. Environment Variable Support:**

```bash
# Users can set defaults via environment
export NEBU_RPC_URL="https://rpc-testnet.nodeswithobsrvr.co"
export NEBU_NETWORK="testnet"
export NEBU_RPC_AUTH="Api-Key tetsf7WV.Wld1LQL5Qp3CjxffiZRC1rXtz0QQthz4"

# Then run without flags
nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200100

# Output shows config from environment:
🚀 nebu run origin token-transfer
   RPC:     https://rpc-testnet.nodeswithobsrvr.co (from NEBU_RPC_URL)
   Network: Test SDF Network ; September 2015 (from NEBU_NETWORK)
   Auth:    Api-Key (***thz4) (from NEBU_RPC_AUTH)
   Range:   60200000 → 60200100
```

**4. Quiet Mode:**

For scripting where you don't want the banner:

```bash
nebu run origin token-transfer --quiet --start 60200000 --end 60200100

# No banner, just JSON output
{"_schema":"nebu.token_transfer.v1",...}
{"_schema":"nebu.token_transfer.v1",...}
```

---

## Scope Line

### MUST HAVE ═══════════════════
- ✅ Print startup banner showing RPC, Network, Auth, Range
- ✅ Validate RPC URL and network passphrase consistency
- ✅ Support `NEBU_RPC_URL` environment variable
- ✅ Support `NEBU_NETWORK` environment variable
- ✅ Support `NEBU_RPC_AUTH` environment variable
- ✅ Show where config came from (flag vs environment)
- ✅ Add `--quiet` flag to suppress banner

### NICE TO HAVE ───────────────
- ⚠️ Show network name in banner (not just passphrase)
- ⚠️ Validate network passphrase is a known value (mainnet/testnet)
- ⚠️ Color-code output (red for mainnet, green for testnet)

### COULD HAVE ─────────────────
- ❌ Query RPC `/health` to get network info from server
- ❌ Detect futurenet, custom networks automatically
- ❌ Interactive confirmation for mainnet operations
- ❌ Config file support (`.neburc`)

---

## Rabbit Holes

**Don't do these:**

1. **❌ Don't query RPC for network info** - The `/health` endpoint might not exist or be slow. Use the `--network` flag value.

2. **❌ Don't implement interactive confirmations** - No "Are you sure you want to use mainnet? [y/N]" prompts. This breaks scriptability.

3. **❌ Don't build a config file system** - Environment variables + flags are sufficient. Config files are future work.

4. **❌ Don't detect all possible networks** - Just validate mainnet vs testnet. Custom networks and futurenet can be added later.

5. **❌ Don't add color output** - Keep it simple black/white text. Color can break in some terminals and log files.

---

## No-Gos

**Explicitly out of scope:**

- **RPC health checks** - Not querying RPC endpoint for network information
- **Interactive prompts** - No "are you sure?" dialogs that break scripts
- **Config files** - No `.neburc` or `nebu.yaml` support
- **Custom network auto-detection** - Only validating known networks (mainnet/testnet)
- **Colored output** - Plain text only

---

## Done Looks Like

### Success Example 1: Clear Startup Banner

```bash
nebu run origin token-transfer \
  --rpc-url https://mainnet.sorobanrpc.com \
  --start-ledger 60200000 \
  --end-ledger 60200100

# Output:
🚀 nebu run origin token-transfer
   RPC:     https://mainnet.sorobanrpc.com
   Network: Public Global Stellar Network ; September 2015
   Auth:    None
   Range:   60200000 → 60200100

Processing ledgers 60200000 to 60200100...
{"_schema":"nebu.token_transfer.v1","type":"transfer",...}
...
Processed 432 events
```

### Success Example 2: Detect Network Mismatch

```bash
nebu run origin token-transfer \
  --rpc-url https://rpc-testnet.nodeswithobsrvr.co \
  --start-ledger 60200000 \
  --end-ledger 60200100

# (Forgot --network testnet, so it defaults to mainnet passphrase)

# Output:
Error: Network configuration mismatch detected
  RPC URL:  https://rpc-testnet.nodeswithobsrvr.co (appears to be TESTNET)
  Network:  Public Global Stellar Network ; September 2015 (MAINNET passphrase)

Did you mean to use --network testnet?

To fix:
  nebu run origin token-transfer \
    --rpc-url https://rpc-testnet.nodeswithobsrvr.co \
    --network testnet \
    --start-ledger 60200000 --end-ledger 60200100

Or set environment:
  export NEBU_NETWORK=testnet
```

### Success Example 3: Environment Variables

```bash
# Set defaults
export NEBU_RPC_URL="https://rpc-pubnet.nodeswithobsrvr.co"
export NEBU_NETWORK="mainnet"
export NEBU_RPC_AUTH="Api-Key tetsf7WV.Wld1LQL5Qp3CjxffiZRC1rXtz0QQthz4"

# Run without specifying flags
nebu run origin token-transfer --start-ledger 60200000 --end-ledger 60200100

# Output shows where config came from:
🚀 nebu run origin token-transfer
   RPC:     https://rpc-pubnet.nodeswithobsrvr.co (from NEBU_RPC_URL)
   Network: Public Global Stellar Network ; September 2015 (from NEBU_NETWORK)
   Auth:    Api-Key (***thz4) (from NEBU_RPC_AUTH)
   Range:   60200000 → 60200100

Processing ledgers...
```

### Success Example 4: Quiet Mode for Scripts

```bash
# In a script where you want clean JSON output
nebu run origin token-transfer \
  --quiet \
  --start-ledger 60200000 \
  --end-ledger 60200100 \
  | jq 'select(.type == "transfer")' \
  | duckdb analytics.db -c "CREATE TABLE transfers AS SELECT * FROM read_json('/dev/stdin')"

# No banner printed, just JSON events
```

### Success Example 5: Override Environment with Flags

```bash
# Environment has testnet
export NEBU_NETWORK="testnet"

# But this run overrides to mainnet
nebu run origin token-transfer \
  --network mainnet \
  --start-ledger 60200000 \
  --end-ledger 60200100

# Output shows flag took precedence:
🚀 nebu run origin token-transfer
   RPC:     https://mainnet.sorobanrpc.com
   Network: Public Global Stellar Network ; September 2015 (from --network flag)
   Auth:    None
   Range:   60200000 → 60200100
```

---

## Implementation Checklist

When implementing this shape, you should:

- [ ] Create `printStartupBanner()` function in cmd/nebu
- [ ] Add `validateNetworkConfig()` function to detect mismatches
- [ ] Support `NEBU_RPC_URL` environment variable
- [ ] Support `NEBU_NETWORK` environment variable
- [ ] Support `NEBU_RPC_AUTH` environment variable
- [ ] Add `--quiet` / `-q` flag to all commands
- [ ] Show config source in banner (flag vs env)
- [ ] Call validation before connecting to RPC
- [ ] Update README with environment variable documentation
- [ ] Test mismatch detection (testnet URL + mainnet network)
- [ ] Test environment variable precedence (env < flags)

---

## Notes

- This is a **1-day appetite** - mostly logging and simple validation
- Network validation is heuristic-based (checking URL for "testnet"/"mainnet" strings)
- This is "good enough" validation - catches 90% of mistakes
- `--quiet` flag is important for scriptability and piping
- Environment variables make it easier to work consistently with one network
- Banner is printed to **stderr** so it doesn't interfere with JSON output to stdout
