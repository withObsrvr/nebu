// Package rpc provides a LedgerSource implementation backed by Stellar RPC.
//
// This package implements the source.LedgerSource interface from the parent
// pkg/source package. Importers pay the full cost of the Stellar RPC
// ledgerbackend dependencies; code that only needs the interface should
// import pkg/source directly.
package rpc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/withObsrvr/nebu/pkg/source/internal/ledgerstream"
)

// LedgerSource streams ledgers from Stellar RPC.
type LedgerSource struct {
	stream ledgerbackend.LedgerStream
	rpcURL string
}

// headerTransport is an HTTP transport that adds custom headers to all requests.
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

// RoundTrip implements http.RoundTripper by adding custom headers.
func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid modifying the original
	reqCopy := req.Clone(req.Context())

	// Add custom headers
	for key, value := range t.headers {
		reqCopy.Header.Set(key, value)
	}

	// Use the base transport to actually make the request
	return t.base.RoundTrip(reqCopy)
}

// NewLedgerSource creates a new ledger source that connects to Stellar RPC.
// The rpcURL should be a fully qualified URL (e.g., "https://archive-rpc.lightsail.network").
func NewLedgerSource(rpcURL string) (*LedgerSource, error) {
	return NewLedgerSourceWithHeaders(rpcURL, nil)
}

// NewLedgerSourceWithHeaders creates a new ledger source with custom HTTP headers.
// The headers map allows adding authentication headers like:
//
//	headers := map[string]string{"Authorization": "Api-Key YOUR_KEY"}
func NewLedgerSourceWithHeaders(rpcURL string, headers map[string]string) (*LedgerSource, error) {
	if rpcURL == "" {
		return nil, fmt.Errorf("rpcURL cannot be empty")
	}

	opts := ledgerbackend.RPCLedgerBackendOptions{
		RPCServerURL: rpcURL,
	}

	// If custom headers are provided, create an HTTP client with custom transport
	if len(headers) > 0 {
		transport := &headerTransport{
			base:    http.DefaultTransport,
			headers: headers,
		}
		opts.HttpClient = &http.Client{
			Transport: transport,
		}
	}

	return &LedgerSource{
		stream: ledgerbackend.NewRPCStream(opts, nil),
		rpcURL: rpcURL,
	}, nil
}

// Stream implements source.LedgerSource.Stream.
// Supports both bounded and unbounded ranges:
//   - Bounded: start > 0, end > 0 (streams ledgers from start to end)
//   - Unbounded: start > 0, end == 0 (streams ledgers from start continuously)
func (s *LedgerSource) Stream(
	ctx context.Context,
	start, end uint32,
	out chan<- xdr.LedgerCloseMeta,
) error {
	defer close(out)

	ledgerRange, err := ledgerstream.Range(start, end)
	if err != nil {
		return err
	}

	return ledgerstream.Pump(ctx, s.stream, ledgerRange, out)
}

// Close implements source.LedgerSource.Close.
//
// There is nothing to release: a LedgerStream sets its backend up on the
// first pull of a Stream call and tears it down when that iteration ends, so
// a source that is not streaming holds no connection. Close remains part of
// the contract and stays safe to call at any time.
func (s *LedgerSource) Close() error {
	return nil
}
