package pvm

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/colorfulnotion/jam/common"
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
	EJECT             = 12 // formerly Quit
	QUERY             = 13
	SOLICIT           = 14
	FORGET            = 15
	YIELD             = 16
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

	SP1VERIFY     = 64
	ED25519VERIFY = 65
	LOG           = 100
	FOR_TEST      = 105
	DELAY         = 99
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
	"Gas":              {OK},
	"Lookup":           {OK, NONE, OOB},
	"Read":             {OK, OOB, NONE},
	"Write":            {OK, OOB, NONE, FULL},
	"Info":             {OK, OOB, NONE},
	"Sp1Groth16Verify": {OK, OOB, HUH},
	// B.7 Accumulate Functions
	"Bless":      {OK, OOB, WHO},
	"Assign":     {OK, OOB, CORE},
	"Designate":  {OK, OOB},
	"Checkpoint": {OK},
	"New":        {OK, OOB, CASH},
	"Upgrade":    {OK, OOB},
	"Transfer":   {OK, WHO, CASH, LOW, HIGH},
	"Quit":       {OK, OOB, WHO, LOW},
	"Solicit":    {OK, OOB, FULL, HUH},
	"Forget":     {OK, OOB, HUH},
	// B.8 Refine Functions
	"Historical_lookup": {OK, OOB},
	"Import":            {OK, OOB, NONE},
	"Export":            {OK, OOB, NONE},
	"Machine":           {OK, OOB},
	"Peek":              {OK, OOB, WHO},
	"Poke":              {OK, OOB, WHO},
	"Zero":              {OK, OOB, WHO},
	"Void":              {OK, OOB, WHO},
	"Invoke":            {OK, OOB, WHO, HOST, FAULT, OOB, PANIC},
	"Expunge":           {OK, OOB, WHO},
}

// Mapping of host function names to their indexes
var hostIndexMap = map[string]int{
	// B.6 General Functions
	"Gas":       GAS,
	"Lookup":    LOOKUP,
	"Read":      READ,
	"Write":     WRITE,
	"Info":      INFO,
	"Sp1Verify": SP1VERIFY,
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
	"For_test": FOR_TEST,
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

// InvokeHostCall handles host calls
// Returns true if the call results in a halt condition, otherwise false
func (vm *VM) InvokeHostCall(host_fn int) (bool, error) {
	if debug_pvm {
		fmt.Printf("vm.host_fn=%v\n", vm.host_func_id) //Do you need operand here?
	}

	if vm.Gas-g < 0 {
		vm.ResultCode = OOG
		return true, fmt.Errorf("Out of gas\n")
	} else {
		vm.Gas = vm.Gas - g
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
		omega_8, _ := vm.ReadRegister(8)
		omega_9, _ := vm.ReadRegister(9)
		vm.Gas = vm.Gas - int64(omega_8) - int64(omega_9)*(1<<32)
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

	// Refine functions
	case HISTORICAL_LOOKUP:
		vm.hostHistoricalLookup(0)
		return true, nil

	case FETCH:
		vm.hostFetch()
		return true, nil

	case EXPORT:
		vm.hostExport(0)
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

	case DELAY:
		vm.hostDelay()
		return true, nil

	default:
		vm.Gas = vm.Gas + g
		return false, fmt.Errorf("unknown host call: %d\n", host_fn)
	}
}

// func min(x, y uint64) uint64 {
// 	if x < y {
// 		return x
// 	}
// 	return y
// }

// Information-on-Service
func (vm *VM) hostInfo() {
	omega_7, _ := vm.ReadRegister(7)

	t, errCode := vm.getXUDS(omega_7)
	if errCode != OK {
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		return
	}
	bo, _ := vm.ReadRegister(8)

	e := []interface{}{t.CodeHash, t.Balance, t.ComputeThreshold(), t.GasLimitG, t.GasLimitM, t.ComputeNumStorageItems(), t.ComputeStorageSize()}
	m, err := types.Encode(e)
	if err != nil {
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		return
	}
	errcode := vm.Ram.WriteRAMBytes(uint32(bo), m[:])
	if errcode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// Bless updates
func (vm *VM) hostBless() {
	m, _ := vm.ReadRegister(7)
	a, _ := vm.ReadRegister(8)
	v, _ := vm.ReadRegister(9)
	// Set (x'p)_m, (x'p)_a, (x'p)_v
	vm.X.U.PrivilegedState.Kai_m = uint32(m)
	vm.X.U.PrivilegedState.Kai_a = uint32(a)
	vm.X.U.PrivilegedState.Kai_v = uint32(v)
	vm.HostResultCode = OK
}

// Assign Core x_c[i]
func (vm *VM) hostAssign() {
	core, _ := vm.ReadRegister(7)
	if core >= numCores {
		vm.HostResultCode = CORE
		return
	}
	o, _ := vm.ReadRegister(8)
	c, _ := vm.Ram.ReadRAMBytes(uint32(o), 32*types.MaxAuthorizationQueueItems)
	qi := make([]common.Hash, 32)
	for i := 0; i < 32; i++ {
		qi[i] = common.BytesToHash(c[i:(i + 32)])
	}
	copy(vm.X.U.QueueWorkReport[core][:], qi[:])
	vm.HostResultCode = OK
}

// Designate validators
func (vm *VM) hostDesignate() {
	o, _ := vm.ReadRegister(7)
	v, errCode := vm.Ram.ReadRAMBytes(uint32(o), 176*V)
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
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
	vm.HostResultCode = OK
}

// Checkpoint gets Gas-remaining
func (vm *VM) hostCheckpoint() {
	vm.Y = vm.X.Clone()
	vm.WriteRegister(7, uint64(vm.Gas)) // CHECK
	vm.HostResultCode = OK
}

func bump(i uint32) uint32 {
	const lowerLimit uint32 = 1 << 8               // 2^8 = 256
	const upperLimit uint32 = (1 << 32) - (1 << 9) // 2^32 - 2^9 = 4294966784

	//(i - 256 + 42) =  (i - 241)??
	adjusted := int64(i) - int64(lowerLimit) + 42

	// need to make sure the result of the modulus operation is non-negative.
	// This is done by: ((adjusted % upperLimit) + upperLimit) % upperLimit.
	// This expression guarantees that if `adjusted` is negative, adding `upperLimit` first makes it positive.
	// Then, applying `% upperLimit` again ensures the result is within the range [0, upperLimit).
	// modResult := ((adjusted % int64(upperLimit)) + int64(upperLimit)) % int64(upperLimit)
	modResult := lowerLimit + uint32(adjusted)%upperLimit

	// Step 3: Return the result by adding `lowerLimit` back.
	// This aligns the final result with the desired range: [2^8, 2^32 - 2^9].
	// return lowerLimit + uint32(modResult)
	return uint32(modResult)
}

func check(i uint32, u_d map[uint32]*types.ServiceAccount) uint32 {
	for {
		if _, ok := u_d[i]; !ok {
			return i
		}
		i = ((i - 256 + 1) % (4294967296 - 512)) + 256 // 2^32 - 2^9 + 2^8
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
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	l, _ := vm.ReadRegister(8)
	g, _ := vm.ReadRegister(9)
	m, _ := vm.ReadRegister(10)

	xi := xContext.I

	// simulate a with c, g, m
	a := &types.ServiceAccount{
		ServiceIndex:    xi,
		Mutable:         true,
		Dirty:           true,
		CodeHash:        common.BytesToHash(c),
		GasLimitG:       uint64(g),
		GasLimitM:       uint64(m),
		NumStorageItems: 2*1 + 0,            //a_s = 2⋅∣al∣+∣as∣
		StorageSize:     uint64(81 + l + 0), //a_l =  ∑ 81+z per (h,z) + ∑ 32+s
		Storage:         make(map[common.Hash]types.StorageObject),
		Lookup:          make(map[common.Hash]types.LookupObject),
		Preimage:        make(map[common.Hash]types.PreimageObject),
	}
	a.Balance = a.ComputeThreshold()
	// fmt.Printf("Service %d hostNew %v => %d, code hash: %v\n", s, a.CodeHash, a.GetServiceIndex(), common.BytesToHash(c))

	if debug_host {
		fmt.Printf("Service %d hostNew %v => %d\n", xContext.S, a.CodeHash, a.GetServiceIndex())
	}
	// Compute footprint & threshold: a_l, a_s, a-t

	xs.DecBalance(a.Balance)
	if xs.Balance >= xs.ComputeThreshold() {
		//xs has enough balance to fund the creation of a AND covering its own threshold
		// xi' <- check(bump(xi))
		// vm.WriteRegister(7, xi)
		// xContext.I = vm.hostenv.Check(bump(xi)) // this is the next xi
		xContext.I = check(bump(xi), xContext.U.D)

		// I believe this is the same as solicit. where l∶{(c, l)↦[]} need to be set, which will later be provided by E_P
		a.WriteLookup(common.BytesToHash(c), uint32(l), []uint32{}) // *** CHECK

		// (x's)b <- (xs)b - at
		// xContext.S = a.ServiceIndex()

		// Here we are adding the new service account to the map
		xContext.U.D[xi] = a

		vm.X = xContext
		vm.WriteRegister(7, uint64(xi))
		vm.HostResultCode = OK
	} else {
		if debug_host {
			fmt.Println("Balance insufficient")
		}
		xs.IncBalance(a.Balance)
		vm.WriteRegister(7, CASH)
		vm.HostResultCode = CASH //balance insufficient
	}
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
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	xs.Dirty = true
	xs.CodeHash = common.BytesToHash(c)
	xs.GasLimitG = g
	xs.GasLimitM = m
	vm.WriteRegister(7, OK)
	// xContext.D[s] = xs // not sure if this is needed
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
			return
		}
		vm.X.U.D[uint32(d)] = receiver
	}

	m, errCode := vm.Ram.ReadRAMBytes(uint32(o), M)
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	t := types.DeferredTransfer{Amount: a, GasLimit: g, SenderIndex: vm.X.S, ReceiverIndex: uint32(d)} // CHECK

	if g < receiver.GasLimitM {
		vm.WriteRegister(7, LOW)
		vm.HostResultCode = LOW
		return
	}

	xs.DecBalance(a)
	b := xs.Balance

	if b < xs.ComputeThreshold() {
		xs.IncBalance(a)
		vm.WriteRegister(7, CASH)
		vm.HostResultCode = CASH
		return
	}

	copy(t.Memo[:], m[:])
	vm.X.T = append(vm.X.T, t)
	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// Gas Service
func (vm *VM) hostGas() {
	vm.WriteRegister(7, uint64(vm.Gas))
	vm.HostResultCode = OK
}

func (vm *VM) hostQuery() {
	o, _ := vm.ReadRegister(7)
	z, _ := vm.ReadRegister(8)
	h, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	a, _ := vm.X.GetX_s()
	ok, anchor_timeslot := a.ReadLookup(common.BytesToHash(h), uint32(z), vm.hostenv)
	if !ok {
		vm.WriteRegister(7, NONE)
		vm.WriteRegister(8, 0)
		vm.HostResultCode = NONE
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
		break
	case 2:
		x := anchor_timeslot[0]
		y := anchor_timeslot[1]
		vm.WriteRegister(7, 2+(1<<32)*uint64(x))
		vm.WriteRegister(8, uint64(y))
		break
	case 3:
		x := anchor_timeslot[0]
		y := anchor_timeslot[1]
		z := anchor_timeslot[2]
		vm.WriteRegister(7, 3+(1<<32)*uint64(x))
		vm.WriteRegister(8, uint64(y)+(1<<32)*uint64(z))
		break
	}
	vm.HostResultCode = OK
}
func (vm *VM) getWorkPackage() []byte {
	// TODO
	return []byte{}
}

func (vm *VM) getAuthorizerOutput() []byte {
	// TODO
	return []byte{}
}

func (vm *VM) getPayload() []byte {
	// TODO
	return []byte{}
}

func (vm *VM) getNumberOfExtrinsics() []byte {
	// TODO
	numExtrinsics := uint32(1)
	return common.Uint32ToBytes(numExtrinsics)
}

func (vm *VM) getNumberOfWorkItems() []byte {
	// TODO
	numWorkItems := uint32(1)
	return common.Uint32ToBytes(numWorkItems)
}
func (vm *VM) getWorkPackageByWorkItemExtrinsic(workitemindex uint32, extrinsicindex uint32) (data []byte, ok bool) {
	return []byte{}, false
}

func (vm *VM) getWorkPackageByWorkItemExtrinsicAll(workitemindex uint32) (data []byte, ok bool) {
	return []byte{}, false
}

func (vm *VM) getWorkPackageByExtrinsicHash(h common.Hash) (data []byte, ok bool) {
	return []byte{}, false
}

func (vm *VM) getWorkPackageImportSegment(workitemindex uint32, importindex uint32) (data []byte, ok bool) {
	return []byte{}, false
}

func (vm *VM) getWorkPackageImportedSegmentAll(workitemindex uint32) (data []byte, ok bool) {
	return []byte{}, false
}

// TODO: Fetch (0.5.5) https://github.com/gavofyork/graypaper/issues/186
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
	var v_Bytes []byte
	if datatype == 0 {
		v_Bytes, _ = types.Encode(vm.WorkPackage)
	}
	if datatype == 1 {
		v_Bytes = vm.Authorization
	}
	if datatype == 2 && omega_11 < uint64(len(vm.WorkPackage.WorkItems)) {
		v_Bytes = vm.WorkPackage.WorkItems[omega_11].Payload
	}
	if datatype == 3 && omega_11 < uint64(len(vm.WorkPackage.WorkItems)) && omega_12 < uint64(len(vm.WorkPackage.WorkItems[omega_11].Extrinsics)) {
		// get extrinsic by omega 11 and omega 12
		extrinsicHash := common.Blake2Hash(vm.Extrinsics[omega_11][:])
		extrinsicLength := len(vm.Extrinsics[omega_11])
		workitemExtrinisc := types.WorkItemExtrinsic{
			Hash: extrinsicHash,
			Len:  uint32(extrinsicLength),
		}
		if vm.WorkPackage.WorkItems[omega_11].Extrinsics[omega_12] == workitemExtrinisc {
			v_Bytes = vm.Extrinsics[omega_11][:]
		}
	}
	if datatype == 4 && omega_11 < uint64(len(vm.WorkPackage.WorkItems[vm.WorkItemIndex].Extrinsics)) {
		// get extrinsic by index within the sequence specified by this work-item
		extrinsicHash := common.Blake2Hash(vm.Extrinsics[vm.WorkItemIndex][:])
		extrinsicLength := len(vm.Extrinsics[vm.WorkItemIndex])
		workitemExtrinisc := types.WorkItemExtrinsic{
			Hash: extrinsicHash,
			Len:  uint32(extrinsicLength),
		}
		if vm.WorkPackage.WorkItems[vm.WorkItemIndex].Extrinsics[omega_11] == workitemExtrinisc {
			v_Bytes = vm.Extrinsics[vm.WorkItemIndex][:]
		}
	}
	if datatype == 5 && len(vm.Imports) > 0 {
		// get imported segment by omega 11 and omega 12
		if omega_11 < uint64(len(vm.Imports)) && omega_12 < uint64(len(vm.Imports[omega_11])) {
			v_Bytes = vm.Imports[omega_11][:]
		}
	}
	if datatype == 6 && len(vm.Imports) > 0 {
		// get imported segment by work item index
		if omega_11 < uint64(len(vm.Imports[vm.WorkItemIndex])) {
			v_Bytes = vm.Imports[vm.WorkItemIndex][:]
		}
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
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	vm.WriteRegister(7, uint64(l))
}

func (vm *VM) hostYield() {
	o, _ := vm.ReadRegister(7)
	h, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	y := common.BytesToHash(h)
	vm.X.Y = &y
	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

func (vm *VM) hostEject() {
	d, _ := vm.ReadRegister(7)
	o, _ := vm.ReadRegister(8)
	h, err := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if err != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	bold_d, ok := vm.X.U.D[uint32(d)]
	if d == uint64(vm.X.S) || !ok || bold_d.CodeHash != common.Hash(types.E_l(uint64(vm.X.S), 32)) {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}
	l := max(81, bold_d.ComputeStorageSize()) - 81

	ok, D_lookup := bold_d.ReadLookup(common.BytesToHash(h), uint32(l), vm.hostenv)
	if !ok || bold_d.ComputeNumStorageItems() != 2 {
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		return
	}

	s, _ := vm.getXUDS(uint64(vm.X.S))
	s = s.Clone()
	s.Balance += bold_d.Balance

	if len(D_lookup) == 2 && D_lookup[1] < vm.Timeslot-uint32(D) {
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
		delete(vm.X.U.D, uint32(d))
		vm.X.U.D[vm.X.S] = s
		return
	}

	vm.WriteRegister(7, HUH)
	vm.HostResultCode = HUH
}
func (vm *VM) setGasRegister(gasBytes, registerBytes []byte) {

	// gas todo
	registers := make([]uint64, 13)
	for i := 0; i < 13; i++ {
		registers[i] = binary.LittleEndian.Uint64(registerBytes[i*8 : (i+1)*8])
	}
	vm.register = registers
}

// Invoke5
func (vm *VM) hostInvoke() {
	n, _ := vm.ReadRegister(7)
	o, _ := vm.ReadRegister(8)
	gasBytes, errCodeGas := vm.Ram.ReadRAMBytes(uint32(o), 8)
	if errCodeGas != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	g := types.DecodeE_l(gasBytes)

	m_n_reg := make([]uint64, 13)
	for i := 1; i < 14; i++ {
		reg_bytes, errCodeReg := vm.Ram.ReadRAMBytes(uint32(o)+8*uint32(i), 8)
		if errCodeReg != OK {
			vm.WriteRegister(7, OOB)
			vm.HostResultCode = OOB
			return
		}
		m_n_reg[i-1] = types.DecodeE_l(reg_bytes)
	}

	m_n, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}

	program := DecodeProgram_pure_pvm_blob(m_n.P)
	new_machine := &VM{
		JSize:   program.JSize,
		Z:       program.Z,
		J:       program.J,
		code:    program.Code,
		bitmask: program.K[0],

		pc:       m_n.I,
		Gas:      int64(g),
		register: m_n_reg,
		Ram:      m_n.U,
	}

	new_machine.Execute(0)
	m_n.I = new_machine.pc
	m_n.U = new_machine.Ram
	vm.RefineM_map[uint32(n)] = m_n

	// Result after execution
	gasBytes = types.E_l(uint64(new_machine.Gas), 8)
	errCodeGas = vm.Ram.WriteRAMBytes(uint32(o), gasBytes)
	if errCodeGas != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	for i := 1; i < 14; i++ {
		reg_bytes := types.E_l(new_machine.register[i-1], 8)
		errCode := vm.Ram.WriteRAMBytes(uint32(o)+8*uint32(i), reg_bytes)
		if errCode != OK {
			vm.WriteRegister(7, OOB)
			vm.HostResultCode = OOB
			return
		}
	}

	if new_machine.ResultCode == HOST {
		vm.WriteRegister(7, HOST)
		vm.WriteRegister(8, uint64(new_machine.host_func_id))
		return
	}

	if new_machine.ResultCode == FAULT {
		vm.WriteRegister(7, FAULT)
		vm.WriteRegister(8, uint64(new_machine.Fault_address))
		return
	}

	if new_machine.ResultCode == OOG {
		vm.WriteRegister(7, OOG)
		return
	}

	if new_machine.ResultCode == PANIC {
		vm.WriteRegister(7, PANIC)
		return
	}

	if new_machine.ResultCode == HALT {
		vm.WriteRegister(7, HALT)
		return
	}

}

// Lookup preimage
func (vm *VM) hostLookup() {
	omega_7, _ := vm.ReadRegister(7)

	var a *types.ServiceAccount
	if omega_7 == uint64(vm.Service_index) || omega_7 == maxUint64 {
		a = vm.ServiceAccount
	} else {
		a, _ = vm.getXUDS(omega_7)
	}

	ho, _ := vm.ReadRegister(8)
	bo, _ := vm.ReadRegister(9)
	bz, _ := vm.ReadRegister(10)
	k_bytes, err_k := vm.Ram.ReadRAMBytes(uint32(ho), 32)
	if err_k != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	var account_blobhash common.Hash

	var v []byte
	var ok bool

	account_blobhash = common.Blake2Hash(k_bytes)
	ok, v = a.ReadPreimage(account_blobhash, vm.hostenv)
	if !ok {
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		return
	}
	l := uint64(len(v))
	l = min(l, uint64(bz))

	err := vm.Ram.WriteRAMBytes(uint32(bo), v[:l])
	if err != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	if len(v) != 0 {
		vm.WriteRegister(7, l)
	}
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
	bz, _ := vm.ReadRegister(11)
	k, err_k := vm.Ram.ReadRAMBytes(uint32(ko), uint32(kz)) // this is the raw key.
	if err_k != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	// var account_storagehash common.Hash
	var val []byte
	_, val = a.ReadStorage(k, vm.hostenv)

	l := uint64(len(val))
	l = min(l, uint64(bz))
	if l != 0 {
		vm.Ram.WriteRAMBytes(uint32(bo), val[:l])
		vm.WriteRegister(7, l)
	} else {
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
	}
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
	k, err_k := vm.Ram.ReadRAMBytes(uint32(ko), uint32(kz))
	if err_k != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	var l uint64
	_, storage := a.ReadStorage(k, vm.hostenv)
	l = uint64(len(storage))

	if vz == 0 {
		a.WriteStorage(a.ServiceIndex, k, []byte{})
		vm.WriteRegister(7, NONE)
		vm.HostResultCode = NONE
		return
	}

	if a.ComputeThreshold() <= a.Balance {
		// adjust S
		v := []byte{}
		err := uint64(0)
		if vz > 0 {
			v, err = vm.Ram.ReadRAMBytes(uint32(vo), uint32(vz))
			if err != OK {
				vm.WriteRegister(7, OOB)
				vm.HostResultCode = OOB
				return
			}
		}
		a.WriteStorage(a.ServiceIndex, k, v)

		vm.WriteRegister(7, l)
		vm.HostResultCode = OK
	} else {
		vm.WriteRegister(7, FULL)
		vm.HostResultCode = FULL
	}
}

// Solicit preimage
func (vm *VM) hostSolicit() {
	xs, _ := vm.X.GetX_s()
	// Got l of X_s by setting s = 1, z = z(from RAM)
	o, _ := vm.ReadRegister(7)
	z, _ := vm.ReadRegister(8)                          // z: blob_len
	hBytes, err_h := vm.Ram.ReadRAMBytes(uint32(o), 32) // h: blobHash
	if err_h != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	account_lookuphash := common.BytesToHash(hBytes)

	ok, X_s_l := xs.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		// when preimagehash is not found, put it into solicit request - so we can ask other DAs
		xs.WriteLookup(account_lookuphash, uint32(z), []uint32{})
	} else if len(X_s_l) == 2 { // [x, y]
		xs.WriteLookup(account_lookuphash, uint32(z), append(X_s_l, []uint32{vm.Timeslot}...))
	} else {
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		return
	}
	if xs.Balance < xs.ComputeThreshold() {
		xs.WriteLookup(account_lookuphash, uint32(z), X_s_l)
		vm.WriteRegister(7, FULL)
		vm.HostResultCode = FULL
		return
	}
	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// Forget preimage
func (vm *VM) hostForget() {
	x_s, _ := vm.X.GetX_s()
	o, _ := vm.ReadRegister(7)
	z, _ := vm.ReadRegister(8)
	hBytes, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	account_lookuphash := common.BytesToHash(hBytes)
	account_blobhash := common.Blake2Hash(hBytes)

	ok, X_s_l := x_s.ReadLookup(account_lookuphash, uint32(z), vm.hostenv)
	if !ok {
		vm.WriteRegister(7, HUH)
		vm.HostResultCode = HUH
		return
	}

	if len(X_s_l) == 0 || (len(X_s_l) == 2) && X_s_l[1] < (vm.Timeslot-D) {
		x_s.WriteLookup(account_lookuphash, uint32(z), nil) // nil means delete the lookup
		x_s.WritePreimage(account_blobhash, []byte{})       // []byte{} means delete the preimage
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
	} else if len(X_s_l) == 1 {
		x_s.WriteLookup(account_lookuphash, uint32(z), append(X_s_l, []uint32{vm.Timeslot}...))
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
	} else if len(X_s_l) == 3 && X_s_l[1] < (vm.Timeslot-D) {
		X_s_l = []uint32{X_s_l[2], vm.Timeslot}
		x_s.WriteLookup(account_lookuphash, uint32(z), X_s_l)
		vm.WriteRegister(7, OK)
		vm.HostResultCode = OK
	}
}

// HistoricalLookup determines whether the preimage of some hash h was available for lookup by some service account a at some timeslot t, and if so, provide its preimage
func (vm *VM) hostHistoricalLookup(t uint32) {
	var a = &types.ServiceAccount{}
	delta := vm.Delta
	s := vm.Service_index
	omega_7, _ := vm.ReadRegister(7)
	ho, _ := vm.ReadRegister(8)
	bo, _ := vm.ReadRegister(9)
	bz, _ := vm.ReadRegister(10)

	if omega_7 == NONE {
		a = delta[s]
	} else {
		a = delta[uint32(omega_7)]
	}

	hBytes, errCode := vm.Ram.ReadRAMBytes(uint32(ho), 32)
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
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
		l := uint64(vLength)
		l = min(l, bz)
		vm.Ram.WriteRAMBytes(uint32(bo), v[:l])
		vm.WriteRegister(7, vLength)
	}
}

// Import Segment
// func (vm *VM) hostImport() {
// 	// import  - which copies  a specific i  (e.g. holding the bytes "9") into RAM from "ImportDA" to be "accumulated"
// 	omega_0, _ := vm.ReadRegister(7) // a0 = 7
// 	var v_Bytes []byte
// 	if omega_0 < uint64(len(vm.Imports)) {
// 		v_Bytes = vm.Imports[omega_0][:]
// 	} else {
// 		v_Bytes = []byte{}
// 	}
// 	o, _ := vm.ReadRegister(8) // a1 = 8
// 	l, _ := vm.ReadRegister(9) // a2 = 9
// 	if l > (W_E * W_S) {
// 		l = W_E * W_S
// 	}

// 	if len(v_Bytes) != 0 {
// 		errCode := vm.Ram.WriteRAMBytes(uint32(o), v_Bytes[:])
// 		if errCode != OK {
// 			vm.WriteRegister(7, OOB)
// 			vm.HostResultCode = OOB
// 			return
// 		}
// 		vm.WriteRegister(7, OK)
// 		vm.HostResultCode = OK
// 	} else {
// 		vm.WriteRegister(7, NONE)
// 		vm.HostResultCode = NONE
// 	}
// }

// Export segment host-call
func (vm *VM) hostExport(pi uint32) [][]byte {
	p, _ := vm.ReadRegister(7) // a0 = 7
	z, _ := vm.ReadRegister(8) // a1 = 8
	if z > (W_E * W_S) {
		z = W_E * W_S
	}

	e := vm.Exports

	x, errCode := vm.Ram.ReadRAMBytes(uint32(p), uint32(z))
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.Exports = e
		vm.HostResultCode = OOB
		return e
	}

	/*  apply eq(187) zero-padding function:

	And P is the zero-padding function to take an octet array to some multiple of n in length:
	(187) 	P n∈N 1∶ ∶{ Y → Y k⋅n
			x ↦ x ⌢ [0, 0, ...] ((∣x∣+n−1) mod n)+1...n

	n := (W_E * W_S)
	length := n - ((len(x) + n - 1) % n) + 1
	zeroSequence := make([]byte, length)
	x = append(x, zeroSequence...)
	*/
	x = common.PadToMultipleOfN(x, W_E*W_S)

	ς := vm.ExportSegmentIndex   // Assume ς (sigma, Represent segment offset), need to get ς properly
	if ς+uint32(len(e)) >= W_X { // W_X
		vm.WriteRegister(7, FULL)
		vm.Exports = e
		vm.HostResultCode = FULL
		return e
	} else {
		vm.WriteRegister(7, uint64(ς)+uint64(len(e)))
		e = append(e, x)
		vm.Exports = e
		// errCode = vm.hostenv.ExportSegment(x)
		vm.HostResultCode = OK
		return e
	}
}

func (vm *VM) hostMachine() {
	po, _ := vm.ReadRegister(7)
	pz, _ := vm.ReadRegister(8)
	i, _ := vm.ReadRegister(9)
	p, errCode := vm.Ram.ReadRAMBytes(uint32(po), uint32(pz))
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	min_n := uint32(0)
	for n, _ := range vm.RefineM_map {
		if n == min_n {
			min_n = n + 1
		}
	}

	u := NewRAM()
	vm.RefineM_map[min_n] = &RefineM{}

	vm.RefineM_map[min_n].P = p
	vm.RefineM_map[min_n].U = u
	vm.RefineM_map[min_n].I = i
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
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

func (vm *VM) hostPoke() {
	n, _ := vm.ReadRegister(7)
	s, _ := vm.ReadRegister(8)
	o, _ := vm.ReadRegister(9)
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
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
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
	_, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.HostResultCode = WHO
		vm.WriteRegister(7, WHO)
		return
	}

	delete(vm.RefineM_map, uint32(n))

	vm.WriteRegister(7, OK)
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
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	for _, page := range m.U.Pages {
		if page.Access.Inaccessible {
			vm.WriteRegister(7, OOB)
			vm.HostResultCode = OOB
			return
		}
	}
	var err uint64
	var access_mode AccessMode

	// set page access to writable to write [0,0,0,....] to the page
	access_mode = AccessMode{Inaccessible: false, Writable: true, Readable: false}
	err = m.U.SetPageAccess(uint32(p), uint32(c), access_mode)
	if err != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	// write [0,0,0,....] to the page
	err = m.U.WriteRAMBytes(uint32(p)*PageSize, make([]byte, 0))
	if err != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	// set page access to inaccessible
	access_mode = AccessMode{Inaccessible: true, Writable: false, Readable: false}
	err = m.U.SetPageAccess(uint32(p), uint32(c), access_mode)
	if err != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

func (vm *VM) hostZero() {
	n, _ := vm.ReadRegister(7)
	p, _ := vm.ReadRegister(8)
	c, _ := vm.ReadRegister(9)
	if p < 16 || p+c > (1<<32)/Z_P {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}
	m, ok := vm.RefineM_map[uint32(n)]
	if !ok {
		vm.WriteRegister(7, WHO)
		vm.HostResultCode = WHO
		return
	}
	var err uint64
	access_mode := AccessMode{Inaccessible: false, Writable: true, Readable: false}
	err = m.U.SetPageAccess(uint32(p), uint32(c), access_mode)
	if err != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	err = m.U.WriteRAMBytes(uint32(p)*PageSize, make([]byte, 0))
	if err != OK {
		vm.WriteRegister(7, OOB)
		vm.HostResultCode = OOB
		return
	}

	vm.WriteRegister(7, OK)
	vm.HostResultCode = OK
}

// For empty test case
func (vm *VM) hostDelay() {
	delay_seconds, _ := vm.ReadRegister(7)
	time.Sleep(time.Duration(delay_seconds) * time.Second)
	vm.HostResultCode = OK
	vm.WriteRegister(7, OK)
}

// func (vm *VM) hostSP1Groth16Verify()

func getLogLevelName(level uint64) string {
	switch level {
	case 0:
		return "⛔️ FATAL"
	case 1:
		return "⚠️ WARNING"
	case 2:
		return "ℹ️ INFO"
	case 3:
		return "💁 HELPFUL"
	case 4:
		return "🪡 PEDANTIC"
	default:
		return "UNKNOWN"
	}
}

// JIP-1 https://hackmd.io/@polkadot/jip1
func (vm *VM) hostLog() {

	level, _ := vm.ReadRegister(7)
	target, _ := vm.ReadRegister(8)
	targetlen, _ := vm.ReadRegister(9)
	message, _ := vm.ReadRegister(10)
	messagelen, _ := vm.ReadRegister(11)
	targetBytes, errCode := vm.Ram.ReadRAMBytes(uint32(target), uint32(targetlen))
	if errCode != OK {
		vm.HostResultCode = OOB
		return
	}
	messageBytes, errCode := vm.Ram.ReadRAMBytes(uint32(message), uint32(messagelen))
	if errCode != OK {
		vm.HostResultCode = OOB
		return
	}
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	levelName := getLogLevelName(level) // Assume a function that maps level numbers to log level names.

	// <YYYY-MM-DD hh-mm-ss> <LEVEL>[@<CORE>]?[#<SERVICE_ID>]? [<TARGET>]? <MESSAGE>
	fmt.Printf("[%s] %s [TARGET: %s] %s\n", currentTime, levelName, string(targetBytes), string(messageBytes))
	vm.HostResultCode = OK
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
		fmt.Printf("Register %d: %d\n", i, regs[i])
	}
	return gas, regs, OK
}
