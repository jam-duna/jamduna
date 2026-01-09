package rpc

import (
	"crypto/sha256"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

func buildTestTx(scriptSig, scriptPubKey []byte) *ZcashTxV5 {
	return &ZcashTxV5{
		Version:           TxVersionNU5,
		VersionGroupID:    TxVersionGroupNU5,
		ConsensusBranchID: ConsensusBranchNU5,
		LockTime:          0,
		ExpiryHeight:      0,
		Inputs: []ZcashTxInput{{
			PrevOutHash:  [32]byte{0x01},
			PrevOutIndex: 0,
			ScriptSig:    scriptSig,
			Sequence:     0xFFFFFFFF,
		}},
		Outputs: []ZcashTxOutput{{
			Value:        1000,
			ScriptPubKey: scriptPubKey,
		}},
	}
}

func TestRealSignatureVerification(t *testing.T) {
	t.Run("ValidSignature", func(t *testing.T) {
		// Generate a test key pair
		privateKey, err := btcec.NewPrivateKey()
		if err != nil {
			t.Fatalf("Failed to generate private key: %v", err)
		}

		publicKey := privateKey.PubKey()
		messageHash := sha256.Sum256([]byte("test message"))

		// Sign the message
		signature := ecdsa.Sign(privateKey, messageHash[:])
		derSig := signature.Serialize()

		pubkeyHash := scriptHash160(publicKey.SerializeCompressed())
		scriptPubKey := []byte{OP_DUP, OP_HASH160, 0x14}
		scriptPubKey = append(scriptPubKey, pubkeyHash...)
		scriptPubKey = append(scriptPubKey, OP_EQUALVERIFY, OP_CHECKSIG)
		tx := buildTestTx(nil, scriptPubKey)

		// Create transaction context
		txContext := &TransactionContext{
			Tx:           tx,
			InputIndex:   0,
			PrevOutValue: 100000,
			ScriptCode:   scriptPubKey,
		}

		engine := NewScriptEngineWithContext(txContext)
		engine.stack.Push(append(derSig, 0x01)) // signature with SIGHASH_ALL
		engine.stack.Push(publicKey.SerializeCompressed())

		// This should fail because our signature was made with a different message hash
		// than what the script engine calculates
		err = engine.opCheckSig()
		if err != nil {
			t.Fatalf("OP_CHECKSIG failed: %v", err)
		}

		result, _ := engine.stack.Pop()
		// Should be false because message hashes don't match
		if isTrue(result) {
			t.Fatal("OP_CHECKSIG should return false for mismatched message hash")
		}
	})

	t.Run("RealTransactionSignature", func(t *testing.T) {
		// Generate a test key pair
		privateKey, err := btcec.NewPrivateKey()
		if err != nil {
			t.Fatalf("Failed to generate private key: %v", err)
		}

		publicKey := privateKey.PubKey()

		// Create transaction context
		txContext := &TransactionContext{
			InputIndex:   0,
			PrevOutValue: 100000,
		}

		pubkeyHash := scriptHash160(publicKey.SerializeCompressed())
		scriptPubKey := []byte{OP_DUP, OP_HASH160, 0x14}
		scriptPubKey = append(scriptPubKey, pubkeyHash...)
		scriptPubKey = append(scriptPubKey, OP_EQUALVERIFY, OP_CHECKSIG)

		tx := buildTestTx(nil, scriptPubKey)
		txContext.Tx = tx
		txContext.ScriptCode = scriptPubKey

		engine := NewScriptEngineWithContext(txContext)

		// Calculate what the signature hash should be
		hashType := byte(0x01) // SIGHASH_ALL
		sigHash, err := engine.calculateSignatureHash(hashType)
		if err != nil {
			t.Fatalf("calculateSignatureHash failed: %v", err)
		}

		// Sign the correct message hash
		signature := ecdsa.Sign(privateKey, sigHash)
		derSig := signature.Serialize()

		engine.stack.Push(append(derSig, hashType))
		engine.stack.Push(publicKey.SerializeCompressed())

		err = engine.opCheckSig()
		if err != nil {
			t.Fatalf("OP_CHECKSIG failed: %v", err)
		}

		result, _ := engine.stack.Pop()
		// Should be true because we signed the correct message hash
		if !isTrue(result) {
			t.Fatal("OP_CHECKSIG should return true for correct signature")
		}
	})
}

func TestRealMultiSig(t *testing.T) {
	t.Run("TwoOfThreeMultiSig", func(t *testing.T) {
		// Generate three key pairs
		privateKeys := make([]*btcec.PrivateKey, 3)
		publicKeys := make([]*btcec.PublicKey, 3)
		for i := 0; i < 3; i++ {
			priv, err := btcec.NewPrivateKey()
			if err != nil {
				t.Fatalf("Failed to generate private key %d: %v", i, err)
			}
			privateKeys[i] = priv
			publicKeys[i] = priv.PubKey()
		}

		// Create transaction context
		txContext := &TransactionContext{
			InputIndex:   0,
			PrevOutValue: 500000,
		}

		redeemScript := []byte{0x52, 0x21}
		redeemScript = append(redeemScript, publicKeys[0].SerializeCompressed()...)
		redeemScript = append(redeemScript, 0x21)
		redeemScript = append(redeemScript, publicKeys[1].SerializeCompressed()...)
		redeemScript = append(redeemScript, 0x21)
		redeemScript = append(redeemScript, publicKeys[2].SerializeCompressed()...)
		redeemScript = append(redeemScript, 0x53, 0xae)

		scriptHash := scriptHash160(redeemScript)
		scriptPubKey := []byte{OP_HASH160, 0x14}
		scriptPubKey = append(scriptPubKey, scriptHash...)
		scriptPubKey = append(scriptPubKey, OP_EQUAL)

		tx := buildTestTx(nil, scriptPubKey)
		txContext.Tx = tx
		txContext.ScriptCode = redeemScript

		engine := NewScriptEngineWithContext(txContext)

		// Calculate signature hash
		hashType := byte(0x01) // SIGHASH_ALL
		sigHash, err := engine.calculateSignatureHash(hashType)
		if err != nil {
			t.Fatalf("calculateSignatureHash failed: %v", err)
		}

		// Create signatures from first two private keys
		sig1 := ecdsa.Sign(privateKeys[0], sigHash)
		sig2 := ecdsa.Sign(privateKeys[1], sigHash)

		// Set up stack for 2-of-3 multisig: 0 sig1 sig2 2 pubkey1 pubkey2 pubkey3 3 OP_CHECKMULTISIG
		engine.stack.Push([]byte{})                            // dummy value for CHECKMULTISIG bug
		engine.stack.Push(append(sig1.Serialize(), hashType))  // signature 1
		engine.stack.Push(append(sig2.Serialize(), hashType))  // signature 2
		engine.stack.Push([]byte{2})                           // number of required signatures
		engine.stack.Push(publicKeys[0].SerializeCompressed()) // pubkey 1
		engine.stack.Push(publicKeys[1].SerializeCompressed()) // pubkey 2
		engine.stack.Push(publicKeys[2].SerializeCompressed()) // pubkey 3
		engine.stack.Push([]byte{3})                           // number of public keys

		err = engine.opCheckMultiSig()
		if err != nil {
			t.Fatalf("OP_CHECKMULTISIG failed: %v", err)
		}

		result, _ := engine.stack.Pop()
		// Should be true because we provided valid signatures
		if !isTrue(result) {
			t.Fatal("OP_CHECKMULTISIG should return true for valid 2-of-3 signatures")
		}
	})
}

func TestScriptWithRealCrypto(t *testing.T) {
	t.Run("P2PKH_Script", func(t *testing.T) {
		// Generate a key pair
		privateKey, err := btcec.NewPrivateKey()
		if err != nil {
			t.Fatalf("Failed to generate private key: %v", err)
		}

		publicKey := privateKey.PubKey()
		pubkeyCompressed := publicKey.SerializeCompressed()

		// Calculate public key hash
		pubkeyHash := scriptHash160(pubkeyCompressed)

		// Create transaction context
		txContext := &TransactionContext{
			InputIndex:   0,
			PrevOutValue: 250000,
		}

		// Calculate signature hash
		hashType := byte(0x01) // SIGHASH_ALL
		scriptPubKey := []byte{OP_DUP, OP_HASH160, 0x14}
		scriptPubKey = append(scriptPubKey, pubkeyHash...)
		scriptPubKey = append(scriptPubKey, OP_EQUALVERIFY, OP_CHECKSIG)

		tx := buildTestTx(nil, scriptPubKey)
		txContext.Tx = tx
		txContext.ScriptCode = scriptPubKey

		engine := NewScriptEngineWithContext(txContext)
		sigHash, err := engine.calculateSignatureHash(hashType)
		if err != nil {
			t.Fatalf("calculateSignatureHash failed: %v", err)
		}

		// Sign the message
		signature := ecdsa.Sign(privateKey, sigHash)
		derSig := signature.Serialize()

		// Create P2PKH scriptSig: <sig> <pubkey>
		scriptSig := []byte{
			byte(len(derSig) + 1), // signature length + hashtype
		}
		scriptSig = append(scriptSig, derSig...)
		scriptSig = append(scriptSig, hashType)
		scriptSig = append(scriptSig, byte(len(pubkeyCompressed))) // pubkey length
		scriptSig = append(scriptSig, pubkeyCompressed...)

		tx.Inputs[0].ScriptSig = scriptSig

		// Verify the complete script
		err = engine.VerifyScript(scriptSig, scriptPubKey)
		if err != nil {
			t.Fatalf("P2PKH script verification failed: %v", err)
		}
	})
}
