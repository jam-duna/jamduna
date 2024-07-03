package ring_vrf

import (
	"testing"
	"fmt"
)

/*
Bandersnatch Bare VRF:
vrf_sign(secret, input, extra) -> VrfSignature
vrf_verify(public, input, extra, signature) -> Unsigned<1>
vrf_output(secret, input) -> OctetString<32>
vrf_signed_output(signature) -> OctetString<32>

Bandersnatch Ring VRF:
ring_vrf_sign(secret, prover, input, extra) -> RingVrfSignature
ring_vrf_verify(verifier, input, extra, signature) -> Unsigned<1>
ring_vrf_signed_output(signature) -> OctetString<32>
*/

func TestRingVRF(t *testing.T) {

	ringsinerIndex := 4

	pksData := `{"pk_secrets":["8762880e25d88e1b0c0efa30531f8a3c8a56cc98a7df0480eaa73eed720c0c94","da4351a216a0cb4723a2c5df4d8fd57818fee3793b487b093d7d158591d6d141","afc98d3cabfe0583e9d79f072efe183cbbd9eef0a41ee3b13cdf6cafa856337d","dbab239e8eb0216c6d789e4c864d3f88c6e3b2e5a53dccbfc5cd7f6a89d4428e","6d7a227ac5ab1001877ae2d8b9d5427798688911ebf7293627eee32a9ac95ddd","d6493ca8a9c80b44d0de1661ec2ba963bc8369bd294f62bab23e580f81c6e143"],"pks":["931f04f075e00a4db4390aac4f4da2dae3147e06c5f115c5759b11a17b8a953d80","a3d261f0820318c14a5d7dcd16fe44550a378a5374cb606dd2ec852040f9f62100","1cd2db1482150c96893527a2e63528b11b6e9509d17ca449ea0571cec4e67c1e80","f251de621f0cff4f089f6f7a0497c739221dd9bd27ec975188a72e935f77816f00","1562bbab53fb84fe3c00cc520775acf608803b0a0abe36cd7ae0aa6bef5ae63300","af3e537693030ba6350295ddb143290a56c8a9f6eacd0a07b68b6ea3f5513e3a80"]}`
	//fmt.Println("Marshaled JSON:", jsonData)
	recovered_secrets, recovered_pks, err := UnmarshalPKS(pksData)
	if err != nil {
		t.Fatalf("Error unmarshaling PKS: %v", err)
	}
	pks := recovered_pks

	secret := recovered_secrets[ringsinerIndex]
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	pubkey, err := RingVRFPublicKey(secret)
	if err != nil {
		t.Fatalf("Failed to generate public key: %v", err)
	}

	domain := []byte("example.com")
	message := []byte("Hello, world!")
	transcript := []byte("Meow")

	fmt.Printf("Private=%x(Len=%v)\nPubkey=%x(len=%v)\n", secret, len(secret), pubkey, len(pubkey))
	fmt.Printf("domain=%x (len=%v)\n", domain, len(domain))
	fmt.Printf("message=%x (len=%v)\n", message, len(message))
	fmt.Printf("transcript=%x (len=%v)\n", transcript, len(transcript))
	signature, err := RingVRFSignPKS(secret, domain, message, transcript, pks)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}
	fmt.Printf("Signature: %x(len=%v)\n", signature, len(signature))

	fmt.Printf("RingVRFVerify(pks=%x, pubkey=%x, msg=%x, transcript=%x, sig=%x)\n\n", pks, pubkey, message, transcript, signature)

	isValid, err := RingVRFVerifyPKS(domain, message, transcript, signature, pks)
	if err != nil {
		t.Fatalf("Failed to verify: %v", err)
	}
	t.Logf("Verified signature %v", isValid)
}

func TestRingVRFPublicKey(t *testing.T) {
	secret, err := GenerateRandomSecret()
	//secret, err := InitByteSliceFromHex("0xdd7bc1dcfa71dff916cc3636ea1c800b4220416259d0cd48d39d92f574786c32")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	pubkey, err := RingVRFPublicKey(secret)
	if err != nil {
		t.Fatalf("Failed to generate public key: %v", err)
	}
	fmt.Printf("Private=%x(Len=%v) Pubkey=%x(len=%v)\n", secret, len(secret), pubkey, len(pubkey))
}

func TestGeneratePKS(t *testing.T) {
	/*
	targetRingSize := 20
	secrets, pks, err := GeneratePKS(targetRingSize)
	if err != nil {
		t.Fatalf("Error generating PKS: %v", err)
	}

	pksData, err := MarshalPKS(secrets, pks)
	if err != nil {
		t.Fatalf("Error marshaling PKS: %v", err)
	}
	*/
	pksData := `{"pk_secrets":["8762880e25d88e1b0c0efa30531f8a3c8a56cc98a7df0480eaa73eed720c0c94","da4351a216a0cb4723a2c5df4d8fd57818fee3793b487b093d7d158591d6d141","afc98d3cabfe0583e9d79f072efe183cbbd9eef0a41ee3b13cdf6cafa856337d","dbab239e8eb0216c6d789e4c864d3f88c6e3b2e5a53dccbfc5cd7f6a89d4428e","6d7a227ac5ab1001877ae2d8b9d5427798688911ebf7293627eee32a9ac95ddd","d6493ca8a9c80b44d0de1661ec2ba963bc8369bd294f62bab23e580f81c6e143"],"pks":["931f04f075e00a4db4390aac4f4da2dae3147e06c5f115c5759b11a17b8a953d80","a3d261f0820318c14a5d7dcd16fe44550a378a5374cb606dd2ec852040f9f62100","1cd2db1482150c96893527a2e63528b11b6e9509d17ca449ea0571cec4e67c1e80","f251de621f0cff4f089f6f7a0497c739221dd9bd27ec975188a72e935f77816f00","1562bbab53fb84fe3c00cc520775acf608803b0a0abe36cd7ae0aa6bef5ae63300","af3e537693030ba6350295ddb143290a56c8a9f6eacd0a07b68b6ea3f5513e3a80"]}`

	//fmt.Println("Marshaled JSON:", jsonData)
	recovered_secrets, recovered_pks, err := UnmarshalPKS(pksData)
	if err != nil {
		t.Fatalf("Error unmarshaling PKS: %v", err)
	}

	// Print out keys as hex
	for i := 0; i < len(recovered_pks); i++ {
		fmt.Printf("Key#%v - Private=%x (Len=%v) Pubkey=%x (Len=%v)\n", i, recovered_secrets[i], len(recovered_secrets[i]), recovered_pks[i], len(recovered_pks[i]))
	}
}
