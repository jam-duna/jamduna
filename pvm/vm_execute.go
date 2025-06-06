package pvm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/colorfulnotion/jam/log"
)

func (vm *VM) compileBasicBlock(startStep int) (*BasicBlock, error) {
	if vm.pc >= uint64(len(vm.code)) {
		return nil, errors.New("program counter out of bounds")
	}
	this_step_pc := vm.pc
	opcode := vm.code[this_step_pc]
	len_operands := vm.skip(this_step_pc)
	operands := vm.code[this_step_pc+1 : this_step_pc+1+len_operands]
	basicBlock := NewBasicBlock(vm.Gas)
	if IsBasicBlockInstruction(opcode) {
		basicBlock.AddInstruction(opcode, operands, startStep, this_step_pc)
		return basicBlock, nil
	} else {
		for !IsBasicBlockInstruction(opcode) && this_step_pc < uint64(len(vm.code)) {
			basicBlock.AddInstruction(opcode, operands, startStep, this_step_pc)
			this_step_pc += uint64(len_operands) + 1
			if vm.pc >= uint64(len(vm.code)) || this_step_pc >= uint64(len(vm.code)) {
				break
			}
			opcode = vm.code[this_step_pc]
			len_operands = vm.skip(this_step_pc)
			operands = vm.code[this_step_pc+1 : this_step_pc+1+len_operands]
		}
		if vm.pc >= uint64(len(vm.code)) || this_step_pc >= uint64(len(vm.code)) {
			basicBlock.AddInstruction(0, nil, startStep, this_step_pc) // Add a TRAP instruction if we reached the end of the code
		} else {
			basicBlock.AddInstruction(opcode, operands, startStep, this_step_pc)
		}
	}
	return basicBlock, nil
}

func (vm *VM) executeInstruction(instruction Instruction, is_child bool) error {
	stepn := instruction.Step
	opcode := instruction.Opcode
	operands := instruction.Args
	len_operands := uint64(len(operands))
	if vm.pc != instruction.Pc {
		log.Warn(vm.logging, "pc mismatch", "service", string(vm.ServiceMetadata), "expected_pc", vm.pc, "actual_pc", instruction.Pc)
		return fmt.Errorf("pc mismatch: expected %d, got %d", vm.pc, instruction.Pc)
	}
	switch {
	case opcode <= 1: // A.5.1 No arguments
		vm.HandleNoArgs(opcode)
	case opcode == ECALLI: // A.5.2 One immediate
		vm.HandleOneImm(opcode, operands)
	case opcode == LOAD_IMM_64: // A.5.3 One Register and One Extended Width Immediate
		vm.HandleOneRegOneEWImm(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case 30 <= opcode && opcode <= 33: // A.5.4 Two Immediates
		vm.HandleTwoImms(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case opcode == JUMP: // A.5.5 One offset
		vm.HandleOneOffset(opcode, operands)
	case 50 <= opcode && opcode <= 62: // A.5.6 One Register and One Immediate
		vm.HandleOneRegOneImm(opcode, operands)
		if opcode != JUMP_IND {
			if !vm.terminated {
				vm.pc += 1 + len_operands
			}
		}
	case 70 <= opcode && opcode <= 73: // A.5.7 One Register and Two Immediate
		vm.HandleOneRegTwoImm(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case 80 <= opcode && opcode <= 90: // A.5.8 One Register, One Immediate and One Offset
		vm.HandleOneRegOneImmOneOffset(opcode, operands)
	case 100 <= opcode && opcode <= 111: // A.5.9 Two Registers
		vm.HandleTwoRegs(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case 120 <= opcode && opcode <= 161: // A.5.10 Two Registers and One Immediate
		//fmt.Printf("OPCODE %d\n", opcode)
		vm.HandleTwoRegsOneImm(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}
	case 170 <= opcode && opcode <= 175: // A.5.11 Two Registers and One Offset
		vm.HandleTwoRegsOneOffset(opcode, operands)
	case opcode == LOAD_IMM_JUMP_IND: // A.5.12 Two Register and Two Immediate
		vm.HandleTwoRegsTwoImms(opcode, operands)
	case 190 <= opcode && opcode <= 230: // A.5.13 Three Registers
		vm.HandleThreeRegs(opcode, operands)
		if !vm.terminated {
			vm.pc += 1 + len_operands
		}

	default:

		log.Warn(vm.logging, "terminated: unknown opcode", "service", string(vm.ServiceMetadata), "opcode", opcode)
		vm.HandleNoArgs(0) //TRAP
	}

	// avoid this: this is expensive
	if PvmLogging {
		registersJSON, _ := json.Marshal(vm.ReadRegisters())
		prettyJSON := strings.ReplaceAll(string(registersJSON), ",", ", ")
		//fmt.Printf("%-18s step:%6d pc:%6d g:%6d Registers:%s\n", opcode_str(opcode), stepn-1, this_step_pc, vm.Gas, prettyJSON)
		fmt.Printf("%s %d %d Registers:%s\n", opcode_str(opcode), stepn, vm.pc, prettyJSON)
	}

	if vm.hostCall && is_child {
		return nil
	}
	// host call invocation
	if vm.hostCall {
		vm.InvokeHostCall(vm.host_func_id)
		vm.hostCall = false
		vm.terminated = false
	}

	return nil
}

func (vm *VM) executeBasicBlock(bb *BasicBlock, is_child bool) error {
	if bb == nil {
		return fmt.Errorf("nil basic block")
	}
	if PvmLogging {
		fmt.Printf("Executing Basic Block with %d instructions\n", len(bb.Instructions))
	}
	for _, instruction := range bb.Instructions {
		if err := vm.executeInstruction(instruction, is_child); err != nil {
			return fmt.Errorf("error executing instruction at pc %d: %w", instruction.Pc, err)
		}
		bb.GasUsage++
	}
	if err := vm.executeInstruction(bb.EndPoint, is_child); err != nil {
		return fmt.Errorf("error executing end point instruction at pc %d: %w", bb.EndPoint.Pc, err)
	}
	bb.GasUsage++
	if PvmLogging {
		fmt.Printf("Basic Block executed successfully: %s\n", bb.String())
	}
	vm.Gas -= int64(bb.GasUsage)
	if vm.Gas < 0 {
		return fmt.Errorf("gas limit exceeded: %d < %d", vm.Gas, bb.GasUsage)
	}

	fmt.Printf("%s\n", bb.String())
	return nil
}
