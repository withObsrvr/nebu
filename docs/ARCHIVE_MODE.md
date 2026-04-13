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

### Default: the public AWS Stellar archive

The simplest setup — **no AWS account required**. AWS maintains a public S3 bucket
with Stellar pubnet ledgers in Galexie format; the SDK falls back to anonymous
access automatically when no AWS credentials are present.

```bash
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "aws-public-blockchain/v1.1/stellar/ledgers/pubnet" \
  --region us-east-2 \
  62080000 62080100
```

- **Bucket:** `s3://aws-public-blockchain/v1.1/stellar/ledgers/pubnet/`
- **Region:** `us-east-2`
- **Network:** pubnet only (testnet data in this bucket uses a different,
  non-Galexie layout and is not supported by `nebu fetch`)
- **Schema:** `zstd` compression, 1 ledger per file, 64,000 files per partition
- **Source of truth:**
  <https://aws-public-blockchain.s3.us-east-2.amazonaws.com/index.html#v1.1/stellar/ledgers/>

### Environment variables (any archive)

```bash
export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2
export NEBU_BUFFER_SIZE=200
export NEBU_NUM_WORKERS=20

nebu fetch 62080000 62080100 > ledgers.xdr
```

### Alternative: GCS or a private S3 bucket

Archive mode also works with any Galexie-format GCS or S3 bucket — e.g. an
internal mirror or a partner-hosted archive:

```bash
# Private S3 bucket (uses AWS SDK default credential chain)
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "my-org-stellar-archive/landing/ledgers/pubnet" \
  --region us-west-2 \
  62080000 62080100

# GCS bucket (uses Application Default Credentials)
nebu fetch --mode archive \
  --datastore-type GCS \
  --bucket-path "my-gcs-bucket/landing/ledgers/pubnet" \
  62080000 62080100
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

### Example 1: Public AWS Stellar archive (pubnet, no credentials)

The fastest way to get started — `aws-public-blockchain` is a public S3 bucket
maintained by AWS. The Stellar SDK's S3 client detects the missing credential
chain and switches to `AnonymousCredentials` automatically.

```bash
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "aws-public-blockchain/v1.1/stellar/ledgers/pubnet" \
  --region us-east-2 \
  62080000 62080010 > ledgers.xdr
```

### Example 2: A private (or internal) S3 archive

```bash
# Uses the standard AWS credential chain (env vars, ~/.aws/credentials, IAM role).
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "my-org-archive/stellar/ledgers/pubnet" \
  --region us-west-2 \
  60000000 60100000 > ledgers.xdr
```

### Example 3: A GCS archive

```bash
# Uses Application Default Credentials (gcloud auth application-default login,
# or GOOGLE_APPLICATION_CREDENTIALS).
nebu fetch --mode archive \
  --datastore-type GCS \
  --bucket-path "my-gcs-archive/landing/ledgers/pubnet" \
  60000000 60100000 > ledgers.xdr
```

### Example 4: Environment-Based Configuration

```bash
# Set once
export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2

# Use multiple times
nebu fetch 62000000 62100000 > batch1.xdr
nebu fetch 62100001 62200000 > batch2.xdr
nebu fetch 62200001 62300000 > batch3.xdr
```

## Real-World Use Cases

### Use Case 1: Building Bronze Layer for Data Lake

Fetch historical ledgers, compress them, and store in S3 for data lakehouse:

```bash
#!/bin/bash
# backfill_bronze_layer.sh

export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2
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
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2

# Fetch, process, filter, and store
nebu fetch 60200000 60300000 | \
  token-transfer -q | \
  jq -c 'select(.transfer != null)' | \
  jq -c 'select(.transfer.assetCode == "USDC")' | \
  gzip > usdc-transfers.jsonl.gz
```

### Use Case 3: Parallel Processing with GNU Parallel

Process multiple ledger ranges in parallel:

```bash
#!/bin/bash
# parallel_process.sh

export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2

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
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2

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

# Then use archive mode against your GCS bucket
nebu fetch --mode archive \
  --datastore-type GCS \
  --bucket-path "my-gcs-archive/landing/ledgers/pubnet" \
  60000000 60100000
```

**Method 2: Service Account in Code**

The DataStore automatically uses Google Cloud SDK's credential chain.

### S3 Authentication

**Method 1: Anonymous (no credentials) — public buckets**

For public buckets like `aws-public-blockchain`, no setup is required. The AWS
SDK attempts its default credential chain first; if nothing is found, it
automatically falls back to anonymous (`AnonymousCredentials`) access.

```bash
# Works with no AWS_* env vars set, no ~/.aws/credentials, no IAM role
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "aws-public-blockchain/v1.1/stellar/ledgers/pubnet" \
  --region us-east-2 \
  62080000 62080100
```

The process logs `No default AWS credentials found, configuring S3 client for
anonymous access` when this path is taken.

**Method 2: AWS CLI Credentials**

For a private bucket (an internal archive, a partner-hosted mirror), use the
standard AWS credential chain:

```bash
# Configure AWS CLI
aws configure

# Or use environment variables
export AWS_ACCESS_KEY_ID="your-access-key"
export AWS_SECRET_ACCESS_KEY="your-secret-key"

nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "my-org-archive/stellar/ledgers/pubnet" \
  --region us-west-2 \
  62080000 62080100
```

**Method 3: IAM Roles (EC2/ECS)**

When running on EC2 or ECS, use IAM roles — no credentials needed:

```bash
nebu fetch --mode archive \
  --datastore-type S3 \
  --bucket-path "my-org-archive/stellar/ledgers/pubnet" \
  --region us-west-2 \
  62080000 62080100
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
