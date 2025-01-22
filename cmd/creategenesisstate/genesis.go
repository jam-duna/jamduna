package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
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
		outputFilename, err := statedb.CreateGenesisState(sdb, chainSpec, 0, network)
		if err != nil {
			log.Printf("Error writing genesis file for network %s: %v", network, err)
			panic(1)
		}
		fmt.Printf("Genesis state created: %s\n", outputFilename)
	}
}
