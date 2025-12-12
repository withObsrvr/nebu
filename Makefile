.PHONY: all test build lint clean fmt vet build-ttpd run-ttpd build-cli install gen-protos build-processors test-integration

all: test build

test:
	go test -v ./...

build:
	go build -v ./...

build-cli:
	mkdir -p bin
	go build -o bin/nebu ./cmd/nebu

build-ttpd:
	mkdir -p bin
	go build -o bin/nebu-ttpd ./cmd/nebu-ttpd

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
	go build -o bin/token-transfer ./examples/processors/token-transfer/cmd
	go build -o bin/usdc-filter ./examples/processors/usdc-filter/cmd
	go build -o bin/amount-filter ./examples/processors/amount-filter/cmd
	go build -o bin/time-window ./examples/processors/time-window/cmd
	go build -o bin/dedup ./examples/processors/dedup/cmd
	go build -o bin/json-file-sink ./examples/processors/json-file-sink/cmd
	@echo "✓ All processors built in ./bin/"

# Run integration tests
test-integration:
	@./tests/integration/test_pipelines.sh
