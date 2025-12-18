# Deploying nebu Indexers to Fly.io

**Deploy your nebu blockchain indexer to production in 30 minutes.**

This guide shows you how to run a nebu indexer on [Fly.io](https://fly.io) with PostgreSQL for storing events. Fly.io provides a simple, cost-effective platform for running long-lived processes like blockchain indexers.

---

## Table of Contents

- [What You'll Deploy](#what-youll-deploy)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Step-by-Step Guide](#step-by-step-guide)
- [Local Testing](#local-testing)
- [Customization Guide](#customization-guide)
- [Monitoring and Debugging](#monitoring-and-debugging)
- [Cost Estimate](#cost-estimate)
- [Troubleshooting](#troubleshooting)
- [FAQ](#faq)

---

## What You'll Deploy

This example deploys a **contract events indexer** that:

- ✅ Reads contract invocation events from Stellar blockchain
- ✅ Writes events to PostgreSQL database
- ✅ Runs 24/7 with auto-restart on failures
- ✅ Supports historical backfill
- ✅ Includes health checks and monitoring

**Architecture:**

```
┌─────────────────────────────────────────┐
│  Stellar Blockchain (Mainnet/Testnet)  │
└──────────────────┬──────────────────────┘
                   │ RPC
                   ▼
┌─────────────────────────────────────────┐
│  Fly.io App: nebu-indexer               │
│  ┌───────────────────────────────────┐  │
│  │ contract-events | postgres-sink  │  │
│  └───────────────────────────────────┘  │
└──────────────────┬──────────────────────┘
                   │ INSERT events
                   ▼
┌─────────────────────────────────────────┐
│  Fly Postgres: contract_events table    │
└─────────────────────────────────────────┘
```

---

## Prerequisites

### Required

1. **Fly.io Account**
   - Sign up at [fly.io/signup](https://fly.io/signup)
   - Free tier includes: 3 shared VMs, 3GB storage, 160GB bandwidth

2. **flyctl CLI**
   ```bash
   # Install flyctl
   curl -L https://fly.io/install.sh | sh

   # Login
   fly auth login
   ```

3. **Docker** (for local testing)
   ```bash
   docker --version  # Should show v20.10 or later
   ```

**Note:** The Dockerfile uses the latest stable Go version (currently 1.23+) to build nebu processors. If you're building locally, ensure you have a compatible Go version installed.

### Optional

- **PostgreSQL client** (for database access)
  ```bash
  # macOS
  brew install postgresql

  # Ubuntu/Debian
  apt install postgresql-client
  ```

---

## Quick Start

**Deploy in 5 commands:**

```bash
# 1. Copy this example directory
cp -r examples/deployments/flyio my-indexer
cd my-indexer

# 2. Create Fly.io app
fly launch --no-deploy

# 3. Create PostgreSQL cluster
fly postgres create --name my-indexer-db

# 4. Attach database to app
fly postgres attach my-indexer-db

# 5. Deploy!
fly deploy
```

**Check status:**

```bash
fly logs -f
```

You should see:
```
[info] Starting nebu Indexer
[info] Configuration:
[info]   RPC URL: https://archive-rpc.lightsail.network
[info]   Start Ledger: latest
[info] Starting indexer pipeline...
[info] Processing ledger 62500000...
```

✅ **Your indexer is now running 24/7!**

---

## Step-by-Step Guide

### Step 1: Clone Example

```bash
# Navigate to nebu repository
cd /path/to/nebu

# Copy deployment example
cp -r examples/deployments/flyio my-indexer
cd my-indexer
```

**What's included:**

```
my-indexer/
├── Dockerfile             # Multi-stage Docker build
├── fly.toml              # Fly.io configuration
├── docker-compose.yml    # Local testing setup
├── scripts/
│   ├── start-indexer.sh  # Main indexer startup
│   ├── backfill.sh       # Historical data backfill
│   └── health-check.sh   # Health monitoring
└── migrations/
    ├── 001_create_contract_events_table.sql
    └── 002_create_metadata_tables.sql
```

---

### Step 2: Customize Configuration

#### Edit `fly.toml`

```toml
# Change app name to something unique
app = "my-stellar-indexer"  # ← Change this

# Choose region close to you or Stellar infrastructure
primary_region = "sjc"  # San Jose (good for Stellar)
# Other options: iad (Ashburn), ams (Amsterdam), syd (Sydney)
```

#### Edit `scripts/start-indexer.sh` (Optional)

Customize the pipeline for your needs:

```bash
# Default pipeline
contract-events \
    --start-ledger "$START_LEDGER" \
    --follow \
    --rpc-url "$RPC_URL" \
    | postgres-sink \
        --table contract_events \
        --database-url "$DATABASE_URL" \
        --batch-size "$BATCH_SIZE"
```

See [Customization Guide](#customization-guide) for more options.

---

### Step 3: Create Fly.io App

```bash
fly launch --no-deploy
```

**What this does:**
- Creates app in Fly.io
- Generates unique app name (or uses name from fly.toml)
- Does NOT deploy yet (we'll do that after database setup)

**Output:**
```
? Choose an app name (leave blank to generate one): my-stellar-indexer
? Choose a region for deployment: San Jose, California (US) (sjc)
Created app 'my-stellar-indexer' in organization 'personal'
```

---

### Step 4: Create PostgreSQL Database

```bash
fly postgres create --name my-indexer-db
```

**Configuration prompts:**

```
? Select configuration:
  > Development - Single node, 1x shared CPU, 256MB RAM, 1GB disk
    Production - Highly available, 2x shared CPU, 4GB RAM, 40GB disk

? Select region: San Jose, California (US) (sjc)
```

**Recommendation:** Start with **Development** to save costs. Upgrade later if needed.

**Output:**
```
Postgres cluster my-indexer-db created
  Username:    postgres
  Password:    <generated-password>
  Hostname:    my-indexer-db.internal
  Proxy port:  5432
  Postgres port: 5433
  Connection string: postgres://postgres:<password>@my-indexer-db.internal:5432
```

---

### Step 5: Attach Database to App

```bash
fly postgres attach my-indexer-db --app my-stellar-indexer
```

**What this does:**
- Sets `DATABASE_URL` secret in your app
- Allows your indexer to connect to Postgres

**Output:**
```
Postgres cluster my-indexer-db is now attached to my-stellar-indexer
The following secret was added to my-stellar-indexer:
  DATABASE_URL=postgres://...
```

---

### Step 6: Create Persistent Volume (Optional but Recommended)

```bash
fly volumes create indexer_data --size 10 --region sjc
```

**Why:** Stores checkpoints and logs that persist across restarts.

**Output:**
```
        ID: vol_xyz123
      Name: indexer_data
       App: my-stellar-indexer
    Region: sjc
      Zone: b
   Size GB: 10
```

---

### Step 7: Deploy Indexer

```bash
fly deploy
```

**What happens:**
1. Builds Docker image (multi-stage build)
2. Pushes image to Fly.io registry
3. Creates release and starts app
4. Runs health checks

**Output:**
```
==> Building image
...
--> Building image done
==> Pushing image to fly
...
--> Pushing image done
==> Creating release
--> Release v1 created

--> This deployment will:
 * create 1 "indexer" machine

--> Creating machine
...
Machine started!

Visit your newly deployed app at https://my-stellar-indexer.fly.dev/
```

**Note:** The indexer doesn't serve HTTP, so the URL won't show anything. That's normal!

---

### Step 8: Verify Deployment

#### Check logs

```bash
fly logs -f
```

**Expected output:**
```
[info] ========================================
[info] Starting nebu Indexer
[info] ========================================
[info] Configuration:
[info]   RPC URL: https://archive-rpc.lightsail.network
[info]   Start Ledger: latest
[info]   Batch Size: 1000
[info] Running pre-flight checks...
[info] ✓ Database connection successful
[info] ✓ Pre-flight checks complete
[info] Running database migrations...
[info]   Applying 001_create_contract_events_table.sql...
[info]   Applying 002_create_metadata_tables.sql...
[info] ✓ Migrations complete
[info] Starting indexer pipeline...
[info] Processing ledger 62500123...
```

#### Connect to database

```bash
fly postgres connect -a my-indexer-db
```

**Query events:**

```sql
-- Count total events
SELECT COUNT(*) FROM contract_events;

-- View recent events
SELECT
  ledger_sequence,
  contract_id,
  event_type,
  ledger_closed_at
FROM contract_events
ORDER BY ledger_sequence DESC
LIMIT 10;

-- Exit
\q
```

✅ **If you see events, your indexer is working!**

---

## Local Testing

Test your indexer locally before deploying to Fly.io.

### Using Docker Compose

```bash
# Start PostgreSQL + indexer
docker-compose up

# Check logs
docker-compose logs -f indexer

# Stop
docker-compose down
```

**Access database:**
```bash
# PostgreSQL (port 5432)
psql postgresql://nebu:nebu_local_password@localhost:5432/nebu_indexer

# pgAdmin (optional - requires --profile debug)
docker-compose --profile debug up
# Open http://localhost:5050
# Email: admin@nebu.local, Password: admin
```

### Test with Testnet

Edit `docker-compose.yml`:

```yaml
environment:
  RPC_URL: https://soroban-testnet.stellar.org
  START_LEDGER: "500000"  # Lower ledger for testnet
```

---

## Customization Guide

### 1. Add More Processors

Edit `Dockerfile` to build additional processors:

```dockerfile
# Build token-transfer processor
WORKDIR /build/nebu/examples/processors/token-transfer
RUN go build -o /processors/token-transfer \
    -ldflags="-s -w" \
    ./cmd/token-transfer
```

**Note:** Each processor has its own `go.mod` file, so you must `WORKDIR` into the processor directory before building.

Edit `scripts/start-indexer.sh` to use them:

```bash
# Run multiple pipelines in parallel
token-transfer --follow --rpc-url "$RPC_URL" \
    | postgres-sink --table token_transfers --database-url "$DATABASE_URL" &

contract-events --follow --rpc-url "$RPC_URL" \
    | postgres-sink --table contract_events --database-url "$DATABASE_URL" &

wait  # Wait for all background processes
```

### 2. Add Pipeline Filters

The deployment includes built-in support for transform processors. Configure filters using the `PIPELINE_FILTERS` environment variable.

#### Available Filters

**contract-filter** - Filter events by specific contract IDs
```toml
[env]
  PIPELINE_FILTERS = "contract-filter"
  # Comma-separated list of contract IDs to index
  CONTRACT_IDS = "CAIXKZD...UUQ,CBVMR...ABC,CDZYX...XYZ"
```

**dedup** - Remove duplicate events
```toml
[env]
  PIPELINE_FILTERS = "dedup"
```

**amount-filter** - Filter events by amount threshold
```toml
[env]
  PIPELINE_FILTERS = "amount-filter"
  MIN_AMOUNT = "1000000"  # Minimum amount in stroops
```

**jq filter** - Custom JSON filtering using jq expressions
```toml
[env]
  # Only index events where amount > 1000000
  PIPELINE_FILTERS = "jq:.data.amount>1000000"
```

#### Chain Multiple Filters

Use `|` to chain filters together:

```toml
[env]
  # Filter specific contracts, then deduplicate
  PIPELINE_FILTERS = "contract-filter|dedup"
  CONTRACT_IDS = "CAIXKZD...UUQ,CBVMR...ABC"
```

```toml
[env]
  # Filter contracts, deduplicate, then filter by amount
  PIPELINE_FILTERS = "contract-filter|dedup|amount-filter"
  CONTRACT_IDS = "CAIXKZD...UUQ"
  MIN_AMOUNT = "5000000"
```

```toml
[env]
  # Deduplicate, then custom jq filter
  PIPELINE_FILTERS = "dedup|jq:.data.amount>1000000"
```

#### Configure via Fly Secrets

For production, use secrets instead of env vars in fly.toml:

```bash
# Filter specific contracts
fly secrets set PIPELINE_FILTERS="contract-filter|dedup"
fly secrets set CONTRACT_IDS="CAIXKZD...UUQ,CBVMR...ABC"

# Or filter by amount
fly secrets set PIPELINE_FILTERS="dedup|amount-filter"
fly secrets set MIN_AMOUNT="1000000"
```

#### View Active Pipeline

Check logs to see your configured pipeline:

```bash
fly logs
```

Output shows the active filter chain:
```
Building pipeline...
  Origin: contract-events
  Filter: contract-filter (contracts: CAIXKZD...UUQ,CBVMR...ABC)
  Filter: dedup (remove duplicates)
  Filter: amount-filter (min: 1000000)
  Sink: postgres-sink (table: contract_events)
```

### 3. Configure Historical Backfill

Set starting ledger in `fly.toml`:

```toml
[env]
  START_LEDGER = "60000000"  # Start from specific ledger
```

Or run manual backfill:

```bash
fly ssh console
/scripts/backfill.sh 60000000
```

### 4. Use External Postgres

Instead of Fly Postgres, use Neon, Supabase, or any Postgres provider:

```bash
fly secrets set DATABASE_URL="postgresql://user:pass@external-db.com:5432/db"
```

### 5. Multi-Region Deployment

Deploy to multiple regions for redundancy:

```bash
# Scale to 2 machines in different regions
fly scale count 2 --region sjc,ams
```

**Warning:** Both will write to the same database. Ensure your pipeline handles deduplication.

### 6. Resource Tuning

Edit `fly.toml` based on event volume:

```toml
[vm]
  # Low volume (< 1000 events/min)
  cpus = 1
  memory_mb = 1024

  # Medium volume (1000-10000 events/min)
  cpus = 2
  memory_mb = 2048

  # High volume (> 10000 events/min)
  cpu_kind = "performance"
  cpus = 4
  memory_mb = 4096
```

---

## Monitoring and Debugging

### View Logs

```bash
# Real-time logs
fly logs -f

# Last 100 lines
fly logs --lines 100

# Search logs
fly logs | grep ERROR
```

### Check App Status

```bash
fly status
```

**Output:**
```
App
  Name     = my-stellar-indexer
  Owner    = personal
  Hostname = my-stellar-indexer.fly.dev
  Platform = machines

Machines
ID              STATE   REGION  HEALTH  LAST UPDATED
148ed12c49e789  started sjc     passing 2m ago
```

### SSH into Machine

```bash
fly ssh console

# Inside the machine:
ps aux | grep contract-events
tail -f /data/indexer.log
psql $DATABASE_URL
```

### View Metrics

```bash
fly dashboard metrics
```

Opens web dashboard showing:
- CPU usage
- Memory usage
- Network I/O
- Request count (if HTTP service enabled)

### Database Performance

```bash
fly postgres connect -a my-indexer-db
```

```sql
-- Check table size
SELECT
  pg_size_pretty(pg_total_relation_size('contract_events')) AS total_size;

-- Check index usage
SELECT
  indexrelname,
  idx_scan,
  idx_tup_read
FROM pg_stat_user_indexes
WHERE schemaname = 'public';

-- Slow queries
SELECT
  query,
  mean_exec_time,
  calls
FROM pg_stat_statements
ORDER BY mean_exec_time DESC
LIMIT 10;
```

---

## Cost Estimate

### Free Tier

Fly.io free tier (no credit card required):
- **3 shared-cpu-1x VMs** (256MB RAM each)
- **3GB persistent storage**
- **160GB outbound bandwidth**

**Can you run on free tier?** Yes, if:
- Single indexer with minimal resources
- Use external Postgres (Neon free tier)
- Low-moderate event volume

### Production Costs

**Example setup:**

| Resource | Specification | Cost/Month |
|----------|--------------|------------|
| **Indexer VM** | shared-cpu-1x, 1GB RAM | ~$6 |
| **Fly Postgres** | Development (256MB RAM, 1GB disk) | ~$2 |
| **Persistent Volume** | 10GB | ~$1.50 |
| **Bandwidth** | ~50GB/month | Free (under 160GB) |
| **Total** | | **~$10/month** |

**Medium-volume setup:**

| Resource | Specification | Cost/Month |
|----------|--------------|------------|
| **Indexer VM** | shared-cpu-2x, 2GB RAM | ~$12 |
| **Fly Postgres** | Production (4GB RAM, 40GB disk) | ~$30 |
| **Persistent Volume** | 25GB | ~$3.75 |
| **Total** | | **~$46/month** |

**Compare to:**
- DigitalOcean VPS: $24/month + $25/month (managed DB) = $49/month
- Managed indexing services: $100-1000+/month

**Check current pricing:** [fly.io/docs/about/pricing](https://fly.io/docs/about/pricing)

---

## Troubleshooting

### Indexer Not Starting

**Check logs:**
```bash
fly logs
```

**Common issues:**

1. **Database connection failed**
   ```
   ERROR: Cannot connect to database
   ```
   **Fix:** Verify `DATABASE_URL` secret is set:
   ```bash
   fly secrets list
   fly postgres attach my-indexer-db
   ```

2. **RPC connection failed**
   ```
   WARNING: Cannot connect to Stellar RPC
   ```
   **Fix:** Check RPC_URL in fly.toml or try different endpoint:
   ```bash
   fly secrets set RPC_URL="https://archive-rpc.lightsail.network"
   ```

3. **Out of memory**
   ```
   OOMKilled
   ```
   **Fix:** Increase memory in fly.toml:
   ```toml
   [vm]
     memory_mb = 2048
   ```
   Then redeploy: `fly deploy`

### No Events in Database

**Check:**

1. **Indexer is running:**
   ```bash
   fly ssh console
   ps aux | grep contract-events
   ```

2. **Recent activity:**
   ```sql
   SELECT MAX(ledger_sequence), MAX(ledger_closed_at)
   FROM contract_events;
   ```

3. **Logs for errors:**
   ```bash
   fly logs | grep ERROR
   ```

**Common causes:**
- Starting from `latest` on a quiet contract (no recent events)
- RPC rate limiting (switch to dedicated RPC endpoint)
- Processor crashed (check logs for panic/segfault)

### Slow Performance

**Symptoms:** Events taking long time to appear in database.

**Diagnosis:**

```bash
# Check CPU/memory usage
fly dashboard metrics

# Check database performance
fly postgres connect -a my-indexer-db
```

```sql
SELECT * FROM pg_stat_activity WHERE state = 'active';
```

**Fixes:**

1. **Increase batch size** (fewer commits):
   ```bash
   fly secrets set BATCH_SIZE=5000
   ```

2. **Add database indexes** (see migration files)

3. **Scale up resources**:
   ```bash
   fly scale vm shared-cpu-2x --memory 2048
   ```

### Build Failures

**Error 1: Go version mismatch**
```
go: go.mod requires go >= 1.25.4 (running go 1.21.13; GOTOOLCHAIN=local)
```

**Fix:** The Dockerfile should use `FROM golang:alpine` (not a pinned version). If you see this error, update the Dockerfile:

```dockerfile
# Change this:
FROM golang:1.21-alpine AS builder

# To this:
FROM golang:alpine AS builder
```

**Error 2: Module does not contain package**
```
main module (github.com/withObsrvr/nebu) does not contain package github.com/withObsrvr/nebu/examples/processors/contract-events/cmd/contract-events
```

**Fix:** Each processor has its own `go.mod` and must be built from its own directory:

```dockerfile
# Correct:
WORKDIR /build/nebu/examples/processors/contract-events
RUN go build -o /processors/contract-events ./cmd/contract-events

# Not:
RUN go build -o /processors/contract-events ./examples/processors/contract-events/cmd/contract-events
```

**Error 3: No Go files in directory**
```
no Go files in /build/nebu/examples/processors/contract-events/cmd
```

**Fix:** The processor path needs an extra nested directory:

```dockerfile
# Correct:
./cmd/contract-events

# Not:
./cmd
```

**Error 4: Build process failed**
```
ERROR: failed to solve: process "/bin/sh -c go build..." did not complete successfully
```

**Fix:** Check Dockerfile syntax and ensure nebu repository is accessible:

```bash
# Test build locally
docker build -t nebu-indexer .
```

### Database Full

**Error:**
```
ERROR: disk quota exceeded
```

**Fix:** Extend volume size:

```bash
fly volumes list
fly volumes extend <volume-id> --size 25
```

---

## FAQ

### Q: Can I run multiple indexers on one Fly.io app?

**A:** Yes! Edit `scripts/start-indexer.sh` to run multiple pipelines:

```bash
contract-events --follow | postgres-sink --table contract_events &
token-transfer --follow | postgres-sink --table token_transfers &
wait
```

### Q: How do I upgrade to a newer version of nebu?

**A:** Rebuild and redeploy:

```bash
# Edit Dockerfile to use new version
ARG NEBU_VERSION=v1.2.0  # Change this

# Redeploy
fly deploy
```

### Q: Can I use this with Stellar testnet?

**A:** Yes! Change RPC_URL:

```bash
fly secrets set RPC_URL="https://soroban-testnet.stellar.org"
fly deploy
```

### Q: How do I backup the database?

**A:** Use Fly Postgres snapshots:

```bash
# Create snapshot
fly postgres backup create -a my-indexer-db

# List backups
fly postgres backup list -a my-indexer-db
```

Or pg_dump:

```bash
fly proxy 5432 -a my-indexer-db &
pg_dump -h localhost -U postgres nebu_indexer > backup.sql
```

### Q: Can I expose an HTTP API from my indexer?

**A:** Yes! Add HTTP service to fly.toml:

```toml
[[services]]
  internal_port = 8080
  protocol = "tcp"

  [[services.ports]]
    port = 80
    handlers = ["http"]

  [[services.ports]]
    port = 443
    handlers = ["http", "tls"]
```

Update start-indexer.sh to run HTTP server alongside indexer.

### Q: How do I handle schema changes?

**A:** Create new migration file:

```bash
# Create migration
echo "ALTER TABLE contract_events ADD COLUMN new_field TEXT;" \
  > migrations/003_add_new_field.sql

# Apply
fly ssh console
psql $DATABASE_URL -f migrations/003_add_new_field.sql
```

### Q: Can I use a different database than Postgres?

**A:** The `postgres-sink` processor requires PostgreSQL. For other databases, you'd need to:
1. Write a custom sink processor (e.g., `mysql-sink`, `mongodb-sink`)
2. Or use `json-file-sink` and process files separately

---

## Next Steps

- **Monitor your indexer**: Set up alerts with Fly.io monitoring
- **Optimize performance**: Tune batch sizes, add indexes
- **Build custom processors**: Extract specific data for your use case
- **Share with community**: Contribute processors to nebu registry

---

## Resources

- **nebu Documentation:** [nebu/docs](https://github.com/withObsrvr/nebu/tree/main/docs)
- **Fly.io Docs:** [fly.io/docs](https://fly.io/docs)
- **Stellar RPC:** [stellar.org/developers/data](https://stellar.org/developers/data/rpc)
- **Community:** [GitHub Discussions](https://github.com/withObsrvr/nebu/discussions)

---

## Support

- **Deployment issues**: Open issue in [nebu-processor-registry](https://github.com/withObsrvr/nebu-processor-registry/issues)
- **nebu processor questions**: [nebu GitHub](https://github.com/withObsrvr/nebu/issues)
- **Fly.io platform**: [Fly.io Community](https://community.fly.io)

---

**Last updated:** December 17, 2025
**Tested with:** nebu v0.1.0, flyctl v0.2.0, Fly Postgres 16
