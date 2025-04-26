package node

import (
	"bytes"
	"context"
	"fmt"
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
	CoreIndex   uint16                `json:"core_index"`
	WorkPackage types.WorkPackage     `json:"work_package"`
	Extrinsic   types.ExtrinsicsBlobs `json:"extrinsics"`
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
	pkg.Extrinsic = decodedData.Extrinsic

	return nil
}

// TODO: review
func (p *Peer) SendWorkPackageSubmission(ctx context.Context, pkg types.WorkPackage, extrinsics types.ExtrinsicsBlobs, core_idx uint16) (err error) {
	if pkg.RefineContext.LookupAnchorSlot == 1 {
		if len(pkg.RefineContext.Prerequisites) == 0 {
			// TODO "Prerequisite is empty"
		}
	}
	if err != nil {
		return fmt.Errorf("failed to get self core index: %w", err)
	}

	req := JAMSNPWorkPackage{
		CoreIndex:   core_idx,
		WorkPackage: pkg,
		Extrinsic:   extrinsics,
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
	defer stream.Close()
	err = sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code)
	if err != nil {
		return err
	}

	log.Trace(debugG, "submitted Workpackage to core", "p", p.String(), "len", len(reqBytes), "core", core_idx)

	return nil
}

func (n *Node) onWorkPackageSubmission(ctx context.Context, stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()

	var newReq JAMSNPWorkPackage

	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		log.Error(debugG, "onWorkPackageSubmission:FromBytes", "err", err)
		return fmt.Errorf("onWorkPackageSubmission: decode failed: %w", err)
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
		extrinsics:         newReq.Extrinsic,
		addTS:              time.Now().Unix(),
		nextAttemptAfterTS: time.Now().Unix(),
	})

	return nil
}
