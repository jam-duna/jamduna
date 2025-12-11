package node

import (
	"context"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
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
	return types.Encode(info)
}

func (info *JAMSNP_WpInfo) FromBytes(data []byte) error {
	// Decode the data into the struct
	info_interface, _, err := types.Decode(data, reflect.TypeOf(*info))
	if err != nil {
		return err
	}
	*info = info_interface.(JAMSNP_WpInfo)
	return nil
}

func (p *Peer) SendBundleSubmission(ctx context.Context, coreIndex uint16, segmentsRootMappings []JAMSNPSegmentRootMapping, pkg types.WorkPackage, extrinsics types.ExtrinsicsBlobs, segments [][]byte, importProofs []common.Hash) (err error) {
	wpInfo := JAMSNP_WpInfo{
		CoreIndex:            coreIndex,
		SegmentsRootMappings: segmentsRootMappings,
	}
	reqBytes, err := wpInfo.ToBytes()
	if err != nil {
		return err
	}
	code := uint8(CE146_WPbundlesubmission)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return err
	}
	// --> Core Index ++ Segments-Root Mappings
	err = sendQuicBytes(ctx, stream, reqBytes, p.PeerID, code)
	if err != nil {
		return err
	}
	// --> Work-Package
	pkgBytes, err := types.Encode(pkg)
	if err != nil {
		return err
	}
	err = sendQuicBytes(ctx, stream, pkgBytes, p.PeerID, code)
	if err != nil {
		return err
	}
	// --> [Extrinsic] (Message size should equal sum of extrinsic data lengths)
	extrinsicsBytes, err := types.Encode(extrinsics)
	if err != nil {
		return err
	}
	err = sendQuicBytes(ctx, stream, extrinsicsBytes, p.PeerID, code)
	if err != nil {
		return err
	}
	// --> [Segment] (All imported segments)
	for _, segment := range segments {
		err = sendQuicBytes(ctx, stream, segment, p.PeerID, code)
		if err != nil {
			return err
		}
	}
	// --> [Import-Proof] (Import proofs for all imported segments)
	importProofsBytes, err := types.Encode(importProofs)
	if err != nil {
		return err
	}
	err = sendQuicBytes(ctx, stream, importProofsBytes, p.PeerID, code)
	if err != nil {
		return err
	}
	// --> FIN
	stream.Close()
	return nil
}

func (n *Node) onBundleSubmission(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) error {
	defer stream.Close()
	var info JAMSNP_WpInfo
	err := info.FromBytes(msg)
	if err != nil {
		return err
	}
	var pkg types.WorkPackage
	pkgBytes, err := receiveQuicBytes(ctx, stream, peerID, uint8(CE146_WPbundlesubmission))
	if err != nil {
		return err
	}
	pkgInterface, _, err := types.Decode(pkgBytes, reflect.TypeOf(pkg))
	if err != nil {
		return err
	}
	pkg = pkgInterface.(types.WorkPackage)

	extrinsicsBytes, err := receiveQuicBytes(ctx, stream, peerID, uint8(CE146_WPbundlesubmission))
	if err != nil {
		return err
	}
	var extrinsics types.ExtrinsicsBlobs
	extrinsicsInterface, _, err := types.Decode(extrinsicsBytes, reflect.TypeOf(extrinsics))
	if err != nil {
		return err
	}
	extrinsics = extrinsicsInterface.(types.ExtrinsicsBlobs)

	var segments [][]byte
	for range info.SegmentsRootMappings {
		segmentBytes, err := receiveQuicBytes(ctx, stream, peerID, uint8(CE146_WPbundlesubmission))
		if err != nil {
			return err
		}
		segments = append(segments, segmentBytes)
	}

	importProofsBytes, err := receiveQuicBytes(ctx, stream, peerID, uint8(CE146_WPbundlesubmission))
	if err != nil {
		return err
	}
	var importProofs []common.Hash
	importProofsInterface, _, err := types.Decode(importProofsBytes, reflect.TypeOf(importProofs))
	if err != nil {
		return err
	}
	importProofs = importProofsInterface.([]common.Hash)

	// Handle the received work-package bundle submission
	return n.HandleBundleSubmission(peerID, info.CoreIndex, info.SegmentsRootMappings, pkg, extrinsics, segments, importProofs)
}

func (n *Node) HandleBundleSubmission(peerID uint16, coreIndex uint16, segmentsRootMappings []JAMSNPSegmentRootMapping, pkg types.WorkPackage, extrinsics types.ExtrinsicsBlobs, segments [][]byte, importProofs []common.Hash) error {
	fmt.Printf("Node %d received bundle submission from peer %d: coreIndex=%d, numSegments=%d, numImportProofs=%d\n", n.id, peerID, coreIndex, len(segments), len(importProofs))
	// Further processing logic would go here
	return nil
}
