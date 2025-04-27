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

func (n *Node) processBlockAnnouncement(ctx context.Context, blockAnnouncement JAMSNP_BlockAnnounce) ([]types.Block, error) {
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
	parentHash := blockAnnouncement.Header.ParentHeaderHash
	var mode int
	var num uint32
	if _, ok := n.block_tree.GetBlockNode(parentHash); ok {
		mode = 0
	} else if len(n.block_tree.TreeMap) == 1 {
		mode = 1
	} else {
		finalized_block := n.block_tree.GetLastFinalizedBlock()
		finalized_block_slot := finalized_block.Block.Header.Slot
		if finalized_block_slot < blockAnnouncement.Header.Slot {
			num = blockAnnouncement.Header.Slot - finalized_block_slot
			mode = 2
		} else {
			return nil, errors.New("block announcement is too old")
		}
	}
	var lastErr error
	var blocksRaw []types.Block
	for attempt := 1; attempt <= 3; attempt++ {
		// Respect cancellation early
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("processBlockAnnouncement canceled: %w, attempt %v, mode %v", ctx.Err(), attempt, mode)
		default:
		}

		// Small timeout context per attempt
		attemptCtx, cancel := context.WithTimeout(ctx, NormalTimeout)
		defer cancel()
		switch mode {
		case 0:
			blocksRaw, lastErr = p.GetOneBlock(headerHash, attemptCtx)
		case 1:
			blocksRaw, lastErr = p.GetAllBlocks(headerHash, attemptCtx)
			log.Warn(log.BlockMonitoring, "GetAllBlocks", "blockHash", headerHash, "blocksRaw", len(blocksRaw), "isSync", false)
			n.SetIsSync(false, "no blocks")
		case 2:
			blocksRaw, lastErr = p.GetMiddleBlocks(headerHash, num, attemptCtx)
			log.Warn(log.BlockMonitoring, "GetMiddleBlocks",
				"num", num,
				"blockHash", headerHash, "blocksRaw", blocksRaw, "isSync", false)
			n.SetIsSync(false, "behind others")
		default:
			return nil, fmt.Errorf("invalid mode %d", mode)
		}
		cancel()

		if lastErr == nil && len(blocksRaw) > 0 {
			if attempt > 1 {
				// if attempt >1, we can try to find a new peer
				log.Info(module, "SendBlockRequest succeeded", "attempt", attempt, "blockHash", headerHash)
				origin := validatorIndex
				// find a new peer
				for {
					validatorIndex = uint16(rand0.Intn(len(n.peersInfo)))
					if validatorIndex == origin || validatorIndex == n.id {
						continue
					} else {
						break
					}
				}
				p, ok = n.peersInfo[validatorIndex]
				if !ok {
					err := fmt.Errorf("invalid validator index %d", validatorIndex)
					log.Error(module, "processBlockAnnouncement", "err", err)
					return nil, err
				}
			}
			break
		}

		if attempt == 3 {
			log.Warn(module, "SendBlockRequest failed after 3 attempts", "blockHash", headerHash, "lastErr", lastErr)
			return nil, fmt.Errorf("SendBlockRequest failed after 3 attempts: %w", lastErr)
		}
	}
	for i := len(blocksRaw) - 1; i >= 0; i-- {
		block := &blocksRaw[i]
		err := n.processBlock(block)
		if err != nil {
			log.Error(module, "processBlockAnnouncement", "err", err, "mode", mode, "blockHash", block.Header.Hash())
			return nil, err
		}
	}
	for _, peer := range n.peersInfo {
		go func(peer *Peer) {
			if peer.PeerID == n.id {
				return
			}
			peer_id := peer.PeerID
			if !n.ba_checker.CheckAndSet(headerHash, peer_id) {
				up0_stream, err := peer.GetOrInitBlockAnnouncementStream(context.Background())
				if err != nil {
					log.Trace(debugStream, "GetOrInitBlockAnnouncementStream", "n", n.String(), "->p", peer.PeerID, "err", err)
					return
				}
				block_a_bytes := blockAnnouncement.ToBytes()

				err = sendQuicBytes(context.Background(), up0_stream, block_a_bytes, peer_id, CE128_BlockRequest)
				if err != nil {
					log.Warn(debugStream, "SendBlockAnnouncement:sendQuicBytes (whisper)", "n", n.String(), "err", err)
				}
			}
		}(peer)
	}
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
		case blockAnnouncement := <-n.blockAnnouncementsCh:
			// SmallTimeout to processBlockAnnouncement
			blk_hash := blockAnnouncement.Header.Hash()

			latest_block := n.GetLatestBlockInfo()
			newinfo := JAMSNP_BlockInfo{
				HeaderHash: blk_hash,
				Slot:       blockAnnouncement.Header.Slot,
			}
			if latest_block == nil || blockAnnouncement.Header.Slot > latest_block.Slot {
				n.SetLatestBlockInfo(&newinfo, "latest block")
			}
			blockCtx, cancel := context.WithTimeout(context.Background(), SmallTimeout)
			blocks, err := n.processBlockAnnouncement(blockCtx, blockAnnouncement)
			cancel()

			if err != nil {
				log.Warn(debugBlock, "processBlockAnnouncement failed", "n", n.String(), "err", err)
				n.SetIsSync(false, "fail to get latest block")
				continue
			} else {
				if len(blocks) == 0 { // check
					block := blocks[0]
					log.Trace(debugBlock, "processBlock",
						"author", blockAnnouncement.Header.AuthorIndex,
						"p", common.Str(block.GetParentHeaderHash()),
						"h", common.Str(block.Header.Hash()),
						"b", block.Str(),
						"goroutines", runtime.NumGoroutine())
				}
			}
			extendCtx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
			start := time.Now()
			if err := n.extendChain(extendCtx); err != nil {
				log.Warn(debugBlock, "runReceiveBlock: extendChain failed", "n", n.String(), "err", err)
			}
			elapsed := time.Since(start)
			n.jce_timestamp_mutex.Lock()
			block_to_apply := time.Since(n.jce_timestamp[blockAnnouncement.Header.Slot])
			n.jce_timestamp_mutex.Unlock()
			if elapsed > time.Second {
				log.Debug(debugBlock, "runReceiveBlock: extendChain time", "n", n.String(), "takes", elapsed, "takes to apply", block_to_apply)
			}
			if GrandpaEasy {
				n.block_tree.EasyFinalization()
			}
			cancel()
		case <-pulseTicker.C:
			// MediumTimeout to extend the chain
		case <-n.stop_receive_blk:
			log.Trace(debugBlock, "runReceiveBlock: received stop signal", "n", n.String())
			select {
			case <-n.restart_receive_blk:
				log.Trace(debugBlock, "runReceiveBlock: restart signal received", "n", n.String())
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
			err := n.processGuarantee(guarantee)
			if err != nil {
				log.Error(debugG, "runGuarantees:processGuarantee", "n", n.String(), "err", err)
			}
		}
	}
}

func (n *Node) runAssurances() {
	for {
		select {
		case assurance := <-n.assurancesCh:
			err := n.processAssurance(assurance)
			if err != nil {
				fmt.Printf("%s processAssurance: %v\n", n.String(), err)
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

		startTime := time.Now()
		bundleShard, sClub, encodedPath, err := peer.SendBundleShardRequest(ctx, erasureRoot, peerID)
		rtt := time.Since(startTime)
		log.Debug(debugDA, "CE138_request: SendBundleShardRequest RTT", "n", n.String(), "peerID", peerID, "rtt", rtt)
		if err != nil {
			log.Trace(debugDA, "CE138_request: SendBundleShardRequest ERROR on resp", "n", n.String(), "erasureRoot", erasureRoot, "shardIndex(peerID)", peerID, "ERR", err)
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
		resultsCh    = make(chan interface{}, len(objs)) // still bounded
		mu           sync.Mutex
		successCount int
		finalResults []interface{}
		doneOnce     sync.Once
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
				log.Trace(debugDA, "sendRequest failed", "peerID", peerID, "err", err)
				return
			}

			select {
			case resultsCh <- res:
				// result sent successfully
			case <-ctx.Done():
				// parent canceled, don't block
				return
			}
		}(peerID, obj)
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

	log.Trace(debugDA, "makeRequests: successCount", sc)

	if sc < minSuccess {
		return nil, fmt.Errorf("not enough successful requests (successCount:%d < minSuccess:%d)", sc, minSuccess)
	}
	return finalResults, nil
}
