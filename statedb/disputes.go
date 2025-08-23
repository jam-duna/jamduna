package statedb

import (
	"bytes"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

const CoreNum = types.TotalCores
const ValidatorNum = types.TotalValidators
const E = types.EpochLength

// full
// const CoreNum = 341
// const ValidatorNum = 1023
// const E = 600

// ==========Output=======
type DOutput struct {
	Ok *struct {
		OffenderMark []types.Ed25519Key `json:"offenders_mark"` // SEQUENCE OF Ed25519Key (ByteArray32 in disputes.asn)
	} `json:"ok,omitempty"`
	Err *string `json:"err,omitempty" ` // ErrorCode
}

type VerdictResult struct {
	WorkReportHash common.Hash
	PositveCount   int
}

func (j *JamState) IsValidateDispute(input *types.Dispute) ([]VerdictResult, error) {
	err := checkNotPresentVerdict(input.Verdict, input.Culprit, input.Fault)
	if err != nil {
		return []VerdictResult{}, err
	}
	// gp 0.5.0 (10.3)
	for _, v := range input.Verdict {
		err = j.checkSignature(v)
		if err != nil {
			return []VerdictResult{}, err
		}
	}
	//gp 0.5.0 (10.7) v: v should index by the work report hash and no duplicates
	err = checkVerdicts(input.Verdict)
	if err != nil {
		return []VerdictResult{}, err
	}
	//gp 0.5.0 (10.8) c: should be index by key and no duplicates
	err = checkCulprit(input.Culprit)
	if err != nil {
		return []VerdictResult{}, err
	}
	//gp 0.5.0 (10.8) f: should be index by key and no duplicates
	err = checkFault(input.Fault)
	if err != nil {
		return []VerdictResult{}, err
	}
	// gp 0.5.0 (10.5)
	for _, c := range input.Culprit {
		err := j.checkIfKeyOffend(c.Key)
		if err != nil && err.Error() == "already in the offenders" {
			return []VerdictResult{}, jamerrors.ErrDCulpritAlreadyInOffenders
		} else if err != nil {
			return []VerdictResult{}, err
		}
	}
	// gp 0.5.0 (10.6)
	for _, f := range input.Fault {
		err := j.checkIfKeyOffend(f.Key)
		if err != nil && err.Error() == "already in the offenders" {
			return []VerdictResult{}, jamerrors.ErrDFaultOffenderInOffendersList
		} else if err != nil {
			return []VerdictResult{}, err
		}
	}
	//gp 0.5.0 (10.9) v: work report hash should not be in the Good/Bad/Wonky set
	err = checkWorkReportHash(input.Verdict, j.DisputesState.GoodSet, j.DisputesState.BadSet, j.DisputesState.WonkySet)
	if err != nil {
		return []VerdictResult{}, err
	}
	//gp 0.5.0 (10.10) v: the vote should be index by validator index and no duplicates
	err = checkVote(input.Verdict)
	if err != nil {
		return []VerdictResult{}, err
	}
	//process the dispute
	//gp 0.5.0 (10.11, 12) r,v (r=> report, v=> sum of votes)
	/* only have 3 cases
	zero => bad
	1/3 => wonky
	2/3+1 => good
	*/
	result := verdict2result(input.Verdict)
	//gp 0.5.0 (10.11, 12) r,v (r=> report, v=> sum of votes)
	state_prime := j.sortSet_Prime(result)
	//gp 0.5.0 (10.13)if the Verdict is good, always have a least one fault
	err = isFaultEnoughAndValid(state_prime, input.Fault)
	if err != nil {
		return []VerdictResult{}, err
	}
	//gp 0.5.0 (10.14) eq 110 if the Verdict is bad, always have at least two culprit
	err = isCulpritEnoughAndValid(state_prime, input.Culprit)
	if err != nil {
		return []VerdictResult{}, err
	}
	return result, nil
}
func (j *JamState) NeedsOffendersMarker(d *types.Dispute) bool {
	if len(d.Culprit) == 0 && len(d.Fault) == 0 {
		return false
	}
	return true
}

// func (j *JamState) GetDisputesStateBytes() ([]byte, error) {
// 	// use scale to encode the DisputeState
// 	//use json marshal to get the bytes
// 	scale_bytes, err := json.Marshal(j.DisputesState)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return scale_bytes, nil

// }

func (j *JamState) GetOffenderMark(input types.Dispute) (types.OffenderMarker, error) {
	for _, v := range input.Verdict {
		err := j.checkSignature(v)
		if err != nil {
			return types.OffenderMarker{}, err
		}
	}

	for _, c := range input.Culprit {
		err := j.checkIfKeyOffend(c.Key)
		if err != nil {
			return types.OffenderMarker{}, err
		}
	}
	//eq 102 f: the key shouldn't be in old offenders set
	for _, f := range input.Fault {
		err := j.checkIfKeyOffend(f.Key)
		if err != nil {
			return types.OffenderMarker{}, err
		}
	}
	//eq 103 v: v should index by the work report hash and no duplicates
	err := checkVerdicts(input.Verdict)
	if err != nil {
		return types.OffenderMarker{}, err
	}
	//eq 104 c: should be index by key and no duplicates
	err = checkCulprit(input.Culprit)
	if err != nil {
		return types.OffenderMarker{}, err
	}
	//eq 104 f: should be index by key and no duplicates
	err = checkFault(input.Fault)
	if err != nil {
		return types.OffenderMarker{}, err
	}
	//eq 105 v: work report hash should not be in the Good/Bad/Wonky set
	err = checkWorkReportHash(input.Verdict, j.DisputesState.GoodSet, j.DisputesState.BadSet, j.DisputesState.WonkySet)
	if err != nil {
		return types.OffenderMarker{}, err
	}
	//eq 106 v: the vote should be index by validator index and no duplicates
	err = checkVote(input.Verdict)
	if err != nil {
		return types.OffenderMarker{}, err
	}
	//process the dispute
	//eq 107, 108 r,v (r=> report, v=> sum of votes)
	/* only have 3 cases
	zero => bad
	1/3 => wonky
	2/3+1 => good
	*/
	result := verdict2result(input.Verdict)
	//eq 107, 108 r,v (r=> report, v=> sum of votes)
	state_prime := j.sortSet_Prime(result)
	//eq 109 if the Verdict is good, always have a least one fault
	err = isFaultEnoughAndValid(state_prime, input.Fault)
	if err != nil {
		return types.OffenderMarker{}, err
	}
	//eq 110 if the Verdict is bad, always have at least two culprit
	err = isCulpritEnoughAndValid(state_prime, input.Culprit)
	if err != nil {
		return types.OffenderMarker{}, err
	}
	o_out := processTypesOutput(input.Culprit, input.Fault)
	return o_out, nil
}

func (j *JamState) ProcessDispute(result []VerdictResult, c []types.Culprit, f []types.Fault) {
	j.sortSet(result)
	//eq 10.15 start: clear old report in availability_assignment ,report and timeout
	j.updateOffender(c, f)
	j.clearReportAvailabilityAssignments(result)
}

func (j *JamState) Disputes(input *types.Dispute) (types.OffenderMarker, error) {
	// Implement the function logic here
	// check the all input data are valid ,eq 98~106
	//eq 99 check the signature of the verdicts
	//eq 101 c: the key shouldn't be in old offenders set
	if !j.NeedsOffendersMarker(input) {
		return types.OffenderMarker{}, nil
	}

	o_out, err := j.GetOffenderMark(*input)
	if err != nil {
		return types.OffenderMarker{}, err
	}
	result, err := j.IsValidateDispute(input)
	if err != nil {
		return types.OffenderMarker{}, err
	}
	//state changing here
	j.ProcessDispute(result, input.Culprit, input.Fault)
	return o_out, nil
}

func (j *JamState) checkSignature(v types.Verdict) error {
	// check the signature
	if v.Epoch == j.SafroleState.Timeslot/E {
		// check the signature
		err := v.Verify(j.SafroleState.CurrValidators)
		if err != nil {
			return jamerrors.ErrDBadSignatureInVerdict
		}
	} else if v.Epoch == j.SafroleState.Timeslot/E-1 {
		err := v.Verify(j.SafroleState.PrevValidators)
		if err != nil {
			return jamerrors.ErrDBadSignatureInVerdict
		}
	} else {
		log.Trace(log.Audit, "Verdict Error: the epoch of the verdict is invalid, current epoch", "e", v.Epoch, "ts/E", j.SafroleState.Timeslot/E)
		return jamerrors.ErrDAgeTooOldInVerdicts
	}
	return nil
}

// eq 101
// gp 0.5.0 (10.6)
func (j *JamState) checkIfKeyOffend(key types.Ed25519Key) error {
	for _, k := range j.DisputesState.Offenders {
		if bytes.Equal(k.Bytes(), key.Bytes()) {
			//drop the key
			return fmt.Errorf("already in the offenders")
		}
	}
	// check if the key is in the validator set
	for _, k := range j.SafroleState.CurrValidators {
		if bytes.Equal(k.Ed25519.Bytes(), key.Bytes()) {
			return nil
		}
	}
	for _, k := range j.SafroleState.PrevValidators {
		if bytes.Equal(k.Ed25519.Bytes(), key.Bytes()) {
			return nil
		}
	}
	return fmt.Errorf("already in the Offenders")
}

// eq 103 v: v should index by the work report hash and no duplicates
// gp 0.5.0 (10.7)
func checkVerdicts(v []types.Verdict) error {

	for i, verdict := range v {
		// check duplicate
		for j, veverdict2 := range v {
			if i == j {
				continue
			}
			if bytes.Equal(veverdict2.Target.Bytes(), verdict.Target.Bytes()) {
				return jamerrors.ErrDNotSortedWorkReports
			}
		}
		// check index
		if i == 0 {
			continue
		}
		if bytes.Compare(v[i].Target.Bytes(), v[i-1].Target.Bytes()) < 0 {
			return jamerrors.ErrDNotSortedValidVerdicts
		}
	}
	return nil
}

// eq 104 c: should be index by key and no duplicates
// gp 0.5.0 (10.8)
func checkCulprit(c []types.Culprit) error {
	for i, culprit := range c {
		//check culprit signature is valid
		//verify the signature
		if !culprit.Verify() {
			return jamerrors.ErrDBadSignatureInCulprits
		}
		// check duplicate
		for j, c2 := range c {
			if i == j {
				continue
			}
			if bytes.Equal(c2.Key[:], culprit.Key[:]) {
				return jamerrors.ErrDTwoCulpritsBadVerdictNotSorted
			}
		}
		// check index
		if i == 0 {
			continue
		}
		if bytes.Compare(c[i].Key[:], c[i-1].Key[:]) < 0 {
			return jamerrors.ErrDTwoCulpritsBadVerdictNotSorted

		}
	}
	return nil
}

// eq 104 f: should be index by key and no duplicates
// gp 0.5.0 (10.8)
func checkFault(f []types.Fault) error {
	for i, fault := range f {
		//check fault signature is valid
		// jam_guarantee concat the work report hash
		//verify the signature
		if !fault.Verify() {
			return jamerrors.ErrDBadSignatureInVerdict
		}
		// check duplicate
		for j, f2 := range f {
			if i == j {
				continue
			}
			if bytes.Equal(f2.Key[:], fault.Key[:]) {
				return jamerrors.ErrDTwoFaultOffendersGoodVerdict
			}
		}
		// check index
		if i == 0 {
			continue
		}
		if bytes.Compare(f[i].Key[:], f[i-1].Key[:]) < 0 {
			return jamerrors.ErrDTwoFaultOffendersGoodVerdict
		}
	}
	return nil
}

// eq 105 v: work report hash should not be in the goodSet, badSet, wonkySet
// gp 0.5.0 (10.9)
func checkWorkReportHash(v []types.Verdict, goodSet [][]byte, badSet [][]byte, wonkySet [][]byte) error {
	for _, verdict := range v {
		if checkWorkReportHashInSet(verdict.Target.Bytes(), goodSet) {
			return jamerrors.ErrDAlreadyRecordedVerdict
		}
		if checkWorkReportHashInSet(verdict.Target.Bytes(), badSet) {
			log.Warn(log.Audit, "Verdict Error: WorkReportHash already in BadSet", "Target", verdict.Target)
			return jamerrors.ErrDAlreadyRecordedVerdict
		}
		if checkWorkReportHashInSet(verdict.Target.Bytes(), wonkySet) {
			log.Warn(log.Audit, "Verdict Error: WorkReportHash already in WonkySet", "Target", verdict.Target)
			return jamerrors.ErrDAlreadyRecordedVerdictWithFaults
		}
	}
	return nil
}
func checkWorkReportHashInSet(hash []byte, set [][]byte) bool {
	for _, h := range set {
		if bytes.Equal(h, hash) {
			return true
		}
	}
	return false
}

func checkVote(v []types.Verdict) error {
	for _, verdict := range v {
		vote_counter := 0
		for i, vote := range verdict.Votes {
			// check duplicate
			if vote.Voting {
				vote_counter++
			}
			for j, vote2 := range verdict.Votes {
				if i == j {
					continue
				}
				if vote2.Index == vote.Index {
					log.Warn(log.Audit, "checkVote", "Vote Error: duplicate index", "vote.Index", vote.Index, "j", j)
					return jamerrors.ErrDNotUniqueVotes
				}
			}
			// check index
			if i == 0 {
				continue
			}
			if vote.Index < verdict.Votes[i-1].Index {
				log.Warn(log.Audit, "Vote Error: index ordering issue", "v0", vote.Index, "v1", verdict.Votes[i-1].Index)

				return jamerrors.ErrDNotSortedWorkReports
			}
		}
		if vote_counter == 0 || vote_counter == types.ValidatorsSuperMajority || vote_counter == types.WonkyTrueThreshold {
			continue
		} else {
			log.Warn(log.Audit, "Vote Error: vote count is invalid", "vote_counter", vote_counter)
			return jamerrors.ErrDNotHomogenousJudgements
		}
	}
	return nil
}

func VerdictsToResults(v []types.Verdict) []VerdictResult {
	return verdict2result(v)
}

// process the dispute
func verdict2result(v []types.Verdict) []VerdictResult {
	var result []VerdictResult
	for _, verdict := range v {
		// count the vote
		positiveCount := 0
		for _, vote := range verdict.Votes {
			if vote.Voting {
				positiveCount++
			}
		}
		result = append(result, VerdictResult{
			WorkReportHash: verdict.Target,
			PositveCount:   positiveCount,
		})
		// for _, r := range result {
		// 	fmt.Printf("WorkReportHash: %x, PositveCount: %v\n", r.WorkReportHash, r.PositveCount)
		// }

	}

	return result
}
func (j *JamState) sortSet(VerdictResult []VerdictResult) {
	for _, v := range VerdictResult {
		if v.PositveCount == 0 {
			//bad
			j.DisputesState.BadSet = append(j.DisputesState.BadSet, v.WorkReportHash.Bytes())

		} else if v.PositveCount == ValidatorNum/3 {
			//wonky
			j.DisputesState.WonkySet = append(j.DisputesState.WonkySet, v.WorkReportHash.Bytes())
		} else if v.PositveCount == ValidatorNum*2/3+1 {
			//good
			j.DisputesState.GoodSet = append(j.DisputesState.GoodSet, v.WorkReportHash.Bytes())
		}
	}
	j.DisputesState.BadSet = sortByHash(j.DisputesState.BadSet)
	j.DisputesState.WonkySet = sortByHash(j.DisputesState.WonkySet)
	j.DisputesState.GoodSet = sortByHash(j.DisputesState.GoodSet)

}
func (j *JamState) sortSet_Prime(VerdictResult []VerdictResult) JamState {
	state_prime := JamState{}
	for _, v := range VerdictResult {
		if v.PositveCount == 0 {
			//bad
			state_prime.DisputesState.BadSet = append(state_prime.DisputesState.BadSet, v.WorkReportHash.Bytes())

		} else if v.PositveCount == ValidatorNum/3 {
			//wonky
			state_prime.DisputesState.WonkySet = append(state_prime.DisputesState.WonkySet, v.WorkReportHash.Bytes())
		} else if v.PositveCount == ValidatorNum*2/3+1 {
			//good
			state_prime.DisputesState.GoodSet = append(state_prime.DisputesState.GoodSet, v.WorkReportHash.Bytes())
		}
	}
	state_prime.DisputesState.BadSet = sortByHash(state_prime.DisputesState.BadSet)
	state_prime.DisputesState.WonkySet = sortByHash(state_prime.DisputesState.WonkySet)
	state_prime.DisputesState.GoodSet = sortByHash(state_prime.DisputesState.GoodSet)
	return state_prime
}
func (j *JamState) clearReportAvailabilityAssignments(V []VerdictResult) {

	for i := range j.AvailabilityAssignments {
		availability_assignmento := j.AvailabilityAssignments[i]
		if availability_assignmento == nil {
			continue
		}
		for _, h := range V {

			wrHash := common.ComputeHash(availability_assignmento.WorkReport.Bytes())
			if bytes.Equal(wrHash, h.WorkReportHash.Bytes()) && h.PositveCount < ValidatorNum*2/3 {
				// clear the old report
				j.AvailabilityAssignments[i] = nil

			}
		}
	}

}

func (j *JamState) updateOffender(c []types.Culprit, f []types.Fault) {
	for _, c := range c {
		j.DisputesState.Offenders = append(j.DisputesState.Offenders, c.Key)
	}
	for _, f := range f {
		j.DisputesState.Offenders = append(j.DisputesState.Offenders, f.Key)

	}
	//sort the key
	j.DisputesState.Offenders = sortByKey(j.DisputesState.Offenders)
}
func processTypesOutput(c []types.Culprit, f []types.Fault) types.OffenderMarker {
	var output2 types.OffenderMarker
	for _, culprit := range c {
		output2.OffenderKey = append(output2.OffenderKey, culprit.Key)
	}
	for _, fault := range f {
		output2.OffenderKey = append(output2.OffenderKey, fault.Key)
	}
	if output2.OffenderKey == nil {
		output2.OffenderKey = []types.Ed25519Key{}
	}
	return output2
}

func sortByHash(set [][]byte) [][]byte {
	for i := range set {
		for j := range set {
			if bytes.Compare(set[i], set[j]) < 0 {
				set[i], set[j] = set[j], set[i]
			}
		}
	}
	return set
}
func sortByKey(set []types.Ed25519Key) []types.Ed25519Key {
	for i := range set {
		for j := range set {
			if bytes.Compare(set[i][:], set[j][:]) < 0 {
				set[i], set[j] = set[j], set[i]
			}
		}
	}
	return set
}

func checkNotPresentVerdict(v []types.Verdict, c []types.Culprit, f []types.Fault) error {
	// make a map to all the target hash
	targetMap := make(map[common.Hash]struct{})
	for _, verdict := range v {
		targetMap[verdict.Target] = struct{}{}
	}
	for _, culprit := range c {
		if _, ok := targetMap[culprit.Target]; !ok {
			log.Error(log.Audit, "Culprit Error: work report hash should be in bad set", "Target", culprit.Target)
			return jamerrors.ErrDOffenderNotPresentVerdict
		}
	}
	for _, fault := range f {
		if _, ok := targetMap[fault.Target]; !ok {
			log.Error(log.Audit, "Fault Error: work report hash should be in good set", "Target", fault.Target)
			return jamerrors.ErrDOffenderNotPresentVerdict
		}
	}
	return nil
}

func isFaultEnoughAndValid(state_prime JamState, f []types.Fault) error {
	counter := 0
	for _, s := range state_prime.DisputesState.GoodSet {
		for _, f := range f {
			if bytes.Equal(s, f.Target.Bytes()) {
				counter++
			}
		}
		if counter < 1 {
			return jamerrors.ErrDMissingFaultsGoodVerdict
		}
	}
	found := false
	for _, f := range f {
		if f.Voting {
			log.Trace(log.Audit, "Fault Error: fault should be false, invalid key", "f.Key", f.Key)
			return jamerrors.ErrDAuditorMarkedOffender
		}
		for _, s := range state_prime.DisputesState.GoodSet {
			if bytes.Equal(s, f.Target.Bytes()) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("fault Error: work report hash %x should be in good set", f.Target)
		}
	}
	return nil
}

func isCulpritEnoughAndValid(state_prime JamState, c []types.Culprit) error {
	counter := 0
	for _, s := range state_prime.DisputesState.BadSet {
		for _, c := range c {
			if bytes.Equal(s, c.Target.Bytes()) {
				counter++
			}
		}
		if counter == 0 {
			return jamerrors.ErrDMissingCulpritsBadVerdict
		}
		if counter < 2 {
			log.Error(log.Audit, "Culprit Error: work report hash in BadSet should have at least two culprit", "s", s)
			return jamerrors.ErrDSingleCulpritBadVerdict
		}
	}
	found := false
	for _, c := range c {
		for _, s := range state_prime.DisputesState.BadSet {
			if bytes.Equal(s, c.Target.Bytes()) {
				found = true
			}
		}
		if !found {
			log.Error(log.Audit, "Culprit Error: work report hash should be in bad set", "Target", c.Target)

			return jamerrors.ErrDOffenderNotPresentVerdict
		}
	}
	return nil
}
