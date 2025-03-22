package statedb

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// make block generate block prior to state execution
func (s *StateDB) MakeBlock(credential types.ValidatorSecret, targetJCE uint32, ticketID common.Hash, extrinsic_pool *types.ExtrinsicPool) (bl *types.Block, err error) {
	sf := s.GetSafrole()
	isNewEpoch := sf.IsNewEpoch(targetJCE)
	needWinningMarker := sf.IseWinningMarkerNeeded(targetJCE)
	stateRoot := s.GetStateRoot()
	s.JamState.CheckInvalidCoreIndex()
	s.RecoverJamState(stateRoot)
	s.JamState.CheckInvalidCoreIndex()

	b := types.NewBlock()
	h := types.NewBlockHeader()
	extrinsicData := types.NewExtrinsic()
	h.ParentHeaderHash = s.HeaderHash
	h.ParentStateRoot = stateRoot
	h.Slot = targetJCE
	b.Header = *h
	// eq 71
	if isNewEpoch {
		epochMarker := sf.GenerateEpochMarker()
		//a tuple of the epoch randomness and a sequence of Bandersnatch keys defining the Bandersnatch valida- tor keys (kb) beginning in the next epoch
		h.EpochMark = epochMarker
	}
	// Extrinsic Data has 5 different Extrinsics
	// E_P - Preimages:  aggregate queuedPreimageLookups into extrinsicData.Preimages
	extrinsicData.Preimages = make([]types.Preimages, 0)

	// Make sure this Preimages is ready to be included..
	queued_preimage := extrinsic_pool.GetPreimageFromPool()
	log.Debug(debugP, "MakeBlock: Queued Preimages", "len", len(queued_preimage), "slot", targetJCE)
	for _, preimageLookup := range queued_preimage {
		_, err := s.ValidateLookup(preimageLookup)
		if err == nil {
			pl, err := preimageLookup.DeepCopy()
			if err != nil {
				continue
			}
			extrinsicData.Preimages = append(extrinsicData.Preimages, pl)
			extrinsic_pool.RemoveOldPreimages([]types.Preimages{*preimageLookup}, targetJCE)
		} else {
			log.Warn(debugP, "ValidateLookup", "err", err)
			extrinsic_pool.RemoveOldPreimages([]types.Preimages{*preimageLookup}, targetJCE)
			continue
		}
	}

	// 156: These pairs must be ordered and without duplicates
	for i := 0; i < len(extrinsicData.Preimages); i++ {
		for j := 0; j < len(extrinsicData.Preimages)-1; j++ {
			if extrinsicData.Preimages[j].Requester > extrinsicData.Preimages[j+1].Requester {
				extrinsicData.Preimages[j], extrinsicData.Preimages[j+1] = extrinsicData.Preimages[j+1], extrinsicData.Preimages[j]
			}
		}
	}

	// E_A - Assurances
	// 126 - The assurances must ordered by validator index
	extrinsicData.Assurances = extrinsic_pool.GetAssurancesFromPool(h.ParentHeaderHash)
	SortAssurances(extrinsicData.Assurances)

	tmpState := s.JamState.Copy()
	_, _ = tmpState.ProcessAssurances(extrinsicData.Assurances, targetJCE)
	// E_G - Guarantees: aggregate queuedGuarantees into extrinsicData.Guarantees
	extrinsicData.Guarantees = make([]types.Guarantee, 0)
	queuedGuarantees := make([]types.Guarantee, 0)
	currRotationIdx := s.GetTimeslot() / types.ValidatorCoreRotationPeriod
	previousIdx := currRotationIdx - 1
	acceptedTimeslot := previousIdx * types.ValidatorCoreRotationPeriod
	queuedGuarantees = extrinsic_pool.GetGuaranteesFromPool(acceptedTimeslot)
	log.Trace(debugG, "MakeBlock: Queued Guarantees for slot", "len", len(queuedGuarantees), "slot", targetJCE, "acceptedTs", acceptedTimeslot)
	for _, guarantee := range queuedGuarantees {
		g, err := guarantee.DeepCopy()
		if err != nil {
			continue
		}
		s.JamState.CheckInvalidCoreIndex()
		err = s.Verify_Guarantee_MakeBlock(g, b, tmpState)
		if err != nil {
			log.Debug(debugG, "Verify_Guarantee_MakeBlock", "err", err)
			continue
		}
		extrinsicData.Guarantees = append(extrinsicData.Guarantees, g)
		log.Debug(debugG, "MakeBlock: Added Guarantee", "g", g)
		// check guarantee one per core
		// check guarantee is not a duplicate
	}
	for i := 0; i < len(extrinsicData.Guarantees); i++ {
		log.Debug(debugG, "ExtrinsicData.Guarantees", "i", i, "wph", extrinsicData.Guarantees[i].Report.GetWorkPackageHash(), "coreIndex", extrinsicData.Guarantees[i].Report.CoreIndex)
	}
	extrinsicData.Guarantees, err, _ = s.VerifyGuaranteesMakeBlock(extrinsicData.Guarantees, b)
	if err != nil {
		log.Debug(debugG, "VerifyGuaranteesMakeBlock", "err", err)
	}
	// E_D - Disputes: aggregate queuedDisputes into extrinsicData.Disputes
	// d := s.GetJamState()

	// extrinsicData.Disputes = make([]types.Dispute, 0)
	// dispute := FormDispute(s.queuedVotes)
	// if d.NeedsOffendersMarker(&dispute) {
	// 	// Handle the case where the dispute does not need an offenders marker.
	// 	OffendMark, err := d.GetOffenderMark(dispute)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	h.OffendersMark = OffendMark.OffenderKey
	// }

	// TODO: 103 Verdicts v must be ordered by report hash.
	// TODO: 104 Offender signatures c and f must each be ordered by the validator’s Ed25519 key.
	// TODO: 105 There may be no duplicate report hashes within the extrinsic, nor amongst any past reported hashes.
	// target_Epoch, target_Phase := sf.EpochAndPhase(targetJCE)

	// eq 72
	if needWinningMarker {
		winningMarker, err := sf.GenerateWinningMarker()
		//block is the first after the end of the submission period for tickets and if the ticket accumulator is saturated
		if err == nil {
			h.TicketsMark = winningMarker
		}
	} else {
		// If there's new ticketID, add them into extrinsic
		// Question: can we submit tickets at the exact tail end block?
		extrinsicData.Tickets = make([]types.Ticket, 0)
		// add the limitation for receiving tickets
		if s.JamState.SafroleState.IsTicketSubmissionClosed(targetJCE) && !isNewEpoch {
			// s.queuedTickets = make(map[common.Hash]types.Ticket)

		} else {
			next_n2 := s.JamState.SafroleState.GetNextN2()
			tmp_accumulator := make([]types.TicketBody, len(s.JamState.SafroleState.NextEpochTicketsAccumulator))
			copy(tmp_accumulator, s.JamState.SafroleState.NextEpochTicketsAccumulator)
			// remove the tickets that already in state from the pool
			for _, ticket := range tmp_accumulator {
				extrinsic_pool.RemoveTicketFromPool(ticket.Id, next_n2)
			}
			// get the clean tickets out from the pool
			tickets := extrinsic_pool.GetTicketsFromPool(next_n2)
			SortTicketsById(tickets) // first include the better id
			if len(tickets) > types.MaxTicketsPerExtrinsic {
				tickets = tickets[:types.MaxTicketsPerExtrinsic]
			}
			for _, ticket := range tickets {
				ticket_body := ticket.ToBody()
				tmp_accumulator = append(tmp_accumulator, ticket_body)
			}
			SortTicketBodies(tmp_accumulator)
			tmp_accumulator = TrimTicketBodies(tmp_accumulator)
			// only include the tickets that will be included in the accumulator
			for _, ticket := range tickets {
				t, err := ticket.DeepCopy()
				if err != nil {
					continue
				}
				ticketID, _ := t.TicketID()
				if s.JamState.SafroleState.InTicketAccumulator(ticketID) {
					continue
				}
				if TicketInTmpAccumulator(ticketID, tmp_accumulator) {
					extrinsicData.Tickets = append(extrinsicData.Tickets, t)
				} else {
					extrinsic_pool.RemoveTicketFromPool(ticketID, next_n2)
				}
			}

		}
	}

	h.ExtrinsicHash = extrinsicData.Hash()
	author_index, err := sf.GetAuthorIndex(credential.BandersnatchPub.Hash(), "Curr")
	if err != nil {
		return bl, err
	}
	h.AuthorIndex = author_index
	b.Extrinsic = extrinsicData

	block_author_ietf_priv, err := ConvertBanderSnatchSecret(credential.BandersnatchSecret)
	if err != nil {
		return bl, err
	}
	block_author_ietf_pub, err := ConvertBanderSnatchPub(credential.BandersnatchPub[:])
	if err != nil {
		return bl, err
	}

	// For Primary, Verify ticketID actually matched the expected winning ticket
	_, ticketIDErr := s.ValidateVRFSealInput(ticketID, targetJCE)
	if ticketIDErr != nil {
		return bl, err
	}

	b.Header = *h
	sealedBlock, sealErr := s.SealBlockWithEntropy(block_author_ietf_pub, block_author_ietf_priv, author_index, targetJCE, b)
	if sealErr != nil {
		return bl, sealErr
	}
	return sealedBlock, nil
}

func (s *StateDB) ReSignDisputeBlock(credential types.ValidatorSecret, new_assurances []types.Assurance) error {
	sf := s.GetSafrole()
	b := s.GetBlock()

	b.Extrinsic.Assurances = new_assurances
	extrinsicData := b.Extrinsic
	b.Header.ExtrinsicHash = extrinsicData.Hash()
	author_index, err := sf.GetAuthorIndex(credential.BandersnatchPub.Hash(), "Curr")
	if err != nil {
		return err
	}
	b.Header.AuthorIndex = author_index
	b.Extrinsic = extrinsicData
	block_author_ietf_priv, err := ConvertBanderSnatchSecret(credential.BandersnatchSecret)
	if err != nil {
		return err
	}
	block_author_ietf_pub, err := ConvertBanderSnatchPub(credential.BandersnatchPub[:])
	if err != nil {
		return err
	}
	//make offender marker
	offenderMap := make(map[types.Ed25519Key]bool)
	for _, cruprit := range b.Extrinsic.Disputes.Culprit {
		offenderMap[cruprit.Key] = true
	}
	for _, fault := range b.Extrinsic.Disputes.Fault {
		offenderMap[fault.Key] = true
	}
	offenderMark := make([]types.Ed25519Key, 0)
	for key, _ := range offenderMap {
		offenderMark = append(offenderMark, key)
	}
	b.Header.OffendersMark = offenderMark
	if b.Header.EpochMark != nil {
		without_offenders_validators := [types.TotalValidators]types.ValidatorKeyTuple{}
		for i, key := range b.Header.EpochMark.Validators {
			//gamma k'
			validator_set := s.GetSafrole().DesignedValidators
			var ed25519Key types.Ed25519Key
			for _, validator := range validator_set {
				if validator.Bandersnatch.Hash() == key.BandersnatchKey {
					ed25519Key = validator.Ed25519
					break
				}
			}
			if offenderMap[ed25519Key] {
				without_offenders_validators[i] = types.ValidatorKeyTuple{}
			} else {
				without_offenders_validators[i] = key
			}
		}
		b.Header.EpochMark.Validators = without_offenders_validators
	}
	author_index = b.Header.AuthorIndex
	targetJCE := b.TimeSlot()
	sealedBlock, sealErr := s.SealBlockWithEntropy(block_author_ietf_pub, block_author_ietf_priv, author_index, targetJCE, b)
	if sealErr != nil {
		return sealErr
	}
	s.Block = sealedBlock
	return nil

}
