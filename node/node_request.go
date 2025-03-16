package node

import (
	"bytes"
	"context"
	"fmt"
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
	ShardIndex    uint16
	HeaderHash    common.Hash `json:"headerHash"`
	Direction     uint8       `json:"direction"`
	MaximumBlocks uint32      `json:"maximumBlocks"`
}

type CE128_response struct {
	ShardIndex uint16
	HeaderHash common.Hash `json:"headerHash"`
	Direction  uint8       `json:"direction"`
	Blocks     []types.Block
}

func (n *Node) OnHandshake(validatorIndex uint16, headerHash common.Hash, timeslot uint32, leaves []types.ChainLeaf) (err error) {
	// TODO: Sourabh
	fmt.Println("OnHandshake")
	return nil
}

func (n *Node) GetBlockByHeaderHash(headerHash common.Hash) (*types.Block, error) {
	blk, cachedFound := n.cacheHeadersRead(headerHash)
	if cachedFound {
		return blk, nil
	}
	blk, err := n.GetStoredBlockByHeader(headerHash)
	if err != nil {
		return blk, err
	}
	return blk, nil
}

func (n *Node) BlocksLookup(headerHash common.Hash, direction uint8, maximumBlocks uint32) (blocks []types.Block, ok bool, err error) {
	blocks = make([]types.Block, 0)
	if direction == 0 {
		//Direction = 0 (Ascending exclusive)  - child, grandchild, ...
		//panic("does not support Ascending exclusive fetch")

		currentHash := headerHash
		for i := uint32(0); i < maximumBlocks; i++ {
			childBlksWithFork, err := n.GetAscendingBlockByHeader(currentHash)
			if err != nil {
				return blocks, false, err
			}

			// not sure which path to ascend to ... pick first one for now...
			if len(childBlksWithFork) > 1 {
				panic(fmt.Sprintf("BlocksLookup: multiple childBlksWithFork found %v\n", childBlksWithFork))
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
			blocks = append(blocks, *blk)
			if blk.Header.ParentHeaderHash == (common.Hash{}) {
				break
			}
			currentHash = blk.Header.ParentHeaderHash
		}
	}
	return blocks, true, nil
}

func (n *Node) WorkReportLookup(workReportHash common.Hash) (workReport types.WorkReport, ok bool, err error) {
	workReport, found := n.cacheWorkReportRead(workReportHash)
	if found {
		return workReport, true, nil
	}
	return workReport, false, nil

}

func (n *Node) PreimageLookup(preimageHash common.Hash) ([]byte, bool, error) {
	n.preimagesMutex.Lock()
	defer n.preimagesMutex.Unlock()

	preimage, ok := n.preimages[preimageHash]
	if !ok {
		fmt.Printf("%s preimageHash not ok\n", n.String())
		return []byte{}, false, nil
	}
	return preimage, true, nil
}

func (n *Node) GetState(headerHash common.Hash, startKey [31]byte, endKey [31]byte, maximumSize uint32) (boundarynodes [][]byte, keyvalues types.StateKeyValueList, ok bool, err error) {
	// TODO: Stanley
	s := n.getPVMStateDB()
	// stateRoot := s.GetStateRoot()
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

func (n *Node) getServiceIdxStorage(headerHash common.Hash, service_idx uint32, rawKey common.Hash) (boundarynodes [][]byte, keyvalues types.StateKeyValueList, ok bool, err error) {
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

func (n *Node) IsSelfRequesting(peer_id uint16) bool {
	if peer_id == n.id {
		return true
	}
	return false
}

func (n *Node) processBlockAnnouncement(blockAnnouncement JAMSNP_BlockAnnounce) (block *types.Block, err error) {
	if n.store.SendTrace {
		tracer := n.store.Tp.Tracer("NodeTracer")
		ctx, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] processBlockAnnouncement", n.store.NodeID))
		n.store.UpdateBlockAnnouncementContext(ctx)
		defer span.End()
	}

	// initiate CE128_BlockRequest
	validatorIndex := blockAnnouncement.Header.AuthorIndex
	p, ok := n.peersInfo[validatorIndex]
	if !ok {
		fmt.Printf("processBlockAnnouncement %d NOT Found\n", validatorIndex)
		for i, p := range n.peersInfo {
			fmt.Printf("%d => %s\n", i, p.PeerAddr)
		}
		panic(120)
		return block, fmt.Errorf("Invalid validator index %d", validatorIndex)
	}
	headerHash := blockAnnouncement.Header.HeaderHash()
	var blocksRaw []types.Block
	for attempt := 1; attempt <= 3; attempt++ {
		blocksRaw, err = p.SendBlockRequest(headerHash, 1, 1)
		if err == nil {
			if attempt > 1 {
				fmt.Printf("%s processBlockAnnouncement:SendBlockRequest(%v) attempt %d success\n", n.String(), headerHash, attempt)
			}
			break // exit loop if request succeeds
		}
		//time.Sleep(100)
		if attempt == 3 {
			fmt.Printf("%s processBlockAnnouncement:SendBlockRequest(%v) failed after 3 attempts\n", n.String(), headerHash)
			return nil, err
		}
	}

	block = &blocksRaw[0]
	receivedHeaderHash := block.Header.Hash()
	if receivedHeaderHash != headerHash {
		panic(6665)
		return block, fmt.Errorf("failed header hash retrieval")
	}
	n.cacheHeaders(receivedHeaderHash, block)
	n.cacheBlock(block)
	return block, nil
}

func (n *Node) cacheBlockRead(parentHash common.Hash) (b *types.Block, ok bool) {
	n.blocksMutex.Lock()
	defer n.blocksMutex.Unlock()
	b, ok = n.blocks[parentHash]
	return b, ok
}

func (n *Node) cacheBlock(block *types.Block) {
	n.blocksMutex.Lock()
	defer n.blocksMutex.Unlock()
	n.blocks[block.GetParentHeaderHash()] = block
}

func (n *Node) cacheHeadersRead(h common.Hash) (b *types.Block, ok bool) {
	n.headersMutex.Lock()
	defer n.headersMutex.Unlock()
	b, ok = n.headers[h]
	return b, ok
}

func (n *Node) cacheHeaders(h common.Hash, block *types.Block) {
	n.headersMutex.Lock()
	defer n.headersMutex.Unlock()
	n.headers[h] = block
	// fmt.Printf("  %s cacheHeaders %v <- %v\n", n.String(), h, block.Header.ParentHeaderHash)
}

func (n *Node) cacheWorkReport(workReport types.WorkReport) {
	n.workReportsMutex.Lock()
	defer n.workReportsMutex.Unlock()
	n.workReports[workReport.Hash()] = workReport
}

func (n *Node) cacheWorkReportRead(h common.Hash) (workReport types.WorkReport, ok bool) {
	n.workReportsMutex.Lock()
	defer n.workReportsMutex.Unlock()
	workReport, ok = n.workReports[h]
	return
}

func (n *Node) runBlocksTickets() {
	// ticker here to avoid high CPU usage
	pulseTicker := time.NewTicker(20 * time.Millisecond)
	defer pulseTicker.Stop()

	for {
		select {
		case <-pulseTicker.C:
			// Small pause to reduce CPU load when channels are quiet
		case ticket := <-n.ticketsCh:
			n.processTicket(ticket)
		}
	}
}

func (n *Node) runReceiveBlock() {
	// ticker here to avoid high CPU usage
	pulseTicker := time.NewTicker(20 * time.Millisecond)
	defer pulseTicker.Stop()

	for {
		select {
		case <-pulseTicker.C:
			// Small pause to reduce CPU load when channels are quiet
		case blockAnnouncement := <-n.blockAnnouncementsCh:
			b, err := n.processBlockAnnouncement(blockAnnouncement)
			if err != nil {
				fmt.Printf("%s processBlockAnnouncement ERR %v\n", n.String(), err)
			} else {
				log.Trace(debugBlock, fmt.Sprintf("%s Received Block Announcement from validator %d", n.String(), blockAnnouncement.Header.AuthorIndex), "p", common.Str(b.GetParentHeaderHash()), "h", common.Str(b.Header.Hash()), "t", b.Header.Slot)
				n.processBlock(b)
			}
		}
	}
}

func (n *Node) runMain() {
	// ticker here to avoid high CPU usage
	pulseTicker := time.NewTicker(20 * time.Millisecond)
	defer pulseTicker.Stop()

	for {
		select {
		case <-pulseTicker.C:
			// Small pause to reduce CPU load when channels are quiet
		case workReport := <-n.workReportsCh:
			if n.workReports == nil {
				n.workReports = make(map[common.Hash]types.WorkReport)
			}
			n.cacheWorkReport(workReport)
		case guarantee := <-n.guaranteesCh:
			err := n.processGuarantee(guarantee)
			if err != nil {
				log.Error(debugG, "runMain:processGuarantee", "n", n.String(), "err", err, "guarantee.Report", guarantee.Report.String())
			}
		case assurance := <-n.assurancesCh:
			err := n.processAssurance(assurance)
			if err != nil {
				fmt.Printf("%s processAssurance: %v\n", n.String(), err)
			}
		case announcement := <-n.announcementsCh:
			// TODO: Shawn to review
			err := n.processAnnouncement(announcement)
			if err != nil {
				fmt.Printf("%s processAnnouncement: %v\n", n.String(), err)
			}
		case judgement := <-n.judgementsCh:
			// TODO: Shawn to review
			err := n.processJudgement(judgement)
			if err != nil {
				fmt.Printf("%s processJudgement: %v\n", n.String(), err)
			}
		}
	}
}

// process request
func (n *Node) sendRequest(obj interface{}) (resp interface{}, err error) {

	// Get the peer ID from the object
	msgType := getMessageType(obj)
	var peerID uint16
	if msgType == "unknown" {
		return nil, fmt.Errorf("unknown type: %s", msgType)
	}
	switch msgType {
	case "DA_request":
		req := obj.(DA_request)
		peerID = req.ShardIndex
		shard_hash := req.Hash
		peer, err := n.getPeerByIndex(peerID)
		if err != nil {
			return resp, err
		}
		// handle selfRequesting case
		isSelfRequesting := req.ShardIndex == uint16(n.id)
		if isSelfRequesting {
			value, ok := n.chunkMap.Load(req.Hash)
			if !ok {
				self_response := DA_response{
					Hash:       shard_hash,
					ShardIndex: peerID,
					Data:       nil,
				}
				return self_response, fmt.Errorf("hash %v not found in chunkMap", req.Hash)
			}
			chunk := value.([]byte)
			data := chunk
			self_response := DA_response{
				Hash:       shard_hash,
				ShardIndex: peerID,
				Data:       data,
			}
			return self_response, nil
		}
		data, shard_idx, err := peer.DA_Reconstruction(req)
		if err != nil {
			return resp, err
		}
		response := DA_response{
			Hash:       shard_hash,
			ShardIndex: shard_idx,
			Data:       data,
		}
		return response, nil

	case "CE128_request":
		req := obj.(CE128_request)
		peerID = req.ShardIndex
		headerHash := req.HeaderHash
		direction := req.Direction
		maximumBlocks := req.MaximumBlocks
		// handle selfRequesting case
		isSelfRequesting := req.ShardIndex == uint16(n.id)
		if isSelfRequesting {
			// no reason to even get here
			return nil, fmt.Errorf("selfRequesting not supported")
		}
		peer, err := n.getPeerByIndex(peerID)
		if err != nil {
			return resp, err
		}
		blocks, err := peer.SendBlockRequest(headerHash, direction, maximumBlocks)
		if err != nil {
			return resp, err
		}
		response := CE128_response{
			HeaderHash: headerHash,
			ShardIndex: peerID,
			Direction:  direction,
			Blocks:     blocks,
		}
		return response, nil

	case "CE138_request":
		req := obj.(CE138_request)
		peerID = req.ShardIndex
		erasureRoot := req.ErasureRoot

		// handle selfRequesting case
		isSelfRequesting := req.ShardIndex == uint16(n.id)
		if isSelfRequesting {
			bundleShard, sClub, encodedPath, _, err := n.GetBundleShard_Assurer(req.ErasureRoot, req.ShardIndex)
			if err != nil {
				return resp, err
			}
			self_response := CE138_response{
				ShardIndex:    req.ShardIndex,
				BundleShard:   bundleShard,
				SClub:         sClub,
				Justification: encodedPath,
			}
			return self_response, nil
		}

		peer, err := n.getPeerByIndex(peerID)
		if err != nil {
			return resp, err
		}
		bundleShard, sClub, encodedPath, err := peer.SendBundleShardRequest(erasureRoot, peerID)
		if err != nil {
			return resp, err
		}
		response := CE138_response{
			ShardIndex:    req.ShardIndex,
			BundleShard:   bundleShard,
			SClub:         sClub,
			Justification: encodedPath,
		}
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
		isSelfRequesting := req.ShardIndex == uint16(n.id)
		if isSelfRequesting {
			selected_segmentshards, ok, err := n.GetSegmentShard_AssurerSimple(erasureRoot, peerID, segmentIndices)
			if err != nil {
				return resp, err
			}
			if !ok {
				return resp, fmt.Errorf("GetSegmentShard_AssurerSimple failed")
			}
			combined_segmentShards := bytes.Join(selected_segmentshards, nil)
			self_response := CE139_response{
				ErasureRoot:   erasureRoot,
				ShardIndex:    n.id,
				SegmentShards: combined_segmentShards,
				//SegmentJustifications: segmentJustifications -- not ready yet
			}
			return self_response, nil
		}
		segmentShards, _, err := peer.SendSegmentShardRequest(erasureRoot, peerID, segmentIndices, false)
		if err != nil {
			return resp, err
		}
		response := CE139_response{
			ErasureRoot:   erasureRoot,
			ShardIndex:    peerID,
			SegmentShards: segmentShards,
			// SegmentJustifications: segmentJustifications -- not ready yet
		}
		return response, nil

	default:
		return nil, fmt.Errorf("unsupported type: %s", msgType)

	}
}

// internal makeRquest call with ctx implementation
func (n *Node) makeRequestInternal(ctx context.Context, obj interface{}) (interface{}, error) {

	// Channel to receive the response or error
	responseCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)

	// Goroutine to handle the request
	go func() {
		response, err := n.sendRequest(obj)
		if err != nil {
			errCh <- err
		} else {
			responseCh <- response
		}
	}()

	msgType := getMessageType(obj)
	if msgType == "unknown" {
		return nil, fmt.Errorf("unsupported type")
	}
	select {
	case <-ctx.Done():
		// Context was canceled before the request completed
		return nil, ctx.Err()
	case err := <-errCh:
		// Request encountered an error
		return nil, err
	case response := <-responseCh:
		// Request succeeded
		return response, nil
	}
}

// single makeRequest call via makeRequestInternal
//	ctx, cancel := context.WithTimeout(context.Background(), singleTimeout)
//	defer cancel()

// plural makeRequest calls via makeRequestInternal, with a minSuccess required because cancelling other simantanteous req
func (n *Node) makeRequests(objs []interface{}, minSuccess int, singleTimeout, overallTimeout time.Duration) ([]interface{}, error) {
	var wg sync.WaitGroup
	results := make(chan interface{}, len(objs))
	errorsCh := make(chan error, len(objs))
	var mu sync.Mutex
	successCount := 0

	// Create a cancellable context -- this is the parent ctx
	ctx, cancel := context.WithTimeout(context.Background(), overallTimeout)
	defer cancel()
	for _, obj := range objs {
		wg.Add(1)
		go func(obj interface{}) {
			defer wg.Done()

			// Create a context with a 2-second timeout for each individual request
			// This is the child context derived from ctx.
			reqCtx, reqCancel := context.WithTimeout(ctx, singleTimeout)
			defer reqCancel()

			res, err := n.makeRequestInternal(reqCtx, obj)
			if err != nil {
				errorsCh <- err
				return
			}

			select {
			case results <- res:
				mu.Lock()
				successCount++
				if successCount >= minSuccess {
					cancel() // Cancel remaining requests once minSuccess is reached, include its childCtx
				}
				mu.Unlock()
			case <-ctx.Done():
				return
			}
		}(obj)
	}

	// Wait for all requests to finish
	wg.Wait()
	log.Trace(debugDA, "makeRequests: Receive DONE", "n", n.id)
	close(results)
	close(errorsCh)

	var finalResults []interface{}
	for res := range results {
		finalResults = append(finalResults, res)
	}
	log.Trace(debugDA, "makeRequests: successCount", successCount)
	// If not enough successful responses
	if successCount < minSuccess {
		return nil, fmt.Errorf("not enough successful requests (successCount:%d < minSuccess:%d)", successCount, minSuccess)
	}
	//TODO..need somekind of sorting here..
	return finalResults, nil
}
