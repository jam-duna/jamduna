package rpc

import (
	"fmt"
	"math/big"
	"testing"
)

func TestScriptStack(t *testing.T) {
	t.Run("BasicOperations", func(t *testing.T) {
		stack := NewScriptStack()

		// Test push and size
		err := stack.Push([]byte{1})
		if err != nil {
			t.Fatalf("Push failed: %v", err)
		}
		if stack.Size() != 1 {
			t.Fatalf("Expected size 1, got %d", stack.Size())
		}

		// Test peek
		data, err := stack.Peek(0)
		if err != nil {
			t.Fatalf("Peek failed: %v", err)
		}
		if len(data) != 1 || data[0] != 1 {
			t.Fatalf("Expected [1], got %v", data)
		}

		// Test pop
		data, err = stack.Pop()
		if err != nil {
			t.Fatalf("Pop failed: %v", err)
		}
		if len(data) != 1 || data[0] != 1 {
			t.Fatalf("Expected [1], got %v", data)
		}
		if !stack.Empty() {
			t.Fatal("Stack should be empty")
		}
	})

	t.Run("StackLimits", func(t *testing.T) {
		stack := NewScriptStack()

		// Test stack size limit
		for i := 0; i < MaxStackSize; i++ {
			err := stack.Push([]byte{byte(i)})
			if err != nil {
				t.Fatalf("Push %d failed: %v", i, err)
			}
		}

		// This should fail
		err := stack.Push([]byte{255})
		if err == nil {
			t.Fatal("Expected stack overflow error")
		}

		// Test data size limit
		largeData := make([]byte, MaxDataSize+1)
		err = stack.Push(largeData)
		if err == nil {
			t.Fatal("Expected data too large error")
		}
	})

	t.Run("StackDup", func(t *testing.T) {
		stack := NewScriptStack()
		stack.Push([]byte{42})

		err := stack.Dup()
		if err != nil {
			t.Fatalf("Dup failed: %v", err)
		}

		if stack.Size() != 2 {
			t.Fatalf("Expected size 2, got %d", stack.Size())
		}

		top, _ := stack.Pop()
		second, _ := stack.Pop()
		if len(top) != 1 || top[0] != 42 || len(second) != 1 || second[0] != 42 {
			t.Fatal("Dup produced incorrect results")
		}
	})

	t.Run("StackSwap", func(t *testing.T) {
		stack := NewScriptStack()
		stack.Push([]byte{1})
		stack.Push([]byte{2})

		err := stack.Swap()
		if err != nil {
			t.Fatalf("Swap failed: %v", err)
		}

		top, _ := stack.Pop()
		second, _ := stack.Pop()
		if len(top) != 1 || top[0] != 1 || len(second) != 1 || second[0] != 2 {
			t.Fatal("Swap produced incorrect results")
		}
	})
}

func TestScriptNumber(t *testing.T) {
	tests := []struct {
		input    []byte
		expected int64
	}{
		{[]byte{}, 0},
		{[]byte{1}, 1},
		{[]byte{255, 0}, 255},
		{[]byte{129}, -1}, // 0x81 = -1
		{[]byte{255, 127}, 32767},
		{[]byte{255, 255}, -32767}, // 0xffff with sign bit set
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("Test%d", i), func(t *testing.T) {
			result, err := numberFromBytes(test.input)
			if err != nil {
				t.Fatalf("numberFromBytes failed: %v", err)
			}

			if result.Int64() != test.expected {
				t.Fatalf("Expected %d, got %d", test.expected, result.Int64())
			}

			// Test round trip
			converted := numberToBytes(result)
			back, err := numberFromBytes(converted)
			if err != nil {
				t.Fatalf("Round trip failed: %v", err)
			}

			if back.Int64() != test.expected {
				t.Fatalf("Round trip failed: expected %d, got %d", test.expected, back.Int64())
			}
		})
	}
}

func TestBasicOpcodes(t *testing.T) {
	t.Run("OP_DUP", func(t *testing.T) {
		engine := NewScriptEngine()
		engine.stack.Push([]byte{42})

		err := engine.executeOpcode(OP_DUP)
		if err != nil {
			t.Fatalf("OP_DUP failed: %v", err)
		}

		if engine.stack.Size() != 2 {
			t.Fatalf("Expected stack size 2, got %d", engine.stack.Size())
		}

		top, _ := engine.stack.Pop()
		second, _ := engine.stack.Pop()
		if len(top) != 1 || top[0] != 42 || len(second) != 1 || second[0] != 42 {
			t.Fatal("OP_DUP produced incorrect results")
		}
	})

	t.Run("OP_EQUAL", func(t *testing.T) {
		engine := NewScriptEngine()
		engine.stack.Push([]byte{1, 2, 3})
		engine.stack.Push([]byte{1, 2, 3})

		err := engine.executeOpcode(OP_EQUAL)
		if err != nil {
			t.Fatalf("OP_EQUAL failed: %v", err)
		}

		result, _ := engine.stack.Pop()
		if !isTrue(result) {
			t.Fatal("OP_EQUAL should return true for equal values")
		}

		// Test unequal values
		engine.stack.Push([]byte{1, 2, 3})
		engine.stack.Push([]byte{4, 5, 6})

		err = engine.executeOpcode(OP_EQUAL)
		if err != nil {
			t.Fatalf("OP_EQUAL failed: %v", err)
		}

		result, _ = engine.stack.Pop()
		if isTrue(result) {
			t.Fatal("OP_EQUAL should return false for unequal values")
		}
	})

	t.Run("OP_ADD", func(t *testing.T) {
		engine := NewScriptEngine()
		engine.stack.Push(numberToBytes(big.NewInt(5)))
		engine.stack.Push(numberToBytes(big.NewInt(3)))

		err := engine.executeOpcode(OP_ADD)
		if err != nil {
			t.Fatalf("OP_ADD failed: %v", err)
		}

		result, _ := engine.stack.Pop()
		num, err := numberFromBytes(result)
		if err != nil {
			t.Fatalf("Result parsing failed: %v", err)
		}

		if num.Int64() != 8 {
			t.Fatalf("Expected 8, got %d", num.Int64())
		}
	})

	t.Run("OP_HASH160", func(t *testing.T) {
		engine := NewScriptEngine()
		data := []byte("test data")
		engine.stack.Push(data)

		err := engine.executeOpcode(OP_HASH160)
		if err != nil {
			t.Fatalf("OP_HASH160 failed: %v", err)
		}

		result, _ := engine.stack.Pop()
		if len(result) != 20 {
			t.Fatalf("Expected 20-byte hash, got %d bytes", len(result))
		}

		// Verify it's actual HASH160 (SHA256 + RIPEMD160)
		expected := hash160(data)
		for i := 0; i < 20; i++ {
			if result[i] != expected[i] {
				t.Fatal("HASH160 result doesn't match expected")
			}
		}
	})
}

func TestPushOpcodes(t *testing.T) {
	t.Run("OP_0", func(t *testing.T) {
		engine := NewScriptEngine()
		script := []byte{OP_0}

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("Script execution failed: %v", err)
		}

		if engine.stack.Size() != 1 {
			t.Fatalf("Expected stack size 1, got %d", engine.stack.Size())
		}

		result, _ := engine.stack.Pop()
		if len(result) != 0 {
			t.Fatalf("OP_0 should push empty array, got %v", result)
		}
	})

	t.Run("OP_1_to_16", func(t *testing.T) {
		for i := byte(OP_1); i <= byte(OP_16); i++ {
			engine := NewScriptEngine()
			script := []byte{i}

			err := engine.ExecuteScript(script)
			if err != nil {
				t.Fatalf("Script execution failed for OP_%d: %v", i-byte(OP_1)+1, err)
			}

			result, _ := engine.stack.Pop()
			expected := i - byte(OP_1) + 1
			if len(result) != 1 || result[0] != expected {
				t.Fatalf("OP_%d should push %d, got %v", i-byte(OP_1)+1, expected, result)
			}
		}
	})

	t.Run("DirectPush", func(t *testing.T) {
		engine := NewScriptEngine()
		data := []byte{1, 2, 3, 4}
		script := append([]byte{byte(len(data))}, data...)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("Script execution failed: %v", err)
		}

		result, _ := engine.stack.Pop()
		for i := 0; i < len(data); i++ {
			if result[i] != data[i] {
				t.Fatalf("Push data mismatch at index %d: expected %d, got %d", i, data[i], result[i])
			}
		}
	})
}

func TestSimpleScripts(t *testing.T) {
	t.Run("DupHash", func(t *testing.T) {
		engine := NewScriptEngine()
		// Script: <data> OP_DUP OP_HASH160
		data := []byte{1, 2, 3, 4}
		script := []byte{byte(len(data))}
		script = append(script, data...)
		script = append(script, OP_DUP, OP_HASH160)

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("Script execution failed: %v", err)
		}

		if engine.stack.Size() != 2 {
			t.Fatalf("Expected stack size 2, got %d", engine.stack.Size())
		}

		hash, _ := engine.stack.Pop()
		original, _ := engine.stack.Pop()

		if len(hash) != 20 {
			t.Fatalf("Expected 20-byte hash, got %d bytes", len(hash))
		}

		for i := 0; i < len(data); i++ {
			if original[i] != data[i] {
				t.Fatal("Original data corrupted")
			}
		}
	})

	t.Run("ArithmeticScript", func(t *testing.T) {
		engine := NewScriptEngine()
		// Script: 5 3 OP_ADD 2 OP_SUB (should result in 6)
		script := []byte{OP_5, OP_3, OP_ADD, OP_2, OP_SUB}

		err := engine.ExecuteScript(script)
		if err != nil {
			t.Fatalf("Script execution failed: %v", err)
		}

		result, _ := engine.stack.Pop()
		num, err := numberFromBytes(result)
		if err != nil {
			t.Fatalf("Result parsing failed: %v", err)
		}

		if num.Int64() != 6 {
			t.Fatalf("Expected 6, got %d", num.Int64())
		}
	})
}

func TestScriptLimits(t *testing.T) {
	t.Run("ScriptTooLarge", func(t *testing.T) {
		engine := NewScriptEngine()
		script := make([]byte, MaxScriptSize+1)

		err := engine.ExecuteScript(script)
		if err == nil {
			t.Fatal("Expected script too large error")
		}
	})

	t.Run("TooManyOps", func(t *testing.T) {
		engine := NewScriptEngine()
		// Create script with too many operations
		script := make([]byte, MaxScriptOps+1)
		for i := range script {
			script[i] = OP_1
		}

		err := engine.ExecuteScript(script)
		if err == nil {
			t.Fatal("Expected too many operations error")
		}
	})

	t.Run("StackOverflow", func(t *testing.T) {
		engine := NewScriptEngine()
		// Create script that pushes too many items
		script := []byte{}
		for i := 0; i < MaxStackSize+1; i++ {
			script = append(script, OP_1)
		}

		err := engine.ExecuteScript(script)
		if err == nil {
			t.Fatal("Expected stack overflow error")
		}
	})
}

func TestMultiSig(t *testing.T) {
	t.Run("BasicMultiSig", func(t *testing.T) {
		engine := NewScriptEngine()

		// Create 2-of-3 multisig script
		// Stack layout: 0 sig1 sig2 2 pubkey1 pubkey2 pubkey3 3 OP_CHECKMULTISIG

		// Push dummy signatures and pubkeys
		engine.stack.Push([]byte{})                       // dummy value for CHECKMULTISIG bug
		engine.stack.Push([]byte{0x30, 0x44, 0x02, 0x20}) // dummy signature 1
		engine.stack.Push([]byte{0x30, 0x44, 0x02, 0x20}) // dummy signature 2
		engine.stack.Push(numberToBytes(big.NewInt(2)))   // number of signatures
		engine.stack.Push(make([]byte, 33))               // dummy pubkey 1
		engine.stack.Push(make([]byte, 33))               // dummy pubkey 2
		engine.stack.Push(make([]byte, 33))               // dummy pubkey 3
		engine.stack.Push(numberToBytes(big.NewInt(3)))   // number of pubkeys

		err := engine.executeOpcode(OP_CHECKMULTISIG)
		if err != nil {
			t.Fatalf("OP_CHECKMULTISIG failed: %v", err)
		}

		// Should have result on stack
		if engine.stack.Size() != 1 {
			t.Fatalf("Expected stack size 1, got %d", engine.stack.Size())
		}

		result, _ := engine.stack.Pop()
		// Should be false since we used dummy data
		if isTrue(result) {
			t.Fatal("OP_CHECKMULTISIG should return false for dummy signatures")
		}
	})

	t.Run("MultiSigLimits", func(t *testing.T) {
		engine := NewScriptEngine()

		// Test too many pubkeys
		engine.stack.Push([]byte{})                                          // dummy value
		engine.stack.Push(numberToBytes(big.NewInt(0)))                      // 0 signatures
		engine.stack.Push(numberToBytes(big.NewInt(MaxPubkeysPerMulti + 1))) // too many pubkeys

		err := engine.executeOpcode(OP_CHECKMULTISIG)
		if err == nil {
			t.Fatal("Expected too many pubkeys error")
		}
	})
}

func TestSignatureVerification(t *testing.T) {
	t.Run("BasicChecksig", func(t *testing.T) {
		engine := NewScriptEngine()

		// Test with properly formatted dummy data
		dummySig := []byte{0x30, 0x44, 0x02, 0x20}       // DER signature format
		dummySig = append(dummySig, make([]byte, 32)...) // r value
		dummySig = append(dummySig, 0x02, 0x20)          // s value marker
		dummySig = append(dummySig, make([]byte, 32)...) // s value
		dummySig = append(dummySig, 0x01)                // hash type

		dummyPubkey := make([]byte, 33) // compressed pubkey format
		dummyPubkey[0] = 0x02           // compression flag

		engine.stack.Push(dummySig)
		engine.stack.Push(dummyPubkey)

		err := engine.opCheckSig()
		if err != nil {
			t.Fatalf("OP_CHECKSIG failed: %v", err)
		}

		result, _ := engine.stack.Pop()
		// Should be false because we use placeholder verification
		if isTrue(result) {
			t.Fatal("OP_CHECKSIG should return false for placeholder implementation")
		}
	})

	t.Run("EmptySignature", func(t *testing.T) {
		engine := NewScriptEngine()

		engine.stack.Push([]byte{})         // empty signature
		engine.stack.Push(make([]byte, 33)) // dummy pubkey

		err := engine.opCheckSig()
		if err != nil {
			t.Fatalf("OP_CHECKSIG failed: %v", err)
		}

		result, _ := engine.stack.Pop()
		// Should be false for empty signature
		if isTrue(result) {
			t.Fatal("OP_CHECKSIG should return false for empty signature")
		}
	})
}

func TestVerifyScript(t *testing.T) {
	t.Run("P2PKH_Style", func(t *testing.T) {
		engine := NewScriptEngine()

		// Simulate P2PKH: scriptSig pushes sig and pubkey, scriptPubKey does DUP HASH160 <hash> EQUALVERIFY CHECKSIG
		scriptSig := []byte{
			0x47, // push 71 bytes (dummy signature)
		}
		scriptSig = append(scriptSig, make([]byte, 71)...) // dummy signature
		scriptSig = append(scriptSig, 0x21)                // push 33 bytes (pubkey)
		scriptSig = append(scriptSig, make([]byte, 33)...) // dummy pubkey

		pubkeyHash := make([]byte, 20) // dummy hash
		scriptPubKey := []byte{
			OP_DUP,
			OP_HASH160,
			0x14, // push 20 bytes
		}
		scriptPubKey = append(scriptPubKey, pubkeyHash...)
		scriptPubKey = append(scriptPubKey, OP_EQUALVERIFY, OP_CHECKSIG)

		err := engine.VerifyScript(scriptSig, scriptPubKey)
		// This will fail because of dummy data, but should not crash
		if err == nil {
			t.Log("Script verification passed (unexpected with dummy data)")
		} else {
			t.Logf("Script verification failed as expected: %v", err)
		}
	})
}
