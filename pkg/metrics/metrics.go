// Package metrics provides Prometheus metrics for nebu processors.
package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Recorder collects Prometheus metrics for processor operations.
type Recorder struct {
	registry *prometheus.Registry

	eventsProcessed prometheus.Counter
	batchFlushes    prometheus.Counter
	retries         prometheus.Counter
	currentLedger   prometheus.Gauge
}

// NewRecorder creates a new metrics recorder with the given namespace.
func NewRecorder(namespace string) *Recorder {
	reg := prometheus.NewRegistry()

	r := &Recorder{
		registry: reg,
		eventsProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "events_processed_total",
			Help:      "Total number of events processed",
		}),
		batchFlushes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "batch_flushes_total",
			Help:      "Total number of batch flushes",
		}),
		retries: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "retries_total",
			Help:      "Total number of retry attempts",
		}),
		currentLedger: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "current_ledger_sequence",
			Help:      "Most recently processed ledger sequence",
		}),
	}

	reg.MustRegister(r.eventsProcessed)
	reg.MustRegister(r.batchFlushes)
	reg.MustRegister(r.retries)
	reg.MustRegister(r.currentLedger)

	return r
}

// RecordEventsProcessed increments the events processed counter by n.
func (r *Recorder) RecordEventsProcessed(n int) {
	r.eventsProcessed.Add(float64(n))
}

// RecordBatchFlush increments the batch flush counter.
func (r *Recorder) RecordBatchFlush() {
	r.batchFlushes.Inc()
}

// RecordRetry increments the retry counter.
func (r *Recorder) RecordRetry() {
	r.retries.Inc()
}

// SetCurrentLedger sets the current ledger sequence gauge.
func (r *Recorder) SetCurrentLedger(seq int64) {
	r.currentLedger.Set(float64(seq))
}

// Serve starts an HTTP server exposing /metrics on the given port.
// This function blocks and should be called in a goroutine.
func (r *Recorder) Serve(port int) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{}))

	addr := fmt.Sprintf(":%d", port)
	return http.ListenAndServe(addr, mux)
}
