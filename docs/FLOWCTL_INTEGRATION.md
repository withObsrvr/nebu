# nebu ↔ flowctl process contract

Nebu remains a CLI and processor runtime. Flowctl owns discovery, process or
container supervision, restart policy, health timeouts, and pipeline wiring.
Nebu does not register with a control plane or require flowctl libraries.

This document defines the nebu side of the integration boundary. A separate
flowctl implementation can consume it without adding orchestration behavior to
nebu.

## Discovery

Before starting a processor, invoke:

```bash
token-transfer --describe-json
```

The command prints one `DescribeEnvelope` JSON document to stdout and exits 0
without requiring runtime flags or starting processing. Flowctl should use the
manifest's processor type, flags, and input/output JSON Schemas to validate
wiring. Parsers must ignore unknown fields for forward compatibility.

## Launch contract

Create a dedicated status path for each component, then launch the processor:

```bash
NEBU_STATUS_FILE=/run/flowctl/token-transfer.status.ndjson \
NEBU_PROGRAM_STATUS=never \
token-transfer --start-ledger 60200000 --end-ledger 60200100
```

The process exposes four independent signals:

| Signal | Contract |
|---|---|
| stdout | Processor NDJSON data only. |
| stderr | Human diagnostics and optional OSC 7501. |
| status file | Versioned `nebu.status.v1` NDJSON lifecycle and progress. |
| process exit | Authoritative success, failure, or interruption result. |

Flowctl should associate the status path with the child process it created;
nebu does not require flowctl-specific run or component identifiers.

## Status and health

The status schema is defined in [STATUS_STREAM.md](STATUS_STREAM.md). Origins
report current ledger and bounded-range lag. Transforms and sinks report the
number of successfully consumed events.

Terminal states are:

- `done`: successful completion;
- `idle`: cancellation or interruption;
- `error`: failed completion.

Status describes semantic state and progress. Flowctl should determine process
liveness and heartbeat timeouts from the child process it supervises rather
than requiring nebu to run a control-plane heartbeat client. Exit codes remain
authoritative if a process terminates before its final status record is read.

## OSC 7501

OSC 7501 is complementary. Set `NEBU_PROGRAM_STATUS=always` when flowctl wants
to consume terminal-native lifecycle reports from stderr, or `never` when the
dedicated NDJSON status stream is sufficient. Structured status does not
disable or replace OSC 7501.

## Cancellation

Flowctl should send SIGINT or SIGTERM, continue draining stdout and the status
file, and wait for graceful termination. Nebu processor helpers emit `idle` and
use exit code 130 for forced interruption. Flowctl may escalate termination
after its own configured grace period.

## Compatibility

- Treat `--describe-json` and `nebu.status.v1` as versioned contracts.
- Ignore unknown JSON fields.
- Never parse human stderr text for state or progress.
- Never mix the status file with stdout, stderr, checkpoint, or data paths.
- Do not assume a status record is a durable data acknowledgement.

Nebu works normally when neither status nor flowctl is present. The integration
adds no daemon, scheduler, pipeline registry, or required external service.
