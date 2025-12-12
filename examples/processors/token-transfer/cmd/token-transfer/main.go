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
	"github.com/stellar/go-stellar-sdk/processors/token_transfer"
	token_transfer_processor "github.com/withObsrvr/nebu/examples/processors/token-transfer"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.3.0"

func main() {
	config := cli.OriginConfig{
		Name:        "token-transfer",
		Description: "Stream token transfer events from Stellar ledgers (transfers, mints, burns, clawbacks, fees)",
		Version:     version,
	}

	cli.RunProtoOriginCLI(config, func(networkPass string) cli.ProtoOriginProcessor[*token_transfer.TokenTransferEvent] {
		return token_transfer_processor.NewOrigin(networkPass)
	})
}
