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

func ApplyStateTransitionTickets(oldState *StateDB, ctx context.Context, blk *types.Block, validated_tickets map[common.Hash]common.Hash) (safroleState *SafroleState, err error) {
	s := oldState.Copy()
	if s.StateRoot != blk.Header.ParentStateRoot {
		//fmt.Printf("Apply Block %v\n", blk.Header.Hash())
		return safroleState, fmt.Errorf("ParentStateRoot does not match")
	}
	recentBlocks := s.JamState.RecentBlocks.B_H
	if len(recentBlocks) > 0 && blk.Header.ParentHeaderHash != recentBlocks[len(recentBlocks)-1].HeaderHash {
		log.Error(log.SDB, "ApplyStateTransitionFromBlock", "ParentHeaderHash", blk.Header.ParentHeaderHash, "recentBlocks", recentBlocks[len(recentBlocks)-1].HeaderHash)
		return safroleState, fmt.Errorf("ParentHeaderHash does not match recent block")
	}
	s.JamState = oldState.JamState.Copy()
	s.Block = blk
	s.ParentHeaderHash = blk.Header.ParentHeaderHash
	s.HeaderHash = blk.Header.Hash()
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
	select {
	case <-ctx.Done():
		return safroleState, fmt.Errorf("ApplyStateRecentHistoryDagga canceled")
	default:
	}

	// dispute should go here
	disputes := blk.Disputes()
	if len(disputes.Verdict) != 0 {
		err = s.ApplyStateTransitionDispute(disputes)
		if err != nil {
			return safroleState, err
		}
	}
	// TODO - 4.12 - Dispute
	// 0.6.2 Safrole 4.5,4.8,4.9,4.10,4.11 [post dispute state , pre designed validators iota]
	sf := s.GetSafrole()
	// Shawn to check: should it be sf0 here?
	sf.OffenderState = s.GetJamState().DisputesState.Psi_o
	ss, err := sf.ApplyStateTransitionTickets(ctx, ticketExts, targetJCE, sf_header, validated_tickets) // Entropy computed!
	if err != nil {
		log.Error(log.SDB, "ApplyStateTransitionTickets", "err", jamerrors.GetErrorName(err))
		return safroleState, err
	}
	return &ss, nil
}

// given previous safrole, applt state transition using block
// σ'≡Υ(σ,B)
func ApplyStateTransitionFromBlock(oldState *StateDB, ctx context.Context, blk *types.Block, validated_tickets map[common.Hash]common.Hash, pvmBackend string) (s *StateDB, err error) {
	s = oldState.Copy()
	if s.StateRoot != blk.Header.ParentStateRoot {
		//fmt.Printf("Apply Block %v\n", blk.Header.Hash())
		return s, fmt.Errorf("ParentStateRoot does not match")
	}
	recentBlocks := s.JamState.RecentBlocks.B_H
	if len(recentBlocks) > 0 && blk.Header.ParentHeaderHash != recentBlocks[len(recentBlocks)-1].HeaderHash {
		log.Error(log.SDB, "ApplyStateTransitionFromBlock", "ParentHeaderHash", blk.Header.ParentHeaderHash, "recentBlocks", recentBlocks[len(recentBlocks)-1].HeaderHash)
		return s, fmt.Errorf("ParentHeaderHash does not match recent block")
	}
	old_timeslot := s.GetSafrole().Timeslot
	s.JamState = oldState.JamState.Copy()
	s.Block = blk
	s.ParentHeaderHash = blk.Header.ParentHeaderHash
	s.HeaderHash = blk.Header.Hash()
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
	select {
	case <-ctx.Done():
		return s, fmt.Errorf("ApplyStateRecentHistoryDagga canceled")
	default:
	}

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
	// Shawn to check: should it be sf0 here?
	sf.OffenderState = s.GetJamState().DisputesState.Psi_o
	s2, err := sf.ApplyStateTransitionTickets(ctx, ticketExts, targetJCE, sf_header, validated_tickets) // Entropy computed!
	if err != nil {
		log.Error(log.SDB, "ApplyStateTransitionTickets", "err", jamerrors.GetErrorName(err))
		return s, err
	}
	//fmt.Printf("Safrole state transition done: %v | validated_tickets: %v\n", s2, validated_tickets)
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
	isValid, _, _, headerErr := s.VerifyBlockHeader(blk, &s2)
	if !isValid || headerErr != nil {
		return s, fmt.Errorf("block header is not valid err=%v", headerErr)
	}
	safrole_debug := false
	if safrole_debug {
		err = VerifySafroleSTF(sf, &s2, blk)
		if err != nil {
			return s, fmt.Errorf("VerifySafroleSTF %v", err)
		}
	}

	s.JamState.SafroleState = &s2
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "tickets", uint32(len(ticketExts)))
	// use post entropy state rotate the guarantors
	s.RotateGuarantors()

	// preparing for the rho transition

	assurances := blk.Assurances()
	assurances, err = s.GetValidAssurances(assurances, blk.Header.ParentHeaderHash, false)
	if err != nil {
		return s, err
	}
	guarantees := blk.Guarantees()
	log.Trace(log.A, "ApplyStateTransitionFromBlock", "len(assurances)", len(assurances))
	// 4.13,4.14,4.15 - Rho [disputes, assurances, guarantees] [kappa',lamda',tau', beta dagga, prestate service, prestate accumulate related state]
	// 4.16 available work report also updated
	num_reports, num_assurances, err := s.ApplyStateTransitionRho(ctx, assurances, guarantees, targetJCE)
	if err != nil {
		return s, err
	}
	for validatorIndex, nassurances := range num_assurances {
		s.JamState.tallyStatistics(uint32(validatorIndex), "assurances", uint32(nassurances))
	}
	for ed25519Key, _ := range num_reports {
		validatorIndex := s.JamState.SafroleState.GetCurrValidatorIndex(ed25519Key)
		s.JamState.tallyStatistics(uint32(validatorIndex), "reports", 1)
		// fmt.Printf("Validator %d: %d reports\n", validatorIndex, nreports)
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
	var b []types.AccumulationOutput
	accumulate_input_wr := s.AvailableWorkReport
	accumulate_input_wr = s.AccumulatableSequence(accumulate_input_wr)

	// this will hold the gasUsed + numWorkreports -- ServiceStatistics
	accumulateStats := make(map[uint32]*accumulateStatistics)

	n, t, b, U := s.OuterAccumulate(gas, accumulate_input_wr, o, f, pvmBackend) // outer accumulate

	// (χ′, δ†, ι′, φ′)
	// 12.24 transfer δ‡
	tau := s.GetTimeslot() // τ′
	transferStats, err := s.ProcessDeferredTransfers(o, tau, t, pvmBackend)
	if err != nil {
		return s, err
	}
	// make sure all service accounts can be written
	for _, sa := range o.D {
		sa.ALLOW_MUTABLE()
		sa.Dirty = true
	}

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

	// NOTE: we swapped the odring of the (ApplyXContext and computeStateUpdates) vs accumulate statistics in order to support potential idea of GP 0.6.7 (12.31) - updating a_r
	// writeAccount and initializes s.stateUpdate
	s.stateUpdate = s.ApplyXContext(o)
	// finalize stateUpdates
	s.computeStateUpdates(blk) // review targetJCE input
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
	err = s.JamState.tallyCoreStatistics(guarantees, s.AvailableWorkReport, assurances)
	if err != nil {
		return s, err
	}

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
		leafBytes := append(common.Uint32ToBytes(sa.Service), sa.Output.Bytes()...)
		empty := common.Hash{}
		if sa.Output == empty {
			// should not have gotten here!
			log.Warn(log.GeneralAuthoring, "BEEFY-C", "output", sa.Output)
		} else {
			leaves = append(leaves, leafBytes)
			log.Info(debugB, "BEEFY-C", "s", fmt.Sprintf("%d", sa.Service), "h", sa.Output, "encoded", fmt.Sprintf("%x", leafBytes))
		}
	}

	select {
	case <-ctx.Done():
		return s, fmt.Errorf("ApplyStateRecentHistoryDagga canceled")
	default:
	}
	tree := trie.NewWellBalancedTree(leaves, types.Keccak)
	accumulationRoot := common.Hash(tree.Root())
	if len(leaves) > 0 {
		log.Debug(debugB, "BEEFY accumulation root", "r", accumulationRoot)
	}
	// 4.7 - Recent History [No other state related, but need to do it after rho, AFTER accumulation]
	s.ApplyStateRecentHistory(blk, &(accumulationRoot), b)
	// 4.20 - compute pi
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "blocks", 1)
	s.StateRoot = s.UpdateTrieState()
	//duration := time.Since(timer)
	return s, nil
}

func (s *StateDB) computeStateUpdates(blk *types.Block) {
	// setup workpackage updates (guaranteed, queued, accumulated)
	log.Trace(log.SDB, "computeStateUpdates", "len(e_p)", len(blk.Extrinsic.Preimages), "len(e_g)", len(blk.Extrinsic.Guarantees), "len(ah)", len(s.JamState.AccumulationHistory[types.EpochLength-1].WorkPackageHash))
	for _, g := range blk.Extrinsic.Guarantees {
		wph := g.Report.AvailabilitySpec.WorkPackageHash
		log.Trace(log.SDB, "computeStateUpdates-GUARANTEE", "hash", wph, g.Report.String())
		s.stateUpdate.WorkPackageUpdates[wph] = &types.SubWorkPackageResult{
			WorkPackageHash: wph,
			HeaderHash:      s.HeaderHash,
			Slot:            s.GetTimeslot(),
			Status:          "guaranteed",
		}
	}

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
		sp.ServicePreimage[hash.Hex()] = &types.SubServicePreimageResult{
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
func (s *StateDB) ApplyStateTransitionRho(ctx context.Context, assurances []types.Assurance, guarantees []types.Guarantee, targetJCE uint32) (num_reports map[types.Ed25519Key]uint16, num_assurances map[uint16]uint16, err error) {
	d := s.GetJamState()
	assuranceErr := s.ValidateAssurances(ctx, assurances, s.Block.Header.ParentHeaderHash, false)
	if assuranceErr != nil {
		log.Error(log.SDB, "ApplyStateTransitionRho", "assuranceErr", assuranceErr)
		return nil, nil, assuranceErr
	}

	// Assurances: get the bitstring from the availability
	// core's data is now available
	//ρ††
	availableWorkReport, num_assurances := d.ComputeAvailabilityAssignments(assurances, targetJCE)
	//_ = availableWorkReport                     // availableWorkReport is the work report that is available for the core, will be used in the audit section
	s.AvailableWorkReport = availableWorkReport // every block has new available work report
	log.Trace(log.A, "ApplyStateTransitionRho", "len(s.AvailableWorkReport)", len(s.AvailableWorkReport))

	// Guarantees checks
	for _, g := range guarantees {
		if err := s.VerifyGuaranteeBasic(g, targetJCE); err != nil {
			return nil, nil, err
		}
	}

	// ensure global sort order  (sorted in makeblock)
	if err := CheckSortedGuarantees(guarantees); err != nil {
		return nil, nil, err
	}

	// length constraint (makeblock ensure unique wps -- CHECK)
	if err := s.checkLength(); err != nil {
		return nil, nil, err

	}

	// inter-dependency checks among guarantees
	for _, g := range guarantees {
		if err := s.checkRecentWorkPackage(g, guarantees); err != nil {
			return nil, nil, err
		}
		if err := s.checkPrereq(g, guarantees); err != nil {
			return nil, nil, err
		}
	}

	num_reports, err = d.ProcessGuarantees(ctx, guarantees, s.PreviousGuarantorAssignments)
	if err != nil {
		log.Error(log.SDB, "ApplyStateTransitionRho", "GuaranteeErr", err)
		return nil, nil, err
	}
	return num_reports, num_assurances, nil
}
