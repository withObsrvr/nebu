.PHONY: all test build lint clean fmt vet build-cli install gen-protos build-processors test-integration api-snapshot api-check

# Version information
VERSION ?= 0.6.0
LDFLAGS = -X github.com/withObsrvr/nebu/pkg/version.Version=$(VERSION)

all: test build

test:
	go test -v ./...

build:
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
	@echo "Building processor binaries..."
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/token-transfer ./examples/processors/token-transfer/cmd/token-transfer
	cd examples/processors/contract-events && go build -ldflags "$(LDFLAGS)" -o ../../../bin/contract-events ./cmd/contract-events
	go build -ldflags "$(LDFLAGS)" -o bin/contract-invocation ./examples/processors/contract-invocation/cmd/contract-invocation
	go build -ldflags "$(LDFLAGS)" -o bin/usdc-filter ./examples/processors/usdc-filter/cmd/usdc-filter
	go build -ldflags "$(LDFLAGS)" -o bin/amount-filter ./examples/processors/amount-filter/cmd/amount-filter
	go build -ldflags "$(LDFLAGS)" -o bin/time-window ./examples/processors/time-window/cmd/time-window
	go build -ldflags "$(LDFLAGS)" -o bin/dedup ./examples/processors/dedup/cmd/dedup
	go build -ldflags "$(LDFLAGS)" -o bin/json-file-sink ./examples/processors/json-file-sink/cmd/json-file-sink
	cd examples/processors/nats-sink && go build -ldflags "$(LDFLAGS)" -o ../../../bin/nats-sink ./cmd/nats-sink
	go build -ldflags "$(LDFLAGS)" -o bin/postgres-sink ./examples/processors/postgres-sink/cmd/postgres-sink
	@echo "✓ All processors built in ./bin/"

# Run integration tests
test-integration:
	@./tests/integration/test_pipelines.sh

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
