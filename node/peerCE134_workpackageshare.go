package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

var isSilent = false

/*
CE 134: Work-package sharing
Sharing of a work-package between guarantors on the same core assignment.

A work-package received via CE 133 should be shared with the other guarantors assigned to the core using this protocol, but only after:

It has been determined that it is possible to generate a work-report that could be included on chain. This will involve, for example, verifying the WP's authorization.
All import segments have been retrieved. Note that this will involve mapping any WP hashes in the import list to segment roots.
The refine logic need not be executed before sharing a work-package; ideally, refinement should be done while waiting for the other guarantors to respond.

Unlike CE 133, a full work-package bundle is sent, along with any necessary work-package hash to segment root mappings.
The bundle includes imported data segments and their justifications as well as the work-package and extrinsic data.
The bundle should precisely match the one that is ultimately erasure coded and made available in the case where the work-report gets included on chain.

The guarantor receiving the work-package bundle should perform basic verification first and then execute the refine logic, returning the hash of the resulting work-report
and a signature that can be included in a guaranteed work-report.
The basic verification should include checking the validity of the authorization and checking the work-package hash to segment root mappings.
If the mappings cannot be verified, the guarantor may, at their discretion, either refuse to refine the work-package or blindly trust the mappings.

Core Index = u16
Segment Root Mappings = len++[Work Package Hash ++ Segment Root]
Work Package Bundle = As in GP
Work Report Hash = [u8; 32]
Ed25519 Signature = [u8; 64]

Guarantor -> Guarantor

--> Core Index ++ Segment Root Mappings
--> Work Package Bundle
--> FIN
<-- Work Report Hash ++ Ed25519 Signature
<-- FIN
*/
type JAMSNPSegmentRootMapping struct {
	WorkPackageHash common.Hash `json:"work_package_hash"`
	SegmentRoot     common.Hash `json:"segment_root"`
}

func (mapping *JAMSNPSegmentRootMapping) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize WorkPackageHash (32 bytes for common.Hash)
	if _, err := buf.Write(mapping.WorkPackageHash[:]); err != nil {
		return nil, err
	}

	// Serialize SegmentRoot (32 bytes for common.Hash)
	if _, err := buf.Write(mapping.SegmentRoot[:]); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (mapping *JAMSNPSegmentRootMapping) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize WorkPackageHash (32 bytes)
	if _, err := io.ReadFull(buf, mapping.WorkPackageHash[:]); err != nil {
		return err
	}

	// Deserialize SegmentRoot (32 bytes)
	if _, err := io.ReadFull(buf, mapping.SegmentRoot[:]); err != nil {
		return err
	}

	return nil
}

type JAMSNPWorkPackageShare struct {
	CoreIndex    uint16                     `json:"core"`
	Len          uint                       `json:"len"`
	SegmentRoots []JAMSNPSegmentRootMapping `json:"segment_roots"`
}

func (share *JAMSNPWorkPackageShare) ToBytes() (encodedBytes []byte, err error) {
	// encodedBytes, err = types.Encode(share)
	// if err != nil {
	// 	return nil, fmt.Errorf("JAMSNPWorkPackageShare.ToBytes: %w", err)
	// }

	buf := new(bytes.Buffer)

	// Serialize CoreIndex (2 bytes)
	if err := binary.Write(buf, binary.LittleEndian, share.CoreIndex); err != nil {
		return nil, err
	}
	segmentRootsBytes, err := types.Encode(share.SegmentRoots)
	if err != nil {
		return nil, fmt.Errorf("JAMSNPWorkPackageShare.ToBytes: %w", err)
	}
	if _, err := buf.Write(segmentRootsBytes); err != nil {
		return nil, err
	}

	/*

		// Serialize Len (1 byte)
		share.Len = uint8(len(share.SegmentRoots)) // Set the Len field before serialization
		if err := buf.WriteByte(share.Len); err != nil {
			return nil, err
		}

		// Serialize SegmentRoots (dynamically sized)
		for _, rootMapping := range share.SegmentRoots {
			rootMappingBytes, err := rootMapping.ToBytes()
			if err != nil {
				return nil, err
			}
			if _, err := buf.Write(rootMappingBytes); err != nil {
				return nil, err
			}
		}
	*/
	encodedBytes = buf.Bytes()

	return encodedBytes, nil
}

func (share *JAMSNPWorkPackageShare) FromBytes(data []byte) error {

	buf := bytes.NewReader(data)

	// Deserialize CoreIndex (2 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &share.CoreIndex); err != nil {
		return err
	}
	decoded, _, err := types.Decode(data[2:], reflect.TypeOf([]JAMSNPSegmentRootMapping{}))
	if err != nil {
		return err
	}
	jamsnpSegmentRootMappings := decoded.([]JAMSNPSegmentRootMapping)
	share.SegmentRoots = jamsnpSegmentRootMappings
	share.Len = uint(len(share.SegmentRoots))

	/*
		// Deserialize Len (1 byte)
		lenByte, err := buf.ReadByte()
		if err != nil {
			return err
		}
		share.Len = lenByte

		// Deserialize SegmentRoots based on Len
		share.SegmentRoots = make([]JAMSNPSegmentRootMapping, share.Len)
		for i := uint8(0); i < share.Len; i++ {
			var rootMapping JAMSNPSegmentRootMapping
			rootMappingBytes := make([]byte, 64)
			if _, err := buf.Read(rootMappingBytes); err != nil {
				return err
			}
			if err := rootMapping.FromBytes(rootMappingBytes); err != nil {
				return err
			}
			share.SegmentRoots[i] = rootMapping
		}
	*/

	return nil
}

type JAMSNPWorkPackageShareResponse struct {
	WorkReportHash common.Hash            `json:"work_report_hash"`
	Signature      types.Ed25519Signature `json:"signature"`
}

// ToBytes serializes the JAMSNPWorkPackageShareResponse struct into a byte array
func (response *JAMSNPWorkPackageShareResponse) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize WorkReportHash (32 bytes)
	if _, err := buf.Write(response.WorkReportHash[:]); err != nil {
		return nil, err
	}

	// Serialize Signature (64 bytes for Ed25519Signature)
	if _, err := buf.Write(response.Signature[:]); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPWorkPackageShareResponse struct
func (response *JAMSNPWorkPackageShareResponse) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize WorkReportHash (32 bytes)
	if _, err := io.ReadFull(buf, response.WorkReportHash[:]); err != nil {
		return err
	}

	// Deserialize Signature (64 bytes for Ed25519Signature)
	if _, err := io.ReadFull(buf, response.Signature[:]); err != nil {
		return err
	}

	return nil
}

func (p *Peer) ShareWorkPackage(
	ctx context.Context,
	coreIndex uint16,
	bundle types.WorkPackageBundle,
	segmentRootLookup types.SegmentRootLookup,
	pubKey types.Ed25519Key,
	peerID uint16,
	EventId uint64,
) (newReq JAMSNPWorkPackageShareResponse, err error) {

	// Prepare the request
	segmentroots := make([]JAMSNPSegmentRootMapping, 0, len(segmentRootLookup))
	for _, item := range segmentRootLookup {
		segmentroots = append(segmentroots, JAMSNPSegmentRootMapping{
			WorkPackageHash: item.WorkPackageHash,
			SegmentRoot:     item.SegmentRoot,
		})
	}

	encodedBundle := bundle.Bytes()
	req := JAMSNPWorkPackageShare{
		CoreIndex:    coreIndex,
		Len:          uint(len(segmentroots)),
		SegmentRoots: segmentroots,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return
	}

	code := uint8(CE134_WorkPackageShare)

	stream, streamErr := p.openStream(ctx, code)
	if streamErr != nil {
		err = fmt.Errorf("openStream[CE134_WorkPackageShare]: %v", streamErr)
		// Telemetry: Work package failed (event 92)
		p.node.telemetryClient.WorkPackageFailed(EventId, streamErr.Error())
		return
	}
	// Telemetry: Sharing work package (event 98)
	bundleBytes := bundle.Bytes()
	wpOutline := telemetry.WorkPackageOutline{
		WorkPackageHash: bundle.WorkPackage.Hash(),
		SizeInBytes:     uint32(len(bundleBytes)),
	}
	p.node.telemetryClient.SharingWorkPackage(EventId, p.GetPeer32(), wpOutline)

	slot := common.GetWallClockJCE(fudgeFactorJCE)
	log.Debug(log.R, "CE134-ShareWorkPackage OUTGOING", "NODE", p.node.id, "peer", peerID, "coreIndex", coreIndex, "slot", slot)
	// Send request
	// --> Core Index ++ Segment Root Mappings
	if err = sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code); err != nil {
		err = fmt.Errorf("sendQuicBytes1[CE134_WorkPackageShare]: %v", err)
		// Telemetry: Work package failed (event 92)
		p.node.telemetryClient.WorkPackageFailed(EventId, err.Error())
		return
	}
	// --> Work Package Bundle
	if err = sendQuicBytes(ctx, stream, encodedBundle, p.PeerID, code); err != nil {
		err = fmt.Errorf("sendQuicBytes2[CE134_WorkPackageShare]: %v", err)
		// Telemetry: Work package failed (event 92)
		p.node.telemetryClient.WorkPackageFailed(EventId, err.Error())
		return
	}
	log.Trace(log.G, "onWorkPackageShare OUTGOING QUIC", "mapLen", len(reqBytes), "bundleLen", len(encodedBundle), "CoreIndex ++ SegmentRootMappings", fmt.Sprintf("0x%x", reqBytes))
	log.Trace(log.G, "onWorkPackageShare OUTGOING QUIC", "bundleLen", len(encodedBundle), "encodedBundle", fmt.Sprintf("0x%x", encodedBundle))
	log.Trace(log.G, "onWorkPackageShare OUTGOING",
		"workpackage", bundle.WorkPackage.Hash(),
		"bundle_Len", len(encodedBundle),
		"coreIndex", coreIndex,
		"segmentRootsLen", len(segmentroots),
		"workPackageBundleBytes", fmt.Sprintf("0x%x", bundle.Bytes()),
		"workPackageBundle", bundle.String(),
	)
	p.node.telemetryClient.BundleSent(EventId, p.node.PeerID32(p.PeerID))
	// --> FIN
	stream.Close()

	// Receive response
	//<-- Work Report Hash ++ Ed25519 Signature
	respBytes, err := receiveQuicBytes(ctx, stream, p.PeerID, code)
	if err != nil {
		err = fmt.Errorf("receiveQuicBytes[CE134_WorkPackageShare]: %v", err)
		// Telemetry: Work package failed (event 92)
		p.node.telemetryClient.WorkPackageFailed(EventId, err.Error())
		return
	}

	// Deserialize and verify signature against the workReportHash
	if err = newReq.FromBytes(respBytes); err != nil {
		err = fmt.Errorf("FromBytes[CE134_WorkPackageShare]: %v", err)
		// Telemetry: Work package failed (event 92)
		p.node.telemetryClient.WorkPackageFailed(EventId, err.Error())
		return
	}
	workReportHash := newReq.WorkReportHash
	signature := newReq.Signature
	if !types.Ed25519Verify(pubKey, types.ComputeWorkReportSignBytesWithHash(workReportHash), signature) {
		err = fmt.Errorf("invalid signature on WorkReportHash: %x", workReportHash)
		// Telemetry: Work package failed (event 92)
		p.node.telemetryClient.WorkPackageFailed(EventId, err.Error())
		return newReq, err
	}

	// Telemetry: Work report signature received (event 104)
	p.node.telemetryClient.WorkReportSignatureReceived(EventId, p.GetPeer32(), workReportHash)

	return newReq, nil
}

func CompareSegmentRootLookup(a, b types.SegmentRootLookup) (bool, error) {
	if len(a) != len(b) {
		return false, fmt.Errorf("length mismatch")
	}
	mismatchIdx := []int{}
	for i := range a {
		if a[i].WorkPackageHash != b[i].WorkPackageHash || a[i].SegmentRoot != b[i].SegmentRoot {
			fmt.Printf("Mismatch at index %v %v_%v | %v_%v\n", i, a[i].WorkPackageHash, a[i].SegmentRoot, b[i].WorkPackageHash, b[i].SegmentRoot)
			mismatchIdx = append(mismatchIdx, i)
		}
	}
	return len(mismatchIdx) == 0, fmt.Errorf("diff at %v", mismatchIdx)
}

func (n *Node) onWorkPackageShare(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) (err error) {
	defer stream.Close()

	eventID := n.telemetryClient.GetEventID()

	// --> Core Index ++ Segment Root Mappings
	var newReq JAMSNPWorkPackageShare
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		// Telemetry: Work package failed (event 92)
		n.telemetryClient.WorkPackageFailed(eventID, err.Error())
		stream.CancelRead(ErrInvalidData)
		return fmt.Errorf("onWorkPackageShare: decode share message: %w", err)
	}

	// Calculate slot based on JCE mode:
	// - JCEDefault: Use statedb's current timeslot to ensure correct safrole state
	var slot uint32
	if n.jceMode == JCEDefault {
		slot = n.statedb.GetTimeslot()
	} else {
		slot = common.GetWallClockJCE(fudgeFactorJCE)
	}
	prevAssignments, assignments := n.statedb.CalculateAssignments(slot)
	inSet := false
	for _, assignment := range assignments {
		if assignment.CoreIndex == newReq.CoreIndex && types.Ed25519Key(assignment.Validator.Ed25519.PublicKey()) == n.GetEd25519Key() {
			inSet = true
			break
		}
	}
	// Allow work packages from previous epoch assignments during transitions
	allowPrevAssignments := true
	if allowPrevAssignments && !inSet {
		for _, assignment := range prevAssignments {
			if assignment.CoreIndex == newReq.CoreIndex && types.Ed25519Key(assignment.Validator.Ed25519.PublicKey()) == n.GetEd25519Key() {
				inSet = true
				break
			}
		}
	}
	log.Debug(log.R, "CE134-ShareWorkPackage INCOMING", "NODE", n.id, "peer", peerID, "coreIndex", newReq.CoreIndex, "slot", slot)
	if !inSet {
		return fmt.Errorf("core index %d is not in the current guarantor assignments", newReq.CoreIndex)
	}

	if isSilent {
		log.Info(log.R, "CE134-ShareWorkPackage INCOMING - SILENT MODE", "NODE", n.id)
		return fmt.Errorf("Node %d is silent mode", n.id)
	}

	n.telemetryClient.WorkPackageBeingShared(n.PeerID32(peerID))
	// --> Work Package Bundle
	// Read message length (4 bytes)
	msgLenBytes := make([]byte, 4)
	if _, err := io.ReadFull(stream, msgLenBytes); err != nil {
		log.Trace(log.Node, "DispatchIncomingQUICStream - length prefix", "err", err)
		_ = stream.Close()
		return err
	}
	msgLen := binary.LittleEndian.Uint32(msgLenBytes)

	// Read message body, which is the encoded bundle
	encodedBundle := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, encodedBundle); err != nil {
		stream.CancelRead(ErrCECode)
		_ = stream.Close()
		return err
	}

	bundle, _, err := types.DecodeBundle(encodedBundle)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		// Telemetry: Work package failed (event 92)
		n.telemetryClient.WorkPackageFailed(eventID, err.Error())
		stream.CancelRead(ErrInvalidData)
		return fmt.Errorf("onWorkPackageShare: decode bundle: %w\nencodedBundle data:\n%x", err, encodedBundle)
	}
	log.Trace(log.G, "onWorkPackageShare INCOMING QUIC", "mapLen", len(msg), "bundleLen", len(encodedBundle), "CoreIndex ++ SegmentRootMappings", fmt.Sprintf("0x%x", msg))
	log.Trace(log.G, "onWorkPackageShare INCOMING QUIC", "bundleLen", len(encodedBundle), "encodedBundle", fmt.Sprintf("0x%x", encodedBundle))
	workPackageHash := bundle.WorkPackage.Hash()
	if _, loaded := n.seenWorkPackages.LoadOrStore(workPackageHash, struct{}{}); loaded {
		n.telemetryClient.DuplicateWorkPackage(eventID, newReq.CoreIndex, workPackageHash)
		return nil
	}

	recoveredBundleByte := bundle.Bytes()

	if !bytes.Equal(encodedBundle, recoveredBundleByte) {
		log.Error(log.G, "onWorkPackageShare INCOMING Bundle Encode/Decode mismatch",
			"workpackage", bundle.WorkPackage.Hash(),
			"bundle_Len", len(encodedBundle),
			"coreIndex", newReq.CoreIndex,
			"segmentRootsLen", newReq.Len,
			"msgLen", msgLen,
			"ReceivedBundleBytes", fmt.Sprintf("0x%x", encodedBundle),
			"workPackageBundleBytes", fmt.Sprintf("0x%x", recoveredBundleByte),
			"workPackageBundle", bundle.String(),
		)
		return fmt.Errorf("onWorkPackageShare: encodedBundle length mismatch: %d != %d", len(encodedBundle), len(bundle.Bytes()))
	}

	wpCoreIndex := newReq.CoreIndex
	log.Trace(log.G, "onWorkPackageShare INCOMING",
		"workpackage", bundle.WorkPackage.Hash(),
		"bundle_Len", len(encodedBundle),
		"coreIndex", newReq.CoreIndex,
		"segmentRootsLen", newReq.Len,
		"msgLen", msgLen,
		"workPackageBundleBytes", fmt.Sprintf("0x%x", encodedBundle),
		"workPackageBundle", bundle.String(),
	)
	wpOutline := telemetry.WorkPackageOutline{
		WorkPackageHash: bundle.WorkPackage.Hash(),
		SizeInBytes:     uint32(len(encodedBundle)),
	}
	n.telemetryClient.WorkPackageReceived(eventID, wpOutline)

	received_segmentRootLookup := make([]types.SegmentRootLookupItem, 0)
	for _, sr := range newReq.SegmentRoots {
		item := types.SegmentRootLookupItem{
			WorkPackageHash: sr.WorkPackageHash,
			SegmentRoot:     sr.SegmentRoot,
		}
		received_segmentRootLookup = append(received_segmentRootLookup, item)
	}

	// Respect context before expensive operations
	select {
	case <-ctx.Done():
		return fmt.Errorf("onWorkPackageShare: context cancelled before VerifyBundle")
	default:
	}

	// Verify extrinsic data consistency with work package extrinsic hashes
	for itemIndex, workItem := range bundle.WorkPackage.WorkItems {
		if itemIndex < len(bundle.ExtrinsicData) {
			if err := workItem.CheckExtrinsics(bundle.ExtrinsicData[itemIndex]); err != nil {
				log.Error(log.Node, "onWorkPackageShare: extrinsic verification failed", "itemIndex", itemIndex, "err", err)
				return fmt.Errorf("extrinsic data inconsistent with hashes for item %d: %w", itemIndex, err)
			}
		}
	}

	// Telemetry: Extrinsic data received and verified (event 96)
	n.telemetryClient.ExtrinsicDataReceived(eventID)

	// Since the bundle is not trusted, do a VerifyBundle first
	verified, err := n.statedb.VerifyBundle(bundle, received_segmentRootLookup, eventID)
	if !verified {
		log.Warn(log.Node, "VerifyBundle failure", "node", n.id, "verified", verified)
		// TODO: reconstruct the segments
	}
	if err != nil {
		return fmt.Errorf("onWorkPackageShare: VerifyBundle err: %w", err)
	}

	// Respect context again before executing
	select {
	case <-ctx.Done():
		return fmt.Errorf("onWorkPackageShare: context cancelled before executing work package")
	default:
	}

	workReport, err := n.executeWorkPackageBundle(wpCoreIndex, *bundle, received_segmentRootLookup, n.statedb.GetTimeslot(), log.OtherGuarantor, eventID)
	if err != nil {
		log.Warn(log.Node, "onWorkPackageShare: executeWorkPackageBundle", "node", n.id, "err", err)
		// Telemetry: Work package failed (event 92)
		n.telemetryClient.WorkPackageFailed(eventID, err.Error())
		return fmt.Errorf("onWorkPackageShare: executeWorkPackageBundle: %w", err)
	}

	select {
	case n.workReportsCh <- workReport:
		// successfully sent
	default:
		log.Warn(log.Audit, "onWorkPackageShare: workReportsCh full, dropping workReport", "workReport", workReport.Hash())
	}

	// Respect context again before executing
	select {
	case <-ctx.Done():
		return fmt.Errorf("onWorkPackageShare: context cancelled before executing work package")
	default:
	}

	signerSecret := n.GetEd25519Secret()
	gc := workReport.Sign(signerSecret, uint16(n.GetCurrValidatorIndex()))
	guarantee := types.Guarantee{
		Report:     workReport,
		Signatures: []types.GuaranteeCredential{gc},
	}
	if len(guarantee.Signatures) == 0 {
		return fmt.Errorf("onWorkPackageShare: No guarantee signature")
	}

	// Log fellow work report details before computing hash
	fellowWorkReportHash := guarantee.Report.Hash()
	req := JAMSNPWorkPackageShareResponse{
		WorkReportHash: fellowWorkReportHash,
		Signature:      guarantee.Signatures[0].Signature,
	}
	log.Trace(log.G, "onWorkPackageShare", "n", n.String(), "reportHash", req.WorkReportHash) //, "wp", workReport.String(), "sig", req.Signature.String())

	reqBytes, err := req.ToBytes()
	if err != nil {
		return fmt.Errorf("onWorkPackageShare: ToBytes failed: %w", err)
	}

	// Telemetry: Sending work report signature (event 103)
	n.telemetryClient.WorkReportSignatureSent(eventID)

	err = sendQuicBytes(ctx, stream, reqBytes, n.id, CE134_WorkPackageShare)
	if err != nil {
		return fmt.Errorf("onWorkPackageShare: sendQuicBytes failed: %w", err)
	}

	// <-- FIN
	return nil
}
