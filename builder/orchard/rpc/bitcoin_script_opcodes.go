package rpc

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"golang.org/x/crypto/ripemd160"
)

const (
	lockTimeThreshold          = 500000000
	finalSequence              = 0xffffffff
	sequenceLocktimeDisableFlag = 1 << 31
	sequenceLocktimeTypeFlag    = 1 << 22
	sequenceLocktimeMask        = 0x0000ffff
)

// executeOpcode handles execution of all non-push opcodes
func (e *ScriptEngine) executeOpcode(opcode byte) error {
	switch opcode {
	// Flow control
	case OP_NOP, OP_NOP1, OP_NOP4, OP_NOP5, OP_NOP6, OP_NOP7, OP_NOP8, OP_NOP9, OP_NOP10:
		// No operation
		return nil

	case OP_VERIFY:
		return e.opVerify()

	case OP_RETURN:
		return fmt.Errorf("script failed: OP_RETURN executed")

	case OP_1NEGATE:
		return e.stack.Push([]byte{0x81})

	case OP_1, OP_2, OP_3, OP_4, OP_5, OP_6, OP_7, OP_8, OP_9, OP_10, OP_11, OP_12, OP_13, OP_14, OP_15, OP_16:
		return e.stack.Push([]byte{opcode - OP_1 + 1})

	// Stack operations
	case OP_TOALTSTACK:
		return e.opToAltStack()

	case OP_FROMALTSTACK:
		return e.opFromAltStack()

	case OP_2DROP:
		return e.op2Drop()

	case OP_2DUP:
		return e.op2Dup()

	case OP_3DUP:
		return e.op3Dup()

	case OP_2OVER:
		return e.op2Over()

	case OP_2ROT:
		return e.op2Rot()

	case OP_2SWAP:
		return e.op2Swap()

	case OP_IFDUP:
		return e.opIfDup()

	case OP_DEPTH:
		return e.opDepth()

	case OP_DROP:
		return e.opDrop()

	case OP_DUP:
		return e.opDup()

	case OP_NIP:
		return e.opNip()

	case OP_OVER:
		return e.opOver()

	case OP_PICK:
		return e.opPick()

	case OP_ROLL:
		return e.opRoll()

	case OP_ROT:
		return e.opRot()

	case OP_SWAP:
		return e.opSwap()

	case OP_TUCK:
		return e.opTuck()

	case OP_SIZE:
		return e.opSize()

	// Bitwise logic
	case OP_EQUAL:
		return e.opEqual()

	case OP_EQUALVERIFY:
		return e.opEqualVerify()

	// Arithmetic
	case OP_1ADD:
		return e.op1Add()

	case OP_1SUB:
		return e.op1Sub()

	case OP_NEGATE:
		return e.opNegate()

	case OP_ABS:
		return e.opAbs()

	case OP_NOT:
		return e.opNot()

	case OP_0NOTEQUAL:
		return e.op0NotEqual()

	case OP_ADD:
		return e.opAdd()

	case OP_SUB:
		return e.opSub()

	case OP_BOOLAND:
		return e.opBoolAnd()

	case OP_BOOLOR:
		return e.opBoolOr()

	case OP_NUMEQUAL:
		return e.opNumEqual()

	case OP_NUMEQUALVERIFY:
		return e.opNumEqualVerify()

	case OP_NUMNOTEQUAL:
		return e.opNumNotEqual()

	case OP_LESSTHAN:
		return e.opLessThan()

	case OP_GREATERTHAN:
		return e.opGreaterThan()

	case OP_LESSTHANOREQUAL:
		return e.opLessThanOrEqual()

	case OP_GREATERTHANOREQUAL:
		return e.opGreaterThanOrEqual()

	case OP_MIN:
		return e.opMin()

	case OP_MAX:
		return e.opMax()

	case OP_WITHIN:
		return e.opWithin()

	// Crypto operations
	case OP_RIPEMD160:
		return e.opRipemd160()

	case OP_SHA1:
		return fmt.Errorf("OP_SHA1 disabled")

	case OP_SHA256:
		return e.opSha256()

	case OP_HASH160:
		return e.opHash160()

	case OP_HASH256:
		return e.opHash256()

	case OP_CHECKSIG:
		return e.opCheckSig()

	case OP_CHECKSIGVERIFY:
		return e.opCheckSigVerify()

	case OP_CHECKMULTISIG:
		return e.opCheckMultiSig()

	case OP_CHECKMULTISIGVERIFY:
		return e.opCheckMultiSigVerify()

	case OP_CHECKLOCKTIMEVERIFY:
		return e.opCheckLockTimeVerify()

	case OP_CHECKSEQUENCEVERIFY:
		return e.opCheckSequenceVerify()

	// Disabled opcodes
	case OP_VER, OP_VERIF, OP_VERNOTIF:
		return fmt.Errorf("opcode 0x%02x is disabled", opcode)

	case OP_CAT, OP_SUBSTR, OP_LEFT, OP_RIGHT:
		return fmt.Errorf("splice opcode 0x%02x is disabled", opcode)

	case OP_INVERT, OP_AND, OP_OR, OP_XOR:
		return fmt.Errorf("bitwise opcode 0x%02x is disabled", opcode)

	case OP_2MUL, OP_2DIV, OP_MUL, OP_DIV, OP_MOD, OP_LSHIFT, OP_RSHIFT:
		return fmt.Errorf("arithmetic opcode 0x%02x is disabled", opcode)

	default:
		return fmt.Errorf("unknown opcode: 0x%02x", opcode)
	}
}

// Stack operation implementations

func (e *ScriptEngine) opVerify() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	if !isTrue(data) {
		return fmt.Errorf("script failed: OP_VERIFY condition false")
	}

	return nil
}

func (e *ScriptEngine) opCheckLockTimeVerify() error {
	if e.txContext == nil || e.txContext.Tx == nil {
		return fmt.Errorf("missing transaction context")
	}
	lockBytes, err := e.stack.Peek(0)
	if err != nil {
		return err
	}
	lockValue, err := numberFromBytes(lockBytes)
	if err != nil {
		return err
	}
	if lockValue.Sign() < 0 {
		return fmt.Errorf("negative locktime")
	}
	if !lockValue.IsInt64() {
		return fmt.Errorf("locktime too large")
	}

	required := lockValue.Int64()
	txLockTime := int64(e.txContext.Tx.LockTime)

	if required < lockTimeThreshold && txLockTime >= lockTimeThreshold {
		return fmt.Errorf("locktime type mismatch")
	}
	if required >= lockTimeThreshold && txLockTime < lockTimeThreshold {
		return fmt.Errorf("locktime type mismatch")
	}
	if txLockTime < required {
		return fmt.Errorf("locktime not yet reached")
	}

	if e.txContext.InputIndex < 0 || e.txContext.InputIndex >= len(e.txContext.Tx.Inputs) {
		return fmt.Errorf("input index out of bounds")
	}
	if e.txContext.Tx.Inputs[e.txContext.InputIndex].Sequence == finalSequence {
		return fmt.Errorf("sequence disables locktime")
	}

	return nil
}

func (e *ScriptEngine) opCheckSequenceVerify() error {
	if e.txContext == nil || e.txContext.Tx == nil {
		return fmt.Errorf("missing transaction context")
	}
	seqBytes, err := e.stack.Peek(0)
	if err != nil {
		return err
	}
	seqValue, err := numberFromBytes(seqBytes)
	if err != nil {
		return err
	}
	if seqValue.Sign() < 0 {
		return fmt.Errorf("negative sequence")
	}
	if !seqValue.IsInt64() {
		return fmt.Errorf("sequence too large")
	}
	if e.txContext.Tx.Version < 2 {
		return fmt.Errorf("tx version too low for CSV")
	}

	required := seqValue.Int64()
	if required&sequenceLocktimeDisableFlag != 0 {
		return fmt.Errorf("sequence disable flag set")
	}

	if e.txContext.InputIndex < 0 || e.txContext.InputIndex >= len(e.txContext.Tx.Inputs) {
		return fmt.Errorf("input index out of bounds")
	}

	txSequence := e.txContext.Tx.Inputs[e.txContext.InputIndex].Sequence
	if (txSequence & sequenceLocktimeDisableFlag) != 0 {
		return fmt.Errorf("input sequence disables CSV")
	}

	if (required&sequenceLocktimeTypeFlag) != (int64(txSequence)&sequenceLocktimeTypeFlag) {
		return fmt.Errorf("sequence type mismatch")
	}

	requiredMasked := required & sequenceLocktimeMask
	txMasked := int64(txSequence) & sequenceLocktimeMask
	if txMasked < requiredMasked {
		return fmt.Errorf("sequence not yet satisfied")
	}

	return nil
}

func (e *ScriptEngine) opToAltStack() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}
	return e.altStack.Push(data)
}

func (e *ScriptEngine) opFromAltStack() error {
	data, err := e.altStack.Pop()
	if err != nil {
		return err
	}
	return e.stack.Push(data)
}

func (e *ScriptEngine) op2Drop() error {
	_, err := e.stack.Pop()
	if err != nil {
		return err
	}
	_, err = e.stack.Pop()
	return err
}

func (e *ScriptEngine) op2Dup() error {
	if e.stack.Size() < 2 {
		return fmt.Errorf("stack underflow: need 2 elements for 2DUP")
	}

	a, err := e.stack.Peek(1)
	if err != nil {
		return err
	}
	b, err := e.stack.Peek(0)
	if err != nil {
		return err
	}

	if err := e.stack.Push(a); err != nil {
		return err
	}
	return e.stack.Push(b)
}

func (e *ScriptEngine) op3Dup() error {
	if e.stack.Size() < 3 {
		return fmt.Errorf("stack underflow: need 3 elements for 3DUP")
	}

	a, err := e.stack.Peek(2)
	if err != nil {
		return err
	}
	b, err := e.stack.Peek(1)
	if err != nil {
		return err
	}
	c, err := e.stack.Peek(0)
	if err != nil {
		return err
	}

	if err := e.stack.Push(a); err != nil {
		return err
	}
	if err := e.stack.Push(b); err != nil {
		return err
	}
	return e.stack.Push(c)
}

func (e *ScriptEngine) op2Over() error {
	if e.stack.Size() < 4 {
		return fmt.Errorf("stack underflow: need 4 elements for 2OVER")
	}

	a, err := e.stack.Peek(3)
	if err != nil {
		return err
	}
	b, err := e.stack.Peek(2)
	if err != nil {
		return err
	}

	if err := e.stack.Push(a); err != nil {
		return err
	}
	return e.stack.Push(b)
}

func (e *ScriptEngine) op2Rot() error {
	if e.stack.Size() < 6 {
		return fmt.Errorf("stack underflow: need 6 elements for 2ROT")
	}

	// Pop 6 elements: x1 x2 x3 x4 x5 x6
	// Push back: x3 x4 x5 x6 x1 x2
	x6, _ := e.stack.Pop()
	x5, _ := e.stack.Pop()
	x4, _ := e.stack.Pop()
	x3, _ := e.stack.Pop()
	x2, _ := e.stack.Pop()
	x1, _ := e.stack.Pop()

	e.stack.Push(x3)
	e.stack.Push(x4)
	e.stack.Push(x5)
	e.stack.Push(x6)
	e.stack.Push(x1)
	e.stack.Push(x2)

	return nil
}

func (e *ScriptEngine) op2Swap() error {
	if e.stack.Size() < 4 {
		return fmt.Errorf("stack underflow: need 4 elements for 2SWAP")
	}

	// Pop 4 elements: x1 x2 x3 x4
	// Push back: x3 x4 x1 x2
	x4, _ := e.stack.Pop()
	x3, _ := e.stack.Pop()
	x2, _ := e.stack.Pop()
	x1, _ := e.stack.Pop()

	e.stack.Push(x3)
	e.stack.Push(x4)
	e.stack.Push(x1)
	e.stack.Push(x2)

	return nil
}

func (e *ScriptEngine) opIfDup() error {
	data, err := e.stack.Peek(0)
	if err != nil {
		return err
	}

	if isTrue(data) {
		return e.stack.Push(data)
	}
	return nil
}

func (e *ScriptEngine) opDepth() error {
	depth := numberToBytes(big.NewInt(int64(e.stack.Size())))
	return e.stack.Push(depth)
}

func (e *ScriptEngine) opDrop() error {
	_, err := e.stack.Pop()
	return err
}

func (e *ScriptEngine) opDup() error {
	data, err := e.stack.Peek(0)
	if err != nil {
		return err
	}
	return e.stack.Push(data)
}

func (e *ScriptEngine) opNip() error {
	if e.stack.Size() < 2 {
		return fmt.Errorf("stack underflow: need 2 elements for NIP")
	}

	top, _ := e.stack.Pop()
	_, _ = e.stack.Pop()
	return e.stack.Push(top)
}

func (e *ScriptEngine) opOver() error {
	if e.stack.Size() < 2 {
		return fmt.Errorf("stack underflow: need 2 elements for OVER")
	}

	data, err := e.stack.Peek(1)
	if err != nil {
		return err
	}
	return e.stack.Push(data)
}

func (e *ScriptEngine) opPick() error {
	nData, err := e.stack.Pop()
	if err != nil {
		return err
	}

	n, err := numberFromBytes(nData)
	if err != nil {
		return err
	}

	if n.Sign() < 0 || !n.IsInt64() {
		return fmt.Errorf("invalid pick index")
	}

	index := int(n.Int64())
	if index >= e.stack.Size() {
		return fmt.Errorf("stack index out of bounds")
	}

	data, err := e.stack.Peek(index)
	if err != nil {
		return err
	}
	return e.stack.Push(data)
}

func (e *ScriptEngine) opRoll() error {
	nData, err := e.stack.Pop()
	if err != nil {
		return err
	}

	n, err := numberFromBytes(nData)
	if err != nil {
		return err
	}

	if n.Sign() < 0 || !n.IsInt64() {
		return fmt.Errorf("invalid roll index")
	}

	index := int(n.Int64())
	if index >= e.stack.Size() {
		return fmt.Errorf("stack index out of bounds")
	}

	if index == 0 {
		return nil // No-op if rolling top element
	}

	// Remove element at index and push to top
	stackSize := e.stack.Size()
	targetIndex := stackSize - 1 - index

	data := e.stack.data[targetIndex]
	copy(e.stack.data[targetIndex:], e.stack.data[targetIndex+1:])
	e.stack.data[len(e.stack.data)-1] = data

	return nil
}

func (e *ScriptEngine) opRot() error {
	if e.stack.Size() < 3 {
		return fmt.Errorf("stack underflow: need 3 elements for ROT")
	}

	// Pop 3 elements: x1 x2 x3
	// Push back: x2 x3 x1
	x3, _ := e.stack.Pop()
	x2, _ := e.stack.Pop()
	x1, _ := e.stack.Pop()

	e.stack.Push(x2)
	e.stack.Push(x3)
	e.stack.Push(x1)

	return nil
}

func (e *ScriptEngine) opSwap() error {
	return e.stack.Swap()
}

func (e *ScriptEngine) opTuck() error {
	if e.stack.Size() < 2 {
		return fmt.Errorf("stack underflow: need 2 elements for TUCK")
	}

	// Pop 2 elements: x1 x2
	// Push back: x2 x1 x2
	x2, _ := e.stack.Pop()
	x1, _ := e.stack.Pop()

	e.stack.Push(x2)
	e.stack.Push(x1)
	e.stack.Push(x2)

	return nil
}

func (e *ScriptEngine) opSize() error {
	data, err := e.stack.Peek(0)
	if err != nil {
		return err
	}

	size := numberToBytes(big.NewInt(int64(len(data))))
	return e.stack.Push(size)
}

// Bitwise logic operations

func (e *ScriptEngine) opEqual() error {
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}

	result := []byte{}
	if bytes.Equal(a, b) {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opEqualVerify() error {
	if err := e.opEqual(); err != nil {
		return err
	}
	return e.opVerify()
}

// Arithmetic operations

func (e *ScriptEngine) op1Add() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	num, err := numberFromBytes(data)
	if err != nil {
		return err
	}

	result := new(big.Int).Add(num, big.NewInt(1))
	return e.stack.Push(numberToBytes(result))
}

func (e *ScriptEngine) op1Sub() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	num, err := numberFromBytes(data)
	if err != nil {
		return err
	}

	result := new(big.Int).Sub(num, big.NewInt(1))
	return e.stack.Push(numberToBytes(result))
}

func (e *ScriptEngine) opNegate() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	num, err := numberFromBytes(data)
	if err != nil {
		return err
	}

	result := new(big.Int).Neg(num)
	return e.stack.Push(numberToBytes(result))
}

func (e *ScriptEngine) opAbs() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	num, err := numberFromBytes(data)
	if err != nil {
		return err
	}

	result := new(big.Int).Abs(num)
	return e.stack.Push(numberToBytes(result))
}

func (e *ScriptEngine) opNot() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	result := []byte{}
	if !isTrue(data) {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) op0NotEqual() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	result := []byte{}
	if isTrue(data) {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opAdd() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numA, err := numberFromBytes(a)
	if err != nil {
		return err
	}
	numB, err := numberFromBytes(b)
	if err != nil {
		return err
	}

	result := new(big.Int).Add(numA, numB)
	return e.stack.Push(numberToBytes(result))
}

func (e *ScriptEngine) opSub() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numA, err := numberFromBytes(a)
	if err != nil {
		return err
	}
	numB, err := numberFromBytes(b)
	if err != nil {
		return err
	}

	result := new(big.Int).Sub(numA, numB)
	return e.stack.Push(numberToBytes(result))
}

func (e *ScriptEngine) opBoolAnd() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	result := []byte{}
	if isTrue(a) && isTrue(b) {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opBoolOr() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	result := []byte{}
	if isTrue(a) || isTrue(b) {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opNumEqual() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numA, err := numberFromBytes(a)
	if err != nil {
		return err
	}
	numB, err := numberFromBytes(b)
	if err != nil {
		return err
	}

	result := []byte{}
	if numA.Cmp(numB) == 0 {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opNumEqualVerify() error {
	if err := e.opNumEqual(); err != nil {
		return err
	}
	return e.opVerify()
}

func (e *ScriptEngine) opNumNotEqual() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numA, err := numberFromBytes(a)
	if err != nil {
		return err
	}
	numB, err := numberFromBytes(b)
	if err != nil {
		return err
	}

	result := []byte{}
	if numA.Cmp(numB) != 0 {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opLessThan() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numA, err := numberFromBytes(a)
	if err != nil {
		return err
	}
	numB, err := numberFromBytes(b)
	if err != nil {
		return err
	}

	result := []byte{}
	if numA.Cmp(numB) < 0 {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opGreaterThan() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numA, err := numberFromBytes(a)
	if err != nil {
		return err
	}
	numB, err := numberFromBytes(b)
	if err != nil {
		return err
	}

	result := []byte{}
	if numA.Cmp(numB) > 0 {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opLessThanOrEqual() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numA, err := numberFromBytes(a)
	if err != nil {
		return err
	}
	numB, err := numberFromBytes(b)
	if err != nil {
		return err
	}

	result := []byte{}
	if numA.Cmp(numB) <= 0 {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opGreaterThanOrEqual() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numA, err := numberFromBytes(a)
	if err != nil {
		return err
	}
	numB, err := numberFromBytes(b)
	if err != nil {
		return err
	}

	result := []byte{}
	if numA.Cmp(numB) >= 0 {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opMin() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numA, err := numberFromBytes(a)
	if err != nil {
		return err
	}
	numB, err := numberFromBytes(b)
	if err != nil {
		return err
	}

	var result *big.Int
	if numA.Cmp(numB) < 0 {
		result = numA
	} else {
		result = numB
	}

	return e.stack.Push(numberToBytes(result))
}

func (e *ScriptEngine) opMax() error {
	b, err := e.stack.Pop()
	if err != nil {
		return err
	}
	a, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numA, err := numberFromBytes(a)
	if err != nil {
		return err
	}
	numB, err := numberFromBytes(b)
	if err != nil {
		return err
	}

	var result *big.Int
	if numA.Cmp(numB) > 0 {
		result = numA
	} else {
		result = numB
	}

	return e.stack.Push(numberToBytes(result))
}

func (e *ScriptEngine) opWithin() error {
	max, err := e.stack.Pop()
	if err != nil {
		return err
	}
	min, err := e.stack.Pop()
	if err != nil {
		return err
	}
	x, err := e.stack.Pop()
	if err != nil {
		return err
	}

	numX, err := numberFromBytes(x)
	if err != nil {
		return err
	}
	numMin, err := numberFromBytes(min)
	if err != nil {
		return err
	}
	numMax, err := numberFromBytes(max)
	if err != nil {
		return err
	}

	result := []byte{}
	if numX.Cmp(numMin) >= 0 && numX.Cmp(numMax) < 0 {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

// Crypto operations

func (e *ScriptEngine) opRipemd160() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	hasher := ripemd160.New()
	hasher.Write(data)
	hash := hasher.Sum(nil)

	return e.stack.Push(hash)
}

func (e *ScriptEngine) opSha256() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	hash := sha256.Sum256(data)
	return e.stack.Push(hash[:])
}

func (e *ScriptEngine) opHash160() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	hash := scriptHash160(data)
	return e.stack.Push(hash)
}

func (e *ScriptEngine) opHash256() error {
	data, err := e.stack.Pop()
	if err != nil {
		return err
	}

	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return e.stack.Push(second[:])
}

func (e *ScriptEngine) opCheckSig() error {
	pubkeyData, err := e.stack.Pop()
	if err != nil {
		return err
	}
	sigData, err := e.stack.Pop()
	if err != nil {
		return err
	}

	success := false
	if len(sigData) > 0 {
		// Extract signature and hash type
		hashType := sigData[len(sigData)-1]
		derSig := sigData[:len(sigData)-1]

		// Calculate signature hash based on transaction data
		// This requires transaction context which we don't have in this isolated script engine
		// For now, we'll compute a deterministic hash but this needs integration with transaction data
		messageHash, err := e.calculateSignatureHash(hashType)
		if err == nil {
			success = e.verifySignature(pubkeyData, derSig, messageHash)
		}
	}

	result := []byte{}
	if success {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opCheckSigVerify() error {
	if err := e.opCheckSig(); err != nil {
		return err
	}
	return e.opVerify()
}

func (e *ScriptEngine) verifySignature(pubkeyData, derSig, messageHash []byte) bool {
	if len(pubkeyData) == 0 || len(derSig) == 0 {
		return false
	}

	// Parse public key
	pubkey, err := btcec.ParsePubKey(pubkeyData)
	if err != nil {
		return false
	}

	// Parse DER signature
	sig, err := ecdsa.ParseDERSignature(derSig)
	if err != nil {
		return false
	}

	// Verify signature against message hash
	return sig.Verify(messageHash, pubkey)
}

// calculateSignatureHash computes the ZIP-244 signature hash for transparent inputs.
func (e *ScriptEngine) calculateSignatureHash(hashType byte) ([]byte, error) {
	if e.txContext == nil || e.txContext.Tx == nil {
		return nil, fmt.Errorf("missing transaction context")
	}
	if e.txContext.ScriptCode == nil {
		return nil, fmt.Errorf("missing script code")
	}

	sighash, err := ComputeSignatureHashV5(
		e.txContext.Tx,
		e.txContext.InputIndex,
		hashType,
		e.txContext.PrevOutValue,
		e.txContext.ScriptCode,
	)
	if err != nil {
		return nil, err
	}
	return sighash[:], nil
}

func (e *ScriptEngine) opCheckMultiSig() error {
	// Pop number of public keys
	nPubkeysData, err := e.stack.Pop()
	if err != nil {
		return err
	}

	nPubkeys, err := numberFromBytes(nPubkeysData)
	if err != nil {
		return err
	}

	if nPubkeys.Sign() < 0 || !nPubkeys.IsInt64() {
		return fmt.Errorf("invalid number of public keys")
	}

	numPubkeys := int(nPubkeys.Int64())
	if numPubkeys > MaxPubkeysPerMulti {
		return fmt.Errorf("too many public keys: %d > %d", numPubkeys, MaxPubkeysPerMulti)
	}

	// Pop public keys
	pubkeys := make([][]byte, numPubkeys)
	for i := 0; i < numPubkeys; i++ {
		pubkey, err := e.stack.Pop()
		if err != nil {
			return err
		}
		pubkeys[i] = pubkey
	}

	// Pop number of signatures
	nSigsData, err := e.stack.Pop()
	if err != nil {
		return err
	}

	nSigs, err := numberFromBytes(nSigsData)
	if err != nil {
		return err
	}

	if nSigs.Sign() < 0 || !nSigs.IsInt64() {
		return fmt.Errorf("invalid number of signatures")
	}

	numSigs := int(nSigs.Int64())
	if numSigs > numPubkeys {
		return fmt.Errorf("more signatures than public keys: %d > %d", numSigs, numPubkeys)
	}

	// Pop signatures
	sigs := make([][]byte, numSigs)
	for i := 0; i < numSigs; i++ {
		sig, err := e.stack.Pop()
		if err != nil {
			return err
		}
		sigs[i] = sig
	}

	// Bitcoin CHECKMULTISIG bug: pop an extra value off the stack
	if e.stack.Size() == 0 {
		return fmt.Errorf("CHECKMULTISIG missing dummy value")
	}
	dummy, err := e.stack.Pop()
	if err != nil {
		return err
	}
	if len(dummy) != 0 {
		return fmt.Errorf("CHECKMULTISIG dummy value not null (NULLDUMMY)")
	}

	// Verify signatures
	success := e.verifyMultiSig(sigs, pubkeys)

	result := []byte{}
	if success {
		result = []byte{1}
	}

	return e.stack.Push(result)
}

func (e *ScriptEngine) opCheckMultiSigVerify() error {
	if err := e.opCheckMultiSig(); err != nil {
		return err
	}
	return e.opVerify()
}

func (e *ScriptEngine) verifyMultiSig(sigs [][]byte, pubkeys [][]byte) bool {
	if len(sigs) == 0 {
		return true // No signatures to verify
	}

	sigIndex := 0
	for _, pubkeyData := range pubkeys {
		if sigIndex >= len(sigs) {
			break
		}

		sigData := sigs[sigIndex]
		if len(sigData) == 0 {
			continue
		}

		// Extract signature and hash type
		if len(sigData) < 1 {
			continue
		}
		hashType := sigData[len(sigData)-1]
		derSig := sigData[:len(sigData)-1]
		if len(derSig) == 0 {
			continue
		}

		// Calculate signature hash for this signature
		messageHash, err := e.calculateSignatureHash(hashType)
		if err == nil && e.verifySignature(pubkeyData, derSig, messageHash) {
			sigIndex++
		}
	}

	// All signatures must be verified
	return sigIndex == len(sigs)
}

// scriptHash160 performs SHA256 followed by RIPEMD160 for Bitcoin Script
func scriptHash160(data []byte) []byte {
	sha := sha256.Sum256(data)
	hasher := ripemd160.New()
	hasher.Write(sha[:])
	return hasher.Sum(nil)
}
