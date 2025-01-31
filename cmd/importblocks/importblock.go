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
		HTTP:        "http://localhost:8088/fuzz_json",
		QUIC:        "",
		Verbose:     false,
		NumBlocks:   500,
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
	fReg.ProcessRegistry()
	fmt.Printf("%v\n", jConfig)

	validateImportBlockConfig(jConfig)

	fuzzer, err := fuzz.NewFuzzer(dir)
	if err != nil {
		log.Fatalf("Failed to initialize fuzzer: %v", err)
	}

	if enableRPC {
		go func() {
			log.Println("Starting RPC server...")
			fuzzer.RunRPCServer()
		}()
	}

	// Handle interrupts for graceful shutdown
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	startTime := time.Now()
	generatedBlockCnt := 0
	numBlocks := jConfig.NumBlocks
	nonStopFlagSet := jConfig.NumBlocks == magicConst
	mode := jConfig.Mode

	log.Printf("[INFO] Starting block generation: mode=%s, numBlocks=%d, dir=%s\n", mode, numBlocks, dir)

	// load stf
	baseDir := "./"
	stfs, err := fuzz.ReadStateTransitions(baseDir, mode)
	if err != nil || len(stfs) == 0 {
		log.Printf("No %v mode data avaialbe. Exit!", mode)
		return
	}

	fmt.Printf("Fuzz ready! sources=%v\n", len(stfs))
	fuzzAnswer, fuzzErr := fuzzer.FuzzWithTargetedInvalidRate([]string{mode}, stfs, jConfig.InvalidRate, numBlocks)
	if fuzzErr != nil {
		log.Fatal(fuzzErr)
	}

	mutatedBlks := make([]common.Hash, 0)
	originalBlks := make([]common.Hash, 0)

	for i := 0; i < numBlocks || nonStopFlagSet; i++ {
		select {
		case <-stopCh:
			log.Println("Received interrupt, stopping early.")
			goto finish
		default:
			time.Sleep(50 * time.Millisecond)
			if numBlocks >= generatedBlockCnt {
				stfAns := fuzzAnswer[generatedBlockCnt]
				identifierHash := stfAns.STF.Block.Hash()
				//TODO: Send to client's RPC or Quic
				if stfAns.Mutated {
					log.Printf("B#%.3d Fuzzed: %v\n", i, jamerrors.GetErrorStr(stfAns.Error))
					mutatedBlks = append(mutatedBlks, identifierHash)
				} else {
					//log.Printf("B#%.3d Original\n", i)
					originalBlks = append(originalBlks, identifierHash)
				}
				generatedBlockCnt++
				if generatedBlockCnt%jConfig.Statistics == 0 {
					validCnt := len(originalBlks)
					invalidCnt := len(mutatedBlks)
					actualInvalidRate := float64(invalidCnt) / float64(validCnt+invalidCnt)
					log.Printf("[Stats] Generated %d blocks. Fuzzed=%v Original=%v InvalidRate=%f (mode=%s)", generatedBlockCnt, validCnt, invalidCnt, actualInvalidRate, mode)
				}
			}
		}
	}

finish:
	elapsed := time.Since(startTime).Seconds()
	validCnt := len(originalBlks)
	invalidCnt := len(mutatedBlks)
	actualInvalidRate := float64(invalidCnt) / float64(validCnt+invalidCnt)
	log.Printf("Done in %.2fs. Generated %d blocks Fuzzed=%v Original=%v, InvalidRate=%f (mode=%s)\n", elapsed, generatedBlockCnt, validCnt, invalidCnt, actualInvalidRate, mode)
}
