# PVM FFI Library Design

We will map the CGo PVM interpreter into a standalone FFI-compatible library with key API operations:
- **Create**: `pvm_create()` - VM initialization with code, registers, PC
- **Destroy**: `pvm_destroy()` - VM cleanup
- **Sync**: Register and state synchronization between C/Go
- **Execute**: `pvm_execute()` - Main interpreter loop
- **Memory**: Shared memory regions with bounds checking
- **Host Calls**: Bidirectional callback system

We will have a single authoritative state in C with direct C function calls for state access, so host languages never hold stale state, with the same sync model works for all FFI languages.  Critically, **we want the recompiler to have the same API**

The VM state lives entirely in C:

```c
typedef struct pvm_vm_t pvm_vm_t;  // << this is the handle each language needs

// Go:   type VM struct { handle *C.pvm_vm_t }
```


Core PVM API:

```c
// VM Lifecycle
typedef struct pvm_vm_t pvm_vm_t;
pvm_vm_t* pvm_vm_create(uint32_t service_index, 
                        const uint8_t* code, size_t code_len,
                        const uint64_t* initial_regs, size_t num_regs,
                        uint64_t initial_pc);
pvm_result_t pvm_vm_execute(pvm_vm_t* vm, uint32_t entry_point);
void pvm_vm_destroy(pvm_vm_t* vm);
typedef enum {
    PVM_RESULT_OK = 0,
    PVM_RESULT_OOG = 1,
    PVM_RESULT_PANIC = 2,
    PVM_RESULT_HOST_CALL = 3
} pvm_result_t;

// Host Function Callbacks <<< KEY INTEGRATION >>>
typedef void (*pvm_host_callback_t)(pvm_vm_t* vm, int func_id, void* userdata);
void pvm_vm_set_host_callback(pvm_vm_t* vm, pvm_host_callback_t callback, void* userdata);

// Program Initialializaion
void pvm_vm_set_bitmask(pvm_vm_t* vm, const uint8_t* bitmask, size_t len);
void pvm_vm_set_jump_table(pvm_vm_t* vm, const uint32_t* table, size_t len);
void pvm_vm_set_gas(pvm_vm_t* vm, uint64_t gas); 
void pvm_vm_set_memory_regions(pvm_vm_t* vm,
    uint8_t* rw_data, uint32_t rw_size,
    uint8_t* ro_data, uint32_t ro_size, 
    uint8_t* output, uint32_t output_size,
    uint8_t* stack, uint32_t stack_size);
// Host functions use these 
uint64_t pvm_vm_get_gas(pvm_vm_t* vm);
uint64_t pvm_vm_get_pc(pvm_vm_t* vm);
uint8_t pvm_vm_get_machine_state(pvm_vm_t* vm);
void pvm_vm_set_register(pvm_vm_t* vm, int index, uint64_t value);
uint64_t pvm_vm_get_register(pvm_vm_t* vm, int index);
const uint64_t* pvm_vm_get_registers(pvm_vm_t* vm);

// WriteRAMBytes functions uses this, with GetHeapPointer abstraction maybe
uint32_t pvm_vm_get_heap_pointer(pvm_vm_t* vm);
```

Memory regions are **shared pointers** so WriteRAMBytes can access rwData and check that its within the valid :

```go
currentHeap := vm.GetHeapPointer()
if currentHeap + length > vm.rwDataEnd {
    return OOB
}
vm.rwData[address] = value
```


When the VM hits a host call instruction in C in handler_ECALLI, the flow is like this:

1. VM executing in C interpreter loop
2. Encounters ECALLI instruction  
3. C calls registered host_callback(vm, func_id, user_data)
4. Host language callback executes:
   - Can call pvm_vm_get_register(vm, 7) to read args
   - Can call pvm_vm_set_register(vm, 7, result) to set return value
   - Can access/modify shared memory directly
5. Returns PVM_HOST_CONTINUE
6. C VM resumes execution with updated state


```c

typedef enum {
... TODO
} pvm_host_result_t;

typedef pvm_host_result_t (*pvm_host_callback_t)(
    pvm_vm_t* vm,           // VM handle for state access
    int func_id,            // Host function ID
    void* user_data         // User context
);

void pvm_vm_set_host_callback(pvm_vm_t* vm, pvm_host_callback_t callback, void* user_data);
```


The key handle_ECALLI:

```c
...
    if (vm->host_callback) {
        // VM state is already current (no sync needed)
        pvm_host_result_t result = vm->host_callback(vm, host_func_id, vm->user_data);
        
        switch (result) {
            case PVM_HOST_TERMINATE:
                vm->terminated = 1;
                return PVM_RESULT_OK;
            case PVM_HOST_ERROR:
                vm->machine_state = PANIC;
                return PVM_RESULT_PANIC;
            default:
                continue;
        }
    } else {
        // No host callback registered
        vm_panic(vm, HOST_ERROR);
    }
...
```

A language binding to the API looks like this:

```go
type VM struct {
    handle *C.pvm_vm_t
}

// State access - direct C calls, no caching
func (vm *VM) GetPC() uint64 {
    return uint64(C.pvm_vm_get_pc(vm.handle))
}

func (vm *VM) ReadRegister(idx int) uint64 {
    return uint64(C.pvm_vm_get_register(vm.handle, C.int(idx)))  
}

func (vm *VM) GetHeapPointer() uint32 {
    return uint32(C.pvm_vm_get_heap_pointer(vm.handle))
}

func (vm *VM) SetHeapPointer(pointer uint32) {
    C.pvm_vm_set_heap_pointer(vm.handle, C.uint32_t(pointer))
}

func (vm *VM) SetHostCallback(callback func(vm *VM, funcID int) HostResult) {
    // Store Go callback in global map keyed by VM handle
    hostCallbacks[vm.handle] = callback
    C.pvm_vm_set_host_callback(vm.handle, C.go_host_callback_wrapper, unsafe.Pointer(vm.handle))
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
Shawn: Adjust this to fit both the C interpreter and the recompiler.

