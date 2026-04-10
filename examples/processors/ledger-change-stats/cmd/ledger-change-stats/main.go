package main

import (
	ledger_change_stats "github.com/withObsrvr/nebu/examples/processors/ledger-change-stats"
	changestatspb "github.com/withObsrvr/nebu/examples/processors/ledger-change-stats/proto"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

func main() {
	config := cli.OriginConfig{
		Name:        "ledger-change-stats",
		Description: "Emit per-ledger created/updated/deleted change counts and change reasons",
		Version:     version,
		SchemaID:    "nebu.ledger_change_stats.v1",
	}

	cli.RunProtoOriginCLI(config, func(networkPass string) cli.ProtoOriginProcessor[*changestatspb.LedgerChangeStats] {
		return ledger_change_stats.NewOrigin(networkPass)
	})
}
