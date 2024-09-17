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
)

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
	fmt.Printf("vm.host_fn=%v\n", vm.host_func_id) //Do you need operand here?
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
		vm.hostEmpower()
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
		vm.hostSolicit()
		return true, nil

	case FORGET:
		vm.hostForget()
		return true, nil

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

	case EXTRINSIC:
		vm.hostExtrinsic()
		return true, nil

	case PAYLOAD:
		vm.hostPayload()
		return true, nil

	case ECRECOVER:
		vm.hostECRecover()
		return true, nil

	default:
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
	service, _ := vm.readRegister(0)
	bo, _ := vm.readRegister(1)
	t, err := vm.hostenv.GetService(service)
	if err != nil {
		return NONE
	}

	e := []interface{}{t.CodeHash, t.Balance, t.GasLimitG, t.GasLimitM}
	m := types.Encode(e)
	vm.writeRAMBytes(bo, m[:])

	return OK
}

// Empower updates
func (vm *VM) hostEmpower() uint32 {
	m, _ := vm.readRegister(0)
	a, _ := vm.readRegister(1)
	v, _ := vm.readRegister(0)
	// Set (x'p)_m, (x'p)_a, (x'p)_v
	vm.hostenv.SetX(types.Empower{M: m, A: a, V: v})
	return OK
}

// Assign Core x_c[i]
func (vm *VM) hostAssign() uint32 {
	core, _ := vm.readRegister(0)
	if core >= numCores {
		return CORE
	}
	o, _ := vm.readRegister(1)
	c, _ := vm.readRAMBytes(o, 32*types.MaxAuthorizationQueueItems)
	vm.hostenv.SetX(types.Assign{Core: core, C: c})
	return OK
}

// Designate validators
func (vm *VM) hostDesignate() uint32 {
	o, _ := vm.readRegister(0)
	v, errCode := vm.readRAMBytes(o, 176*V)
	if errCode == OOB {
		return OOB
	}
	return vm.hostenv.SetX(types.Designate{V: v})
}

// Checkpoint gets Gas-remaining
func (vm *VM) hostCheckpoint() uint32 {
	ξ := vm.ξ
	vm.writeRegister(0, uint32(ξ%(1<<32)))
	vm.writeRegister(1, uint32(ξ/(1<<32)))
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
	modResult := ((adjusted % int64(upperLimit)) + int64(upperLimit)) % int64(upperLimit)

	// Step 3: Return the result by adding `lowerLimit` back.
	// This aligns the final result with the desired range: [2^8, 2^32 - 2^9].
	return lowerLimit + uint32(modResult)
}

func (vm *VM) k_exist(i uint32) bool {
	//check i not in K(δ†) or c(255,i)
	_, err := vm.hostenv.GetService(i)
	if err == nil {
		// account found
		return true
	}
	return false
}

func (vm *VM) check(i uint32) uint32 {
	const lowerLimit uint32 = 1 << 8               // 2^8 = 256
	const upperLimit uint32 = (1 << 32) - (1 << 9) // 2^32 - 2^9 = 4294966784

	// Base case: return i if it is not in the set K(delta)
	if !vm.k_exist(i) {
		return i
	}
	// Correct handling of the adjustment to prevent negative values:
	// Convert the expression to int64 to handle potential negatives safely.
	adjusted := int64(i) - int64(lowerLimit) + 1

	// Ensure the adjusted value is non-negative and within the valid range:
	modResult := ((adjusted % int64(upperLimit)) + int64(upperLimit)) % int64(upperLimit)

	// Return check with the adjusted result, adding lowerLimit back to align the range
	// (i−2^8 +1)mod(2^32 −2^9)+2^8
	return vm.check(lowerLimit + uint32(modResult))
}

// New service
func (vm *VM) hostNew() uint32 {
	// put 'g' and 'm' together
	o, _ := vm.readRegister(0)
	c, errCode := vm.readRAMBytes(o, 32)
	if errCode == OOB {
		return errCode
	}
	l, _ := vm.readRegister(1)
	gl, _ := vm.readRegister(2)
	gh, _ := vm.readRegister(3)
	ml, _ := vm.readRegister(4)
	mh, _ := vm.readRegister(5)
	g := uint64(gh)<<32 + uint64(gl)
	m := uint64(mh)<<32 + uint64(ml)

	xContext := vm.hostenv.GetXContext()
	xs := xContext.GetX_s()

	// vm also needs xContext_i. DO NOT HARDCODE
	xi := xContext.GetX_i()

	// simulate a with c, g, m
	a := types.ServiceAccount{
		CodeHash:  common.BytesToHash(c),
		GasLimitG: g,
		GasLimitM: m,
	}
	// Compute footprint & threshold: a_l, a_s, a-t
	a.StorageSize = uint64(81 + l + 0) //a_l =  ∑ 81+z per (h,z) + ∑ 32+s
	a.NumStorageItems = 2*1 + 0        //a_s = 2⋅∣al∣+∣as∣
	a.Balance = a.ComputeThreshold()   //set a's balance to a_t

	b := uint64(0)
	if xs.Balance >= a.Balance {
		// make sure no overflow here
		b = xs.Balance - a.Balance
	}
	if b >= xs.ComputeThreshold() {
		//xs has enough balance to fund the creation of a AND covering its own threshold

		// updating (ω0',xi',xn',(x's)b)

		// ω0' <- xi
		vm.writeRegister(0, xi)

		// xi' <- check(bump(xi))
		next_xi := vm.check(bump(xi)) // this is the next xi
		xContext.SetX_i(next_xi)

		// xi ↦ a
		a.SetServiceIndex(xi)

		// I believe this is the same as solicit. where l∶{(c, l)↦[]} need to be set, which will later be provided by E_P
		// vm.hostenv.WriteServicePreimageLookup(new_service_xi, common.BytesToHash(c), l, []uint32{})
		a.JournalInsertLookup(common.BytesToHash(c), l, []uint32{})

		// xn' <- xn ∪ {xi ↦ a}.
		// TODO: add new_service_xi -> a mapping
		// this is using xi but not xi??
		xContext.SetX_n(xi, &a)

		// (x's)b <- (xs)b - at
		xs.Balance = xs.Balance - a.Balance
		xContext.SetX_s(xs)
		vm.hostenv.SetXContext(xContext)

		solicit := Solicit{
			BlobHash: common.BytesToHash(c),
			Length:   l,
		}
		vm.Solicits = append(vm.Solicits, solicit)
		return OK
	} else {
		return CASH //balance insufficient
	}
}

// Upgrade service
func (vm *VM) hostUpgrade() uint32 {
	o, _ := vm.readRegister(0)
	gl, _ := vm.readRegister(1)
	gh, _ := vm.readRegister(2)
	ml, _ := vm.readRegister(3)
	mh, _ := vm.readRegister(4)
	c, errCode := vm.readRAMBytes(o, 32)
	if errCode == OOB {
		return errCode
	}
	g := uint64(gh)<<32 + uint64(gl)
	m := uint64(mh)<<32 + uint64(ml)

	xContext := vm.hostenv.GetXContext()
	xs := xContext.GetX_s()
	xs.CodeHash = common.BytesToHash(c)
	xs.GasLimitG = g
	xs.GasLimitM = m
	xContext.SetX_s(xs)
	vm.hostenv.SetXContext(xContext)
	return OK
}

// Transfer host call
func (vm *VM) hostTransfer() uint32 {
	d, _ := vm.readRegister(0)
	al, _ := vm.readRegister(1)
	ah, _ := vm.readRegister(2)
	gl, _ := vm.readRegister(3)
	gh, _ := vm.readRegister(4)
	o, _ := vm.readRegister(5)

	a := uint64(ah)<<32 + uint64(al)
	g := uint64(gh)<<32 + uint64(gl)
	// TODO check d -- WHO; check g -- LOW, HIGH
	m, errCode := vm.readRAMBytes(o, M)
	if errCode == OOB {
		return OOB
	}
	return vm.hostenv.SetX(types.AddTransfer{M: m, A: a, G: g, D: d})
}

// Gas Service
func (vm *VM) hostGas() uint32 {
	vm.writeRegister(0, uint32(vm.ξ&0xFFFFFFFF))
	vm.writeRegister(1, uint32((vm.ξ>>32)&0xFFFFFFFF))
	return OK
}

// Quit Service
func (vm *VM) hostQuit() uint32 {
	/*
		d, _ := vm.readRegister(0)
		s := vm.hostenv.GetService(vm.S)
		a := s.Balance + types.BaseServiceBalance // TODO: (x_s)_t
		var transferMemo *TransferMemo
		if d == vm.S || d == 0xFFFFFFFF {
			transferMemo = nil
		} else {
			o, _ := vm.readRegister(1)
			transferMemo := types.TransferMemoFromBytes(transferMemoBytes)
			transferBytes, _ := vm.readRAMBytes(o, types.TransferMemoSize)
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
	n, _ := vm.readRegister(0)
	o, _ := vm.readRegister(1)
	gasBytes, _ := vm.readRAMBytes(o, 8)
	registerBytes, _ := vm.readRAMBytes(o, 8+13*4)
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
	s, _ := vm.readRegister(0)
	ho, _ := vm.readRegister(1)
	bo, _ := vm.readRegister(2)
	bz, _ := vm.readRegister(3)
	k_bytes, err_k := vm.readRAMBytes(ho, 32)
	if err_k == OOB {
		vm.writeRegister(0, OOB)
		return OOB
	}
	h := common.Blake2Hash(k_bytes)
	v := vm.hostenv.ReadServicePreimageBlob(s, h)

	l := uint32(len(v))
	if bz < l {
		l = bz
	}
	vm.writeRAMBytes(bo, v[:l])

	if len(h) != 0 {
		vm.writeRegister(0, l)
		return l
	} else {
		vm.writeRegister(0, OOB)
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
	// Assume that all ram can be read and written
	s, _ := vm.readRegister(0)
	ko, _ := vm.readRegister(1)
	kz, _ := vm.readRegister(2)
	bo, _ := vm.readRegister(3)
	bz, _ := vm.readRegister(4)
	k, err_k := vm.readRAMBytes(ko, int(kz)) // this is the raw key.
	if err_k == OOB {
		fmt.Println("Read RAM Error")
		vm.writeRegister(0, OOB)
		return OOB
	}

	v := vm.hostenv.ReadServiceStorage(s, k)

	l := uint32(len(v))
	if bz < l {
		l = bz
	}
	vm.writeRAMBytes(bo, v[:l])

	if len(k) != 0 {
		vm.writeRegister(0, l)
		return l
	} else {
		vm.writeRegister(0, OOB)
		return OOB
	}
}

// Write Storage
func (vm *VM) hostWrite() uint32 {
	// Assume that all ram can be read and written
	// Got storage of bold S(service account in GP) by setting s = 0, k = k(from RAM)
	s := uint32(0)
	ko, _ := vm.readRegister(0)
	kz, _ := vm.readRegister(1)
	vo, _ := vm.readRegister(2)
	vz, _ := vm.readRegister(3)
	k, err_k := vm.readRAMBytes(ko, int(kz))
	if err_k == OOB {
		fmt.Println("Read RAM Error")
		vm.writeRegister(0, OOB)
		return OOB
	}
	S_s_k := vm.hostenv.ReadServiceStorage(s, k) // get bold s in GP
	l := uint32(len(S_s_k))
	if l == 0 {
		l = NONE
	}

	// check balance a_t <= a_b
	service_byte := vm.hostenv.ReadServiceBytes(s)
	a_b_byte := service_byte[len(service_byte)-36 : len(service_byte)-24]
	a_l_byte := service_byte[len(service_byte)-12 : len(service_byte)-4]
	a_i_byte := service_byte[len(service_byte)-4:]
	a_b := binary.LittleEndian.Uint64(a_b_byte)
	a_l := binary.LittleEndian.Uint64(a_l_byte)
	a_i := binary.LittleEndian.Uint64(a_i_byte)
	a_t := 10 + 1*a_i + 100*a_l

	if a_t <= a_b {
		vm.writeRegister(0, uint32(l))
		// adjust S
		if vz == 0 {
			vm.hostenv.DeleteServiceStorageKey(s, k)
		} else {
			v, _ := vm.readRAMBytes(vo, int(vz))
			vm.hostenv.WriteServiceStorage(s, k, v)
		}
		vm.writeRegister(0, l)
		return l
	} else if a_t > a_b {
		vm.writeRegister(0, FULL)
		return FULL
	} else {
		vm.writeRegister(0, OOB)
		return OOB
	}
}

func (vm *VM) GetJCETime() uint32 {
	return common.ComputeCurrentJCETime()
}

// Solicit preimage
func (vm *VM) hostSolicit() uint32 {
	// Got l of X_s by setting s = 1, z = z(from RAM)
	// Question: William why this is hardcoded as 49?

	xContext := vm.hostenv.GetXContext()
	xs := xContext.GetX_s()

	o, _ := vm.readRegister(0)
	z, _ := vm.readRegister(1)              // z: blob_len
	hBytes, err_h := vm.readRAMBytes(o, 32) // h: blobHash
	if err_h == OOB {
		fmt.Println("Read RAM Error")
		vm.writeRegister(0, OOB)
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
			vm.writeRegister(0, FULL)
			return FULL
		}
	}
	*/

	solicit := Solicit{
		BlobHash: common.BytesToHash(hBytes),
		Length:   z,
	}
	//hBytes = falseBytes(hBytes[4:])
	//lbytes := uint32ToBytes(z)
	//key := append(lbytes, hBytes...) //GP(292)
	//h := common.BytesToHash(key)

	X_s_l := vm.hostenv.ReadServicePreimageLookup(xs.ServiceIndex(), common.BytesToHash(hBytes), z)
	if len(X_s_l) == 0 {
		// when preimagehash is not found, put it into solicit request - so we can ask other DAs
		vm.Solicits = append(vm.Solicits, solicit)
		//vm.hostenv.WriteServicePreimageLookup(s, common.BytesToHash(hBytes), z, []uint32{})
		xs.JournalInsertLookup(common.BytesToHash(hBytes), z, []uint32{})
		xContext.SetX_s(xs)
		vm.hostenv.SetXContext(xContext)
		vm.writeRegister(0, OK)
		return OK
	} else if X_s_l[0] == 2 { // [x, y]
		anchor_timeslot := vm.GetJCETime()
		vm.Solicits = append(vm.Solicits, solicit)
		//vm.hostenv.WriteServicePreimageLookup(s, common.BytesToHash(hBytes), z, append(X_s_l, []uint32{anchor_timeslot}...))
		xContext.SetX_s(xs)
		vm.hostenv.SetXContext(xContext)
		xs.JournalInsertLookup(common.BytesToHash(hBytes), z, append(X_s_l, []uint32{anchor_timeslot}...))
		vm.writeRegister(0, OK)
		return OK
	} else {
		vm.writeRegister(0, HUH)
		return HUH
	}
}

// Forget preimage
func (vm *VM) hostForget() uint32 {

	s := uint32(1) // s: serviceIndex
	o, _ := vm.readRegister(0)
	z, _ := vm.readRegister(1)
	hBytes, errCode := vm.readRAMBytes(o, 32)
	if errCode == OOB {
		fmt.Println("Read RAM Error")
		return OOB
	}

	X_s_l := vm.hostenv.ReadServicePreimageLookup(s, common.BytesToHash(hBytes), z)
	anchor_timeslot := vm.GetJCETime()
	if len(X_s_l) == 0 || (len(X_s_l) == 2 && X_s_l[1] < (anchor_timeslot-D)) {
		vm.hostenv.DeleteServicePreimageLookupKey(s, common.BytesToHash(hBytes), z)
		vm.hostenv.DeleteServicePreimageKey(s, common.BytesToHash(hBytes))
		vm.writeRegister(0, OK)
		return OK
	} else if len(X_s_l) == 1 {
		vm.hostenv.WriteServicePreimageLookup(s, common.BytesToHash(hBytes), z, append(X_s_l, []uint32{anchor_timeslot}...))
		vm.writeRegister(0, OK)
		return OK
	} else if len(X_s_l) == 3 && X_s_l[1] < (anchor_timeslot-D) {
		X_s_l[2] = uint32(anchor_timeslot)
		vm.hostenv.WriteServicePreimageLookup(s, common.BytesToHash(hBytes), z, X_s_l)
		vm.writeRegister(0, OK)
		return OK
	} else {
		vm.writeRegister(0, HUH)
		return HUH
	}
}

// HistoricalLookup determines whether the preimage of some hash h was available for lookup by some service account a at some timeslot t, and if so, provide its preimage
func (vm *VM) hostHistoricalLookup(t uint32) uint32 {
	// take a service s (from \omega_0 ),
	s, _ := vm.readRegister(0)
	ho, _ := vm.readRegister(1)
	bo, _ := vm.readRegister(2)
	bz, _ := vm.readRegister(3)

	hBytes, errCode := vm.readRAMBytes(ho, 32)
	if errCode == OOB {
		fmt.Println("Read RAM Error")
		vm.writeRegister(0, OOB)
		return errCode
	}
	h := common.Blake2Hash(hBytes)
	fmt.Println(h)
	v := vm.hostenv.HistoricalLookup(s, t, h)
	vLength := uint32(len(v))
	if vLength == 0 {
		vm.writeRegister(0, NONE)
		return NONE
	}

	if vLength != 0 {
		l := vLength
		if bz < l {
			l = bz
		}
		vm.writeRAMBytes(bo, v[:l])
		vm.writeRegister(0, vLength)
		fmt.Println(v[:l])
		return vLength
	} else {
		vm.writeRegister(0, NONE)
		return NONE
	}
}

// Import Segment
func (vm *VM) hostImport() uint32 {
	// import  - which copies  a specific i  (e.g. holding the bytes "9") into RAM from "ImportDA" to be "accumulated"
	omega_0, _ := vm.readRegister(0)
	var v_Bytes []byte
	if omega_0 < uint32(len(vm.Imports)) {
		v_Bytes = vm.Imports[omega_0][:]
	} else {
		v_Bytes = []byte{}
	}

	o, _ := vm.readRegister(1)
	l, _ := vm.readRegister(2)
	if l > (W_C * W_S) {
		l = W_C * W_S
	}

	if len(v_Bytes) != 0 {
		errCode := vm.writeRAMBytes(o, v_Bytes[:])
		if errCode == OOB {
			vm.writeRegister(0, OOB)
			return errCode
		}
		vm.writeRegister(0, OK)
		return OK
	} else {
		vm.writeRegister(0, NONE)
		return NONE
	}
}

// Extrinsic
func (vm *VM) hostExtrinsic() uint32 {

	omega_0, _ := vm.readRegister(0)
	var v_Bytes []byte
	if omega_0 < uint32(len(vm.Extrinsics)) {
		v_Bytes = vm.Extrinsics[omega_0]
	} else {
		v_Bytes = []byte{}
	}

	o, _ := vm.readRegister(1)
	l, _ := vm.readRegister(2)

	if len(v_Bytes) != 0 {
		errCode := vm.writeRAMBytes(o, v_Bytes[:l])
		if errCode == OOB {
			vm.writeRegister(0, OOB)
			return errCode
		}
		vm.writeRegister(0, OK)
		return OK
	} else {
		vm.writeRegister(0, NONE)
		return NONE
	}
}

// Payload
func (vm *VM) hostPayload() uint32 {

	v_Bytes := vm.payload[:]
	o, _ := vm.readRegister(0)
	l, _ := vm.readRegister(1)

	if len(v_Bytes) != 0 {
		errCode := vm.writeRAMBytes(o, v_Bytes[:l])
		if errCode == OOB {
			vm.writeRegister(0, OOB)
			return errCode
		}
		vm.writeRegister(0, OK)
		return OK
	} else {
		return NONE
	}
}

// ECRecover
func (vm *VM) hostECRecover() uint32 {
	o, _ := vm.readRegister(0)
	ho, _ := vm.readRegister(1)
	sig, _ := vm.readRAMBytes(o, 65)
	h, _ := vm.readRAMBytes(ho, 32)
	recoveredPubKey, err := crypto.SigToPub(h, sig)
	if err != nil {
		vm.writeRegister(0, NONE)
	}
	recoveredPubKeyBytes := crypto.FromECDSAPub(recoveredPubKey)
	p, _ := vm.readRegister(2)
	vm.writeRAMBytes(p, recoveredPubKeyBytes[:])

	vm.writeRegister(0, OK)
	return OK

}

func (vm *VM) GetExport() [][]byte {
	e := vm.Exports
	if e == nil {
		e = make([][]byte, 0)
	}
	return e
}

func (vm *VM) SetExport(updated_e [][]byte) {
	vm.Exports = updated_e
}

// Export segment host-call
func (vm *VM) hostExport(pi uint32) (uint32, [][]byte) {
	/*
		need to get
			ς
		properly
	*/
	p, _ := vm.readRegister(0)
	z, _ := vm.readRegister(1)
	if z > (W_C * W_S) {
		z = W_C * W_S
	}

	e := vm.Exports

	x, errCode := vm.readRAMBytes(p, int(z))
	if errCode == OOB {
		vm.writeRegister(0, OOB)
		return OOB, e
	}

	/*  apply eq(187) zero-padding function:

	And P is the zero-padding function to take an octet array to some multiple of n in length:
	(187) 	P n∈N 1∶ ∶{ Y → Y k⋅n
			x ↦ x ⌢ [0, 0, ...] ((∣x∣+n−1) mod n)+1...n

	n := (W_C * W_S)
	length := n - ((len(x) + n - 1) % n) + 1
	zeroSequence := make([]byte, length)
	x = append(x, zeroSequence...)
	*/
	x = common.PadToMultipleOfN(x, W_C*W_S)

	ς := uint32(0)               // Assume ς (sigma, Represent segment offset), need to get ς properly
	if ς+uint32(len(e)) >= W_X { // W_X
		vm.writeRegister(0, FULL)
		return FULL, e
	} else {
		vm.writeRegister(0, ς+uint32(len(e)))
		e = append(e, x)
		vm.SetExport(e)
		// errCode = vm.hostenv.ExportSegment(x)
		return OK, e
	}
}

// Not sure but this is running a machine at instruction i? using bytes from po to pz
func (vm *VM) hostMachine() uint32 {
	po, _ := vm.readRegister(0)
	pz, _ := vm.readRegister(1)
	i, _ := vm.readRegister(2)

	p, errCode := vm.readRAMBytes(po, int(pz))
	if errCode != OK {
		return errCode
	}
	// need service account here??
	serviceAcct := uint32(0)
	n := vm.CreateVM(serviceAcct, p, i)
	return n
}

func (vm *VM) hostPeek() uint32 {
	n, _ := vm.readRegister(0)
	a, _ := vm.readRegister(1)
	b, _ := vm.readRegister(2)
	l, _ := vm.readRegister(3)
	m, ok := vm.GetVM(n) // hostenv.
	if !ok {
		return WHO
	}
	// read l bytes from m
	s, errCode := m.readRAMBytes(b, int(l))
	if errCode == OOB {
		return errCode
	}
	// write l bytes to vm
	errCode = vm.writeRAMBytes(a, s[:])
	if errCode == OOB {
		return errCode
	}
	return OK
}

func (vm *VM) hostPoke() uint32 {
	n, _ := vm.readRegister(0)
	a, _ := vm.readRegister(1)
	b, _ := vm.readRegister(2)
	l, _ := vm.readRegister(3)
	m, ok := vm.GetVM(n) // hostenv.
	if !ok {
		return WHO
	}
	s, errCode := m.readRAMBytes(a, int(l))
	if errCode == OOB {
		return errCode
	}
	errCode = m.writeRAMBytes(b, s[:])
	if errCode == OOB {
		return errCode
	}
	return OK
}

func (vm *VM) hostExpunge() uint32 {
	n, _ := vm.readRegister(0)
	if vm.ExpungeVM(n) {
		return OK
	}
	return WHO
}
