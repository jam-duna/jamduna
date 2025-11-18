package fuzz

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func ReadStateTransitionBIN(filename string) (stf *statedb.StateTransition, err error) {
	stBytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file %s: %v\n", filename, err)
		return nil, fmt.Errorf("error reading file %s: %v", filename, err)
	}
	// Decode st from stBytes
	b, _, err := types.Decode(stBytes, reflect.TypeOf(statedb.StateTransition{}))
	if err != nil {
		fmt.Printf("Error decoding block %s: %v\n", filename, err)
		return nil, fmt.Errorf("error decoding block %s: %v", filename, err)
	}
	st, ok := b.(statedb.StateTransition)
	if !ok {
		return nil, fmt.Errorf("failed to type assert decoded data to StateTransition; got type %T", b)
	}
	stf = &st // Assign the address of the resulting value to the pointer 'stf'.
	return stf, nil
}

func ReadStateTransitionJSON(path string) (*statedb.StateTransition, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %w", path, err)
	}

	var st statedb.StateTransition
	if err := json.Unmarshal(file, &st); err != nil {
		return nil, fmt.Errorf("could not unmarshal json from %s: %w", path, err)
	}
	return &st, nil
}

func ReadStateTransition(filename string) (stf *statedb.StateTransition, err error) {
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}
	if len(filename) > 0 && filename[len(filename)-4:] == ".bin" {
		return ReadStateTransitionBIN(filename)
	}
	return ReadStateTransitionJSON(filename)
}

func ReadStateTransitions(baseDir string) (stfs []*statedb.StateTransition, err error) {
	stfs = make([]*statedb.StateTransition, 0)
	stFiles, err := os.ReadDir(baseDir)
	if err != nil {
		return stfs, fmt.Errorf("failed to read directory: %v", err)
	}
	fmt.Printf("Selected Dir: %v\n", baseDir)
	file_idx := 0
	useJSON := true
	useBIN := true
	for _, file := range stFiles {
		//fmt.Printf("Processing file: %s\n", file.Name())
		// Skip guarantor files (they are bundle files, not STF files)
		if strings.Contains(file.Name(), "guarantor") {
			continue
		}
		if strings.HasSuffix(file.Name(), ".bin") || strings.HasSuffix(file.Name(), ".json") {
			stPath := filepath.Join(baseDir, file.Name())
			isJSON := strings.HasSuffix(file.Name(), ".json")
			isBin := strings.HasSuffix(file.Name(), ".bin")
			if useJSON && isJSON {
				//fmt.Printf("Reading JSON file: %s\n", stPath)
				stf, err := ReadStateTransition(stPath)
				if err != nil {
					log.Printf("Error reading state transition file %s: %v\n", file.Name(), err)
					continue
				}
				stfs = append(stfs, stf)
				file_idx++
			}
			if useBIN && isBin {
				//fmt.Printf("Reading BIN file: %s\n", stPath)
				stf, err := ReadStateTransition(stPath)
				if err != nil {
					log.Printf("Error reading state transition file %s: %v\n", file.Name(), err)
					continue
				}
				stfs = append(stfs, stf)
				file_idx++
			}
		}
	}
	fmt.Printf("Loaded %v state transitions\n", len(stfs))

	// Deduplicate based on prestate->poststate roots to handle bin vs json duplicates
	deduplicated_stfs := DeduplicateStateTransitions(stfs)
	fmt.Printf("Deduplicated to %v unique state transitions\n", len(deduplicated_stfs))

	return deduplicated_stfs, nil
}

func ReadRefineBundles(baseDir string, pvmBackend string, stateDB *statedb.StateDB) (bundles []*RefineBundleQA, stfs []*statedb.StateTransition, err error) {
	// First load state transitions
	stfs, err = ReadStateTransitions(baseDir)
	if err != nil {
		log.Printf("Warning: Failed to read state transitions from bundle directory: %v", err)
		stfs = make([]*statedb.StateTransition, 0)
	}

	// Then load bundle snapshots
	bundleSnapshots := make([]*types.WorkPackageBundleSnapshot, 0)
	bundleFiles, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, stfs, fmt.Errorf("failed to read bundle directory: %v", err)
	}

	fmt.Printf("Reading WorkPackageBundleSnapshot tests from: %v\n", baseDir)
	for _, file := range bundleFiles {
		if strings.Contains(file.Name(), "guarantor") && strings.HasSuffix(file.Name(), ".json") {
			bundlePath := filepath.Join(baseDir, file.Name())
			snapshot, err := ReadWorkPackageBundleSnapshot(bundlePath)
			if err != nil {
				log.Printf("Error reading WorkPackageBundleSnapshot file %s: %v\n", file.Name(), err)
				continue
			}
			bundleSnapshots = append(bundleSnapshots, snapshot)
		}
	}

	/*
		// Convert snapshots to multiple RefineBundleQA variants using refine package
		bundles = make([]*RefineBundleQA, 0)
		numVariantsPerSnapshot := 10 // Generate 10 variants per snapshot for testing

		// First, generate all variants for each snapshot
		allVariants := make([][]*RefineBundleQA, len(bundleSnapshots))

			for i, snapshot := range bundleSnapshots {
				bundleVariants, err := ConvertSnapshotToMultipleRefineBundleQA(snapshot, stfs, numVariantsPerSnapshot, pvmBackend, stateDB)
				if err != nil {
					log.Printf("Error converting snapshot for slot %d: %v", snapshot.Slot, err)
					continue
				}
				allVariants[i] = bundleVariants
			}

			// Interleave variants from different snapshots to ensure balanced testing
			maxVariants := numVariantsPerSnapshot
			for variantIndex := 0; variantIndex < maxVariants; variantIndex++ {
				for _, variants := range allVariants {
					if variantIndex < len(variants) {
						bundles = append(bundles, variants[variantIndex])
					}
				}
			}

			fmt.Printf("Generated %d total RefineBundleQA test cases from %d snapshots\n", len(bundles), len(bundleSnapshots))
	*/
	return bundles, stfs, nil
}

func ReadEmbeddedRefineBundles(embeddedFS embed.FS, pvmBackend string, stateDB *statedb.StateDB) (bundles []*RefineBundleQA, stfs []*statedb.StateTransition, err error) {
	// First load state transitions from embedded files
	stfs, err = ReadEmbeddedStateTransitions(embeddedFS)
	if err != nil {
		log.Printf("Warning: Failed to read state transitions from embedded files: %v", err)
		stfs = make([]*statedb.StateTransition, 0)
	}

	bundleSnapshots := make([]*types.WorkPackageBundleSnapshot, 0)

	entries, err := fs.ReadDir(embeddedFS, "refine")
	if err != nil {
		return nil, stfs, fmt.Errorf("failed to read embedded refine directory: %v", err)
	}

	for _, entry := range entries {
		if strings.Contains(entry.Name(), "guarantor") && strings.HasSuffix(entry.Name(), ".bin") {
			filePath := "refine/" + entry.Name()
			snapshot, err := ReadEmbeddedWorkPackageBundleSnapshot(embeddedFS, filePath)
			if err != nil {
				log.Printf("Error reading embedded WorkPackageBundleSnapshot file %s: %v\n", entry.Name(), err)
				continue
			}
			bundleSnapshots = append(bundleSnapshots, snapshot)
		}
	}

	bundles = make([]*RefineBundleQA, 0)
	//numVariantsPerSnapshot := 10 // Generate 10 variants per snapshot for testing

	// First, generate all variants for each snapshot
	//allVariants := make([][]*RefineBundleQA, len(bundleSnapshots))
	/*
		for i, snapshot := range bundleSnapshots {
			bundleVariants, err := ConvertSnapshotToMultipleRefineBundleQA(snapshot, stfs, numVariantsPerSnapshot, pvmBackend, stateDB)
			if err != nil {
				log.Printf("Error converting snapshot for slot %d: %v", snapshot.Slot, err)
				continue
			}
			allVariants[i] = bundleVariants
		}

		maxVariants := numVariantsPerSnapshot
		for variantIndex := 0; variantIndex < maxVariants; variantIndex++ {
			for _, variants := range allVariants {
				if variantIndex < len(variants) {
					bundles = append(bundles, variants[variantIndex])
				}
			}
		}
	*/
	fmt.Printf("Generated %d total RefineBundleQA test cases from %d embedded snapshots\n", len(bundles), len(bundleSnapshots))

	return bundles, stfs, nil
}

func ReadEmbeddedWorkPackageBundleSnapshot(embeddedFS embed.FS, filePath string) (snapshot *types.WorkPackageBundleSnapshot, err error) {
	bundleBytes, err := fs.ReadFile(embeddedFS, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded file %s: %v", filePath, err)
	}

	if strings.HasSuffix(filePath, ".bin") {
		// Decode binary format
		b, _, err := types.Decode(bundleBytes, reflect.TypeOf(types.WorkPackageBundleSnapshot{}))
		if err != nil {
			return nil, fmt.Errorf("failed to decode embedded WorkPackageBundleSnapshot from %s: %v", filePath, err)
		}
		snapshot2, ok := b.(types.WorkPackageBundleSnapshot)
		if !ok {
			return nil, fmt.Errorf("failed to type assert decoded data to WorkPackageBundleSnapshot; got type %T", b)
		}
		return &snapshot2, nil
	} else {
		// Decode JSON format
		var snapshot2 types.WorkPackageBundleSnapshot
		err = json.Unmarshal(bundleBytes, &snapshot2)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedded WorkPackageBundleSnapshot from %s: %v", filePath, err)
		}
		return &snapshot2, nil
	}
}

func ReadEmbeddedStateTransitions(embeddedFS embed.FS) ([]*statedb.StateTransition, error) {
	var stfs []*statedb.StateTransition

	// Read the refine directory from embedded FS
	entries, err := fs.ReadDir(embeddedFS, "refine")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded refine directory: %v", err)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".bin") && !strings.Contains(entry.Name(), "guarantor") {
			filePath := "refine/" + entry.Name()
			stf, err := ReadEmbeddedStateTransition(embeddedFS, filePath)
			if err != nil {
				log.Printf("Warning: Failed to read embedded STF %s: %v", entry.Name(), err)
				continue
			}
			stfs = append(stfs, stf)
		}
	}

	fmt.Printf("Loaded %d state transitions from embedded files\n", len(stfs))
	return stfs, nil
}

func ReadEmbeddedStateTransition(embeddedFS embed.FS, filePath string) (*statedb.StateTransition, error) {
	stfBytes, err := fs.ReadFile(embeddedFS, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded file %s: %v", filePath, err)
	}

	if strings.HasSuffix(filePath, ".bin") {
		// Decode binary format
		b, _, err := types.Decode(stfBytes, reflect.TypeOf(statedb.StateTransition{}))
		if err != nil {
			return nil, fmt.Errorf("failed to decode embedded StateTransition from %s: %v", filePath, err)
		}
		st, ok := b.(statedb.StateTransition)
		if !ok {
			return nil, fmt.Errorf("failed to type assert decoded data to StateTransition; got type %T", b)
		}
		return &st, nil
	} else {
		// Decode JSON format
		var stf statedb.StateTransition
		err = json.Unmarshal(stfBytes, &stf)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedded StateTransition from %s: %v", filePath, err)
		}
		return &stf, nil
	}
}

func ReadWorkPackageBundleSnapshot(filename string) (snapshot *types.WorkPackageBundleSnapshot, err error) {
	bundleBytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s: %v", filename, err)
	}

	snapshot = &types.WorkPackageBundleSnapshot{}
	if err := json.Unmarshal(bundleBytes, snapshot); err != nil {
		return nil, fmt.Errorf("error unmarshaling WorkPackageBundleSnapshot from %s: %v", filename, err)
	}

	return snapshot, nil
}

func (dr *JsonDiffReport) SaveToFile(dir string, currTS uint32, slot uint32, stfQA *StateTransitionQA) error {
	if dir == "" {
		return fmt.Errorf("directory cannot be empty")
	}

	//tfQA, *targetStateKeyVals

	if dr != nil {
		jsonBytes, err := json.MarshalIndent(dr, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal execution report: %v", err)
		}

		fn := fmt.Sprintf("report_%08d.json", slot)
		filePath := filepath.Join(dir, fmt.Sprintf("%d", currTS), fn)

		fileDir := filepath.Dir(filePath)

		if err := os.MkdirAll(fileDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", fileDir, err)
		}

		if err := os.WriteFile(filePath, jsonBytes, 0644); err != nil {
			return fmt.Errorf("failed to write execution report to file %s: %w", filePath, err)
		}
		log.Printf("✅ Execution report saved to: %s", filePath)
	}
	if stfQA != nil && stfQA.STF != nil {
		stfBytes, err := json.MarshalIndent(stfQA, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal state transition: %v", err)
		}

		fn := fmt.Sprintf("%08d.json", slot)
		filePath := filepath.Join(dir, fmt.Sprintf("%d", currTS), "traces", fn)

		fileDir := filepath.Dir(filePath)

		if err := os.MkdirAll(fileDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", fileDir, err)
		}

		if err := os.WriteFile(filePath, stfBytes, 0644); err != nil {
			return fmt.Errorf("failed to write state transition to file %s: %w", filePath, err)
		}
		log.Printf("✅ State transition saved to: %s", filePath)
	}
	return nil
}
