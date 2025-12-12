#!/usr/bin/env bash

# Integration tests for nebu pipelines
# Tests origin → transform → sink flows

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track test results
TESTS_PASSED=0
TESTS_FAILED=0

# Test data
TEST_LEDGER_START=60200000
TEST_LEDGER_END=60200000

# Directories
BIN_DIR="./bin"
TEST_TMP="/tmp/nebu-integration-tests"

# Create temp directory
mkdir -p "$TEST_TMP"

# Cleanup function
cleanup() {
    rm -rf "$TEST_TMP"
}
trap cleanup EXIT

# Helper functions
pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((TESTS_PASSED++))
}

fail() {
    echo -e "${RED}✗${NC} $1"
    ((TESTS_FAILED++))
}

test_header() {
    echo ""
    echo -e "${YELLOW}Testing: $1${NC}"
}

# Build binaries if needed
build_binaries() {
    test_header "Building binaries"

    if [[ ! -f "$BIN_DIR/token-transfer" ]]; then
        go build -o "$BIN_DIR/token-transfer" ./examples/processors/token-transfer/cmd || fail "Failed to build token-transfer"
    fi

    if [[ ! -f "$BIN_DIR/usdc-filter" ]]; then
        go build -o "$BIN_DIR/usdc-filter" ./examples/processors/usdc-filter/cmd || fail "Failed to build usdc-filter"
    fi

    if [[ ! -f "$BIN_DIR/amount-filter" ]]; then
        go build -o "$BIN_DIR/amount-filter" ./examples/processors/amount-filter/cmd || fail "Failed to build amount-filter"
    fi

    if [[ ! -f "$BIN_DIR/dedup" ]]; then
        go build -o "$BIN_DIR/dedup" ./examples/processors/dedup/cmd || fail "Failed to build dedup"
    fi

    if [[ ! -f "$BIN_DIR/json-file-sink" ]]; then
        go build -o "$BIN_DIR/json-file-sink" ./examples/processors/json-file-sink/cmd || fail "Failed to build json-file-sink"
    fi

    pass "All binaries built"
}

# Test 1: Origin processor produces JSON output
test_origin_output() {
    test_header "Origin processor produces valid JSON"

    OUTPUT=$("$BIN_DIR/token-transfer" -q --start-ledger "$TEST_LEDGER_START" --end-ledger "$TEST_LEDGER_END" | head -1)

    if echo "$OUTPUT" | jq -e . > /dev/null 2>&1; then
        pass "Origin produces valid JSON"
    else
        fail "Origin output is not valid JSON"
    fi
}

# Test 2: Quiet mode suppresses stderr
test_quiet_mode() {
    test_header "Quiet mode suppresses progress messages"

    STDERR=$("$BIN_DIR/token-transfer" -q --start-ledger "$TEST_LEDGER_START" --end-ledger "$TEST_LEDGER_END" 2>&1 >/dev/null | wc -l)

    if [[ $STDERR -eq 0 ]]; then
        pass "Quiet mode produces no stderr"
    else
        fail "Quiet mode produced $STDERR lines of stderr"
    fi
}

# Test 3: USDC filter works
test_usdc_filter() {
    test_header "USDC filter only passes USDC events"

    OUTPUT=$("$BIN_DIR/token-transfer" -q --start-ledger "$TEST_LEDGER_START" --end-ledger "$TEST_LEDGER_END" | \
             "$BIN_DIR/usdc-filter" -q | \
             jq -r '.asset.code' | \
             sort | uniq)

    if [[ "$OUTPUT" == "USDC" ]]; then
        pass "USDC filter passes only USDC events"
    else
        fail "USDC filter passed non-USDC events: $OUTPUT"
    fi
}

# Test 4: Amount filter works
test_amount_filter() {
    test_header "Amount filter filters by minimum amount"

    # Get all amounts after filtering for min 10M
    MIN_AMOUNT=$("$BIN_DIR/token-transfer" -q --start-ledger "$TEST_LEDGER_START" --end-ledger "$TEST_LEDGER_END" | \
                 "$BIN_DIR/amount-filter" -q --min 10000000 | \
                 jq -r '.amount' | \
                 sort -n | \
                 head -1)

    if [[ $MIN_AMOUNT -ge 10000000 ]]; then
        pass "Amount filter respects minimum ($MIN_AMOUNT >= 10000000)"
    else
        fail "Amount filter passed amounts below minimum ($MIN_AMOUNT < 10000000)"
    fi
}

# Test 5: Dedup removes duplicates
test_dedup() {
    test_header "Dedup removes duplicate events"

    # Create test data with duplicates
    TEST_FILE="$TEST_TMP/test_dedup.jsonl"
    cat > "$TEST_FILE" <<EOF
{"tx_hash":"abc123","amount":"100"}
{"tx_hash":"def456","amount":"200"}
{"tx_hash":"abc123","amount":"100"}
{"tx_hash":"ghi789","amount":"300"}
{"tx_hash":"def456","amount":"200"}
EOF

    UNIQUE_COUNT=$(cat "$TEST_FILE" | "$BIN_DIR/dedup" -q | wc -l)

    if [[ $UNIQUE_COUNT -eq 3 ]]; then
        pass "Dedup removed duplicates (3 unique out of 5)"
    else
        fail "Dedup produced $UNIQUE_COUNT events (expected 3)"
    fi
}

# Test 6: Full pipeline (origin → transform → sink)
test_full_pipeline() {
    test_header "Full pipeline: origin → filter → sink"

    OUTPUT_FILE="$TEST_TMP/pipeline_output.jsonl"

    "$BIN_DIR/token-transfer" -q --start-ledger "$TEST_LEDGER_START" --end-ledger "$TEST_LEDGER_END" | \
        "$BIN_DIR/usdc-filter" -q | \
        "$BIN_DIR/json-file-sink" -q --out "$OUTPUT_FILE"

    if [[ -f "$OUTPUT_FILE" ]]; then
        LINE_COUNT=$(wc -l < "$OUTPUT_FILE")
        if [[ $LINE_COUNT -gt 0 ]]; then
            pass "Full pipeline produced output ($LINE_COUNT events)"
        else
            fail "Full pipeline produced empty file"
        fi
    else
        fail "Full pipeline did not create output file"
    fi
}

# Test 7: Chain multiple transforms
test_multi_transform() {
    test_header "Chain multiple transforms"

    OUTPUT_FILE="$TEST_TMP/multi_transform.jsonl"

    "$BIN_DIR/token-transfer" -q --start-ledger "$TEST_LEDGER_START" --end-ledger "$TEST_LEDGER_END" | \
        "$BIN_DIR/amount-filter" -q --min 10000000 | \
        "$BIN_DIR/usdc-filter" -q | \
        "$BIN_DIR/json-file-sink" -q --out "$OUTPUT_FILE"

    if [[ -f "$OUTPUT_FILE" ]]; then
        # Verify all events are USDC and >= 10M
        VALID=true
        while IFS= read -r line; do
            ASSET=$(echo "$line" | jq -r '.asset.code')
            AMOUNT=$(echo "$line" | jq -r '.amount')

            if [[ "$ASSET" != "USDC" ]] || [[ $AMOUNT -lt 10000000 ]]; then
                VALID=false
                break
            fi
        done < "$OUTPUT_FILE"

        if [[ "$VALID" == "true" ]]; then
            pass "Multi-transform pipeline filters correctly"
        else
            fail "Multi-transform pipeline produced invalid events"
        fi
    else
        fail "Multi-transform pipeline did not create output file"
    fi
}

# Test 8: DuckDB integration
test_duckdb_integration() {
    test_header "DuckDB integration via cookbook pattern"

    if ! command -v duckdb &> /dev/null; then
        echo -e "${YELLOW}⊘${NC} DuckDB not installed, skipping test"
        return
    fi

    DB_FILE="$TEST_TMP/test.duckdb"

    "$BIN_DIR/token-transfer" -q --start-ledger "$TEST_LEDGER_START" --end-ledger "$TEST_LEDGER_END" | \
        "$BIN_DIR/usdc-filter" -q | \
        duckdb "$DB_FILE" -c "CREATE TABLE transfers AS SELECT * FROM read_json('/dev/stdin')"

    COUNT=$(duckdb "$DB_FILE" -c "SELECT COUNT(*) FROM transfers" -noheader -list)

    if [[ $COUNT -gt 0 ]]; then
        pass "DuckDB integration works ($COUNT events imported)"
    else
        fail "DuckDB integration produced no data"
    fi
}

# Run all tests
main() {
    echo "======================================"
    echo "  nebu Integration Tests"
    echo "======================================"

    build_binaries
    test_origin_output
    test_quiet_mode
    test_usdc_filter
    test_amount_filter
    test_dedup
    test_full_pipeline
    test_multi_transform
    test_duckdb_integration

    echo ""
    echo "======================================"
    echo "  Test Results"
    echo "======================================"
    echo -e "${GREEN}Passed:${NC} $TESTS_PASSED"
    echo -e "${RED}Failed:${NC} $TESTS_FAILED"

    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo -e "\n${GREEN}All tests passed!${NC}"
        exit 0
    else
        echo -e "\n${RED}Some tests failed${NC}"
        exit 1
    fi
}

main
