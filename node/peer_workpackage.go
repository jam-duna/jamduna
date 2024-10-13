package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"io"
	"reflect"
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
	CoreIndex   uint16            `json:"coreIndex"`
	WorkPackage types.WorkPackage `json:"workPackage"`
}

// ToBytes serializes the JAMSNPWorkPackage struct into a byte array
func (pkg *JAMSNPWorkPackage) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize CoreIndex (2 bytes)
	if err := binary.Write(buf, binary.BigEndian, pkg.CoreIndex); err != nil {
		return nil, err
	}

	// Serialize WorkPackage (dynamically sized, using WorkPackage's ToBytes method)
	workPackageBytes := pkg.WorkPackage.Bytes()
	if _, err := buf.Write(workPackageBytes); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPWorkPackage struct
func (pkg *JAMSNPWorkPackage) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize CoreIndex (2 bytes)
	if err := binary.Read(buf, binary.BigEndian, &pkg.CoreIndex); err != nil {
		return err
	}

	// Deserialize WorkPackage (dynamically sized)
	workPackageBytes := make([]byte, buf.Len()) // Remaining bytes are for the WorkPackage
	wp, _, err := types.Decode(workPackageBytes, reflect.TypeOf(types.WorkPackage{}))
	if err != nil {
		fmt.Println("Error in decodeWorkPackage:", err)
	}
	pkg.WorkPackage = wp.(types.WorkPackage)
	return nil
}

func (p *Peer) processWorkPackageSubmission(msg []byte) (err error) {
	var newReq JAMSNPWorkPackage
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	// TODO: read extrinsics
	fmt.Println("Work Package Submission:", msg)

	return nil
}

func (p *Peer) broadcastWorkPackageSubmission(coreIndex uint16, pkg types.WorkPackage, extrinsics []byte) (err error) {
	req := JAMSNPWorkPackage{
		CoreIndex:   coreIndex,
		WorkPackage: pkg,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	p.sendCode(CE133_WorkPackageSubmission)
	err = p.sendQuicBytes(reqBytes)
	if err != nil {
		return err
	}
	err = p.sendQuicBytes(extrinsics)
	if err != nil {
		return err
	}
	// --> FIN
	p.sendFIN()
	// <-- FIN
	p.receiveFIN()

	return nil
}

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
	WorkPackageHash common.Hash `json:"workPackageHash"`
	SegmentRoot     common.Hash `json:"segmentRoot"`
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
	CoreIndex    uint16                     `json:"coreIndex"`
	Len          uint8                      `json:"len"`
	SegmentRoots []JAMSNPSegmentRootMapping `json:"segmentRoots"`
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

	return nil
}

type JAMSNPWorkPackageShareResponse struct {
	WorkReportHash common.Hash `json:"workReportHash"`
	Signature      types.Ed25519Signature
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

func (p *Peer) ShareWorkPackage(coreIndex uint16, workpackagehash []common.Hash, segmentRoot []common.Hash, bundle []byte) (err error) {
	segmentroots := make([]JAMSNPSegmentRootMapping, 0)
	for i, h := range workpackagehash {
		segmentroots = append(segmentroots, JAMSNPSegmentRootMapping{
			WorkPackageHash: h,
			SegmentRoot:     segmentRoot[i],
		})
	}
	req := JAMSNPWorkPackageShare{
		CoreIndex:    coreIndex,
		SegmentRoots: segmentroots,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	p.sendCode(CE134_WorkPackageShare)
	err = p.sendQuicBytes(reqBytes)
	if err != nil {
		return err
	}
	err = p.sendQuicBytes(bundle)
	if err != nil {
		return err
	}
	// <-- Work Report Hash ++ Ed25519 Signature
	respBytes, err := p.receiveQuicBytes()
	if err != nil {
		return err
	}

	var newReq JAMSNPWorkPackageShareResponse
	err = newReq.FromBytes(respBytes)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	// --> FIN
	p.sendFIN()
	// <-- FIN
	p.receiveFIN()

	return nil
}

func (p *Peer) processWorkPackageShare(msg []byte) (err error) {
	// --> Core Index ++ Segment Root Mappings
	var newReq JAMSNPWorkPackageShare
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	// --> Work Package Bundle
	bundle, err := p.receiveQuicBytes()
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}

	workpackagehashes := make([]common.Hash, 0)
	segmentroots := make([]common.Hash, 0)

	for _, sr := range newReq.SegmentRoots {
		workpackagehashes = append(workpackagehashes, sr.WorkPackageHash)
		segmentroots = append(segmentroots, sr.SegmentRoot)
	}

	workPackageHash, signature, err := p.node.RefineBundle(newReq.CoreIndex, workpackagehashes, segmentroots, bundle)
	if err != nil {
		return
	}
	req := JAMSNPWorkPackageShareResponse{
		WorkReportHash: workPackageHash,
		Signature:      signature,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	err = p.sendQuicBytes(reqBytes)
	if err != nil {
		return err
	}
	// --> FIN
	p.receiveFIN()
	// <-- FIN
	p.sendFIN()

	return nil
}
