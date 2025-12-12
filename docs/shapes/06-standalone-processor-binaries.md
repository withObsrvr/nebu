# Shape: Install Processors as Standalone Binaries

## Problem
Processors can only be run via `nebu run origin processor-name`. They're not standalone commands, which:
- Breaks Unix mental model (processors aren't "just programs")
- Adds ceremony to simple tasks (`nebu run origin` prefix required)
- Makes it unclear that processors are independent, composable tools
- Prevents installing processors globally like other CLI tools

You can't do:
```bash
which token-transfer
# → /usr/local/bin/token-transfer

token-transfer --help
# → shows processor help
```

## Appetite
**3 days** - Build system changes + installation logic + testing

## Solution

Add `nebu install <processor>` command that builds and installs processors as standalone binaries:

```bash
# Install a processor globally
$ nebu install token-transfer
Installing token-transfer...
Built: ./bin/token-transfer
Installed: /usr/local/bin/token-transfer

# Use as standalone command
$ token-transfer --help
token-transfer - Stream token transfer events from Stellar ledgers

Usage:
  token-transfer [flags]

Flags:
  --start-ledger uint32   Start ledger sequence
  --end-ledger uint32     End ledger sequence
  --rpc-url string        Stellar RPC endpoint
  -q, --quiet             Suppress progress messages

# Use in pipelines without nebu prefix
$ nebu fetch 60200000 60200100 | token-transfer | jq 'select(.type == "transfer")'
```

### Implementation Sketch

1. **Create `install` subcommand**:
   ```go
   var installCmd = &cobra.Command{
       Use:   "install <processor-name>",
       Short: "Install a processor as a standalone binary",
       RunE: func(cmd *cobra.Command, args []string) error {
           proc := registry.Get(args[0])

           // Build processor binary
           binary := buildProcessor(proc)

           // Install to $GOPATH/bin or /usr/local/bin
           installPath := getInstallPath()
           copyFile(binary, filepath.Join(installPath, proc.Name))

           fmt.Printf("Installed: %s\n", installPath)
           return nil
       },
   }
   ```

2. **Create processor binary template**:
   ```go
   // examples/processors/token-transfer/cmd/main.go
   package main

   import (
       "github.com/withObsrvr/nebu/pkg/processor/cli"
       "github.com/withObsrvr/nebu/examples/processors/token-transfer"
   )

   func main() {
       cli.RunOriginProcessor(token_transfer.NewProcessor())
   }
   ```

3. **Add `cli` package** for processor wrappers:
   ```go
   // pkg/processor/cli/origin.go
   func RunOriginProcessor(p processor.Origin) {
       // Handle stdin, RPC, flags, etc.
       // Same logic as "nebu process" but standalone
   }
   ```

4. **Update Makefile**:
   ```makefile
   install-processor:
       @processor=$(PROCESSOR); \
       go build -o $(GOPATH)/bin/$$processor \
           ./examples/processors/$$processor/cmd
   ```

### Scope Line

```
MUST HAVE ══════════════
- nebu install <processor> command
- Standalone binaries in $GOPATH/bin
- Processors work without nebu prefix
- Same flags as nebu process (--start-ledger, etc.)

NICE TO HAVE ───────────
- nebu uninstall <processor>
- List installed processors
- Version management

COULD HAVE ─────────────
- Auto-update processors
- Install from git URLs
- Plugin system
```

## Rabbit Holes

**Don't** implement a plugin system - just copy binaries to PATH.

**Don't** add version management - install current version only.

**Don't** support remote processors yet - local registry only.

## No-Gos

- **No package manager** - just copy binaries, don't build a package ecosystem
- **No versioning** - always install latest from local build
- **No dependency resolution** - processors are standalone
- **No auto-updates** - manual reinstall only

## Done

When complete:

```bash
# Install processor
$ nebu install token-transfer
Building token-transfer...
Installing to /home/user/go/bin/token-transfer
Done!

# Verify installation
$ which token-transfer
/home/user/go/bin/token-transfer

$ token-transfer --version
token-transfer v0.3.0 (nebu processor)

# Use without nebu prefix
$ token-transfer --help
token-transfer - Stream token transfer events from Stellar ledgers

Usage:
  token-transfer [flags]

Flags:
  --start-ledger uint32   Start ledger sequence (required)
  --end-ledger uint32     End ledger sequence (required)
  --rpc-url string        Stellar RPC endpoint (default: mainnet)
  -q, --quiet             Suppress progress output
  -h, --help              Show help

Examples:
  # Fetch and process in one command
  token-transfer --start-ledger 60200000 --end-ledger 60200100

  # Read from stdin
  nebu fetch 60200000 60200100 | token-transfer

  # Chain with other tools
  token-transfer --start 60200000 --end 60200100 | jq '.type' | sort | uniq -c

# Works in pipelines naturally
$ nebu fetch 60200000 60200100 | token-transfer | duckdb events.db

# Install multiple processors
$ nebu install soroban-events
$ nebu install amm-swaps
$ ls $(go env GOPATH)/bin/
token-transfer  soroban-events  amm-swaps  nebu

# Uninstall (future)
$ nebu uninstall token-transfer
Removed /home/user/go/bin/token-transfer
```

## gRPC Compatibility

✅ **This COMPLEMENTS gRPC processors**

Standalone binaries are for **local execution**. gRPC processors are for **remote execution**. Both can coexist:

**Local processors** (this shape):
- Installed as binaries in PATH
- Run locally, read stdin, write stdout
- Zero network overhead
- Great for development, testing, single-machine pipelines

**gRPC processors** (future):
- Run as services on remote hosts
- Communicate via gRPC protocol
- Scalable across machines
- Great for production, distributed systems

Example hybrid workflow:
```bash
# Fetch locally
nebu fetch 60200000 60200100 | \

# Process locally
token-transfer | \

# Transform remotely via gRPC
grpc-call transform.example.com:9000 usdc-filter | \

# Sink locally
postgres-sink --db localhost
```

You could even wrap standalone binaries as gRPC services:
```bash
# Start processor as gRPC service
$ nebu serve token-transfer --port 9000
Serving token-transfer on grpc://localhost:9000

# Now callable remotely
$ grpc-client call localhost:9000 ProcessLedgers < ledgers.xdr
```
