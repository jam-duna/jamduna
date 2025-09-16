package statedb

import (
	"bytes"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

type BeefyPool []HistoryState

// v0.4.5 eq.165 - W^!
func AccumulatedImmediately(workReports []types.WorkReport) []types.WorkReport {
	outputWorkReports := []types.WorkReport{}
	for _, workReport := range workReports {
		if len(workReport.RefineContext.Prerequisites) == 0 && len(workReport.SegmentRootLookup) == 0 {
			outputWorkReports = append(outputWorkReports, workReport)
		}
	}
	return outputWorkReports
}

// v0.4.5 eq.166 - W^Q
func (j *JamState) QueuedExecution(workReports []types.WorkReport) []types.AccumulationQueue {
	outputWorkReports := []types.WorkReport{}
	for _, workReport := range workReports {
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
/*
Example from  https://github.com/davxy/jam-test-vectors/pull/90#issuecomment-3217905803
we end up with three reports to accumulate:

w1 = 0x30106542a08638f4b03aa444a502e84c5d8c0af2f4e7d78dec6fcf20839ba96c w1 has 4 work items with 2_500_000 gas each
w2 = 0x9e93aef22e437721ab38c1e6cde35faee26a67d04d00e3cd68d1b6e6509d12c8 w2 has 4 work items with 2_500_000 gas each
w3 = 0x59763c52ed98c413c82bdba8f8483dde3d1249bb8330f46cbf5b87b7297a45dc w3 has 2 work items with 5_000_000 gas each
How many times service 0 calls into accumulate?  You call into OuterAccumulate just once with all the reports in one shot.

However, this approach does not account for the fact that each call to accumulate must be limited by the maximum allowed gas per block (as per 12.21 and 12.16).

In our case we have:
 G_A = 10000000 (max_accumulate_gas_per_core), C = 2 (cores)
 G_T = 20000000 (total gas for all accumulations)
Thus, given 12.21 we have that the max gas that can be used is 20_000_000.

All the three reports have a gas for accumulate set to 10_000_000.

Given that w1_gas + w2_gas hits the max accumulate gas we first call accumulate for w1 and w2 (i.e. Δ∗) as prescribed by Δ+ (12.16).
When Δ∗ returns, we compute the effective gas consumed, we subtract it from g and we call into Δ+ again for w3.
The effective gas consumed by w1+w2 is 91_982. So, since the gas required by w3 + 91_982 <= 20_000_000 we accumulate w3 as well.

In the end the service 0 calls into accumulate twice and not once: One time for w1+w2 and one for w3
*/
func (s *StateDB) updateRecentAccumulation(o *types.PartialState, accumulated_partial map[uint32]*types.XContext) {
	ts := s.JamState.SafroleState.Timeslot
	for svc, part := range accumulated_partial {
		if part == nil || part.U == nil || part.U.ServiceAccounts == nil {
			continue
		}
		if sa, ok := part.U.ServiceAccounts[svc]; ok && sa != nil {
			//fmt.Printf(" svc %v sa.RecentAccumulation %v ts %v\n", svc, sa.RecentAccumulation, ts)
			sa.UpdateRecentAccumulation(ts)
			o.ServiceAccounts[svc] = sa
		}
	}
}

func (s *StateDB) OuterAccumulate(g uint64, workReports []types.WorkReport, o *types.PartialState, freeAccumulation map[uint32]uint32, pvmBackend string, accumulated_partial map[uint32]*types.XContext) (num_accumulations uint64, transfers []types.DeferredTransfer, accumulation_output []types.AccumulationOutput, GasUsage []Usage) {
	var gas_tmp uint64
	i := uint64(0)
	// calculate how to maximize the work reports to enter the parallelized accumulation
	done := false
	for _, workReport := range workReports {
		for _, workDigest := range workReport.Results {
			if gas_tmp+workDigest.Gas <= types.AccumulateGasAllocation_GT {
				gas_tmp += workDigest.Gas
			} else {
				done = true
				break
			}
		}
		if i >= uint64(len(workReports))+1 || done {
			break
		} else {
			i++
		}
	}

	if len(workReports) == 0 { // if i = 0, then nothing to do
		num_accumulations = 0
		transfers = make([]types.DeferredTransfer, 0)
		accumulation_output = make([]types.AccumulationOutput, 0)
		GasUsage = make([]Usage, 0)
		return
	}
	p_gasUsed, p_transfers, p_outputs, p_gasUsage := s.ParallelizedAccumulate(o, workReports[0:i], freeAccumulation, pvmBackend, accumulated_partial) // parallelized accumulation the 0 to i work reports

	if len(workReports[i:]) > 0 { // if i = len(workReports), then nothing more to do
		incNum, incTransfers, incAccumulationOutput, incGasUsage := s.OuterAccumulate(g-p_gasUsed, workReports[i:], o, nil, pvmBackend, accumulated_partial) // recursive call to the rest of the work reports
		num_accumulations = i + incNum
		accumulation_output = append(p_outputs, incAccumulationOutput...)
		transfers = append(incTransfers, p_transfers...)
		GasUsage = append(p_gasUsage, incGasUsage...)
		s.updateRecentAccumulation(o, accumulated_partial)
		return
	}
	s.updateRecentAccumulation(o, accumulated_partial)
	return i, p_transfers, p_outputs, p_gasUsage
}

/*
the parallelized accumulation function ∆∗ which, with the help of the single-service accumulation function ∆1, transforms an initial state-context, together with a sequence of work-reports and a dictionary of
privileged always-accumulate services, into a tuple of the total gas utilized in pvm execution u, a posterior statecontext (x′,d′,i′,q′) and the resultant accumulationoutput pairings b and deferred-transfers Ì

now with accRess go func!
*/
func (s *StateDB) ParallelizedAccumulate(
	o *types.PartialState,
	workReports []types.WorkReport,
	freeAccumulation map[uint32]uint32,
	pvmBackend string,
	accumulated_partial map[uint32]*types.XContext,
) (totalGasUsed uint64, transfers []types.DeferredTransfer, accumulation_output []types.AccumulationOutput, GasUsage []Usage) {

	GasUsage = make([]Usage, 0, 64)

	// Build the service set: s = {rs S w ∈ w, r ∈ wr} ∪ K(f ) - optimized with map
	serviceMap := make(map[uint32]struct{}, 64)
	for _, wr := range workReports {
		for _, digest := range wr.Results {
			serviceMap[digest.ServiceID] = struct{}{}
		}
	}
	for k := range freeAccumulation {
		serviceMap[k] = struct{}{}
	}

	services := make([]uint32, 0, len(serviceMap))
	for service := range serviceMap {
		services = append(services, service)
	}
	if len(services) == 0 {
		return 0, nil, nil, nil
	}

	// --- Parallel section: call SingleAccumulate per service in a worker pool ---
	type accRes struct {
		service     uint32
		output      common.Hash
		gasUsed     uint64
		XY          *types.XContext
		exceptional bool
	}

	par := runtime.GOMAXPROCS(0) // tune if desired
	if par < 1 {
		par = 1
	}

	jobCh := make(chan uint32, len(services))
	resCh := make(chan accRes, len(services))
	var wg sync.WaitGroup

	// SingleAccumulate!
	worker := func() {
		defer wg.Done()
		for service := range jobCh {
			t0 := time.Now()
			out, gas, XY, exceptional := s.SingleAccumulate(o, workReports, freeAccumulation, service, pvmBackend)
			benchRec.Add("SingleAccumulate", time.Since(t0))
			resCh <- accRes{
				service:     service,
				output:      out,
				gasUsed:     gas,
				XY:          XY,
				exceptional: exceptional,
			}
		}
	}

	wg.Add(par)
	for i := 0; i < par; i++ {
		go worker()
	}
	for _, svc := range services {
		jobCh <- svc
	}
	close(jobCh)
	wg.Wait()
	close(resCh)

	// Collect results into a slice and sort for deterministic merge order
	results := make([]accRes, 0, len(services))
	for r := range resCh {
		if r.XY == nil {
			log.Warn(log.SDB, "SingleAccumulate returned nil XContext", "service", r.service)
			continue
		}
		results = append(results, r)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].service < results[j].service })

	// put all the results back together in o
	empty := common.Hash{}

	for _, r := range results {
		totalGasUsed += r.gasUsed
		GasUsage = append(GasUsage, Usage{Service: r.service, Gas: r.gasUsed})

		// b = {(s,b) | b = ∆1(...)_b, b ≠ ∅}
		if r.output != empty {
			accumulation_output = append(accumulation_output, types.AccumulationOutput{
				Service: r.service,
				Output:  r.output,
			})
		}

		// t = [∆1(... )_t]  (deferred transfers)
		if r.XY != nil && len(r.XY.Transfers) > 0 {
			transfers = append(transfers, r.XY.Transfers...)
		}

		// Service accounts
		if r.XY != nil && r.XY.U != nil {
			if r.XY.U.ServiceAccounts != nil {
				for sid, sa := range r.XY.U.ServiceAccounts {
					if r.exceptional {
						if sa.Checkpointed {
							sa.Dirty = true
							o.ServiceAccounts[sid] = sa
						}
					} else {
						sa.Dirty = true
						o.ServiceAccounts[sid] = sa
					}
				}
			}
			// QueueWorkReport / UpcomingValidators
			o.QueueWorkReport = r.XY.U.QueueWorkReport
			o.UpcomingValidators = r.XY.U.UpcomingValidators

			for k, v := range r.XY.U.PrivilegedState.AlwaysAccServiceID {
				o.PrivilegedState.AlwaysAccServiceID[k] = v
			}
		}

		accumulated_partial[r.service] = r.XY
	}

	// sort by Service id
	if len(accumulation_output) > 1 {
		sort.Slice(accumulation_output, func(i, j int) bool {
			return accumulation_output[i].Service < accumulation_output[j].Service
		})
	}

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
	encoded_service, _ := types.Encode(uint(s))
	encoded_entropy := sdb.JamState.SafroleState.Entropy[0].Bytes()
	encoded_timeslot, _ := types.Encode(uint(sdb.JamState.SafroleState.Timeslot))
	encoded := append(encoded_service, append(encoded_entropy, encoded_timeslot...)...)
	hash := common.Blake2Hash(encoded).Bytes()
	decoded := uint32(types.DecodeE_l(hash[:4]))

	x := &types.XContext{
		U:               u.Clone(), // IMPORTANT: writes in one service (Fib, Trib) are readable by another (Meg) in ordered accumulation
		ServiceIndex:    s,
		NewServiceIndex: sdb.NewXContext_Check(decoded%((1<<32)-(1<<9)) + (1 << 8)),
	}

	// IMPORTABLE NOW WE MAKE A COPY of serviceAccount AND MAKE IT MUTABLE
	mutableServiceAccount := serviceAccount.Clone()
	mutableServiceAccount.ALLOW_MUTABLE()
	x.U.ServiceAccounts[s] = mutableServiceAccount // NOTE: this is a distinct COPY of serviceAccount and CAN have Set{...}
	return x
}

func (sd *StateDB) InitXContextService(s uint32, o *types.PartialState, xy *types.XContext) error {
	// Initialize the service account for the given service ID and XContext
	if xy.U == nil {
		xy.U = o.Clone() // make a copy of the partial state
		xy.U.ServiceAccounts = make(map[uint32]*types.ServiceAccount)
	}

	if xy.U != nil && xy.U.ServiceAccounts == nil {
		xy.U.ServiceAccounts = make(map[uint32]*types.ServiceAccount)

	}

	if xy.U.ServiceAccounts[s] == nil {
		x_s, ok, err := sd.GetService(s) // misplaced
		if !ok || err != nil {
			fmt.Printf("service %v ok %v err %v\n", s, ok, err)
			return fmt.Errorf("service %v Err: %v", s, err)
		}
		xy.U.ServiceAccounts[s] = x_s
	}
	return nil
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
func (sd *StateDB) SingleAccumulate(o *types.PartialState, workReports []types.WorkReport, freeAccumulation map[uint32]uint32, serviceID uint32, pvmBackend string) (accumulation_output common.Hash, gasUsed uint64, xy *types.XContext, exceptional bool) {
	t0 := time.Now()

	// gas need to check again
	// check if serviceID is in f
	gas := uint32(0)
	if _, ok := freeAccumulation[serviceID]; ok {
		gas = freeAccumulation[serviceID]
	}
	// add all gas from work reports
	for _, workReport := range workReports {
		for _, workDigest := range workReport.Results {
			if workDigest.ServiceID == serviceID {
				gas += uint32(workDigest.Gas)
			}
		}
	}

	operandElements := make([]types.AccumulateOperandElements, 0)
	g := uint64(0)
	for _, workReport := range workReports {
		for _, workDigest := range workReport.Results {
			if workDigest.ServiceID == serviceID {
				g += workDigest.Gas
				operandElement := types.AccumulateOperandElements{
					WorkPackageHash:     workReport.AvailabilitySpec.WorkPackageHash,
					ExportedSegmentRoot: workReport.AvailabilitySpec.ExportedSegmentRoot,
					Gas:                 uint(workDigest.Gas),
					AuthorizerHash:      workReport.AuthorizerHash,
					Trace:               workReport.Trace,
					PayloadHash:         workDigest.PayloadHash,
					Result:              workDigest.Result,
				}
				log.Debug(sd.Authoring, "SINGLE ACCUMULATE", "s", fmt.Sprintf("%d", serviceID), "wrangledResults", types.DecodedWrangledResults(&operandElement))
				operandElements = append(operandElements, operandElement)
			}
		}
	}

	//  get serviceAccount from U.D[s] FIRST
	var err error
	serviceAccount, ok := o.GetService(serviceID)
	if !ok {
		serviceAccount, ok, err = sd.GetService(serviceID)
		if err != nil || !ok {
			log.Error(log.SDB, "GetService ERR", "ok", ok, "err", err, "s", serviceID)
			return
		}
	}

	xContext := sd.NewXContext(o, serviceID, serviceAccount)

	xy = xContext // if code does not exist, fallback to this
	err = sd.InitXContextService(serviceID, o, xy)
	if err != nil {
		log.Error(log.SDB, "InitXContextService ERR", "s", serviceID, "error", err)
		return
	}
	ok, code, preimage_source := serviceAccount.ReadPreimage(serviceAccount.CodeHash, sd)
	if !ok {
		log.Warn(log.SDB, "GetPreimage ERR", "ok", ok, "s", serviceID, "codeHash", serviceAccount.CodeHash, "preimage_source", preimage_source)
		return
	}
	benchRec.Add("SingleAccumulate:Setup", time.Since(t0))

	//(B.8) start point
	t0 = time.Now()
	vm := NewVMFromCode(serviceID, code, 0, 0, sd, pvmBackend, g)
	pvmContext := log.PvmValidating
	if sd.Authoring == log.GeneralAuthoring {
		pvmContext = log.PvmAuthoring
	}
	benchRec.Add("SingleAccumulate:NewVMFromCode", time.Since(t0))

	vm.SetPVMContext(pvmContext)
	timeslot := sd.JamState.SafroleState.Timeslot
	vm.Timeslot = timeslot
	t0 = time.Now()

	r, _, x_s := vm.ExecuteAccumulate(timeslot, serviceID, operandElements, xContext, sd.JamState.SafroleState.Entropy[0])
	benchRec.Add("ExecuteAccumulate", time.Since(t0))
	exceptional = false
	if r.Err == types.WORKDIGEST_OOG || r.Err == types.WORKDIGEST_PANIC {
		exceptional = true
		accumulation_output = vm.Y.Yield
		gasUsed = g - uint64(max(vm.GetGas(), 0))
		xy = &(vm.Y)

		if r.Err == types.WORKDIGEST_OOG {
			log.Trace(sd.Authoring, "BEEFY OOG   @SINGLE ACCUMULATE", "s", fmt.Sprintf("%d", serviceID), "accumulation_output", accumulation_output, "x_s", x_s)
		} else {
			log.Trace(sd.Authoring, "BEEFY PANIC @SINGLE ACCUMULATE", "s", fmt.Sprintf("%d", serviceID), "accumulation_output", accumulation_output, "x_s", x_s)
		}
		return
	}
	xy = vm.X
	gasUsed = g - uint64(max(vm.GetGas(), 0))
	res := ""
	if len(r.Ok) == 32 {
		accumulation_output = common.BytesToHash(r.Ok)
		res = "32byte"
	} else {
		accumulation_output = vm.X.Yield
		res = "yield"
	}
	log.Trace(debugB, fmt.Sprintf("BEEFY OK-HALT with %s @SINGLE ACCUMULATE", res), "s", fmt.Sprintf("%d", serviceID), "accumulation_output", accumulation_output)
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

func (s *StateDB) HostTransfer(self *types.ServiceAccount, time_slot uint32, self_index uint32, t []types.DeferredTransfer, pvmBackend string) (gasUsed int64, transferCount uint, err error) { // select transfers eq 12.23
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
	ok, code, preimage_source := self.ReadPreimage(self.CodeHash, s)
	if !ok {
		log.Trace(log.SDB, "GetPreimage ERR in HostTransfer", "ok", ok, "s", self_index, "codeHash", self.CodeHash, "preimage_source", preimage_source)
		return 0, uint(len(selectedTransfers)), nil
	}

	vm := NewVMFromCode(self_index, code, 0, 0, s, pvmBackend, gas)
	pvmContext := log.PvmValidating
	if s.Authoring == log.GeneralAuthoring {
		pvmContext = log.PvmAuthoring
	}
	vm.SetPVMContext(pvmContext)

	var input_argument []byte
	encode_time_slot, _ := types.Encode(time_slot)
	encodeService_index, _ := types.Encode(self_index)
	encodeSelectedTransfers, _ := types.Encode(selectedTransfers)

	input_argument = append(input_argument, encode_time_slot...)
	input_argument = append(input_argument, encodeService_index...)
	input_argument = append(input_argument, encodeSelectedTransfers...)
	vm.ExecuteTransfer(input_argument, self)
	gasUsed = int64(gas) - vm.GetGas()
	return gasUsed, uint(len(selectedTransfers)), nil
}

// eq 12.24
func (s *StateDB) ProcessDeferredTransfers(o *types.PartialState, time_slot uint32, t []types.DeferredTransfer, pvmBackend string) (transferStats map[uint32]*transferStatistics, err error) {
	transferStats = make(map[uint32]*transferStatistics)
	for service, serviceAccount := range o.ServiceAccounts {
		gasUsed, transferCount, err := s.HostTransfer(serviceAccount, time_slot, uint32(service), t, pvmBackend)
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
