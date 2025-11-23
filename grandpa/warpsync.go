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
		// TODO: Verify the fragment's justification against the current authority set
		// TODO: Extract the set ID from the fragment header
		// TODO: Update the authority set if the fragment indicates a set change
		// TODO: Store the fragment for future use
		_ = fragment // Suppress unused variable warning
	}

	// TODO: Update g.authority_set_id to the latest set ID from fragments
	// TODO: Store fragments in storage using StoreWarpSyncFragment

	return nil
}
