package statedb

import (
	"encoding/json"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// C2
type RecentBlocks []Beta_state

type Beta_state struct {
	HeaderHash common.Hash `json:"header_hash"`
	B          trie.MMR    `json:"mmr"`
	StateRoot  common.Hash `json:"state_root"`
	Reported   Reported    `json:"reported"` // Use the custom type
}

type Reported map[common.Hash]common.Hash

// MarshalJSON serializes Reported as a map[string]string
func (r Reported) MarshalJSON() ([]byte, error) {
	stringMap := make(map[string]string)
	for k, v := range r {
		stringMap[k.Hex()] = v.Hex() // Convert keys and values to hex strings
	}
	return json.Marshal(stringMap)
}

// UnmarshalJSON deserializes Reported from a map[string]string
func (r *Reported) UnmarshalJSON(data []byte) error {
	stringMap := make(map[string]string)
	if err := json.Unmarshal(data, &stringMap); err != nil {
		return err
	}

	*r = make(Reported)
	for k, v := range stringMap {
		keyHash := common.HexToHash(k) // Convert hex strings back to common.Hash
		valueHash := common.HexToHash(v)
		(*r)[keyHash] = valueHash
	}
	return nil
}

// Recent History : see Section 7
func (s *StateDB) ApplyStateRecentHistory(blk *types.Block, accumulationRoot *common.Hash) {
	// Eq 83 n
	// Eq 83 n.p -- aggregate all the workpackagehashes of the guarantees
	reported := map[common.Hash]common.Hash{}
	for _, g := range blk.Guarantees() {
		reported[g.Report.AvailabilitySpec.WorkPackageHash] = g.Report.AvailabilitySpec.ExportedSegmentRoot
	}

	preRecentBlocks := s.JamState.RecentBlocks
	if len(preRecentBlocks) > 0 {
		preRecentBlocks[len(preRecentBlocks)-1].StateRoot = s.StateRoot // ****
	}

	// Eq 83 n.b
	mmr := trie.NewMMR()
	if len(preRecentBlocks) > 0 {
		mmr.Peaks = preRecentBlocks[len(preRecentBlocks)-1].B.Peaks
	}
	mmr.Append(accumulationRoot)
	n := Beta_state{
		Reported:   reported,          // p
		HeaderHash: blk.Header.Hash(), // h
		B:          *mmr,              // b
		StateRoot:  common.Hash{},     // this will become the POSTERIOR stateroot in the NEXT update, updated via **** above
	}

	// Eq 84 β' ≡ β† ++ n (last H=types.RecentHistorySize)
	postRecentBlocks := append(preRecentBlocks, n)
	if len(postRecentBlocks) > types.RecentHistorySize {
		postRecentBlocks = postRecentBlocks[1 : types.RecentHistorySize+1]
	}
	s.JamState.RecentBlocks = postRecentBlocks
	return
}
