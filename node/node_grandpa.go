package node

import (
	"context"

	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// this function will be called when the nodes finish audited the genesis block
var genesisBlockHash common.Hash

func (n *Node) StartGrandpa(b *types.Block) {
	log.Trace(log.Grandpa, "GRANDPA START")
	if b.GetParentHeaderHash() == (genesisBlockHash) {
		ctx := context.Background()
		n.grandpa.PlayGrandpaRound(ctx, 1)
	}
}

func (n *Node) FinalizedBlockHeader(headerHash common.Hash) {
	// For testing purposes, we can just log the finalized block header
	log.Info("G", "Node finalized block header: %s", headerHash.Hex())
	blsSignature, finalizedEpoch, beefy_hash, err := n.statedb.Finalize(n.credential)
	if err != nil {
		log.Error("G", "Error finalizing block: %v", err)
		return
	}

	if finalizedEpoch > 0 {
		n.Broadcast(JAMEpochFinalized{Epoch: finalizedEpoch, BeefyHash: beefy_hash, Signature: blsSignature})
	}
}

func (n *Node) FinalizedEpoch(epoch uint32, beefyHash common.Hash, aggregatedSignature bls.Signature) {
	log.Info("G", "Epoch finalized", "epoch", epoch, "beefyHash", beefyHash.Hex())

	// Broadcast the finalized epoch with aggregated signature to all validators
	epochFinalized := JAMEpochFinalized{
		Epoch:     epoch,
		BeefyHash: beefyHash,
		Signature: aggregatedSignature,
	}
	// TODO: need to store the warp sync fragment corresponding to beefyHash
	header := types.BlockHeader{}
	fragment := types.WarpSyncFragment{
		Header:        header,
		Justification: aggregatedSignature,
	}
	n.store.StoreWarpSyncFragment(epoch, fragment)

	n.Broadcast(epochFinalized)
}
