package main

import (
	//"bytes"
	"context"
	//"encoding/json"
	//"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

func processBlocks(basePath string) error {
	storage, err := storage.NewStateDBStorage("/tmp/validatetraces2")
	validators := make([]types.Validator, types.TotalValidators)
	genesisConfig := statedb.NewGenesisConfig(validators)
	stateDB, err := statedb.NewGenesisStateDB(storage, &genesisConfig)
	blocksDir := filepath.Join(basePath, "Blocks")
	blocks := make(map[int]map[int]types.Block)

	// Find all block files in blocksDir
	blockFiles, err := os.ReadDir(blocksDir)
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
			blockBytes, err := os.ReadFile(blockPath)
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

			// Apply the state transition
			newStateDB, err := statedb.ApplyStateTransitionFromBlock(stateDB, context.Background(), &block)
			if err != nil {
				log.Printf("Error applying state transition for block %s: %v\n", blockFile, err)
				continue
			}

			fmt.Printf("%v Block.ParentStateRoot: %v statedb.StateRoot: %v\n", blockFile, block.Header.ParentStateRoot, newStateDB.StateRoot)
		}
	}
	return nil
}

func main() {
	// Process the blocks and state transitions
	err := processBlocks(".")
	if err != nil {
		log.Fatalf("Error processing blocks: %v\n", err)
	}

	fmt.Println("Trace validation completed successfully.")
}
