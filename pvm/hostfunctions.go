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

var hostFnNames = map[int]string{
	GAS:               "GAS",
	FETCH:             "FETCH",
	LOOKUP:            "LOOKUP",
	READ:              "READ",
	WRITE:             "WRITE",
	INFO:              "INFO",
	HISTORICAL_LOOKUP: "HISTORICAL_LOOKUP",
	EXPORT:            "EXPORT",
	MACHINE:           "MACHINE",
	PEEK:              "PEEK",
	POKE:              "POKE",
	PAGES:             "PAGES",
	INVOKE:            "INVOKE",
	EXPUNGE:           "EXPUNGE",
	BLESS:             "BLESS",
	ASSIGN:            "ASSIGN",
	DESIGNATE:         "DESIGNATE",
	CHECKPOINT:        "CHECKPOINT",
	NEW:               "NEW",
	UPGRADE:           "UPGRADE",
	TRANSFER:          "TRANSFER",
	EJECT:             "EJECT",
	QUERY:             "QUERY",
	SOLICIT:           "SOLICIT",
	FORGET:            "FORGET",
	YIELD:             "YIELD",
	PROVIDE:           "PROVIDE",
	MANIFEST:          "MANIFEST",
	LOG:               "DEBUG",
}

func HostFnToName(hostFn int) string {
	if name, ok := hostFnNames[hostFn]; ok {
		return name
	}
	return "UNKNOWN"
}

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
	chargedGas := uint64(10)
	switch host_fn {
	case LOG:
		chargedGas = 0
	}
	return int64(chargedGas)
}

// InvokeHostCall handles host calls
// Returns true if the call results in a halt condition, otherwise false
func (vm *VM) InvokeHostCall(host_fn int) (bool, error) {

	if vm.Gas-g < 0 {
		vm.ResultCode = types.WORKDIGEST_OOG
		vm.MachineState = OOG
		log.Warn(vm.logging, "OOG", "host_fn", host_fn, "gas", vm.Gas, "g", g)
		return true, fmt.Errorf("out of gas")
	}
	if PvmLogging {
		fmt.Printf("%d %s: Calling host function: %s %d\n", vm.Service_index, vm.Mode, HostFnToName(host_fn), host_fn)
	}

	return vm.hostFunction(host_fn)
}

func (vm *VM) hostFunction(host_fn int) (bool, error) {
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
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		log.Error(vm.logging, "UNKNOWN HOST CALL", "host_fn", host_fn)
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
		log.Debug(vm.logging, "INFO NONE", "s", omega_7)
		return
	}
	bo, _ := vm.Ram.ReadRegister(8)
	f, _ := vm.Ram.ReadRegister(9)
	l, _ := vm.Ram.ReadRegister(10)

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
			log.Debug(vm.logging, "INFO NONE", "s", omega_7)
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}
	log.Trace(vm.logging, "INFO OK", "s", fmt.Sprintf("%d", omega_7), "info", fmt.Sprintf("%v", elements))
	vm.Ram.WriteRegister(7, lenval)
	vm.HostResultCode = OK
}

// Bless updates
func (vm *VM) hostBless() {
	m, _ := vm.Ram.ReadRegister(7)
	a, _ := vm.Ram.ReadRegister(8)
	v, _ := vm.Ram.ReadRegister(9)
	o, _ := vm.Ram.ReadRegister(10)
	n, _ := vm.Ram.ReadRegister(11)

	bold_z := make(map[uint32]uint64)
	for i := 0; i < int(n); i++ {
		data, err := vm.Ram.ReadRAMBytes(uint32(o)+uint32(i)*12, 12)
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			return
		}
		s := binary.LittleEndian.Uint32(data[0:4])
		g := binary.LittleEndian.Uint64(data[4:12])
		bold_z[s] = g
	}

	bold_a := [types.TotalCores]uint32{}
	for i := 0; i < types.TotalCores; i++ {
		data, err := vm.Ram.ReadRAMBytes(uint32(a)+uint32(i)*4, 4)
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			return
		}
		bold_a[i] = binary.LittleEndian.Uint32(data)
	}

	xContext := vm.X
	xs, _ := xContext.GetX_s() //vm.X.S.ServiceIndex
	privilegedService_m := vm.X.U.PrivilegedState.ManagerServiceID
	if privilegedService_m != xs.ServiceIndex {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "BLESS HUH", "ManagerServiceID", privilegedService_m, "xs", xs.ServiceIndex)
		return
	}
	//TODO: check who!!
	if m > (1<<32)-1 || v > (1<<32)-1 {
		vm.Ram.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		log.Debug(vm.logging, "BLESS WHO", "m", fmt.Sprintf("%d", m), "a", fmt.Sprintf("%d", a), "v", fmt.Sprintf("%d", v))
		return
	}

	// Set (x'p)_m, (x'p)_a, (x'p)_v
	vm.X.U.PrivilegedState.ManagerServiceID = uint32(m)
	vm.X.U.PrivilegedState.AuthQueueServiceID = bold_a
	vm.X.U.PrivilegedState.UpcomingValidatorsServiceID = uint32(v)
	vm.X.U.PrivilegedState.AlwaysAccServiceID = bold_z

	vm.Ram.WriteRegister(7, OK)
	log.Debug(vm.logging, "BLESS OK", "m", fmt.Sprintf("%d", m), "a", fmt.Sprintf("%d", a), "v", fmt.Sprintf("%d", v))
	vm.HostResultCode = OK
}

// Assign Core x_c[i]
func (vm *VM) hostAssign() {
	c, _ := vm.Ram.ReadRegister(7)
	o, _ := vm.Ram.ReadRegister(8)
	a, _ := vm.Ram.ReadRegister(9)

	q, errcode := vm.Ram.ReadRAMBytes(uint32(o), 32*types.MaxAuthorizationQueueItems)
	if errcode != OK {
		vm.Ram.WriteRegister(7, OOB)
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		vm.terminated = true
		return
	}
	if c >= types.TotalCores {
		vm.Ram.WriteRegister(7, CORE)
		vm.HostResultCode = CORE
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
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "ASSIGN HUH", "c", c, "AuthQueueServiceID[c]", privilegedService_a, "xs", xs.ServiceIndex)
		return
	}

	copy(vm.X.U.QueueWorkReport[c][:], bold_q[:])
	vm.X.U.PrivilegedState.AuthQueueServiceID[c] = uint32(a)
	log.Debug(vm.logging, "ASSIGN OK", "c", c, "AuthQueueServiceID[c]", a, "xs", xs.ServiceIndex)
	vm.Ram.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// Designate validators
func (vm *VM) hostDesignate() {
	o, _ := vm.Ram.ReadRegister(7)
	v, errCode := vm.Ram.ReadRAMBytes(uint32(o), 336*types.TotalValidators)
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
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "DESIGNATE HUH", "UpcomingValidatorsServiceID", privilegedService_v, "xs", xs.ServiceIndex)
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
	log.Debug(vm.logging, "DESIGNATE OK", "validatorsLen", len(v_bold), "TotalValidators", types.TotalValidators, "UpcomingValidatorsServiceID", privilegedService_v, "xs", xs.ServiceIndex)
	vm.Ram.WriteRegister(7, OK)
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
	minServiceIndex := uint32(256)
	maxServiceIndex := uint32(4294966784) // 2^32 - 2^9
	serviceIndexRangeSize := maxServiceIndex - minServiceIndex + 1

	if i < minServiceIndex {
		i = minServiceIndex
	}

	for {
		if _, ok := u_d[i]; !ok {
			return i
		}
		offset := i - minServiceIndex
		nextOffset := (offset + bump) % serviceIndexRangeSize
		i = minServiceIndex + nextOffset
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
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
		log.Debug(vm.logging, "hostNew: NEW CASH xs.Balance < x_s_t", "xs.Balance", xs.Balance, "x_s_t", x_s_t, "x_s_index", xs.ServiceIndex)
		return
	}

	privilegedService_m := vm.X.U.PrivilegedState.ManagerServiceID
	if privilegedService_m != xs.ServiceIndex && f != 0 {
		// only ManagerServiceID can bestow gratis
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "hostNew: HUH", "ManagerServiceID", privilegedService_m, "xs", xs.ServiceIndex)
		return
	}

	// xs has enough balance to fund service creation of a AND covering its own threshold

	xi := xContext.NewServiceIndex
	// simulate a with c, g, m

	// [Gratis] a_r:t; a_f,a_a:0; a_p:x_s
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
	xContext.NewServiceIndex = new_check(xi, xContext.U.ServiceAccounts)

	const bump = uint32(42)
	const minServiceIndex = uint32(256)
	const maxServiceIndex = uint32(4294966784)
	const serviceIndexRangeSize = maxServiceIndex - minServiceIndex + 1

	if xi < minServiceIndex {
		xi = minServiceIndex
	}
	offset := xi - minServiceIndex
	nextOffset := (offset + bump) % serviceIndexRangeSize
	proposed_i := minServiceIndex + nextOffset
	xContext.NewServiceIndex = new_check(proposed_i, xContext.U.ServiceAccounts)

	a.WriteLookup(common.BytesToHash(c), uint32(l), []uint32{}, "memory")

	xContext.U.ServiceAccounts[xi] = a // this new account is included but only is written if (a) non-exceptional (b) exceptional and checkpointed
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
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

	D := vm.X.U.ServiceAccounts

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
		vm.X.U.ServiceAccounts[uint32(d)] = receiver
	}

	m, errCode := vm.Ram.ReadRAMBytes(uint32(o), M)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}
	t := types.DeferredTransfer{Amount: a, GasLimit: g, SenderIndex: vm.X.ServiceIndex, ReceiverIndex: uint32(d)} // CHECK
	log.Info(vm.logging, "TRANSFER START", "sender", fmt.Sprintf("%d", t.SenderIndex), "receiver", fmt.Sprintf("%d", d), "amount", fmt.Sprintf("%d", a), "gaslimit", g, "x_s_bal", xs.Balance, "DeferredTransfer", t.String())

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
	vm.X.Transfers = append(vm.X.Transfers, t)
	log.Debug(vm.logging, "TRANSFER OK", "sender", fmt.Sprintf("%d", t.SenderIndex), "receiver", fmt.Sprintf("%d", d), "amount", fmt.Sprintf("%d", a), "gaslimit", g, "x_s_bal", xs.Balance, "DeferredTransfer", t.String())
	vm.Ram.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// Gas Service
func (vm *VM) hostGas() {
	gasCost := int64(0)                             // Define gas cost.TODO: check 0 vs 10 here
	vm.Ram.WriteRegister(7, uint64(vm.Gas-gasCost)) // its gas remaining AFTER the host call
	//vm.Ram.WriteRegister(7, uint64(1234567)) // TEMPORARY
	vm.HostResultCode = OK
}

func (vm *VM) hostQuery() {
	o, _ := vm.Ram.ReadRegister(7)
	z, _ := vm.Ram.ReadRegister(8)
	h, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
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
		vm.Ram.WriteRegister(7, NONE)
		vm.Ram.WriteRegister(8, 0)
		vm.HostResultCode = NONE
		log.Debug(vm.logging, "QUERY NONE", "h", account_lookuphash, "z", z, "lookup_source", lookup_source)
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
	var v_Bytes []byte
	mode := vm.Mode
	allowed := false

	switch mode {
	case ModeIsAuthorized:
		switch datatype {
		case 0, 7, 8, 9, 10, 11, 12, 13:
			allowed = true
		default:
			allowed = false
		}
	case ModeRefine:
		switch datatype {
		case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13:
			allowed = true
		default:
			allowed = false
		}
	case ModeAccumulate:
		switch datatype {
		case 0, 1, 14, 15:
			allowed = true
		default:
			allowed = false
		}
	case ModeOnTransfer:
		switch datatype {
		case 0, 1, 16, 17:
			allowed = true
		default:
			allowed = false
		}
	}
	//log.Trace(vm.logging, "FETCH", "mode", mode, "allowed", allowed, "datatype", datatype, "omega_7", o, "omega_8", omega_8, "omega_9", omega_9, "omega_11", omega_11, "omega_12", omega_12, "vm.Extrinsics", fmt.Sprintf("%x", vm.Extrinsics), "wp", vm.WorkPackage)

	if allowed {
		switch datatype {

		case 0:
			v_Bytes, _ = types.ParameterBytes()
		//0a00000000000000010000000000000064000000000000000200200000000c000000809698000000000080f0fa020000000000ca9a3b00000000002d3101000000000800100008000300403800000300080006005000040080000500060000fa0000017cd20000093d0004000000000c00000204000000c0000080000000000c00000a000000
		//[8: B_I]        [8: B_L].       [8: B_S].       [C.][4:D   ][4:E   ][8:G_A.        ][8:G_I.        ][8:G_R         ][8:G_T*        ][H ][I ][J ][K ][4:L   ][N ][O ][P ][Q ][R ][T ][U ][V ][4:W_A ][4:W_B ][4:W_C ][4:W_E ][4:W_M ][4:W_P ][4:W_R*][4:W_T ][4:W_X ][4:Y.  ]

		case 1: // n
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
				//log.Trace(log.DA, fmt.Sprintf("%s Fetch segment hash:", vm.ServiceMetadata), "I", fmt.Sprintf("%v", common.Blake2Hash(v_Bytes)))
				//log.Trace(log.DA, fmt.Sprintf("%s Fetch segment byte:", vm.ServiceMetadata), "I", fmt.Sprintf("%x", v_Bytes))
				//log.Trace(log.DA, fmt.Sprintf("%s Fetch segment  len:", vm.ServiceMetadata), "I", fmt.Sprintf("%d", len(v_Bytes)))
				//log.Trace(log.DA, fmt.Sprintf("%s Fetch segment  idx:", vm.ServiceMetadata), "I", fmt.Sprintf("%d", omega_12))
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
			//log.Info(vm.logging, "FETCH wp", "len(v_Bytes)", len(v_Bytes))

		case 8: // p_u + | p_p
			type pp struct {
				PHash common.Hash `json:"hash"`
				PBlob []byte      `json:"blob"`
			}

			p := pp{
				PHash: vm.WorkPackage.AuthorizationCodeHash,
				PBlob: vm.WorkPackage.ConfigurationBlob,
			}
			v_Bytes, _ = types.Encode(p)
			//log.Info(vm.logging, "FETCH p_u + | p_p", "p_u", vm.WorkPackage.AuthorizationCodeHash, "p_p", vm.WorkPackage.ConfigurationBlob, "len", len(v_Bytes))
		case 9: // p_j
			v_Bytes = vm.WorkPackage.AuthorizationToken

		case 10: // p_X (refine context)
			v_Bytes, _ = types.Encode(vm.WorkPackage.RefineContext)

		case 11: // all work items
			v_Bytes = make([]byte, 0)
			// TODO: add discriminator in front
			for i, w := range vm.WorkPackage.WorkItems {
				fmt.Printf("WorkItem %d: %v\n", i, types.ToJSONHex(w))
				s_bytes, _ := w.EncodeS() // THIS IS CUSTOM ENCODING
				v_Bytes = append(v_Bytes, s_bytes...)
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
	} else {
		log.Warn(vm.logging, "FETCH FAIL NOT ALLOWED", "mode", mode, "allowed", allowed, "datatype", datatype, "omega_7", o, "omega_8", omega_8, "omega_9", omega_9, "omega_11", omega_11, "omega_12", omega_12)
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
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		log.Warn(vm.logging, "FETCH FAIL", "o", o, "v_Bytes", fmt.Sprintf("%x", v_Bytes), "l", l, "f", f, "f+l", f+l, "v_Bytes[f..f+l]", fmt.Sprintf("%x", v_Bytes[f:f+l]))
		return
	}
	log.Trace(vm.logging, "FETCH SUCC", "datatype", datatype, "v_Bytes", fmt.Sprintf("%x", v_Bytes), "useRawRAM", useRawRam, "o", fmt.Sprintf("%x", o), "v_Bytes", fmt.Sprintf("%x", v_Bytes), "l", l, "f", f, "f+l", f+l, "v_Bytes[f..f+l]", fmt.Sprintf("%x", v_Bytes[f:]))
	vm.Ram.WriteRegister(7, uint64(len(v_Bytes)))
}

func (vm *VM) hostYield() {
	o, _ := vm.Ram.ReadRegister(7)
	h, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}
	y := common.BytesToHash(h)
	vm.X.Yield = y
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}

	h := common.Blake2Hash(i)
	ok, X_s_l, lookup_source := a.ReadLookup(h, uint32(z), vm.hostenv)
	if !ok && len(X_s_l) > 0 {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "PROVIDE HUH", "omega_7", omega_7, "h", h, "z", z, "lookup_source", lookup_source)
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
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "PROVIDE HUH", "omega_7", omega_7, "h", h, "z", z)
		return
	}

	vm.X.Provided = append(vm.X.Provided, types.Provided{
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}

	xContext := vm.X
	if d == uint64(xContext.ServiceIndex) {
		vm.Ram.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		log.Warn(vm.logging, "EJECT WHO -- cannot eject self", "d", fmt.Sprintf("%d", d), "vm.X.ServiceIndex", fmt.Sprintf("%d", vm.X.ServiceIndex))
		return
	}
	bold_d, errCode := vm.getXUDS(d)
	if errCode != OK {
		vm.Ram.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}
	tst := common.Hash(types.E_l(uint64(vm.X.ServiceIndex), 32))
	//fmt.Printf("EJECT: x_s.ServiceIndex=%d bold_d.ServiceIndex=%d e32(s)=%s d.CodeHash=%s\n", vm.X.ServiceIndex, bold_d.ServiceIndex, tst, bold_d.CodeHash)
	l := max(AccountLookupConst, bold_d.StorageSize) - AccountLookupConst
	ok, D_lookup, lookup_source := bold_d.ReadLookup(common.BytesToHash(h), uint32(l), vm.hostenv)
	if !ok || bold_d.NumStorageItems != 2 || !bytes.Equal(tst.Bytes(), bold_d.CodeHash.Bytes()) {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "EJECT HUH", "d", fmt.Sprintf("%d", d), "h", h, "l", l, "lookup_source", lookup_source)
		return
	}
	xs, _ := xContext.GetX_s()
	xs.IncBalance(bold_d.Balance)

	if len(D_lookup) == 2 && D_lookup[1]+uint32(types.PreimageExpiryPeriod) < vm.Timeslot {
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		xContext.U.ServiceAccounts[uint32(d)] = bold_d
		bold_d.DeletedAccount = true
		bold_d.Mutable = true
		log.Debug(vm.logging, "EJECT OK", "d", fmt.Sprintf("%d", d))
		blobHash := common.BytesToHash(h)
		bold_d.WriteLookup(blobHash, uint32(l), nil, "trie") // nil means delete the lookup
		bold_d.WritePreimage(blobHash, []byte{}, "trie")     // []byte{} means delete the preimage. TODO: should be preimage_source
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}

	g := types.DecodeE_l(gasBytes)
	log.Trace(vm.logging, "INVOKE", "n", n, "o", o, "g", g)
	m_n, ok := vm.RefineM_map[uint32(n)]
	for i := 1; i < 14; i++ {
		reg_bytes, errCodeReg := vm.Ram.ReadRAMBytes(uint32(o)+8*uint32(i), 8)
		if errCodeReg != OK {
			vm.terminated = true
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			return
		}
		m_n.U.WriteRegister(i-1, types.DecodeE_l(reg_bytes))
	}

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
		Ram: m_n.U, // m_n.U,

		Backend:                    vm.Backend,
		basicBlockExecutionCounter: make(map[uint64]int),
		OP_tally:                   make(map[string]*X86InstTally),
	}
	initGas := vm.Gas
	//TODO: review here
	new_machine.logging = vm.logging
	new_machine.IsChild = true
	switch vm.Backend {
	case BackendInterpreter:
		new_machine.Execute(int(new_machine.pc), true)
	case BackendCompiler:
		if recRam, ok := m_n.U.(*CompilerRam); ok {
			log.Info(vm.logging, "INVOKE: Compiler", "n", n, "o", o, "g", int64(g))
			new_rvm, err := NewCompilerVMFromRam(new_machine, recRam)
			if err != nil {
				log.Error(vm.logging, "INVOKE: NewCompilerVMFromRam failed", "n", n, "o", o, "g", int64(g), "err", err)
				vm.terminated = true
				vm.ResultCode = types.WORKDIGEST_PANIC
				vm.MachineState = PANIC
				return
			}
			new_rvm.Execute(uint32(new_rvm.pc))
			new_rvm.Close()
		} else {
			log.Error(vm.logging, "INVOKE: m_n.U is not *CompilerRam")
			vm.terminated = true
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			return
		}
	case BackendSandbox:
		if emu, ok := m_n.U.(*Emulator); ok {
			log.Info(vm.logging, "INVOKE: CompilerSandbox", "n", n, "o", o, "g", int64(g))
			new_sandbox, err := NewCompilerSandboxVMFromEmulator(new_machine, emu)
			if err != nil {
				log.Error(vm.logging, "INVOKE: NewCompilerSandboxVMFromEmulator failed", "n", n, "o", o, "g", int64(g), "err", err)
				vm.terminated = true
				vm.ResultCode = types.WORKDIGEST_PANIC
				vm.MachineState = PANIC
				return
			}
			new_sandbox.ExecuteSandBox(uint64(new_sandbox.pc))
		} else {
			log.Error(vm.logging, "INVOKE: m_n.U is not *Emulator")
			vm.terminated = true
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			return
		}
	}

	m_n.I = new_machine.pc
	m_n.U = new_machine.Ram
	vm.RefineM_map[uint32(n)] = m_n
	log.Trace(vm.logging, "INVOKE DONE", "n", n, "o", o, "g", int64(g), "new_machine.Gas", new_machine.Gas, "new_machine.pc", new_machine.pc, "new_machine.MachineState", new_machine.MachineState)
	// Result after execution
	postGas := new_machine.Gas
	gasUsed := initGas - postGas
	log.Info(vm.logging, "INVOKE: gas used", "n", n, "o", o, "g", int64(g), "gasUsed", gasUsed, "new_machine.pc", new_machine.pc, "new_machine.MachineState", new_machine.MachineState)
	gasBytes = types.E_l(uint64(new_machine.Gas), 8)
	errCodeGas = vm.Ram.WriteRAMBytes(uint32(o), gasBytes)
	if errCodeGas != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}

	for i := 1; i < 14; i++ {
		regVal, _ := new_machine.Ram.ReadRegister(i - 1)
		reg_bytes := types.E_l(regVal, 8)
		errCode := vm.Ram.WriteRAMBytes(uint32(o)+8*uint32(i), reg_bytes)
		if errCode != OK {
			vm.terminated = true
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			return
		}
	}

	//TODO: who

	if new_machine.hostCall {
		vm.Ram.WriteRegister(7, HOST)
		vm.Ram.WriteRegister(8, uint64(new_machine.host_func_id))
		m_n.I = new_machine.pc + 1
		return
	}

	if new_machine.MachineState == FAULT {
		vm.Ram.WriteRegister(7, FAULT)
		vm.Ram.WriteRegister(8, uint64(new_machine.Fault_address))
		return
	}

	if new_machine.MachineState == OOG {
		vm.Ram.WriteRegister(7, OOG)
		return
	}

	if new_machine.MachineState == PANIC {
		vm.Ram.WriteRegister(7, PANIC)
		return
	}

	if new_machine.MachineState == HALT {
		vm.Ram.WriteRegister(7, HALT)
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}

	var account_blobhash common.Hash

	var v []byte
	var ok bool
	var preimage_source string

	account_blobhash = common.Hash(k_bytes)
	ok, v, preimage_source = a.ReadPreimage(account_blobhash, vm.hostenv)
	if !ok {
		vm.Ram.WriteRegister(7, NONE)
		log.Debug(vm.logging, "LOOKUP NONE", "s", fmt.Sprintf("%d", a.ServiceIndex), "h", account_blobhash, "preimage_source", preimage_source)
		vm.HostResultCode = NONE
		return
	}
	f = min(f, uint64(len(v)))
	l = min(l, uint64(len(v))-f)

	err := vm.Ram.WriteRAMBytes(uint32(o), v[:l])
	if err != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}

	if len(v) != 0 {
		vm.Ram.WriteRegister(7, uint64(len(v)))
	}
	logStr := fmt.Sprintf("len = %d", len(v))
	if len(v) < 200 {
		logStr = fmt.Sprintf("len = %d, v = %s", len(v), fmt.Sprintf("%x", v))
	}
	log.Trace(vm.logging, "LOOKUP OK", "s", fmt.Sprintf("%d", a.ServiceIndex), "h", h, "v", logStr)
	vm.HostResultCode = OK
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}
	// [0.6.7] No more hashing of mu_k
	//k := common.ServiceStorageKey(a.ServiceIndex, mu_k) // this does E_4(s) ... mu_4
	ok, val, storage_source := a.ReadStorage(mu_k, vm.hostenv)

	if !ok { // || true
		vm.Ram.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		log.Trace(vm.logging, "READ NONE", "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "kLen", len(mu_k), "ok", ok, "val", fmt.Sprintf("%x", val), "len(val)", len(val), "source", storage_source)
		return
	}
	log.Trace(vm.logging, "READ OK", "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "kLen", len(mu_k), "ok", ok, "val", fmt.Sprintf("%x", val), "len(val)", len(val), "source", storage_source)
	lenval := uint64(len(val))
	f = min(f, lenval)
	l = min(l, lenval-f)
	// TODO: check for OOB case again using o, f + l
	if errCode := vm.Ram.WriteRAMBytes(uint32(bo), val[f:f+l]); errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		log.Error(vm.logging, "READ RAM WRITE ERROR", "err", errCode)
		return
	}
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
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
	exists, oldValue, storage_source := a.ReadStorage(mu_k, vm.hostenv)
	v := []byte{}
	err := uint64(0)
	if vz > 0 {
		v, err = vm.Ram.ReadRAMBytes(uint32(vo), uint32(vz))
		if err != OK {
			vm.terminated = true
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
			return
		}
		//l = uint64(len(v))
	}
	a.WriteStorage(a.ServiceIndex, mu_k, v, vz == 0, storage_source)
	vm.ResultCode = OK
	vm.HostResultCode = OK

	key_len := uint64(len(mu_k)) // x in a_s(x,y) |y|
	val_len := uint64(len(v))    // y in a_s(x,y) |x|

	if !exists {
		if val_len > 0 {
			a.NumStorageItems++
			a.StorageSize += (AccountStorageConst + val_len + key_len) // [Gratis] Add ∑ 34 + |y| + |x|
		}
		log.Trace(vm.logging, "WRITE NONE", "numStorageItems", a.NumStorageItems, "StorageSize", a.StorageSize, "s", fmt.Sprintf("%d", a.ServiceIndex), "mu_k", fmt.Sprintf("%x", mu_k), "kLen", len(mu_k), "v", fmt.Sprintf("%x", v), "kLen", key_len, "vlen", len(v), "storage_source", storage_source)

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
		log.Error(vm.logging, "SOLICIT RAM READ ERROR", "err", err_h, "o", o, "z", z)
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}
	account_lookuphash := common.BytesToHash(hBytes)

	ok, X_s_l, lookup_source := xs.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		// when preimagehash is not found, put it into solicit request - so we can ask other DAs
		xs.WriteLookup(account_lookuphash, uint32(z), []uint32{}, lookup_source)
		xs.NumStorageItems += 2
		xs.StorageSize += AccountLookupConst + uint64(z)
		//log.Trace(vm.logging, "SOLICIT OK", "h", account_lookuphash, "z", z, "newvalue", []uint32{}, "lookup_source", lookup_source)
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	}

	if xs.Balance < xs.ComputeThreshold() {
		xs.WriteLookup(account_lookuphash, uint32(z), X_s_l, lookup_source)
		vm.Ram.WriteRegister(7, FULL)
		vm.HostResultCode = FULL
		//log.Trace(vm.logging, "SOLICIT FULL", "h", account_lookuphash, "z", z)
		return
	}
	if len(X_s_l) == 2 { // [x, y] => [x, y, t]
		xs.WriteLookup(account_lookuphash, uint32(z), append(X_s_l, []uint32{vm.Timeslot}...), lookup_source)
		//log.Trace(vm.logging, "SOLICIT OK 2", "h", account_lookuphash, "z", z, "newvalue", append(X_s_l, []uint32{vm.Timeslot}...))
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	} else {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		//log.Trace(vm.logging, "SOLICIT HUH", "h", account_lookuphash, "z", z, "len(X_s_l)", len(X_s_l))
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
		return
	}

	account_lookuphash := common.Hash(hBytes)
	account_blobhash := common.Hash(hBytes)

	lookup_ok, X_s_l, lookup_source := x_s.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	_, _, preimage_source := x_s.ReadPreimage(account_blobhash, vm.hostenv)

	if !lookup_ok {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Debug(vm.logging, "FORGET HUH", "h", account_lookuphash, "o", o, "lookup_source", lookup_source)
		return
	}
	if len(X_s_l) == 0 {
		// 0 [] case is when we solicited but never got a preimage, so we can forget it
		x_s.WriteLookup(account_lookuphash, uint32(z), nil, lookup_source) // nil means delete the lookup
		x_s.WritePreimage(account_blobhash, []byte{}, preimage_source)     // []byte{} means delete the preimage. TODO: should be preimage_source
		// storage accounting
		x_s.NumStorageItems -= 2
		x_s.StorageSize -= AccountLookupConst + uint64(z)
		log.Debug(vm.logging, "FORGET OK A", "h", account_lookuphash, "z", z, "vm.Timeslot", vm.Timeslot, "X_s_l[1]", X_s_l[1], "expiry", (vm.Timeslot - types.PreimageExpiryPeriod), "types.PreimageExpiryPeriod", types.PreimageExpiryPeriod)
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	} else if len(X_s_l) == 2 && X_s_l[1]+types.PreimageExpiryPeriod < vm.Timeslot {
		// 2 [x,y] case is when we have a forgotten a preimage and have PASSED the preimage expiry period, so we can forget it
		x_s.WriteLookup(account_lookuphash, uint32(z), nil, lookup_source) // nil means delete the lookup
		x_s.WritePreimage(account_blobhash, []byte{}, preimage_source)     // []byte{} means delete the preimage
		// storage accounting
		x_s.NumStorageItems -= 2
		x_s.StorageSize -= AccountLookupConst + uint64(z)
		log.Debug(vm.logging, "FORGET OK B", "h", account_lookuphash, "z", z, "vm.Timeslot", vm.Timeslot, "X_s_l[1]", X_s_l[1], "expiry", (vm.Timeslot - types.PreimageExpiryPeriod), "types.PreimageExpiryPeriod", types.PreimageExpiryPeriod)
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		return
	} else if len(X_s_l) == 1 {
		// preimage exists [x] => [x, y] where y is the current time, the time we are forgetting
		x_s.WriteLookup(account_lookuphash, uint32(z), append(X_s_l, []uint32{vm.Timeslot}...), lookup_source) // [x, t]
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		log.Debug(vm.logging, "FORGET OK C", "h", account_lookuphash, "z", z, "newvalue", append(X_s_l, []uint32{vm.Timeslot}...))
		return
	} else if len(X_s_l) == 3 && X_s_l[1]+types.PreimageExpiryPeriod < vm.Timeslot {
		// [x,y,w] => [w, t] where y is the current time, the time we are forgetting
		X_s_l = []uint32{X_s_l[2], vm.Timeslot}                              // w = X_s_l[2], t = vm.Timeslot
		x_s.WriteLookup(account_lookuphash, uint32(z), X_s_l, lookup_source) // [w, t]
		vm.Ram.WriteRegister(7, OK)
		vm.HostResultCode = OK
		log.Debug(vm.logging, "FORGET OK D", "h", account_lookuphash, "z", z, "newvalue", X_s_l)
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
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
			vm.ResultCode = types.WORKDIGEST_PANIC
			vm.MachineState = PANIC
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
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
			if frameCounter >= minFrameCounter && false { // WAS: useEcalli500
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
	if vm.Mode != ModeRefine {
		// TODO:
		vm.Ram.WriteRegister(7, WHAT)
		vm.HostResultCode = WHAT
		return

	}
	po, _ := vm.Ram.ReadRegister(7)
	pz, _ := vm.Ram.ReadRegister(8)
	i, _ := vm.Ram.ReadRegister(9)
	p, errCode := vm.Ram.ReadRAMBytes(uint32(po), uint32(pz))
	if errCode != OK {
		vm.terminated = true
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
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
	// todo: check if deblob sucess
	// TODO: u := NewRAM()
	var ram RAMInterface
	switch vm.Ram.(type) {
	case *RAM, *RawRAM:
		ram = NewRawRAM()
	case *CompilerRam:
		ram, _ = NewCompilerRam()
	case *Emulator:
		ram, _ = NewEmulator()
	}
	vm.RefineM_map[min_n] = &RefineM{
		P: p,
		U: ram,
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
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
		vm.ResultCode = types.WORKDIGEST_PANIC
		vm.MachineState = PANIC
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
	m, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.Ram.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		log.Warn(vm.logging, "hostPages WHO", "n", n, "p", p, "c", c, "r", r)
		return
	}

	if p > maxUint64-c {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Warn(vm.logging, "hostPages HUH", "n", n, "p", p, "c", c, "r", r)
		return
	}

	if p < 16 || p+c >= (1<<32)/Z_P || r > 4 {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Warn(vm.logging, "hostPages HUH", "n", n, "p", p, "c", c, "r", r)
		return
	}
	if p+c >= (1<<32)/Z_P && r > 2 {
		vm.Ram.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		log.Warn(vm.logging, "hostPages HUH", "n", n, "p", p, "c", c, "r", r)
		return
	}
	// TODO : use vm.RAM
	page := uint32(p)
	switch {
	case r == 0: // To deallocate a page (previously void)
		m.U.allocatePages(page, uint32(c))
	case r == 1 || r == 3: // read-only
		m.U.allocatePages(page, uint32(c))
	case r == 2 || r == 4: // read-write
		m.U.allocatePages(page, uint32(c))
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
	serviceMetadata := string(vm.ServiceMetadata)
	if serviceMetadata == "" {
		serviceMetadata = "unknown"
	}

	if vm.IsChild {
		serviceMetadata = fmt.Sprintf("%s-child", serviceMetadata)
	}
	loggingVerbose := false
	if !loggingVerbose {
		return
	}
	switch level {
	case 0: // 0: User agent displays as fatal error
		log.Crit(vm.logging, fmt.Sprintf("HOSTLOG-%s", serviceMetadata), "msg", string(messageBytes))
	case 1: // 1: User agent displays as warning
		log.Warn(vm.logging, fmt.Sprintf("HOSTLOG-%s", serviceMetadata), "msg", string(messageBytes))
	case 2: // 2: User agent displays as important information
		log.Info(vm.logging, fmt.Sprintf("HOSTLOG-%s", serviceMetadata), "msg", string(messageBytes))
	case 3: // 3: User agent displays as helpful information
		log.Debug(vm.logging, fmt.Sprintf("HOSTLOG-%s", serviceMetadata), "msg", string(messageBytes))
	case 4: // 4: User agent displays as pedantic information
		log.Trace(vm.logging, fmt.Sprintf("HOSTLOG-%s", serviceMetadata), "msg", string(messageBytes))
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
