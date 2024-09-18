package node

import (
	"encoding/json"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) Tiny_Audit() error {
	// For tiny setup , we just audit without going thorugh tranche
	W := n.statedb.GetWorkReportNeedAuditTiny()
	jsonData, err := json.MarshalIndent(W, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal work reports: %v", err)
	}
	fmt.Println(string(jsonData))
	W_selected := statedb.WorkReportToSelection(W)
	for _, w := range W_selected {
		// announce w
		announcement, err := n.MakeAnnouncement(0, w)
		n.broadcast(announcement)
		// judge w
		fmt.Printf("Node[%d] is judging work report %v\n", n.id, w)
		judgement, err := n.MakeJudgement(w, 0)
		if err != nil {
			//handle error
			continue
		}
		n.broadcast(judgement)
	}

	return nil

}

func (n *Node) CheckBlockAudited() bool {
	n.judgementMutex.Lock()
	defer n.judgementMutex.Unlock()
	n.announcementMutex.Lock()
	defer n.announcementMutex.Unlock()
	return n.statedb.IsBlockAudited(n.announcementBucket, n.judgementBucket)
}

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

func (n *Node) Audit() {
	//ed25519Key := n.GetEd25519Key()
	ed25519Priv := n.GetEd25519Secret()
	banderSnatchPriv := n.GetBandersnatchSecret()

	// while loop
	var tmp uint32
	for {
		// check if it's new tranche
		tranche := n.statedb.GetTranche()
		if tmp != tranche {
			if tranche == 0 {
				a0, err := n.statedb.Select_a0(banderSnatchPriv)
				if err != nil {
					// log error
				}
				// announce a0
				for _, w := range a0 {
					// announce w
					announcement, err := n.statedb.MakeAnnouncement(0, w, ed25519Priv, uint32(n.id))
					if err != nil {
						// handle error
						continue
					}
					n.announcementBucket.PutAnnouncement(announcement)
					n.broadcast(announcement)
				}
				for _, w := range a0 {
					judgement, err := n.MakeJudgement(w, 0)
					if err != nil {
						// handle error
						continue
					}
					n.broadcast(judgement)
				}
				// Get WorkReportBundle by fetching from the network

				// Judge this WorkReportBundle
				// broadcast the judgement
			} else {
				n.prevAnnouncementBucket = n.announcementBucket
				n.announcementBucket = types.AnnounceBucket{}
				an, err := n.statedb.Select_an(banderSnatchPriv, n.prevAnnouncementBucket, n.judgementBucket)
				if err != nil {
					// log error
				}
				// announce an
				for _, w := range an {
					// announce w
					announcement, err := n.statedb.MakeAnnouncement(tranche, w, ed25519Priv, uint32(n.id))
					if err != nil {
						// handle error
						continue
					}
					n.announcementBucket.PutAnnouncement(announcement)
					n.broadcast(announcement)
				}
				for _, w := range an {
					judgement, err := n.MakeJudgement(w, tranche)
					if err != nil {
						// handle error
						continue
					}
					n.broadcast(judgement)
				}
				if n.statedb.IsBlockAudited(n.announcementBucket, n.judgementBucket) {
					break
				}
				tmp = tranche
			}
		} else {
			if n.statedb.IsBlockAudited(n.announcementBucket, n.judgementBucket) {
				break
			}
		}

	}
}

func (n *Node) Judge(types.WorkReportSelection) bool {
	//TODO: work package=>work report here
	return false
}

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

func (n *Node) MakeJudgement(w types.WorkReportSelection, tranche uint32) (types.Judgement, error) {
	var judgement types.Judgement
	ed25519Key := n.GetEd25519Key()
	ed25519Priv := n.GetEd25519Secret()

	index := n.statedb.GetSafrole().GetCurrValidatorIndex(ed25519Key)

	if n.Judge(w) {
		judgement, err := n.statedb.MakeJudgement(tranche, w, true, ed25519Priv, uint16(index))
		if err != nil {
			return types.Judgement{}, err
		}
		n.processJudgement(judgement)
		return judgement, nil
	} else {
		judgement, err := n.statedb.MakeJudgement(tranche, w, false, ed25519Priv, uint16(index))
		if err != nil {
			return types.Judgement{}, err
		}
		n.processJudgement(judgement)
		return judgement, nil
	}
	return judgement, nil

}

func (n *Node) processAnnouncement(announcement types.Announcement) error {
	n.announcementMutex.Lock()
	defer n.announcementMutex.Unlock()
	//put the announcement in the announcement bucket
	index := int(announcement.ValidatorIndex)
	pubkey := n.statedb.GetSafrole().GetCurrValidator(index).Ed25519
	err := announcement.ValidateSignature(pubkey) // include the signature verification
	if err != nil {
		return err
	}
	n.announcementBucket.PutAnnouncement(announcement)
	return nil
}

func (n *Node) processJudgement(judgement types.Judgement) error {
	n.judgementMutex.Lock()
	defer n.judgementMutex.Unlock()
	// Store the vote in the tip's queued vote
	// TODO: check if the judgement is false => issue a judge for this work report
	emptyHash := common.Hash{}
	if judgement.WorkReport.GetWorkPackageHash() == emptyHash {
		return fmt.Errorf("work report hash is nil")
	}
	index := int(judgement.Validator)
	pubkey := n.statedb.GetSafrole().GetCurrValidator(index).Ed25519
	err := judgement.ValidateSignature(pubkey) // include the signature verification
	if err != nil {
		return err
	}

	n.judgementBucket.PutJudgement(judgement)

	isFullSet := types.TotalValidators >= 1023
	if isFullSet {
		if judgement.Judge == false {
			//issue dispute
			Judgement, isJudged := n.judgementBucket.GetJudgement(judgement.WorkReport.GetWorkPackageHash(), judgement.Core)
			if isJudged {
				n.broadcast(Judgement)
			} else {
				wp := types.WorkReportSelection{
					Core:       judgement.Core,
					WorkReport: judgement.WorkReport,
				}
				judgement, err := n.MakeJudgement(wp, judgement.Tranche)
				if err != nil {
					return err
				}

				n.broadcast(judgement)
			}
		}
	}
	return nil
}
