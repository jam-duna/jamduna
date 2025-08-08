package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

const (
	numBlocksMax = 650
	magicConst   = 1107
)

func validateImportBlockConfig(jConfig types.ConfigJamBlocks, useSocket bool) {
	if !useSocket && jConfig.HTTP == "" && jConfig.QUIC == "" {
		log.Fatalf("You must specify either an HTTP URL (--http), a QUIC address (--quic), or enable socket communication (--use-unix-socket).")
	}
	if jConfig.Network != "tiny" && jConfig.Network != "full" {
		log.Fatalf("Invalid --network value: %s. Must be 'tiny' or 'full'.", jConfig.Network)
	}
	_, modeErr := fuzz.CheckModes(jConfig.Mode)
	if modeErr != nil {
		log.Fatalf(modeErr.Error())
	}
}

func main() {
	fmt.Println("importblocks - JAM Duna Import Blocks generator")

	dir := "/tmp/importBlock"
	//socket := "/tmp/jam_target.sock"
	enableRPC := false
	useUnixSocket := true
	test_dir := "./rawdata"

	jConfig := types.ConfigJamBlocks{
		Mode:        "safrole",
		HTTP:        "http://localhost:8088/",
		Socket:      "/tmp/jam_target.sock",
		QUIC:        "",
		Verbose:     false,
		NumBlocks:   50,
		InvalidRate: 0,
		Statistics:  20,
		Network:     "tiny",
		PVMBackend:  pvm.BackendInterpreter,
		Seed:        "0x44554E41",
	}

	fReg := fuzz.NewFlagRegistry("importblocks")
	fReg.RegisterFlag("seed", nil, jConfig.Seed, "Seed for random number generation (as hex)", &jConfig.Seed)
	fReg.RegisterFlag("mode", "m", jConfig.Mode, "Block generation mode", &jConfig.Mode)
	fReg.RegisterFlag("http", "h", jConfig.HTTP, "HTTP endpoint to send blocks", &jConfig.HTTP)
	fReg.RegisterFlag("quic", "q", jConfig.QUIC, "QUIC endpoint to send blocks", &jConfig.QUIC)
	fReg.RegisterFlag("network", "n", jConfig.Network, "JAM network size", &jConfig.Network)
	fReg.RegisterFlag("verbose", "v", jConfig.Verbose, "Enable detailed logging", &jConfig.Verbose)
	fReg.RegisterFlag("numblocks", nil, jConfig.NumBlocks, "Number of blocks to generate", &jConfig.NumBlocks)
	fReg.RegisterFlag("invalidrate", nil, jConfig.InvalidRate, "Percentage of invalid blocks", &jConfig.InvalidRate)
	fReg.RegisterFlag("statistics", nil, jConfig.Statistics, "Print statistics interval", &jConfig.Statistics)
	fReg.RegisterFlag("dir", nil, dir, "Storage directory", &dir)
	fReg.RegisterFlag("test-dir", nil, test_dir, "Storage directory", &test_dir)
	fReg.RegisterFlag("socket", nil, jConfig.Socket, "Path for the Unix domain socket to connect to", &jConfig.Socket)
	fReg.RegisterFlag("use-unix-socket", nil, useUnixSocket, "Enable to use Unix domain socket for communication", &useUnixSocket)
	fReg.RegisterFlag("pvm-backend", nil, jConfig.PVMBackend, "PVM backend to use (Recompiler or Interpreter)", &jConfig.PVMBackend)
	fReg.ProcessRegistry()
	fmt.Printf("%v\n", jConfig)

	// Set up immediate cancellation on Ctrl-C.
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopCh
		log.Println("\nInterrupt signal received, exiting immediately.")
		os.Exit(0)
	}()

	validateImportBlockConfig(jConfig, useUnixSocket)
	fuzzerInfo := fuzz.PeerInfo{
		Name:       "jam-duna-fuzzer-v0.6",
		AppVersion: fuzz.Version{Major: 0, Minor: 6, Patch: 7},
		JamVersion: fuzz.Version{Major: 0, Minor: 6, Patch: 7},
	}
	fuzzer, err := fuzz.NewFuzzer(dir, jConfig.Socket, fuzzerInfo, jConfig.PVMBackend)
	if err != nil {
		log.Fatalf("Failed to initialize fuzzer: %v", err)
	}
	seed := common.FromHex(jConfig.Seed)
	fuzzer.SetSeed(seed)

	if enableRPC {
		go func() {
			log.Println("Starting RPC server...")
			fuzzer.RunImplementationRPCServer()
		}()
	}

	startTime := time.Now()
	numBlocks := jConfig.NumBlocks
	nonStopFlagSet := jConfig.NumBlocks == magicConst
	mode := jConfig.Mode

	log.Printf("[INFO] Starting block generation: mode=%s, numBlocks=%d, dir=%s\n", mode, numBlocks, dir)
	/*
		baseDir := os.Getenv("TEST_DATA_DIR")
		if baseDir == "" {
			baseDir = "./rawdata"
		}
	*/
	raw_stfs, err := fuzz.ReadStateTransitions(test_dir, mode)

	usable_stfs := make([]*statedb.StateTransition, 0)
	for _, stf := range raw_stfs {
		if fuzz.HasParentStfs(raw_stfs, stf) {
			usable_stfs = append(usable_stfs, stf)
		} else {
			log.Printf("Skipping STF with no parent: %s", stf.Block.Header.HeaderHash().Hex())
		}
	}

	if err != nil || len(usable_stfs) == 0 {
		log.Printf("No %v mode data available on BaseDir=%v. Exit!", mode, test_dir)
		return
	}

	stfTestBank, fuzzErr := fuzzer.FuzzWithTargetedInvalidRate([]string{mode}, usable_stfs, jConfig.InvalidRate, numBlocks)
	if fuzzErr != nil {
		log.Fatal(fuzzErr)
	}

	fStat := fuzz.FuzzStats{}

	if useUnixSocket {
		if err := fuzzer.Connect(); err != nil {
			log.Fatalf("Failed to connect to target for test run: %v", err)
		}
		defer fuzzer.Close()
		if _, err := fuzzer.Handshake(); err != nil {
			log.Fatalf("Handshake failed before test run: %v", err)
		}
	}

	for i := 0; i < numBlocks || nonStopFlagSet; i++ {
		time.Sleep(50 * time.Millisecond)
		if i >= len(stfTestBank) {
			log.Println("No more state transitions available.")
			break
		}
		stfQA := stfTestBank[i]
		challengerFuzzed := stfQA.Mutated

		fStat.TotalBlocks++
		if challengerFuzzed {
			fStat.FuzzedBlocks++
		} else {
			fStat.OriginalBlocks++
		}

		if useUnixSocket {
			isMatch, solverFuzzed, err := fuzz.RunUnixSocketChallenge(fuzzer, &stfQA, jConfig.Verbose, raw_stfs)
			if err != nil {
				log.Printf("B#%.3d Unix Socket communication error: %v", i, err)
				if challengerFuzzed {
					fStat.FuzzResponseErrors++
				} else {
					fStat.OrigResponseErrors++
				}
				continue
			}

			//log.Printf("B#%.3d isMatch: %v, solverFuzzed: %v, challengerFuzzed: %v", i, isMatch, solverFuzzed, challengerFuzzed)

			switch {
			case challengerFuzzed && solverFuzzed && isMatch:
				fStat.FuzzTruePositives++
			case challengerFuzzed && solverFuzzed && !isMatch:
				fStat.FuzzMisclassifications++
			case challengerFuzzed && !solverFuzzed:
				fStat.FuzzFalseNegatives++
			case !challengerFuzzed && !solverFuzzed && isMatch:
				fStat.OrigTrueNegatives++
			case !challengerFuzzed && !solverFuzzed && !isMatch:
				fStat.OrigMisclassifications++
			case !challengerFuzzed && solverFuzzed:
				fStat.OrigFalsePositives++
			}

		} else {
			stfChallenge := stfQA.ToChallenge()
			postSTResp, respOK, _ := fuzzer.SendStateTransitionChallenge(jConfig.HTTP, stfChallenge)
			if respOK {
				solverFuzzed := postSTResp.Mutated
				isMatch, _ := fuzzer.ValidateStateTransitionChallengeResponse(&stfQA, postSTResp)
				switch {
				case challengerFuzzed && solverFuzzed && isMatch:
					fStat.FuzzTruePositives++
				case challengerFuzzed && solverFuzzed && !isMatch:
					fStat.FuzzMisclassifications++
				case challengerFuzzed && !solverFuzzed:
					fStat.FuzzFalseNegatives++
				case !challengerFuzzed && !solverFuzzed && isMatch:
					fStat.OrigTrueNegatives++
				case !challengerFuzzed && !solverFuzzed && !isMatch:
					fStat.OrigMisclassifications++
				case !challengerFuzzed && solverFuzzed:
					fStat.OrigFalsePositives++
				}
			} else {
				if challengerFuzzed {
					fStat.FuzzResponseErrors++
				} else {
					fStat.OrigResponseErrors++
				}
			}
		}

		if fStat.TotalBlocks%jConfig.Statistics == 0 {
			log.Printf("[%s Mode]\nStats:\n%s\n", mode, fStat.DumpMetrics())
		}
	}

	elapsed := time.Since(startTime).Seconds()
	log.Printf("[%s Mode] Done in %.2fs\n%s\n", mode, elapsed, fStat.DumpMetrics())
}
