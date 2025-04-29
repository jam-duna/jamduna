package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

// initial setup for audit, require auditdb, announcement, and judgement (need mutex)
func (n *Node) initAudit(headerHash common.Hash) {
	// initialized auditingMap
	n.auditingMapMutex.Lock()

	if _, exists := n.auditingMap[headerHash]; !exists {
		n.auditingMap[headerHash] = &statedb.StateDB{}
	}
	n.auditingMapMutex.Unlock()

	// initialized announcementMap
	n.announcementMapMutex.Lock()
	if _, exists := n.announcementMap[headerHash]; !exists {
		n.announcementMap[headerHash] = &types.TrancheAnnouncement{
			AnnouncementBucket: map[uint32]*types.AnnounceBucket{
				0: {},
			},
		}
	}
	n.announcementMapMutex.Unlock()
	// initialized judgementMap
	n.judgementMapMutex.Lock()
	if _, exists := n.judgementMap[headerHash]; !exists {
		n.judgementMap[headerHash] = &types.JudgeBucket{
			Judgements: make(map[common.Hash][]types.Judgement),
		}
	}
	n.judgementMapMutex.Unlock()
	return
}

// this function is used to add the auditingMap (need mutex)
func (n *Node) addAuditingStateDB(auditdb *statedb.StateDB) error {
	n.auditingMapMutex.Lock()
	defer n.auditingMapMutex.Unlock()
	headerHash := auditdb.GetHeaderHash()
	if _, loaded := n.auditingMap[headerHash]; loaded {
		return nil
	}
	n.auditingMap[headerHash] = auditdb
	return nil
}

// this function is used to update the auditingMap (need mutex)
func (n *Node) updateAuditingStateDB(auditdb *statedb.StateDB) error {
	n.auditingMapMutex.Lock()
	defer n.auditingMapMutex.Unlock()

	headerHash := auditdb.GetHeaderHash()
	if _, exists := n.auditingMap[headerHash]; !exists {
		return fmt.Errorf("updateAuditingStateDB failed headerHash %v missing!", headerHash)
	}
	n.auditingMap[headerHash] = auditdb
	return nil
}

// this function is used to get the auditingMap (need mutex)
func (n *Node) getAuditingStateDB(headerHash common.Hash) (*statedb.StateDB, error) {
	n.auditingMapMutex.Lock()
	defer n.auditingMapMutex.Unlock()
	auditdb, exists := n.auditingMap[headerHash]
	if !exists {
		return nil, fmt.Errorf("headerHash %v not found for auditing", headerHash)
	}
	return auditdb, nil
}

// this function is used to get the announcementMap (don't need mutex)
func (n *Node) getTrancheAnnouncement(headerHash common.Hash) (*types.TrancheAnnouncement, error) {

	trancheAnnouncement, exists := n.announcementMap[headerHash]
	if !exists {
		return nil, fmt.Errorf("trancheAnnouncement %v not found for auditing", headerHash)
	}
	return trancheAnnouncement, nil
}

func (n *Node) updateTrancheAnnouncement(headerHash common.Hash, trancheAnnouncement *types.TrancheAnnouncement) error {
	n.announcementMapMutex.Lock()
	defer n.announcementMapMutex.Unlock()
	n.announcementMap[headerHash] = trancheAnnouncement
	return nil
}

func (n *Node) checkTrancheAnnouncement(headerHash common.Hash) bool {
	n.announcementMapMutex.Lock()
	defer n.announcementMapMutex.Unlock()

	_, exists := n.announcementMap[headerHash]
	return exists
}

func (n *Node) updateKnownWorkReportMapping(announcement types.Announcement) {
	n.judgementWRMapMutex.Lock()
	defer n.judgementWRMapMutex.Unlock()
	for _, reportHash := range announcement.GetWorkReportHashes() {
		n.judgementWRMap[reportHash] = announcement.HeaderHash
	}
	return
}

func (n *Node) getJudgementBucket(headerHash common.Hash) (*types.JudgeBucket, error) {
	n.judgementMapMutex.Lock()
	defer n.judgementMapMutex.Unlock()
	judgementBucket, exists := n.judgementMap[headerHash]
	if !exists {
		return nil, fmt.Errorf("judgement_bucket %v not found for auditing", headerHash)
	}
	return judgementBucket, nil
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
		log.Trace(debugAudit, "sending waitingAnnouncements AGAIN", "n", n.String(), "a", a.Hash())
		select {
		case n.announcementsCh <- a:
			// sent successfully
		default:
			log.Warn(debugAudit, "cleanWaitingAJ: announcementsCh full, dropping announcement",
				"n", n.String(), "hash", a.Hash().String_short())
		}
	}

	for _, j := range waitingJudgements {
		log.Trace(debugAudit, "sending waitingJudgements AGAIN", "n", n.String(), "j", j.Hash())
		select {
		case n.judgementsCh <- j:
			// sent successfully
		default:
			log.Warn(debugAudit, "cleanWaitingAJ: judgementsCh full, dropping judgement",
				"n", n.String(), "hash", j.Hash().String_short())
		}
	}

	n.waitingAnnouncements = make([]types.Announcement, 0)
	n.waitingJudgements = make([]types.Judgement, 0)
}

func (n *Node) cleanUseless(header_hash common.Hash) {
	n.announcementMapMutex.Lock()
	n.judgementMapMutex.Lock()
	n.auditingMapMutex.Lock()
	defer n.announcementMapMutex.Unlock()
	defer n.judgementMapMutex.Unlock()
	defer n.auditingMapMutex.Unlock()
	if _, exists := n.announcementMap[header_hash]; exists {
		delete(n.announcementMap, header_hash)
	}
	if _, exists := n.judgementMap[header_hash]; exists {
		delete(n.judgementMap, header_hash)
	}
	if _, exists := n.auditingMap[header_hash]; exists {
		delete(n.auditingMap, header_hash)
	}
	return
}

func (n *Node) runAudit() {
	for {
		select {
		case announcement := <-n.announcementsCh:
			err := n.processAnnouncement(announcement)
			if err != nil {
				fmt.Printf("%s processAnnouncement: %v\n", n.String(), err)
			}
		case judgement := <-n.judgementsCh:
			err := n.processJudgement(judgement)
			if err != nil {
				fmt.Printf("%s processJudgement: %v\n", n.String(), err)
			}
		case audit_statedb := <-n.auditingCh:
			go func(audit_statedb *statedb.StateDB) {
				headerHash := audit_statedb.GetHeaderHash()

				log.Debug(debugAudit, "runAudit:start auditing block", "n", n.String(), "ts", audit_statedb.Block.TimeSlot(), "audit_statedb.headerHash", audit_statedb.HeaderHash, "headerHash", headerHash)
				err := n.addAuditingStateDB(audit_statedb)
				if err != nil {
					log.Error(debugAudit, "addAuditingStateDB", "err", err)
				}
				n.cleanWaitingAJ()
				log.Debug(debugAudit, "runAudit: cleanWaitingAJ done", "n", n.String())
				n.initAudit(headerHash)
				log.Debug(debugAudit, "runAudit: initAudit done", "n", n.String())
				err = n.Audit(headerHash)
				if err != nil {
					log.Trace(debugAudit, "Audit Failed", "err", err)
				} else {
					// if the block is audited, we can start grandpa
					log.Debug(debugAudit, "Audit Done", "n", n.String(), "headerHash", headerHash, "audit_statedb.timeslot", audit_statedb.GetTimeslot())

					newBlock := audit_statedb.Block.Copy()
					if newBlock.GetParentHeaderHash() == (genesisBlockHash) && Grandpa {
						n.StartGrandpa(newBlock.Copy())
					}
					time.Sleep(10 * time.Second) // remove it after audited
					n.cleanUseless(headerHash)
				}
			}(audit_statedb)
		}
	}
}

func (n *Node) Audit(headerHash common.Hash) error {
	// in normal situation, we will not have tranche1 unless we have networking problems. we can force it by setting requireTranche1 to true
	// TODO return as bool, err to differentiate between error & audit result
	paulseTicker := time.NewTicker(25 * time.Millisecond)
	tmp := uint32(1 << 31)
	auditing_statedb, err := n.getAuditingStateDB(headerHash)

	if err != nil {
		return err
	}
	//TODO: not sure what JCE to use here...
	currJCE := n.GetCurrJCE()
	n.jce_timestamp_mutex.Lock()
	start_point := n.jce_timestamp[currJCE]
	n.jce_timestamp_mutex.Unlock()
	tranche := auditing_statedb.GetTranche(start_point)
	if tranche == 0 {
		err := n.ProcessAudit(tranche, headerHash)
		if err != nil {
			log.Error(debugAudit, "ProcessAudit failed", "err", err)
		}
	} else {
		log.Debug(debugAudit, "Audit tranche > 0", "n", n.String(), "ts", auditing_statedb.Block.TimeSlot(), "tranche", tranche)
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
			refreshedJCE := auditing_statedb.GetTimeslot()
			n.jce_timestamp_mutex.Lock()
			start_point := n.jce_timestamp[refreshedJCE]
			n.jce_timestamp_mutex.Unlock()
			tranche := auditing_statedb.GetTranche(start_point)
			// log.Debug(debugAudit, "Audit tranche", "n", n.String(), "ts", auditing_statedb.Block.TimeSlot(), "tranche", tranche)
			// use tmp to check if it's a new tranche
			if tranche != tmp && tranche != 0 {

				// if it's the same tranche, check if the block is audited
				// if it's audited, break the loop
				isAudited, err := n.CheckBlockAudited(headerHash, tranche-1)
				if err != nil {
					return fmt.Errorf("CheckBlockAudited failed [slot:%d]:%v", auditing_statedb.GetSafrole().Timeslot, err)
				}
				if isAudited {
					// wait for everyone to finish auditing
					log.Trace(debugAudit, "Tranche audited block", "n", n.String(), "ts", auditing_statedb.Block.TimeSlot(), "tranche-1", tranche-1,
						"headerhash", auditing_statedb.Block.Header.Hash().String_short(),
						"author", auditing_statedb.Block.Header.AuthorIndex,
					)
					done = true
					break
				} else {
					log.Debug(debugAudit, "Tranche not audited block", "n", n.String(), "ts", auditing_statedb.Block.TimeSlot(), "tranche", tranche,
						"blockhash", auditing_statedb.Block.Hash())
				}

				tmp = tranche
				if tranche > 5 {
					//TODO: we should never get here under tiny case
					return fmt.Errorf("%s [T:%d] Audit still not complete after Tranche %v block %v \n", n.String(), auditing_statedb.Block.TimeSlot(), tranche, auditing_statedb.Block.Hash())
				}
				n.ProcessAudit(tranche, headerHash)
			}
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
	if err != nil {
		return err
	}
	log.Debug(debugAudit, "Tranche, start audit", "n", n.String(), auditing_statedb.Block.TimeSlot(), tranche)
	// normal behavior
	switch n.AuditNodeType {
	case "normal":
		reports, err = n.Announce(headerHash, tranche)
		if err != nil {
			return err
		}
		judges, err := n.Judge(headerHash, reports)
		if err != nil {
			return err
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
		if err != nil {
			return err
		}
		judges, err := n.Judge(headerHash, reports)
		if err != nil {
			return err
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
		if err != nil {
			return err
		}
		judges, err := n.Judge(headerHash, reports)
		if err != nil {
			return err
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
			return err
		} else {
			n.DistributeJudgements(judges, headerHash)
		}

	default:
		reports, err = n.Announce(headerHash, tranche)
		if err != nil {
			return err
		}
		log.Debug(debugAudit, "selected reports", "n", n.String(), "ts", auditing_statedb.Block.TimeSlot(), "len(reports)", len(reports))
		judges, err := n.Judge(headerHash, reports)
		if err != nil {
			return err
		} else {
			n.DistributeJudgements(judges, headerHash)
		}
	}
	if len(reports) == 0 {
		log.Debug(debugAudit, "Tranche, no audit reports", "n", n.String(), "ts", auditing_statedb.Block.TimeSlot(), "tranche", tranche)
	} else {
		log.Debug(debugAudit, "Tranche, audit reports", "n", n.String(), "ts", auditing_statedb.Block.TimeSlot(), "tranche", tranche)
		for _, w := range reports {
			log.Debug(debugAudit, "selected work report, from core", "n", n.String(), "ts", auditing_statedb.Block.TimeSlot(), "wph", w.WorkReport.Hash(), "c", w.WorkReport.CoreIndex)
		}
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
		for _, w := range a0 {
			log.Trace(debugAudit, "n", n.String(), "ts", auditing_statedb.GetTimeslot(), "wph", w.WorkReport.Hash())
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
			// fmt.Printf("%s [T:%d] has made announcement %v\n", n.String(), auditing_statedb.GetTimeslot(), announcement.Hash())
			log.Warn(debugAudit, "n", n.String(), "ts", auditing_statedb.GetTimeslot(), "has made announcement", "hash", announcement.Hash().String_short())
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
				Reports:    workReports,
				Signature:  announcement.Signature,
			}
			announcementWithProof.EvidenceTranche0 = Tranche0Evidence(s0)
			for _, w := range a0 {
				log.Trace(debugAudit, "broadcasting announcement", "n", n.String(), "ts", auditing_statedb.Block.TimeSlot(), "wph", w.WorkReport.Hash())
			}
			go n.broadcast(context.TODO(), announcementWithProof)
		}

		return a0, nil
	}

	banderSnatchSecret := bandersnatch.BanderSnatchSecret(n.GetBandersnatchSecret())
	prev_bucket, err := n.GetAnnounceBucketByTranche(tranche-1, headerHash)
	if err != nil {
		return nil, err
	}
	currJCE := n.GetCurrJCE()
	n.jce_timestamp_mutex.Lock()
	start_point := n.jce_timestamp[currJCE]
	n.jce_timestamp_mutex.Unlock()
	tranche = auditing_statedb.GetTranche(start_point)
	an, no_show_a, _, sn, err := auditing_statedb.Select_an(banderSnatchSecret, prev_bucket, judgment_bucket, tranche)
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
			Reports:    workReports,
			Signature:  announcement.Signature,
		}
		evidenceSn := make([]TrancheEvidence, len(sn))
		for i, sig := range sn {
			evidenceSn[i].Signature = types.BandersnatchVrfSignature(sig)

		}
		for i, wr := range an {
			no_show_an := no_show_a[wr.WorkReport.Hash()]
			if no_show_an != nil {
				for _, no_show := range no_show_an {
					Reports := make([]JAMSNPAuditAnnouncementReport, 0)
					for _, w := range no_show.Selected_WorkReport {
						Reports = append(Reports, JAMSNPAuditAnnouncementReport{
							CoreIndex:      w.Core,
							WorkReportHash: w.WorkReportHash,
						})
					}
					evidenceSn[i].NoShows = append(evidenceSn[i].NoShows, JAMSNPNoShow{
						ValidatorIndex: uint16(no_show.ValidatorIndex),
						Reports:        Reports,
						Signature:      no_show.Signature,
					})
				}
			}
		}
		announcementWithProof.EvidenceTrancheN = evidenceSn
		go n.broadcast(context.TODO(), announcementWithProof)

	}

	return an, nil

}

func (n *Node) HaveMadeAnnouncement(announcement types.Announcement, headerHash common.Hash) bool {
	n.announcementMapMutex.Lock()
	defer n.announcementMapMutex.Unlock()

	tranche_announcement, err := n.getTrancheAnnouncement(headerHash)
	if err != nil {
		return false
	}
	return tranche_announcement.HaveMadeAnnouncement(announcement)
}

func (n *Node) GetAnnounceBucketByTranche(tranche uint32, headerHash common.Hash) (*types.AnnounceBucket, error) {
	n.announcementMapMutex.Lock()
	defer n.announcementMapMutex.Unlock()
	tranche_announcement, err := n.getTrancheAnnouncement(headerHash)
	if err != nil {
		return nil, err
	}
	bucket, exists := tranche_announcement.AnnouncementBucket[tranche]
	if !exists {
		return nil, fmt.Errorf("tranche %v not found for auditing", tranche)
	}
	return bucket, nil
}

func (n *Node) Judge(headerHash common.Hash, workReports []types.WorkReportSelection) (judgements []types.Judgement, err error) {

	judgement_bucket, err := n.getJudgementBucket(headerHash)
	if len(workReports) == 0 {
		return nil, nil
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	errCh := make(chan error, len(workReports))
	for _, w := range workReports {
		wg.Add(1)
		go func(w types.WorkReportSelection) {
			defer wg.Done()
			hasmade := judgement_bucket.HaveMadeJudgementByValidator(w.WorkReport.Hash(), uint16(n.GetCurrValidatorIndex()))
			var j types.Judgement
			var err error
			if !hasmade {
				j, err = n.auditWorkReport(w.WorkReport, headerHash)
			} else {
				j, err = judgement_bucket.GetJudgementByValidator(w.WorkReport.Hash(), uint16(n.GetCurrValidatorIndex()))
			}
			if err != nil {
				errCh <- err
				return
			}
			mu.Lock()
			judgements = append(judgements, j)
			mu.Unlock()
		}(w)
	}

	wg.Wait()
	close(errCh)

	if len(errCh) > 0 {
		return nil, <-errCh
	}
	return
}

// auditWorkReport reconstructs the bundle from C assurers, verifies the bundle, and executes the work package
func (n *Node) auditWorkReport(workReport types.WorkReport, headerHash common.Hash) (judgement types.Judgement, err error) {
	judgement = types.Judgement{}

	judgment_bucket, err := n.getJudgementBucket(headerHash)
	if err != nil {
		return
	}
	judgment_bucket.RLock()
	for _, j := range judgment_bucket.Judgements[workReport.Hash()] {
		if j.Validator == n.id {
			return j, nil
		}
	}
	judgment_bucket.RUnlock()

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
	spec := workReport.AvailabilitySpec
	workPackageHash := spec.WorkPackageHash
	coreIndex := workReport.CoreIndex
	// now call C138 to get bundle_shard from C assurers, do ec reconstruction for b
	// IMPORTANT: within reconstructPackageBundleSegments is a call to VerifyBundle
	workPackageBundle, err := n.reconstructPackageBundleSegments(spec.ErasureRoot, spec.BundleLength, workReport.SegmentRootLookup, coreIndex)
	if err != nil {
		log.Error(debugAudit, "FetchWorkPackageBundle:reconstructPackageBundleSegments", "err", err)
		return
	}
	if workPackageBundle.PackageHash() != workPackageHash {
		log.Error(debugAudit, "auditWorkReport:FetchWorkPackageBundle package mismatch")
		return
	}

	wr, _, pvmElapsed, err := n.executeWorkPackageBundle(workReport.CoreIndex, workPackageBundle, workReport.SegmentRootLookup, false)
	if err != nil {
		return
	}

	select {
	case n.workReportsCh <- workReport:
		// successfully sent
	default:
		log.Warn(debugAudit, "auditWorkReport: workReportsCh full, dropping workReport", "workReport", workReport.Hash())
	}

	auditPass := false
	if spec.ErasureRoot == wr.AvailabilitySpec.ErasureRoot {
		auditPass = true
		log.Debug(debugAudit, "auditWorkReport:executeWorkPackageBundle PASS", "n", n.String(), "wph", workPackageBundle.WorkPackage.Hash(), "pvmElapsed", pvmElapsed)
	} else {
		log.Warn(debugAudit, "auditWorkReport:executeWorkPackageBundle FAIL", "n", n.String(), "wph", workPackageBundle.WorkPackage.Hash(), "pvmElapsed", pvmElapsed)
	}

	judgement, err = n.MakeJudgement(workReport, auditPass)

	return
}

func (n *Node) DistributeJudgements(judges []types.Judgement, headerHash common.Hash) {
	for _, j := range judges {
		log.Trace(debugAudit, "distributing judgement", "n", n.String(), "wph", j.WorkReportHash)

		go n.broadcast(context.TODO(), j)

		select {
		case n.judgementsCh <- j:
			// success
		default:
			log.Warn(debugAudit, "DistributeJudgements: judgementsCh full, dropping judgement",
				"n", n.String(),
				"workReportHash", j.WorkReportHash.String_short(),
				"validator", j.Validator)
		}
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
	isBlockAudited := auditing_statedb.IsBlockAudited(announcementBucket, judgment_bucket)

	for _, w := range auditing_statedb.AvailableWorkReport {
		log.Trace(debugAudit, "CheckBlockAudited", "n", n.String(), "ts", auditing_statedb.Block.TimeSlot(), "len announcementBucket", len(announcementBucket.Announcements[w.Hash()]),
			"len judgment_bucket", len(judgment_bucket.Judgements[w.Hash()]))
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
	var block *types.Block
	for _, awr := range auditing_statedb.AvailableWorkReport {
		old_eg, err := n.TraceOldGuarantee(headerHash, awr.GetWorkPackageHash())
		if err != nil {
			return false, err
		}
		block, err_dispute = auditing_statedb.AppendDisputes(judgement_bucket, awr.Hash(), old_eg)
	}
	if err_dispute == nil {
		block.Extrinsic.Disputes.FormatDispute()
		// bloock done here
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
	n.announcementMapMutex.Lock()
	trancheAnnouncement, err := n.getTrancheAnnouncement(headerHash)
	n.announcementMapMutex.Unlock()
	if err != nil {
		fmt.Printf("%s [audit:processAnnouncement] trancheAnnouncement not found %v \n", n.String(), headerHash)
		return err
	}
	err = trancheAnnouncement.PutAnnouncement(announcement)
	if err != nil {
		fmt.Printf("%s [audit:processAnnouncement] trancheAnnouncement.PutAnnouncement failed %v \n", n.String(), headerHash)
		return err
	}
	n.updateTrancheAnnouncement(headerHash, trancheAnnouncement)
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
