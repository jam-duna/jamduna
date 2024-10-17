package node

import (
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// TODO : check the assurance bucket
// actually the assurance bucket might be useless
// we can use the availability specifier to check if the work report is available
func (n *Node) UpdateAssurancesBucket(oldReports []types.WorkReport) error {
	n.assurancesBucket = make(map[common.Hash]types.IsPackageRecieved)
	reports, err := n.statedb.GetJamState().GetWorkReportFromRho()
	if err != nil {
		return err
	}
	for i := 0; i < len(reports); i++ {
		n.assurancesBucket[reports[i].GetWorkPackageHash()] = types.IsPackageRecieved{
			ExportedSegments: false,
			WorkReportBundle: false,
		}
	}
	// todo: check the leveldb data and update the assurance bucket, see if there is any exported segments or work report bundle
	for _, report := range oldReports {
		n.assurancesBucket[report.GetWorkPackageHash()] = types.IsPackageRecieved{
			ExportedSegments: true,
			WorkReportBundle: true,
		}
	}
	return nil
}

// use the bucket to form the extrinsic
// but can be modified to use the availability specifier

func (n *Node) GenerateAssurance() (types.Assurance, error) {
	ed25519Key := n.GetEd25519Key()
	ed25519Priv := n.GetEd25519Secret()
	reports, err := n.statedb.GetJamState().GetWorkReportFromRho()
	if err != nil {
		return types.Assurance{}, err
	}
	assurance := types.Assurance{}
	for i := 0; i < len(reports); i++ {
		u := n.assurancesBucket[reports[i].GetWorkPackageHash()].WorkReportBundle
		s := n.assurancesBucket[reports[i].GetWorkPackageHash()].ExportedSegments
		if u && s {
			assurance.SetBitFieldBit(reports[i].CoreIndex, true)
		} else {
			assurance.SetBitFieldBit(reports[i].CoreIndex, false)
		}
	}
	assurance.Anchor = n.statedb.GetBlock().ParentHash()
	assurance.ValidatorIndex = uint16(n.statedb.GetSafrole().GetCurrValidatorIndex(ed25519Key))
	assurance.Sign(ed25519Priv)
	return assurance, nil
}

// assureData, given a Guarantee with a AvailabiltySpec within a WorkReport, fetches the bundleShard and segmentShards and stores in ImportDA + AuditDA
func (n *Node) assureData(g types.Guarantee) (err error) {
	spec := g.Report.AvailabilitySpec
	erasureRoot := spec.ErasureRoot
	guarantor := g.Signatures[0].ValidatorIndex // TODO: try any of them, not the 0th one
	bundleShard, segmentShards, justification, err := n.peersInfo[guarantor].SendShardRequest(erasureRoot, n.id, false)
	if err != nil {
		fmt.Printf("%s assureData: SendShardRequest %v\n", n.String(), err)
		return
	}
	if false {
		segmentIndex := make([]uint16, 0) // TODO: Michael
		segmentShardsI, justificationsI, err := n.peersInfo[guarantor].SendSegmentShardRequest(erasureRoot, n.id, segmentIndex, false)
		if err != nil {
			fmt.Printf("%s assureData: SendSegmentShardRequest %v\n", n.String(), err)
			return err
		}
		err = n.store.StoreImportDA(erasureRoot, n.id, segmentShardsI, justificationsI)
		if err != nil {
			fmt.Printf("%s assureData: StoreImportDA %v\n", n.String(), err)
			return err
		}
	}
	err = n.store.StoreAuditDA(erasureRoot, n.id, bundleShard, segmentShards, justification)
	if err != nil {
		fmt.Printf("%s assureData: storeAuditDA %v\n", n.String(), err)
		return
	}
	//TODO: Shawn
	//reports, err := n.statedb.GetJamState().GetWorkReportFromRho()
	//if err != nil {
	//	return
	//}
	a := types.Assurance{
		Anchor:         n.statedb.ParentHash,
		ValidatorIndex: n.id,
	}
	a.SetBitFieldBit(g.Report.CoreIndex, false)
	a.Sign(n.credential.Ed25519Secret[:])
	//if debugA {
	fmt.Printf("%s [assureData] Broadcasting assurance CORE %d\n", n.String(), g.Report.CoreIndex)
	//}
	n.broadcast(a)
	return nil
}
