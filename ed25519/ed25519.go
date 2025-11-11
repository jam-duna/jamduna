package ed25519

import (
	stded25519 "crypto/ed25519"
	"crypto/rand"
	"io"

	consensus "github.com/hdevalence/ed25519consensus"
)

// ---- Constants, compatible with the standard library ----

const (
	SeedSize       = stded25519.SeedSize
	PublicKeySize  = stded25519.PublicKeySize
	PrivateKeySize = stded25519.PrivateKeySize
	SignatureSize  = stded25519.SignatureSize
)

// ---- Type definitions (type alias, maintain full compatibility) ----
// We use aliases (=) instead of new types to ensure:
// - TLS/x509 obtains the same concrete type
// - type assertions like cert.PublicKey.(ed25519.PublicKey) succeed
// - preserves the original stdlib PrivateKey.Public() method behavior

type (
	PublicKey  = stded25519.PublicKey
	PrivateKey = stded25519.PrivateKey
)

// ---- Required API ----

// NewKeyFromSeed directly uses the standard library behavior: seed 32 bytes -> 64-byte PrivateKey
func NewKeyFromSeed(seed []byte) PrivateKey {
	return stded25519.NewKeyFromSeed(seed)
}

// GenerateKey (optional but very useful)
func GenerateKey(r io.Reader) (PublicKey, PrivateKey, error) {
	if r == nil {
		r = rand.Reader
	}
	return stded25519.GenerateKey(r)
}

// Sign: signing uses the standard library (ZIP-215 issues are mainly about verification rules, not signing).
func Sign(privateKey PrivateKey, message []byte) []byte {
	return stded25519.Sign(stded25519.PrivateKey(privateKey), message)
}

// Verify: use ed25519consensus, applying ZIP-215 verification rules.
func Verify(publicKey PublicKey, message, sig []byte) bool {
	return consensus.Verify(stded25519.PublicKey(publicKey), message, sig)
}
