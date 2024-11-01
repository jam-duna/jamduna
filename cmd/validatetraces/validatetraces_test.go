package main_test

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
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

func TestSnapshotRaw(t *testing.T) {
	// Read the JSON snapshot file
	fileNames := []string{"366221_8.json", "366221_8.json"}
	for _, fn := range fileNames {
		snapshotRawBytes, err := os.ReadFile(fn)
		if err != nil {
			log.Fatalf("Error reading JSON file %s: %v\n", fn, err)
			return
		}
		var stateSnapshotRaw statedb.StateSnapshotRaw
		err = json.Unmarshal(snapshotRawBytes, &stateSnapshotRaw)
		if err != nil {
			log.Fatalf("Error unmarshaling JSON file: %v\n", err)
			return
		}
		fmt.Printf("READ JSON File: %s\n", fn)

		// Encode the StateSnapshotRaw back into binary
		stateSnapshot := stateSnapshotRaw.FromStateSnapshotRaw()
		snapshotRaw2 := stateSnapshot.Raw()
		snapshotRawBytes2, err := json.MarshalIndent(snapshotRaw2, "", "  ")
		if err != nil {
			log.Fatalf("Error marshaling JSON file: %v\n", err)
			return
		}

		var obj1, obj2 interface{}
		err = json.Unmarshal(snapshotRawBytes, &obj1)
		if err != nil {
			log.Fatalf("Error unmarshaling snapshotRawBytes: %v\n", err)
			return
		}
		err = json.Unmarshal(snapshotRawBytes2, &obj2)
		if err != nil {
			log.Fatalf("Error unmarshaling snapshotRawBytes2: %v\n", err)
			return
		}

		if !reflect.DeepEqual(obj1, obj2) {
			fmt.Printf("snapshotRawBytes: %s\n", snapshotRawBytes)
			fmt.Printf("snapshotRawBytes2: %s\n", snapshotRawBytes2)
			log.Fatalf("snapshotRawBytes != snapshotRawBytes2\n")
		}
	}
}
