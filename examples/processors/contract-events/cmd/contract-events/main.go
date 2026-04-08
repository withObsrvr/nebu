package main

import (
	"github.com/withObsrvr/nebu/examples/processors/contract-events"
	"github.com/withObsrvr/nebu/pkg/processor/cli"
)

var version = "0.1.0"

func main() {
	config := cli.OriginConfig{
		Name:        "contract-events",
		Description: "Extract all contract events from Stellar ledgers",
		Version:     version,
		SchemaID:    "nebu.contract_events.v1",
	}

	cli.RunProtoOriginCLI(config, func(networkPass string) cli.ProtoOriginProcessor[*contract_events.ContractEvent] {
		return contract_events.NewContractEventsOriginProto(networkPass)
	})
}
