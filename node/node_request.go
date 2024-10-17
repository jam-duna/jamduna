package node

import (
	//"bytes"
	//"context"
	//"errors"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
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
	}
	//for h, _ := range n.headers {
	//	fmt.Printf(" %s BlocksLookup(%v) INSPECT: %v\n", n.String(), headerHash, h)
	//}
	return blocks, false, nil
}

func (n *Node) WorkReportLookup(workReportHash common.Hash) (workReport types.WorkReport, ok bool, err error) {
	workReport, found := n.workReports[workReportHash]
	if found {
		return workReport, true, nil
	}
	return workReport, false, nil

}

func (n *Node) PreimageLookup(preimageHash common.Hash) ([]byte, bool, error) {
	// TODO: William to review
	preimage, ok := n.preimages[preimageHash]
	if !ok {
		return []byte{}, false, nil
	}
	return preimage, true, nil
}

func (n *Node) GetState(headerHash common.Hash, startKey [31]byte, endKey [31]byte, maximumSize uint32) (boundarynodes [][]byte, keyvalues types.StateKeyValue, ok bool, err error) {
	// TODO: Stanley
	return boundarynodes, keyvalues, false, nil
}

func (n *Node) IsSelfRequesting(peerIdentifier string) bool {
	ed25519key := n.GetEd25519Key()
	if ed25519key.String() == peerIdentifier {
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
	blockRaw, err := p.SendBlockRequest(headerHash, 0, 1)
	if err != nil {
		fmt.Printf("processBlockAnnouncement ERR %v\n", err)
		panic(123)
		return block, err
	}
	block = &blockRaw
	receivedHeaderHash := block.Header.Hash()
	if receivedHeaderHash != headerHash {
		panic(6665)
		return block, fmt.Errorf("failed header hash retrieval")
	}
	n.headers[receivedHeaderHash] = block
	n.blocks[block.ParentHash()] = block
	//fmt.Printf("  %s received Block %v <- %v\n", n.String(), block.Hash(), block.ParentHash())
	return block, nil
}

func (n *Node) processPreimageAnnouncements(preimageAnnouncement types.PreimageAnnouncement) (err error) {
	// initiate CE143_PreimageRequest
	validatorIndex := preimageAnnouncement.ValidatorIndex
	p, ok := n.peersInfo[validatorIndex]
	if !ok {
		return fmt.Errorf("Invalid validator index %d", validatorIndex)
	}
	preimageHash := preimageAnnouncement.PreimageHash
	preimage, err := p.SendPreimageRequest(preimageAnnouncement.PreimageHash)
	if err != nil {
		return err
	}
	n.preimages[preimageHash] = preimage
	return nil
}

func (n *Node) AddNewImportSegments(treeRoot common.Hash, num int, workpackage_hash common.Hash) error {
	if n.segments == nil {
		n.segments = make(map[common.Hash][]types.ImportSegment)
	}
	segments := make([]types.ImportSegment, num)
	for i := 0; i < num; i++ {
		segments[i] = types.ImportSegment{
			TreeRoot: treeRoot,
			Index:    uint16(i),
		}
	}
	n.segments[workpackage_hash] = segments
	return nil
}

func (n *Node) runMain() {
	for {
		select {
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
		case workPackage := <-n.workPackagesCh:
			_, _, treeRoot, err := n.executeWorkPackage(workPackage)
			if err != nil {
				fmt.Printf("executeWorkPackage: %v\n", err)
			}
			// TODO: Michael+Sourabh to discuss
			exportedsegmentsNum := 1
			err = n.AddNewImportSegments(treeRoot, exportedsegmentsNum, workPackage.Hash())
			if err != nil {
				fmt.Printf("AddNewImportSegments: %v\n", err)
			}
		case workReport := <-n.workReportsCh:
			if n.workReports == nil {
				n.workReports = make(map[common.Hash]types.WorkReport)
			}
			n.workReports[workReport.Hash()] = workReport
		case guarantee := <-n.guaranteesCh:
			err := n.processGuarantee(guarantee)
			if err != nil {
				fmt.Printf("processGuarantee: %v\n", err)
			}
		case assurance := <-n.assurancesCh:
			err := n.processAssurance(assurance)
			if err != nil {
				fmt.Printf("processAssurance: %v\n", err)
			}
		case preimageAnnouncement := <-n.preimageAnnouncementsCh:
			// TODO: William to review
			err := n.processPreimageAnnouncements(preimageAnnouncement)
			if err != nil {
				fmt.Printf("processPreimages: %v\n", err)
			}
		case announcement := <-n.announcementsCh:
			// TODO: Shawn to review
			err := n.processAnnouncement(announcement)
			if err != nil {
				fmt.Printf("processAnnouncement: %v\n", err)
			}
		case judgement := <-n.judgementsCh:
			// TODO: Shawn to review
			err := n.processJudgement(judgement)
			if err != nil {
				fmt.Printf("processJudgement: %v\n", err)
			}
		}
	}
}

/*
// internal makeRquest call with ctx implementation
func (n *Node) makeRequestInternal(ctx context.Context, peerIdentifier string, obj interface{}) ([]byte, error) {
	peerAddr, _ := n.getPeerAddr(peerIdentifier)
	peerID, _ := n.getPeerIndex(peerIdentifier)
	msgType := getMessageType(obj)
	if msgType == "unknown" {
		return nil, fmt.Errorf("unsupported type")
	}

	if n.IsSelfRequesting(peerIdentifier) {
		// for self requesting, no need to open stream channel..
		fmt.Printf("[N%v] %v Self Requesting!!!\n", n.id, msgType)

		// Channel to receive the response or error
		responseCh := make(chan []byte, 1)
		errCh := make(chan error, 1)

		// Run handleQuicMsg in a goroutine to handle context cancellation
		go func() {

			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
			default:
				responseCh <- response
				errCh <- nil
			}
		}()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-errCh:
			if err != nil {
				return nil, err
			}
			response := <-responseCh
			return response, nil
		}
	}

	n.connectionMu.Lock()
	conn, exists := n.connections[peerAddr]
	if exists {
		// Check if the connection is closed
		if conn.Context().Err() != nil {
			// Connection is closed, remove it from the cache
			delete(n.connections, peerAddr)
			exists = false
			conn = nil
		}
	}
	if !exists {
		var err error
		conn, err = quic.DialAddr(ctx, peerAddr, n.tlsConfig, generateQuicConfig())
		if err != nil {
			n.connectionMu.Unlock()
			fmt.Printf("-- [N%v] makeRequest ERR %v peerAddr=%s (N%v)\n", n.id, err, peerAddr, peerID)
			return nil, err
		}
		n.connections[peerAddr] = conn
	}
	n.connectionMu.Unlock()

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		fmt.Printf("-- [N%v] openstreamsync ERR %v\n", n.id, err)
		// Error opening stream, remove the connection from the cache
		n.connectionMu.Lock()
		delete(n.connections, peerAddr)
		n.connectionMu.Unlock()
		return nil, err
	}
	defer stream.Close()

	// Length-prefix the message
	messageLength := uint32(len(messageData))
	lengthPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthPrefix, messageLength)

	// Write the length and message data with context cancellation support
	writeErrCh := make(chan error, 1)
	go func() {
		_, err = stream.Write(lengthPrefix)
		if err != nil {
			writeErrCh <- err
			return
		}
		_, err = stream.Write(messageData)
		writeErrCh <- err
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-writeErrCh:
		if err != nil {
			fmt.Printf("-- [N%v] Write ERR %v\n", n.id, err)
			return nil, err
		}
	}

	// Read the response with context cancellation support
	responseCh := make(chan []byte, 1)
	readErrCh := make(chan error, 1)

	go func() {
		var buffer bytes.Buffer
		tmp := make([]byte, 4096)
		for {
			nRead, err := stream.Read(tmp)
			if nRead > 0 {
				buffer.Write(tmp[:nRead])
			}
			if err != nil {
				if err.Error() == "EOF" {
					responseCh <- buffer.Bytes()
					readErrCh <- nil
				} else {
					readErrCh <- err
				}
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-readErrCh:
		if err != nil {
			fmt.Printf("-- [N%v] Read ERR %v\n", n.id, err)
			return nil, err
		}
		response := <-responseCh
		return response, nil
	}
}
*/
// single makeRequest call via makeRequestInternal
//	ctx, cancel := context.WithTimeout(context.Background(), singleTimeout)
//	defer cancel()

/*
// plural makeRequest calls via makeRequestInternal, with a minSuccess required because cancelling other simantanteous req
func (n *Node) makeRequests(peerIdentifier string, objs []interface{}, minSuccess int, singleTimeout, overallTimeout time.Duration) ([][]byte, error) {
	var wg sync.WaitGroup
	results := make(chan []byte, len(objs))
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

			res, err := n.makeRequestInternal(reqCtx, peerIdentifier, obj)
			if err != nil {
				errorsCh <- err
				return
			}

			select {
			case results <- res:
				mu.Lock()
				fmt.Printf("SUCCESS %d\n", successCount)
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
	fmt.Printf("DONE\n")
	close(results)
	close(errorsCh)

	var finalResults [][]byte
	for res := range results {
		fmt.Printf("res %s\n", res)
		finalResults = append(finalResults, res)
	}
	fmt.Println(successCount, "successCount")
	// If not enough successful responses
	if successCount < minSuccess {
		return nil, errors.New("not enough successful requests")
	}
	//TODO..need somekind of sorting here..
	return finalResults, nil
}
*/
