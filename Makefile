.PHONY: all test build lint clean fmt vet build-ttpd run-ttpd build-cli install gen-protos build-processors test-integration

# Version information
VERSION ?= 0.3.0
LDFLAGS = -X github.com/withObsrvr/nebu/pkg/version.Version=$(VERSION)

all: test build

test:
	go test -v ./...

build:
	go build -v ./...

build-cli:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/nebu ./cmd/nebu

build-ttpd:
	mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/nebu-ttpd ./cmd/nebu-ttpd

install: build-cli
	@echo "Installing nebu to $(GOPATH)/bin (or ~/go/bin)"
	go install ./cmd/nebu

run-ttpd: build-ttpd
	./bin/nebu-ttpd

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
	go build -ldflags "$(LDFLAGS)" -o bin/contract-events ./examples/processors/contract-events/cmd/contract-events
	go build -ldflags "$(LDFLAGS)" -o bin/contract-invocation ./examples/processors/contract-invocation/cmd/contract-invocation
	go build -ldflags "$(LDFLAGS)" -o bin/usdc-filter ./examples/processors/usdc-filter/cmd/usdc-filter
	go build -ldflags "$(LDFLAGS)" -o bin/amount-filter ./examples/processors/amount-filter/cmd/amount-filter
	go build -ldflags "$(LDFLAGS)" -o bin/time-window ./examples/processors/time-window/cmd/time-window
	go build -ldflags "$(LDFLAGS)" -o bin/dedup ./examples/processors/dedup/cmd/dedup
	go build -ldflags "$(LDFLAGS)" -o bin/json-file-sink ./examples/processors/json-file-sink/cmd/json-file-sink
	@echo "✓ All processors built in ./bin/"

# Run integration tests
test-integration:
	@./tests/integration/test_pipelines.sh
