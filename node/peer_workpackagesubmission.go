package node

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"time"

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
	CoreIndex   uint16                `json:"coreIndex"`
	WorkPackage types.WorkPackage     `json:"workPackage"`
	Extrinsic   types.ExtrinsicsBlobs `json:"extrinsic"`
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

func (p *Peer) SendWorkPackageSubmission(pkg types.WorkPackage, extrinsics types.ExtrinsicsBlobs, core_idx uint16) (err error) {
	if pkg.RefineContext.LookupAnchorSlot == 1 {
		if len(pkg.RefineContext.Prerequisites) == 0 {
			panic("Prerequisite is empty")
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
	stream, err := p.openStream(CE133_WorkPackageSubmission)
	if err != nil {
		return err
	}
	defer stream.Close()
	err = sendQuicBytes(stream, reqBytes)
	if err != nil {
		return err
	}
	if debugG {
		fmt.Printf("%s submitted Workpackage %d bytes to core %d\n", p.String(), len(reqBytes), core_idx)
	}

	return nil
}

func (n *Node) onWorkPackageSubmission(stream quic.Stream, msg []byte) (err error) {
	defer stream.Close()
	var newReq JAMSNPWorkPackage
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		if debugG {
			fmt.Println("Error deserializing:", err)
		}
		if err != nil {
			return
		}
	}

	// a, err := json.MarshalIndent(newReq, "", "  ")
	// if err != nil {
	// 	return err
	// }
	// fmt.Printf("receive workpackage: %s\n", a)
	curr_statedb := n.statedb.Copy()
	selfCoreIndex := curr_statedb.GetSelfCoreIndex()
	if err != nil {
		return fmt.Errorf("failed to get self core index: %w", err)
	}
	// reject if the work package is not for this core
	if newReq.CoreIndex != selfCoreIndex {
		return fmt.Errorf("work package submission for core %d received by core %d", newReq.CoreIndex, selfCoreIndex)
	}

	// TODO: Sourabh check if this even makes sense
	//n.workPackagesCh <- newReq.WorkPackage
	//we use current timeslot to broadcast the workpackage, because if we it might be possible that the timeslot has changed by the time the workpackage is executed
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func(ctx context.Context) {
		done := make(chan error, 1)
		go func() {
			_, err := n.broadcastWorkpackage(newReq.WorkPackage, newReq.CoreIndex, curr_statedb, newReq.Extrinsic)
			done <- err
		}()

		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				fmt.Printf("broadcastWorkpackage timed out | wp=%v, core=%v\n", newReq.WorkPackage.Hash(), newReq.CoreIndex)
			} else if ctx.Err() != context.Canceled {
				fmt.Printf("broadcastWorkpackage ERR | wp=%v, core=%v, err %v\n", newReq.WorkPackage.Hash(), newReq.CoreIndex, ctx.Err())
			}
		case err := <-done:
			if err != nil {
				fmt.Printf("%s broadcastWorkpackage Error: %v\n", n.String(), err)
			}
		}
	}(ctx)
	return nil
}
