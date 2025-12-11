package token_transfer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/stellar/go-stellar-sdk/processors/token_transfer"
	"github.com/withObsrvr/nebu/pkg/runtime"
	"github.com/withObsrvr/nebu/pkg/source"
)

// Server is an HTTP server that streams token transfer events.
// For MVP, we use HTTP with JSON streaming instead of gRPC to avoid protoc dependencies.
type Server struct {
	src    source.LedgerSource
	origin *Origin
}

// NewServer creates a new token transfer HTTP server.
func NewServer(src source.LedgerSource, origin *Origin) *Server {
	return &Server{
		src:    src,
		origin: origin,
	}
}

// ServeHTTP implements http.Handler for the token transfer service.
// GET /events?start=X&end=Y streams token transfer events as newline-delimited JSON.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr == "" || endStr == "" {
		http.Error(w, "missing required parameters: start and end", http.StatusBadRequest)
		return
	}

	start64, err := strconv.ParseUint(startStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid start ledger", http.StatusBadRequest)
		return
	}

	end64, err := strconv.ParseUint(endStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid end ledger", http.StatusBadRequest)
		return
	}

	start := uint32(start64)
	end := uint32(end64)

	// Set headers for streaming
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Create a fresh origin for this request
	origin := NewOrigin(s.origin.passphrase)
	defer origin.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Start runtime in background
	rt := runtime.NewRuntime()
	errCh := make(chan error, 1)

	go func() {
		err := rt.RunOrigin(ctx, s.src, origin, start, end)
		if err != nil && err != context.Canceled {
			errCh <- err
		}
		close(errCh)
		origin.Close() // Signal no more events
	}()

	// Stream events to client
	encoder := json.NewEncoder(w)
	flusher, _ := w.(http.Flusher)

	for {
		select {
		case err := <-errCh:
			if err != nil {
				// Write error as final JSON object
				_ = encoder.Encode(map[string]string{"error": err.Error()})
				if flusher != nil {
					flusher.Flush()
				}
			}
			return

		case event, ok := <-origin.Out():
			if !ok {
				// Channel closed, all events sent
				return
			}

			// Convert to a simpler format for JSON
			simplified := simplifyEvent(event)
			if err := encoder.Encode(simplified); err != nil {
				return // Client disconnected
			}

			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// Event is a simplified representation of a token transfer event for JSON serialization.
type Event struct {
	Type            string            `json:"type"`
	LedgerSequence  uint32            `json:"ledger_sequence"`
	TxHash          string            `json:"tx_hash"`
	ContractAddress string            `json:"contract_address,omitempty"`
	From            string            `json:"from,omitempty"`
	To              string            `json:"to,omitempty"`
	Amount          string            `json:"amount"`
	Asset           map[string]string `json:"asset,omitempty"`
}

// simplifyEvent converts a TokenTransferEvent protobuf to a JSON-friendly structure.
func simplifyEvent(ev *token_transfer.TokenTransferEvent) Event {
	meta := ev.GetMeta()
	event := Event{
		LedgerSequence:  meta.LedgerSequence,
		TxHash:          meta.TxHash,
		ContractAddress: meta.ContractAddress,
	}

	// Handle asset
	if asset := ev.GetAsset(); asset != nil {
		event.Asset = make(map[string]string)
		if asset.GetNative() {
			event.Asset["code"] = "native"
		} else if issued := asset.GetIssuedAsset(); issued != nil {
			event.Asset["code"] = issued.AssetCode
			event.Asset["issuer"] = issued.Issuer
		}
	}

	// Handle different event types
	switch {
	case ev.GetTransfer() != nil:
		transfer := ev.GetTransfer()
		event.Type = "transfer"
		event.From = transfer.From
		event.To = transfer.To
		event.Amount = transfer.Amount

	case ev.GetMint() != nil:
		mint := ev.GetMint()
		event.Type = "mint"
		event.To = mint.To
		event.Amount = mint.Amount

	case ev.GetBurn() != nil:
		burn := ev.GetBurn()
		event.Type = "burn"
		event.From = burn.From
		event.Amount = burn.Amount

	case ev.GetClawback() != nil:
		clawback := ev.GetClawback()
		event.Type = "clawback"
		event.From = clawback.From
		event.Amount = clawback.Amount

	case ev.GetFee() != nil:
		fee := ev.GetFee()
		event.Type = "fee"
		event.From = fee.From
		event.Amount = fee.Amount

	default:
		event.Type = "unknown"
	}

	return event
}

// RegisterRoutes registers the HTTP routes for this server on the given mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/events", s)
}

// HealthCheck returns a simple health check handler.
func HealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	}
}
