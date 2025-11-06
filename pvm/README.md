# PVM FFI Library Design

The PVM backend is a standalone FFI-compatible library with key API operations:
- **Create**: `pvm_create()` - VM initialization with code, registers, PC
- **Destroy**: `pvm_destroy()` - VM cleanup
- **Sync**: Register and state synchronization between C/Go
- **Execute**: `pvm_execute()` - Main interpreter loop
- **Memory**: Shared memory regions with bounds checking
- **Host Calls**: Bidirectional callback system

All state is in the backend with a PVM API:
```c
typedef struct pvm_vm_t pvm_vm_t;  // << this is the handle each language needs
// KEY INTEGRATION for ECALLI: Host Function Callbacks 
typedef void (*pvm_host_callback_t)(pvm_vm_t* vm, int func_id);

// Go: type VM struct { handle *C.pvm_vm_t }
// ... 

// VM Lifecycle
pvm_vm_t* pvm_create(uint32_t service_index, 
                        const uint8_t* code, size_t code_len,
                        const uint64_t* initial_regs,
                        const uint8_t* bitmask, size_t bitmask_len,
                        const uint32_t* jump_table, size_t jump_table_len,
                        uint64_t initial_gas);

// Program Initialization
void pvm_set_host_callback(pvm_vm_t* vm, pvm_host_callback_t callback);
pvm_result_t pvm_execute(pvm_vm_t* vm, uint32_t entry_point);
void pvm_destroy(pvm_vm_t* vm);

typedef enum {
    PVM_RESULT_OK = 0,
    PVM_RESULT_OOG = 1,
    PVM_RESULT_PANIC = 2,
    PVM_RESULT_HOST_CALL = 3
} pvm_result_t;

// Host functions use these to read+write registers+memory
void pvm_set_register(pvm_vm_t* vm, int index, uint64_t value);
uint64_t pvm_get_register(pvm_vm_t* vm, int index);
uint8_t pvm_get_machine_state(pvm_vm_t* vm);
const uint64_t* pvm_get_registers(pvm_vm_t* vm);

// Add memory functions here
```

Memory regions are accessed solely via the above API, specifically in host functions

When the VM hits a host call instruction (in handler_ECALLI), the flow is like this:
1. C calls registered host_callback(vm, func_id)
2. Host language callback executes:
   - Can call pvm_get_register(vm, 7) to read args
   - Can call pvm_set_register(vm, 7, result) to set return value
   - Can access/modify shared memory via API
3. Returns PVM_HOST_CONTINUE or ...
4. C VM resumes execution with updated state
```c


A language binding to the Backend API looks like this:

```go
type VM struct {
    handle *C.pvm_vm_t
}

func (vm *VM) ReadRegister(idx int) uint64 {
    return uint64(C.pvm_get_register(vm.handle, C.int(idx)))  
}

func (vm *VM) SetHostCallback(callback func(vm *VM, funcID int) HostResult) {
    // Store Go callback in global map keyed by VM handle
    hostCallbacks[vm.handle] = callback
    C.pvm_set_host_callback(vm.handle, C.go_host_callback_wrapper, unsafe.Pointer(vm.handle))
}

//export go_host_callback_wrapper
func go_host_callback_wrapper(cvm *C.pvm_vm_t, funcID C.int, userData unsafe.Pointer) C.pvm_host_result_t {
    goCallback := hostCallbacks[cvm]
    vm := &VM{handle: cvm}
    result := goCallback(vm, int(funcID))
    return C.pvm_host_result_t(result)
}
```

### NEXT STEPS
Shawn: Adjust the code to accomodate both the C interpreter and the recompiler using the same backend API.

