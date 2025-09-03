# PVM-C High Performance Integration Specification


1. **Hybrid Execution Model**: PVM instruction interpretation happens entirely in C for maximum performance
2. **Minimal Data Copying**: Memory regions are shared between C and Go using pointer-based access
3. **Clean Trampoline Interface**: ECALLI opcodes cleanly trampoline from C to Go's `InvokeHostFunction`
4. **Register Sharing**: 13 uint64 registers are accessible from both C and Go sides

### System Components

```
┌─────────────────┐    CGO FFI      ┌──────────────────┐
│  Go NewVM()     │ ◄─────────────► │  C vm_new()      │
│  - Memory Mgmt  │                 │  - Fast Interp   │
│  - Host Funcs   │                 │  - Dispatch Loop │
│  - Registers    │                 │  - Memory Access │
└─────────────────┘                 └──────────────────┘
        ▲                                    │
        │                                    │
        │            ECALLI Trampoline       │
        └────────────────────────────────────┘
```
### VM Structure 

The existing C `VM` struct in `pvm.h:57-103` 


```c

struct VM {
    uint64_t registers[REG_SIZE];
    uint64_t pc;
    uint8_t* code;
    uint8_t* bitmask;
    uint32_t* ram_read_write;
    uint32_t* ram_read_only;
    uint64_t gas;
    int host_call;
    int host_func_id;
    int is_child;
    uint32_t heap_pointer;
    
    // Memory regions
    uint8_t* rw_data;
    uint8_t* ro_data;
    uint8_t* output;
    uint8_t* stack;
    uint32_t rw_data_address;
    uint32_t rw_data_address_end;
    uint32_t ro_data_address;
    uint32_t ro_data_address_end;
    uint32_t output_address;
    uint32_t output_end;
    uint32_t stack_address;
    uint32_t stack_address_end;
    uint32_t current_heap_pointer;
    
    // Code and data segments
    uint32_t code_len;
    uint32_t bitmask_len;
    uint8_t* o_byte;
    uint32_t o_size;
    uint8_t* w_byte;
    uint32_t w_size;
    uint32_t z_val;
    
    // VM state
    int result_code;
    int machine_state;
    int terminated;
    uint32_t fault_address;
    uint32_t j_size;
    uint32_t* j;
    uint32_t service_index;
    uint32_t core_index;
    
    // CGO Integration - Go memory pointers (shared)
    uint8_t* go_rw_data;            // Pointer to Go's rw_data slice
    uint8_t* go_ro_data;            // Pointer to Go's ro_data slice  
    uint8_t* go_output;             // Pointer to Go's output slice
    uint8_t* go_stack;              // Pointer to Go's stack slice
    
    // Go callback function pointer for host calls
    void (*go_invoke_host_func)(struct VM* vm, int host_func_id);
    
    // State synchronization
    int sync_registers_to_go;       // Flag to sync registers back to Go
};
```


### Go-Side Integration
The Go `VM` struct is extended with:

```go
type VM struct {
    // Existing fields...
    
	// CGO Integration
	cVM     unsafe.Pointer         // Pointer to C VM struct
	cRegs   *[13]C.uint64_t       // Direct pointer to C registers
	
	// Memory region pointers (shared with C)
	cRWDataPtr  *C.uint8_t        // C pointer to rw_data
	cRODataPtr  *C.uint8_t        // C pointer to ro_data  
	cOutputPtr  *C.uint8_t        // C pointer to output
	cStackPtr   *C.uint8_t        // C pointer to stack

	// Synchronization control
	syncRegisters bool             // Flag to control register sync
}
```

- C VM Access: Direct pointer arithmetic, Range-checked memory access, No allocation/deallocation
- Registers: C VM maintains authoritative register state during interpretation, Go maintains a direct pointer (`cRegs`) to C's register array
- Minimal copying only when transitioning between C interpretation and Go host functions

### Memory Management Strategy: Zero-Copy Memory Sharing

1. **Slice Header Extraction**: Go slice headers are converted to C pointers
2. **Direct Memory Access**: C VM accesses Go-managed memory directly via pointers
3. **Bounds Checking**: C-side maintains address ranges for safety
4. **Ownership Model**: Go retains memory ownership; C operates on borrowed references


### 2. Execute Function 

The main VM execution loop `pvm_execute` does all the work of execution

The dispatch_table is initialized at the start of pvm_execute.

The dispatch to an op code handler is able to read the registers natively.  The dump functions are triggerred by a "pvm_trace" flag, which we should be able to set from the outside.




```c
opcode_handler_t dispatch_table[256];

void init_dispatch_table(void) {
    // Initialize all entries to trap handler
    for (int i = 0; i < 256; i++) {
        dispatch_table[i] = handle_TRAP;
    }
    
    // Set specific handlers
    dispatch_table[TRAP] = handle_TRAP;
    dispatch_table[FALLTHROUGH] = handle_FALLTHROUGH;
    dispatch_table[ECALLI] = handle_ECALLI;
    dispatch_table[LOAD_IMM_64] = handle_LOAD_IMM_64;
    // ... continue for all opcodes
    dispatch_table[220] = handle_OR;  // Based on constants
    // ... etc
}
...

void handle_OR(VM* vm, uint8_t* operands, size_t operand_len) {
    int register_index_a = MIN(12, (int)(operands[0] & 0x0F));
    int register_index_b = MIN(12, (int)(operands[0] >> 4));
    int register_index_d = MIN(12, (int)operands[1]);
    
    uint64_t value_a = vm->register[register_index_a];
    uint64_t value_b = vm->register[register_index_b];
    uint64_t result = value_a | value_b;
    
#if PVM_TRACE
dump_three_reg_op("|", register_index_d, register_index_a, 
                         register_index_b, value_a, value_b, result);
#endif    
    vm->register[register_index_d] = result;
    vm->pc += 1 + operand_len;
}
```

- Error Handling: error propagation (`return -1`, set error codes) so FFIing is easy

### API Functions

The following methods of C are the FFIable API.

```c
// VM lifecycle
VM* pvm_create(uint32_t service_index, 
               uint8_t* code, size_t code_len,
               uint64_t* initial_regs, size_t num_regs,
               uint64_t initial_pc) {
    VM* vm = (VM*)calloc(1, sizeof(VM));
    if (!vm) return NULL;

    vm->service_index = service_index;
    vm->code = code;  // Share pointer, don't copy
    vm->code_len = code_len;
    vm->pc = initial_pc;
    vm->core_index = 2048;
    vm->gas = 0;
    vm->terminated = 0;
    vm->result_code = WORKDIGEST_OK;
    vm->machine_state = HALT;

    // Initialize registers
    for (size_t i = 0; i < MIN(num_regs, REG_SIZE); i++) {
        vm->registers[i] = initial_regs[i];
    }
    
    // Initialize CGO integration fields
    vm->go_rw_data = NULL;
    vm->go_ro_data = NULL;
    vm->go_output = NULL;
    vm->go_stack = NULL;
    vm->go_invoke_host_func = NULL;
    vm->sync_registers_to_go = 0;

    return vm;
}

void vm_destroy(VM* vm);

// Memory region setup (called from Go)
void vm_set_memory_regions(VM* vm,
                          uint8_t* rw_data, uint32_t rw_size,
                          uint8_t* ro_data, uint32_t ro_size,
                          uint8_t* output, uint32_t output_size,
                          uint8_t* stack, uint32_t stack_size);

// Set memory addresses for bounds checking
void vm_set_memory_bounds(VM* vm,
                         uint32_t rw_addr, uint32_t rw_end,
                         uint32_t ro_addr, uint32_t ro_end,
                         uint32_t output_addr, uint32_t output_end,
                         uint32_t stack_addr, uint32_t stack_end);

// Register access
uint64_t* vm_get_registers_ptr(VM* vm);
void vm_set_register(VM* vm, int reg_idx, uint64_t value);
uint64_t vm_get_register(VM* vm, int reg_idx);

// Execution
int vm_execute(VM* vm, int entry_point, int is_child);

// Host call callback setup  
void vm_set_host_callback(VM* vm, 
                         void (*callback)(VM* vm, int host_func_id));

// Memory operations
uint64_t vm_write_ram_bytes_64(VM* vm, uint32_t address, uint64_t data);
uint64_t vm_read_ram_bytes_64(VM* vm, uint32_t address, int* error_code);
uint64_t vm_write_ram_bytes_32(VM* vm, uint32_t address, uint32_t data);
uint32_t vm_read_ram_bytes_32(VM* vm, uint32_t address, int* error_code);
uint64_t vm_write_ram_bytes_16(VM* vm, uint32_t address, uint16_t data);
uint16_t vm_read_ram_bytes_16(VM* vm, uint32_t address, int* error_code);
uint64_t vm_write_ram_bytes_8(VM* vm, uint32_t address, uint8_t data);
uint8_t vm_read_ram_bytes_8(VM* vm, uint32_t address, int* error_code);
uint64_t vm_write_ram_bytes(VM* vm, uint32_t address, uint8_t* data, uint32_t length);
uint64_t vm_read_ram_bytes(VM* vm, uint32_t address, uint8_t* buffer, uint32_t length);
```

### Go CGO Bindings

Critically, when the C interpreter handles an ECALLI, it call the Go side with `vm->go_invoke_host_func` which is actually registered with `pvm_set_host_callback`



### C-Side Handler Enhancement

The ECALLI (external call immediate) instruction provides a seamless trampoline from C execution to Go host functions. The C implementation extracts the host function ID and makes the callback to Go:

```c
void handle_ECALLI(VM* vm, uint8_t* operands, size_t operand_len) {
    // Extract host function ID from operands
    uint32_t host_func_id;
    if (operand_len >= 8) {
        host_func_id = (uint32_t)decode_operand_slice(operands, 8);
    } else {
        uint64_t decoded = decode_operand_slice(operands, operand_len);
        host_func_id = (uint32_t)decoded;
    }
    
    // Set host call state
    vm->host_call = 1;
    vm->host_func_id = (int)host_func_id;
    
    // Advance program counter
    vm->pc += 1 + operand_len;
    
    // Skip host calls in child VM execution
    if (vm->is_child) {
        vm->host_call = 0;
        return;
    }
    
    // Charge gas for host function call
    int64_t gas_cost = vm_charge_gas(vm, vm->host_func_id);
    vm->gas -= gas_cost;
    
    // Check for out of gas condition
    if (vm->gas < 0) {
        vm->result_code = WORKDIGEST_OOG;
        vm->machine_state = OOG;
        vm->terminated = 1;
        vm->host_call = 0;
        return;
    }
    
#if PVM_TRACE
        printf("ECALLI: Host function %d, gas_cost=%ld, remaining_gas=%ld\n", 
               vm->host_func_id, gas_cost, vm->gas);
    }
    
    // Trampoline to Go's host function via callback
    if (vm->go_invoke_host_func != NULL) {
        // The callback will handle register sync and error management
        vm->go_invoke_host_func(vm, vm->host_func_id);
    } else {
        // No host function callback set - this is an error
        vm->result_code = WORKDIGEST_PANIC;
        vm->machine_state = PANIC;
        vm->terminated = 1;
#if PVM_TRACE
            printf("ERROR: No host function callback set for ECALLI %d\n", vm->host_func_id);
        }
    }
    
    // Clear host call state
    vm->host_call = 0;
    
    // Check if host function caused termination or panic
    if (vm->terminated || vm->machine_state == PANIC) {
#if PVM_TRACE
            printf("Host function %d caused termination (state=%d, code=%d)\n", 
                   vm->host_func_id, vm->machine_state, vm->result_code);
        }
    }
}
```

```c
void pvm_set_host_callback(VM* vm, 
                          void (*callback)(VM* vm, int host_func_id)) {
    if (!vm) return;
    vm->go_invoke_host_func = callback;
}
```

On the go side: we get the host function ID:

```go
//export goInvokeHostFunction
func goInvokeHostFunction(cvm unsafe.Pointer, hostFuncID C.int) {
    // Get VM from global registry using C VM pointer
    goVMs.mu.RLock()
    vm, exists := goVMs.vms[cvm]
    goVMs.mu.RUnlock()
    
    if !exists {
        fmt.Printf("ERROR: VM not found in registry for host call %d\n", hostFuncID)
        return
    }
    
    // Sync C registers to Go before host call
    vm.SyncVMState("C_to_Go")
    
    // Validate host call before execution
    if err := vm.ValidateHostCall(int(hostFuncID)); err != nil {
        fmt.Printf("Host call validation failed: %v\n", err)
        return
    }
    
    // Invoke Go host function with proper error handling
    defer func() {
        if r := recover(); r != nil {
            fmt.Printf("Host function %d panicked: %v\n", hostFuncID, r)
            vm.MachineState = PANIC
            vm.ResultCode = PANIC
        }
    }()
    
    vm.hostenv.InvokeHostCall(int(hostFuncID), vm)
    
    // Sync Go state back to C after host call
    vm.SyncVMState("Go_to_C")
}
```

This is registered:

```go
func (vm *VM) initCGOIntegration() error {
    if len(vm.code) == 0 || len(vm.bitmask) == 0 {
        return fmt.Errorf("code must be set before CGO integration")
    }
    ...
    // Set host function callback
    C.vm_set_host_callback((*C.VM)(vm.cVM), C.goInvokeHostFunction)
    
    // Register VM for host function callbacks
    goVMs.mu.Lock()
    goVMs.vms[vm.cVM] = vm
    goVMs.mu.Unlock()
    
    return nil
}

// Register synchronization functions
func (vm *VM) SyncRegistersToC() {
    if vm.cRegs == nil || vm.cVM == nil {
        return
    }
    for i := 0; i < 13 && i < len(vm.register); i++ {
        vm.cRegs[i] = C.uint64_t(vm.register[i])
    }
}

func (vm *VM) SyncRegistersFromC() {
    if vm.cRegs == nil || vm.cVM == nil {
        return
    }
    for i := 0; i < 13 && i < len(vm.register); i++ {
        vm.register[i] = uint64(vm.cRegs[i])
    }
}

// VM state synchronization
func (vm *VM) SyncVMState(direction string) {
    if vm.cVM == nil {
        return
    }
    
    cvm := (*C.VM)(vm.cVM)
    
    switch direction {
    case "C_to_Go":
        // Sync C VM state to Go VM
        vm.pc = uint64(cvm.pc)
        vm.Gas = int64(cvm.gas)
        vm.terminated = cvm.terminated != 0
        vm.MachineState = uint8(cvm.machine_state)
        vm.ResultCode = uint8(cvm.result_code)
        vm.SyncRegistersFromC()
        
    case "Go_to_C":
        // Sync Go VM state to C VM
        cvm.pc = C.uint64_t(vm.pc)
        cvm.gas = C.uint64_t(vm.Gas)
        cvm.terminated = 0
        if vm.terminated {
            cvm.terminated = 1
        }
        cvm.machine_state = C.int(vm.MachineState)
        cvm.result_code = C.int(vm.ResultCode)
        vm.SyncRegistersToC()
    }
}

// Host call validation
func (vm *VM) ValidateHostCall(hostFuncID int) error {
    if vm.terminated {
        return fmt.Errorf("cannot invoke host function %d: VM is terminated", hostFuncID)
    }
    
    if vm.MachineState == PANIC {
        return fmt.Errorf("cannot invoke host function %d: VM is in PANIC state", hostFuncID)
    }
    
    if vm.Gas < 0 {
        return fmt.Errorf("cannot invoke host function %d: out of gas", hostFuncID)
    }
    
    return nil
}

// Main execution function
func (vm *VM) ExecuteWithCGO() error {
    if vm.cVM == nil {
        if err := vm.initCGOIntegration(); err != nil {
            return err
        }
        defer vm.destroyCGOIntegration()
    }
    
    // Sync Go state to C before execution
    vm.SyncVMState("Go_to_C")
    
    // Execute in C VM
    result := C.vm_execute((*C.VM)(vm.cVM), C.int(vm.pc), 0)
    
    // Sync C state back to Go after execution
    vm.SyncVMState("C_to_Go")
    
    if result < 0 {
        return fmt.Errorf("C VM execution failed with code %d", result)
    }
    
    return nil
}
```

## ECALLI Trampoline Implementation


### Host Function Error Handling

The trampoline includes comprehensive error handling for various failure scenarios:

- **Out of Gas**: Checks gas availability before and after host calls  
- **Invalid Host Function**: Validates host function IDs
- **Panic Recovery**: Catches panics in Go host functions
- **State Consistency**: Maintains VM state across C/Go boundary
- **Resource Cleanup**: Ensures proper cleanup on errors

### Integration with memory.go

The existing Go memory functions in `memory.go` provide a hybrid approach that uses C implementations when CGO integration is active, with fallback to pure Go implementations:

```go
func (vm *VM) WriteRAMBytes64(address uint32, data uint64) uint64 {
    // Use C implementation if CGO integration is active
    if vm.cVM != nil {
        return uint64(C.vm_write_ram_bytes_64((*C.VM)(vm.cVM), C.uint32_t(address), C.uint64_t(data)))
    }

    // Fall back to Go implementation
    heapEnd := Z_func(vm.current_heap_pointer)

    // RW data / heap region (writable up to heapEnd)
    if address >= vm.rw_data_address && address <= heapEnd-8 {
        offset := address - vm.rw_data_address
        if offEnd := offset + 8; offEnd > uint32(len(vm.rw_data)) {
            return OOB
        }
        binary.LittleEndian.PutUint64(vm.rw_data[offset:offset+8], data)
        return OK
    }

    // Output region, stack region, and RO data region handling...
    // (Similar pattern for all memory regions)

    return OOB
}

func (vm *VM) ReadRAMBytes64(address uint32) (uint64, uint64) {
    // Use C implementation if CGO integration is active
    if vm.cVM != nil {
        var errCode C.int
        result := C.vm_read_ram_bytes_64((*C.VM)(vm.cVM), C.uint32_t(address), &errCode)
        return uint64(result), uint64(errCode)
    }

    // Fall back to Go implementation
    heapEnd := Z_func(vm.current_heap_pointer)

    // RW data / heap
    if address >= vm.rw_data_address && address <= heapEnd-8 {
        offset := address - vm.rw_data_address
        return binary.LittleEndian.Uint64(vm.rw_data[offset : offset+8]), OK
    }

    // RO data, output, and stack region handling...
    // (Similar pattern for all memory regions)

    return 0, OOB
}

// Complete set of memory operations with hybrid Go/C implementation
func (vm *VM) WriteRAMBytes32(address uint32, data uint32) uint64 { ... }
func (vm *VM) WriteRAMBytes16(address uint32, data uint16) uint64 { ... }
func (vm *VM) WriteRAMBytes8(address uint32, data uint8) uint64 { ... }
func (vm *VM) WriteRAMBytes(address uint32, data []byte) uint64 { ... }

func (vm *VM) ReadRAMBytes32(address uint32) (uint32, uint64) { ... }
func (vm *VM) ReadRAMBytes16(address uint32) (uint16, uint64) { ... }
func (vm *VM) ReadRAMBytes8(address uint32) (uint8, uint64) { ... }
func (vm *VM) ReadRAMBytes(address uint32, length uint32) ([]byte, uint64) { ... }
```

### Key Integration Features

**Hybrid Architecture**: Each memory function checks for CGO availability and uses the appropriate backend:
- **C Backend**: Direct C function calls for maximum performance
- **Go Backend**: Pure Go implementation as fallback for compatibility

**Memory Safety**: Both implementations include:
- **Bounds Checking**: Address range validation for all operations
- **Region Validation**: Separate handling for RW, RO, output, and stack regions  
- **Error Handling**: Consistent OOB (out-of-bounds) error reporting

**Zero-Copy Optimization**: When using the C backend:
- Memory regions are shared between Go and C via direct pointers
- No data copying between implementations
- Direct pointer arithmetic in C for maximum performance

**Consistent Interface**: Regardless of backend used:
- Same function signatures and return values
- Same error codes and behavior
- Seamless switching between implementations
