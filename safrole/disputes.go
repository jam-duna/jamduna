package safrole

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"

	//"github.com/colorfulnotion/jam/bandersnatch"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// tiny
const CoreNum = 341
const ValidatorNum = 1023
const E = 600

// full
// const CoreNum = 341
// const ValidatorNum = 1026
// const E = 600

// ==========Output=======
type DOutput struct {
	DOk *struct {
		VerdictMark  []common.Hash     `json:"verdict_mark"`  // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
		OffenderMark []types.PublicKey `json:"offender_mark"` // SEQUENCE OF Ed25519Key (ByteArray32 in disputes.asn)
	} `json:"ok"`
	Err string `json:"err"` // ErrorCode
}

type VerdictResult struct {
	WorkReportHash common.Hash
	PositveCount   int
}

type DisputeState struct {
	Psi    Psi_state         `json:"psi"`    // Disputes
	Rho    []Rho_state       `json:"rho"`    // AvailabilityAssignments
	Tau    uint32            `json:"tau"`    // EpochIndex
	Kappa  []types.Validator `json:"kappa"`  // ValidatorSet
	Lambda []types.Validator `json:"lambda"` // ValidatorSet
}

type DisputeBasicState struct {
	Psi Psi_state   `json:"psi"` // Disputes
	Rho []Rho_state `json:"rho"` // AvailabilityAssignments
}

type Psi_state struct {
	Psi_g [][]byte          `json:"psi_g"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_b [][]byte          `json:"psi_b"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_w [][]byte          `json:"psi_w"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_o []types.PublicKey `json:"psi_o"` // SEQUENCE OF Ed25519Key (ByteArray32 in disputes.asn)
}

type Rho_state struct {
	DummyWorkReport []byte `json:"dummy-work-report"`
	Timeout         uint32 `json:"timeout"`
}

func (d *DisputeState) ValidateProposedDispute(dispute *types.Dispute) error {
	return nil
}
func (d *DisputeState) NeedsVerdictsMarker(targetJCE uint32) bool {
	return false
}
func (d *DisputeState) NeedsOffendersMarker(targetJCE uint32) bool {
	return false
}
// Function to copy a State struct
func (original *DisputeState) Copy() *DisputeState {
	// Create a new instance of DisputeState
	copyState := &DisputeState{
		Psi:                          original.Psi,
		Rho:                       make([]Rho_state, len(original.Rho)),
		Tau:                       original.Tau,
		Kappa:     make([]types.Validator, len(original.Kappa)),
		Lambda:    make([]types.Validator, len(original.Lambda)),
	}

	// Copy the Rho
	//copy(copyState.Rho_state original.Rho)

	// Copy the PrevValidators slice
	for i, v := range original.Kappa {
		copyState.Kappa[i] = v 
	}

	// Copy the NextValidators slice
	for i, v := range original.Lambda {
		copyState.Lambda[i] = v // Same assumption as above
	}

	return copyState
}

//==========func==========

func (d *DisputeState) GetPsiBytes() ([]byte, error) {
	// use scale to encode the Psi_state
	//use json marshal to get the bytes
	scale_bytes, err := json.Marshal(d.Psi)
	if err != nil {
		return nil, err
	}
	return scale_bytes, nil

}
func (d *DisputeState) GetRhoBytes() ([]byte, error) {
	// use scale to encode the Rho_state
	//use json marshal to get the bytes
	scale_bytes, err := json.Marshal(d.Rho)
	if err != nil {
		return nil, err
	}
	return scale_bytes, nil
}
func (d *DisputeState) SetTau(tau uint32) error {
	// set the Tau state
	d.Tau = tau
	return nil
}
func (d *DisputeState) SetPsi(PsiBytes []byte) error {
	// set the Psi state
	//use json unmarshal to set the Psi state
	err := json.Unmarshal(PsiBytes, &d.Psi)
	if err != nil {
		return err
	}

	return nil
}
func (d *DisputeState) SetRho(RhoBytes []byte) error {
	//use json unmarshal to set the Rho state
	err := json.Unmarshal(RhoBytes, &d.Rho)
	if err != nil {
		return err
	}
	return nil
}
func (d *DisputeState) SetKappa(Kappa []types.Validator) error {
	// set the Kappa state
	d.Kappa = Kappa
	return nil
}
func (d *DisputeState) SetLambda(Lambda []types.Validator) error {
	// set the Lambda state
	d.Lambda = Lambda
	return nil
}
func (d *DisputeState) GetDisputesBasicState() DisputeBasicState {
	return DisputeBasicState{
		Psi: d.Psi,
		Rho: d.Rho,
	}
}
func (d *DisputeState) Disputes(input types.Dispute) (types.VerdictMarker, types.OffenderMarker, error) {
	// Implement the function logic here
	// check the all input data are valid ,eq 98~106
	//eq 99 check the signature of the verdicts
	//eq 101 c: the key shouldn't be in old offenders set

	for _, v := range input.Verdict {
		err := checkSignature(v, *d)
		if err != nil {
			return types.VerdictMarker{}, types.OffenderMarker{}, err
		}
	}

	for _, c := range input.Culprit {
		err := checkIfKeyOffend(c.Key, *d)
		if err != nil {
			return types.VerdictMarker{}, types.OffenderMarker{}, err
		}
	}
	//eq 102 f: the key shouldn't be in old offenders set
	for _, f := range input.Fault {
		err := checkIfKeyOffend(f.Key, *d)
		if err != nil {
			return types.VerdictMarker{}, types.OffenderMarker{}, err
		}
	}
	//eq 103 v: v should index by the work report hash and no duplicates
	err := checkVerdicts(input.Verdict)
	if err != nil {
		return types.VerdictMarker{}, types.OffenderMarker{}, err
	}
	//eq 104 c: should be index by key and no duplicates
	err = checkCulprit(input.Culprit)
	if err != nil {
		return types.VerdictMarker{}, types.OffenderMarker{}, err
	}
	//eq 104 f: should be index by key and no duplicates
	err = checkFault(input.Fault)
	if err != nil {
		return types.VerdictMarker{}, types.OffenderMarker{}, err
	}
	//eq 105 v: work report hash should not be in the psi_g, psi_b, psi_w set
	err = checkWorkReportHash(input.Verdict, d.Psi.Psi_g, d.Psi.Psi_b, d.Psi.Psi_w)
	if err != nil {
		return types.VerdictMarker{}, types.OffenderMarker{}, err
	}
	//eq 106 v: the vote should be index by validator index and no duplicates
	err = checkVote(input.Verdict)
	if err != nil {
		return types.VerdictMarker{}, types.OffenderMarker{}, err
	}

	//process the dispute
	//eq 107, 108 r,v (r=> report, v=> sum of votes)
	/* only have 3 cases
	zero => bad
	1/3 => wonky
	2/3+1 => good
	*/
	result := verdict2result(input.Verdict)
	//eq 111 clear old report in rho , dummy report and timeout
	//eq 112~115 update the psi state
	post_state, state_prime, v_out, o_out, err := processDisputeTypes(result, input.Culprit, input.Fault, *d)
	if err != nil {
		return types.VerdictMarker{}, types.OffenderMarker{}, err
	}

	//eq 109 if the Verdict is good, always have at least one fault
	err = isFaultEnoughAndValid(state_prime, input.Fault)
	if err != nil {
		return types.VerdictMarker{}, types.OffenderMarker{}, err
	}
	//eq 110 if the Verdict is bad, always have at least two culprit
	err = isCulpritEnoughAndValid(state_prime, input.Culprit)
	if err != nil {
		return types.VerdictMarker{}, types.OffenderMarker{}, err
	}
	*d = post_state
	// Create the VerdictMarker and OffenderMarker

	return v_out, o_out, nil
}
func Dispute(input types.Dispute, preState DisputeState) (DisputeState, DOutput, error) {
	// Implement the function logic here
	// check the all input data are valid ,eq 98~106
	//eq 99 check the signature of the verdicts
	//eq 101 c: the key shouldn't be in old offenders set

	for _, v := range input.Verdict {
		err := checkSignature(v, preState)
		if err != nil {
			output := DOutput{
				Err: err.Error(),
				DOk:  nil,
			}
			return preState, output, err
		}
	}

	for _, c := range input.Culprit {
		err := checkIfKeyOffend(c.Key, preState)
		if err != nil {
			output := DOutput{
				Err: err.Error(),
				DOk:  nil,
			}
			return preState, output, err
		}
	}
	//eq 102 f: the key shouldn't be in old offenders set
	for _, f := range input.Fault {
		err := checkIfKeyOffend(f.Key, preState)
		if err != nil {
			output := DOutput{
				Err: err.Error(),
				DOk:  nil,
			}
			return preState, output, err
		}
	}
	//eq 103 v: v should index by the work report hash and no duplicates
	err := checkVerdicts(input.Verdict)
	if err != nil {
		output := DOutput{
			Err: err.Error(),
			DOk:  nil,
		}
		return preState, output, err
	}
	//eq 104 c: should be index by key and no duplicates
	err = checkCulprit(input.Culprit)
	if err != nil {
		output := DOutput{
			Err: err.Error(),
			DOk:  nil,
		}
		return preState, output, err
	}
	//eq 104 f: should be index by key and no duplicates
	err = checkFault(input.Fault)
	if err != nil {
		output := DOutput{
			Err: err.Error(),
			DOk:  nil,
		}
		return preState, output, err
	}
	//eq 105 v: work report hash should not be in the psi_g, psi_b, psi_w set
	err = checkWorkReportHash(input.Verdict, preState.Psi.Psi_g, preState.Psi.Psi_b, preState.Psi.Psi_w)
	if err != nil {
		output := DOutput{
			Err: err.Error(),
			DOk:  nil,
		}
		return preState, output, err
	}
	//eq 106 v: the vote should be index by validator index and no duplicates
	err = checkVote(input.Verdict)
	if err != nil {
		output := DOutput{
			Err: err.Error(),
			DOk:  nil,
		}
		return preState, output, err
	}

	//process the dispute
	//eq 107, 108 r,v (r=> report, v=> sum of votes)
	/* only have 3 cases
	zero => bad
	1/3 => wonky
	2/3+1 => good
	*/
	result := verdict2result(input.Verdict)
	//eq 111 clear old report in rho , dummy report and timeout
	//eq 112~115 update the psi state
	post_state, state_prime, output, err := processDispute(result, input.Culprit, input.Fault, preState)
	if err != nil {
		output := DOutput{
			Err: err.Error(),
			DOk:  nil,
		}
		return preState, output, err

	}
	//eq 109 if the Verdict is good, always have at least one fault
	err = isFaultEnoughAndValid(state_prime, input.Fault)
	if err != nil {
		output := DOutput{
			Err: err.Error(),
			DOk:  nil,
		}
		return preState, output, err
	}
	//eq 110 if the Verdict is bad, always have at least two culprit
	err = isCulpritEnoughAndValid(state_prime, input.Culprit)
	if err != nil {
		output := DOutput{
			Err: err.Error(),
			DOk:  nil,
		}
		return preState, output, err
	}

	return post_state, output, err
}

// eq 99
func getPublicKey(K []types.Validator, Index uint32) types.PublicKey {
	return K[Index].Ed25519.Bytes()
}

func checkSignature(v types.Verdict, pre_state DisputeState) error {
	for i, vote := range v.Votes {
		// check the signature
		sign_message := []byte{}
		if vote.Voting {
			sign_message = append([]byte("jam_valid"), v.WorkReportHash.Bytes()...)
		} else {
			sign_message = append([]byte("jam_invalid"), v.WorkReportHash.Bytes()...)
		}
		if v.Epoch == pre_state.Tau/E {
			// check the signature

			if !ed25519.Verify(ed25519.PublicKey(getPublicKey(pre_state.Kappa, uint32(vote.Index))), sign_message, vote.Signature) {
				return fmt.Errorf("Verdict Error: the signature of the voterId %v is invalid", vote.Index)
			}

		} else if v.Epoch == pre_state.Tau/E-1 {
			// check the signature
			// to do : ask davxy , here should be sign by lambda
			if !ed25519.Verify(ed25519.PublicKey(getPublicKey(pre_state.Lambda, uint32(vote.Index))), sign_message, vote.Signature) {
				return fmt.Errorf("Verdict Error: the signature of the voterId %v in verdict %v is invalid, validator %x", vote.Index, i, ed25519.PublicKey(getPublicKey(pre_state.Lambda, uint32(vote.Index))))
			}
		} else {
			return fmt.Errorf("Verdict Error: the epoch of the verdict %v is invalid, current epoch %v", v.Epoch, pre_state.Tau/E)
		}
	}
	return nil
}

// eq 101
func checkIfKeyOffend(key types.PublicKey, pre_state DisputeState) error {
	for _, k := range pre_state.Psi.Psi_o {
		if bytes.Equal(k, key) {
			//drop the key
			return fmt.Errorf("Bad Key: the key %x shouldn't be in old offenders set", key)
		}
	}
	// check if the key is in the validator set
	for _, k := range pre_state.Kappa {
		if bytes.Equal(k.Ed25519.Bytes(), key) {
			return nil
		}
	}
	for _, k := range pre_state.Lambda {
		if bytes.Equal(k.Ed25519.Bytes(), key) {
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
			if bytes.Equal(veverdict2.WorkReportHash.Bytes(), verdict.WorkReportHash.Bytes()) {
				return fmt.Errorf("Verdict Error: duplicate WorkReportHash %v in index %v", verdict.WorkReportHash, j)
			}
		}
		// check index
		if i == 0 {
			continue
		}
		if bytes.Compare(v[i].WorkReportHash.Bytes(), v[i-1].WorkReportHash.Bytes()) < 0 {
			return fmt.Errorf("Verdict Error: WorkReportHash %x should be bigger than %x", v[i].WorkReportHash, v[i-1].WorkReportHash)
		}
	}
	return nil
}

// eq 104 c: should be index by key and no duplicates
func checkCulprit(c []types.Culprit) error {
	for i, culprit := range c {
		//check culprit signature is valid
		sign_message := append([]byte("jam_guarantee"), culprit.WorkReportHash...)
		//verify the signature
		if !ed25519.Verify(ed25519.PublicKey(culprit.Key), sign_message, culprit.Signature) {
			return fmt.Errorf("Culprit Error: the signature of the culprit %v is invalid", culprit.Key)
		}
		// check duplicate
		for j, c2 := range c {
			if i == j {
				continue
			}
			if bytes.Equal(c2.Key, culprit.Key) {
				return fmt.Errorf("Culprit Error: duplicate key %x in index %v and %v", culprit.Key, i, j)
			}
		}
		// check index
		if i == 0 {
			continue
		}
		if bytes.Compare(c[i].Key, c[i-1].Key) < 0 {
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
			sign_message = append([]byte("jam_valid"), fault.WorkReportHash...)
		} else {
			sign_message = append([]byte("jam_invalid"), fault.WorkReportHash...)
		}
		//verify the signature
		if !ed25519.Verify(ed25519.PublicKey(fault.Key), sign_message, fault.Signature) {
			return fmt.Errorf("Fault Error: the signature of the fault %v is invalid", fault.Key)
		}
		// check duplicate
		for j, f2 := range f {
			if i == j {
				continue
			}
			if bytes.Equal(f2.Key, fault.Key) {
				return fmt.Errorf("Fault Error: duplicate key %v in index %v and %v", fault.Key, i, j)
			}
		}
		// check index
		if i == 0 {
			continue
		}
		if bytes.Compare(f[i].Key, f[i-1].Key) < 0 {
			return fmt.Errorf("Fault Error: key %x should be bigger than %x", f[i].Key, f[i-1].Key)
		}
	}
	return nil
}

// eq 105 v: work report hash should not be in the psi_g, psi_b, psi_w set
func checkWorkReportHash(v []types.Verdict, psi_g [][]byte, psi_b [][]byte, psi_w [][]byte) error {
	for _, verdict := range v {
		if checkWorkReportHashInSet(verdict.WorkReportHash.Bytes(), psi_g) {
			return fmt.Errorf("Verdict Error: WorkReportHash %x already in psi_g", verdict.WorkReportHash)
		}
		if checkWorkReportHashInSet(verdict.WorkReportHash.Bytes(), psi_b) {
			return fmt.Errorf("Verdict Error: WorkReportHash %x already in psi_b", verdict.WorkReportHash)
		}
		if checkWorkReportHashInSet(verdict.WorkReportHash.Bytes(), psi_w) {
			return fmt.Errorf("Verdict Error: WorkReportHash %x already in psi_w", verdict.WorkReportHash)
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
			WorkReportHash: verdict.WorkReportHash,
			PositveCount:   positiveCount,
		})
		// for _, r := range result {
		// 	fmt.Printf("WorkReportHash: %x, PositveCount: %v\n", r.WorkReportHash, r.PositveCount)
		// }

	}

	return result
}
func sortSet(VerdictResult []VerdictResult, preState DisputeState) (DisputeState, DisputeState) {
	post_state := preState
	state_prime := DisputeState{}
	for _, v := range VerdictResult {
		if v.PositveCount == 0 {
			//bad
			post_state.Psi.Psi_b = append(post_state.Psi.Psi_b, v.WorkReportHash.Bytes())
			state_prime.Psi.Psi_b = append(state_prime.Psi.Psi_b, v.WorkReportHash.Bytes())

		} else if v.PositveCount == ValidatorNum/3 {
			//wonky
			post_state.Psi.Psi_w = append(post_state.Psi.Psi_w, v.WorkReportHash.Bytes())
			state_prime.Psi.Psi_w = append(state_prime.Psi.Psi_w, v.WorkReportHash.Bytes())
		} else if v.PositveCount == ValidatorNum*2/3+1 {
			//good
			post_state.Psi.Psi_g = append(post_state.Psi.Psi_g, v.WorkReportHash.Bytes())
			state_prime.Psi.Psi_g = append(state_prime.Psi.Psi_g, v.WorkReportHash.Bytes())

		}
	}
	post_state.Psi.Psi_b = sortByHash(post_state.Psi.Psi_b)
	post_state.Psi.Psi_w = sortByHash(post_state.Psi.Psi_w)
	post_state.Psi.Psi_g = sortByHash(post_state.Psi.Psi_g)
	state_prime.Psi.Psi_b = sortByHash(state_prime.Psi.Psi_b)
	state_prime.Psi.Psi_w = sortByHash(state_prime.Psi.Psi_w)
	state_prime.Psi.Psi_g = sortByHash(state_prime.Psi.Psi_g)
	return post_state, state_prime
}

func clearReportRho(preState DisputeState, V []VerdictResult) DisputeState {
	post_state := preState
	for i := range post_state.Rho {
		rhoo := &post_state.Rho[i]
		for _, h := range V {
			wrHash := common.ComputeHash(rhoo.DummyWorkReport)
			if bytes.Equal(wrHash, h.WorkReportHash.Bytes()) && h.PositveCount < ValidatorNum*2/3 {
				// clear the old report
				rhoo.DummyWorkReport = []byte{}
				rhoo.Timeout = 0

			}
		}
	}
	return post_state
}
func updateOffender(preState DisputeState, c []types.Culprit, f []types.Fault) DisputeState {
	post_state := preState
	for _, c := range c {
		post_state.Psi.Psi_o = append(post_state.Psi.Psi_o, c.Key)
	}
	for _, f := range f {
		post_state.Psi.Psi_o = append(post_state.Psi.Psi_o, f.Key)

	}
	//sort the key
	post_state.Psi.Psi_o = sortByKey(post_state.Psi.Psi_o)
	return post_state
}
func processOutput(VerdictResult []VerdictResult, c []types.Culprit, f []types.Fault) DOutput {
	//the Verdict mark is the Verdict PositveCount < ValidatorNum*2/3
	//the Offender mark is the c and f key
	var output DOutput
	output.DOk = &struct {
		VerdictMark  []common.Hash     `json:"verdict_mark"`
		OffenderMark []types.PublicKey `json:"offender_mark"`
	}{}

	for _, v := range VerdictResult {
		if v.PositveCount < ValidatorNum*2/3 {
			output.DOk.VerdictMark = append(output.DOk.VerdictMark, common.BytesToHash(v.WorkReportHash.Bytes()))
		}
	}
	for _, culprit := range c {
		output.DOk.OffenderMark = append(output.DOk.OffenderMark, culprit.Key)
	}
	for _, fault := range f {
		output.DOk.OffenderMark = append(output.DOk.OffenderMark, fault.Key)
	}
	if output.DOk.VerdictMark == nil {
		output.DOk.VerdictMark = []common.Hash{}
	}
	if output.DOk.OffenderMark == nil {
		output.DOk.OffenderMark = []types.PublicKey{}
	}
	return output
}
func processTypesOutput(VerdictResult []VerdictResult, c []types.Culprit, f []types.Fault) (types.VerdictMarker, types.OffenderMarker) {
	var output types.VerdictMarker
	var output2 types.OffenderMarker
	for _, v := range VerdictResult {
		if v.PositveCount < ValidatorNum*2/3 {
			output.WorkReportHash = append(output.WorkReportHash, common.BytesToHash(v.WorkReportHash.Bytes()))
		}
	}
	for _, culprit := range c {
		output2.OffenderKey = append(output2.OffenderKey, culprit.Key)
	}
	for _, fault := range f {
		output2.OffenderKey = append(output2.OffenderKey, fault.Key)
	}
	if output.WorkReportHash == nil {
		output.WorkReportHash = []common.Hash{}
	}
	if output2.OffenderKey == nil {
		output2.OffenderKey = []types.PublicKey{}
	}
	return output, output2
}
func processDispute(VerdictResult []VerdictResult, c []types.Culprit, f []types.Fault, preState DisputeState) (DisputeState, DisputeState, DOutput, error) {
	//eq 107, 108 r,v (r=> report, v=> sum of votes)
	post_state := preState
	post_state, state_prime := sortSet(VerdictResult, post_state)

	//eq 109 if the Verdict is good, always have a least one fault
	//eq 110 if the Verdict is bad, always have a least two culprit
	Output := processOutput(VerdictResult, c, f)
	//eq 111 clear old report in rho , dummy report and timeout
	post_state = updateOffender(post_state, c, f)
	post_state = clearReportRho(post_state, VerdictResult)

	return post_state, state_prime, Output, nil
}

func processDisputeTypes(VerdictResult []VerdictResult, c []types.Culprit, f []types.Fault, preState DisputeState) (DisputeState, DisputeState, types.VerdictMarker, types.OffenderMarker, error) {
	//eq 107, 108 r,v (r=> report, v=> sum of votes)
	post_state := preState
	post_state, state_prime := sortSet(VerdictResult, post_state)

	//eq 109 if the Verdict is good, always have a least one fault
	//eq 110 if the Verdict is bad, always have a least two culprit
	v_out, o_out := processTypesOutput(VerdictResult, c, f)
	//eq 111 clear old report in rho , dummy report and timeout
	post_state = updateOffender(post_state, c, f)
	post_state = clearReportRho(post_state, VerdictResult)

	return post_state, state_prime, v_out, o_out, nil
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
func sortByKey(set []types.PublicKey) []types.PublicKey {
	for i := range set {
		for j := range set {
			if bytes.Compare(set[i], set[j]) < 0 {
				set[i], set[j] = set[j], set[i]
			}
		}
	}
	return set
}
func isFaultEnoughAndValid(state_prime DisputeState, f []types.Fault) error {
	counter := 0
	for _, s := range state_prime.Psi.Psi_g {
		for _, f := range f {
			if bytes.Equal(s, f.WorkReportHash) {
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
		for _, s := range state_prime.Psi.Psi_g {
			if bytes.Equal(s, f.WorkReportHash) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("Fault Error: work report hash %x should be in good set", f.WorkReportHash)
		}
	}
	return nil
}
func isCulpritEnoughAndValid(state_prime DisputeState, c []types.Culprit) error {
	counter := 0
	for _, s := range state_prime.Psi.Psi_b {
		for _, c := range c {
			if bytes.Equal(s, c.WorkReportHash) {
				counter++
			}
		}

		if counter < 2 {
			return fmt.Errorf("Culprit Error: work report hash %x in psi_b should have at least two culprit", s)
		}
	}
	found := false
	for _, c := range c {
		for _, s := range state_prime.Psi.Psi_b {
			if bytes.Equal(s, c.WorkReportHash) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("Culprit Error: work report hash %x should be in bad set", c.WorkReportHash)
		}
	}
	return nil
}
