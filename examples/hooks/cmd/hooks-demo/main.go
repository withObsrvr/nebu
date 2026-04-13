// Command hooks-demo runs a tiny origin processor against Stellar
// mainnet RPC and wires in all three reference hooks from
// github.com/withObsrvr/nebu/examples/hooks:
//
//   - RateLimitHook:   throttles to 10 ledgers/sec
//   - PrometheusHook:  counters + histograms exposed on :9090/metrics
//   - OTelTracingHook: one span per ledger, stdout exporter for the demo
//
// It's a smoke test: if this runs cleanly, the reference hooks work.
// It is not a production-grade example.
//
// Run with:
//
//	go run ./examples/hooks/cmd/hooks-demo
//
// In another terminal, while it's running:
//
//	curl -s http://localhost:9090/metrics | grep nebu_
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stellar/go-stellar-sdk/xdr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	refhooks "github.com/withObsrvr/nebu/examples/hooks"
	"github.com/withObsrvr/nebu/pkg/processor"
	"github.com/withObsrvr/nebu/pkg/runtime"
	"github.com/withObsrvr/nebu/pkg/source/rpc"
)

// tinyOrigin is a no-op origin — we just want to exercise the hook
// lifecycle, not any real event extraction.
type tinyOrigin struct{ count int }

func (o *tinyOrigin) Name() string          { return "hooks-demo" }
func (o *tinyOrigin) Type() processor.Type  { return processor.TypeOrigin }

func (o *tinyOrigin) ProcessLedger(_ context.Context, ledger xdr.LedgerCloseMeta) {
	o.count++
	// Artificial work so the duration histogram has non-trivial values.
	time.Sleep(5 * time.Millisecond)
	fmt.Fprintf(os.Stdout, "{\"ledger\": %d, \"n\": %d}\n", ledger.LedgerSequence(), o.count)
}

func main() {
	// --- OTel: stdout span exporter --------------------------------
	// Swap os.Stderr for io.Discard if spans are too noisy; swap for
	// OTLP in production.
	exporter, err := stdouttrace.New(
		stdouttrace.WithWriter(os.Stderr),
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		log.Fatalf("stdouttrace exporter: %v", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	// --- Prometheus: registry + /metrics on :9090 ------------------
	reg := prometheus.NewRegistry()
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	srv := &http.Server{Addr: ":9090", Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server: %v", err)
		}
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	log.Println("metrics: http://localhost:9090/metrics")

	// --- Origin + source -------------------------------------------
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	src, err := rpc.NewLedgerSource("https://archive-rpc.lightsail.network")
	if err != nil {
		log.Fatalf("new rpc source: %v", err)
	}
	defer src.Close()

	proc := &tinyOrigin{}

	// --- Wire up all three hooks -----------------------------------
	rt := runtime.NewRuntime()
	rt.Use(refhooks.RateLimitHook(10))                  // 10 ledgers/sec cap
	rt.Use(refhooks.PrometheusHook(reg, proc.Name()))   // /metrics counters
	rt.Use(refhooks.OTelTracingHook(tp))                // span per ledger

	// A small bounded range so the demo finishes in a few seconds.
	const startLedger, endLedger uint32 = 62080000, 62080010

	log.Printf("running %s: ledgers %d..%d", proc.Name(), startLedger, endLedger)
	start := time.Now()
	if err := rt.RunOrigin(ctx, src, proc, startLedger, endLedger); err != nil {
		log.Fatalf("run origin: %v", err)
	}

	// Give async flush a moment before exit.
	time.Sleep(250 * time.Millisecond)
	log.Printf("done: %d ledgers in %s", proc.count, time.Since(start).Round(time.Millisecond))
}
