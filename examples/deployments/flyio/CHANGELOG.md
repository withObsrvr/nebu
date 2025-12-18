# Fly.io Deployment Example - Changelog

## 2025-12-18 - Pipeline Filters & Migration Fixes

### Added
- **Configurable pipeline filters** via `PIPELINE_FILTERS` environment variable
  - `contract-filter`: Filter events by specific contract IDs
  - `dedup`: Remove duplicate events
  - `amount-filter`: Filter by amount threshold
  - `jq:<expression>`: Custom JSON filtering
- **Filter chaining**: Combine multiple filters using `|` separator
- New scripts:
  - `scripts/contract-filter.sh` - Contract ID filtering
  - `scripts/jq-filter.sh` - Custom jq filtering

### Fixed
- **Migration idempotency**: Migrations now properly drop and recreate tables
  - Prevents schema mismatch errors when redeploying
  - Uses `DROP TABLE IF EXISTS ... CASCADE` before creating tables
- **Built additional processors**: Now includes dedup and amount-filter in Docker image
- **Documentation updates**: Added comprehensive filter configuration examples

### Configuration Examples

Filter specific contracts:
```bash
fly secrets set PIPELINE_FILTERS="contract-filter"
fly secrets set CONTRACT_IDS="CAG5LRY...JDDH,CBVMR...ABC"
```

Chain multiple filters:
```bash
fly secrets set PIPELINE_FILTERS="contract-filter|dedup|amount-filter"
fly secrets set CONTRACT_IDS="CAG5LRY...JDDH"
fly secrets set MIN_AMOUNT="1000000"
```

Custom jq filter:
```bash
fly secrets set PIPELINE_FILTERS="jq:.data.amount>5000000"
```

### Migration Notes

The updated migrations will:
1. Drop existing tables (losing any data)
2. Recreate tables with correct schema
3. This is safe for initial deployments

For production with existing data, manually backup before deploying:
```bash
fly postgres backup create -a my-indexer-db
```
