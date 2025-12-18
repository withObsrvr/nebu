# Fly.io Deployment Commands Cheat Sheet

Quick reference for common deployment operations.

## Initial Setup

```bash
# Install flyctl
curl -L https://fly.io/install.sh | sh

# Login to Fly.io
fly auth login

# Create new app (from this directory)
fly launch --no-deploy

# Create Postgres cluster
fly postgres create --name my-indexer-db

# Attach database to app
fly postgres attach my-indexer-db

# Create persistent volume
fly volumes create indexer_data --size 10
```

## Deployment

```bash
# Deploy to Fly.io
fly deploy

# Deploy with specific Dockerfile
fly deploy --dockerfile Dockerfile

# Deploy and watch logs
fly deploy && fly logs -f
```

## Configuration

```bash
# Set secret
fly secrets set KEY=value

# Set multiple secrets
fly secrets set KEY1=value1 KEY2=value2

# List all secrets
fly secrets list

# Remove secret
fly secrets unset KEY

# Set RPC URL
fly secrets set RPC_URL="https://archive-rpc.lightsail.network"

# Set starting ledger
fly secrets set START_LEDGER="60000000"
```

## Monitoring

```bash
# View real-time logs
fly logs -f

# View last 100 lines
fly logs --lines 100

# Search logs
fly logs | grep ERROR

# Check app status
fly status

# View metrics dashboard
fly dashboard metrics

# SSH into machine
fly ssh console
```

## Database Operations

```bash
# Connect to Postgres
fly postgres connect -a my-indexer-db

# Proxy database to localhost (background)
fly proxy 5432 -a my-indexer-db &

# Connect via local psql
psql postgresql://postgres:<password>@localhost:5432/postgres

# Create database backup
fly postgres backup create -a my-indexer-db

# List backups
fly postgres backup list -a my-indexer-db

# Restore from backup
fly postgres backup restore <backup-id> -a my-indexer-db
```

## Scaling

```bash
# Scale to multiple machines
fly scale count 2

# Scale to specific regions
fly scale count 2 --region sjc,ams

# Change VM size
fly scale vm shared-cpu-2x

# Change memory
fly scale memory 2048

# Scale to zero (pause)
fly scale count 0

# Resume from zero
fly scale count 1
```

## Volumes

```bash
# List volumes
fly volumes list

# Create new volume
fly volumes create indexer_data --size 10

# Extend volume size
fly volumes extend <volume-id> --size 25

# Delete volume
fly volumes delete <volume-id>

# Create snapshot
fly volumes snapshots create <volume-id>

# List snapshots
fly volumes snapshots list
```

## Troubleshooting

```bash
# View app configuration
fly config show

# Validate fly.toml
fly config validate

# Check app health
fly checks list

# View VM metrics
fly vm status

# Restart app
fly apps restart

# List all apps
fly apps list

# Destroy app (DANGEROUS)
fly apps destroy my-app-name
```

## Local Testing

```bash
# Build Docker image locally
docker build -t nebu-indexer .

# Run locally
docker run --env-file .env nebu-indexer

# Test with docker-compose
docker-compose up

# Run in background
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down

# Rebuild and start
docker-compose up --build
```

## Common Workflows

### Deploy New Version

```bash
# 1. Update code/config
# 2. Deploy
fly deploy

# 3. Monitor
fly logs -f

# 4. Verify
fly status
```

### Run Backfill

```bash
# SSH into machine
fly ssh console

# Run backfill script
/scripts/backfill.sh 60000000

# Monitor progress
tail -f /data/backfill.log

# Exit
exit
```

### Database Migration

```bash
# Connect to database
fly postgres connect -a my-indexer-db

# Apply migration
\i /path/to/migration.sql

# Verify
\dt  # List tables
\d contract_events  # Describe table

# Exit
\q
```

### Change RPC Endpoint

```bash
# Set new RPC URL
fly secrets set RPC_URL="https://new-rpc-endpoint.com"

# Restart to apply
fly apps restart

# Verify in logs
fly logs | grep "RPC URL"
```

### Increase Resources

```bash
# Scale VM
fly scale vm shared-cpu-2x --memory 2048

# Verify
fly status
```

### View Recent Events

```bash
# Connect to database
fly postgres connect -a my-indexer-db

# Query
SELECT * FROM contract_events ORDER BY ledger_sequence DESC LIMIT 10;
```

## Emergency Commands

### App Won't Start

```bash
# Check logs for errors
fly logs --lines 200

# SSH to debug
fly ssh console
ps aux
env | grep DATABASE

# Force restart
fly apps restart
```

### Out of Disk Space

```bash
# Check volume usage
fly ssh console
df -h

# Extend volume
fly volumes list
fly volumes extend <volume-id> --size 25
```

### Rollback Deployment

```bash
# List releases
fly releases

# Rollback to previous
fly releases rollback

# Or specific version
fly releases rollback v2
```

## Useful psql Commands

```sql
-- Count events
SELECT COUNT(*) FROM contract_events;

-- Recent events
SELECT * FROM contract_events ORDER BY ledger_sequence DESC LIMIT 10;

-- Events by contract
SELECT contract_id, COUNT(*)
FROM contract_events
GROUP BY contract_id
ORDER BY COUNT(*) DESC;

-- Database size
SELECT pg_size_pretty(pg_database_size(current_database()));

-- Table size
SELECT pg_size_pretty(pg_total_relation_size('contract_events'));

-- Active queries
SELECT * FROM pg_stat_activity WHERE state = 'active';

-- Index usage
SELECT * FROM pg_stat_user_indexes WHERE schemaname = 'public';
```

---

**Tip:** Add these to your shell aliases:

```bash
alias fly-logs='fly logs -f'
alias fly-ssh='fly ssh console'
alias fly-db='fly postgres connect -a my-indexer-db'
alias fly-deploy='fly deploy && fly logs -f'
```
