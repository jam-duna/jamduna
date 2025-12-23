package node

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	rand0 "math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	storage "github.com/colorfulnotion/jam/storage"
	trie "github.com/colorfulnotion/jam/trie"
	types "github.com/colorfulnotion/jam/types"
)

type CE139_request struct {
	ErasureRoot    common.Hash
	SegmentIndices []uint16
	CoreIndex      uint16
	ShardIndex     uint16
}

type CE139_response struct {
	ErasureRoot           common.Hash
	ShardIndex            uint16
	SegmentShards         []byte
	SegmentJustifications [][]byte
}

type CE138_request struct {
	ErasureRoot common.Hash
	CoreIndex   uint16
	ShardIndex  uint16
}

type CE138_response struct {
	ShardIndex    uint16
	BundleShard   []byte
	SClub         common.Hash
	Justification []byte
}

type CE128_request struct {
	HeaderHash    common.Hash `json:"headerHash"`
	Direction     uint8       `json:"direction"`
	MaximumBlocks uint32      `json:"maximumBlocks"`
}

type CE128_response struct {
	HeaderHash common.Hash `json:"headerHash"`
	Direction  uint8       `json:"direction"`
	Blocks     []types.Block
}

func (n *NodeContent) GetBlockByHeaderHash(headerHash common.Hash) (*types.SBlock, error) {
	blk, err := n.GetStoredBlockByHeader(headerHash)
	if err != nil {
		return blk, err
	}
	return blk, nil
}

func (n *NodeContent) BlocksLookup(headerHash common.Hash, direction uint8, maximumBlocks uint32) (blocks []types.Block, ok bool, err error) {
	blocks = make([]types.Block, 0)
	if direction == 0 {
		//Direction = 0 (Ascending exclusive)  - child, grandchild, ...

		currentHash := headerHash
		for i := uint32(0); i < maximumBlocks; i++ {
			childBlksWithFork, err := n.GetAscendingBlockByHeader(currentHash)
			if err != nil {
				return blocks, false, err
			}

			if len(childBlksWithFork) == 0 {
				break
			}
			if len(childBlksWithFork) == 1 {
				childBlk := childBlksWithFork[0]
				blocks = append(blocks, *childBlk)
				childHeaderHash := childBlk.Header.Hash()
				currentHash = childHeaderHash
			}
			if len(childBlksWithFork) > 1 {
				return blocks, false, fmt.Errorf("BlocksLookup: multiple childBlksWithFork found %d", len(childBlksWithFork))
			}
		}
	} else {
		// Direction = 1 (Descending inclusive) - block, parent, grandparent ...
		// go through fetch of up of maximumBlocks
		currentHash := headerHash
		for i := uint32(0); i < maximumBlocks; i++ {
			blk, err := n.GetBlockByHeaderHash(currentHash)
			if err != nil {
				return blocks, false, err
			}
			blocks = append(blocks, types.Block{
				Header:    blk.Header,
				Extrinsic: blk.Extrinsic,
			})
			if blk.Header.ParentHeaderHash == (common.Hash{}) {
				break
			}
			currentHash = blk.Header.ParentHeaderHash
		}
	}
	return blocks, true, nil
}

func (n *NodeContent) WorkReportLookup(workReportHash common.Hash) (workReport types.WorkReport, ok bool, err error) {
	workReport, found := n.cacheWorkReportRead(workReportHash)
	if found {
		return workReport, true, nil
	}
	return workReport, false, nil
}

func (n *NodeContent) GetState(headerHash common.Hash, startKey [31]byte, endKey [31]byte, maximumSize uint32) (boundarynodes [][]byte, keyvalues types.StateKeyValueList, ok bool, err error) {
	s := n.getPVMStateDB()
	blocks, ok, err := n.BlocksLookup(headerHash, 1, 1)
	if !ok || err != nil {
		return boundarynodes, keyvalues, false, err
	}
	stateRoot := blocks[0].Header.ParentStateRoot
	trie, err := trie.InitMerkleTreeFromHash(stateRoot, s.GetStorage())
	if err != nil {
		return boundarynodes, keyvalues, false, err
	}
	foundKeyVal, boundaryNode, err := trie.GetStateByRange(startKey[:], endKey[:], maximumSize)
	if err != nil {
		return boundarynodes, keyvalues, false, err
	}
	keyvalues = types.StateKeyValueList{Items: foundKeyVal}
	return boundaryNode, keyvalues, true, nil
}

func (n *Node) processBlockAnnouncement(ctx context.Context, np_blockAnnouncement JAMSNP_BlockAnnounce) ([]types.Block, error) {

	validatorIndex := np_blockAnnouncement.Header.AuthorIndex
	newSlot := np_blockAnnouncement.Header.Slot
	// check if epoch has changed
	recoveredStateDB, err := statedb.NewStateDBFromStateRoot(np_blockAnnouncement.Header.ParentStateRoot, n.store)
	if err != nil {
		return nil, err
	}
	safrole := recoveredStateDB.GetSafrole()
	isNewEpoch := safrole.IsNewEpoch(newSlot)
	var author types.Validator
	author, err = safrole.GetCurrValidator(int(validatorIndex))
	if err != nil {
		return nil, err
	}
	if isNewEpoch {
		author, err = safrole.GetNextValidator(int(validatorIndex))
		if err != nil {
			return nil, err
		}
	}
	var p *Peer
	var found bool
	for _, peer := range n.peersByPubKey {
		peerKey := peer.Validator.Ed25519
		if bytes.Equal(peerKey[:], author.Ed25519[:]) {
			p = peer
			found = true
			break
		}
	}
	if !found {
		err := fmt.Errorf("invalid validator index %d", validatorIndex)
		log.Error(log.Node, "processBlockAnnouncement", "err", err)
		return nil, err
	}

	headerHash := np_blockAnnouncement.Header.HeaderHash()
	parentHash := np_blockAnnouncement.Header.ParentHeaderHash
	latst_finalized_block := n.block_tree.GetLastFinalizedBlock()
	last_finalized_block_header_hash := latst_finalized_block.Block.Header.Hash()

	var mode int
	const (
		OneBlockMode     = 0
		AllBlocksMode    = 1
		MiddleBlocksMode = 2
	)
	var num uint32
	if _, ok := n.block_tree.GetBlockNode(parentHash); ok {
		mode = OneBlockMode
	} else if len(n.block_tree.TreeMap) == 1 {
		mode = AllBlocksMode
	} else {
		finalized_block := n.block_tree.GetLastFinalizedBlock()
		finalized_block_slot := finalized_block.Block.Header.Slot
		if finalized_block_slot < np_blockAnnouncement.Header.Slot {
			num = np_blockAnnouncement.Header.Slot - finalized_block_slot
			mode = MiddleBlocksMode
		} else {
			return nil, errors.New("block announcement is too old")
		}
	}
	var lastErr error
	var blocksRaw []types.Block
	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Respect cancellation early
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("processBlockAnnouncement canceled: %w, attempt %v, mode %v", ctx.Err(), attempt, mode)
		default:
		}

		// Small timeout context per attempt

		switch mode {
		// TODO: send request multiple times
		case OneBlockMode:
			attemptCtx, cancel := context.WithTimeout(ctx, NormalTimeout)
			blocksRaw, lastErr = p.GetOneBlock(headerHash, attemptCtx)
			cancel()
		case AllBlocksMode:
			attemptCtx, cancel := context.WithTimeout(ctx, VeryLargeTimeout)
			blocksRaw, lastErr = p.GetMultiBlocks(last_finalized_block_header_hash, attemptCtx)
			cancel()
			log.Warn(log.B, "GetAllBlocks", "attempt", attempt, "blockHash", headerHash, "blocksRaw.Len", len(blocksRaw), "isSync", false)
			n.SetIsSync(false, "no blocks")
		case MiddleBlocksMode:
			attemptCtx, cancel := context.WithTimeout(ctx, VeryLargeTimeout)
			blocksRaw, lastErr = p.GetMultiBlocks(last_finalized_block_header_hash, attemptCtx)
			cancel()
			log.Warn(log.B, "GetMiddleBlocks", "attempt", attempt, "num", num, "blockHash", headerHash, "blocksRaw.Len", len(blocksRaw), "isSync", false)
			n.SetIsSync(false, "behind others")
		default:
			return nil, fmt.Errorf("invalid mode %d", mode)
		}

		if lastErr == nil && len(blocksRaw) > 0 {
			if attempt > 1 {
				log.Info(log.Node, "SendBlockRequest succeeded", "attempt", attempt, "blockHash", headerHash)
			}
			break
		}

		if attempt == maxAttempts {
			log.Warn(log.Node, "SendBlockRequest failed after 3 attempts", "mode", mode, "blockHash", headerHash, "lastErr", lastErr)
			return nil, fmt.Errorf("SendBlockRequest failed after 3 attempts with mode=%v: %w", mode, lastErr)
		}

		// Failed attempt - try to find a different peer for the next attempt
		log.Warn(log.Node, "SendBlockRequest failed, selecting new peer", "attempt", attempt, "blockHash", headerHash, "lastErr", lastErr)
		currentPeerKey := p.Validator.Ed25519.SAN()
		selfPubKey := n.GetEd25519Key().SAN()
		peers := make([]*Peer, 0, len(n.peersByPubKey))
		for pubKey, peer := range n.peersByPubKey {
			if pubKey != currentPeerKey && pubKey != selfPubKey {
				peers = append(peers, peer)
			}
		}
		if len(peers) > 0 {
			p = peers[rand0.Intn(len(peers))]
			log.Info(log.Node, "SendBlockRequest: switched to new peer", "newPeerKey", p.SanKey())
		}
	}
	for i := 0; i < len(blocksRaw); i++ {
		block := &blocksRaw[i]
		err := n.processBlock(block)
		if err != nil {
			log.Error(log.Node, "processBlockAnnouncement", "err", err, "mode", mode, "blockHash", block.Header.Hash())
			return nil, err
		}
	}
	log.Debug(log.Quic, "Broadcast blockAnnouncement", "n", n.String(), "blockHash", headerHash)

	n.broadcast(context.Background(), np_blockAnnouncement)
	return blocksRaw, nil
}

func (n *NodeContent) cacheBlock(block *types.Block) error {

	if block != nil && n.block_tree != nil { // check
		err := n.block_tree.AddBlock(block)
		if err != nil {
			if block.Header.Hash() == genesisBlockHash {
				return nil
			}
			if err.Error() == "already exists" {
				return nil
			}
			return err
		}
		// also prune the block tree
		useless_header_hashes := n.block_tree.PruneBlockTree(10)

		go n.nodeSelf.Clean(useless_header_hashes)

		// Sync to GRANDPA round graphs
		if n.nodeSelf.grandpa != nil {
			n.nodeSelf.grandpa.SyncBlockToRoundGraphs(block)
		}
	}

	return nil
}
func (n *NodeContent) cacheWorkReport(workReport types.WorkReport) {
	n.workReportsMutex.Lock()
	defer n.workReportsMutex.Unlock()
	n.workReports[workReport.Hash()] = workReport
}

func (n *NodeContent) cacheWorkReportRead(h common.Hash) (workReport types.WorkReport, ok bool) {
	n.workReportsMutex.Lock()
	defer n.workReportsMutex.Unlock()
	workReport, ok = n.workReports[h]
	return
}

func (n *Node) runBlocksTickets() {
	for {
		select {
		case ticket := <-n.ticketsCh:
			if n.GetIsSync() {
				n.processTicket(ticket)
			} else {
				log.Info(log.Node, "runBlocksTickets: Node is not in sync, ignoring ticket", "n", n.String(), "ticket", ticket.TicketID)
			}
		}
	}
}

var tmpBlockHash = common.Hash{}

func (n *Node) runReceiveBlock() {
	pulseTicker := time.NewTicker(100 * time.Millisecond)
	defer pulseTicker.Stop()

	for {
		select {
		case blockAnnouncement := <-n.blockAnnouncementsCh:
			// SmallTimeout to processBlockAnnouncement
			received_blk_hash := blockAnnouncement.Header.Hash()
			received_blk_slot := blockAnnouncement.Header.Slot
			latest_block := n.GetLatestBlockInfo()
			newinfo := JAMSNP_BlockInfo{
				HeaderHash: received_blk_hash,
				Slot:       received_blk_slot,
			}
			if latest_block == nil || received_blk_slot > latest_block.Slot {
				n.SetLatestBlockInfo(&newinfo, "latest block")
			} else {
				continue //once it's enough, we don't need to process it;
			}
			start := time.Now()
			var timeout time.Duration
			if n.GetIsSync() {
				timeout = SmallTimeout
			} else {
				timeout = VeryLargeTimeout
			}
			blockCtx, cancel := context.WithTimeout(context.Background(), timeout) // use very large timeout tmply since we have 30 blocks to catch up
			blocks, err := n.processBlockAnnouncement(blockCtx, blockAnnouncement)
			cancel()
			processBlockAnnouncementElapsed := common.ElapsedStr(start)
			if err != nil {
				log.Warn(log.B, "processBlockAnnouncement failed", "n", n.String(), "err", err)
				n.SetIsSync(false, "fail to get latest block")
				continue
			} else {
				if len(blocks) == 0 { // check
					block := blocks[0]
					log.Trace(log.B, "processBlock",
						"author", blockAnnouncement.Header.AuthorIndex,
						"p", common.Str(block.GetParentHeaderHash()),
						"h", common.Str(block.Header.Hash()),
						"b", block.Str(),
						"goroutines", runtime.NumGoroutine())
				}
				for _, block := range blocks {
					log.Debug(log.B, "GetBlock",
						"author", blockAnnouncement.Header.AuthorIndex,
						"p", common.Str(block.GetParentHeaderHash()),
						"h", common.Str(block.Header.Hash()),
						"t", block.Header.Slot,
					)
				}

			}
			extendCtx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
			start = time.Now()
			if err := n.extendChain(extendCtx); err != nil {
				log.Warn(log.B, "runReceiveBlock: extendChain failed", "n", n.String(), "err", err)
			}
			extendChainElapsed := common.ElapsedStr(start)
			n.jce_timestamp_mutex.Lock()
			block_to_apply := time.Since(n.jce_timestamp[blockAnnouncement.Header.Slot])
			n.jce_timestamp_mutex.Unlock()
			if extendChainElapsed > time.Second {
				log.Debug(log.B, "runReceiveBlock: extendChain time", "n", n.String(), "takes", extendChainElapsed, "takes to apply", block_to_apply, "processBlockAnnouncement", processBlockAnnouncementElapsed)
			}
			if GrandpaEasy {
				n.block_tree.EasyFinalization()
				if tmpBlockHash != n.block_tree.GetLastFinalizedBlock().Block.Header.HeaderHash() {
					// new finalized block
					block := n.block_tree.GetLastFinalizedBlock().Block
					n.telemetryClient.FinalizedBlockChanged(block.Header.Slot, tmpBlockHash)
					tmpBlockHash = block.Header.Hash()
				}
			}
			go func() {
				latst_finalized_block := n.block_tree.GetLastFinalizedBlock()
				block := latst_finalized_block.Block
				if block != nil {
					err := n.StoreFinalizedBlock(block)
					if err != nil {
						log.Warn(log.B, "runReceiveBlock: StoreFinalizedBlock failed", "n", n.String(), "err", err)
					}
				}
			}()
			cancel()
		case <-pulseTicker.C:
			// MediumTimeout to extend the chain
		case <-n.stop_receive_blk:
			log.Trace(log.B, "runReceiveBlock: received stop signal", "n", n.String())
			select {
			case <-n.restart_receive_blk:
				log.Trace(log.B, "runReceiveBlock: restart signal received", "n", n.String())
			}
		}
	}
}

func (n *Node) runWorkReports() {
	for {
		select {
		case workReport := <-n.workReportsCh:
			if n.workReports == nil {
				n.workReports = make(map[common.Hash]types.WorkReport)
			}
			n.cacheWorkReport(workReport)
		}
	}

}

func (n *Node) runGuarantees() {
	for {
		select {
		case guarantee := <-n.guaranteesCh:
			if n.GetIsSync() {
				err := n.processGuarantee(guarantee, "runGuarantees-Ch")
				if err != nil {
					if statedb.AcceptableGuaranteeError(err) {
						log.Warn(log.G, "runGuarantees:processGuarantee", "n", n.String(), "problem", err, "TODO", "ignoring...")
					} else {
						log.Error(log.G, "runGuarantees:processGuarantee", "n", n.String(), "err", err)
					}
				}
			}
		}
	}
}

func (n *Node) runAssurances() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case assurance := <-n.assurancesCh:
			n.statedbMapMutex.Lock()
			_, statedbExists := n.statedbMap[assurance.Anchor]
			n.statedbMapMutex.Unlock()

			if !statedbExists {
				n.queueAssuranceMutex.Lock()
				if _, ok2 := n.queueAssurance[assurance.Anchor]; !ok2 {
					n.queueAssurance[assurance.Anchor] = make(map[types.Ed25519Key]AssuranceObject)
				}
				n.queueAssurance[assurance.Anchor][assurance.Ed25519Key] = assurance
				n.queueAssuranceMutex.Unlock()
				continue
			}
			err := n.processAssurance(assurance)
			if err != nil {
				fmt.Printf("%s processAssurance: %v\n", n.String(), err)
			}
		case <-ticker.C:
			n.queueAssuranceMutex.Lock()
			for anchor, assuranceMap := range n.queueAssurance {
				n.statedbMapMutex.Lock()
				_, statedbExists := n.statedbMap[anchor]
				n.statedbMapMutex.Unlock()

				if statedbExists {
					for _, assurance := range assuranceMap {
						err := n.processAssurance(assurance)
						if err != nil {
							fmt.Printf("%s processAssurance from queue: %v\n", n.String(), err)
						} else {
							delete(n.queueAssurance[anchor], assurance.Ed25519Key)
						}
					}
					if len(n.queueAssurance[anchor]) == 0 {
						delete(n.queueAssurance, anchor)
					}
				}
			}
			n.queueAssuranceMutex.Unlock()
		}
	}
}

func (n *Node) RunRPCCommand() {
	for {
		select {
		case command := <-n.command_chan:
			fmt.Printf("Get new command from RPC: %v\n", command)
			switch command {
			case "slot_update":
				for i, rpc_client := range n.RPC_Client {
					var res string
					err := rpc_client.Call("jam.SetTimeSlotReady", []string{fmt.Sprintf("%d", CurrentSlot)}, &res)
					if err != nil {
						fmt.Printf("RPC call failed: %v\n", err)
					} else {
						fmt.Printf("[N%d] RPC call success: %v\n", i, res)
					}
				}
				CurrentSlot++

			case "stop":
				fmt.Printf("Stop block receive\n")
				n.restart_receive_blk = make(chan string)
				n.stop_receive_blk = make(chan string)
				n.stop_receive_blk <- "stop"
			case "restart":
				fmt.Printf("Restart block receive\n")
				n.restart_receive_blk <- "restart"
			}
		}
	}
}

var CurrentSlot = uint32(12)

// sendRequestByPubKey sends a request to a peer identified by their pubKey (stable identity).
func (n *NodeContent) sendRequestByPubKey(ctx context.Context, pubKey types.Ed25519Key, obj interface{}, evID ...uint64) (resp interface{}, err error) {
	var eventID uint64
	if len(evID) > 0 {
		eventID = evID[0]
	}

	msgType := getMessageType(obj)
	if msgType == "unknown" {
		return nil, fmt.Errorf("unknown type: %s", msgType)
	}

	// Check if this is a self-request
	selfPubKey := n.GetEd25519Key()
	isSelfRequest := pubKey == selfPubKey

	// DEBUG: Log request details
	log.Trace(log.Node, "sendRequestByPubKey DEBUG", "n", n.String(),
		"selfPubKey", selfPubKey.String()[:16],
		"targetPubKey", pubKey.String()[:16],
		"msgType", msgType,
		"isSelfRequest", isSelfRequest,
		"eventID", eventID)

	switch msgType {
	case "CE128_request":
		req := obj.(CE128_request)
		headerHash := req.HeaderHash
		direction := req.Direction
		maximumBlocks := req.MaximumBlocks
		if isSelfRequest {
			return nil, fmt.Errorf("selfRequesting not supported for CE128")
		}
		peer, ok := n.GetPeerByPubKey(pubKey)
		if !ok {
			return resp, fmt.Errorf("peer not found for pubkey %s", pubKey.String()[:16])
		}
		blocks, err := peer.SendBlockRequest(ctx, headerHash, direction, maximumBlocks)
		if err != nil {
			return resp, err
		}
		response := CE128_response{
			HeaderHash: headerHash,
			Direction:  direction,
			Blocks:     blocks,
		}
		return response, nil

	case "CE138_request":
		req := obj.(CE138_request)
		erasureRoot := req.ErasureRoot

		log.Trace(log.DA, "CE138_request", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex", req.ShardIndex, "coreIdx", req.CoreIndex)

		// handle selfRequesting case - only if shard actually belongs to us
		selfValidatorIdx := n.statedb.GetSafrole().GetCurrValidatorIndex(selfPubKey)
		myShardIdx := storage.ComputeShardIndex(req.CoreIndex, uint16(selfValidatorIdx))
		if isSelfRequest && req.ShardIndex == myShardIdx {
			log.Trace(log.DA, "CE138_request: selfRequesting", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex", req.ShardIndex)
			bundleShard, sClub, encodedPath, _, err := n.GetBundleShard_Assurer(req.ErasureRoot, req.ShardIndex)
			if err != nil {
				log.Error(log.DA, "CE138_request: selfRequesting ERROR", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex", req.ShardIndex, "ERR", err)
				return resp, err
			}
			self_response := CE138_response{
				ShardIndex:    req.ShardIndex,
				BundleShard:   bundleShard,
				SClub:         sClub,
				Justification: encodedPath,
			}
			log.Trace(log.DA, "CE138_request: selfRequesting OK", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex", req.ShardIndex, "CE138_response", types.ToJSONHex(self_response))
			return self_response, nil
		}

		peer, ok := n.GetPeerByPubKey(pubKey)
		if !ok {
			log.Error(log.DA, "CE138_request: peer not found for pubkey", "n", n.String(), "pubKey", pubKey.String()[:16])
			return resp, fmt.Errorf("peer not found for pubkey %s", pubKey.String()[:16])
		}
		log.Trace(log.DA, "CE138_request: SendBundleShardRequest", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex", req.ShardIndex, "peerKey", peer.SanKey())

		startTime := time.Now()
		bundleShard, sClub, encodedPath, err := peer.SendBundleShardRequest(ctx, erasureRoot, req.ShardIndex, eventID)
		rtt := time.Since(startTime)
		log.Debug(log.DA, "CE138_request: SendBundleShardRequest RTT", "n", n.String(), "pubKey", pubKey.String()[:16], "rtt", rtt)
		if err != nil {
			log.Trace(log.DA, "CE138_request: SendBundleShardRequest ERROR", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex", req.ShardIndex, "ERR", err)
			return resp, err
		}
		response := CE138_response{
			ShardIndex:    req.ShardIndex,
			BundleShard:   bundleShard,
			SClub:         sClub,
			Justification: encodedPath,
		}
		log.Trace(log.DA, "CE138_request: SendBundleShardRequest OK", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex", req.ShardIndex, "CE138_response", types.ToJSONHex(response))
		return response, nil

	case "CE139_request":
		req := obj.(CE139_request)
		erasureRoot := req.ErasureRoot
		segmentIndices := req.SegmentIndices

		log.Trace(log.DA, "CE139_request", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex", req.ShardIndex, "coreIdx", req.CoreIndex)

		// handle selfRequesting case - only if shard actually belongs to us
		selfValidatorIdx := n.statedb.GetSafrole().GetCurrValidatorIndex(selfPubKey)
		myShardIdx := storage.ComputeShardIndex(req.CoreIndex, uint16(selfValidatorIdx))
		if isSelfRequest && req.ShardIndex == myShardIdx {
			selected_segmentshards, selected_justifications, ok, err := n.GetSegmentShard_Assurer(erasureRoot, req.ShardIndex, segmentIndices, false)
			if err != nil {
				return resp, err
			}
			if !ok {
				return resp, fmt.Errorf("GetSegmentShard_Assurer failed")
			}
			combined_segmentShards := bytes.Join(selected_segmentshards, nil)
			self_response := CE139_response{
				ErasureRoot:           erasureRoot,
				ShardIndex:            req.ShardIndex,
				SegmentShards:         combined_segmentShards,
				SegmentJustifications: selected_justifications,
			}
			return self_response, nil
		}

		peer, ok := n.GetPeerByPubKey(pubKey)
		if !ok {
			return resp, fmt.Errorf("peer not found for pubkey %s", pubKey.String()[:16])
		}
		segmentShards, selected_justifications, err := peer.SendSegmentShardRequest(ctx, erasureRoot, req.ShardIndex, segmentIndices, false, eventID)
		if err != nil {
			return resp, err
		}
		response := CE139_response{
			ErasureRoot:           erasureRoot,
			ShardIndex:            req.ShardIndex,
			SegmentShards:         segmentShards,
			SegmentJustifications: selected_justifications,
		}
		return response, nil

	default:
		return nil, fmt.Errorf("unsupported type: %s", msgType)
	}
}

// makeRequestsByPubKey sends requests to peers identified by their pubKey (stable identity).
// The map key is pubKey, which never changes across epoch rotations.
func (n *NodeContent) makeRequestsByPubKey(
	objs map[types.Ed25519Key]interface{},
	minSuccess int,
	singleTimeout, overallTimeout time.Duration,
	evID ...uint64,
) ([]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), overallTimeout)
	defer cancel()
	var eventID uint64
	if len(evID) > 0 {
		eventID = evID[0]
	} else {
		eventID = 0
	}
	var (
		wg           sync.WaitGroup
		resultsCh    = make(chan interface{}, len(objs)) // still bounded
		mu           sync.Mutex
		successCount int
		finalResults []interface{}
		doneOnce     sync.Once
	)

	for pubKey, obj := range objs {
		if getMessageType(obj) == "unknown" {
			continue
		}

		wg.Add(1)
		go func(pubKey types.Ed25519Key, obj interface{}) {
			defer wg.Done()

			reqCtx, reqCancel := context.WithTimeout(ctx, singleTimeout)
			defer reqCancel()

			res, err := n.sendRequestByPubKey(reqCtx, pubKey, obj, eventID)
			if err != nil {
				log.Trace(log.DA, "sendRequestByPubKey failed", "pubKey", pubKey.String()[:16], "err", err)
				return
			}

			// Send result or return if parent context is canceled
			if ctx.Err() != nil {
				return
			}
			resultsCh <- res
		}(pubKey, obj)
	}

	// Streaming collection
	doneCh := make(chan struct{})
	go func() {
		for res := range resultsCh {
			mu.Lock()
			finalResults = append(finalResults, res)
			successCount++
			if successCount >= minSuccess && ctx.Err() == nil {
				doneOnce.Do(cancel)
			}
			mu.Unlock()
		}
		close(doneCh)
	}()

	// Wait for all request goroutines to finish
	wg.Wait()
	close(resultsCh)

	// Wait for the collector goroutine to finish
	<-doneCh

	mu.Lock()
	sc := successCount
	mu.Unlock()

	log.Trace(log.DA, "makeRequestsByPubKey: successCount", sc)

	if sc < minSuccess {
		return nil, fmt.Errorf("not enough successful requests (successCount:%d < minSuccess:%d)", sc, minSuccess)
	}
	return finalResults, nil
}
