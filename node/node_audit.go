package node

import (
	"fmt"
	"time"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

// initial setup for audit, require auditdb, announcement, and judgement
func (n *Node) initAudit(headerHash common.Hash) {
	// initialized auditingMap
	n.auditingMapMutex.Lock()
	defer n.auditingMapMutex.Unlock()

	n.auditingMap.LoadOrStore(headerHash, &statedb.StateDB{})

	// initialized announcementMap
	announcement := types.TrancheAnnouncement{
		AnnouncementBucket: map[uint32]types.AnnounceBucket{
			0: {},
		},
	}
	n.announcementMap.LoadOrStore(headerHash, announcement)

	// initialized judgementMap
	judgementBucket := types.JudgeBucket{
		Judgements: make(map[common.Hash][]types.Judgement),
	}
	n.judgementMap.LoadOrStore(headerHash, judgementBucket)
}

func (n *Node) addAuditingStateDB(auditdb *statedb.StateDB) error {
	n.auditingMapMutex.Lock()
	defer n.auditingMapMutex.Unlock()

	headerHash := auditdb.GetHeaderHash()
	_, loaded := n.auditingMap.LoadOrStore(headerHash, auditdb)
	if loaded {
		return nil
	}
	return nil
}

func (n *Node) updateAuditingStateDB(auditdb *statedb.StateDB) error {
	n.auditingMapMutex.Lock()
	defer n.auditingMapMutex.Unlock()
	headerHash := auditdb.GetHeaderHash()
	_, exists := n.auditingMap.Load(headerHash)
	if !exists {
		return fmt.Errorf("updateAuditingStateDB failed headerHash %v missing!", headerHash)
	}
	n.auditingMap.Store(headerHash, auditdb)
	return nil
}

func (n *Node) getAuditingStateDB(headerHash common.Hash) (*statedb.StateDB, error) {
	n.auditingMapMutex.Lock()
	defer n.auditingMapMutex.Unlock()
	value, exists := n.auditingMap.Load(headerHash)
	if !exists {
		return nil, fmt.Errorf("headerHash %v not found for auditing", headerHash)
	}
	auditdb, ok := value.(*statedb.StateDB)
	if !ok {
		return nil, fmt.Errorf("invalid type assertion for auditingMap")
	}
	return auditdb, nil
}

func (n *Node) getTrancheAnnouncement(headerHash common.Hash) (*types.TrancheAnnouncement, error) {
	n.announcementMapMutex.Lock()
	defer n.announcementMapMutex.Unlock()

	value, exists := n.announcementMap.Load(headerHash)
	if !exists {
		return nil, fmt.Errorf("trancheAnnouncement %v not found for auditing", headerHash)
	}
	trancheAnnouncement, ok := value.(types.TrancheAnnouncement)
	if !ok {
		return nil, fmt.Errorf("invalid type assertion for announcementMap")
	}
	return &trancheAnnouncement, nil
}

func (n *Node) updateTrancheAnnouncement(headerHash common.Hash, trancheAnnouncement types.TrancheAnnouncement) error {
	n.announcementMapMutex.Lock()
	defer n.announcementMapMutex.Unlock()

	n.announcementMap.Store(headerHash, trancheAnnouncement)
	return nil
}

func (n *Node) checkTrancheAnnouncement(headerHash common.Hash) bool {
	n.announcementMapMutex.Lock()
	defer n.announcementMapMutex.Unlock()

	_, exists := n.announcementMap.Load(headerHash)
	return exists
}

func (n *Node) updateKnownWorkReportMapping(announcement types.Announcement) {
	n.judgementWRMapMutex.Lock()
	defer n.judgementWRMapMutex.Unlock()
	for _, reportHash := range announcement.GetWorkReportHashes() {
		n.judgementWRMap[reportHash] = announcement.HeaderHash
	}
}

func (n *Node) getJudgementBucket(headerHash common.Hash) (*types.JudgeBucket, error) {
	value, exists := n.judgementMap.Load(headerHash)
	if !exists {
		return nil, fmt.Errorf("judgement_bucket %v not found for auditing", headerHash)
	}
	judgementBucket, ok := value.(types.JudgeBucket)
	if !ok {
		return nil, fmt.Errorf("invalid type assertion for judgementMap")
	}
	return &judgementBucket, nil
}

func (n *Node) getHeadHashFromWorkReportHash(workReportHash common.Hash) (common.Hash, error) {
	n.judgementWRMapMutex.Lock()
	defer n.judgementWRMapMutex.Unlock()
	headerHash, exists := n.judgementWRMap[workReportHash]
	if !exists {
		return common.Hash{}, fmt.Errorf("workReportHash %v not found for auditing", workReportHash)
	}
	return headerHash, nil
}

func (n *Node) cleanWaitingAJ() {
	n.waitingAnnouncementsMutex.Lock()
	n.waitingJudgementsMutex.Lock()
	defer n.waitingAnnouncementsMutex.Unlock()
	defer n.waitingJudgementsMutex.Unlock()
	waitingAnnouncements := n.waitingAnnouncements
	waitingJudgements := n.waitingJudgements

	for _, a := range waitingAnnouncements {
		if debugAudit {
			fmt.Printf("%s sending waitingAnnouncements AGAIN!! a=%v\n", n.String(), a.Hash())
		}
		n.announcementsCh <- a
	}
	for _, j := range waitingJudgements {
		if debugAudit {
			fmt.Printf("%s sending waitingJudgements AGAIN!! j=%v\n", n.String(), j.Hash())
		}
		n.judgementsCh <- j
	}

	n.waitingAnnouncements = make([]types.Announcement, 0)
	n.waitingJudgements = make([]types.Judgement, 0)

	return
}

func (n *Node) runAudit() {
	pauseTicker := time.NewTicker(10 * time.Millisecond)

	for {
		select {
		case <-pauseTicker.C:
			// Small pause to reduce CPU load when channels are quiet

		case audit_statedb := <-n.auditingCh:
			headerHash := audit_statedb.GetHeaderHash()
			if debugAudit {
				fmt.Printf("%s [T:%d] start auditing block %v (headerHash:%v)\n", n.String(), audit_statedb.Block.TimeSlot(), audit_statedb.HeaderHash, headerHash)
			}
			err := n.addAuditingStateDB(audit_statedb)
			if err != nil {
				err_log := fmt.Sprintf("addAuditingStateDB failed %v\n", err)
				Logger.RecordLogs(storage.Audit_error, err_log, true)
			}
			n.cleanWaitingAJ()
			n.initAudit(headerHash)
			err = n.Audit(headerHash)
			if err != nil {
				err_log := fmt.Sprintf("Audit Failed %v\n", err)
				Logger.RecordLogs(storage.Audit_error, err_log, true)
			} else {
				// if the block is audited, we can start grandpa

				log := fmt.Sprintf("%s Audit Done ! header = %v, timeslot = %d\n", n.String(), headerHash, audit_statedb.GetTimeslot())
				Logger.RecordLogs(storage.Audit_status, log, true)
				newBlock := audit_statedb.Block.Copy()
				if newBlock.GetParentHeaderHash() == (common.Hash{}) {
					if Grandpa {
						n.StartGrandpa(newBlock.Copy())
					} else {
						genesis_blk := newBlock.Copy()
						n.block_tree = types.NewBlockTree(&types.BT_Node{
							Parent:    nil,
							Block:     genesis_blk,
							Height:    0,
							Finalized: true,
						})
					}
				} else {
					// every time we audited a block, we need to update the block tree
					n.block_tree.AddBlock(newBlock)
					// also prune the block tree
					n.block_tree.PruneBlockTree(10)
				}
			}

		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (n *Node) Audit(headerHash common.Hash) error {
	// in normal situation, we will not have tranche1 unless we have networking problems. we can force it by setting requireTranche1 to true
	// TODO return as bool, err to differentiate between error & audit result
	paulseTicker := time.NewTicker(1 * time.Millisecond)
	tmp := uint32(1 << 31)
	auditing_statedb, err := n.getAuditingStateDB(headerHash)
	if err != nil {
		return err
	}
	tranche := auditing_statedb.GetTranche()
	if tranche == 0 {
		n.ProcessAudit(tranche, headerHash)
	}
	done := false
	for !done {
		select {
		case <-paulseTicker.C:
			auditing_statedb, err := n.getAuditingStateDB(headerHash)
			if err != nil {
				return err
			}
			// use this to force tranch1 to happen but wil comsume unnecessary resource
			tranche := auditing_statedb.GetTranche()
			// use tmp to check if it's a new tranche
			if tranche != tmp && tranche != 0 {
				// if it's the same tranche, check if the block is audited
				// if it's audited, break the loop

				isAudited, err := n.CheckBlockAudited(headerHash, tranche-1)
				if err != nil {
					return fmt.Errorf("CheckBlockAudited failed :%v", err)
				}
				if isAudited {
					// wait for everyone to finish auditing
					if debugAudit {
						fmt.Printf("%s [T:%d] Tranche %v audited block %v \n", n.String(), auditing_statedb.Block.TimeSlot(), tranche-1, auditing_statedb.Block.Header.Hash())
					}
					done = true
					break
				} else {
					if debugAudit {
						fmt.Printf("%s [T:%d] Tranche %v not audited block %v \n", n.String(), auditing_statedb.Block.TimeSlot(), tranche, auditing_statedb.Block.Hash())
					}
				}

				tmp = tranche

				if tranche > 5 {
					//TODO: we should never get here under tiny case
					panic(fmt.Sprintf("%s [T:%d] Audit still not complete after Tranche %v block %v \n", n.String(), auditing_statedb.Block.TimeSlot(), tranche, auditing_statedb.Block.Hash()))
				}
				n.ProcessAudit(tranche, headerHash)
			}

		default:
			time.Sleep(1 * time.Millisecond)
		}

	}
	return nil
}

func (n *Node) ProcessAudit(tranche uint32, headerHash common.Hash) error {
	// announce the work report
	// reports=>work report selection need to be audited
	var reports []types.WorkReportSelection
	// "problematic" node - node that announe but didn't provide judgment. this is not necessarily malicious can be natturally occuring due to networking condition
	// "bench/backup" node - node that does not announe nor provide judgment at tranche0. so it can be used to fulfil problematic node at tranche > 0
	var err error
	/*
		Non-selected Auditor	nothing to do & nothing to be malicious about
		"over-zealous" Auditor	not selected but decides to announce & judge nontheless; is this consider bad or accetable behavior?
		Lazy Announcer	should announce but doesn't ==> NoShow
		Lousy Announcer	announces but doesn't judge
		Lying Judge saying false [LJ-F]	announces, judges False for Truthful Guarantor
		Lying Judge saying true [LJ-T]	announces, judges True for Lying Guarantor
	*/
	auditing_statedb, err := n.getAuditingStateDB(headerHash)

	// normal behavior
	switch n.AuditNodeType {
	case "normal":
		reports, err = n.Announce(headerHash, tranche)
		judges, err := n.Judge(headerHash, reports)
		if err != nil {
			fmt.Printf("Error %v\n", err)
		} else {
			n.DistributeJudgements(judges, headerHash)
		}
	case "lazy_announcer":
		// Lazy Announcer: should announce but doesn't
	case "lousy_announcer":
		// Lousy Announcer: announces but doesn't judge
		reports, err = n.Announce(headerHash, tranche)
	case "lying_judger_F":
		// Lying Judge saying false [LJ-F]: announces, judges False for Truthful Guarantor
		reports, err = n.Announce(headerHash, tranche)
		judges, err := n.Judge(headerHash, reports)
		if err != nil {
			fmt.Printf("Error %v\n", err)
		} else {
			for i := range judges {
				judges[i].Judge = false
				judges[i].Sign(n.GetEd25519Secret())
			}
			n.DistributeJudgements(judges, headerHash)
		}
	case "lying_judger_T":
		// Lying Judge saying true [LJ-T]: announces, judges True for Lying Guarantor
		reports, err = n.Announce(headerHash, tranche)
		judges, err := n.Judge(headerHash, reports)
		if err != nil {
			fmt.Printf("Error %v\n", err)
		} else {
			for i := range judges {
				judges[i].Judge = true
				judges[i].Sign(n.GetEd25519Secret())
			}
			n.DistributeJudgements(judges, headerHash)
		}
	case "non-selected_auditor":
		// Non-selected Auditor: nothing to do & nothing to be malicious about
		if tranche > 0 {
			reports, err = n.Announce(headerHash, tranche)
		}
		judges, err := n.Judge(headerHash, reports)
		if err != nil {
			fmt.Printf("Error %v\n", err)
		} else {
			n.DistributeJudgements(judges, headerHash)
		}

	default:
		reports, err = n.Announce(headerHash, tranche)
		judges, err := n.Judge(headerHash, reports)
		if err != nil {
			fmt.Printf("Error %v\n", err)
		} else {
			n.DistributeJudgements(judges, headerHash)
		}
	}
	if debugAudit && len(reports) == 0 {
		fmt.Printf("%s [T:%d] Tranche %v, no audit reports\n", n.String(), auditing_statedb.Block.TimeSlot(), tranche)
	} else if debugAudit {
		fmt.Printf("%s [T:%d] Tranche %v, audit reports:\n", n.String(), auditing_statedb.Block.TimeSlot(), tranche)
		for _, w := range reports {
			fmt.Printf("%s [T:%d] selected work report %v, from core %d\n", n.String(), auditing_statedb.Block.TimeSlot(), w.WorkReport.Hash(), w.WorkReport.CoreIndex)
		}
	}
	if err != nil {
		fmt.Printf("Error %v\n", err)
	}
	// audit the work report

	return nil
}

func (n *Node) Announce(headerHash common.Hash, tranche uint32) ([]types.WorkReportSelection, error) {

	auditing_statedb, err := n.getAuditingStateDB(headerHash)
	if err != nil {
		return nil, err
	}

	s := auditing_statedb
	judgment_bucket, err := n.getJudgementBucket(headerHash)
	if err != nil {
		return nil, err
	}

	if tranche == 0 {
		banderSnatchSecret := bandersnatch.BanderSnatchSecret(n.GetBandersnatchSecret())
		a0, s0, err := s.Select_a0(banderSnatchSecret)
		if debugAudit {
			for _, w := range a0 {
				fmt.Printf("%s [T:%d] selected work report %v\n", n.String(), auditing_statedb.GetTimeslot(), w.WorkReport.Hash())
			}
		}
		if len(a0) == 0 {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		announcement, err := n.MakeAnnouncement(headerHash, 0, a0)
		n.updateKnownWorkReportMapping(announcement)

		hasmade := n.HaveMadeAnnouncement(announcement, headerHash)

		if err != nil {
			return nil, err
		} else if hasmade {
			fmt.Printf("%s [T:%d] has made announcement %v\n", n.String(), auditing_statedb.GetTimeslot(), announcement.Hash())
			return a0, nil
		} else if !hasmade {
			n.announcementsCh <- announcement
			var announcementWithProof JAMSNPAuditAnnouncementWithProof
			workReports := make([]JAMSNPAuditAnnouncementReport, 0)
			for _, w := range announcement.Selected_WorkReport {
				workReports = append(workReports, JAMSNPAuditAnnouncementReport{
					CoreIndex:      w.Core,
					WorkReportHash: w.WorkReportHash,
				})
			}
			announcementWithProof.Announcement = JAMSNPAuditAnnouncement{
				HeaderHash: s.Block.Header.Hash(),
				Tranche:    0,
				Len:        uint8(len(a0)),
				Reports:    workReports,
				Signature:  announcement.Signature,
			}

			announcementWithProof.Evidence_s0 = types.BandersnatchVrfSignature(s0)
			if debugAudit {
				for _, w := range a0 {
					fmt.Printf("%s [T:%d] broadcasting announcement for %v\n", n.String(), auditing_statedb.Block.TimeSlot(), w.WorkReport.Hash())
				}
			}
			go n.broadcast(announcementWithProof)
		}

		return a0, nil
	} else {
		banderSnatchSecret := bandersnatch.BanderSnatchSecret(n.GetBandersnatchSecret())
		prev_bucket, err := n.GetAnnounceBucketByTranche(tranche-1, headerHash)
		if err != nil {
			return nil, err
		}
		an, no_show_a, no_show_len, sn, err := auditing_statedb.Select_an(banderSnatchSecret, prev_bucket, *judgment_bucket)
		if err != nil {
			return nil, err
		}
		announcement, err := n.MakeAnnouncement(headerHash, tranche, an)
		n.updateKnownWorkReportMapping(announcement)
		n.announcementsCh <- announcement
		if err != nil {
			return nil, err
		} else {
			var announcementWithProof JAMSNPAuditAnnouncementWithProof
			workReports := make([]JAMSNPAuditAnnouncementReport, 0)
			for _, w := range announcement.Selected_WorkReport {
				workReports = append(workReports, JAMSNPAuditAnnouncementReport{
					CoreIndex:      w.Core,
					WorkReportHash: w.WorkReportHash,
				})

			}
			announcementWithProof.Announcement = JAMSNPAuditAnnouncement{
				HeaderHash: s.Block.Header.Hash(),
				Tranche:    uint8(tranche),
				Len:        uint8(len(an)),
				Reports:    workReports,
				Signature:  announcement.Signature,
			}

			announcementWithProof.Evidence_sn = make([]types.BandersnatchVrfSignature, len(sn))
			for i, sig := range sn {
				announcementWithProof.Evidence_sn[i] = types.BandersnatchVrfSignature(sig)
			}
			announcementWithProof.NoShowLength = no_show_len
			announcementWithProof.Evidence_sn_no_show = no_show_a
			if debugAudit {
				for _, w := range an {
					fmt.Printf("%s [T:%d] broadcasting announcement for %v\n", n.String(), auditing_statedb.GetTimeslot(), w.WorkReport.Hash())
				}
			}

			go n.broadcast(announcementWithProof)
		}

		return an, nil
	}

}

func (n *Node) HaveMadeAnnouncement(announcement types.Announcement, headerHash common.Hash) bool { // TODO: add mutex
	value, err := n.getTrancheAnnouncement(headerHash)
	if err != nil {
		return false
	}
	bucket := value.AnnouncementBucket[announcement.Tranche]
	return bucket.HaveMadeAnnouncement(announcement)
}

func (n *Node) GetAnnounceBucketByTranche(tranche uint32, headerHash common.Hash) (types.AnnounceBucket, error) {
	value, err := n.getTrancheAnnouncement(headerHash)
	if err != nil {
		return types.AnnounceBucket{}, err
	}
	bucket, exists := value.AnnouncementBucket[tranche]
	if !exists {
		return types.AnnounceBucket{}, fmt.Errorf("tranche %v not found for auditing", tranche)
	}
	return bucket, nil
}

func (n *Node) Judge(headerHash common.Hash, workReports []types.WorkReportSelection) (judgements []types.Judgement, err error) {
	start := time.Now()
	judgement_bucket, err := n.getJudgementBucket(headerHash)
	if len(workReports) == 0 {
		return nil, nil
	}
	for _, w := range workReports {
		hasmade := judgement_bucket.HaveMadeJudgementByValidator(w.WorkReport.Hash(), uint16(n.GetCurrValidatorIndex()))
		if !hasmade {
			j, err := n.auditWorkReport(w.WorkReport, headerHash)
			if err != nil {
				return nil, err
			}
			judgements = append(judgements, j)
		} else {
			j, err := judgement_bucket.GetJudgementByValidator(w.WorkReport.Hash(), uint16(n.GetCurrValidatorIndex()))
			if err != nil {
				return nil, err
			}
			judgements = append(judgements, j)
		}
		if err != nil {
			return nil, err
		}
	}
	if debugE {
		fmt.Printf("%s Judge time: %v\n", n.String(), time.Since(start))
	}
	return
}

func (n *Node) auditWorkReport(workReport types.WorkReport, headerHash common.Hash) (judgement types.Judgement, err error) {
	judgement = types.Judgement{}
	auditing_statedb, err := n.getAuditingStateDB(headerHash)
	if err != nil {
		return
	}
	judgment_bucket, err := n.getJudgementBucket(headerHash)
	if err != nil {
		return
	}

	for _, j := range judgment_bucket.Judgements[workReport.Hash()] {
		if j.Validator == n.id {
			fmt.Printf("%s [T:%d] has made judgement %v\n", n.String(), auditing_statedb.GetTimeslot(), j.WorkReportHash)
			return j, nil
		}
	}

	n.workReportsMutex.Lock()
	if wr, exists := n.workReports[workReport.Hash()]; exists {
		var core uint16
		core, err = n.GetSelfCoreIndex()
		if err != nil {
			fmt.Printf("coreBroadcast Error: %v\n", err)
			return
		}
		if wr.AvailabilitySpec.ErasureRoot == workReport.AvailabilitySpec.ErasureRoot && wr.CoreIndex == core {
			n.workReportsMutex.Unlock()
			judgement, err = n.MakeJudgement(workReport, true)

			return
		}
	}
	n.workReportsMutex.Unlock()
	/* think about it - part A

	workPackageBundle, exist := n.knownPackageBundle[report_hash]
	if ! exist {
		// you really need to reconstruct it...
	}
	*/
	start := time.Now()

	//TODO: use segmentRootLookup
	spec := workReport.AvailabilitySpec
	err = n.StoreImportDAWorkReportMap(spec)
	if err != nil {
		fmt.Printf("%s [auditWorkReport:StoreImportDAWorkReportMap] ERR %v\n", n.String(), err)
		return
	}

	erasureRoot := spec.ErasureRoot
	bundleLength := spec.BundleLength
	workReportCoreIdx := workReport.CoreIndex
	workPackageHash := spec.WorkPackageHash
	workPackageBundle, fetchErr := n.FetchWorkPackageBundle(workPackageHash, erasureRoot, bundleLength)
	if err != nil {
		fmt.Printf("WP=%v | erasureRoot=%v|len=%v [auditWorkReport:FetchWorkPackageBundle] ERR %v\n", workPackageHash, erasureRoot, bundleLength, fetchErr)
		return
	}
	//segmentRootLookup, err := n.GetSegmentRootLookup(workPackageBundle.WorkPackage)

	segmentRootLookup := workReport.SegmentRootLookup // use workReport's segmentRootLookup
	if debugE {
		fmt.Printf("%s auditWorkReport:fetch and decode time: %v\n", n.String(), time.Since(start))
	}
	if debugAudit {
		fmt.Printf("WP=%v | len=%v | byte=%x\n", workPackageBundle.PackageHash(), len(workPackageBundle.Bytes()), workPackageBundle.Bytes())
	}
	wr, err := n.executeWorkPackageBundle(workReportCoreIdx, workPackageBundle, segmentRootLookup)
	if err != nil {
		return
	} else {
		n.workReportsCh <- workReport
	}
	auditPass := false
	if workReport.AvailabilitySpec.ErasureRoot == wr.AvailabilitySpec.ErasureRoot {
		auditPass = true

		audit_log := fmt.Sprintf("%s [auditWorkReport:executeWorkPackageBundle] %s AUDIT PASS\n", n.String(), workPackageBundle.WorkPackage.Hash())
		Logger.RecordLogs(storage.Audit_status, audit_log, true)

	} else {

		audit_log := fmt.Sprintf("%s [auditWorkReport:executeWorkPackageBundle] %s AUDIT FAIL\n", n.String(), workPackageBundle.WorkPackage.Hash())
		Logger.RecordLogs(storage.Audit_status, audit_log, true)

	}

	judgement, err = n.MakeJudgement(workReport, auditPass)

	return
}

func (n *Node) DistributeJudgements(judges []types.Judgement, headerHash common.Hash) {
	for _, j := range judges {
		if debugAudit {
			fmt.Printf("%s distributing judgement %v\n", n.String(), j.WorkReportHash)
		}
		go n.broadcast(j)
		n.judgementsCh <- j
	}
}

// we should have a function to check if the block is audited
// then enter the block finalization
func (n *Node) CheckBlockAudited(headerHash common.Hash, tranche uint32) (bool, error) {

	auditing_statedb, err := n.getAuditingStateDB(headerHash)
	if err != nil {
		return false, err
	}
	judgment_bucket, err := n.getJudgementBucket(headerHash)
	if err != nil {
		return false, err
	}
	announcementBucket, err := n.GetAnnounceBucketByTranche(tranche, headerHash)
	if err != nil {
		return false, err
	}
	isBlockAudited := auditing_statedb.IsBlockAudited(announcementBucket, *judgment_bucket)
	if debugAudit {
		for _, w := range auditing_statedb.AvailableWorkReport {
			fmt.Printf("%s [T:%d] CheckBlockAudited Have %d announcement\n", n.String(), auditing_statedb.Block.TimeSlot(), len(announcementBucket.Announcements[w.Hash()]))
			fmt.Printf("%s [T:%d] CheckBlockAudited Have %d judgement\n", n.String(), auditing_statedb.Block.TimeSlot(), len(judgment_bucket.Judgements[w.Hash()]))

		}
	}
	// try to form disputes if not audited

	disputed, err := n.MakeDisputes(headerHash)
	if disputed {

		audting_statedb, err := n.getAuditingStateDB(headerHash)
		if err != nil {
			return false, err
		}
		audting_statedb.Block.Extrinsic.Disputes.Print()
		return false, fmt.Errorf("%s Block %v is not audited -- issue disputes at tranche: %d", n.String(), headerHash, tranche)
	}

	return isBlockAudited, nil
}

// if there is a dispute (bad judgement), we should make a dispute extrinsic
func (n *Node) MakeDisputes(headerHash common.Hash) (bool, error) {
	var err_dispute error
	err_dispute = fmt.Errorf("No Need To Dispute")
	auditing_statedb, err := n.getAuditingStateDB(headerHash)
	if err != nil {
		return false, err
	}

	judgement_bucket, err := n.getJudgementBucket(headerHash)
	if err != nil {
		return false, err
	}
	for _, awr := range auditing_statedb.AvailableWorkReport {
		old_eg, err := n.TraceOldGuarantee(headerHash, awr.GetWorkPackageHash())
		if err != nil {
			return false, err
		}
		err_dispute = auditing_statedb.AppendDisputes(judgement_bucket, awr.Hash(), old_eg)
	}
	if err_dispute == nil {
		auditing_statedb.Block.Extrinsic.Disputes.FormatDispute()
		n.updateAuditingStateDB(auditing_statedb)
		return true, nil
	}
	return false, nil
}

func (n *Node) TraceOldGuarantee(headerHash common.Hash, workpackage_hash common.Hash) (types.Guarantee, error) {
	auditing_statedb, err := n.getAuditingStateDB(headerHash)
	if err != nil {
		return types.Guarantee{}, err
	}
	parent_hash := auditing_statedb.Block.GetParentHeaderHash()
	empty := common.Hash{}
	n.statedbMapMutex.Lock()
	defer n.statedbMapMutex.Unlock()
	// TODO: change to eg time slots limit
	for i := 0; i < 100; i++ {
		if parent_hash == empty {
			return types.Guarantee{}, fmt.Errorf("TraceOldGuarantee: %v no parent hash", headerHash)
		}
		if statedb, exists := n.statedbMap[parent_hash]; exists {
			block := statedb.Block
			for _, block_eg := range block.Extrinsic.Guarantees {
				if block_eg.Report.AvailabilitySpec.WorkPackageHash == workpackage_hash {
					return block_eg, nil
				}
			}
			parent_hash = block.GetParentHeaderHash()
		} else {
			return types.Guarantee{}, fmt.Errorf("TraceOldGuarantee: parent %v not found", parent_hash)
		}
	}
	return types.Guarantee{}, fmt.Errorf("TraceOldGuarantee: wp %v not found", workpackage_hash)
}

// every time we make an announcement, we should broadcast it to the network
// announcement before judgement
func (n *Node) MakeAnnouncement(headerHash common.Hash, tranche uint32, w []types.WorkReportSelection) (types.Announcement, error) {
	ed25519Key := n.GetEd25519Key()
	ed25519Priv := n.GetEd25519Secret()

	auditing_statedb, err := n.getAuditingStateDB(headerHash)
	if err != nil {
		return types.Announcement{}, err
	}

	index := auditing_statedb.GetSafrole().GetCurrValidatorIndex(ed25519Key)
	announcement, err := auditing_statedb.MakeAnnouncement(tranche, w, ed25519Priv, uint32(index))
	if err != nil {
		return types.Announcement{}, err
	}
	return announcement, nil
}

func (n *Node) MakeJudgement(workreport types.WorkReport, auditPass bool) (judgement types.Judgement, err error) {
	judgement = types.Judgement{
		Judge:          auditPass,
		Validator:      n.id,
		WorkReportHash: workreport.Hash(),
	}
	judgement.Sign(n.GetEd25519Secret())
	return judgement, nil
}

// put it in the announcement bucket
// thus we can check if there is someone absent
func (n *Node) processAnnouncement(announcement types.Announcement) error {
	headerHash := announcement.HeaderHash
	if !n.checkTrancheAnnouncement(headerHash) {
		n.waitingAnnouncementsMutex.Lock()
		defer n.waitingAnnouncementsMutex.Unlock()
		if announcement.Tranche != 0 {
			return fmt.Errorf("No way header %v not found for auditing statedb", announcement.HeaderHash)
		}
		n.waitingAnnouncements = append(n.waitingAnnouncements, announcement)
		return nil
	}
	s, err := n.getAuditingStateDB(headerHash)
	if err != nil {
		fmt.Printf("%s [audit:processAnnouncement] auditingDB not found %v \n", n.String(), headerHash)
		return err
	}
	index := int(announcement.ValidatorIndex)
	pubkey := s.GetSafrole().GetCurrValidator(index).Ed25519

	err = announcement.Verify(pubkey)
	if err != nil {
		fmt.Printf("%s [audit:processAnnouncement] announcement(%v) not verified %v \n", n.String(), announcement.Hash(), headerHash)
		return err
	}
	n.updateKnownWorkReportMapping(announcement)

	trancheAnnouncement, err := n.getTrancheAnnouncement(headerHash)
	if err != nil {
		fmt.Printf("%s [audit:processAnnouncement] trancheAnnouncement not found %v \n", n.String(), headerHash)
		return err
	}
	err = trancheAnnouncement.PutAnnouncement(announcement)
	if err != nil {
		fmt.Printf("%s [audit:processAnnouncement] trancheAnnouncement.PutAnnouncement failed %v \n", n.String(), headerHash)
		return err
	}
	n.updateTrancheAnnouncement(headerHash, *trancheAnnouncement)
	return nil
}

// put it in the judgement bucket
// can use this bucket form the dispute extrinsic
// if it's full set, we should check if there is a bad judgement
// if so, we should make a dispute extrinsic by checking we have the judgement or not
func (n *Node) processJudgement(judgement types.Judgement) error {
	header, err := n.getHeadHashFromWorkReportHash(judgement.WorkReportHash)
	if !n.checkTrancheAnnouncement(header) || err != nil {
		n.waitingJudgementsMutex.Lock()
		defer n.waitingJudgementsMutex.Unlock()
		n.waitingJudgements = append(n.waitingJudgements, judgement)
		return nil
	}

	headerHash, err := n.getHeadHashFromWorkReportHash(judgement.WorkReportHash)
	if err != nil {
		return err
	}
	auditing_statedb, err := n.getAuditingStateDB(headerHash)
	if err != nil {
		return err
	}

	index := int(judgement.Validator)
	pubkey := auditing_statedb.GetSafrole().GetCurrValidator(index).Ed25519
	err = judgement.Verify(pubkey)
	if err != nil {
		return err
	}
	judgementBucket, err := n.getJudgementBucket(headerHash)
	if err != nil {
		return err
	}
	judgementBucket.PutJudgement(judgement)
	n.judgementMap.Store(headerHash, *judgementBucket)
	if !judgement.Judge {
		if judgementBucket.HaveMadeJudgementByValidator(judgement.WorkReportHash, uint16(n.GetCurrValidatorIndex())) {
			return nil
		} else {
			var audit_report types.WorkReport
			for _, a := range auditing_statedb.AvailableWorkReport {
				if a.Hash() == judgement.WorkReportHash {
					audit_report = a
				}
			}
			emptyhash := common.Hash{}
			if audit_report.Hash() == emptyhash {
				return fmt.Errorf("work report not found")
			}
			audit_j, err := n.auditWorkReport(audit_report, headerHash)
			if err != nil {
				return err
			}
			n.DistributeJudgements([]types.Judgement{audit_j}, headerHash)

		}
	}

	return nil
}
