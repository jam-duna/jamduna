package main

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"io/ioutil"
	"log"
)

func main() {
	// Read the JSON file
	fn := common.GetFilePath("chainspecs.json")
	data, err := ioutil.ReadFile(fn)
	if err != nil {
		log.Fatalf("Error reading file %s: %v", fn, err)
	}

	// Parse the JSON data
	networks := make(map[string]types.ChainSpec)
	err = json.Unmarshal(data, &networks)
	if err != nil {
		log.Fatalf("Error parsing JSON: %v", err)
	}

	sdb, err := storage.NewStateDBStorage("/tmp/genesis")
	if err != nil {
		log.Fatalf("Error with storage: %v", err)
	}

	// Process each network
	for network, chainSpec := range networks {
		fmt.Printf("Processing network: %s\n", network)

		// Call createGenesisState for each network
		outputFilename := common.GetFilePath(fmt.Sprintf("%s.json", network))
		err := statedb.CreateGenesisState(sdb, chainSpec, 0, outputFilename)
		if err != nil {
			log.Printf("Error writing genesis file for network %s: %v", network, err)
		} else {
			fmt.Printf("Genesis state written to %s\n", outputFilename)
		}
	}
}
