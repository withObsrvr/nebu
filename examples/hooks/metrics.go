package hooks

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
)

// PrometheusHook returns a runtime.Hooks bundle that records pipeline
// activity as Prometheus metrics. Register the returned hooks with
// OriginConfig.Hooks, then expose `reg` from an HTTP handler via
// promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).
//
// # Metrics emitted
//
//   - nebu_ledgers_processed_total{processor="..."}
//         counter — incremented once per AfterLedger call
//
//   - nebu_ledger_duration_seconds{processor="..."}
//         histogram — seconds spent inside origin.ProcessLedger,
//         buckets tuned for sub-second ledger processing
//         (1ms, 5ms, 10ms, 50ms, 100ms, 500ms, 1s, 5s, 10s)
//
//   - nebu_warnings_total{processor="...", severity="..."}
//         counter — incremented per OnWarning call
//
//   - nebu_fatals_total{processor="...", severity="..."}
//         counter — incremented per OnFatal call (typically 0 or 1)
//
//   - nebu_pipeline_duration_seconds{processor="...", outcome="..."}
//         histogram observation made once on OnEnd; outcome is "ok"
//         for clean completion, "error" for any non-nil FatalErr
//
// # Arguments
//
//   - reg: a prometheus.Registerer (pass prometheus.DefaultRegisterer
//     for the global registry, or a custom *prometheus.Registry for
//     scoped metrics in tests).
//   - processorName: labels all metrics with this value. Use the same
//     name your processor returns from Processor.Name().
//
// # Example wiring
//
//	reg := prometheus.NewRegistry()
//	cfg := cli.OriginConfig{
//	    Processor: myProc,
//	    Hooks: []runtime.Hooks{
//	        hooks.PrometheusHook(reg, myProc.Name()),
//	    },
//	}
//
//	// Expose /metrics on :9090 in a goroutine.
//	go http.ListenAndServe(":9090", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
//
// # Copy this file
//
// Copy metrics.go into your processor's cmd/<name>/ directory, rename
// `package hooks` to your package (usually `main`), and add the
// Prometheus client to your go.mod:
//
//	go get github.com/prometheus/client_golang/prometheus
func PrometheusHook(reg prometheus.Registerer, processorName string) runtime.Hooks {
	labels := prometheus.Labels{"processor": processorName}

	ledgersProcessed := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "nebu_ledgers_processed_total",
		Help:        "Total ledgers processed by the origin.",
		ConstLabels: labels,
	})
	ledgerDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        "nebu_ledger_duration_seconds",
		Help:        "Time spent inside origin.ProcessLedger, in seconds.",
		Buckets:     []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10},
		ConstLabels: labels,
	})
	warnings := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "nebu_warnings_total",
		Help:        "Total processor warnings emitted via ReportWarning.",
		ConstLabels: labels,
	}, []string{"severity"})
	fatals := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name:        "nebu_fatals_total",
		Help:        "Total processor fatals emitted via ReportFatal.",
		ConstLabels: labels,
	}, []string{"severity"})
	pipelineDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        "nebu_pipeline_duration_seconds",
		Help:        "Total wall-clock duration of a pipeline run, in seconds.",
		Buckets:     prometheus.ExponentialBuckets(1, 2, 10), // 1s → ~17min
		ConstLabels: labels,
	}, []string{"outcome"})

	reg.MustRegister(ledgersProcessed, ledgerDuration, warnings, fatals, pipelineDuration)

	return runtime.Hooks{
		AfterLedger: func(_ context.Context, _ xdr.LedgerCloseMeta, stats runtime.LedgerStats) {
			ledgersProcessed.Inc()
			ledgerDuration.Observe(stats.Duration.Seconds())
		},

		OnWarning: func(_ context.Context, r processor.ErrorReport) {
			warnings.WithLabelValues(r.Severity.String()).Inc()
		},

		OnFatal: func(_ context.Context, r processor.ErrorReport) {
			fatals.WithLabelValues(r.Severity.String()).Inc()
		},

		OnEnd: func(_ context.Context, s runtime.PipelineSummary) {
			outcome := "ok"
			if s.FatalErr != nil {
				outcome = "error"
			}
			pipelineDuration.WithLabelValues(outcome).Observe(s.Duration.Seconds())
		},
	}
}

// Compile-time sanity check — fails cleanly if the runtime API drifts.
var _ = func() runtime.Hooks { return PrometheusHook(prometheus.NewRegistry(), "test") }
