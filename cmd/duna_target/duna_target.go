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
)

func Terminate(stopCh chan os.Signal) {
	stopCh <- syscall.SIGTERM
	time.Sleep(100 * time.Millisecond)
}

func main() {
	// ./duna_target_mac -pvm-logging debug
	// ./duna_fuzzer_mac --test-dir ~/Desktop/jamtestnet/0.6.7/algo  --socket=/tmp/jam_target.sock

	var socketPath string = "/tmp/jam_target.sock"
	var pvmLogging string = "none"
	version := false

	fReg := fuzz.NewFlagRegistry("duna_target")
	fReg.RegisterFlag("socket", nil, socketPath, "Path for the Unix domain socket", &socketPath)
	fReg.RegisterFlag("pvm-logging", nil, pvmLogging, "Logging level (none, debug, trace)", &pvmLogging)
	fReg.RegisterFlag("version", "v", version, "Display version information", &version)
	fReg.ProcessRegistry()

	// Define the target's identity.
	targetInfo := fuzz.PeerInfo{
		AppVersion: fuzz.ParseVersion(fuzz.APP_VERSION),
		JamVersion: fuzz.ParseVersion(fuzz.JAM_VERSION),
		Name:       "jam-duna-target",
	}

	targetInfo.SetASNSpecific()

	fmt.Printf("Target Info:\n\n%s\n\n", targetInfo.Info())
	if version {
		return
	}

	statedb.RecordTime = false
	if pvmLogging != "" {
		if pvmLogging == "trace" {
			statedb.PvmLogging = true

		} else if pvmLogging == "debug" {
			statedb.PvmLogging = true
		}
	}
	target := fuzz.NewTarget(socketPath, targetInfo, "interpreter")

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
