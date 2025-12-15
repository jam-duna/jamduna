package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// this function will be called when the nodes finish audited the genesis block
var genesisBlockHash common.Hash

func (n *Node) StartGrandpa(b *types.Block) {
	if !Grandpa {
		return
	}
	log.Trace(log.Grandpa, "GRANDPA START")
	if b.GetParentHeaderHash() == (genesisBlockHash) {
		ctx := context.Background()
		grandpa := n.grandpa.GetOrInitializeGrandpa(0)
		grandpa.PlayGrandpaRound(ctx, 1)
	}
}

func (n *Node) CatchUp(round uint64, setId uint32) (grandpa.CatchUpResponse, error) {
	if !Grandpa {
		return grandpa.CatchUpResponse{}, fmt.Errorf("GRANDPA is disabled")
	}
	// get the peer pubkey that is ready for this round
	peerPubKey, ok := n.grandpa.GetWhoRoundReady(setId, round)
	if !ok {
		return grandpa.CatchUpResponse{}, fmt.Errorf("no peer is ready for catchup for round %d set %d", round, setId)
	}
	// Find peer by Ed25519 pubkey (not by stale PeerID which becomes invalid after epoch rotation)
	peerKey := peerPubKey.String()
	peer, ok := n.peersByPubKey[peerKey]
	if !ok {
		return grandpa.CatchUpResponse{}, fmt.Errorf("peer with pubkey %s not found", peerKey[:16])
	}
	var catchup grandpa.GrandpaCatchUp
	catchup.Round = round
	catchup.SetId = setId
	res, err := peer.SendGrandpaCatchUp(context.Background(), catchup)
	if err != nil {
		return grandpa.CatchUpResponse{}, err
	}
	return *res, nil
}

// WarpSync requests warp sync fragments from peers to quickly sync authority sets
func (n *Node) WarpSync(fromSetID uint32) (types.WarpSyncResponse, error) {
	if !Grandpa {
		return types.WarpSyncResponse{}, fmt.Errorf("GRANDPA is disabled")
	}
	// Try to get warp sync fragments from any available peer
	for _, peer := range n.peersByPubKey {
		response, err := peer.SendWarpSyncRequest(context.Background(), fromSetID)
		if err != nil {
			log.Warn(log.Grandpa, "WarpSync request failed", "peer", peer.PeerID, "err", err)
			continue
		}
		if len(response.Fragments) > 0 {
			return response, nil
		}
	}
	return types.WarpSyncResponse{}, fmt.Errorf("no peer returned warp sync fragments for setID %d", fromSetID)
}

func (n *Node) FinalizedBlockHeader(headerHash common.Hash) {
	if !Grandpa {
		return
	}
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

// NOT PART OF ANY OFFICIAL CE
func (n *Node) FinalizedEpoch(epoch uint32, beefyHash common.Hash, aggregatedSignature bls.Signature) {
	if !Grandpa {
		return
	}
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
		Header: header,
		// Justification: aggregatedSignature,
	}
	n.store.StoreWarpSyncFragment(epoch, fragment)

	n.Broadcast(epochFinalized)
}
