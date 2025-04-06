package types

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"io/ioutil"
	"os"
	"testing"
)

func TestDecodeBlocks(t *testing.T) {
	// Read the file "assurances.bin" from a concatenated array of blocks (forming a chain)
	// cat ../../../jam-duna/jamtestnet/data/assurances/blocks/*.bin  > assurances.bin
	// cat ../../../jam-duna/jamtestnet/data/orderedaccumulation/blocks/*.bin > orderedaccumulation.bin
	filePaths := []string{"assurances.bin", "orderedaccumulation.bin"}
	for _, filePath := range filePaths {
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", filePath, err)
		}

		// Unmarshal the JSON into a CBlock
		blocks, err := DecodeBlocks(data)
		if err != nil {
			t.Fatalf("Failed to DecodeBlocks %s: %v", filePath, err)
		}
		fmt.Printf("Got %d blocks from %s\n", len(blocks), filePath)
		var lastHeader common.Hash
		for i, b := range blocks {
			fmt.Printf("%s block %d/%d (Slot %d) Parent Header hash %s <= header hash %s\n", filePath, i, len(blocks), b.Header.Slot, b.Header.ParentHeaderHash, b.Header.Hash())
			if i > 0 {
				if b.Header.ParentHeaderHash != lastHeader {
					panic(22)
				}
			}
			lastHeader = b.Header.Hash()
		}
	}
}

func TestExtrinsicHash(t *testing.T) {
	// Read the file "1_003.json"
	filePath := "1_003.json"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}

	// Unmarshal the JSON into a CBlock
	var cblock CBlock
	if err := json.Unmarshal(data, &cblock); err != nil {
		t.Fatalf("Failed to unmarshal JSON into CBlock: %v", err)
	}

	// Convert the CBlock into a Block using fromCBlock
	var block Block
	block.fromCBlock(&cblock)

	// Print the resulting Block using its String() method.
	fmt.Println(block.Extrinsic.Hash())
	// func (e *ExtrinsicData) Hash()
}
