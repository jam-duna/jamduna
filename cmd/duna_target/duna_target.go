package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/colorfulnotion/jam/fuzz"
)

func main() {
	socketPath := flag.String("socket", "/tmp/jam_target.sock", "Path for the Unix domain socket")
	pvmBackend := flag.String("pvm-backend", "interpreter", "PVM backend to use (Recompiler or Interpreter)")
	flag.Parse()

	// Define the target's identity.
	targetInfo := fuzz.PeerInfo{
		Name:       "jam-duna-target-v0.13",
		AppVersion: fuzz.Version{Major: 0, Minor: 6, Patch: 7},
		JamVersion: fuzz.Version{Major: 0, Minor: 6, Patch: 7},
	}

	target := fuzz.NewTarget(*socketPath, targetInfo, *pvmBackend)

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
