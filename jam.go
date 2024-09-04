package main

import (
	//	"encoding/json"
	"flag"
	"fmt"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"os"
	"path/filepath"
)

func main() {
	// Parse the command-line flags into config
	config := &types.CommandConfig{}
	flag.BoolVar(&config.Help, "help", false, "Displays help information about the commands and flags.")
	flag.BoolVar(&config.Help, "h", false, "Displays help information about the commands and flags.")
	flag.StringVar(&config.DataDir, "/tmp/jam", filepath.Join(os.Getenv("HOME"), ".jam"), "Specifies the directory for the blockchain, keystore, and other data.")
	flag.IntVar(&config.Port, "port", 9000, "Specifies the network listening port.")
	flag.BoolVar(&config.RPC, "rpc", true, "Enables the HTTP-RPC server.")
	flag.BoolVar(&config.LogJson, "logjson", false, "Logs output in JSON format.")
	flag.StringVar(&config.Genesis, "genesis", "genesis.json", "Specifies the path to the  genesis state.")
	flag.StringVar(&config.NodeName, "nodename", "node0", "Node name")
	// service mindset
	flag.BoolVar(&config.Safrole, "safrole", true, `Supports Safrole messages: "Ticket", "Block".`)
	flag.BoolVar(&config.Guarantee, "guarantee", true, `Supports Guarantee messages: "Guarantee", "WorkPackage".`)
	flag.BoolVar(&config.Assurance, "assurance", true, `Supports Assurance messages: "Assurance", "AvailabilityJustification".`)
	flag.BoolVar(&config.Auditing, "auditing", true, `Supports Auditing messages: "Announcement".`)
	flag.BoolVar(&config.Disputes, "disputes", true, `Supports Disputes messages: "Vote".`)
	flag.BoolVar(&config.Preimages, "preimages", true, `Supports Preimages messages: "Preimages".`)
	flag.BoolVar(&config.DA, "da", true, `Supports DA messages: "DistributeECChunk", "ECChunkQuery", "BlockQuery", "ImportDAQuery", "AuditDAQuery", "ImportDAReconstructQuery".`)
	flag.Parse()

	// If help is requested, print usage and exit
	if config.Help {
		fmt.Println("Usage: jam [options]")
		flag.PrintDefaults()
		os.Exit(0)
	}

	// TODO: generate/genesis configuration, validator secrets, peers for NewNode
	var credential types.ValidatorSecret
	var genesisConfig statedb.GenesisConfig
	var peers []string
	peerList := make(map[string]node.NodeInfo)
	_, err := node.NewNode(config.NodeName, credential, &genesisConfig, peers, peerList)
	if err != nil {
		panic(1)
	}
	for {
		// TODO: receive an input, grab std in, parse it, Decode the hex encoded input, and send it into the node
		// TODO: for all outputs, send it out to stdout
	}
	fmt.Println(config.String())
}
