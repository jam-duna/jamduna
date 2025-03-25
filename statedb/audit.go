/*
we cannot assume consensus, we henceforth consider ourselves a specific validator
of index v and assume ourselves focused on some block B with other terms corresponding,
 so σ′ is said block’s posterior state, H is its header &c.
 Practically, all considerations must be replicated for all blocks and multiple blocks’ considerations may be underway simultaneously.
*/

package statedb

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"sort"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// eq 191
func (s *StateDB) GetWorkReportNeedAudit() types.WorkReportNeedAudit {
	var w types.WorkReportNeedAudit
	for i, W := range s.JamState.AvailabilityAssignments {
		//if W in availableWorkReport , then add to w
		for _, bigW := range s.AvailableWorkReport {
			if W.WorkReport.GetWorkPackageHash() == bigW.GetWorkPackageHash() {
				w.Q[i] = bigW
				fmt.Println("Add to w")
			} else {
				w.Q[i] = types.WorkReport{}
			}
		}

	}
	return w
}

func (s *StateDB) GetWorkReportNeedAuditTiny() []types.WorkReport {
	var w []types.WorkReport
	for _, W := range s.JamState.AvailabilityAssignments {
		//if W in availableWorkReport , then add to w
		for _, bigW := range s.AvailableWorkReport {
			if W.WorkReport.GetWorkPackageHash() == bigW.GetWorkPackageHash() {
				w = append(w, bigW)
			}
		}

	}
	return w
}

// eq 190
func (s *StateDB) Get_s0Quantity(V bandersnatch.BanderSnatchSecret) ([]byte, error) {
	if len(V) != 32 {
		return nil, errors.New("Invalid length of V")
	}
	alias, err := AliasOutput(s.Block.Header.EntropySource.Bytes())
	if err != nil {
		return nil, err
	}
	signtext := append([]byte(types.X_U), alias...)
	// TODO: Check the input of IetfVrfSign, the second parameter should be empty
	signature, _, err := bandersnatch.IetfVrfSign(V, []byte{0}, []byte(signtext))
	if err != nil {
		return nil, err
	}
	return signature, nil
}

func (s *StateDB) Verify_s0(pubkey bandersnatch.BanderSnatchKey) (bool, error) {
	if len(pubkey) != 32 {
		return false, errors.New("Invalid length of pubkey")
	}
	alias, err := AliasOutput(s.Block.Header.EntropySource.Bytes())
	if err != nil {
		return false, err
	}
	signtext := append([]byte(types.X_U), alias...)
	_, err = bandersnatch.IetfVrfVerify(pubkey, []byte{0}, []byte(signtext), s.Block.Header.EntropySource.Bytes())
	if err != nil {
		return false, err
	}
	return true, nil
}

// func 201 TODO check double plus means
func (s *StateDB) Get_snQuantity(V bandersnatch.BanderSnatchSecret, W types.WorkReport, currJCE uint32) ([]byte, error) {
	signcontext := append(append([]byte(types.X_U), W.Hash().Bytes()...), common.Uint32ToBytes(s.GetTranche(currJCE))...)
	signature, _, err := bandersnatch.IetfVrfSign(V, []byte{0}, signcontext)
	if err != nil {
		return nil, err
	}
	return signature, nil
}

func (s *StateDB) Verify_sn(pubkey bandersnatch.BanderSnatchKey, W common.Hash, signature []byte, currJCE uint32) (bool, error) {
	signcontext := append(append([]byte(types.X_U), W.Bytes()...), common.Uint32ToBytes(s.GetTranche(currJCE))...)
	_, err := bandersnatch.IetfVrfVerify(pubkey, []byte{0}, signcontext, signature)
	if err != nil {
		return false, err
	}
	return true, nil
}

func WorkReportToSelection(W []types.WorkReport) []types.WorkReportSelection {
	selections := make([]types.WorkReportSelection, 0)
	for _, w := range W {
		selections = append(selections, types.WorkReportSelection{
			WorkReport: w,
			Core:       w.CoreIndex,
		})
	}
	return selections
}

// eq 192
func (s *StateDB) Select_a0(V bandersnatch.BanderSnatchSecret) ([]types.WorkReportSelection, bandersnatch.BandersnatchVrfSignature, error) {
	s0, err := s.Get_s0Quantity(V)
	if err != nil {
		return nil, bandersnatch.BandersnatchVrfSignature{}, err
	}
	s0_alias, err := AliasOutput(s0)
	if err != nil {
		return nil, bandersnatch.BandersnatchVrfSignature{}, err
	}
	tmp := WorkReportToSelection(s.AvailableWorkReport)

	entropy := NumericSequenceFromHash(common.BytesToHash(s0_alias), uint32(len(tmp)))
	ShuffleWorkReport(tmp, entropy)

	var a0 []types.WorkReportSelection
	count := 0
	for _, W := range tmp {
		//choose the first 10 without empty work report
		if bytes.Equal(W.WorkReport.Bytes(), []byte{}) {
			continue
		}
		a0 = append(a0, W)
		count++
		if count == 10 {
			break
		}

	}
	if len(s0) != 96 {
		return nil, bandersnatch.BandersnatchVrfSignature{}, errors.New("Invalid length of s0")
	}
	var s0Signature bandersnatch.BandersnatchVrfSignature
	copy(s0Signature[:], s0)
	return a0, s0Signature, nil
}

func (s *StateDB) GetAnnouncementWithoutJtrue(A types.AnnounceBucket, J types.JudgeBucket, W_hash common.Hash) ([]types.Announcement, int) {
	count := 0
	var announcements = make([]types.Announcement, 0)
	for _, a := range A.Announcements[W_hash] {
		var tmp types.Judgement
		for _, j := range J.Judgements[W_hash] {
			tmp = j
			if !j.Judge && a.ValidatorIndex == uint32(j.Validator) {
				count++
				announcements = append(announcements, a)
				break
			} else if a.ValidatorIndex == uint32(j.Validator) {
				break
			}
		}
		// check the last one
		if a.ValidatorIndex != uint32(tmp.Validator) {
			log.Trace(debugAudit, "Validator didn't judge in Tranche", "n", s.Id, "ts", s.Block.TimeSlot(), "validatorIndex", a.ValidatorIndex, "tranche", a.Tranche)
			count++
			announcements = append(announcements, a)
		}
	}
	return announcements, count
}
func (s *StateDB) Select_an(V bandersnatch.BanderSnatchSecret, A_sub1 types.AnnounceBucket, J types.JudgeBucket, currJCE uint32) ([]types.WorkReportSelection, map[common.Hash][]types.Announcement, map[common.Hash]int, []bandersnatch.BandersnatchVrfSignature, error) {
	an := []types.WorkReportSelection{}
	availible_workreport := WorkReportToSelection(s.AvailableWorkReport)
	no_show_announcements := make(map[common.Hash][]types.Announcement)
	no_show_length := make(map[common.Hash]int)
	sns := make([]bandersnatch.BandersnatchVrfSignature, 0)
	for _, W := range availible_workreport {
		//get the number of true count
		sn, err := s.Get_snQuantity(V, W.WorkReport, currJCE)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		sn_alias, err := AliasOutput(sn)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		announcements, length := s.GetAnnouncementWithoutJtrue(A_sub1, J, W.WorkReport.Hash())
		if Bytes2Int(sn_alias)*types.AuditBiasFactor/256*types.TotalValidators < length || true {
			an = append(an, W)
			// sn=> bandersnatch.BandersnatchVrfSignature
			var sig bandersnatch.BandersnatchVrfSignature
			copy(sig[:], sn)
			sns = append(sns, sig)
			no_show_announcements[W.WorkReport.Hash()] = announcements
			no_show_length[W.WorkReport.Hash()] = length
		}
	}
	return an, no_show_announcements, no_show_length, sns, nil
}

// func (s *StateDB) SelectionToAnnouncement(an []types.WorkReportSelection) []types.Announcement

func Bytes2Int(b []byte) int {
	var x int
	for i := 0; i < len(b); i++ {
		x = x << 8
		x = x | int(b[i])
	}
	return x
}

// for eq 194
func AliasOutput(x []byte) ([]byte, error) {
	//check length > 32
	if len(x) < 32 {
		return nil, errors.New("invalid length")
	}
	return x[:32], nil
}

// for eq 193
func ShuffleWorkReport(slice []types.WorkReportSelection, entropy []uint32) {
	n := len(slice)
	for i := n - 1; i >= 0; i-- {
		j := entropy[i] % uint32(i+1)
		slice[i], slice[j] = slice[j], slice[i]
	}
	return
}

// TODO: tranch is potentially touching JCE logic with now() -- shawn to fix it
// eq 195
func (s *StateDB) GetTranche(currJCE uint32) uint32 {
	// timeslot mark
	//currentTime := time.Now().Unix()
	//JCE := uint32(common.ComputeJCETime(currentTime, true))
	timeslot := s.Block.TimeSlot()
	if s.Block == nil {
		fmt.Printf("Block is nil\n")
	}
	return (currJCE - types.SecondsPerSlot*timeslot) / types.PeriodSecond
	// return 0
}

// eq 196
func (s *StateDB) MakeAnnouncement(tranche uint32, workreport []types.WorkReportSelection, Ed25519Secret []byte, validatoridx uint32) (types.Announcement, error) {
	var annReports []types.AnnouncementReport
	for _, w := range workreport {
		annReports = append(annReports, types.AnnouncementReport{
			Core:           w.Core,
			WorkReportHash: w.WorkReport.Hash(),
		})
	}

	announcement := types.Announcement{
		HeaderHash:          s.GetHeaderHash(),
		Tranche:             tranche,
		Selected_WorkReport: annReports,
		ValidatorIndex:      validatoridx,
	}
	announcement.Sign(Ed25519Secret)
	return announcement, nil
}

// eq 203

func (s *StateDB) ValidateWorkReport(wp types.WorkPackage) bool {
	// Simulate a 10% chance of returning false
	if rand.Intn(10) == 0 {
		return false
	}
	// TODO: Implement the actual validation logic
	return true
}

// eq 205
func (a *StateDB) IsReportAudited(A types.AnnounceBucket, J types.JudgeBucket, W_hash common.Hash) bool {
	_, length := a.GetAnnouncementWithoutJtrue(A, J, W_hash)
	if length == 0 {
		//double check no invalid
		for _, j := range J.Judgements[W_hash] {
			if !j.Judge {
				return false
			}
		}
		return true
	} else if J.GetTrueCount(W_hash) >= types.ValidatorsSuperMajority {
		return true
	}

	return false
}

func (s *StateDB) IsReportAuditedTiny(A types.AnnounceBucket, J types.JudgeBucket, W_hash common.Hash) error {
	_, length := s.GetAnnouncementWithoutJtrue(A, J, W_hash)
	if length == 0 {
		//double check no invalid
		for _, j := range J.Judgements[W_hash] {
			if j.Judge != true {
				return fmt.Errorf("Validator[%d] said false /n", j.Validator)
			}
		}
		return nil
	}
	if J.GetTrueCount(W_hash) >= types.ValidatorsSuperMajority {
		return nil
	}
	return fmt.Errorf("Audit not yet finished")

}

// 206 big U
func (s *StateDB) IsBlockAudited(A types.AnnounceBucket, J types.JudgeBucket) bool {

	for _, av := range s.AvailableWorkReport {
		if s.IsReportAudited(A, J, av.Hash()) {
			continue
		} else {
			return false
		}
	}
	return true
}

func (s *StateDB) IsBlockAuditedTiny(A types.AnnounceBucket, J types.JudgeBucket) error {
	for _, av := range s.AvailableWorkReport {
		if s.IsReportAuditedTiny(A, J, av.GetWorkPackageHash()) == nil {
			continue
		} else {
			return fmt.Errorf("WorkReport:%v didn't finish audit\n", av.GetWorkPackageHash())
		}
	}
	return nil
}

func JudgementToVote(J []types.Judgement) []types.Vote {
	votes := make([]types.Vote, 0)
	for _, j := range J {
		votes = append(votes, types.Vote{
			Voting:    j.Judge,
			Index:     j.Validator,
			Signature: j.Signature,
		})
	}
	//sort by Index
	sort.Slice(votes, func(i, j int) bool {
		return votes[i].Index < votes[j].Index
	})
	return votes
}

func (s *StateDB) JudgementToFault(J []types.Judgement, W_hash common.Hash) []types.Fault {
	faults := make([]types.Fault, 0)
	for _, j := range J {
		if j.Judge {
			continue
		} else {
			faults = append(faults, types.Fault{
				Target:    W_hash,
				Voting:    j.Judge,
				Key:       s.GetSafrole().CurrValidators[int(j.Validator)].Ed25519,
				Signature: j.Signature,
			})
		}
	}
	//sort by key
	sort.Slice(faults, func(i, j int) bool {
		return bytes.Compare(faults[i].Key[:], faults[j].Key[:]) < 0
	})
	return faults
}

func (s *StateDB) JudgementToCulprit(old_guarantee types.Guarantee) []types.Culprit {
	//Stub: need culprit from judgment here.
	culprits := make([]types.Culprit, 0)
	for _, cred := range old_guarantee.Signatures {
		key := s.GetSafrole().GetCurrValidator(int(cred.ValidatorIndex)).Ed25519
		culprits = append(culprits, types.Culprit{
			Target:    old_guarantee.Report.Hash(),
			Key:       key,
			Signature: cred.Signature,
		})
	}
	return culprits
}

// TODO: here is the stub code to lookup the EG
func (s *StateDB) GetCulprits(ts uint32, report types.WorkReport) []types.Culprit {
	culprits := make([]types.Culprit, 0)
	// for _, G := range s.GuarantorAssignments {
	// 	if G.CoreIndex == report.CoreIndex {
	// 		culprits = append(culprits, types.Culprit{
	// 			Target:    report.GetWorkPackageHash(),
	// 			Key:       G.Validator.Ed25519,
	// 			Signature: types.Ed25519Sign(G.Validator., []byte("Something")),
	// 		})
	// 	}
	// }
	return culprits
}

// issue dispute extrinsic
func (s *StateDB) AppendDisputes(J *types.JudgeBucket, W_hash common.Hash, old_guarantee types.Guarantee) (*types.Block, error) {
	true_count := J.GetTrueCount(W_hash)
	false_count := J.GetFalseCount(W_hash)
	block := s.Block.Copy()
	if true_count+false_count < types.ValidatorsSuperMajority {
		return nil, fmt.Errorf("Not enough votes: true_count=%d, false_count=%d", true_count, false_count)
	}

	if true_count >= types.ValidatorsSuperMajority {
		if false_count == 0 {
			return nil, errors.New("Not Required")
		}
		s.appendGoodSetDispute(block, J, W_hash)
		return block, nil
	} else if false_count >= types.ValidatorsSuperMajority {
		s.appendBadSetDispute(block, J, W_hash, old_guarantee)
		return block, nil
	} else if true_count >= types.WonkyTrueThreshold && false_count >= types.WonkyFalseThreshold {
		s.appendWonkySetDispute(block, J, W_hash)
		return block, nil
	}

	return nil, fmt.Errorf("No need to dispute")
}

func (s *StateDB) appendGoodSetDispute(block *types.Block, J *types.JudgeBucket, W_hash common.Hash) {
	true_votes := JudgementToVote(J.GetTrueJudgement(W_hash))
	faults := s.JudgementToFault(J.GetFalseJudgement(W_hash), W_hash)
	goodset_verdict := types.Verdict{
		Target: W_hash,
		Epoch:  s.GetSafrole().GetEpoch(),
	}

	for i, true_vote := range true_votes {
		goodset_verdict.Votes[i] = true_vote
	}

	block.Extrinsic.Disputes.Verdict = append(s.Block.Extrinsic.Disputes.Verdict, goodset_verdict)
	block.Extrinsic.Disputes.Fault = append(s.Block.Extrinsic.Disputes.Fault, faults...)
}

func (s *StateDB) appendBadSetDispute(block *types.Block, J *types.JudgeBucket, W_hash common.Hash, old_guarantee types.Guarantee) {
	false_votes := JudgementToVote(J.GetFalseJudgement(W_hash))
	badset_verdict := types.Verdict{
		Target: W_hash,
		Epoch:  s.GetSafrole().GetEpoch(),
	}

	for i, false_vote := range false_votes {
		badset_verdict.Votes[i] = false_vote
	}

	block.Extrinsic.Disputes.Verdict = append(block.Extrinsic.Disputes.Verdict, badset_verdict)
	culprits := s.JudgementToCulprit(old_guarantee)
	block.Extrinsic.Disputes.Culprit = append(block.Extrinsic.Disputes.Culprit, culprits...)
}

func (s *StateDB) appendWonkySetDispute(block *types.Block, J *types.JudgeBucket, W_hash common.Hash) {
	wonky_votes := JudgementToVote(J.GetWonkeyJudgement(W_hash))
	wonky_verdict := types.Verdict{
		Target: W_hash,
		Epoch:  s.GetSafrole().GetEpoch(),
	}

	true_c, false_c := 0, 0
	for i, wonky_vote := range wonky_votes {
		if wonky_vote.Voting {
			if true_c >= types.WonkyTrueThreshold {
				continue
			}
			true_c++
		} else {
			if false_c >= types.WonkyFalseThreshold {
				continue
			}
			false_c++
		}
		wonky_verdict.Votes[i] = wonky_vote
	}

	block.Extrinsic.Disputes.Verdict = append(block.Extrinsic.Disputes.Verdict, wonky_verdict)
}
