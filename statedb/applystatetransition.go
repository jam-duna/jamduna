package statedb

import (
	"context"
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// given previous safrole, applt state transition using block
// σ'≡Υ(σ,B)
func ApplyStateTransitionFromBlock(oldState *StateDB, ctx context.Context, blk *types.Block) (s *StateDB, err error) {

	s = oldState.Copy()
	if s.StateRoot != blk.Header.ParentStateRoot {
		return s, fmt.Errorf("ParentStateRoot does not match")
	}
	old_timeslot := s.GetSafrole().Timeslot
	s.JamState = oldState.JamState.Copy()
	s.Block = blk
	s.ParentHeaderHash = blk.Header.ParentHeaderHash
	s.HeaderHash = blk.Header.Hash()
	isValid, _, _, headerErr := s.VerifyBlockHeader(blk)
	if !isValid || headerErr != nil {
		// panic("MK validation check!! Block header is not valid")
		return s, fmt.Errorf("Block header is not valid")
	}
	if s.Id == blk.Header.AuthorIndex {
		s.Authoring = log.GeneralAuthoring
	} else {
		s.Authoring = log.GeneralValidating
	}
	log.Trace(s.Authoring, "ApplyStateTransitionFromBlock", "n", s.Id, "p", s.ParentHeaderHash, "headerhash", s.HeaderHash, "stateroot", s.StateRoot)
	targetJCE := blk.TimeSlot()
	// 17+18 -- takes the PREVIOUS accumulationRoot which summarizes C a set of (service, result) pairs and
	// 19-22 - Safrole last
	ticketExts := blk.Tickets()
	sf_header := blk.GetHeader()
	epochMark := blk.EpochMark()

	if epochMark != nil {
		// s.queuedTickets = make(map[common.Hash]types.Ticket)
		s.GetJamState().ResetTallyStatistics()
	}
	// 0.6.2 4.7 - Recent History Dagga (β†) [No other state related]
	s.ApplyStateRecentHistoryDagga(blk.Header.ParentStateRoot)
	// dispute should go here
	disputes := blk.Disputes()
	if len(disputes.Verdict) != 0 {
		err = s.ApplyStateTransitionDispute(disputes)
		if err != nil {
			return s, err
		}
	}
	// TODO - 4.12 - Dispute
	// 0.6.2 Safrole 4.5,4.8,4.9,4.10,4.11 [post dispute state , pre designed validators iota]
	sf := s.GetSafrole()
	sf.OffenderState = s.GetJamState().DisputesState.Psi_o
	s2, err := sf.ApplyStateTransitionTickets(ticketExts, targetJCE, sf_header) // Entropy computed!
	if err != nil {
		log.Error(module, "ApplyStateTransitionTickets", "err", jamerrors.GetErrorName(err))
		return s, err
	}
	//the epochMark validators should be in gamma k'
	if epochMark != nil {
		// (6.27)Bandersnatch validator keys (kb) beginning in the next epoch.
		bandersnatch_keys_map := make(map[common.Hash]bool)
		for _, keys := range epochMark.Validators {
			bandersnatch_keys_map[keys.BandersnatchKey] = false
		}
		for _, validator := range s2.NextValidators {
			if _, ok := bandersnatch_keys_map[validator.Bandersnatch.Hash()]; ok {
				bandersnatch_keys_map[validator.Bandersnatch.Hash()] = true
			}
		}
		for _, ok := range bandersnatch_keys_map {
			if !ok {
				return s, fmt.Errorf("EpochMark validators are not in NextValidators")
			}
		}
	}
	safrole_debug := false
	if safrole_debug {
		err = VerifySafroleSTF(sf, &s2, blk)
		if err != nil {
			panic(fmt.Sprintf("VerifySafroleSTF %v\n", err))
		}
	}

	s.JamState.SafroleState = &s2
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "tickets", uint32(len(ticketExts)))
	// use post entropy state rotate the guarantors
	s.RotateGuarantors()

	// preparing for the rho transition

	assurances := blk.Assurances()
	guarantees := blk.Guarantees()
	// 4.13,4.14,4.15 - Rho [disputes, assurances, guarantees] [kappa',lamda',tau', beta dagga, prestate service, prestate accumulate related state]
	// 4.16 available work report also updated
	num_reports, num_assurances, err := s.ApplyStateTransitionRho(assurances, guarantees, targetJCE)
	if err != nil {
		return s, err
	}
	for validatorIndex, nassurances := range num_assurances {
		s.JamState.tallyStatistics(uint32(validatorIndex), "assurances", uint32(nassurances))
	}
	for validatorIndex, nreports := range num_reports {
		s.JamState.tallyStatistics(uint32(validatorIndex), "reports", uint32(nreports))
	}
	// 4.17 Accmuulation [need available work report, ϑ, ξ, δ, χ, ι, φ]
	// 12.20 gas counting
	var gas uint64
	var gas_counting uint64
	gas = types.AccumulateGasAllocation_GT
	gas_counting = types.AccumulationGasAllocation * types.TotalCores
	// get the partial state
	o := s.JamState.newPartialState()
	kai_g := o.PrivilegedState.Kai_g
	for _, g := range kai_g {
		gas_counting += uint64(g)
	}
	if gas < gas_counting {
		gas = gas_counting
	}
	var f map[uint32]uint32
	var b []BeefyCommitment
	accumulate_input_wr := s.AvailableWorkReport
	accumulate_input_wr = s.AccumulatableSequence(accumulate_input_wr)

	// this will hold the gasUsed + numWorkreports -- ServiceStatistics
	accumulateStats := make(map[uint32]*accumulateStatistics)

	n, t, b, U := s.OuterAccumulate(gas, accumulate_input_wr, o, f)
	// (χ′, δ†, ι′, φ′)
	// 12.24 transfer δ‡
	tau := s.GetTimeslot() // τ′
	transferStats, err := s.ProcessDeferredTransfers(o, tau, t)
	if err != nil {
		return s, err
	}
	// make sure all service accounts can be written
	for _, sa := range o.D {
		sa.Mutable = true
		sa.Dirty = true
	}
	// writeAccount and initializes s.stateUpdate
	s.stateUpdate = s.ApplyXContext(o)
	// finalize stateUpdates
	s.computeStateUpdates(blk, targetJCE) // review targetJCE input

	// accumulate statistics
	accumulated_workreports := accumulate_input_wr[:n]
	for _, report := range accumulated_workreports {
		for _, result := range report.Results {
			service := result.ServiceID
			stats, ok := accumulateStats[service]
			if !ok {
				stats = &accumulateStatistics{}
			}
			stats.numWorkReports++
			if stats.numWorkReports > 0 {
				accumulateStats[service] = stats
			} else {
				continue
			}
		}
	}

	for _, gasusage := range U {
		service := gasusage.Service
		stats, ok := accumulateStats[service]
		if !ok {
			stats = &accumulateStatistics{}
		}
		stats.gasUsed += uint(gasusage.Gas)
		accumulateStats[service] = stats
	}

	//after accumulation, we need to update the accumulate state
	s.ApplyStateTransitionAccumulation(accumulate_input_wr, n, old_timeslot)
	// 0.6.2 4.18 - Preimages [ δ‡, τ′]
	preimages := blk.PreimageLookups()
	num_preimage, num_octets, err := s.ApplyStateTransitionPreimages(preimages, targetJCE)
	if err != nil {
		return s, err
	}

	// tally validator statistics
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "preimages", num_preimage)
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "octets", num_octets)

	// tally core statistics + service statistics -- the newly available work reports and incoming work reports ... along with assurances + preimages
	//
	s.JamState.tallyCoreStatistics(guarantees, s.AvailableWorkReport, assurances)
	s.JamState.tallyServiceStatistics(guarantees, preimages, accumulateStats, transferStats)

	// Update Authorization Pool alpha
	// 4.19 α'[need φ', so after accumulation]
	err = s.ApplyStateTransitionAuthorizations()
	if err != nil {
		return s, err
	}
	// n.r = M_B( [ s \ E_4(s) ++ E(h) | (s,h) in C] , H_K)
	sort.Slice(b, func(i, j int) bool {
		return b[i].Service < b[j].Service
	})
	var leaves [][]byte
	for _, sa := range b {
		// put (s,h) of C  into leaves
		leafBytes := append(common.Uint32ToBytes(sa.Service), sa.Commitment.Bytes()...)
		empty := common.Hash{}
		if sa.Commitment == empty {
			// should not have gotten here!
			log.Warn(log.GeneralAuthoring, "BEEFY-C", "commitment", sa.Commitment)
		} else {
			leaves = append(leaves, leafBytes)
			log.Debug(s.Authoring, "BEEFY-C", "s", fmt.Sprintf("%d", sa.Service), "h", sa.Commitment, "encoded", fmt.Sprintf("%x", leafBytes))
		}
	}
	tree := trie.NewWellBalancedTree(leaves, types.Keccak)
	accumulationRoot := common.Hash(tree.Root())
	if len(leaves) > 0 {
		log.Debug(log.GeneralAuthoring, "BEEFY accumulation root", "r", accumulationRoot)
	}
	// 4.7 - Recent History [No other state related, but need to do it after rho, AFTER accumulation]
	s.ApplyStateRecentHistory(blk, &(accumulationRoot))

	// 4.20 - compute pi
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "blocks", 1)
	s.StateRoot = s.UpdateTrieState()

	return s, nil
}

func (s *StateDB) computeStateUpdates(blk *types.Block, targetJCE uint32) {
	// setup workpackage updates (guaranteed, queued, accumulated)
	log.Trace(module, "computeStateUpdates", "len(e_p)", len(blk.Extrinsic.Preimages), "len(e_g)", len(blk.Extrinsic.Guarantees), "len(ah)", len(s.JamState.AccumulationHistory[types.EpochLength-1].WorkPackageHash))
	for _, g := range blk.Extrinsic.Guarantees {
		wph := g.Report.AvailabilitySpec.WorkPackageHash
		log.Trace(s.Authoring, "computeStateUpdates-G", "hash", wph)
		s.stateUpdate.WorkPackageUpdates[wph] = &types.SubWorkPackageResult{
			WorkPackageHash: wph,
			HeaderHash:      s.HeaderHash,
			Slot:            s.GetTimeslot(),
			Status:          "guaranteed",
		}
	}
	/*
		_, currPhase := s.JamState.SafroleState.EpochAndPhase(targetJCE) // REVIEW
		for _, q := range s.JamState.AccumulationQueue[currPhase] {
			for _, wph := range q.WorkPackageHash {
				log.Trace(s.Authoring, "ApplyStateTransitionFromBlock: SubWorkPackageResult queued", "s", 0, "hash", wph)
				s.stateUpdate.WorkPackageUpdates[wph] = &types.SubWorkPackageResult{
					WorkPackageHash: wph,
					HeaderHash:      s.HeaderHash,
					Slot:            s.GetTimeslot(),
					Status:          "queued",
				}
			}
		}
	*/
	h := s.JamState.AccumulationHistory[types.EpochLength-1]
	for _, wph := range h.WorkPackageHash {
		log.Trace(s.Authoring, "computeStateUpdates-A", "hash", wph)
		s.stateUpdate.WorkPackageUpdates[wph] = &types.SubWorkPackageResult{
			WorkPackageHash: wph,
			HeaderHash:      s.HeaderHash,
			Slot:            s.GetTimeslot(),
			Status:          "accumulated",
		}
	}
	for _, p := range blk.Extrinsic.Preimages {
		serviceID := p.Requester
		hash := common.Blake2Hash(p.Blob)
		sp, ok := s.stateUpdate.ServiceUpdates[serviceID]
		if !ok {
			sp = types.NewServiceUpdate(serviceID)
			s.stateUpdate.ServiceUpdates[serviceID] = sp
		}
		log.Trace(s.Authoring, "computeStateUpdates-P", "s", serviceID, "hash", hash, "l", len(p.Blob))
		sp.ServicePreimage[hash] = &types.SubServicePreimageResult{
			HeaderHash: s.HeaderHash,
			Slot:       s.GetTimeslot(),
			Hash:       hash,
			ServiceID:  serviceID,
			Preimage:   common.Bytes2Hex(p.Blob),
		}
	}
}

func (s *StateDB) ApplyStateTransitionDispute(disputes types.Dispute) (err error) {
	// (25) / (111) We clear any work-reports which we judged as uncertain or invalid from their core
	d := s.GetJamState()
	// checking the Ho
	header := s.Block.GetHeader()
	if len(disputes.Verdict) != 0 {
		offendermark := header.OffendersMark
		if offendermark == nil {
			return fmt.Errorf("OffendersMark is nil")
		}
		// key need to be either in culprits or faults
		// make a map of the key
		offendermarkMap := make(map[types.Ed25519Key]bool)
		for _, offendkey := range offendermark {
			offendermarkMap[offendkey] = false
		}
		for _, culprit := range disputes.Culprit {
			if _, ok := offendermarkMap[culprit.Key]; ok {
				offendermarkMap[culprit.Key] = true
			}
		}
		for _, fault := range disputes.Fault {
			if _, ok := offendermarkMap[fault.Key]; ok {
				offendermarkMap[fault.Key] = true
			}
		}
		for _, isOffender := range offendermarkMap {
			if !isOffender {
				return fmt.Errorf("OffendersMark is not in Culprit or Fault")
			}
		}
	}
	//apply the dispute
	result, err := d.IsValidateDispute(&disputes)
	if err != nil {
		return
	}
	//state changing here
	//cores reading the old jam state
	//ρ†
	d.ProcessDispute(result, disputes.Culprit, disputes.Fault)
	return nil
}

// Process Rho - Eq 25/26/27 using disputes, assurances, guarantees in that order
func (s *StateDB) ApplyStateTransitionRho(assurances []types.Assurance, guarantees []types.Guarantee, targetJCE uint32) (num_reports map[uint16]uint16, num_assurances map[uint16]uint16, err error) {
	d := s.GetJamState()
	// original validate assurances logic (prior to guarantees) -- we cannot do signature checking here ... otherwise it would trigger bad sig
	// for fuzzing to work, we cannot check signature until everything has been properly considered
	// assuranceErr := s.ValidateAssurancesWithSig(assurances)
	// if assuranceErr != nil {
	// 	return 0, 0, err
	// }

	err = s.ValidateAssurancesTransition(assurances)
	if err != nil {
		return
	}

	// Assurances: get the bitstring from the availability
	// core's data is now available
	//ρ††
	num_assurances, availableWorkReport := d.ProcessAssurances(assurances, targetJCE)
	_ = availableWorkReport                     // availableWorkReport is the work report that is available for the core, will be used in the audit section
	s.AvailableWorkReport = availableWorkReport // every block has new available work report

	for i, rho := range s.JamState.AvailabilityAssignments {
		if rho == nil {
			log.Trace(debugA, "ApplyStateTransitionRho before Verify_Guarantees", "core", i, "WorkPackage Hash", rho)
		} else {
			log.Trace(debugA, "ApplyStateTransitionRho before Verify_Guarantees", "core", i, "WorkPackage Hash", rho.WorkReport.GetWorkPackageHash())
		}
	}

	// Sort the assurances by validator index
	// sortingErr := CheckSortingEAs(assurances)
	// if sortingErr != nil {
	// 	return 0, 0, sortingErr
	// }

	// Verify each assurance's signature
	// sigErr := s.ValidateAssurancesSig(assurances)
	// if sigErr != nil {
	// 	return 0, 0, sigErr
	// }

	// Guarantees
	err = s.Verify_Guarantees()
	if err != nil {
		return
	}

	num_reports = d.ProcessGuarantees(guarantees)
	for i, rho := range s.JamState.AvailabilityAssignments {
		if rho == nil {
			log.Trace(debugA, "ApplyStateTransitionRho after ProcessGuarantees", "core", i, "WorkPackage Hash", rho)
		} else {
			log.Trace(debugA, "ApplyStateTransitionRhoafter ProcessGuarantees", "core", i, "WorkPackage Hash", rho.WorkReport.GetWorkPackageHash())
		}
	}
	return num_reports, num_assurances, nil
}
