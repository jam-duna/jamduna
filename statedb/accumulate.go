package statedb

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/types"
)

type BeefyPool []Beta_state
type BeefyCommitment struct {
	Service    uint32      `json:"service"`
	Commitment common.Hash `json:"commitment"`
}

// v0.4.5 eq.165 - W^!
func AccumulatedImmediately(W []types.WorkReport) []types.WorkReport {
	outputWorkReports := []types.WorkReport{}
	for _, workReport := range W {
		if len(workReport.RefineContext.Prerequisites) == 0 && len(workReport.SegmentRootLookup) == 0 {
			outputWorkReports = append(outputWorkReports, workReport)
		}
	}
	return outputWorkReports
}

// v0.4.5 eq.166 - W^Q
func (j *JamState) QueuedExecution(W []types.WorkReport) []types.AccumulationQueue {
	outputWorkReports := []types.WorkReport{}
	for _, workReport := range W {
		if len(workReport.RefineContext.Prerequisites) != 0 || len(workReport.SegmentRootLookup) != 0 {
			outputWorkReports = append(outputWorkReports, workReport)
		}
	}
	accumulatedcup := []common.Hash{}
	accumulated := j.AccumulationHistory
	for i := 0; i < len(accumulated); i++ {
		accumulatedcup = append(accumulatedcup, accumulated[i].WorkPackageHash...)
	}

	depandancy := []types.AccumulationQueue{}
	for _, workReport := range outputWorkReports {
		depandancy = append(depandancy, Depandancy(workReport))
	}
	return QueueEditing(depandancy, accumulatedcup)
}

// v0.4.5 eq.167 - D(w)
func Depandancy(w types.WorkReport) types.AccumulationQueue {
	result := types.AccumulationQueue{}
	result.WorkReport = w
	// w.RefineContext.Prerequisite union key(w.SegmentRootLookup)
	hashSet := make(map[common.Hash]struct{})
	for _, p := range w.RefineContext.Prerequisites {
		hashSet[p] = struct{}{}
	}
	/*
		for _, key := range w.SegmentRootLookup {
			hashSet[key] = struct{}{}
		}
		depandancy := []common.Hash{}
		for key := range hashSet {
			depandancy = append(depandancy, key)
		}
	*/
	// Shawn please check
	for _, segmentRootLookupItem := range w.SegmentRootLookup {
		hashSet[segmentRootLookupItem.WorkPackageHash] = struct{}{}
	}
	depandancy := []common.Hash{}
	for key := range hashSet {
		depandancy = append(depandancy, key)
	}
	result.WorkPackageHash = depandancy
	return result
}

// v0.4.5 eq.168 - function E
func QueueEditing(r []types.AccumulationQueue, x []common.Hash) []types.AccumulationQueue {
	result := make([]types.AccumulationQueue, 0)
	hashSet := make(map[common.Hash]struct{}, len(x))
	for _, h := range x {
		hashSet[h] = struct{}{}
	}

	for _, item := range r {
		found := false
		h := item.WorkReport.AvailabilitySpec.WorkPackageHash
		if _, exists := hashSet[h]; exists {
			found = true
		}
		if !found {
			new_wp_hash := []common.Hash{}
			for _, h := range item.WorkPackageHash {
				if _, exists := hashSet[h]; !exists {
					new_wp_hash = append(new_wp_hash, h)
				}
			}
			item.WorkPackageHash = new_wp_hash
			//sort the hashes
			sort.Slice(item.WorkPackageHash, func(i, j int) bool {
				return bytes.Compare(item.WorkPackageHash[i][:], item.WorkPackageHash[j][:]) < 0
			})
			result = append(result, item)
		}
	}

	return result
}

// v0.4.5 eq.169 - function Q
func PriorityQueue(r []types.AccumulationQueue) []types.WorkReport {
	g := []types.WorkReport{}
	for _, item := range r {
		if len(item.WorkPackageHash) == 0 {
			g = append(g, item.WorkReport)
		}
	}
	if len(g) == 0 {
		return []types.WorkReport{}
	}
	round_result := PriorityQueue(QueueEditing(r, Mapping(g)))
	g = append(g, round_result...)
	return g
}

// v0.4.5 eq.170 - function P
func Mapping(w []types.WorkReport) []common.Hash {
	result := make([]common.Hash, 0)
	for _, workReport := range w {
		result = append(result, workReport.AvailabilitySpec.WorkPackageHash)
	}
	return result
}

// v0.4.5 eq.171 - m
func (s *StateDB) CurrentEpochSlot() uint32 {
	Ht := s.Block.Header.Slot
	return Ht % types.EpochLength
}

func get_workreport_workpackagehashes(W []types.WorkReport) []common.Hash {
	wphs := make([]common.Hash, len(W))
	for i, wr := range W {
		wphs[i] = wr.GetWorkPackageHash()
	}
	return wphs
}

func get_accumulationqueue_workpackagehashes(q []types.AccumulationQueue) [][]common.Hash {
	wphs := make([][]common.Hash, len(q))
	for i, aq := range q {
		wphs[i] = aq.WorkPackageHash
	}
	return wphs
}

// v0.4.5 eq.172 - W^*
func (s *StateDB) AccumulatableSequence(W []types.WorkReport) []types.WorkReport {
	accumulated_immediately := AccumulatedImmediately(W)
	result := accumulated_immediately
	j := s.GetJamState()
	queued_execution := j.QueuedExecution(W)
	q := s.ComputeReadyQueue(queued_execution, accumulated_immediately)
	Q_q := PriorityQueue(q)
	result = append(result, Q_q...)

	if len(accumulated_immediately) != len(result) {
		log.Trace(s.Authoring, "ORDERED ACCUMULATION", "W^! (wphs accumulated immediately)", get_workreport_workpackagehashes(accumulated_immediately),
			"q", get_accumulationqueue_workpackagehashes(q), "Q(q)-priority queue result", get_workreport_workpackagehashes(Q_q), "W^*-wphs of accumulatable work reports)", get_workreport_workpackagehashes(result))
	}
	return result
}

// v0.4.5 eq.173 - q
func (s *StateDB) GetReadyQueue(W []types.WorkReport) []common.Hash {
	accumulated_immediately := AccumulatedImmediately(W)
	j := s.GetJamState()
	queued_execution := j.QueuedExecution(W)
	readyQueue := PriorityQueue(s.ComputeReadyQueue(queued_execution, accumulated_immediately))
	readyQueueHeader := []common.Hash{}
	for _, workReport := range readyQueue {
		readyQueueHeader = append(readyQueueHeader, workReport.AvailabilitySpec.WorkPackageHash)
	}
	return readyQueueHeader
}

// v0.4.5 eq.173 - q
// v0.6.1 12.12
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

type Usage struct {
	Service uint32
	Gas     uint64
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
func (s *StateDB) OuterAccumulate(g uint64, w []types.WorkReport, o *types.PartialState, f map[uint32]uint32) (num uint64, output_t []types.DeferredTransfer, output_b []BeefyCommitment, GasUsage []Usage) { // not really sure i here , the max meaning. use uint32 for now
	var gas_tmp uint64
	i := uint64(0)
	// calculate how to maximize the work reports to enter the parallelized accumulation
	for _, workReport := range w {
		for _, workResult := range workReport.Results {
			gas_tmp += workResult.Gas
			if gas_tmp <= g {
				i++
				if i >= uint64(len(w))+1 {
					break
				}
			}

		}
	}

	if i == 0 { // if i = 0, then nothing to do
		num = 0

		output_t = make([]types.DeferredTransfer, 0)
		output_b = make([]BeefyCommitment, 0)
		GasUsage = make([]Usage, 0)
		return
	}
	if i >= uint64(len(w)) { // if i >= len(w), then all work reports are accumulated
		i = uint64(len(w))
	}
	g_star, t_star, b_star, U := s.ParallelizedAccumulate(o, w[0:i], f) // parallelized accumulation the 0 to i work reports // parallelized accumulation the 0 to i work reports

	if i >= uint64(len(w)) { // no more reports
		return i, t_star, b_star, U
	}
	j, outputT, outputB, GasUsage := s.OuterAccumulate(g-g_star, w[i+1:], o, nil) // recursive call to the rest of the work reports
	num = i + j

	output_t = append(outputT, t_star...)
	for _, b := range b_star {
		duplicate := false
		for _, existingB := range outputB {
			if existingB.Service == b.Service && existingB.Commitment == b.Commitment {
				duplicate = true
				break
			}
		}

		if b.Commitment != (common.Hash{}) && !duplicate {
			output_b = append(output_b, b)
		}
	}
	GasUsage = append(U, GasUsage...)
	return
}

/*
the parallelized accumulation function ∆∗ which, with the help of the single-service accumulation function ∆1, transforms an initial state-context, together with a sequence of work-reports and a dictionary of
privileged always-accumulate services, into a tuple of the total gas utilized in pvm execution u, a posterior statecontext (x′,d′,i′,q′) and the resultant accumulationoutput pairings b and deferred-transfers Ì
*/
// the parallelized accumulation function ∆*
// eq 174
func (s *StateDB) ParallelizedAccumulate(o *types.PartialState, w []types.WorkReport, f map[uint32]uint32) (output_u uint64, output_t []types.DeferredTransfer, output_b []BeefyCommitment, GasUsage []Usage) {
	GasUsage = make([]Usage, 0)
	services := make([]uint32, 0)
	for _, workReport := range w {
		for _, workResult := range workReport.Results {
			services = append(services, workResult.ServiceID)
		}
	}
	output_u = 0
	// get services from f key
	for k := range f {
		services = append(services, k)
	}
	// remove duplicates
	services = UniqueUint32Slice(services)
	for _, service := range services {
		// this is parallelizable
		B, U, XY, exceptional := s.SingleAccumulate(o, w, f, service)
		if XY == nil {
			log.Warn(module, "SingleAccumulate returned nil XContext", "service", service)
			continue
		}
		output_u += U
		empty := common.Hash{}
		GasUsage = append(GasUsage, Usage{
			Service: service,
			Gas:     U,
		})
		if B == empty {

		} else {
			output_b = append(output_b, BeefyCommitment{
				Service:    service,
				Commitment: B,
			})
		}
		if XY.U != nil {
			if XY.U.D != nil {
				for s, sa := range XY.U.D {
					if exceptional {
						if sa.Checkpointed {
							sa.Dirty = true
							o.D[s] = sa
						}
					} else {
						sa.Dirty = true
						o.D[s] = sa
					}
				}
			}
		}
		// ASSIGN
		if s.JamState.PrivilegedServiceIndices.Kai_a == service {
			o.QueueWorkReport = XY.U.QueueWorkReport
		}
		// VALIDATE
		if s.JamState.PrivilegedServiceIndices.Kai_v == service {
			o.UpcomingValidators = XY.U.UpcomingValidators
		}
		// BLESS
		if s.JamState.PrivilegedServiceIndices.Kai_m == service {
			o.PrivilegedState.Kai_a = XY.U.PrivilegedState.Kai_a
			o.PrivilegedState.Kai_v = XY.U.PrivilegedState.Kai_v
			o.PrivilegedState.Kai_m = XY.U.PrivilegedState.Kai_m
			for k, v := range XY.U.PrivilegedState.Kai_g {
				o.PrivilegedState.Kai_g[k] = v
			}
		}

		output_t = append(output_t, XY.T...)

		// apply p
		for _, p := range XY.P {
			ServiceIndex := p.ServiceIndex
			P_data := p.P_data
			if ServiceIndex == service {
				sa, ok := o.GetService(ServiceIndex)
				if ok {
					lookup_ok, X_s_l := sa.ReadLookup(common.Blake2Hash(P_data), uint32(len(P_data)), s)
					if lookup_ok && len(X_s_l) == 0 {
						sa.WriteLookup(common.Blake2Hash(P_data), uint32(len(P_data)), []uint32{s.JamState.SafroleState.Timeslot})
						sa.WritePreimage(common.Blake2Hash(P_data), P_data)
						sa.Dirty = true
					}
				}
			}
		}
	}

	// s ∈ K(d) ∖ s
	/*s.SingleAccumulate(o, w, f, o.PrivilegedState.Kai_m)
	s.SingleAccumulate(o, w, f, o.PrivilegedState.Kai_a)
	s.SingleAccumulate(o, w, f, o.PrivilegedState.Kai_v) */

	return
}

func UniqueUint32Slice(slice []uint32) []uint32 {
	keys := make(map[uint32]bool)
	list := []uint32{}
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// Implements B13 recursive check https://graypaper.fluffylabs.dev/#/5f542d7/2e0d032e0d03
func (sdb *StateDB) NewXContext_Check(i uint32) uint32 {
	for sdb.k_exist(i) {
		// 2^8 = 256, 2^32 - 2^9 = 4294966784
		i = (uint32(i-uint32(256)+1) % uint32(4294966784)) + uint32(256)
	}
	return i
}

func (sdb *StateDB) k_exist(i uint32) bool {
	//check i not in K(δ†) or c(255,i)
	_, ok, err := sdb.GetService(i)
	if err == nil && ok {
		// account found
		return true
	}
	return false
}

func (sdb *StateDB) NewXContext(u *types.PartialState, s uint32, serviceAccount *types.ServiceAccount) *types.XContext {

	// Calculate x.I 0.6.2 (B.9) https://graypaper.fluffylabs.dev/#/5f542d7/2efd002efd00
	encoded_service, _ := types.Encode(s)
	encoded_entropy := sdb.JamState.SafroleState.Entropy[0].Bytes()
	encoded_timeslot, _ := types.Encode(sdb.JamState.SafroleState.Timeslot)
	encoded := append(encoded_service, append(encoded_entropy, encoded_timeslot...)...)
	hash := common.Blake2Hash(encoded).Bytes()
	decoded := uint32(types.DecodeE_l(hash[:4]))

	x := &types.XContext{
		U: u.Clone(), // IMPORTANT: writes in one service (Fib, Trib) are readable by another (Meg) in ordered accumulation
		S: s,
		I: sdb.NewXContext_Check(decoded%((1<<32)-(1<<9)) + (1 << 8)),
	}
	//log.Trace(module, "s", s, "eta_0'", sdb.JamState.SafroleState.Entropy[0].Bytes(), "timeslot", sdb.JamState.SafroleState.Timeslot)
	//log.Trace(module, "encode(s, eta_0', timeslot)", encoded, "hash(encoded)", hash, "hashed[:4]", hash[:4], "decode(hashed[:4])", decoded)
	//log.Trace(module, "decoded mod 4294966784 + 256= %d  ====> this will be the new service id\n\n", x.I)

	// IMPORTABLE NOW WE MAKE A COPY of serviceAccount AND MAKE IT MUTABLE
	mutableServiceAccount := serviceAccount.Clone()
	mutableServiceAccount.ALLOW_MUTABLE()
	x.U.D[s] = mutableServiceAccount // NOTE: this is a distinct COPY of serviceAccount and CAN have Set{...}
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
func (sd *StateDB) SingleAccumulate(o *types.PartialState, w []types.WorkReport, f map[uint32]uint32, s uint32) (output_b common.Hash, output_u uint64, xy *types.XContext, exceptional bool) {
	// gas need to check again
	// check if s is in f
	gas := uint32(0)
	if _, ok := f[s]; ok {
		gas = f[s]
	}
	// add all gas from work reports
	for _, workReport := range w {
		for _, workResult := range workReport.Results {
			if workResult.ServiceID == s {
				gas += uint32(workResult.Gas)
			}
		}
	}

	var codeHash common.Hash
	p := make([]types.AccumulateOperandElements, 0)
	g := uint64(0)
	for _, workReport := range w {
		for _, workResult := range workReport.Results {
			if workResult.ServiceID == s {
				codeHash = workResult.CodeHash
				g += workResult.Gas
				o := types.AccumulateOperandElements{
					H: workReport.AvailabilitySpec.WorkPackageHash,
					E: workReport.AvailabilitySpec.ExportedSegmentRoot,
					G: workResult.Gas,
					A: workReport.AuthorizerHash,
					O: workReport.AuthOutput,
					Y: workResult.PayloadHash,
					D: workResult.Result,
				}
				log.Trace(sd.Authoring, "SINGLE ACCUMULATE", "s", fmt.Sprintf("%d", s), "wrangledResults", types.DecodedWrangledResults(&o))
				p = append(p, o)
			}
		}
	}

	//  get serviceAccount from U.D[s] FIRST
	var err error
	serviceAccount, ok := o.GetService(s)
	if !ok {
		serviceAccount, ok, err = sd.GetService(s)
		if err != nil || !ok {
			log.Error(module, "GetService ERR", "ok", ok, "err", err, "s", s)
			return
		}
	}
	xContext := sd.NewXContext(o, s, serviceAccount)

	xy = xContext // if code does not exist, fallback to this
	ok, code := serviceAccount.ReadPreimage(codeHash, sd)
	if !ok {
		// CHECK
		return
	}

	//(B.8) start point
	vm := pvm.NewVMFromCode(s, code, 0, sd)
	pvmContext := log.PvmValidating
	if sd.Authoring == log.GeneralAuthoring {
		pvmContext = log.PvmAuthoring
	}
	vm.SetPVMContext(pvmContext)
	t := sd.JamState.SafroleState.Timeslot
	vm.Timeslot = t
	r, _, serviceAccount := vm.ExecuteAccumulate(t, s, g, p, xContext)

	exceptional = false
	if r.Err == types.RESULT_OOG || r.Err == types.RESULT_PANIC {
		exceptional = true
		output_b = vm.Y.Y
		output_u = g - uint64(max(vm.Gas, 0))
		xy = &(vm.Y)
		if r.Err == types.RESULT_OOG {
			log.Trace(sd.Authoring, "BEEFY OOG   @SINGLE ACCUMULATE", "s", fmt.Sprintf("%d", s), "B", output_b)
		} else {
			log.Trace(sd.Authoring, "BEEFY PANIC @SINGLE ACCUMULATE", "s", fmt.Sprintf("%d", s), "B", output_b)
		}
		return
	}
	xy = vm.X
	output_u = g - uint64(max(vm.Gas, 0))
	res := ""
	if len(r.Ok) == 32 {
		output_b = common.BytesToHash(r.Ok)
		res = "32byte"
	} else {
		output_b = vm.X.Y
		res = "yield"
	}
	log.Debug(debugB, fmt.Sprintf("BEEFY OK-HALT with %s @SINGLE ACCUMULATE", res), "s", fmt.Sprintf("%d", s), "B", output_b)
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

func (s *StateDB) HostTransfer(self *types.ServiceAccount, time_slot uint32, self_index uint32, t []types.DeferredTransfer) (gasUsed int64, transferCount uint, err error) { // select transfers eq 12.23
	selectedTransfers := TransferSelect(t, self_index)
	if len(selectedTransfers) == 0 {
		return 0, 0, nil
	}
	gas := uint64(0)
	for _, transfer := range selectedTransfers {
		self.Balance += transfer.Amount
		gas += transfer.GasLimit
	}

	// this create PreimageObject in ServiceAccount with Accessed = true
	ok, code := self.ReadPreimage(self.CodeHash, s)
	if !ok {
		return 0, uint(len(selectedTransfers)), nil
	}

	vm := pvm.NewVMFromCode(self_index, code, 0, s)
	pvmContext := log.PvmValidating
	if s.Authoring == log.GeneralAuthoring {
		pvmContext = log.PvmAuthoring
	}
	vm.SetPVMContext(pvmContext)
	vm.Gas = int64(gas)

	var input_argument []byte
	encode_time_slot, _ := types.Encode(time_slot)
	encodeService_index, _ := types.Encode(self_index)
	encodeSelectedTransfers, _ := types.Encode(selectedTransfers)

	input_argument = append(input_argument, encode_time_slot...)
	input_argument = append(input_argument, encodeService_index...)
	input_argument = append(input_argument, encodeSelectedTransfers...)
	vm.ExecuteTransfer(input_argument, self)
	gasUsed = int64(gas) - vm.Gas
	return gasUsed, uint(len(selectedTransfers)), nil
}

// eq 12.24
func (s *StateDB) ProcessDeferredTransfers(o *types.PartialState, time_slot uint32, t []types.DeferredTransfer) (transferStats map[uint32]*transferStatistics, err error) {
	transferStats = make(map[uint32]*transferStatistics)
	for service, serviceAccount := range o.D {
		gasUsed, transferCount, err := s.HostTransfer(serviceAccount, time_slot, uint32(service), t)
		if err != nil {
			return transferStats, err
		}
		transferStats[serviceAccount.ServiceIndex] = &transferStatistics{
			gasUsed:      uint(gasUsed),
			numTransfers: transferCount,
		}
		log.Debug(s.Authoring, "ProcessDeferredTransfers", "service", fmt.Sprintf("%d", serviceAccount.ServiceIndex), "gasUsed", gasUsed, "transferCount", transferCount)
	}
	return transferStats, nil
}

func (s *StateDB) ApplyStateTransitionAccumulation(w_star []types.WorkReport, num uint64, previousTimeslot uint32) {
	jam := s.GetJamState()
	w_q := jam.QueuedExecution(s.AvailableWorkReport)
	jam.UpdateLatestHistory(w_star, int(num))
	jam.UpdateReadyQueuedReport(w_q, previousTimeslot)
}

// eq 186, 187 0.4.5
func (j *JamState) UpdateLatestHistory(w_star []types.WorkReport, num int) {
	// get accumulated work report
	accumulated_wr := w_star[:num]
	// phasing every history
	// 187
	for i := 0; i < types.EpochLength-1; i++ {
		tmp_history := j.AccumulationHistory[i+1]
		j.AccumulationHistory[i] = tmp_history
	}
	// eq 186 0.4.5
	j.AccumulationHistory[types.EpochLength-1].WorkPackageHash = Mapping(accumulated_wr)

	SortByHash(j.AccumulationHistory[types.EpochLength-1].WorkPackageHash)

	// if len(j.AccumulationHistory[types.EpochLength-1].WorkPackageHash) > 0 {
	// 	fmt.Printf("UpdateLatestHistory WorkPackageHash=%v\n", j.AccumulationHistory[types.EpochLength-1].WorkPackageHash)
	// }
}

func SortByHash(hashes []common.Hash) {
	// sort the hashes
	sort.Slice(hashes, func(i, j int) bool {
		return bytes.Compare(hashes[i][:], hashes[j][:]) < 0
	})
}

func (j *JamState) UpdateReadyQueuedReport(w_q []types.AccumulationQueue, previous_t uint32) {
	timeslot := j.SafroleState.Timeslot
	_, phase := j.SafroleState.EpochAndPhase(timeslot)
	if previous_t == 0 {
		return
	}
	for i := uint32(0); i < types.EpochLength; i++ {
		// mod to get the correct index
		num := (int(phase) - int(i) + types.EpochLength) % types.EpochLength
		if i == 0 {
			j.AccumulationQueue[num] = QueueEditing(w_q, j.AccumulationHistory[types.EpochLength-1].WorkPackageHash)
		} else if i >= 1 && i < timeslot-previous_t {
			j.AccumulationQueue[num] = make([]types.AccumulationQueue, 0)
		} else {
			j.AccumulationQueue[num] = QueueEditing(j.AccumulationQueue[num], j.AccumulationHistory[types.EpochLength-1].WorkPackageHash)
		}

	}
}
