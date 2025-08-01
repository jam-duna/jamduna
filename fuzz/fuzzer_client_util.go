package fuzz

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

// hack. find actual parent block
func FindParentBlock(stfs []*statedb.StateTransition, stfQA *StateTransitionQA) *types.Block {
	parentHeaderHash := stfQA.STF.Block.Header.ParentHeaderHash
	for _, stf := range stfs {
		if stf.Block.Header.HeaderHash() == parentHeaderHash {
			return &stf.Block
		}
	}
	return nil
}

func HasParentStfs(stfs []*statedb.StateTransition, target_stf *statedb.StateTransition) bool {
	parentHeaderHash := target_stf.Block.Header.ParentHeaderHash
	for _, stf := range stfs {
		if stf.Block.Header.HeaderHash() == parentHeaderHash {
			return true
		}
	}
	return false
}

// RunUnixSocketChallenge executes a state transition test over an existing connection.
func RunUnixSocketChallenge(fuzzer *Fuzzer, stfQA *StateTransitionQA, verbose bool, stfs []*statedb.StateTransition) (matched bool, solverFuzzed bool, err error) {
	initialStatePayload := &HeaderWithState{
		State: statedb.StateKeyVals{KeyVals: stfQA.STF.PreState.KeyVals},
	}
	parentBlock := FindParentBlock(stfs, stfQA)
	if parentBlock != nil {
		//log.Printf("Challenge B#%.3d Using parent HeaderHash: %s", stfQA.STF.Block.Header.Slot, parentBlock.Header.HeaderHash().Hex())
		initialStatePayload.Header = parentBlock.Header
	} else {
		log.Printf("FATAL: Challenge B#%.3d Parent block not found for STF: %s", stfQA.STF.Block.Header.Slot, stfQA.STF.Block.Header.HeaderHash().Hex())
		return false, false, fmt.Errorf("parent block not found for STF: %s", stfQA.STF.Block.Header.HeaderHash().Hex())
	}
	expectedPreStateRoot := stfQA.STF.PreState.StateRoot

	if stfQA.Mutated {
		log.Printf("%sFUZZED%s %s%s%s B#%.3d", colorMagenta, colorReset, colorMagenta, jamerrors.GetErrorName(stfQA.Error), colorReset, stfQA.STF.Block.Header.Slot)
	} else {
		log.Printf("%sORIGINAL%s B#%.3d", colorGray, colorReset, stfQA.STF.Block.Header.Slot)
	}

	targetPreStateRoot, err := fuzzer.SetState(initialStatePayload)
	if err != nil {
		return false, false, fmt.Errorf("SetState failed: %w", err)
	}

	// Verification for pre-state.
	if !bytes.Equal(targetPreStateRoot.Bytes(), expectedPreStateRoot.Bytes()) {
		log.Printf("FATAL: Pre-state root MISMATCH!\n  Got:  %s\n  Want: %s", targetPreStateRoot.Hex(), expectedPreStateRoot.Hex())
		return false, false, fmt.Errorf("SetState failed: %w", err)
	}

	blockToProcess := &stfQA.STF.Block
	headerHash := blockToProcess.Header.HeaderHash()
	expectedPostStateRoot := stfQA.STF.PostState.StateRoot
	expectedPreStateRoot = stfQA.STF.PreState.StateRoot

	targetPostStateRoot, err := fuzzer.ImportBlock(blockToProcess)
	if err != nil {
		return false, false, fmt.Errorf("ImportBlock failed: %w", err)
	}

	//fmt.Printf("B#%.3d HeaderHash: %s | PostStateRoot: %s | PreStateRoot: %s\n", blockToProcess.Header.Slot, headerHash.Hex(), targetPostStateRoot.Hex(), expectedPreStateRoot.Hex())
	if bytes.Equal(targetPostStateRoot.Bytes(), expectedPreStateRoot.Bytes()) {
		solverFuzzed = true
		//log.Printf("B#%.3d solverFuzzed : %v\n", blockToProcess.Header.Slot, jamerrors.GetErrorName(stfQA.Error))
	}

	if stfQA.Mutated {
		// fuzzed blocks should return as pre-state root
		if bytes.Equal(targetPostStateRoot.Bytes(), expectedPreStateRoot.Bytes()) {
			//log.Printf("B#%.3d Fuzzed block returned expected pre-state root.", stfQA.STF.Block.Header.Slot)
			matched = true
		} else {
			log.Printf("FATAL: Fuzzed block returned expected post-state root: %s | Pre-State Root:%s", targetPostStateRoot.Hex(), expectedPreStateRoot.Hex())
			matched = false // Fuzzed Undetected
		}
	} else {
		// Unfuzzed. Do verification for post-state.
		if bytes.Equal(targetPostStateRoot.Bytes(), expectedPostStateRoot.Bytes()) {
			matched = true
			//log.Printf("B#%.3d Original block returned expected post-state root: %s", blockToProcess.Header.Slot, targetPostStateRoot.Hex())
		} else {
			log.Printf("FATAL: Post-state root MISMATCH!\n  Got:  %s\n  Want: %s", targetPostStateRoot.Hex(), expectedPostStateRoot.Hex())
			matched = false // Post-state root MISMATCH
		}
	}

	if !matched || verbose {
		// Log the mismatch for further analysis.
		if !matched {
			log.Printf("B#%.3d MISMATCH: HeaderHash: %s | PostStateRoot: %s", blockToProcess.Header.Slot, headerHash.Hex(), targetPostStateRoot.Hex())
		} else {
			log.Printf("B#%.3d MATCH: HeaderHash: %s | PostStateRoot: %s", blockToProcess.Header.Slot, headerHash.Hex(), targetPostStateRoot.Hex())
		}
		targetStateKeyVals, err := fuzzer.GetState(&headerHash) // Fetch the state for debugging.
		if err != nil {
			log.Printf("Error fetching state for headerHash %s: %v", headerHash.Hex(), err)
		} else if targetStateKeyVals == nil {
			log.Printf("No state found for headerHash %s", headerHash.Hex())
		} else {
			executionReport := fuzzer.GenerateExecutionReport(stfQA, *targetStateKeyVals)
			log.Printf("Execution Report for B#%.3d:\n%s", blockToProcess.Header.Slot, executionReport.String())
			diffs := statedb.CompareKeyValsWithOutput(executionReport.PreState.KeyVals, executionReport.TargetPostState.KeyVals, executionReport.PostState.KeyVals)
			//fmt.Printf("B#%.3d Diffs:\v%s", blockToProcess.Header.Slot, diffs)
			statedb.HandleDiffs(diffs)
		}
		if !matched {
			os.Exit(1)
		}
	}
	return matched, solverFuzzed, nil
}
