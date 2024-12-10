package main_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
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

func processStateTransitions(basePath string) error {
	storage, err := storage.NewStateDBStorage("/tmp/validatetraces2")
	if err != nil {
		return err
	}
	stDir := filepath.Join(basePath, "state_transitions")
	stateTransitions := make(map[int]map[int]statedb.StateTransition)

	// Find all state transition files in stDir
	stFiles, err := os.ReadDir(stDir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %v", err)
	}
	for _, file := range stFiles {
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

			// Read the st file
			stPath := filepath.Join(stDir, file.Name())
			stBytes, err := os.ReadFile(stPath)
			if err != nil {
				log.Printf("Error reading block file %s: %v\n", file.Name(), err)
				continue
			}

			// Decode st from stBytes
			b, _, err := types.Decode(stBytes, reflect.TypeOf(statedb.StateTransition{}))
			if err != nil {
				log.Printf("Error decoding block %s: %v\n", file.Name(), err)
				continue
			}
			// Store the state transition in the stateTransitions map
			if stateTransitions[epoch] == nil {
				stateTransitions[epoch] = make(map[int]statedb.StateTransition)
			}
			stateTransitions[epoch][phase] = b.(statedb.StateTransition)
		}
	}

	// Sort epochs and phases to process in order
	var epochs []int
	for epoch := range stateTransitions {
		epochs = append(epochs, epoch)
	}
	sort.Ints(epochs)

	// Iterate through epochs and phases in order
	for _, epoch := range epochs {
		var phases []int
		for phase := range stateTransitions[epoch] {
			phases = append(phases, phase)
		}
		sort.Ints(phases)

		for _, phase := range phases {
			st := stateTransitions[epoch][phase]
			// Apply the state transition
			err := statedb.CheckStateTransition(storage, &st, nil, common.Hash{})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func testStateTransitions(t *testing.T, dir string) {
	// Process the blocks and state transitions
	err := processStateTransitions(dir)
	if err != nil {
		log.Fatalf("Error processing blocks: %v\n", err)
	}

	fmt.Println("Trace validation completed successfully.")
}

func TestFallback(t *testing.T) {
	testStateTransitions(t, "fallback")
}

func TestSafrole(t *testing.T) {
	testStateTransitions(t, "safrole")
}

func TestAssurances(t *testing.T) {
	testStateTransitions(t, "assurances")
}
