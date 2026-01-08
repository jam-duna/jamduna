package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/pvm/interpreter"
	"github.com/colorfulnotion/jam/statedb"
)

// defaultBackend can be set at build time via -ldflags "-X main.defaultBackend=compiler"
// Default is interpreter for compatibility
var defaultBackend = statedb.BackendInterpreter

func Terminate(stopCh chan os.Signal) {
	stopCh <- syscall.SIGTERM
	time.Sleep(100 * time.Millisecond)
}

func main() {
	// ./duna_target_mac -pvm-logging debug
	// ./duna_fuzzer_mac --test-dir ~/Desktop/jamtestnet/0.6.7/algo  --socket=/tmp/jam_target.sock

	var socketPath string = "/tmp/jam_target.sock"
	var pvmLogging string = "none"
	var debugState bool = false
	var dumpStf bool = false
	var dumpLocation string = "./stf"
	version := false

	fReg := fuzz.NewFlagRegistry("duna_target")
	fReg.RegisterFlag("socket", nil, socketPath, "Path for the Unix domain socket", &socketPath)
	fReg.RegisterFlag("pvm-logging", nil, pvmLogging, "Logging level (none, debug, trace)", &pvmLogging)
	fReg.RegisterFlag("debug-state", nil, debugState, "Enable detailed state debugging output", &debugState)
	fReg.RegisterFlag("dump-stf", nil, dumpStf, "Dump state transition files for debugging", &dumpStf)
	fReg.RegisterFlag("dump-location", nil, dumpLocation, "Directory path for STF dump files", &dumpLocation)
	fReg.RegisterFlag("version", "v", version, "Display version information", &version)
	fReg.ProcessRegistry()

	// Define the target's identity.
	targetInfo := fuzz.CreatePeerInfo("duna-target")
	targetInfo.SetDefaults()

	fmt.Printf("Target Info:\n\n%s\n\n", targetInfo.Info())
	if version {
		return
	}

	statedb.RecordTime = false
	if pvmLogging != "" {
		if pvmLogging == "trace" {
			interpreter.PvmLogging = true

		} else if pvmLogging == "debug" {
			interpreter.PvmLogging = true
		}
	}
	target := fuzz.NewTarget(socketPath, targetInfo, defaultBackend, debugState, dumpStf, dumpLocation)

	// Graceful shutdown setup
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopCh
		log.Println("TERMINATING TARGET")
		target.Stop()
		os.Exit(0)
	}()

	log.Printf("Starting target on socket: %s", socketPath)
	if err := target.Start(); err != nil {
		log.Fatalf("Target failed to start: %v", err)
	}
}
