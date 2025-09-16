package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func main() {

	dir := "/tmp/fuzzer"
	report_dir := "./reports"
	socket := "/tmp/jam_target.sock"
	enableRPC := false

	jConfig := types.ConfigJamBlocks{
		HTTP:        "http://localhost:8088/",
		Verbose:     false,
		NumBlocks:   500,
		InvalidRate: 0.25,
		Statistics:  20,
		Network:     "tiny",
	}

	fReg := fuzz.NewFlagRegistry("fuzzer")
	// fReg.RegisterFlag("mode", "m", jConfig.Mode, "Block generation mode", &jConfig.Mode)
	fReg.RegisterFlag("http", "h", jConfig.HTTP, "HTTP endpoint to send blocks", &jConfig.HTTP)
	// fReg.RegisterFlag("network", "n", jConfig.Network, "JAM network size", &jConfig.Network)
	fReg.RegisterFlag("verbose", "v", jConfig.Verbose, "Enable detailed logging", &jConfig.Verbose)
	// fReg.RegisterFlag("numblocks", nil, jConfig.NumBlocks, "Number of blocks to generate", &jConfig.NumBlocks)
	fReg.RegisterFlag("invalidrate", nil, jConfig.InvalidRate, "Percentage of invalid blocks", &jConfig.InvalidRate)
	fReg.RegisterFlag("statistics", nil, jConfig.Statistics, "Print statistics interval", &jConfig.Statistics)
	fReg.RegisterFlag("dir", nil, dir, "Storage directory", &dir)
	fReg.RegisterFlag("report-dir", nil, report_dir, "Report directory", &report_dir)
	fReg.RegisterFlag("rpc", nil, enableRPC, "Start RPC server", &enableRPC)
	fReg.RegisterFlag("socket", nil, socket, "Path for the Unix domain socket to connect to", &socket)

	fuzzerInfo := fuzz.CreatePeerInfo("passive-duna-fuzzer")
	fuzzerInfo.SetDefaults()

	fReg.ProcessRegistry()

	fmt.Printf("jConfig: %v\n", jConfig)

	fuzzer, err := fuzz.NewFuzzer(dir, report_dir, socket, fuzzerInfo, statedb.BackendInterpreter)
	if err != nil {
		log.Fatalf("Failed to initialize fuzzer: %v", err)
	}

	// Handle interrupts for graceful shutdown
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go fuzzer.RunImplementationRPCServer()
	go fuzzer.RunInternalRPCServer()
	startTime := time.Now()

	for {
		time.Sleep(50 * time.Millisecond)
		select {
		case <-stopCh:
			log.Println("Received interrupt, stopping early.")
			goto finish
		default:
		}
	}

finish:
	elapsed := time.Since(startTime).Seconds()
	log.Printf("Done in %.2fs\n", elapsed)
}
