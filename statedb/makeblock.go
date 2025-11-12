package statedb

import (
	"context"
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

func (s *StateDB) CollectExtrinsicData(
	ctx context.Context,
	targetJCE uint32,
	extrinsicPool *types.ExtrinsicPool,
) (types.ExtrinsicData, error) {

	sf := s.GetSafrole()
	needWinningMarker := sf.IseWinningMarkerNeeded(targetJCE)

	extrinsicData := types.NewExtrinsic()

	// ---------- E_P: Preimages ----------
	extrinsicData.Preimages = make([]types.Preimages, 0)
	queuedPreimage := extrinsicPool.GetPreimageFromPool()
	log.Trace(log.P, "MakeBlock: Queued Preimages", "len", len(queuedPreimage), "slot", targetJCE)

	for _, preimageLookup := range queuedPreimage {
		if _, err := s.ValidateAddPreimage(preimageLookup.Requester, preimageLookup.Blob); err != nil {
			log.Warn(log.P, "ValidateLookup", "err", err)
			continue
		}
		pl, err := preimageLookup.DeepCopy()
		if err != nil {
			continue
		}
		extrinsicData.Preimages = append(extrinsicData.Preimages, pl)
		extrinsicPool.RemoveOldPreimages([]types.Preimages{*preimageLookup}, targetJCE)
	}

	// ordered & dedup by Requester (using the original bubble sort implementation)
	for i := 0; i < len(extrinsicData.Preimages); i++ {
		for j := 0; j < len(extrinsicData.Preimages)-1; j++ {
			if extrinsicData.Preimages[j].Requester > extrinsicData.Preimages[j+1].Requester {
				extrinsicData.Preimages[j], extrinsicData.Preimages[j+1] =
					extrinsicData.Preimages[j+1], extrinsicData.Preimages[j]
			}
		}
	}

	// ---------- E_A: Assurances ----------
	unvalidatedAssurances := extrinsicPool.GetAssurancesFromPool(s.HeaderHash)
	assurances, err := s.GetValidAssurances(unvalidatedAssurances, s.HeaderHash, true)
	if err != nil {
		return types.ExtrinsicData{}, err
	}
	extrinsicData.Assurances = assurances

	// Build tmpState for Guarantees verification
	tmpState := s.JamState.Copy()
	tmpState.ComputeAvailabilityAssignments(extrinsicData.Assurances, targetJCE)

	// ---------- E_G: Guarantees ----------
	extrinsicData.Guarantees = make([]types.Guarantee, 0)

	currRotationIdx := s.GetTimeslot() / types.ValidatorCoreRotationPeriod
	previousIdx := currRotationIdx - 1
	acceptedTimeslot := previousIdx * types.ValidatorCoreRotationPeriod

	queuedGuarantees := extrinsicPool.GetGuaranteesFromPool(acceptedTimeslot)
	log.Trace(log.G, "MakeBlock: Queued Guarantees for slot",
		"len", len(queuedGuarantees), "slot", targetJCE, "acceptedTs", acceptedTimeslot)

	var valid []types.Guarantee
	tmpstatedb := s.Copy()
	tmpstatedb.JamState = tmpState

	for _, q := range queuedGuarantees {
		g, err := q.DeepCopy()
		if err != nil {
			continue
		}
		if err := tmpstatedb.VerifyGuaranteeBasic(g, targetJCE); err != nil {
			if AcceptableGuaranteeError(err) {
				log.Trace(log.G, "[IGNORE]MakeBlock: VerifyGuaranteeBasic:acceptable error", "err", err)
			} else {
				extrinsicPool.RemoveOldGuarantees(g, types.GuaranteeDiscardReasonCannotReportOnChain)
			}
			continue
		}
		valid = append(valid, g)

		if err := ctx.Err(); err != nil {
			return types.ExtrinsicData{}, fmt.Errorf("CollectExtrinsicData: %w", err)
		}
	}

	// Inter-dependency: recent work / prerequisites / uniqueness by wph + coreIndex
	var final []types.Guarantee
	p_w := make(map[common.Hash]bool)
	p_c := make(map[uint16]bool)

	for _, g := range valid {
		if err := s.checkRecentWorkPackage(g, valid); err != nil {
			extrinsicPool.RemoveOldGuarantees(g, types.GuaranteeDiscardReasonCannotReportOnChain)
			continue
		}
		if err := s.checkPrereq(g, valid); err != nil {
			extrinsicPool.RemoveOldGuarantees(g, types.GuaranteeDiscardReasonCannotReportOnChain)
			continue
		}
		wph := g.Report.GetWorkPackageHash()
		coreIndex := uint16(g.Report.CoreIndex)

		if !p_w[wph] && !p_c[coreIndex] {
			p_w[wph] = true
			p_c[coreIndex] = true
			final = append(final, g)
		} else {
			extrinsicPool.RemoveOldGuarantees(g, types.GuaranteeDiscardReasonReplacedByBetter)
		}
	}

	sort.Slice(final, func(i, j int) bool {
		return final[i].Report.CoreIndex < final[j].Report.CoreIndex
	})
	extrinsicData.Guarantees = final

	// ---------- E_D: Disputes ----------
	// TODO retained; per your original note, bring it in if needed

	// ---------- Tickets ----------
	if !needWinningMarker {
		extrinsicData.Tickets = make([]types.Ticket, 0)

		if !s.JamState.SafroleState.IsTicketSubmissionClosed(targetJCE) {
			nextN2 := s.JamState.SafroleState.GetNextN2(targetJCE)

			tmpAcc := make([]types.TicketBody, len(s.JamState.SafroleState.NextEpochTicketsAccumulator))
			copy(tmpAcc, s.JamState.SafroleState.NextEpochTicketsAccumulator)

			for _, ticket := range tmpAcc {
				extrinsicPool.RemoveTicketFromPool(ticket.Id, nextN2)
			}

			tickets := extrinsicPool.GetTicketsFromPool(nextN2)
			SortTicketsById(tickets)
			if len(tickets) > types.MaxTicketsPerExtrinsic {
				tickets = tickets[:types.MaxTicketsPerExtrinsic]
			}

			for _, ticket := range tickets {
				tmpAcc = append(tmpAcc, ticket.ToBody())
			}
			SortTicketBodies(tmpAcc)
			tmpAcc = TrimTicketBodies(tmpAcc)

			for _, ticket := range tickets {
				t, err := ticket.DeepCopy()
				if err != nil {
					continue
				}
				ticketID, _ := t.TicketID()
				if s.JamState.SafroleState.InTicketAccumulator(ticketID) {
					continue
				}
				if TicketInTmpAccumulator(ticketID, tmpAcc) {
					extrinsicData.Tickets = append(extrinsicData.Tickets, t)
				} else {
					extrinsicPool.RemoveTicketFromPool(ticketID, nextN2)
				}

				select {
				case <-ctx.Done():
					return types.ExtrinsicData{}, fmt.Errorf("CollectExtrinsicData: Add ticket canceled")
				default:
				}
			}
		}
	}

	return extrinsicData, nil
}
func (s *StateDB) BuildBlock(
	ctx context.Context,
	credential types.ValidatorSecret,
	targetJCE uint32,
	ticketID common.Hash,
	extrinsicData *types.ExtrinsicData,
) (*types.Block, error) {
	sf := s.GetSafrole()
	isNewEpoch := sf.IsNewEpoch(targetJCE)
	needWinningMarker := sf.IseWinningMarkerNeeded(targetJCE)

	stateRoot := s.GetStateRoot()
	s.JamState.CheckInvalidCoreIndex()
	if err := s.RecoverJamState(stateRoot); err != nil {
		return nil, err
	}
	s.JamState.CheckInvalidCoreIndex()
	s.Authoring = log.GeneralAuthoring

	// 2) Assemble Header
	h := types.NewBlockHeader()
	h.ParentHeaderHash = s.HeaderHash
	h.ParentStateRoot = stateRoot
	h.Slot = targetJCE

	if isNewEpoch {
		epochMarker := sf.GenerateEpochMarker()
		h.EpochMark = epochMarker
	}

	if needWinningMarker {
		winningMarker, err := sf.GenerateWinningMarker()
		if err == nil {
			h.TicketsMark = winningMarker
		}
	}

	h.ExtrinsicHash = extrinsicData.Hash()

	authorIndex, err := sf.GetAuthorIndex(credential.BandersnatchPub.Hash(), "Curr")
	if err != nil {
		return nil, err
	}
	h.AuthorIndex = authorIndex

	// 3) Assemble Block
	b := types.NewBlock()
	b.Header = *h
	b.Extrinsic = *extrinsicData

	// 4) VRF / Seal
	blockAuthorPriv, err := ConvertBanderSnatchSecret(credential.BandersnatchSecret)
	if err != nil {
		return nil, err
	}
	blockAuthorPub, err := ConvertBanderSnatchPub(credential.BandersnatchPub[:])
	if err != nil {
		return nil, err
	}

	if _, ticketErr := s.ValidateVRFSealInput(ticketID, targetJCE); ticketErr != nil {
		return nil, ticketErr
	}

	sealedBlock, sealErr := s.SealBlockWithEntropy(
		blockAuthorPub,
		blockAuthorPriv,
		authorIndex,
		targetJCE,
		b,
	)
	if sealErr != nil {
		return nil, sealErr
	}

	return sealedBlock, nil
}

func (s *StateDB) MakeBlock(ctx context.Context, credential types.ValidatorSecret, targetJCE uint32, ticketID common.Hash, extrinsic_pool *types.ExtrinsicPool) (bl *types.Block, err error) {
	extrinsicData, err := s.CollectExtrinsicData(ctx, targetJCE, extrinsic_pool)
	if err != nil {
		return nil, err
	}
	return s.BuildBlock(ctx, credential, targetJCE, ticketID, &extrinsicData)
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
	for key := range offenderMap {
		offenderMark = append(offenderMark, key)
	}
	b.Header.OffendersMark = offenderMark
	if b.Header.EpochMark != nil {
		without_offenders_validators := [types.TotalValidators]types.ValidatorKeyTuple{}
		for i, key := range b.Header.EpochMark.Validators {
			//gamma k'
			validator_set := s.GetSafrole().DesignatedValidators
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
