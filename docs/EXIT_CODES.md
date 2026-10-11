# nebu exit codes

The `nebu` CLI uses stable exit codes so shell scripts, CI systems, agents,
and orchestrators can distinguish failures without parsing stderr.

| Code | Meaning | Examples |
|---:|---|---|
| 0 | Completed successfully | A bounded fetch emitted every requested ledger |
| 1 | Command ran and failed | Output write failure, processor build failure |
| 2 | Invalid usage or configuration | Bad flags, missing arguments, invalid mode |
| 3 | Requested item or range was not found or is ambiguous | Unknown processor, invalid ledger range |
| 4 | Data source is unavailable | RPC/archive connection, authentication, or rate-limit failure |
| 130 | Interrupted | Command canceled by SIGINT (`Ctrl+C`) |

Human-readable diagnostics continue to go to stderr. Data remains on stdout,
so checking an exit code never requires parsing or mixing with the data plane.

Processor executables have a smaller contract: zero means clean completion and
nonzero means failure; processors may use 2 for invalid usage. See
[PROCESSOR_CONTRACT.md](PROCESSOR_CONTRACT.md#1-process-model).
