# Deploy nebu to Fly.io (Simplified)

**Deploy a nebu indexer in 5 minutes using pre-built Docker images.**

This simplified deployment uses the official `withobsrvr/nebu` Docker image with all processors pre-built. No compilation needed - just configure and deploy.

---

## What This Deploys

- **Contract events indexer** streaming from Stellar blockchain
- **PostgreSQL sink** writing events to your database
- **Auto-restart** on failures (handled by Fly.io)
- **Persistent storage** via Fly.io volumes

**Architecture:**
```
Stellar RPC → contract-events | postgres-sink → PostgreSQL
```

---

## Prerequisites

1. **Fly.io account** - Sign up at [fly.io/signup](https://fly.io/signup)
2. **flyctl CLI** - Install with:
   ```bash
   curl -L https://fly.io/install.sh | sh
   fly auth login
   ```

---

## Quick Start

### 1. Clone This Directory

```bash
cd examples/deployments/flyio-simple
```

### 2. Create Fly.io App

```bash
fly launch --no-deploy
```

This creates your app configuration. You can customize the app name and region.

### 3. Create PostgreSQL Database

```bash
# Option A: Fly Postgres (recommended)
fly postgres create --name nebu-db

# Option B: Use your own Postgres
# Just set the DATABASE_URL secret in the next step
```

### 4. Set Secrets

```bash
# Set database connection
fly secrets set DATABASE_URL="postgres://user:pass@host:5432/dbname"

# Optional: Set RPC URL (default uses archive-rpc.lightsail.network)
fly secrets set RPC_URL="https://rpc-pubnet.nodeswithobsrvr.co"

# Optional: Set RPC authentication header
fly secrets set RPC_AUTH="Api-Key YOUR_KEY_HERE"
```

### 5. Deploy

```bash
fly deploy
```

That's it! Your indexer is now running.

---

## Viewing Logs

```bash
# Stream logs
fly logs

# View recent logs
fly logs --recent
```

---

## Querying Data

Connect to your PostgreSQL database and query the `contract_events` table:

```bash
# Connect to Fly Postgres
fly postgres connect -a nebu-db

# Example queries
SELECT COUNT(*) FROM contract_events;
SELECT event_type, COUNT(*) FROM contract_events GROUP BY event_type;
SELECT * FROM contract_events WHERE contract_id = 'YOUR_CONTRACT_ID' LIMIT 10;
```

---

## Customization

### Change Start Ledger

To index from a specific historical ledger:

```bash
fly secrets set START_LEDGER="60200000"
fly deploy
```

### Change Indexed Processor

Edit `start.sh` to use a different processor:

```bash
# Instead of contract-events, use token-transfer
exec token-transfer \
  --start-ledger "$START_LEDGER" \
  --rpc-url "$RPC_URL" \
  --follow \
  | postgres-sink \
      --dsn "$DATABASE_URL" \
      --table token_transfers \
      --batch-size "$BATCH_SIZE"
```

Then update `001_schema.sql` with the appropriate table schema and redeploy.

### Add Multiple Sinks

Pipe to multiple destinations using `tee`:

```bash
exec contract-events \
  --start-ledger "$START_LEDGER" \
  --rpc-url "$RPC_URL" \
  --follow \
  | tee >(postgres-sink --dsn "$DATABASE_URL" --table contract_events) \
  | json-file-sink --out /data/events.jsonl
```

---

## How This Works

1. **Dockerfile** pulls the official `withobsrvr/nebu:latest` image from Docker Hub
2. All processors (contract-events, token-transfer, postgres-sink, etc.) are pre-built in the image
3. **start.sh** runs migrations and starts the indexer pipeline
4. Fly.io keeps the process running and restarts it on failure
5. Data persists in your PostgreSQL database

**No compilation, no build time - just configure and run.**

---

## Comparison: Before vs After

**Before (build from source):**
- 98-line Dockerfile with multi-stage Go compilation
- 5+ minutes to build processors from source
- Complex build orchestration with separate go.mod files

**After (pre-built image):**
- 10-line Dockerfile pulling published image
- 30 seconds to pull image
- Zero build complexity

---

## When to Use the Full Deployment

This simplified deployment is perfect for:
- ✅ Development and data exploration
- ✅ Simple indexing pipelines
- ✅ Getting started quickly

For production workloads with advanced needs, see the [full deployment example](../flyio/) which includes:
- Backfill orchestration for historical data
- Dynamic filter chains
- Health monitoring
- Metadata tables and helper views

---

## Troubleshooting

### Deployment fails to pull image

The `withobsrvr/nebu` image is published on Docker Hub when a new release is tagged. If the image doesn't exist yet, you can build it locally using goreleaser:

```bash
cd /path/to/nebu
goreleaser release --snapshot --clean
docker tag withobsrvr/nebu:latest-amd64 withobsrvr/nebu:latest
docker push withobsrvr/nebu:latest
```

### Database connection errors

Check your `DATABASE_URL` secret:
```bash
fly secrets list
```

Test the connection:
```bash
fly ssh console
psql "$DATABASE_URL" -c "SELECT 1"
```

### RPC connection issues

If you see RPC errors, check your `RPC_URL` and ensure it's accessible. Some RPC endpoints require authentication headers.

### Process keeps restarting

Check logs for errors:
```bash
fly logs
```

Common issues:
- Missing `DATABASE_URL` or `RPC_URL` secrets
- Invalid database credentials
- RPC endpoint not accessible

---

## Cost Estimate

Typical monthly cost on Fly.io:
- **VM**: $5-10/month (1 CPU, 1GB RAM, shared)
- **Postgres**: $0-15/month (depends on size)
- **Bandwidth**: Usually covered by free tier

Total: **~$10-25/month** for a basic indexer.

---

## Next Steps

- Customize the pipeline in `start.sh`
- Add your own processors
- Scale up VM resources if needed: `fly scale vm ...`
- Explore other nebu processors: `docker run withobsrvr/nebu:latest ls /usr/local/bin`

For questions or issues, see the [main nebu docs](https://github.com/withObsrvr/nebu).
