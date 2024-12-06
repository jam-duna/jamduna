package statedb

import (
	"bytes"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
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

/*
ErrDNotSortedWorkReports	No - Shawn to review	Not sorted work reports within a verdict
ErrDNotUniqueVotes	No - Shawn to review	Not unique votes within a verdict
ErrDNotSortedValidVerdicts	Yes	Not sorted, valid verdicts
ErrDNotHomogenousJudgements	No - Shawn to review	Not homogeneous judgements, but positive votes count is not correct
ErrDMissingCulpritsBadVerdict	No - Shawn to review	Missing culprits for bad verdict
ErrDSingleCulpritBadVerdict	No - Shawn to review	Single culprit for bad verdict
ErrDTwoCulpritsBadVerdictNotSorted	Yes	Two culprits for bad verdict, not sorted
ErrDAlreadyRecordedVerdict	No - Shawn to review	Report an already recorded verdict, with culprits
ErrDCulpritAlreadyInOffenders	No - Shawn to review	Culprit offender already in the offenders list
ErrDOffenderNotPresentVerdict	No - Shawn to review	Offender relative to a not present verdict
ErrDMissingFaultsGoodVerdict	No - Shawn to review	Missing faults for good verdict
ErrDTwoFaultOffendersGoodVerdict	No - Shawn to review	Two fault offenders for a good verdict, not sorted
ErrDAlreadyRecordedVerdictWithFaults	No - Shawn to review	Report an already recorded verdict, with faults
ErrDFaultOffenderInOffendersList	No - Shawn to review	Fault offender already in the offenders list
ErrDAuditorMarkedOffender	No - Shawn to review	Auditor marked as offender, but vote matches the verdict.
ErrDBadSignatureInVerdict	Yes	Bad signature within the verdict judgements
ErrDBadSignatureInCulprits	Yes	Bad signature within the culprits sequence
ErrDAgeTooOldInVerdicts	No - Shawn to review	Age too old for verdicts judgements
*/
type VerdictResult struct {
	WorkReportHash common.Hash
	PositveCount   int
}

func (j *JamState) IsValidateDispute(input *types.Dispute) ([]VerdictResult, error) {
	// gp 0.5.0 (10.3)
	for _, v := range input.Verdict {
		err := j.checkSignature(v)
		if err != nil {
			return []VerdictResult{}, err
		}
	}
	// gp 0.5.0 (10.5)
	for _, c := range input.Culprit {
		err := j.checkIfKeyOffend(c.Key)
		if err != nil && err.Error() == "Already in the Offenders" {
			return []VerdictResult{}, jamerrors.ErrDCulpritAlreadyInOffenders
		} else if err != nil {
			return []VerdictResult{}, err
		}
	}
	// gp 0.5.0 (10.6)
	for _, f := range input.Fault {
		err := j.checkIfKeyOffend(f.Key)
		if err != nil && err.Error() == "Already in the Offenders" {
			return []VerdictResult{}, jamerrors.ErrDFaultOffenderInOffendersList
		} else if err != nil {
			return []VerdictResult{}, err
		}
	}
	//gp 0.5.0 (10.7) v: v should index by the work report hash and no duplicates
	err := checkVerdicts(input.Verdict)
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
	//gp 0.5.0 (10.9) v: work report hash should not be in the psi_g, psi_b, psi_w set
	err = checkWorkReportHash(input.Verdict, j.DisputesState.Psi_g, j.DisputesState.Psi_b, j.DisputesState.Psi_w)
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

// func (j *JamState) GetPsiBytes() ([]byte, error) {
// 	// use scale to encode the Psi_state
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
	//eq 105 v: work report hash should not be in the psi_g, psi_b, psi_w set
	err = checkWorkReportHash(input.Verdict, j.DisputesState.Psi_g, j.DisputesState.Psi_b, j.DisputesState.Psi_w)
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
	o_out := processTypesOutput(result, input.Culprit, input.Fault)
	return o_out, nil
}

func (j *JamState) ProcessDispute(result []VerdictResult, c []types.Culprit, f []types.Fault) {
	j.sortSet(result)
	//eq 10.15 start: clear old report in rho ,report and timeout
	j.updateOffender(c, f)
	j.clearReportRho(result)
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

// eq 99
// gp 0.5.0 (10.4)
func getPublicKey(K []types.Validator, Index uint32) types.Ed25519Key {
	return K[Index].Ed25519
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
		fmt.Printf("Verdict Error: the epoch of the verdict %v is invalid, current epoch %v\n", v.Epoch, j.SafroleState.Timeslot/E)
		return jamerrors.ErrDAgeTooOldInVerdicts
	}
	return nil
}

// eq 101
// gp 0.5.0 (10.6)
func (j *JamState) checkIfKeyOffend(key types.Ed25519Key) error {
	for _, k := range j.DisputesState.Psi_o {
		if bytes.Equal(k.Bytes(), key.Bytes()) {
			//drop the key
			return fmt.Errorf("Already in the Offenders")
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
	return fmt.Errorf("Already in the Offenders")
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

// eq 105 v: work report hash should not be in the psi_g, psi_b, psi_w set
// gp 0.5.0 (10.9)
func checkWorkReportHash(v []types.Verdict, psi_g [][]byte, psi_b [][]byte, psi_w [][]byte) error {
	for _, verdict := range v {
		if checkWorkReportHashInSet(verdict.Target.Bytes(), psi_g) {
			return jamerrors.ErrDAlreadyRecordedVerdict
		}
		if checkWorkReportHashInSet(verdict.Target.Bytes(), psi_b) {
			fmt.Printf("Verdict Error: WorkReportHash %x already in psi_b\n", verdict.Target)
			return jamerrors.ErrDAlreadyRecordedVerdict
		}
		if checkWorkReportHashInSet(verdict.Target.Bytes(), psi_w) {
			fmt.Printf("Verdict Error: WorkReportHash %x already in psi_w\n", verdict.Target)
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
					fmt.Printf("Vote Error: duplicate index %v in index %v\n", vote.Index, j)
					return jamerrors.ErrDNotUniqueVotes
				}
			}
			// check index
			if i == 0 {
				continue
			}
			if vote.Index < verdict.Votes[i-1].Index {
				fmt.Printf("Vote Error: index %v should be bigger than %v\n", vote.Index, verdict.Votes[i-1].Index)
				return jamerrors.ErrDNotSortedWorkReports
			}
		}
		if vote_counter == 0 || vote_counter == types.ValidatorsSuperMajority || vote_counter == types.WonkyTrueThreshold {
			continue
		} else {
			fmt.Printf("Vote Error: vote count %v is invalid\n", vote_counter)
			return jamerrors.ErrDNotHomogenousJudgements
		}
	}
	return nil
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
			j.DisputesState.Psi_b = append(j.DisputesState.Psi_b, v.WorkReportHash.Bytes())

		} else if v.PositveCount == ValidatorNum/3 {
			//wonky
			j.DisputesState.Psi_w = append(j.DisputesState.Psi_w, v.WorkReportHash.Bytes())
		} else if v.PositveCount == ValidatorNum*2/3+1 {
			//good
			j.DisputesState.Psi_g = append(j.DisputesState.Psi_g, v.WorkReportHash.Bytes())
		}
	}
	j.DisputesState.Psi_b = sortByHash(j.DisputesState.Psi_b)
	j.DisputesState.Psi_w = sortByHash(j.DisputesState.Psi_w)
	j.DisputesState.Psi_g = sortByHash(j.DisputesState.Psi_g)

}
func (j *JamState) sortSet_Prime(VerdictResult []VerdictResult) JamState {
	state_prime := JamState{}
	for _, v := range VerdictResult {
		if v.PositveCount == 0 {
			//bad
			state_prime.DisputesState.Psi_b = append(state_prime.DisputesState.Psi_b, v.WorkReportHash.Bytes())

		} else if v.PositveCount == ValidatorNum/3 {
			//wonky
			state_prime.DisputesState.Psi_w = append(state_prime.DisputesState.Psi_w, v.WorkReportHash.Bytes())
		} else if v.PositveCount == ValidatorNum*2/3+1 {
			//good
			state_prime.DisputesState.Psi_g = append(state_prime.DisputesState.Psi_g, v.WorkReportHash.Bytes())
		}
	}
	state_prime.DisputesState.Psi_b = sortByHash(state_prime.DisputesState.Psi_b)
	state_prime.DisputesState.Psi_w = sortByHash(state_prime.DisputesState.Psi_w)
	state_prime.DisputesState.Psi_g = sortByHash(state_prime.DisputesState.Psi_g)
	return state_prime
}
func (j *JamState) clearReportRho(V []VerdictResult) {

	for i := range j.AvailabilityAssignments {
		rhoo := j.AvailabilityAssignments[i]
		if rhoo == nil {
			continue
		}
		for _, h := range V {

			wrHash := common.ComputeHash(rhoo.WorkReport.Bytes())
			if bytes.Equal(wrHash, h.WorkReportHash.Bytes()) && h.PositveCount < ValidatorNum*2/3 {
				// clear the old report
				j.AvailabilityAssignments[i] = nil

			}
		}
	}

}

func (j *JamState) updateOffender(c []types.Culprit, f []types.Fault) {
	for _, c := range c {
		j.DisputesState.Psi_o = append(j.DisputesState.Psi_o, c.Key)
	}
	for _, f := range f {
		j.DisputesState.Psi_o = append(j.DisputesState.Psi_o, f.Key)

	}
	//sort the key
	j.DisputesState.Psi_o = sortByKey(j.DisputesState.Psi_o)
}
func processTypesOutput(VerdictResult []VerdictResult, c []types.Culprit, f []types.Fault) types.OffenderMarker {
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
func isFaultEnoughAndValid(state_prime JamState, f []types.Fault) error {
	counter := 0
	for _, s := range state_prime.DisputesState.Psi_g {
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
			fmt.Printf("Fault Error: fault should be false, invalid key: %v\n", f.Key)
			return jamerrors.ErrDAuditorMarkedOffender
		}
		for _, s := range state_prime.DisputesState.Psi_g {
			if bytes.Equal(s, f.Target.Bytes()) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("Fault Error: work report hash %x should be in good set", f.Target)
		}
	}
	return nil
}
func isCulpritEnoughAndValid(state_prime JamState, c []types.Culprit) error {
	counter := 0
	for _, s := range state_prime.DisputesState.Psi_b {
		for _, c := range c {
			if bytes.Equal(s, c.Target.Bytes()) {
				counter++
			}
		}
		if counter == 0 {
			return jamerrors.ErrDMissingCulpritsBadVerdict
		}
		if counter < 2 {
			fmt.Printf("Culprit Error: work report hash %x in psi_b should have at least two culprit\n", s)
			return jamerrors.ErrDSingleCulpritBadVerdict
		}
	}
	found := false
	for _, c := range c {
		for _, s := range state_prime.DisputesState.Psi_b {
			if bytes.Equal(s, c.Target.Bytes()) {
				found = true
			}
		}
		if !found {
			fmt.Printf("Culprit Error: work report hash %x should be in bad set\n", c.Target)
			return jamerrors.ErrDOffenderNotPresentVerdict
		}
	}
	return nil
}
