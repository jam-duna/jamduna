// Copyright 2024 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm

import (
	gomath "math"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/params"
)

func makeCallVariantGasEIP4762(oldCalculator gasFunc, withTransferCosts bool) gasFunc {
	return func(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
		var (
			target           = common.Address(stack.Back(1).Bytes20())
			witnessGas       uint64
			_, isPrecompile  = evm.precompile(target)
			isSystemContract = target == params.HistoryStorageAddress
		)

		// If value is transferred, it is charged before 1/64th
		// is subtracted from the available gas pool.
		if withTransferCosts && !stack.Back(2).IsZero() {
			wantedValueTransferWitnessGas := evm.AccessEvents.ValueTransferGas(contract.Address(), target, contract.Gas)
			if wantedValueTransferWitnessGas > contract.Gas {
				return wantedValueTransferWitnessGas, nil
			}
			witnessGas = wantedValueTransferWitnessGas
		} else if isPrecompile || isSystemContract {
			witnessGas = params.WarmStorageReadCostEIP2929
		} else {
			// The charging for the value transfer is done BEFORE subtracting
			// the 1/64th gas, as this is considered part of the CALL instruction.
			// (so before we get to this point)
			// But the message call is part of the subcall, for which only 63/64th
			// of the gas should be available.
			wantedMessageCallWitnessGas := evm.AccessEvents.MessageCallGas(target, contract.Gas-witnessGas)
			var overflow bool
			if witnessGas, overflow = math.SafeAdd(witnessGas, wantedMessageCallWitnessGas); overflow {
				return 0, ErrGasUintOverflow
			}
			if witnessGas > contract.Gas {
				return witnessGas, nil
			}
		}

		contract.Gas -= witnessGas
		// if the operation fails, adds witness gas to the gas before returning the error
		gas, err := oldCalculator(evm, contract, stack, mem, memorySize)
		contract.Gas += witnessGas // restore witness gas so that it can be charged at the callsite
		var overflow bool
		if gas, overflow = math.SafeAdd(gas, witnessGas); overflow {
			return 0, ErrGasUintOverflow
		}
		return gas, err
	}
}

func gasSStore4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	return evm.AccessEvents.SlotGas(contract.Address(), stack.peek().Bytes32(), true, contract.Gas, true), nil
}

func gasSLoad4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	return evm.AccessEvents.SlotGas(contract.Address(), stack.peek().Bytes32(), false, contract.Gas, true), nil
}

func gasBalance4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	address := stack.peek().Bytes20()
	return evm.AccessEvents.BasicDataGas(address, false, contract.Gas, true), nil
}

func gasExtCodeSize4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	address := stack.peek().Bytes20()
	if _, isPrecompile := evm.precompile(address); isPrecompile {
		return 0, nil
	}
	return 0, nil
}

func gasExtCodeHash4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	address := stack.peek().Bytes20()
	if _, isPrecompile := evm.precompile(address); isPrecompile {
		return 0, nil
	}
	return evm.AccessEvents.CodeHashGas(address, false, contract.Gas, true), nil
}

func gasExtCodeCopyEIP4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	// memory expansion first (dynamic part of pre-2929 implementation)
	gas, err := gasExtCodeCopy(evm, contract, stack, mem, memorySize)
	if err != nil {
		return 0, err
	}
	wgas := uint64(0)
	var overflow bool
	if gas, overflow = math.SafeAdd(gas, wgas); overflow {
		return 0, ErrGasUintOverflow
	}
	return gas, nil
}

func gasCodeCopyEip4762(evm *EVM, contract *Contract, stack *Stack, mem *Memory, memorySize uint64) (uint64, error) {
	gas, err := gasCodeCopy(evm, contract, stack, mem, memorySize)
	if err != nil {
		return 0, err
	}
	if !contract.IsDeployment && !contract.IsSystemCall {
		var (
			codeOffset = stack.Back(1)
			length     = stack.Back(2)
		)
		uint64CodeOffset, overflow := codeOffset.Uint64WithOverflow()
		if overflow {
			uint64CodeOffset = gomath.MaxUint64
		}

		_, copyOffset, nonPaddedCopyLength := getDataAndAdjustedBounds(contract.Code, uint64CodeOffset, length.Uint64())
		_, wanted := evm.AccessEvents.CodeChunksRangeGas(contract.Address(), copyOffset, nonPaddedCopyLength, uint64(len(contract.Code)), false, contract.Gas-gas)
		gas += wanted
	}
	return gas, nil
}
