package grandpa

import (
	"fmt"

	"github.com/colorfulnotion/jam/types"
)

/*
CE 153: Warp Sync Request
A request for a number of warp sync fragments starting at (and including) a given set id.
The responding node should return as many sequential warp sync fragments as it can
until the latest set has been reached or the message size is too big.

If the responding node is unable to respond it should stop the stream.

Node -> Node

--> Set Id
--> FIN
<-- len++[Warp Sync Fragment]
<-- FIN


## Grandpa Justification

Grandpa Justification is something that when combined with a Header and proven authority set proves finality of a block.
It does not include the Header as this is assumed to already be stored elsewhere.
It does however include Headers for any auxiliary blocks that might be needed for routing all precommit target blocks to the commit target block.
For example, these blocks might not exist in the canonical chain but were still voted on, hence the need to include the Header.
This set of Headers is called the Votes Ancestries.
In normal operation this will be empty as validators will vote for the same proposed block that will then be the committed block.


## Warp Sync Fragment = Header ++ Grandpa Justification

A Warp Sync Fragment is simply a Header and Grandpa Justification for a block that instigates an authority set change.
These are used to quickly prove finality.
It includes the Header so that a chain of Warp Sync Fragments is all that is needed to prove finality up to the latest authority set.
One Warp Sync Fragment will exist for each block that instigates a set change and they will be ordered by and indexed by Set Id.
In normal operation the first block of a new epoch is a set change block.
The Warp Sync Fragment with index (Set ID) 0 provides proof of finality for the first set change block after genesis.
A syncing node can quickly prove finality by doing the following:
* Step 1: Send warp sync requests to peers until all fragments have been downloaded
* Step 2: Prove finality of the next set change block using the next fragment and the current authority set (The very first fragment will be Set Id 0 with genesis authority set).
* Step 3: Proving finality of the block also proves the set change. Change current authority set based on the marker in the change block header.
Repeat steps 2 and 3 until the latest set has been proven. At this point a Grandpa Justification for the latest block can be requested and finality can be proven using the latest set.
*/

func (g *Grandpa) GetWarpSyncResponse(setID uint32) (types.WarpSyncResponse, error) {
	fragments := []types.WarpSyncFragment{}
	for i := setID; i < g.authority_set_id; i++ {
		warpsyncfragment, err := g.storage.GetWarpSyncFragment(i)
		if err != nil {
			return types.WarpSyncResponse{}, fmt.Errorf("GetWarpSyncResponse: getWarpSyncFragment failed: %w", err)
		}
		fragments = append(fragments, warpsyncfragment)
	}

	response := types.WarpSyncResponse{
		Fragments: fragments,
	}

	return response, nil
}

func (g *Grandpa) ProcessWarpSyncRequest(req uint32) (response types.WarpSyncResponse, err error) {
	response, err = g.GetWarpSyncResponse(req)
	if err != nil {
		return response, err
	}
	return response, nil
}

// ProcessWarpSyncResponse processes a received warp sync response
// This verifies and stores the warp sync fragments to quickly sync finality
func (g *Grandpa) ProcessWarpSyncResponse(response types.WarpSyncResponse) error {
	if len(response.Fragments) == 0 {
		return fmt.Errorf("ProcessWarpSyncResponse: empty fragments")
	}

	// Process each fragment sequentially
	for _, fragment := range response.Fragments {
		if err := g.ProcessWarpSyncFragment(fragment); err != nil {
			return fmt.Errorf("ProcessWarpSyncResponse: failed to process fragment: %w", err)
		}
	}

	return nil
}

// ProcessWarpSyncFragment processes a single warp sync fragment to update authority set
// This verifies the justification and extracts the new authority set from the header
func (g *Grandpa) ProcessWarpSyncFragment(fragment types.WarpSyncFragment) error {
	setID := fragment.Justification.SetId

	// Verify the fragment's justification against the current authority set
	if err := g.VerifyWarpSyncJustification(fragment); err != nil {
		return fmt.Errorf("justification verification failed for setID %d: %w", setID, err)
	}

	// Extract the new authority set from the header's EpochMark
	if fragment.Header.EpochMark != nil {
		newValidators := make([]types.Validator, len(fragment.Header.EpochMark.Validators))
		for i, validatorKeyTuple := range fragment.Header.EpochMark.Validators {
			newValidators[i] = types.Validator{
				Ed25519:      types.Ed25519Key(validatorKeyTuple.Ed25519Key),
				Bandersnatch: types.BandersnatchKey(validatorKeyTuple.BandersnatchKey),
			}
		}
		// Update the scheduled authority set for the next epoch
		g.SetScheduledAuthoritySet(newValidators)
	}

	// Store the fragment for future use
	if g.storage != nil {
		if err := g.storage.StoreWarpSyncFragment(setID, fragment); err != nil {
			return fmt.Errorf("failed to store warp sync fragment: %w", err)
		}
	}

	// Update authority set ID to reflect we've processed this fragment
	g.authority_set_id = setID + 1

	return nil
}

// VerifyWarpSyncJustification verifies a warp sync fragment's justification
// against the current authority set
func (g *Grandpa) VerifyWarpSyncJustification(fragment types.WarpSyncFragment) error {
	justification := fragment.Justification
	commit := justification.Commit

	// Verify we have enough precommits (need >2/3 of authority set weight)
	signedWeight := uint64(0)
	authorities := g.GetCurrentScheduledAuthoritySet(justification.Round)
	totalWeight := uint64(len(authorities))

	// Verify each precommit signature
	for _, signedPrecommit := range commit.Precommits {
		validatorKey := signedPrecommit.Ed25519Pub

		// Find the validator in the authority set
		validatorIdx := -1
		for i, validator := range authorities {
			if validator.Ed25519 == validatorKey {
				validatorIdx = i
				break
			}
		}

		if validatorIdx == -1 {
			continue // Skip precommits from non-authorities
		}

		// Verify Ed25519 signature
		if !VerifyPrecommitSignature(signedPrecommit, justification.Round, justification.SetId) {
			continue // Skip invalid signatures
		}

		signedWeight += 1
	}

	// Check if we have >2/3 weight (using ceiling to be safe with integer division)
	threshold := (totalWeight*2 + 2) / 3
	if signedWeight < threshold {
		return fmt.Errorf("insufficient precommit weight: got %d, need >=%d", signedWeight, threshold)
	}

	// Verify the target block hash matches the header
	targetHash := fragment.Header.Hash()
	if commit.HeaderHash != targetHash {
		return fmt.Errorf("target hash mismatch: commit has %s, header is %s",
			commit.HeaderHash.Hex(), targetHash.Hex())
	}

	return nil
}

// VerifyPrecommitSignature verifies the Ed25519 signature of a GrandpaSignedPrecommit
func VerifyPrecommitSignature(precommit types.GrandpaSignedPrecommit, round uint64, setId uint32) bool {
	// Construct the unsigned data that was signed
	unsignedData := GrandpaVoteUnsignedData{
		Message: Message{
			Stage: PrecommitStage,
			Vote: Vote{
				HeaderHash: precommit.Vote.HeaderHash,
				Slot:       precommit.Vote.Slot,
			},
		},
		Round:           round,
		Authorities_set: uint64(setId),
	}

	// Get the bytes that were signed (with "jam_grandpa_vote" prefix)
	unsignedBytes := getGrandpaVoteUnsignedBytes(unsignedData)
	if unsignedBytes == nil {
		return false
	}

	// Verify the signature
	return types.Ed25519Verify(precommit.Ed25519Pub, unsignedBytes, precommit.MessageSignature)
}
