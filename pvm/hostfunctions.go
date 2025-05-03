package pvm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// Appendix B - Host function
// Host function indexes
const (
	GAS               = 0
	LOOKUP            = 1
	READ              = 2
	WRITE             = 3
	INFO              = 4
	BLESS             = 5
	ASSIGN            = 6
	DESIGNATE         = 7
	CHECKPOINT        = 8
	NEW               = 9
	UPGRADE           = 10
	TRANSFER          = 11
	EJECT             = 12
	QUERY             = 13
	SOLICIT           = 14
	FORGET            = 15
	YIELD             = 16
	PROVIDE           = 27 // TEMPORARY
	HISTORICAL_LOOKUP = 17
	FETCH             = 18
	EXPORT            = 19
	MACHINE           = 20
	PEEK              = 21
	POKE              = 22
	ZERO              = 23
	VOID              = 24
	INVOKE            = 25
	EXPUNGE           = 26

	MANIFEST = 64

	LOG = 100
)

const maxUint64 = ^uint64(0)

const (
	Debug_Service_Storage = false
)

const (
	g = 10
)

// Mapping of host function names to their error cases
var errorCases = map[string][]uint64{
	// B.6 General Functions
	"Gas":    {OK},
	"Lookup": {OK, NONE, OOB},
	"Read":   {OK, OOB, NONE},
	"Write":  {OK, OOB, NONE, FULL},
	"Info":   {OK, OOB, NONE},
	// B.7 Accumulate Functions
	"Bless":      {OK, OOB, WHO},
	"Assign":     {OK, OOB, CORE},
	"Designate":  {OK, OOB},
	"Checkpoint": {OK},
	"New":        {OK, OOB, CASH},
	"Upgrade":    {OK, OOB},
	"Transfer":   {OK, WHO, CASH, LOW},
	"Quit":       {OK, OOB, WHO, LOW},
	"Solicit":    {OK, OOB, FULL, HUH},
	"Forget":     {OK, OOB, HUH},
	"Provide":    {OK, WHO, HUH},
	// B.8 Refine Functions
	"Historical_lookup": {OK, OOB},
	"Import":            {OK, OOB, NONE},
	"Export":            {OK, OOB, NONE},
	"Machine":           {OK, OOB},
	"Peek":              {OK, OOB, WHO},
	"Poke":              {OK, OOB, WHO},
	"Zero":              {OK, OOB, WHO},
	"Void":              {OK, OOB, WHO},
	"Invoke":            {OK, OOB, WHO, types.PVM_HOST, types.PVM_FAULT, OOB, types.PVM_PANIC},
	"Expunge":           {OK, OOB, WHO},
}

// Mapping of host function names to their indexes
var hostIndexMap = map[string]int{
	// B.6 General Functions
	"Gas":    GAS,
	"Lookup": LOOKUP,
	"Read":   READ,
	"Write":  WRITE,
	"Info":   INFO,

	// B.7 Accumulate Functions
	"Bless":      BLESS,
	"Assign":     ASSIGN,
	"Designate":  DESIGNATE,
	"Checkpoint": CHECKPOINT,
	"New":        NEW,
	"Upgrade":    UPGRADE,
	"Transfer":   TRANSFER,
	"Eject":      EJECT,
	"Query":      QUERY,
	"Fetch":      FETCH,
	"Yield":      YIELD,
	"Solicit":    SOLICIT,
	"Forget":     FORGET,
	"Provide":    PROVIDE,
	// B.8 Refine Functions
	"Historical_lookup": HISTORICAL_LOOKUP,
	"Export":            EXPORT,
	"Machine":           MACHINE,
	"Peek":              PEEK,
	"Poke":              POKE,
	"Zero":              ZERO,
	"Void":              VOID,
	"Invoke":            INVOKE,
	"Expunge":           EXPUNGE,

	// Other
	"Log":      LOG,
	"Manifest": MANIFEST,
}

// Function to retrieve index and error cases
func GetHostFunctionDetails(name string) (int, []uint64) {
	index, exists := hostIndexMap[name]
	if !exists {
		return 0, nil
	}
	errors, hasErrors := errorCases[name]
	if !hasErrors {
		errors = []uint64{} // No error cases defined
	}
	return index, errors
}

type RefineM struct {
	P []byte `json:"P"`
	U *RAM   `json:"U"`
	I uint64 `json:"I"`
}

// GP-0.5 B.5
type Refine_parameters struct {
	Gas                  uint64
	Ram                  *RAM
	Register             []uint32
	Machine              map[uint32]*RefineM
	Export_segment       [][]byte
	Import_segement      [][]byte
	Export_segment_index uint32
	service_index        uint32
	Delta                map[uint32]uint32
	C_t                  uint32
}

func (vm *VM) chargeGas(host_fn int) {
	beforeGas := vm.Gas
	chargedGas := uint64(10)
	exp := fmt.Sprintf("HOSTFUNC %d", host_fn)

	switch host_fn {
	case TRANSFER:
		omega_9, _ := vm.ReadRegister(9)
		chargedGas = omega_9 + 10
		exp = "TRANSFER"
	case READ:
		exp = "READ"
	case WRITE:
		exp = "WRITE"
	case NEW:
		exp = "NEW"
	case FETCH:
		exp = "FETCH"
	case EXPORT:
		exp = "EXPORT"
	case GAS:
		exp = "GAS"
	case LOOKUP:
		exp = "LOOKUP"
	case INFO:
		exp = "INFO"
	case BLESS:
		exp = "BLESS"
	case ASSIGN:
		exp = "ASSIGN"
	case DESIGNATE:
		exp = "DESIGNATE"
	case CHECKPOINT:
		exp = "CHECKPOINT"
	case UPGRADE:
		exp = "UPGRADE"
	case EJECT:
		exp = "EJECT"
	case QUERY:
		exp = "QUERY"
	case SOLICIT:
		exp = "SOLICIT"
	case FORGET:
		exp = "FORGET"
	case YIELD:
		exp = "YIELD"
	case PROVIDE:
		exp = "PROVIDE"
	case HISTORICAL_LOOKUP:
		exp = "HISTORICAL_LOOKUP"
	case MACHINE:
		exp = "MACHINE"
	case PEEK:
		exp = "PEEK"
	case POKE:
		exp = "POKE"
	case ZERO:
		exp = "ZERO"
	case VOID:
		exp = "VOID"
	case INVOKE:
		exp = "INVOKE"
	case EXPUNGE:
		exp = "EXPUNGE"
	case LOG:
		exp = "LOG"
		chargedGas = 0
	}

	vm.Gas = beforeGas - int64(chargedGas)
	log.Debug(vm.logging, vm.Str(exp), "reg", vm.ReadRegisters(), "gasCharged", chargedGas, "beforeGas", beforeGas, "afterGas", vm.Gas)
}

// InvokeHostCall handles host calls
// Returns true if the call results in a halt condition, otherwise false
func (vm *VM) InvokeHostCall(host_fn int) (bool, error) {

	if vm.Gas-g < 0 {
		vm.ResultCode = types.PVM_OOG
		return true, fmt.Errorf("Out of gas\n")
	}
	vm.chargeGas(host_fn)

	switch host_fn {
	case GAS:
		vm.hostGas()
		return true, nil

	case LOOKUP:
		vm.hostLookup()
		return true, nil

	case READ:
		vm.hostRead()
		return true, nil

	case WRITE:
		vm.hostWrite()
		return true, nil

	case INFO:
		vm.hostInfo()
		return true, nil

	case BLESS:
		vm.hostBless()
		return true, nil

	case ASSIGN:
		vm.hostAssign()
		return true, nil

	case DESIGNATE:
		vm.hostDesignate()
		return true, nil

	case CHECKPOINT:
		vm.hostCheckpoint()
		return true, nil

	case NEW:
		vm.hostNew()
		return true, nil

	case UPGRADE:
		vm.hostUpgrade()
		return true, nil

	case TRANSFER:
		vm.hostTransfer()
		return true, nil

	case EJECT:
		vm.hostEject()
		return true, nil

	case QUERY:
		vm.hostQuery()
		return true, nil

	case SOLICIT:
		vm.hostSolicit()
		return true, nil

	case FORGET:
		// t := vm.hostenv.GetTimeslot()
		vm.hostForget()
		return true, nil

	case YIELD:
		vm.hostYield()
		return true, nil

	case PROVIDE:
		vm.hostProvide()
		return true, nil

	// Refine functions
	case HISTORICAL_LOOKUP:
		vm.hostHistoricalLookup()
		return true, nil

	case FETCH:
		vm.hostFetch()
		return true, nil

	case EXPORT:
		vm.hostExport()
		return true, nil

	case MACHINE:
		vm.hostMachine()
		return true, nil

	case PEEK:
		vm.hostPeek()
		return true, nil

	case POKE:
		vm.hostPoke()
		return true, nil

	case ZERO:
		vm.hostZero()
		return true, nil

	case VOID:
		vm.hostVoid()
		return true, nil

	case INVOKE:
		vm.hostInvoke()
		return true, nil

	case EXPUNGE:
		vm.hostExpunge()
		return true, nil

	case LOG:
		vm.hostLog()
		return true, nil

	case MANIFEST:
		vm.hostManifest()
		return true, nil

	default:
		vm.Gas = vm.Gas + g
		return false, fmt.Errorf("unknown host call: %d\n", host_fn)
	}
}

// Information-on-Service
func (vm *VM) hostInfo() {
	omega_7, _ := vm.ReadRegister(7)

	t, errCode := vm.getXUDS(omega_7)
	if errCode != OK {
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		log.Debug(vm.logging, vm.Str("INFO NONE"), "s", omega_7)
		return
	}
	bo, _ := vm.ReadRegister(8)

	var buf bytes.Buffer

	elements := []interface{}{t.CodeHash, t.Balance, t.ComputeThreshold(), t.GasLimitG, t.GasLimitM, t.NumStorageItems, t.StorageSize}

	for _, elem := range elements {
		encoded, err := types.Encode(elem)
		if err != nil {
			vm.WriteRegister(7, NONE)
			vm.HostResultCode = NONE
			log.Debug(vm.logging, vm.Str("INFO NONE"), "s", omega_7)
			return
		}
		buf.Write(encoded)
	}

	m := buf.Bytes()
	errcode := vm.Ram.WriteRAMBytes(uint32(bo), m[:])
	if errcode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	log.Debug(vm.logging, vm.Str("INFO OK"), "s", fmt.Sprintf("%d", omega_7), "info", fmt.Sprintf("%v", elements), "bytes", fmt.Sprintf("%x", m))

	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// Bless updates
func (vm *VM) hostBless() {
	m, _ := vm.ReadRegister(7)
	a, _ := vm.ReadRegister(8)
	v, _ := vm.ReadRegister(9)
	o, _ := vm.ReadRegister(10)
	n, _ := vm.ReadRegister(11)

	if m > (1<<32)-1 || a > (1<<32)-1 || v > (1<<32)-1 {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		log.Debug(vm.logging, vm.Str("BLESS WHO"), "m", fmt.Sprintf("%d", m), "a", fmt.Sprintf("%d", a), "v", fmt.Sprintf("%d", v))
		return
	}

	bold_g := make(map[uint32]uint64)
	for i := 0; i < int(n); i++ {
		data, err := vm.Ram.ReadRAMBytes(uint32(o)+uint32(i)*12, 12)
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.PVM_PANIC
			return
		}
		s := binary.LittleEndian.Uint32(data[0:4])
		g := binary.LittleEndian.Uint64(data[4:12])
		bold_g[s] = g
	}

	vm.X.U.PrivilegedState.Kai_g = bold_g

	// Set (x'p)_m, (x'p)_a, (x'p)_v
	vm.X.U.PrivilegedState.Kai_m = uint32(m)
	vm.X.U.PrivilegedState.Kai_a = uint32(a)
	vm.X.U.PrivilegedState.Kai_v = uint32(v)

	vm.WriteRegister(7, OK)
	log.Debug(vm.logging, vm.Str("BLESS OK"), "m", fmt.Sprintf("%d", m), "a", fmt.Sprintf("%d", a), "v", fmt.Sprintf("%d", v), "g", fmt.Sprintf("%v", bold_g))
	vm.HostResultCode = OK
}

// Assign Core x_c[i]
func (vm *VM) hostAssign() {
	core, _ := vm.ReadRegister(7)
	if core >= numCores {
		vm.WriteRegister(7, CORE)
		vm.HostResultCode = CORE
		log.Debug(vm.logging, vm.Str("ASSIGN CORE"), "c", core)
		return
	}
	o, _ := vm.ReadRegister(8)
	c, errcode := vm.Ram.ReadRAMBytes(uint32(o), 32*types.MaxAuthorizationQueueItems)
	if errcode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = types.PVM_PANIC
		return
	}
	qi := make([]common.Hash, types.MaxAuthorizationQueueItems)
	for i := 0; i < types.MaxAuthorizationQueueItems; i++ {
		qi[i] = common.BytesToHash(c[i*32 : (i+1)*32])
	}
	copy(vm.X.U.QueueWorkReport[core][:], qi[:])
	log.Debug(vm.logging, vm.Str("ASSIGN OK"), "c", core)
	vm.HostResultCode = OK
}

// Designate validators
func (vm *VM) hostDesignate() {
	o, _ := vm.ReadRegister(7)
	v, errCode := vm.Ram.ReadRAMBytes(uint32(o), 176*V)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	qi := make([]types.Validator, V)
	for i := 0; i < types.TotalValidators; i++ {
		newv := types.Validator{}
		copy(newv.Bandersnatch[:], v[0:32])
		copy(newv.Ed25519[:], v[64:92])
		copy(newv.Bls[:], v[92:128])
		copy(newv.Metadata[:], v[128:])

		qi[i] = newv
	}
	vm.X.U.UpcomingValidators = qi
	log.Debug(vm.logging, vm.Str("DESIGNATE OK"))
	vm.HostResultCode = OK
}

// Checkpoint gets Gas-remaining
func (vm *VM) hostCheckpoint() {
	vm.Y = vm.X.Clone()
	vm.Y.U.Checkpoint()
	vm.WriteRegister(7, uint64(vm.Gas)) // CHECK
	log.Debug(vm.logging, vm.Str("CHECKPOINT"), "g", fmt.Sprintf("%d", vm.Gas))
	vm.HostResultCode = OK
}

// implements https://graypaper.fluffylabs.dev/#/5f542d7/313103313103
func new_check(i uint32, u_d map[uint32]*types.ServiceAccount) uint32 {
	bump := uint32(1)
	for {
		if _, ok := u_d[i]; !ok {
			return i
		}
		// aligns the final result with the desired range: [2^8, 2^32 - 2^9] = [256, 4294966784]
		i = uint32(256) + uint32(i-256+bump)%(uint32(4294966784))
		// ump = 1 // initially 42 coming in from hostNew
	}
}

// New service
func (vm *VM) hostNew() {
	xContext := vm.X
	xs, _ := xContext.GetX_s()

	// put 'g' and 'm' together
	o, _ := vm.ReadRegister(7)
	c, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	l, _ := vm.ReadRegister(8)
	g, _ := vm.ReadRegister(9)
	m, _ := vm.ReadRegister(10)

	x_s_t := xs.ComputeThreshold()
	if xs.Balance < x_s_t {
		vm.WriteRegister(7, CASH)
		vm.HostResultCode = CASH //balance insufficient
		log.Debug(vm.logging, vm.Str("NEW CASH xs.Balance < x_s_t"), "xs.Balance", xs.Balance, "x_s_t", x_s_t)
		return
	}

	// xs has enough balance to fund service creation of a AND covering its own threshold

	xi := xContext.I
	// simulate a with c, g, m
	a := &types.ServiceAccount{
		ServiceIndex:    xi,
		Mutable:         false,
		Dirty:           false,
		CodeHash:        common.BytesToHash(c),
		GasLimitG:       uint64(g),
		GasLimitM:       uint64(m),
		NumStorageItems: 2,              //a_s = 2⋅∣al∣+∣as∣
		StorageSize:     uint64(81 + l), //a_l =  ∑ 81+z per (h,z) + ∑ 32+s
		Storage:         make(map[common.Hash]types.StorageObject),
		Lookup:          make(map[common.Hash]types.LookupObject),
		Preimage:        make(map[common.Hash]types.PreimageObject),
		Checkpointed:    false, // this is updated to true upon Checkpoint
		NewAccount:      true,  // with this flag, if an account is Dirty OR Checkpointed && NewAccount then it is written
	}
	a.ALLOW_MUTABLE()
	a.Balance = a.ComputeThreshold()
	xs.DecBalance(a.Balance) // (x's)b <- (xs)b - at
	xContext.I = new_check(uint32(256)+uint32(xi-256+42)%(uint32(4294966784)), xContext.U.D)
	a.WriteLookup(common.BytesToHash(c), uint32(l), []uint32{})

	xContext.U.D[xi] = a // this new account is included but only is written if (a) non-exceptional (b) exceptional and checkpointed
	vm.WriteRegister(7, uint64(xi))
	vm.HostResultCode = OK
	log.Debug(vm.logging, vm.Str("NEW OK"), "SERVICE", fmt.Sprintf("%d", xi), "code_hash_ptr", fmt.Sprintf("%x", o), "code_hash_ptr", fmt.Sprintf("%x", c), "code_len", l, "min_item_gas", g, "min_memo_gas", m)
}

// Upgrade service
func (vm *VM) hostUpgrade() {
	xContext := vm.X
	xs, _ := xContext.GetX_s()
	o, _ := vm.ReadRegister(7)
	g, _ := vm.ReadRegister(8)
	m, _ := vm.ReadRegister(9)

	c, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}

	xs.Dirty = true
	xs.CodeHash = common.BytesToHash(c)
	xs.GasLimitG = g
	xs.GasLimitM = m
	vm.WriteRegister(7, OK)
	// xContext.D[s] = xs // not sure if this is needed
	log.Debug(vm.logging, vm.Str("UPGRADE OK"), "code_hash", fmt.Sprintf("%x", o), "code_hash_ptr", fmt.Sprintf("%x", c), "min_item_gas", g, "min_memo_gas", m)
	vm.HostResultCode = OK
}

// Transfer host call
func (vm *VM) hostTransfer() {
	d, _ := vm.ReadRegister(7)
	a, _ := vm.ReadRegister(8)
	g, _ := vm.ReadRegister(9)
	o, _ := vm.ReadRegister(10)

	xs, _ := vm.X.GetX_s()

	D := vm.X.U.D

	var receiver *types.ServiceAccount
	var founded bool
	receiver, founded = D[uint32(d)]
	if !founded {
		receiver, founded, _ = vm.hostenv.GetService(uint32(d))
		if !founded {
			vm.WriteRegister(7, WHO)
			vm.HostResultCode = WHO
			log.Debug(vm.logging, vm.Str("TRANSFER WHO"), "d", d)
			return
		}
		vm.X.U.D[uint32(d)] = receiver
	}

	m, errCode := vm.Ram.ReadRAMBytes(uint32(o), M)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	t := types.DeferredTransfer{Amount: a, GasLimit: g, SenderIndex: vm.X.S, ReceiverIndex: uint32(d)} // CHECK

	if g < receiver.GasLimitM {
		vm.WriteRegister(7, LOW)
		vm.HostResultCode = LOW
		log.Debug(vm.logging, vm.Str("TRANSFER LOW"), "g", g, "GasLimitM", receiver.GasLimitM)
		return
	}

	if xs.Balance < xs.ComputeThreshold() {
		vm.WriteRegister(7, CASH)
		vm.HostResultCode = CASH
		log.Debug(vm.logging, vm.Str("TRANSFER CASH"), "xs.Balance", xs.Balance, "xs_t", xs.ComputeThreshold())
		return
	}

	xs.DecBalance(a)
	copy(t.Memo[:], m[:])
	vm.X.T = append(vm.X.T, t)
	log.Debug(vm.logging, vm.Str("TRANSFER OK"), "sender", fmt.Sprintf("%d", t.SenderIndex), "receiver", fmt.Sprintf("%d", d), "amount", fmt.Sprintf("%d", a), "gaslimit", g)
	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// Gas Service
func (vm *VM) hostGas() {
	vm.WriteRegister(7, uint64(vm.Gas-10)) // its gas remaining AFTER the host call
	vm.HostResultCode = OK
}

func (vm *VM) hostQuery() {
	o, _ := vm.ReadRegister(7)
	z, _ := vm.ReadRegister(8)
	h, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	a, _ := vm.X.GetX_s()
	account_lookuphash := common.BytesToHash(h)
	ok, anchor_timeslot := a.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		vm.WriteRegister(7, NONE)
		vm.WriteRegister(8, 0)
		vm.HostResultCode = NONE
		log.Debug(vm.logging, vm.Str("QUERY NONE"), "h", account_lookuphash, "z", z)
		return
	}
	switch len(anchor_timeslot) {
	case 0:
		vm.WriteRegister(7, 0)
		vm.WriteRegister(8, 0)
		break
	case 1:
		x := anchor_timeslot[0]
		vm.WriteRegister(7, 1+(1<<32)*uint64(x))
		vm.WriteRegister(8, 0)
		log.Debug(vm.logging, vm.Str("QUERY 1"), "x", x)
		break
	case 2:
		x := anchor_timeslot[0]
		y := anchor_timeslot[1]
		vm.WriteRegister(7, 2+(1<<32)*uint64(x))
		vm.WriteRegister(8, uint64(y))
		log.Debug(vm.logging, vm.Str("QUERY 2"), "x", x, "y", y)
		break
	case 3:
		x := anchor_timeslot[0]
		y := anchor_timeslot[1]
		z := anchor_timeslot[2]
		log.Debug(vm.logging, vm.Str("QUERY 3"), "x", x, "y", y, "z", z)
		vm.WriteRegister(7, 3+(1<<32)*uint64(x))
		vm.WriteRegister(8, uint64(y)+(1<<32)*uint64(z))
		break
	}
	w7, _ := vm.ReadRegister(7)
	w8, _ := vm.ReadRegister(8)
	log.Debug(vm.logging, vm.Str("QUERY OK"), "h", account_lookuphash, "z", z, "w7", w7, "w8", w8, "len(anchor_timeslot)", len(anchor_timeslot))
	vm.HostResultCode = OK
}

/*
Reading is windowed, so both a length and offset is specified for the buffer. This allows streaming and avoids allocating more data into the PVM than necessary if only a small piece of the overall data is needed.

ω7 : The buffer pointer.
ω8 mod 2^32 : The maximum number of bytes of data to copy into the buffer.
⌊ω8 ÷ 2^32⌋: The offset into the input data to read from.
ω9 : The data type ID.

1: The authorizer output
2: Payload data; the work-item index is ω10
16: An extrinsic; the work-item index is ω10, the extrinsic index within the sequence specified by that work-item is ω11.
17: An extrinsic by index within the sequence specified by this work-item; the index is ω10
18: An extrinsic by hash; the data returned hashes to the 32 bytes found at ω10
32: An imported segment; the work-item index is ω10, the import index within the sequence specified by that work-item is ω11.
33: An imported segment by index within the sequence specified by this work-item; the index is ω10
3: number of extrinsics in WP
4: number of work items in WP
*/
func (vm *VM) hostFetch() {
	o, _ := vm.ReadRegister(7)
	omega_8, _ := vm.ReadRegister(8)
	omega_9, _ := vm.ReadRegister(9)
	datatype, _ := vm.ReadRegister(10)
	omega_11, _ := vm.ReadRegister(11)
	omega_12, _ := vm.ReadRegister(12)
	log.Debug(vm.logging, vm.Str("FETCH"), "datatype", datatype, "omega_8", omega_8, "omega_9", omega_9, "omega_11", omega_11, "omega_12", omega_12, "vm.Extrinsics", fmt.Sprintf("%x", vm.Extrinsics))
	var v_Bytes []byte
	switch datatype {
	case 0:
		v_Bytes, _ = types.Encode(vm.WorkPackage)
	case 1:
		v_Bytes = vm.Authorization
	case 2:
		if omega_11 < uint64(len(vm.WorkPackage.WorkItems)) {
			v_Bytes = append([]byte{}, vm.WorkPackage.WorkItems[omega_11].Payload...)
		}
	case 3:
		if omega_11 < uint64(len(vm.WorkPackage.WorkItems)) && omega_12 < uint64(len(vm.WorkPackage.WorkItems[omega_11].Extrinsics)) {
			// get extrinsic by omega 11 and omega 12
			extrinsicHash := common.Blake2Hash(vm.Extrinsics[omega_11][:])
			extrinsicLength := len(vm.Extrinsics[omega_11])
			workitemExtrinisc := types.WorkItemExtrinsic{
				Hash: extrinsicHash,
				Len:  uint32(extrinsicLength),
			}
			if vm.WorkPackage.WorkItems[omega_11].Extrinsics[omega_12] == workitemExtrinisc {
				v_Bytes = append([]byte{}, vm.Extrinsics[omega_11][:]...)
				//fmt.Printf("***** FETCH EXTRINSIC %s %d %x (%d bytes)\n", extrinsicHash, extrinsicLength, v_Bytes, len(v_Bytes))
			} else {
				fmt.Printf("***** FETCH %d fail1\n", datatype)
			}
		} else {
			fmt.Printf("***** FETCH %d fail0\n", datatype)
		}
	case 4:
		if omega_11 < uint64(len(vm.WorkPackage.WorkItems[vm.WorkItemIndex].Extrinsics)) {
			// get extrinsic by index within the sequence specified by this work-item
			extrinsicHash := common.Blake2Hash(vm.Extrinsics[vm.WorkItemIndex][:])
			extrinsicLength := len(vm.Extrinsics[vm.WorkItemIndex])
			workitemExtrinisc := types.WorkItemExtrinsic{
				Hash: extrinsicHash,
				Len:  uint32(extrinsicLength),
			}
			if vm.WorkPackage.WorkItems[vm.WorkItemIndex].Extrinsics[omega_11] == workitemExtrinisc {
				v_Bytes = append([]byte{}, vm.Extrinsics[vm.WorkItemIndex][:]...)
			}
		}
	case 5:
		// get imported segment by omega 11 and omega 12
		if omega_11 < uint64(len(vm.Imports)) && omega_12 < uint64(len(vm.Imports[omega_11])) {
			v_Bytes = append([]byte{}, vm.Imports[omega_11][omega_12][:]...)
			log.Debug(log.SegmentMonitoring, fmt.Sprintf("%s Fetch segment hash:", vm.ServiceMetadata), "I", fmt.Sprintf("%v", common.Blake2Hash(v_Bytes)))
			log.Debug(log.SegmentMonitoring, fmt.Sprintf("%s Fetch segment byte:", vm.ServiceMetadata), "I", fmt.Sprintf("%x", v_Bytes))
			log.Debug(log.SegmentMonitoring, fmt.Sprintf("%s Fetch segment  len:", vm.ServiceMetadata), "I", fmt.Sprintf("%d", len(v_Bytes)))
			log.Debug(log.SegmentMonitoring, fmt.Sprintf("%s Fetch segment  idx:", vm.ServiceMetadata), "I", fmt.Sprintf("%d", omega_12))
		}

	case 6:
		// get imported segment by work item index
		if omega_11 < uint64(len(vm.Imports[vm.WorkItemIndex])) {
			v_Bytes = append([]byte{}, vm.Imports[vm.WorkItemIndex][omega_11][:]...)
			log.Debug(vm.logging, fmt.Sprintf("[N%d] %s Fetch imported segment", vm.hostenv.GetID(), vm.ServiceMetadata),
				"h", fmt.Sprintf("%v", common.Blake2Hash(v_Bytes)),
				"bytes", v_Bytes[0:20],
				"l", len(v_Bytes),
				"workItemIndex", vm.WorkItemIndex,
				"w11", omega_11)
		} else {
			// fmt.Printf("FETCH 6 FAIL omega_11 %d vs len(vm.Imports[vm.WorkItemIndex=%d])=%d\n", omega_11, vm.WorkItemIndex, len(vm.Imports[vm.WorkItemIndex]))
		}
	case 7:
		v_Bytes = append([]byte{}, vm.WorkPackage.ParameterizationBlob...)
	}

	if v_Bytes == nil {
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		return
	}

	f := min(uint64(len(v_Bytes)), omega_8)   // offset
	l := min(uint64(len(v_Bytes))-f, omega_9) // max length

	errCode := vm.Ram.WriteRAMBytes(uint32(o), v_Bytes[f:f+l])
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}

	vm.WriteRegister(7, uint64(l))
}

func (vm *VM) hostYield() {
	o, _ := vm.ReadRegister(7)
	h, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	y := common.BytesToHash(h)
	vm.X.Y = y
	vm.WriteRegister(7, OK)
	log.Debug(vm.logging, vm.Str("YIELD OK"), "h", y)
	vm.HostResultCode = OK
}

func (vm *VM) hostProvide() {
	omega_7, _ := vm.ReadRegister(7)
	o, _ := vm.ReadRegister(8)
	z, _ := vm.ReadRegister(9)
	if omega_7 == NONE {
		omega_7 = uint64(vm.Service_index)
	}

	var a *types.ServiceAccount
	a, _ = vm.getXUDS(omega_7)

	if a == nil {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		log.Debug(vm.logging, vm.Str("PROVIDE WHO"), "omega_7", omega_7)
		return
	}

	i, errCode := vm.Ram.ReadRAMBytes(uint32(o), uint32(z))
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}

	h := common.Blake2Hash(i)
	ok, X_s_l := a.ReadLookup(h, uint32(z), vm.hostenv)
	if !ok && len(X_s_l) > 0 {
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, vm.Str("PROVIDE HUH"), "omega_7", omega_7, "h", h, "z", z)
		return
	}

	exists := false
	for _, p := range vm.X.P {
		if p.ServiceIndex == a.ServiceIndex && bytes.Equal(p.P_data, i) {
			exists = true
			break
		}
	}

	if exists {
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, vm.Str("PROVIDE HUH"), "omega_7", omega_7, "h", h, "z", z)
		return
	}

	vm.X.P = append(vm.X.P, types.P{
		ServiceIndex: a.ServiceIndex,
		P_data:       i,
	})

	vm.WriteRegister(7, OK)
	log.Debug(vm.logging, vm.Str("PROVIDE OK"), "omega_7", omega_7, "h", h, "z", z)
	vm.HostResultCode = OK
}

func (vm *VM) hostEject() {
	d, _ := vm.ReadRegister(7)
	o, _ := vm.ReadRegister(8)
	h, err := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if err != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}

	bold_d, ok := vm.X.U.D[uint32(d)]
	if d == uint64(vm.X.S) || !ok || bold_d.CodeHash != common.Hash(types.E_l(uint64(vm.X.S), 32)) {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		log.Debug(vm.logging, vm.Str("EJECT WHO"), "d", fmt.Sprintf("%d", d))
		return
	}
	l := max(81, bold_d.StorageSize) - 81

	ok, D_lookup := bold_d.ReadLookup(common.BytesToHash(h), uint32(l), vm.hostenv)
	if !ok || bold_d.NumStorageItems != 2 {
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, vm.Str("EJECT HUH"), "d", fmt.Sprintf("%d", d), "h", h, "l", l)
		return
	}

	s, _ := vm.getXUDS(uint64(vm.X.S))
	s = s.Clone()
	s.Balance += bold_d.Balance

	if len(D_lookup) == 2 && D_lookup[1] < vm.Timeslot-uint32(types.PreimageExpiryPeriod) {
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
		delete(vm.X.U.D, uint32(d))
		vm.X.U.D[vm.X.S] = s
		log.Debug(vm.logging, vm.Str("EJECT OK"), "d", fmt.Sprintf("%d", d))
		return
	}

	vm.WriteRegister(7, HUH)
	vm.HostResultCode = HUH
}

// Invoke
func (vm *VM) hostInvoke() {
	n, _ := vm.ReadRegister(7)
	o, _ := vm.ReadRegister(8)

	gasBytes, errCodeGas := vm.Ram.ReadRAMBytes(uint32(o), 8)
	if errCodeGas != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}

	g := types.DecodeE_l(gasBytes)

	m_n_reg := make([]uint64, 13)
	for i := 1; i < 14; i++ {
		reg_bytes, errCodeReg := vm.Ram.ReadRAMBytes(uint32(o)+8*uint32(i), 8)
		if errCodeReg != OK {
			vm.terminated = true
			vm.ResultCode = types.PVM_PANIC
			return
		}
		m_n_reg[i-1] = types.DecodeE_l(reg_bytes)
	}
	m_n, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		log.Debug(vm.logging, vm.Str("INVOKE WHO"), "n", n)
		vm.HostResultCode = WHO
		return
	}

	program := DecodeProgram_pure_pvm_blob(m_n.P)
	new_machine := &VM{
		JSize:   program.JSize,
		Z:       program.Z,
		J:       program.J,
		code:    program.Code,
		bitmask: program.K,

		pc:       m_n.I,
		Gas:      int64(g),
		register: m_n_reg,
		Ram:      m_n.U,
	}

	new_machine.logging = vm.logging
	new_machine.Execute(int(new_machine.pc), true)

	m_n.I = new_machine.pc
	m_n.U = new_machine.Ram
	vm.RefineM_map[uint32(n)] = m_n

	// Result after execution
	gasBytes = types.E_l(uint64(new_machine.Gas), 8)
	errCodeGas = vm.Ram.WriteRAMBytes(uint32(o), gasBytes)
	if errCodeGas != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}

	for i := 1; i < 14; i++ {
		reg_bytes := types.E_l(new_machine.register[i-1], 8)
		errCode := vm.Ram.WriteRAMBytes(uint32(o)+8*uint32(i), reg_bytes)
		if errCode != OK {
			vm.terminated = true
			vm.ResultCode = types.PVM_PANIC
			return
		}
	}

	if new_machine.ResultCode == types.PVM_HOST {
		vm.WriteRegister(7, types.PVM_HOST)
		vm.WriteRegister(8, uint64(new_machine.host_func_id))
		m_n.I = new_machine.pc + 1
		return
	}

	if new_machine.ResultCode == types.PVM_FAULT {
		vm.WriteRegister(7, types.PVM_FAULT)
		vm.WriteRegister(8, uint64(new_machine.Fault_address))
		return
	}

	if new_machine.ResultCode == types.PVM_OOG {
		vm.WriteRegister(7, types.PVM_OOG)
		return
	}

	if new_machine.ResultCode == types.PVM_PANIC {
		vm.WriteRegister(7, types.PVM_PANIC)
		return
	}

	if new_machine.ResultCode == types.PVM_HALT {
		vm.WriteRegister(7, types.PVM_HALT)
		return
	}

}

// Lookup preimage
func (vm *VM) hostLookup() {
	omega_7, _ := vm.ReadRegister(7)

	var a *types.ServiceAccount
	if omega_7 == uint64(vm.Service_index) || omega_7 == maxUint64 {
		a = vm.ServiceAccount
	}
	if a == nil {
		a, _ = vm.getXUDS(omega_7)
	}

	h, _ := vm.ReadRegister(8)
	o, _ := vm.ReadRegister(9)
	f, _ := vm.ReadRegister(10)
	l, _ := vm.ReadRegister(11)
	k_bytes, err_k := vm.Ram.ReadRAMBytes(uint32(h), 32)
	if err_k != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}

	var account_blobhash common.Hash

	var v []byte
	var ok bool

	account_blobhash = common.Hash(k_bytes)
	ok, v = a.ReadPreimage(account_blobhash, vm.hostenv)
	if !ok {
		vm.WriteRegister(7, NONE)
		log.Debug(vm.logging, vm.Str("LOOKUP NONE"), "s", fmt.Sprintf("%d", a.ServiceIndex), "h", account_blobhash)
		vm.HostResultCode = NONE
		return
	}
	f = min(f, uint64(len(v)))
	l = min(l, uint64(len(v))-f)

	err := vm.Ram.WriteRAMBytes(uint32(o), v[:l])
	if err != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}

	if len(v) != 0 {
		vm.WriteRegister(7, l)
	}
	log.Debug(vm.logging, vm.Str("LOOKUP OK"), "s", fmt.Sprintf("%d", a.ServiceIndex), "h", h, "len(v)", len(v))
	vm.HostResultCode = OK
}

// Key Idea: fetch potential mutated (with Mutable=true) ServiceAccount from the XContext Partial State (X.U.D),
// which may have been changed
func (vm *VM) getXUDS(serviceindex uint64) (a *types.ServiceAccount, errCode uint64) {
	var ok bool
	var err error
	s := uint32(serviceindex)
	if serviceindex == maxUint64 || uint32(serviceindex) == vm.X.S {
		return vm.X.U.D[s], OK
	}
	a, ok = vm.X.U.D[s]
	if !ok {
		a, ok, err = vm.hostenv.GetService(s)
		if err != nil || !ok {
			return nil, NONE
		}
		vm.X.U.D[s] = a
	}
	return a, OK
}

// Read Storage
func (vm *VM) hostRead() {
	// Assume that all ram can be read and written
	omega_7, _ := vm.ReadRegister(7)

	var a *types.ServiceAccount
	if omega_7 == uint64(vm.Service_index) || omega_7 == maxUint64 {
		a = vm.ServiceAccount
	}
	if a == nil {
		a, _ = vm.getXUDS(omega_7)
	}

	ko, _ := vm.ReadRegister(8)
	kz, _ := vm.ReadRegister(9)
	bo, _ := vm.ReadRegister(10)
	f, _ := vm.ReadRegister(11)
	l, _ := vm.ReadRegister(12)
	mu_k, err_k := vm.Ram.ReadRAMBytes(uint32(ko), uint32(kz)) // this is the raw key.
	if err_k != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	k := common.ServiceStorageKey(a.ServiceIndex, mu_k) // this does E_4(s) ... mu_4
	ok, val := a.ReadStorage(mu_k, k, vm.hostenv)

	if !ok {
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		log.Debug(vm.logging, vm.Str("READ NONE"), "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "k", k, "ok", ok, "val", fmt.Sprintf("%x", val), "len(val)", len(val))
		return
	}
	log.Debug(vm.logging, vm.Str("READ OK"), "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "k", k, "ok", ok, "val", fmt.Sprintf("%x", val), "len(val)", len(val))
	lenval := uint64(len(val))
	f = min(f, lenval)
	l = min(l, lenval-f)
	// TODO: check for OOB case again using o, f + l
	vm.Ram.WriteRAMBytes(uint32(bo), val[f:])
	vm.WriteRegister(7, l)
}

// Write Storage
func (vm *VM) hostWrite() {
	var a *types.ServiceAccount
	a = vm.ServiceAccount
	if a == nil {
		a, _ = vm.getXUDS(uint64(vm.Service_index))
	}
	ko, _ := vm.ReadRegister(7)
	kz, _ := vm.ReadRegister(8)
	vo, _ := vm.ReadRegister(9)
	vz, _ := vm.ReadRegister(10)
	mu_k, err_k := vm.Ram.ReadRAMBytes(uint32(ko), uint32(kz))
	if err_k != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	k := common.ServiceStorageKey(a.ServiceIndex, mu_k) // this does E_4(s) ... mu_4
	a_t := a.ComputeThreshold()
	if a_t > a.Balance {
		vm.WriteRegister(7, FULL)
		vm.HostResultCode = FULL
		log.Debug(vm.logging, vm.Str("WRITE FULL"), "a_t", a_t, "balance", a.Balance)
		return
	}

	l := uint64(NONE)
	exists, oldValue := a.ReadStorage(mu_k, k, vm.hostenv)
	v := []byte{}
	err := uint64(0)
	if vz > 0 {
		v, err = vm.Ram.ReadRAMBytes(uint32(vo), uint32(vz))
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.PVM_PANIC
			return
		}
		l = uint64(len(v))
	}

	a.WriteStorage(a.ServiceIndex, mu_k, k, v)
	vm.HostResultCode = OK
	val_len := uint64(len(v))
	if !exists {
		if val_len > 0 {
			a.NumStorageItems++
			a.StorageSize += (32 + val_len)
			log.Debug(vm.logging, vm.Str("WRITE NONE"), "numStorageItems", a.NumStorageItems, "StorageSize", a.StorageSize, "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "k", k, "v", fmt.Sprintf("%x", v), "vlen", len(v))
		}
		vm.WriteRegister(7, NONE)
	} else {
		prev_l := uint64(len(oldValue))
		if val_len == 0 {
			// delete
			if a.NumStorageItems > 0 {
				a.NumStorageItems--
			}
			a.StorageSize -= (32 + prev_l)
			l = uint64(prev_l) // this should not be NONE
			vm.WriteRegister(7, l)
			log.Debug(vm.logging, vm.Str("WRITE (as DELETE) NONE "), "numStorageItems", a.NumStorageItems, "StorageSize", a.StorageSize, "l", l, "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "k", k, "v", fmt.Sprintf("%x", v), "vlen", len(v))
		} else {
			// write
			a.StorageSize += val_len
			a.StorageSize -= prev_l
			l = prev_l
			vm.WriteRegister(7, l)
			log.Debug(vm.logging, vm.Str("WRITE OK"), "numStorageItems", a.NumStorageItems, "StorageSize", a.StorageSize, "l", l, "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "k", k, "v", fmt.Sprintf("%x", v), "vlen", len(v))
		}
	}
	log.Debug(vm.logging, vm.Str("WRITE storage"), "s", fmt.Sprintf("%d", a.ServiceIndex), "a_o", a.StorageSize, "a_i", a.NumStorageItems)
}

// Solicit preimage
func (vm *VM) hostSolicit() {
	xs, _ := vm.X.GetX_s()
	// Got l of X_s by setting s = 1, z = z(from RAM)
	o, _ := vm.ReadRegister(7)
	z, _ := vm.ReadRegister(8)                          // z: blob_len
	hBytes, err_h := vm.Ram.ReadRAMBytes(uint32(o), 32) // h: blobHash
	if err_h != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	account_lookuphash := common.BytesToHash(hBytes)

	ok, X_s_l := xs.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		// when preimagehash is not found, put it into solicit request - so we can ask other DAs
		xs.WriteLookup(account_lookuphash, uint32(z), []uint32{})
		xs.NumStorageItems += 2
		xs.StorageSize += 81 + uint64(z)
		log.Debug(vm.logging, vm.Str("SOLICIT OK"), "h", account_lookuphash, "z", z, "newvalue", []uint32{})
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	}

	if xs.Balance < xs.ComputeThreshold() {
		xs.WriteLookup(account_lookuphash, uint32(z), X_s_l)
		vm.WriteRegister(7, FULL)
		vm.HostResultCode = FULL
		log.Debug(vm.logging, vm.Str("SOLICIT FULL"), "h", account_lookuphash, "z", z)
		return
	}
	if len(X_s_l) == 2 { // [x, y] => [x, y, t]
		xs.WriteLookup(account_lookuphash, uint32(z), append(X_s_l, []uint32{vm.Timeslot}...))
		log.Debug(vm.logging, vm.Str("SOLICIT OK BBB"), "h", account_lookuphash, "z", z, "newvalue", append(X_s_l, []uint32{vm.Timeslot}...))
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	} else {
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, vm.Str("SOLICIT HUH"), "h", account_lookuphash, "z", z, "len(X_s_l)", len(X_s_l))
		return
	}
}

// Forget preimage
func (vm *VM) hostForget() {
	x_s, _ := vm.X.GetX_s()
	o, _ := vm.ReadRegister(7)
	z, _ := vm.ReadRegister(8)
	hBytes, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}

	account_lookuphash := common.Hash(hBytes)
	account_blobhash := common.Hash(hBytes)

	ok, X_s_l := x_s.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, vm.Str("FORGET HUH"), "h", account_lookuphash, "o", o)
		return
	}
	// 0 [] case is when we solicited but never got a preimage, so we can forget it
	// 2 [x,y] case is when we have a forgotten a preimage and have PASSED the preimage expiry period, so we can forget it
	if len(X_s_l) == 0 || (len(X_s_l) == 2) && X_s_l[1] < (vm.Timeslot-types.PreimageExpiryPeriod) { // D = types.PreimageExpiryPeriod
		x_s.WriteLookup(account_lookuphash, uint32(z), nil) // nil means delete the lookup
		x_s.WritePreimage(account_blobhash, []byte{})       // []byte{} means delete the preimage
		// storage accounting
		x_s.NumStorageItems -= 2
		x_s.StorageSize -= 81 + uint64(z)
		log.Debug(vm.logging, vm.Str("FORGET OK1"), "h", account_lookuphash, "z", z)
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	} else if len(X_s_l) == 1 {
		// preimage exists [x] => [x, y] where y is the current time, the time we are forgetting
		x_s.WriteLookup(account_lookuphash, uint32(z), append(X_s_l, []uint32{vm.Timeslot}...)) // [x, t]
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
		log.Debug(vm.logging, vm.Str("FORGET OK2"), "h", account_lookuphash, "z", z, "newvalue", append(X_s_l, []uint32{vm.Timeslot}...))
		return
	} else if len(X_s_l) == 3 && X_s_l[1] < (vm.Timeslot-types.PreimageExpiryPeriod) {
		// [x,y,w] => [w, t] where y is the current time, the time we are forgetting
		X_s_l = []uint32{X_s_l[2], vm.Timeslot}               // w = X_s_l[2], t = vm.Timeslot
		x_s.WriteLookup(account_lookuphash, uint32(z), X_s_l) // [w, t]
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
		log.Debug(vm.logging, vm.Str("FORGET OK3"), "h", account_lookuphash, "z", z, "newvalue", X_s_l)
		return
	}
	vm.WriteRegister(7, HUH)
	vm.HostResultCode = HUH
	log.Debug(vm.logging, vm.Str("FORGET HUH"), "h", account_lookuphash, "o", o)
	return

}

// HistoricalLookup determines whether the preimage of some hash h was available for lookup by some service account a at some timeslot t, and if so, provide its preimage
func (vm *VM) hostHistoricalLookup() {
	var a = &types.ServiceAccount{}
	delta := vm.Delta
	s := vm.Service_index
	omega_7, _ := vm.ReadRegister(7)
	h, _ := vm.ReadRegister(8)
	o, _ := vm.ReadRegister(9)
	omega_10, _ := vm.ReadRegister(10)
	omega_11, _ := vm.ReadRegister(11)

	if omega_7 == NONE {
		a = delta[s]
	} else {
		a = delta[uint32(omega_7)]
	}

	if a == nil {
		a, _, _ = vm.hostenv.GetService(uint32(omega_7))
	}

	hBytes, errCode := vm.Ram.ReadRAMBytes(uint32(h), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	// h := common.Hash(hBytes) not sure whether this is needed
	v := vm.hostenv.HistoricalLookup(a, vm.Timeslot, common.BytesToHash(hBytes))
	vLength := uint64(len(v))
	if vLength == 0 {
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		return
	} else {
		f := min(omega_10, vLength)
		l := min(omega_11, vLength-f)
		err := vm.Ram.WriteRAMBytes(uint32(o), v[f:l])
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.PVM_PANIC
			return
		}
		vm.WriteRegister(7, vLength)
	}
}

// Export segment host-call
func (vm *VM) hostExport() {
	p, _ := vm.ReadRegister(7) // a0 = 7
	z, _ := vm.ReadRegister(8) // a1 = 8

	z = min(z, types.SegmentSize)

	x, errCode := vm.Ram.ReadRAMBytes(uint32(p), uint32(z))
	if errCode != OK {
		vm.ResultCode = types.PVM_PANIC
		vm.terminated = true
		return
	}

	x = common.PadToMultipleOfN(x, types.SegmentSize)

	if vm.ExportSegmentIndex+uint32(len(vm.Exports)) >= W_X { // W_X
		vm.WriteRegister(7, FULL)
		vm.HostResultCode = FULL
		return
	} else {
		vm.WriteRegister(7, uint64(vm.ExportSegmentIndex)+uint64(len(vm.Exports)))
		log.Debug(vm.logging, vm.Str(fmt.Sprintf("%s EXPORT#%d OK", vm.ServiceMetadata, uint64(len(vm.Exports)))),
			"p", p, "z", z, "vm.ExportSegmentIndex", vm.ExportSegmentIndex,
			"segmenthash", fmt.Sprintf("%v", common.Blake2Hash(x)),
			"segment20", fmt.Sprintf("%x", x[0:20]),
			"len", fmt.Sprintf("%d", len(x)))
		// vm.ExportSegmentIndex += 1
		vm.Exports = append(vm.Exports, x)
		vm.HostResultCode = OK
		return
	}
}

func (vm *VM) hostManifest() {
	n, _ := vm.ReadRegister(7)
	mode, _ := vm.ReadRegister(8)
	s, _ := vm.ReadRegister(9)
	z, _ := vm.ReadRegister(10)
	page_id, _ := vm.ReadRegister(11)

	m, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}

	if m.U.Pages == nil {
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		return
	}

	switch mode {
	case 0:
		// reset manifest
		for _, page := range m.U.Pages {
			page.Access.Accessed = false
			page.Access.Dirty = false
		}
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	case 1:
		// get manifest
		var manifest []uint32
		for pageIndex, page := range m.U.Pages {
			m := uint32(0)
			if page.Access.Dirty {
				// m = uint32(pageIndex) | 0xC0000000
				m = uint32(pageIndex)
				manifest = append(manifest, m)
			}
		}
		sort.Slice(manifest, func(i, j int) bool { return manifest[i] < manifest[j] })
		manifestBytes := make([]byte, 4*len(manifest))
		for i, m := range manifest {
			binary.LittleEndian.PutUint32(manifestBytes[i*4:], m)
		}

		if z%4 != 0 {
			//  should be a multiple of 4 though!
			vm.WriteRegister(7, HUH)
			vm.HostResultCode = HUH
			return
		}
		if z < uint64(len(manifestBytes)) {
			manifestBytes = manifestBytes[:z]
		}
		// write manifestBytes bytes to vm
		errCode := vm.Ram.WriteRAMBytes(uint32(s), manifestBytes[:])
		if errCode != OK {
			vm.terminated = true
			vm.ResultCode = types.PVM_PANIC
			return
		}
		vm.WriteRegister(7, uint64(len(manifestBytes)))
		vm.HostResultCode = OK

	case 2:
		// check whether page_id is dirty
		page, ok := m.U.Pages[uint32(page_id)]
		if !ok {
			vm.WriteRegister(7, HUH)
			vm.HostResultCode = HUH
			return
		}

		if page.Access.Dirty {
			vm.WriteRegister(7, OK)
			vm.HostResultCode = OK
		} else {
			vm.WriteRegister(7, NONE)
			vm.HostResultCode = NONE
		}
		return
	case 3:
		// check whether page_id is all zero
		page, ok := m.U.Pages[uint32(page_id)]
		if !ok {
			vm.WriteRegister(7, HUH)
			vm.HostResultCode = HUH
			return
		}

		if page.Access.Inaccessible {
			vm.WriteRegister(7, HUH)
			vm.HostResultCode = HUH
			return
		} else {
			for _, page_byte := range page.Value {
				if page_byte != 0 {
					vm.WriteRegister(7, NONE)
					vm.HostResultCode = NONE
					return
				}
			}
			vm.WriteRegister(7, OK)
			vm.HostResultCode = OK
		}
		return
	default:
		// unknown mode
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		return
	}
	return
}

func (vm *VM) hostMachine() {
	po, _ := vm.ReadRegister(7)
	pz, _ := vm.ReadRegister(8)
	i, _ := vm.ReadRegister(9)
	p, errCode := vm.Ram.ReadRAMBytes(uint32(po), uint32(pz))
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}

	if vm.RefineM_map == nil {
		vm.RefineM_map = make(map[uint32]*RefineM)
	}
	min_n := uint32(16)
	for n := uint32(0); n < min_n; n++ {
		_, ok := vm.RefineM_map[n]
		if !ok {
			min_n = n
			break
		}
	}

	u := NewRAM()

	vm.RefineM_map[min_n] = &RefineM{
		P: p,
		U: u,
		I: i,
	}

	vm.WriteRegister(7, uint64(min_n))
}

func (vm *VM) hostPeek() {
	n, _ := vm.ReadRegister(7)
	o, _ := vm.ReadRegister(8)
	s, _ := vm.ReadRegister(9)
	z, _ := vm.ReadRegister(10)
	m_n, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}
	// read l bytes from m
	s_data, errCode := m_n.U.ReadRAMBytes(uint32(s), uint32(z))
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	// write l bytes to vm
	errCode = vm.Ram.WriteRAMBytes(uint32(o), s_data[:])
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

func (vm *VM) hostPoke() {
	n, _ := vm.ReadRegister(7) // machine
	s, _ := vm.ReadRegister(8) // source
	o, _ := vm.ReadRegister(9) // dest
	z, _ := vm.ReadRegister(10)
	m_n, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}
	// read data from original vm
	s_data, errCode := vm.Ram.ReadRAMBytes(uint32(s), uint32(z))
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.PVM_PANIC
		return
	}
	// write data to m_n
	errCode = m_n.U.WriteRAMBytes(uint32(o), s_data[:])
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

func (vm *VM) hostExpunge() {
	n, _ := vm.ReadRegister(7)
	m, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.HostResultCode = WHO
		vm.WriteRegister(7, WHO)
		return
	}

	i := m.I
	delete(vm.RefineM_map, uint32(n))

	vm.WriteRegister(7, i)
	vm.HostResultCode = OK
}

func (vm *VM) hostVoid() {
	n, _ := vm.ReadRegister(7)
	p, _ := vm.ReadRegister(8)
	c, _ := vm.ReadRegister(9)

	m, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}
	if p+c >= (1 << 32) {
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		return
	}

	for _, page := range m.U.Pages {
		if page.Access.Inaccessible {
			vm.WriteRegister(7, HUH)
			vm.HostResultCode = HUH
			return
		}
	}
	var access_mode AccessMode

	// set page access to writable to write [0,0,0,....] to the page
	access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: false}
	_ = m.U.SetPageAccess(uint32(p), uint32(c), access_mode)

	// write [0,0,0,....] to the page
	_ = m.U.WriteRAMBytes(uint32(p)*PageSize, make([]byte, 0))

	// set page access to inaccessible
	access_mode = AccessMode{Inaccessible: true, Writable: false, Readable: false}
	_ = m.U.SetPageAccess(uint32(p), uint32(c), access_mode)

	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

func (vm *VM) hostZero() {
	n, _ := vm.ReadRegister(7)
	p, _ := vm.ReadRegister(8)
	c, _ := vm.ReadRegister(9)
	if p < 16 || p+c > (1<<32)/Z_P {
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		return
	}
	m, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}
	access_mode := AccessMode{Inaccessible: false, Writable: true, Readable: false}
	_ = m.U.SetPageAccess(uint32(p), uint32(c), access_mode)

	_ = m.U.WriteRAMBytes(uint32(p)*PageSize, make([]byte, PageSize))

	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// func (vm *VM) hostSP1Groth16Verify()

func getLogLevelName(level uint64, core uint16, serviceName string) string {
	levelName := "UNKNOWN"
	switch level {
	case 0:
		levelName = "CRIT"
	case 1:
		levelName = "WARN"
	case 2:
		levelName = "INFO"
	case 3:
		levelName = "DEBUG"
	case 4:
		levelName = "TRACE"
	case 5:
		levelName = "TRACE"
	}
	coreStr := ""
	if core < 1024 {
		coreStr = fmt.Sprintf("@%d", core)
	}
	return fmt.Sprintf("%s%s#%s", levelName, coreStr, serviceName)
}

// JIP-1 https://hackmd.io/@polkadot/jip1
func (vm *VM) hostLog() {
	level, _ := vm.ReadRegister(7)
	message, _ := vm.ReadRegister(10)
	messagelen, _ := vm.ReadRegister(11)

	messageBytes, errCode := vm.Ram.ReadRAMBytes(uint32(message), uint32(messagelen))
	if errCode != OK {
		vm.HostResultCode = OOB
		return
	}
	levelName := getLogLevelName(level, vm.CoreIndex, string(vm.ServiceMetadata))

	vm.HostResultCode = OK
	switch level {
	case 0: // 0: User agent displays as fatal error
		log.Crit(vm.logging, levelName, "m", string(messageBytes))
		break
	case 1: // 1: User agent displays as warning
		log.Warn(vm.logging, levelName, "m", string(messageBytes))
		break
	case 2: // 2: User agent displays as important information
		log.Debug(vm.logging, levelName, "m", string(messageBytes))
		break
	case 3: // 3: User agent displays as helpful information
		log.Debug(vm.logging, levelName, "m", string(messageBytes))
		break
	case 4: // 4: User agent displays as pedantic information
		log.Trace(vm.logging, levelName, "m", string(messageBytes))
		break
	}
}

func (vm *VM) PutGasAndRegistersToMemory(input_address uint32, gas uint64, regs []uint64) {
	gasBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(gasBytes, gas)
	errCode := vm.Ram.WriteRAMBytes(input_address, gasBytes)
	if errCode != OK {
		vm.HostResultCode = OOB
		return
	}
	for i, reg := range regs {
		regBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(regBytes, reg)
		errCode = vm.Ram.WriteRAMBytes(input_address+8+uint32(i*8), regBytes)
		if errCode != OK {
			vm.HostResultCode = OOB
			return
		}
	}
	vm.HostResultCode = OK
}

// This should only be called OUTSIDE pvm package
func (vm *VM) SetPVMContext(l string) {
	vm.logging = l
}

func (vm *VM) GetGasAndRegistersFromMemory(input_address uint32) (gas uint64, regs []uint64, errCode uint64) {
	gasBytes, errCode := vm.Ram.ReadRAMBytes(input_address, 8)
	if errCode != OK {
		return 0, nil, errCode
	}
	gas = binary.LittleEndian.Uint64(gasBytes)
	regs = make([]uint64, 13)
	for i := 0; i < 13; i++ {
		regBytes, errCode := vm.Ram.ReadRAMBytes(input_address+8+uint32(i*8), 8)
		if errCode != OK {
			return 0, nil, errCode
		}
		regs[i] = binary.LittleEndian.Uint64(regBytes)
	}
	return gas, regs, OK
}
