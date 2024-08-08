package bandersnatch

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"
)

func generateRandomSeed() []byte {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		panic("Failed to generate random seed")
	}
	// Ensure little-endian order
	for i := 0; i < len(seed); i += 8 {
		binary.LittleEndian.PutUint64(seed[i:i+8], binary.LittleEndian.Uint64(seed[i:i+8]))
	}
	return seed
}

func TestVRFOperations(t *testing.T) {
	// Generate 6 different random seeds
	fmt.Println("TestVRFOperations: Generating 6 random seeds")
	seeds := make([][]byte, 6)
	pubKeys := make([][]byte, 6)
	privateKeys := make([][]byte, 6)
	for i := 0; i < 6; i++ {
		seeds[i] = generateRandomSeed()
		fmt.Printf("TestVRFOperations seed %d: %s\n", i, hex.EncodeToString(seeds[i]))
		/*
			pubKey, err := GetPublicKey(seeds[i])
			if err != nil {
				t.Fatalf("GetPublicKey failed: %v", err)
			}
			privateKey, err := GetPrivateKey(seeds[i])
			if err != nil {
				t.Fatalf("GetPrivateKey failed: %v", err)
			}
		*/
		banderSnatch_pub, banderSnatch_priv, err := InitBanderSnatchKey(seeds[i])
		if err != nil {
			t.Fatalf("InitBanderSnatchKey failed: %v", err)
		}
		pubKeys[i] = banderSnatch_pub
		privateKeys[i] = banderSnatch_priv
		fmt.Printf("TestVRFOperations Public Key %d: %s\n", i, hex.EncodeToString(banderSnatch_pub))
		fmt.Printf("TestVRFOperations Private Key %d: %s\n", i, hex.EncodeToString(banderSnatch_priv))
	}

	// Create a ring set by concatenating all public keys
	var ringSet []byte
	for _, pubKey := range pubKeys {
		ringSet = append(ringSet, pubKey...)
	}

	// Example data to be signed
	vrfInputData := []byte("example input data")
	auxData := []byte("example aux data")

	// Use the third private key to sign the data with IETF VRF
	proverIdx := 2
	signature, ietfvrfOutput, err := IetfVrfSign(privateKeys[proverIdx], vrfInputData, auxData)
	if err != nil {
		t.Fatalf("IetfVrfSign: %v", err)
	}
	fmt.Printf("TestVRFOperations IetfVrfSign -- VRFOutput: %x Signature: %x (%d bytes)\n", ietfvrfOutput, signature, len(signature))

	ietfRecoveredVRFOutput, err := VRFSignedOutput(signature)
	if err != nil {
		t.Fatalf("VRFSignedOutput (IETF): %v", err)
	}
	fmt.Printf("TestVRFOperations Ietf Recovered VRFOutput: %x\n", ietfRecoveredVRFOutput)

	// Verify the IETF VRF signature
	vrfOutput, err := IetfVrfVerify(pubKeys[proverIdx], signature, vrfInputData, auxData)
	if err != nil {
		t.Fatalf("IetfVrfVerify failed: %v", err)
	}
	fmt.Printf("TestVRFOperations IETF VRF Output: %x (%d bytes) [VERIFIED signature]\n", vrfOutput, len(vrfOutput))

	// Check that ietfRecoveredVRFOutput, ietfvrfOutput, and vrfOutput are identical
	if !bytes.Equal(ietfRecoveredVRFOutput, ietfvrfOutput) {
		t.Fatalf("ietfRecoveredVRFOutput and ietfvrfOutput are not identical")
	}
	if !bytes.Equal(ietfRecoveredVRFOutput, vrfOutput) {
		t.Fatalf("ietfRecoveredVRFOutput and vrfOutput are not identical")
	}
	if !bytes.Equal(ietfvrfOutput, vrfOutput) {
		t.Fatalf("ietfvrfOutput and vrfOutput are not identical")
	}
	fmt.Printf("IETF VRF Outputs MATCH!\n")

	// Sign the data with Ring VRF using the third private key
	ringSignature, ringvrfOutput, err := RingVrfSign(privateKeys[proverIdx], ringSet, vrfInputData, auxData)
	if err != nil {
		t.Fatalf("RingVrfSign failed: %v", err)
	}
	fmt.Printf("TestVRFOperations RingVrfSign -- VRFOutput: %x Signature: %x (%d bytes)\n", ringvrfOutput, ringSignature, len(ringSignature))

	ringRecoveredVRFOutput, err := VRFSignedOutput(ringSignature)
	if err != nil {
		t.Fatalf("VRFSignedOutput (Ring): %v", err)
	}
	fmt.Printf("TestVRFOperations Ring Recovered VRFOutput: %x\n", ringRecoveredVRFOutput)

	// Verify the Ring VRF signature
	ringVrfOutput, err := RingVrfVerify(ringSet, ringSignature, vrfInputData, auxData)
	if err != nil {
		t.Fatalf("RingVrfVerify failed: %v", err)
	}
	fmt.Printf("TestVRFOperations Ring VRF Output: %x\n", ringVrfOutput)

	// Check that ringvrfOutput, ringRecoveredVRFOutput, and ringVrfOutput are identical
	if !bytes.Equal(ringRecoveredVRFOutput, ringvrfOutput) {
		t.Fatalf("ringRecoveredVRFOutput and ringvrfOutput are not identical")
	}
	if !bytes.Equal(ringRecoveredVRFOutput, ringVrfOutput) {
		t.Fatalf("ringRecoveredVRFOutput and ringVrfOutput are not identical")
	}
	if !bytes.Equal(ringvrfOutput, ringVrfOutput) {
		t.Fatalf("ringvrfOutput and ringVrfOutput are not identical")
	}
	fmt.Printf("Ring VRF Outputs MATCH!\n")

	// FINAL CHECK
	if !bytes.Equal(ietfvrfOutput, ringVrfOutput) {
		t.Fatalf("ietfvrfOutput and ringVrfOutput are not identical")
	}
	fmt.Printf("IETF + Ring VRF Outputs MATCH!\n")
}
func introduceError(data []byte) []byte {
	errorData := make([]byte, len(data))
	copy(errorData, data)
	if len(errorData) > 0 {
		errorData[0] ^= 0xFF // Flip the first byte to introduce an error
	}
	return errorData
}

func introduceLengthError(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	if getRandomInt(2) == 0 {
		// Remove a random byte
		index := getRandomInt(len(data))
		return append(data[:index], data[index+1:]...)
	} else {
		// Add a random byte
		index := getRandomInt(len(data))
		return append(data[:index], append([]byte{0x00}, data[index:]...)...)
	}
}

func getRandomInt(max int) int {
	randomBytes := make([]byte, 1)
	if _, err := rand.Read(randomBytes); err != nil {
		panic("Failed to generate random byte")
	}
	return int(randomBytes[0]) % max
}

func TestVRFOperationsSimulation(t *testing.T) {
	// Generate 6 different random seeds
	fmt.Println("TestVRFOperations: Generating 6 random seeds")
	seeds := make([][]byte, 6)
	pubKeys := make([][]byte, 6)
	privateKeys := make([][]byte, 6)
	for i := 0; i < 6; i++ {
		seeds[i] = generateRandomSeed()
		fmt.Printf("TestVRFOperations seed %d: %s\n", i, hex.EncodeToString(seeds[i]))
		banderSnatch_pub, banderSnatch_priv, err := InitBanderSnatchKey(seeds[i])
		if err != nil {
			t.Fatalf("InitBanderSnatchKey failed: %v", err)
		}
		pubKeys[i] = banderSnatch_pub
		privateKeys[i] = banderSnatch_priv
		fmt.Printf("TestVRFOperations Public Key %d: %s\n", i, hex.EncodeToString(banderSnatch_pub))
		fmt.Printf("TestVRFOperations Private Key %d: %s\n", i, hex.EncodeToString(banderSnatch_priv))
	}

	// Create a ring set by concatenating all public keys
	var ringSet []byte
	for _, pubKey := range pubKeys {
		ringSet = append(ringSet, pubKey...)
	}

	// Example data to be signed
	vrfInputData := []byte("example input data")
	auxData := []byte("example aux data")

	// Perform N iterations
	iterationCnt := 10
	for i := 0; i < iterationCnt; i++ {
		fmt.Printf("Iteration %d:\n", i+1)

		// Use the third seed to sign the data with IETF VRF
		proverIdx := 5
		signature, ietfvrfOutput, err := IetfVrfSign(privateKeys[proverIdx], vrfInputData, auxData)
		if err != nil {
			t.Fatalf("IetfVrfSign: %v", err)
		}
		fmt.Printf("IetfVrfSign -- VRFOutput: %x Signature: %x (%d bytes)\n", ietfvrfOutput, signature, len(signature))

		// Randomly introduce errors for the last 90 iterations
		if i >= 10 {
			switch getRandomInt(5) {
			case 0:
				// No error
				fmt.Println("No error introduced.")
			case 1:
				// Signature error
				signature = introduceError(signature)
				fmt.Println("Introduced error in signature.")
			case 2:
				// PubKey error
				pubKeys[proverIdx] = introduceError(pubKeys[proverIdx])
				fmt.Println("Introduced error in pubKey.")
			case 3:
				// PrivateKey error
				privateKeys[proverIdx] = introduceError(privateKeys[proverIdx])
				fmt.Println("Introduced error in privateKey.")
			case 4:
				// RingSet error
				ringSet = introduceError(ringSet)
				fmt.Println("Introduced error in ringSet.")
			}

			// Randomly introduce length errors
			switch getRandomInt(5) {
			case 0:
				// No length error
				fmt.Println("No length error introduced.")
			case 1:
				// Signature length error
				signature = introduceLengthError(signature)
				fmt.Println("Introduced length error in signature.")
			case 2:
				// PubKey length error
				pubKeys[proverIdx] = introduceLengthError(pubKeys[proverIdx])
				fmt.Println("Introduced length error in pubKey.")
			case 3:
				// PrivateKey length error
				privateKeys[proverIdx] = introduceLengthError(privateKeys[proverIdx])
				fmt.Println("Introduced length error in privateKey.")
			case 4:
				// RingSet length error
				ringSet = introduceLengthError(ringSet)
				fmt.Println("Introduced length error in ringSet.")
			}
		}

		ietfRecoveredVRFOutput, err := VRFSignedOutput(signature)
		if err != nil {
			fmt.Printf("VRFSignedOutput (IETF) failed: %v\n", err)
			continue
		}
		fmt.Printf("Ietf Recovered VRFOutput: %x\n", ietfRecoveredVRFOutput)

		// Verify the IETF VRF signature
		vrfOutput, err := IetfVrfVerify(pubKeys[proverIdx], signature, vrfInputData, auxData)
		if err != nil {
			fmt.Printf("IetfVrfVerify failed: %v\n", err)
			continue
		}
		fmt.Printf("IETF VRF Output: %x (%d bytes) [VERIFIED signature]\n", vrfOutput, len(vrfOutput))

		// Check that ietfRecoveredVRFOutput, ietfvrfOutput, and vrfOutput are identical
		if !bytes.Equal(ietfRecoveredVRFOutput, ietfvrfOutput) {
			fmt.Println("ietfRecoveredVRFOutput and ietfvrfOutput are not identical")
			continue
		}
		if !bytes.Equal(ietfRecoveredVRFOutput, vrfOutput) {
			fmt.Println("ietfRecoveredVRFOutput and vrfOutput are not identical")
			continue
		}
		if !bytes.Equal(ietfvrfOutput, vrfOutput) {
			fmt.Println("ietfvrfOutput and vrfOutput are not identical")
			continue
		}
		fmt.Println("IETF VRF Outputs MATCH!")

		// Sign the data with Ring VRF
		ringSignature, ringvrfOutput, err := RingVrfSign(privateKeys[proverIdx], ringSet, vrfInputData, auxData)
		if err != nil {
			fmt.Printf("RingVrfSign failed: %v\n", err)
			continue
		}
		fmt.Printf("RingVrfSign -- VRFOutput: %x Signature: %x (%d bytes)\n", ringvrfOutput, ringSignature, len(ringSignature))

		ringRecoveredVRFOutput, err := VRFSignedOutput(ringSignature)
		if err != nil {
			fmt.Printf("VRFSignedOutput (Ring) failed: %v\n", err)
			continue
		}
		fmt.Printf("Ring Recovered VRFOutput: %x\n", ringRecoveredVRFOutput)

		// Verify the Ring VRF signature
		ringVrfOutput, err := RingVrfVerify(ringSet, ringSignature, vrfInputData, auxData)
		if err != nil {
			fmt.Printf("RingVrfVerify failed: %v\n", err)
			continue
		}
		fmt.Printf("Ring VRF Output: %x\n", ringVrfOutput)

		// Check that ringvrfOutput, ringRecoveredVRFOutput, and ringVrfOutput are identical
		if !bytes.Equal(ringRecoveredVRFOutput, ringvrfOutput) {
			fmt.Println("ringRecoveredVRFOutput and ringvrfOutput are not identical")
			continue
		}
		if !bytes.Equal(ringRecoveredVRFOutput, ringVrfOutput) {
			fmt.Println("ringRecoveredVRFOutput and ringVrfOutput are not identical")
			continue
		}
		if !bytes.Equal(ringvrfOutput, ringVrfOutput) {
			fmt.Println("ringvrfOutput and ringVrfOutput are not identical")
			continue
		}
		fmt.Println("Ring VRF Outputs MATCH!")

		// FINAL CHECK
		if !bytes.Equal(ietfvrfOutput, ringVrfOutput) {
			fmt.Println("ietfvrfOutput and ringVrfOutput are not identical")
			continue
		}
		fmt.Println("IETF + Ring VRF Outputs MATCH!")
	}
}

func TestRingCommitment(t *testing.T) {
	// Generate 6 different random seeds
	fmt.Println("TestRingCommitment: Generating 6 random seeds")
	seeds := make([][]byte, 6)
	pubKeys := make([][]byte, 6)
	privateKeys := make([][]byte, 6)
	for i := 0; i < 6; i++ {
		seeds[i] = generateRandomSeed()
		fmt.Printf("TestRingCommitment seed %d: %s\n", i, hex.EncodeToString(seeds[i]))
		pubKey, err := getPublicKey(seeds[i])
		if err != nil {
			t.Fatalf("GetPublicKey failed: %v", err)
		}
		privateKey, err := getPrivateKey(seeds[i])
		if err != nil {
			t.Fatalf("GetPrivateKey failed: %v", err)
		}
		pubKeys[i] = pubKey
		privateKeys[i] = privateKey
		//fmt.Printf("TestRingCommitment Public Key %d: %s\n", i, hex.EncodeToString(pubKey))
		//fmt.Printf("TestRingCommitment Private Key %d: %s\n", i, hex.EncodeToString(privateKey))
	}

	// Create a ring set by concatenating all public keys
	extraByte := []byte{0x11, 0x12, 0x13}
	ringSet := []byte{}
	for _, pubKey := range pubKeys {
		ringSet = append(ringSet, pubKey...)
	}
	invalidRingSet := append(extraByte, ringSet...)
	ringCommitment, err := GetRingCommitment(ringSet)
	if err != nil {
		t.Fatalf("RingCommitment failed: %v", err)
	}
	inValidRingCommitment, err := GetRingCommitment(invalidRingSet)
	if err == nil {
		t.Fatalf("invalidRingSet(len=%v) should fail RingCommitment %v\n", len(inValidRingCommitment), inValidRingCommitment)
	}
	fmt.Printf("TestRingCommitment Ring Commitment: %x\n", ringCommitment)

}
