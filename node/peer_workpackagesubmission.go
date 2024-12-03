package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"

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
		return fmt.Errorf("failed to read CoreIndex: %w", err)
	}

	// Deserialize WorkPackage (dynamically sized)
	workPackageBytes := make([]byte, buf.Len()) // Remaining bytes are for the WorkPackage
	if _, err := buf.Read(workPackageBytes); err != nil {
		return fmt.Errorf("failed to read WorkPackage bytes: %w", err)
	}

	// Decode the WorkPackage
	wp, _, err := types.Decode(workPackageBytes, reflect.TypeOf(types.WorkPackage{}))
	if err != nil {
		return fmt.Errorf("error in decoding WorkPackage: %w", err)
	}

	// Type assertion for WorkPackage
	workPackage, ok := wp.(types.WorkPackage)
	if !ok {
		return fmt.Errorf("decoded value is not of type WorkPackage")
	}

	// Assign decoded WorkPackage to the struct field
	pkg.WorkPackage = workPackage
	return nil
}
func (p *Peer) SendWorkPackageSubmission(pkg types.WorkPackage, extrinsics []byte) (err error) {
	if pkg.RefineContext.LookupAnchorSlot == 1 {
		if len(pkg.RefineContext.Prerequisites) == 0 {
			panic("Prerequisite is empty")
		}
	}
	core_idx, err := p.node.GetCoreIndexFromEd25519Key(p.Validator.Ed25519)
	if err != nil {
		return fmt.Errorf("failed to get self core index: %w", err)
	}

	req := JAMSNPWorkPackage{
		CoreIndex:   core_idx,
		WorkPackage: pkg,
	}
	/*
		Here need to setup some kind of verification for the work package
	*/

	// a, err := json.MarshalIndent(req, "", "  ")
	// if err != nil {
	// 	return err
	// }
	// fmt.Printf("send workpackage: %s\n", a)

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	stream, _ := p.openStream(CE133_WorkPackageSubmission)
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return err
	}
	if debugG {
		fmt.Printf("%s submitted Workpackage %d bytes\n", p.String(), len(reqBytes))
	}
	/*
		// TODO: write extrinsics
		err = sendQuicBytes(stream, extrinsics)
		if err != nil {
			return err
		}*/

	return nil
}
func (n *Node) onWorkPackageSubmission(stream quic.Stream, msg []byte) (err error) {

	var newReq JAMSNPWorkPackage
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}

	// a, err := json.MarshalIndent(newReq, "", "  ")
	// if err != nil {
	// 	return err
	// }
	// fmt.Printf("receive workpackage: %s\n", a)

	selfCoreIndex, err := n.GetSelfCoreIndex()
	if err != nil {
		return fmt.Errorf("failed to get self core index: %w", err)
	}
	if newReq.CoreIndex != selfCoreIndex {
		return fmt.Errorf("Core index mismatch: %d != %d", newReq.CoreIndex, selfCoreIndex)
	}
	if debugG {
		fmt.Printf("%s received Work Package Submission [CORE %+v]\n", n.String(), newReq.CoreIndex)
	}
	// TODO: read extrinsics

	// TODO: Sourabh check if this even makes sense
	//n.workPackagesCh <- newReq.WorkPackage
	n.broadcastWorkpackage(newReq.WorkPackage)
	return nil
}
