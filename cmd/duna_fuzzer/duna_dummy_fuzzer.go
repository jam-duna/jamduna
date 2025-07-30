package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"

	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func main() {
	storageDir := "/tmp/fuzzer"
	socketPath := flag.String("socket", "/tmp/jam_target.sock", "Path for the Unix domain socket to connect to")
	testVectorPath := flag.String("test.vector", "test/state_transitions/00000036.json", "Path to the state transition file")
	flag.Parse()

	if *testVectorPath == "" {
		log.Fatal("A test vector file must be provided with the -test.vector flag")
	}

	stf, err := ReadStateTransition(*testVectorPath)
	if err != nil {
		log.Fatalf("Failed to load test vector: %v", err)
	}
	log.Printf("Loaded test vector. Expecting pre-state root: %s", stf.PreState.StateRoot.Hex())

	fuzzerInfo := fuzz.PeerInfo{
		Name:       "jam-duna-dummy-fuzzer-v0.1",
		AppVersion: fuzz.Version{Major: 0, Minor: 6, Patch: 7},
		JamVersion: fuzz.Version{Major: 0, Minor: 6, Patch: 7},
	}

	// Create the fuzzer engine.
	dummy_fuzzer, err := fuzz.NewFuzzer(storageDir, *socketPath, fuzzerInfo, pvm.BackendInterpreter)
	if err != nil {
		log.Fatalf("Failed to create fuzzer: %v", err)
	}

	// 1. Fuzzer Connection - This step was missing.
	log.Printf("Connecting to target on socket: %s", *socketPath)
	if err := dummy_fuzzer.Connect(); err != nil {
		log.Fatalf("Failed to connect to target: %v", err)
	}
	defer dummy_fuzzer.Close()
	log.Println("Connection successful.")

	// 2. Handshake
	log.Println("Performing handshake...")
	_, err = dummy_fuzzer.Handshake()
	if err != nil {
		log.Fatalf("Handshake failed: %v", err)
	}
	log.Println("Handshake successful.")

	// --- Fuzzing Workflow Begins ---

	// 3. Initialize Target State using data from the test vector.
	log.Println("Initializing target state...")
	initialStatePayload := &fuzz.HeaderWithState{
		Header: stf.Block.Header,
		State:  statedb.StateKeyVals{KeyVals: stf.PreState.KeyVals},
	}
	expectedPreStateRoot := stf.PreState.StateRoot

	targetPreStateRoot, err := dummy_fuzzer.SetState(initialStatePayload)
	if err != nil {
		log.Fatalf("SetState failed: %v", err)
	}
	log.Printf("Target initialized. Received pre-state root: %s", targetPreStateRoot.Hex())

	// Verification
	if !bytes.Equal(targetPreStateRoot.Bytes(), expectedPreStateRoot.Bytes()) {
		log.Fatalf("FATAL: Pre-state root MISMATCH!\n  Got:  %s\n  Want: %s", targetPreStateRoot.Hex(), expectedPreStateRoot.Hex())
	}
	log.Println("SUCCESS: Pre-state root matches.")

	// 4. Process the Block from the test vector.
	log.Println("Processing block...")
	blockToProcess := &stf.Block
	headerHash := blockToProcess.Header.HeaderHash()
	expectedPostStateRoot := stf.PostState.StateRoot

	targetPostStateRoot, err := dummy_fuzzer.ImportBlock(blockToProcess)
	if err != nil {
		log.Fatalf("ImportBlock failed: %v", err)
	}
	log.Printf("Target processed block. Received post-state root: %s", targetPostStateRoot.Hex())

	// Verification
	if !bytes.Equal(targetPostStateRoot.Bytes(), expectedPostStateRoot.Bytes()) {
		log.Printf("FATAL: Post-state root MISMATCH!\n  Got:  %s\n  Want: %s", targetPostStateRoot.Hex(), expectedPostStateRoot.Hex())
		log.Println("Requesting full state from target for analysis...")

		targetState, err := dummy_fuzzer.GetState(&headerHash)
		if err != nil {
			log.Fatalf("GetState failed after mismatch: %v", err)
		}
		log.Printf("Received %d key-value pairs from target.", len(targetState.KeyVals))
		os.Exit(1)
	}

	targetState, err := dummy_fuzzer.GetState(&headerHash)
	if err != nil {
		log.Fatalf("GetState failed: %v", err)
	}
	log.Printf("Received %d key-value pairs from target.", len(targetState.KeyVals))

	log.Println("Fuzzer run finished successfully.")
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

func ReadStateTransitionBIN(filename string) (stf *statedb.StateTransition, err error) {
	stBytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file %s: %v\n", filename, err)
		return nil, fmt.Errorf("Error reading file %s: %v", filename, err)
	}
	// Decode st from stBytes
	b, _, err := types.Decode(stBytes, reflect.TypeOf(statedb.StateTransition{}))
	if err != nil {
		fmt.Printf("Error decoding block %s: %v\n", filename, err)
		return nil, fmt.Errorf("Error decoding block %s: %v", filename, err)
	}
	st, ok := b.(statedb.StateTransition)
	if !ok {
		return nil, fmt.Errorf("failed to type assert decoded data to StateTransition; got type %T", b)
	}
	stf = &st // Assign the address of the resulting value to the pointer 'stf'.
	return stf, nil
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
