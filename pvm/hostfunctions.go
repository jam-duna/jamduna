package pvm

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"

	"github.com/ethereum/go-ethereum/crypto"
	//"github.com/ethereum/go-ethereum/crypto/secp256k1"
)

// Appendix B - Host function
// Host function indexes
const (
	GAS               = 0
	LOOKUP            = 1
	READ              = 2
	WRITE             = 3
	INFO              = 4
	EMPOWER           = 5
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
	INVOKE            = 21
	EXPUNGE           = 22
	EXTRINSIC         = 23
	PAYLOAD           = 24
	ECRECOVER         = 25
	FOR_TEST          = 105
)

const (
	g = 10
)

// Mapping of host function names to their error cases
var errorCases = map[string][]uint32{
	"Machine":  {OK, OOB},
	"Peek":     {OK, OOB, WHO},
	"Poke":     {OK, OOB, WHO},
	"Invoke":   {OK, OOB, WHO, HOST, FAULT, OOB, PANIC},
	"Expunge":  {OK, WHO},
	"Import":   {OK, OOB, NONE},
	"Export":   {OK, OOB, NONE},
	"New":      {OK, OOB, CASH},
	"Read":     {OK, OOB, NONE},
	"Write":    {OK, OOB, NONE, FULL},
	"Solicit":  {OK, OOB, FULL, HUH},
	"Forget":   {OK, OOB, HUH},
	"Transfer": {OK, WHO, CASH, LOW, HIGH},
}

// Mapping of host function names to their indexes
var hostIndexMap = map[string]int{
	"Gas":               GAS,
	"Lookup":            LOOKUP,
	"Read":              READ,
	"Write":             WRITE,
	"Info":              INFO,
	"Empower":           EMPOWER,
	"Assign":            ASSIGN,
	"Designate":         DESIGNATE,
	"Checkpoint":        CHECKPOINT,
	"New":               NEW,
	"Upgrade":           UPGRADE,
	"Transfer":          TRANSFER,
	"Quit":              QUIT,
	"Solicit":           SOLICIT,
	"Forget":            FORGET,
	"Historical_lookup": HISTORICAL_LOOKUP,
	"Import":            IMPORT,
	"Export":            EXPORT,
	"Machine":           MACHINE,
	"Peek":              PEEK,
	"Poke":              POKE,
	"Invoke":            INVOKE,
	"Expunge":           EXPUNGE,
	"Extrinsic":         EXTRINSIC,
	"Payload":           PAYLOAD,
	"Ecrecover":         ECRECOVER,
	"For_test":          FOR_TEST,
}

// Function to retrieve index and error cases
func GetHostFunctionDetails(name string) (int, []uint32) {
	index, exists := hostIndexMap[name]
	if !exists {
		return 0, nil
	}
	errors, hasErrors := errorCases[name]
	if !hasErrors {
		errors = []uint32{} // No error cases defined
	}
	return index, errors
}

type RefineM struct {
	P []byte `json:"P"`
	U *Page  `json:"U"`
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

	case EMPOWER:
		vm.hostEmpower(vm.X.U)
		return true, nil

	case ASSIGN:
		vm.hostAssign(vm.X.U)
		return true, nil

	case DESIGNATE:
		vm.hostDesignate(vm.X.U)
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

	case INVOKE:
		vm.hostInvoke()
		return true, nil

	case EXPUNGE:
		vm.hostExpunge()
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
func (vm *VM) hostInfo() uint32 {
	//s := vm.X.S
	//d := vm.X.U.D
	omega_7, _ := vm.ReadRegister(7)
	var service uint32
	if omega_7 == (1<<32 - 1) {
		service = vm.Service_index
	} else {
		service = omega_7
	}
	bo, _ := vm.ReadRegister(8)
	t, err := vm.hostenv.GetService(service)
	if err != nil {
		return NONE
	}

	e := []interface{}{t.CodeHash, t.Balance, t.GasLimitG, t.GasLimitM}
	m, err := types.Encode(e)
	if err != nil {
		return NONE
	}
	vm.WriteRAMBytes(bo, m[:])

	return OK
}

// Empower updates
func (vm *VM) hostEmpower(X_U *types.PartialState) uint32 {
	m, _ := vm.ReadRegister(7)
	a, _ := vm.ReadRegister(8)
	v, _ := vm.ReadRegister(9)
	// Set (x'p)_m, (x'p)_a, (x'p)_v
	X_U.PrivilegedState.Kai_m = m
	X_U.PrivilegedState.Kai_a = a
	X_U.PrivilegedState.Kai_v = v
	return OK
}

// Assign Core x_c[i]
func (vm *VM) hostAssign(X_U *types.PartialState) uint32 {
	core, _ := vm.ReadRegister(7)
	if core >= numCores {
		return CORE
	}
	o, _ := vm.ReadRegister(8)
	c, _ := vm.ReadRAMBytes(o, 32*types.MaxAuthorizationQueueItems)
	qi := make([]common.Hash, 32)
	for i := 0; i < 32; i++ {
		qi[i] = common.BytesToHash(c[i:(i + 32)])
	}
	X_U.QueueWorkReport[core] = qi
	return OK
}

// Designate validators
func (vm *VM) hostDesignate(X_U *types.PartialState) uint32 {
	o, _ := vm.ReadRegister(7)
	v, errCode := vm.ReadRAMBytes(o, 176*V)
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
	X_U.UpcomingValidators = qi
	return OK
}

// Checkpoint gets Gas-remaining
func (vm *VM) hostCheckpoint() uint32 {
	vm.Y = vm.X.Clone()
	vm.WriteRegister(7, uint32(vm.Gas))
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
func (vm *VM) hostNew() uint32 {
	xContext := vm.X
	s := xContext.S
	xs, ok := xContext.GetMutableServiceAccount(s, vm.hostenv)
	if !ok {
		fmt.Printf("hostNew: %d not found\n", s)
		return OOB
	}
	// put 'g' and 'm' together
	o, _ := vm.ReadRegister(7)
	c, errCode := vm.ReadRAMBytes(o, 32)
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
	// fmt.Printf("Service %d hostNew %v => %d, code hash: %v\n", s, a.CodeHash, a.GetServiceIndex(), common.BytesToHash(c))
	a.SetServiceIndex(xi)
	a.Balance = a.ComputeThreshold()

	if debug_host {
		fmt.Printf("Service %d hostNew %v => %d\n", s, a.CodeHash, a.GetServiceIndex())
	}
	// Compute footprint & threshold: a_l, a_s, a-t

	xs.Balance = xs.Balance - a.Balance
	if xs.Balance >= xs.ComputeThreshold() {
		//xs has enough balance to fund the creation of a AND covering its own threshold
		// xi' <- check(bump(xi))
		// vm.WriteRegister(7, xi)
		// xContext.I = vm.hostenv.Check(bump(xi)) // this is the next xi
		xContext.I = check(bump(xi), xContext.U.D)

		// I believe this is the same as solicit. where l∶{(c, l)↦[]} need to be set, which will later be provided by E_P
		// a.WriteLookup(common.BytesToHash(c), l, []uint32{common.ComputeCurrenTS()})
		a.WriteLookup(common.BytesToHash(c), l, []uint32{})

		// (x's)b <- (xs)b - at
		// xContext.S = a.ServiceIndex()

		xContext.U.D[xi] = a
		xContext.U.D[s] = xs

		vm.X = xContext
		vm.WriteRegister(7, xi)
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
func (vm *VM) hostUpgrade() uint32 {
	xContext := vm.X
	s := xContext.S
	xs := xContext.U.D[s]
	o, _ := vm.ReadRegister(7)
	gl, _ := vm.ReadRegister(8)
	gh, _ := vm.ReadRegister(9)
	ml, _ := vm.ReadRegister(10)
	mh, _ := vm.ReadRegister(11)
	c, errCode := vm.ReadRAMBytes(o, 32)
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
func (vm *VM) hostTransfer() uint32 {
	d, _ := vm.ReadRegister(7)
	al, _ := vm.ReadRegister(8)
	ah, _ := vm.ReadRegister(9)
	gl, _ := vm.ReadRegister(10)
	gh, _ := vm.ReadRegister(11)
	o, _ := vm.ReadRegister(12)

	a := uint64(ah)<<32 + uint64(al)
	g := uint64(gh)<<32 + uint64(gl)
	// TODO check d -- WHO; check g -- LOW, HIGH
	m, errCode := vm.ReadRAMBytes(o, M)
	if errCode == OOB {
		return OOB
	}
	t := types.DeferredTransfer{Amount: a, GasLimit: g, SenderIndex: vm.X.S, ReceiverIndex: d}
	copy(t.Memo[:], m[:])
	vm.X.T = append(vm.X.T, t)
	return OK
}

// Gas Service
func (vm *VM) hostGas() uint32 {
	vm.WriteRegister(7, uint32(vm.Gas))
	return OK
}

// Quit Service
func (vm *VM) hostQuit() uint32 {
	/*
		d, _ := vm.ReadRegister(0)
		s := vm.hostenv.GetService(vm.S)
		a := s.Balance + types.BaseServiceBalance // TODO: (x_s)_t
		var transferMemo *TransferMemo
		if d == vm.S || d == 0xFFFFFFFF {
			transferMemo = nil
		} else {
			o, _ := vm.ReadRegister(1)
			transferMemo := types.TransferMemoFromBytes(transferMemoBytes)
			transferBytes, _ := vm.ReadRAMBytes(o, types.TransferMemoSize)
		}
		g := vm.ξ
	*/
	// return WHO
	// return LOW
	return OK
}
func (vm *VM) setGasRegister(gasBytes, registerBytes []byte) {
	// TODO
}

// Invoke
func (vm *VM) hostInvoke() uint32 {
	n, _ := vm.ReadRegister(7)
	o, _ := vm.ReadRegister(8)
	gasBytes, _ := vm.ReadRAMBytes(o, 8)
	registerBytes, _ := vm.ReadRAMBytes(o, 8+13*4)
	m, ok := vm.GetVM(n) // hostenv.
	if !ok {
		return WHO
	}
	m.setGasRegister(gasBytes, registerBytes)
	m.Execute(types.EntryPointGeneric)
	// TODO: HOST, FAULT, PANIC
	return HALT
}

// Lookup preimage
func (vm *VM) hostLookup() uint32 {
	s, _ := vm.ReadRegister(7)
	ho, _ := vm.ReadRegister(8)
	bo, _ := vm.ReadRegister(9)
	bz, _ := vm.ReadRegister(10)
	k_bytes, err_k := vm.ReadRAMBytes(ho, 32)
	if err_k == OOB {
		vm.WriteRegister(0, OOB)
		return OOB
	}
	h := common.Blake2Hash(k_bytes)
	v := vm.hostenv.ReadServicePreimageBlob(s, h)

	l := uint32(len(v))
	if bz < l {
		l = bz
	}
	vm.WriteRAMBytes(bo, v[:l])

	if len(h) != 0 {
		vm.WriteRegister(7, l)
		return l
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

// Read Storage
func (vm *VM) hostRead() uint32 {
	var s uint32
	var d map[uint32]*types.ServiceAccount
	var service *types.ServiceAccount
	if vm.X == nil {
		s = vm.Service_index
		d = vm.Delta
		service = vm.ServiceAccount
	} else {
		s = vm.X.S
		d = vm.X.U.D
		service = d[s]
	}
	// Assume that all ram can be read and written
	w7, _ := vm.ReadRegister(7)
	ko, _ := vm.ReadRegister(8)
	kz, _ := vm.ReadRegister(9)
	bo, _ := vm.ReadRegister(10)
	bz, _ := vm.ReadRegister(11)
	k, err_k := vm.ReadRAMBytes(ko, int(kz)) // this is the raw key.
	if err_k == OOB {
		fmt.Println("Read RAM Error")
		vm.WriteRegister(7, OOB)
		return OOB
	}

	key := common.Compute_storageKey_internal(s, k)
	var a *types.ServiceAccount
	var ok bool
	if w7 == s || w7 == 0xFFFFFFFF {
		a = service
	} else {
		a, ok = d[w7]
		if !ok {
			vm.WriteRegister(7, OOB)
			return OOB
		}
	}

	var val []byte
	_, val = a.ReadStorage(key, vm.hostenv)
	l := uint32(len(val))
	if bz < l {
		l = bz
	}
	if len(k) != 0 {
		vm.WriteRAMBytes(bo, val[:l])
		vm.WriteRegister(7, l)
		return l
	}
	return OK
}

// Write Storage
func (vm *VM) hostWrite() uint32 {
	xContext := vm.X
	s := xContext.S
	xs, ok := xContext.GetMutableServiceAccount(s, vm.hostenv)
	if !ok {
		fmt.Printf("hostWrite: %d not found\n", s)
		return OOB
	}

	// Assume that all ram can be read and written
	// Got storage of bold S(service account in GP) by setting s = 0, k = k(from RAM)
	ko, _ := vm.ReadRegister(7)
	kz, _ := vm.ReadRegister(8)
	vo, _ := vm.ReadRegister(9)
	vz, _ := vm.ReadRegister(10)
	k, err_k := vm.ReadRAMBytes(ko, int(kz))
	if err_k == OOB {
		vm.WriteRegister(7, OOB)
		return OOB
	}
	key := common.Compute_storageKey_internal(s, k)
	// fmt.Printf("hostWrite s=%d, k=%v (%d) => Key: %v (hash(E_4(s)+k))\n", s, k, len(k), key)

	// C(255, s) ↦ a c ⌢E 8 (a b ,a g ,a m ,a l )⌢E 4 (a i ) ,
	a_t := uint64(0)
	if a_t <= xs.Balance {
		// adjust S
		v := []byte{}
		if vz > 0 {
			v, _ = vm.ReadRAMBytes(vo, int(vz))
		}
		xs.WriteStorage(key, v)
		vm.WriteRegister(7, uint32(len(v)))
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
func (vm *VM) hostSolicit(t uint32) uint32 {
	xContext := vm.X
	s := xContext.S
	d := xContext.U.D
	xs := d[s]
	// Got l of X_s by setting s = 1, z = z(from RAM)
	o, _ := vm.ReadRegister(7)
	z, _ := vm.ReadRegister(8)              // z: blob_len
	hBytes, err_h := vm.ReadRAMBytes(o, 32) // h: blobHash
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
	X_s_l := vm.hostenv.ReadServicePreimageLookup(xs.GetServiceIndex(), blobHash, z)
	fmt.Printf("xs.ServiceIndex() = %d, blobHash = %v, z = %d, X_s_l = %v\n", xs.GetServiceIndex(), blobHash, z, X_s_l)
	if len(X_s_l) == 0 {
		// when preimagehash is not found, put it into solicit request - so we can ask other DAs
		xs.WriteLookup(blobHash, z, []uint32{})
		vm.WriteRegister(7, OK)
		return OK
	} else if X_s_l[0] == 2 { // [x, y]
		xs.WriteLookup(blobHash, z, append(X_s_l, []uint32{t}...))
		vm.WriteRegister(7, OK)
		return OK
	} else {
		vm.WriteRegister(7, HUH)
		return HUH
	}
}

// Forget preimage
func (vm *VM) hostForget(t uint32) uint32 {

	s := uint32(1) // s: serviceIndex
	o, _ := vm.ReadRegister(7)
	z, _ := vm.ReadRegister(8)
	hBytes, errCode := vm.ReadRAMBytes(o, 32)
	if errCode == OOB {
		fmt.Println("Read RAM Error")
		return OOB
	}

	blobHash := common.Hash(hBytes)
	X_s_l := vm.hostenv.ReadServicePreimageLookup(s, blobHash, z)
	if len(X_s_l) == 0 || (len(X_s_l) == 2 && X_s_l[1] < (t-D)) {
		vm.hostenv.DeleteServicePreimageLookupKey(s, blobHash, z)
		vm.hostenv.DeleteServicePreimageKey(s, blobHash)
		vm.WriteRegister(7, OK)
		return OK
	} else if len(X_s_l) == 1 {
		vm.hostenv.WriteServicePreimageLookup(s, blobHash, z, append(X_s_l, []uint32{t}...))
		vm.WriteRegister(7, OK)
		return OK
	} else if len(X_s_l) == 3 && X_s_l[1] < (t-D) {
		X_s_l[2] = uint32(t)
		vm.hostenv.WriteServicePreimageLookup(s, blobHash, z, X_s_l)
		vm.WriteRegister(7, OK)
		return OK
	} else {
		vm.WriteRegister(7, HUH)
		return HUH
	}
}

// HistoricalLookup determines whether the preimage of some hash h was available for lookup by some service account a at some timeslot t, and if so, provide its preimage
func (vm *VM) hostHistoricalLookup(t uint32) uint32 {
	// take a service s (from \omega_0 ),
	s := vm.Service_index
	ho, _ := vm.ReadRegister(8)
	bo, _ := vm.ReadRegister(9)
	bz, _ := vm.ReadRegister(10)

	hBytes, errCode := vm.ReadRAMBytes(ho, 32)
	if errCode == OOB {
		fmt.Println("Read RAM Error")
		vm.WriteRegister(0, OOB)
		return errCode
	}
	h := common.Hash(hBytes)
	fmt.Println(h)
	v := vm.hostenv.HistoricalLookup(s, t, h)
	vLength := uint32(len(v))
	if vLength == 0 {
		vm.WriteRegister(7, NONE)
		return NONE
	}

	if vLength != 0 {
		l := vLength
		if bz < l {
			l = bz
		}
		vm.WriteRAMBytes(bo, v[:l])
		vm.WriteRegister(7, vLength)
		fmt.Println(v[:l])
		return vLength
	} else {
		vm.WriteRegister(7, NONE)
		return NONE
	}
}

// Import Segment
func (vm *VM) hostImport() uint32 {
	// import  - which copies  a specific i  (e.g. holding the bytes "9") into RAM from "ImportDA" to be "accumulated"
	omega_0, _ := vm.ReadRegister(7) // a0 = 7
	var v_Bytes []byte
	if omega_0 < uint32(len(vm.Imports)) {
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
		errCode := vm.WriteRAMBytes(o, v_Bytes[:])
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

// ECRecover
func (vm *VM) hostECRecover() uint32 {
	o, _ := vm.ReadRegister(7)
	ho, _ := vm.ReadRegister(8)
	sig, _ := vm.ReadRAMBytes(o, 65)
	h, _ := vm.ReadRAMBytes(ho, 32)
	recoveredPubKey, err := crypto.SigToPub(h, sig)
	if err != nil {
		vm.WriteRegister(7, NONE)
	}
	recoveredPubKeyBytes := crypto.FromECDSAPub(recoveredPubKey)
	p, _ := vm.ReadRegister(9)
	vm.WriteRAMBytes(p, recoveredPubKeyBytes[:])

	vm.WriteRegister(7, OK)
	return OK

}

// Export segment host-call
func (vm *VM) hostExport(pi uint32) (uint32, [][]byte) {
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

	x, errCode := vm.ReadRAMBytes(p, int(z))
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
		vm.WriteRegister(7, ς+uint32(len(e)))
		e = append(e, x)
		vm.Exports = e
		// errCode = vm.hostenv.ExportSegment(x)
		return OK, e
	}
}

// Not sure but this is running a machine at instruction i? using bytes from po to pz
func (vm *VM) hostMachine() uint32 {
	po, _ := vm.ReadRegister(7)
	pz, _ := vm.ReadRegister(8)
	i, _ := vm.ReadRegister(9)

	p, errCode := vm.ReadRAMBytes(po, int(pz))
	if errCode != OK {
		return errCode
	}
	// need service account here??
	serviceAcct := uint32(0)
	n := vm.CreateVM(serviceAcct, p, i)
	return n
}

func (vm *VM) hostPeek() uint32 {
	n, _ := vm.ReadRegister(7)
	a, _ := vm.ReadRegister(8)
	b, _ := vm.ReadRegister(9)
	l, _ := vm.ReadRegister(10)
	m, ok := vm.GetVM(n) // hostenv.
	if !ok {
		return WHO
	}
	// read l bytes from m
	s, errCode := m.ReadRAMBytes(b, int(l))
	if errCode == OOB {
		return errCode
	}
	// write l bytes to vm
	errCode = vm.WriteRAMBytes(a, s[:])
	if errCode == OOB {
		return errCode
	}
	return OK
}

func (vm *VM) hostPoke() uint32 {
	n, _ := vm.ReadRegister(7)
	a, _ := vm.ReadRegister(8)
	b, _ := vm.ReadRegister(9)
	l, _ := vm.ReadRegister(10)
	m, ok := vm.GetVM(n) // hostenv.
	if !ok {
		return WHO
	}
	s, errCode := m.ReadRAMBytes(a, int(l))
	if errCode == OOB {
		return errCode
	}
	errCode = m.WriteRAMBytes(b, s[:])
	if errCode == OOB {
		return errCode
	}
	return OK
}

func (vm *VM) hostExpunge() uint32 {
	n, _ := vm.ReadRegister(7)
	if vm.ExpungeVM(n) {
		return OK
	}
	return WHO
}
