package node

import (

	"github.com/colorfulnotion/jam/common"
//	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

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
			assurance.SetBitFied_Bit(reports[i].CoreIndex, true)
		} else {
			assurance.SetBitFied_Bit(reports[i].CoreIndex, false)
		}
	}
	assurance.Anchor = n.statedb.GetBlock().ParentHash()
	assurance.ValidatorIndex = uint16(n.statedb.GetSafrole().GetCurrValidatorIndex(ed25519Key))
	assurance.Sign(ed25519Priv)
	return assurance, nil
}
