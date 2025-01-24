package main

import (
	"log"

	"flag"
	"fmt"

	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/types"
)

func main() {
	fmt.Printf("importblocks - JAM Import Blocks generator\n")

	enableFlag := false
	config := types.ConfigJamBlocks{}
	storageDir := "/tmp/fuzzer"

	if enableFlag {
		mode := flag.String("m", "safrole", "Block generation mode: fallback, safrole, assurances, orderedaccumulation (under development: authorization, recenthistory, blessed, basichostfunctions, disputes, gas, finalization)")
		flag.StringVar(mode, "mode", *mode, "Block generation mode: fallback, safrole, assurances, orderedaccumulation")

		httpEndpoint := flag.String("h", "", "HTTP endpoint to send blocks")
		flag.StringVar(httpEndpoint, "http", *httpEndpoint, "HTTP endpoint to send blocks")

		quicEndpoint := flag.String("q", "", "QUIC endpoint to send blocks")
		flag.StringVar(quicEndpoint, "quic", *quicEndpoint, "QUIC endpoint to send blocks")

		//rpc := flag.Bool("rpc", false, "Start RPC server to accept STF objects")

		verbose := flag.Bool("v", false, "Enable detailed logging")
		flag.BoolVar(verbose, "verbose", *verbose, "Enable detailed logging")

		network := flag.String("n", "tiny", "JAM network size: tiny, full")
		flag.StringVar(network, "network", *network, "JAM network size: tiny, full")

		numBlocks := flag.Int("numblocks", 50, "Number of valid blocks to generate (max 600)")
		invalidRate := flag.Int("invalidrate", 0, "Percentage of blocks that are invalid (under development)")
		statistics := flag.Int("statistics", 10, "Number of valid blocks between statistics dumps")
		datadir := flag.String("datadir", "/tmp/fuzzer", "Directory for storage")
		flag.StringVar(datadir, "dir", *datadir, "Directory for storage")
		storageDir = *datadir

		flag.Parse()
		config = types.ConfigJamBlocks{
			Mode:        *mode,
			HTTP:        *httpEndpoint,
			QUIC:        *quicEndpoint,
			Verbose:     *verbose,
			NumBlocks:   *numBlocks,
			InvalidRate: *invalidRate,
			Statistics:  *statistics,
			Network:     *network,
		}

	} else {
		config = types.ConfigJamBlocks{
			Mode:        "safrole",
			HTTP:        "http://localhost:8088/fuzz_json",
			QUIC:        "",
			Verbose:     false,
			NumBlocks:   50,
			InvalidRate: 0,
			Statistics:  10,
			Network:     "tiny",
		}
	}

	fmt.Printf("config: %v\n", config)

	fuzzer, err := fuzz.NewFuzzer(storageDir)
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
