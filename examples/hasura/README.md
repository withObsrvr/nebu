# Hasura GraphQL for nebu

Complete Hasura setup for querying nebu postgres-sink data via GraphQL.

## Quick Start

1. **Configure Environment**:
   ```bash
   cd examples/hasura

   # Copy the example env file and edit with your credentials
   cp .env.example .env
   # Edit .env with your PostgreSQL credentials
   ```

   The `.env` file is gitignored and won't be committed.

2. **Start Hasura**:
   ```bash
   docker compose up -d
   ```

3. **Open the Console**:
   Open http://localhost:8091/console in your browser

4. **Track Your Views**:
   In the Hasura console:
   - Go to "Data" tab
   - Click "public" schema
   - You'll see all the views created (token_transfers, carbon_offsets, etc.)
   - Click "Track" next to each view you want to query via GraphQL
   - Or click "Track All" to enable them all at once

4. **Start Querying**:
   - Go to the "API" tab (GraphiQL interface)
   - Copy queries from `example-queries.graphql`
   - Run them and see your data!

## What's Included

### SQL Views (`hasura-views.sql`)
All views are already created in PostgreSQL. These transform your JSONB data into clean, queryable tables:

**Token Transfers**:
- `token_transfer_events` - All transfer events (transfers, mints, burns, fees)
- `token_transfers` - Transfer events only
- `usdc_transfers` - USDC transfers only
- `large_transfers` - Transfers over 100 XLM

**Contract Events**:
- `contract_events_decoded` - All contract events with decoded topics/data
- `contract_transfers` - Contract transfer events
- `contract_swaps` - Contract swap events
- `contract_deposits` - Contract deposit events

**Contract Invocations**:
- `contract_invocations_summary` - All contract calls with metadata
- `failed_invocations` - Failed contract calls only
- `successful_invocations` - Successful calls only

**Carbon Offsets**:
- `carbon_offsets` - Validated carbon offset events
- `carbon_project_stats` - Aggregated stats by project
- `carbon_funder_stats` - Aggregated stats by funder

**Analytics**:
- `hourly_activity` - Hourly transfer statistics
- `daily_usdc_volume` - Daily USDC volume and stats
- `contract_activity` - Contract activity leaderboard
- `top_movers` - Top tokens by 24h volume
- `recent_activity` - Last 1000 events feed
- `daily_stats` - Materialized view for daily aggregations

### Example Queries (`example-queries.graphql`)
Comprehensive GraphQL query examples covering:
- Carbon offsets (recent, by funder, project stats, leaderboard)
- Token transfers (recent, USDC, by address, large transfers)
- Contract events (recent, by contract, by type)
- Contract invocations (recent, by contract, failed, by function)
- Analytics and aggregations
- Real-time subscriptions

## Example Queries

### Get Recent USDC Transfers
```graphql
query GetRecentUSDC {
  usdc_transfers(
    order_by: {timestamp: desc}
    limit: 10
  ) {
    from_address
    to_address
    amount
    tx_hash
    timestamp
  }
}
```

### Get Carbon Offset Project Stats
```graphql
query GetProjectStats {
  carbon_project_stats(
    order_by: {total_amount: desc}
  ) {
    project_id
    offset_count
    total_amount
    unique_funders
  }
}
```

### Subscribe to New Transfers (Real-time)
```graphql
subscription NewTransfers {
  token_transfers(
    order_by: {timestamp: desc}
    limit: 5
  ) {
    from_address
    to_address
    amount
    asset_code
    timestamp
  }
}
```

## Architecture

```
nebu processors → postgres-sink → PostgreSQL (JSONB) → SQL Views → Hasura → GraphQL API
```

- **postgres-sink** stores raw JSONB events in tables
- **SQL views** decode JSONB into clean, typed columns
- **Hasura** auto-generates GraphQL API from views
- **You** query everything via GraphQL!

## Configuration

### Docker Compose (`docker-compose.yml`)
- **Port**: 8091 (http://localhost:8091)
- **Network**: host mode (for PostgreSQL access)
- **Database**: Configured via `.env` file
- **Console**: Enabled
- **Dev Mode**: Enabled (no admin secret required)

For production:
1. Uncomment `HASURA_GRAPHQL_ADMIN_SECRET` in docker-compose.yml
2. Set an admin secret: `export HASURA_ADMIN_SECRET="your-secret-here"`
3. Uncomment `HASURA_GRAPHQL_UNAUTHORIZED_ROLE: anonymous` for public queries
4. Disable dev mode: `HASURA_GRAPHQL_DEV_MODE: "false"`

### Permissions

By default (dev mode), all queries are allowed. For production:

1. Create roles in Hasura console (Data → Permissions)
2. Set row-level permissions (e.g., users can only see their own data)
3. Set column-level permissions (e.g., hide sensitive fields)

Example permission: "anonymous users can only see successful transfers from last 24 hours"

## Feeding Data to Hasura

### From Processor Output
```bash
# Token transfers
~/go/bin/token-transfer --start-ledger 60200000 --end-ledger 60200001 | \
  ~/go/bin/postgres-sink --table test_events

# Contract events
~/go/bin/contract-events --start-ledger 60200000 --end-ledger 60200001 | \
  ~/go/bin/postgres-sink --table contract_events_v2

# Contract invocations
~/go/bin/contract-invocation --start-ledger 60200000 --end-ledger 60200001 | \
  ~/go/bin/postgres-sink --table full_invocations
```

### From Custom jq Pipelines
```bash
# Custom carbon sink events
~/go/bin/contract-invocation --start-ledger 60200000 --end-ledger 60200001 | \
  jq -c 'select(.functionName == "sink_carbon") | {
    event_type: .functionName,
    funder: .arguments[0].addressValue,
    recipient: .arguments[1].addressValue,
    amount: .arguments[2].u128Value,
    # ... your custom fields ...
  }' | \
  ~/go/bin/postgres-sink --table carbon_sink_fixed
```

## Advanced Features

### Materialized Views
For expensive aggregations, use materialized views:

```sql
-- Refresh materialized view (run periodically)
REFRESH MATERIALIZED VIEW CONCURRENTLY daily_stats;
```

Set up a cron job:
```bash
# Refresh every hour
0 * * * * psql -U tillman -d postgres -c "REFRESH MATERIALIZED VIEW CONCURRENTLY daily_stats;"
```

### Real-time Subscriptions
Hasura supports WebSocket subscriptions for live data:

```graphql
subscription LiveUSDCTransfers {
  usdc_transfers(
    order_by: {timestamp: desc}
    limit: 5
  ) {
    from_address
    to_address
    amount
    timestamp
  }
}
```

Connect from JavaScript:
```javascript
import { createClient } from 'graphql-ws';

const client = createClient({
  url: 'ws://localhost:8091/v1/graphql',
});

client.subscribe(
  {
    query: '{ usdc_transfers { from_address amount } }',
  },
  {
    next: (data) => console.log('New transfer:', data),
    error: (error) => console.error(error),
    complete: () => console.log('Done'),
  }
);
```

### GraphQL to REST

Hasura can expose GraphQL queries as REST endpoints:

1. Go to "API" → "REST" tab
2. Create a REST endpoint from any query
3. Access at: http://localhost:8091/api/rest/transfers

Example:
```bash
curl http://localhost:8091/api/rest/usdc-transfers | jq
```

## Troubleshooting

### Hasura can't connect to PostgreSQL
Check that PostgreSQL is listening on localhost:
```bash
ss -tln | grep :5432
```

Should show: `127.0.0.1:5432`

### Views are empty
Make sure you've populated the base tables:
```bash
# Check if data exists
psql -U postgres -d postgres -c "SELECT COUNT(*) FROM test_events;"
```

If empty, run processors to populate data.

### Port 8091 already in use
Change the port in docker-compose.yml:
```yaml
environment:
  HASURA_GRAPHQL_SERVER_PORT: "9090"  # Use different port
```

## Next Steps

1. **Track all views** in Hasura console
2. **Try example queries** from `example-queries.graphql`
3. **Set up subscriptions** for real-time data
4. **Build a dashboard** using React + GraphQL
5. **Add permissions** for production use

## Resources

- [Hasura Docs](https://hasura.io/docs/latest/graphql/core/index.html)
- [GraphQL Introduction](https://graphql.org/learn/)
- [nebu Documentation](../../README.md)
- [postgres-sink Documentation](../processors/postgres-sink/README.md)

---

**Generated with Claude Code**
https://claude.com/claude-code
