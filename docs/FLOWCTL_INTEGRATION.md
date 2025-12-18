# nebu + flowctl Integration (Optional)

This document describes how nebu and flowctl can optionally work together. **This integration is NOT required** - both tools work independently.

## Quick Overview

**nebu:** CLI tool for Stellar data extraction with Unix pipes
**flowctl:** Service orchestrator for complex, distributed pipelines

You can use nebu alone, flowctl alone, or combine them for specific use cases.

## Use Case: nebu as a flowctl Source Component

If you're building a production pipeline with flowctl and need Stellar data, you can wrap a nebu processor as a flowctl source component.

### Example: Token Transfer Source

**Scenario:** You have a flowctl pipeline that processes token transfers with custom Python analytics and a Kafka sink.

**Architecture:**
```
┌────────────────┐
│ nebu processor │ (Go binary)
│ token-transfer │ Internal: Protobuf structs
└───────┬────────┘
        │ stdout: Newline-delimited JSON
        ▼
┌────────────────┐
│ flowctl wraps  │ (Reads JSON, converts to gRPC protobuf)
│ as source      │
└───────┬────────┘
        │ gRPC: Native protobuf streams
        ▼
┌────────────────┐
│ Python         │
│ analytics      │
└───────┬────────┘
        │ gRPC: Native protobuf streams
        ▼
┌────────────────┐
│ Kafka sink     │
└────────────────┘
```

**Wire format transition:**
- nebu → flowctl: JSON (for CLI compatibility)
- flowctl components: Native protobuf (for type safety and performance)

### Future: Automated Export (Not Implemented Yet)

In the future, nebu could generate flowctl-compatible configurations:

```bash
# Generate flowctl component definition from nebu processor
nebu export token-transfer --format flowctl > token-transfer.yaml

# Use in flowctl pipeline
flowctl run pipeline.yaml
```

**pipeline.yaml:**
```yaml
apiVersion: flowctl/v1
kind: Pipeline
metadata:
  name: stellar-analytics
spec:
  sources:
    - id: stellar-source
      command: ["token-transfer", "--start-ledger", "60200000", "--end-ledger", "60300000"]
      # flowctl reads stdout (JSON), converts to gRPC Events
  processors:
    - id: analytics
      image: my-python-analytics:latest
      inputs: ["stellar-source"]
  sinks:
    - id: kafka
      image: kafka-sink:latest
      inputs: ["analytics"]
```

## When to Combine nebu + flowctl

**Good reasons:**
- You need Stellar data in a flowctl pipeline
- You want observability/monitoring for nebu processors
- You're mixing nebu with non-Go components (Python, Rust, etc.)
- Production deployment requires service management

**Bad reasons (use nebu CLI instead):**
- Quick prototyping or ad-hoc analysis
- Single-machine, simple pipelines
- No need for service orchestration

## Wrapping nebu Processors for flowctl (Current Manual Method)

Until `nebu export` is implemented, you can manually wrap nebu processors:

### Option 1: Process Driver

flowctl can run nebu processors as local processes:

**pipeline.yaml:**
```yaml
apiVersion: flowctl/v1
kind: Pipeline
metadata:
  name: nebu-pipeline
spec:
  driver: process
  sources:
    - id: token-transfer
      command: ["token-transfer", "--start-ledger", "60200000", "--end-ledger", "60200100"]
      env:
        NEBU_RPC_URL: "https://archive-rpc.lightsail.network"
      # flowctl captures stdout as source data
```

### Option 2: Docker Driver

Package nebu processor as a container:

**Dockerfile:**
```dockerfile
FROM golang:1.25 AS builder
WORKDIR /build
COPY . .
RUN go build -o token-transfer ./cmd/token-transfer

FROM debian:bookworm-slim
COPY --from=builder /build/token-transfer /usr/local/bin/
ENTRYPOINT ["token-transfer"]
```

**Build and use:**
```bash
docker build -t nebu-token-transfer:latest .

# flowctl pipeline.yaml
apiVersion: flowctl/v1
kind: Pipeline
metadata:
  name: nebu-docker-pipeline
spec:
  driver: docker
  sources:
    - id: token-transfer
      image: nebu-token-transfer:latest
      args: ["--start-ledger", "60200000", "--end-ledger", "60200100"]
```

## Links

- **flowctl Documentation:** https://github.com/withObsrvr/flowctl
- **nebu Architecture Decision:** [ARCHITECTURE_DECISIONS.md](./ARCHITECTURE_DECISIONS.md)
- **nebu CLI Guide:** [../README.md](../README.md)

## Summary

nebu and flowctl are independent tools that can optionally work together.

Use nebu CLI for simple, fast pipelines. Use flowctl when you need orchestration, multi-language, or production deployment.
