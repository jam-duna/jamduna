package ring_vrf

import (
	"testing"
	"fmt"
)

func TestRingVRF(t *testing.T) {
	pks := []byte("toodaloo")
	secret := []byte{0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3}
	pubkey := []byte{0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3, 0, 1, 2, 3}
	domain := []byte("example.com")
	message := []byte("Hello, world!")
	transcript := []byte("Meow")

	signature, err := RingVRFSign(secret, domain, message, transcript)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}
	fmt.Printf("Signature: %x\n", signature)


	err = RingVRFVerify(pks, pubkey, domain, message, transcript, signature)
	if err != nil {
		t.Fatalf("Failed to verify: %v", err)
	}
	t.Logf("Verified signature")
}
