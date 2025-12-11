# Taint Tracking System

## Overview

The taint tracking system is a **data flow analysis tool** for the PVM (Polkadot Virtual Machine) that tracks how data propagates through memory and registers during execution. It builds a **Static Single Assignment (SSA)** style graph representing the flow of tainted data, enabling backward tracing to identify the source of any value in memory or registers.

### Key Features

- **SSA-style graph representation**: Each node represents a unique value creation point
- **Memory and register tracking**: Tracks data flow through both memory addresses and registers
- **Backward tracing**: Trace any memory value back to its origin
- **Instruction union analysis**: Identify all unique instructions involved in data propagation
- **Configurable tracking window**: Optionally limit tracking to specific step ranges for performance

### Use Cases

1. **Debugging incorrect computation results**: Find where a wrong value originated
2. **Security analysis**: Track sensitive data flow through the VM
3. **Understanding data dependencies**: Visualize how values influence each other
4. **Performance analysis**: Identify hot paths in data flow

---

## Architecture

### Core Components

#### 1. TaintGraph
The main data structure that stores all taint tracking information.

```go
type TaintGraph struct {
    Nodes     []*TaintNode           // All nodes, indexed by ID-1
    nextID    TaintNodeID            // Next node ID to assign
    regSource [13]TaintNodeID        // Current source node for each register (r0-r12)
    memSource map[uint64]TaintNodeID // Current source node for each memory address
}
```

#### 2. TaintNode
Represents a single point where a new value is created.

```go
type TaintNode struct {
    ID      TaintNodeID      // Unique node identifier
    Kind    TaintNodeKind    // Type of operation (LOAD/STORE/ALU/IMM/EXTERNAL)
    Step    int              // Execution step number
    PC      uint64           // Program counter
    Opcode  byte             // Instruction opcode
    Inputs  []TaintNodeID    // Input nodes this node depends on
    MemAddr uint64           // Memory address (for LOAD/STORE)
    MemSize uint64           // Memory size (for LOAD/STORE)
    RegDest int              // Destination register (for ALU/LOAD)
    RegSrc  int              // Source register (for STORE)
    Value   uint64           // Actual value (for debugging)
}
```

#### 3. TaintNodeKind
Types of operations that create new values:

- **TaintKindExternal**: External input (e.g., from host functions like `hostPoke`)
- **TaintKindLoad**: Memory load operation
- **TaintKindStore**: Memory store operation (tracks the stored value's source)
- **TaintKindALU**: Arithmetic/Logic operation (ADD, XOR, etc.)
- **TaintKindImm**: Immediate/constant value from instruction

#### 4. TaintConfig
Per-VM configuration for taint tracking.

```go
type TaintConfig struct {
    Enabled    bool // Whether taint tracking is active
    TargetStep int  // Target step to trace from (0 = track all)
    StepWindow int  // Window before TargetStep to track
    MaxNodes   int  // Maximum nodes limit (0 = unlimited)
}
```

---

## How It Works

### 1. Node Creation

When an instruction executes, it creates a new **TaintNode** representing the new value:

```
[Instruction] -> [Create Node] -> [Update regSource/memSource mapping]
```

#### Example: ADD instruction

```assembly
ADD r3, r1, r2  ; r3 = r1 + r2
```

**Taint tracking**:
1. Create a new ALU node
2. Set inputs to `[regSource[r1], regSource[r2]]`
3. Update `regSource[r3] = new_node_id`

### 2. Data Flow Tracking

The system maintains two mappings:

- **regSource[reg]**: Points to the node that last wrote to this register
- **memSource[addr]**: Points to the node that last wrote to this memory address

#### Example Flow

```assembly
LOAD_IMM r1, 100      ; Node1: IMM -> r1
LOAD_IMM r2, 200      ; Node2: IMM -> r2
ADD r3, r1, r2        ; Node3: ALU(Node1, Node2) -> r3
STORE r3, [0x1000]    ; Node4: STORE(Node3) -> Mem[0x1000]
```

**Resulting graph**:
```
Node1 (IMM) ---------> Node3 (ALU) ---------> Node4 (STORE)
Node2 (IMM) -------/                             |
                                                 v
                                            Mem[0x1000]
```

### 3. MOV Handling

`MOV` instructions don't create new values - they just **alias** existing values:

```assembly
MOV r2, r1  ; r2 = r1
```

**Taint tracking**: `regSource[r2] = regSource[r1]` (no new node)

### 4. Backward Tracing

To find the source of a value at `Mem[0x1000]`:

1. Look up `memSource[0x1000]` → Get starting node
2. BFS traversal through `node.Inputs`
3. Collect all nodes in the dependency chain
4. Return nodes in execution order

---

## API Reference

### Enabling Taint Tracking

#### `EnableTaintForStep(targetStep, stepWindow int)`

Enables taint tracking for a specific step range.

**Parameters**:
- `targetStep`: The target step to trace from (0 = track all steps)
- `stepWindow`: Number of steps before targetStep to track (e.g., 10000)

**Example**:
```go
// Track all steps (no window restriction)
vm.EnableTaintForStep(0, 0)

// Track steps [990000, 1000000]
vm.EnableTaintForStep(1000000, 10000)
```

**Use case**: When you know a specific step has a problem, track only the relevant window to save memory.

#### `DisableTaint()`

Disables taint tracking for this VM.

```go
vm.DisableTaint()
```

---

### Recording Operations

These methods are called automatically during instruction execution:

#### `TaintRecordImm(dstReg int, opcode byte)`

Records an immediate value load (e.g., `LOAD_IMM r1, 42`).

**Creates**: IMM node with no inputs
**Updates**: `regSource[dstReg]`

#### `TaintRecordALU(dstReg int, srcRegs []int, opcode byte)`

Records an ALU operation (e.g., `ADD r3, r1, r2`).

**Creates**: ALU node with inputs from `srcRegs`
**Updates**: `regSource[dstReg]`

**Example**:
```go
// ADD r3, r1, r2
vm.TaintRecordALU(3, []int{1, 2}, ADD_64)
```

#### `TaintRecordLoad(dstReg int, memAddr, memSize uint64, opcode byte)`

Records a memory load (e.g., `LOAD r1, [0x1000]`).

**Creates**: LOAD node with input from `memSource[memAddr:memAddr+memSize]`
**Updates**: `regSource[dstReg]`

#### `TaintRecordStore(srcReg int, memAddr, memSize uint64, opcode byte)`

Records a memory store (e.g., `STORE r1, [0x1000]`).

**Creates**: STORE node with input from `regSource[srcReg]`
**Updates**: `memSource[memAddr:memAddr+memSize]`

#### `TaintRecordMov(dstReg, srcReg int)`

Records a MOV operation (alias, no new node).

**Updates**: `regSource[dstReg] = regSource[srcReg]`

#### `TaintRecordExternalWrite(memAddr, memSize uint64)`

Records an external write from host functions (e.g., `hostPoke`).

**Creates**: EXTERNAL node with no inputs
**Updates**: `memSource[memAddr:memAddr+memSize]`

---

### Querying and Analysis

#### `PrintTaintTrace(memAddr, memSize uint64, maxDepth int)`

Prints a backward trace from a memory address.

**Parameters**:
- `memAddr`: Starting memory address
- `memSize`: Size of memory region (typically 1 byte)
- `maxDepth`: Maximum trace depth (0 = unlimited)

**Output**:
1. All nodes in the backward trace
2. Warning if trace was truncated
3. Instruction union (unique opcodes used)

**Example**:
```go
// Trace the source of byte at 0x4a1a59
vmgo.PrintTaintTrace(0x4a1a59, 1, 0)
```

**Output format**:
```
=== Backward Trace for Mem[0x4a1a59..0x4a1a59] ===
Total nodes in graph: 174677401
Nodes in trace: 9873

[0] Node120025677[Step=442778 PC=0x76ba3 STORE_IND_U64 STORE Mem[0x4a1a58..0x4a1a5f] r7 -> inputs=[120025627]]
[1] Node120025627[Step=442706 PC=0x11bd LOAD_IND_U64 LOAD Mem[0xfffdf7b8..0xfffdf7bf] -> r5 inputs=[120025611]]
...

=== Instruction Union (Unique Opcodes) ===
Total unique opcodes: 15

[0] LOAD_IND_U64 (opcode=0x15)
[1] STORE_IND_U64 (opcode=0x18)
[2] ADD_64 (opcode=0x2a)
...
```

#### `GetTaintStats()`

Returns statistics about the taint graph.

**Returns**: String with node counts and memory addresses tracked

**Example**:
```go
stats := vmgo.GetTaintStats()
fmt.Println(stats)
// Output: "Nodes: 174677401 (EXTERNAL=413443, LOAD=15895487, STORE=13574665, ALU=143411480, IMM=1382326), MemAddrs tracked: 3510272"
```

---

## Integration Points

### VM Initialization

Taint tracking is enabled during VM creation:

```go
// In pvm.go: NewEmptyExecutionVM()
machine := NewVMGo(service_index, p, initialRegs, initialPC, uint64(initialGas), hostENV)
machine.EnableTaintForStep(0, 0)  // Track all steps
```

### Instruction Handlers

Each instruction handler calls the appropriate taint recording method:

#### Example: LOAD instruction
```go
case LOAD_IND_U64:
    value, errCode := vm.ReadRAMBytes(addr, 8)
    if errCode != OK {
        // handle error
    }
    result = types.DecodeE_l(value)
    vm.TaintRecordLoad(registerIndexA, uint64(addr), 8, inst.Opcode)  // ← Taint tracking
    vm.Ram.WriteRegister(registerIndexA, result)
```

#### Example: ALU instruction
```go
case ADD_64:
    result = valueA + valueB
    vm.TaintRecordALU(registerIndexD, []int{registerIndexA, registerIndexB}, inst.Opcode)  // ← Taint tracking
    vm.Ram.WriteRegister(registerIndexD, result)
```

### Host Functions

External writes must be tracked:

```go
// In hostPoke()
(*m_n).WriteRAMBytes(uint32(o), toWrite)
if vmgo, ok := (*m_n).(*VMGo); ok {
    vmgo.TaintRecordExternalWrite(o, z)  // ← Track external write
}
```

---

## Performance Considerations

### Memory Usage

The taint graph can grow **very large** for long-running programs:

- **Example**: 174 million nodes ≈ several GB of memory
- Each node: ~100-150 bytes (struct + slice overhead)
- `memSource` map: Can have millions of entries

### Optimization Strategies

#### 1. Use Step Windows
Only track steps relevant to debugging:

```go
// Instead of tracking all 1M steps:
vm.EnableTaintForStep(0, 0)  // ~10 GB memory

// Track only last 10k steps:
vm.EnableTaintForStep(1000000, 10000)  // ~100 MB memory
```

#### 2. Set MaxNodes Limit
Prevent unbounded growth:

```go
vm.TaintConfig.MaxNodes = 100000  // Stop after 100k nodes
```

#### 3. Disable When Not Needed
Taint tracking has ~5-10% performance overhead:

```go
vm.DisableTaint()  // No tracking overhead
```

---

## Debugging Examples

### Example 1: Find Source of Wrong Memory Value

**Problem**: Byte at `0x4a1a59` has value `0x18` but expected `0xbd`

**Solution**:
```go
// Enable taint tracking
vm.EnableTaintForStep(0, 0)

// Run VM execution
vm.Execute()

// Trace back the wrong byte
vm.PrintTaintTrace(0x4a1a59, 1, 0)
```

**Output shows**:
- All instructions that contributed to this value
- The original source (IMM, EXTERNAL, or LOAD from elsewhere)
- Step numbers and PC locations for debugging

### Example 2: Track Register Value

To trace a register value, first find when it was written to memory, then trace that memory location.

**Alternative**: Extend the API with `PrintRegisterTrace(regIndex int)` if needed.

---

## Limitations

### Current Limitations

1. **No register-only tracing**: Must trace via memory
2. **Large memory footprint**: Can use several GB for long executions
3. **No compression**: Old nodes are never freed
4. **Sequential only**: Not optimized for concurrent access

### Future Improvements

1. **Incremental GC**: Free nodes older than window
2. **Compression**: Store only recent nodes in detail
3. **Register tracing**: Add `PrintRegisterTrace()` API
4. **Filtering**: Track only specific memory ranges
5. **Export format**: Save graph to file for offline analysis

---

## Implementation Details

### Node ID Assignment

Node IDs are assigned sequentially starting from 1:
- `ID = 0` is reserved for `TaintNodeNone` (uninitialized)
- `ID = 1` is the first node created
- `Nodes[ID-1]` stores the node (array is 0-indexed)

### Fallback Mechanism

If `memSource` lookup fails (e.g., due to map corruption), the system:
1. Prints debug info about nearby addresses
2. Searches the last 10,000 nodes for STORE operations
3. Uses the most recent matching STORE as starting point

This ensures robustness even with very large graphs.

### Instruction Union

The backward trace automatically computes:
- All unique opcodes used in the data flow
- Sorted by opcode value
- Useful for understanding what operations influenced a value

---

## Testing

### Manual Testing

```go
// Create a small test program
vm := NewVMGo(...)
vm.EnableTaintForStep(0, 0)

// Execute some instructions
vm.step(0)  // LOAD_IMM r1, 100
vm.step(1)  // LOAD_IMM r2, 200
vm.step(2)  // ADD r3, r1, r2
vm.step(3)  // STORE r3, [0x1000]

// Verify taint tracking
vm.PrintTaintTrace(0x1000, 8, 0)
// Should show: STORE <- ALU <- IMM (r1) + IMM (r2)
```

### Statistics Verification

```go
stats := vm.GetTaintStats()
// Verify node counts match expected operations
```

---

## Troubleshooting

### Problem: "No taint source found"

**Cause**: The memory address was never written, or written before tracking started

**Solution**:
- Check if address is correct
- Enable tracking earlier (larger step window)
- Check for typos in address (hex vs decimal)

### Problem: "Nodes in trace: 0"

**Causes**:
1. `memSource` map lookup failed
2. Address never written during tracking window
3. Taint tracking was disabled

**Debug**:
- Check debug output for nearby addresses
- Verify `TaintConfig.Enabled = true`
- Check step window covers the write

### Problem: Out of memory

**Cause**: Taint graph too large (millions of nodes)

**Solution**:
- Use smaller step window
- Set `MaxNodes` limit
- Only enable tracking when needed

---

## References

- **SSA Form**: Static Single Assignment representation
- **Data Flow Analysis**: Compiler optimization technique
- **Taint Analysis**: Security analysis technique for tracking data flow

---

## Changelog

### Version 1.0 (Current)
- Initial implementation
- SSA-style graph with backward tracing
- Memory and register tracking
- Instruction union analysis
- Step window optimization
- Fallback for large graphs
