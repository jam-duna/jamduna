package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

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
	Len          uint8                      `json:"len"`
	SegmentRoots []JAMSNPSegmentRootMapping `json:"segment_roots"`
	Bundle       []byte                     `json:"bundle"`
}

func (share *JAMSNPWorkPackageShare) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize CoreIndex (2 bytes)
	if err := binary.Write(buf, binary.BigEndian, share.CoreIndex); err != nil {
		return nil, err
	}

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

	// Serialize Bundle (dynamically sized)
	if _, err := buf.Write(share.Bundle); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (share *JAMSNPWorkPackageShare) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize CoreIndex (2 bytes)
	if err := binary.Read(buf, binary.BigEndian, &share.CoreIndex); err != nil {
		return err
	}

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

	// Deserialize Bundle (dynamically sized)
	share.Bundle = make([]byte, buf.Len())
	if _, err := buf.Read(share.Bundle); err != nil {
		return err
	}

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

func (p *Peer) ShareWorkPackage(coreIndex uint16, bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, pubKey types.Ed25519Key) (newReq JAMSNPWorkPackageShareResponse, err error) {
	// TODO: add span for share work package => get  Work Report Hash  back here
	if p.node.store.SendTrace {
		tracer := p.node.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(p.node.store.WorkPackageContext, fmt.Sprintf("[N%d] ShareWorkPackage", p.node.store.NodeID))
		// p.node.UpdateWorkPackageContext(ctx)
		defer span.End()
	}
	segmentroots := make([]JAMSNPSegmentRootMapping, 0)
	for _, item := range segmentRootLookup {
		lookupItem := JAMSNPSegmentRootMapping{
			WorkPackageHash: item.WorkPackageHash,
			SegmentRoot:     item.SegmentRoot,
		}
		segmentroots = append(segmentroots, lookupItem)
	}

	bundleBytes := bundle.Bytes()
	req := JAMSNPWorkPackageShare{
		CoreIndex:    coreIndex,
		Len:          uint8(len(segmentroots)),
		SegmentRoots: segmentroots,
		Bundle:       bundleBytes,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return
	}
	stream, err := p.openStream(CE134_WorkPackageShare)
	if err != nil {
		return
	}
	defer stream.Close()
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return
	}
	// <-- Work Report Hash ++ Ed25519 Signature
	respBytes, err := receiveQuicBytes(stream)
	if err != nil {
		return
	}

	err = newReq.FromBytes(respBytes)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}

	workReportHash := newReq.WorkReportHash
	signature := newReq.Signature
	// validate the signature against the workReportHash
	if !types.Ed25519Verify(pubKey, types.ComputeWorkReportSignBytesWithHash(workReportHash), signature) {
		fmt.Printf("WARNING: Sig not verified: %v", workReportHash)
		//TEMP: return
	}
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

func (n *Node) onWorkPackageShare(stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()

	// --> Core Index ++ Segment Root Mappings
	var newReq JAMSNPWorkPackageShare
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	// --> Work Package Bundle
	bundle := newReq.Bundle
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}

	wpCoreIndex := newReq.CoreIndex

	received_segmentRootLookup := make([]types.SegmentRootLookupItem, 0)
	for _, sr := range newReq.SegmentRoots {
		item := types.SegmentRootLookupItem{
			WorkPackageHash: sr.WorkPackageHash,
			SegmentRoot:     sr.SegmentRoot,
		}
		received_segmentRootLookup = append(received_segmentRootLookup, item)
	}

	bp, err := types.WorkPackageBundleFromBytes(bundle)
	if err != nil {
		panic(123)
	}

	// should use original's segmentRootLookup --- no need to fetch here
	segmentRootLookup, err := n.GetSegmentRootLookup(bp.WorkPackage)
	if err != nil {
		fmt.Printf("[N%v] AAA [auditWorkReport:GetSegmentRootLookup] ERR %v\n", n.id, err)
		return
	}

	matched, err := CompareSegmentRootLookup(received_segmentRootLookup, segmentRootLookup)
	if !matched {
		fmt.Printf("[N%v] Segment root lookup mismatch at indices: %v\n", n.id, err)
		return fmt.Errorf("segment root lookup mismatch")
	}
	// Since the bundle is not trusted, do a VerifyBundle first
	verified, err := n.VerifyBundle(bp, segmentRootLookup)
	if !verified {
		fmt.Printf("!!! [N%v] NOT Verified: %v\n", n.id, verified)
	}
	if err != nil {
		return
	}
	workReport, err := n.executeWorkPackageBundle(wpCoreIndex, *bp, segmentRootLookup) //TODO: replace it with segmentroots
	if err != nil {
		return
	} else {
		n.workReportsCh <- workReport
	}
	//fmt.Printf("WR %v\n", workReport.String())

	//TODO: Shawn this is potentially problematic. How can we have deterministic ValidatorIndex here???
	signerSecret := n.GetEd25519Secret()
	gc := workReport.Sign(signerSecret, uint16(n.GetCurrValidatorIndex()))
	guarantee := types.Guarantee{
		Report:     workReport,
		Signatures: []types.GuaranteeCredential{gc},
	}
	if len(guarantee.Signatures) == 0 {
		return fmt.Errorf("onWorkPackageShare: No guarantee signature")
	}

	req := JAMSNPWorkPackageShareResponse{
		WorkReportHash: guarantee.Report.Hash(),
		Signature:      guarantee.Signatures[0].Signature,
	}
	log.Trace(debugG, "onWorkPackageShare", "n", n.String(), "wph", req.WorkReportHash, "wp", workReport.String(), "sig", req.Signature)
	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return err
	}
	// <-- FIN

	return nil
}
