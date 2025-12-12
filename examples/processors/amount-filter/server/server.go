// Package server implements the gRPC server for the amount-filter transform processor.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/withObsrvr/nebu/examples/processors/amount-filter"
	pb "github.com/withObsrvr/nebu/examples/processors/amount-filter/proto"
)

// Server implements the AmountFilterService gRPC server.
type Server struct {
	pb.UnimplementedAmountFilterServiceServer
	mu     sync.RWMutex
	filter *amount_filter.Filter
}

// NewServer creates a new amount filter gRPC server with default configuration.
func NewServer() *Server {
	return &Server{
		filter: amount_filter.NewFilter(0, 0, ""), // Default: no filtering
	}
}

// Configure sets the filter parameters.
func (s *Server) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	cfg := req.GetConfig()

	s.mu.Lock()
	s.filter = amount_filter.NewFilter(
		cfg.GetMinAmount(),
		cfg.GetMaxAmount(),
		cfg.GetAssetCode(),
	)
	s.mu.Unlock()

	return &pb.ConfigureResponse{
		Success: true,
		Message: fmt.Sprintf("Filter configured: min=%d, max=%d, asset=%s",
			cfg.GetMinAmount(), cfg.GetMaxAmount(), cfg.GetAssetCode()),
	}, nil
}

// GetConfig returns the current filter configuration.
func (s *Server) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &pb.GetConfigResponse{
		Config: &pb.FilterConfig{
			MinAmount: s.filter.MinAmount,
			MaxAmount: s.filter.MaxAmount,
			AssetCode: s.filter.AssetCode,
		},
	}, nil
}

// Transform applies the filter to a single event.
func (s *Server) Transform(ctx context.Context, req *pb.TransformRequest) (*pb.TransformResponse, error) {
	// Decode JSON event
	var event map[string]interface{}
	if err := json.Unmarshal(req.GetEventJson(), &event); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Apply filter
	s.mu.RLock()
	result := s.filter.FilterEvent(event)
	s.mu.RUnlock()

	if result == nil {
		// Event filtered out
		return &pb.TransformResponse{
			Filtered: true,
		}, nil
	}

	// Encode result
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to encode result: %w", err)
	}

	return &pb.TransformResponse{
		EventJson: resultJSON,
		Filtered:  false,
	}, nil
}

// TransformStream applies the filter to a stream of events.
func (s *Server) TransformStream(stream pb.AmountFilterService_TransformStreamServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Decode JSON event
		var event map[string]interface{}
		if err := json.Unmarshal(req.GetEventJson(), &event); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}

		// Apply filter
		s.mu.RLock()
		result := s.filter.FilterEvent(event)
		s.mu.RUnlock()

		resp := &pb.TransformResponse{
			Filtered: result == nil,
		}

		if result != nil {
			resultJSON, err := json.Marshal(result)
			if err != nil {
				return fmt.Errorf("failed to encode result: %w", err)
			}
			resp.EventJson = resultJSON
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}
