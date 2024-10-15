package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func processBlocks(genesisFile string, basePath string) error {
	storage, err := storage.NewStateDBStorage("/tmp/validatetraces")
	if err != nil {
		log.Fatalf("Error decoding genesis file %s: %v\n", genesisFile, err)
	}

	// Read the snapshot file into snapshotBytes
	fmt.Printf("reading genesis snapshot: %s\n", genesisFile)
	snapshotBytes, err := ioutil.ReadFile(genesisFile)
	if err != nil {
		log.Fatalf("Error decoding genesis snapshot %s: %v\n", genesisFile, err)
	}
	// fmt.Printf("%x\n", snapshotBytes);
	// Decode genesis into the initial StateDB
	var statesnapshot *statedb.StateSnapshot
	if strings.Contains(genesisFile, ".bin") {
		s, _, err := types.Decode(snapshotBytes, reflect.TypeOf(statedb.StateSnapshot{}))
		if err != nil {
			log.Fatalf("Error decoding genesis file %s: %v\n", genesisFile, err)
		}
		st := s.(statedb.StateSnapshot)
		statesnapshot = &st
	} else {
		// JSON unmarshal snapshotBytes into statesnapshot
		err := json.Unmarshal(snapshotBytes, &statesnapshot)
		if err != nil {
			log.Fatalf("Error unmarshaling genesis JSON file %s: %v\n", genesisFile, err)
		}
	}

	stateDB, err := statedb.InitStateDBFromSnapshot(storage, statesnapshot)
	if err != nil {
		log.Fatalf("Error InitStateDBFromSnapshot %v\n", err)
	}
	blocksDir := filepath.Join(basePath, "blocks")
	snapshotsDir := filepath.Join(basePath, "state_snapshots")

	blocks := make(map[int]map[int]types.Block)

	// Find all block files in blocksDir
	blockFiles, err := ioutil.ReadDir(blocksDir)
	if err != nil {
		return fmt.Errorf("failed to read blocks directory: %v", err)
	}

	// Process all block files ending with `.bin`
	for _, file := range blockFiles {
		if strings.HasSuffix(file.Name(), ".bin") {
			// Extract epoch and phase from filename `${epoch}_${phase}.bin`
			parts := strings.Split(strings.TrimSuffix(file.Name(), ".bin"), "_")
			if len(parts) != 2 {
				log.Printf("Invalid block filename format: %s\n", file.Name())
				continue
			}

			epoch, err := strconv.Atoi(parts[0])
			if err != nil {
				log.Printf("Invalid epoch in filename: %s\n", file.Name())
				continue
			}

			phase, err := strconv.Atoi(parts[1])
			if err != nil {
				log.Printf("Invalid phase in filename: %s\n", file.Name())
				continue
			}

			// Read the block file
			blockPath := filepath.Join(blocksDir, file.Name())
			blockBytes, err := ioutil.ReadFile(blockPath)
			if err != nil {
				log.Printf("Error reading block file %s: %v\n", blockPath, err)
				continue
			}

			// Decode block from blockBytes
			b, _, err := types.Decode(blockBytes, reflect.TypeOf(types.Block{}))
			if err != nil {
				log.Printf("Error decoding block %s: %v\n", blockPath, err)
				continue
			}
			block := b.(types.Block)

			// Store the block in the blocks map
			if blocks[epoch] == nil {
				blocks[epoch] = make(map[int]types.Block)
			}
			blocks[epoch][phase] = block
		}
	}

	// Sort epochs and phases to process in order
	var epochs []int
	for epoch := range blocks {
		epochs = append(epochs, epoch)
	}
	sort.Ints(epochs)

	// Iterate through epochs and phases in order
	for _, epoch := range epochs {
		var phases []int
		for phase := range blocks[epoch] {
			phases = append(phases, phase)
		}
		sort.Ints(phases)

		for _, phase := range phases {
			block := blocks[epoch][phase]
			blockFile := fmt.Sprintf("%d_%d.bin", epoch, phase)
			snapshotFile := fmt.Sprintf("%d_%d.bin", epoch, phase)
			snapshotPath := filepath.Join(snapshotsDir, snapshotFile)

			// Apply the state transition
			newStateDB, err := statedb.ApplyStateTransitionFromBlock(stateDB, context.Background(), &block)
			if err != nil {
				log.Printf("Error applying state transition for block %s: %v\n", blockFile, err)
				continue
			}

			snapshot := newStateDB.JamState.Snapshot()
			snapshotBytes, err := types.Encode(snapshot)

			// Check if the corresponding snapshot file exists
			if _, err := os.Stat(snapshotPath); err == nil {
				// Read the snapshot file into expectedSnapshotBytes
				expectedSnapshotBytes, err := ioutil.ReadFile(snapshotPath)
				if err != nil {
					log.Printf("Error reading snapshot file %s: %v\n", snapshotPath, err)
					continue
				}

				// Validate the snapshot
				if bytes.Equal(snapshotBytes, expectedSnapshotBytes) {
					fmt.Printf("VALIDATED Block %s => State %s\n", blockFile, snapshotFile)
					stateDB = newStateDB
				} else {
					log.Printf("Validation failed for Block %s => State %s\n", blockFile, snapshotFile)
					panic(1)
				}
			} else {
				log.Printf("Snapshot file not found for %s\n", snapshotFile)
				panic(fmt.Sprintf("Missing snapshot file: %s", snapshotFile))
			}
		}
	}
	return nil
}

func main() {
	// Define command-line flags
	mode := flag.String("mode", "safrole", "Mode to use (default: safrole)")
	team := flag.String("team", "jam-duna", "Team name to use (default: jam-duna)")
	traceDir := flag.String("traceDir", "/root/go/src/github.com/jam-duna/jamtestnet/traces", "Directory path to trace files")

	// Parse the flags
	flag.Parse()

	// Construct the paths using the flags
	modeDir := filepath.Join(*traceDir, *mode)
	teamDir := filepath.Join(modeDir, *team)
	genesisFile := filepath.Join(modeDir, "genesis.json")

	// Process the blocks and state transitions
	err := processBlocks(genesisFile, teamDir)
	if err != nil {
		log.Fatalf("Error processing blocks: %v\n", err)
	}

	fmt.Println("Trace validation completed successfully.")
}
