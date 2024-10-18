package main_test

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"log"
	"os"
	"reflect"
	"testing"
)

func TestSnapshot(t *testing.T) {
	// Read the JSON snapshot file
	fn := "349463_0.json"
	snapshotBytes, err := os.ReadFile(fn)
	if err != nil {
		log.Fatalf("Error reading JSON file %s: %v\n", fn, err)
		return
	}

	// Unmarshal the JSON into the StateSnapshot struct
	var statesnapshot statedb.StateSnapshot
	err = json.Unmarshal(snapshotBytes, &statesnapshot)
	if err != nil {
		log.Fatalf("Error unmarshaling JSON file: %v\n", err)
		return
	}
	fmt.Printf("READ JSON File: %s\n", fn)

	// Encode the StateSnapshot back into binary
	snapshotBytes, err = types.Encode(statesnapshot)
	if err != nil {
		log.Fatalf("Encode err %v\n", err)
	}
	fmt.Printf("Encoded to %d bytes\n", len(snapshotBytes))

	// Decode the binary back into the StateSnapshot
	_, _, err = types.Decode(snapshotBytes, reflect.TypeOf(statedb.StateSnapshot{}))
	if err != nil {
		log.Fatalf("Decode failure: %v\n", err)
		return
	}
	fmt.Printf("Successfully encoded and decoded\n")
}
