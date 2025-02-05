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
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
	//"github.com/mkchungs/flags"
)

const (
	numBlocksMax = 650
	magicConst   = 1107
)

func validateImportBlockConfig(jConfig types.ConfigJamBlocks) {
	//fmt.Printf("ImportBlockConfig %s\n", jConfig)
	if jConfig.HTTP == "" && jConfig.QUIC == "" {
		log.Fatalf("You must specify either an HTTP URL or a QUIC address. Use --http or --quic.")
	}
	if jConfig.Network != "tiny" && jConfig.Network != "full" {
		log.Fatalf("Invalid --network value: %s. Must be 'tiny' or 'full'.", jConfig.Network)
	}
	_, modeErr := fuzz.CheckModes(jConfig.Mode)
	if modeErr != nil {
		log.Fatalf(modeErr.Error())
	}
	if jConfig.NumBlocks < 1 || (jConfig.NumBlocks > numBlocksMax) {
		if jConfig.NumBlocks != magicConst {
			log.Fatalf("--numblocks must be between 1 and 600 (got %d)", jConfig.NumBlocks)
		}
	}
	if jConfig.Statistics <= 0 {
		log.Fatalf("--statistics must be a positive integer")
	}
}

func main() {

	fmt.Println("importblocks - JAM Duna Import Blocks generator")

	dir := "/tmp/importBlock"
	enableRPC := false

	jConfig := types.ConfigJamBlocks{
		Mode:        "assurances",
		HTTP:        "http://localhost:8088/",
		QUIC:        "",
		Verbose:     false,
		NumBlocks:   100,
		InvalidRate: 0.14285,
		Statistics:  20,
		Network:     "tiny",
	}

	fReg := fuzz.NewFlagRegistry("importblocks")
	fReg.RegisterFlag("mode", "m", jConfig.Mode, "Block generation mode", &jConfig.Mode)
	fReg.RegisterFlag("http", "h", jConfig.HTTP, "HTTP endpoint to send blocks", &jConfig.HTTP)
	fReg.RegisterFlag("quic", "q", jConfig.QUIC, "QUIC endpoint to send blocks", &jConfig.QUIC)
	fReg.RegisterFlag("network", "n", jConfig.Network, "JAM network size", &jConfig.Network)
	fReg.RegisterFlag("verbose", "v", jConfig.Verbose, "Enable detailed logging", &jConfig.Verbose)
	fReg.RegisterFlag("numblocks", nil, jConfig.NumBlocks, "Number of blocks to generate", &jConfig.NumBlocks)
	fReg.RegisterFlag("invalidrate", nil, jConfig.InvalidRate, "Percentage of invalid blocks", &jConfig.InvalidRate)
	fReg.RegisterFlag("statistics", nil, jConfig.Statistics, "Print statistics interval", &jConfig.Statistics)
	fReg.RegisterFlag("dir", nil, dir, "Storage directory", &dir)
	//fReg.RegisterFlag("rpc", nil, enableRPC, "Start RPC server", &enableRPC)
	//fReg.RegisterFlag("genesis", nil, genesis, "Initial Genesis State", &genesis) // not planned
	fReg.ProcessRegistry()
	fmt.Printf("%v\n", jConfig)

	validateImportBlockConfig(jConfig)

	fuzzer, err := fuzz.NewFuzzer(dir)
	if err != nil {
		log.Fatalf("Failed to initialize fuzzer: %v", err)
	}

	if enableRPC && false {
		go func() {
			log.Println("Starting RPC server...")
			fuzzer.RunImplementationRPCServer()
		}()
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	startTime := time.Now()
	numBlocks := jConfig.NumBlocks
	nonStopFlagSet := jConfig.NumBlocks == magicConst
	mode := jConfig.Mode

	log.Printf("[INFO] Starting block generation: mode=%s, numBlocks=%d, dir=%s\n", mode, numBlocks, dir)

	baseDir := os.Getenv("TEST_DATA_DIR")
	if baseDir == "" {
		baseDir = "./"
	}
	stfs, err := fuzz.ReadStateTransitions(baseDir, mode)
	if err != nil || len(stfs) == 0 {
		log.Printf("No %v mode data avaialbe. Exit!", mode)
		return
	}

	fmt.Printf("Fuzz ready! sources=%v\n", len(stfs))
	stfTestBank, fuzzErr := fuzzer.FuzzWithTargetedInvalidRate([]string{mode}, stfs, jConfig.InvalidRate, numBlocks)
	if fuzzErr != nil {
		log.Fatal(fuzzErr)
	}

	mutatedBlks := make([]common.Hash, 0)
	originalBlks := make([]common.Hash, 0)

	fStat := fuzz.FuzzStats{}

	for i := 0; i < numBlocks || nonStopFlagSet; i++ {
		select {
		case <-stopCh:
			log.Println("Received interrupt, stopping early.")
			goto finish
		default:
			time.Sleep(50 * time.Millisecond)
			if i >= len(stfTestBank) {
				log.Println("No more state transitions available.")
				break
			}
			stfQA := stfTestBank[i]
			identifierHash := stfQA.STF.Block.Hash()
			challengerFuzzed := stfQA.Mutated // Challenger's belief.

			// Basic Metrics
			fStat.TotalBlocks++
			if challengerFuzzed {
				fStat.FuzzedBlocks++
			} else {
				fStat.OriginalBlocks++
			}

			if challengerFuzzed {
				log.Printf("B#%.3d Fuzzed: %v\n", i, jamerrors.GetErrorStr(stfQA.Error))
				mutatedBlks = append(mutatedBlks, identifierHash)
			} else {
				//log.Printf("B#%.3d Original\n", i)
				originalBlks = append(originalBlks, identifierHash)
			}

			// Send challenge to HTTP endpoint.
			/*
				FuzzTruePositives      int `json:"fuzz_true_positives"`     // Fuzzed blocks correctly detected.
				FuzzFalseNegatives     int `json:"fuzz_false_negatives"`    // Fuzzed blocks that were missed.
				FuzzResponseErrors     int `json:"fuzz_response_errors"`    // Response errors in fuzzed blocks.
				FuzzMisclassifications int `json:"fuzz_misclassifications"` // Fuzzed blocks misclassified.
				OrigFalsePositives     int `json:"orig_false_positives"`    // Original blocks wrongly flagged.
				OrigTrueNegatives      int `json:"orig_true_negatives"`     // Original blocks correctly validated.
				OrigResponseErrors     int `json:"orig_response_errors"`    // Response errors in original blocks.
				OrigMisclassifications int `json:"orig_misclassifications"` // Original blocks misclassified.
			*/
			stfChallenge := stfQA.ToChallenge()
			postSTResp, respOK, _ := fuzzer.SendStateTransitionChallenge(jConfig.HTTP, stfChallenge)
			if respOK {
				solverFuzzed := postSTResp.Mutated // Solver's belief.
				isMatch, validationErr := fuzzer.ValidateStateTransitionChallengeResponse(&stfQA, postSTResp)
				if jConfig.Verbose {
					log.Printf("B#%.3d respOK. isMatch:%v. vErr:%v\n", i, isMatch, validationErr)
				}
				switch {
				case challengerFuzzed && solverFuzzed && isMatch:
					// Fuzzed blocks correctly detected.
					fStat.FuzzTruePositives++
				case challengerFuzzed && solverFuzzed && !isMatch:
					// Fuzzed blocks misclassified.
					fStat.FuzzMisclassifications++
				case challengerFuzzed && !solverFuzzed:
					// Fuzzed blocks that were missed.
					fStat.FuzzFalseNegatives++
				case !challengerFuzzed && !solverFuzzed && isMatch:
					// Original blocks correctly validated.
					fStat.OrigTrueNegatives++
				case !challengerFuzzed && !solverFuzzed && !isMatch:
					// Original blocks misclassified.
					fStat.OrigMisclassifications++
				case !challengerFuzzed && solverFuzzed:
					// Original blocks wrongly flagged.
					fStat.OrigFalsePositives++
				}
			} else {
				// Handle response errors.
				if challengerFuzzed {
					fStat.FuzzResponseErrors++
				} else {
					fStat.OrigResponseErrors++
				}
			}

			if fStat.TotalBlocks%jConfig.Statistics == 0 {
				log.Printf("[%s Mode]\nStats:\n%s\n", mode, fStat.DumpMetrics())
			}
		}
	}

finish:
	elapsed := time.Since(startTime).Seconds()
	log.Printf("[%s Mode] Done in %.2fs\n%s\n", mode, elapsed, fStat.DumpMetrics())
}
