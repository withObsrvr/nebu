# Shape: RPC Authorization Support

**Status**: Ready for Betting
**Appetite**: 2 days
**Created**: 2025-12-12

---

## Problem

nebu cannot connect to RPC endpoints that require authorization headers (e.g., OBSRVR's premium RPC endpoints). Users are forced to use public RPCs which may have rate limits, lower reliability, or higher latency.

**Current blockers:**
- `pkg/source/rpc.go` doesn't support custom headers
- No CLI flags for auth configuration
- No environment variable support for auth tokens
- No visibility into which RPC is being used during execution

**Real use case:**
```bash
# Want to use OBSRVR's premium RPC but can't:
nebu run origin token-transfer \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co \
  --start-ledger 60200000 \
  --end-ledger 60200100

# Error: unauthorized (403)
```

---

## Appetite

**2 days** - This is a focused enhancement to existing RPC client code.

---

## Solution

### Fat-Marker Sketch

**1. Add header support to RPC source:**

```go
// pkg/source/rpc.go
type RPCLedgerSource struct {
    client  *jrpc2.Client
    headers map[string]string  // NEW: Custom headers
}

func NewRPCLedgerSourceWithHeaders(rpcURL string, headers map[string]string) (*RPCLedgerSource, error) {
    // Create HTTP client with custom headers
    transport := &headerTransport{
        base:    http.DefaultTransport,
        headers: headers,
    }
    httpClient := &http.Client{Transport: transport}

    client := jrpc2.NewClient(rpcURL, &jrpc2.ClientOptions{
        HTTPClient: httpClient,
    })

    return &RPCLedgerSource{client: client, headers: headers}, nil
}
```

**2. Add CLI flags for auth:**

```bash
# Method 1: Direct header flag
nebu run origin token-transfer \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co \
  --rpc-header "Authorization: Api-Key tetsf7WV.Wld1LQL5Qp3CjxffiZRC1rXtz0QQthz4" \
  --start-ledger 60200000 --end-ledger 60200100

# Method 2: Environment variable (safer for secrets)
export NEBU_RPC_AUTH="Api-Key tetsf7WV.Wld1LQL5Qp3CjxffiZRC1rXtz0QQthz4"
nebu run origin token-transfer \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co \
  --start-ledger 60200000 --end-ledger 60200100
```

**3. Add startup logging:**

```
🚀 nebu run origin token-transfer
   RPC:     https://rpc-pubnet.nodeswithobsrvr.co
   Network: Public Global Stellar Network ; September 2015
   Auth:    Api-Key (***z4)
   Range:   60200000 → 60200100

Processing ledgers...
```

---

## Scope Line

### MUST HAVE ═══════════════════
- ✅ Support `Authorization` header in RPC client
- ✅ Support `NEBU_RPC_AUTH` environment variable
- ✅ Log RPC URL and auth status on startup
- ✅ Mask secrets in logs (show last 4 chars only)
- ✅ Update `pkg/source/rpc.go` to accept headers
- ✅ Update CLI commands (fetch, run) to support auth flags

### NICE TO HAVE ───────────────
- ✅ Support multiple headers via `--rpc-header` (repeatable flag)
- ✅ Support custom header names (not just Authorization)
- ⚠️ Validate auth token format
- ⚠️ Log network name from RPC `/health` endpoint

### COULD HAVE ─────────────────
- ❌ Config file for RPC profiles (`.neburc` with saved endpoints)
- ❌ Auto-retry with different RPC on 403/429
- ❌ Bearer token refresh logic
- ❌ OAuth flow support

---

## Rabbit Holes

**Don't do these:**

1. **❌ Don't build a config file system** - Just environment variables and flags for now. Config files can come later if needed.

2. **❌ Don't implement RPC health checking** - Just connect and fail fast if auth is wrong. Don't pre-validate endpoints.

3. **❌ Don't add retry logic** - If auth fails, fail immediately with clear error. Retries add complexity and hide problems.

4. **❌ Don't support multiple auth schemes** - Just header-based auth (Api-Key, Bearer tokens). No OAuth, no mTLS, no JWT signing.

5. **❌ Don't parse network info from RPC** - The `/health` endpoint might not exist or might be slow. Just use the `--network` flag value.

---

## No-Gos

**Explicitly out of scope:**

- **Config file support** (`.neburc`, `nebu.yaml`) - Environment variables are sufficient
- **Multiple RPC failover** - Use a load balancer if you need this
- **Token refresh/rotation** - Users manage their own API keys
- **mTLS certificate support** - Header-based auth only
- **RPC health pre-checks** - Just try to connect, fail fast if it doesn't work
- **Auto-detection of auth requirements** - User must explicitly provide auth if needed

---

## Done Looks Like

### Success Example 1: Using OBSRVR Premium RPC

```bash
# Set auth token
export NEBU_RPC_AUTH="Api-Key tetsf7WV.Wld1LQL5Qp3CjxffiZRC1rXtz0QQthz4"

# Run with premium RPC
nebu run origin token-transfer \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co \
  --start-ledger 60200000 \
  --end-ledger 60200100

# Output shows:
🚀 nebu run origin token-transfer
   RPC:     https://rpc-pubnet.nodeswithobsrvr.co
   Network: Public Global Stellar Network ; September 2015
   Auth:    Api-Key (***thz4)
   Range:   60200000 → 60200100

Processing ledgers 60200000 to 60200100...
{"type":"transfer","ledger_sequence":60200000,...}
{"type":"mint","ledger_sequence":60200001,...}
...
Processed 432 events
```

### Success Example 2: Custom Headers

```bash
# Multiple custom headers
nebu run origin token-transfer \
  --rpc-url https://custom-rpc.example.com \
  --rpc-header "Authorization: Bearer sk_live_abc123" \
  --rpc-header "X-Request-ID: nebu-run-001" \
  --start-ledger 60200000 \
  --end-ledger 60200100

# Output shows:
🚀 nebu run origin token-transfer
   RPC:     https://custom-rpc.example.com
   Network: Public Global Stellar Network ; September 2015
   Auth:    Bearer (***c123)
   Headers: X-Request-ID
   Range:   60200000 → 60200100
```

### Success Example 3: Fetch Command with Auth

```bash
export NEBU_RPC_AUTH="Api-Key tetsf7WV.Wld1LQL5Qp3CjxffiZRC1rXtz0QQthz4"

nebu fetch 60200000 60200100 \
  --rpc-url https://rpc-testnet.nodeswithobsrvr.co \
  --network testnet \
  --output ledgers.xdr

# Output shows:
🚀 nebu fetch
   RPC:     https://rpc-testnet.nodeswithobsrvr.co
   Network: Test SDF Network ; September 2015
   Auth:    Api-Key (***thz4)
   Range:   60200000 → 60200100
   Output:  ledgers.xdr

Fetching ledgers...
Fetched 100 ledgers
```

### Success Example 4: Clear Error on Missing Auth

```bash
# Try to use premium RPC without auth
nebu run origin token-transfer \
  --rpc-url https://rpc-pubnet.nodeswithobsrvr.co \
  --start-ledger 60200000 \
  --end-ledger 60200100

# Output shows:
🚀 nebu run origin token-transfer
   RPC:     https://rpc-pubnet.nodeswithobsrvr.co
   Network: Public Global Stellar Network ; September 2015
   Auth:    None
   Range:   60200000 → 60200100

Error: RPC request failed: 403 Forbidden

This endpoint requires authorization. Try:
  export NEBU_RPC_AUTH="Api-Key YOUR_KEY_HERE"
Or use the --rpc-header flag:
  --rpc-header "Authorization: Api-Key YOUR_KEY_HERE"
```

---

## Implementation Checklist

When implementing this shape, you should:

- [ ] Update `pkg/source/rpc.go` to accept custom headers
- [ ] Create `headerTransport` wrapper for http.RoundTripper
- [ ] Add `--rpc-header` flag to `fetch` command
- [ ] Add `--rpc-header` flag to `run origin` command
- [ ] Support `NEBU_RPC_AUTH` environment variable
- [ ] Add startup logging showing RPC URL, network, auth status
- [ ] Mask secrets in logs (show only last 4 characters)
- [ ] Update README with auth examples
- [ ] Test with OBSRVR premium RPC endpoints
- [ ] Test with missing auth (should fail with helpful error)
- [ ] Test with invalid auth token (should fail clearly)

---

## Notes

- This is a **2-day appetite** - should be straightforward HTTP header injection
- If hitting problems with the Stellar Go SDK's RPC client, **cut scope** - just get basic Authorization header working
- Don't get pulled into config file design - that's a future cycle
- Focus on the 80% use case: one `Authorization` header with an API key
