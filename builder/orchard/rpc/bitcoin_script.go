package rpc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
)

// Bitcoin Script execution limits (consensus rules)
const (
	MaxScriptSize      = 10000 // Maximum script size in bytes
	MaxStackSize       = 1000  // Maximum stack size
	MaxScriptOps       = 201   // Maximum number of operations
	MaxPubkeysPerMulti = 20    // Maximum public keys in CHECKMULTISIG
	MaxDataSize        = 520   // Maximum size of a stack element
)

// Bitcoin Script opcodes
const (
	OP_0         = 0x00
	OP_PUSHDATA1 = 0x4c
	OP_PUSHDATA2 = 0x4d
	OP_PUSHDATA4 = 0x4e
	OP_1NEGATE   = 0x4f
	OP_1         = 0x51
	OP_2         = 0x52
	OP_3         = 0x53
	OP_4         = 0x54
	OP_5         = 0x55
	OP_6         = 0x56
	OP_7         = 0x57
	OP_8         = 0x58
	OP_9         = 0x59
	OP_10        = 0x5a
	OP_11        = 0x5b
	OP_12        = 0x5c
	OP_13        = 0x5d
	OP_14        = 0x5e
	OP_15        = 0x5f
	OP_16        = 0x60

	// Flow control
	OP_NOP      = 0x61
	OP_VER      = 0x62 // Disabled
	OP_IF       = 0x63
	OP_NOTIF    = 0x64
	OP_VERIF    = 0x65 // Disabled
	OP_VERNOTIF = 0x66 // Disabled
	OP_ELSE     = 0x67
	OP_ENDIF    = 0x68
	OP_VERIFY   = 0x69
	OP_RETURN   = 0x6a

	// Stack operations
	OP_TOALTSTACK   = 0x6b
	OP_FROMALTSTACK = 0x6c
	OP_2DROP        = 0x6d
	OP_2DUP         = 0x6e
	OP_3DUP         = 0x6f
	OP_2OVER        = 0x70
	OP_2ROT         = 0x71
	OP_2SWAP        = 0x72
	OP_IFDUP        = 0x73
	OP_DEPTH        = 0x74
	OP_DROP         = 0x75
	OP_DUP          = 0x76
	OP_NIP          = 0x77
	OP_OVER         = 0x78
	OP_PICK         = 0x79
	OP_ROLL         = 0x7a
	OP_ROT          = 0x7b
	OP_SWAP         = 0x7c
	OP_TUCK         = 0x7d

	// Splice operations
	OP_CAT    = 0x7e // Disabled
	OP_SUBSTR = 0x7f // Disabled
	OP_LEFT   = 0x80 // Disabled
	OP_RIGHT  = 0x81 // Disabled
	OP_SIZE   = 0x82

	// Bitwise logic
	OP_INVERT      = 0x83 // Disabled
	OP_AND         = 0x84 // Disabled
	OP_OR          = 0x85 // Disabled
	OP_XOR         = 0x86 // Disabled
	OP_EQUAL       = 0x87
	OP_EQUALVERIFY = 0x88

	// Arithmetic
	OP_1ADD               = 0x8b
	OP_1SUB               = 0x8c
	OP_2MUL               = 0x8d // Disabled
	OP_2DIV               = 0x8e // Disabled
	OP_NEGATE             = 0x8f
	OP_ABS                = 0x90
	OP_NOT                = 0x91
	OP_0NOTEQUAL          = 0x92
	OP_ADD                = 0x93
	OP_SUB                = 0x94
	OP_MUL                = 0x95 // Disabled
	OP_DIV                = 0x96 // Disabled
	OP_MOD                = 0x97 // Disabled
	OP_LSHIFT             = 0x98 // Disabled
	OP_RSHIFT             = 0x99 // Disabled
	OP_BOOLAND            = 0x9a
	OP_BOOLOR             = 0x9b
	OP_NUMEQUAL           = 0x9c
	OP_NUMEQUALVERIFY     = 0x9d
	OP_NUMNOTEQUAL        = 0x9e
	OP_LESSTHAN           = 0x9f
	OP_GREATERTHAN        = 0xa0
	OP_LESSTHANOREQUAL    = 0xa1
	OP_GREATERTHANOREQUAL = 0xa2
	OP_MIN                = 0xa3
	OP_MAX                = 0xa4
	OP_WITHIN             = 0xa5

	// Crypto operations
	OP_RIPEMD160           = 0xa6
	OP_SHA1                = 0xa7
	OP_SHA256              = 0xa8
	OP_HASH160             = 0xa9
	OP_HASH256             = 0xaa
	OP_CODESEPARATOR       = 0xab
	OP_CHECKSIG            = 0xac
	OP_CHECKSIGVERIFY      = 0xad
	OP_CHECKMULTISIG       = 0xae
	OP_CHECKMULTISIGVERIFY = 0xaf

	// Locktime operations
	OP_CHECKLOCKTIMEVERIFY = 0xb1
	OP_CHECKSEQUENCEVERIFY = 0xb2

	// Reserved operations
	OP_NOP1  = 0xb0
	OP_NOP4  = 0xb3
	OP_NOP5  = 0xb4
	OP_NOP6  = 0xb5
	OP_NOP7  = 0xb6
	OP_NOP8  = 0xb7
	OP_NOP9  = 0xb8
	OP_NOP10 = 0xb9

	// Invalid opcodes (cause script failure)
	OP_INVALIDOPCODE = 0xff
)

// ScriptStack represents the main execution stack
type ScriptStack struct {
	data [][]byte
}

func NewScriptStack() *ScriptStack {
	return &ScriptStack{
		data: make([][]byte, 0),
	}
}

func (s *ScriptStack) Push(data []byte) error {
	if len(s.data) >= MaxStackSize {
		return fmt.Errorf("stack overflow: maximum size %d exceeded", MaxStackSize)
	}
	if len(data) > MaxDataSize {
		return fmt.Errorf("stack element too large: %d > %d", len(data), MaxDataSize)
	}

	// Make a copy to avoid external modification
	elem := make([]byte, len(data))
	copy(elem, data)
	s.data = append(s.data, elem)
	return nil
}

func (s *ScriptStack) Pop() ([]byte, error) {
	if len(s.data) == 0 {
		return nil, fmt.Errorf("stack underflow: cannot pop from empty stack")
	}

	top := s.data[len(s.data)-1]
	s.data = s.data[:len(s.data)-1]
	return top, nil
}

func (s *ScriptStack) Peek(depth int) ([]byte, error) {
	if depth < 0 || depth >= len(s.data) {
		return nil, fmt.Errorf("stack index out of bounds: %d", depth)
	}

	index := len(s.data) - 1 - depth
	return s.data[index], nil
}

func (s *ScriptStack) Size() int {
	return len(s.data)
}

func (s *ScriptStack) Empty() bool {
	return len(s.data) == 0
}

// Duplicate the top stack element
func (s *ScriptStack) Dup() error {
	if len(s.data) == 0 {
		return fmt.Errorf("stack underflow: cannot dup empty stack")
	}

	top := s.data[len(s.data)-1]
	return s.Push(top)
}

// Swap the top two stack elements
func (s *ScriptStack) Swap() error {
	if len(s.data) < 2 {
		return fmt.Errorf("stack underflow: need at least 2 elements for swap")
	}

	s.data[len(s.data)-1], s.data[len(s.data)-2] = s.data[len(s.data)-2], s.data[len(s.data)-1]
	return nil
}

// TransactionContext holds the transaction data needed for signature verification.
type TransactionContext struct {
	Tx           *ZcashTxV5
	InputIndex   int
	PrevOutValue uint64
	ScriptCode   []byte
}

// ScriptEngine represents the complete script execution environment
type ScriptEngine struct {
	stack        *ScriptStack
	altStack     *ScriptStack
	opCount      int
	conditionals []bool              // Stack for IF/ELSE/ENDIF tracking
	txContext    *TransactionContext // Transaction context for signature verification
}

func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{
		stack:        NewScriptStack(),
		altStack:     NewScriptStack(),
		opCount:      0,
		conditionals: make([]bool, 0),
		txContext:    nil,
	}
}

func NewScriptEngineWithContext(txContext *TransactionContext) *ScriptEngine {
	return &ScriptEngine{
		stack:        NewScriptStack(),
		altStack:     NewScriptStack(),
		opCount:      0,
		conditionals: make([]bool, 0),
		txContext:    txContext,
	}
}

func isConditionalOpcode(opcode byte) bool {
	switch opcode {
	case OP_IF, OP_NOTIF, OP_ELSE, OP_ENDIF:
		return true
	default:
		return false
	}
}

func (e *ScriptEngine) isExecuting() bool {
	for _, cond := range e.conditionals {
		if !cond {
			return false
		}
	}
	return true
}

func (e *ScriptEngine) handleConditional(opcode byte, executing bool) error {
	switch opcode {
	case OP_IF:
		if executing {
			data, err := e.stack.Pop()
			if err != nil {
				return err
			}
			e.conditionals = append(e.conditionals, isTrue(data))
		} else {
			e.conditionals = append(e.conditionals, false)
		}
	case OP_NOTIF:
		if executing {
			data, err := e.stack.Pop()
			if err != nil {
				return err
			}
			e.conditionals = append(e.conditionals, !isTrue(data))
		} else {
			e.conditionals = append(e.conditionals, false)
		}
	case OP_ELSE:
		if len(e.conditionals) == 0 {
			return fmt.Errorf("OP_ELSE without OP_IF")
		}
		parentExec := true
		for _, cond := range e.conditionals[:len(e.conditionals)-1] {
			if !cond {
				parentExec = false
				break
			}
		}
		current := e.conditionals[len(e.conditionals)-1]
		if parentExec {
			e.conditionals[len(e.conditionals)-1] = !current
		} else {
			e.conditionals[len(e.conditionals)-1] = false
		}
	case OP_ENDIF:
		if len(e.conditionals) == 0 {
			return fmt.Errorf("OP_ENDIF without OP_IF")
		}
		e.conditionals = e.conditionals[:len(e.conditionals)-1]
	default:
		return fmt.Errorf("unknown conditional opcode: 0x%02x", opcode)
	}
	return nil
}

// checkLimits validates script size and operation count
func (e *ScriptEngine) checkLimits(script []byte) error {
	if len(script) > MaxScriptSize {
		return fmt.Errorf("script too large: %d > %d bytes", len(script), MaxScriptSize)
	}
	return nil
}

// incrementOpCount tracks the number of operations executed
func (e *ScriptEngine) incrementOpCount() error {
	e.opCount++
	if e.opCount > MaxScriptOps {
		return fmt.Errorf("too many operations: %d > %d", e.opCount, MaxScriptOps)
	}
	return nil
}

// isTrue returns whether a stack element represents true (non-zero, non-negative-zero)
func isTrue(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	// Check for negative zero (0x80 or 0x8000 etc.)
	for i := 0; i < len(data)-1; i++ {
		if data[i] != 0 {
			return true
		}
	}

	// Check last byte - if it's 0x80, it's negative zero (false)
	lastByte := data[len(data)-1]
	return lastByte != 0 && lastByte != 0x80
}

// numberFromBytes converts a byte array to a number (Bitcoin script format)
func numberFromBytes(data []byte) (*big.Int, error) {
	if len(data) == 0 {
		return big.NewInt(0), nil
	}

	if len(data) > 4 {
		return nil, fmt.Errorf("number too large: %d bytes", len(data))
	}

	// Convert from little-endian with sign bit on the most-significant byte.
	signBit := data[len(data)-1] & 0x80
	var value uint32
	for i := 0; i < len(data); i++ {
		byteVal := data[i]
		if i == len(data)-1 && signBit != 0 {
			byteVal &= 0x7f
		}
		value |= uint32(byteVal) << (8 * i)
	}

	result := big.NewInt(int64(value))
	if signBit != 0 {
		result.Neg(result)
	}
	return result, nil
}

// numberToBytes converts a number to Bitcoin script byte format
func numberToBytes(num *big.Int) []byte {
	if num.Sign() == 0 {
		return []byte{}
	}

	absNum := new(big.Int).Abs(num)
	negative := num.Sign() < 0

	// Convert to little-endian bytes
	bytes := absNum.Bytes()

	// Reverse to little-endian
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}

	// Handle sign bit
	if len(bytes) > 0 && bytes[len(bytes)-1]&0x80 != 0 {
		// Need to add extra byte to avoid sign confusion
		if negative {
			bytes = append(bytes, 0x80)
		} else {
			bytes = append(bytes, 0x00)
		}
	} else if negative && len(bytes) > 0 {
		bytes[len(bytes)-1] |= 0x80
	}

	return bytes
}

// ExecuteScript executes a Bitcoin script with proper limits and validation
func (e *ScriptEngine) ExecuteScript(script []byte) error {
	if err := e.checkLimits(script); err != nil {
		return err
	}

	cursor := 0
	for cursor < len(script) {
		opcode := script[cursor]
		cursor++
		executing := e.isExecuting()

		// Handle push operations first
		if opcode <= OP_PUSHDATA4 {
			data, err := e.executePushOp(script, &cursor, opcode)
			if err != nil {
				return err
			}
			if executing {
				if err := e.stack.Push(data); err != nil {
					return err
				}
			}
			continue
		}

		if isConditionalOpcode(opcode) {
			if err := e.incrementOpCount(); err != nil {
				return err
			}
			if err := e.handleConditional(opcode, executing); err != nil {
				return err
			}
			continue
		}

		if !executing {
			continue
		}

		if err := e.incrementOpCount(); err != nil {
			return err
		}

		// Handle other opcodes
		if err := e.executeOpcode(opcode); err != nil {
			return err
		}

		// Check stack size limits after each operation
		if err := e.checkStackLimits(); err != nil {
			return err
		}
	}

	if len(e.conditionals) != 0 {
		return fmt.Errorf("unterminated conditional")
	}

	return nil
}

// checkStackLimits validates that stacks are within limits
func (e *ScriptEngine) checkStackLimits() error {
	if e.stack.Size() > MaxStackSize {
		return fmt.Errorf("main stack too large: %d > %d", e.stack.Size(), MaxStackSize)
	}
	if e.altStack.Size() > MaxStackSize {
		return fmt.Errorf("alt stack too large: %d > %d", e.altStack.Size(), MaxStackSize)
	}
	return nil
}

// VerifyScript executes a complete Bitcoin script verification
func (e *ScriptEngine) VerifyScript(scriptSig, scriptPubKey []byte) error {
	var redeemScript []byte
	isP2shScript := isP2SH(scriptPubKey)
	if isP2shScript {
		var err error
		redeemScript, err = extractRedeemScript(scriptSig)
		if err != nil {
			return fmt.Errorf("extract redeem script: %v", err)
		}
		if e.txContext != nil {
			e.txContext.ScriptCode = redeemScript
		}
	} else if e.txContext != nil && e.txContext.ScriptCode == nil {
		e.txContext.ScriptCode = scriptPubKey
	}

	// Execute scriptSig first
	if err := e.ExecuteScript(scriptSig); err != nil {
		return fmt.Errorf("scriptSig execution failed: %v", err)
	}

	// Save the stack state
	savedStack := make([][]byte, len(e.stack.data))
	for i, item := range e.stack.data {
		copied := make([]byte, len(item))
		copy(copied, item)
		savedStack[i] = copied
	}

	// Execute scriptPubKey
	if err := e.ExecuteScript(scriptPubKey); err != nil {
		return fmt.Errorf("scriptPubKey execution failed: %v", err)
	}

	// Verify final result
	if e.stack.Empty() {
		return fmt.Errorf("script verification failed: empty stack")
	}

	result, err := e.stack.Peek(0)
	if err != nil {
		return fmt.Errorf("script verification failed: cannot peek stack top: %v", err)
	}

	if !isTrue(result) {
		return fmt.Errorf("script verification failed: top stack element is false")
	}

	if isP2shScript {
		expectedHash := scriptPubKey[2:22]
		computedHash := scriptHash160(redeemScript)
		if !bytes.Equal(computedHash, expectedHash) {
			return fmt.Errorf("script hash mismatch")
		}

		e.stack.data = savedStack
		if len(e.stack.data) == 0 {
			return fmt.Errorf("missing redeem script on stack")
		}
		e.stack.data = e.stack.data[:len(e.stack.data)-1]

		if err := e.ExecuteScript(redeemScript); err != nil {
			return fmt.Errorf("redeem script execution failed: %v", err)
		}
		if e.stack.Empty() {
			return fmt.Errorf("script verification failed: empty stack after redeem script")
		}
		result, err := e.stack.Peek(0)
		if err != nil {
			return fmt.Errorf("script verification failed: cannot peek stack top: %v", err)
		}
		if !isTrue(result) {
			return fmt.Errorf("script verification failed: redeem script false")
		}
	}

	return nil
}

// executePushOp handles all push operations (OP_0 through OP_PUSHDATA4)
func (e *ScriptEngine) executePushOp(script []byte, cursor *int, opcode byte) ([]byte, error) {
	switch opcode {
	case OP_0:
		return []byte{}, nil

	case OP_1NEGATE:
		return []byte{0x81}, nil // -1 in script number format

	default:
		if opcode >= OP_1 && opcode <= OP_16 {
			// OP_1 through OP_16
			num := opcode - OP_1 + 1
			return []byte{num}, nil
		}

		// Handle push data operations
		var length int

		if opcode >= 0x01 && opcode <= 0x4b {
			length = int(opcode)
		} else {
			switch opcode {
			case OP_PUSHDATA1:
				if *cursor >= len(script) {
					return nil, fmt.Errorf("PUSHDATA1 out of bounds")
				}
				length = int(script[*cursor])
				*cursor++

			case OP_PUSHDATA2:
				if *cursor+2 > len(script) {
					return nil, fmt.Errorf("PUSHDATA2 out of bounds")
				}
				length = int(binary.LittleEndian.Uint16(script[*cursor : *cursor+2]))
				*cursor += 2

			case OP_PUSHDATA4:
				if *cursor+4 > len(script) {
					return nil, fmt.Errorf("PUSHDATA4 out of bounds")
				}
				length = int(binary.LittleEndian.Uint32(script[*cursor : *cursor+4]))
				*cursor += 4

			default:
				return nil, fmt.Errorf("unsupported push opcode: 0x%02x", opcode)
			}
		}

		if *cursor+length > len(script) {
			return nil, fmt.Errorf("push data exceeds script length")
		}

		if length > MaxDataSize {
			return nil, fmt.Errorf("push data too large: %d > %d", length, MaxDataSize)
		}

		data := make([]byte, length)
		copy(data, script[*cursor:*cursor+length])
		*cursor += length

		return data, nil
	}
}
