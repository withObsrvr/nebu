# gRPC Processors Architecture

This document explains how gRPC processors work alongside CLI processors in nebu, using `amount-filter` as an example.

## Table of Contents
- [Architecture Overview](#architecture-overview)
- [Dual-Mode Processors](#dual-mode-processors)
- [Building a gRPC Transform](#building-a-grpc-transform)
- [Running gRPC Processors](#running-grpc-processors)
- [Use Cases](#use-cases)

---

## Architecture Overview

nebu supports **two execution modes** for processors:

### 1. Local Mode (CLI)
Processors run as local binaries, communicating via stdin/stdout:

```bash
# All local processes
token-transfer --start-ledger 60200000 --end-ledger 60200100 | \
  amount-filter --min 10000000 | \
  json-file-sink --out results.jsonl
```

**Characteristics**:
- ✅ Simple deployment (single binaries)
- ✅ Easy development and testing
- ✅ Works with standard Unix tools
- ✅ No network overhead
- ❌ All processing on one machine
- ❌ No load balancing

### 2. Remote Mode (gRPC)
Processors run as network services, communicating via gRPC:

```bash
# Mix of local and remote processors
nebu run \
  --origin grpc://token-transfer-service:9000 \
  --transform grpc://amount-filter-service:9001 \
  --sink grpc://postgres-sink-service:9002
```

**Characteristics**:
- ✅ Distributed processing across machines
- ✅ Horizontal scaling (run multiple instances)
- ✅ Language-agnostic (Python, Go, Rust, etc.)
- ✅ Load balancing and failover
- ❌ More complex deployment
- ❌ Network latency overhead

### 3. Hybrid Mode
Mix local and remote processors in the same pipeline:

```bash
# Origin is remote (shared), transform and sink are local
nebu run \
  --origin grpc://token-transfer-service:9000 | \
  amount-filter --min 10000000 | \
  json-file-sink --out results.jsonl
```

---

## Dual-Mode Processors

The **same processor logic** can work in both modes. Here's how:

### Processor Core (Shared Logic)

```go
// examples/processors/amount-filter/filter.go
package amount_filter

import (
	"strconv"
)

// Filter encapsulates the filtering logic
type Filter struct {
	minAmount int64
	maxAmount int64
	assetCode string
}

func NewFilter(minAmount, maxAmount int64, assetCode string) *Filter {
	return &Filter{
		minAmount: minAmount,
		maxAmount: maxAmount,
		assetCode: assetCode,
	}
}

// FilterEvent applies the amount filter logic
// Returns the event if it passes, nil if filtered out
func (f *Filter) FilterEvent(event map[string]interface{}) map[string]interface{} {
	// Get amount field
	amountStr, ok := event["amount"].(string)
	if !ok {
		return nil
	}

	// Parse amount
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		return nil
	}

	// Check minimum
	if f.minAmount > 0 && amount < f.minAmount {
		return nil
	}

	// Check maximum
	if f.maxAmount > 0 && amount > f.maxAmount {
		return nil
	}

	// Check asset if specified
	if f.assetCode != "" {
		asset, ok := event["asset"].(map[string]interface{})
		if !ok {
			return nil
		}

		code, ok := asset["code"].(string)
		if !ok || code != f.assetCode {
			return nil
		}
	}

	return event
}
```

### CLI Mode (Local Binary)

```go
// examples/processors/amount-filter/cmd/main.go
package main

import (
	"github.com/spf13/cobra"
	"github.com/withObsrvr/nebu/examples/processors/amount-filter"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var (
	minAmount int64
	maxAmount int64
	assetCode string
)

func main() {
	config := cli.TransformConfig{
		Name:        "amount-filter",
		Description: "Filter events by amount range",
		Version:     "0.1.0",
	}

	cli.RunTransformCLI(config, filterFunc, addFlags)
}

func addFlags(cmd *cobra.Command) {
	cmd.Flags().Int64Var(&minAmount, "min", 0, "Minimum amount")
	cmd.Flags().Int64Var(&maxAmount, "max", 0, "Maximum amount")
	cmd.Flags().StringVar(&assetCode, "asset", "", "Asset code")
}

func filterFunc(event map[string]interface{}) map[string]interface{} {
	filter := amount_filter.NewFilter(minAmount, maxAmount, assetCode)
	return filter.FilterEvent(event)
}
```

### gRPC Mode (Network Service)

**1. Define protobuf service:**

```protobuf
// examples/processors/amount-filter/proto/amount_filter_service.proto
syntax = "proto3";

package nebu.amount_filter;

option go_package = "github.com/withObsrvr/nebu/processors/amount-filter/proto;amount_filter_pb";

// TransformRequest contains an event to transform
message TransformRequest {
  // JSON-encoded event
  bytes event_json = 1;
}

// TransformResponse contains the transformed event (or null if filtered)
message TransformResponse {
  // JSON-encoded transformed event (empty if filtered out)
  bytes event_json = 1;

  // Whether the event was filtered out
  bool filtered = 2;
}

// FilterConfig configures the amount filter
message FilterConfig {
  int64 min_amount = 1;
  int64 max_amount = 2;
  string asset_code = 3;
}

// ConfigureRequest sets filter parameters
message ConfigureRequest {
  FilterConfig config = 1;
}

message ConfigureResponse {
  bool success = 1;
}

// AmountFilterService is a gRPC transform service
service AmountFilterService {
  // Configure sets the filter parameters
  rpc Configure(ConfigureRequest) returns (ConfigureResponse);

  // Transform applies the filter to an event
  rpc Transform(TransformRequest) returns (TransformResponse);

  // TransformStream applies the filter to a stream of events
  rpc TransformStream(stream TransformRequest) returns (stream TransformResponse);
}
```

**2. Implement gRPC server:**

```go
// examples/processors/amount-filter/server/server.go
package server

import (
	"context"
	"encoding/json"

	"github.com/withObsrvr/nebu/examples/processors/amount-filter"
	pb "github.com/withObsrvr/nebu/examples/processors/amount-filter/proto"
)

type AmountFilterServer struct {
	pb.UnimplementedAmountFilterServiceServer
	filter *amount_filter.Filter
}

func NewAmountFilterServer() *AmountFilterServer {
	return &AmountFilterServer{
		filter: amount_filter.NewFilter(0, 0, ""),
	}
}

func (s *AmountFilterServer) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	cfg := req.GetConfig()
	s.filter = amount_filter.NewFilter(
		cfg.GetMinAmount(),
		cfg.GetMaxAmount(),
		cfg.GetAssetCode(),
	)
	return &pb.ConfigureResponse{Success: true}, nil
}

func (s *AmountFilterServer) Transform(ctx context.Context, req *pb.TransformRequest) (*pb.TransformResponse, error) {
	// Decode JSON event
	var event map[string]interface{}
	if err := json.Unmarshal(req.GetEventJson(), &event); err != nil {
		return nil, err
	}

	// Apply filter
	result := s.filter.FilterEvent(event)

	if result == nil {
		// Event filtered out
		return &pb.TransformResponse{
			Filtered: true,
		}, nil
	}

	// Encode result
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	return &pb.TransformResponse{
		EventJson: resultJSON,
		Filtered:  false,
	}, nil
}

func (s *AmountFilterServer) TransformStream(stream pb.AmountFilterService_TransformStreamServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		// Decode, filter, encode (same as Transform)
		var event map[string]interface{}
		if err := json.Unmarshal(req.GetEventJson(), &event); err != nil {
			return err
		}

		result := s.filter.FilterEvent(event)

		resp := &pb.TransformResponse{
			Filtered: result == nil,
		}

		if result != nil {
			resultJSON, err := json.Marshal(result)
			if err != nil {
				return err
			}
			resp.EventJson = resultJSON
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}
```

**3. Create gRPC server binary:**

```go
// examples/processors/amount-filter/cmd/grpc-server/main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"github.com/withObsrvr/nebu/examples/processors/amount-filter/server"
	pb "github.com/withObsrvr/nebu/examples/processors/amount-filter/proto"
)

var (
	port = flag.Int("port", 9001, "The server port")
)

func main() {
	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterAmountFilterServiceServer(grpcServer, server.NewAmountFilterServer())

	log.Printf("amount-filter gRPC server listening on :%d", *port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
```

---

## Running gRPC Processors

### Starting a gRPC Service

**Build and run the server:**
```bash
# Build gRPC server
go build -o ./bin/amount-filter-grpc-server \
  ./examples/processors/amount-filter/cmd/grpc-server

# Run on port 9001
./bin/amount-filter-grpc-server --port 9001

# Output: amount-filter gRPC server listening on :9001
```

### Using with nebu CLI

**Option 1: Configure via environment**
```bash
# Set filter config via environment or config file
export AMOUNT_FILTER_MIN=10000000
export AMOUNT_FILTER_ASSET=USDC

# Run nebu with gRPC processor
nebu run \
  --origin token-transfer --start-ledger 60200000 --end-ledger 60200100 \
  --transform grpc://localhost:9001 \
  --sink json-file-sink --out results.jsonl
```

**Option 2: Configure via nebu**
```bash
# nebu sends Configure RPC before streaming events
nebu run \
  --origin token-transfer --start-ledger 60200000 --end-ledger 60200100 \
  --transform grpc://localhost:9001?min=10000000&asset=USDC \
  --sink json-file-sink --out results.jsonl
```

### Docker Deployment

**Dockerfile:**
```dockerfile
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN go build -o /bin/amount-filter-grpc-server \
    ./examples/processors/amount-filter/cmd/grpc-server

FROM alpine:latest
COPY --from=builder /bin/amount-filter-grpc-server /usr/local/bin/
EXPOSE 9001
ENTRYPOINT ["amount-filter-grpc-server"]
CMD ["--port", "9001"]
```

**Docker Compose:**
```yaml
version: '3.8'

services:
  token-transfer:
    image: nebu/token-transfer:latest
    ports:
      - "9000:9000"
    environment:
      - STELLAR_RPC_URL=https://mainnet.sorobanrpc.com

  amount-filter:
    image: nebu/amount-filter:latest
    ports:
      - "9001:9001"
    environment:
      - MIN_AMOUNT=10000000
      - ASSET_CODE=USDC

  postgres-sink:
    image: nebu/postgres-sink:latest
    ports:
      - "9002:9002"
    environment:
      - POSTGRES_URL=postgresql://user:pass@db:5432/events
```

**Run pipeline:**
```bash
docker-compose up -d

nebu run \
  --origin grpc://localhost:9000 \
  --transform grpc://localhost:9001 \
  --sink grpc://localhost:9002 \
  --start-ledger 60200000 --end-ledger 60300000
```

---

## Use Cases

### When to Use CLI Mode

✅ **Development and Testing**
```bash
# Quick local testing
echo '{"amount":"50000000","asset":{"code":"USDC"}}' | \
  amount-filter --min 10000000 --asset USDC
```

✅ **Single-Machine Workloads**
```bash
# Process data on one server
token-transfer --start 60200000 --end 60300000 | \
  amount-filter --min 10000000 | \
  duckdb events.db
```

✅ **Ad-Hoc Analysis**
```bash
# Quick data exploration
cat historical-events.jsonl | amount-filter --min 100000000 | wc -l
```

### When to Use gRPC Mode

✅ **Distributed Processing**
```bash
# Process across multiple machines
nebu run \
  --origin grpc://rpc-cluster:9000 \      # Heavy RPC calls
  --transform grpc://filter-cluster:9001 \ # Filter farm
  --sink grpc://db-cluster:9002           # Write to sharded DB
```

✅ **Horizontal Scaling**
```bash
# Load balance across 3 filter instances
nebu run \
  --origin token-transfer \
  --transform grpc://filter-1:9001,grpc://filter-2:9001,grpc://filter-3:9001 \
  --sink postgres-sink
```

✅ **Language Diversity**
```python
# Python transform service
# Can integrate with Python ML models, pandas, etc.
class AmountFilterService(amount_filter_pb2_grpc.AmountFilterServiceServicer):
    def Transform(self, request, context):
        event = json.loads(request.event_json)
        # Use numpy, pandas, scikit-learn, etc.
        filtered = apply_ml_model(event)
        return TransformResponse(event_json=json.dumps(filtered))
```

✅ **Shared Infrastructure**
```bash
# Multiple teams use same services
# Team A
nebu run --transform grpc://shared-filter:9001 --config team-a.yaml

# Team B
nebu run --transform grpc://shared-filter:9001 --config team-b.yaml
```

### Hybrid Mode Example

```bash
# Origin is remote (expensive Stellar RPC calls shared across team)
# Transforms are local (fast, no network overhead)
# Sink is remote (shared database cluster)

nebu run \
  --origin grpc://token-transfer.prod.company.com:9000 | \
  amount-filter --min 10000000 | \
  usdc-filter | \
  dedup --key tx_hash | \
  grpc://postgres-sink.prod.company.com:9002
```

**Why this works well**:
- Origin: Shared RPC infrastructure (avoid duplicate API calls)
- Transforms: Local processing (low latency, easy debugging)
- Sink: Shared database (consistent data storage)

---

## Benefits of Dual-Mode Architecture

### 1. Gradual Migration
```bash
# Start: All local
token-transfer | amount-filter | json-file-sink

# Step 1: Move origin to service
grpc://token-transfer:9000 | amount-filter | json-file-sink

# Step 2: Move sink to service
grpc://token-transfer:9000 | amount-filter | grpc://postgres:9002

# Final: All distributed
grpc://token-transfer:9000 | grpc://amount-filter:9001 | grpc://postgres:9002
```

### 2. Same Code, Different Deployment
```go
// The FilterEvent logic is identical in both modes
func (f *Filter) FilterEvent(event map[string]interface{}) map[string]interface{} {
	// ... same filtering logic ...
}

// CLI wraps it:
cli.RunTransformCLI(config, filter.FilterEvent, flags)

// gRPC wraps it:
type Server struct {
	filter *Filter
}
func (s *Server) Transform(req) {
	return s.filter.FilterEvent(event)
}
```

### 3. Local Development, Remote Production
```bash
# Dev: Test locally with instant feedback
cat test-events.jsonl | amount-filter --min 1000

# Prod: Deploy as distributed services
kubectl apply -f amount-filter-deployment.yaml
```

---

## Implementation Checklist

To add gRPC support to an existing CLI processor:

- [ ] Extract core logic into shared package (e.g., `filter.go`)
- [ ] Define protobuf service (`.proto` file)
- [ ] Generate Go code: `protoc --go_out=. --go-grpc_out=. *.proto`
- [ ] Implement gRPC server wrapping core logic
- [ ] Create gRPC server binary (`cmd/grpc-server/main.go`)
- [ ] Build and test locally
- [ ] Create Dockerfile
- [ ] Deploy to infrastructure (Docker Compose, Kubernetes, etc.)
- [ ] Update nebu CLI to support `grpc://` URLs

---

## Summary

**CLI Mode** (`amount-filter --min 10000000`):
- ✅ Simple, fast, local
- ✅ Perfect for development and small workloads
- ✅ Works with Unix tools

**gRPC Mode** (`grpc://amount-filter:9001`):
- ✅ Distributed, scalable, multi-language
- ✅ Perfect for production and large workloads
- ✅ Enables microservices architecture

**The same processor logic works in both modes!** 🎯

Start with CLI for development, graduate to gRPC for production scale.
