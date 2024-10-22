package node

import (
	"fmt"
	"reflect"

	//"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) auditWorkReport(workReport types.WorkReport) (err error) {
	erasureRoot := workReport.AvailabilitySpec.ErasureRoot
	// reconstruct the work package
	bundleShards := make([][]byte, types.TotalValidators)
	bundleLength := workReport.AvailabilitySpec.BundleLength
	// TODO: optimize with gofunc ala makeRequests
	for i := uint16(0); i < types.TotalValidators; i++ {
		if i == n.id {
			bundleShard, _, ok, err := n.GetBundleShard(erasureRoot, i)
			if err != nil {
			} else if ok {
				bundleShards[i] = bundleShard
				if debugJ {
					fmt.Printf("%s [auditWorkReport:GetBundleShard] SHARD %d = %d bytes\n", n.String(), i, len(bundleShard))
				}
			}
		} else {
			// bundleShard, bundleJustification, err
			bundleShard, _, err := n.peersInfo[i].SendBundleShardRequest(erasureRoot, i)
			if err != nil {

			} else {
				bundleShards[i] = bundleShard
				if debugJ {
					fmt.Printf("%s [auditWorkReport:SendBundleShardRequest] SHARD %d = %d bytes\n", n.String(), i, len(bundleShard))
				}
			}
		}
	}
	bundleShardsRaw := make([][][]byte, 1)
	bundleShardsRaw[0] = bundleShards
	bundleData, err := n.decode(bundleShardsRaw, false, int(bundleLength))
	if err != nil {
		fmt.Printf("[auditWorkReport:decode] ERR %v\n", err)
		return
	}
	if debugJ {
		fmt.Printf("%s [auditWorkReport:decode] %d bytes\n", n.String(), len(bundleData))
	}
	workPackageBundleRaw, _, err := types.Decode(bundleData, reflect.TypeOf(types.WorkPackageBundle{}))
	if err != nil {
		fmt.Printf("[auditWorkReport] ERR %v\n", err)
		return
	}
	workPackageBundle := workPackageBundleRaw.(types.WorkPackageBundle)
	guarantee, spec, _, err := n.executeWorkPackage(workPackageBundle.WorkPackage)
	if err != nil {
		return
	}
	auditPass := false
	if workReport.AvailabilitySpec.ErasureRoot == spec.ErasureRoot {
		auditPass = true
		fmt.Printf("%s [auditWorkReport:executeWorkPackage] %s AUDIT PASS\n", n.String(), workPackageBundle.WorkPackage.Hash())
	} else {
		fmt.Printf("%s [auditWorkReport:executeWorkPackage] %s AUDIT FAIL\n", n.String(), workPackageBundle.WorkPackage.Hash())
	}

	tranche := uint32(0) // TODO: Shawn
	judgement, err := n.MakeJudgement(guarantee.Report, tranche, auditPass)
	if err != nil {
		return err
	}
	n.broadcast(judgement)
	return nil
}

// we should have a function to check if the block is audited
// then enter the block finalization
func (n *Node) CheckBlockAudited() bool {
	n.judgementMutex.Lock()
	defer n.judgementMutex.Unlock()
	n.announcementMutex.Lock()
	defer n.announcementMutex.Unlock()
	return n.statedb.IsBlockAudited(n.announcementBucket, n.judgementBucket)
}

// if there is a dispute (bad judgement), we should make a dispute extrinsic
func (n *Node) MakeDisputes() error {
	n.judgementMutex.Lock()
	defer n.judgementMutex.Unlock()
	for _, rho := range n.statedb.JamState.AvailabilityAssignments {
		if rho == nil {
			continue
		}
		n.statedb.AppendDisputes(n.judgementBucket, rho.WorkReport.GetWorkPackageHash())
	}
	return nil
}

// every time we make an announcement, we should broadcast it to the network
// announcement before judgement
func (n *Node) MakeAnnouncement(tranche uint32, w types.WorkReportSelection) (types.Announcement, error) {
	ed25519Key := n.GetEd25519Key()
	ed25519Priv := n.GetEd25519Secret()
	index := n.statedb.GetSafrole().GetCurrValidatorIndex(ed25519Key)
	announcement, err := n.statedb.MakeAnnouncement(tranche, w, ed25519Priv, uint32(index))
	if err != nil {
		return types.Announcement{}, err
	}
	n.processAnnouncement(announcement)
	return announcement, nil
}

func (n *Node) MakeJudgement(workreport types.WorkReport, tranche uint32, auditPass bool) (judgement types.Judgement, err error) {
	judgement = types.Judgement{
		Tranche:    tranche,
		Judge:      auditPass,
		Validator:  n.id,
		WorkReport: workreport,
	}
	judgement.Sign(n.GetEd25519Secret())
	return judgement, nil
}

// put it in the announcement bucket
// thus we can check if there is someone absent
func (n *Node) processAnnouncement(announcement types.Announcement) error {
	n.announcementMutex.Lock()
	defer n.announcementMutex.Unlock()
	//put the announcement in the announcement bucket
	index := int(announcement.ValidatorIndex)
	pubkey := n.statedb.GetSafrole().GetCurrValidator(index).Ed25519
	err := announcement.Verify(pubkey) // include the signature verification
	if err != nil {
		return err
	}
	n.announcementBucket.PutAnnouncement(announcement)
	return nil
}

// put it in the judgement bucket
// can use this bucket form the dispute extrinsic
// if it's full set, we should check if there is a bad judgement
// if so, we should make a dispute extrinsic by checking we have the judgement or not
func (n *Node) processJudgement(judgement types.Judgement) error {
	n.judgementMutex.Lock()
	defer n.judgementMutex.Unlock()
	// Store the vote in the tip's queued vote
	// TODO: check if the judgement is false => issue a judge for this work report

	index := int(judgement.Validator)
	pubkey := n.statedb.GetSafrole().GetCurrValidator(index).Ed25519
	err := judgement.Verify(pubkey) // include the signature verification
	if err != nil {
		return err
	}

	n.judgementBucket.PutJudgement(judgement)
	/* ======================================
			this part is for the full set
	======================================= */
	// isFullSet := types.TotalValidators >= 1023
	// if isFullSet {
	// 	if judgement.Judge == false {
	// 		//issue dispute
	// 		Judgement, isJudged := n.judgementBucket.GetJudgement(judgement.WorkReport.GetWorkPackageHash(), judgement.Core)
	// 		if isJudged {
	// 			n.broadcast(Judgement)
	// 		} else {
	// 			wp := types.WorkReportSelection{
	// 				Core:       judgement.Core,
	// 				WorkReport: judgement.WorkReport,
	// 			}
	// 			judgement, err := n.MakeJudgement(wp, judgement.Tranche)
	// 			if err != nil {
	// 				return err
	// 			}

	// 			n.broadcast(judgement)
	// 		}
	// 	}
	// }
	return nil
}

// func (n *Node) Audit() {
// 	//ed25519Key := n.GetEd25519Key()
// 	ed25519Priv := n.GetEd25519Secret()
// 	banderSnatchPriv := n.GetBandersnatchSecret()

// 	// while loop
// 	var tmp uint32
// 	for {
// 		// check if it's new tranche
// 		tranche := n.statedb.GetTranche()
// 		if tmp != tranche {
// 			if tranche == 0 {
// 				a0, err := n.statedb.Select_a0(banderSnatchPriv)
// 				if err != nil {
// 					// log error
// 				}
// 				// announce a0
// 				for _, w := range a0 {
// 					// announce w
// 					announcement, err := n.statedb.MakeAnnouncement(0, w, ed25519Priv, uint32(n.id))
// 					if err != nil {
// 						// handle error
// 						continue
// 					}
// 					n.announcementBucket.PutAnnouncement(announcement)
// 					n.broadcast(announcement)
// 				}
// 				for _, w := range a0 {
// 					judgement, err := n.MakeJudgement(w, 0)
// 					if err != nil {
// 						// handle error
// 						continue
// 					}
// 					n.broadcast(judgement)
// 				}
// 				// Get WorkReportBundle by fetching from the network

// 				// Judge this WorkReportBundle
// 				// broadcast the judgement
// 			} else {
// 				n.prevAnnouncementBucket = n.announcementBucket
// 				n.announcementBucket = types.AnnounceBucket{}
// 				an, err := n.statedb.Select_an(banderSnatchPriv, n.prevAnnouncementBucket, n.judgementBucket)
// 				if err != nil {
// 					// log error
// 				}
// 				// announce an
// 				for _, w := range an {
// 					// announce w
// 					announcement, err := n.statedb.MakeAnnouncement(tranche, w, ed25519Priv, uint32(n.id))
// 					if err != nil {
// 						// handle error
// 						continue
// 					}
// 					n.announcementBucket.PutAnnouncement(announcement)
// 					n.broadcast(announcement)
// 				}
// 				for _, w := range an {
// 					judgement, err := n.MakeJudgement(w, tranche)
// 					if err != nil {
// 						// handle error
// 						continue
// 					}
// 					n.broadcast(judgement)
// 				}
// 				if n.statedb.IsBlockAudited(n.announcementBucket, n.judgementBucket) {
// 					break
// 				}
// 				tmp = tranche
// 			}
// 		} else {
// 			if n.statedb.IsBlockAudited(n.announcementBucket, n.judgementBucket) {
// 				break
// 			}
// 		}

// 	}
// }
