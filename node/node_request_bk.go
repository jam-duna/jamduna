package node

//
// import (
// 	"bytes"
// 	"context"
// 	"errors"
// 	"fmt"
// 	"sync"
// 	"time"
//
// 	"encoding/binary"
// 	"io"
// 	"log"
// 	"reflect"
//
// 	"github.com/colorfulnotion/jam/common"
// 	"github.com/colorfulnotion/jam/types"
// 	"github.com/quic-go/quic-go"
// )
//
// // internal makeRquest call with ctx implementation
// func (n *Node) makeRequestInternal(ctx context.Context, peerIdentifier string, obj interface{}) ([]byte, error) {
// 	peerAddr, _ := n.getPeerAddr(peerIdentifier)
// 	peerID, _ := n.getPeerIndex(peerIdentifier)
// 	msgType := getMessageType(obj)
// 	if msgType == "unknown" {
// 		return nil, fmt.Errorf("unsupported type")
// 	}
//
// 	messageData := EncodeAsQuicMessage(obj, n.id)
// 	if n.IsSelfRequesting(peerIdentifier) {
// 		// for self requesting, no need to open stream channel..
// 		fmt.Printf("[N%v] %v Self Requesting!!!\n", n.id, msgType)
// 		QuicMsg := DecodeAsQuicMessage(messageData)
//
// 		// Channel to receive the response or error
// 		responseCh := make(chan []byte, 1)
// 		errCh := make(chan error, 1)
//
// 		// Run handleQuicMsg in a goroutine to handle context cancellation
// 		go func() {
// 			_, response := n.handleQuicMsg(QuicMsg)
// 			select {
// 			case <-ctx.Done():
// 				errCh <- ctx.Err()
// 			default:
// 				responseCh <- response
// 				errCh <- nil
// 			}
// 		}()
//
// 		select {
// 		case <-ctx.Done():
// 			return nil, ctx.Err()
// 		case err := <-errCh:
// 			if err != nil {
// 				return nil, err
// 			}
// 			response := <-responseCh
// 			return response, nil
// 		}
// 	}
//
// 	n.connectionMu.Lock()
// 	conn, exists := n.connections[peerAddr]
// 	if exists {
// 		// Check if the connection is closed
// 		if conn.Context().Err() != nil {
// 			// Connection is closed, remove it from the cache
// 			delete(n.connections, peerAddr)
// 			exists = false
// 			conn = nil
// 		}
// 	}
// 	if !exists {
// 		var err error
// 		conn, err = quic.DialAddr(ctx, peerAddr, n.tlsConfig, generateQuicConfig())
// 		if err != nil {
// 			n.connectionMu.Unlock()
// 			fmt.Printf("-- [N%v] makeRequest ERR %v peerAddr=%s (N%v)\n", n.id, err, peerAddr, peerID)
// 			return nil, err
// 		}
// 		n.connections[peerAddr] = conn
// 	}
// 	n.connectionMu.Unlock()
//
// 	stream, err := conn.OpenStreamSync(ctx)
// 	if err != nil {
// 		fmt.Printf("-- [N%v] openstreamsync ERR %v\n", n.id, err)
// 		// Error opening stream, remove the connection from the cache
// 		n.connectionMu.Lock()
// 		delete(n.connections, peerAddr)
// 		n.connectionMu.Unlock()
// 		return nil, err
// 	}
// 	defer stream.Close()
//
// 	// Length-prefix the message
// 	messageLength := uint32(len(messageData))
// 	lengthPrefix := make([]byte, 4)
// 	binary.BigEndian.PutUint32(lengthPrefix, messageLength)
//
// 	// Write the length and message data with context cancellation support
// 	writeErrCh := make(chan error, 1)
// 	go func() {
// 		_, err = stream.Write(lengthPrefix)
// 		if err != nil {
// 			writeErrCh <- err
// 			return
// 		}
// 		_, err = stream.Write(messageData)
// 		writeErrCh <- err
// 	}()
//
// 	select {
// 	case <-ctx.Done():
// 		return nil, ctx.Err()
// 	case err := <-writeErrCh:
// 		if err != nil {
// 			fmt.Printf("-- [N%v] Write ERR %v\n", n.id, err)
// 			return nil, err
// 		}
// 	}
//
// 	// Read the response with context cancellation support
// 	responseCh := make(chan []byte, 1)
// 	readErrCh := make(chan error, 1)
//
// 	go func() {
// 		var buffer bytes.Buffer
// 		tmp := make([]byte, 4096)
// 		for {
// 			nRead, err := stream.Read(tmp)
// 			if nRead > 0 {
// 				buffer.Write(tmp[:nRead])
// 			}
// 			if err != nil {
// 				if err.Error() == "EOF" {
// 					responseCh <- buffer.Bytes()
// 					readErrCh <- nil
// 				} else {
// 					readErrCh <- err
// 				}
// 				return
// 			}
// 		}
// 	}()
//
// 	select {
// 	case <-ctx.Done():
// 		return nil, ctx.Err()
// 	case err := <-readErrCh:
// 		if err != nil {
// 			fmt.Printf("-- [N%v] Read ERR %v\n", n.id, err)
// 			return nil, err
// 		}
// 		response := <-responseCh
// 		return response, nil
// 	}
// }
//
// // single makeRequest call via makeRequestInternal
// func (n *Node) makeRequest(peerIdentifier string, obj interface{}, singleTimeout time.Duration) ([]byte, error) {
// 	ctx, cancel := context.WithTimeout(context.Background(), singleTimeout)
// 	defer cancel()
//
// 	res, err := n.makeRequestInternal(ctx, peerIdentifier, obj)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return res, nil
// }
//
// // plural makeRequest calls via makeRequestInternal, with a minSuccess required because cancelling other simantanteous req
// func (n *Node) makeRequests(peerIdentifiers []string, objs []interface{}, minSuccess int, singleTimeout, overallTimeout time.Duration) ([][]byte, error) {
// 	var wg sync.WaitGroup
// 	results := make(chan []byte, len(objs))
// 	errorsCh := make(chan error, len(objs))
// 	var mu sync.Mutex
// 	successCount := 0
//
// 	if (len(peerIdentifiers) != len(objs)){
// 		panic("Invalid makeRequests batch!")
// 	}
// 	// Create a cancellable context -- this is the parent ctx
// 	ctx, cancel := context.WithTimeout(context.Background(), overallTimeout)
// 	defer cancel()
//
// 	for i, obj := range objs {
// 		wg.Add(1)
// 		go func(obj interface{}) {
// 			defer wg.Done()
//
// 			// Create a context with a 2-second timeout for each individual request
// 			// This is the child context derived from ctx.
// 			reqCtx, reqCancel := context.WithTimeout(ctx, singleTimeout)
// 			defer reqCancel()
//
// 			res, err := n.makeRequestInternal(reqCtx, peerIdentifiers[i], obj)
// 			if err != nil {
// 				errorsCh <- err
// 				return
// 			}
//
// 			select {
// 			case results <- res:
// 				mu.Lock()
// 				//fmt.Printf("SUCCESS %d\n", successCount)
// 				successCount++
// 				if successCount >= minSuccess {
// 					cancel() // Cancel remaining requests once minSuccess is reached, include its childCtx
// 				}
// 				mu.Unlock()
// 			case <-ctx.Done():
// 				return
// 			}
// 		}(obj)
// 	}
//
// 	// Wait for all requests to finish
// 	wg.Wait()
// 	fmt.Printf("DONE\n")
// 	close(results)
// 	close(errorsCh)
//
// 	var finalResults [][]byte
// 	for res := range results {
// 		//fmt.Printf("res %s\n", res)
// 		finalResults = append(finalResults, res)
// 	}
// 	fmt.Println(successCount, "successCount")
// 	// If not enough successful responses
// 	if successCount < minSuccess {
// 		return nil, errors.New("not enough successful requests")
// 	}
// 	//TODO..need somekind of sorting here..
// 	return finalResults, nil
// }
//
// func (n *Node) handleQuicMsg(msg QuicMessage) (msgType string, response []byte) {
// 	ok := []byte("0")
// 	response = []byte("1")
// 	switch msg.MsgType {
// 	case "BlockQuery":
// 		var query types.BlockQuery
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(query))
// 		if err != nil {
// 			fmt.Printf("BlockQuery Decode Error: %v\n", err)
// 		}
// 		query = decoded.(types.BlockQuery)
// 		blk, found := n.blocks[query.BlockHash]
// 		if debug {
// 			fmt.Printf("[N%d] Received BlockQuery %v found: %v\n", n.id, query.BlockHash, found)
// 		}
// 		if found {
// 			serializedR, err := types.Encode(blk)
// 			if err != nil {
// 				fmt.Printf("BlockQuery Encode Error: %v\n", err)
// 			}
// 			//serializedR := types.Encode(blk)
// 			if err == nil {
// 				if debug {
// 					fmt.Printf("[N%d] Responded to BlockQuery %v with len=%v\n", n.id, query.BlockHash, len(serializedR))
// 				}
// 				response = serializedR
// 			}
// 		}
//
// 	case "Ticket":
// 		var ticket types.Ticket
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(ticket))
// 		if err != nil {
// 			fmt.Printf("Ticket Decode Error: %v\n", err)
// 		}
// 		ticket = decoded.(types.Ticket)
// 		err = n.processTicket(ticket)
// 		if err == nil {
// 			response = ok
// 		}
// 		if debug {
// 			fmt.Printf(" -- [N%d] received ticket From N%d\n", n.id, msg.Id)
// 		}
// 	case "AvailabilityJustification":
// 		var aj *types.AvailabilityJustification
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(aj))
// 		if err != nil {
// 			fmt.Printf("AvailabilityJustification Decode Error: %v\n", err)
// 		}
// 		aj = decoded.(*types.AvailabilityJustification)
// 		if err == nil {
// 			err = n.processAvailabilityJustification(aj)
// 			if err == nil {
// 				response = ok
// 			}
// 		}
// 	case "Guarantee":
// 		var guarantee types.Guarantee
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(guarantee))
// 		if err != nil {
// 			fmt.Printf("Guarantee Decode Error: %v\n", err)
// 		}
// 		guarantee = decoded.(types.Guarantee)
// 		if err == nil {
// 			err = n.processGuarantee(guarantee)
// 			if err == nil {
// 				response = ok
// 			}
// 		}
// 		fmt.Printf(" -- [N%d] received guarantee From N%d\n", n.id, msg.Id)
// 	case "Assurance":
// 		var assurance types.Assurance
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(assurance))
// 		if err != nil {
// 			fmt.Printf("Assurance Decode Error: %v\n", err)
// 		}
// 		assurance = decoded.(types.Assurance)
// 		if err == nil {
// 			err = n.processAssurance(assurance)
// 			if err == nil {
// 				response = ok
// 			}
// 		}
// 		fmt.Printf(" -- [N%d] received assurance From N%d\n", n.id, msg.Id)
// 	case "Judgement":
// 		var judgement types.Judgement
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(judgement))
// 		if err != nil {
// 			fmt.Printf("Judgement Decode Error: %v\n", err)
// 		}
// 		judgement = decoded.(types.Judgement)
// 		if err == nil {
// 			err = n.processJudgement(judgement)
// 			if err == nil {
// 				response = ok
// 			} else {
// 				fmt.Println(err.Error())
// 			}
// 			fmt.Printf(" -- [N%d] received judgement From N%d\n", n.id, msg.Id)
// 			fmt.Printf(" -- [N%d] received judgement From N%d (%v <- %v)\n", n.id, msg.Id, judgement.WorkReport.GetWorkPackageHash(), judgement.WorkReport.GetWorkPackageHash())
//
// 		}
// 	case "Announcement":
// 		var announcement types.Announcement
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(announcement))
// 		if err != nil {
// 			fmt.Printf("Announcement Decode Error: %v\n", err)
// 		}
// 		announcement = decoded.(types.Announcement)
// 		err = n.processAnnouncement(announcement)
// 		if err == nil {
// 			response = ok
// 		}
// 		fmt.Printf(" -- [N%d] received announcement From N%d\n", n.id, msg.Id)
// 		fmt.Printf(" -- [N%d] received announcement From N%d (%v <- %v)\n", n.id, msg.Id, announcement.WorkReport.GetWorkPackageHash(), announcement.WorkReport.GetWorkPackageHash())
// 	case "Preimages":
// 		var preimages types.Preimages
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(preimages))
// 		if err != nil {
// 			fmt.Printf("Preimages Decode Error: %v\n", err)
// 		}
// 		preimages = decoded.(types.Preimages)
// 		if err == nil {
// 			// err = n.processPreimageLookup(preimageLookup)
// 			err = n.processLookup(preimages)
// 			if err == nil {
// 				response = ok
// 			}
// 		}
// 	case "Block":
// 		block, err := types.BlockFromBytes(msg.Payload)
// 		//err := interface{}
// 		if err == nil {
// 			if debug {
// 				fmt.Printf(" -- [N%d] received block From N%d (%s <- %s)\n", n.id, msg.Id, common.Str(block.ParentHash()), common.Str(block.Hash()))
// 			}
// 			err = n.processBlock(block)
// 			if err == nil {
// 				response = ok
// 			}
//
// 		}
// 	case "WorkPackage":
// 		var workPackage types.WorkPackage
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(workPackage))
// 		if err != nil {
// 			fmt.Printf("WorkPackage Decode Error: %v\n", err)
// 		}
// 		workPackage = decoded.(types.WorkPackage)
// 		var work types.GuaranteeReport
// 		var wg sync.WaitGroup
// 		wg.Add(1)
// 		go func() {
// 			defer wg.Done()
// 			work, _, _, err = n.ProcessWorkPackage(workPackage)
// 			if err != nil {
// 				fmt.Printf(" -- [N%d] WorkPackage Error: %v\n", n.id, err)
// 			}
// 		}()
// 		wg.Wait()
// 		if n.isBadGuarantor {
// 			fmt.Printf(" -- [N%d] Is a Bad Guarantor\n", n.id)
// 			work.Report.Results[0].Result.Ok = []byte("I am Culprits><")
// 			work.Sign(n.GetEd25519Secret())
// 		}
// 		_ = n.processGuaranteeReport(work)
// 		if err == nil {
// 			response = ok
// 		}
// 		n.coreBroadcast(work)
// 		// }
// 		fmt.Printf(" -- [N%d] received WorkPackage From N%d\n", n.id, msg.Id)
// 	case "GuaranteeReport":
// 		var guaranteeReport types.GuaranteeReport
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(guaranteeReport))
// 		if err != nil {
// 			fmt.Printf("GuaranteeReport Decode Error: %v\n", err)
// 		}
// 		guaranteeReport = decoded.(types.GuaranteeReport)
// 		err = n.processGuaranteeReport(guaranteeReport)
// 		if err == nil {
// 			response = ok
// 		}
// 		fmt.Printf(" -- [N%d] received GuaranteeReport From N%d\n", n.id, msg.Id)
//
// 	// -----Custom messages for tiny QUIC experiment-----
//
// 	case "DistributeECChunk":
// 		var chunk types.DistributeECChunk
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(chunk))
// 		if err != nil {
// 			fmt.Printf("DistributeECChunk Decode Error: %v\n", err)
// 		}
// 		chunk = decoded.(types.DistributeECChunk)
// 		err = n.processDistributeECChunk(chunk)
// 		if err == nil {
// 			response = ok
// 		}
// 		fmt.Printf(" -- [N%d] received DistributeECChunk From N%d\n", n.id, msg.Id)
//
// 	// case "ECChunkResponse":
// 	// 	var chunk ECChunkResponse
// 	// 	err := json.Unmarshal([]byte(msg.Payload), &chunk)
// 	// 	if err == nil {
// 	// 		err = n.processECChunkResponse(chunk)
// 	// 		if err == nil {
// 	// 			response = ok
// 	// 		}
// 	// 	}
//
// 	case "ECChunkQuery":
// 		var query types.ECChunkQuery
// 		decoded, _, err := types.Decode([]byte(msg.Payload), reflect.TypeOf(query))
// 		if err != nil {
// 			fmt.Printf("ECChunkQuery Decode Error: %v\n", err)
// 		}
// 		query = decoded.(types.ECChunkQuery)
// 		r, err := n.processECChunkQuery(query)
// 		if err == nil {
// 			serializedR, err := types.Encode(r)
// 			if err != nil {
// 				fmt.Printf("ECChunkQuery Encode Error: %v\n", err)
// 			}
// 			response = serializedR
// 		} else {
// 			response = []byte{}
// 			fmt.Printf("processECChunkQuery error: %v\n", err)
// 		}
// 	}
// 	return msg.MsgType, response
// }
//
// func (n *Node) handleStream(peerAddr string, stream quic.Stream) {
//
// 	defer stream.Close()
// 	var lengthPrefix [4]byte
// 	_, err := io.ReadFull(stream, lengthPrefix[:])
// 	if err != nil {
// 		fmt.Printf("[N%v] handleStream: Read length prefix error: %v\n", n.id, err)
// 		return
// 	}
// 	messageLength := binary.BigEndian.Uint32(lengthPrefix[:])
// 	buf := make([]byte, messageLength)
// 	_, err = io.ReadFull(stream, buf)
// 	if err != nil {
// 		fmt.Printf("[N%v] handleStream: Read message error: %v\n", n.id, err)
// 		return
// 	}
//
// 	// response := []byte("1")
//
// 	// handleQuicMsg logic has been moved out to support self-requesting case
// 	msg := DecodeAsQuicMessage(buf)
// 	msgType, response := n.handleQuicMsg(msg)
// 	if msgType != "unknown" {
// 		//fmt.Printf(" -- [N%v] handleStream Read From N%v (msgType=%v)\n", n.id, msg.Id, msg.MsgType)
// 	}
//
// 	_, err = stream.Write(response)
// 	if err != nil {
// 		log.Println(err)
// 	}
// 	//fmt.Printf("responded with: %s\n", string(response))
// 	//stream.Close()
// }
