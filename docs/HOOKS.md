# Runtime Hooks

**Status:** Available in `pkg/runtime` as of v0.6.0. Additive — existing code keeps working without any changes.

nebu's runtime fires **hooks** at specific points in a pipeline's lifecycle: pipeline start, before/after each ledger, on warnings, on fatals, and at pipeline end. Hooks are how you plug observability (metrics, tracing, logging, progress bars) or lightweight control flow (rate limiting, agent approval) into a pipeline without owning the runtime loop.

Hooks live in [`pkg/runtime/hooks.go`](../pkg/runtime/hooks.go). They are **not** part of the stable surface — `pkg/runtime` is explicitly marked unstable in [STABILITY.md](STABILITY.md) — so the API may evolve as usage patterns emerge. The spirit of the design is stable even if individual fields move.

## The six hook points

| Hook | When it fires | Can it influence control flow? |
|---|---|---|
| `OnStart` | Once, before the first ledger is fetched | No |
| `BeforeLedger` | Before each `origin.ProcessLedger` call | **Yes** — returning error aborts the pipeline |
| `AfterLedger` | After each `origin.ProcessLedger` call, with timing | No |
| `OnWarning` | When a processor calls `processor.ReportWarning` | No |
| `OnFatal` | When a processor calls `processor.ReportFatal` | No |
| `OnEnd` | Once, as the pipeline terminates (any reason) | No |

`OnEnd` fires exactly once for every `RunOrigin` invocation — clean completion, source error, fatal processor report, context cancellation, or `BeforeLedger` abort. It runs while ctx is still live, so hooks that need to make outbound network calls (metrics flush, span export, checkpoint save) can still do so.

## Registering hooks

```go
rt := runtime.NewRuntime()

rt.Use(runtime.Hooks{
    OnStart: func(ctx context.Context, info runtime.PipelineInfo) {
        log.Printf("pipeline starting: %s [%d..%d]",
            info.ProcessorName, info.StartLedger, info.EndLedger)
    },
    OnEnd: func(ctx context.Context, s runtime.PipelineSummary) {
        log.Printf("pipeline done: %d ledgers, %d warnings, %v",
            s.LedgersProcessed, s.Warnings, s.Duration)
    },
})

rt.RunOrigin(ctx, src, origin, 60200000, 60200100)
```

Multiple `Use` calls stack — each registered bundle fires independently, in registration order. Independent concerns compose without manual merging:

```go
rt.Use(metricsHooks())   // Prometheus counters and histograms
rt.Use(tracingHooks())   // OTel spans around each ledger
rt.Use(progressBarHooks()) // Terminal progress bar
rt.Use(checkpointHooks(db)) // Persist last-processed ledger
```

## Use cases

### 1. Prometheus metrics

```go
var (
    ledgersTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "nebu_ledgers_total"},
        []string{"processor"})
    ledgerDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{Name: "nebu_ledger_duration_seconds"},
        []string{"processor"})
    warningsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "nebu_warnings_total"},
        []string{"processor"})
)

func metricsHooks(proc string) runtime.Hooks {
    return runtime.Hooks{
        AfterLedger: func(ctx context.Context, _ xdr.LedgerCloseMeta, s runtime.LedgerStats) {
            ledgersTotal.WithLabelValues(proc).Inc()
            ledgerDuration.WithLabelValues(proc).Observe(s.Duration.Seconds())
        },
        OnWarning: func(ctx context.Context, r processor.ErrorReport) {
            warningsTotal.WithLabelValues(r.Processor).Inc()
        },
    }
}
```

### 2. OpenTelemetry tracing

```go
func tracingHooks(tracer trace.Tracer) runtime.Hooks {
    var currentSpan trace.Span
    return runtime.Hooks{
        OnStart: func(ctx context.Context, info runtime.PipelineInfo) {
            _, span := tracer.Start(ctx, "pipeline",
                trace.WithAttributes(
                    attribute.String("processor", info.ProcessorName),
                    attribute.Int64("start", int64(info.StartLedger)),
                    attribute.Int64("end", int64(info.EndLedger)),
                ))
            currentSpan = span
        },
        AfterLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta, s runtime.LedgerStats) {
            currentSpan.AddEvent("ledger",
                trace.WithAttributes(
                    attribute.Int64("seq", int64(ledger.LedgerSequence())),
                    attribute.Int64("duration_us", s.Duration.Microseconds()),
                ))
        },
        OnEnd: func(ctx context.Context, s runtime.PipelineSummary) {
            if s.FatalErr != nil {
                currentSpan.RecordError(s.FatalErr)
            }
            currentSpan.SetAttributes(
                attribute.Int64("ledgers_processed", int64(s.LedgersProcessed)),
                attribute.Int64("warnings", s.Warnings),
            )
            currentSpan.End()
        },
    }
}
```

### 3. Progress bar for long backfills

```go
func progressBarHooks(start, end uint32) runtime.Hooks {
    bar := progressbar.NewOptions64(int64(end-start+1),
        progressbar.OptionSetDescription("processing ledgers"),
        progressbar.OptionShowCount(),
    )
    return runtime.Hooks{
        AfterLedger: func(ctx context.Context, _ xdr.LedgerCloseMeta, _ runtime.LedgerStats) {
            _ = bar.Add(1)
        },
        OnEnd: func(ctx context.Context, _ runtime.PipelineSummary) {
            _ = bar.Finish()
        },
    }
}
```

### 4. Checkpointing for resume

Persist the last-processed ledger sequence every N ledgers, so a killed pipeline can resume without reprocessing. `AfterLedger` is the right hook because it runs only after the processor has actually emitted events — resuming from the last checkpoint will re-emit at most N ledgers.

```go
func checkpointHooks(db *sql.DB, interval uint32) runtime.Hooks {
    var lastSaved uint32
    return runtime.Hooks{
        AfterLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta, _ runtime.LedgerStats) {
            seq := ledger.LedgerSequence()
            if seq-lastSaved < interval {
                return
            }
            if _, err := db.ExecContext(ctx,
                `UPDATE checkpoint SET last_ledger = $1 WHERE processor = $2`,
                seq, "token-transfer"); err != nil {
                log.Printf("checkpoint save failed: %v", err)
                return
            }
            lastSaved = seq
        },
        OnEnd: func(ctx context.Context, s runtime.PipelineSummary) {
            // Flush final checkpoint on clean termination.
            if s.FatalErr == nil && s.LedgersProcessed > 0 {
                _, _ = db.ExecContext(ctx,
                    `UPDATE checkpoint SET last_ledger = $1 WHERE processor = $2`,
                    lastSaved+uint32(s.LedgersProcessed), "token-transfer")
            }
        },
    }
}
```

### 5. Rate limiting / backpressure

`BeforeLedger` can sleep or block, pausing the pipeline until a downstream backlog drains:

```go
func rateLimitHooks(backlog func() int) runtime.Hooks {
    return runtime.Hooks{
        BeforeLedger: func(ctx context.Context, _ xdr.LedgerCloseMeta) error {
            for backlog() > 10_000 {
                select {
                case <-ctx.Done():
                    return ctx.Err()
                case <-time.After(100 * time.Millisecond):
                }
            }
            return nil
        },
    }
}
```

### 6. Agent-driven intervention

`BeforeLedger` can also gate each ledger on an external decision — useful when an AI agent or human operator wants to approve, skip, or pause a pipeline mid-run:

```go
func agentHooks(controller AgentController) runtime.Hooks {
    return runtime.Hooks{
        BeforeLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
            decision, err := controller.Decide(ctx, ledger.LedgerSequence())
            if err != nil {
                return fmt.Errorf("agent decision failed: %w", err)
            }
            switch decision {
            case DecisionApprove:
                return nil
            case DecisionAbort:
                return errors.New("agent aborted pipeline")
            case DecisionWait:
                <-controller.Wait(ctx) // blocks until agent signals
                return nil
            }
            return nil
        },
    }
}
```

This is the pi-mono-style "agent-as-user" pattern: the runtime exposes an observation-and-control point, and the agent wires into it externally. The processor doesn't know or care that it's being gated by an agent.

## Design notes

1. **`Hooks` is a struct of optional functions, not an interface.** An interface would force users to implement every method or embed a no-op base struct. A struct-of-functions lets you set only what you care about and leave the rest nil. This is the same pattern `http.Server` uses (`http.Server.ConnState`, `http.Server.BaseContext`, etc.).

2. **Only `BeforeLedger` can abort.** Observational hooks (`AfterLedger`, `OnWarning`, `OnFatal`, `OnEnd`) cannot influence control flow. `OnFatal` reports on a decision the runtime has already made. `OnStart` is too early to abort cleanly. Giving `BeforeLedger` an error return is the one place where a hook can say "stop the pipeline" — it's rare but real (rate limiting, agent approval, quota enforcement).

3. **Warnings and fatals don't abort via hook.** The reporter decides fatal-means-halt. Hooks just observe. If a hook wants to *upgrade* a warning into a fatal, it can call `processor.ReportFatal(ctx, ...)` directly — the ctx it receives is the runtime's, so the call wires back through the same reporter.

4. **`context.Context` is threaded through every hook.** Standard idiom for cancellation, deadlines, and carrying structured values (trace spans, request IDs, whatever).

5. **Thread safety.** Hooks may be called from multiple goroutines in a future version of the runtime; implementations should be safe for concurrent use. In the current implementation, all hooks fire from the same goroutine as `RunOrigin`, but treat that as a subject-to-change detail.

6. **Registration must precede `RunOrigin`.** Registering hooks concurrently with a running pipeline is a data race. Call `Use` during setup, then call `RunOrigin`.

## Observed failure modes

- **A hook panics.** Currently the runtime does not recover from hook panics — they propagate up and crash the process. If you want panic isolation, wrap your hook function body in a `defer recover()`.
- **A hook blocks forever.** The runtime waits synchronously for each hook to return. A deadlocked hook hangs the pipeline. Keep hook work fast; offload long operations to a buffered channel consumed by a separate goroutine.
- **Multiple `Use` bundles in different goroutines.** Registering hooks from two goroutines simultaneously is a data race. Register during single-threaded setup code.

## What hooks are NOT

- **A replacement for processor logic.** Hooks observe and lightly gate; they don't process events. Events flow through processors, not hooks.
- **A stable external API.** `pkg/runtime` is unstable per [STABILITY.md](STABILITY.md). Hooks may change before v1.0.
- **A middleware chain.** Hooks are not chained handlers — there's no `next()` to call. They're independent observation points.

## See also

- [`pkg/runtime/hooks.go`](../pkg/runtime/hooks.go) — the `Hooks` struct and supporting types
- [`pkg/runtime/runner.go`](../pkg/runtime/runner.go) — where hooks are invoked
- [STABILITY.md](STABILITY.md) — why `pkg/runtime` is unstable
- [ARCHITECTURE_DECISIONS.md](ARCHITECTURE_DECISIONS.md) — why nebu is CLI-only and composes via Unix pipes
