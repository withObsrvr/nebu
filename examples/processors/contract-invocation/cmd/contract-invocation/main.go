// Package main provides a standalone CLI for the contract-invocation processor.
//
// This binary can be installed and run independently of the nebu CLI:
//
//	# Install
//	go install github.com/withObsrvr/nebu/examples/processors/contract-invocation/cmd/contract-invocation@latest
//
//	# Run
//	contract-invocation --start-ledger 60200000 --end-ledger 60200100
//	cat ledgers.xdr | contract-invocation
//	nebu fetch 60200000 60200100 | contract-invocation
package main

import (
	contract_invocation_processor "github.com/withObsrvr/nebu/examples/processors/contract-invocation"
	cipb "github.com/withObsrvr/nebu/examples/processors/contract-invocation/proto"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

func main() {
	config := cli.OriginConfig{
		Name:        "contract-invocation",
		Description: "Stream contract invocation events from Stellar ledgers (function calls, cross-contract calls, state changes)",
		Version:     version,
	}

	cli.RunProtoOriginCLI(config, func(networkPass string) cli.ProtoOriginProcessor[*cipb.ContractInvocation] {
		return contract_invocation_processor.NewOrigin(networkPass)
	})
}
