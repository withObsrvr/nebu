package main

import (
	transaction_stats "github.com/withObsrvr/nebu/examples/processors/transaction-stats"
	txstatspb "github.com/withObsrvr/nebu/examples/processors/transaction-stats/proto"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

func main() {
	config := cli.OriginConfig{
		Name:        "transaction-stats",
		Description: "Summarize successful/failed transactions and operations across a ledger range",
		Version:     version,
		SchemaID:    "nebu.transaction_stats.v1",
	}

	cli.RunProtoOriginCLI(config, func(networkPass string) cli.ProtoOriginProcessor[*txstatspb.TransactionStats] {
		return transaction_stats.NewOrigin(networkPass)
	})
}
