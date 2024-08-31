package common

import (
	"crypto/rand"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
)

func TestEthSign(t *testing.T) {
	privateKeyHex := "4c0883a69102937d6231471b5dbb6204fe5129617082790e20c3a52a9e7efed2"
	authToken := make([]byte, 32)
	_, err := rand.Read(authToken)
	assert.NoError(t, err, "Error generating authToken")

	// Call the EthSign function
	messageHash, signature, err := EthSign(privateKeyHex, authToken)
	assert.NoError(t, err, "Error during EthSign")

	// Derive the public key from the private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	assert.NoError(t, err, "Error converting private key")

	// Call the VerifyEthSignature function
	err = VerifyEthSignature(&privateKey.PublicKey, messageHash, signature)
	assert.NoError(t, err, "Signature verification failed")
}
