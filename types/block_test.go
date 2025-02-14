package types

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
)

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
