package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"reflect"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 146: Work-package bundle submission
Submission of a complete work-package bundle from a builder to a guarantor.

Note that the bundle parts are sent in separate messages to allow for authorizing the work-package before reading the rest of the bundle.

The import proof corresponds to J as defined in the gray paper.

Work-Package = As in GP
Extrinsic = [u8]
Import-Proof = len++[Hash]
Segments-Root Mappings = len++[Work-Package Hash ++ Segments-Root]

Builder -> Guarantor

--> Core Index ++ Segments-Root Mappings
--> Work-Package
--> [Extrinsic] (Message size should equal sum of extrinsic data lengths)
--> [Segment] (All imported segments)
--> [Import-Proof] (Import proofs for all imported segments)
--> FIN
<-- FIN
*/

type JAMSNP_WpInfo struct {
	CoreIndex            uint16                     `json:"core_index"`
	SegmentsRootMappings []JAMSNPSegmentRootMapping `json:"segments_root_mappings"`
}

func (info *JAMSNP_WpInfo) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize CoreIndex (2 bytes) - raw little-endian u16
	if err := binary.Write(buf, binary.LittleEndian, info.CoreIndex); err != nil {
		return nil, err
	}

	// Serialize SegmentsRootMappings - SCALE encoded (len++[...])
	segmentsRootMappingsBytes, err := types.Encode(info.SegmentsRootMappings)
	if err != nil {
		return nil, fmt.Errorf("JAMSNP_WpInfo.ToBytes: %w", err)
	}
	if _, err := buf.Write(segmentsRootMappingsBytes); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (info *JAMSNP_WpInfo) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize CoreIndex (2 bytes) - raw little-endian u16
	if err := binary.Read(buf, binary.LittleEndian, &info.CoreIndex); err != nil {
		return err
	}

	// Deserialize SegmentsRootMappings - SCALE encoded
	decoded, _, err := types.Decode(data[2:], reflect.TypeOf([]JAMSNPSegmentRootMapping{}))
	if err != nil {
		return err
	}
	info.SegmentsRootMappings = decoded.([]JAMSNPSegmentRootMapping)

	return nil
}

func (p *Peer) SendBundleSubmission(ctx context.Context, coreIndex uint16, segmentsRootMappings []JAMSNPSegmentRootMapping, pkg types.WorkPackage, extrinsics types.ExtrinsicsBlobs, segments [][]byte, importProofs [][]common.Hash) (err error) {
	log.Info(log.Node, "SendBundleSubmission START", "peer", p.SanKey(), "coreIndex", coreIndex, "numSegmentRootMappings", len(segmentsRootMappings), "wpHash", pkg.Hash().Hex(), "numExtrinsics", len(extrinsics), "numSegments", len(segments), "numImportProofs", len(importProofs))
	// Validate segment and proof counts match work package requirements

	expectedSegments := 0
	for _, workItem := range pkg.WorkItems {
		expectedSegments += len(workItem.ImportedSegments)
	}
	if len(segments) != expectedSegments {
		log.Error(log.Node, "SendBundleSubmission segment count mismatch", "peer", p.SanKey(), "expected", expectedSegments, "got", len(segments))
		return fmt.Errorf("CE146 SendBundleSubmission: segment count mismatch: expected %d, got %d", expectedSegments, len(segments))
	}
	if len(importProofs) != expectedSegments {
		log.Error(log.Node, "SendBundleSubmission import proof count mismatch", "peer", p.SanKey(), "expected", expectedSegments, "got", len(importProofs))
		return fmt.Errorf("CE146 SendBundleSubmission: import proof count mismatch: expected %d, got %d", expectedSegments, len(importProofs))
	}
	// Validate each segment has proper size
	// Note: Empty proofs are valid for single-element CDT trees (leaf hash == root)
	for i, seg := range segments {
		if len(seg) != types.SegmentSize {
			log.Error(log.Node, "SendBundleSubmission invalid segment size", "peer", p.SanKey(), "index", i, "expected", types.SegmentSize, "got", len(seg))
			return fmt.Errorf("CE146 SendBundleSubmission: segment %d has invalid size %d (expected %d)", i, len(seg), types.SegmentSize)
		}
	}

	wpInfo := JAMSNP_WpInfo{
		CoreIndex:            coreIndex,
		SegmentsRootMappings: segmentsRootMappings,
	}
	reqBytes, err := wpInfo.ToBytes()
	if err != nil {
		log.Error(log.Node, "SendBundleSubmission wpInfo.ToBytes failed", "peer", p.SanKey(), "err", err)
		return err
	}
	code := uint8(CE146_WPbundlesubmission)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		log.Error(log.Node, "SendBundleSubmission openStream failed", "peer", p.SanKey(), "err", err)
		return err
	}
	// --> Core Index ++ Segments-Root Mappings
	err = sendQuicBytes(ctx, stream, reqBytes, p.SanKey(), code)
	if err != nil {
		log.Error(log.Node, "SendBundleSubmission sendQuicBytes(wpInfo) failed", "peer", p.SanKey(), "err", err)
		return err
	}
	// --> Work-Package
	pkgBytes, err := types.Encode(pkg)
	if err != nil {
		log.Error(log.Node, "SendBundleSubmission types.Encode(pkg) failed", "peer", p.SanKey(), "err", err)
		return err
	}
	err = sendQuicBytes(ctx, stream, pkgBytes, p.SanKey(), code)
	if err != nil {
		log.Error(log.Node, "SendBundleSubmission sendQuicBytes(pkg) failed", "peer", p.SanKey(), "err", err)
		return err
	}
	// --> [Extrinsic] (Message size should equal sum of extrinsic data lengths)
	var extrinsicsBytes []byte
	if len(extrinsics) == 0 {
		extrinsicsBytes = []byte{}
	} else {
		for _, blob := range extrinsics {
			extrinsicsBytes = append(extrinsicsBytes, blob...)
		}
	}
	err = sendQuicBytes(ctx, stream, extrinsicsBytes, p.SanKey(), code)
	if err != nil {
		log.Error(log.Node, "SendBundleSubmission sendQuicBytes(extrinsics) failed", "peer", p.SanKey(), "err", err)
		return err
	}
	// --> [Segment] (All imported segments) - sent as single concatenated message
	var allSegments []byte
	for _, segment := range segments {
		allSegments = append(allSegments, segment...)
	}
	err = sendQuicBytes(ctx, stream, allSegments, p.SanKey(), code)
	if err != nil {
		log.Error(log.Node, "SendBundleSubmission sendQuicBytes(segments) failed", "peer", p.SanKey(), "err", err)
		return err
	}
	// --> [Import-Proof] (Import proofs for all imported segments)
	// Each Import-Proof is len++[Hash], concatenated into single message
	var allImportProofs []byte
	for _, proof := range importProofs {
		allImportProofs = append(allImportProofs, encodeImportProof(proof)...)
	}
	err = sendQuicBytes(ctx, stream, allImportProofs, p.SanKey(), code)
	if err != nil {
		log.Error(log.Node, "SendBundleSubmission sendQuicBytes(importProofs) failed", "peer", p.SanKey(), "err", err)
		return err
	}
	// --> FIN
	stream.Close()
	log.Info(log.Node, "SendBundleSubmission COMPLETE", "peer", p.SanKey(), "coreIndex", coreIndex, "wpHash", pkg.Hash().Hex())
	return nil
}

func (n *Node) onBundleSubmission(ctx context.Context, stream quic.Stream, msg []byte, peerKey string) error {
	log.Info(log.Node, "onBundleSubmission RECEIVED", "NODE", n.id, "peerKey", peerKey, "msgLen", len(msg))
	defer stream.Close()

	// Helper to cancel stream on error
	cancelOnError := func(err error) error {
		log.Error(log.Node, "onBundleSubmission cancelOnError", "NODE", n.id, "err", err)
		stream.CancelRead(ErrInvalidData)
		return err
	}

	var info JAMSNP_WpInfo
	err := info.FromBytes(msg)
	if err != nil {
		return cancelOnError(err)
	}
	var pkg types.WorkPackage
	pkgBytes, err := receiveQuicBytes(ctx, stream, peerKey, uint8(CE146_WPbundlesubmission))
	if err != nil {
		return cancelOnError(err)
	}
	pkgInterface, _, err := types.Decode(pkgBytes, reflect.TypeOf(pkg))
	if err != nil {
		return cancelOnError(err)
	}
	pkg = pkgInterface.(types.WorkPackage)
	log.Info(log.Node, "onBundleSubmission parsed WP", "NODE", n.id, "wpHash", pkg.Hash().Hex(), "numWorkItems", len(pkg.WorkItems))

	extrinsicsBytes, err := receiveQuicBytes(ctx, stream, peerKey, uint8(CE146_WPbundlesubmission))
	if err != nil {
		return cancelOnError(err)
	}
	// Parse raw concatenated extrinsics using lengths from work package
	var extrinsics types.ExtrinsicsBlobs
	offset := 0
	for _, workItem := range pkg.WorkItems {
		for _, ext := range workItem.Extrinsics {
			extLen := int(ext.Len)
			if offset+extLen > len(extrinsicsBytes) {
				return cancelOnError(fmt.Errorf("extrinsics data too short: need %d bytes at offset %d, have %d total", extLen, offset, len(extrinsicsBytes)))
			}
			extrinsics = append(extrinsics, extrinsicsBytes[offset:offset+extLen])
			offset += extLen
		}
	}

	// Receive all segments as single concatenated message, then split by SegmentSize
	allSegmentsBytes, err := receiveQuicBytes(ctx, stream, peerKey, uint8(CE146_WPbundlesubmission))
	if err != nil {
		return cancelOnError(err)
	}
	var segments [][]byte
	for i := 0; i+types.SegmentSize <= len(allSegmentsBytes); i += types.SegmentSize {
		segments = append(segments, allSegmentsBytes[i:i+types.SegmentSize])
	}

	importProofsBytes, err := receiveQuicBytes(ctx, stream, peerKey, uint8(CE146_WPbundlesubmission))
	if err != nil {
		return cancelOnError(err)
	}
	// Decode [Import-Proof] - each Import-Proof is len++[Hash]
	// Number of import proofs matches number of segments
	var importProofs [][]common.Hash
	data := importProofsBytes
	for len(data) > 0 {
		proof, bytesConsumed, err := decodeImportProofWithLength(data)
		if err != nil {
			return cancelOnError(fmt.Errorf("decodeImportProof: %w", err))
		}
		importProofs = append(importProofs, proof)
		data = data[bytesConsumed:]
	}

	// Handle the received work-package bundle submission
	return n.HandleBundleSubmission(peerKey, info.CoreIndex, info.SegmentsRootMappings, pkg, extrinsics, segments, importProofs)
}

func (n *Node) HandleBundleSubmission(peerKey string, coreIndex uint16, segmentsRootMappings []JAMSNPSegmentRootMapping, pkg types.WorkPackage, extrinsics types.ExtrinsicsBlobs, segments [][]byte, importProofs [][]common.Hash) error {
	log.Debug(log.R, "CE146-HandleBundleSubmission INCOMING", "NODE", n.id, "peerKey", peerKey, "coreIndex", coreIndex, "workpackage", pkg.Hash(), "numSegments", len(segments), "numImportProofs", len(importProofs))

	if isSilent {
		log.Info(log.R, "CE146-HandleBundleSubmission INCOMING - SILENT MODE", "NODE", n.id)
		return fmt.Errorf("Node %d is silent mode", n.id)
	}

	// Calculate slot based on JCE mode
	var slot uint32
	if n.jceMode == JCEDefault {
		slot = n.statedb.GetTimeslot()
	} else {
		slot = common.GetWallClockJCE(fudgeFactorJCE)
	}

	// Validate core assignment
	prevAssignments, assignments := n.statedb.CalculateAssignments(slot)
	inSet := false
	for _, assignment := range assignments {
		if assignment.CoreIndex == coreIndex && types.Ed25519Key(assignment.Validator.Ed25519.PublicKey()) == n.GetEd25519Key() {
			inSet = true
			break
		}
	}
	// Allow work packages from previous epoch assignments during transitions
	if !inSet {
		for _, assignment := range prevAssignments {
			if assignment.CoreIndex == coreIndex && types.Ed25519Key(assignment.Validator.Ed25519.PublicKey()) == n.GetEd25519Key() {
				inSet = true
				break
			}
		}
	}
	if !inSet {
		return fmt.Errorf("CE146: core index %d is not in the current guarantor assignments", coreIndex)
	}

	// Verify extrinsic data consistency with work package extrinsic hashes
	if len(pkg.WorkItems) > 0 {
		if err := pkg.WorkItems[0].CheckExtrinsics(extrinsics); err != nil {
			log.Error(log.Node, "CE146-HandleBundleSubmission: extrinsic verification failed", "err", err)
			return fmt.Errorf("extrinsic data inconsistent with hashes: %w", err)
		}
	}

	workPackageHash := pkg.Hash()

	// Check for duplicate work packages
	if _, loaded := n.seenWorkPackages.LoadOrStore(workPackageHash, struct{}{}); loaded {
		log.Trace(log.R, "CE146-HandleBundleSubmission: duplicate work package", "NODE", n.id, "workpackage", workPackageHash)
		return nil
	}

	// Avoid duplicating work package submissions already in RecentBlocks
	s := n.statedb
	for _, block := range s.JamState.RecentBlocks.B_H {
		if len(block.Reported) != 0 {
			for _, segmentRootLookup := range block.Reported {
				if segmentRootLookup.WorkPackageHash == workPackageHash {
					return nil
				}
			}
		}
	}

	// Compute expected number of segments from work package
	expectedSegments := 0
	for _, workItem := range pkg.WorkItems {
		expectedSegments += len(workItem.ImportedSegments)
	}

	// Validate segment and proof counts
	if expectedSegments > 0 {
		if len(segments) != expectedSegments {
			log.Error(log.Node, "CE146-HandleBundleSubmission: segment count mismatch",
				"expected", expectedSegments, "actual", len(segments))
			return fmt.Errorf("CE146: expected %d segments, got %d", expectedSegments, len(segments))
		}
		if len(importProofs) != expectedSegments {
			log.Error(log.Node, "CE146-HandleBundleSubmission: import proof count mismatch",
				"expected", expectedSegments, "actual", len(importProofs))
			return fmt.Errorf("CE146: expected %d import proofs, got %d", expectedSegments, len(importProofs))
		}
		// Validate each segment has proper size
		// Note: Empty proofs are valid for single-element CDT trees (leaf hash == root)
		for i, seg := range segments {
			if len(seg) != types.SegmentSize {
				log.Error(log.Node, "CE146-HandleBundleSubmission: invalid segment size",
					"index", i, "expected", types.SegmentSize, "actual", len(seg))
				return fmt.Errorf("CE146: segment %d has invalid size %d (expected %d)", i, len(seg), types.SegmentSize)
			}
		}
	}

	// Reorganize flat segments/importProofs into [workItemIndex][importedSegmentIndex] structure
	// The segments and importProofs are sent in order: all segments for workItem 0, then workItem 1, etc.
	importSegmentData := make([][][]byte, len(pkg.WorkItems))
	justification := make([][][]common.Hash, len(pkg.WorkItems))
	segmentIdx := 0
	for workItemIdx, workItem := range pkg.WorkItems {
		numImports := len(workItem.ImportedSegments)
		importSegmentData[workItemIdx] = make([][]byte, numImports)
		justification[workItemIdx] = make([][]common.Hash, numImports)
		for impIdx := 0; impIdx < numImports; impIdx++ {
			importSegmentData[workItemIdx][impIdx] = segments[segmentIdx]
			justification[workItemIdx][impIdx] = importProofs[segmentIdx]
			segmentIdx++
		}
	}

	// Convert segmentsRootMappings to types.SegmentRootLookup for verification
	segmentRootLookup := make(types.SegmentRootLookup, len(segmentsRootMappings))
	for i, mapping := range segmentsRootMappings {
		segmentRootLookup[i] = types.SegmentRootLookupItem{
			WorkPackageHash: mapping.WorkPackageHash,
			SegmentRoot:     mapping.SegmentRoot,
		}
	}

	// Verify the bundle justifications before storing to queue
	// This ensures the builder provided valid segments with correct CDT proofs
	if len(segments) > 0 {
		tempBundle := &types.WorkPackageBundle{
			WorkPackage:       pkg,
			ExtrinsicData:     []types.ExtrinsicsBlobs{extrinsics},
			ImportSegmentData: importSegmentData,
			Justification:     justification,
		}
		eventID := n.telemetryClient.GetEventID()
		verified, err := n.statedb.VerifyBundle(tempBundle, segmentRootLookup, eventID)
		if err != nil {
			log.Error(log.Node, "CE146-HandleBundleSubmission: VerifyBundle error", "err", err)
			return fmt.Errorf("CE146: VerifyBundle failed: %w", err)
		}
		if !verified {
			log.Warn(log.Node, "CE146-HandleBundleSubmission: bundle verification failed", "workpackage", workPackageHash)
			return fmt.Errorf("CE146: bundle verification failed for %s", workPackageHash)
		}
		log.Debug(log.R, "CE146-HandleBundleSubmission VERIFIED", "NODE", n.id, "workpackage", workPackageHash)
	}

	// Store to work package queue with pre-fetched bundle data
	n.workPackageQueue.Store(workPackageHash, &types.WPQueueItem{
		WorkPackage:        pkg,
		CoreIndex:          coreIndex,
		Extrinsics:         extrinsics,
		AddTS:              time.Now().Unix(),
		NextAttemptAfterTS: time.Now().Unix(),
		Slot:               slot,
		EventID:            n.telemetryClient.GetEventID(),
		ImportSegmentData:  importSegmentData,
		Justification:      justification,
	})

	log.Debug(log.R, "CE146-HandleBundleSubmission QUEUED", "NODE", n.id, "workpackage", workPackageHash, "coreIndex", coreIndex, "slot", slot, "numSegments", len(segments), "numImportProofs", len(importProofs))

	return nil
}

// decodeImportProofWithLength decodes an import proof (len++[Hash]) and returns bytes consumed
func decodeImportProofWithLength(data []byte) ([]common.Hash, int, error) {
	if len(data) == 0 {
		return nil, 0, fmt.Errorf("empty import proof data")
	}

	// len++[Hash]
	numHashes, bytesRead := types.DecodeE(data)
	if bytesRead == 0 {
		return nil, 0, fmt.Errorf("failed to decode import proof length")
	}

	totalBytes := int(bytesRead) + int(numHashes)*32
	if len(data) < totalBytes {
		return nil, 0, fmt.Errorf("insufficient data for import proof: expected %d bytes, got %d", totalBytes, len(data))
	}

	proof := make([]common.Hash, numHashes)
	for i := uint64(0); i < numHashes; i++ {
		offset := int(bytesRead) + int(i)*32
		proof[i] = common.BytesToHash(data[offset : offset+32])
	}

	return proof, totalBytes, nil
}
