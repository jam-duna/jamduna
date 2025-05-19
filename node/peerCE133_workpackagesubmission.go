package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 133: Work-package submission
Submission of a work-package from a builder to a guarantor assigned to the relevant core.

Core Index = u16
Work Package = As in GP
Extrinsic = [u8]

Builder -> Guarantor

--> Core Index ++ Work Package
--> [Extrinsic] (Message length should equal sum of extrinsic data lengths)
--> FIN
<-- FIN
*/

type JAMSNPWorkPackage struct {
	CoreIndex   uint16            `json:"core_index"`
	WorkPackage types.WorkPackage `json:"work_package"`
}

// ToBytes serializes the JAMSNPWorkPackage struct into a byte array
func (pkg *JAMSNPWorkPackage) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	encodedData, err := types.Encode(pkg)
	if err != nil {
		return nil, err
	}
	if _, err := buf.Write(encodedData); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPWorkPackage struct
func (pkg *JAMSNPWorkPackage) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	decodeDataBytes := make([]byte, buf.Len())
	if _, err := buf.Read(decodeDataBytes); err != nil {
		return fmt.Errorf("failed to read WorkPackage bytes: %w", err)
	}

	dd, _, err := types.Decode(decodeDataBytes, reflect.TypeOf(JAMSNPWorkPackage{}))
	if err != nil {
		return fmt.Errorf("error in decoding data: %w", err)
	}

	decodedData := dd.(JAMSNPWorkPackage)
	pkg.CoreIndex = decodedData.CoreIndex
	pkg.WorkPackage = decodedData.WorkPackage

	return nil
}

func (p *Peer) SendWorkPackageSubmission(ctx context.Context, pkg types.WorkPackage, extrinsics types.ExtrinsicsBlobs, core_idx uint16) (err error) {
	if pkg.RefineContext.LookupAnchorSlot == 1 {
		if len(pkg.RefineContext.Prerequisites) == 0 {
			// TODO "Prerequisite is empty"
		}
	}

	req := JAMSNPWorkPackage{
		CoreIndex:   core_idx,
		WorkPackage: pkg,
	}
	// Here need to setup some kind of verification for the work package

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	code := uint8(CE133_WorkPackageSubmission)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return err
	}
	//--> Core Index ++ Work Package
	err = sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code)
	if err != nil {
		stream.Close()
		return err
	}

	//--> [Extrinsic] (Message length should equal sum of extrinsic data lengths)
	extrinsicsBytes, err := types.Encode(extrinsics)
	if err != nil {
		stream.Close()
		return err
	}

	// send length of extrinsicsBytes
	msgLen := uint32(len(extrinsicsBytes))
	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, msgLen)
	_, err = stream.Write(lenBuf)
	if err != nil {
		log.Error(module, "sendWorkPackageSubmission3", "err", err)
		stream.Close()
		return err
	}

	_, err = stream.Write(extrinsicsBytes)
	if err != nil {
		log.Error(module, "sendWorkPackageSubmission4", "err", err)
		stream.Close()
		return err
	}

	//--> FIN
	stream.Close()

	return nil
}

func (n *Node) onWorkPackageSubmission(ctx context.Context, stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()

	// --> Core Index ++ Work Package
	var newReq JAMSNPWorkPackage

	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		log.Error(debugG, "onWorkPackageSubmission:FromBytes", "err", err)
		return fmt.Errorf("onWorkPackageSubmission: decode failed: %w", err)
	}

	// --> [Extrinsic] (Message length should equal sum of extrinsic data lengths)
	// Read message length (4 bytes)
	msgLenBytes := make([]byte, 4)
	if _, err := io.ReadFull(stream, msgLenBytes); err != nil {
		log.Trace(module, "DispatchIncomingQUICStream - length prefix", "err", err)
		_ = stream.Close()
		return err
	}
	msgLen := binary.LittleEndian.Uint32(msgLenBytes)

	// Read message body, which is the encoded extrinsics
	extrinsicsBytes := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, extrinsicsBytes); err != nil {
		log.Error(module, "onWorkPackageSubmission4", "err", err)
		stream.CancelRead(ErrCECode)
		_ = stream.Close()
		return err
	}
	// map extrinsicsBytes to extrinsics
	ext, _, err := types.Decode(extrinsicsBytes, reflect.TypeOf(types.ExtrinsicsBlobs{}))
	if err != nil {
		log.Error(module, "onWorkPackageSubmission4a", "err", err)
		return fmt.Errorf("error in decoding data: %w", err)
	}
	extrinsics := ext.(types.ExtrinsicsBlobs)
	if err != nil {
		log.Error(module, "onWorkPackageSubmission4b", "err", err)
		return fmt.Errorf("error in decoding data: %w", err)
	}

	s := n.statedb
	workPackageHash := newReq.WorkPackage.Hash()

	// Respect context cancellation early if already expired
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Avoid duplicating work package submissions already in RecentBlocks
	for _, block := range s.JamState.RecentBlocks {
		if len(block.Reported) != 0 {
			for _, segmentRootLookup := range block.Reported {
				if segmentRootLookup.WorkPackageHash == workPackageHash {
					return nil
				}
			}
		}
	}

	// Only the FIRST guarantor will receive this
	n.workPackageQueue.Store(workPackageHash, &WPQueueItem{
		workPackage:        newReq.WorkPackage,
		coreIndex:          newReq.CoreIndex,
		extrinsics:         extrinsics,
		addTS:              time.Now().Unix(),
		nextAttemptAfterTS: time.Now().Unix(),
	})

	return nil
}
