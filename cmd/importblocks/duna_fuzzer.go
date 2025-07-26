package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

const (
	numBlocksMax = 650
	magicConst   = 1107
)

// runUnixSocketChallenge executes a state transition test over an existing connection.
func runUnixSocketChallenge(fuzzer *fuzz.Fuzzer, stfQA *fuzz.StateTransitionQA, verbose bool) (matched bool, err error) {
	initialStatePayload := &fuzz.HeaderWithState{
		Header: stfQA.STF.Block.Header,
		State:  statedb.StateKeyVals{KeyVals: stfQA.STF.PreState.KeyVals},
	}
	expectedPreStateRoot := stfQA.STF.PreState.StateRoot

	targetPreStateRoot, err := fuzzer.SetState(initialStatePayload)
	if err != nil {
		return false, fmt.Errorf("SetState failed: %w", err)
	}

	// Verification for pre-state.
	if !bytes.Equal(targetPreStateRoot.Bytes(), expectedPreStateRoot.Bytes()) {
		log.Printf("FATAL: Pre-state root MISMATCH!\n  Got:  %s\n  Want: %s", targetPreStateRoot.Hex(), expectedPreStateRoot.Hex())
		return false, nil // Mismatch is a valid test outcome, not an error.
	}

	blockToProcess := &stfQA.STF.Block
	headerHash := blockToProcess.Header.HeaderHash()
	expectedPostStateRoot := stfQA.STF.PostState.StateRoot

	targetPostStateRoot, err := fuzzer.ImportBlock(blockToProcess)
	if err != nil {
		return false, fmt.Errorf("ImportBlock failed: %w", err)
	}

	if stfQA.Mutated {
		log.Printf("B#%.3d Fuzzed: %v\n", stfQA.STF.Block.Header.Slot, jamerrors.GetErrorName(stfQA.Error))
		// fuzzed blocks should return as 0x0000....0000
		if *targetPostStateRoot == (common.Hash{}) {
			log.Printf("B#%.3d Fuzzed block returned expected zero post-state root.", stfQA.STF.Block.Header.Slot)
			matched = true
		} else {
			log.Printf("FATAL: Fuzzed block returned non-zero post-state root: %s", targetPostStateRoot.Hex())
			matched = false // Fuzzed Undetected
		}
	} else {
		log.Printf("B#%.3d Original: %v\n", stfQA.STF.Block.Header.Slot, jamerrors.GetErrorName(stfQA.Error))
		// Unfuzzed. Do verification for post-state.
		if bytes.Equal(targetPostStateRoot.Bytes(), expectedPostStateRoot.Bytes()) {
			matched = true
			log.Printf("B#%.3d Original block returned expected post-state root: %s", blockToProcess.Header.Slot, targetPostStateRoot.Hex())
		} else {
			log.Printf("FATAL: Post-state root MISMATCH!\n  Got:  %s\n  Want: %s", targetPostStateRoot.Hex(), expectedPostStateRoot.Hex())
			matched = false // Post-state root MISMATCH
		}
	}

	if !matched || verbose {
		// Log the mismatch for further analysis.
		if !matched {
			log.Printf("B#%.3d MISMATCH: HeaderHash: %s | PostStateRoot: %s", blockToProcess.Header.Slot, headerHash.Hex(), targetPostStateRoot.Hex())
		} else {
			log.Printf("B#%.3d MATCH: HeaderHash: %s | PostStateRoot: %s", blockToProcess.Header.Slot, headerHash.Hex(), targetPostStateRoot.Hex())
		}
		targetStateKeyVals, err := fuzzer.GetState(&headerHash) // Fetch the state for debugging.
		if err != nil {
			log.Printf("Error fetching state for headerHash %s: %v", headerHash.Hex(), err)
		} else if targetStateKeyVals == nil {
			log.Printf("No state found for headerHash %s", headerHash.Hex())
		} else {
			executionReport := fuzzer.GenerateExecutionReport(stfQA, *targetStateKeyVals)
			log.Printf("Execution Report for B#%.3d:\n%s", blockToProcess.Header.Slot, executionReport.String())
		}
	}

	return matched, nil
}

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
	socket := "/tmp/jam_target.sock"
	enableRPC := false
	useUnixSocket := true
	seedHex := "0x44554E41"

	jConfig := types.ConfigJamBlocks{
		Mode:        "safrole",
		HTTP:        "http://localhost:8088/",
		QUIC:        "",
		Verbose:     false,
		NumBlocks:   50,
		InvalidRate: 0.14285,
		Statistics:  20,
		Network:     "tiny",
	}

	fReg := fuzz.NewFlagRegistry("importblocks")
	fReg.RegisterFlag("seed", nil, seedHex, "Seed for random number generation (as hex)", &seedHex)
	fReg.RegisterFlag("mode", "m", jConfig.Mode, "Block generation mode", &jConfig.Mode)
	fReg.RegisterFlag("http", "h", jConfig.HTTP, "HTTP endpoint to send blocks", &jConfig.HTTP)
	fReg.RegisterFlag("quic", "q", jConfig.QUIC, "QUIC endpoint to send blocks", &jConfig.QUIC)
	fReg.RegisterFlag("network", "n", jConfig.Network, "JAM network size", &jConfig.Network)
	fReg.RegisterFlag("verbose", "v", jConfig.Verbose, "Enable detailed logging", &jConfig.Verbose)
	fReg.RegisterFlag("numblocks", nil, jConfig.NumBlocks, "Number of blocks to generate", &jConfig.NumBlocks)
	fReg.RegisterFlag("invalidrate", nil, jConfig.InvalidRate, "Percentage of invalid blocks", &jConfig.InvalidRate)
	fReg.RegisterFlag("statistics", nil, jConfig.Statistics, "Print statistics interval", &jConfig.Statistics)
	fReg.RegisterFlag("dir", nil, dir, "Storage directory", &dir)
	fReg.RegisterFlag("socket", nil, socket, "Path for the Unix domain socket to connect to", &socket)
	fReg.RegisterFlag("use-unix-socket", nil, useUnixSocket, "Enable to use Unix domain socket for communication", &useUnixSocket)
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
		Name:       "jam-duna-fuzzer-v0.1",
		AppVersion: fuzz.Version{Major: 0, Minor: 6, Patch: 7},
		JamVersion: fuzz.Version{Major: 0, Minor: 6, Patch: 7},
	}
	fuzzer, err := fuzz.NewFuzzer(dir, socket, fuzzerInfo)
	if err != nil {
		log.Fatalf("Failed to initialize fuzzer: %v", err)
	}
	seed := common.FromHex(seedHex)
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

	baseDir := os.Getenv("TEST_DATA_DIR")
	if baseDir == "" {
		baseDir = "./rawdata"
	}
	stfs, err := fuzz.ReadStateTransitions(baseDir, mode)
	if err != nil || len(stfs) == 0 {
		log.Printf("No %v mode data available on BaseDir=%v. Exit!", mode, baseDir)
		return
	}

	stfTestBank, fuzzErr := fuzzer.FuzzWithTargetedInvalidRate([]string{mode}, stfs, jConfig.InvalidRate, numBlocks)
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

		if challengerFuzzed {
			log.Printf("B#%.3d Fuzzed: %v\n", i, jamerrors.GetErrorName(stfQA.Error))
		}

		if useUnixSocket {
			isMatch, err := runUnixSocketChallenge(fuzzer, &stfQA, jConfig.Verbose)
			if err != nil {
				log.Printf("B#%.3d Unix Socket communication error: %v", i, err)
				if challengerFuzzed {
					fStat.FuzzResponseErrors++
				} else {
					fStat.OrigResponseErrors++
				}
				continue
			}

			if challengerFuzzed && isMatch {
				fStat.FuzzFalseNegatives++
			} else if challengerFuzzed && !isMatch {
				fStat.FuzzTruePositives++
			} else if !challengerFuzzed && isMatch {
				fStat.OrigTrueNegatives++
			} else if !challengerFuzzed && !isMatch {
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
