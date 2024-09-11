package common

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
)

// EthSign signs the given authToken using the provided private key in hex format.
// It returns the message hash, the 65-byte signature, and any error encountered.
func EthSign(privateKeyHex string, authToken []byte) ([]byte, []byte, error) {
	// Convert the private key hex string to ECDSA private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, nil, fmt.Errorf("error converting private key: %v", err)
	}

	// Ethereum message prefix
	message := append([]byte("\x19Ethereum Signed Message:\n32"), authToken...)

	messageHash := Keccak256(message).Bytes()

	// Sign the hash
	signature, err := crypto.Sign(messageHash, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("error signing the hash: %v", err)
	}

	return messageHash, signature, nil
}

// VerifyEthSignature verifies the Ethereum signature against the provided public key and message hash.
// Returns an error if the signature is invalid or does not match.
func VerifyEthSignature(publicKey *ecdsa.PublicKey, messageHash, signature []byte) error {
	// Recover the public key from the signature
	recoveredPubKey, err := crypto.SigToPub(messageHash, signature)
	if err != nil {
		return errors.New("error recovering public key from signature")
	}

	// Compare the recovered public key with the provided public key
	sigPublicKeyBytes := crypto.FromECDSAPub(recoveredPubKey)
	publicKeyBytes := crypto.FromECDSAPub(publicKey)

	if !bytes.Equal(sigPublicKeyBytes, publicKeyBytes) {
		return errors.New("public key does not match")
	}

	// Verify using crypto.VerifySignature
	signatureNoRecoverID := signature[:len(signature)-1] // remove recovery id
	valid := crypto.VerifySignature(publicKeyBytes, messageHash, signatureNoRecoverID)
	if !valid {
		return errors.New("signature verification failed")
	}

	return nil
}
