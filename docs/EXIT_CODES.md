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
| 8 | Ran, but the output is incomplete | A ledger could not be processed completely and `--on-ledger-error strict` is in effect |
| 130 | Interrupted | Command canceled by SIGINT (`Ctrl+C`) |

Code `8` is deliberately distinct from `4`. Both mean "you did not get the
whole dataset", but `4` is a transport problem worth retrying, while `8` means
the range was reachable and some of it could not be processed — retrying the
same range fails the same way. A consumer that cannot tell them apart will
either retry forever or silently accept a dataset with holes.

Human-readable diagnostics continue to go to stderr. Data remains on stdout,
so checking an exit code never requires parsing or mixing with the data plane.

Processor executables have a smaller contract: zero means clean completion and
nonzero means failure; processors may use 2 for invalid usage. See
[PROCESSOR_CONTRACT.md](PROCESSOR_CONTRACT.md#1-process-model).
