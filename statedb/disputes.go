package statedb

import (
	"bytes"
	"crypto/ed25519"
	"fmt"

	//"github.com/colorfulnotion/jam/bandersnatch"

	"github.com/colorfulnotion/jam/common"
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
	DOk *struct {
		OffenderMark []types.Ed25519Key `json:"offender_mark"` // SEQUENCE OF Ed25519Key (ByteArray32 in disputes.asn)
	} `json:"ok"`
	Err string `json:"err"` // ErrorCode
}

type VerdictResult struct {
	WorkReportHash common.Hash
	PositveCount   int
}

func (j *JamState) IsValidateDispute(input *types.Dispute) ([]VerdictResult, error) {
	for _, v := range input.Verdict {
		err := j.checkSignature(v)
		if err != nil {
			return []VerdictResult{}, err
		}
	}

	for _, c := range input.Culprit {
		err := j.checkIfKeyOffend(c.Key)
		if err != nil {
			return []VerdictResult{}, err
		}
	}
	//eq 102 f: the key shouldn't be in old offenders set
	for _, f := range input.Fault {
		err := j.checkIfKeyOffend(f.Key)
		if err != nil {
			return []VerdictResult{}, err
		}
	}
	//eq 103 v: v should index by the work report hash and no duplicates
	err := checkVerdicts(input.Verdict)
	if err != nil {
		return []VerdictResult{}, err
	}
	//eq 104 c: should be index by key and no duplicates
	err = checkCulprit(input.Culprit)
	if err != nil {
		return []VerdictResult{}, err
	}
	//eq 104 f: should be index by key and no duplicates
	err = checkFault(input.Fault)
	if err != nil {
		return []VerdictResult{}, err
	}
	//eq 105 v: work report hash should not be in the psi_g, psi_b, psi_w set
	err = checkWorkReportHash(input.Verdict, j.DisputesState.Psi_g, j.DisputesState.Psi_b, j.DisputesState.Psi_w)
	if err != nil {
		return []VerdictResult{}, err
	}
	//eq 106 v: the vote should be index by validator index and no duplicates
	err = checkVote(input.Verdict)
	if err != nil {
		return []VerdictResult{}, err
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
		return []VerdictResult{}, err
	}
	//eq 110 if the Verdict is bad, always have at least two culprit
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
	//eq 111 clear old report in rho , dummy report and timeout
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
func getPublicKey(K []types.Validator, Index uint32) types.Ed25519Key {
	return K[Index].Ed25519
}

func (j *JamState) checkSignature(v types.Verdict) error {
	for i, vote := range v.Votes {
		// check the signature
		sign_message := []byte{}
		if vote.Voting {
			sign_message = append([]byte(types.X_True), v.Target.Bytes()...)
		} else {
			sign_message = append([]byte(types.X_False), v.Target.Bytes()...)
		}
		if v.Epoch == j.SafroleState.Timeslot/E {
			// check the signature

			if !ed25519.Verify(ed25519.PublicKey(getPublicKey(j.SafroleState.CurrValidators, uint32(vote.Index)).Bytes()), sign_message, vote.Signature.Bytes()) {
				return fmt.Errorf("Verdict Error: the signature of the voterId %v is invalid", vote.Index)
			}

		} else if v.Epoch == j.SafroleState.Timeslot/E-1 {
			// check the signature
			// to do : ask davxy , here should be sign by lambda
			if !ed25519.Verify(ed25519.PublicKey(getPublicKey(j.SafroleState.PrevValidators, uint32(vote.Index)).Bytes()), sign_message, vote.Signature.Bytes()) {
				return fmt.Errorf("Verdict Error: the signature of the voterId %v in verdict %v is invalid, validator %x", vote.Index, i, ed25519.PublicKey(getPublicKey(j.SafroleState.PrevValidators, uint32(vote.Index)).Bytes()))
			}
		} else {
			return fmt.Errorf("Verdict Error: the epoch of the verdict %v is invalid, current epoch %v", v.Epoch, j.SafroleState.Timeslot/E)
		}
	}
	return nil
}

// eq 101
func (j *JamState) checkIfKeyOffend(key types.Ed25519Key) error {
	for _, k := range j.DisputesState.Psi_o {
		if bytes.Equal(k.Bytes(), key.Bytes()) {
			//drop the key
			return fmt.Errorf("Bad Key: the key %x shouldn't be in old offenders set", key)
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
	return fmt.Errorf("Bad Key: the key %v shouldn't be in old offenders set", key)
}

// eq 103 v: v should index by the work report hash and no duplicates
func checkVerdicts(v []types.Verdict) error {

	for i, verdict := range v {
		// check duplicate
		for j, veverdict2 := range v {
			if i == j {
				continue
			}
			if bytes.Equal(veverdict2.Target.Bytes(), verdict.Target.Bytes()) {
				return fmt.Errorf("Verdict Error: duplicate WorkReportHash %v in index %v", verdict.Target, j)
			}
		}
		// check index
		if i == 0 {
			continue
		}
		if bytes.Compare(v[i].Target.Bytes(), v[i-1].Target.Bytes()) < 0 {
			return fmt.Errorf("Verdict Error: WorkReportHash %x should be bigger than %x", v[i].Target, v[i-1].Target)
		}
	}
	return nil
}

// eq 104 c: should be index by key and no duplicates
func checkCulprit(c []types.Culprit) error {
	for i, culprit := range c {
		//check culprit signature is valid
		sign_message := append([]byte(types.X_G), culprit.Target.Bytes()...)
		//verify the signature
		if !ed25519.Verify(ed25519.PublicKey(culprit.Key[:]), sign_message, culprit.Signature[:]) {
			return fmt.Errorf("Culprit Error: the signature of the culprit %v is invalid", culprit.Key)
		}
		// check duplicate
		for j, c2 := range c {
			if i == j {
				continue
			}
			if bytes.Equal(c2.Key[:], culprit.Key[:]) {
				return fmt.Errorf("Culprit Error: duplicate key %x in index %v and %v", culprit.Key, i, j)
			}
		}
		// check index
		if i == 0 {
			continue
		}
		if bytes.Compare(c[i].Key[:], c[i-1].Key[:]) < 0 {
			return fmt.Errorf("Culprit Error: key %x should be bigger than %x", c[i].Key, c[i-1].Key)
		}
	}
	return nil
}

// eq 104 f: should be index by key and no duplicates
func checkFault(f []types.Fault) error {
	for i, fault := range f {
		//check fault signature is valid
		// jam_guarantee concat the work report hash
		sign_message := []byte{}
		if fault.Voting {
			sign_message = append([]byte(types.X_True), fault.Target.Bytes()...)
		} else {
			sign_message = append([]byte(types.X_False), fault.Target.Bytes()...)
		}
		//verify the signature
		if !ed25519.Verify(ed25519.PublicKey(fault.Key[:]), sign_message, fault.Signature[:]) {
			return fmt.Errorf("Fault Error: the signature of the fault %v is invalid", fault.Key)
		}
		// check duplicate
		for j, f2 := range f {
			if i == j {
				continue
			}
			if bytes.Equal(f2.Key[:], fault.Key[:]) {
				return fmt.Errorf("Fault Error: duplicate key %v in index %v and %v", fault.Key, i, j)
			}
		}
		// check index
		if i == 0 {
			continue
		}
		if bytes.Compare(f[i].Key[:], f[i-1].Key[:]) < 0 {
			return fmt.Errorf("Fault Error: key %x should be bigger than %x", f[i].Key, f[i-1].Key)
		}
	}
	return nil
}

// eq 105 v: work report hash should not be in the psi_g, psi_b, psi_w set
func checkWorkReportHash(v []types.Verdict, psi_g [][]byte, psi_b [][]byte, psi_w [][]byte) error {
	for _, verdict := range v {
		if checkWorkReportHashInSet(verdict.Target.Bytes(), psi_g) {
			return fmt.Errorf("Verdict Error: WorkReportHash %x already in psi_g", verdict.Target)
		}
		if checkWorkReportHashInSet(verdict.Target.Bytes(), psi_b) {
			return fmt.Errorf("Verdict Error: WorkReportHash %x already in psi_b", verdict.Target)
		}
		if checkWorkReportHashInSet(verdict.Target.Bytes(), psi_w) {
			return fmt.Errorf("Verdict Error: WorkReportHash %x already in psi_w", verdict.Target)
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
		for i, vote := range verdict.Votes {
			// check duplicate
			for j, vote2 := range verdict.Votes {
				if i == j {
					continue
				}
				if vote2.Index == vote.Index {
					return fmt.Errorf("Vote Error: duplicate index %v in index %v", vote.Index, j)
				}
			}
			// check index
			if i == 0 {
				continue
			}
			if vote.Index < verdict.Votes[i-1].Index {
				return fmt.Errorf("Vote Error: index %v should be bigger than %v", vote.Index, verdict.Votes[i-1].Index)
			}
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
			return fmt.Errorf("Fault Error: psi_g should have at least one fault work report hash")
		}
	}
	found := false
	for _, f := range f {
		if f.Voting {
			return fmt.Errorf("Fault Error: fault should be false, invalid key: %x", f.Key)
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

		if counter < 2 {
			return fmt.Errorf("Culprit Error: work report hash %x in psi_b should have at least two culprit", s)
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
			return fmt.Errorf("Culprit Error: work report hash %x should be in bad set", c.Target)
		}
	}
	return nil
}
