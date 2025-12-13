# Archive Mode: Fetching Ledgers from GCS/S3

Archive mode enables `nebu fetch` to read Stellar ledger data directly from cloud storage (GCS or S3), bypassing RPC endpoints for faster, more cost-effective access to historical data.

## Table of Contents

- [Overview](#overview)
- [When to Use Archive Mode](#when-to-use-archive-mode)
- [Configuration](#configuration)
- [Quick Start Examples](#quick-start-examples)
- [Real-World Use Cases](#real-world-use-cases)
- [Performance Tuning](#performance-tuning)
- [Authentication](#authentication)
- [Troubleshooting](#troubleshooting)

## Overview

Archive mode uses Stellar SDK's `BufferedStorageBackend` to read ledger data from cloud storage buckets where Stellar's Galexie service exports ledgers in XDR format.

### RPC Mode vs Archive Mode

| Feature | RPC Mode | Archive Mode |
|---------|----------|--------------|
| **Speed** | 10-20 ledgers/sec | 100-500 ledgers/sec |
| **History** | Last 24 hours only | Full Stellar history |
| **Cost** | RPC rate limits apply | Direct bucket access |
| **Best For** | Recent ledgers, live streaming | Historical data, data lakes |
| **Requirements** | RPC endpoint | Cloud storage credentials |

## When to Use Archive Mode

**Use Archive Mode for:**
- Building data lakehouses (Bronze layer ingestion)
- Backfilling historical data
- Processing millions of ledgers
- Avoiding RPC rate limits
- Cost-effective bulk data access

**Use RPC Mode for:**
- Recent ledgers (last 24 hours)
- Live streaming / real-time processing
- Quick one-off queries
- Testing and development

## Configuration

### Command-Line Flags

```bash
nebu fetch --mode archive \
  --datastore-type GCS \
  --bucket-path "obsrvr-stellar-ledger-data-pubnet-data/landing/ledgers/pubnet" \
  --buffer-size 100 \
  --num-workers 10 \
  60000000 60100000
```

### Environment Variables

```bash
export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=GCS
export NEBU_BUCKET_PATH="obsrvr-stellar-ledger-data-pubnet-data/landing/ledgers/pubnet"
export NEBU_BUFFER_SIZE=200
export NEBU_NUM_WORKERS=20

nebu fetch 60000000 60100000 > ledgers.xdr
```

### Configuration Options

| Flag / Env Var | Description | Default | Values |
|----------------|-------------|---------|--------|
| `--mode` / `NEBU_MODE` | Source mode | `rpc` | `rpc`, `archive` |
| `--datastore-type` / `NEBU_DATASTORE_TYPE` | Cloud storage type | `GCS` | `GCS`, `S3` |
| `--bucket-path` / `NEBU_BUCKET_PATH` | Bucket path | (required) | Bucket name + path |
| `--region` / `NEBU_REGION` | S3 region | `us-east-1` | AWS region |
| `--buffer-size` / `NEBU_BUFFER_SIZE` | Ledger cache size | `100` | 10-1000 |
| `--num-workers` / `NEBU_NUM_WORKERS` | Parallel workers | `10` | 1-50 |

## Quick Start Examples

### Example 1: Fetch from GCS (Mainnet)

```bash
# Using OBSRVR's public mainnet bucket
nebu fetch --mode archive \
  --datastore-type GCS \
  --bucket-path "obsrvr-stellar-ledger-data-pubnet-data/landing/ledgers/pubnet" \
  59785581 59785583 > ledgers.xdr

# Output: Fetched ledgers 59785581-59785583 from GCS
```

### Example 2: Fetch from GCS (Testnet)

```bash
# Using OBSRVR's public testnet bucket
nebu fetch --mode archive \
  --datastore-type GCS \
  --bucket-path "obsrvr-stellar-ledger-data-testnet-data/landing/ledgers/testnet" \
  30000 40000 > testnet-ledgers.xdr
```

### Example 3: Fetch from S3

```bash
# Using AWS S3 bucket
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "my-s3-bucket/stellar/ledgers/mainnet" \
  --region us-west-2 \
  60000000 60100000 > ledgers.xdr
```

### Example 4: Environment-Based Configuration

```bash
# Set once
export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=GCS
export NEBU_BUCKET_PATH="obsrvr-stellar-ledger-data-pubnet-data/landing/ledgers/pubnet"

# Use multiple times
nebu fetch 60000000 60100000 > batch1.xdr
nebu fetch 60100001 60200000 > batch2.xdr
nebu fetch 60200001 60300000 > batch3.xdr
```

## Real-World Use Cases

### Use Case 1: Building Bronze Layer for Data Lake

Fetch historical ledgers, compress them, and store in S3 for data lakehouse:

```bash
#!/bin/bash
# backfill_bronze_layer.sh

export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=GCS
export NEBU_BUCKET_PATH="obsrvr-stellar-ledger-data-pubnet-data/landing/ledgers/pubnet"
export NEBU_BUFFER_SIZE=500
export NEBU_NUM_WORKERS=20

START_LEDGER=50000000
END_LEDGER=60000000
BATCH_SIZE=100000

for ((current=$START_LEDGER; current<$END_LEDGER; current+=$BATCH_SIZE)); do
  end=$((current + BATCH_SIZE - 1))
  if [ $end -gt $END_LEDGER ]; then
    end=$END_LEDGER
  fi

  echo "Fetching ledgers $current to $end..."

  nebu fetch $current $end | \
    gzip | \
    aws s3 cp - "s3://my-data-lake/bronze/ledgers_${current}_${end}.xdr.gz"

  echo "Completed batch $current-$end"
done
```

### Use Case 2: Stream to Processor Pipeline

Fetch from archive and process through token-transfer pipeline:

```bash
export NEBU_MODE=archive
export NEBU_BUCKET_PATH="obsrvr-stellar-ledger-data-pubnet-data/landing/ledgers/pubnet"

# Fetch, process, filter, and store
nebu fetch 60200000 60300000 | \
  token-transfer -q | \
  jq -c 'select(.transfer != null)' | \
  jq -c 'select(.transfer.asset.code == "USDC")' | \
  gzip > usdc-transfers.jsonl.gz
```

### Use Case 3: Parallel Processing with GNU Parallel

Process multiple ledger ranges in parallel:

```bash
#!/bin/bash
# parallel_process.sh

export NEBU_MODE=archive
export NEBU_BUCKET_PATH="obsrvr-stellar-ledger-data-pubnet-data/landing/ledgers/pubnet"

# Generate ledger ranges
seq 60000000 100000 61000000 | \
  parallel -j 4 "nebu fetch {} $(({}+99999)) | \
                  token-transfer -q | \
                  gzip > output_{}.jsonl.gz"
```

### Use Case 4: DuckDB Analytics Pipeline

Fetch from archive and load into DuckDB for analytics:

```bash
export NEBU_MODE=archive
export NEBU_BUCKET_PATH="obsrvr-stellar-ledger-data-pubnet-data/landing/ledgers/pubnet"

# Fetch and process into JSONL
nebu fetch 60200000 60300000 | \
  token-transfer -q | \
  jq -c 'select(.transfer)' > transfers.jsonl

# Analyze with DuckDB
duckdb <<SQL
CREATE TABLE transfers AS
SELECT * FROM read_json_auto('transfers.jsonl');

-- Top assets by transfer count
SELECT
  transfer->>'asset' as asset,
  COUNT(*) as transfer_count,
  SUM((transfer->>'amount')::BIGINT) as total_amount
FROM transfers
WHERE transfer IS NOT NULL
GROUP BY asset
ORDER BY transfer_count DESC
LIMIT 10;
SQL
```

## Performance Tuning

### Buffer Size

Controls how many ledgers are cached in memory:

```bash
# Small buffer (low memory, slower)
--buffer-size 50

# Default (balanced)
--buffer-size 100

# Large buffer (more memory, faster for sequential access)
--buffer-size 500
```

**Recommendations:**
- **Small datasets (<10k ledgers)**: 50-100
- **Medium datasets (10k-100k ledgers)**: 100-200
- **Large datasets (>100k ledgers)**: 200-500
- **Memory-constrained systems**: 50-100

**Memory usage**: ~(buffer_size × 100KB) per ledger

### Number of Workers

Controls parallel fetch threads:

```bash
# Low parallelism (slower, less network usage)
--num-workers 5

# Default (balanced)
--num-workers 10

# High parallelism (faster, more network usage)
--num-workers 20
```

**Recommendations:**
- **Slow network**: 5-10
- **Fast network**: 10-20
- **Very fast network / local datacenter**: 20-50

**Note**: Performance scales linearly up to ~20 workers, then diminishing returns.

### Optimal Configuration for Common Scenarios

#### Scenario 1: Fast Backfill (Speed Priority)

```bash
export NEBU_BUFFER_SIZE=500
export NEBU_NUM_WORKERS=20
```

Expected: ~300-500 ledgers/sec

#### Scenario 2: Memory-Constrained (Low Memory Priority)

```bash
export NEBU_BUFFER_SIZE=50
export NEBU_NUM_WORKERS=5
```

Expected: ~50-100 ledgers/sec, ~5MB RAM

#### Scenario 3: Balanced (Default)

```bash
export NEBU_BUFFER_SIZE=100
export NEBU_NUM_WORKERS=10
```

Expected: ~100-200 ledgers/sec, ~10MB RAM

## Authentication

### GCS Authentication

**Method 1: Application Default Credentials (Recommended)**

```bash
# Authenticate with gcloud
gcloud auth application-default login

# Or use service account
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"

# Then use archive mode normally
nebu fetch --mode archive \
  --bucket-path "obsrvr-stellar-ledger-data-pubnet-data/landing/ledgers/pubnet" \
  60000000 60100000
```

**Method 2: Service Account in Code**

The DataStore automatically uses Google Cloud SDK's credential chain.

### S3 Authentication

**Method 1: AWS CLI Credentials (Recommended)**

```bash
# Configure AWS CLI
aws configure

# Or use environment variables
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"
export AWS_REGION="us-west-2"

# Then use archive mode
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "my-s3-bucket/stellar/ledgers" \
  --region us-west-2 \
  60000000 60100000
```

**Method 2: IAM Roles (EC2/ECS)**

When running on EC2 or ECS, use IAM roles - no credentials needed:

```bash
# Just specify bucket path, credentials come from instance role
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "my-s3-bucket/stellar/ledgers" \
  60000000 60100000
```

## Troubleshooting

### Error: "datastore type cannot be empty"

**Cause**: Missing `--datastore-type` flag

**Solution**:
```bash
# Add --datastore-type flag
nebu fetch --mode archive --datastore-type GCS --bucket-path "..." ...
```

### Error: "bucket-path is required when using --mode archive"

**Cause**: Missing `--bucket-path` flag

**Solution**:
```bash
# Add --bucket-path flag
nebu fetch --mode archive --bucket-path "my-bucket/path" ...
```

### Error: "Access Denied" or "403 Forbidden"

**Cause**: Missing or invalid cloud storage credentials

**GCS Solution**:
```bash
# Authenticate with gcloud
gcloud auth application-default login

# Or set service account key
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/key.json"
```

**S3 Solution**:
```bash
# Configure AWS credentials
aws configure

# Or set environment variables
export AWS_ACCESS_KEY_ID="your-key"
export AWS_SECRET_ACCESS_KEY="your-secret"
```

### Error: "failed to get ledger X"

**Cause**: Ledger doesn't exist in bucket or bucket schema mismatch

**Solution**:
1. Verify ledger range exists in bucket
2. Check bucket path is correct
3. Ensure bucket uses standard Galexie schema (ledgers_per_file=1, files_per_partition=64000)

### Slow Performance

**Diagnosis**:
```bash
# Test with small range first
nebu fetch --mode archive --bucket-path "..." 60000000 60000010

# Monitor with verbose logging
NEBU_LOG_LEVEL=debug nebu fetch --mode archive ...
```

**Solutions**:
1. **Increase workers**: `--num-workers 20`
2. **Increase buffer**: `--buffer-size 500`
3. **Check network**: Run from same cloud region as bucket
4. **Verify bucket access**: Ensure low-latency access

### Memory Issues

**Symptom**: Out of memory errors or system slowdown

**Solution**: Reduce buffer size
```bash
# Reduce from default 100 to 50
nebu fetch --mode archive --buffer-size 50 ...
```

## Bucket Schema

Archive mode expects ledgers to be organized using Galexie's standard schema:

```
bucket-path/
├── 00/
│   ├── 00/
│   │   ├── 00/
│   │   │   ├── ledger-00000001.xdr.zst
│   │   │   ├── ledger-00000002.xdr.zst
│   │   │   └── ...
│   │   └── 01/
│   │       ├── ledger-00064001.xdr.zst
│   │       └── ...
│   └── ...
└── ...
```

**Default Schema Parameters:**
- `ledgers_per_file`: 1 (one ledger per file)
- `files_per_partition`: 64,000 (64k files per directory)
- Compression: zstd
- Format: XDR

These defaults are automatically used and match Galexie's output format.

## Further Reading

- [Stellar Galexie Documentation](https://github.com/stellar/go/tree/master/services/galexie)
- [Stellar SDK BufferedStorageBackend](https://pkg.go.dev/github.com/stellar/go/ingest/ledgerbackend#BufferedStorageBackend)
- [Building Data Lakehouses](../README.md#medallion-architecture)
- [nebu Architecture](../README.md#architecture)
