package statedb

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm/program"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/types"
	"github.com/gorilla/websocket"
)

const (
	BackendInterpreter     = "interpreter" // Go interpreter
	BackendCompiler        = "compiler"    // X86 recompiler
	BackendCompilerSandbox = "sandbox"
)

const (
	M   = types.TransferMemoSize
	V   = 1023
	Z_A = 2

	Z_P = (1 << 12)
	Z_Q = (1 << 16)
	Z_I = (1 << 24)
	Z_Z = (1 << 16)
)

const (
	ModeAccumulate   = "accumulate"
	ModeIsAuthorized = "is_authorized"
	ModeRefine       = "refine"
)

var (
	PvmLogging                = false
	EnableTaintTrackingGlobal = false
)

// serializeHashes serializes an array of common.Hash back-to-back with no separators
func serializeHashes(hashes []common.Hash) []byte {
	result := make([]byte, 0, len(hashes)*32)
	for _, h := range hashes {
		result = append(result, h[:]...)
	}
	return result
}

// serializeECChunks serializes an array of DistributeECChunk back-to-back with no separators
func serializeECChunks(chunks []types.DistributeECChunk) []byte {
	result := make([]byte, 0)
	for _, chunk := range chunks {
		result = append(result, chunk.Data...)
	}
	return result
}

// saveToLogDir is a helper function to save data to a file in the log directory.
// It creates the directory if needed and silently ignores errors if logDir is empty.
// If logDir contains "SKIP" anywhere in the path, no files are written.
func saveToLogDir(logDir, filename string, data []byte) {
	if logDir == "" || strings.Contains(logDir, "SKIP") {
		return
	}
	if !PvmTraceMode {
		return
	}
	os.MkdirAll(logDir, 0755)
	os.WriteFile(filepath.Join(logDir, filename), data, 0644)
}

// saveToLogDirOutput saves the result to either "output" or "err" file based on the result code.
// If r.Ok is set, writes to "output" file. If r.Err is set, writes to "err" file.
func saveToLogDirOutput(logDir string, r types.Result, res uint64) {
	if logDir == "" || strings.Contains(logDir, "SKIP") {
		return
	}
	if !PvmTraceMode {
		return
	}
	os.MkdirAll(logDir, 0755)
	if len(r.Ok) > 0 {
		os.WriteFile(filepath.Join(logDir, "output"), r.Ok, 0644)
	} else if r.Err != 0 {
		// Write error code as a single byte
		os.WriteFile(filepath.Join(logDir, "err"), []byte{r.Err}, 0644)
	}
}

type ExecutionVM interface {
	ReadRegister(int) uint64
	ReadRegisters() [13]uint64
	WriteRegister(int, uint64)
	ReadRAMBytes(uint32, uint32) ([]byte, uint64)
	WriteRAMBytes(uint32, []byte) uint64
	SetHeapPointer(uint32)
	SetPagesAccessRange(startPage, pageCount int, access int) error
	GetGas() int64
	SetGas(int64)
	GetPC() uint64
	SetPC(uint64)
	GetMachineState() uint8
	GetFaultAddress() uint64
	Panic(uint64)
	SetHostResultCode(uint64)
	Init(argument_data_a []byte) (err error)
	Execute(vm *VM, EntryPoint uint32, logDir string) error
	ExecuteAsChild(entryPoint uint32) error
	GetHostID() uint64
	Destroy()

	// to support ExecuteWorkPackageBundleSteps
	InitStepwise(vm *VM, entryPoint uint32) error
	ExecuteStep(vm *VM) []byte
}

type VM struct {
	ExecutionVM
	VMs             map[uint32]*ExecutionVM
	VmsEntryCounter map[uint32]int

	Backend string
	IsChild bool
	LogDir  string // Log directory for trace files

	ResultCode     uint8
	HostResultCode uint64
	MachineState   uint8
	Fault_address  uint32
	terminated     bool

	Mode       string
	hostenv    types.HostEnv
	logging    string
	InitialGas uint64
	FinalGas   int64 // Gas remaining after execution (saved before destroying ExecutionVM)

	// service metadata
	ServiceAccount  *types.ServiceAccount
	Service_index   uint32
	ServiceMetadata []byte

	// Refine Inputs and Outputs
	WorkItemIndex    uint32
	WorkPackage      types.WorkPackage
	Extrinsics       types.ExtrinsicsBlobs
	Authorization    []byte
	Imports          [][][]byte
	AccumulateInputs []types.AccumulateInput
	Transfers        []types.DeferredTransfer
	N                common.Hash
	Delta            map[uint32]*types.ServiceAccount

	Exports       [][]byte
	TotalExported uint64

	// Accumulate (used in host functions)
	X        *types.XContext
	Y        types.XContext
	Timeslot uint32

	// Verkle tree witness transition (Phase 1): Witness recording for builder
	// These track read dependencies during execution to export as witnesses
	codeWitness    map[common.Address][]byte                   // code reads
	storageWitness map[common.Address]evmtypes.ContractStorage // storage reads
	pushFrame      func([]byte)
}

type Program program.Program

const (
	NONE = (1 << 64) - 1 // 2^32 - 1 15
	WHAT = (1 << 64) - 2 // 2^32 - 2 14
	OOB  = (1 << 64) - 3 // 2^32 - 3 13
	WHO  = (1 << 64) - 4 // 2^32 - 4 12
	FULL = (1 << 64) - 5 // 2^32 - 5 11
	CORE = (1 << 64) - 6 // 2^32 - 6 10
	CASH = (1 << 64) - 7 // 2^32 - 7 9
	LOW  = (1 << 64) - 8 // 2^32 - 8 8
	HUH  = (1 << 64) - 9 // 2^32 - 9 7
)

const (
	HALT  = 0 // regular halt ∎
	PANIC = 1 // panic ☇
	FAULT = 2 // page-fault F
	HOST  = 3 // host-call̵ h
	OOG   = 4 // out-of-gas ∞
)

func DecodeProgram(p []byte) (*Program, uint32, uint32, uint32, uint32, []byte, []byte) {
	pure := p
	// see A.37
	o_size := types.DecodeE_l(pure[:3])
	w_size := types.DecodeE_l(pure[3:6])
	z_val := types.DecodeE_l(pure[6:8])
	s_val := types.DecodeE_l(pure[8:11])
	//fmt.Printf("DecodeProgram: o_size=%d, w_size=%d, z_val=%d, s_val=%d, total_header_size=11\n",o_size, w_size, z_val, s_val)
	var o_byte, w_byte []byte
	offset := uint64(11)
	if offset+o_size <= uint64(len(pure)) {
		o_byte = pure[offset : offset+o_size]
	} else {
		o_byte = make([]byte, o_size)
	}
	offset += o_size

	if offset+w_size <= uint64(len(pure)) {
		w_byte = pure[offset : offset+w_size]
	} else {
		w_byte = make([]byte, w_size)
	}
	offset += w_size

	c_size := types.DecodeE_l(pure[offset : offset+4])
	offset += 4
	if len(pure[offset:]) != int(c_size) {
		// fmt.Printf("DecodeProgram o_size: %d, w_size: %d, z_val: %d, s_val: %d len(w_byte)=%d\n", o_size, w_size, z_val, s_val, len(w_byte))
		return nil, 0, 0, 0, 0, nil, nil
	}
	return (*Program)(decodeCorePart(pure[offset:])), uint32(o_size), uint32(w_size), uint32(z_val), uint32(s_val), o_byte, w_byte
}

func DecodeProgram_pure_pvm_blob(p []byte) *Program {
	return (*Program)(decodeCorePart(p))
}

// EncodeProgram encodes raw instruction code and bitmask into a PVM blob format
// that can be decoded by decodeCorePart
func EncodeProgram(code []byte, bitmask []byte) []byte {
	j_size := uint64(0) // TODO: add J support
	z := uint64(0)      // TODO: add z support correctly
	c_size := uint64(len(code))

	// Encode the header values using JAM's E() function
	j_size_encoded := types.E(j_size)
	z_encoded := types.E(z)
	c_size_encoded := types.E(c_size)

	// Compress bitmask: convert from bit array to byte array
	k_bytes := compressBits(bitmask)

	// Build the blob: header + j_bytes + c_bytes + k_bytes
	blob := make([]byte, 0)
	blob = append(blob, j_size_encoded...)
	blob = append(blob, z_encoded...)
	blob = append(blob, c_size_encoded...)
	// TODO: No j_bytes since j_size * z = 0 * 0 = 0
	blob = append(blob, code...)    // c_bytes
	blob = append(blob, k_bytes...) // k_bytes (compressed bitmask)

	fmt.Printf("EncodeProgram: j_size=%d, z=%d, c_size=%d, code_len=%d, bitmask_len=%d, k_bytes_len=%d, total_blob_len=%d BLOB: %v\n",
		j_size, z, c_size, len(code), len(bitmask), len(k_bytes), len(blob), blob)
	return blob
}

// compressBits converts a bitmask bit array ([]byte with 0/1 values) to compressed bytes
func compressBits(bitmask []byte) []byte {
	if len(bitmask) == 0 {
		return []byte{}
	}

	// Pack bits into bytes
	numBytes := (len(bitmask) + 7) / 8
	compressed := make([]byte, numBytes)

	for i, bit := range bitmask {
		if bit != 0 {
			byteIndex := i / 8 // optimize?
			bitIndex := i % 8
			compressed[byteIndex] |= (1 << bitIndex)
		}
	}

	return compressed
}

var decodeCorePart = program.DecodeCorePart

func CeilingDivide(a, b uint32) uint32 {
	return (a + b - 1) / b
}

func P_func(x uint32) uint32 {
	return Z_P * CeilingDivide(x, Z_P)
}

func Z_func(x uint32) uint32 {
	return Z_Z * CeilingDivide(x, Z_Z)
}

// NewVM initializes a new VM with a given program
func NewVM(service_index uint32, code []byte, initialRegs []uint64, initialPC uint64, initialHeap uint64, hostENV types.HostEnv, jam_ready_blob bool, Metadata []byte, pvmBackend string, initialGas uint64) *VM {

	if len(pvmBackend) == 0 {
		panic("pvmBackend cannot be empty")
	}
	if len(code) == 0 {
		return nil
	}

	var p *Program
	var o_size, w_size, z, s uint32
	var o_byte, w_byte []byte

	if jam_ready_blob {
		p, o_size, w_size, z, s, o_byte, w_byte = DecodeProgram(code)
	} else {
		p = DecodeProgram_pure_pvm_blob(code)
		o_size = 0
		w_size = uint32(initialHeap)
		z = 0
		s = 0
		o_byte = []byte{}
		w_byte = make([]byte, w_size)
	}
	vm := &VM{
		hostenv:         hostENV, //check if we need this
		Exports:         make([][]byte, 0),
		Service_index:   service_index,
		ServiceMetadata: Metadata,
		Backend:         pvmBackend,
		VMs:             make(map[uint32]*ExecutionVM),
		VmsEntryCounter: make(map[uint32]int),
	}

	requiredMemory := uint64(uint64(5*Z_Z) + uint64(Z_func(o_size)) + uint64(Z_func(w_size+z*Z_P)) + uint64(Z_func(s)) + uint64(Z_I))
	if requiredMemory > math.MaxUint32 {
		log.Error(vm.logging, "Standard Program Initialization Error")
		return nil
	}

	// Create C VM immediately with initial registers
	if len(initialRegs) == 0 {
		initialRegs = make([]uint64, 13)
	} else if len(initialRegs) < 13 {
		// Extend to 13 registers if needed
		extended := make([]uint64, 13)
		copy(extended, initialRegs)
		initialRegs = extended
	}
	if len(p.Code) == 0 {
		panic("No code provided to NewVM")
	}
	vm.InitialGas = initialGas
	if vm.Backend == BackendCompiler {
		rvm := NewRecompilerVM(service_index, initialRegs, initialPC, initialHeap, hostENV, jam_ready_blob, Metadata, initialGas, p, o_size, w_size, z, s, o_byte, w_byte)
		if rvm == nil {
			return nil
		}
		vm.ExecutionVM = rvm
	} else if vm.Backend == BackendInterpreter {
		machine := NewVMGo(service_index, p, initialRegs, initialPC, initialGas, hostENV)
		machine.Gas = int64(initialGas)
		// Enable taint tracking if requested
		if EnableTaintTrackingGlobal {
			machine.EnableTaintForStep(0, 0)
		}
		vm.ExecutionVM = machine
		// o - read-only
		rw_data_address := uint32(2*Z_Z) + Z_func(o_size)
		rw_data_address_end := rw_data_address + P_func(w_size) + z*Z_P
		current_heap_pointer := rw_data_address_end
		machine.SetHeapPointer(current_heap_pointer)

		z_o := Z_func(o_size)
		z_w := Z_func(w_size + z*Z_P)
		z_s := Z_func(s)
		requiredMemory = uint64(5*Z_Z + z_o + z_w + z_s + Z_I)
		if requiredMemory > math.MaxUint32 {
			log.Error(vm.logging, "Standard Program Initialization Error")
		}

		machine.SetHeapPointer(current_heap_pointer)
		machine.SetMemoryBounds(o_size, w_size, z, s, o_byte, w_byte)
	}
	//  else if vm.Backend == BackendCompilerSandbox {
	// 	rvm := NewRecompilerVMSandbox(service_index, code, initialRegs, initialPC, initialHeap, hostENV, jam_ready_blob, Metadata, initialGas, pvmBackend)
	// 	if rvm == nil {
	// 		return nil
	// 	}
	// 	vm.ExecutionVM = rvm
	// }

	vm.VMs = nil
	return vm
}

func (vm *VM) attachFrameServer(addr, htmlPath string) error {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	var (
		connMu sync.Mutex
		wsConn *websocket.Conn
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, htmlPath)
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("upgrade error:", err)
			return
		}
		fmt.Println("WS client connected")

		connMu.Lock()
		if wsConn != nil {
			wsConn.Close()
		}
		wsConn = c
		connMu.Unlock()

		c.SetCloseHandler(func(code int, text string) error {
			fmt.Printf("WS closed: %d %s\n", code, text)
			connMu.Lock()
			if wsConn == c {
				wsConn = nil
			}
			connMu.Unlock()
			return nil
		})
	})

	vm.pushFrame = func(data []byte) {
		connMu.Lock()
		defer connMu.Unlock()
		if wsConn != nil {
			if err := wsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				fmt.Println("WS write error:", err)
				wsConn.Close()
				wsConn = nil
			}
		}
	}

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		fmt.Println("Viewer server listening on", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Println("ListenAndServe:", err)
		}
	}()
	return nil
}

func (vm *VM) NewEmptyExecutionVM(service_index uint32, p *Program, initialRegs []uint64, initialPC uint64, initialHeap uint64, hostENV types.HostEnv, machineIndex uint32, childEntryIndex int) *ExecutionVM {
	var e_vm ExecutionVM
	initialGas := uint64(0x7FFFFFFFFFFFFFFF)
	if vm.Backend == BackendInterpreter {
		machine := NewVMGo(service_index, p, initialRegs, initialPC, initialGas, hostENV)
		machine.Gas = int64(initialGas)
		// Track all steps (TargetStep=0 means no window restriction)
		if EnableTaintTrackingGlobal {
			machine.EnableTaintForStep(0, 0)
		}
		e_vm = machine
		machine.ChildIndex = int(machineIndex)
		machine.ChildeEntryCount = childEntryIndex
		machine.LogDir = vm.LogDir // Inherit logDir from parent VM
		// Trace writers will be initialized lazily when first needed (allows dynamic enabling)
	} else if vm.Backend == BackendCompiler {
		rvm := NewRecompilerVMWithoutSetup(service_index, initialRegs, initialPC, hostENV, false, []byte{}, uint64(initialGas), p)
		// Set child VM info for trace verification
		rvm.IsChild = true
		rvm.ChildIndex = int(machineIndex)
		rvm.ChildEntryCount = childEntryIndex
		rvm.LogDir = vm.LogDir // Inherit logDir from parent VM
		e_vm = rvm
	}

	return &e_vm
}

func NewVMFromCode(serviceIndex uint32, code []byte, i uint64, initialHeap uint64, hostENV types.HostEnv, pvmBackend string, initialGas uint64) *VM {
	// strip metadata
	metadata, c := types.SplitMetadataAndCode(code)

	code = c

	return NewVM(serviceIndex, c, []uint64{}, i, initialHeap, hostENV, true, []byte(metadata), pvmBackend, initialGas)
}

var RecordTime = true

func (vm *VM) GetVMLogging() string {
	return vm.logging
}

func (vm *VM) SetServiceIndex(index uint32) {
	vm.Service_index = index
}

func (vm *VM) GetServiceIndex() uint32 {
	return vm.Service_index
}

// input by order([work item index],[workpackage itself], [result from IsAuthorized], [import segments], [export count])
func (vm *VM) ExecuteRefine(core uint16, workitemIndex uint32, workPackage types.WorkPackage, authorization types.Result, importsegments [][][]byte, export_count uint16, extrinsics types.ExtrinsicsBlobs, p_a common.Hash, n common.Hash, logDir string) (r types.Result, res uint64, exportedSegments [][]byte) {
	vm.Mode = ModeRefine
	vm.LogDir = logDir // Set logDir so child VMs can inherit it

	workitem := workPackage.WorkItems[workitemIndex]

	// core index is now a refine argument in 0.7.4
	a := append(types.E(uint64(core)), types.E(uint64(workitemIndex))...)
	serviceBytes := types.E(uint64(workitem.Service))
	a = append(a, serviceBytes...)
	//fmt.Printf("ExecuteRefine  s %d bytes - %x\n", len(serviceBytes), serviceBytes)
	encoded_workitem_payload, _ := types.Encode(workitem.Payload)
	a = append(a, encoded_workitem_payload...) // variable number of bytes
	//fmt.Printf("ExecuteRefine  payload %d bytes - %x\n", len(encoded_workitem_payload), encoded_workitem_payload)
	a = append(a, workPackage.Hash().Bytes()...) // 32

	//fmt.Printf("ExecuteRefine TOTAL len(a)=%d %x\n", len(a), a)
	vm.WorkItemIndex = workitemIndex
	vm.WorkPackage = workPackage

	vm.N = n
	vm.Authorization = authorization.Ok
	vm.Extrinsics = extrinsics
	vm.Imports = importsegments

	for i := 0; i < int(workitemIndex); i++ {
		item := workPackage.WorkItems[i]
		vm.TotalExported += uint64(item.ExportCount)
	}

	// Save inputs
	saveToLogDir(logDir, "input", a)

	vm.executeWithBackend(a, types.EntryPointRefine, logDir)
	r, res = vm.getArgumentOutputs()
	exportedSegments = vm.Exports
	vm.destroyVMs()
	saveToLogDirOutput(logDir, r, res)
	log.Trace(vm.logging, string(vm.ServiceMetadata), "Result", r.String(), "fault_address", vm.Fault_address, "resultCode", vm.ResultCode)

	// Save all exported segments back to back in a single file
	if len(exportedSegments) > 0 {
		var allExports []byte
		for _, segment := range exportedSegments {
			allExports = append(allExports, segment...)
		}
		saveToLogDir(logDir, "exports", allExports)
	}

	return r, res, exportedSegments
}

// input by order([work item index],[workpackage itself], [result from IsAuthorized], [import segments], [export count])
func (vm *VM) SetupRefine(core uint16, workitemIndex uint32, workPackage types.WorkPackage, authorization types.Result, importsegments [][][]byte, export_count uint16, extrinsics types.ExtrinsicsBlobs, p_a common.Hash, n common.Hash) {
	vm.Mode = ModeRefine

	workitem := workPackage.WorkItems[workitemIndex]

	// core index is now a refine argument in 0.7.4
	a := append(types.E(uint64(core)), types.E(uint64(workitemIndex))...)
	serviceBytes := types.E(uint64(workitem.Service))
	a = append(a, serviceBytes...)
	//fmt.Printf("ExecuteRefine  s %d bytes - %x\n", len(serviceBytes), serviceBytes)
	encoded_workitem_payload, _ := types.Encode(workitem.Payload)
	a = append(a, encoded_workitem_payload...) // variable number of bytes
	//fmt.Printf("ExecuteRefine  payload %d bytes - %x\n", len(encoded_workitem_payload), encoded_workitem_payload)
	a = append(a, workPackage.Hash().Bytes()...) // 32

	//fmt.Printf("ExecuteRefine TOTAL len(a)=%d %x\n", len(a), a)
	vm.WorkItemIndex = workitemIndex
	vm.WorkPackage = workPackage

	vm.N = n
	vm.Authorization = authorization.Ok
	vm.Extrinsics = extrinsics
	vm.Imports = importsegments

	for i := 0; i < int(workitemIndex); i++ {
		item := workPackage.WorkItems[i]
		vm.TotalExported += uint64(item.ExportCount)
	}

	vm.Init(a)
	vm.IsChild = false

	// Initialize for stepwise execution
	err := vm.InitStepwise(vm, types.EntryPointRefine)
	if err != nil {
		log.Error(vm.logging, "InitStepwise failed", "error", err)
	}

	// do not execute ... we will do ExecuteStep one step at a time
}

func (vm *VM) ExecuteAccumulate(t uint32, s uint32, inputs []types.AccumulateInput, X *types.XContext, n common.Hash, logDir string) (r types.Result, res uint64, xs *types.ServiceAccount) {
	vm.Mode = ModeAccumulate
	vm.X = X //⎩I(u, s), I(u, s)⎫⎭
	vm.Y = X.Clone()
	input_bytes := make([]byte, 0)
	t_bytes := types.E(uint64(t))
	s_bytes := types.E(uint64(s))
	o_bytes := types.E(uint64(len(inputs)))

	input_bytes = append(input_bytes, t_bytes...)
	input_bytes = append(input_bytes, s_bytes...)
	input_bytes = append(input_bytes, o_bytes...)
	vm.AccumulateInputs = inputs
	vm.N = n

	x_s, found := X.U.ServiceAccounts[s]
	if !found {
		log.Error(vm.logging, "ExecuteAccumulate - ServiceAccount not found in X.U.ServiceAccounts", "s", s, "X.U.ServiceAccounts", X.U.ServiceAccounts)
		return
	}
	x_s.Mutable = true
	vm.X.U.ServiceAccounts[s] = x_s
	vm.ServiceAccount = x_s

	// Save inputs
	saveToLogDir(logDir, "input", input_bytes)
	// Save AccumulateInputs for inspection
	if inputsEncoded, err := types.Encode(vm.AccumulateInputs); err == nil {
		saveToLogDir(logDir, "accumulate_input", inputsEncoded)
	}

	vm.executeWithBackend(input_bytes, types.EntryPointAccumulate, logDir)
	r, res = vm.getArgumentOutputs()
	vm.destroyVMs()
	saveToLogDirOutput(logDir, r, res)

	return r, res, x_s
}

func (vm *VM) ExecuteAuthorization(p types.WorkPackage, c uint16, logDir string) (r types.Result) {
	vm.Mode = ModeIsAuthorized
	a, _ := types.Encode(uint8(c))

	// Save inputs
	saveToLogDir(logDir, "input", a)

	// fmt.Printf("ExecuteAuthorization - c=%d len(p_bytes)=%d len(c_bytes)=%d len(a)=%d a=%x WP=%s\n", c, len(p_bytes), len(c_bytes), len(a), a, p.String())
	vm.executeWithBackend(a, types.EntryPointAuthorization, logDir)
	r, res := vm.getArgumentOutputs()
	vm.destroyVMs()
	saveToLogDirOutput(logDir, r, res)

	return r
}

func (vm *VM) executeWithBackend(argumentData []byte, entryPoint uint32, logDir string) {
	// fmt.Printf("Standard Program Initialization: %s=%x %s=%x\n", reg(7), argAddr, reg(8), uint32(len(argument_data_a)))
	if vm.ExecutionVM == nil {
		log.Error(vm.logging, "executeWithBackend: ExecutionVM is nil", "entryPoint", entryPoint)
		vm.ResultCode = types.WORKDIGEST_PANIC
		return
	}
	vm.Init(argumentData)
	vm.IsChild = false
	err := vm.ExecutionVM.Execute(vm, entryPoint, logDir)
	if err != nil {
		log.Error(vm.logging, "VM execution failed", "error", err)
	}
	vm.ResultCode = vm.GetResultCode()
	// Note: Do NOT destroy ExecutionVM here - it's needed by getArgumentOutputs() to read registers
	// The caller is responsible for calling destroyVMs() after reading outputs
}

// destroyVMs cleans up all VMs after execution outputs have been read.
// This writes gzip trailers for trace files and frees resources.
// Gas is saved to FinalGas before destroying, so SafeGetGas() can be called after.
func (vm *VM) destroyVMs() {
	if vm.ExecutionVM != nil {
		vm.FinalGas = vm.ExecutionVM.GetGas()
		vm.ExecutionVM.Destroy()
		vm.ExecutionVM = nil
	}
	for _, m := range vm.VMs {
		(*m).Destroy()
	}
	vm.VMs = nil
}

// SafeGetGas returns the gas remaining. It's safe to call after destroyVMs().
// If ExecutionVM is still active, returns live gas. If destroyed, returns saved FinalGas.
func (vm *VM) SafeGetGas() int64 {
	if vm == nil {
		return 0
	}
	if vm.ExecutionVM != nil {
		return vm.ExecutionVM.GetGas()
	}
	return vm.FinalGas
}

// GetGas forwards to SafeGetGas to preserve existing callers while avoiding nil deref
func (vm *VM) GetGas() int64 {
	return vm.SafeGetGas()
}

func (vm *VM) getArgumentOutputs() (r types.Result, res uint64) {
	if vm.ExecutionVM == nil {
		r.Err = types.WORKDIGEST_PANIC
		log.Error(vm.logging, "getArgumentOutputs: ExecutionVM is nil", "mode", vm.Mode, "service", string(vm.ServiceMetadata))
		return r, 0
	}
	if vm.ResultCode == types.WORKDIGEST_OOG {
		r.Err = types.WORKDIGEST_OOG
		log.Debug(vm.logging, "getArgumentOutputs - OOG", "service", string(vm.ServiceMetadata))
		return r, 0
	}
	//o := 0xFFFFFFFF - Z_Z - Z_I + 1
	if vm.ResultCode != types.WORKDIGEST_OK {
		r.Err = vm.ResultCode
		log.Trace(vm.logging, "getArgumentOutputs - Error", "result", vm.ResultCode, "mode", vm.Mode, "service", string(vm.ServiceMetadata))
		return r, 0
	}
	o := vm.ReadRegister(7)
	l := vm.ReadRegister(8)
	output, res := vm.ReadRAMBytes(uint32(o), uint32(l))
	//log.Info(vm.logging, "getArgumentOutputs - OK", "output", fmt.Sprintf("%x", output), "l", l)
	if vm.ResultCode == types.WORKDIGEST_OK && res == 0 {
		r.Ok = output
		return r, res
	}
	if vm.ResultCode == types.WORKDIGEST_OK && res != 0 {
		r.Ok = []byte{}
		return r, res
	}
	r.Err = types.WORKDIGEST_PANIC
	//log.Error(vm.logging, "getArgumentOutputs - PANIC", "result", vm.ResultCode, "mode", vm.Mode, "service", string(vm.ServiceMetadata))
	return r, 0
}

func (vm *VM) GetMachineState() uint8 {
	return vm.MachineState
}

func (vm *VM) GetResultCode() uint8 {
	return vm.ResultCode
}
