package pvm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// Appendix B - Host function
// Host function indexes
const (
	GAS               = 0
	FETCH             = 1 // NEW
	LOOKUP            = 2
	READ              = 3
	WRITE             = 4
	INFO              = 5 // ADJ
	HISTORICAL_LOOKUP = 6
	EXPORT            = 7
	MACHINE           = 8
	PEEK              = 9
	POKE              = 10
	PAGES             = 11 // NEW
	INVOKE            = 12
	EXPUNGE           = 13
	BLESS             = 14
	ASSIGN            = 15
	DESIGNATE         = 16
	CHECKPOINT        = 17
	NEW               = 18
	UPGRADE           = 19
	TRANSFER          = 20
	EJECT             = 21
	QUERY             = 22
	SOLICIT           = 23
	FORGET            = 24
	YIELD             = 25
	PROVIDE           = 26

	MANIFEST = 64 // Not in 0.6.7

	LOG = 100
)

const maxUint64 = ^uint64(0)

const (
	Debug_Service_Storage = false
)

const (
	g = 10
)

const (
	AccountStorageConst = 34 //[Gratis]
	AccountLookupConst  = 81 //[Gratis]
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
	"Invoke":            {OK, OOB, WHO, OOB},
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
	"Pages":             PAGES,
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
	P []byte       `json:"P"`
	U RAMInterface `json:"U"`
	I uint64       `json:"I"`
}

// GP-0.5 B.5
type Refine_parameters struct {
	Gas                  uint64
	Ram                  *RAMInterface
	Register             []uint32
	Machine              map[uint32]*RefineM
	Export_segment       [][]byte
	Import_segement      [][]byte
	Export_segment_index uint32
	//service_index        uint32
	Delta map[uint32]uint32
	C_t   uint32
}

func (vm *VM) chargeGas(host_fn int) int64 {
	beforeGas := vm.Gas
	chargedGas := uint64(10)
	exp := fmt.Sprintf("HOSTFUNC %d", host_fn)

	switch host_fn {
	case TRANSFER:
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
	case PAGES:
		exp = "PAGES"
	case INVOKE:
		exp = "INVOKE"
	case EXPUNGE:
		exp = "EXPUNGE"
	case LOG:
		exp = "LOG"
		chargedGas = 0
	}

	log.Trace(vm.logging, exp, "reg", vm.Ram.ReadRegisters(), "gasCharged", chargedGas, "beforeGas", beforeGas, "afterGas", vm.Gas)
	return int64(chargedGas)
}

// InvokeHostCall handles host calls
// Returns true if the call results in a halt condition, otherwise false
func (vm *VM) InvokeHostCall(host_fn int) (bool, error) {

	if vm.Gas-g < 0 {
		vm.ResultCode = types.RESULT_OOG
		return true, fmt.Errorf("out of gas")
	}

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
		gas, _ := vm.Ram.ReadRegister(9)
		vm.Gas = vm.Gas - int64(gas)
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

	case PAGES:
		vm.hostPages()
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

	default:
		vm.Gas = vm.Gas + g
		return false, fmt.Errorf("unknown host call: %d", host_fn)
	}
}

// Information-on-Service (similar to hostRead)
func (vm *VM) hostInfo() {
	omega_7, _ := vm.Ram.ReadRegister(7)
	var fetch uint64
	if omega_7 == NONE {
		fetch = uint64(vm.Service_index)
	} else {
		fetch = omega_7
	}
	t, errCode := vm.getXUDS(fetch)
	if errCode != OK {
		vm.Ram.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		log.Info(vm.logging, "INFO NONE", "s", omega_7)
		return
	}
	bo, _ := vm.Ram.ReadRegister(8)
	f, _ := vm.Ram.ReadRegister(11)
	l, _ := vm.Ram.ReadRegister(12)

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
			vm.Ram.WriteRegister(7, NONE)
			vm.HostResultCode = NONE
			log.Info(vm.logging, "INFO NONE", "s", omega_7)
			return
		}
		//fmt.Printf("INFO %d %x\n", i, encoded)
		buf.Write(encoded)
	}

	val := buf.Bytes()
	lenval := uint64(len(val))
	f = min(f, lenval)
	l = min(l, lenval-f)

	errcode := vm.Ram.WriteRAMBytes(uint32(bo), val[f:f+l])
	if errcode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	log.Debug(vm.logging, "INFO OK", "s", fmt.Sprintf("%d", omega_7), "info", fmt.Sprintf("%v", elements))
	vm.Ram.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// Bless updates
func (vm *VM) hostBless() {
	m, _ := vm.Ram.ReadRegister(7)
	a, _ := vm.Ram.ReadRegister(8)
	v, _ := vm.Ram.ReadRegister(9)
	o, _ := vm.Ram.ReadRegister(10)
	n, _ := vm.Ram.ReadRegister(11)

	if m > (1<<32)-1 || v > (1<<32)-1 {
		vm.Ram.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		log.Debug(vm.logging, "BLESS WHO", "m", fmt.Sprintf("%d", m), "a", fmt.Sprintf("%d", a), "v", fmt.Sprintf("%d", v))
		return
	}

	bold_z := make(map[uint32]uint64)
	for i := 0; i < int(n); i++ {
		data, err := vm.Ram.ReadRAMBytes(uint32(o)+uint32(i)*12, 12)
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.RESULT_FAULT
			return
		}
		s := binary.LittleEndian.Uint32(data[0:4])
		g := binary.LittleEndian.Uint64(data[4:12])
		bold_z[s] = g
	}

	//TODO: Shawn to check
	bold_a := [types.TotalCores]uint32{}
	for i := 0; i < types.TotalCores; i++ {
		data, err := vm.Ram.ReadRAMBytes(uint32(a)+uint32(i)*4, 4)
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.RESULT_FAULT
			return
		}
		bold_a[i] = binary.LittleEndian.Uint32(data)
	}
	xContext := vm.X
	xs, _ := xContext.GetX_s() //vm.X.S.ServiceIndex
	privilegedService_m := vm.X.U.PrivilegedState.Kai_m
	if privilegedService_m != xs.ServiceIndex {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "BLESS HUH", "kai_m", privilegedService_m, "xs", xs.ServiceIndex)
		return
	}
	// Set (x'p)_m, (x'p)_a, (x'p)_v
	vm.X.U.PrivilegedState.Kai_m = uint32(m)
	vm.X.U.PrivilegedState.Kai_a = bold_a
	vm.X.U.PrivilegedState.Kai_v = uint32(v)
	vm.X.U.PrivilegedState.Kai_g = bold_z

	vm.Ram.WriteRegister(7, OK)
	log.Debug(vm.logging, "BLESS OK", "m", fmt.Sprintf("%d", m), "a", fmt.Sprintf("%d", a), "v", fmt.Sprintf("%d", v))
	vm.HostResultCode = OK
}

// Assign Core x_c[i]
func (vm *VM) hostAssign() {

	c, _ := vm.Ram.ReadRegister(7)
	o, _ := vm.Ram.ReadRegister(8)
	a, _ := vm.Ram.ReadRegister(9)

	if c >= types.TotalCores {
		vm.Ram.WriteRegister(7, CORE)
		vm.HostResultCode = CORE
		log.Debug(vm.logging, "ASSIGN CORE", "c", c)
		return
	}
	q, errcode := vm.Ram.ReadRAMBytes(uint32(o), 32*types.MaxAuthorizationQueueItems)
	if errcode != OK {
		vm.Ram.WriteRegister(7, OOB)
		vm.ResultCode = types.RESULT_FAULT
		//vm.WriteRegister(7, OOB)               // OOB seems wrong
		//vm.HostResultCode = types.RESULT_FAULT // Check: should this be types.RESULT_PANIC?
		return
	}
	bold_q := make([]common.Hash, types.MaxAuthorizationQueueItems)
	for i := 0; i < types.MaxAuthorizationQueueItems; i++ {
		bold_q[i] = common.BytesToHash(q[i*32 : (i+1)*32])
	}
	xContext := vm.X
	xs, _ := xContext.GetX_s()
	privilegedService_a := vm.X.U.PrivilegedState.Kai_a[c]
	if privilegedService_a != xs.ServiceIndex {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "ASSIGN HUH", "c", c, "kai_a[c]", privilegedService_a, "xs", xs.ServiceIndex)
		return
	}
	copy(vm.X.U.QueueWorkReport[c][:], bold_q[:])
	vm.X.U.PrivilegedState.Kai_a[c] = uint32(a)
	log.Debug(vm.logging, "ASSIGN OK", "c", c, "kai_a[c]", privilegedService_a, "xs", xs.ServiceIndex)
	vm.HostResultCode = OK
}

// Designate validators
func (vm *VM) hostDesignate() {
	o, _ := vm.Ram.ReadRegister(7)
	v, errCode := vm.Ram.ReadRAMBytes(uint32(o), 176*V)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	xContext := vm.X
	xs, _ := xContext.GetX_s()
	privilegedService_v := vm.X.U.PrivilegedState.Kai_v
	if privilegedService_v != xs.ServiceIndex {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "DESIGNATE HUH", "kai_v", privilegedService_v, "xs", xs.ServiceIndex)
		return
	}
	v_bold := make([]types.Validator, V)
	for i := 0; i < types.TotalValidators; i++ {
		newv := types.Validator{}
		copy(newv.Bandersnatch[:], v[0:32])
		copy(newv.Ed25519[:], v[64:92])
		copy(newv.Bls[:], v[92:128])
		copy(newv.Metadata[:], v[128:])
		v_bold[i] = newv
	}
	vm.X.U.UpcomingValidators = v_bold
	log.Debug(vm.logging, "DESIGNATE OK")
	vm.HostResultCode = OK
}

// Checkpoint gets Gas-remaining
func (vm *VM) hostCheckpoint() {
	vm.Y = vm.X.Clone()
	vm.Y.U.Checkpoint()
	vm.Ram.WriteRegister(7, uint64(vm.Gas)) // CHECK
	log.Debug(vm.logging, "CHECKPOINT", "g", fmt.Sprintf("%d", vm.Gas))
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
	o, _ := vm.Ram.ReadRegister(7)
	c, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	l, _ := vm.Ram.ReadRegister(8)
	g, _ := vm.Ram.ReadRegister(9)
	m, _ := vm.Ram.ReadRegister(10)
	f, _ := vm.Ram.ReadRegister(11)

	x_s_t := xs.ComputeThreshold()
	if xs.Balance < x_s_t {
		vm.Ram.WriteRegister(7, CASH)
		vm.HostResultCode = CASH //balance insufficient
		log.Info(vm.logging, "hostNew: NEW CASH xs.Balance < x_s_t", "xs.Balance", xs.Balance, "x_s_t", x_s_t, "x_s_index", xs.ServiceIndex)
		return
	}

	privilegedService_m := vm.X.U.PrivilegedState.Kai_m
	if privilegedService_m != xs.ServiceIndex && f != 0 {
		// only kai_m can bestow gratis
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "hostNew: HUH", "privilegedService", privilegedService_m, "xs", xs.ServiceIndex)
		return
	}

	// xs has enough balance to fund service creation of a AND covering its own threshold

	xi := xContext.I
	// simulate a with c, g, m

	// [Michael Gratis] a_r:t; a_f,a_a:0; a_p:x_s
	a := &types.ServiceAccount{
		ServiceIndex:       xi,
		Mutable:            false,
		Dirty:              false,
		CodeHash:           common.BytesToHash(c),
		Balance:            0,
		GasLimitG:          uint64(g),
		GasLimitM:          uint64(m),
		StorageSize:        uint64(AccountLookupConst + l), //a_l =  ∑ 81+z per (h,z) + ∑ 34 + |y| + |x|; Initialized for first a_l
		GratisOffset:       uint64(f),                      //a_f = 0
		NumStorageItems:    2,                              //a_s = 2⋅∣al∣+∣as∣; Initialized for first a_l
		CreateTime:         vm.Timeslot,                    //a_r = t // check pre vs post?
		RecentAccumulation: 0,                              //a_a = 0
		ParentService:      xs.ServiceIndex,                //a_p = x_s
		Storage:            make(map[string]*types.StorageObject),
		Lookup:             make(map[string]*types.LookupObject),
		Preimage:           make(map[string]*types.PreimageObject),
		Checkpointed:       false, // this is updated to true upon Checkpoint
		NewAccount:         true,  // with this flag, if an account is Dirty OR Checkpointed && NewAccount then it is written
	}
	a.ALLOW_MUTABLE()
	a.Balance = a.ComputeThreshold()
	xs.DecBalance(a.Balance) // (x's)b <- (xs)b - at
	xContext.I = new_check(uint32(256)+uint32(xi-256+42)%(uint32(4294966784)), xContext.U.D)
	a.WriteLookup(common.BytesToHash(c), uint32(l), []uint32{})

	xContext.U.D[xi] = a // this new account is included but only is written if (a) non-exceptional (b) exceptional and checkpointed
	vm.Ram.WriteRegister(7, uint64(xi))
	vm.HostResultCode = OK
	log.Debug(vm.logging, "NEW OK", "SERVICE", fmt.Sprintf("%d", xi), "code_hash_ptr", fmt.Sprintf("%x", o), "code_hash_ptr", fmt.Sprintf("%x", c), "code_len", l, "min_item_gas", g, "min_memo_gas", m)
}

// Upgrade service
func (vm *VM) hostUpgrade() {
	xContext := vm.X
	xs, _ := xContext.GetX_s()
	o, _ := vm.Ram.ReadRegister(7)
	g, _ := vm.Ram.ReadRegister(8)
	m, _ := vm.Ram.ReadRegister(9)

	c, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}

	xs.Dirty = true
	xs.CodeHash = common.BytesToHash(c)
	xs.GasLimitG = g
	xs.GasLimitM = m
	vm.Ram.WriteRegister(7, OK)
	// xContext.D[s] = xs // not sure if this is needed
	log.Debug(vm.logging, "UPGRADE OK", "code_hash", fmt.Sprintf("%x", o), "code_hash_ptr", fmt.Sprintf("%x", c), "min_item_gas", g, "min_memo_gas", m)
	vm.HostResultCode = OK
}

// Transfer host call
func (vm *VM) hostTransfer() {
	d, _ := vm.Ram.ReadRegister(7)
	a, _ := vm.Ram.ReadRegister(8)
	g, _ := vm.Ram.ReadRegister(9)
	o, _ := vm.Ram.ReadRegister(10)

	xs, _ := vm.X.GetX_s()

	D := vm.X.U.D

	var receiver *types.ServiceAccount
	var founded bool
	receiver, founded = D[uint32(d)]
	if !founded {
		receiver, founded, _ = vm.hostenv.GetService(uint32(d))
		if !founded {
			vm.Ram.WriteRegister(7, WHO)
			vm.HostResultCode = WHO
			log.Debug(vm.logging, "TRANSFER WHO", "d", d)
			return
		}
		vm.X.U.D[uint32(d)] = receiver
	}

	m, errCode := vm.Ram.ReadRAMBytes(uint32(o), M)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	t := types.DeferredTransfer{Amount: a, GasLimit: g, SenderIndex: vm.X.S, ReceiverIndex: uint32(d)} // CHECK

	if g < receiver.GasLimitM {
		vm.Ram.WriteRegister(7, LOW)
		vm.HostResultCode = LOW
		log.Debug(vm.logging, "TRANSFER LOW", "g", g, "GasLimitM", receiver.GasLimitM)
		return
	}

	if xs.Balance < xs.ComputeThreshold() {
		vm.Ram.WriteRegister(7, CASH)
		vm.HostResultCode = CASH
		log.Debug(vm.logging, "TRANSFER CASH", "xs.Balance", xs.Balance, "xs_t", xs.ComputeThreshold())
		return
	}

	xs.DecBalance(a)
	copy(t.Memo[:], m[:])
	vm.X.T = append(vm.X.T, t)
	log.Debug(vm.logging, "TRANSFER OK", "sender", fmt.Sprintf("%d", t.SenderIndex), "receiver", fmt.Sprintf("%d", d), "amount", fmt.Sprintf("%d", a), "gaslimit", g)
	vm.Ram.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// Gas Service
func (vm *VM) hostGas() {
	vm.Ram.WriteRegister(7, uint64(vm.Gas-10)) // its gas remaining AFTER the host call
	//vm.Ram.WriteRegister(7, uint64(1234567)) // TEMPORARY
	vm.HostResultCode = OK
}

func (vm *VM) hostQuery() {
	o, _ := vm.Ram.ReadRegister(7)
	z, _ := vm.Ram.ReadRegister(8)
	h, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	a, _ := vm.X.GetX_s()
	account_lookuphash := common.BytesToHash(h)
	ok, anchor_timeslot := a.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		vm.Ram.WriteRegister(7, NONE)
		vm.Ram.WriteRegister(8, 0)
		vm.HostResultCode = NONE
		log.Debug(vm.logging, "QUERY NONE", "h", account_lookuphash, "z", z)
		return
	}
	switch len(anchor_timeslot) {
	case 0:
		vm.Ram.WriteRegister(7, 0)
		vm.Ram.WriteRegister(8, 0)
	case 1:
		x := anchor_timeslot[0]
		vm.Ram.WriteRegister(7, 1+(1<<32)*uint64(x))
		vm.Ram.WriteRegister(8, 0)
		log.Debug(vm.logging, "QUERY 1", "x", x)
	case 2:
		x := anchor_timeslot[0]
		y := anchor_timeslot[1]
		vm.Ram.WriteRegister(7, 2+(1<<32)*uint64(x))
		vm.Ram.WriteRegister(8, uint64(y))
		log.Debug(vm.logging, "QUERY 2", "x", x, "y", y)
	case 3:
		x := anchor_timeslot[0]
		y := anchor_timeslot[1]
		z := anchor_timeslot[2]
		log.Debug(vm.logging, "QUERY 3", "x", x, "y", y, "z", z)
		vm.Ram.WriteRegister(7, 3+(1<<32)*uint64(x))
		vm.Ram.WriteRegister(8, uint64(y)+(1<<32)*uint64(z))
	}
	w7, _ := vm.Ram.ReadRegister(7)
	w8, _ := vm.Ram.ReadRegister(8)
	log.Debug(vm.logging, "QUERY OK", "h", account_lookuphash, "z", z, "w7", w7, "w8", w8, "len(anchor_timeslot)", len(anchor_timeslot))
	vm.HostResultCode = OK
}

// https://graypaper.fluffylabs.dev/#/7e6ff6a/323800323800?v=0.6.7
func (vm *VM) hostFetch() {
	o, _ := vm.Ram.ReadRegister(7)
	omega_8, _ := vm.Ram.ReadRegister(8)
	omega_9, _ := vm.Ram.ReadRegister(9)
	datatype, _ := vm.Ram.ReadRegister(10)
	omega_11, _ := vm.Ram.ReadRegister(11)
	omega_12, _ := vm.Ram.ReadRegister(12)
	// log.Info(vm.logging, "FETCH", "datatype", datatype, "omega_7", o, "omega_8", omega_8, "omega_9", omega_9, "omega_11", omega_11, "omega_12", omega_12, "vm.Extrinsics", fmt.Sprintf("%x", vm.Extrinsics), "wp", vm.WorkPackage)
	var v_Bytes []byte
	switch datatype {
	case 0:
		v_Bytes, _ = types.ParameterBytes()
	case 1:
		v_Bytes = vm.N.Bytes()
	case 2:
		v_Bytes = vm.Authorization
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
			}
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
			log.Trace(log.DA, fmt.Sprintf("%s Fetch segment hash:", vm.ServiceMetadata), "I", fmt.Sprintf("%v", common.Blake2Hash(v_Bytes)))
			log.Trace(log.DA, fmt.Sprintf("%s Fetch segment byte:", vm.ServiceMetadata), "I", fmt.Sprintf("%x", v_Bytes))
			log.Trace(log.DA, fmt.Sprintf("%s Fetch segment  len:", vm.ServiceMetadata), "I", fmt.Sprintf("%d", len(v_Bytes)))
			log.Trace(log.DA, fmt.Sprintf("%s Fetch segment  idx:", vm.ServiceMetadata), "I", fmt.Sprintf("%d", omega_12))
		}

	case 6:
		// get imported segment by work item index
		if omega_11 < uint64(len(vm.Imports[vm.WorkItemIndex])) {
			v_Bytes = append([]byte{}, vm.Imports[vm.WorkItemIndex][omega_11][:]...)
			log.Trace(vm.logging, fmt.Sprintf("[N%d] %s Fetch imported segment", vm.hostenv.GetID(), vm.ServiceMetadata),
				"h", fmt.Sprintf("%v", common.Blake2Hash(v_Bytes)),
				"bytes", v_Bytes[0:20],
				"l", len(v_Bytes),
				"workItemIndex", vm.WorkItemIndex,
				"w11", omega_11)
		} else {
			// fmt.Printf("FETCH 6 FAIL omega_11 %d vs len(vm.Imports[vm.WorkItemIndex=%d])=%d\n", omega_11, vm.WorkItemIndex, len(vm.Imports[vm.WorkItemIndex]))
		}
	case 7: // encode work package
		v_Bytes, _ = types.Encode(vm.WorkPackage)
	case 8: // p_u + | p_p
		v_Bytes = append(vm.WorkPackage.AuthorizationCodeHash.Bytes(), vm.WorkPackage.ParameterizationBlob...)
		log.Trace(vm.logging, "FETCH p_u + | p_p", "p_u", vm.WorkPackage.AuthorizationCodeHash, "p_p", vm.WorkPackage.ParameterizationBlob)
	case 9: // p_j
		v_Bytes = vm.WorkPackage.Authorization
	case 10: // p_X (refine context)
		v_Bytes, _ = types.Encode(vm.WorkPackage.RefineContext)
	case 11: // all work items
		v_Bytes = make([]byte, 0)
		for _, w := range vm.WorkPackage.WorkItems {
			s_bytes, _ := w.EncodeS()
			v_Bytes = append(v_Bytes, s_bytes...) // TODO: add discriminator in front
		}
	case 12: // S(w) for specific work item w_11
		if omega_11 < uint64(len(vm.WorkPackage.WorkItems)) {
			w := vm.WorkPackage.WorkItems[omega_11]
			v_Bytes, _ = w.EncodeS()
		}
	case 13: // p_w[w_11]_y
		if omega_11 < uint64(len(vm.WorkPackage.WorkItems)) {
			w := vm.WorkPackage.WorkItems[omega_11]
			v_Bytes = w.Payload
			log.Trace(vm.logging, "FETCH p_w[w_11]_y", "w_11", omega_11, "payload", fmt.Sprintf("%x", v_Bytes), "len", len(v_Bytes))
		}
	case 14: // E(|o) all accumulation operands
		if vm.AccumulateOperandElements != nil {
			v_Bytes, _ = types.Encode(vm.AccumulateOperandElements)
		}
	case 15: // E(o[w_11])
		if vm.AccumulateOperandElements != nil && omega_11 < uint64(len(vm.AccumulateOperandElements)) {
			v_Bytes, _ = types.Encode(vm.AccumulateOperandElements[omega_11])
			log.Trace(vm.logging, "FETCH E(o[w_11])", "w_11", omega_11, "v_Bytes", fmt.Sprintf("%x", v_Bytes), "len", len(v_Bytes))
		}
	case 16: // E(|t) all transfers
		if vm.Transfers != nil {
			v_Bytes, _ = types.Encode(vm.Transfers)
		}
	case 17: // E(t[w_11])
		if vm.Transfers != nil && omega_11 < uint64(len(vm.Transfers)) {
			v_Bytes, _ = types.Encode(vm.Transfers[omega_11])
		}
	}
	if v_Bytes == nil {
		vm.Ram.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		return
	}

	f := min(uint64(len(v_Bytes)), omega_8)   // offset
	l := min(uint64(len(v_Bytes))-f, omega_9) // max length

	errCode := vm.Ram.WriteRAMBytes(uint32(o), v_Bytes[f:f+l])
	if errCode != OK {
		log.Info(vm.logging, "FETCH FAIL", "o", o, "v_Bytes", fmt.Sprintf("%x", v_Bytes), "l", l, "f", f, "f+l", f+l, "v_Bytes[f..f+l]", fmt.Sprintf("%x", v_Bytes[f:f+l]))
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	// log.Info(vm.logging, "FETCH SUCC", "o", o, "v_Bytes", fmt.Sprintf("%x", v_Bytes), "l", l, "f", f, "f+l", f+l, "v_Bytes[f..f+l]", fmt.Sprintf("%x", v_Bytes[f:f+l]))

	vm.Ram.WriteRegister(7, uint64(l))
}

func (vm *VM) hostYield() {
	o, _ := vm.Ram.ReadRegister(7)
	h, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	y := common.BytesToHash(h)
	vm.X.Y = y
	vm.Ram.WriteRegister(7, OK)
	log.Debug(vm.logging, "YIELD OK", "h", y)
	vm.HostResultCode = OK
}

func (vm *VM) hostProvide() {
	omega_7, _ := vm.Ram.ReadRegister(7)
	o, _ := vm.Ram.ReadRegister(8)
	z, _ := vm.Ram.ReadRegister(9)
	if omega_7 == NONE {
		omega_7 = uint64(vm.Service_index)
	}

	var a *types.ServiceAccount
	a, _ = vm.getXUDS(omega_7)

	if a == nil {
		vm.Ram.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		log.Debug(vm.logging, "PROVIDE WHO", "omega_7", omega_7)
		return
	}

	i, errCode := vm.Ram.ReadRAMBytes(uint32(o), uint32(z))
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}

	h := common.Blake2Hash(i)
	ok, X_s_l := a.ReadLookup(h, uint32(z), vm.hostenv)
	if !ok && len(X_s_l) > 0 {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "PROVIDE HUH", "omega_7", omega_7, "h", h, "z", z)
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
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "PROVIDE HUH", "omega_7", omega_7, "h", h, "z", z)
		return
	}

	vm.X.P = append(vm.X.P, types.P{
		ServiceIndex: a.ServiceIndex,
		P_data:       i,
	})

	vm.Ram.WriteRegister(7, OK)
	log.Debug(vm.logging, "PROVIDE OK", "omega_7", omega_7, "h", h, "z", z)
	vm.HostResultCode = OK
}

func (vm *VM) hostEject() {
	d, _ := vm.Ram.ReadRegister(7)
	o, _ := vm.Ram.ReadRegister(8)
	h, err := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if err != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}

	bold_d, ok := vm.X.U.D[uint32(d)]
	if d == uint64(vm.X.S) || !ok || bold_d.CodeHash != common.Hash(types.E_l(uint64(vm.X.S), 32)) {
		vm.Ram.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		log.Debug(vm.logging, "EJECT WHO", "d", fmt.Sprintf("%d", d))
		return
	}
	l := max(AccountLookupConst, bold_d.StorageSize) - AccountLookupConst

	ok, D_lookup := bold_d.ReadLookup(common.BytesToHash(h), uint32(l), vm.hostenv)
	if !ok || bold_d.NumStorageItems != 2 {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "EJECT HUH", "d", fmt.Sprintf("%d", d), "h", h, "l", l)
		return
	}

	s, _ := vm.getXUDS(uint64(vm.X.S))
	s = s.Clone()
	s.Balance += bold_d.Balance

	if len(D_lookup) == 2 && D_lookup[1] < vm.Timeslot-uint32(types.PreimageExpiryPeriod) {
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		delete(vm.X.U.D, uint32(d))
		vm.X.U.D[vm.X.S] = s
		log.Debug(vm.logging, "EJECT OK", "d", fmt.Sprintf("%d", d))
		return
	}

	vm.Ram.WriteRegister(7, HUH)
	vm.HostResultCode = HUH
}

// Invoke
func (vm *VM) hostInvoke() {
	n, _ := vm.Ram.ReadRegister(7)
	o, _ := vm.Ram.ReadRegister(8)

	gasBytes, errCodeGas := vm.Ram.ReadRAMBytes(uint32(o), 8)
	if errCodeGas != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}

	g := types.DecodeE_l(gasBytes)

	m_n_reg := make([]uint64, 13)
	for i := 1; i < 14; i++ {
		reg_bytes, errCodeReg := vm.Ram.ReadRAMBytes(uint32(o)+8*uint32(i), 8)
		if errCodeReg != OK {
			vm.terminated = true
			vm.ResultCode = types.RESULT_PANIC
			return
		}
		m_n_reg[i-1] = types.DecodeE_l(reg_bytes)
	}
	m_n, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.Ram.WriteRegister(7, WHO)
		log.Debug(vm.logging, "INVOKE WHO", "n", n)
		vm.HostResultCode = WHO
		return
	}

	program := DecodeProgram_pure_pvm_blob(m_n.P)
	new_machine := &VM{
		JSize:   program.JSize,
		Z:       program.Z,
		J:       program.J,
		code:    program.Code,
		bitmask: []byte(program.K),

		pc:  m_n.I,
		Gas: int64(g),
		// TODO: ***** register: m_n_reg,
		Ram: nil, // m_n.U,
	}

	new_machine.logging = vm.logging
	new_machine.Execute(int(new_machine.pc), true, nil)

	m_n.I = new_machine.pc
	m_n.U = new_machine.Ram
	vm.RefineM_map[uint32(n)] = m_n

	// Result after execution
	gasBytes = types.E_l(uint64(new_machine.Gas), 8)
	errCodeGas = vm.Ram.WriteRAMBytes(uint32(o), gasBytes)
	if errCodeGas != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	// TODO: use register ram model
	/*
		for i := 1; i < 14; i++ {
			reg_bytes := types.E_l(new_machine.register[i-1], 8)
			errCode := vm.Ram.WriteRAMBytes(uint32(o)+8*uint32(i), reg_bytes)
			if errCode != OK {
				vm.terminated = true
				vm.ResultCode = types.RESULT_FAULT
				return
			}
		}
	*/
	if new_machine.ResultCode == types.RESULT_HOST {
		vm.Ram.WriteRegister(7, types.RESULT_HOST)
		vm.Ram.WriteRegister(8, uint64(new_machine.host_func_id))
		m_n.I = new_machine.pc + 1
		return
	}

	if new_machine.ResultCode == types.RESULT_FAULT {
		vm.Ram.WriteRegister(7, types.RESULT_FAULT)
		vm.Ram.WriteRegister(8, uint64(new_machine.Fault_address))
		return
	}

	if new_machine.ResultCode == types.RESULT_OOG {
		vm.Ram.WriteRegister(7, types.RESULT_OOG)
		return
	}

	if new_machine.ResultCode == types.RESULT_OK {
		vm.Ram.WriteRegister(7, types.RESULT_OK)
		return
	}

}

// Lookup preimage
func (vm *VM) hostLookup() {
	omega_7, _ := vm.Ram.ReadRegister(7)

	var a *types.ServiceAccount
	if omega_7 == uint64(vm.Service_index) || omega_7 == maxUint64 {
		a = vm.ServiceAccount
	}
	if a == nil {
		a, _ = vm.getXUDS(omega_7)
	}

	h, _ := vm.Ram.ReadRegister(8)
	o, _ := vm.Ram.ReadRegister(9)
	f, _ := vm.Ram.ReadRegister(10)
	l, _ := vm.Ram.ReadRegister(11)
	k_bytes, err_k := vm.Ram.ReadRAMBytes(uint32(h), 32)
	if err_k != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}

	var account_blobhash common.Hash

	var v []byte
	var ok bool

	account_blobhash = common.Hash(k_bytes)
	ok, v = a.ReadPreimage(account_blobhash, vm.hostenv)
	if !ok {
		vm.Ram.WriteRegister(7, NONE)
		log.Debug(vm.logging, "LOOKUP NONE", "s", fmt.Sprintf("%d", a.ServiceIndex), "h", account_blobhash)
		vm.HostResultCode = NONE
		return
	}
	f = min(f, uint64(len(v)))
	l = min(l, uint64(len(v))-f)

	err := vm.Ram.WriteRAMBytes(uint32(o), v[:l])
	if err != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}

	if len(v) != 0 {
		vm.Ram.WriteRegister(7, l)
	}
	log.Debug(vm.logging, "LOOKUP OK", "s", fmt.Sprintf("%d", a.ServiceIndex), "h", h, "len(v)", len(v))
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
	omega_7, _ := vm.Ram.ReadRegister(7)
	s_star := omega_7
	var a *types.ServiceAccount
	var errCode uint64
	if omega_7 == maxUint64 {
		s_star = uint64(vm.Service_index)
	}
	if s_star == uint64(vm.Service_index) {
		a = vm.ServiceAccount
	} else {
		a, errCode = vm.getXUDS(s_star)
		if errCode != OK {
			vm.Ram.WriteRegister(7, NONE)
			vm.HostResultCode = NONE
			return
		}
	}
	ko, _ := vm.Ram.ReadRegister(8)
	kz, _ := vm.Ram.ReadRegister(9)
	bo, _ := vm.Ram.ReadRegister(10)
	f, _ := vm.Ram.ReadRegister(11)
	l, _ := vm.Ram.ReadRegister(12)
	mu_k, err_k := vm.Ram.ReadRAMBytes(uint32(ko), uint32(kz)) // this is the raw key.
	if err_k != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	// [0.6.7] No more hashing of mu_k
	//k := common.ServiceStorageKey(a.ServiceIndex, mu_k) // this does E_4(s) ... mu_4
	ok, val := a.ReadStorage(mu_k, vm.hostenv)

	if !ok { // || true
		vm.Ram.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		log.Info(vm.logging, "READ NONE", "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "kLen", len(mu_k), "ok", ok, "val", fmt.Sprintf("%x", val), "len(val)", len(val))
		return
	}
	log.Info(vm.logging, "READ OK", "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "kLen", len(mu_k), "ok", ok, "val", fmt.Sprintf("%x", val), "len(val)", len(val))
	lenval := uint64(len(val))
	f = min(f, lenval)
	l = min(l, lenval-f)
	// TODO: check for OOB case again using o, f + l
	vm.Ram.WriteRAMBytes(uint32(bo), val[f:f+l])
	vm.Ram.WriteRegister(7, lenval)
}

// Write Storage a_s(x,y)
func (vm *VM) hostWrite() {
	var a *types.ServiceAccount
	a = vm.ServiceAccount
	if a == nil {
		a, _ = vm.getXUDS(uint64(vm.Service_index))
	}
	ko, _ := vm.Ram.ReadRegister(7)
	kz, _ := vm.Ram.ReadRegister(8)
	vo, _ := vm.Ram.ReadRegister(9)
	vz, _ := vm.Ram.ReadRegister(10)
	mu_k, err_k := vm.Ram.ReadRAMBytes(uint32(ko), uint32(kz))
	if err_k != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		log.Error(vm.logging, "WRITE RAM", "err", err_k)
		return
	}
	// [0.6.7] No more hashing of mu_k
	//k := common.ServiceStorageKey(a.ServiceIndex, mu_k) // this does E_4(s) ... mu_4
	a_t := a.ComputeThreshold()
	if a_t > a.Balance {
		vm.Ram.WriteRegister(7, FULL)
		vm.HostResultCode = FULL
		log.Error(vm.logging, "WRITE FULL", "a_t", a_t, "balance", a.Balance)
		return
	}

	l := uint64(NONE)
	exists, oldValue := a.ReadStorage(mu_k, vm.hostenv)
	v := []byte{}
	err := uint64(0)
	if vz > 0 {
		v, err = vm.Ram.ReadRAMBytes(uint32(vo), uint32(vz))
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.RESULT_FAULT
			return
		}
		//l = uint64(len(v))
	}
	a.WriteStorage(a.ServiceIndex, mu_k, v)
	vm.ResultCode = OK
	vm.HostResultCode = OK
	key_len := uint64(len(mu_k)) // x in a_s(x,y) |y|
	val_len := uint64(len(v))    // y in a_s(x,y) |x|
	if !exists {
		if val_len > 0 {
			a.NumStorageItems++
			a.StorageSize += (AccountStorageConst + val_len + key_len) // [Gratis] Add ∑ 34 + |y| + |x|
			log.Debug(vm.logging, "WRITE NONE", "numStorageItems", a.NumStorageItems, "StorageSize", a.StorageSize, "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "kLen", len(mu_k), "v", fmt.Sprintf("%x", v), "kLen", key_len, "vlen", len(v))
		}
		vm.Ram.WriteRegister(7, NONE)
	} else {
		prev_l := uint64(len(oldValue))
		if val_len == 0 {
			// delete
			if a.NumStorageItems > 0 {
				a.NumStorageItems--
			}
			a.StorageSize -= (AccountStorageConst + prev_l + key_len) // [Gratis] Sub ∑ 34 + |y| + |x|
			l = uint64(prev_l)                                        // this should not be NONE
			vm.Ram.WriteRegister(7, l)
			//log.Debug(vm.logging, "WRITE (as DELETE) NONE ", "numStorageItems", a.NumStorageItems, "StorageSize", a.StorageSize, "l", l, "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "kLen", len(mu_k), "v", fmt.Sprintf("%x", v), "vlen", len(v))
		} else {
			// write via update;  |x| (val_len), a_i (storageItem) unchanged
			a.StorageSize += val_len
			a.StorageSize -= prev_l
			l = prev_l
			vm.Ram.WriteRegister(7, l)
			//log.Debug(vm.logging, "WRITE OK", "numStorageItems", a.NumStorageItems, "StorageSize", a.StorageSize, "l", l, "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "kLen", len(mu_k), "v", fmt.Sprintf("%x", v), "vlen", len(v), "oldValue", fmt.Sprintf("%x", oldValue))
		}
	}
	log.Trace(vm.logging, "WRITE storage", "s", fmt.Sprintf("%d", a.ServiceIndex), "a_o", a.StorageSize, "a_i", a.NumStorageItems)
}

// Solicit preimage a_l(h,z)
func (vm *VM) hostSolicit() {
	xs, _ := vm.X.GetX_s()
	// Got l of X_s by setting s = 1, z = z(from RAM)
	o, _ := vm.Ram.ReadRegister(7)
	z, _ := vm.Ram.ReadRegister(8)                      // z: blob_len
	hBytes, err_h := vm.Ram.ReadRAMBytes(uint32(o), 32) // h: blobHash
	if err_h != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	account_lookuphash := common.BytesToHash(hBytes)

	ok, X_s_l := xs.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		// when preimagehash is not found, put it into solicit request - so we can ask other DAs
		xs.WriteLookup(account_lookuphash, uint32(z), []uint32{})
		xs.NumStorageItems += 2
		xs.StorageSize += AccountLookupConst + uint64(z)
		log.Debug(vm.logging, "SOLICIT OK", "h", account_lookuphash, "z", z, "newvalue", []uint32{})
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	}

	if xs.Balance < xs.ComputeThreshold() {
		xs.WriteLookup(account_lookuphash, uint32(z), X_s_l)
		vm.Ram.WriteRegister(7, FULL)
		vm.HostResultCode = FULL
		log.Debug(vm.logging, "SOLICIT FULL", "h", account_lookuphash, "z", z)
		return
	}
	if len(X_s_l) == 2 { // [x, y] => [x, y, t]
		xs.WriteLookup(account_lookuphash, uint32(z), append(X_s_l, []uint32{vm.Timeslot}...))
		log.Debug(vm.logging, "SOLICIT OK BBB", "h", account_lookuphash, "z", z, "newvalue", append(X_s_l, []uint32{vm.Timeslot}...))
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	} else {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "SOLICIT HUH", "h", account_lookuphash, "z", z, "len(X_s_l)", len(X_s_l))
		return
	}
}

// Forget preimage a_l(h,z)
func (vm *VM) hostForget() {
	x_s, _ := vm.X.GetX_s()
	o, _ := vm.Ram.ReadRegister(7)
	z, _ := vm.Ram.ReadRegister(8)
	hBytes, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}

	account_lookuphash := common.Hash(hBytes)
	account_blobhash := common.Hash(hBytes)

	ok, X_s_l := x_s.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "FORGET HUH", "h", account_lookuphash, "o", o)
		return
	}
	// 0 [] case is when we solicited but never got a preimage, so we can forget it
	// 2 [x,y] case is when we have a forgotten a preimage and have PASSED the preimage expiry period, so we can forget it
	if len(X_s_l) == 0 || (len(X_s_l) == 2) && X_s_l[1] < (vm.Timeslot-types.PreimageExpiryPeriod) { // D = types.PreimageExpiryPeriod
		x_s.WriteLookup(account_lookuphash, uint32(z), nil) // nil means delete the lookup
		x_s.WritePreimage(account_blobhash, []byte{})       // []byte{} means delete the preimage
		// storage accounting
		x_s.NumStorageItems -= 2
		x_s.StorageSize -= AccountLookupConst + uint64(z)
		log.Debug(vm.logging, "FORGET OK1", "h", account_lookuphash, "z", z)
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	} else if len(X_s_l) == 1 {
		// preimage exists [x] => [x, y] where y is the current time, the time we are forgetting
		x_s.WriteLookup(account_lookuphash, uint32(z), append(X_s_l, []uint32{vm.Timeslot}...)) // [x, t]
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		log.Debug(vm.logging, "FORGET OK2", "h", account_lookuphash, "z", z, "newvalue", append(X_s_l, []uint32{vm.Timeslot}...))
		return
	} else if len(X_s_l) == 3 && X_s_l[1] < (vm.Timeslot-types.PreimageExpiryPeriod) {
		// [x,y,w] => [w, t] where y is the current time, the time we are forgetting
		X_s_l = []uint32{X_s_l[2], vm.Timeslot}               // w = X_s_l[2], t = vm.Timeslot
		x_s.WriteLookup(account_lookuphash, uint32(z), X_s_l) // [w, t]
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		log.Debug(vm.logging, "FORGET OK3", "h", account_lookuphash, "z", z, "newvalue", X_s_l)
		return
	}
	vm.Ram.WriteRegister(7, HUH)
	vm.HostResultCode = HUH
	log.Debug(vm.logging, "FORGET HUH", "h", account_lookuphash, "o", o)
}

// HistoricalLookup determines whether the preimage of some hash h was available for lookup by some service account a at some timeslot t, and if so, provide its preimage
func (vm *VM) hostHistoricalLookup() {
	var a = &types.ServiceAccount{}
	delta := vm.Delta
	s := vm.Service_index
	omega_7, _ := vm.Ram.ReadRegister(7)
	h, _ := vm.Ram.ReadRegister(8)
	o, _ := vm.Ram.ReadRegister(9)
	omega_10, _ := vm.Ram.ReadRegister(10)
	omega_11, _ := vm.Ram.ReadRegister(11)

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
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	// h := common.Hash(hBytes) not sure whether this is needed
	v := vm.hostenv.HistoricalLookup(a, vm.Timeslot, common.BytesToHash(hBytes))
	vLength := uint64(len(v))
	if vLength == 0 {
		vm.Ram.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		return
	} else {
		f := min(omega_10, vLength)
		l := min(omega_11, vLength-f)
		err := vm.Ram.WriteRAMBytes(uint32(o), v[f:l])
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.RESULT_FAULT
			return
		}
		vm.Ram.WriteRegister(7, vLength)
	}
}

var lastFrameTime time.Time
var frameCounter uint16

const minFrameCounter = 350 // when analytics start

// Export segment host-call
func (vm *VM) hostExport() {
	p, _ := vm.Ram.ReadRegister(7) // a0 = 7
	z, _ := vm.Ram.ReadRegister(8) // a1 = 8

	z = min(z, types.SegmentSize)

	x, errCode := vm.Ram.ReadRAMBytes(uint32(p), uint32(z))
	if errCode != OK {
		vm.ResultCode = types.RESULT_FAULT
		vm.terminated = true
		return
	}

	if vm.pushFrame != nil {
		vm.Exports = append(vm.Exports, x)

		if z != types.SegmentSize {
			frame := bytes.Join(vm.Exports, nil)
			now := time.Now()
			frameCounter++
			if !lastFrameTime.IsZero() {
				fps := float64(time.Second) / float64(now.Sub(lastFrameTime))
				fmt.Printf("Push frame %d %d bytes (FPS: %.2f)\n", frameCounter, len(frame), fps)
			}
			if frameCounter >= minFrameCounter && useEcalli500 {
				fn := fmt.Sprintf("test/doom_frame_%d.json", frameCounter)
				vm.TallyJSON(fn)
				fmt.Printf(" -- wrote %s\n", fn)
			}
			lastFrameTime = now

			vm.pushFrame(frame)
			vm.Exports = vm.Exports[:0]
		}

		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	}

	x = common.PadToMultipleOfN(x, types.SegmentSize)
	x = slices.Clone(x)

	if vm.ExportSegmentIndex+uint32(len(vm.Exports)) >= W_X { // W_X
		vm.Ram.WriteRegister(7, FULL)
		vm.HostResultCode = FULL
		return
	} else {
		vm.Ram.WriteRegister(7, uint64(vm.ExportSegmentIndex)+uint64(len(vm.Exports)))
		log.Debug(vm.logging, fmt.Sprintf("%s EXPORT#%d OK", vm.ServiceMetadata, uint64(len(vm.Exports))),
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

func (vm *VM) hostMachine() {
	po, _ := vm.Ram.ReadRegister(7)
	pz, _ := vm.Ram.ReadRegister(8)
	i, _ := vm.Ram.ReadRegister(9)
	p, errCode := vm.Ram.ReadRAMBytes(uint32(po), uint32(pz))
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
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

	// TODO: u := NewRAM()

	vm.RefineM_map[min_n] = &RefineM{
		P: p,
		//	U: u,
		I: i,
	}

	vm.Ram.WriteRegister(7, uint64(min_n))
}

func (vm *VM) hostPeek() {
	n, _ := vm.Ram.ReadRegister(7)
	o, _ := vm.Ram.ReadRegister(8)
	s, _ := vm.Ram.ReadRegister(9)
	z, _ := vm.Ram.ReadRegister(10)
	m_n, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.Ram.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}
	// read l bytes from m
	s_data, errCode := m_n.U.ReadRAMBytes(uint32(s), uint32(z))
	if errCode != OK {
		vm.Ram.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	// write l bytes to vm
	errCode = vm.Ram.WriteRAMBytes(uint32(o), s_data[:])
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	vm.Ram.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

func (vm *VM) hostPoke() {
	n, _ := vm.Ram.ReadRegister(7) // machine
	s, _ := vm.Ram.ReadRegister(8) // source
	o, _ := vm.Ram.ReadRegister(9) // dest
	z, _ := vm.Ram.ReadRegister(10)
	m_n, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.Ram.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}
	// read data from original vm
	s_data, errCode := vm.Ram.ReadRAMBytes(uint32(s), uint32(z))
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.RESULT_FAULT
		return
	}
	// write data to m_n
	errCode = m_n.U.WriteRAMBytes(uint32(o), s_data[:])
	if errCode != OK {
		vm.Ram.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	vm.Ram.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

func (vm *VM) hostExpunge() {
	n, _ := vm.Ram.ReadRegister(7)
	m, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.HostResultCode = WHO
		vm.Ram.WriteRegister(7, WHO)
		return
	}

	i := m.I
	delete(vm.RefineM_map, uint32(n))

	vm.Ram.WriteRegister(7, i)
	vm.HostResultCode = OK
}

func (vm *VM) hostPages() {
	n, _ := vm.Ram.ReadRegister(7)  // n: machine number
	p, _ := vm.Ram.ReadRegister(8)  // p: page number
	c, _ := vm.Ram.ReadRegister(9)  // c: number of pages to change
	r, _ := vm.Ram.ReadRegister(10) // r: access characteristics

	_, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.Ram.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}
	if p < 16 || p+c >= (1<<32)/Z_P || r > 4 {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		return
	}
	if p+c >= (1<<32)/Z_P && r > 2 {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		return
	}
	// TODO
	for i := int(0); i < int(c); i++ {
		switch {
		case r == 0: // To deallocate a page (previously void)
		//	m.U.SetPageAccess(int(p)+i, PageInaccessible)
		case r == 1 || r == 3: // read-only
		//	m.U.SetPageAccess(int(p)+i, PageImmutable)
		case r == 2 || r == 4: // read-write
			//	m.U.SetPageAccess(int(p)+i, PageMutable)
		}
	}

	vm.Ram.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// JIP-1 https://hackmd.io/@polkadot/jip1
func (vm *VM) hostLog() {
	level, _ := vm.Ram.ReadRegister(7)
	message, _ := vm.Ram.ReadRegister(10)
	messagelen, _ := vm.Ram.ReadRegister(11)

	messageBytes, errCode := vm.Ram.ReadRAMBytes(uint32(message), uint32(messagelen))
	if errCode != OK {
		vm.HostResultCode = OOB
		return
	}
	//levelName := getLogLevelName(level, vm.CoreIndex, string(vm.ServiceMetadata))
	vm.HostResultCode = OK
	switch level {
	case 0: // 0: User agent displays as fatal error
		log.Crit(vm.logging, string(messageBytes))
	case 1: // 1: User agent displays as warning
		log.Warn(vm.logging, string(messageBytes))
	case 2: // 2: User agent displays as important information
		log.Info(vm.logging, string(messageBytes))
		//fmt.Printf("INFO: %s\n", string(messageBytes)) // For CLI output
	case 3: // 3: User agent displays as helpful information
		log.Debug(vm.logging, string(messageBytes))
	case 4: // 4: User agent displays as pedantic information
		log.Trace(vm.logging, string(messageBytes))
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
