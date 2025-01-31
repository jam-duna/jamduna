package main

import (
	"log"

	"fmt"

	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/types"
)

func main() {

	dir := "/tmp/importBlock"
	enableRPC := false

	jConfig := types.ConfigJamBlocks{
		Mode:        "assurances",
		HTTP:        "http://localhost:8088/fuzz_json",
		QUIC:        "",
		Verbose:     false,
		NumBlocks:   500,
		InvalidRate: 0.25,
		Statistics:  20,
		Network:     "tiny",
	}

	fReg := fuzz.NewFlagRegistry("fuzzer")
	//fReg.RegisterFlag("mode", "m", jConfig.Mode, "Block generation mode", &jConfig.Mode)
	fReg.RegisterFlag("http", "h", jConfig.HTTP, "HTTP endpoint to send blocks", &jConfig.HTTP)
	fReg.RegisterFlag("quic", "q", jConfig.QUIC, "QUIC endpoint to send blocks", &jConfig.QUIC)
	//fReg.RegisterFlag("network", "n", jConfig.Network, "JAM network size", &jConfig.Network)
	fReg.RegisterFlag("verbose", "v", jConfig.Verbose, "Enable detailed logging", &jConfig.Verbose)
	//fReg.RegisterFlag("numblocks", nil, jConfig.NumBlocks, "Number of blocks to generate", &jConfig.NumBlocks)
	fReg.RegisterFlag("invalidrate", nil, jConfig.InvalidRate, "Percentage of invalid blocks", &jConfig.InvalidRate)
	fReg.RegisterFlag("statistics", nil, jConfig.Statistics, "Print statistics interval", &jConfig.Statistics)

	fReg.RegisterFlag("dir", nil, dir, "Storage directory", &dir)
	fReg.RegisterFlag("rpc", nil, enableRPC, "Start RPC server", &enableRPC)
	fReg.ProcessRegistry()

	fmt.Printf("jConfig: %v\n", jConfig)

	fuzzer, err := fuzz.NewFuzzer(dir)
	if err != nil {
		log.Fatalf("Failed to initialize fuzzer: %v", err)
	}

	go fuzzer.RunRPCServer()

	// set up network with config
	// node.ImportBlocks(&config)
	for {
		//TODO: tally stats??
	}
}
