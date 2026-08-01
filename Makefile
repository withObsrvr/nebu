.PHONY: all test build lint clean fmt vet build-cli install gen-protos build-processors test-integration api-snapshot api-check docs-smoke sync-skill-docs

# Force bash for all recipes. The api-check target uses process
# substitution (diff -u file <(cmd)), which is a bash extension and
# not supported by POSIX /bin/sh — Ubuntu's default sh is dash, which
# would otherwise fail with "Syntax error: '(' unexpected" when CI
# spawns recipes with /bin/sh.
#
# Resolve bash via PATH at make-parse time so this works both in
# nix dev shells (bash lives in /nix/store/...) and in standard
# Linux/CI environments (bash at /bin/bash). Falls back to /bin/bash
# if 'which' fails for any reason.
SHELL := $(shell command -v bash 2>/dev/null || echo /bin/bash)

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -X github.com/withObsrvr/nebu/pkg/version.Version=$(VERSION)

all: test build

test:
	go test -v ./...

build: build-cli
	go build -v ./...

build-cli:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/nebu ./cmd/nebu

install: build-cli
	@echo "Installing nebu to $(GOPATH)/bin (or ~/go/bin)"
	go install ./cmd/nebu

lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed, skipping"; exit 0)
	golangci-lint run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	go clean
	rm -rf bin/

# Run a simple origin example
run-example:
	go run examples/simple_origin/main.go

# Generate protobuf code
gen-protos:
	@echo "Generating protobuf code..."
	@./scripts/gen-protos.sh

# Build all processor binaries
build-processors:
	@echo "Installing published processors from nebu-processor-registry..."
	@mkdir -p bin
	GOWORK=off GOBIN=$(CURDIR)/bin go install github.com/withObsrvr/nebu-processor-registry/processors/token-transfer/cmd/token-transfer@latest
	GOWORK=off GOBIN=$(CURDIR)/bin go install github.com/withObsrvr/nebu-processor-registry/processors/contract-events/cmd/contract-events@latest
	GOWORK=off GOBIN=$(CURDIR)/bin go install github.com/withObsrvr/nebu-processor-registry/processors/contract-invocation/cmd/contract-invocation@latest
	GOWORK=off GOBIN=$(CURDIR)/bin go install github.com/withObsrvr/nebu-processor-registry/processors/usdc-filter/cmd/usdc-filter@latest
	GOWORK=off GOBIN=$(CURDIR)/bin go install github.com/withObsrvr/nebu-processor-registry/processors/amount-filter/cmd/amount-filter@latest
	GOWORK=off GOBIN=$(CURDIR)/bin go install github.com/withObsrvr/nebu-processor-registry/processors/time-window/cmd/time-window@latest
	GOWORK=off GOBIN=$(CURDIR)/bin go install github.com/withObsrvr/nebu-processor-registry/processors/dedup/cmd/dedup@latest
	GOWORK=off GOBIN=$(CURDIR)/bin go install github.com/withObsrvr/nebu-processor-registry/processors/json-file-sink/cmd/json-file-sink@latest
	GOWORK=off GOBIN=$(CURDIR)/bin go install github.com/withObsrvr/nebu-processor-registry/processors/nats-sink/cmd/nats-sink@latest
	GOWORK=off GOBIN=$(CURDIR)/bin go install github.com/withObsrvr/nebu-processor-registry/processors/postgres-sink/cmd/postgres-sink@latest
	@echo "Building in-repo educational processors..."
	go build -ldflags "$(LDFLAGS)" -o bin/transaction-stats ./examples/processors/transaction-stats/cmd/transaction-stats
	go build -ldflags "$(LDFLAGS)" -o bin/ledger-change-stats ./examples/processors/ledger-change-stats/cmd/ledger-change-stats
	@echo "✓ All processor binaries built in ./bin/"

# Run integration tests against installed registry processors.
test-integration: build-processors
	@./tests/integration/test_pipelines.sh

docs-smoke:
	@./scripts/docs-smoke.sh

sync-skill-docs:
	@./scripts/sync-skill-docs.sh

# Regenerate the public-API snapshots for the stable surfaces
# (pkg/processor and pkg/source). Run this after intentionally
# changing a stable surface, then commit the updated .api/ files
# alongside the code change. The api-check target (run in CI)
# verifies the snapshots match the current source.
api-snapshot:
	@mkdir -p .api
	@go doc -all ./pkg/processor > .api/processor.txt
	@go doc -all ./pkg/source > .api/source.txt
	@echo "✓ API snapshots regenerated in .api/"

# Verify the public-API surface of pkg/processor and pkg/source
# matches the committed snapshots. Fails the build if they have
# drifted. The intended workflow on drift is:
#   1. Verify the change is intentional and acceptable
#   2. Run 'make api-snapshot' to regenerate the snapshots
#   3. Commit the .api/ updates alongside the code change
#
# This is the lightweight enforcement layer for the stability
# policy in docs/STABILITY.md — every PR that touches the stable
# surface gets a visible diff in .api/ for reviewers to scrutinize.
api-check:
	@mkdir -p .api
	@if ! diff -u .api/processor.txt <(go doc -all ./pkg/processor); then \
		echo ""; \
		echo "ERROR: pkg/processor public API has drifted from the committed snapshot."; \
		echo "If this change is intentional and acceptable per docs/STABILITY.md,"; \
		echo "regenerate the snapshot with: make api-snapshot"; \
		exit 1; \
	fi
	@if ! diff -u .api/source.txt <(go doc -all ./pkg/source); then \
		echo ""; \
		echo "ERROR: pkg/source public API has drifted from the committed snapshot."; \
		echo "If this change is intentional and acceptable per docs/STABILITY.md,"; \
		echo "regenerate the snapshot with: make api-snapshot"; \
		exit 1; \
	fi
	@echo "✓ Stable API surfaces match committed snapshots"
