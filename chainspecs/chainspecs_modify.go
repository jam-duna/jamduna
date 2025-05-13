package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

func main() {
	targetFile := "tiny"
	db_path := "/tmp/jam_test"
	path := node.GetGenesisFile(targetFile)
	sdb, err := storage.NewStateDBStorage(db_path)
	fn := common.GetFilePath(path)
	snapshotRawBytes, err := os.ReadFile(fn)
	var statetransition statedb.StateTransition
	err = json.Unmarshal(snapshotRawBytes, &statetransition)
	if err != nil {
		fmt.Printf("Error unmarshalling JSON: %v\n", err)
		return
	}
	statedb_, err := statedb.NewStateDB(sdb, common.Hash{})
	if err != nil {
		fmt.Printf("Error creating StateDB: %v\n", err)
		return
	}
	statedb_.Block = &(statetransition.Block)
	statedb_.StateRoot = statedb_.UpdateAllTrieStateRaw(statetransition.PostState) // NOTE: MK -- USE PRESTATE
	statedb_.JamState = statedb.NewJamState()
	statedb_.RecoverJamState(statedb_.StateRoot)
	safrole := statedb_.JamState.SafroleState
	// update the metadata of the validators
	UpdateValidatorMetadata(&safrole.PrevValidators)
	UpdateValidatorMetadata(&safrole.CurrValidators)
	UpdateValidatorMetadata(&safrole.NextValidators)
	UpdateValidatorMetadata(&safrole.DesignedValidators)
	statedb_.JamState.SafroleState = safrole
	fmt.Printf("DesignedValidators[0].Metadata: %x\n", statedb_.JamState.SafroleState.DesignedValidators[0].Metadata)
	statedb_.StateRoot = statedb_.UpdateTrieState()
	PostState := statedb.StateSnapshotRaw{
		StateRoot: statedb_.StateRoot,
		KeyVals:   statedb_.GetAllKeyValues(),
	}
	statetransition.PostState = PostState
	modifiedFileName := "tiny_with_metadata-00000000.json"
	modifiedFile, err := os.Create(modifiedFileName)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}

	defer modifiedFile.Close()
	// write the modified state transition to a JSON file
	modifiedFileBytes, err := json.MarshalIndent(statetransition, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling JSON: %v\n", err)
		return
	}
	_, err = modifiedFile.Write(modifiedFileBytes)
	if err != nil {
		fmt.Printf("Error writing to file: %v\n", err)
		return
	}
	fmt.Printf("Modified JSON written to %s\n", modifiedFileName)

	modifiedBin := "tiny_with_metadata-00000000.bin"
	modifiedBinFile, err := os.Create(modifiedBin)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}
	defer modifiedBinFile.Close()
	// write the modified state transition to a binary file
	modifiedBinBytes, err := types.Encode(statetransition)
	if err != nil {
		fmt.Printf("Error encoding to binary: %v\n", err)
		return
	}
	_, err = modifiedBinFile.Write(modifiedBinBytes)
	if err != nil {
		fmt.Printf("Error writing to file: %v\n", err)
		return
	}
}

func UpdateValidatorMetadata(validators *types.Validators) error {
	default_node_0_port := uint16(40000)
	local := "127.0.0.1"
	for i := range *validators {
		newMetaData, err := common.ToIPv6PortBytes(local, default_node_0_port+uint16(i))
		if err != nil {
			return fmt.Errorf("failed to convert IP and port to bytes: %v", err)
		}
		fmt.Printf("newMetaData: %x\n", newMetaData)
		copy((*validators)[i].Metadata[:], newMetaData)
	}
	return nil
}
