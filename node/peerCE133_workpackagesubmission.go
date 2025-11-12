package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	telemetry "github.com/colorfulnotion/jam/telemetry"
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

	req := JAMSNPWorkPackage{
		CoreIndex:   core_idx,
		WorkPackage: pkg,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	code := uint8(CE133_WorkPackageSubmission)

	// Telemetry: Work package submission (event 90)
	EventID := p.node.telemetryClient.GetEventID()
	wpBytes, _ := types.Encode(pkg)
	wpOutline := telemetry.WorkPackageOutline{
		WorkPackageHash: pkg.Hash(),
		SizeInBytes:     uint32(len(wpBytes)),
	}

	stream, err := p.openStream(ctx, code)
	if err != nil {
		log.Error(log.Node, "SendWorkPackageSubmission0", "err", err)
		// Telemetry: Work package failed (event 92)
		p.node.telemetryClient.WorkPackageFailed(EventID, err.Error())
		return err
	}
	p.node.telemetryClient.WorkPackageSubmission(EventID, p.GetPeer32(), wpOutline)
	slot := common.GetWallClockJCE(fudgeFactorJCE)
	log.Debug(log.R, "CE133-SendWorkPackageSubmission OUTGOING", "NODE", p.node.id, "peerID", p.PeerID, "coreIndex", core_idx, "slot", slot, "wp_hash", pkg.Hash())
	//--> Core Index ++ Work Package
	err = sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code)
	if err != nil {
		log.Error(log.Node, "SendWorkPackageSubmission1", "err", err)
		stream.Close()
		// Telemetry: Work package failed (event 92)
		p.node.telemetryClient.WorkPackageFailed(EventID, err.Error())
		return err
	}

	//--> [Extrinsic] (Message length should equal sum of extrinsic data lengths)
	var extrinsicsBytes []byte
	if len(extrinsics) == 0 {
		extrinsicsBytes = []byte{}
	} else {
		extrinsicsBytes = extrinsics.Bytes()
	}
	if err = sendQuicBytes(ctx, stream, extrinsicsBytes, p.PeerID, code); err != nil {
		log.Error(log.Node, "SendWorkPackageSubmission Encode", "err", err)
		stream.Close()
		// Telemetry: Work package failed (event 92)
		p.node.telemetryClient.WorkPackageFailed(EventID, err.Error())
		return
	}
	stream.Close()
	return nil
}

func (n *Node) onWorkPackageSubmission(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) (err error) {
	defer stream.Close()

	// Telemetry: Receiving work package submission
	EventID := n.telemetryClient.GetEventID()

	// --> Core Index ++ Work Package
	var newReq JAMSNPWorkPackage
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		log.Error(log.G, "onWorkPackageSubmission:FromBytes", "err", err)
		// Telemetry: Work package failed (event 92)
		n.telemetryClient.WorkPackageFailed(EventID, err.Error())
		return fmt.Errorf("onWorkPackageSubmission: decode failed: %w", err)
	}
	// --> [Extrinsic] (Message length should equal sum of extrinsic data lengths)
	// Read message length (4 bytes)
	msgLenBytes := make([]byte, 4)
	if _, err := io.ReadFull(stream, msgLenBytes); err != nil {
		log.Trace(log.Node, "DispatchIncomingQUICStream - length prefix", "err", err)
		_ = stream.Close()
		return err
	}
	msgLen := binary.LittleEndian.Uint32(msgLenBytes)
	// Calculate slot based on JCE mode:
	// - JCEDefault: Use statedb's current timeslot to ensure correct safrole state
	var slot uint32
	if n.jceMode == JCEDefault {
		slot = n.statedb.GetTimeslot()
	} else {
		slot = common.GetWallClockJCE(fudgeFactorJCE)
	}
	log.Debug(log.R, "CE133-onWorkPackageSubmission INCOMING", "NODE", n.id, "peer", peerID, "workpackage", newReq.WorkPackage.Hash(), "slot", slot)
	prevAssignments, assignments := n.statedb.CalculateAssignments(slot)
	inSet := false
	for _, assignment := range assignments {
		if assignment.CoreIndex == newReq.CoreIndex && types.Ed25519Key(assignment.Validator.Ed25519.PublicKey()) == n.GetEd25519Key() {
			inSet = true
			break
		}
	}
	allowPrevAssignments := true // Allow work packages from previous epoch assignments during transitions
	if allowPrevAssignments {
		for _, assignment := range prevAssignments {
			if assignment.CoreIndex == newReq.CoreIndex && types.Ed25519Key(assignment.Validator.Ed25519.PublicKey()) == n.GetEd25519Key() {
				inSet = true
				break
			}
		}
	}
	if !inSet {
		return fmt.Errorf("core index %d is not in the current guarantor assignments", newReq.CoreIndex)
	}

	// Read message body, which is the encoded extrinsics
	extrinsicsBytes := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, extrinsicsBytes); err != nil {
		log.Error(log.Node, "onWorkPackageSubmission4", "err", err)
		stream.CancelRead(ErrCECode)
		_ = stream.Close()
		return err
	}
	// map extrinsicsBytes to extrinsics
	ext, _, err := types.Decode(extrinsicsBytes, reflect.TypeOf(types.ExtrinsicsBlobs{}))
	if err != nil {
		log.Error(log.Node, "onWorkPackageSubmission4a", "err", err)
		return fmt.Errorf("error in decoding data: %w", err)
	}
	extrinsics := ext.(types.ExtrinsicsBlobs)
	if err != nil {
		log.Error(log.Node, "onWorkPackageSubmission4b", "err", err)
		return fmt.Errorf("error in decoding data: %w", err)
	}

	// Verify extrinsic data consistency with work package extrinsic hashes
	if len(newReq.WorkPackage.WorkItems) > 0 {
		if err := newReq.WorkPackage.WorkItems[0].CheckExtrinsics(extrinsics); err != nil {
			log.Error(log.Node, "onWorkPackageSubmission: extrinsic verification failed", "err", err)
			return fmt.Errorf("extrinsic data inconsistent with hashes: %w", err)
		}
	}

	// Telemetry: Extrinsic data received and verified (event 96)
	n.telemetryClient.ExtrinsicDataReceived(EventID)

	s := n.statedb
	workPackageHash := newReq.WorkPackage.Hash()

	if _, loaded := n.seenWorkPackages.LoadOrStore(workPackageHash, struct{}{}); loaded {
		n.telemetryClient.DuplicateWorkPackage(EventID, newReq.CoreIndex, workPackageHash)
		return nil
	}

	// Respect context cancellation early if already expired
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Avoid duplicating work package submissions already in RecentBlocks
	for _, block := range s.JamState.RecentBlocks.B_H {
		if len(block.Reported) != 0 {
			for _, segmentRootLookup := range block.Reported {
				if segmentRootLookup.WorkPackageHash == workPackageHash {
					return nil
				}
			}
		}
	}

	// Telemetry: Work package received (event 94)
	wpBytes, _ := types.Encode(newReq.WorkPackage)
	wpOutline := telemetry.WorkPackageOutline{
		WorkPackageHash: newReq.WorkPackage.Hash(),
		SizeInBytes:     uint32(len(wpBytes)),
	}
	n.telemetryClient.WorkPackageReceived(EventID, wpOutline)

	// Only the FIRST guarantor will receive this
	n.workPackageQueue.Store(workPackageHash, &types.WPQueueItem{
		WorkPackage:        newReq.WorkPackage,
		CoreIndex:          newReq.CoreIndex,
		Extrinsics:         extrinsics,
		AddTS:              time.Now().Unix(),
		NextAttemptAfterTS: time.Now().Unix(),
		Slot:               slot,
		EventID:            EventID,
	})

	return nil
}
