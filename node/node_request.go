package node

import (
	//"bytes"
	//"context"
	//"errors"

	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	//"github.com/colorfulnotion/jam/trie"
)

func (n *Node) OnHandshake(validatorIndex uint16, headerHash common.Hash, timeslot uint32, leaves []types.ChainLeaf) (err error) {
	// TODO: Sourabh
	fmt.Println("OnHandshake")
	return nil
}

func (n *Node) BlocksLookup(headerHash common.Hash, direction uint8, maximumBlocks uint32) (blocks []types.Block, ok bool, err error) {
	blocks = make([]types.Block, 0)
	//fmt.Printf("%s BlocksLookup(%v) requested\n", n.String(), headerHash)
	blk, found := n.headers[headerHash]
	if found {
		blocks = append(blocks, *blk)
		// TODO: Sourabh - go in the direction up to maximumBlocks
		return blocks, true, nil
	} else {
		//fmt.Printf("%s BlocksLookup %v\n", n.String(), headerHash)
		blkFromDB, err := n.GetBlockByHeader(headerHash)
		if err != nil {
			return blocks, false, err
		}
		blocks = append(blocks, blkFromDB)
		return blocks, true, nil
	}
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
	blocks, ok, err := n.BlocksLookup(headerHash, 0, 1)
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

func (n *Node) GetServiceIdxStorage(headerHash common.Hash, service_idx uint32, key common.Hash) (boundarynodes [][]byte, keyvalues types.StateKeyValueList, ok bool, err error) {
	return n.getServiceIdxStorage(headerHash, service_idx, key)
}

func (n *Node) getServiceIdxStorage(headerHash common.Hash, service_idx uint32, key common.Hash) (boundarynodes [][]byte, keyvalues types.StateKeyValueList, ok bool, err error) {
	s := n.getPVMStateDB()
	// stateRoot := s.GetStateRoot()
	blocks, ok, err := n.BlocksLookup(headerHash, 0, 1)
	if !ok || err != nil {
		fmt.Printf("BlocksLookup ERR %v\n", err)
		return boundarynodes, keyvalues, false, err
	}
	stateRoot := blocks[0].Header.ParentStateRoot
	stateTrie := s.CopyTrieState(stateRoot)
	service_account := common.ComputeC_sh(service_idx, key)
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

func (n *Node) processBlockAnnouncement(blockAnnouncement types.BlockAnnouncement) (block *types.Block, err error) {
	// initiate CE128_BlockRequest
	validatorIndex := blockAnnouncement.ValidatorIndex
	p, ok := n.peersInfo[validatorIndex]
	if !ok {
		fmt.Printf("processBlockAnnouncement %d NOT Found\n", validatorIndex)
		for i, p := range n.peersInfo {
			fmt.Printf("%d => %s\n", i, p.PeerAddr)
		}
		panic(120)
		return block, fmt.Errorf("Invalid validator index %d", validatorIndex)
	}
	headerHash := blockAnnouncement.HeaderHash
	//fmt.Printf("%s processBlockAnnouncement:SendBlockRequest(%v) to N%d\n", n.String(), headerHash, validatorIndex)
	var blockRaw types.Block
	for attempt := 1; attempt <= 3; attempt++ {
		blockRaw, err = p.SendBlockRequest(headerHash, 0, 1)
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

	block = &blockRaw
	receivedHeaderHash := block.Header.Hash()
	if receivedHeaderHash != headerHash {
		panic(6665)
		return block, fmt.Errorf("failed header hash retrieval")
	}
	n.cacheHeaders(receivedHeaderHash, block)
	n.cacheBlock(block)
	//fmt.Printf("  %s received Block %v <- %v\n", n.String(), block.Hash(), block.ParentHash())
	return block, nil
}

func (n *Node) cacheBlockRead(parentHash common.Hash) (b *types.Block, ok bool) {
	n.blocksMutex.Lock()
	defer n.blocksMutex.Unlock()
	b, ok = n.blocks[parentHash]
	return
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
	return
}

func (n *Node) cacheHeaders(h common.Hash, block *types.Block) {
	n.headersMutex.Lock()
	defer n.headersMutex.Unlock()
	n.headers[h] = block
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
		case blockAnnouncement := <-n.blockAnnouncementsCh:
			//fmt.Printf("[N%d] received Block Announcement from %d\n", n.id, blockAnnouncement.ValidatorIndex)
			b, err := n.processBlockAnnouncement(blockAnnouncement)
			if err != nil {
				fmt.Printf("%s processBlockAnnouncement ERR %v\n", n.String(), err)
			} else {
				n.processBlock(b)
			}
		case ticket := <-n.ticketsCh:
			n.processTicket(ticket)
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
		case workPackage := <-n.workPackagesCh:
			importSegments, err := n.FetchWorkpackageImportSegments(workPackage)
			if err != nil {
				// fmt.Printf("FetchWorkpackageImportSegments: %v\n", err)
			}
			g, _, _, err := n.executeWorkPackage(workPackage, importSegments)
			wr := g.Report
			if err != nil {
				fmt.Printf("executeWorkPackage: %v\n", err)
			} else {
				n.workReportsCh <- wr
			}
		case workReport := <-n.workReportsCh:
			if n.workReports == nil {
				n.workReports = make(map[common.Hash]types.WorkReport)
			}
			n.cacheWorkReport(workReport)
		case guarantee := <-n.guaranteesCh:
			err := n.processGuarantee(guarantee)
			if err != nil {
				fmt.Printf("%s processGuarantee: %v\n", n.String(), err)
			}
		case assurance := <-n.assurancesCh:
			err := n.processAssurance(&assurance)
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

// internal makeRquest call with ctx implementation
func (n *Node) makeRequestInternal(ctx context.Context, obj interface{}) (interface{}, error) {
	// Get the peer ID from the object
	writeErrCh := make(chan error, 1)
	msgType := getMessageType(obj)
	var peerID uint16
	if msgType == "unknown" {
		return nil, fmt.Errorf("unsupported type")
	}
	switch msgType {
	case "DA_request":

		req := obj.(DA_request)
		peerID = req.ShardIndex
		shard_hash := req.Hash
		peer, err := n.getPeerByIndex(peerID)
		if err != nil {
			writeErrCh <- err
		}
		data, v_idx, err := peer.DA_Reconstruction(req)
		if err != nil {
			writeErrCh <- err
		}
		var response DA_response
		response.Hash = shard_hash
		response.ShardIndex = v_idx
		response.Data = data
		return response, nil
	// case "CE138_request":
	// 	req := obj.(CE138_request)
	// 	peerID = req.ShardIndex
	// 	shard_hash := req.WorkPackageHash
	// 	shardIndex := req.ShardIndex
	// 	peer, err := n.getPeerByIndex(peerID)
	// 	if err != nil {
	// 		writeErrCh <- err
	// 	}
	// 	// func (p *Peer) SendSegmentShardRequest(erasureRoot common.Hash, shardIndex uint16, segmentIndex []uint16, withJustification bool) (segmentShards []byte, justifications [][]byte, err error)
	// 	erasureRoot, err := n.getErasureRootFromHash(shard_hash)
	// 	if err != nil {
	// 		fmt.Printf("getErasureRootFromHash: %v\n", err)
	// 		writeErrCh <- err
	// 	}
	// 	segmentShards, justifications, err := peer.SendSegmentShardRequest(erasureRoot, peerID, segmentIndex, false)
	// 	response := CE138_response{
	// 		WorkPackageHash: shard_hash,
	// 		segmentShards:   segmentShards,
	// 		justifications:  justifications,
	// 	}
	// 	return response, nil
	case "CE139_request":
		req := obj.(CE139_request)
		peerID = req.ShardIndex
		shard_hash := req.WorkPackageHash
		segmentIndices := req.SegmentIndices
		peer, err := n.getPeerByIndex(peerID)
		if err != nil {
			writeErrCh <- err
		}
		// func (p *Peer) SendSegmentShardRequest(erasureRoot common.Hash, shardIndex uint16, segmentIndex []uint16, withJustification bool) (segmentShards []byte, justifications [][]byte, err error)
		erasureRoot, err := n.getErasureRootFromHash(shard_hash)
		if err != nil {
			fmt.Printf("getErasureRootFromHash: %v\n", err)
			writeErrCh <- err
		}
		segmentShards, _, err := peer.SendSegmentShardRequest(erasureRoot, peerID, segmentIndices, true)
		if err != nil {
			fmt.Printf("SendSegmentShardRequest: %v\n", err)
			writeErrCh <- err
		}
		response := CE139_response{
			WorkPackageHash: shard_hash,
			ShardIndex:      peerID,
			segmentShards:   segmentShards,
		}
		return response, nil

	}
	// select {
	// case <-ctx.Done():
	// 	return nil, ctx.Err()
	// case err := <-writeErrCh:
	// 	if err != nil {
	// 		fmt.Printf("-- [N%v] Write ERR %v\n", n.id, err)
	// 		return nil, err
	// 	}
	// }
	//n.chunkCh <- Chunk{Hash: shard_hash, Data: data}
	// IMPORTANT: We handle self requesting inside the DA_Request
	// if n.IsSelfRequesting(peerID) {
	// 	// for self requesting, no need to open stream channel..
	// 	fmt.Printf("[N%v] %v Self Requesting!!!\n", n.id, msgType)

	// 	// Channel to receive the response or error
	// 	responseCh := make(chan []byte, 1)
	// 	errCh := make(chan error, 1)

	// 	// Run handleQuicMsg in a goroutine to handle context cancellation
	// 	go func() {

	// 		select {
	// 		case <-ctx.Done():
	// 			errCh <- ctx.Err()
	// 		default:
	// 			responseCh <- response
	// 			errCh <- nil
	// 		}
	// 	}()

	// 	select {
	// 	case <-ctx.Done():
	// 		return nil, ctx.Err()
	// 	case err := <-errCh:
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		response := <-responseCh
	// 		return response, nil
	// 	}
	// }

	// Write the length and message data with context cancellation support

	// go func() {
	// 	_, err = stream.Write(lengthPrefix)
	// 	if err != nil {
	// 		writeErrCh <- err
	// 		return
	// 	}
	// 	_, err = stream.Write(messageData)
	// 	writeErrCh <- err
	// }()

	// Read the response with context cancellation support
	// responseCh := make(chan []byte, 1)
	// readErrCh := make(chan error, 1)

	// select {
	// case <-ctx.Done():
	// 	// we get into this case if (1) internal(individual)request is complete OR parent
	// 	return nil, ctx.Err()
	// case err := <-readErrCh:
	// 	if err != nil {
	// 		fmt.Printf("-- [N%v] Read ERR %v\n", n.id, err)
	// 		return nil, err
	// 	}
	// 	// when we receive
	// 	response := <-responseCh
	// 	return response, nil
	// }
	return nil, fmt.Errorf("unsupported type")
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
				// fmt.Printf("SUCCESS %d\n", successCount)
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
	if debugSegments {
		fmt.Printf("[N%d] Receive DONE\n", n.id)
	}
	close(results)
	close(errorsCh)

	var finalResults []interface{}
	for res := range results {
		finalResults = append(finalResults, res)
	}
	if debugSegments {
		fmt.Println(successCount, "successCount")
	}
	// If not enough successful responses
	if successCount < minSuccess {
		return nil, errors.New("not enough successful requests")
	}
	//TODO..need somekind of sorting here..
	return finalResults, nil
}
