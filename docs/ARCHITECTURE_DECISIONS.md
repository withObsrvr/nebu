# Architecture Decision: nebu Stays CLI-Only

## The Decision

**nebu will remain CLI-focused with Unix pipes.** It will NOT become a gRPC orchestrator or service manager.

For complex orchestration (gRPC, multi-language, distributed pipelines), use [flowctl](https://github.com/withObsrvr/flowctl).

## Rationale

### Why nebu Stays Simple

> "Nebu felt cool and minimal being able to string together processors on the command line versus having to write a pipeline file to keep track of services"

nebu follows the Unix philosophy:
- **Do one thing well** - Extract data from Stellar, stream it as JSON
- **Composable** - Chain processors with pipes, `jq`, `grep`, `awk`
- **Minimal** - No service management, no port allocation, no process orchestration
- **Fast to prototype** - Get results in 2 minutes, not 2 hours

### Why Not Add Orchestration?

The original GRPC_PROCESSORS.md proposed making nebu an orchestrator. This would require:
- Service lifecycle management (start, stop, health checks)
- Port allocation and discovery
- gRPC client/server setup
- Pipeline YAML configuration
- Process monitoring

**This duplicates flowctl's functionality** and adds complexity to nebu.

## Clear Separation of Concerns

| Aspect | nebu | flowctl |
|--------|------|---------|
| **Purpose** | Stellar data extraction | General pipeline orchestration |
| **Target User** | Developers prototyping, analysts exploring data | DevOps running production pipelines |
| **Communication** | Unix pipes (stdin/stdout, JSON) | gRPC (protobuf, type-safe) |
| **Setup Time** | 2 minutes (install + run) | 10-30 minutes (write pipeline YAML, deploy) |
| **Complexity** | Minimal (spawn process, pipe data) | Full orchestration (service manager, control plane) |
| **Multi-Language** | Go only (currently) | Any language via containers |
| **Use Case** | Quick analysis, ad-hoc queries, testing | Production indexers, distributed systems, monitoring |

## Communication Formats

### nebu (CLI mode)
- **Wire format:** Newline-delimited JSON via Unix pipes (stdin/stdout)
- **Internal:** Protobuf structs from Stellar SDK for type safety
- **Language:** Go only (currently)
- **Why JSON?** Easy to debug with `cat`, `jq`, `grep`. Works with DuckDB, shell scripts, any language.

### flowctl (gRPC mode)
- **Wire format:** Native protobuf over gRPC streams
- **Internal:** Protobuf-generated types
- **Language:** Any (Python, Rust, TypeScript, Go, etc.)
- **Why Protobuf?** Type safety, binary efficiency, streaming performance

**Key difference:** nebu uses JSON for _interoperability_ (any tool can consume it). flowctl uses protobuf for _type safety and performance_ (controlled environment).

## When to Use Each

### Use nebu CLI when:
- Prototyping a new processor or pipeline
- One-off analysis ("show me USDC transfers in ledger range X")
- Piping to `jq`, DuckDB, or shell scripts
- Single-machine pipelines
- Simple Go processors
- Fast iteration (no YAML, no service management)

### Use flowctl when:
- Production pipelines with SLA requirements
- Multi-language processors (Python, Rust, TypeScript)
- Distributed systems (multiple machines)
- Need observability (metrics, health checks, tracing)
- Complex DAG topologies (fan-out, fan-in)
- Service lifecycle management required

## Future: Multi-Language Processors

**For simple processors:** Stay with nebu CLI mode (Go, stdin/stdout, JSON)

**For complex processors:** Build as flowctl components from the start:
- Python, Rust, TypeScript, etc.
- gRPC with protobuf definitions
- Full type safety and streaming
- Deployed as containers or processes

**No tight coupling:** nebu and flowctl are independent projects with different goals.

## Optional Integration (Future)

In the future, nebu processors could optionally be packaged as flowctl components:
- `nebu export token-transfer > flowctl-component.yaml`
- flowctl wraps nebu processor in a process/container
- Users get choice: CLI simplicity or orchestration power

But this is **not required** - both tools work independently.

## Summary

nebu is a **data extraction tool** optimized for speed and simplicity.

flowctl is a **service orchestrator** optimized for production and complexity.

Use the right tool for the job.
