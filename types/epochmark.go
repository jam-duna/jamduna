package types

import (
	"github.com/colorfulnotion/jam/common"
	//"encoding/json"
)

// EpochMark (see 6.4 Epoch change Signal) represents the descriptor for parameters to be used in the next epoch
type EpochMark struct {
	// Randomness accumulator snapshot
	Entropy common.Hash `json:"entropy"`
	// List of authorities scheduled for next epoch
	Validators []common.Hash `json:"validators"` //bandersnatch keys
}
