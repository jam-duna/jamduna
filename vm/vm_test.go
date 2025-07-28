package vm

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

func RunEVMBytecode(bytecode []byte, input []byte) ([]byte, error) {
	state := NewMockStateDB()

	from := common.Address{0x1}
	to := common.Address{}

	context := BlockContext{
		GetHash:     func(uint64) common.Hash { return common.Hash{} },
		Coinbase:    common.Address{},
		BlockNumber: big.NewInt(1),
		Difficulty:  big.NewInt(0),
		GasLimit:    1_000_000,
		BaseFee:     big.NewInt(0),
	}

	chainID := big.NewInt(2)
	chainConfig := &params.ChainConfig{ChainID: chainID}

	evm := NewEVM(context, state, chainConfig, Config{})

	contract := NewContract(from, to, uint256.NewInt(0), 1_000_000, nil)
	contract.Code = bytecode
	contract.Input = input // optional, if relevant

	return evm.Interpreter().Run(contract, input, false)
}

func TestSimpleAdd(t *testing.T) {
	// PUSH1 0x02 PUSH1 0x03 ADD MSTORE PUSH1 0x20 PUSH1 0x00 RETURN
	code := []byte{
		0x60, 0x02, // PUSH1 0x02
		0x60, 0x03, // PUSH1 0x03
		0x01,       // ADD
		0x60, 0x00, // PUSH1 0x00
		0x52,       // MSTORE
		0x60, 0x20, // PUSH1 0x20
		0x60, 0x00, // PUSH1 0x00
		0xf3, // RETURN
	}
	out, err := RunEVMBytecode(code, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Output: %x\n", out)
}

func getRunSelector(idx uint8) []byte {
	// Method selector for run(uint8): first 4 bytes of keccak256("run(uint8)")
	selector := []byte{0xc4, 0xe5, 0x55, 0x7a}

	// ABI encode the uint8 parameter (32 bytes, big-endian)
	param := make([]byte, 32)
	param[31] = idx // uint8 goes in the least significant byte

	// Combine selector + parameter
	return append(selector, param...)
}

func TestRunRecipeIdx0(t *testing.T) {
	// solc --optimize --bin-runtime Recipes.sol
	// get the "Binary of the runtime part:"
	bytecodeHex := "608060405234801561000f575f5ffd5b5060043610610029575f3560e01c8063c4e5557a1461002d575b5f5ffd5b61004061003b36600461027f565b610052565b60405190815260200160405180910390f35b5f8160ff165f0361006d57610067600a6100a0565b92915050565b8160ff166001036100845761006760056003610113565b8160ff1660020361009957610067600a610179565b505f919050565b5f815f036100af57505f919050565b81600114806100be5750816002145b156100cb57506001919050565b5f6001808260035b86811161010857826100e585876102b3565b6100ef91906102b3565b939450919291829150610101816102c6565b90506100d3565b509095945050505050565b5f82158061011f575081155b8061012957508282115b1561013557505f610067565b826101408484610228565b61015e61014e6001876102de565b6101596001876102de565b610228565b61016891906102f1565b6101729190610308565b9392505050565b5f815f0361018957506001919050565b8160010361019957506001919050565b6001805f60025b85811161021e576101b28160026102b3565b846101be6001846102de565b6101c99060036102f1565b6101d391906102f1565b846101df8460026102f1565b6101ea9060016102b3565b6101f491906102f1565b6101fe91906102b3565b6102089190610308565b929350829150610217816102c6565b90506101a0565b5090949350505050565b5f8282111561023857505f610067565b60015f5b838110156102775761024f8160016102b3565b61025982876102de565b61026390846102f1565b61026d9190610308565b915060010161023c565b509392505050565b5f6020828403121561028f575f5ffd5b813560ff81168114610172575f5ffd5b634e487b7160e01b5f52601160045260245ffd5b808201808211156100675761006761029f565b5f600182016102d7576102d761029f565b5060010190565b818103818111156100675761006761029f565b80820281158282048414176100675761006761029f565b5f8261032257634e487b7160e01b5f52601260045260245ffd5b50049056fea26469706673582212202ba054cf30a1b9a860d56e0a582481cd9c33c5c80f6fc6f4b02d82a199c6e80064736f6c634300081e0033"
	bytecode := common.FromHex(bytecodeHex)

	for i := 0; i < 3; i++ {
		out, err := RunEVMBytecode(bytecode, getRunSelector(uint8(i)))
		if err != nil {
			t.Fatal(err)
		}

		result := new(big.Int).SetBytes(out)
		fmt.Printf("Output: %x (decimal: %s)\n", out, result.String())
		if i == 0 {
			// tribonacci(10) should return 149
			expected := big.NewInt(149)
			if result.Cmp(expected) != 0 {
				t.Fatalf("Expected %s, got %s", expected.String(), result.String())
			}
		} else if i == 1 {
			// narayana(5, 3) should return 12
			expected := big.NewInt(12)
			if result.Cmp(expected) != 0 {
				t.Fatalf("Expected %s, got %s", expected.String(), result.String())
			}
		} else if i == 2 {
			// motzkin(10) should return 2188
			expected := big.NewInt(2188)
			if result.Cmp(expected) != 0 {
				t.Fatalf("Expected %s, got %s", expected.String(), result.String())
			}
		}
	}
	fmt.Printf("All tests passed!\n")
}
