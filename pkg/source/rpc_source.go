package source

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stellar/go-stellar-sdk/ingest/ledgerbackend"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// RPCLedgerSource streams ledgers from Stellar RPC using the RPCLedgerBackend.
type RPCLedgerSource struct {
	backend ledgerbackend.LedgerBackend
	rpcURL  string
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

// NewRPCLedgerSource creates a new ledger source that connects to Stellar RPC.
// The rpcURL should be a fully qualified URL (e.g., "https://archive-rpc.lightsail.network").
func NewRPCLedgerSource(rpcURL string) (*RPCLedgerSource, error) {
	return NewRPCLedgerSourceWithHeaders(rpcURL, nil)
}

// NewRPCLedgerSourceWithHeaders creates a new ledger source with custom HTTP headers.
// The headers map allows adding authentication headers like:
//
//	headers := map[string]string{"Authorization": "Api-Key YOUR_KEY"}
func NewRPCLedgerSourceWithHeaders(rpcURL string, headers map[string]string) (*RPCLedgerSource, error) {
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

	backend := ledgerbackend.NewRPCLedgerBackend(opts)

	return &RPCLedgerSource{
		backend: backend,
		rpcURL:  rpcURL,
	}, nil
}

// Stream implements LedgerSource.Stream.
// Supports both bounded and unbounded ranges:
//   - Bounded: start > 0, end > 0 (streams ledgers from start to end)
//   - Unbounded: start > 0, end == 0 (streams ledgers from start continuously)
func (s *RPCLedgerSource) Stream(
	ctx context.Context,
	start, end uint32,
	out chan<- xdr.LedgerCloseMeta,
) error {
	defer close(out)

	// Validate start ledger
	if start == 0 {
		return fmt.Errorf("start ledger must be > 0 (got %d)", start)
	}

	// Validate bounded range
	if end > 0 && start > end {
		return fmt.Errorf("invalid range: start (%d) must be <= end (%d)", start, end)
	}

	// Prepare the range (bounded or unbounded)
	var rang ledgerbackend.Range
	if end == 0 {
		// Unbounded range - stream continuously from start
		rang = ledgerbackend.UnboundedRange(start)
		log.Infof("Preparing unbounded range starting at ledger %d", start)
	} else {
		// Bounded range - stream from start to end
		rang = ledgerbackend.BoundedRange(start, end)
		log.Infof("Preparing bounded range [%d, %d]", start, end)
	}

	if err := s.backend.PrepareRange(ctx, rang); err != nil {
		log.Errorf("failed to prepare range: %v", err)
		return fmt.Errorf("failed to prepare range: %w", err)
	}

	// Stream ledgers
	seq := start
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ledger, err := s.backend.GetLedger(ctx, seq)
		if err != nil {
			return fmt.Errorf("failed to get ledger %d: %w", seq, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- ledger:
			// Ledger sent successfully
		}

		seq++

		// Stop if bounded range and reached end
		if end > 0 && seq > end {
			return nil
		}
	}
}

// Close implements LedgerSource.Close.
func (s *RPCLedgerSource) Close() error {
	return s.backend.Close()
}
