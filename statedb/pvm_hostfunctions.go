package statedb

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"sort"

	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/pvm/pvmtypes"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

const maxUint64 = ^uint64(0)

const (
	Debug_Service_Storage = false
)

const (
	g = 10
)

var (
	debugPeek = os.Getenv("PVM_DEBUG_PEEK") == "1"
)

type externalWriteTracker interface {
	TaintRecordExternalWrite(offset uint64, length uint64, reason string)
}

type traceBufferFlusher interface {
	FlushTraceBuffers()
}

type daFetcher interface {
	FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) ([]byte, error)
}

func getEVMStorage(hostenv types.HostEnv) (types.EVMJAMStorage, bool) {
	if env, ok := hostenv.(interface {
		EVMStorage() (types.EVMJAMStorage, bool)
	}); ok {
		return env.EVMStorage()
	}
	if env, ok := hostenv.(interface {
		GetStorage() types.JAMStorage
	}); ok {
		if evmStorage, ok := env.GetStorage().(types.EVMJAMStorage); ok {
			return evmStorage, true
		}
	}
	return nil, false
}

const (
	AccountStorageConst = 34 //[Gratis]
	AccountLookupConst  = 81 //[Gratis]
)

// FETCH_UBT operation types
const (
	FETCH_BALANCE   = 0
	FETCH_NONCE     = 1
	FETCH_CODE      = 2
	FETCH_CODE_HASH = 3
	FETCH_STORAGE   = 4
)

func (vm *VM) panic(errCode uint64) {
	vm.ResultCode = types.WORKDIGEST_PANIC
	vm.MachineState = PANIC
	vm.terminated = true
	vm.Fault_address = uint32(errCode)
}

type RefineM struct {
	P []byte `json:"P"`
	//U RAMInterface `json:"U"`
	I uint64 `json:"I"`
}

// GP-0.5 B.5
type Refine_parameters struct {
	Gas uint64
	//Ram                  *RAMInterface
	Register             []uint32
	Machine              map[uint32]*RefineM
	Export_segment       [][]byte
	Import_segement      [][]byte
	Export_segment_index uint32
	//service_index        uint32
	Delta map[uint32]uint32
	C_t   uint32
}

// InvokeHostCall handles host calls
// Returns true if the call results in a halt condition, otherwise false
func (vm *VM) InvokeHostCall(host_fn int) (bool, error) {

	rawGas := vm.GetGas()
	if rawGas < 0 {
		rawGas = 0
	}
	currentGas := uint64(rawGas)

	var gasUsed uint64
	if vm.InitialGas > 0 && vm.InitialGas >= currentGas {
		gasUsed = vm.InitialGas - currentGas
	}

	ok, err := vm.hostFunction(host_fn)
	if vm.MachineState == PANIC {
		vm.ExecutionVM.Panic(uint64(host_fn))
	}
	// Log after host function call to match javajam format
	vm.DebugHostFunction(host_fn, "Gas Used: %d, Gas Left: %d", gasUsed, currentGas)

	return ok, err
}

var RefineHostFunctionMap = map[int]func(*VM){
	GAS:               (*VM).hostGas,
	FETCH:             (*VM).hostFetch,
	HISTORICAL_LOOKUP: (*VM).hostHistoricalLookup,
	EXPORT:            (*VM).hostExport,
	MACHINE:           (*VM).hostMachine,
	PEEK:              (*VM).hostPeek,
	POKE:              (*VM).hostPoke,
	PAGES:             (*VM).hostPages,
	INVOKE:            (*VM).hostInvoke,
	EXPUNGE:           (*VM).hostExpunge,
	LOG:               (*VM).hostLog,
}

var AccumulateHostFunctionMap = map[int]func(*VM){
	GAS:        (*VM).hostGas,
	FETCH:      (*VM).hostFetch,
	READ:       (*VM).hostRead,
	WRITE:      (*VM).hostWrite,
	LOOKUP:     (*VM).hostLookup,
	INFO:       (*VM).hostInfo,
	BLESS:      (*VM).hostBless,
	ASSIGN:     (*VM).hostAssign,
	DESIGNATE:  (*VM).hostDesignate,
	CHECKPOINT: (*VM).hostCheckpoint,
	NEW:        (*VM).hostNew,
	UPGRADE:    (*VM).hostUpgrade,
	TRANSFER:   (*VM).hostTransfer,
	EJECT:      (*VM).hostEject,
	QUERY:      (*VM).hostQuery,
	SOLICIT:    (*VM).hostSolicit,
	FORGET:     (*VM).hostForget,
	YIELD:      (*VM).hostYield,
	PROVIDE:    (*VM).hostProvide,
	LOG:        (*VM).hostLog,
}

func (vm *VM) hostFunction(host_fn int) (bool, error) {
	// Handle Refine mode
	if vm.Mode == ModeRefine {
		// Try standard Refine host functions
		if fn, ok := RefineHostFunctionMap[host_fn]; ok {
			fn(vm)
			return true, nil
		}

		// Handle special UBT functions (253-255)
		switch host_fn {
		case FETCH_WITNESS: // 254
			vm.HostFetchWitness()
			return true, nil
		case FETCH_UBT: // 255
			vm.hostFetchUBT()
			return true, nil
		}

		// Unknown function in Refine mode
		vm.WriteRegister(7, WHAT)
		return true, nil
	}

	// Handle Accumulate mode
	if vm.Mode == ModeAccumulate {
		if fn, ok := AccumulateHostFunctionMap[host_fn]; ok {
			fn(vm)
			return true, nil
		}

		// Unknown function in Accumulate mode
		log.Warn(vm.logging, "Unknown Host Function in Accumulate Mode", "host_fn", host_fn)
		vm.WriteRegister(7, WHAT)
		return true, nil
	}

	// Unknown mode
	vm.WriteRegister(7, WHAT)
	return true, nil
}

// Information-on-Service (similar to hostRead)
func (vm *VM) hostInfo() {
	omega_7 := vm.ReadRegister(7)

	var fetch uint64
	if omega_7 == NONE {
		fetch = uint64(vm.Service_index)
	} else {
		fetch = omega_7
	}

	t, errCode := vm.getXUDS(fetch)
	if errCode != OK {
		vm.WriteRegister(7, NONE)
		vm.SetHostResultCode(NONE)
		log.Trace(vm.logging, "INFO NONE", "s", omega_7)
		return
	}

	bo := vm.ReadRegister(8)
	f := vm.ReadRegister(9)
	l := vm.ReadRegister(10)

	// [Gratis] Different encoding than serviceAccount; E(a_c, E8(a_b,a_t,a_g,a_m,a_o), E4(a_i), E8(a_f), E4(a_r,a_a,a_p))
	var buf bytes.Buffer
	elements := []interface{}{
		t.CodeHash,           // a_c E
		t.Balance,            // a_b E8
		t.ComputeThreshold(), // a_t E8
		t.GasLimitG,          // a_g E8
		t.GasLimitM,          // a_m E8
		t.StorageSize,        // a_o E8
		t.NumStorageItems,    // a_i E4
		t.GratisOffset,       // a_f E8
		t.CreateTime,         // a_r E4
		t.RecentAccumulation, // a_a E4
		t.ParentService,      // a_p E4
	}

	for _, elem := range elements {
		encoded, err := types.Encode(elem)
		if err != nil {
			vm.WriteRegister(7, NONE)
			vm.SetHostResultCode(NONE)
			log.Trace(vm.logging, "INFO NONE", "s", omega_7)
			return
		}
		buf.Write(encoded)
	}

	val := buf.Bytes()
	lenval := uint64(len(val))

	f_orig := f
	l_orig := l
	// Check if the ORIGINAL (unclamped) request is out of bounds - this should panic!
	if f_orig > lenval || f_orig+l_orig > lenval {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}

	// Gray Paper 0.7.1 (B.27): let f = min(φ₁₁, |v|), let l = min(φ₁₂, |v| − f)
	f = min(f, lenval)
	l = min(l, lenval-f)

	bytesToWrite := val[f : f+l]
	errcode := vm.WriteRAMBytes(uint32(bo), bytesToWrite)

	// Record taint for external write
	if tracker, ok := vm.ExecutionVM.(externalWriteTracker); ok {
		tracker.TaintRecordExternalWrite(uint64(bo), uint64(l), "PROVIDE")
	}

	if errcode != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}
	vm.DebugHostFunction(INFO, "bo=%x, f=%d, l=%d , v=0x%x", bo, f, l, val[f:f+l])

	bytesPreview := bytesToWrite
	if len(bytesPreview) > 96 {
		bytesPreview = bytesPreview[:96]
	}
	vm.WriteRegister(7, lenval)
	vm.SetHostResultCode(OK)
}

// Bless updates
func (vm *VM) hostBless() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	m := vm.ReadRegister(7)
	a := vm.ReadRegister(8)
	v := vm.ReadRegister(9)
	// 0.7.1 introduces RegistrarServiceID which is set in Bless
	r := vm.ReadRegister(10)
	o := vm.ReadRegister(11)
	n := vm.ReadRegister(12)
	//fmt.Printf("BLESS m=%d a=0x%x v=%d r=%d o=%d n=%d\n", m, a, v, r, o, n)
	bold_z := make(map[uint32]uint64)
	for i := 0; i < int(n); i++ {
		data, err := vm.ReadRAMBytes(uint32(o)+uint32(i)*12, 12)
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			log.Trace(vm.logging, "BLESS MEM VIOLATION 1", "o", o, "n", n, "i", i)
			vm.Panic(PANIC)
			return
		}
		s := binary.LittleEndian.Uint32(data[0:4])
		g := binary.LittleEndian.Uint64(data[4:12])
		bold_z[s] = g
	}
	bold_a := [types.TotalCores]uint32{}
	for i := 0; i < types.TotalCores; i++ {

		data, err := vm.ReadRAMBytes(uint32(a)+uint32(i)*4, 4)
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			vm.Panic(PANIC)
			log.Trace(vm.logging, "BLESS MEM VIOLATION 2", "o", o, "n", n, "i", i)
			return
		}
		bold_a[i] = binary.LittleEndian.Uint32(data)
	}
	set := []uint64{m, v, r}
	for _, id := range set {

		if id > (1<<32)-1 {
			vm.WriteRegister(7, WHO)
			vm.SetHostResultCode(WHO)
			log.Trace(vm.logging, "BLESS WHO", "m", fmt.Sprintf("%d", m), "a", fmt.Sprintf("%d", a), "v", fmt.Sprintf("%d", v))
			return
		} else {
			_, found, _ := vm.hostenv.GetService(uint32(id))
			if !found {
				vm.WriteRegister(7, WHO)
				vm.SetHostResultCode(WHO)
				log.Trace(vm.logging, "BLESS WHO", "m", fmt.Sprintf("%d", m), "a", fmt.Sprintf("%d", a), "v", fmt.Sprintf("%d", v))
			}
		}
	}

	// Set (x'p)_m, (x'p)_a, (x'p)_v
	vm.X.U.PrivilegedDirty = true
	vm.X.U.PrivilegedState.ManagerServiceID = uint32(m)
	vm.X.U.PrivilegedState.AuthQueueServiceID = bold_a
	vm.X.U.PrivilegedState.UpcomingValidatorsServiceID = uint32(v)
	vm.X.U.PrivilegedState.RegistrarServiceID = uint32(r)
	vm.X.U.PrivilegedState.AlwaysAccServiceID = bold_z

	vm.WriteRegister(7, OK)
	log.Trace(vm.logging, "BLESS OK", "m", fmt.Sprintf("%d", m), "a", fmt.Sprintf("%d", a), "v", fmt.Sprintf("%d", v), "r", fmt.Sprintf("%d", r), "o", o, "n", n)
	vm.SetHostResultCode(OK)
}

// Assign Core x_c[i]
func (vm *VM) hostAssign() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	c := vm.ReadRegister(7)
	o := vm.ReadRegister(8)
	a := vm.ReadRegister(9)

	q, errcode := vm.ReadRAMBytes(uint32(o), 32*types.MaxAuthorizationQueueItems)
	if errcode != OK {
		vm.WriteRegister(7, OOB)
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return
	}
	if c >= types.TotalCores {
		vm.WriteRegister(7, CORE)
		vm.SetHostResultCode(CORE)
		return
	}
	bold_q := make([]common.Hash, types.MaxAuthorizationQueueItems)
	for i := 0; i < types.MaxAuthorizationQueueItems; i++ {
		bold_q[i] = common.BytesToHash(q[i*32 : (i+1)*32])
	}
	xContext := vm.X
	xs, _ := xContext.GetX_s()
	privilegedService_a := vm.X.U.PrivilegedState.AuthQueueServiceID[c]
	if privilegedService_a != xs.ServiceIndex {
		vm.WriteRegister(7, HUH)
		vm.SetHostResultCode(HUH)
		log.Trace(vm.logging, "ASSIGN HUH", "c", c, "AuthQueueServiceID[c]", privilegedService_a, "xs", xs.ServiceIndex)
		return
	}

	copy(vm.X.U.QueueWorkReport[c][:], bold_q[:])
	vm.X.U.PrivilegedState.AuthQueueServiceID[c] = uint32(a)
	vm.X.U.QueueDirty = true

	log.Trace(vm.logging, "ASSIGN OK", "c", c, "AuthQueueServiceID[c]", a, "xs", xs.ServiceIndex)
	vm.WriteRegister(7, OK)
	vm.SetHostResultCode(OK)
}

// Designate validators
func (vm *VM) hostDesignate() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	o := vm.ReadRegister(7)
	v, errCode := vm.ReadRAMBytes(uint32(o), 336*types.TotalValidators)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}
	xContext := vm.X
	xs, _ := xContext.GetX_s()
	privilegedService_v := vm.X.U.PrivilegedState.UpcomingValidatorsServiceID
	if privilegedService_v != xs.ServiceIndex {
		vm.WriteRegister(7, HUH)
		vm.SetHostResultCode(HUH)
		log.Trace(vm.logging, "DESIGNATE HUH", "UpcomingValidatorsServiceID", privilegedService_v, "xs", xs.ServiceIndex)
		return
	}
	v_bold := make([]types.Validator, types.TotalValidators)
	for i := 0; i < types.TotalValidators; i++ {
		keys := v[i*336 : (i+1)*336]
		newv := types.Validator{}
		copy(newv.Bandersnatch[:], keys[0:32])
		copy(newv.Ed25519[:], keys[32:32+32])
		copy(newv.Bls[:], keys[64:64+144])
		copy(newv.Metadata[:], keys[64+144:])

		v_bold[i] = newv
	}
	vm.X.U.UpcomingValidators = v_bold
	vm.X.U.UpcomingDirty = true

	log.Trace(vm.logging, "DESIGNATE OK", "UpcomingValidatorsServiceID", privilegedService_v, "validatorsLen", len(v_bold), "TotalValidators", types.TotalValidators, "xs", xs.ServiceIndex)
	vm.WriteRegister(7, OK)
	vm.SetHostResultCode(OK)
}

// Checkpoint gets Gas-remaining
func (vm *VM) hostCheckpoint() {
	vm.Y = vm.X.Clone()
	vm.Y.U.Checkpoint()
	vm.WriteRegister(7, uint64(vm.GetGas())) // CHECK
	log.Trace(vm.logging, "CHECKPOINT", "g", fmt.Sprintf("%d", vm.GetGas()))
	vm.SetHostResultCode(OK)
}

// implements https://graypaper.fluffylabs.dev/#/5f542d7/313103313103
func new_check(i uint32, u_d map[uint32]*types.ServiceAccount) uint32 {
	bump := uint32(1)

	// Define S = 2^16 = 65536, the Minimum Public Service Index
	minPubserviceIdx := uint32(types.MinPubServiceIndex) // 65536

	// The range size is R = 2^32 - 2^8 - S, 4294967296 - 256 - 65536, or 4294901504
	serviceIndexRangeSize := uint32(4294967040) - minPubserviceIdx

	if i < minPubserviceIdx {
		i = minPubserviceIdx
	}
	for {
		if _, ok := u_d[i]; !ok {
			return i
		}
		offset := i - minPubserviceIdx
		nextOffset := (offset + bump) % serviceIndexRangeSize
		i = minPubserviceIdx + nextOffset
	}
}

// New service
func (vm *VM) hostNew() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	xContext := vm.X
	xs, _ := xContext.GetX_s()

	// put 'g' and 'm' together
	o := vm.ReadRegister(7)
	c, errCode := vm.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		log.Trace(vm.logging, "hostNew: MEM VIOLATION reading code hash", "o", o, "service_index", xs.ServiceIndex)
		return
	}
	l := vm.ReadRegister(8)
	g := vm.ReadRegister(9)
	m := vm.ReadRegister(10)
	f := vm.ReadRegister(11)
	vm.DebugHostFunction(NEW, "l=%d, g=%d, m=%d, f=%d", l, g, m, f)
	// in 0.7.1 this "i" is used with the registrar to choose serviceIDs < 64K https://graypaper.fluffylabs.dev/#/1c979cb/36da0336da03?v=0.7.1
	// *** TODO: MC to review Small serviceIDs < 64K with registrar below
	i := vm.ReadRegister(12)
	x_s_t := xs.ComputeThreshold()
	privilegedService_m := vm.X.U.PrivilegedState.ManagerServiceID
	if privilegedService_m != xs.ServiceIndex && f != 0 {
		// only ManagerServiceID can bestow gratis
		vm.WriteRegister(7, HUH)
		vm.SetHostResultCode(HUH)
		log.Trace(vm.logging, "hostNew: HUH", "ManagerServiceID", privilegedService_m, "xs", xs.ServiceIndex)
		return
	}
	if xs.Balance < x_s_t {
		vm.WriteRegister(7, CASH)
		vm.SetHostResultCode(CASH) //balance insufficient
		log.Trace(vm.logging, "hostNew: NEW CASH xs.Balance < x_s_t", "xs.Balance", xs.Balance, "x_s_t", x_s_t, "x_s_index", xs.ServiceIndex)
		return
	}

	x_e_r := xContext.U.PrivilegedState.RegistrarServiceID
	_, alreadyInservice := xContext.U.ServiceAccounts[uint32(i)]
	if x_e_r == xs.ServiceIndex && i < types.MinPubServiceIndex && alreadyInservice {
		vm.WriteRegister(7, FULL)
		vm.SetHostResultCode(FULL)
		log.Trace(vm.logging, "hostNew: NEW FULL", "i", i, "RegistrarServiceID", x_e_r, "xs", xs.ServiceIndex)
		return
	}
	a := &types.ServiceAccount{}
	var newServiceIndex uint32

	// selected service index (registrar privilege for small indices)
	if x_e_r == xs.ServiceIndex && i < types.MinPubServiceIndex {
		newServiceIndex = uint32(i)
		a = types.NewEmptyServiceAccount(
			newServiceIndex,
			c,
			uint64(g),
			uint64(m),
			uint64(AccountLookupConst+l),
			uint64(f),
			vm.Timeslot,
			xs.ServiceIndex,
		)
	} else { // auto-select service index
		// xs has enough balance to fund service creation of a AND covering its own threshold

		const bump = uint32(42)
		const minPubserviceIdx = uint32(types.MinPubServiceIndex) // 65536
		const serviceIndexRangeSize = uint32(4294901504)          // 2^32 - 2^8 - S(65536)

		xi := xContext.NewServiceIndex
		if xi < minPubserviceIdx {
			xi = minPubserviceIdx
		} else {
			xi = minPubserviceIdx + ((xi - minPubserviceIdx) % serviceIndexRangeSize)
		}
		// update the new service index in x_i = check(S + (xi - S + 42) mod (2^42 - S - 2^8))
		newServiceIndex = new_check(xi, xContext.U.ServiceAccounts)
		// simulate a with c, g, m
		// [Gratis] a_r:t; a_f,a_a:0; a_p:x_s
		a = types.NewEmptyServiceAccount(
			newServiceIndex,
			c,
			uint64(g),
			uint64(m),
			uint64(AccountLookupConst+l),
			uint64(f),
			vm.Timeslot,
			xs.ServiceIndex,
		)
		// GP 0.7.1: x_i' = check(S + (x_i - S + 42) mod (2^32 - S - 2^8))
		offset := (newServiceIndex - minPubserviceIdx + bump) % serviceIndexRangeSize
		to_check := minPubserviceIdx + offset
		xContext.NewServiceIndex = new_check(to_check, xContext.U.ServiceAccounts)
	}
	a.ALLOW_MUTABLE()
	a.Balance = a.ComputeThreshold()
	// Guard against underflow: ensure xs has enough balance for new service's threshold
	if a.Balance > xs.Balance || (xs.Balance-a.Balance) < x_s_t {
		vm.WriteRegister(7, CASH)
		vm.SetHostResultCode(CASH)
		log.Trace(vm.logging, "hostNew: CASH insufficient balance for new service", "xs.Balance", xs.Balance, "a.Balance", a.Balance, "x_s_t", x_s_t, "would_underflow", a.Balance > xs.Balance)
		return
	}
	xs.DecBalance(a.Balance) // (x's)b <- (xs)b - at
	newServiceIndex = a.ServiceIndex
	a.WriteLookup(common.BytesToHash(c), uint32(l), []uint32{}, "memory")

	xContext.U.ServiceAccounts[newServiceIndex] = a // this new account is included but only is written if (a) non-exceptional (b) exceptional and checkpointed
	vm.WriteRegister(7, uint64(newServiceIndex))
	vm.SetHostResultCode(OK)
	log.Trace(vm.logging, "NEW OK", "SERVICE", fmt.Sprintf("%d", newServiceIndex), "code_hash_ptr", fmt.Sprintf("%x", o), "code_hash_ptr", fmt.Sprintf("%x", c), "code_len", l, "min_item_gas", g, "min_memo_gas", m)
}

// Upgrade service
func (vm *VM) hostUpgrade() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		log.Info(vm.logging, "UPGRADE WHAT mode", "mode", vm.Mode)
		return
	}

	xContext := vm.X
	xs, _ := xContext.GetX_s()
	o := vm.ReadRegister(7)
	g := vm.ReadRegister(8)
	m := vm.ReadRegister(9)

	c, errCode := vm.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		log.Info(vm.logging, "UPGRADE PANIC", "mode", vm.Mode)
		return
	}

	xs.Dirty = true
	xs.CodeHash = common.BytesToHash(c)
	xs.GasLimitG = g
	xs.GasLimitM = m
	vm.WriteRegister(7, OK)
	// xContext.D[s] = xs // not sure if this is needed
	log.Info(vm.logging, "UPGRADE OK", "code_hash", fmt.Sprintf("%x", o), "code_hash_ptr", fmt.Sprintf("%x", c), "min_item_gas", g, "min_memo_gas", m)
	vm.SetHostResultCode(OK)
}

// Transfer host call
func (vm *VM) hostTransfer() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	d := vm.ReadRegister(7)
	a := vm.ReadRegister(8)
	g := vm.ReadRegister(9)
	o := vm.ReadRegister(10)
	xs, _ := vm.X.GetX_s()
	m, errCode := vm.ReadRAMBytes(uint32(o), M)
	log.Trace(vm.logging, "TRANSFER attempt", "d", d, "a", a, "g", g, "o", o)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		log.Trace(vm.logging, "TRANSFER PANIC", "d", d)
		return
	}
	receiver, _ := vm.getXUDS(d)
	if receiver == nil {
		vm.WriteRegister(7, WHO)
		vm.SetHostResultCode(WHO)
		log.Trace(vm.logging, "TRANSFER WHO", "d", d)
		return
	}
	log.Trace(vm.logging, "TRANSFER receiver", "g", g, "receiver.GasLimitM", receiver.GasLimitM, "receiver.ServiceIndex", receiver.ServiceIndex)
	if g < receiver.GasLimitM {
		vm.WriteRegister(7, LOW)
		vm.SetHostResultCode(LOW)
		log.Trace(vm.logging, "TRANSFER LOW", "g", g, "GasLimitM", receiver.GasLimitM)
		return
	}

	// against underflow: if a > xs.Balance, the subtraction would underflow
	threshold := xs.ComputeThreshold()
	if a > xs.Balance || (xs.Balance-a) < threshold {
		vm.WriteRegister(7, CASH)
		vm.SetHostResultCode(CASH)
		log.Trace(vm.logging, "TRANSFER CASH", "amount", a, "xs.Balance", xs.Balance, "threshold", threshold, "would_underflow", a > xs.Balance)
		return
	}
	var memo [M]byte
	copy(memo[:], m)
	t := types.DeferredTransfer{
		Amount:        a,
		GasLimit:      g,
		SenderIndex:   vm.X.ServiceIndex,
		Memo:          memo,
		ReceiverIndex: uint32(d),
	} // CHECK
	xs.DecBalance(a)
	receiver.ALLOW_MUTABLE() // make sure all service accounts can be written

	copy(t.Memo[:], m[:])
	vm.X.Transfers = append(vm.X.Transfers, t)
	log.Trace(vm.logging, "TRANSFER OK", "g", g, "sender", fmt.Sprintf("%d", t.SenderIndex), "receiver", fmt.Sprintf("%d", d), "amount", fmt.Sprintf("%d", a), "gaslimit", g, "x_s_bal", xs.Balance, "DeferredTransfer", t.String())
	vm.WriteRegister(7, OK)
	vm.SetHostResultCode(OK)
}

// Gas Service
func (vm *VM) hostGas() {
	gasCost := int64(0)                              // Define gas cost.TODO: check 0 vs 10 here
	vm.WriteRegister(7, uint64(vm.GetGas()-gasCost)) // its gas remaining AFTER the host call
	//vm.register[7] = uint64(1234567) // TEMPORARY
	vm.SetHostResultCode(OK)
}

func (vm *VM) hostQuery() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	o := vm.ReadRegister(7)
	z := vm.ReadRegister(8)
	h, errCode := vm.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}
	a, _ := vm.X.GetX_s()
	account_lookuphash := common.BytesToHash(h)
	ok, anchor_timeslot, lookup_source := a.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		vm.WriteRegister(7, NONE)
		vm.WriteRegister(8, 0)
		vm.SetHostResultCode(NONE)
		log.Trace(vm.logging, "QUERY NONE", "h", account_lookuphash, "z", z, "lookup_source", lookup_source)
		return
	}
	switch len(anchor_timeslot) {
	case 0:
		vm.WriteRegister(7, 0)
		vm.WriteRegister(8, 0)
	case 1:
		x := anchor_timeslot[0]
		vm.WriteRegister(7, 1+(1<<32)*uint64(x))
		vm.WriteRegister(8, 0)
		log.Trace(vm.logging, "QUERY 1", "x", x)
	case 2:
		x := anchor_timeslot[0]
		y := anchor_timeslot[1]
		vm.WriteRegister(7, 2+(1<<32)*uint64(x))
		vm.WriteRegister(8, uint64(y))
		log.Trace(vm.logging, "QUERY 2", "x", x, "y", y)
	case 3:
		x := anchor_timeslot[0]
		y := anchor_timeslot[1]
		z := anchor_timeslot[2]
		log.Trace(vm.logging, "QUERY 3", "x", x, "y", y, "z", z)
		vm.WriteRegister(7, 3+(1<<32)*uint64(x))
		vm.WriteRegister(8, uint64(y)+(1<<32)*uint64(z))
	}
	w7 := vm.ReadRegister(7)
	w8 := vm.ReadRegister(8)
	log.Trace(vm.logging, "QUERY OK", "h", account_lookuphash, "z", z, "w7", w7, "w8", w8, "len(anchor_timeslot)", len(anchor_timeslot))
	vm.SetHostResultCode(OK)
}

// https://graypaper.fluffylabs.dev/#/7e6ff6a/323800323800?v=0.6.7
func (vm *VM) hostFetch() {
	o := vm.ReadRegister(7)
	omega_8 := vm.ReadRegister(8)
	omega_9 := vm.ReadRegister(9)
	datatype := vm.ReadRegister(10)
	omega_11 := vm.ReadRegister(11)
	omega_12 := vm.ReadRegister(12)
	var v_Bytes []byte
	mode := vm.Mode
	allowed := false

	const (
		PARAMETER_BYTE_0                              = 0  // 0: Parameter Bytes
		ENTROPY_1                                     = 1  // 1: The Entropy/Random Accumulator (n)
		AUTHORIZATION_2                               = 2  // 2: The Authorization Trace/Data (r)
		EXTRINSIC_BY_WORK_PACKAGE_EXTRINSIC_INDEX_3   = 3  // 3: Specific Extrinsic by Work Package Index (x[φ11]φ12)
		EXTRINSICS_BY_WORK_ITEM_4                     = 4  // 4: Extrinsics by Current Work Item Index (x[i]φ11)
		IMPORTED_SEGMENT_BY_WORK_ITEM_SEGMENT_INDEX_5 = 5  // 5: Specific Imported Segment by Work Package Index (i[φ11]φ12)
		IMPORTED_SEGMENT_BY_WORK_ITEM_INDEX_6         = 6  // 6: Imported Segment by Current Work Item Index (i[i]φ11)
		WORK_PACKAGE_7                                = 7  // 7: Encoded Work Package (E(p))
		AUTH_CODE_AND_CONFIG_BLOB_8                   = 8  // 8: Authorization Code Hash + Configuration Blob (E(pu, ↕pf))
		AUTHORIZATION_TOKEN_9                         = 9  // 9: Authorization Token (pj)
		REFINE_CONTEXT_10                             = 10 // 10: Encoded Refine Context (E(pc))
		ALL_WORK_ITEMS_11                             = 11 // 11: All Work Items (Encoded using Custom Encoding S, E(↕[S(w) ∣ w <− pw]))
		SPECIFIC_WORK_ITEM_S_ENCODED_12               = 12 // 12: Specific Work Item S-Encoding (S(pw[φ11]))
		WORK_ITEM_PAYLOAD_13                          = 13 // 13: Specific Work Item Payload (pw[φ11]y)
		ALL_ACCUMULATION_OPERANDS_14                  = 14 // 14: All Accumulation Operands (E(↕o))
		SPECIFIC_ACCUMULATION_OPERAND_15              = 15 // 15: Specific Accumulation Operand (E(o[φ11]))
	)

	// determine if allowed
	// datatype:
	switch mode {
	case ModeIsAuthorized:
		switch datatype {
		//0, 7, 8, 9, 10, 11, 12, 13
		case PARAMETER_BYTE_0, WORK_PACKAGE_7, AUTH_CODE_AND_CONFIG_BLOB_8, AUTHORIZATION_TOKEN_9, REFINE_CONTEXT_10, ALL_WORK_ITEMS_11, SPECIFIC_WORK_ITEM_S_ENCODED_12, WORK_ITEM_PAYLOAD_13:
			allowed = true
		default:
			allowed = false
		}
	case ModeRefine:
		switch datatype {
		//0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13
		case PARAMETER_BYTE_0, ENTROPY_1, AUTHORIZATION_2, EXTRINSIC_BY_WORK_PACKAGE_EXTRINSIC_INDEX_3, EXTRINSICS_BY_WORK_ITEM_4, IMPORTED_SEGMENT_BY_WORK_ITEM_SEGMENT_INDEX_5, IMPORTED_SEGMENT_BY_WORK_ITEM_INDEX_6, WORK_PACKAGE_7, AUTH_CODE_AND_CONFIG_BLOB_8, AUTHORIZATION_TOKEN_9, REFINE_CONTEXT_10, ALL_WORK_ITEMS_11, SPECIFIC_WORK_ITEM_S_ENCODED_12, WORK_ITEM_PAYLOAD_13:
			allowed = true
		default:
			allowed = false
		}
	case ModeAccumulate:
		switch datatype {
		//0, 1, 14, 15, 250
		case PARAMETER_BYTE_0, ENTROPY_1, ALL_ACCUMULATION_OPERANDS_14, SPECIFIC_ACCUMULATION_OPERAND_15:
			allowed = true
		default:
			allowed = false
		}
	}
	log.Info(vm.logging, "FETCH", "mode", mode, "allowed", allowed, "datatype", datatype, "omega_7", o, "omega_8", omega_8, "omega_9", omega_9, "omega_11", omega_11, "omega_12", omega_12)

	if allowed {
		switch datatype {

		case PARAMETER_BYTE_0:
			var err error
			v_Bytes, err = types.ParameterBytes()
			//0a00000000000000010000000000000064000000000000000200200000000c000000809698000000000080f0fa020000000000ca9a3b00000000002d3101000000000800100008000300403800000300080006005000040080000500060000fa0000017cd20000093d0004000000000c00000204000000c0000080000000000c00000a000000
			//[8: B_I]        [8: B_L].       [8: B_S].       [C.][4:D   ][4:E   ][8:G_A.        ][8:G_I.        ][8:G_R         ][8:G_T*        ][H ][I ][J ][K ][4:L   ][N ][O ][P ][Q ][R ][T ][U ][V ][4:W_A ][4:W_B ][4:W_C ][4:W_E ][4:W_M ][4:W_P ][4:W_R*][4:W_T ][4:W_X ][4:Y.  ]
			if err != nil {
				log.Error(vm.logging, "ParameterBytes() failed", "error", err)
			}
			log.Trace(vm.logging, "ParameterBytes result", "len", len(v_Bytes), "bytes", hex.EncodeToString(v_Bytes))

		case ENTROPY_1: // n
			v_Bytes = vm.N.Bytes()

		case AUTHORIZATION_2: // r
			v_Bytes = vm.Authorization

		case EXTRINSIC_BY_WORK_PACKAGE_EXTRINSIC_INDEX_3: // a SPECIFIC extrinsic of a work item -- note that this does NOT have a variable-length prefix
			extrinsic_number := omega_12
			if int(extrinsic_number) < len(vm.Extrinsics) {
				v_Bytes = vm.Extrinsics[extrinsic_number]
				fmt.Printf("hostFetch case 3: Extrinsic # %d length %d\n", extrinsic_number, len(v_Bytes))
			} else {
				log.Warn(vm.logging, "FETCH case 3: extrinsic_number out of range", "extrinsic_number", extrinsic_number, "len(Extrinsics)", len(vm.Extrinsics))
			}
		case EXTRINSICS_BY_WORK_ITEM_4: // ALL extrinsics of a work item -- note that this has a variable-length prefix
			if len(vm.Extrinsics) > 0 {
				v_Bytes, _ = types.Encode(vm.Extrinsics)
			} else {
				v_Bytes = []byte{0}
			}
			fmt.Printf("hostFetch case 4: Extrinsics length %d %x\n", len(v_Bytes), v_Bytes)
		case IMPORTED_SEGMENT_BY_WORK_ITEM_SEGMENT_INDEX_5: // a SPECIFIC imported segment of a work item -- not that this does not have a variable length prefix
			workItem := omega_11
			segmentIndex := omega_12
			if vm.Imports != nil {
				if int(workItem) < len(vm.Imports) {
					segments := vm.Imports[workItem]
					if int(segmentIndex) < len(segments) {
						v_Bytes = segments[segmentIndex]
					}
				}
			}
		case IMPORTED_SEGMENT_BY_WORK_ITEM_INDEX_6:
			// get imported segment by work item index
			if omega_11 < uint64(len(vm.Imports[vm.WorkItemIndex])) {
				v_Bytes = append([]byte{}, vm.Imports[vm.WorkItemIndex][omega_11][:]...)

			} else {
				// fmt.Printf("FETCH 6 FAIL omega_11 %d vs len(vm.Imports[vm.WorkItemIndex=%d])=%d\n", omega_11, vm.WorkItemIndex, len(vm.Imports[vm.WorkItemIndex]))
			}

		case WORK_PACKAGE_7: // encode work package
			v_Bytes, _ = types.Encode(vm.WorkPackage)
			//log.Trace(vm.logging, "FETCH wp", "len(v_Bytes)", len(v_Bytes))

		case AUTH_CODE_AND_CONFIG_BLOB_8: // p_f
			v_Bytes = vm.WorkPackage.ConfigurationBlob
			//log.Trace(vm.logging, "FETCH p_u + | p_p", "p_u", vm.WorkPackage.AuthorizationCodeHash, "p_p", vm.WorkPackage.ConfigurationBlob, "len", len(v_Bytes))

		case AUTHORIZATION_TOKEN_9: // p_j
			v_Bytes = vm.WorkPackage.AuthorizationToken

		case REFINE_CONTEXT_10: // p_X (refine context)
			v_Bytes = vm.WorkPackage.RefineContext.SerializeRefineContext()

		case ALL_WORK_ITEMS_11: // all work items
			v_Bytes = make([]byte, 0)
			// TODO: add discriminator in front
			for i, w := range vm.WorkPackage.WorkItems {
				fmt.Printf("WorkItem %d: %v\n", i, types.ToJSONHex(w))
				s_bytes, _ := w.EncodeS() // THIS IS CUSTOM ENCODING
				v_Bytes = append(v_Bytes, s_bytes...)
			}

		case SPECIFIC_WORK_ITEM_S_ENCODED_12: // S(w) for specific work item w_11
			if omega_11 < uint64(len(vm.WorkPackage.WorkItems)) {
				w := vm.WorkPackage.WorkItems[omega_11]
				v_Bytes, _ = types.Encode(w)
			}
			break

		case WORK_ITEM_PAYLOAD_13: // p_w[w_11]_y
			if omega_11 < uint64(len(vm.WorkPackage.WorkItems)) {
				w := vm.WorkPackage.WorkItems[omega_11]
				v_Bytes = w.Payload
				log.Trace(vm.logging, "FETCH p_w[w_11]_y", "w_11", omega_11, "payload", fmt.Sprintf("%x", v_Bytes), "len", len(v_Bytes))
			}

		case ALL_ACCUMULATION_OPERANDS_14: // E(|o) all accumulation operands
			if vm.AccumulateInputs != nil {
				// CHECK: these should be encoded with the # of inputs, then a byte discriminator in front to indicate transfer vs accum operand (0 vs 1)
				v_Bytes, _ = types.Encode(vm.AccumulateInputs)
			} else {
				v_Bytes = []byte{0}
			}

		case SPECIFIC_ACCUMULATION_OPERAND_15: // E(o[w_11])
			if vm.AccumulateInputs != nil && omega_11 < uint64(len(vm.AccumulateInputs)) {
				// CHECK: these should a byte discriminator in front to indicate transfer vs accum operand (0 vs 1)
				v_Bytes, _ = types.Encode(vm.AccumulateInputs[omega_11])
				log.Trace(vm.logging, "FETCH E(o[w_11])", "w_11", omega_11, "v_Bytes", fmt.Sprintf("%x", v_Bytes), "len", len(v_Bytes))
			}

		}
	} else {
		log.Trace(vm.logging, "FETCH FAIL NOT ALLOWED", "mode", mode, "allowed", allowed, "datatype", datatype, "omega_7", o, "omega_8", omega_8, "omega_9", omega_9, "omega_11", omega_11, "omega_12", omega_12)
	}

	if v_Bytes == nil {
		log.Trace(vm.logging, "FETCH FAIL v_Bytes==nil", "mode", mode, "allowed", allowed, "datatype", datatype, "omega_7", o, "omega_8", omega_8, "omega_9", omega_9, "omega_11", omega_11, "omega_12", omega_12)
		vm.WriteRegister(7, NONE)
		vm.SetHostResultCode(NONE)
		return
	}

	f := min(uint64(len(v_Bytes)), omega_8)   // offset
	l := min(uint64(len(v_Bytes))-f, omega_9) // max length
	vm.DebugHostFunction(FETCH, "datatype = %d, o=%x, f=%d, l=%d , v=0x%x", datatype, o, f, l, v_Bytes[f:f+l])
	errCode := vm.WriteRAMBytes(uint32(o), v_Bytes[f:f+l])

	// Record taint for external write
	if tracker, ok := vm.ExecutionVM.(externalWriteTracker); ok {
		tracker.TaintRecordExternalWrite(uint64(o), l, "FETCH")
	}

	if errCode != OK {
		vm.Panic(errCode)
		log.Trace(vm.logging, "FETCH FAIL", "datatype", datatype, "o", o, "v_Bytes", fmt.Sprintf("%x", v_Bytes), "l", l, "f", f, "f+l", f+l, "v_Bytes[f..f+l]", fmt.Sprintf("%x", v_Bytes[f:f+l]))
		return
	}
	dataPreview := v_Bytes[f : f+l]
	if len(dataPreview) > 160 {
		dataPreview = dataPreview[:160]
	}

	vm.WriteRegister(7, uint64(len(v_Bytes)))
}

func (vm *VM) hostYield() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	o := vm.ReadRegister(7)
	h, errCode := vm.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.Panic(errCode)

		return
	}
	y := common.BytesToHash(h)
	vm.X.Yield = y
	vm.WriteRegister(7, OK)
	log.Trace(vm.logging, "YIELD OK", "h", y)
	vm.SetHostResultCode(OK)
}

func (vm *VM) hostProvide() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	omega_7 := vm.ReadRegister(7)
	o := vm.ReadRegister(8)
	z := vm.ReadRegister(9)
	i, errCode := vm.ReadRAMBytes(uint32(o), uint32(z))
	if errCode != OK {
		vm.Panic(errCode)

		return
	}
	i = slices.Clone(i)
	if omega_7 == NONE {
		omega_7 = uint64(vm.Service_index)
	}

	var a *types.ServiceAccount
	a, _ = vm.getXUDS(omega_7)

	if a == nil {
		vm.WriteRegister(7, WHO)
		vm.SetHostResultCode(WHO)
		log.Trace(vm.logging, "PROVIDE WHO", "omega_7", omega_7)
		return
	}

	h := common.Blake2Hash(i)
	ok, X_s_l, lookup_source := a.ReadLookup(h, uint32(z), vm.hostenv)
	if !(len(X_s_l) == 0) || !ok {
		vm.WriteRegister(7, HUH)
		vm.SetHostResultCode(HUH)
		log.Trace(vm.logging, "PROVIDE HUH", "omega_7", omega_7, "h", h, "z", z, "lookup_source", lookup_source)
		return
	}

	exists := false
	for _, p := range vm.X.Provided {
		if p.ServiceIndex == a.ServiceIndex && bytes.Equal(p.P_data, i) {
			exists = true
			break
		}
	}

	if exists {
		vm.WriteRegister(7, HUH)
		vm.SetHostResultCode(HUH)
		log.Trace(vm.logging, "PROVIDE HUH", "omega_7", omega_7, "h", h, "z", z)
		return
	}

	vm.X.Provided = append(vm.X.Provided, types.Provided{
		ServiceIndex: a.ServiceIndex,
		P_data:       i,
	})

	vm.WriteRegister(7, OK)
	log.Trace(vm.logging, "PROVIDE OK", "omega_7", omega_7, "h", h, "z", z)
	vm.SetHostResultCode(OK)
}

func (vm *VM) hostEject() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		log.Trace(vm.logging, "EJECT WHAT - wrong mode", "mode", vm.Mode)
		return
	}

	d := vm.ReadRegister(7)
	o := vm.ReadRegister(8)
	h, err := vm.ReadRAMBytes(uint32(o), 32)
	if err != OK {
		vm.Panic(err)
		log.Trace(vm.logging, "EJECT PANIC - ReadRAMBytes failed", "err", err)
		return
	}
	vm.DebugHostFunction(EJECT, "h 0x%x from %o", h, o)

	xContext := vm.X
	bold_d, errCode := vm.getXUDS(d)
	if errCode != OK || bold_d.DeletedAccount {
		vm.WriteRegister(7, WHO)
		vm.SetHostResultCode(WHO)
		log.Trace(vm.logging, "EJECT WHO - getXUDS failed", "d", d, "errCode", errCode)
		return
	}
	tst := common.Hash(types.E_l(uint64(vm.X.ServiceIndex), 32))
	if d == uint64(xContext.ServiceIndex) || !bytes.Equal(tst.Bytes(), bold_d.CodeHash.Bytes()) {
		vm.WriteRegister(7, WHO)
		vm.SetHostResultCode(WHO)
		log.Trace(vm.logging, "EJECT WHO -- cannot eject self", "d", fmt.Sprintf("%d", d), "vm.X.ServiceIndex", fmt.Sprintf("%d", vm.X.ServiceIndex))
		return
	}

	if errCode != OK {
		vm.WriteRegister(7, WHO)
		vm.SetHostResultCode(WHO)
		log.Trace(vm.logging, "EJECT WHO - getXUDS failed", "d", d, "errCode", errCode)
		return
	}
	//fmt.Printf("EJECT: x_s.ServiceIndex=%d bold_d.ServiceIndex=%d e32(s)=%s d.CodeHash=%s\n", vm.X.ServiceIndex, bold_d.ServiceIndex, tst, bold_d.CodeHash)
	l := max(AccountLookupConst, bold_d.StorageSize) - AccountLookupConst
	ok, D_lookup, lookup_source := bold_d.ReadLookup(common.BytesToHash(h), uint32(l), vm.hostenv)

	if !ok || bold_d.NumStorageItems != 2 {
		vm.WriteRegister(7, HUH)
		vm.SetHostResultCode(HUH)
		log.Trace(vm.logging, "EJECT HUH - conditions not met",
			"d", fmt.Sprintf("%d", d),
			"h", h,
			"l", l,
			"lookup_ok", ok,
			"NumStorageItems", bold_d.NumStorageItems,
			"CodeHashMatch", bytes.Equal(tst.Bytes(), bold_d.CodeHash.Bytes()),
			"lookup_source", lookup_source)
		return
	}

	// Check if D_lookup has any elements and use the last element for expiry check
	if len(D_lookup) > 0 {
		lastLookupTimeslot := D_lookup[len(D_lookup)-1]
		expiryTimeslot := lastLookupTimeslot + uint32(types.PreimageExpiryPeriod)

		if expiryTimeslot < vm.Timeslot {
			// credit balance AFTER expiry check succeeds
			xs, _ := xContext.GetX_s()
			xs.IncBalance(bold_d.Balance)

			vm.WriteRegister(7, OK)
			vm.SetHostResultCode(OK)
			xContext.U.ServiceAccounts[uint32(d)] = bold_d
			bold_d.DeletedAccount = true
			bold_d.Mutable = true
			log.Trace(vm.logging, "EJECT OK", "d", fmt.Sprintf("%d", d))
			blobHash := common.BytesToHash(h)
			bold_d.WriteLookup(blobHash, uint32(l), nil, "trie") // nil means delete the lookup
			bold_d.WritePreimage(blobHash, []byte{}, "trie")     // []byte{} means delete the preimage. TODO: should be preimage_source
			return
		}
	}

	vm.WriteRegister(7, HUH)
	vm.SetHostResultCode(HUH)
	log.Trace(vm.logging, "EJECT HUH - expiry condition not met",
		"len(D_lookup)", len(D_lookup),
		"D_lookup", D_lookup,
		"timeslot", vm.Timeslot)
}

// Invoke
func (vm *VM) hostInvoke() {

	if vm.Mode != ModeRefine {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	n := vm.ReadRegister(7)
	o := vm.ReadRegister(8)

	gasBytes, errCodeGas := vm.ReadRAMBytes(uint32(o), 8)
	if errCodeGas != OK {
		vm.Panic(errCodeGas)
		return
	}

	g := types.DecodeE_l(gasBytes)
	log.Trace(vm.logging, "INVOKE", "n", n, "o", o, "g", g)
	m_n, ok := vm.VMs[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		log.Trace(vm.logging, "INVOKE WHO", "n", n)
		vm.SetHostResultCode(WHO)
		return
	}
	new_machine := *m_n
	for i := 1; i < 14; i++ {
		reg_bytes, errCodeReg := vm.ReadRAMBytes(uint32(o)+8*uint32(i), 8)
		if errCodeReg != OK {
			vm.Panic(errCodeReg)
			return
		}
		// set the register
		regVal := types.DecodeE_l(reg_bytes)
		new_machine.WriteRegister(i-1, regVal)
	}

	initGas := int64(g)
	new_machine.SetGas(initGas)
	pc := new_machine.GetPC()

	// Flush parent trace buffers before invoke
	if flusher, ok := vm.ExecutionVM.(traceBufferFlusher); ok {
		flusher.FlushTraceBuffers()
	}

	new_machine.ExecuteAsChild(uint32(pc))

	// Flush child trace buffers after invoke
	if flusher, ok := new_machine.(traceBufferFlusher); ok {
		flusher.FlushTraceBuffers()
	}

	postGas := new_machine.GetGas()
	gasUsed := initGas - postGas
	log.Info(vm.logging, "INVOKE: gas used", "n", n, "o", o, "g", g, "postGas", postGas, "gasUsed", gasUsed, "pc", pc)
	gasVal := new_machine.GetGas()
	gasBytes = types.E_l(uint64(gasVal), 8)
	errCodeGas = vm.WriteRAMBytes(uint32(o), gasBytes)

	// Record taint for external write (gas)
	if tracker, ok := vm.ExecutionVM.(externalWriteTracker); ok {
		tracker.TaintRecordExternalWrite(uint64(o), 8, "INVOKE_GAS")
	}

	if errCodeGas != OK {
		vm.Panic(errCodeGas)
		return
	}

	for i := 1; i < 14; i++ {
		regVal := new_machine.ReadRegister(i - 1)
		reg_bytes := types.E_l(regVal, 8)
		errCode := vm.WriteRAMBytes(uint32(o)+8*uint32(i), reg_bytes)

		// Record taint for external write (register)
		if tracker, ok := vm.ExecutionVM.(externalWriteTracker); ok {
			tracker.TaintRecordExternalWrite(uint64(o)+8*uint64(i), 8, "INVOKE_REG")
		}

		if errCode != OK {
			vm.Panic(errCode)
			return
		}
	}

	//TODO: who

	//if new_machine.hostCall {
	//vm.WriteRegister(7, HOST)
	//vm.WriteRegister(8, uint64(new_machine.host_func_id))
	// m_n.I = new_machine.pc + 1
	//	return
	//}
	// 786
	status := new_machine.GetMachineState()
	pvmtypes.ResultMap[uint64(status)]++
	switch status {
	case FAULT:
		log.Info(vm.logging, "INVOKE FAULT", "n", n, "new_machine.GetFaultAddress()", new_machine.GetFaultAddress())
		vm.WriteRegister(7, FAULT)
		vm.WriteRegister(8, new_machine.GetFaultAddress())
	case HOST:
		log.Info(vm.logging, "INVOKE HOST", "n", n, "new_machine.host_func_id", new_machine.GetHostID())
		vm.WriteRegister(7, HOST)
		vm.WriteRegister(8, new_machine.GetHostID())
		if vm.Backend == BackendCompiler {
			new_machine.SetPC(new_machine.GetPC() + 1)
		}
	default:
		state := new_machine.GetMachineState()
		log.Info(vm.logging, "INVOKE returning to parent", "n", n, "child.MachineState", state, "gasRemaining", new_machine.GetGas())
		vm.WriteRegister(7, uint64(state))
	}
	log.Info(vm.logging, "INVOKE OK", "n", n, "new_machine.MachineState", new_machine.GetMachineState(), "gasRemaining", new_machine.GetGas(), "pc", new_machine.GetPC())
	vm.VMs[uint32(n)] = &new_machine
}

// Lookup preimage
func (vm *VM) hostLookup() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	omega_7 := vm.ReadRegister(7)

	var a *types.ServiceAccount
	if omega_7 == uint64(vm.Service_index) || omega_7 == maxUint64 {
		a = vm.ServiceAccount
	}
	if a == nil {
		a, _ = vm.getXUDS(omega_7)
	}

	h := vm.ReadRegister(8)
	o := vm.ReadRegister(9)
	f := vm.ReadRegister(10)
	l := vm.ReadRegister(11)
	k_bytes, err_k := vm.ReadRAMBytes(uint32(h), 32)
	if err_k != OK {
		vm.Panic(err_k)
		return
	}

	var account_blobhash common.Hash

	var v []byte
	var ok bool
	var preimage_source string

	account_blobhash = common.Hash(k_bytes)
	ok, v, preimage_source = a.ReadPreimage(account_blobhash, vm.hostenv)
	if !ok {
		vm.WriteRegister(7, NONE)
		log.Trace(vm.logging, "LOOKUP NONE", "s", fmt.Sprintf("%d", a.ServiceIndex), "h", account_blobhash, "preimage_source", preimage_source)
		vm.SetHostResultCode(NONE)
		return
	}
	f = min(f, uint64(len(v)))
	l = min(l, uint64(len(v))-f)

	err := vm.WriteRAMBytes(uint32(o), v[:l])

	// Record taint for external write
	if tracker, ok := vm.ExecutionVM.(externalWriteTracker); ok {
		tracker.TaintRecordExternalWrite(uint64(o), l, "PROVIDE_CHILD")
	}

	if err != OK {
		vm.Panic(err)
		return
	}

	if len(v) != 0 {
		vm.WriteRegister(7, uint64(len(v)))
	}
	logStr := fmt.Sprintf("len = %d", len(v))
	if len(v) < 200 {
		logStr = fmt.Sprintf("len = %d, v = %s", len(v), fmt.Sprintf("%x", v))
	}
	log.Trace(vm.logging, "LOOKUP OK", "s", fmt.Sprintf("%d", a.ServiceIndex), "h", h, "v", logStr)
	vm.SetHostResultCode(OK)
}

// Key Idea: fetch potential mutated (with Mutable=true) ServiceAccount from the XContext Partial State (X.U.D),
// which may have been changed
func (vm *VM) getXUDS(serviceindex uint64) (a *types.ServiceAccount, errCode uint64) {
	var ok bool
	var err error
	s := uint32(serviceindex)
	if serviceindex == maxUint64 || uint32(serviceindex) == vm.X.ServiceIndex {
		return vm.X.U.ServiceAccounts[s], OK
	}
	a, ok = vm.X.U.ServiceAccounts[s]
	if !ok {
		a, ok, err = vm.hostenv.GetService(s)
		if err != nil || !ok {
			return nil, NONE
		}
		vm.X.U.ServiceAccounts[s] = a
	}
	return a, OK
}

// Read Storage
func (vm *VM) hostRead() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}
	// Assume that all ram can be read and written
	omega_7 := vm.ReadRegister(7)
	s_star := omega_7
	var a *types.ServiceAccount
	var errCode uint64
	if omega_7 == maxUint64 {
		s_star = uint64(vm.Service_index)
	}
	ko := vm.ReadRegister(8)
	kz := vm.ReadRegister(9)
	mu_k, err_k := vm.ReadRAMBytes(uint32(ko), uint32(kz)) // this is the raw key.
	if err_k != OK {
		vm.Panic(err_k)
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}
	if s_star == uint64(vm.Service_index) {
		a = vm.ServiceAccount
	} else {
		a, errCode = vm.getXUDS(s_star)
		if errCode != OK {
			vm.WriteRegister(7, NONE)
			vm.SetHostResultCode(NONE)
			return
		}
	}
	bo := vm.ReadRegister(10)
	f := vm.ReadRegister(11)
	l := vm.ReadRegister(12)
	// [0.6.7] No more hashing of mu_k
	//k := common.ServiceStorageKey(a.ServiceIndex, mu_k) // this does E_4(s) ... mu_4
	ok, val, storage_source := a.ReadStorage(mu_k, vm.hostenv)
	lenval := uint64(len(val))
	f = min(f, lenval)
	l = min(l, lenval-f)

	vm.DebugHostFunction(READ, "s=%d, mu_k=0x%x, kLen=%d, bo=0x%x, f=%d, l=%d", a.ServiceIndex, mu_k, len(mu_k), bo, f, l)
	if !ok { // || true
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		//log.Trace(vm.logging, "READ NONE", "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "kLen", len(mu_k), "ok", ok, "val", fmt.Sprintf("%x", val), "len(val)", len(val), "source", storage_source)
		vm.DebugHostFunction(READ, "bo=%x, f=%d, l=%d, val=0x%x", bo, f, l, val[f:f+l])
		return
	}
	log.Trace(vm.logging, "READ OK", "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "kLen", len(mu_k), "ok", ok, "val", fmt.Sprintf("%x", val), "len(val)", len(val), "source", storage_source)
	// reuse existing lenval, f, l variables redefining based on current register values
	lenval = uint64(len(val))
	f = min(uint64(len(val)), vm.ReadRegister(11))   // offset
	l = min(uint64(len(val))-f, vm.ReadRegister(12)) // max length
	vm.DebugHostFunction(READ, "bo=%x, f=%d, l=%d, val=0x%x", bo, f, l, val[f:f+l])
	if errCode := vm.WriteRAMBytes(uint32(bo), val[f:f+l]); errCode != OK {
		log.Trace(vm.logging, "READ RAM WRITE ERROR", "err", errCode)
		vm.Panic(errCode)
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}

	// Record taint for external write
	if tracker, ok := vm.ExecutionVM.(externalWriteTracker); ok {
		tracker.TaintRecordExternalWrite(uint64(bo), l, "READ")
	}

	vm.WriteRegister(7, lenval)
}

// Write Storage a_s(x,y)
func (vm *VM) hostWrite() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	var a *types.ServiceAccount
	a = vm.ServiceAccount
	if a == nil {
		a, _ = vm.getXUDS(uint64(vm.Service_index))
	}
	ko := vm.ReadRegister(7)
	kz := vm.ReadRegister(8)
	vo := vm.ReadRegister(9)
	vz := vm.ReadRegister(10)

	mu_k, err_k := vm.ReadRAMBytes(uint32(ko), uint32(kz))
	if err_k != OK {
		vm.Panic(err_k)
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		log.Info(vm.logging, "WRITE RAM ERROR", "err", err_k)
		return
	}
	mu_k = slices.Clone(mu_k)
	key_len := uint64(len(mu_k)) // x in a_s(x,y) |y|
	exists, oldValue, storage_source := a.ReadStorage(mu_k, vm.hostenv)
	prevLen := uint64(len(oldValue))

	v := []byte{}
	err := uint64(0)
	val_len := uint64(0) // y in a_s(x,y) |x|
	if vz > 0 {
		v, err = vm.ReadRAMBytes(uint32(vo), uint32(vz))
		if err != OK {
			vm.Panic(err)
			vm.terminated = true
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			log.Info(vm.logging, "WRITE RAM ERROR", "err", err)
			return
		}
		v = slices.Clone(v)
		val_len = uint64(len(v))
	}
	vm.DebugHostFunction(WRITE, "[mu_k 0x%x]writing val 0x%x from address 0x%x, length %d", mu_k, v, vo, vz)

	deltaItems := int32(0)
	deltaSize := int64(0)
	response := uint64(NONE)

	if !exists {
		if val_len > 0 {
			deltaItems = 1
			deltaSize = int64(AccountStorageConst + val_len + key_len)
		}
	} else {
		response = prevLen
		if val_len == 0 {
			deltaItems = -1
			deltaSize = -int64(AccountStorageConst + prevLen + key_len)
		} else {
			deltaSize = int64(val_len) - int64(prevLen)
		}
	}

	var threshold uint64
	if deltaItems > 0 || deltaSize > 0 {
		threshold = a.ComputeThresholdDelta(deltaItems, deltaSize)
	} else {
		threshold = a.ComputeThreshold()
	}
	if a.Balance < threshold {
		vm.WriteRegister(7, FULL)
		vm.SetHostResultCode(FULL)
		log.Trace(vm.logging, "@@@ WRITE FULL", "service", a.ServiceIndex, "threshold", threshold, "balance", a.Balance, "deltaItems", deltaItems, "deltaSize", deltaSize)
		return
	}

	a.WriteStorage(a.ServiceIndex, mu_k, v, vz == 0, storage_source)
	vm.ResultCode = uint8(OK)
	vm.SetHostResultCode(OK)

	if deltaItems != 0 {
		a.AdjustNumStorageItems(deltaItems)
	}
	if deltaSize != 0 {
		a.AdjustStorageSize(deltaSize)
	}
	vm.WriteRegister(7, response)

	as_internal_key := common.Compute_storageKey_internal(mu_k)
	as_internal_key_str := common.Bytes2Hex(as_internal_key[:])
	account_storage_key := fmt.Sprintf("0x%x", common.ComputeC_sh(a.ServiceIndex, as_internal_key).Bytes()[:31])

	log.Trace(vm.logging, "WRITE storage",
		"mu_k", fmt.Sprintf("%x", mu_k),
		"opaque_key", as_internal_key_str,
		"as_key", account_storage_key,
		"service", a.ServiceIndex,
		"deltaItems", deltaItems,
		"deltaSize", deltaSize,
		"key_len", int64(key_len),
		"val", fmt.Sprintf("%x", v),
		"val_len", int64(val_len),
		"prevLen", int64(prevLen),
		"s", fmt.Sprintf("%d", a.ServiceIndex),
		"v_o", fmt.Sprintf("0x%x", vo),
		"v_z", fmt.Sprintf("%d", vz),
		"a_o", a.StorageSize, "a_i", a.NumStorageItems)
}

// Solicit preimage a_l(h,z)
func (vm *VM) hostSolicit() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	xs, _ := vm.X.GetX_s()
	// Got l of X_s by setting s = 1, z = z(from RAM)
	o := vm.ReadRegister(7)
	z := vm.ReadRegister(8)                         // z: blob_len
	hBytes, err_h := vm.ReadRAMBytes(uint32(o), 32) // h: blobHash
	if err_h != OK {
		log.Trace(vm.logging, "SOLICIT RAM READ ERROR", "err", err_h, "o", o, "z", z)
		vm.Panic(err_h)
		return
	}
	account_lookuphash := common.BytesToHash(hBytes)

	// Calculate threshold with overflow protection using ComputeThresholdDelta
	// SOLICIT adds 2 items (lookup + preimage) and z bytes
	threshold := xs.ComputeThresholdDelta(2, int64(z))
	if threshold == ^uint64(0) {
		// Overflow occurred in threshold calculation
		vm.WriteRegister(7, FULL)
		vm.SetHostResultCode(FULL)
		return
	}
	if xs.Balance < threshold {
		vm.WriteRegister(7, FULL)
		vm.SetHostResultCode(FULL)
		log.Trace(vm.logging, "SOLICIT FULL threshold reached", "h", account_lookuphash, "z", z, "threshold", threshold, "xs.Balance", xs.Balance)
		return
	}
	ok, X_s_l, lookup_source := xs.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		// when preimagehash is not found, put it into solicit request - so we can ask other DAs
		xs.WriteLookup(account_lookuphash, uint32(z), []uint32{}, lookup_source)
		xs.AdjustNumStorageItems(2)
		xs.AdjustStorageSize(int64(AccountLookupConst + uint64(z)))
		vm.WriteRegister(7, OK)
		vm.SetHostResultCode(OK)
		return
	}

	if len(X_s_l) == 2 { // [x, y] => [x, y, t]
		xs.WriteLookup(account_lookuphash, uint32(z), append(X_s_l, []uint32{vm.Timeslot}...), lookup_source)
		al_internal_key := common.Compute_preimageLookup_internal(account_lookuphash, uint32(z))
		account_storage_key := fmt.Sprintf("0x%x", common.ComputeC_sh(xs.ServiceIndex, al_internal_key).Bytes()[:31])
		log.Trace(vm.logging, "SOLICIT OK 2", "service", xs.ServiceIndex, "h", account_lookuphash, "z", z, "newvalue", append(X_s_l, []uint32{vm.Timeslot}...), "account_storage_key", account_storage_key)
		vm.WriteRegister(7, OK)
		vm.SetHostResultCode(OK)
		return
	} else {
		vm.WriteRegister(7, HUH)
		vm.SetHostResultCode(HUH)
		//log.Trace(vm.logging, "SOLICIT HUH", "h", account_lookuphash, "z", z, "len(X_s_l)", len(X_s_l))
		return
	}
}

// Forget preimage a_l(h,z)
func (vm *VM) hostForget() {
	if vm.Mode != ModeAccumulate {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	x_s, _ := vm.X.GetX_s()
	o := vm.ReadRegister(7)
	z := vm.ReadRegister(8)
	hBytes, errCode := vm.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.Panic(errCode)
		return
	}

	account_lookuphash := common.Hash(hBytes)
	account_blobhash := common.Hash(hBytes)

	lookup_ok, X_s_l, lookup_source := x_s.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	_, _, preimage_source := x_s.ReadPreimage(account_blobhash, vm.hostenv)

	if !lookup_ok {
		vm.WriteRegister(7, HUH)
		vm.SetHostResultCode(HUH)
		log.Trace(vm.logging, "FORGET HUH", "h", account_lookuphash, "o", o, "lookup_source", lookup_source)
		return
	}
	al_internal_key := common.Compute_preimageLookup_internal(account_lookuphash, uint32(z))
	account_storage_key := fmt.Sprintf("0x%x", common.ComputeC_sh(x_s.ServiceIndex, al_internal_key).Bytes()[:31])
	if len(X_s_l) == 0 {
		// 0 [] case is when we solicited but never got a preimage, so we can forget it
		x_s.WriteLookup(account_lookuphash, uint32(z), nil, lookup_source) // nil means delete the lookup
		x_s.WritePreimage(account_blobhash, []byte{}, preimage_source)     // []byte{} means delete the preimage. TODO: should be preimage_source
		// storage accounting
		x_s.AdjustNumStorageItems(-2)
		x_s.AdjustStorageSize(-int64(AccountLookupConst + uint64(z)))
		log.Trace(vm.logging, "FORGET OK A", "h", account_lookuphash, "z", z, "vm.Timeslot", vm.Timeslot, "expiry", (vm.Timeslot - types.PreimageExpiryPeriod), "types.PreimageExpiryPeriod", types.PreimageExpiryPeriod, "account_storage_key", account_storage_key)
		vm.WriteRegister(7, OK)
		vm.SetHostResultCode(OK)
		return
	} else if len(X_s_l) == 2 && X_s_l[1]+types.PreimageExpiryPeriod < vm.Timeslot {
		// 2 [x,y] case is when we have a forgotten a preimage and have PASSED the preimage expiry period, so we can forget it
		x_s.WriteLookup(account_lookuphash, uint32(z), nil, lookup_source) // nil means delete the lookup
		x_s.WritePreimage(account_blobhash, []byte{}, preimage_source)     // []byte{} means delete the preimage
		// storage accounting
		x_s.AdjustNumStorageItems(-2)
		x_s.AdjustStorageSize(-int64(AccountLookupConst + uint64(z)))
		log.Trace(vm.logging, "FORGET OK B", "h", account_lookuphash, "z", z, "vm.Timeslot", vm.Timeslot, "X_s_l[1]", X_s_l[1], "expiry", (vm.Timeslot - types.PreimageExpiryPeriod), "types.PreimageExpiryPeriod", types.PreimageExpiryPeriod, "account_storage_key", account_storage_key)
		vm.WriteRegister(7, OK)
		vm.SetHostResultCode(OK)
		return
	} else if len(X_s_l) == 1 {
		// preimage exists [x] => [x, y] where y is the current time, the time we are forgetting
		x_s.WriteLookup(account_lookuphash, uint32(z), append(X_s_l, []uint32{vm.Timeslot}...), lookup_source) // [x, t]
		vm.WriteRegister(7, OK)
		vm.SetHostResultCode(OK)
		log.Trace(vm.logging, "FORGET OK C", "h", account_lookuphash, "z", z, "newvalue", append(X_s_l, []uint32{vm.Timeslot}...), "account_storage_key", account_storage_key)
		return
	} else if len(X_s_l) == 3 && X_s_l[1]+types.PreimageExpiryPeriod < vm.Timeslot {
		// [x,y,w] => [w, t] where y is the current time, the time we are forgetting
		X_s_l = []uint32{X_s_l[2], vm.Timeslot}                              // w = X_s_l[2], t = vm.Timeslot
		x_s.WriteLookup(account_lookuphash, uint32(z), X_s_l, lookup_source) // [w, t]
		vm.WriteRegister(7, OK)
		vm.SetHostResultCode(OK)
		log.Trace(vm.logging, "FORGET OK D", "h", account_lookuphash, "z", z, "newvalue", X_s_l, "account_storage_key", account_storage_key)
		return
	}
	vm.WriteRegister(7, HUH)
	vm.SetHostResultCode(HUH)
	log.Trace(vm.logging, "FORGET HUH", "h", account_lookuphash, "o", o)
}

// HistoricalLookup determines whether the preimage of some hash h was available for lookup by some service account a at some timeslot t, and if so, provide its preimage
func (vm *VM) hostHistoricalLookup() {
	if vm.Mode != ModeRefine {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	var a = &types.ServiceAccount{}
	s := vm.Service_index
	omega_7 := vm.ReadRegister(7)
	h := vm.ReadRegister(8)
	o := vm.ReadRegister(9)
	omega_10 := vm.ReadRegister(10)
	omega_11 := vm.ReadRegister(11)
	var ok bool
	var err error
	if omega_7 == NONE {
		a, ok, err = vm.hostenv.GetService(s)
		if err != nil || !ok {
			vm.WriteRegister(7, NONE)
			vm.SetHostResultCode(NONE)
			return
		}

	} else {
		a, ok, err = vm.hostenv.GetService(uint32(omega_7))
		if err != nil || !ok {
			vm.WriteRegister(7, NONE)
			vm.SetHostResultCode(NONE)
			return
		}
	}

	hBytes, errCode := vm.ReadRAMBytes(uint32(h), 32)
	if errCode != OK {
		vm.Panic(errCode)
		return
	}
	// h := common.Hash(hBytes) not sure whether this is needed
	v := vm.hostenv.HistoricalLookup(a, vm.Timeslot, common.BytesToHash(hBytes))
	vLength := uint64(len(v))

	f := min(omega_10, vLength)
	l := min(omega_11, vLength-f)
	errCode = vm.WriteRAMBytes(uint32(o), v[f:f+l])

	// Record taint for external write
	if tracker, ok := vm.ExecutionVM.(externalWriteTracker); ok {
		tracker.TaintRecordExternalWrite(uint64(o), l, "LOOKUP")
	}

	if errCode != OK {
		vm.Panic(errCode)
		return
	}
	vm.WriteRegister(7, vLength)
	vm.DebugHostFunction(HISTORICAL_LOOKUP, "s=%d, h=0x%x, o=0x%x, f=%d, l=%d, valvLength=%d", a.ServiceIndex, hBytes, o, f, l, vLength)

}

var CheckSegments [][]byte

// Export segment host-call
func (vm *VM) hostExport() {
	if vm.Mode != ModeRefine {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	p := vm.ReadRegister(7) // a0 = 7
	z := vm.ReadRegister(8) // a1 = 8

	z = min(z, types.SegmentSize)

	x, errCode := vm.ReadRAMBytes(uint32(p), uint32(z))
	if errCode != OK {
		vm.Panic(errCode)
		return
	}

	vm.WriteRegister(7, OK)
	vm.SetHostResultCode(OK)

	y := slices.Clone(x)
	y = common.PadToMultipleOfN(y, types.SegmentSize)

	if vm.TotalExported+uint64(len(vm.Exports)) > types.MaxExports {
		vm.DebugHostFunction(EXPORT, "p=0x%x, z=%d, total_exports=%d, l=%d -- FULL", p, z, len(vm.Exports), vm.TotalExported)
		vm.WriteRegister(7, FULL)
		vm.SetHostResultCode(FULL)
		return
	} else {
		vm.WriteRegister(7, uint64(vm.TotalExported)+uint64(len(vm.Exports)))
		vm.Exports = append(vm.Exports, y)
		vm.SetHostResultCode(OK)
		//		vm.DebugHostFunction(EXPORT, "p=0x%x, z=%d, total_exports=%d, l=%d", p, z, len(vm.Exports), vm.TotalExported)
		log.Trace(vm.logging, "EXPORT", "p", fmt.Sprintf("0x%x", p), "z", z, "total_exports", len(vm.Exports), "l", len(y))
		if vm.pushFrame != nil {
			// Stream the latest segment to any attached frame server without clearing exports
			// so exports remain available to the caller.
			vm.pushFrame(y)
		}

		return
	}
}

func (vm *VM) hostMachine() {
	if vm.Mode != ModeRefine {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}
	po := vm.ReadRegister(7)
	pz := vm.ReadRegister(8)
	i := vm.ReadRegister(9)
	p, errCode := vm.ReadRAMBytes(uint32(po), uint32(pz))
	if errCode != OK {
		vm.Panic(errCode)
		return
	}

	min_n := uint32(16)
	for n := uint32(0); n < min_n; n++ {
		_, ok := vm.VMs[n]
		if !ok {
			min_n = n
			break
		}
	}
	log.Info(vm.logging, "hostMachine", "po", po, "pz", pz, "i", i, "n", min_n, "program_size", len(p))
	program, err := pvm.DecodeProgram_pure_pvm_blob(p)
	if err != nil {
		log.Error(vm.logging, "hostMachine: DecodeProgram_pure_pvm_blob failed", "error", err)
		vm.Panic(WHO)
		return
	}
	currentCounter := vm.VmsEntryCounter[min_n]
	execMachine := vm.NewEmptyExecutionVM(vm.Service_index, program, make([]uint64, 13), i, 0, vm.hostenv, uint32(min_n), currentCounter)
	vm.VmsEntryCounter[min_n]++
	if vm.VMs == nil {
		vm.VMs = make(map[uint32]*ExecutionVM)
	}
	// recompiler.SetShowDisassembly(true)
	vm.VMs[min_n] = execMachine

	vm.WriteRegister(7, uint64(min_n))
}

func (vm *VM) hostPeek() {
	if vm.Mode != ModeRefine {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}

	n := vm.ReadRegister(7)
	o := vm.ReadRegister(8)
	s := vm.ReadRegister(9)
	z := vm.ReadRegister(10)
	m_n, ok := vm.VMs[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		vm.SetHostResultCode(WHO)
		return
	}
	log.Info(vm.logging, "hostPeek", "mn", m_n, "n", n, "o", o, "s", s, "z", z)
	// read l bytes from m

	var errCode uint64
	var s_data []byte
	s_data, errCode = (*m_n).ReadRAMBytes(uint32(s), uint32(z))
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.SetHostResultCode(OOB)
		return
	}
	// write l bytes to vm || TODO check order
	if debugPeek && len(s_data) >= 2 {
		fmt.Printf("[PEEK DEBUG] child src=0x%x -> parent dst=0x%x len=%d first bytes: 0x%02x 0x%02x\n",
			s, o, len(s_data), s_data[0], s_data[1])
	}
	// Debug: ALWAYS show first bytes for PEEK to trace result corruption
	errCode = vm.WriteRAMBytes(uint32(o), s_data[:])

	// Record taint for external write
	if tracker, ok := vm.ExecutionVM.(externalWriteTracker); ok {
		tracker.TaintRecordExternalWrite(o, z, "PEEK")
	}

	if errCode != OK {
		vm.Panic(errCode)
		return
	}
	vm.WriteRegister(7, OK)
	vm.SetHostResultCode(OK)
}

func (vm *VM) hostPoke() {
	if vm.Mode != ModeRefine {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}
	n := vm.ReadRegister(7) // machine
	s := vm.ReadRegister(8) // source
	o := vm.ReadRegister(9) // dest
	z := vm.ReadRegister(10)
	// read data from original vm
	s_data, errCode := vm.ReadRAMBytes(uint32(s), uint32(z))
	if errCode != OK {
		vm.Panic(errCode)
		return
	}
	m_n, ok := vm.VMs[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		vm.SetHostResultCode(WHO)
		return
	}

	// write data to m_n
	log.Info(vm.logging, "hostPoke", "mn", m_n, "n", n, "o", o, "s", s, "z", z)
	errCode = (*m_n).WriteRAMBytes(uint32(o), s_data[:])
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.SetHostResultCode(OOB)
		return
	}
	// Record taint for external write to child VM
	if tracker, ok := (*m_n).(externalWriteTracker); ok {
		tracker.TaintRecordExternalWrite(o, z, "POKE")
	}
	vm.WriteRegister(7, OK)
	vm.SetHostResultCode(OK)
}

func (vm *VM) hostExpunge() {
	if vm.Mode != ModeRefine {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}
	n := vm.ReadRegister(7)
	m, ok := vm.VMs[uint32(n)]
	if !ok {
		vm.SetHostResultCode(WHO)
		vm.WriteRegister(7, WHO)
		return
	}

	i := (*m).GetPC()
	(*m).Destroy()
	delete(vm.VMs, uint32(n))
	vm.WriteRegister(7, i)
	vm.SetHostResultCode(OK)
	log.Info(vm.logging, "hostExpunge", "n", n, "pc", i)
}

func (vm *VM) hostPages() {
	if vm.Mode != ModeRefine {
		vm.WriteRegister(7, WHAT)
		vm.SetHostResultCode(WHAT)
		return
	}
	n := vm.ReadRegister(7)  // n: machine number
	p := vm.ReadRegister(8)  // p: page number
	c := vm.ReadRegister(9)  // c: number of pages to change
	r := vm.ReadRegister(10) // r: access characteristics
	m, ok := vm.VMs[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		vm.SetHostResultCode(WHO)
		log.Trace(vm.logging, "hostPages WHO", "n", n, "p", p, "c", c, "r", r)
		return
	}

	if p > maxUint64-c {
		vm.WriteRegister(7, HUH)
		vm.SetHostResultCode(HUH)
		log.Trace(vm.logging, "hostPages HUH", "n", n, "p", p, "c", c, "r", r)
		return
	}

	if p < 16 || p+c >= (1<<32)/pvmtypes.Z_P || r > 4 {
		vm.WriteRegister(7, HUH)
		vm.SetHostResultCode(HUH)
		log.Trace(vm.logging, "hostPages HUH", "n", n, "p", p, "c", c, "r", r)
		return
	}
	page := int(p)
	count := int(c)
	// get the page address
	var toWrite []byte
	pageAddress := page * pvmtypes.Z_P
	countLength := count * pvmtypes.Z_P
	if r > 2 {
		var errCode uint64
		toWrite, errCode = vm.ReadRAMBytes(uint32(pageAddress), uint32(countLength))
		if errCode != OK {
			vm.WriteRegister(7, HUH)
			vm.SetHostResultCode(HUH)

			log.Trace(vm.logging, "hostPages HUH", "n", n, "p", p, "c", c, "r", r)
			return
		}
		(*m).SetPagesAccessRange(page, count, pvmtypes.PageMutable)
		(*m).WriteRAMBytes(uint32(pageAddress), toWrite)
		// Record taint for external write to child VM
		if tracker, ok := (*m).(externalWriteTracker); ok {
			tracker.TaintRecordExternalWrite(uint64(pageAddress), uint64(countLength), "PAGES")
		}
	} else {
		(*m).SetPagesAccessRange(page, count, pvmtypes.PageMutable)
		toWrite = make([]byte, countLength)
		(*m).WriteRAMBytes(uint32(pageAddress), toWrite)
		// Record taint for external write to child VM
		if tracker, ok := (*m).(externalWriteTracker); ok {
			tracker.TaintRecordExternalWrite(uint64(pageAddress), uint64(countLength), "PAGES")
		}
	}
	switch {
	case r == 0: // To deallocate a page (previously void)
		(*m).SetPagesAccessRange(page, count, pvmtypes.PageInaccessible)
	case r == 1 || r == 3: // read-only
		(*m).SetPagesAccessRange(page, count, pvmtypes.PageImmutable)
	case r == 2 || r == 4: // read-write
		(*m).SetPagesAccessRange(page, count, pvmtypes.PageMutable)
	}
	if r > 2 {
		vm.DebugHostFunction(PAGES, "PAGES WRITE n=%d, p=%d, c=%d, r=%d -- wrote data", n, p, c, r)
	} else {
		vm.DebugHostFunction(PAGES, "PAGES n=%d, p=%d, c=%d, r=%d", n, p, c, r)
	}
	vm.WriteRegister(7, OK)
	vm.SetHostResultCode(OK)
}

// JIP-1 https://hackmd.io/@polkadot/jip1
func (vm *VM) hostLog() {
	level := vm.ReadRegister(7)
	message := vm.ReadRegister(10)
	messagelen := vm.ReadRegister(11)

	messageBytes, errCode := vm.ReadRAMBytes(uint32(message), uint32(messagelen))

	if errCode != OK {
		fmt.Printf("HOSTLOG-ERROR: OOB reading log message at %d len %d\n", message, messagelen)
		vm.SetHostResultCode(OOB)
		return
	}
	//levelName := getLogLevelName(level, vm.CoreIndex, string(vm.ServiceMetadata))
	vm.SetHostResultCode(OK)
	serviceMetadata := string(vm.ServiceMetadata)
	if serviceMetadata == "" {
		serviceMetadata = "unknown"
	}
	vm.WriteRegister(7, WHAT)
	if vm.IsChild {
		serviceMetadata = fmt.Sprintf("%s-child", serviceMetadata)
	}
	vm.WriteRegister(7, WHAT)
	loggingVerbose := true
	if vm.logging == log.Builder || vm.logging == log.FirstGuarantor || vm.logging == log.OtherGuarantor {
		loggingVerbose = true
	}
	if !loggingVerbose {
		return
	}
	gasRemaining := vm.SafeGetGas()
	switch level {
	case 0: // 0: User agent displays as fatal error
		fmt.Printf("\x1b[31m[FATAL-%s] %s (gas: %d)\x1b[0m\n", vm.logging, string(messageBytes), gasRemaining)
	case 1: // 1: User agent displays as warning
		fmt.Printf("\x1b[33m[WARN-%s] %s (gas: %d)\x1b[0m\n", vm.logging, string(messageBytes), gasRemaining)
	case 2: // 2: User agent displays as important information
		fmt.Printf("\x1b[32m[INFO-%s] %s (gas: %d)\x1b[0m\n", vm.logging, string(messageBytes), gasRemaining)
	case 3: // 3: User agent displays as helpful information
		fmt.Printf("\x1b[36m[DEBUG-%s] %s (gas: %d)\x1b[0m\n", vm.logging, string(messageBytes), gasRemaining)
	case 4: // 4: User agent displays as pedantic information
		fmt.Printf("\x1b[37m[TRACE-%s] %s (gas: %d)\x1b[0m\n", vm.logging, string(messageBytes), gasRemaining)
	}

}

func (vm *VM) PutGasAndRegistersToMemory(input_address uint32, gas uint64, regs []uint64) {
	gasBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(gasBytes, gas)
	errCode := vm.WriteRAMBytes(input_address, gasBytes)

	// Record taint for external write (gas)
	if tracker, ok := vm.ExecutionVM.(externalWriteTracker); ok {
		tracker.TaintRecordExternalWrite(uint64(input_address), 8, "PUT_GAS_REGS_GAS")
	}

	if errCode != OK {
		vm.SetHostResultCode(OOB)
		return
	}
	for i, reg := range regs {
		regBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(regBytes, reg)
		errCode = vm.WriteRAMBytes(input_address+8+uint32(i*8), regBytes)

		// Record taint for external write (register)
		if tracker, ok := vm.ExecutionVM.(externalWriteTracker); ok {
			tracker.TaintRecordExternalWrite(uint64(input_address+8+uint32(i*8)), 8, "PUT_GAS_REGS_REG")
		}

		if errCode != OK {
			vm.SetHostResultCode(OOB)
			return
		}
	}
	vm.SetHostResultCode(OK)
}

// This should only be called OUTSIDE pvm package
func (vm *VM) SetPVMContext(l string) {
	vm.logging = l
}

func (vm *VM) GetGasAndRegistersFromMemory(input_address uint32) (gas uint64, regs []uint64, errCode uint64) {
	gasBytes, errCode := vm.ReadRAMBytes(input_address, 8)
	if errCode != OK {
		return 0, nil, errCode
	}
	gas = binary.LittleEndian.Uint64(gasBytes)
	regs = make([]uint64, 13)
	for i := 0; i < 13; i++ {
		regBytes, errCode := vm.ReadRAMBytes(input_address+8+uint32(i*8), 8)
		if errCode != OK {
			return 0, nil, errCode
		}
		regs[i] = binary.LittleEndian.Uint64(regBytes)
	}
	return gas, regs, OK
}

// HostFetchWitness implements host function 254
// fetch_object(s: u64, ko: u64, kz: u64, o: u64, f: u64, l: u64) -> u64
// Parameters:
//
//	s (w7): service_id
//	ko (w8): object_id pointer (32 bytes)
//	kz (w9): object_id length (should be 32)
//	o (w10): output buffer pointer
//	f (w11): FIRST_READABLE_ADDRESS (for bounds checking)
//	l (w12): output buffer length (max size)
//
// Returns:
//
//	w7: total bytes written (ObjectRef (64 bytes) + payload), or 0 if not found
//
// Non-builders will get empty responses from this and might as well not call it!
func (vm *VM) HostFetchWitness() error {
	// Read parameters from registers
	service_id := uint32(vm.ReadRegister(7))
	object_id_ptr := uint32(vm.ReadRegister(8))
	object_id_len := uint32(vm.ReadRegister(9))
	output_ptr := uint32(vm.ReadRegister(10))
	//first_readable := vm.ReadRegister(11)
	output_max_len := uint32(vm.ReadRegister(12))

	funcName := "HostFetchWitness"
	// Validate object_id length
	if object_id_len != 32 {
		log.Info(vm.logging, funcName+": invalid object_id length", "object_id_len", object_id_len)

		vm.WriteRegister(7, 0) // Return 0 = not found
		return nil
	}

	// Read object_id from memory
	object_id_bytes, errCode := vm.ReadRAMBytes(object_id_ptr, object_id_len)
	if errCode != OK {
		log.Info(vm.logging, funcName+": failed to read object_id from memory", "object_id_ptr", fmt.Sprintf("0x%x", object_id_ptr), "object_id_len", object_id_len, "error", errCode)
		vm.WriteRegister(7, 0)
		return nil
	}
	// Convert object_id bytes to common.Hash
	object_id := common.BytesToHash(object_id_bytes)

	// Log all fetch_object calls to track meta-shard fetches
	log.Trace(vm.logging, "🔍 HostFetchWitness called", "role", vm.logging, "object_id", object_id, "service_id", service_id)

	if vm.logging != log.Builder {
		log.Info(vm.logging, "HostFetchWitness returning empty (not builder)", "role", vm.logging, "object_id", object_id)
		vm.WriteRegister(7, 0)
		// return nil
	}

	// Witness path: use ReadStateWitness which returns complete witness with proof
	// Pass FetchJAMDASegments=true to fetch payload and populate witness.Payload
	// Use the service_id from register a0 instead of vm.Service_index to support cross-service imports
	witness, found, err := vm.hostenv.ReadObject(service_id, object_id)
	if err != nil {
		// Object not found in meta-shard is normal (e.g., EOA has no code, uninitialized storage)
		log.Info(vm.logging, funcName+": ❌ ReadObject returned error", "object_id", object_id, "err", err)
		vm.WriteRegister(7, 0) // Return 0 = not found
		return nil
	} else if !found {
		log.Info(vm.logging, funcName+": ⚠️ ReadObject returned found=false", "object_id", object_id)
		vm.WriteRegister(7, 0)
		return nil
	}

	// Phase 1: Check witness caches first (UBT witness transition)
	// Extract address from objectID (first 20 bytes)
	var address common.Address
	copy(address[:], object_id[:20])

	// Determine object kind from objectID byte 31
	objectKind := evmtypes.ObjectKind(object_id[31])

	var payload []byte

	if evmStorage, ok := getEVMStorage(vm.hostenv); ok {
		switch objectKind {
		case evmtypes.ObjectKindCode:
			// Look up code from witness cache
			if code, exists := evmStorage.GetCode(address); exists {
				payload = code
				found = true
				// Record as read dependency for witness export
				if vm.codeWitness == nil {
					vm.codeWitness = make(map[common.Address][]byte)
				}
				vm.codeWitness[address] = code
				log.Trace(vm.logging, funcName+": ✅ Found code in witness cache", "address", address.Hex(), "codeSize", len(code))

				// Create minimal witness for code
				witness = &types.StateWitness{
					ServiceID: service_id,
					ObjectID:  object_id,
					Payload:   payload,
					Ref: types.ObjectRef{
						WorkPackageHash: common.Hash{}, // Will be filled during export
						PayloadLength:   uint32(len(payload)),
						ObjectKind:      uint8(objectKind),
					},
				}
			}

		case evmtypes.ObjectKindStorageShard:
			// Post-SSR: Always use root shard (ld=0)
			shardID := evmtypes.ShardID{Ld: 0, Prefix56: [7]byte{}}

			// Look up from witness cache
			if storageData, exists := evmStorage.GetContractStorage(address); exists {
				if contractStorage, ok := storageData.(evmtypes.ContractStorage); ok {
					shard := contractStorage.Shard
					payload = shard.Serialize()
					found = true
					// Record as read dependency for witness export
					if vm.storageWitness == nil {
						vm.storageWitness = make(map[common.Address]evmtypes.ContractStorage)
					}
					// Post-SSR: Store single shard
					storage := evmtypes.ContractStorage{
						Shard: shard,
					}
					vm.storageWitness[address] = storage
					log.Trace(vm.logging, funcName+": ✅ Found storage shard in witness cache", "address", address.Hex(), "shardLD", shardID.Ld, "entryCount", len(shard.Entries))

					// Create minimal witness for storage shard
					witness = &types.StateWitness{
						ServiceID: service_id,
						ObjectID:  object_id,
						Payload:   payload,
						Ref: types.ObjectRef{
							WorkPackageHash: common.Hash{}, // Will be filled during export
							PayloadLength:   uint32(len(payload)),
							ObjectKind:      uint8(objectKind),
						},
					}
				} else {
					log.Trace(vm.logging, funcName+": ❌ ContractStorage type assertion failed from witness cache", "address", address.Hex())
				}
			} else {
				//fmt.Printf("❌ HostFetchWitness: No storage found in witness cache for address=%s\n", address.Hex())

				// Create minimal witness for storage shard
				witness = &types.StateWitness{
					ServiceID: service_id,
					ObjectID:  object_id,
					Payload:   payload,
					Ref: types.ObjectRef{
						WorkPackageHash: common.Hash{}, // Will be filled during export
						PayloadLength:   uint32(len(payload)),
						ObjectKind:      uint8(objectKind),
					},
				}
			}
		}
	}

	// If payload not found in cache but witness exists, fetch payload from DA
	// This handles meta-shards and other objects not in witness caches
	if found && len(payload) == 0 && witness != nil && witness.Ref.PayloadLength > 0 {
		// Fetch the payload from DA segments
		var err error
		if fetcher, ok := vm.hostenv.(daFetcher); ok {
			payload, err = fetcher.FetchJAMDASegments(
				witness.Ref.WorkPackageHash,
				witness.Ref.IndexStart,
				witness.Ref.IndexStart+uint16((witness.Ref.PayloadLength+types.SegmentSize-1)/types.SegmentSize),
				witness.Ref.PayloadLength,
			)
			if err != nil {
				log.Trace(vm.logging, "HostFetchWitness: failed to fetch payload from DA",
					"object_id", object_id.Hex(),
					"err", err)
			} else {
				log.Trace(vm.logging, "HostFetchWitness: Fetched payload from DA",
					"object_id", object_id.Hex(),
					"payload_len", len(payload))
				witness.Payload = payload
			}
		}
	}

	// If not found in cache, fall back to ReadObject (fetch from DA)
	if !found {
		// Witness path: use ReadStateWitness which returns complete witness with proof
		// Pass FetchJAMDASegments=true to fetch payload and populate witness.Payload
		// Use the service_id from register a0 instead of vm.Service_index to support cross-service imports
		var err error
		witness, found, err = vm.hostenv.ReadObject(service_id, object_id)
		if err != nil {
			// Object not found in meta-shard is normal (e.g., EOA has no code, uninitialized storage)
			log.Trace(vm.logging, funcName+": ❌ ReadObject returned error", "object_id", object_id, "err", err)
			vm.WriteRegister(7, 0) // Return 0 = not found
			return nil
		} else if !found {
			log.Trace(vm.logging, funcName+": ❌ ReadObject returned found=false", "object_id", object_id)
			vm.WriteRegister(7, 0)
			return nil
		}

		// Verify BMT proof for all objects including meta-shards
		stateRoot := vm.hostenv.GetStateRoot()
		if !trie.Verify(witness.ServiceID, witness.ObjectID.Bytes(), witness.Value, stateRoot.Bytes(), witness.Path) {
			log.Trace(log.SDB, "BMT Proof verification failed", "object_id", object_id, "serviceID", witness.ServiceID, "stateRoot", stateRoot)
			// return fmt.Errorf("BMT Proof verification failed for object %s [stateRoot: %s]", object_id, stateRoot)
		} else {
			log.Trace(log.SDB, "HostFetchWitness: BMT Proof verified", "object_id", object_id, "serviceID", witness.ServiceID,
				"MetaShardKey", witness.ObjectID, "value", fmt.Sprintf("%x", witness.Value), "path", witness.Path, "stateRoot", stateRoot)
		}

		payload = witness.Payload
	}

	objRef := witness.Ref

	// Serialize ObjectRef (37 bytes compact format: 32B work_package_hash + 5B packed fields)
	serialized := objRef.Serialize()
	if len(serialized) != types.ObjectRefSerializedSize {
		fmt.Printf("%s: invalid ObjectRef serialization size %d (expected %d)\n", funcName, len(serialized), types.ObjectRefSerializedSize)
		vm.WriteRegister(7, 0)
		return nil
	}
	total_size := uint64(len(serialized) + len(payload))

	// Check output buffer size
	if total_size > uint64(output_max_len) {
		fmt.Printf("%s: output buffer too small (%d bytes needed, %d available)\n", funcName, total_size, output_max_len)
		vm.WriteRegister(7, 0)
		return nil
	}

	// Write ObjectRef to output buffer
	errCode = vm.WriteRAMBytes(output_ptr, serialized)
	if errCode != OK {
		fmt.Printf("%s: failed to write ObjectRef to memory, errCode=%d\n", funcName, errCode)
		vm.WriteRegister(7, 0)
		return nil
	}

	// Write payload after ObjectRef (if any)
	if len(payload) > 0 {
		errCode = vm.WriteRAMBytes(output_ptr+uint32(types.ObjectRefSerializedSize), payload)
		if errCode != OK {
			fmt.Printf("%s: failed to write payload to memory, errCode=%d\n", funcName, errCode)
			vm.WriteRegister(7, 0)
			return nil
		}
	}
	vm.WriteRegister(7, total_size) // Return total bytes written
	return nil
}

func (vm *VM) GetBuilderWitnesses() ([]types.ImportSegment, []types.StateWitness, error) {
	// Build per-object witnesses from meta-shard cache
	type sortableWitness struct {
		objectID common.Hash
		witness  types.StateWitness
	}

	sortableRefs := make([]sortableWitness, 0)
	importedSegments := make([]types.ImportSegment, 0)

	// Phase 1: Export code witnesses (UBT witness transition)
	for address, code := range vm.codeWitness {
		objectID := evmtypes.CodeToObjectID(address)
		witness := types.StateWitness{
			ServiceID: vm.Service_index,
			ObjectID:  objectID,
			Ref: types.ObjectRef{
				WorkPackageHash: common.Hash{}, // Will be filled by caller
				PayloadLength:   uint32(len(code)),
				ObjectKind:      uint8(evmtypes.ObjectKindCode),
			},
			Payload: code,
		}
		sortableRefs = append(sortableRefs, sortableWitness{objectID: objectID, witness: witness})
		log.Trace(vm.logging, "GetBuilderWitnesses: exporting code witness", "address", address.Hex(), "codeSize", len(code))
	}

	// Post-SSR: Export single storage shard per contract
	for address, contractStorage := range vm.storageWitness {
		shardID := evmtypes.ShardID{Ld: 0, Prefix56: [7]byte{}}
		shard := contractStorage.Shard
		objectID := evmtypes.ShardToObjectID(address, shardID.ToBytes())
		payload := shard.Serialize()
		witness := types.StateWitness{
			ServiceID: vm.Service_index,
			ObjectID:  objectID,
			Ref: types.ObjectRef{
				WorkPackageHash: common.Hash{}, // Will be filled by caller
				PayloadLength:   uint32(len(payload)),
				ObjectKind:      uint8(evmtypes.ObjectKindStorageShard),
			},
			Payload: payload,
		}
		sortableRefs = append(sortableRefs, sortableWitness{objectID: objectID, witness: witness})
		log.Trace(vm.logging, "GetBuilderWitnesses: exporting storage witness", "address", address.Hex(), "shardLD", shardID.Ld, "entryCount", len(shard.Entries))
	}

	msWitnesses := vm.hostenv.GetWitnesses()

	for msobjectID, ms := range msWitnesses {
		log.Trace(log.EVM, "Processing meta-shard witness", "metaShardObjectID", msobjectID)
		if ms.ObjectProofs == nil {
			ms.ObjectProofs = make(map[common.Hash][]common.Hash)
		}

		// Add a witness for the meta-shard object itself
		msWitness := types.StateWitness{
			ServiceID:           ms.ServiceID,
			ObjectID:            ms.ObjectID,
			Ref:                 ms.Ref,
			Path:                ms.Path,
			Value:               ms.Value,
			Payload:             ms.MetaShardPayload,
			MetaShardMerkleRoot: ms.MetaShardMerkleRoot,
			MetaShardPayload:    ms.MetaShardPayload,
			BlockNumber:         ms.BlockNumber,
			Timeslot:            ms.Timeslot,
			ObjectRefs:          ms.ObjectRefs,
			Payloads:            ms.Payloads,
			ObjectProofs:        ms.ObjectProofs,
		}
		sortableRefs = append(sortableRefs, sortableWitness{objectID: ms.ObjectID, witness: msWitness})

		for objectID, ref := range ms.ObjectRefs {
			if ref.WorkPackageHash == (common.Hash{}) {
				return nil, nil, fmt.Errorf("invalid witness for object %s: empty work_package_hash", objectID.Hex())
			}
			// if ref.PayloadLength == 0 {
			// 	return nil, nil, fmt.Errorf("invalid witness for object %s: zero payload_length", objectID.Hex())
			// }

			// Generate proof if missing
			proof := ms.ObjectProofs[objectID]
			if len(proof) == 0 && len(ms.MetaShardPayload) > 0 {
				entries, err := parseMetaShardEntries(ms.MetaShardPayload)
				if err == nil {
					proof, err = generateMetaShardInclusionProof(ms.MetaShardMerkleRoot, objectID, entries)
					if err == nil {
						ms.ObjectProofs[objectID] = proof
					} else {
						log.Warn(vm.logging, "Failed to generate meta-shard inclusion proof (builder path)",
							"objectID", objectID.Hex(), "error", err)
					}
				} else {
					log.Warn(vm.logging, "Failed to parse meta-shard payload for proof generation",
						"objectID", objectID.Hex(), "error", err)
				}
				// TODO: supply objectID, ref + proof in the big extrinsic of proof data
			}

			// Add import segments for this object
			numSegments, _ := types.CalculateSegmentsAndLastBytes(ref.PayloadLength)
			log.Trace(log.EVM, "Adding import segments - object", "objectID", objectID, "workPackageHash", ref.WorkPackageHash,
				"start", ref.IndexStart, "numSegments", numSegments, "end", ref.IndexStart+numSegments)
			for idx := ref.IndexStart; idx < ref.IndexStart+numSegments; idx++ {
				importedSegments = append(importedSegments, types.ImportSegment{
					RequestedHash: ref.WorkPackageHash,
					Index:         idx,
				})
			}
		}
		ref := ms.Ref
		numSegments, _ := types.CalculateSegmentsAndLastBytes(ref.PayloadLength)
		log.Trace(log.EVM, "Adding import segment - metashard", "objectID", msobjectID, "workPackageHash", ref.WorkPackageHash,
			"start", ref.IndexStart, "numSegments", numSegments, "end", ref.IndexStart+numSegments,
			"payloadLength", ref.PayloadLength)
		for idx := ref.IndexStart; idx < ref.IndexStart+numSegments; idx++ {
			importedSegments = append(importedSegments, types.ImportSegment{
				RequestedHash: ref.WorkPackageHash,
				Index:         idx,
			})
		}

		sortableRefs = append(sortableRefs, sortableWitness{objectID: msobjectID, witness: *ms})
	}

	// Sort witnesses by objectID for deterministic ordering
	sort.Slice(sortableRefs, func(i, j int) bool {
		return bytes.Compare(sortableRefs[i].objectID[:], sortableRefs[j].objectID[:]) < 0
	})

	witnessSlice := make([]types.StateWitness, 0, len(sortableRefs))
	for _, sortable := range sortableRefs {
		witnessSlice = append(witnessSlice, sortable.witness)
	}
	log.Trace(log.EVM, "Compiled builder witnesses", "numWitnesses", len(witnessSlice), "numImportSegments", len(importedSegments))
	return importedSegments, witnessSlice, nil
}

// Register layout:
//
//	r7: fetch_type (0=Balance, 1=Nonce, 2=Code, 3=CodeHash, 4=Storage)
//	r8: address_ptr (pointer to 20-byte address)
//	r9: key_ptr (pointer to 32-byte storage key, only for Storage)
//	r10: output_ptr (pointer to output buffer)
//	r11: output_max_len (0 = query size, >0 = fetch data)
//
// Returns (via r7):
//
//	Step 1 (output_max_len=0): actual size needed
//	Step 2 (output_max_len>0): bytes written (0 = insufficient buffer or not found)
func (vm *VM) hostFetchUBT() {
	fetchType := uint8(vm.ReadRegister(7))
	addressPtr := uint32(vm.ReadRegister(8))
	keyPtr := uint32(vm.ReadRegister(9))
	outputPtr := uint32(vm.ReadRegister(10))
	outputMaxLen := uint32(vm.ReadRegister(11))
	txIndex := uint32(vm.ReadRegister(12))

	evmStorage, ok := getEVMStorage(vm.hostenv)
	if !ok {
		vm.WriteRegister(7, 0)
		return
	}

	// Read address (20 bytes)
	var address common.Address
	addrBytes, errCode := vm.ReadRAMBytes(addressPtr, 20)
	if errCode != OK {
		vm.WriteRegister(7, 0)
		return
	}
	copy(address[:], addrBytes)

	vm.DebugHostFunction(FETCH_UBT, "fetchType=%d address=%s outputMaxLen=%d txIndex=%d",
		fetchType, address.Hex(), outputMaxLen, txIndex)

	switch fetchType {
	case FETCH_BALANCE:
		vm.fetchBalance(evmStorage, address, outputPtr, outputMaxLen, txIndex)
	case FETCH_NONCE:
		vm.fetchNonce(evmStorage, address, outputPtr, outputMaxLen, txIndex)
	case FETCH_CODE:
		vm.fetchCode(evmStorage, address, outputPtr, outputMaxLen, txIndex)
	case FETCH_CODE_HASH:
		vm.fetchCodeHash(evmStorage, address, outputPtr, outputMaxLen, txIndex)
	case FETCH_STORAGE:
		// Read storage key (32 bytes)
		var storageKey [32]byte
		keyBytes, errCode2 := vm.ReadRAMBytes(keyPtr, 32)
		if errCode2 != OK {
			vm.WriteRegister(7, 0)
			return
		}
		copy(storageKey[:], keyBytes)
		vm.fetchStorage(evmStorage, address, storageKey, outputPtr, outputMaxLen, txIndex)
	default:
		// Unknown fetch type - return 0
		vm.WriteRegister(7, 0)
	}
}

// fetchBalance fetches balance from UBT tree
func (vm *VM) fetchBalance(evmStorage types.EVMJAMStorage, address common.Address, outputPtr uint32, outputMaxLen uint32, txIndex uint32) {
	// Step 1: Query size
	if outputMaxLen == 0 {
		vm.WriteRegister(7, 32) // Balance is always 32 bytes
		return
	}

	// Step 2: Fetch data
	if outputMaxLen < 32 {
		vm.WriteRegister(7, 0) // Insufficient buffer
		return
	}

	balance, err := evmStorage.FetchBalance(address, txIndex)
	if err != nil {
		log.Trace(vm.logging, "fetchBalance error", "address", address.Hex(), "txIndex", txIndex, "err", err)
		vm.WriteRegister(7, 0)
		return
	}

	log.Trace(vm.logging, "fetchBalance", "address", address.Hex(), "balanceBE", fmt.Sprintf("%x", balance[:]), "txIndex", txIndex)

	// Write to output
	if errCode := vm.WriteRAMBytes(outputPtr, balance[:]); errCode != OK {
		vm.WriteRegister(7, 0)
		return
	}
	vm.WriteRegister(7, 32)
}

// fetchNonce fetches nonce from UBT tree
func (vm *VM) fetchNonce(evmStorage types.EVMJAMStorage, address common.Address, outputPtr uint32, outputMaxLen uint32, txIndex uint32) {
	// Step 1: Query size
	if outputMaxLen == 0 {
		vm.WriteRegister(7, 32) // Nonce is always 32 bytes
		return
	}

	// Step 2: Fetch data
	if outputMaxLen < 32 {
		vm.WriteRegister(7, 0) // Insufficient buffer
		return
	}

	nonce, err := evmStorage.FetchNonce(address, txIndex)
	if err != nil {
		log.Trace(vm.logging, "fetchNonce error", "address", address.Hex(), "txIndex", txIndex, "err", err)
		vm.WriteRegister(7, 0)
		return
	}

	log.Trace(vm.logging, "fetchNonce", "address", address.Hex(), "nonceBE", fmt.Sprintf("%x", nonce[:]), "txIndex", txIndex)

	// Write to output
	if errCode := vm.WriteRAMBytes(outputPtr, nonce[:]); errCode != OK {
		vm.WriteRegister(7, 0)
		return
	}
	vm.WriteRegister(7, 32)
}

// fetchCode fetches code from UBT tree (two-step pattern)
func (vm *VM) fetchCode(evmStorage types.EVMJAMStorage, address common.Address, outputPtr uint32, outputMaxLen uint32, txIndex uint32) {
	code, codeSize, err := evmStorage.FetchCode(address, txIndex)
	if err != nil {
		log.Info(vm.logging, "fetchCode error", "address", address.Hex(), "err", err)
		vm.WriteRegister(7, 0)
		return
	}

	// Step 1: Query size
	if outputMaxLen == 0 {
		vm.WriteRegister(7, uint64(codeSize))
		return
	}

	// Step 2: Fetch data
	if outputMaxLen < codeSize {
		vm.WriteRegister(7, 0) // Insufficient buffer
		return
	}

	if codeSize == 0 {
		vm.WriteRegister(7, 0) // No code
		return
	}

	// Write code to output
	if errCode := vm.WriteRAMBytes(outputPtr, code); errCode != OK {
		vm.WriteRegister(7, 0)
		return
	}
	vm.WriteRegister(7, uint64(len(code)))
}

// fetchCodeHash fetches code hash from UBT tree
func (vm *VM) fetchCodeHash(evmStorage types.EVMJAMStorage, address common.Address, outputPtr uint32, outputMaxLen uint32, txIndex uint32) {
	// Step 1: Query size
	if outputMaxLen == 0 {
		vm.WriteRegister(7, 32) // Code hash is always 32 bytes
		return
	}

	// Step 2: Fetch data
	if outputMaxLen < 32 {
		vm.WriteRegister(7, 0) // Insufficient buffer
		return
	}

	codeHash, err := evmStorage.FetchCodeHash(address, txIndex)
	if err != nil {
		log.Info(vm.logging, "fetchCodeHash error", "address", address.Hex(), "txIndex", txIndex, "err", err)
		vm.WriteRegister(7, 0)
		return
	}

	// Write to output
	if errCode := vm.WriteRAMBytes(outputPtr, codeHash[:]); errCode != OK {
		vm.WriteRegister(7, 0)
		return
	}
	vm.WriteRegister(7, 32)
}

// fetchStorage fetches storage value from UBT tree
func (vm *VM) fetchStorage(evmStorage types.EVMJAMStorage, address common.Address, storageKey [32]byte, outputPtr uint32, outputMaxLen uint32, txIndex uint32) {
	// Step 1: Query size
	if outputMaxLen == 0 {
		vm.WriteRegister(7, 32) // Storage values are always 32 bytes
		return
	}

	// Step 2: Fetch data
	if outputMaxLen < 32 {
		vm.WriteRegister(7, 0) // Insufficient buffer
		return
	}

	value, found, err := evmStorage.FetchStorage(address, storageKey, txIndex)
	if err != nil || !found {
		log.Info(vm.logging, "fetchStorage miss", "address", address.Hex(), "err", err)
		vm.WriteRegister(7, 0)
		return
	}

	// Write to output
	if errCode := vm.WriteRAMBytes(outputPtr, value[:]); errCode != OK {
		vm.WriteRegister(7, 0)
		return
	}
	vm.WriteRegister(7, 32)
}
