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
	alias, err := AliasOutput(s.Block.Header.EntropySource.Bytes())
	if err != nil {
		return nil, err
	}
	signtext := append([]byte(types.X_U), alias...)
	signature, _, err := bandersnatch.IetfVrfSign(V, []byte{}, []byte(signtext))
	if err != nil {
		return nil, err
	}
	return signature, nil
}

// func 201 TODO check double plus means
func (s *StateDB) Get_snQuantity(V bandersnatch.BanderSnatchSecret, W types.WorkReport) ([]byte, error) {
	signcontext := append(append([]byte(types.X_U), W.GetWorkPackageHash().Bytes()...), common.Uint32ToBytes(s.GetTranche())...)
	signature, _, err := bandersnatch.IetfVrfSign(V, []byte{0}, signcontext)
	if err != nil {
		return nil, err
	}
	return signature, nil
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
func (s *StateDB) Select_a0(V bandersnatch.BanderSnatchSecret) ([]types.WorkReportSelection, error) {
	s0, err := s.Get_s0Quantity(V)
	if err != nil {
		return nil, err
	}
	s0_alias, err := AliasOutput(s0)
	if err != nil {
		return nil, err
	}
	tmp := WorkReportToSelection(s.AvailableWorkReport)
	entropy := Compute_QL(common.BytesToHash(s0_alias), len(tmp))
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
	return a0, nil
}

func GetAnnouncementWithoutJtrue(A types.AnnounceBucket, J types.JudgeBucket, W_hash common.Hash) int {
	count := 0
	for _, a := range A.Announcements[W_hash] {
		for _, j := range J.Judgements[W_hash] {
			if (a.Core == j.Core) && (j.Judge == true) {
				count++
			}
		}
	}
	return count
}
func (s *StateDB) Select_an(V bandersnatch.BanderSnatchSecret, A_sub1 types.AnnounceBucket, J types.JudgeBucket) ([]types.WorkReportSelection, error) {
	an := []types.WorkReportSelection{}
	availible_workreport := WorkReportToSelection(s.AvailableWorkReport)
	for _, W := range availible_workreport {
		//get the number of true count
		sn, err := s.Get_snQuantity(V, W.WorkReport)
		if err != nil {
			return nil, err
		}
		sn_alias, err := AliasOutput(sn)
		if Bytes2Int(sn_alias)*types.AuditBiasFactor/256*types.TotalValidators < GetAnnouncementWithoutJtrue(A_sub1, J, W.WorkReport.GetWorkPackageHash()) {
			an = append(an, W)
		}
	}
	return an, nil
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

// eq 195
func (s *StateDB) GetTranche() uint32 {
	// timeslot mark
	currentJCETime := common.ComputeTimeUnit(types.TimeUnitMode)
	// currentJCETime := common.ComputeCurrentJCETime() // Replace with the actual value or variable representing the current JCE time
	return (currentJCETime - types.SecondsPerSlot*s.JamState.SafroleState.GetTimeSlot()) / types.PeriodSecond
}

// eq 196
func (s *StateDB) MakeAnnouncement(tranche uint32, workreport types.WorkReportSelection, Ed25519Secret []byte, validatoridx uint32) (types.Announcement, error) {
	announcement := types.Announcement{
		Core:           workreport.Core,
		Tranche:        tranche,
		WorkReport:     workreport.WorkReport,
		ValidatorIndex: validatoridx,
	}
	announcement.Sign(Ed25519Secret)
	return announcement, nil
}

func (s *StateDB) MakeJudgement(tranche uint32, workreport types.WorkReportSelection, judge bool, Ed25519Secret []byte, validator uint16) (types.Judgement, error) {
	judgement := types.Judgement{
		Core:       workreport.Core,
		Tranche:    tranche,
		Judge:      judge,
		Validator:  validator,
		WorkReport: workreport.WorkReport,
	}
	judgement.Sign(Ed25519Secret)
	return judgement, nil
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
	if GetAnnouncementWithoutJtrue(A, J, W_hash) == 0 {
		//double check no invalid
		for _, j := range J.Judgements[W_hash] {
			if j.Judge != true {
				return false
			}
		}
		return true
	}
	if J.GetTrueCount(W_hash) >= types.ValidatorsSuperMajority {
		return true
	}
	return false
}

func (s *StateDB) IsReportAuditedTiny(A types.AnnounceBucket, J types.JudgeBucket, W_hash common.Hash) error {
	if GetAnnouncementWithoutJtrue(A, J, W_hash) == 0 {
		//double check no invalid
		for _, j := range J.Judgements[W_hash] {
			if j.Judge != true {
				return fmt.Errorf("Validator[%d] said false /n", j.Core)
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
		if s.IsReportAudited(A, J, av.GetWorkPackageHash()) {
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
				Key:       s.GetSafrole().CurrValidators[int(j.Core)].Ed25519,
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

func (s *StateDB) JudgementToCulprit(J []types.Judgement, W_hash common.Hash) []types.Culprit {
	//Stub: need culprit from judgment here.
	culprits := make([]types.Culprit, 0)
	for _, rho := range s.JamState.AvailabilityAssignments {
		if rho.WorkReport.GetWorkPackageHash() == W_hash {
			culprits = append(culprits, s.GetCulprits(rho.Timeslot, rho.WorkReport)...) //using time slot to lookup the extrinsic G??
		}
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
func (s *StateDB) AppendDisputes(J types.JudgeBucket, W_hash common.Hash) error {
	//types.TotalValidators*2/3 + 1 = supep majority
	true_count := J.GetTrueCount(W_hash)
	false_count := J.GetFalseCount(W_hash)

	if true_count+false_count < types.ValidatorsSuperMajority {
		return fmt.Errorf("Not enough votes: true_count=%d, false_count=%d", true_count, false_count)
	} else if true_count >= types.ValidatorsSuperMajority && false_count == 0 {
		return errors.New("Not Required")
	} else if true_count >= types.ValidatorsSuperMajority && false_count > 0 {
		// disput required - Good set
		true_votes := JudgementToVote(J.GetTrueJudgement(W_hash))
		faults := s.JudgementToFault(J.GetFalseJudgement(W_hash), W_hash) //the false goes to fault
		goodset_verdict := types.Verdict{
			Target: W_hash,
			Epoch:  s.GetSafrole().Epoch,
		}

		for i, true_vote := range true_votes {
			goodset_verdict.Votes[i] = true_vote //get 2/3+1 true
		}

		s.Block.Extrinsic.Disputes.Verdict = append(s.Block.Extrinsic.Disputes.Verdict, goodset_verdict)
		s.Block.Extrinsic.Disputes.Fault = append(s.Block.Extrinsic.Disputes.Fault, faults...)
		fmt.Println("PrintSomething")

	} else if false_count >= types.ValidatorsSuperMajority {
		// disput required - Bad set
		false_votes := JudgementToVote(J.GetFalseJudgement(W_hash))
		badset_verdict := types.Verdict{
			Target: W_hash,
			Epoch:  s.GetSafrole().Epoch,
		}
		for i, false_vote := range false_votes {
			badset_verdict.Votes[i] = false_vote //get 2/3+1 false
		}
		s.Block.Extrinsic.Disputes.Verdict = append(s.Block.Extrinsic.Disputes.Verdict, badset_verdict)

		culprits := s.JudgementToCulprit(J.GetFalseJudgement(W_hash), W_hash)
		s.Block.Extrinsic.Disputes.Culprit = culprits
	} else if true_count >= types.WonkyTrueThreshold && false_count >= types.WonkyFalseThreshold {
		wonky_votes := JudgementToVote(J.GetWonkeyJudgement(W_hash))
		wonky_verdict := types.Verdict{
			Target: W_hash,
			Epoch:  s.GetSafrole().Epoch,
		}
		true_c := 0
		false_c := 0
		for i, wonky_vote := range wonky_votes {
			if wonky_vote.Voting == true {
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
		s.Block.Extrinsic.Disputes.Verdict = append(s.Block.Extrinsic.Disputes.Verdict)
	} else {
		// shouldn't go here ..
		panic(0)
	}

	return nil

}
