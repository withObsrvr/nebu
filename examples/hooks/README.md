# Reference hooks

Four drop-in `runtime.Hooks` implementations you can copy into your own nebu processor's `cmd/<name>/` directory. Each file is self-contained: copy one file, rename `package hooks` to your package (usually `main`), add any required dependency to your `go.mod`, and you're done.

## What's here

| File | Callbacks | Deps beyond stdlib |
|---|---|---|
| [`rate_limit.go`](./rate_limit.go) | `BeforeLedger` | none |
| [`metrics.go`](./metrics.go) | `AfterLedger`, `OnWarning`, `OnFatal`, `OnEnd` | [`prometheus/client_golang`](https://github.com/prometheus/client_golang) |
| [`tracing.go`](./tracing.go) | `OnStart`, `BeforeLedger`, `AfterLedger`, `OnWarning`, `OnFatal`, `OnEnd` | [`go.opentelemetry.io/otel`](https://github.com/open-telemetry/opentelemetry-go) |
| [`token-transfer/progress.go`](https://github.com/withObsrvr/nebu-processor-registry/blob/main/processors/token-transfer/cmd/token-transfer/progress.go) | `OnStart`, `AfterLedger`, `OnEnd` | none |

(The progress-bar example is the original reference implementation and lives alongside token-transfer, not here.)

## Using them in your processor

All four hooks plug into `OriginConfig.Hooks`. Mix and match — the runtime composes multiple hook bundles independently:

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "go.opentelemetry.io/otel"

    "github.com/withObsrvr/nebu/examples/hooks"
    "github.com/withObsrvr/nebu/pkg/processor/cli"
    "github.com/withObsrvr/nebu/pkg/runtime"
)

reg := prometheus.NewRegistry()
tp  := otel.GetTracerProvider()

cfg := cli.OriginConfig{
    Processor: myProcessor,
    Hooks: []runtime.Hooks{
        hooks.RateLimitHook(50),                           // throttle to 50 ledgers/sec
        hooks.PrometheusHook(reg, myProcessor.Name()),     // /metrics endpoint
        hooks.OTelTracingHook(tp),                         // span per ledger
    },
}
```

## Smoke test

A minimal demo that wires all four hooks into a run against Stellar mainnet lives at [`cmd/hooks-demo/`](./cmd/hooks-demo). Build and run:

```bash
go build -o /tmp/hooks-demo ./cmd/hooks-demo
/tmp/hooks-demo
```

You'll see the progress bar on stderr, a `/metrics` endpoint on `:9090`, and OTel spans written to stdout (via a stdout exporter, for the demo — swap for OTLP in production).

## Why a separate module

This directory is its own Go module so the Prometheus and OTel dependencies don't get pulled into the main nebu binary or the reference processors. If you only want `rate_limit.go`, you add zero dependencies to your processor. If you want metrics, you add only Prometheus. Tracing pulls in only OTel.

## The copy-paste pattern

Each file opens with a docstring that says *"copy this file into your own processor's cmd/<name>/ directory"*. That's the whole upgrade path — no framework, no abstract factory, no init function. A hook is a struct of closures; these files just show you how to fill that struct.

## See also

- [`docs/HOOKS.md`](../../docs/HOOKS.md) — full reference for the `runtime.Hooks` API, firing order, abort semantics, thread safety.
- [`pkg/runtime/hooks.go`](../../pkg/runtime/hooks.go) — the canonical Go type definitions.
- [`pkg/runtime/hooks_test.go`](../../pkg/runtime/hooks_test.go) — tests that exercise every callback.
