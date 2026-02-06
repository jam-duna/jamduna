package recompiler

import "fmt"

var debugGasModel bool = false

func (g *GasModel) TransitionCycle(insts []Instruction) bool {
	if debugGasModel {
		fmt.Printf("start cycle instructions %d\n", len(insts))
		for _, inst := range insts {
			fmt.Printf("inst: %s\n", inst.String())
		}
	}
	for {
		n := g.Index
		index := g.Index
		var d_hat int
		var isBasicBlock = false
		var inst Instruction
		if len(insts) > int(index) {
			inst = insts[index]
			gasCost := inst.GetGasCost()
			d_hat = gasCost.GetDecodeGas(inst)
		} else {
			isBasicBlock = true
		}

		current_d := g.DecodingSlots
		current_s_len := len(g.StatusVector)
		if n == 0 || (!isBasicBlock && d_hat <= int(current_d) && current_s_len < 32) {
			if debugGasModel {
				fmt.Printf("[trans] step for %s d_hat=%d , current d=%d  d_hat <= int(current_d)=%v\n", inst.String(), d_hat, current_d, d_hat <= int(current_d))
			}
			g.Step(inst)
		} else if g.GetNextReadyInstrunction() < 99999 && g.ExecutionSlots > 0 {
			if debugGasModel {
				fmt.Printf("[trans]GetNextReadyInstrunction\n")
			}
			g.NextPendingInstrunction()
		} else if n != 0 && current_s_len == 0 && isBasicBlock {
			if debugGasModel {
				for i := 0; i < int(n); i++ {
					inst := g.DebugInsts[i]
					slots := g.DebugSlots[i]
					fmt.Printf("%s| [%d]%s [src]%v, [dest]%v\n", slots, i, inst.String(), inst.SourceRegs, inst.DestRegs)
				}
			}
			return true
		} else {
			if debugGasModel {
				fmt.Printf("[trans] SimulationCycle %d\n", g.CycleCounter)
			}
			g.SimulationCycle()
		}
	}

}

// A.48
func (g *GasModel) Step(inst Instruction) {
	g.Index++
	if inst.Opcode == MOVE_REG {
		g.DecodeMoveReg(inst)
	} else {
		g.DecodeInstrunction(inst)
	}
}

func (g *GasModel) DecodeMoveReg(inst Instruction) {
	g.DecodingSlots--
	srcReg := inst.SourceRegs[0]
	destReg := inst.DestRegs[0]
	if srcReg == destReg {
		// no operation needed
		return
	}

	for j := 0; j < int(g.NextReorderBufferEntryIndex); j++ {
		srcRegHit := false
		for reg, _ := range g.RegisterVector[j] {
			if reg == srcReg {
				srcRegHit = true
			}
		}

		idx := j
		if srcRegHit {
			if _, ok := g.RegisterVector[idx]; !ok {
				g.RegisterVector[idx] = make(map[int]struct{})
			}
			g.RegisterVector[idx][destReg] = struct{}{}
			return
		} else {
			delete(g.RegisterVector[idx], destReg)
		}
	}
}

func (g *GasModel) DecodeInstrunction(inst Instruction) {
	gas_cost := inst.GetGasCost()
	g.DecodingSlots -= int64(gas_cost.GetDecodeGas(inst))

	//status vector update
	g.StatusVector[int(g.NextReorderBufferEntryIndex)] = 1
	g.DebugInsts[int(g.NextReorderBufferEntryIndex)] = inst
	//cycle counter vector update
	for len(g.CycleCounterVector) <= int(g.NextReorderBufferEntryIndex) {
		g.CycleCounterVector = append(g.CycleCounterVector, 0)
	}
	if !inst.IsBranchInstruction() {
		g.CycleCounterVector[g.NextReorderBufferEntryIndex] = gas_cost.CycleCost
	} else {
		branchCycleCost := gas_cost.GetBranchCycleCost(inst, inst.BranchTarget1, inst.BranchTarget2)
		g.CycleCounterVector[g.NextReorderBufferEntryIndex] = branchCycleCost
	}

	// execution unit cost vector update
	for len(g.ExecuteionUnits) <= int(g.NextReorderBufferEntryIndex) {
		g.ExecuteionUnits = append(g.ExecuteionUnits, ExecuteionUnit{})
	}

	g.ExecuteionUnits[g.NextReorderBufferEntryIndex] = ExecuteionUnit{
		ALU:   gas_cost.ALUGas,
		Load:  gas_cost.LoadGas,
		Store: gas_cost.StoreGas,
		Mul:   gas_cost.MulGas,
		Div:   gas_cost.DivGas,
	}

	// update the processing slots first
	for _, srcReg := range inst.SourceRegs {
		for j_index := int(g.NextReorderBufferEntryIndex); j_index >= 0; j_index-- {
			regVec := g.RegisterVector[j_index]
			for reg, _ := range regVec {
				if reg == srcReg {
					if debugGasModel {
						fmt.Printf("instruction %d have a dependency on instruction %d on reg %d\n", int(g.NextReorderBufferEntryIndex), j_index, srcReg)
					}
					g.PredecessorVector[int(g.NextReorderBufferEntryIndex)] = append(g.PredecessorVector[int(g.NextReorderBufferEntryIndex)], j_index)
					break
				}
			}
		}
	}
	idx := int(g.NextReorderBufferEntryIndex)
	g.RegisterVector[idx] = make(map[int]struct{})
	for _, destReg := range inst.DestRegs {
		for j := int(g.NextReorderBufferEntryIndex); j >= 0; j-- {
			if j == idx {
				g.RegisterVector[idx][destReg] = struct{}{}
			} else {
				delete(g.RegisterVector[j], destReg)
			}
		}
	}
	g.NextReorderBufferEntryIndex++
}

// A.51
func (g *GasModel) NextPendingInstrunction() {
	//update the staus vector and cycle counter vector
	nextReadyIndex := g.GetNextReadyInstrunction()
	g.StatusVector[nextReadyIndex] = 3
	g.CompareExecutionUnit.ALU -= g.ExecuteionUnits[nextReadyIndex].ALU
	g.CompareExecutionUnit.Load -= g.ExecuteionUnits[nextReadyIndex].Load
	g.CompareExecutionUnit.Store -= g.ExecuteionUnits[nextReadyIndex].Store
	g.CompareExecutionUnit.Mul -= g.ExecuteionUnits[nextReadyIndex].Mul
	g.CompareExecutionUnit.Div -= g.ExecuteionUnits[nextReadyIndex].Div
	if debugGasModel {
		inst := g.DebugInsts[nextReadyIndex]
		fmt.Printf("[DEDUCT] Instruction %d %s: compare alu %d load %d store %d mul %d div %d\n",
			nextReadyIndex, inst.String(), g.CompareExecutionUnit.ALU, g.CompareExecutionUnit.Load,
			g.CompareExecutionUnit.Store, g.CompareExecutionUnit.Mul, g.CompareExecutionUnit.Div)
	}
	// update execution slots
	g.ExecutionSlots--
}

// A.53
func (g *GasModel) SimulationCycle() {
	current_status_vector := make(map[int]int)
	current_cycles_vector := make([]int, len(g.CycleCounterVector))
	for k, v := range g.StatusVector {
		current_status_vector[k] = v
	}
	copy(current_cycles_vector, g.CycleCounterVector)
	for i := 0; i < int(g.NextReorderBufferEntryIndex); i++ {
		//status vector update
		currentStatus, exist := current_status_vector[i]
		currentCycle := current_cycles_vector[i]
		for len(g.DebugSlots[i]) < int(g.CycleCounter) && debugGasModel {
			g.DebugSlots[i] = g.DebugSlots[i] + "."
		}
		//cycle counter vector update
		if currentStatus == 3 {
			//executing [e]
			if debugGasModel {
				if currentCycle > 0 && debugGasModel {
					g.DebugSlots[i] = g.DebugSlots[i] + "e"
				}
			}
			g.CycleCounterVector[i]--
		}

		if currentStatus == 4 {
			retirable := true
			for k := 0; k < i; k++ {
				to_check_status, ok := current_status_vector[k]
				if !ok {
					continue
				}
				if to_check_status < 4 {
					retirable = false
					if debugGasModel {
						g.DebugSlots[i] = g.DebugSlots[i] + "-"
					}
					break
				}
			}
			if retirable {
				//retired instruction [R]
				delete(g.StatusVector, i)
				if debugGasModel {
					g.DebugSlots[i] = g.DebugSlots[i] + "R"
				}
			}
		} else if currentStatus == 1 {
			//decoded [D]
			g.StatusVector[i] = 2
			if debugGasModel {
				g.DebugSlots[i] = g.DebugSlots[i] + "D"
			}
		} else if currentStatus == 3 && currentCycle == 0 {
			//retiring [E]
			g.StatusVector[i] = 4
			if debugGasModel {
				g.DebugSlots[i] = g.DebugSlots[i] + "E"
			}

		} else if !exist && debugGasModel {
			//retired instruction [-]
			g.DebugSlots[i] = g.DebugSlots[i] + "."
		} else if currentStatus == 2 && debugGasModel {
			//pending instruction [=]
			g.DebugSlots[i] = g.DebugSlots[i] + "="
		}

		//update the register vector and free the registers
		if currentStatus == 3 && currentCycle == 1 {
			delete(g.RegisterVector, i)
			g.CompareExecutionUnit = AddExecutionUnits(g.CompareExecutionUnit, g.ExecuteionUnits[i])
			if debugGasModel {
				inst := g.DebugInsts[i]
				fmt.Printf("[RESTORE] Instruction %d %s: compare alu %d load %d store %d mul %d div %d\n",
					i, inst.String(), g.CompareExecutionUnit.ALU, g.CompareExecutionUnit.Load,
					g.CompareExecutionUnit.Store, g.CompareExecutionUnit.Mul, g.CompareExecutionUnit.Div)
			}
		}
		//update execution unit
		if currentCycle == 1 {

		}
	}

	g.CycleCounter++
	g.DecodingSlots = 4
	g.ExecutionSlots = 5
}

// (A.52) S(Ξn)
func (g *GasModel) GetNextReadyInstrunction() int {
	nextReadyIndex := 99999
	for j := 0; j < int(g.NextReorderBufferEntryIndex); j++ {
		statusCheck := g.StatusVector[j] == 2
		executionUnitCheck := CompareExecutionUnits(g.ExecuteionUnits[j], g.CompareExecutionUnit)
		costCheck := true
		for _, regVecIndex := range g.PredecessorVector[j] {
			if g.CycleCounterVector[regVecIndex] > 0 {
				costCheck = false
				break
			}
		}
		if statusCheck && executionUnitCheck && costCheck && j < nextReadyIndex {
			nextReadyIndex = j
		}
	}
	return nextReadyIndex
}

// this function will return true if eu1 is less than or equal to eu2
func CompareExecutionUnits(eu1, eu2 ExecuteionUnit) bool {
	if eu1.ALU > eu2.ALU {
		return false
	}
	if eu1.Load > eu2.Load {
		return false
	}
	if eu1.Store > eu2.Store {
		return false
	}
	if eu1.Mul > eu2.Mul {
		return false
	}
	if eu1.Div > eu2.Div {
		return false
	}
	return true
}

func AddExecutionUnits(eu1, eu2 ExecuteionUnit) ExecuteionUnit {
	return ExecuteionUnit{
		ALU:   eu1.ALU + eu2.ALU,
		Load:  eu1.Load + eu2.Load,
		Store: eu1.Store + eu2.Store,
		Mul:   eu1.Mul + eu2.Mul,
		Div:   eu1.Div + eu2.Div,
	}
}
