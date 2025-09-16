package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

//go:embed refine/*.bin
var embeddedRefineFiles embed.FS

const (
	numBlocksMax = 650
	magicConst   = 1107
)

func validateImportBlockConfig(jConfig types.ConfigJamBlocks) {
	if jConfig.Network != "tiny" {
		log.Fatalf("Invalid --network value: %s. Must be 'tiny'.", jConfig.Network)
	}
}

func Terminate(stopCh chan os.Signal) {
	stopCh <- syscall.SIGTERM
	time.Sleep(100 * time.Millisecond)
}

func main() {

	dir := "/tmp/importBlock"
	enableRPC := false
	useUnixSocket := true
	test_dir := "./rawdata"
	report_dir := "./reports"
	refineMode := false
	shuffling := false

	version := false

	jConfig := types.ConfigJamBlocks{
		HTTP:        "http://localhost:8088/",
		Socket:      "/tmp/jam_target.sock",
		Verbose:     false,
		NumBlocks:   100,
		InvalidRate: 0,
		Statistics:  100,
		Network:     "tiny",
		PVMBackend:  statedb.BackendInterpreter,
		Seed:        "0x44554E41",
	}

	fReg := fuzz.NewFlagRegistry("importblocks")
	fReg.RegisterFlag("seed", nil, jConfig.Seed, "Seed for random number generation (as hex)", &jConfig.Seed)
	fReg.RegisterFlag("network", "n", jConfig.Network, "JAM network size", &jConfig.Network)
	fReg.RegisterFlag("verbose", nil, jConfig.Verbose, "Enable detailed logging", &jConfig.Verbose)
	fReg.RegisterFlag("numblocks", nil, jConfig.NumBlocks, "Number of blocks to generate", &jConfig.NumBlocks)
	fReg.RegisterFlag("invalidrate", nil, jConfig.InvalidRate, "Percentage of invalid blocks", &jConfig.InvalidRate)
	fReg.RegisterFlag("statistics", nil, jConfig.Statistics, "Print statistics interval", &jConfig.Statistics)
	fReg.RegisterFlag("dir", nil, dir, "Storage directory", &dir)
	fReg.RegisterFlag("test-dir", nil, test_dir, "Storage directory", &test_dir)
	fReg.RegisterFlag("report-dir", nil, report_dir, "Report directory", &report_dir)
	fReg.RegisterFlag("socket", nil, jConfig.Socket, "Path for the Unix domain socket to connect to", &jConfig.Socket)
	fReg.RegisterFlag("use-unix-socket", nil, useUnixSocket, "Enable to use Unix domain socket for communication", &useUnixSocket)
	fReg.RegisterFlag("pvm-backend", nil, jConfig.PVMBackend, "PVM backend to use (Compiler or Interpreter)", &jConfig.PVMBackend)
	fReg.RegisterFlag("refine", "r", refineMode, "Enable RefineBundle challenge testing (mutually exclusive with block testing)", &refineMode)
	fReg.RegisterFlag("shuffling", nil, shuffling, "Enable shuffling of blocks", &shuffling)
	fReg.RegisterFlag("version", "v", version, "Display version information", &version)
	fReg.ProcessRegistry()

	if !jConfig.Verbose {
		statedb.RecordTime = false
	}

	fuzzerInfo := fuzz.CreatePeerInfo("duna-fuzzer")
	fuzzerInfo.SetDefaults()

	fmt.Printf("Fuzzer Info:\n\n%s\n\n", fuzzerInfo.Info())
	if version {
		return
	}

	disableShuffling := !shuffling
	if refineMode {
		disableShuffling = true
		//log.Printf("Refinement mode: will read RefineBundle tests from embedded files")
		//log.Printf("Note: Block challenges are disabled in refinement mode")
		//log.Printf("Note: Shuffling automatically disabled for deterministic bundle processing")
	}

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopCh
		log.Println("TERMINATING FUZZER")
		os.Exit(0)
	}()

	fmt.Printf("%v\n", jConfig)
	validateImportBlockConfig(jConfig)

	fuzzer, err := fuzz.NewFuzzer(dir, report_dir, jConfig.Socket, fuzzerInfo, jConfig.PVMBackend)
	if err != nil {
		//log.Printf("Failed to initialize fuzzer: %v", err)
		Terminate(stopCh)
	}
	seed := common.FromHex(jConfig.Seed)
	fuzzer.SetSeed(seed)

	if enableRPC {
		go func() {
			log.Println("Starting RPC server...")
			fuzzer.RunImplementationRPCServer()
		}()
	}

	var raw_stfs []*statedb.StateTransition
	var usable_stfs []*statedb.StateTransition
	var bundle_tests []*fuzz.RefineBundleQA

	if refineMode {
		// Refinement mode - load bundle tests and STFs from embedded files
		bundle_tests, raw_stfs, err = fuzz.ReadEmbeddedRefineBundles(embeddedRefineFiles, jConfig.PVMBackend, nil)
		if err != nil {
			log.Printf("Failed to read embedded RefineBundleQA tests: %v", err)
			Terminate(stopCh)
		}
		if len(bundle_tests) == 0 {
			log.Printf("No RefineBundleQA test data available in embedded files. Exit!")
			return
		}
		//log.Printf("Loaded %d RefineBundleQA test cases and %d STFs from embedded refine files", len(bundle_tests), len(raw_stfs))
	} else {
		// Block challenge mode - only load state transitions from test-dir
		raw_stfs, err = fuzz.ReadStateTransitions(test_dir)
		if err != nil {
			log.Printf("Failed to read raw state transitions: %v", err)
			Terminate(stopCh)
		}

		usable_stfs = make([]*statedb.StateTransition, 0)
		for _, stf := range raw_stfs {
			if fuzz.HasParentStfs(raw_stfs, stf) {
				usable_stfs = append(usable_stfs, stf)
			} else {
				//log.Printf("Skipping STF with no parent: %s", stf.Block.Header.HeaderHash().Hex())
			}
		}

		if len(usable_stfs) == 0 {
			log.Printf("No usable state transition test data available in %v. Exit!", test_dir)
			return
		}
		log.Printf("Loaded %d usable state transitions from %s", len(usable_stfs), test_dir)
	}

	startTime := time.Now()
	numBlocks := jConfig.NumBlocks
	fStat := fuzz.FuzzStats{}
	fStat.FuzzRateTarget = jConfig.InvalidRate

	var stfChannel chan fuzz.StateTransitionQA

	if !refineMode {
		// Only start block fuzzing producer in block mode
		stfChannel = make(chan fuzz.StateTransitionQA, 10)
		go fuzz.StartFuzzingProducerWithOptions(fuzzer, stfChannel, usable_stfs, jConfig.InvalidRate, numBlocks, disableShuffling)
	}

	if useUnixSocket {
		if err := fuzzer.Connect(); err != nil {
			log.Printf("Failed to connect to target: %v", jConfig.Socket)
			Terminate(stopCh)
		}
		defer fuzzer.Close()
		peerInfo, err := fuzzer.Handshake()
		if err != nil {
			log.Printf("Handshake Failed: %v", err)
			Terminate(stopCh)
		} else {
			log.Printf("Handshake SUCC: %s", peerInfo.PrettyString(false))
			fuzzer.SetTargetPeerInfo(peerInfo)
		}
	}

	if refineMode {
		// Bundle refinement mode - process all bundle challenges
		log.Printf("[INFO] Processing %d RefineBundleQA challenges...", len(bundle_tests))
		for _, bundleQA := range bundle_tests {
			time.Sleep(50 * time.Millisecond)

			challengerFuzzed := bundleQA.Mutated
			fStat.TotalBlocks++
			if challengerFuzzed {
				fStat.FuzzedBlocks++
			} else {
				fStat.OriginalBlocks++
			}

			if useUnixSocket {
				isMatch, solverFuzzed, err := fuzz.RunRefineBundleChallenge(fuzzer, bundleQA, jConfig.Verbose)
				if err != nil {
					// Check if this is a WorkReport mismatch error - if so, terminate
					if strings.Contains(err.Error(), "WorkReport mismatch") {
						log.Printf("❌ ERROR: %v", err)
						Terminate(stopCh)
						return
					}

					log.Printf("Bundle refinement error: %v", err)
					if challengerFuzzed {
						fStat.FuzzResponseErrors++
					} else {
						fStat.OrigResponseErrors++
					}
					continue
				}

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
			}

			if fStat.TotalBlocks%jConfig.Statistics == 0 {
				log.Printf("Bundle Stats:\n%s\n", fStat.DumpMetrics())
			}
		}
		log.Println("[INFO] Completed RefineBundleQA challenges")
	} else {
		// Block challenge mode - process state transitions from producer
		log.Println("[INFO] Consumer: Starting to process blocks from producer...")

		for stfQA := range stfChannel {
			time.Sleep(50 * time.Millisecond)

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
					//log.Printf("Unix Socket Err: %v", err)
					if strings.Contains(err.Error(), "broken pipe") {
						log.Println("Target connection lost (broken pipe)")
						Terminate(stopCh)
						return
					}
					if challengerFuzzed {
						fStat.FuzzResponseErrors++
					} else {
						fStat.OrigResponseErrors++
					}
					continue
				}

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
				//log.Printf("Stats:\n%s\n", fStat.DumpMetrics())
			}
		}
	}

	elapsed := time.Since(startTime).Seconds()
	log.Printf("Done in %.2fs\n%s\n", elapsed, fStat.DumpMetrics())
}
