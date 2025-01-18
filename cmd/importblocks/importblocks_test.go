package main_test

// import (
// 	"fmt"
// 	"log"
// 	"sort"
// 	"strconv"
// 	"strings"
// 	"testing"

// 	"github.com/colorfulnotion/jam/statedb"
// 	"github.com/colorfulnotion/jam/types"
// )

// // Helper function to parse epoch and phase from filename
// func parseEpochPhase(filename string) (int, int) {
// 	fileParts := strings.TrimSuffix(filename, ".json")
// 	parts := strings.Split(fileParts, "_")
// 	if len(parts) != 2 {
// 		return -1, -1 // Invalid format
// 	}

// 	epoch, err1 := strconv.Atoi(parts[0])
// 	phase, err2 := strconv.Atoi(parts[1])
// 	if err1 != nil || err2 != nil {
// 		return -1, -1 // Parsing error
// 	}
// 	return epoch, phase
// }

// func loadImportBlocksSTF(dir string, use_generic bool) ([]*statedb.StateTransition, error) {
// 	stfs, err := readStateTransitions(dir)
// 	if err == nil {
// 		return stfs, nil
// 	}
// 	if err != nil && !use_generic {
// 		return nil, fmt.Errorf("Error reading state transitions: %v", err)
// 	}
// 	fmt.Printf("Fallback to generic state transitions: %v\n", err)
// 	stfs_generic, err := readStateTransitions("generic")
// 	if err != nil {
// 		return nil, fmt.Errorf("Error reading state transitions for STF Generic: %v", err)
// 	}
// 	return stfs_generic, nil
// }

// func processStateTransitions(basePath string, use_generic bool) error {

// 	stateTransitions := make(map[int]map[int]statedb.StateTransition)
// 	stfs, loadErr := loadImportBlocksSTF(basePath, use_generic)
// 	if loadErr != nil {
// 		return fmt.Errorf("unable to load state transitions for %v testvector", basePath)
// 	}

// 	for _, stf := range stfs {
// 		timeSlot := stf.Block.TimeSlot()
// 		//TODO: somewhat dangerous
// 		epoch := int(timeSlot / types.EpochLength)
// 		phase := int(timeSlot % types.EpochLength)
// 		// Store the state transition in the stateTransitions map
// 		if stateTransitions[epoch] == nil {
// 			stateTransitions[epoch] = make(map[int]statedb.StateTransition)
// 		}
// 		stateTransitions[epoch][phase] = *stf
// 	}

// 	storage, err := InitFuzzStorage("/tmp/validatetraces2")
// 	if err != nil {
// 		return err
// 	}

// 	// Sort epochs and phases to process in order
// 	var epochs []int
// 	for epoch := range stateTransitions {
// 		epochs = append(epochs, epoch)
// 	}
// 	sort.Ints(epochs)

// 	// Iterate through epochs and phases in order
// 	for _, epoch := range epochs {
// 		var phases []int
// 		for phase := range stateTransitions[epoch] {
// 			phases = append(phases, phase)
// 		}
// 		sort.Ints(phases)

// 		for _, phase := range phases {
// 			st := stateTransitions[epoch][phase]
// 			// Apply the state transition
// 			err := statedb.CheckStateTransition(storage, &st, nil)
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}
// 	return nil
// }

// // TODO: need to switch this to false and provide generic state transitions data dump
// const USE_GENERIC = true

// func testStateTransitions(t *testing.T, dir string) {
// 	// Process the blocks and state transitions
// 	err := processStateTransitions(dir, USE_GENERIC)
// 	if err != nil {
// 		log.Fatalf("Error processing blocks: %v\n", err)
// 	}
// 	fmt.Println("Trace validation completed successfully.")
// }

// func TestFallback(t *testing.T) {
// 	testStateTransitions(t, "fallback")
// }

// func TestSafrole(t *testing.T) {
// 	testStateTransitions(t, "safrole")
// }

// func TestAssurances(t *testing.T) {
// 	testStateTransitions(t, "assurances")
// }
