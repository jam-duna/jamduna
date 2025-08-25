package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
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
	fmt.Printf("fuzzer - JAM Duna %s fuzzer\n", fuzz.FUZZ_VERSION)

	dir := "/tmp/importBlock"
	enableRPC := false
	useUnixSocket := true
	test_dir := "./rawdata"
	report_dir := "./reports"

	jConfig := types.ConfigJamBlocks{
		HTTP:        "http://localhost:8088/",
		Socket:      "/tmp/jam_target.sock",
		Verbose:     false,
		NumBlocks:   50,
		InvalidRate: 0,
		Statistics:  10,
		Network:     "tiny",
		PVMBackend:  pvm.BackendInterpreter,
		Seed:        "0x44554E41",
	}

	fReg := fuzz.NewFlagRegistry("importblocks")
	fReg.RegisterFlag("seed", nil, jConfig.Seed, "Seed for random number generation (as hex)", &jConfig.Seed)
	fReg.RegisterFlag("network", "n", jConfig.Network, "JAM network size", &jConfig.Network)
	fReg.RegisterFlag("verbose", "v", jConfig.Verbose, "Enable detailed logging", &jConfig.Verbose)
	fReg.RegisterFlag("numblocks", nil, jConfig.NumBlocks, "Number of blocks to generate", &jConfig.NumBlocks)
	fReg.RegisterFlag("invalidrate", nil, jConfig.InvalidRate, "Percentage of invalid blocks", &jConfig.InvalidRate)
	fReg.RegisterFlag("statistics", nil, jConfig.Statistics, "Print statistics interval", &jConfig.Statistics)
	fReg.RegisterFlag("dir", nil, dir, "Storage directory", &dir)
	fReg.RegisterFlag("test-dir", nil, test_dir, "Storage directory", &test_dir)
	fReg.RegisterFlag("report-dir", nil, report_dir, "Report directory", &report_dir)
	fReg.RegisterFlag("socket", nil, jConfig.Socket, "Path for the Unix domain socket to connect to", &jConfig.Socket)
	fReg.RegisterFlag("use-unix-socket", nil, useUnixSocket, "Enable to use Unix domain socket for communication", &useUnixSocket)
	fReg.RegisterFlag("pvm-backend", nil, jConfig.PVMBackend, "PVM backend to use (Recompiler or Interpreter)", &jConfig.PVMBackend)
	fReg.ProcessRegistry()
	fmt.Printf("%v\n", jConfig)

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopCh
		log.Println("TERMINATING FUZZER")
		os.Exit(0)
	}()

	validateImportBlockConfig(jConfig)
	fuzzerInfo := fuzz.PeerInfo{
		Name:       fmt.Sprintf("jam-duna-fuzzer-%s", fuzz.FUZZ_VERSION),
		AppVersion: fuzz.Version{Major: 0, Minor: 7, Patch: 0},
		JamVersion: fuzz.Version{Major: 0, Minor: 7, Patch: 0},
	}
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

	raw_stfs, err := fuzz.ReadStateTransitions(test_dir)
	if err != nil {
		log.Printf("Failed to read raw state transitions: %v", err)
		Terminate(stopCh)
	}

	usable_stfs := make([]*statedb.StateTransition, 0)
	for _, stf := range raw_stfs {
		if fuzz.HasParentStfs(raw_stfs, stf) {
			usable_stfs = append(usable_stfs, stf)
		} else {
			//log.Printf("Skipping STF with no parent: %s", stf.Block.Header.HeaderHash().Hex())
		}
	}

	if len(usable_stfs) == 0 {
		log.Printf("No test data available on BaseDir=%v. Exit!", test_dir)
		return
	}

	startTime := time.Now()
	numBlocks := jConfig.NumBlocks
	fStat := fuzz.FuzzStats{}
	fStat.FuzzRateTarget = jConfig.InvalidRate

	stfChannel := make(chan fuzz.StateTransitionQA, 10)

	go fuzz.StartFuzzingProducer(fuzzer, stfChannel, usable_stfs, jConfig.InvalidRate, numBlocks)

	if useUnixSocket {
		if err := fuzzer.Connect(); err != nil {
			log.Printf("Failed to connect to target: %v", jConfig.Socket)
			Terminate(stopCh)
		}
		defer fuzzer.Close()
		peerInfo, err := fuzzer.Handshake()
		if err != nil {
			log.Printf("Handshake failed: %v", err)
			Terminate(stopCh)
		} else {
			log.Printf("Handshake successful: %v", peerInfo)
			fuzzer.SetTargetPeerInfo(*peerInfo)
		}
	}

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
			log.Printf("Stats:\n%s\n", fStat.DumpMetrics())
		}
	}

	elapsed := time.Since(startTime).Seconds()
	log.Printf("Done in %.2fs\n%s\n", elapsed, fStat.DumpMetrics())
}
