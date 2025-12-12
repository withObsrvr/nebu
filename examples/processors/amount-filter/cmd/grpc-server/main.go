// Package main provides a gRPC server for the amount-filter transform processor.
//
// This server exposes the amount filter as a gRPC service that can be used
// in distributed nebu pipelines.
//
// Usage:
//
//	# Start server on default port (9001)
//	amount-filter-grpc-server
//
//	# Start server on custom port
//	amount-filter-grpc-server --port 9002
//
//	# Configure filter via environment variables
//	MIN_AMOUNT=10000000 ASSET_CODE=USDC amount-filter-grpc-server
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "github.com/withObsrvr/nebu/examples/processors/amount-filter/proto"
	"github.com/withObsrvr/nebu/examples/processors/amount-filter/server"
)

var (
	port = flag.Int("port", 9001, "The server port")
)

func main() {
	flag.Parse()

	// Create gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	amountFilterServer := server.NewServer()

	// Register service
	pb.RegisterAmountFilterServiceServer(grpcServer, amountFilterServer)

	// Enable reflection for debugging with grpcurl
	reflection.Register(grpcServer)

	// Apply environment-based configuration if provided
	if err := configureFromEnv(amountFilterServer); err != nil {
		log.Printf("Warning: failed to configure from environment: %v", err)
	}

	log.Printf("amount-filter gRPC server listening on :%d", *port)
	log.Printf("Use grpcurl for testing:")
	log.Printf("  grpcurl -plaintext localhost:%d list", *port)
	log.Printf("  grpcurl -plaintext localhost:%d nebu.amount_filter.AmountFilterService/GetConfig", *port)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

// configureFromEnv configures the filter from environment variables
func configureFromEnv(srv *server.Server) error {
	var configured bool
	var minAmount, maxAmount int64
	var assetCode string

	if val := os.Getenv("MIN_AMOUNT"); val != "" {
		parsed, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid MIN_AMOUNT: %w", err)
		}
		minAmount = parsed
		configured = true
	}

	if val := os.Getenv("MAX_AMOUNT"); val != "" {
		parsed, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid MAX_AMOUNT: %w", err)
		}
		maxAmount = parsed
		configured = true
	}

	if val := os.Getenv("ASSET_CODE"); val != "" {
		assetCode = val
		configured = true
	}

	if configured {
		_, err := srv.Configure(nil, &pb.ConfigureRequest{
			Config: &pb.FilterConfig{
				MinAmount: minAmount,
				MaxAmount: maxAmount,
				AssetCode: assetCode,
			},
		})
		if err != nil {
			return err
		}
		log.Printf("Configured from environment: min=%d, max=%d, asset=%s",
			minAmount, maxAmount, assetCode)
	}

	return nil
}
