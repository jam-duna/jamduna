package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/pvm"
)

func main() {
	// ./duna_target_mac -pvm-logging debug
	// ./duna_fuzzer_mac --test-dir ~/Desktop/jamtestnet/0.6.7/algo  --socket=/tmp/jam_target.sock
	socketPath := flag.String("socket", "/tmp/jam_target.sock", "Path for the Unix domain socket")
	pvmLogging := flag.String("pvm-logging", "none", "Logging level (none, debug, trace)")
	flag.Parse()

	// Define the target's identity.
	targetInfo := fuzz.PeerInfo{
		Name:       fmt.Sprintf("jam-duna-target-%s", fuzz.FUZZ_VERSION),
		AppVersion: fuzz.Version{Major: 0, Minor: 6, Patch: 7},
		JamVersion: fuzz.Version{Major: 0, Minor: 6, Patch: 7},
	}
	if pvmLogging != nil {
		if *pvmLogging == "trace" {
			pvm.PvmLogging = true
			pvm.PvmTrace = true
		} else if *pvmLogging == "debug" {
			pvm.PvmLogging = true
		}
	}

	target := fuzz.NewTarget(*socketPath, targetInfo, "interpreter")

	// Graceful shutdown setup
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		log.Println("Shutdown signal received, stopping target...")
		target.Stop()
		os.Exit(0)
	}()

	log.Printf("Starting target on socket: %s", *socketPath)
	if err := target.Start(); err != nil {
		log.Fatalf("Target failed to start: %v", err)
	}
}
