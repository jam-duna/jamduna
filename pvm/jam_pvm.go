package pvm

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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

// input by order([work item index],[workpackage itself], [result from IsAuthorized], [import segments], [export count])
func (vm *VM) ExecuteRefine(workitemIndex uint32, workPackage types.WorkPackage, authorization types.Result, importsegments [][][]byte, export_count uint16, extrinsics types.ExtrinsicsBlobs, p_a common.Hash, n common.Hash) (r types.Result, res uint64, exportedSegments [][]byte) {
	vm.Mode = ModeRefine

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
	initialRefineGasLimit := int64(workitem.RefineGasLimit)
	vm.Gas = int64(workitem.RefineGasLimit)
	vm.WorkPackage = workPackage
	vm.N = n
	vm.Authorization = authorization.Ok
	vm.Extrinsics = extrinsics
	vm.Imports = importsegments
	startTime := time.Now()
	initTime := startTime
	switch vm.Backend {
	case BackendInterpreter:
		vm.Standard_Program_Initialization(a) // eq 264/265
		vm.standardInitTime = common.Elapsed(startTime)
		startTime = time.Now()
		vm.Execute(types.EntryPointRefine, false)
		vm.executionTime = common.Elapsed(startTime)
	case BackendSandbox:
		rvm, err := NewCompilerSandboxVM(vm)
		if err != nil {
			log.Error(vm.logging, "CompilerVM creation failed", "error", err)
			return
		}
		vm.initializationTime = common.Elapsed(startTime)
		startTime = time.Now()
		if err = rvm.Standard_Program_Initialization_SandBox(a); err != nil {
			log.Error(vm.logging, "CompilerVM Standard_Program_Initialization failed", "error", err)
			return
		} // eq 264/265
		vm.standardInitTime = common.Elapsed(startTime)
		rvm.ExecuteSandBox(types.EntryPointRefine)
	case BackendCompiler:
		rvm, err := NewCompilerVM(vm)
		if err != nil {
			log.Error(vm.logging, "CompilerVM creation failed", "error", err)
			return
		}
		vm.initializationTime = common.Elapsed(startTime)
		startTime = time.Now()
		if err = rvm.Standard_Program_Initialization(a); err != nil {
			log.Error(vm.logging, "CompilerVM Standard_Program_Initialization failed", "error", err)
			return
		} // eq 264/265
		vm.standardInitTime = common.Elapsed(startTime)
		rvm.Execute(types.EntryPointRefine)
	default:
		log.Crit(vm.logging, "Unknown VM mode", "mode", vm.Backend)
		panic(0)
	}

	r, res = vm.getArgumentOutputs()
	//vm.saveLogs()

	log.Trace(vm.logging, string(vm.ServiceMetadata), "Result", r.String(), "pc", vm.pc, "fault_address", vm.Fault_address, "resultCode", vm.ResultCode)
	exportedSegments = vm.Exports

	AllExecutionTimes := common.Elapsed(initTime)
	if RecordTime {
		serviceMeta := string(vm.ServiceMetadata)
		if serviceMeta != "auth_copy" {
			fmt.Printf("=== VM %s Execution Summary (service %s, %d bytes, refineGasUsed %d)===\n", vm.Backend, serviceMeta, len(vm.code), initialRefineGasLimit-vm.Gas)
			fmt.Printf("%-15s  %-12s  %8s\n", "Phase", "Duration", "Percent")
			phases := []struct {
				name string
				time uint32
			}{
				{"Initialization", vm.initializationTime},
				{"StandardInit", vm.standardInitTime},
				{"Compile", vm.compileTime},
				{"Execution", vm.executionTime},
			}

			for _, p := range phases {
				pct := float64(p.time) / float64(AllExecutionTimes) * 100
				fmt.Printf("%-15s  %-12s  %7.2f%%\n",
					p.name,
					common.FormatElapsed(p.time),
					pct,
				)
			}

			// total doesn’t need a percent
			fmt.Printf("%-15s  %-12s\n",
				"Total",
				common.FormatElapsed(AllExecutionTimes),
			)
		}
	}

	return r, res, exportedSegments
}

var RecordTime = true

func (vm *VM) ExecuteAccumulate(t uint32, s uint32, g uint64, elements []types.AccumulateOperandElements, X *types.XContext, n common.Hash) (r types.Result, res uint64, xs *types.ServiceAccount) {
	vm.Mode = ModeAccumulate
	vm.X = X //⎩I(u, s), I(u, s)⎫⎭
	vm.Y = X.Clone()
	input_bytes := make([]byte, 0)
	t_bytes := types.E(uint64(t))
	s_bytes := types.E(uint64(s))
	o_bytes := types.E(uint64(len(elements))) // TODO: check
	input_bytes = append(input_bytes, t_bytes...)
	input_bytes = append(input_bytes, s_bytes...)
	input_bytes = append(input_bytes, o_bytes...)
	vm.AccumulateOperandElements = elements
	vm.N = n
	vm.Gas = int64(g)
	// (*ServiceAccount, bool, error)

	x_s, found := X.U.ServiceAccounts[s]
	if !found {
		log.Error(vm.logging, "ExecuteAccumulate - ServiceAccount not found in X.U.ServiceAccounts", "s", s, "X.U.ServiceAccounts", X.U.ServiceAccounts)
		return
	}
	x_s.Mutable = true
	vm.X.U.ServiceAccounts[s] = x_s
	vm.ServiceAccount = x_s

	switch vm.Backend {
	case BackendInterpreter:
		vm.Standard_Program_Initialization(input_bytes) // eq 264/265
		vm.Execute(types.EntryPointAccumulate, false)   // F ∈ Ω⟨(X, X)⟩
	case BackendSandbox:
		rvm, err := NewCompilerSandboxVM(vm)
		if err != nil {
			log.Error(vm.logging, "CompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization_SandBox(input_bytes); err != nil {
			log.Error(vm.logging, "CompilerVM Standard_Program_Initialization failed", "error", err)
			return
		}
		rvm.ExecuteSandBox(types.EntryPointAccumulate)
	case BackendCompiler:
		rvm, err := NewCompilerVM(vm)
		if err != nil {
			log.Error(vm.logging, "CompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization(input_bytes); err != nil {
			log.Error(vm.logging, "CompilerVM Standard_Program_Initialization failed", "error", err)
			return
		}
		rvm.Execute(types.EntryPointAccumulate)
	default:
		log.Crit(vm.logging, "Unknown VM mode", "mode", vm.Backend)
		panic(0)
	}
	r, res = vm.getArgumentOutputs()

	return r, res, x_s
}

func (vm *VM) serviceIDlog() string {
	return fmt.Sprintf("%d_%s.json", vm.Service_index, vm.Mode)
}

func (vm *VM) initLogs() {
	if true {
		return
	}

	// ensure the directory exists
	if err := os.MkdirAll(vm.Backend, 0755); err != nil {
		log.Error(vm.logging, "Error ensuring directory exists", "error", err)
		return
	}
	// check if file exists, if not create it with 0 length
	filePath := filepath.Join(vm.Backend, vm.serviceIDlog())
	f, err := os.Create(filePath)
	if err != nil {
		log.Error(vm.logging, "Error creating log file", "file", filePath, "error", err)
		return
	}
	defer f.Close()
}

func (vm *VM) ExecuteTransfer(arguments []byte, service_account *types.ServiceAccount) (r types.Result, res uint64) {
	vm.Mode = ModeOnTransfer
	// a = E(t)   take transfer memos t and encode them
	vm.ServiceAccount = service_account
	switch vm.Backend {
	case BackendInterpreter:
		vm.Standard_Program_Initialization(arguments) // eq 264/265
		vm.Execute(types.EntryPointOnTransfer, false)
	case BackendSandbox:
		rvm, err := NewCompilerSandboxVM(vm)
		if err != nil {
			log.Error(vm.logging, "CompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization_SandBox(arguments); err != nil {
			log.Error(vm.logging, "CompilerVM Standard_Program_Initialization failed", "error", err)
			return
		} // eq 264/265
		rvm.ExecuteSandBox(types.EntryPointOnTransfer)
	case BackendCompiler:
		rvm, err := NewCompilerVM(vm)
		if err != nil {
			log.Error(vm.logging, "CompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization(arguments); err != nil {
			log.Error(vm.logging, "CompilerVM Standard_Program_Initialization failed", "error", err)
			return
		}
		rvm.Execute(types.EntryPointOnTransfer)
	default:
		log.Crit(vm.logging, "Unknown VM mode", "mode", vm.Backend)
		panic(12)
	}
	// return vm.getArgumentOutputs()
	r.Err = vm.ResultCode
	r.Ok = []byte{}
	return r, 0
}

func (vm *VM) ExecuteAuthorization(p types.WorkPackage, c uint16) (r types.Result) {
	vm.Mode = ModeIsAuthorized
	// NOT 0.7.0 COMPLIANT
	a, _ := types.Encode(uint8(c))
	vm.Gas = types.IsAuthorizedGasAllocation
	// fmt.Printf("ExecuteAuthorization - c=%d len(p_bytes)=%d len(c_bytes)=%d len(a)=%d a=%x WP=%s\n", c, len(p_bytes), len(c_bytes), len(a), a, p.String())
	switch vm.Backend {
	case BackendInterpreter:
		vm.Standard_Program_Initialization(a) // eq 264/265
		vm.Execute(types.EntryPointAuthorization, false)
	case BackendSandbox:
		rvm, err := NewCompilerSandboxVM(vm)
		if err != nil {
			log.Error(vm.logging, "CompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization_SandBox(a); err != nil {
			log.Error(vm.logging, "CompilerVM Standard_Program_Initialization failed", "error", err)
			return
		} // eq 264/265
		rvm.ExecuteSandBox(types.EntryPointAuthorization)
	case BackendCompiler:
		rvm, err := NewCompilerVM(vm)
		if err != nil {
			log.Error(vm.logging, "CompilerVM creation failed", "error", err)
			return
		}
		if err = rvm.Standard_Program_Initialization(a); err != nil {
			log.Error(vm.logging, "CompilerVM Standard_Program_Initialization failed", "error", err)
			return
		}
		rvm.Execute(types.EntryPointAuthorization)
	default:
		log.Crit(vm.logging, "Unknown VM mode", "mode", vm.Backend)
		panic(22)
	}
	r, _ = vm.getArgumentOutputs()
	return r
}
func (vm *VM) getArgumentOutputs() (r types.Result, res uint64) {
	if vm.ResultCode == types.WORKDIGEST_OOG {
		r.Err = types.WORKDIGEST_OOG
		log.Error(vm.logging, "getArgumentOutputs - OOG", "service", string(vm.ServiceMetadata))
		return r, 0
	}
	//o := 0xFFFFFFFF - Z_Z - Z_I + 1
	if vm.ResultCode != types.WORKDIGEST_OK {
		r.Err = vm.ResultCode
		log.Trace(vm.logging, "getArgumentOutputs - Error", "result", vm.ResultCode, "mode", vm.Mode, "service", string(vm.ServiceMetadata))
		return r, 0
	}
	o, _ := vm.Ram.ReadRegister(7)
	l, _ := vm.Ram.ReadRegister(8)
	output, res := vm.Ram.ReadRAMBytes(uint32(o), uint32(l))
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
