package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
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

func (p *Peer) SendWorkPackageSubmission(coreIndex uint16, pkg types.WorkPackage, extrinsics []byte) (err error) {
	req := JAMSNPWorkPackage{
		CoreIndex:   coreIndex,
		WorkPackage: pkg,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	stream, err := p.openStream(CE133_WorkPackageSubmission)
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return err
	}
	err = sendQuicBytes(stream, extrinsics)
	if err != nil {
		return err
	}

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
	// TODO: read extrinsics
	fmt.Println("Work Package Submission:", msg)

	return nil
}
