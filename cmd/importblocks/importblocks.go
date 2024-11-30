package main

import (
	"flag"
	"fmt"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/types"
	"log"
)

func validateConfig(config types.ConfigJamBlocks) {
	if config.HTTP == "" && config.QUIC == "" {
		log.Fatalf("You must specify either an HTTP URL or a QUIC address")
	}
	if config.QUIC != "" {
		log.Fatalf("QUIC functionality is not implemented yet. Endpoint: %s", config.Endpoint)
	}
	if config.Network != "tiny" {
		log.Fatalf("Tiny network only")
	}
	if config.Mode != "fallback" && config.Mode != "safrole" && config.Mode != "assurances" && config.Mode != "orderedaccumulation" {
		log.Fatalf("Invalid mode: %s. Must be one of fallback, safrole, assurances, orderedaccumulation", config.Mode)
	}
}

func main() {
	fmt.Printf("importblocks - JAM Import Blocks generator\n")

	mode := flag.String("m", "safrole", "Block generation mode: fallback, safrole, assurances, orderedaccumulation (under development: authorization, recenthistory, blessed, basichostfunctions, disputes, gas, finalization)")
	flag.StringVar(mode, "mode", *mode, "Block generation mode: fallback, safrole, assurances, orderedaccumulation")

	httpEndpoint := flag.String("h", "", "HTTP endpoint to send blocks")
	flag.StringVar(httpEndpoint, "http", *httpEndpoint, "HTTP endpoint to send blocks")

	quicEndpoint := flag.String("q", "", "QUIC endpoint to send blocks")
	flag.StringVar(quicEndpoint, "quic", *quicEndpoint, "QUIC endpoint to send blocks")

	verbose := flag.Bool("v", false, "Enable detailed logging")
	flag.BoolVar(verbose, "verbose", *verbose, "Enable detailed logging")

	network := flag.String("n", "tiny", "JAM network size: tiny, full")
	flag.StringVar(network, "network", *network, "JAM network size: tiny, full")

	numBlocks := flag.Int("numblocks", 50, "Number of valid blocks to generate (max 600)")
	invalidRate := flag.Int("invalidrate", 0, "Percentage of blocks that are invalid (under development)")
	statistics := flag.Int("statistics", 10, "Number of valid blocks between statistics dumps")

	flag.Parse()
	config := types.ConfigJamBlocks{
		Mode:        *mode,
		HTTP:        *httpEndpoint,
		QUIC:        *quicEndpoint,
		Verbose:     *verbose,
		NumBlocks:   *numBlocks,
		InvalidRate: *invalidRate,
		Statistics:  *statistics,
		Network:     *network,
	}
	validateConfig(config)
	// set up network with config
	node.ImportBlocks(&config)
	for {

	}
}
