package node

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

type CE139_request struct {
	ErasureRoot    common.Hash
	SegmentIndices []uint16
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

func (n *NodeContent) PreimageLookup(preimageHash common.Hash) ([]byte, bool, error) {
	n.preimagesMutex.Lock()
	defer n.preimagesMutex.Unlock()

	preimage, ok := n.preimages[preimageHash]
	if !ok {
		fmt.Printf("%s preimageHash not ok\n", n.String())
		return []byte{}, false, nil
	}
	return preimage, true, nil
}

func (n *NodeContent) GetState(headerHash common.Hash, startKey [31]byte, endKey [31]byte, maximumSize uint32) (boundarynodes [][]byte, keyvalues types.StateKeyValueList, ok bool, err error) {
	s := n.getPVMStateDB()
	blocks, ok, err := n.BlocksLookup(headerHash, 1, 1)
	if !ok || err != nil {
		return boundarynodes, keyvalues, false, err
	}
	stateRoot := blocks[0].Header.ParentStateRoot
	trie := s.CopyTrieState(stateRoot)
	foundKeyVal, boundaryNode, err := trie.GetStateByRange(startKey[:], endKey[:], maximumSize)
	if err != nil {
		return boundarynodes, keyvalues, false, err
	}
	keyvalues = types.StateKeyValueList{Items: foundKeyVal}
	return boundaryNode, keyvalues, true, nil
}

func (n *Node) GetServiceIdxStorage(headerHash common.Hash, service_idx uint32, rawKey common.Hash) (boundarynodes [][]byte, keyvalues types.StateKeyValueList, ok bool, err error) {
	return n.getServiceIdxStorage(headerHash, service_idx, rawKey)
}

func (n *NodeContent) getServiceIdxStorage(headerHash common.Hash, service_idx uint32, rawKey common.Hash) (boundarynodes [][]byte, keyvalues types.StateKeyValueList, ok bool, err error) {
	s := n.getPVMStateDB()
	// stateRoot := s.GetStateRoot()
	blocks, ok, err := n.BlocksLookup(headerHash, 1, 1)
	if !ok || err != nil {
		fmt.Printf("BlocksLookup ERR %v\n", err)
		return boundarynodes, keyvalues, false, err
	}
	stateRoot := blocks[0].Header.ParentStateRoot
	stateTrie := s.CopyTrieState(stateRoot)

	storageKey := common.Compute_storageKey_internal(rawKey)
	service_account := common.ComputeC_sh(service_idx, storageKey)
	maxSize := uint32(1000000)
	foundKeyVal, boundaryNode, err := stateTrie.GetStateByRange(service_account[:], common.Hex2Bytes("0xFFFFFFFFFF"), maxSize)
	if err != nil {
		fmt.Printf("GetState ERR %v\n", err)
		return boundarynodes, keyvalues, false, err
	}
	keyvalues = types.StateKeyValueList{Items: foundKeyVal}
	return boundaryNode, keyvalues, true, nil
}

func (n *Node) processBlockAnnouncement(ctx context.Context, blockAnnouncement JAMSNP_BlockAnnounce) (*types.Block, error) {
	if n.store.SendTrace {
		tracer := n.store.Tp.Tracer("NodeTracer")
		traceCtx, span := tracer.Start(ctx, fmt.Sprintf("[N%d] processBlockAnnouncement", n.store.NodeID))
		n.store.UpdateBlockAnnouncementContext(traceCtx)
		defer span.End()
		ctx = traceCtx // Use the span context for everything that follows
	}

	validatorIndex := blockAnnouncement.Header.AuthorIndex
	p, ok := n.peersInfo[validatorIndex]
	if !ok {
		err := fmt.Errorf("invalid validator index %d", validatorIndex)
		log.Error(module, "processBlockAnnouncement", "err", err)
		return nil, err
	}

	headerHash := blockAnnouncement.Header.HeaderHash()

	var lastErr error
	var blocksRaw []types.Block
	for attempt := 1; attempt <= 3; attempt++ {
		// Respect cancellation early
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("processBlockAnnouncement canceled: %w", ctx.Err())
		default:
		}

		// Small timeout context per attempt
		attemptCtx, cancel := context.WithTimeout(ctx, SmallTimeout)
		blocksRaw, lastErr = p.SendBlockRequest(attemptCtx, headerHash, 1, 1)
		cancel()

		if lastErr == nil && len(blocksRaw) > 0 {
			if attempt > 1 {
				log.Info(module, "SendBlockRequest succeeded", "attempt", attempt, "blockHash", headerHash)
			}
			break
		}

		if attempt == 3 {
			log.Warn(module, "SendBlockRequest failed after 3 attempts", "blockHash", headerHash, "lastErr", lastErr)
			return nil, fmt.Errorf("SendBlockRequest failed after 3 attempts: %w", lastErr)
		}
	}

	block := &blocksRaw[0]
	if receivedHash := block.Header.Hash(); receivedHash != headerHash {
		err := fmt.Errorf("block hash mismatch: expected %s, got %s", headerHash.String_short(), receivedHash.String_short())
		log.Error(module, "processBlockAnnouncement", "err", err)
		return nil, err
	}

	n.processBlock(block)
	return block, nil
}

func (n *NodeContent) cacheBlock(block *types.Block) error {

	if block != nil && n.block_tree != nil { // check
		err := n.block_tree.AddBlock(block)
		if err != nil {
			return fmt.Errorf("cacheBlock: AddBlock failed %v", err)
		}
		// also prune the block tree
		n.block_tree.PruneBlockTree(10)
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
			n.processTicket(ticket)
		}
	}
}

func (n *Node) runReceiveBlock() {
	pulseTicker := time.NewTicker(100 * time.Millisecond)
	defer pulseTicker.Stop()

	for {
		select {

		case <-pulseTicker.C:
			// MediumTimeout to extend the chain
			extendCtx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
			if err := n.extendChain(extendCtx); err != nil {
				log.Warn(debugBlock, "runReceiveBlock: extendChain failed", "n", n.String(), "err", err)
			}
			cancel()

		case blockAnnouncement := <-n.blockAnnouncementsCh:
			// SmallTimeout to processBlockAnnouncement
			blockCtx, cancel := context.WithTimeout(context.Background(), SmallTimeout)
			block, err := n.processBlockAnnouncement(blockCtx, blockAnnouncement)
			cancel()

			if err != nil {
				log.Warn(debugBlock, "processBlockAnnouncement failed", "n", n.String(), "err", err)
			} else {
				log.Info(debugBlock, "processBlock",
					"author", blockAnnouncement.Header.AuthorIndex,
					"p", common.Str(block.GetParentHeaderHash()),
					"h", common.Str(block.Header.Hash()),
					"b", block.Str(),
					"goroutines", runtime.NumGoroutine())
			}

		case <-n.stop_receive_blk:
			log.Trace(debugBlock, "runReceiveBlock: received stop signal", "n", n.String())
			select {
			case <-n.restart_receive_blk:
				log.Trace(debugBlock, "runReceiveBlock: restart signal received", "n", n.String())
			}
		}
	}
}

func (n *Node) runMain() {
	for {
		select {
		case workReport := <-n.workReportsCh:
			if n.workReports == nil {
				n.workReports = make(map[common.Hash]types.WorkReport)
			}
			n.cacheWorkReport(workReport)
		case guarantee := <-n.guaranteesCh:
			err := n.processGuarantee(guarantee)
			if err != nil {
				log.Error(debugG, "runMain:processGuarantee", "n", n.String(), "err", err)
				// COMMON ERROR: G15|AnchorNotRecent
			}
		case assurance := <-n.assurancesCh:
			err := n.processAssurance(assurance)
			if err != nil {
				fmt.Printf("%s processAssurance: %v\n", n.String(), err)
			}
		case announcement := <-n.announcementsCh:
			err := n.processAnnouncement(announcement)
			if err != nil {
				fmt.Printf("%s processAnnouncement: %v\n", n.String(), err)
			}
		case judgement := <-n.judgementsCh:
			err := n.processJudgement(judgement)
			if err != nil {
				fmt.Printf("%s processJudgement: %v\n", n.String(), err)
			}
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

// process request
func (n *NodeContent) sendRequest(ctx context.Context, peerID uint16, obj interface{}) (resp interface{}, err error) {
	// Get the peer ID from the object
	msgType := getMessageType(obj)
	if msgType == "unknown" {
		return nil, fmt.Errorf("unknown type: %s", msgType)
	}
	switch msgType {

	case "CE128_request":
		req := obj.(CE128_request)
		headerHash := req.HeaderHash
		direction := req.Direction
		maximumBlocks := req.MaximumBlocks
		if peerID == uint16(n.id) { // selfRequesting case
			// no reason to even get here
			return nil, fmt.Errorf("selfRequesting not supported")
		}
		peer, err := n.getPeerByIndex(peerID)
		if err != nil {
			return resp, err
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

		// handle selfRequesting case
		if peerID == uint16(n.id) {
			log.Trace(debugDA, "CE138_request: selfRequesting", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex", req.ShardIndex)
			bundleShard, sClub, encodedPath, _, err := n.GetBundleShard_Assurer(req.ErasureRoot, req.ShardIndex)
			if err != nil {
				log.Error(debugDA, "CE138_request: selfRequesting ERROR", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex(self)", n.id, "ERR", err)
				return resp, err
			}
			self_response := CE138_response{
				ShardIndex:    n.id,
				BundleShard:   bundleShard,
				SClub:         sClub,
				Justification: encodedPath,
			}
			log.Trace(debugDA, "CE138_request: selfRequesting OK", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex(peerID)", peerID, "CE138_response", types.ToJSONHex(self_response))
			return self_response, nil
		}

		peer, err := n.getPeerByIndex(peerID)
		if err != nil {
			log.Error(debugDA, "CE138_request: SendBundleShardRequest ERROR on getPeerByIndex", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex(peerID)", peerID, "ERR", err)
			return resp, err
		}
		log.Trace(debugDA, "CE138_request: SendBundleShardRequest", "n", n.String(), "erasureRoot", erasureRoot, "Req peer shardIndex", peerID, "peer.PeerID", peer.PeerID)
		bundleShard, sClub, encodedPath, err := peer.SendBundleShardRequest(ctx, erasureRoot, peerID)
		if err != nil {
			log.Error(debugDA, "CE138_request: SendBundleShardRequest ERROR on resp", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex(peerID)", peerID, "ERR", err)
			return resp, err
		}
		response := CE138_response{
			ShardIndex:    req.ShardIndex,
			BundleShard:   bundleShard,
			SClub:         sClub,
			Justification: encodedPath,
		}
		log.Trace(debugDA, "CE138_request: SendBundleShardRequest OK", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex(peerID)", peerID, "CE138_response", types.ToJSONHex(response))
		return response, nil

	case "CE139_request":
		req := obj.(CE139_request)
		peerID = req.ShardIndex
		erasureRoot := req.ErasureRoot
		segmentIndices := req.SegmentIndices
		peer, err := n.getPeerByIndex(peerID)
		if err != nil {
			return resp, err
		}
		// handle selfRequesting case
		if req.ShardIndex == uint16(n.id) {
			selected_segmentshards, selected_justifications, ok, err := n.GetSegmentShard_Assurer(erasureRoot, peerID, segmentIndices, false)
			if err != nil {
				return resp, err
			}
			if !ok {
				return resp, fmt.Errorf("GetSegmentShard_Assurer failed")
			}
			combined_segmentShards := bytes.Join(selected_segmentshards, nil)
			self_response := CE139_response{
				ErasureRoot:           erasureRoot,
				ShardIndex:            n.id,
				SegmentShards:         combined_segmentShards,
				SegmentJustifications: selected_justifications,
			}
			return self_response, nil
		}
		segmentShards, selected_justifications, err := peer.SendSegmentShardRequest(ctx, erasureRoot, peerID, segmentIndices, false)
		if err != nil {
			return resp, err
		}
		response := CE139_response{
			ErasureRoot:           erasureRoot,
			ShardIndex:            peerID,
			SegmentShards:         segmentShards,
			SegmentJustifications: selected_justifications,
		}
		return response, nil

	default:
		return nil, fmt.Errorf("unsupported type: %s", msgType)

	}
}

func (n *NodeContent) makeRequests(
	objs map[uint16]interface{},
	minSuccess int,
	singleTimeout, overallTimeout time.Duration,
) ([]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), overallTimeout)
	defer cancel()

	var (
		wg           sync.WaitGroup
		resultsCh    = make(chan interface{}, len(objs))
		successCount int
		mu           sync.Mutex
	)

	for peerID, obj := range objs {
		if getMessageType(obj) == "unknown" {
			continue
		}

		wg.Add(1)
		go func(peerID uint16, obj interface{}) {
			defer wg.Done()

			reqCtx, reqCancel := context.WithTimeout(ctx, singleTimeout)
			defer reqCancel()

			res, err := n.sendRequest(reqCtx, peerID, obj)
			if err != nil {
				return
			}

			select {
			case resultsCh <- res:
				mu.Lock()
				successCount++
				// NOTE: some of successCount could be "malicious" so this is not the right criteria --
				// INSTEAD: (a) reconstruct the bundle/segment and (b) verify the bundle (CE137), segment (CE139+140) or block (CE128)
				if successCount >= minSuccess && ctx.Err() == nil {
					cancel() // IMPORTANT: this results in ALL the reqCancel() since reqCtx has ctx as its parent
				}
				mu.Unlock()
			case <-ctx.Done():
			}
		}(peerID, obj)
	}

	wg.Wait()
	close(resultsCh)

	var finalResults []interface{}
	for res := range resultsCh {
		finalResults = append(finalResults, res)
	}

	mu.Lock()
	sc := successCount
	mu.Unlock()

	log.Trace(debugDA, "makeRequests: successCount", sc)
	if sc < minSuccess {
		return nil, fmt.Errorf("not enough successful requests (successCount:%d < minSuccess:%d)", sc, minSuccess)
	}
	return finalResults, nil
}
