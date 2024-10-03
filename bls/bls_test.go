package bls

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	// use random seed
	seed := []byte("random seed")
	secret, err := GetSecretKey(seed)
	if err != nil {
		t.Errorf("failed to generate secret key: %v", err)
	}
	if len(secret) != SecretKeyLen {
		t.Errorf("invalid secret key length: expected %d bytes, got %d", SecretKeyLen, len(secret))
	}
	pub, err := GetDoublePublicKey(seed)
	if err != nil {
		t.Errorf("failed to generate double public key: %v", err)
	}
	if len(pub) != DoubleKeyLen {
		t.Errorf("invalid double public key length: expected %d bytes, got %d", DoubleKeyLen, len(pub))
	}
	pubG2, err := GetPublicKey_G2(seed)
	if err != nil {
		t.Errorf("failed to generate G2 public key: %v", err)
	}
	if len(pubG2) != G2Len {
		t.Errorf("invalid G2 public key length: expected %d bytes, got %d", G2Len, len(pubG2))
	}
	fmt.Printf("Success: Secret Key=%x,\n Double Public Key=%x,\n G2 Public Key=%x\n", secret, pub, pubG2)
}

func TestSignAndVerify(t *testing.T) {
	// use random seed
	seed := []byte("handsome carlos")
	secret, err := GetSecretKey(seed)
	if err != nil {
		t.Errorf("failed to generate secret key: %v", err)
	}
	if len(secret) != SecretKeyLen {
		t.Errorf("invalid secret key length: expected %d bytes, got %d", SecretKeyLen, len(secret))
	}
	pub, err := GetDoublePublicKey(seed)
	if err != nil {
		t.Errorf("failed to generate double public key: %v", err)
	}
	if len(pub) != DoubleKeyLen {
		t.Errorf("invalid double public key length: expected %d bytes, got %d", DoubleKeyLen, len(pub))
	}
	pubG2, err := GetPublicKey_G2(seed)
	if err != nil {
		t.Errorf("failed to generate G2 public key: %v", err)
	}
	if len(pubG2) != G2Len {
		t.Errorf("invalid G2 public key length: expected %d bytes, got %d", G2Len, len(pubG2))
	}
	message := []byte("colorful notion")
	sig, err := secret.Sign(message)
	if err != nil {
		t.Errorf("failed to sign message: %v", err)
	}
	if len(sig) != SigLen {
		t.Errorf("invalid signature length: expected %d bytes, got %d", SigLen, len(sig))
	}
	if !pubG2.Verify(message, sig) {
		t.Errorf("failed to verify signature")
	}
	fake_massage := []byte("fake message")
	if pubG2.Verify(fake_massage, sig) {
		t.Errorf("failed to verify signature")
	}
	fmt.Printf("Success: Signature=%x\n", sig)
}

func TestAggregateSignatures(t *testing.T) {

	var secrets []SecretKey
	var pubs []DoublePublicKey
	var pubsG2 []G2PublicKey
	for i := 0; i < 6; i++ {
		seed := make([]byte, SecretKeyLen)
		rand.Read(seed)
		secret, err := GetSecretKey(seed)
		if err != nil {
			t.Errorf("failed to generate secret key %d: %v", i, err)
		}
		if len(secret) != SecretKeyLen {
			t.Errorf("invalid secret key length for key %d: expected %d bytes, got %d", i, SecretKeyLen, len(secret))
		}
		secrets = append(secrets, secret)
		pub, err := GetDoublePublicKey(seed)
		if err != nil {
			t.Errorf("failed to generate double public key %d: %v", i, err)
		}
		if len(pub) != DoubleKeyLen {
			t.Errorf("invalid double public key length for key %d: expected %d bytes, got %d", i, DoubleKeyLen, len(pub))
		}
		pubs = append(pubs, pub)
		pubG2, err := GetPublicKey_G2(seed)
		if err != nil {
			t.Errorf("failed to generate G2 public key %d: %v", i, err)
		}
		if len(pubG2) != G2Len {
			t.Errorf("invalid G2 public key length for key %d: expected %d bytes, got %d", i, G2Len, len(pubG2))
		}
		pubsG2 = append(pubsG2, pubG2)

	}
	message := []byte("colorful notion")
	var sigs []Signature
	for i, secret := range secrets {
		sig, err := secret.Sign(message)
		if err != nil {
			t.Errorf("failed to sign message for key %d: %v", i, err)
		}
		if len(sig) != SigLen {
			t.Errorf("invalid signature length for key %d: expected %d bytes, got %d", i, SigLen, len(sig))
		}
		sigs = append(sigs, sig)
	}
	aggSig, err := AggregateSignatures(sigs, message)
	if err != nil {
		t.Errorf("failed to aggregate signatures: %v", err)
	}
	if len(aggSig) != SigLen {
		t.Errorf("invalid aggregated signature length: expected %d bytes, got %d", SigLen, len(aggSig))
	}
	if !AggregateVerify(pubs, aggSig, message) {
		t.Errorf("failed to verify aggregated signature")
	}
	// use fake message
	fake_massage := []byte("fake message")
	if AggregateVerify(pubs, aggSig, fake_massage) {
		t.Errorf("failed to verify aggregated signature")
	}
	fmt.Printf("Success: Aggregated Signature=%x\n", aggSig)

}
