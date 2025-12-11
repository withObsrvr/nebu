.PHONY: all test build lint clean fmt vet build-ttpd run-ttpd

all: test build

test:
	go test -v ./...

build:
	go build -v ./...

build-ttpd:
	mkdir -p bin
	go build -o bin/nebu-ttpd ./cmd/nebu-ttpd

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
