package statedb

import (
	"errors"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/types"
)

type BeefyPool []Beta_state
type BeefyCommitment struct {
	Service    uint32      `json:"service"`
	Commitment common.Hash `json:"commitment"`
}

type Peaks []*common.Hash
type MMR struct {
	Peaks Peaks `json:"peaks"`
}

// eq.161 - W^!
func AccumulatedImmediately(W []types.WorkReport) []types.WorkReport {
	outputWorkReports := []types.WorkReport{}
	for _, workReport := range W {
		if workReport.RefineContext.Prerequisite == nil && len(workReport.SegmentRootLookup) == 0 {
			outputWorkReports = append(outputWorkReports, workReport)
		}
	}
	return outputWorkReports
}

// eq.162 - W^Q
func (j *JamState) QueuedExecution(W []types.WorkReport) []types.AccumulationQueue {
	outputWorkReports := []types.WorkReport{}
	for _, workReport := range W {
		if workReport.RefineContext.Prerequisite != nil || len(workReport.SegmentRootLookup) != 0 {
			outputWorkReports = append(outputWorkReports, workReport)
		}
	}
	accumulatedcup := map[common.Hash]common.Hash{}
	//	accumulated := j.AccumulationHistory
	/*	for _, a := range accumulated {
			for key, value := range a {
				accumulatedcup[key] = value
			}
		}
		type AccumulationHistory struct {
			WorkPackageHash []common.Hash `json:"work_package_hash"`
		}
	*/

	depandancy := []types.AccumulationQueue{}
	for _, workReport := range outputWorkReports {
		depandancy = append(depandancy, Depandancy(workReport))
	}
	return QueueEditing(depandancy, accumulatedcup)
}

// eq.163 - D(w)
func Depandancy(w types.WorkReport) types.AccumulationQueue {
	unionMap := make(map[common.Hash]bool)
	if w.RefineContext.Prerequisite != nil {
		//prerequisite := *(w.RefineContext.Prerequisite)
		//prerequisiteHash = common.Hash{} // (prerequisite)
		//unionMap[prerequisiteHash] = true
	}
	if w.SegmentRootLookup != nil {
		for k := range w.SegmentRootLookup {
			unionMap[k] = true
		}
	}
	unionResult := []common.Hash{}
	for key := range unionMap {
		unionResult = append(unionResult, key)
	}

	result := types.AccumulationQueue{

		WorkPackageHash: unionResult,
	}
	return result
}

// In eq.164 - union compare
func union(a map[common.Hash]common.Hash, b map[common.Hash]common.Hash) map[common.Hash]common.Hash {
	result := make(map[common.Hash]common.Hash)
	for key, value := range a {
		result[key] = value
	}
	for key, value := range b {
		result[key] = value
	}
	return result
}
func isEqualSet(a, b map[common.Hash]common.Hash) bool {
	if len(a) != len(b) {
		return false
	}

	for key, valueA := range a {
		valueB, exists := b[key]
		if !exists || valueA != valueB {
			return false
		}
	}
	return true
}

// eq.164 - function E
func QueueEditing(r []types.AccumulationQueue, x map[common.Hash]common.Hash) []types.AccumulationQueue {
	var result []types.AccumulationQueue
	for _, item := range r {
		/*		if _, exists := x[item.WorkReport.AvailabilitySpec.WorkPackageHash]; exists {
					continue
				}
				segmentRootLookup := item.WorkReport.SegmentRootLookup
				if segmentRootLookup == nil {
					segmentRootLookup = make(map[common.Hash]common.Hash)
				}
				union1 := union(x, segmentRootLookup)
				union2 := union(segmentRootLookup, x)
				if !isEqualSet(union1, union2) {
					continue
				}*/
		result = append(result, item)
	}
	return result
}

// eq.165 - function Q
func PriorityQueue(r []types.AccumulationQueue, a map[common.Hash]common.Hash) []types.WorkReport {
	if a == nil {
		a = make(map[common.Hash]common.Hash)
	}
	g := []types.WorkReport{}
	for _, item := range r {
		if len(item.WorkPackageHash) == 0 {
			//		g = append(g, item.WorkReport)
		}
	}
	if len(g) == 0 {
		return []types.WorkReport{}
	} else {
		result := g
		// a ∪ P(g)
		result = append(result, PriorityQueue(QueueEditing(r, Mapping(g)), union(a, Mapping(g)))...)
		return result
	}
}

// eq.166 - function P
func Mapping(w []types.WorkReport) map[common.Hash]common.Hash {
	result := make(map[common.Hash]common.Hash)
	for _, workReport := range w {
		result[workReport.AvailabilitySpec.WorkPackageHash] = workReport.AvailabilitySpec.ExportedSegmentRoot
	}
	return result
}

// eq.167 - m
func (s *StateDB) CurrentEpochSlot() uint32 {
	Ht := s.Block.Header.Slot
	return Ht % types.EpochLength
}

// eq.168 - W^*
func (s *StateDB) AccumulatableSequence(W []types.WorkReport) []types.WorkReport {
	accumulated_immediately := AccumulatedImmediately(W)
	result := accumulated_immediately
	accumulatedcup := map[common.Hash]common.Hash{}
	j := s.GetJamState()
	//accumulated := j.AccumulationHistory
	/*for _, a := range accumulated {
		for key, value := range a {
			accumulatedcup[key] = value
		}
	}*/
	queued_execution := j.QueuedExecution(W)
	result = append(result, PriorityQueue(s.ComputeReadyQueue(queued_execution, accumulated_immediately), union(accumulatedcup, Mapping(accumulated_immediately)))...)
	return result
}

// eq.169 - q
func (s *StateDB) ComputeReadyQueue(queued_execution []types.AccumulationQueue, accumulated_immediately []types.WorkReport) []types.AccumulationQueue {
	j := s.GetJamState()
	ready_state := j.AccumulationQueue
	m := s.CurrentEpochSlot()
	ready := []types.AccumulationQueue{}
	for i := m; i < types.EpochLength; i++ {
		ready = append(ready, ready_state[i]...)
	}
	for i := 0; i < int(m); i++ {
		ready = append(ready, ready_state[i]...)
	}
	return QueueEditing(append(ready, queued_execution...), Mapping(accumulated_immediately))
}

// CalculateGasAttributable calculates the gas attributable for each service.
func CalculateGasAttributable(
	workReports map[int][]types.WorkReport, // workReports maps service index to their respective work reports
	privilegedServices []int, // privilegedServices contains indices of privileged services
	electiveAccumulationGas float64, // electiveAccumulationGas represents GA in the formula
) []types.GasAttributable {
	gasAttributable := []types.GasAttributable{}
	serviceGasTotals := make(map[int]float64)

	for serviceIndex, reports := range workReports {
		var gasSum float64
		for range reports {
			gasSum += 1 // TODO
		}
		serviceGasTotals[serviceIndex] = gasSum
	}

	for _, serviceIndex := range privilegedServices {
		if _, exists := serviceGasTotals[serviceIndex]; !exists {
			serviceGasTotals[serviceIndex] = 0
		}
	}

	var totalGasSum float64
	for _, gas := range serviceGasTotals {
		totalGasSum += gas
	}

	for serviceIndex, gasSum := range serviceGasTotals {
		gas := gasSum + electiveAccumulationGas*(1-(gasSum/totalGasSum))
		gasAttributable = append(gasAttributable, types.GasAttributable{
			ServiceIndex: serviceIndex,
			Gas:          gas,
		})
	}

	return gasAttributable
}

/*
We define the outer accumulation function ∆+ which
transforms a gas-limit, a sequence of work-reports, an
initial partial-state and a dictionary of services enjoying
free accumulation, into a tuple of the number of work-
results accumulated, a posterior state-context, the resul-
tant deferred-transfers and accumulation-output pairings:
*/
// eq 173
// ∆+
func (s *StateDB) OuterAccumulate(g uint64, w []types.WorkReport, o *types.PartialState, f map[uint32]uint32) (gas uint64, output_t []types.DeferredTransfer, output_b []BeefyCommitment) {
	// not really sure i here , the max meaning. use uint32 for now
	var gas_tmp uint64
	i := uint64(0)
	// calculate how to maximize the work reports to enter the parallelized accumulation
	for _, workReport := range w {
		for _, workResult := range workReport.Results {
			gas_tmp += workResult.GasRatio // the gas ratio here maybe need to check again, the name is not clear
			if gas_tmp <= g {
				i++
				if i >= uint64(len(w))+1 {
					break
				}
			}

		}
	}
	if i == 0 { // if i = 0, then nothing to do
		gas = 0

		output_t = make([]types.DeferredTransfer, 0)
		output_b = make([]BeefyCommitment, 0)
		return
	}
	//fmt.Printf("ParallelizedAccumulate i=%d/%d\n", i, len(w))
	g_star, t_star, b_star := s.ParallelizedAccumulate(o, w[0:i], f) // parallelized accumulation the 0 to i work reports
	if i >= uint64(len(w)) {                                         // no more reports
		return g_star, t_star, b_star
	}
	j, outputT, outputB := s.OuterAccumulate(g-g_star, w[i+1:], o, nil) // recursive call to the rest of the work reports
	gas = i + j

	output_t = append(outputT, t_star...)
	for _, b := range b_star {
		duplicate := false
		for _, existingB := range outputB {
			if existingB.Service == b.Service && existingB.Commitment == b.Commitment {
				duplicate = true
				break
			}
		}
		if !duplicate {
			output_b = append(output_b, b)
		}
	}

	return
}

/*
the parallelized accumulation function ∆∗ which, with the help of the single-service accumulation function ∆1, transforms an initial state-context, together with a sequence of work-reports and a dictionary of
privileged always-accumulate services, into a tuple of the total gas utilized in pvm execution u, a posterior statecontext (x′,d′,i′,q′) and the resultant accumulationoutput pairings b and deferred-transfers Ì
*/
// the parallelized accumulation function ∆*
// eq 174
func (s *StateDB) ParallelizedAccumulate(o *types.PartialState, w []types.WorkReport, f map[uint32]uint32) (output_u uint64, output_t []types.DeferredTransfer, output_b []BeefyCommitment) {
	services := make([]uint32, 0)
	for _, workReport := range w {
		for _, workResult := range workReport.Results {
			services = append(services, workResult.Service)
		}
	}

	//fmt.Printf(" ParallelizedAccumulate - services %v\n", services)
	output_u = 0
	// get services from f key
	for k := range f {
		services = append(services, k)
	}
	for _, service := range services {
		// this is parallelizable
		T, B, U := s.SingleAccumulate(o, w, f, service)
		output_u += U
		output_b = append(output_b, BeefyCommitment{
			Service:    service,
			Commitment: B,
		})
		output_t = append(output_t, T...)
	}

	// s ∈ K(d) ∖ s
	/*s.SingleAccumulate(o, w, f, o.PrivilegedState.Kai_m)
	s.SingleAccumulate(o, w, f, o.PrivilegedState.Kai_a)
	s.SingleAccumulate(o, w, f, o.PrivilegedState.Kai_v) */

	return
}

func (sdb *StateDB) Check(i uint32) uint32 {
	const lowerLimit uint32 = 1 << 8               // 2^8 = 256
	const upperLimit uint32 = (1 << 32) - (1 << 9) // 2^32 - 2^9 = 4294966784

	// Base case: return i if it is not in the set K(delta)
	// if !vm.k_exist(i) {
	// 	return i
	// }
	// // Correct handling of the adjustment to prevent negative values:
	// // Convert the expression to int64 to handle potential negatives safely.
	// adjusted := int64(i) - int64(lowerLimit) + 1

	// // Ensure the adjusted value is non-negative and within the valid range:
	// // modResult := ((adjusted % int64(upperLimit)) + int64(upperLimit)) % int64(upperLimit)
	// modResult := (adjusted % int64(upperLimit)) + int64(lowerLimit)

	// // Return check with the adjusted result, adding lowerLimit back to align the range
	// // (i−2^8 +1)mod(2^32 −2^9)+2^8
	// // return vm.check(lowerLimit + uint32(modResult))
	// return vm.check(uint32(modResult))
	for sdb.k_exist(i) {
		adjusted := int64(i) - int64(lowerLimit) + 1
		modResult := (adjusted % int64(upperLimit)) + int64(lowerLimit)
		i = uint32(modResult)
	}

	return i
}

func (sdb *StateDB) k_exist(i uint32) bool {
	//check i not in K(δ†) or c(255,i)
	_, err := sdb.GetService(i)
	if err == nil {
		// account found
		return true
	}
	return false
}

func (sdb *StateDB) NewXContext(s uint32, serviceAccount *types.ServiceAccount, u *types.PartialState) *types.XContext {
	serviceAccount.SetServiceIndex(s)

	// Calculate i for X_i eq(277)
	encoded_service, _ := types.Encode(s)
	encoded_entropy, _ := types.Encode(sdb.JamState.SafroleState.Entropy[0].Bytes())
	encoded_timeslot, _ := types.Encode(sdb.JamState.SafroleState.Timeslot)
	encoded := append(encoded_service, append(encoded_entropy, encoded_timeslot...)...)
	hash := common.Blake2Hash(encoded).Bytes()
	decoded := uint32(types.DecodeE_l(hash[:4]))

	x := &types.XContext{
		D: make(map[uint32]*types.ServiceAccount, 0), // this is NOT mutated but holds the state that could get mutatted
		S: s,
		I: sdb.Check(decoded%((1<<32)-(1<<9)) + (1 << 8)),
	}
	x.D[s] = serviceAccount
	js := sdb.JamState
	if u != nil {
		x.U = u
	} else if x.U == nil {
		x.U = &types.PartialState{
			D:                  make(map[uint32]*types.ServiceAccount), // this IS mutated
			UpcomingValidators: js.SafroleState.NextValidators,
			QueueWorkReport:    js.AuthorizationQueue,
			PrivilegedState:    js.PrivilegedServiceIndices,
		}
	}
	return x
}

/*
The single-service accumulation function, ∆1, trans-
forms an initial state-context, sequence of work-reports
and a service index into an alterations state-context, a
sequence of transfers, a possible accumulation-output and
the actual pvm gas used. This function wrangles the work-
items of a particular service from a set of work-reports and
invokes pvm execution
*/
// ∆1
// eq 176
func (sd *StateDB) SingleAccumulate(o *types.PartialState, w []types.WorkReport, f map[uint32]uint32, s uint32) (output_t []types.DeferredTransfer, output_b common.Hash, output_u uint64) {
	// gas need to check again
	// check if s is in f
	gas := uint32(0)
	if _, ok := f[s]; ok {
		gas = f[s]
	}
	// add all gas from work reports
	for _, workReport := range w {
		for _, workResult := range workReport.Results {
			if workResult.Service == s {
				gas += uint32(workResult.GasRatio)
			}
		}
	}

	var codeHash common.Hash
	p := make([]types.AccumulateOperandElements, 0)
	for _, workReport := range w {
		for _, workResult := range workReport.Results {
			if workResult.Service == s {
				codeHash = workResult.CodeHash
				p = append(p, types.AccumulateOperandElements{
					Results: types.Result{
						Ok:  workResult.Result.Ok[:],
						Err: types.RESULT_OK,
					},
					Payload:         workResult.PayloadHash,
					WorkPackageHash: workReport.AvailabilitySpec.WorkPackageHash,
					AuthOutput:      workReport.AuthOutput,
				})
				//fmt.Printf("SingleAccumulate %d workpackagehash=%v result=%v len(result)=%d\n", s, workReport.AvailabilitySpec.WorkPackageHash, workResult.Result.Ok[:], len(workResult.Result.Ok[:]))
			}
		}
	}

	serviceAccount, _ := sd.GetService(s)
	xContext := sd.NewXContext(s, serviceAccount, o)
	code := sd.ReadServicePreimageBlob(s, codeHash)
	fmt.Printf(" SingleAccumulate(%d) codehash=%v\n", s, codeHash)
	vm := pvm.NewVMFromCode(s, code, 0, sd)
	r, _ := vm.ExecuteAccumulate(p, xContext)
	//xContext.U.Dump("POST-ExecuteAccumulate", sd.Id)

	// BeefyCommitment represents a service accumulation result.
	if r.Err == types.RESULT_OK {
		output_b = common.Blake2Hash(r.Ok)
	}
	return
}

// input t is a sequence of deferred transfers, d is the desired destination service index, output is a sequence of transfers targeting said service
// eq 179
func TransferSelect(t []types.DeferredTransfer, d uint32) []types.DeferredTransfer {
	var output []types.DeferredTransfer
	for _, transfer := range t {
		if transfer.ReceiverIndex == d {
			output = append(output, transfer)
		}
	}
	return output
}

func (s *StateDB) HostTransfer(services map[uint32]types.ServiceAccount, self_index uint32, t []types.DeferredTransfer) (types.ServiceAccount, error) {
	// stub
	if _, ok := services[self_index]; !ok {
		return types.ServiceAccount{}, errors.New("HostTransfer Error: service not found")
	}
	return types.ServiceAccount{}, nil
}
