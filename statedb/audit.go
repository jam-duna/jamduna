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
	"math/rand"
	"sort"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// eq 191
func (s *StateDB) GetWorkReportNeedAudit(availableWorkReport []types.WorkReport) *types.WorkReportNeedAudit {
	var w types.WorkReportNeedAudit
	for i, W := range s.JamState.AvailabilityAssignments {
		//if W in availableWorkReport , then add to w
		for _, bigW := range availableWorkReport {
			if bytes.Equal(W.WorkReport.Bytes(), bigW.Bytes()) {
				w.Q[i] = bigW
			}
		}
		//if W not in availableWorkReport , then nothing
		w.Q[i] = types.WorkReport{}

	}
	return &w

}

// eq 190, Hv should be removed, skip for now
func (s *StateDB) Get_s0Quantity(V bandersnatch.PrivateKey) ([]byte, error) {
	signature, _, err := bandersnatch.IetfVrfSign(V, []byte{}, []byte(types.X_U))
	if err != nil {
		return nil, err
	}
	return signature, nil
}

// func 201
func (s *StateDB) Get_snQuantity(V bandersnatch.PrivateKey, W types.WorkReport) ([]byte, error) {
	signcontext := append(append([]byte(types.X_U), W.Hash().Bytes()...), types.Uint32ToBytes(s.GetTranche())...)
	signature, _, err := bandersnatch.IetfVrfSign(V, []byte{}, signcontext)
	if err != nil {
		return nil, err
	}
	return signature, nil
}

// eq 192
func (s *StateDB) Select_a0(availible_workreport []types.WorkReportSelection, V bandersnatch.PrivateKey) ([]types.WorkReportSelection, error) {
	s0, err := s.Get_s0Quantity(V)
	if err != nil {
		return nil, err
	}
	s0_alias, err := AliasOutput(s0)
	if err != nil {
		return nil, err
	}
	tmp := availible_workreport
	FisherYatesShuffle_WorkReport(tmp, s0_alias)

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
func GetAnnouncementWithoutJtrue(A types.WhoAudit, J types.WhoJudge, W_hash common.Hash) int {
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
func (s *StateDB) Select_an(availible_workreport []types.WorkReportSelection, V bandersnatch.PrivateKey, A_sub1 types.WhoAudit, J types.WhoJudge) ([]types.WorkReportSelection, error) {
	an := []types.WorkReportSelection{}
	for _, W := range availible_workreport {
		//get the number of true count
		sn, err := s.Get_snQuantity(V, W.WorkReport)
		if err != nil {
			return nil, err
		}
		sn_alias, err := AliasOutput(sn)
		if Bytes2Int(sn_alias)*types.AuditBiasFactor/256*types.TotalValidators < GetAnnouncementWithoutJtrue(A_sub1, J, W.WorkReport.Hash()) {
			an = append(an, W)
		}
	}
	return an, nil

}

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
func FisherYatesShuffle_WorkReport(slice []types.WorkReportSelection, entropy []byte) {
	n := len(slice)
	for i := n - 1; i > 0; i-- {
		j := int(entropy[i%len(entropy)]) % (i + 1)
		slice[i], slice[j] = slice[j], slice[i]
	}
}

// eq 195
func (s *StateDB) GetTranche() uint32 {
	currentJCETime := ComputeCurrentJCETime() // Replace with the actual value or variable representing the current JCE time
	return (currentJCETime - types.SecondsPerSlot*s.JamState.SafroleState.GetTimeSlot()) / types.PeriodSecond
}

// eq 196
func (s *StateDB) GetAnnouncement(core uint16, workReport types.WorkReport, Ed25519Secret []byte) (*types.Announcement, error) {
	tranche := s.GetTranche()
	announcement := &types.Announcement{
		Core:       core,
		Tranche:    tranche,
		WorkReport: workReport,
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
func (a *StateDB) IsAuditedReport(A types.WhoAudit, J types.WhoJudge, W_hash common.Hash) bool {
	if GetAnnouncementWithoutJtrue(A, J, W_hash) == 0 {
		//double check no invalid
		for _, j := range J.Judgements[W_hash] {
			if j.Judge != true {
				return false
			}
		}
		return true
	}
	if J.GetTrueCount(W_hash) > types.TotalValidators*2/3 {
		return true
	}
	return false
}

func JudgementToVote(J []types.Judgement) []types.Vote {
	votes := make([]types.Vote, 0)
	for _, j := range J {
		votes = append(votes, types.Vote{
			Voting:    j.Judge,
			Index:     j.Core,
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
	return culprits
}

// issue dispute extrinsic
func (s *StateDB) AppendDisputes(J types.WhoJudge, W_hash common.Hash) {
	//types.TotalValidators*2/3 + 1 = supep majority
	true_count := J.GetTrueCount(W_hash)
	false_count := J.GetFalseCount(W_hash)

	if true_count+false_count < types.ValidatorsSuperMajority {
		// not enough vote to compute verdict
	} else if true_count >= types.ValidatorsSuperMajority && false_count == 0 {
		// super majority met - disput not required
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

		//TODO Get Culprits Core's Validator??
		culprits := s.JudgementToCulprit(J.GetFalseJudgement(W_hash), W_hash)
		s.Block.Extrinsic.Disputes.Culprit = culprits
	} else if true_count+false_count >= types.ValidatorsSuperMajority {
		// wonky ?
		wonky_votes := JudgementToVote(J.GetWonkeyJudgement(W_hash))
		wonky_verdict := types.Verdict{
			Target: W_hash,
			Epoch:  s.GetSafrole().Epoch,
		}
		for i, wonky_vote := range wonky_votes {
			wonky_verdict.Votes[i] = wonky_vote
		}
		s.Block.Extrinsic.Disputes.Verdict = append(s.Block.Extrinsic.Disputes.Verdict)
	} else {
		// shouldn't go here ..
		panic(0)
	}

}
