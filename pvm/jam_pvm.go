package pvm

import (
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
func (vm *VM) ExecuteRefine(workitemIndex uint32, workPackage types.WorkPackage, authorization types.Result, importsegments [][][]byte, export_count uint16, extrinsics types.ExtrinsicsBlobs, p_a common.Hash) (r types.Result, res uint64, exportedSegments [][]byte) {
	vm.Mode = "refine"

	workitem := workPackage.WorkItems[workitemIndex]

	// ADD IN 0.6.6
	//a := types.E(uint64(workitemIndex)) -- 0.6.6
	//fmt.Printf("ExecuteRefine  workitemIndex %d bytes - %x\n", len(a), a)
	a := make([]byte, 0)
	serviceBytes := types.E(uint64(workitem.Service))
	a = append(a, serviceBytes...)
	//fmt.Printf("ExecuteRefine  s %d bytes - %x\n", len(serviceBytes), serviceBytes)
	encoded_workitem_payload, _ := types.Encode(workitem.Payload)
	a = append(a, encoded_workitem_payload...) // variable number of bytes
	//fmt.Printf("ExecuteRefine  payload %d bytes - %x\n", len(encoded_workitem_payload), encoded_workitem_payload)
	a = append(a, workPackage.Hash().Bytes()...) // 32
	//fmt.Printf("ExecuteRefine  workPackage %d bytes - %x\n", len(workPackage.Hash().Bytes()), workPackage.Hash().Bytes())

	encoded_workPackage_RefineContext, _ := types.Encode(workPackage.RefineContext)
	a = append(a, encoded_workPackage_RefineContext...)
	//fmt.Printf("ExecuteRefine  refineContext %d bytes - %x\n", len(encoded_workPackage_RefineContext), encoded_workPackage_RefineContext)

	encoded_p_a, _ := types.Encode(p_a)
	//fmt.Printf("ExecuteRefine  p_a %d bytes - %x\n", len(encoded_p_a), encoded_p_a)
	a = append(a, encoded_p_a...)
	//fmt.Printf("ExecuteRefine TOTAL len(a)=%d %x\n", len(a), a)
	vm.WorkItemIndex = workitemIndex
	vm.Gas = int64(workitem.RefineGasLimit)
	vm.WorkPackage = workPackage

	// Sourabh , William pls validate this
	vm.Authorization = authorization.Ok
	//===================================
	vm.Extrinsics = extrinsics
	vm.Imports = importsegments

	vm.Standard_Program_Initialization(a) // eq 264/265
	vm.Execute(types.EntryPointRefine, false)
	r, res = vm.getArgumentOutputs()

	log.Trace(vm.logging, string(vm.ServiceMetadata), "Result", r.String(), "pc", vm.pc, "fault_address", vm.Fault_address, "resultCode", vm.ResultCode)

	exportedSegments = vm.Exports
	return r, res, exportedSegments
}

func (vm *VM) ExecuteAccumulate(t uint32, s uint32, g uint64, elements []types.AccumulateOperandElements, X *types.XContext) (r types.Result, res uint64, xs *types.ServiceAccount) {

	vm.Mode = "accumulate"
	vm.X = X //⎩I(u, s), I(u, s)⎫⎭
	vm.Y = X.Clone()

	input_bytes := make([]byte, 0)
	t_bytes := types.E(uint64(t))
	s_bytes := types.E(uint64(s))
	encoded_elements, _ := types.Encode(elements)
	input_bytes = append(input_bytes, t_bytes...)
	input_bytes = append(input_bytes, s_bytes...)
	input_bytes = append(input_bytes, encoded_elements...)

	vm.Standard_Program_Initialization(input_bytes) // eq 264/265
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
	vm.Execute(types.EntryPointAccumulate, false) // F ∈ Ω⟨(X, X)⟩
	r, res = vm.getArgumentOutputs()

	return r, res, x_s
}
func (vm *VM) ExecuteTransfer(arguments []byte, service_account *types.ServiceAccount) (r types.Result, res uint64) {
	vm.Mode = "transfer"
	// a = E(t)   take transfer memos t and encode them
	vm.ServiceAccount = service_account

	vm.Standard_Program_Initialization(arguments) // eq 264/265
	vm.Execute(types.EntryPointOnTransfer, false)
	// return vm.getArgumentOutputs()
	r.Err = vm.ResultCode
	r.Ok = []byte{}
	return r, 0
}

func (vm *VM) ExecuteAuthorization(p types.WorkPackage, c uint16) (r types.Result) {
	vm.Mode = "authorization"
	// 0.6.6 E_2(c) only
	// a := common.Uint16ToBytes(c)
	// 0.6.5 E(p_p, p, c)  (non-GP Compliant)
	p_p, _ := types.Encode(p.ParameterizationBlob)
	p_bytes, _ := types.Encode(p)
	c_bytes, _ := types.Encode(uint16(c))
	a := append(p_p, p_bytes...)
	a = append(a, c_bytes...)
	vm.Gas = types.IsAuthorizedGasAllocation
	// fmt.Printf("ExecuteAuthorization - c=%d len(p_bytes)=%d len(c_bytes)=%d len(a)=%d a=%x WP=%s\n", c, len(p_bytes), len(c_bytes), len(a), a, p.String())
	vm.Standard_Program_Initialization(a) // eq 264/265
	vm.Execute(types.EntryPointAuthorization, false)
	r, _ = vm.getArgumentOutputs()
	return r
}
func (vm *VM) getArgumentOutputs() (r types.Result, res uint64) {
	if vm.ResultCode == types.PVM_OOG {
		r.Err = types.RESULT_OOG
		log.Debug(vm.logging, "getArgumentOutputs - OOG", "service", string(vm.ServiceMetadata))
		return r, 0
	}
	//o := 0xFFFFFFFF - Z_Z - Z_I + 1
	o, _ := vm.ReadRegister(7)
	l, _ := vm.ReadRegister(8)
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
