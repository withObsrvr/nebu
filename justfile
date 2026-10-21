# nebu task runner.
#
# Two commands from a fresh clone: `direnv allow` (or `nix develop`),
# then `just test`. Run `just` with no arguments to list every recipe.

# Process substitution in `api-check` is a bash extension; dash would
# fail on it. -u catches unset variables, -c runs the recipe body as a
# single script.
set shell := ["bash", "-uc"]

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-X github.com/withObsrvr/nebu/pkg/version.Version=" + version

# List all recipes (default when `just` is run with no arguments).
default:
    @just --list

# Everything the CI merge gate enforces, cheap checks first so failures
# surface fast. Run this before pushing.

# Run every check the CI merge gate enforces.
ci: fmt-check vet build test-short api-check
    @echo "✓ CI checks passed"

alias all := everything

# Full test suite, then build.
everything: test build

# Includes the network-dependent pkg/source/rpc tests; use `just
# test-short` to skip those.

# Full test suite with the race detector.
test:
    go test -race -v ./...

# This is what CI runs, so a green gate never depends on a reachable
# Stellar RPC endpoint.

# Test suite without the network-dependent RPC tests.
test-short:
    go test -race -short ./...

# Run pipelines end to end against installed processor binaries.
test-integration: build-processors
    ./tests/integration/test_pipelines.sh

# Build the nebu CLI, then compile every package.
build: build-cli
    go build -v ./...

# Build just the nebu CLI into bin/nebu.
build-cli:
    mkdir -p bin
    go build -ldflags "{{ ldflags }}" -o bin/nebu ./cmd/nebu

# Install nebu into $GOBIN (or ~/go/bin).
install: build-cli
    go install ./cmd/nebu

# gofmt the tree.
fmt:
    go fmt ./...

# `gofmt -l` prints offenders but exits 0, so the status has to come
# from its output.

# Fail if any file is not gofmt-clean.
fmt-check:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted="$(gofmt -l . | grep -v '^.direnv/' || true)"
    if [ -n "$unformatted" ]; then
        echo "ERROR: these files are not gofmt-clean:"
        echo "$unformatted"
        echo ""
        echo "Fix with: just fmt"
        exit 1
    fi
    echo "✓ All files are gofmt-clean"

# Run go vet.
vet:
    go vet ./...

# Not part of `just ci`: the existing findings are untriaged, so wiring
# it into the merge gate would block every PR. Provided by the dev shell.

# Run golangci-lint.
lint:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v golangci-lint > /dev/null; then
        echo "ERROR: golangci-lint not found."
        echo "It is provided by the dev shell — run 'direnv allow' or 'nix develop'."
        exit 1
    fi
    golangci-lint run ./...

# Remove bin/ and Go build artifacts.
clean:
    go clean
    rm -rf bin/

# Regenerate protobuf code.
gen-protos:
    @echo "Generating protobuf code..."
    ./scripts/gen-protos.sh

# Verify the CLI examples in the docs still work.
docs-smoke:
    ./scripts/docs-smoke.sh

# Regenerate the agent skill docs from their sources.
sync-skill-docs:
    ./scripts/sync-skill-docs.sh

# Run the minimal origin example.
run-example:
    go run examples/simple_origin/main.go

# Installs published processors from nebu-processor-registry and builds
# the two in-repo educational processors.

# Build every processor binary into bin/.
build-processors:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Installing published processors from nebu-processor-registry..."
    mkdir -p bin
    for p in token-transfer contract-events contract-invocation usdc-filter \
             amount-filter time-window dedup json-file-sink nats-sink postgres-sink; do
        GOWORK=off GOBIN="$(pwd)/bin" go install \
            "github.com/withObsrvr/nebu-processor-registry/processors/$p/cmd/$p@latest"
    done
    echo "Building in-repo educational processors..."
    go build -ldflags "{{ ldflags }}" -o bin/transaction-stats ./examples/processors/transaction-stats/cmd/transaction-stats
    go build -ldflags "{{ ldflags }}" -o bin/ledger-change-stats ./examples/processors/ledger-change-stats/cmd/ledger-change-stats
    echo "✓ All processor binaries built in ./bin/"

# Run after an intentional change to pkg/processor or pkg/source, then
# commit the .api/ diff alongside the code change.

# Regenerate the stable-API snapshots in .api/.
api-snapshot:
    mkdir -p .api
    go doc -all ./pkg/processor > .api/processor.txt
    go doc -all ./pkg/source > .api/source.txt
    @echo "✓ API snapshots regenerated in .api/"

# On drift: confirm the change is acceptable per docs/STABILITY.md, run
# `just api-snapshot`, and commit the .api/ diff so reviewers see the
# contract surface move.

# Verify the stable Go API matches the committed snapshots.
api-check:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p .api
    if ! diff -u .api/processor.txt <(go doc -all ./pkg/processor); then
        echo ""
        echo "ERROR: pkg/processor public API has drifted from the committed snapshot."
        echo "If this change is intentional and acceptable per docs/STABILITY.md,"
        echo "regenerate the snapshot with: just api-snapshot"
        exit 1
    fi
    if ! diff -u .api/source.txt <(go doc -all ./pkg/source); then
        echo ""
        echo "ERROR: pkg/source public API has drifted from the committed snapshot."
        echo "If this change is intentional and acceptable per docs/STABILITY.md,"
        echo "regenerate the snapshot with: just api-snapshot"
        exit 1
    fi
    echo "✓ Stable API surfaces match committed snapshots"
