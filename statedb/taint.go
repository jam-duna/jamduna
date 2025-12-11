package statedb

import (
	"fmt"
	"sort"
)

// TaintConfig holds configuration for taint tracking on a specific VM
type TaintConfig struct {
	Enabled    bool // Whether taint tracking is active for this VM
	TargetStep int  // Target step to trace back from (0 = disabled)
	StepWindow int  // Number of steps before TargetStep to track (e.g., 10000)
	MaxNodes   int  // Maximum number of nodes (0 = unlimited)
}

// TaintNodeID is a unique identifier for a taint node
type TaintNodeID int

const (
	TaintNodeNone TaintNodeID = 0 // No source / uninitialized
)

// TaintNodeKind represents the type of operation that created this node
type TaintNodeKind int

const (
	TaintKindExternal TaintNodeKind = iota // External input (hostPoke, initial memory)
	TaintKindLoad                          // Memory load
	TaintKindStore                         // Memory store (node represents the stored value's source)
	TaintKindALU                           // ALU operation (ADD, SUB, XOR, etc.)
	TaintKindImm                           // Immediate/constant value
)

func (k TaintNodeKind) String() string {
	switch k {
	case TaintKindExternal:
		return "EXTERNAL"
	case TaintKindLoad:
		return "LOAD"
	case TaintKindStore:
		return "STORE"
	case TaintKindALU:
		return "ALU"
	case TaintKindImm:
		return "IMM"
	default:
		return "UNKNOWN"
	}
}

// TaintNode represents a point where a new bit pattern is created
// MOV doesn't create nodes - it just updates regSource mapping
type TaintNode struct {
	ID      TaintNodeID
	Kind    TaintNodeKind
	Step    int      // Execution step
	PC      uint64   // Program counter
	Opcode  byte     // Instruction opcode
	Inputs  []TaintNodeID // Input nodes (e.g., for ADD: [node_from_r1, node_from_r2])
	MemAddr uint64   // For LOAD/STORE: memory address
	MemSize uint64   // For LOAD/STORE: size in bytes
	RegDest int      // For ALU/LOAD: destination register (-1 if N/A)
	RegSrc  int      // For STORE: source register (-1 if N/A)
	Value   uint64   // The actual value (for debugging)
}

func (n *TaintNode) String() string {
	inputStr := ""
	if len(n.Inputs) > 0 {
		inputStr = fmt.Sprintf(" inputs=%v", n.Inputs)
	}
	memStr := ""
	if n.Kind == TaintKindLoad || n.Kind == TaintKindStore || n.Kind == TaintKindExternal {
		memStr = fmt.Sprintf(" Mem[0x%x..0x%x]", n.MemAddr, n.MemAddr+n.MemSize-1)
	}
	regStr := ""
	if n.RegDest >= 0 {
		regStr = fmt.Sprintf(" -> r%d", n.RegDest)
	}
	if n.Kind == TaintKindStore && n.RegSrc >= 0 {
		regStr = fmt.Sprintf(" r%d ->", n.RegSrc)
	}
	return fmt.Sprintf("Node%d[Step=%d PC=0x%x %s %s%s%s%s]",
		n.ID, n.Step, n.PC, opcode_str(n.Opcode), n.Kind.String(), memStr, regStr, inputStr)
}

// TaintGraph is the SSA-style taint tracking graph
type TaintGraph struct {
	Nodes     []*TaintNode           // All nodes, indexed by ID-1 (ID 0 is reserved for None)
	nextID    TaintNodeID            // Next node ID to assign
	regSource [13]TaintNodeID        // Current source node for each register (r0-r12)
	memSource map[uint64]TaintNodeID // Current source node for each memory address
}

// NewTaintGraph creates a new taint graph
func NewTaintGraph() *TaintGraph {
	return &TaintGraph{
		Nodes:     make([]*TaintNode, 0),
		nextID:    1, // Start from 1, 0 is TaintNodeNone
		memSource: make(map[uint64]TaintNodeID),
	}
}

// newNode creates a new node and returns its ID
func (g *TaintGraph) newNode(kind TaintNodeKind, step int, pc uint64, opcode byte) *TaintNode {
	node := &TaintNode{
		ID:      g.nextID,
		Kind:    kind,
		Step:    step,
		PC:      pc,
		Opcode:  opcode,
		RegDest: -1,
		RegSrc:  -1,
	}
	g.nextID++
	g.Nodes = append(g.Nodes, node)
	return node
}

// getNode returns a node by ID (nil if not found or ID is 0)
func (g *TaintGraph) getNode(id TaintNodeID) *TaintNode {
	if id <= 0 || int(id) > len(g.Nodes) {
		return nil
	}
	return g.Nodes[id-1]
}

// RecordMov handles MOV dst, src - just updates alias, no new node
func (g *TaintGraph) RecordMov(dstReg, srcReg int) {
	if srcReg >= 0 && srcReg < 13 && dstReg >= 0 && dstReg < 13 {
		g.regSource[dstReg] = g.regSource[srcReg]
	}
}

// RecordImm handles LOAD_IMM dst, imm - creates IMM node (constant source)
func (g *TaintGraph) RecordImm(dstReg int, step int, pc uint64, opcode byte) {
	node := g.newNode(TaintKindImm, step, pc, opcode)
	node.RegDest = dstReg
	if dstReg >= 0 && dstReg < 13 {
		g.regSource[dstReg] = node.ID
	}
}

// RecordALU handles ALU operations like ADD, SUB, XOR, etc.
// Creates a new node with inputs from source registers
func (g *TaintGraph) RecordALU(dstReg int, srcRegs []int, step int, pc uint64, opcode byte) {
	node := g.newNode(TaintKindALU, step, pc, opcode)
	node.RegDest = dstReg

	// Collect input nodes from source registers
	for _, srcReg := range srcRegs {
		if srcReg >= 0 && srcReg < 13 {
			node.Inputs = append(node.Inputs, g.regSource[srcReg])
		}
	}

	if dstReg >= 0 && dstReg < 13 {
		g.regSource[dstReg] = node.ID
	}
}

// RecordLoad handles memory load: dst = Mem[addr]
func (g *TaintGraph) RecordLoad(dstReg int, memAddr, memSize uint64, step int, pc uint64, opcode byte) {
	node := g.newNode(TaintKindLoad, step, pc, opcode)
	node.RegDest = dstReg
	node.MemAddr = memAddr
	node.MemSize = memSize

	// Find memory source nodes that overlap with this load
	for addr := memAddr; addr < memAddr+memSize; addr++ {
		if srcID := g.memSource[addr]; srcID != TaintNodeNone {
			// Add unique input
			found := false
			for _, id := range node.Inputs {
				if id == srcID {
					found = true
					break
				}
			}
			if !found {
				node.Inputs = append(node.Inputs, srcID)
			}
		}
	}

	if dstReg >= 0 && dstReg < 13 {
		g.regSource[dstReg] = node.ID
	}
}

// RecordStore handles memory store: Mem[addr] = srcReg
func (g *TaintGraph) RecordStore(srcReg int, memAddr, memSize uint64, step int, pc uint64, opcode byte) {
	node := g.newNode(TaintKindStore, step, pc, opcode)
	node.MemAddr = memAddr
	node.MemSize = memSize
	node.RegSrc = srcReg

	// The input is the source register's current node
	if srcReg >= 0 && srcReg < 13 {
		node.Inputs = append(node.Inputs, g.regSource[srcReg])
	}

	// Update memory source for all bytes in this store
	for addr := memAddr; addr < memAddr+memSize; addr++ {
		g.memSource[addr] = node.ID
	}
}

// RecordExternalWrite handles external memory writes (e.g., hostPoke)
func (g *TaintGraph) RecordExternalWrite(memAddr, memSize uint64, step int, pc uint64) {
	node := g.newNode(TaintKindExternal, step, pc, OPCODE_EXTERNAL_WRITE)
	node.MemAddr = memAddr
	node.MemSize = memSize

	// Update memory source for all bytes
	for addr := memAddr; addr < memAddr+memSize; addr++ {
		g.memSource[addr] = node.ID
	}
}

// OPCODE_EXTERNAL_WRITE is a pseudo-opcode for external writes
const OPCODE_EXTERNAL_WRITE = 255

// GetBackwardTrace traces backward from a memory address to find all contributing nodes
func (g *TaintGraph) GetBackwardTrace(memAddr, memSize uint64, maxDepth int) []*TaintNode {
	result := make([]*TaintNode, 0)
	visited := make(map[TaintNodeID]bool)

	// Find starting nodes from memory
	var startNodes []TaintNodeID
	for addr := memAddr; addr < memAddr+memSize; addr++ {
		if nodeID := g.memSource[addr]; nodeID != TaintNodeNone && !visited[nodeID] {
			visited[nodeID] = true
			startNodes = append(startNodes, nodeID)
		}
	}

	// Debug: if no starting nodes found, print diagnostic info
	if len(startNodes) == 0 {
		fmt.Printf("DEBUG: No taint source found in memSource map for address 0x%x..0x%x\n", memAddr, memAddr+memSize-1)
		fmt.Printf("DEBUG: memSource map size: %d addresses\n", len(g.memSource))
		fmt.Printf("DEBUG: Checking nearby addresses in memSource:\n")
		for addr := memAddr - 8; addr <= memAddr + 8; addr++ {
			if nodeID, ok := g.memSource[addr]; ok && nodeID != TaintNodeNone {
				fmt.Printf("  memSource[0x%x] = Node%d\n", addr, nodeID)
			}
		}
		fmt.Printf("DEBUG: Searching for STORE nodes to this address range in full node list...\n")

		// Search backwards through nodes (most recent first) for STORE to this address
		// This is more efficient than searching all nodes
		fmt.Printf("DEBUG: Searching last 10000 nodes for STOREs to this address...\n")
		matchingStores := make([]*TaintNode, 0)
		searchStart := len(g.Nodes) - 1
		searchEnd := len(g.Nodes) - 10000
		if searchEnd < 0 {
			searchEnd = 0
		}

		for i := searchStart; i >= searchEnd; i-- {
			node := g.Nodes[i]
			if node.Kind == TaintKindStore || node.Kind == TaintKindExternal {
				// Check if this store overlaps with target address
				if node.MemAddr <= memAddr+memSize-1 && node.MemAddr+node.MemSize-1 >= memAddr {
					matchingStores = append(matchingStores, node)
					if len(matchingStores) >= 10 {
						break // Found enough matches
					}
				}
			}
		}

		if len(matchingStores) > 0 {
			fmt.Printf("DEBUG: Found %d recent STORE/EXTERNAL nodes that wrote to this address:\n", len(matchingStores))
			for i, node := range matchingStores {
				fmt.Printf("  [%d] %s\n", i, node.String())
			}

			// Use the FIRST match (most recent write) as starting point
			lastStore := matchingStores[0]
			fmt.Printf("DEBUG: Using most recent store: %s\n", lastStore.String())
			startNodes = append(startNodes, lastStore.ID)
			visited[lastStore.ID] = true
		} else {
			fmt.Printf("DEBUG: No STORE nodes found in last 10000 nodes for this address!\n")
			fmt.Printf("DEBUG: This suggests memSource map is corrupted or memory address was not written.\n")
			return result
		}
	}

	// BFS backward
	type queueItem struct {
		nodeID TaintNodeID
		depth  int
	}
	queue := make([]queueItem, 0)
	for _, nodeID := range startNodes {
		queue = append(queue, queueItem{nodeID, 0})
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		node := g.getNode(item.nodeID)
		if node == nil {
			continue
		}

		result = append(result, node)

		if maxDepth > 0 && item.depth >= maxDepth {
			continue
		}

		// Add input nodes to queue
		for _, inputID := range node.Inputs {
			if inputID != TaintNodeNone && !visited[inputID] {
				visited[inputID] = true
				queue = append(queue, queueItem{inputID, item.depth + 1})
			}
		}
	}

	return result
}

// GetBackwardTraceFromStep traces backward from a specific step/PC
func (g *TaintGraph) GetBackwardTraceFromStep(targetStep int, targetPC uint64, maxDepth int) []*TaintNode {
	result := make([]*TaintNode, 0)
	visited := make(map[TaintNodeID]bool)

	// Find starting node at target step/PC
	var startNode *TaintNode
	for _, node := range g.Nodes {
		if node.Step == targetStep && node.PC == targetPC {
			startNode = node
			break
		}
	}

	if startNode == nil {
		return result
	}

	// BFS backward
	type queueItem struct {
		nodeID TaintNodeID
		depth  int
	}
	queue := []queueItem{{startNode.ID, 0}}
	visited[startNode.ID] = true

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		node := g.getNode(item.nodeID)
		if node == nil {
			continue
		}

		result = append(result, node)

		if maxDepth > 0 && item.depth >= maxDepth {
			continue
		}

		// Add input nodes to queue
		for _, inputID := range node.Inputs {
			if inputID != TaintNodeNone && !visited[inputID] {
				visited[inputID] = true
				queue = append(queue, queueItem{inputID, item.depth + 1})
			}
		}
	}

	return result
}

// PrintBackwardTrace prints the backward trace from a memory address
func (g *TaintGraph) PrintBackwardTrace(memAddr, memSize uint64, maxDepth int) {
	nodes := g.GetBackwardTrace(memAddr, memSize, maxDepth)
	fmt.Printf("\n=== Backward Trace for Mem[0x%x..0x%x] ===\n", memAddr, memAddr+memSize-1)
	fmt.Printf("Total nodes in graph: %d\n", len(g.Nodes))
	fmt.Printf("Nodes in trace: %d", len(nodes))
	if maxDepth > 0 {
		fmt.Printf(" (maxDepth=%d)", maxDepth)
	}
	fmt.Printf("\n\n")

	// Track which nodes are at the boundary (have untraced inputs due to depth limit)
	visited := make(map[TaintNodeID]bool)
	for _, node := range nodes {
		visited[node.ID] = true
	}

	truncated := false
	for i, node := range nodes {
		fmt.Printf("[%d] %s", i, node.String())

		// Check if this node has inputs that weren't traced (due to depth limit)
		hasUntracedInputs := false
		for _, inputID := range node.Inputs {
			if inputID != TaintNodeNone && !visited[inputID] {
				hasUntracedInputs = true
				break
			}
		}

		if hasUntracedInputs {
			fmt.Printf(" [TRUNCATED - has untraced inputs]")
			truncated = true
		}
		fmt.Printf("\n")
	}

	if truncated {
		fmt.Printf("\n⚠️  WARNING: Trace was truncated due to depth limit. Increase maxDepth to see full trace.\n")
	}

	// Collect unique opcodes (instruction union)
	fmt.Printf("\n=== Instruction Union (Unique Opcodes) ===\n")
	opcodeSet := make(map[byte]bool)
	for _, node := range nodes {
		opcodeSet[node.Opcode] = true
	}

	// Convert to sorted slice
	opcodes := make([]byte, 0, len(opcodeSet))
	for opcode := range opcodeSet {
		opcodes = append(opcodes, opcode)
	}

	// Sort for consistent output
	sort.Slice(opcodes, func(i, j int) bool {
		return opcodes[i] < opcodes[j]
	})

	fmt.Printf("Total unique opcodes: %d\n\n", len(opcodes))
	for i, opcode := range opcodes {
		fmt.Printf("[%d] %s (opcode=0x%02x)\n", i, opcode_str(opcode), opcode)
	}
}

// Stats returns statistics about the graph
func (g *TaintGraph) Stats() string {
	kinds := make(map[TaintNodeKind]int)
	for _, node := range g.Nodes {
		kinds[node.Kind]++
	}
	return fmt.Sprintf("Nodes: %d (EXTERNAL=%d, LOAD=%d, STORE=%d, ALU=%d, IMM=%d), MemAddrs tracked: %d",
		len(g.Nodes), kinds[TaintKindExternal], kinds[TaintKindLoad], kinds[TaintKindStore],
		kinds[TaintKindALU], kinds[TaintKindImm], len(g.memSource))
}

// === VMGo integration methods ===

// shouldTrackStep checks if current step is within the tracking window
func (vm *VMGo) shouldTrackStep() bool {
	if vm.TaintConfig == nil || !vm.TaintConfig.Enabled {
		return false
	}
	// If no target step specified, track all steps
	if vm.TaintConfig.TargetStep == 0 {
		return true
	}
	// Only track steps within [TargetStep-StepWindow, TargetStep]
	minStep := vm.TaintConfig.TargetStep - vm.TaintConfig.StepWindow
	if minStep < 0 {
		minStep = 0
	}
	return vm.currentStep >= minStep && vm.currentStep <= vm.TaintConfig.TargetStep
}

// TaintRecordMov records a MOV operation (alias update only)
func (vm *VMGo) TaintRecordMov(dstReg, srcReg int) {
	if vm.TaintConfig == nil || !vm.TaintConfig.Enabled || vm.TaintGraph == nil {
		return
	}
	if !vm.shouldTrackStep() {
		return
	}
	vm.TaintGraph.RecordMov(dstReg, srcReg)
}

// TaintRecordImm records an immediate load
func (vm *VMGo) TaintRecordImm(dstReg int, opcode byte) {
	if vm.TaintConfig == nil || !vm.TaintConfig.Enabled || vm.TaintGraph == nil {
		return
	}
	if !vm.shouldTrackStep() {
		return
	}
	vm.TaintGraph.RecordImm(dstReg, vm.currentStep, vm.currentPC, opcode)
}

// TaintRecordALU records an ALU operation
func (vm *VMGo) TaintRecordALU(dstReg int, srcRegs []int, opcode byte) {
	if vm.TaintConfig == nil || !vm.TaintConfig.Enabled || vm.TaintGraph == nil {
		return
	}
	if !vm.shouldTrackStep() {
		return
	}
	vm.TaintGraph.RecordALU(dstReg, srcRegs, vm.currentStep, vm.currentPC, opcode)
}

// TaintRecordLoad records a memory load
func (vm *VMGo) TaintRecordLoad(dstReg int, memAddr, memSize uint64, opcode byte) {
	if vm.TaintConfig == nil || !vm.TaintConfig.Enabled || vm.TaintGraph == nil {
		return
	}
	if !vm.shouldTrackStep() {
		return
	}
	vm.TaintGraph.RecordLoad(dstReg, memAddr, memSize, vm.currentStep, vm.currentPC, opcode)
}

// TaintRecordStore records a memory store
func (vm *VMGo) TaintRecordStore(srcReg int, memAddr, memSize uint64, opcode byte) {
	if vm.TaintConfig == nil || !vm.TaintConfig.Enabled || vm.TaintGraph == nil {
		return
	}
	if !vm.shouldTrackStep() {
		return
	}
	vm.TaintGraph.RecordStore(srcReg, memAddr, memSize, vm.currentStep, vm.currentPC, opcode)
}

// TaintRecordExternalWrite records an external memory write
func (vm *VMGo) TaintRecordExternalWrite(memAddr, memSize uint64) {
	if vm.TaintConfig == nil || !vm.TaintConfig.Enabled || vm.TaintGraph == nil {
		return
	}
	if !vm.shouldTrackStep() {
		return
	}
	vm.TaintGraph.RecordExternalWrite(memAddr, memSize, vm.currentStep, vm.currentPC)
}

// PrintTaintTrace prints the backward trace from a memory address
func (vm *VMGo) PrintTaintTrace(memAddr, memSize uint64, maxDepth int) {
	if vm.TaintGraph == nil {
		fmt.Println("Taint tracking is not enabled")
		return
	}
	vm.TaintGraph.PrintBackwardTrace(memAddr, memSize, maxDepth)
}

// GetTaintStats returns taint graph statistics
func (vm *VMGo) GetTaintStats() string {
	if vm.TaintGraph == nil {
		return "Taint tracking is not enabled"
	}
	return vm.TaintGraph.Stats()
}
