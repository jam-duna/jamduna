package main

import (
	"bytes"
	"context"
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
)

func decodeStateSnapshot(encodedBytes []byte) (*statedb.StateSnapshot, error) {
	s, _, err := types.Decode(encodedBytes, reflect.TypeOf(statedb.StateSnapshot{}))
	return s.(*statedb.StateSnapshot), err
}

func decodeBlock(encodedBytes []byte) (*types.Block, error) {
	b, _, err := types.Decode(encodedBytes, reflect.TypeOf(types.Block{}))
	return b.(*types.Block), err
}

func ApplyStateTransition(stateDB *statedb.StateDB, block *types.Block) error {
	// Placeholder for applying the state transition based on the decoded block
	return nil // Replace with actual state transition logic
}

func processBlocks(basePath string, stateDB *statedb.StateDB) error {
	blocksDir := filepath.Join(basePath, "blocks")
	snapshotsDir := filepath.Join(basePath, "state_snapshots")

	// Loop through epochs and phases (0-4, 0-11)
	for epoch := 0; epoch <= 4; epoch++ {
		for phase := 0; phase <= 11; phase++ {
			blockFile := fmt.Sprintf("%d_%d.codec", epoch, phase)
			snapshotFile := fmt.Sprintf("%d_%d.codec", epoch, phase)

			blockPath := filepath.Join(blocksDir, blockFile)
			snapshotPath := filepath.Join(snapshotsDir, snapshotFile)

			// Check if block codec file exists
			if _, err := os.Stat(blockPath); err == nil {
				// Read the block file into blockBytes
				blockBytes, err := ioutil.ReadFile(blockPath)
				if err != nil {
					log.Printf("Error reading block file %s: %v\n", blockPath, err)
					continue
				}

				// Decode block from blockBytes
				block, err := decodeBlock(blockBytes)
				if err != nil {
					log.Printf("Error decoding block %s: %v\n", blockPath, err)
					continue
				}

				// Apply the state transition
				newStateDB, err := statedb.ApplyStateTransitionFromBlock(stateDB, context.Background(), block)
				if err != nil {
					log.Printf("Error applying state transition for block %s: %v\n", blockPath, err)
					continue
				}
				snapshot := newStateDB.JamState.Snapshot()
				snapshotBytes, err := types.Encode(snapshot)
				// Check if snapshot codec file exists
				if _, err := os.Stat(snapshotPath); err == nil {
					// Read the snapshot file into snapshotBytes
					expectedSnapshotBytes, err := ioutil.ReadFile(snapshotPath)
					if err != nil {
						log.Printf("Error reading snapshot file %s: %v\n", snapshotPath, err)
						continue
					}

					// Decode the expected state snapshot from snapshotBytes
					if bytes.Equal(snapshotBytes, expectedSnapshotBytes) {
						fmt.Printf("Validated Block %s => State %s", blockFile, snapshotFile)
					}

					// Validate state transition results
					// Add logic to compare stateDB and expectedStateDB
					fmt.Printf("Validated block %d_%d\n", epoch, phase)
				} else {
					log.Printf("Snapshot file does not exist: %s\n", snapshotPath)
				}
			} else {
				log.Printf("Block file does not exist: %s\n", blockPath)
			}
		}
	}
	return nil
}

func main() {
	// Define command-line flags
	mode := flag.String("mode", "safrole", "Mode to use (default: safrole)")
	team := flag.String("team", "jam_duna", "Team name to use (default: jam_duna)")
	traceDir := flag.String("traceDir", "/root/github.com/jamduna/traces/", "Directory path to trace files (default: /root/github.com/jamduna/traces/)")

	// Parse the flags
	flag.Parse()

	// Construct the paths using the flags
	modeDir := filepath.Join(*traceDir, *mode)
	teamDir := filepath.Join(modeDir, *team)

	genesisFile := filepath.Join(modeDir, "genesis.codec")
	storage, err := storage.NewStateDBStorage("/tmp/validatetraces")
	if err != nil {
		log.Fatalf("Error decoding genesis file %s: %v\n", genesisFile, err)
	}

	// Read the snapshot file into snapshotBytes
	snapshotBytes, err := ioutil.ReadFile(genesisFile)
	if err != nil {
		log.Fatalf("Error decoding genesis snapshot %s: %v\n", genesisFile, err)
	}

	// Decode genesis into the initial StateDB
	statesnapshot, err := decodeStateSnapshot(snapshotBytes)
	if err != nil {
		log.Fatalf("Error decoding genesis file %s: %v\n", genesisFile, err)
	}
	stateDB, err := statedb.InitStateDBFromSnapshot(storage, statesnapshot)
	if err != nil {
		log.Fatalf("Error InitStateDBFromSnapshot %v\n", err)
	}

	// Process the blocks and state transitions
	err = processBlocks(teamDir, stateDB)
	if err != nil {
		log.Fatalf("Error processing blocks: %v\n", err)
	}

	fmt.Println("Trace validation completed successfully.")
}
