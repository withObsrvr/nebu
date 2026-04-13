# Backfill Strategies: Processing Millions of Historical Ledgers

This guide covers strategies for backfilling large volumes of historical Stellar ledger data using nebu's archive mode.

## Table of Contents

- [Overview](#overview)
- [Quick Math](#quick-math)
- [Strategy 1: Batched Sequential Processing](#strategy-1-batched-sequential-processing)
- [Strategy 2: Parallel Processing with GNU Parallel](#strategy-2-parallel-processing-with-gnu-parallel)
- [Strategy 3: Process and Store (Medallion Architecture)](#strategy-3-process-and-store-medallion-architecture)
- [Strategy 4: Multi-Machine Parallelization](#strategy-4-multi-machine-parallelization)
- [Performance Tuning](#performance-tuning)
- [Monitoring Progress](#monitoring-progress)
- [Cost Optimization](#cost-optimization)
- [Troubleshooting](#troubleshooting)

## Overview

Backfilling millions of historical ledgers requires careful planning for:
- **Throughput optimization** - Maximize ledgers/second processing rate
- **Fault tolerance** - Handle network issues, rate limits, and failures gracefully
- **Checkpointing** - Resume from failures without reprocessing
- **Parallelization** - Distribute work across multiple workers
- **Cost management** - Minimize egress, storage, and compute costs

Archive mode is **essential** for large backfills because:
- **100-500 ledgers/sec** vs 10-20 for RPC mode
- **No rate limits** from direct bucket access
- **Full history access** - All 60M+ ledgers available
- **Cost effective** - No RPC infrastructure costs

## Quick Math

For a 60 million ledger backfill:

| Metric | Value |
|--------|-------|
| **Total Ledgers** | 60,000,000 |
| **Average Size** | ~100KB per ledger |
| **Total Data** | ~6TB of XDR |
| **Archive Speed** | 100-500 ledgers/sec |
| **Sequential Time** | ~33-55 hours |
| **Parallel Time (8 workers)** | ~4-7 hours |
| **Compressed Size** | ~600GB (10x compression) |

## Strategy 1: Batched Sequential Processing

Simple, reliable approach with checkpointing for resumability.

### Use Case
- Single machine processing
- Moderate urgency
- Simple operational requirements
- Learning/testing workflows

### Implementation

```bash
#!/bin/bash
# backfill_sequential.sh - Batch processing with resume capability

START_LEDGER=1
END_LEDGER=60000000
BATCH_SIZE=100000
OUTPUT_DIR="s3://my-data-lake/bronze/ledgers"
CHECKPOINT_FILE="/tmp/backfill_checkpoint.txt"

# Resume from checkpoint if exists
if [ -f "$CHECKPOINT_FILE" ]; then
  START_LEDGER=$(cat "$CHECKPOINT_FILE")
  echo "Resuming from ledger $START_LEDGER"
fi

# Configure nebu for archive mode
export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2
export NEBU_BUFFER_SIZE=500
export NEBU_NUM_WORKERS=20

for ((current=$START_LEDGER; current<$END_LEDGER; current+=$BATCH_SIZE)); do
  end=$((current + BATCH_SIZE - 1))
  if [ $end -gt $END_LEDGER ]; then
    end=$END_LEDGER
  fi

  echo "[$(date)] Processing ledgers $current to $end..."

  # Fetch and compress to S3
  nebu fetch $current $end | \
    gzip | \
    aws s3 cp - "${OUTPUT_DIR}/ledgers_${current}_${end}.xdr.gz"

  if [ $? -eq 0 ]; then
    # Success - update checkpoint
    echo $((end + 1)) > "$CHECKPOINT_FILE"
    echo "✅ Completed batch $current-$end"
  else
    # Failure - retry once
    echo "❌ Failed batch $current-$end, retrying..."
    sleep 10

    nebu fetch $current $end | \
      gzip | \
      aws s3 cp - "${OUTPUT_DIR}/ledgers_${current}_${end}.xdr.gz"

    if [ $? -ne 0 ]; then
      echo "❌ Retry failed. Stopped at ledger $current"
      exit 1
    fi

    echo "✅ Retry succeeded for $current-$end"
    echo $((end + 1)) > "$CHECKPOINT_FILE"
  fi

  # Optional: Rate limiting / cooling off
  sleep 1
done

echo "🎉 Backfill complete!"
rm "$CHECKPOINT_FILE"
```

### Pros & Cons

**Pros:**
- Simple to understand and debug
- Easy checkpoint/resume logic
- Low resource requirements
- Predictable performance

**Cons:**
- Slowest option (~55 hours for 60M ledgers)
- Single point of failure
- Underutilizes available bandwidth

**Time Estimate:** 33-55 hours

## Strategy 2: Parallel Processing with GNU Parallel

Process multiple ledger ranges simultaneously for much faster throughput.

### Use Case
- Single machine with multiple cores
- Faster completion required
- Cost-effective parallelization
- Production workloads

### Installation

```bash
# Install GNU Parallel
# Ubuntu/Debian
sudo apt-get install parallel

# macOS
brew install parallel

# CentOS/RHEL
sudo yum install parallel
```

### Implementation

```bash
#!/bin/bash
# backfill_parallel.sh - Process multiple ranges in parallel

START_LEDGER=1
END_LEDGER=60000000
BATCH_SIZE=100000
PARALLEL_JOBS=8  # Run 8 batches simultaneously
OUTPUT_DIR="s3://my-data-lake/bronze/ledgers"
LOG_DIR="/tmp/backfill_logs"

mkdir -p "$LOG_DIR"

# Configure nebu for archive mode
export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2
export NEBU_BUFFER_SIZE=500
export NEBU_NUM_WORKERS=20

# Function to process a single batch
process_batch() {
  local start=$1
  local end=$2
  local log_file="${LOG_DIR}/batch_${start}_${end}.log"

  echo "[$(date)] Processing $start-$end" >> "$log_file"

  nebu fetch $start $end 2>> "$log_file" | \
    gzip | \
    aws s3 cp - "${OUTPUT_DIR}/ledgers_${start}_${end}.xdr.gz" 2>> "$log_file"

  if [ $? -eq 0 ]; then
    echo "[$(date)] ✅ Completed $start-$end" >> "$log_file"
    echo "✅ $start-$end"
    return 0
  else
    echo "[$(date)] ❌ Failed $start-$end" >> "$log_file"
    echo "❌ $start-$end"
    return 1
  fi
}

export -f process_batch
export OUTPUT_DIR NEBU_MODE NEBU_BUCKET_PATH NEBU_BUFFER_SIZE NEBU_NUM_WORKERS LOG_DIR

# Generate batch ranges and process in parallel
seq $START_LEDGER $BATCH_SIZE $END_LEDGER | \
  awk -v batch=$BATCH_SIZE -v end=$END_LEDGER \
    '{print $1, ($1 + batch - 1 > end ? end : $1 + batch - 1)}' | \
  parallel -j $PARALLEL_JOBS --colsep ' ' --joblog "${LOG_DIR}/parallel.log" \
    process_batch {1} {2}

# Check results
failed=$(grep -c "❌" "${LOG_DIR}/parallel.log" || true)
if [ "$failed" -gt 0 ]; then
  echo "⚠️  $failed batches failed. Check logs in $LOG_DIR"
  exit 1
fi

echo "🎉 Parallel backfill complete!"
echo "Logs available in: $LOG_DIR"
```

### Resuming Failed Batches

```bash
#!/bin/bash
# retry_failed.sh - Retry failed batches from parallel run

LOG_DIR="/tmp/backfill_logs"
PARALLEL_LOG="${LOG_DIR}/parallel.log"

# Extract failed batches from parallel log
grep "1$" "$PARALLEL_LOG" | \
  awk '{print $NF}' | \
  while read args; do
    echo "Retrying: $args"
    eval "process_batch $args"
  done
```

### Pros & Cons

**Pros:**
- Much faster (7-10 hours for 60M ledgers)
- Better resource utilization
- Built-in retry mechanisms
- Detailed logging per batch

**Cons:**
- More complex than sequential
- Requires GNU Parallel
- Higher memory usage
- Need to manage parallel job count

**Time Estimate:** 7-10 hours (with 8 parallel jobs)

## Strategy 3: Process and Store (Medallion Architecture)

Fetch raw XDR AND process to structured formats simultaneously.

### Use Case
- Building data lakehouse
- Immediate analytics requirements
- Multi-layer storage (Bronze/Silver/Gold)
- Production data pipelines

### Architecture

```
Archive Storage (GCS)
    ↓
nebu fetch (archive mode)
    ↓
    ├─→ Bronze Layer (compressed XDR)    → S3/GCS
    └─→ token-transfer processor
        └─→ Silver Layer (structured JSON) → S3/GCS
```

### Implementation

```bash
#!/bin/bash
# backfill_medallion.sh - Multi-layer processing

START_LEDGER=1
END_LEDGER=60000000
BATCH_SIZE=100000
PARALLEL_JOBS=4
DATA_LAKE="s3://my-data-lake"

export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2
export NEBU_BUFFER_SIZE=500
export NEBU_NUM_WORKERS=20

# Function to calculate date partition from ledger sequence
get_date_partition() {
  local ledger=$1
  # Stellar genesis: 2015-09-30 16:00:00 UTC (ledger 1)
  local genesis_timestamp=1443628800
  local ledger_timestamp=$((genesis_timestamp + (ledger - 1) * 5))
  date -u -d "@$ledger_timestamp" +%Y-%m-%d
}

process_batch() {
  local start=$1
  local end=$2
  local date_partition=$(get_date_partition $start)
  local tmp_dir="/tmp/backfill_$$"

  mkdir -p "$tmp_dir"

  echo "[$(date)] Processing $start-$end (partition: $date_partition)"

  # Fetch ledgers to temporary file
  nebu fetch $start $end > "${tmp_dir}/ledgers.xdr"

  if [ $? -ne 0 ]; then
    echo "❌ Failed to fetch ledgers $start-$end"
    rm -rf "$tmp_dir"
    return 1
  fi

  # Bronze Layer: Store raw compressed XDR
  cat "${tmp_dir}/ledgers.xdr" | gzip > "${tmp_dir}/ledgers.xdr.gz"
  aws s3 cp "${tmp_dir}/ledgers.xdr.gz" \
    "${DATA_LAKE}/bronze/ledgers/date=${date_partition}/ledgers_${start}_${end}.xdr.gz"

  # Silver Layer: Process to structured token transfers
  cat "${tmp_dir}/ledgers.xdr" | \
    token-transfer -q | \
    gzip > "${tmp_dir}/token_transfers.jsonl.gz"

  aws s3 cp "${tmp_dir}/token_transfers.jsonl.gz" \
    "${DATA_LAKE}/silver/token_transfers/date=${date_partition}/transfers_${start}_${end}.jsonl.gz"

  # Optional: Contract events
  cat "${tmp_dir}/ledgers.xdr" | \
    contract-events -q | \
    gzip > "${tmp_dir}/contract_events.jsonl.gz"

  aws s3 cp "${tmp_dir}/contract_events.jsonl.gz" \
    "${DATA_LAKE}/silver/contract_events/date=${date_partition}/events_${start}_${end}.jsonl.gz"

  # Cleanup
  rm -rf "$tmp_dir"

  echo "✅ Completed $start-$end → ${date_partition}"
}

export -f process_batch get_date_partition
export DATA_LAKE NEBU_MODE NEBU_BUCKET_PATH NEBU_BUFFER_SIZE NEBU_NUM_WORKERS

# Run in parallel
seq $START_LEDGER $BATCH_SIZE $END_LEDGER | \
  awk -v batch=$BATCH_SIZE -v end=$END_LEDGER \
    '{print $1, ($1 + batch - 1 > end ? end : $1 + batch - 1)}' | \
  parallel -j $PARALLEL_JOBS --colsep ' ' process_batch {1} {2}

echo "🎉 Medallion backfill complete!"
```

### Data Lake Structure

```
s3://my-data-lake/
├── bronze/
│   └── ledgers/
│       ├── date=2015-09-30/
│       │   └── ledgers_1_100000.xdr.gz
│       ├── date=2015-10-01/
│       │   └── ledgers_100001_200000.xdr.gz
│       └── ...
├── silver/
│   ├── token_transfers/
│   │   ├── date=2015-09-30/
│   │   │   └── transfers_1_100000.jsonl.gz
│   │   └── ...
│   └── contract_events/
│       ├── date=2015-09-30/
│       │   └── events_1_100000.jsonl.gz
│       └── ...
└── gold/
    └── (aggregated analytics)
```

### Pros & Cons

**Pros:**
- Multi-layer data storage
- Immediate analytics capability
- Date partitioning for efficient queries
- Supports multiple downstream use cases

**Cons:**
- Higher processing time per batch
- More storage required
- More complex error handling
- Higher resource requirements

**Time Estimate:** 12-20 hours (with 4 parallel jobs)

## Strategy 4: Multi-Machine Parallelization

Distribute work across multiple machines for maximum throughput.

### Use Case
- Urgent deadlines
- Very large backfills (100M+ ledgers)
- Available cluster resources
- Enterprise data pipelines

### Architecture

```
Coordinator Node
    ↓
    ├─→ Worker 1: Ledgers 1-15M
    ├─→ Worker 2: Ledgers 15M-30M
    ├─→ Worker 3: Ledgers 30M-45M
    └─→ Worker 4: Ledgers 45M-60M
```

### Coordinator Script

```bash
#!/bin/bash
# coordinator.sh - Distribute work across workers

START_LEDGER=1
END_LEDGER=60000000
WORKERS=(
  "worker1.example.com"
  "worker2.example.com"
  "worker3.example.com"
  "worker4.example.com"
)
BATCH_SIZE=100000
OUTPUT_DIR="s3://my-data-lake/bronze/ledgers"

# Calculate work distribution
total_ledgers=$((END_LEDGER - START_LEDGER))
ledgers_per_worker=$((total_ledgers / ${#WORKERS[@]}))

echo "Distributing $total_ledgers ledgers across ${#WORKERS[@]} workers"
echo "Each worker processes ~$ledgers_per_worker ledgers"

for i in "${!WORKERS[@]}"; do
  worker_start=$((START_LEDGER + i * ledgers_per_worker))
  worker_end=$((worker_start + ledgers_per_worker - 1))

  # Last worker gets remainder
  if [ $i -eq $((${#WORKERS[@]} - 1)) ]; then
    worker_end=$END_LEDGER
  fi

  echo "Assigning ledgers $worker_start-$worker_end to ${WORKERS[$i]}"

  # Copy worker script to remote machine
  scp backfill_worker.sh ${WORKERS[$i]}:/tmp/

  # Start worker in background
  ssh ${WORKERS[$i]} "nohup /tmp/backfill_worker.sh $worker_start $worker_end $BATCH_SIZE $OUTPUT_DIR > /tmp/backfill.log 2>&1 &"

  echo "Started worker ${WORKERS[$i]}"
done

echo "All workers started. Monitor progress with:"
for worker in "${WORKERS[@]}"; do
  echo "  ssh $worker 'tail -f /tmp/backfill.log'"
done
```

### Worker Script

```bash
#!/bin/bash
# backfill_worker.sh - Run on each worker machine

START_LEDGER=$1
END_LEDGER=$2
BATCH_SIZE=$3
OUTPUT_DIR=$4

HOSTNAME=$(hostname)
CHECKPOINT_FILE="/tmp/backfill_checkpoint_${HOSTNAME}.txt"

# Resume from checkpoint if exists
if [ -f "$CHECKPOINT_FILE" ]; then
  START_LEDGER=$(cat "$CHECKPOINT_FILE")
  echo "[$HOSTNAME] Resuming from ledger $START_LEDGER"
fi

export NEBU_MODE=archive
export NEBU_DATASTORE_TYPE=S3
export NEBU_BUCKET_PATH="aws-public-blockchain/v1.1/stellar/ledgers/pubnet"
export NEBU_REGION=us-east-2
export NEBU_BUFFER_SIZE=500
export NEBU_NUM_WORKERS=20

echo "[$HOSTNAME] Starting backfill: $START_LEDGER to $END_LEDGER"

for ((current=$START_LEDGER; current<=$END_LEDGER; current+=$BATCH_SIZE)); do
  end=$((current + BATCH_SIZE - 1))
  if [ $end -gt $END_LEDGER ]; then
    end=$END_LEDGER
  fi

  echo "[$HOSTNAME] [$(date)] Processing $current-$end"

  nebu fetch $current $end | \
    gzip | \
    aws s3 cp - "${OUTPUT_DIR}/ledgers_${current}_${end}.xdr.gz"

  if [ $? -eq 0 ]; then
    echo $((end + 1)) > "$CHECKPOINT_FILE"
    echo "[$HOSTNAME] ✅ Completed $current-$end"
  else
    echo "[$HOSTNAME] ❌ Failed $current-$end"
    exit 1
  fi
done

echo "[$HOSTNAME] 🎉 Worker completed!"
rm "$CHECKPOINT_FILE"
```

### Monitoring All Workers

```bash
#!/bin/bash
# monitor_workers.sh

WORKERS=(
  "worker1.example.com"
  "worker2.example.com"
  "worker3.example.com"
  "worker4.example.com"
)

while true; do
  clear
  echo "=== Worker Status at $(date) ==="
  echo

  for worker in "${WORKERS[@]}"; do
    echo "[$worker]"
    ssh $worker "tail -5 /tmp/backfill.log 2>/dev/null || echo 'No logs yet'"
    echo
  done

  sleep 10
done
```

### Pros & Cons

**Pros:**
- Fastest option (2-4 hours for 60M ledgers)
- Scales linearly with worker count
- Fault isolation per worker
- Maximum throughput

**Cons:**
- Most complex setup
- Requires multiple machines
- Network coordination overhead
- Higher operational complexity

**Time Estimate:** 2-4 hours (with 4+ workers)

## Performance Tuning

### Optimize nebu Configuration

```bash
# Maximum throughput configuration
export NEBU_BUFFER_SIZE=500      # Larger buffer (more memory, faster)
export NEBU_NUM_WORKERS=30       # More parallel fetchers

# Memory-constrained configuration
export NEBU_BUFFER_SIZE=100      # Default buffer
export NEBU_NUM_WORKERS=10       # Default workers

# Balanced configuration (recommended)
export NEBU_BUFFER_SIZE=300
export NEBU_NUM_WORKERS=20
```

### Batch Size Selection

| Batch Size | Files Created | Checkpoint Granularity | Memory Usage | Recommended For |
|------------|---------------|------------------------|--------------|-----------------|
| 10,000 | 6,000 | Very fine (50KB checkpoints) | Low | Testing, debugging |
| 50,000 | 1,200 | Fine (250KB checkpoints) | Medium | Conservative production |
| **100,000** | **600** | **Good (500KB checkpoints)** | **Medium** | **Recommended** |
| 500,000 | 120 | Coarse (2.5MB checkpoints) | High | High-throughput |
| 1,000,000 | 60 | Very coarse (5MB checkpoints) | Very high | Maximum speed |

**Recommendation:** Use **100,000 ledgers per batch** for the best balance of:
- Manageable file count
- Reasonable checkpoint granularity
- Good retry efficiency
- Optimal performance

### Network Optimization

```bash
# Run from same cloud region as bucket
# GCS bucket: Run from GCP us-central1
# S3 bucket: Run from AWS us-east-1

# Use cloud VMs with high network bandwidth
# GCP: n2-standard-8 or higher
# AWS: c6i.2xlarge or higher
```

### Resource Monitoring

```bash
# Monitor nebu performance
watch -n 1 'ps aux | grep nebu | head -5'

# Monitor network throughput
nload

# Monitor disk I/O
iostat -x 1

# Monitor S3 upload speed
watch -n 1 'aws s3 ls s3://my-data-lake/bronze/ledgers/ --recursive | tail -5'
```

## Monitoring Progress

### Real-Time Progress Dashboard

```bash
#!/bin/bash
# monitor_progress.sh

OUTPUT_DIR="s3://my-data-lake/bronze/ledgers"
TOTAL_BATCHES=$((60000000 / 100000))

while true; do
  clear
  echo "=== Backfill Progress Dashboard ==="
  echo "Date: $(date)"
  echo

  # Count completed batches
  completed=$(aws s3 ls "$OUTPUT_DIR/" --recursive | wc -l)
  pct=$((completed * 100 / TOTAL_BATCHES))
  remaining=$((TOTAL_BATCHES - completed))

  echo "Completed: $completed / $TOTAL_BATCHES batches ($pct%)"
  echo "Remaining: $remaining batches"
  echo

  # Estimate time remaining (based on last 10 minutes)
  if [ -f /tmp/progress_timestamp ]; then
    last_count=$(cat /tmp/progress_count 2>/dev/null || echo 0)
    last_time=$(cat /tmp/progress_timestamp)
    current_time=$(date +%s)

    elapsed=$((current_time - last_time))
    completed_since=$((completed - last_count))

    if [ $completed_since -gt 0 ] && [ $elapsed -gt 0 ]; then
      rate=$((completed_since * 60 / elapsed))  # batches per hour
      if [ $rate -gt 0 ]; then
        hours_remaining=$((remaining / rate))
        echo "Current rate: $rate batches/hour"
        echo "Est. time remaining: ~$hours_remaining hours"
      fi
    fi
  fi

  # Update progress tracking
  echo $completed > /tmp/progress_count
  date +%s > /tmp/progress_timestamp

  # Show recent files
  echo
  echo "Recent uploads:"
  aws s3 ls "$OUTPUT_DIR/" --recursive | tail -5

  sleep 60
done
```

### Alerts on Completion

```bash
#!/bin/bash
# alert_on_completion.sh

OUTPUT_DIR="s3://my-data-lake/bronze/ledgers"
TOTAL_BATCHES=$((60000000 / 100000))

while true; do
  completed=$(aws s3 ls "$OUTPUT_DIR/" --recursive | wc -l)

  if [ $completed -ge $TOTAL_BATCHES ]; then
    # Send notification (customize for your system)
    echo "🎉 Backfill completed!" | mail -s "Backfill Complete" your-email@example.com

    # Or use Slack
    # curl -X POST -H 'Content-type: application/json' \
    #   --data '{"text":"🎉 Backfill completed!"}' \
    #   YOUR_SLACK_WEBHOOK_URL

    exit 0
  fi

  sleep 300  # Check every 5 minutes
done
```

## Cost Optimization

### Estimate Costs

```bash
# GCS Egress Costs (example)
# 6TB download from GCS (same region): FREE
# 6TB download from GCS (cross-region): ~$600
# 6TB download from GCS (internet): ~$720

# S3 Storage Costs
# 600GB compressed XDR (Standard): ~$15/month
# 600GB compressed XDR (Glacier): ~$2.50/month
```

### Cost Reduction Tips

1. **Run in Same Region**
   ```bash
   # GCS bucket in us-central1
   # Run workers in GCP us-central1 (free egress)
   ```

2. **Use Compression**
   ```bash
   # Always use gzip compression
   nebu fetch 1 100000 | gzip > ledgers.xdr.gz
   # Saves ~90% storage costs
   ```

3. **Lifecycle Policies**
   ```bash
   # Move old data to cheaper storage tiers
   # S3 example:
   aws s3api put-bucket-lifecycle-configuration \
     --bucket my-data-lake \
     --lifecycle-configuration file://lifecycle.json
   ```

   ```json
   {
     "Rules": [{
       "Id": "Archive old ledgers",
       "Filter": {"Prefix": "bronze/ledgers/"},
       "Status": "Enabled",
       "Transitions": [{
         "Days": 90,
         "StorageClass": "GLACIER"
       }]
     }]
   }
   ```

4. **Spot Instances**
   ```bash
   # Use AWS Spot or GCP Preemptible VMs
   # Save 60-80% on compute costs
   # Perfect for batch workloads with checkpointing
   ```

5. **Requester Pays Buckets**
   ```bash
   # Already configured for OBSRVR buckets ✅
   export GOOGLE_APPLICATION_CREDENTIALS="/path/to/credentials.json"
   ```

## Troubleshooting

### Problem: "failed to get ledger X"

**Cause:** Ledger doesn't exist in archive bucket

**Solution:**
```bash
# Find the actual range available in bucket
# Test incremental ranges to find gaps
nebu fetch --mode archive \
  --bucket-path "..." \
  59000000 59100000  # Try different ranges
```

### Problem: "rate limited" or "too many requests"

**Cause:** Too many parallel workers hitting the bucket

**Solution:**
```bash
# Reduce parallel workers
export NEBU_NUM_WORKERS=10  # Instead of 30

# Or reduce parallel batch count
PARALLEL_JOBS=4  # Instead of 8
```

### Problem: Checkpoint corruption

**Cause:** Multiple processes writing to same checkpoint file

**Solution:**
```bash
# Use unique checkpoint files per process
CHECKPOINT_FILE="/tmp/backfill_checkpoint_${START_LEDGER}.txt"
```

### Problem: Out of memory

**Cause:** Too many buffers allocated

**Solution:**
```bash
# Reduce buffer size
export NEBU_BUFFER_SIZE=100  # Instead of 500

# Or reduce parallel jobs
PARALLEL_JOBS=2  # Instead of 8
```

### Problem: Slow S3 uploads

**Cause:** Network bandwidth saturation

**Solution:**
```bash
# Add throttling between batches
sleep 5  # Wait 5 seconds between uploads

# Or use multipart uploads for large files
aws configure set default.s3.multipart_threshold 64MB
```

## Recommendations by Scale

### Small Backfill (< 1M ledgers)

**Recommended:** Strategy 1 (Sequential)
- Simple, reliable
- ~1-2 hours
- No special setup needed

```bash
./backfill_sequential.sh
```

### Medium Backfill (1M - 20M ledgers)

**Recommended:** Strategy 2 (Parallel)
- Good balance of speed and complexity
- ~3-6 hours
- Single machine

```bash
PARALLEL_JOBS=8 ./backfill_parallel.sh
```

### Large Backfill (20M - 60M ledgers)

**Recommended:** Strategy 2 or 3 (Parallel or Medallion)
- Strategy 2 for raw storage: ~7-10 hours
- Strategy 3 for immediate analytics: ~12-20 hours
- Single powerful machine

```bash
# Raw storage
PARALLEL_JOBS=8 ./backfill_parallel.sh

# Or with processing
PARALLEL_JOBS=4 ./backfill_medallion.sh
```

### Very Large Backfill (> 60M ledgers)

**Recommended:** Strategy 4 (Multi-Machine)
- Maximum speed: ~2-4 hours
- Requires cluster/cloud infrastructure
- Enterprise production use

```bash
./coordinator.sh
```

## Summary

| Strategy | Time | Complexity | Use Case |
|----------|------|------------|----------|
| **Sequential** | 33-55h | Low | Learning, testing, small scale |
| **Parallel** | 7-10h | Medium | Production single-machine |
| **Medallion** | 12-20h | Medium-High | Data lakehouse building |
| **Multi-Machine** | 2-4h | High | Enterprise, urgent deadlines |

**For most production use cases, start with Strategy 2 (Parallel Processing)** - it provides the best balance of speed, simplicity, and reliability.

All strategies support:
- ✅ Checkpointing and resume
- ✅ Error handling and retries
- ✅ Progress monitoring
- ✅ Cost optimization via compression
- ✅ Archive mode for maximum performance

Happy backfilling! 🚀
