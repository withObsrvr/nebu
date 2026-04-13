package hooks

import (
	"context"
	"fmt"
	"sync"

	"github.com/stellar/go-stellar-sdk/xdr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
)

// OTelTracingHook returns a runtime.Hooks bundle that emits an
// OpenTelemetry span for each ledger and a parent span for the whole
// pipeline.
//
// # Span structure
//
//	pipeline span: "nebu.pipeline"
//	  attrs: processor.name, nebu.start_ledger, nebu.end_ledger
//	  ├── ledger span: "nebu.ledger"
//	  │     attrs: nebu.ledger_sequence, nebu.ledger_duration_ms
//	  ├── ledger span: "nebu.ledger"
//	  │     ...
//	  └── status: Ok on clean completion, Error with FatalErr otherwise
//
// Warnings are recorded as events on the current ledger span (when
// possible) or on the pipeline span as a fallback. Fatals set
// status=Error on the pipeline span.
//
// # Arguments
//
//   - tp: an OpenTelemetry trace.TracerProvider. Use the global
//     otel.GetTracerProvider() if you've already configured one, or
//     pass a scoped *trace.TracerProvider in tests.
//
// # Example wiring
//
//	import (
//	    "go.opentelemetry.io/otel"
//	    sdktrace "go.opentelemetry.io/otel/sdk/trace"
//	)
//
//	// Configure a real exporter elsewhere (stdout, OTLP, Jaeger, ...).
//	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
//	defer tp.Shutdown(context.Background())
//	otel.SetTracerProvider(tp)
//
//	cfg := cli.OriginConfig{
//	    Processor: myProc,
//	    Hooks: []runtime.Hooks{
//	        hooks.OTelTracingHook(tp),
//	    },
//	}
//
// # Thread safety
//
// The hook mutates a single pipelineSpan field under a mutex. The
// current runtime fires hooks from one goroutine, but the
// runtime.Hooks docstring reserves the right to parallelize — the
// lock is cheap insurance.
//
// # Copy this file
//
// Copy tracing.go into your processor's cmd/<name>/ directory, rename
// `package hooks` to your package (usually `main`), and add OTel to
// your go.mod:
//
//	go get go.opentelemetry.io/otel
//	go get go.opentelemetry.io/otel/trace
func OTelTracingHook(tp trace.TracerProvider) runtime.Hooks {
	tracer := tp.Tracer("github.com/withObsrvr/nebu")

	var (
		mu            sync.Mutex
		pipelineCtx   context.Context
		pipelineSpan  trace.Span
		currentLedger trace.Span
	)

	return runtime.Hooks{
		OnStart: func(ctx context.Context, info runtime.PipelineInfo) {
			c, s := tracer.Start(ctx, "nebu.pipeline",
				trace.WithAttributes(
					attribute.String("processor.name", info.ProcessorName),
					attribute.Int64("nebu.start_ledger", int64(info.StartLedger)),
					attribute.Int64("nebu.end_ledger", int64(info.EndLedger)),
				),
			)
			mu.Lock()
			pipelineCtx, pipelineSpan = c, s
			mu.Unlock()
		},

		BeforeLedger: func(ctx context.Context, ledger xdr.LedgerCloseMeta) error {
			mu.Lock()
			parent := pipelineCtx
			mu.Unlock()
			if parent == nil {
				// OnStart didn't run (shouldn't happen in normal flow).
				parent = ctx
			}
			_, s := tracer.Start(parent, "nebu.ledger",
				trace.WithAttributes(
					attribute.Int64("nebu.ledger_sequence", int64(ledger.LedgerSequence())),
				),
			)
			mu.Lock()
			currentLedger = s
			mu.Unlock()
			return nil
		},

		AfterLedger: func(_ context.Context, _ xdr.LedgerCloseMeta, stats runtime.LedgerStats) {
			mu.Lock()
			s := currentLedger
			currentLedger = nil
			mu.Unlock()
			if s == nil {
				return
			}
			s.SetAttributes(attribute.Int64("nebu.ledger_duration_ms", stats.Duration.Milliseconds()))
			s.End()
		},

		OnWarning: func(_ context.Context, r processor.ErrorReport) {
			mu.Lock()
			target := currentLedger
			if target == nil {
				target = pipelineSpan
			}
			mu.Unlock()
			if target == nil {
				return
			}
			target.AddEvent("nebu.warning",
				trace.WithAttributes(
					attribute.String("processor", r.Processor),
					attribute.String("severity", r.Severity.String()),
					attribute.String("error", fmt.Sprint(r.Err)),
					attribute.String("event_id", r.EventID),
				),
			)
		},

		OnFatal: func(_ context.Context, r processor.ErrorReport) {
			mu.Lock()
			s := pipelineSpan
			mu.Unlock()
			if s == nil {
				return
			}
			s.RecordError(r.Err,
				trace.WithAttributes(
					attribute.String("severity", r.Severity.String()),
					attribute.String("event_id", r.EventID),
				),
			)
			s.SetStatus(codes.Error, fmt.Sprint(r.Err))
		},

		OnEnd: func(_ context.Context, summary runtime.PipelineSummary) {
			mu.Lock()
			s := pipelineSpan
			pipelineSpan, pipelineCtx = nil, nil
			mu.Unlock()
			if s == nil {
				return
			}
			s.SetAttributes(
				attribute.Int64("nebu.ledgers_processed", int64(summary.LedgersProcessed)),
				attribute.Int64("nebu.warnings", summary.Warnings),
				attribute.Int64("nebu.duration_ms", summary.Duration.Milliseconds()),
			)
			if summary.FatalErr != nil {
				// OnFatal already set Error status above; this keeps
				// the fallback path correct when FatalErr comes from a
				// non-ReportFatal source (source stream error, ctx
				// cancellation, BeforeLedger abort).
				s.SetStatus(codes.Error, fmt.Sprint(summary.FatalErr))
			} else {
				s.SetStatus(codes.Ok, "")
			}
			s.End()
		},
	}
}

