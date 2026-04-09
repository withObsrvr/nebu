// Package main provides a standalone CLI for the token-transfer processor.
//
// This binary can be installed and run independently of the nebu CLI:
//
//	# Install
//	go install github.com/withObsrvr/nebu/examples/processors/token-transfer/cmd@latest
//
//	# Run
//	token-transfer --start-ledger 60200000 --end-ledger 60200100
//	cat ledgers.xdr | token-transfer
//	nebu fetch 60200000 60200100 | token-transfer
package main

import (
	token_transfer_processor "github.com/withObsrvr/nebu/examples/processors/token-transfer"
	ttpb "github.com/withObsrvr/nebu/examples/processors/token-transfer/proto"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
	"github.com/withObsrvr/nebu/pkg/runtime"
)

var version = "0.3.0"

func main() {
	config := cli.OriginConfig{
		Name:        "token-transfer",
		Description: "Stream token transfer events from Stellar ledgers (transfers, mints, burns, clawbacks, fees)",
		Version:     version,
		SchemaID:    "nebu.token_transfer.v1",

		// Hooks: a single bundle that prints live ledger progress to
		// stderr during long backfills. See progress.go for the
		// implementation. Showing this here as a reference for how
		// processor authors wire observability via the v0.6.1 hooks
		// interface — drop in any [runtime.Hooks] bundle to plug in
		// metrics, tracing, checkpoints, or anything else.
		Hooks: []runtime.Hooks{
			progressHook(),
		},
	}

	cli.RunProtoOriginCLI(config, func(networkPass string) cli.ProtoOriginProcessor[*ttpb.TokenTransferEvent] {
		return token_transfer_processor.NewOrigin(networkPass)
	})
}
