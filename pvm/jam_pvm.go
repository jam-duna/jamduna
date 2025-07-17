package pvm

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

func (vm *VM) SetServiceIndex(index uint32) {
	vm.Service_index = index
}

func (vm *VM) GetServiceIndex() uint32 {
	return vm.Service_index
}

func (vm *VM) SetCore(coreIndex uint16) {
	vm.CoreIndex = coreIndex
}

var VM_MODE = "interpreter"

// input by order([work item index],[workpackage itself], [result from IsAuthorized], [import segments], [export count])
func (vm *VM) ExecuteRefine(workitemIndex uint32, workPackage types.WorkPackage, authorization types.Result, importsegments [][][]byte, export_count uint16, extrinsics types.ExtrinsicsBlobs, p_a common.Hash, n common.Hash) (r types.Result, res uint64, exportedSegments [][]byte) {
	vm.Mode = "refine"

	workitem := workPackage.WorkItems[workitemIndex]

	// ADD IN 0.6.6
	a := types.E(uint64(workitemIndex))
	//fmt.Printf("ExecuteRefine  workitemIndex %d bytes - %x\n", len(a), a)
	serviceBytes := types.E(uint64(workitem.Service))
	a = append(a, serviceBytes...)
	//fmt.Printf("ExecuteRefine  s %d bytes - %x\n", len(serviceBytes), serviceBytes)
	encoded_workitem_payload, _ := types.Encode(workitem.Payload)
	a = append(a, encoded_workitem_payload...) // variable number of bytes
	//fmt.Printf("ExecuteRefine  payload %d bytes - %x\n", len(encoded_workitem_payload), encoded_workitem_payload)
	a = append(a, workPackage.Hash().Bytes()...) // 32

	//fmt.Printf("ExecuteRefine TOTAL len(a)=%d %x\n", len(a), a)
	vm.WorkItemIndex = workitemIndex
	vm.Gas = int64(workitem.RefineGasLimit)
	vm.WorkPackage = workPackage
	vm.N = n
	vm.Authorization = authorization.Ok
	vm.Extrinsics = extrinsics
	vm.Imports = importsegments
	switch VM_MODE {
	case "interpreter":
		vm.Standard_Program_Initialization(a) // eq 264/265
		vm.Execute(types.EntryPointRefine, false, nil)
	case "recompiler_sandbox":
		rvm, err := NewRecompilerSandboxVM(vm)
		if err != nil {
			log.Error(vm.logging, "RecompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization_SandBox(a); err != nil {
			log.Error(vm.logging, "RecompilerVM Standard_Program_Initialization failed", "error", err)
			return
		} // eq 264/265
		rvm.ExecuteSandBox(types.EntryPointRefine)
	case "recompiler":
		rvm, err := NewRecompilerVM(vm)
		if err != nil {
			log.Error(vm.logging, "RecompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization(a); err != nil {
			log.Error(vm.logging, "RecompilerVM Standard_Program_Initialization failed", "error", err)
			return
		} // eq 264/265
		rvm.Execute(types.EntryPointRefine)
	default:
		log.Error(vm.logging, "Unknown VM mode", "mode", VM_MODE)
	}

	r, res = vm.getArgumentOutputs()
	vm.saveLogs()

	log.Trace(vm.logging, string(vm.ServiceMetadata), "Result", r.String(), "pc", vm.pc, "fault_address", vm.Fault_address, "resultCode", vm.ResultCode)

	exportedSegments = vm.Exports
	return r, res, exportedSegments
}

func (vm *VM) ExecuteAccumulate(t uint32, s uint32, g uint64, elements []types.AccumulateOperandElements, X *types.XContext, n common.Hash) (r types.Result, res uint64, xs *types.ServiceAccount) {

	vm.Mode = "accumulate"
	vm.X = X //⎩I(u, s), I(u, s)⎫⎭
	vm.Y = X.Clone()

	input_bytes := make([]byte, 0)
	t_bytes := types.E(uint64(t))
	s_bytes := types.E(uint64(s))
	o_bytes := types.E(uint64(len(elements)))
	input_bytes = append(input_bytes, t_bytes...)
	input_bytes = append(input_bytes, s_bytes...)
	input_bytes = append(input_bytes, o_bytes...)
	vm.AccumulateOperandElements = elements
	vm.N = n
	vm.Gas = int64(g)
	// (*ServiceAccount, bool, error)
	x_s, ok, err := vm.hostenv.GetService(X.S)
	if !ok || err != nil {
		// fmt.Printf("service %v ok %v err %v\n", X.S, ok, err)
		return
	}
	x_s.Mutable = true
	vm.X.U.D[s] = x_s
	vm.ServiceAccount = x_s
	switch VM_MODE {
	case "interpreter":
		vm.Standard_Program_Initialization(input_bytes)    // eq 264/265
		vm.Execute(types.EntryPointAccumulate, false, nil) // F ∈ Ω⟨(X, X)⟩
	case "recompiler_sandbox":
		rvm, err := NewRecompilerSandboxVM(vm)
		if err != nil {
			log.Error(vm.logging, "RecompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization_SandBox(input_bytes); err != nil {
			log.Error(vm.logging, "RecompilerVM Standard_Program_Initialization failed", "error", err)
			return
		}
		rvm.ExecuteSandBox(types.EntryPointAccumulate)
	case "recompiler":
		rvm, err := NewRecompilerVM(vm)
		if err != nil {
			log.Error(vm.logging, "RecompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization(input_bytes); err != nil {
			log.Error(vm.logging, "RecompilerVM Standard_Program_Initialization failed", "error", err)
			return
		}
		rvm.Execute(types.EntryPointAccumulate)
	default:
		log.Error(vm.logging, "Unknown VM mode", "mode", VM_MODE)
		return
	}
	r, res = vm.getArgumentOutputs()
	x_s.UpdateRecentAccumulation(vm.Timeslot) // [Gratis TODO: potentially need to be moved out]

	vm.saveLogs()

	return r, res, x_s
}

func (vm *VM) initLogs() {
	// decide filename
	fileName := "vm_log"
	dir := VM_MODE
	filePath := filepath.Join(dir, fileName+".json")
	// ensure the directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Error(vm.logging, "Error ensuring directory exists", "error", err)
		return
	}
	// check if file exists, if not create it with 0 length
	f, err := os.Create(filePath)
	if err != nil {
		log.Error(vm.logging, "Error creating log file", "file", filePath, "error", err)
		return
	}
	defer f.Close()
}

// write the vm.Log  (appending only)
func (vm *VM) saveLogs() {
	// decide filename
	dir := VM_MODE
	filePath := filepath.Join(dir, "vm_log.json")

	// ensure the directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Error(vm.logging, "Error ensuring directory exists", "error", err)
		return
	}

	// open for append (create if missing), do NOT truncate
	f, err := os.OpenFile(filePath,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		log.Error(vm.logging, "Error opening log file for append", "file", filePath, "error", err)
		return
	}
	defer f.Close()

	// append each log entry as one JSON object per line
	for _, entry := range vm.Logs {
		data, err := json.Marshal(entry)
		if err != nil {
			log.Error(vm.logging, "Error marshalling vm.Log to JSON", "error", err)
			return
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			log.Error(vm.logging, "Error writing JSON to file", "file", filePath, "error", err)
			return
		}
		//fmt.Println(string(data))
	}
	vm.Logs = make([]VMLog, 0)
}

func (vm *VM) ExecuteTransfer(arguments []byte, service_account *types.ServiceAccount) (r types.Result, res uint64) {
	vm.Mode = "transfer"
	// a = E(t)   take transfer memos t and encode them
	vm.ServiceAccount = service_account
	switch VM_MODE {
	case "interpreter":
		vm.Standard_Program_Initialization(arguments) // eq 264/265
		vm.Execute(types.EntryPointOnTransfer, false, nil)
	case "recompiler_sandbox":
		rvm, err := NewRecompilerSandboxVM(vm)
		if err != nil {
			log.Error(vm.logging, "RecompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization_SandBox(arguments); err != nil {
			log.Error(vm.logging, "RecompilerVM Standard_Program_Initialization failed", "error", err)
			return
		} // eq 264/265
		rvm.ExecuteSandBox(types.EntryPointOnTransfer)
	case "recompiler":
		rvm, err := NewRecompilerVM(vm)
		if err != nil {
			log.Error(vm.logging, "RecompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization(arguments); err != nil {
			log.Error(vm.logging, "RecompilerVM Standard_Program_Initialization failed", "error", err)
			return
		}
		rvm.Execute(types.EntryPointOnTransfer)
	default:
		log.Error(vm.logging, "Unknown VM mode", "mode", VM_MODE)
		return
	}
	// return vm.getArgumentOutputs()
	r.Err = vm.ResultCode
	r.Ok = []byte{}
	return r, 0
}

func (vm *VM) ExecuteAuthorization(p types.WorkPackage, c uint16) (r types.Result) {
	vm.Mode = "authorization"
	// 0.6.6 E_2(c) only
	a, _ := types.Encode(uint16(c))
	vm.Gas = types.IsAuthorizedGasAllocation
	// fmt.Printf("ExecuteAuthorization - c=%d len(p_bytes)=%d len(c_bytes)=%d len(a)=%d a=%x WP=%s\n", c, len(p_bytes), len(c_bytes), len(a), a, p.String())
	switch VM_MODE {
	case "interpreter":
		vm.Standard_Program_Initialization(a) // eq 264/265
		vm.Execute(types.EntryPointAuthorization, false, nil)
	case "recompiler_sandbox":
		rvm, err := NewRecompilerSandboxVM(vm)
		if err != nil {
			log.Error(vm.logging, "RecompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization_SandBox(a); err != nil {
			log.Error(vm.logging, "RecompilerVM Standard_Program_Initialization failed", "error", err)
			return
		} // eq 264/265
		rvm.ExecuteSandBox(types.EntryPointAuthorization)
	case "recompiler":
		rvm, err := NewRecompilerVM(vm)
		if err != nil {
			log.Error(vm.logging, "RecompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization(a); err != nil {
			log.Error(vm.logging, "RecompilerVM Standard_Program_Initialization failed", "error", err)
			return
		}
		rvm.Execute(types.EntryPointAuthorization)
	default:
		log.Error(vm.logging, "Unknown VM mode", "mode", VM_MODE)
		return
	}
	r, _ = vm.getArgumentOutputs()
	return r
}
func (vm *VM) getArgumentOutputs() (r types.Result, res uint64) {
	if vm.ResultCode == types.RESULT_OOG {
		r.Err = types.RESULT_OOG
		log.Debug(vm.logging, "getArgumentOutputs - OOG", "service", string(vm.ServiceMetadata))
		return r, 0
	}
	//o := 0xFFFFFFFF - Z_Z - Z_I + 1
	o, _ := vm.Ram.ReadRegister(7)
	l, _ := vm.Ram.ReadRegister(8)
	output, res := vm.Ram.ReadRAMBytes(uint32(o), uint32(l))
	if vm.ResultCode == types.RESULT_OK && res == 0 {
		r.Ok = output
		return r, res
	}
	if vm.ResultCode == types.RESULT_OK && res != 0 {
		r.Ok = []byte{}
		return r, res
	}
	r.Err = types.RESULT_PANIC
	log.Debug(vm.logging, "getArgumentOutputs - PANIC", "result", vm.ResultCode, "mode", vm.Mode, "service", string(vm.ServiceMetadata))
	return r, 0
}
