package pvm

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/sp1"
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
	QUIT              = 12
	SOLICIT           = 13
	FORGET            = 14
	HISTORICAL_LOOKUP = 15
	IMPORT            = 16
	EXPORT            = 17
	MACHINE           = 18
	PEEK              = 19
	POKE              = 20
	ZERO              = 21
	VOID              = 22
	INVOKE            = 23
	EXPUNGE           = 24
	LOG               = 100
	SP1GROTH16VERIFY  = 101
	FOR_TEST          = 105
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
	"Gas":              GAS,
	"Lookup":           LOOKUP,
	"Read":             READ,
	"Write":            WRITE,
	"Info":             INFO,
	"Sp1Groth16Verify": SP1GROTH16VERIFY,
	// B.7 Accumulate Functions
	"Bless":      BLESS,
	"Assign":     ASSIGN,
	"Designate":  DESIGNATE,
	"Checkpoint": CHECKPOINT,
	"New":        NEW,
	"Upgrade":    UPGRADE,
	"Transfer":   TRANSFER,
	"Quit":       QUIT,
	"Solicit":    SOLICIT,
	"Forget":     FORGET,
	// B.8 Refine Functions
	"Historical_lookup": HISTORICAL_LOOKUP,
	"Import":            IMPORT,
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
	I uint32 `json:"I"`
}

type RefineM_map map[uint32]*RefineM

// GP-0.5 B.5
type Refine_parameters struct {
	Gas                  uint64
	Ram                  *RAM
	Register             []uint32
	Machine              RefineM_map
	Export_segment       [][]byte
	Import_segement      [][]byte
	Export_segment_index uint32
	service_index        uint32
	Delta                map[uint32]uint32
	C_t                  uint32
}

// false byte
func falseBytes(data []byte) []byte {
	result := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		result[i] = 0xFF - data[i]
		// result[i] = ^data[i]
	}
	return result
}

// InvokeHostCall handles host calls
// Returns true if the call results in a halt condition, otherwise false
func (vm *VM) InvokeHostCall(host_fn int) (bool, error) {
	if debug_pvm {
		fmt.Printf("vm.host_fn=%v\n", vm.host_func_id) //Do you need operand here?
	}
	vm.Gas = vm.Gas - g
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

	case QUIT:
		vm.hostQuit()
		return true, nil

	case SOLICIT:
		t := vm.hostenv.GetTimeslot()
		vm.hostSolicit(t)
		return true, nil

	case FORGET:
		t := vm.hostenv.GetTimeslot()
		vm.hostForget(t)
		return true, nil

	// Refine functions
	case HISTORICAL_LOOKUP:
		vm.hostHistoricalLookup(0)
		return true, nil

	case IMPORT:
		vm.hostImport()
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

	case SP1GROTH16VERIFY:
		vm.hostSP1Groth16Verify()
		return true, nil

	default:
		vm.Gas = vm.Gas + g
		return false, fmt.Errorf("unknown host call: %d\n", host_fn)
	}
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// Information-on-Service
func (vm *VM) hostInfo() uint64 {
	omega_7, _ := vm.ReadRegister(7)

	t, errCode := vm.getXUDS(omega_7)
	if errCode != OK {
		return errCode
	}
	bo, _ := vm.ReadRegister(8)

	e := []interface{}{t.CodeHash, t.Balance, t.GasLimitG, t.GasLimitM}
	m, err := types.Encode(e)
	if err != nil {
		return NONE
	}
	vm.Ram.WriteRAMBytes(uint32(bo), m[:])

	return OK
}

// Bless updates
func (vm *VM) hostBless() uint64 {
	m, _ := vm.ReadRegister(7)
	a, _ := vm.ReadRegister(8)
	v, _ := vm.ReadRegister(9)
	// Set (x'p)_m, (x'p)_a, (x'p)_v
	vm.X.U.PrivilegedState.Kai_m = uint32(m)
	vm.X.U.PrivilegedState.Kai_a = uint32(a)
	vm.X.U.PrivilegedState.Kai_v = uint32(v)
	return OK
}

// Assign Core x_c[i]
func (vm *VM) hostAssign() uint64 {
	core, _ := vm.ReadRegister(7)
	if core >= numCores {
		return CORE
	}
	o, _ := vm.ReadRegister(8)
	c, _ := vm.Ram.ReadRAMBytes(uint32(o), 32*types.MaxAuthorizationQueueItems)
	qi := make([]common.Hash, 32)
	for i := 0; i < 32; i++ {
		qi[i] = common.BytesToHash(c[i:(i + 32)])
	}
	copy(vm.X.U.QueueWorkReport[core][:], qi[:])
	return OK
}

// Designate validators
func (vm *VM) hostDesignate() uint64 {
	o, _ := vm.ReadRegister(7)
	v, errCode := vm.Ram.ReadRAMBytes(uint32(o), 176*V)
	if errCode == OOB {
		return OOB
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
	return OK
}

// Checkpoint gets Gas-remaining
func (vm *VM) hostCheckpoint() uint32 {
	vm.Y = vm.X.Clone()
	vm.WriteRegister(7, uint64(vm.Gas)) // CHECK
	return OK
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
func (vm *VM) hostNew() uint64 {
	xContext := vm.X
	xs, _ := xContext.GetX_s()

	// put 'g' and 'm' together
	o, _ := vm.ReadRegister(7)
	c, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		return errCode
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
		// a.WriteLookup(common.BytesToHash(c), l, []uint32{common.ComputeCurrenTS()})
		a.WriteLookup(common.BytesToHash(c), uint32(l), []uint32{}) // *** CHECK

		// (x's)b <- (xs)b - at
		// xContext.S = a.ServiceIndex()

		// Here we are adding the new service account to the map
		xContext.U.D[xi] = a

		vm.X = xContext
		vm.WriteRegister(7, uint64(xi))
		return OK
	} else {
		if debug_host {
			fmt.Println("Balance insufficient")
		}
		vm.WriteRegister(7, CASH)
		return CASH //balance insufficient
	}
}

// Upgrade service
func (vm *VM) hostUpgrade() uint64 {
	xContext := vm.X
	xs, s := xContext.GetX_s()
	o, _ := vm.ReadRegister(7)
	gl, _ := vm.ReadRegister(8)
	gh, _ := vm.ReadRegister(9)
	ml, _ := vm.ReadRegister(10)
	mh, _ := vm.ReadRegister(11)
	c, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode == OOB {
		return errCode
	}
	g := uint64(gh)<<32 + uint64(gl)
	m := uint64(mh)<<32 + uint64(ml)

	xs.Dirty = true
	xs.CodeHash = common.BytesToHash(c)
	xs.GasLimitG = g
	xs.GasLimitM = m
	xContext.D[s] = xs
	return OK
}

// Transfer host call
func (vm *VM) hostTransfer() uint64 {
	d, _ := vm.ReadRegister(7)
	a, _ := vm.ReadRegister(8)
	g, _ := vm.ReadRegister(9)
	o, _ := vm.ReadRegister(10)

	// TODO check d -- WHO; check g -- LOW, HIGH
	m, errCode := vm.Ram.ReadRAMBytes(uint32(o), M)
	if errCode == OOB {
		return OOB
	}
	t := types.DeferredTransfer{Amount: a, GasLimit: g, SenderIndex: vm.X.S, ReceiverIndex: uint32(d)} // CHECK
	copy(t.Memo[:], m[:])
	vm.X.T = append(vm.X.T, t)
	return OK
}

// Gas Service
func (vm *VM) hostGas() uint32 {
	vm.WriteRegister(7, uint64(vm.Gas))
	return OK
}

// Quit Service
func (vm *VM) hostQuit() uint32 {
	/*	d, _ := vm.ReadRegister(7)
		o, _ := vm.ReadRegister(8)
		xs, s := vm.X.GetX_s()
		a := xs.Balance - xs.StorageSize + types.BaseServiceBalance // TODO: (x_s)_t
		var transferMemo *TransferMemo
		if d == vm.X.S || d == maxUint64 {
			transferMemo = nil
		} else {
			o, _ := vm.ReadRegister(1)
			transferMemo, _ := types.TransferMemoFromBytes(transferMemoBytes)
			transferBytes, _ := vm.ReadRAMBytes(o, types.TransferMemoSize)
		}
		g := vm.ξ
	*/
	return OK
}
func (vm *VM) setGasRegister(gasBytes, registerBytes []byte) {

	// gas todo
	registers := make([]uint64, 13)
	for i := 0; i < 13; i++ {
		registers[i] = binary.LittleEndian.Uint64(registerBytes[i*8 : (i+1)*8])
	}
	vm.register = registers
}

// Invoke
func (vm *VM) hostInvoke() uint64 {
	n, _ := vm.ReadRegister(7)
	o, _ := vm.ReadRegister(8)
	gasBytes, errCodeGas := vm.Ram.ReadRAMBytes(uint32(o), 8)
	if errCodeGas != OK {
		vm.WriteRegister(7, OOB)
		return OOB
	}
	// gas := binary.LittleEndian.Uint32(gasBytes)
	m_n_reg := make([]uint32, 13)
	for i := 0; i < 13; i++ {
		reg_bytes, errCodeReg := vm.Ram.ReadRAMBytes(uint32(o)+8+4*uint32(i), 4)
		if errCodeReg != OK {
			vm.WriteRegister(7, OOB)
			return OOB
		}
		m_n_reg[i] = binary.LittleEndian.Uint32(reg_bytes)
	}
	// intialize invoke

	registerBytes, _ := vm.Ram.ReadRAMBytes(uint32(o)+8, 13*4)
	m, ok := vm.GetVM(uint32(n))
	if !ok {
		return WHO
	}
	m.setGasRegister(gasBytes, registerBytes)
	m.Execute(5)
	// put the register and gas back to the memory
	gas := binary.LittleEndian.Uint64(gasBytes)
	vm.PutGasAndRegistersToMemory(uint32(o), gas, m.register)
	// TODO: HOST, FAULT, PANIC
	return HALT
}

// Lookup preimage
func (vm *VM) hostLookup() uint64 {
	s, _ := vm.ReadRegister(7)
	ho, _ := vm.ReadRegister(8)
	bo, _ := vm.ReadRegister(9)
	bz, _ := vm.ReadRegister(10)
	k_bytes, err_k := vm.Ram.ReadRAMBytes(uint32(ho), 32)
	if err_k == OOB {
		vm.WriteRegister(0, OOB)
		return OOB
	}
	h := common.Blake2Hash(k_bytes)
	a, errCode := vm.getXUDS(s)
	if errCode != OK {
		return NONE
	}
	ok, v := a.ReadPreimage(h, vm.hostenv)
	if !ok {
		return NONE
	}
	l := uint64(len(v))
	if bz < l {
		l = bz
	}
	vm.Ram.WriteRAMBytes(uint32(bo), v[:l])

	if len(h) != 0 {
		vm.WriteRegister(7, l)
		return uint64(l)
	} else {
		vm.WriteRegister(7, OOB)
		return OOB
	}
}

func uint32ToBytes(s uint32) []byte {
	sbytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sbytes, s)
	return sbytes
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
	a, ok = vm.X.D[s]
	if !ok {
		a, ok, err = vm.hostenv.GetService(s)
		if err != nil || !ok {
			return nil, NONE
		}
		vm.X.D[s] = a
	}
	return a, OK
}

// Read Storage
func (vm *VM) hostRead() uint64 {
	// Assume that all ram can be read and written
	w7, _ := vm.ReadRegister(7)
	ko, _ := vm.ReadRegister(8)
	kz, _ := vm.ReadRegister(9)
	bo, _ := vm.ReadRegister(10)
	bz, _ := vm.ReadRegister(11)
	k, err_k := vm.Ram.ReadRAMBytes(uint32(ko), uint32(kz)) // this is the raw key.
	if err_k == OOB {
		fmt.Println("Read RAM Error")
		vm.WriteRegister(7, OOB)
		return OOB
	}
	// k for original raw key
	a, errCode := vm.getXUDS(w7)
	if errCode != OK {
		vm.WriteRegister(7, OOB)
		return OOB
	}

	var val []byte
	_, val = a.ReadStorage(k, vm.hostenv)
	l := uint64(len(val))
	if bz < l {
		l = bz
	}
	if len(k) != 0 {
		vm.Ram.WriteRAMBytes(uint32(bo), val[:l])
		vm.WriteRegister(7, l)
		return l
	}
	return OK
}

// Write Storage
func (vm *VM) hostWrite() uint64 {
	xContext := vm.X
	xs, s := xContext.GetX_s()

	// Assume that all ram can be read and written
	// Got storage of bold S(service account in GP) by setting s = 0, k = k(from RAM)
	ko, _ := vm.ReadRegister(7)
	kz, _ := vm.ReadRegister(8)
	vo, _ := vm.ReadRegister(9)
	vz, _ := vm.ReadRegister(10)
	k, err_k := vm.Ram.ReadRAMBytes(uint32(ko), uint32(kz))
	if err_k == OOB {
		vm.WriteRegister(7, OOB)
		return OOB
	}

	// fmt.Printf("hostWrite s=%d, k=%v (%d) => Key: %v (hash(E_4(s)+k))\n", s, k, len(k), key)

	// C(255, s) ↦ a c ⌢E 8 (a b ,a g ,a m ,a l )⌢E 4 (a i ) ,
	a_t := uint64(0)
	if a_t <= xs.Balance {
		// adjust S
		v := []byte{}
		if vz > 0 {
			v, _ = vm.Ram.ReadRAMBytes(uint32(vo), uint32(vz))
		}

		if Debug_Service_Storage {
			fmt.Printf("ServiceAccount.WriteStorage: s=%d xs.ServiceIndex=%d\n", s, xs.ServiceIndex)
		}

		xs.WriteStorage(s, k, v)
		vm.WriteRegister(7, uint64(len(v)))
		//fmt.Printf("hostwrite: WriteStorage(%d, %v => %v) len(v)=%d Dirty: %v\n", s, key, v, len(v), xs.Dirty)
		return 0
	} else {
		vm.WriteRegister(7, FULL)
		return FULL
	}
}

func (vm *VM) GetJCETime() uint32 {
	// timeslot mark

	// return common.ComputeCurrentJCETime()
	return common.ComputeTimeUnit(types.TimeUnitMode)
}

// Solicit preimage
func (vm *VM) hostSolicit(t uint32) uint64 {
	xs, _ := vm.X.GetX_s()
	// Got l of X_s by setting s = 1, z = z(from RAM)
	o, _ := vm.ReadRegister(7)
	z, _ := vm.ReadRegister(8)                          // z: blob_len
	hBytes, err_h := vm.Ram.ReadRAMBytes(uint32(o), 32) // h: blobHash
	if err_h == OOB {
		fmt.Println("Read RAM Error")
		vm.WriteRegister(7, OOB)
		return OOB
	}

	// check balance a_t <= a_b
	/* TODO: william to figure out proper struct
	service_data := vm.hostenv.ReadServiceBytes(s) // read service account(X_s) where service index = 1
	if len(service_data) > 0 {
		a_b_byte := service_data[len(service_data)-36 : len(service_data)-28]
		a_l_byte := service_data[len(service_data)-12 : len(service_data)-4]
		a_i_byte := service_data[len(service_data)-4:]
		a_b := binary.LittleEndian.Uint64(a_b_byte)
		a_l := binary.LittleEndian.Uint64(a_l_byte)
		a_i := binary.LittleEndian.Uint32(a_i_byte)
		a_t := 10 + uint64(1*a_i) + 100*a_l
		if a_b < a_t {
			fmt.Println("a_b < a_t")
			vm.WriteRegister(0, FULL)
			return FULL
		}
	}
	*/
	blobHash := common.Hash(hBytes)
	ok, X_s_l := xs.ReadLookup(blobHash, uint32(z), vm.hostenv)
	if !ok {
		return HUH
	}
	// TODO: FULL
	//fmt.Printf("xs.ServiceIndex() = %d, blobHash = %v, z = %d, X_s_l = %v\n", xs.GetServiceIndex(), blobHash, z, X_s_l)
	if len(X_s_l) == 0 {
		// when preimagehash is not found, put it into solicit request - so we can ask other DAs
		xs.WriteLookup(blobHash, uint32(z), []uint32{})
		vm.WriteRegister(7, OK)
		return OK
	} else if X_s_l[0] == 2 { // [x, y]
		xs.WriteLookup(blobHash, uint32(z), append(X_s_l, []uint32{t}...))
		vm.WriteRegister(7, OK)
		return OK
	} else {
		vm.WriteRegister(7, HUH)
		return HUH
	}
}

// Forget preimage
func (vm *VM) hostForget(t uint32) uint64 {
	x_s, _ := vm.X.GetX_s()
	o, _ := vm.ReadRegister(7)
	z, _ := vm.ReadRegister(8)
	hBytes, errCode := vm.Ram.ReadRAMBytes(uint32(o), 32)
	if errCode == OOB {
		fmt.Println("Read RAM Error")
		return OOB
	}

	blobHash := common.Hash(hBytes)
	ok, X_s_l := x_s.ReadLookup(blobHash, uint32(z), vm.hostenv)
	if !ok {
		return HUH
	}
	if len(X_s_l) == 0 || (len(X_s_l) == 2 && X_s_l[1] < (t-D)) {
		x_s.WriteLookup(blobHash, uint32(z), []uint32{})
		vm.WriteRegister(7, OK)
		return OK
	} else if len(X_s_l) == 1 {
		x_s.WriteLookup(blobHash, uint32(z), append(X_s_l, []uint32{t}...))
		vm.WriteRegister(7, OK)
		return OK
	} else if len(X_s_l) == 3 && X_s_l[1] < (t-D) {
		X_s_l[2] = uint32(t)
		x_s.WriteLookup(blobHash, uint32(z), X_s_l)
		vm.WriteRegister(7, OK)
		return OK
	} else {
		vm.WriteRegister(7, HUH)
		return HUH
	}
}

// HistoricalLookup determines whether the preimage of some hash h was available for lookup by some service account a at some timeslot t, and if so, provide its preimage
func (vm *VM) hostHistoricalLookup(t uint32) uint64 {
	// take a service s (from \omega_0 ),
	s := vm.Service_index
	ho, _ := vm.ReadRegister(8)
	bo, _ := vm.ReadRegister(9)
	bz, _ := vm.ReadRegister(10)

	hBytes, errCode := vm.Ram.ReadRAMBytes(uint32(ho), 32)
	if errCode == OOB {
		fmt.Println("Read RAM Error")
		vm.WriteRegister(0, OOB)
		return errCode
	}
	h := common.Hash(hBytes)
	fmt.Println(h)
	v := vm.hostenv.HistoricalLookup(s, t, h)
	vLength := uint64(len(v))
	if vLength == 0 {
		vm.WriteRegister(7, NONE)
		return NONE
	}

	if vLength != 0 {
		l := uint64(vLength)
		if bz < l {
			l = bz
		}
		vm.Ram.WriteRAMBytes(uint32(bo), v[:l])
		vm.WriteRegister(7, vLength)
		fmt.Println(v[:l])
		return vLength
	} else {
		vm.WriteRegister(7, NONE)
		return NONE
	}
}

// Import Segment
func (vm *VM) hostImport() uint64 {
	// import  - which copies  a specific i  (e.g. holding the bytes "9") into RAM from "ImportDA" to be "accumulated"
	omega_0, _ := vm.ReadRegister(7) // a0 = 7
	var v_Bytes []byte
	if omega_0 < uint64(len(vm.Imports)) {
		v_Bytes = vm.Imports[omega_0][:]
	} else {
		v_Bytes = []byte{}
	}
	o, _ := vm.ReadRegister(8) // a1 = 8
	l, _ := vm.ReadRegister(9) // a2 = 9
	if l > (W_E * W_S) {
		l = W_E * W_S
	}

	if len(v_Bytes) != 0 {
		errCode := vm.Ram.WriteRAMBytes(uint32(o), v_Bytes[:])
		if errCode != OK {
			vm.WriteRegister(7, OOB)
			return errCode
		}
		if debug_pvm {
			fmt.Printf("Write RAM Bytes: %v\n", v_Bytes[:])
		}
		vm.WriteRegister(7, OK)
		return OK
	} else {
		vm.WriteRegister(7, NONE)
		return NONE
	}
}

// Export segment host-call
func (vm *VM) hostExport(pi uint32) (uint64, [][]byte) {
	/*
		need to get
			ς
		properly
	*/
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
		return OOB, e
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
		return FULL, e
	} else {
		vm.WriteRegister(7, uint64(ς)+uint64(len(e)))
		e = append(e, x)
		vm.Exports = e
		// errCode = vm.hostenv.ExportSegment(x)
		return OK, e
	}
}

// Not sure but this is running a machine at instruction i? using bytes from po to pz
func (vm *VM) hostMachine() uint64 {
	po, _ := vm.ReadRegister(7)
	pz, _ := vm.ReadRegister(8)
	i, _ := vm.ReadRegister(9)

	p, errCode := vm.Ram.ReadRAMBytes(uint32(po), uint32(pz))
	if errCode != OK {
		vm.WriteRegister(7, uint64(errCode))
		return errCode
	}

	// need service account here??
	serviceAcct := uint32(0)
	n := vm.CreateVM(serviceAcct, p, uint32(i))
	vm.WriteRegister(7, uint64(n))
	return uint64(n)
}

func (vm *VM) hostPeek() uint64 {
	n, _ := vm.ReadRegister(7)
	o, _ := vm.ReadRegister(8)
	s, _ := vm.ReadRegister(9)
	z, _ := vm.ReadRegister(10)
	m_n, ok := vm.GetVM(uint32(n))
	if !ok {
		vm.WriteRegister(7, WHO)
		return WHO
	}
	// read l bytes from m
	s_data, errCode := m_n.Ram.ReadRAMBytes(uint32(s), uint32(z))
	if errCode == OOB {
		vm.WriteRegister(7, uint64(errCode))
		return errCode
	}
	// write l bytes to vm
	errCode = vm.Ram.WriteRAMBytes(uint32(o), s_data[:])
	if errCode == OOB {
		vm.WriteRegister(7, OOB)
		return errCode
	}
	vm.WriteRegister(7, OK)
	return OK
}

func (vm *VM) hostPoke() uint64 {
	n, _ := vm.ReadRegister(7)
	o, _ := vm.ReadRegister(8)
	s, _ := vm.ReadRegister(9)
	z, _ := vm.ReadRegister(10)
	m_n, ok := vm.GetVM(uint32(n))
	fmt.Printf("ok? %v\n", ok)
	if !ok {
		vm.WriteRegister(7, WHO)
		return WHO
	}
	// read data from original vm
	s_data, errCode := vm.Ram.ReadRAMBytes(uint32(s), uint32(z))
	fmt.Printf("ok? %v\n", errCode)
	if errCode == OOB {
		vm.WriteRegister(7, uint64(errCode))
		return errCode
	}
	// write data to m_n
	errCode = m_n.Ram.WriteRAMBytes(uint32(o), s_data[:])
	if errCode == OOB {
		vm.WriteRegister(7, OOB)
		return errCode
	}
	fmt.Printf("ok? %v\n", errCode)
	vm.WriteRegister(7, OK)
	return OK
}

func (vm *VM) hostExpunge() uint64 {
	n, _ := vm.ReadRegister(7)
	vm.WriteRegister(7, uint64(vm.VMs[uint32(n)].pc))
	if vm.ExpungeVM(uint32(n)) {
		return OK
	}
	vm.WriteRegister(7, WHO)
	return WHO
}

func (vm *VM) hostVoid() uint64 {
	n, _ := vm.ReadRegister(7)
	p, _ := vm.ReadRegister(8)
	c, _ := vm.ReadRegister(9)

	m, ok := vm.GetVM(uint32(n))
	if !ok {
		return WHO
	}
	for i := uint32(0); i < uint32(c); i++ {
		page, err := m.Ram.getOrAllocatePage(uint32(p) + i)
		if err != nil {
			return OOB // TODO
		}
		page.void()
	}
	// TODO
	access_mode := AccessMode{Inaccessible: true, Writable: false, Readable: false}
	m.Ram.SetPageAccess(uint32(p), uint32(c), access_mode)
	return WHO
}

func (vm *VM) hostZero() uint64 {
	n, _ := vm.ReadRegister(7)
	p, _ := vm.ReadRegister(8)
	c, _ := vm.ReadRegister(9)

	m, ok := vm.GetVM(uint32(n))
	if !ok {
		return WHO
	}

	for i := uint32(0); i < uint32(c); i++ {
		page, err := m.Ram.getOrAllocatePage(uint32(p) + uint32(i))
		if err != nil {
			return OOB // TODO
		}
		page.zero()
	}
	// TODO
	access_mode := AccessMode{Inaccessible: false, Writable: true, Readable: true}
	m.Ram.SetPageAccess(uint32(p), uint32(c), access_mode)
	return WHO
}

func (vm *VM) hostSP1Groth16Verify() uint64 {
	proof, _ := vm.ReadRegister(7)
	proof_length, _ := vm.ReadRegister(8)
	verifierkey, _ := vm.ReadRegister(9)
	verifierkey_length, _ := vm.ReadRegister(10)
	public, _ := vm.ReadRegister(11)
	public_length, _ := vm.ReadRegister(12)

	proofBytes, errCode := vm.Ram.ReadRAMBytes(uint32(proof), uint32(proof_length))
	if errCode != OK {
		return OOB
	}
	verifierBytes, errCode := vm.Ram.ReadRAMBytes(uint32(verifierkey), uint32(verifierkey_length))
	if errCode != OK {
		return OOB
	}
	pubBytes, errCode := vm.Ram.ReadRAMBytes(uint32(public), uint32(public_length))
	if errCode != OK {
		return OOB
	}

	verified := sp1.VerifyGroth16(proofBytes, string(verifierBytes), pubBytes)
	if verified {
		return OK
	}
	return HUH
}

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
func (vm *VM) hostLog() uint64 {

	level, _ := vm.ReadRegister(7)
	target, _ := vm.ReadRegister(8)
	targetlen, _ := vm.ReadRegister(9)
	message, _ := vm.ReadRegister(10)
	messagelen, _ := vm.ReadRegister(11)
	targetBytes, errCode := vm.Ram.ReadRAMBytes(uint32(target), uint32(targetlen))
	if errCode != OK {
		return errCode
	}
	messageBytes, errCode := vm.Ram.ReadRAMBytes(uint32(message), uint32(messagelen))
	if errCode != OK {
		return errCode
	}
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	levelName := getLogLevelName(level) // Assume a function that maps level numbers to log level names.

	// <YYYY-MM-DD hh-mm-ss> <LEVEL>[@<CORE>]?[#<SERVICE_ID>]? [<TARGET>]? <MESSAGE>
	fmt.Printf("[%s] %s [TARGET: %s] %s\n", currentTime, levelName, string(targetBytes), string(messageBytes))
	return OK
}

func (vm *VM) PutGasAndRegistersToMemory(input_address uint32, gas uint64, regs []uint64) (errCode uint64) {
	gasBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(gasBytes, gas)
	errCode = vm.Ram.WriteRAMBytes(input_address, gasBytes)
	if errCode != OK {
		return errCode
	}
	for i, reg := range regs {
		regBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(regBytes, reg)
		errCode = vm.Ram.WriteRAMBytes(input_address+8+uint32(i*4), regBytes)
		if errCode != OK {
			return errCode
		}
	}
	return OK
}

func (vm *VM) GetGasAndRegistersFromMemory(input_address uint32) (gas uint64, regs []uint64, errCode uint64) {
	gasBytes, errCode := vm.Ram.ReadRAMBytes(input_address, 8)
	if errCode != OK {
		return 0, nil, errCode
	}
	gas = binary.LittleEndian.Uint64(gasBytes)
	regs = make([]uint64, 13)
	for i := 0; i < 13; i++ {
		regBytes, errCode := vm.Ram.ReadRAMBytes(input_address+8+uint32(i*4), 4)
		if errCode != OK {
			return 0, nil, errCode
		}
		regs[i] = binary.LittleEndian.Uint64(regBytes)
	}
	return gas, regs, OK
}
