package main_test

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"

	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// Helper function to parse epoch and phase from filename
func parseEpochPhase(filename string) (int, int) {
	fileParts := strings.TrimSuffix(filename, ".json")
	parts := strings.Split(fileParts, "_")
	if len(parts) != 2 {
		return -1, -1 // Invalid format
	}

	epoch, err1 := strconv.Atoi(parts[0])
	phase, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return -1, -1 // Parsing error
	}
	return epoch, phase
}

func TestSnapshot(t *testing.T) {

	// set up maps to hold Blocks and Snapshots
	blockParentStateRoot := make(map[string]common.Hash)
	stateRoots := make(map[string]common.Hash)

	// read all the Blocks  files in dir
	dir := "Blocks"
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("Error reading directory %s: %v\n", dir, err)
		return
	}

	fmt.Printf("Loading %s:\n", dir)
	for _, file := range files {
		// Process only JSON files
		if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
			filePath := filepath.Join(dir, file.Name())

			// Read the file contents
			fileBytes, err := os.ReadFile(filePath)
			if err != nil {
				log.Printf("Error reading file %s: %v\n", filePath, err)
				continue
			}

			// Unmarshal the JSON into a CBlock struct
			var block types.CBlock
			err = json.Unmarshal(fileBytes, &block)
			if err != nil {
				log.Printf("Error unmarshaling JSON file %s: %v\n", filePath, err)
				continue
			}
			blockParentStateRoot[file.Name()] = block.Header.ParentStateRoot
			// Print out the parent_state_root
			fmt.Printf("Blocks File: %s => ParentStateRoot: %s\n", file.Name(), block.Header.ParentStateRoot)
		}
	}

	// Sort block files by epoch and phase
	var sortedBlockFiles []string
	for filename := range blockParentStateRoot {
		sortedBlockFiles = append(sortedBlockFiles, filename)
	}
	sort.Slice(sortedBlockFiles, func(i, j int) bool {
		e1, p1 := parseEpochPhase(sortedBlockFiles[i])
		e2, p2 := parseEpochPhase(sortedBlockFiles[j])
		if e1 == e2 {
			return p1 < p2
		}
		return e1 < e2
	})

	// Set the directory to scan for JSON files
	snapshotsDir := "Traces"
	fmt.Printf("\nLoading %s:\n", snapshotsDir)
	files, err = os.ReadDir(snapshotsDir)
	if err != nil {
		log.Fatalf("Error reading directory %s: %v\n", snapshotsDir, err)
		return
	}
	storage, _ := storage.NewStateDBStorage("/tmp/validatetraces")

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			fn := filepath.Join(snapshotsDir, file.Name())

			snapshotRawBytes, err := os.ReadFile(fn)
			if err != nil {
				log.Fatalf("Error reading JSON file %s: %v\n", fn, err)
				continue
			}

			var stateSnapshotRaw statedb.StateSnapshotRaw
			err = json.Unmarshal(snapshotRawBytes, &stateSnapshotRaw)
			if err != nil {
				log.Fatalf("Error unmarshaling JSON file %s: %v\n", fn, err)
				continue
			}

			keyvals := stateSnapshotRaw.KeyVals
			data := make([][2][]byte, 0)
			trie := trie.NewMerkleTree(data, storage)
			for _, kv := range keyvals {
				trie.Insert(kv[0], kv[1])
			}
			stateRoot := trie.GetRoot()

			stateRoots[file.Name()] = stateRoot
			fmt.Printf("Snapshot File: %s =>  StateRoot: %v\n", file.Name(), stateRoot)
		}
	}

	// Iterate over sorted block files and compare state roots
	fmt.Printf("\nChecking matches between %s and %s:\n", dir, snapshotsDir)
	for _, blockFile := range sortedBlockFiles {
		parentStateRoot := blockParentStateRoot[blockFile]
		epoch, phase := parseEpochPhase(blockFile)

		// Generate the guessed snapshot filename
		var snapshotFile string
		if phase == 0 {
			snapshotFile = fmt.Sprintf("%d_11.json", epoch-1)
		} else {
			snapshotFile = fmt.Sprintf("%d_%d.json", epoch, phase-1)
		}

		// Retrieve the state root, default to genesis if not found
		stateRoot, found := stateRoots[snapshotFile]
		if !found {
			stateRoot = stateRoots["genesis.json"]
		}

		// Print and compare
		if parentStateRoot == stateRoot {
			fmt.Printf("PASS: ")
		} else {
			t.Errorf("FAIL: Block File: %s => ParentStateRoot: %s, Snapshot File: %s => StateRoot: %v\n", blockFile, parentStateRoot, snapshotFile, stateRoot)
		}
		fmt.Printf("Block File: %s => ParentStateRoot: %s, Snapshot File: %s => StateRoot: %v\n", blockFile, parentStateRoot, snapshotFile, stateRoot)
	}

}
