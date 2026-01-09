package rpc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"golang.org/x/crypto/ripemd160"
)

// Zcash v5 (NU5) transaction constants
const (
	TxVersionNU5       = 5
	TxVersionGroupNU5  = 0x26A7270A
	ConsensusBranchNU5 = 0xC2D6D0B4 // NU5 mainnet
)

// ZcashTxV5 represents a parsed Zcash v5 (NU5) transparent transaction
type ZcashTxV5 struct {
	// Header
	Version           uint32
	VersionGroupID    uint32
	ConsensusBranchID uint32
	LockTime          uint32
	ExpiryHeight      uint32

	// Transparent bundle
	Inputs  []ZcashTxInput
	Outputs []ZcashTxOutput

	// Shielded components (must be empty for transparent-only)
	NumSpendsSapling  uint64
	NumOutputsSapling uint64
	NumActionsOrchard uint64

	// Computed
	TxID        [32]byte // Internal byte order (big-endian)
	TxIDDisplay string   // Little-endian hex for display
	RawBytes    []byte   // Full serialized transaction
}

// ZcashTxInput represents a transparent input
type ZcashTxInput struct {
	PrevOutHash  [32]byte // Internal byte order
	PrevOutIndex uint32
	ScriptSig    []byte
	Sequence     uint32
}

// ZcashTxOutput represents a transparent output
type ZcashTxOutput struct {
	Value        uint64 // Zatoshis
	ScriptPubKey []byte
}

// ParseZcashTxV5 parses a raw Zcash v5 (NU5) transaction
func ParseZcashTxV5(rawTx []byte) (*ZcashTxV5, error) {
	r := bytes.NewReader(rawTx)
	tx := &ZcashTxV5{RawBytes: rawTx}

	// Version
	if err := binary.Read(r, binary.LittleEndian, &tx.Version); err != nil {
		return nil, fmt.Errorf("read version: %w", err)
	}
	if tx.Version != TxVersionNU5 {
		return nil, fmt.Errorf("not a v5 transaction: version=%d", tx.Version)
	}

	// Version group ID
	if err := binary.Read(r, binary.LittleEndian, &tx.VersionGroupID); err != nil {
		return nil, fmt.Errorf("read version group ID: %w", err)
	}
	if tx.VersionGroupID != TxVersionGroupNU5 {
		return nil, fmt.Errorf("invalid v5 version group: 0x%08x", tx.VersionGroupID)
	}

	// Consensus branch ID (serialized in v5)
	if err := binary.Read(r, binary.LittleEndian, &tx.ConsensusBranchID); err != nil {
		return nil, fmt.Errorf("read consensus branch ID: %w", err)
	}

	// Locktime
	if err := binary.Read(r, binary.LittleEndian, &tx.LockTime); err != nil {
		return nil, fmt.Errorf("read locktime: %w", err)
	}

	// Expiry height
	if err := binary.Read(r, binary.LittleEndian, &tx.ExpiryHeight); err != nil {
		return nil, fmt.Errorf("read expiry height: %w", err)
	}

	// Transparent inputs
	vinCount, err := readVarInt(r)
	if err != nil {
		return nil, fmt.Errorf("read vin count: %w", err)
	}
	tx.Inputs = make([]ZcashTxInput, vinCount)
	for i := range tx.Inputs {
		if err := readInput(r, &tx.Inputs[i]); err != nil {
			return nil, fmt.Errorf("read input %d: %w", i, err)
		}
	}

	// Transparent outputs
	voutCount, err := readVarInt(r)
	if err != nil {
		return nil, fmt.Errorf("read vout count: %w", err)
	}
	tx.Outputs = make([]ZcashTxOutput, voutCount)
	for i := range tx.Outputs {
		if err := readOutput(r, &tx.Outputs[i]); err != nil {
			return nil, fmt.Errorf("read output %d: %w", i, err)
		}
	}

	// Sapling spends
	tx.NumSpendsSapling, err = readVarInt(r)
	if err != nil {
		return nil, fmt.Errorf("read sapling spends: %w", err)
	}
	if tx.NumSpendsSapling > 0 {
		return nil, fmt.Errorf("sapling spends not supported (transparent-only)")
	}

	// Sapling outputs
	tx.NumOutputsSapling, err = readVarInt(r)
	if err != nil {
		return nil, fmt.Errorf("read sapling outputs: %w", err)
	}
	if tx.NumOutputsSapling > 0 {
		return nil, fmt.Errorf("sapling outputs not supported (transparent-only)")
	}

	// Orchard actions
	tx.NumActionsOrchard, err = readVarInt(r)
	if err != nil {
		return nil, fmt.Errorf("read orchard actions: %w", err)
	}
	if tx.NumActionsOrchard > 0 {
		return nil, fmt.Errorf("orchard actions not supported (transparent-only)")
	}

	// Compute v5 TxID (ZIP-244 BLAKE2b)
	txid, err := computeV5TxID(tx)
	if err != nil {
		return nil, fmt.Errorf("compute txid: %w", err)
	}
	tx.TxID = txid
	tx.TxIDDisplay = txIDToDisplay(txid)

	return tx, nil
}

func readInput(r io.Reader, input *ZcashTxInput) error {
	if _, err := io.ReadFull(r, input.PrevOutHash[:]); err != nil {
		return fmt.Errorf("read prevout hash: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &input.PrevOutIndex); err != nil {
		return fmt.Errorf("read prevout index: %w", err)
	}
	scriptSig, err := readVarBytes(r)
	if err != nil {
		return fmt.Errorf("read scriptSig: %w", err)
	}
	input.ScriptSig = scriptSig
	if err := binary.Read(r, binary.LittleEndian, &input.Sequence); err != nil {
		return fmt.Errorf("read sequence: %w", err)
	}
	return nil
}

func readOutput(r io.Reader, output *ZcashTxOutput) error {
	if err := binary.Read(r, binary.LittleEndian, &output.Value); err != nil {
		return fmt.Errorf("read value: %w", err)
	}
	scriptPubKey, err := readVarBytes(r)
	if err != nil {
		return fmt.Errorf("read scriptPubKey: %w", err)
	}
	output.ScriptPubKey = scriptPubKey
	return nil
}

func readVarInt(r io.Reader) (uint64, error) {
	var first [1]byte
	if _, err := io.ReadFull(r, first[:]); err != nil {
		return 0, err
	}
	switch first[0] {
	case 0xfd:
		var val uint16
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return 0, err
		}
		return uint64(val), nil
	case 0xfe:
		var val uint32
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return 0, err
		}
		return uint64(val), nil
	case 0xff:
		var val uint64
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return 0, err
		}
		return val, nil
	default:
		return uint64(first[0]), nil
	}
}

func readVarBytes(r io.Reader) ([]byte, error) {
	length, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	if length > 10000 {
		return nil, fmt.Errorf("script too large: %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func writeVarInt(w io.Writer, n uint64) {
	if n < 0xfd {
		w.Write([]byte{byte(n)})
	} else if n <= 0xffff {
		w.Write([]byte{0xfd})
		binary.Write(w, binary.LittleEndian, uint16(n))
	} else if n <= 0xffffffff {
		w.Write([]byte{0xfe})
		binary.Write(w, binary.LittleEndian, uint32(n))
	} else {
		w.Write([]byte{0xff})
		binary.Write(w, binary.LittleEndian, n)
	}
}

func personalizationFromString(value string) [16]byte {
	var out [16]byte
	copy(out[:], value)
	return out
}

func blake2b256PersonalizedParts(personalization [16]byte, parts ...[]byte) [32]byte {
	total := 0
	for _, part := range parts {
		total += len(part)
	}
	buf := make([]byte, 0, total)
	for _, part := range parts {
		buf = append(buf, part...)
	}
	return blake2b256Personalized(personalization, buf)
}

func appendUint32LE(buf []byte, value uint32) []byte {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], value)
	return append(buf, tmp[:]...)
}

func appendUint64LE(buf []byte, value uint64) []byte {
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], value)
	return append(buf, tmp[:]...)
}

func appendVarInt(buf []byte, n uint64) []byte {
	switch {
	case n < 0xfd:
		return append(buf, byte(n))
	case n <= 0xffff:
		var tmp [2]byte
		binary.LittleEndian.PutUint16(tmp[:], uint16(n))
		buf = append(buf, 0xfd)
		return append(buf, tmp[:]...)
	case n <= 0xffffffff:
		var tmp [4]byte
		binary.LittleEndian.PutUint32(tmp[:], uint32(n))
		buf = append(buf, 0xfe)
		return append(buf, tmp[:]...)
	default:
		var tmp [8]byte
		binary.LittleEndian.PutUint64(tmp[:], n)
		buf = append(buf, 0xff)
		return append(buf, tmp[:]...)
	}
}

// computeV5TxID implements ZIP-244 TxID computation
func computeV5TxID(tx *ZcashTxV5) ([32]byte, error) {
	var personalization [16]byte
	copy(personalization[:], "ZcashTxHash_")
	binary.LittleEndian.PutUint32(personalization[12:], tx.ConsensusBranchID)

	headerDigest := hashV5Header(tx)
	transparentDigest := hashV5Transparent(tx)
	saplingDigest := [32]byte{} // Empty
	orchardDigest := [32]byte{} // Empty

	txid := blake2b256PersonalizedParts(
		personalization,
		headerDigest[:],
		transparentDigest[:],
		saplingDigest[:],
		orchardDigest[:],
	)
	return txid, nil
}

func hashV5Header(tx *ZcashTxV5) [32]byte {
	personalization := personalizationFromString("ZTxIdHeadersHash")
	buf := make([]byte, 0, 20)
	buf = appendUint32LE(buf, tx.Version)
	buf = appendUint32LE(buf, tx.VersionGroupID)
	buf = appendUint32LE(buf, tx.ConsensusBranchID)
	buf = appendUint32LE(buf, tx.LockTime)
	buf = appendUint32LE(buf, tx.ExpiryHeight)
	return blake2b256Personalized(personalization, buf)
}

func hashV5Transparent(tx *ZcashTxV5) [32]byte {
	if len(tx.Inputs) == 0 {
		return [32]byte{}
	}

	prevoutsDigest := hashPrevouts(tx.Inputs)
	sequenceDigest := hashSequence(tx.Inputs)
	outputsDigest := hashOutputs(tx.Outputs)

	personalization := personalizationFromString("ZTxIdTranspaHash")
	return blake2b256PersonalizedParts(
		personalization,
		prevoutsDigest[:],
		sequenceDigest[:],
		outputsDigest[:],
	)
}

func hashPrevouts(inputs []ZcashTxInput) [32]byte {
	personalization := personalizationFromString("ZTxIdPrevoutHash")
	buf := make([]byte, 0, len(inputs)*36)
	for _, input := range inputs {
		buf = append(buf, input.PrevOutHash[:]...)
		buf = appendUint32LE(buf, input.PrevOutIndex)
	}
	return blake2b256Personalized(personalization, buf)
}

func hashSequence(inputs []ZcashTxInput) [32]byte {
	personalization := personalizationFromString("ZTxIdSequencHash")
	buf := make([]byte, 0, len(inputs)*4)
	for _, input := range inputs {
		buf = appendUint32LE(buf, input.Sequence)
	}
	return blake2b256Personalized(personalization, buf)
}

func hashOutputs(outputs []ZcashTxOutput) [32]byte {
	personalization := personalizationFromString("ZTxIdOutputsHash")
	buf := make([]byte, 0, len(outputs)*40)
	for _, output := range outputs {
		buf = appendUint64LE(buf, output.Value)
		buf = appendVarInt(buf, uint64(len(output.ScriptPubKey)))
		buf = append(buf, output.ScriptPubKey...)
	}
	return blake2b256Personalized(personalization, buf)
}

func txIDToDisplay(txid [32]byte) string {
	reversed := reverseBytes(txid[:])
	return fmt.Sprintf("%x", reversed)
}

// IsCoinbase returns true if input is a coinbase input
func (input *ZcashTxInput) IsCoinbase() bool {
	for _, b := range input.PrevOutHash {
		if b != 0 {
			return false
		}
	}
	return input.PrevOutIndex == 0xFFFFFFFF
}

// ComputeSignatureHashV5 implements ZIP-244 signature hash
func ComputeSignatureHashV5(tx *ZcashTxV5, inputIdx int, hashType byte, value uint64, scriptCode []byte) ([32]byte, error) {
	if inputIdx < 0 || inputIdx >= len(tx.Inputs) {
		return [32]byte{}, fmt.Errorf("invalid input index: %d", inputIdx)
	}

	var personalization [16]byte
	copy(personalization[:], "ZcashTxHash_")
	binary.LittleEndian.PutUint32(personalization[12:], tx.ConsensusBranchID)

	headerDigest := hashV5Header(tx)
	transparentDigest := hashV5Transparent(tx)
	saplingDigest := [32]byte{}
	orchardDigest := [32]byte{}
	txinSigDigest := hashTxInSig(tx, inputIdx, value, scriptCode)

	sighash := blake2b256PersonalizedParts(
		personalization,
		headerDigest[:],
		transparentDigest[:],
		saplingDigest[:],
		orchardDigest[:],
		txinSigDigest[:],
		[]byte{hashType},
	)
	return sighash, nil
}

func hashTxInSig(tx *ZcashTxV5, inputIdx int, value uint64, scriptCode []byte) [32]byte {
	input := tx.Inputs[inputIdx]
	personalization := personalizationFromString("Zcash___TxInHash")
	buf := make([]byte, 0, 36+8+len(scriptCode)+8)
	buf = append(buf, input.PrevOutHash[:]...)
	buf = appendUint32LE(buf, input.PrevOutIndex)
	buf = appendUint64LE(buf, value)
	buf = appendVarInt(buf, uint64(len(scriptCode)))
	buf = append(buf, scriptCode...)
	buf = appendUint32LE(buf, input.Sequence)
	return blake2b256Personalized(personalization, buf)
}

// VerifyP2PKHSignature verifies a P2PKH signature
func VerifyP2PKHSignature(tx *ZcashTxV5, inputIdx int, value uint64, scriptPubKey []byte, scriptSig []byte) error {
	// Extract signature and pubkey from scriptSig
	sig, pubkey, err := extractP2PKHScriptSig(scriptSig)
	if err != nil {
		return fmt.Errorf("extract scriptSig: %w", err)
	}

	// Verify scriptPubKey format (OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG)
	if len(scriptPubKey) != 25 ||
		scriptPubKey[0] != 0x76 || // OP_DUP
		scriptPubKey[1] != 0xa9 || // OP_HASH160
		scriptPubKey[2] != 0x14 || // Push 20 bytes
		scriptPubKey[23] != 0x88 || // OP_EQUALVERIFY
		scriptPubKey[24] != 0xac { // OP_CHECKSIG
		return fmt.Errorf("invalid P2PKH scriptPubKey")
	}

	pubkeyHash := scriptPubKey[3:23]

	// Verify pubkey hash (HASH160 = RIPEMD160(SHA256(pubkey)))
	computedHash := hash160(pubkey)
	if !bytes.Equal(computedHash, pubkeyHash) {
		return fmt.Errorf("pubkey hash mismatch")
	}

	// Extract sighash type
	if len(sig) < 1 {
		return fmt.Errorf("signature too short")
	}
	hashType := sig[len(sig)-1]
	derSig := sig[:len(sig)-1]

	// Compute sighash
	sighash, err := ComputeSignatureHashV5(tx, inputIdx, hashType, value, scriptPubKey)
	if err != nil {
		return fmt.Errorf("compute sighash: %w", err)
	}

	// Verify ECDSA signature
	if err := verifyECDSA(pubkey, sighash[:], derSig); err != nil {
		return fmt.Errorf("ecdsa verify: %w", err)
	}

	return nil
}

func extractP2PKHScriptSig(scriptSig []byte) (sig []byte, pubkey []byte, err error) {
	cursor := 0
	sig, err = readScriptPush(scriptSig, &cursor)
	if err != nil {
		return nil, nil, err
	}
	pubkey, err = readScriptPush(scriptSig, &cursor)
	if err != nil {
		return nil, nil, err
	}
	if cursor != len(scriptSig) {
		return nil, nil, fmt.Errorf("unexpected trailing bytes in scriptSig")
	}
	return sig, pubkey, nil
}

func readScriptPush(script []byte, cursor *int) ([]byte, error) {
	if *cursor >= len(script) {
		return nil, fmt.Errorf("script push out of bounds")
	}

	opcode := script[*cursor]
	*cursor++

	var length int
	switch {
	case opcode == 0x00:
		length = 0
	case opcode >= 0x01 && opcode <= 0x4b:
		length = int(opcode)
	case opcode == 0x4c:
		if *cursor >= len(script) {
			return nil, fmt.Errorf("pushdata1 out of bounds")
		}
		length = int(script[*cursor])
		*cursor++
	case opcode == 0x4d:
		if *cursor+2 > len(script) {
			return nil, fmt.Errorf("pushdata2 out of bounds")
		}
		length = int(binary.LittleEndian.Uint16(script[*cursor : *cursor+2]))
		*cursor += 2
	case opcode == 0x4e:
		if *cursor+4 > len(script) {
			return nil, fmt.Errorf("pushdata4 out of bounds")
		}
		length = int(binary.LittleEndian.Uint32(script[*cursor : *cursor+4]))
		*cursor += 4
	default:
		return nil, fmt.Errorf("unsupported push opcode: 0x%02x", opcode)
	}

	if *cursor+length > len(script) {
		return nil, fmt.Errorf("pushdata exceeds script length")
	}

	data := make([]byte, length)
	copy(data, script[*cursor:*cursor+length])
	*cursor += length
	return data, nil
}

func hash160(data []byte) []byte {
	// Bitcoin/Zcash HASH160 = RIPEMD160(SHA256(data))
	sha := sha256.Sum256(data)
	hasher := ripemd160.New()
	hasher.Write(sha[:])
	return hasher.Sum(nil)
}

func verifyECDSA(pubkey []byte, msg []byte, sig []byte) error {
	// Parse public key
	pk, err := btcec.ParsePubKey(pubkey)
	if err != nil {
		return fmt.Errorf("parse pubkey: %w", err)
	}

	// Parse DER signature
	signature, err := ecdsa.ParseDERSignature(sig)
	if err != nil {
		return fmt.Errorf("parse signature: %w", err)
	}

	// Verify signature
	if !signature.Verify(msg, pk) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// P2SH (Pay-to-Script-Hash) support functions

// isP2SH checks if a script is P2SH format: OP_HASH160 <20-byte hash> OP_EQUAL
func isP2SH(script []byte) bool {
	return len(script) == 23 &&
		script[0] == 0xa9 && // OP_HASH160
		script[1] == 0x14 && // Push 20 bytes
		script[22] == 0x87 // OP_EQUAL
}

// isP2PKH checks if a script is P2PKH format: OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
func isP2PKH(script []byte) bool {
	return len(script) == 25 &&
		script[0] == 0x76 && // OP_DUP
		script[1] == 0xa9 && // OP_HASH160
		script[2] == 0x14 && // Push 20 bytes
		script[23] == 0x88 && // OP_EQUALVERIFY
		script[24] == 0xac // OP_CHECKSIG
}

// extractP2SHHash extracts the script hash from a P2SH scriptPubKey
func extractP2SHHash(script []byte) ([]byte, error) {
	if !isP2SH(script) {
		return nil, fmt.Errorf("not a P2SH script")
	}
	hash := make([]byte, 20)
	copy(hash, script[2:22])
	return hash, nil
}

// extractRedeemScript extracts the redeem script from P2SH scriptSig (last push operation)
func extractRedeemScript(scriptSig []byte) ([]byte, error) {
	cursor := 0
	var lastPush []byte

	for cursor < len(scriptSig) {
		data, err := readScriptPush(scriptSig, &cursor)
		if err != nil {
			return nil, fmt.Errorf("read script push: %w", err)
		}
		lastPush = data
	}

	if lastPush == nil {
		return nil, fmt.Errorf("no redeem script found")
	}

	return lastPush, nil
}

// extractUnlockingData extracts all pushes except the last one (redeem script)
func extractUnlockingData(scriptSig []byte) ([][]byte, error) {
	cursor := 0
	var pushes [][]byte

	for cursor < len(scriptSig) {
		data, err := readScriptPush(scriptSig, &cursor)
		if err != nil {
			return nil, fmt.Errorf("read script push: %w", err)
		}
		pushes = append(pushes, data)
	}

	// Remove last push (redeem script)
	if len(pushes) > 0 {
		pushes = pushes[:len(pushes)-1]
	}

	return pushes, nil
}

// VerifyP2SHSignature verifies a P2SH signature using full script execution.
func VerifyP2SHSignature(tx *ZcashTxV5, inputIdx int, value uint64, scriptPubKey []byte, scriptSig []byte) error {
	redeemScript, err := extractRedeemScript(scriptSig)
	if err != nil {
		return fmt.Errorf("extract redeem script: %w", err)
	}

	// Verify redeem script size limit (BIP 16: max 520 bytes)
	if len(redeemScript) > 520 {
		return fmt.Errorf("redeem script too large: %d bytes", len(redeemScript))
	}

	// Compute hash160 of redeem script
	computedHash := hash160(redeemScript)

	// Extract expected hash from P2SH scriptPubKey
	expectedHash, err := extractP2SHHash(scriptPubKey)
	if err != nil {
		return fmt.Errorf("extract P2SH hash: %w", err)
	}

	// Verify hashes match
	if !bytes.Equal(computedHash, expectedHash) {
		return fmt.Errorf("script hash mismatch")
	}

	txContext := &TransactionContext{
		Tx:           tx,
		InputIndex:   inputIdx,
		PrevOutValue: value,
		ScriptCode:   redeemScript,
	}
	engine := NewScriptEngineWithContext(txContext)
	if err := engine.VerifyScript(scriptSig, scriptPubKey); err != nil {
		return fmt.Errorf("script execution failed: %w", err)
	}
	return nil
}
