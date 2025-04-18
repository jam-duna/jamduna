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
	pubKeys := make([]BanderSnatchKey, 6)
	privateKeys := make([]BanderSnatchSecret, 6)
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
		fmt.Printf("TestVRFOperations Public Key %d: %s\n", i, hex.EncodeToString(banderSnatch_pub.Bytes()))
		fmt.Printf("TestVRFOperations Private Key %d: %s\n", i, hex.EncodeToString(banderSnatch_priv.Bytes()))
	}

	// Create a ring set by concatenating all public keys
	var ringSet []byte
	for _, pubKey := range pubKeys {
		ringSet = append(ringSet, pubKey.Bytes()...)
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

func TestVRFOperationsPaddingPoint(t *testing.T) {
	// Generate 6 different random seeds
	fmt.Println("TestVRFOperations: Generating 6 random seeds")
	seeds := make([][]byte, 6)
	pubKeys := make([]BanderSnatchKey, 6)
	privateKeys := make([]BanderSnatchSecret, 6)
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
		fmt.Printf("TestVRFOperations Public Key %d: %s\n", i, hex.EncodeToString(banderSnatch_pub.Bytes()))
		fmt.Printf("TestVRFOperations Private Key %d: %s\n", i, hex.EncodeToString(banderSnatch_priv.Bytes()))
	}

	// Create a ring set by concatenating all public keys
	var ringSet []byte
	proverIdx := 2
	for i, pubKey := range pubKeys {
		if i == proverIdx {
			// Add padding point
			ringSet = append(ringSet, make([]byte, 32)...)
			continue
		}
		ringSet = append(ringSet, pubKey.Bytes()...)
	}

	// Example data to be signed
	vrfInputData := []byte("example input data")
	auxData := []byte("example aux data")

	// Use the third private key to sign the data with IETF VRF

	// Sign the data with Ring VRF using the third private key
	ringSignature, ringvrfOutput, err := RingVrfSign(privateKeys[proverIdx], ringSet, vrfInputData, auxData)
	if err != nil {
		fmt.Printf("err = %v\n", err)
	}
	fmt.Printf("TestVRFOperations RingVrfSign -- VRFOutput: %x Signature: %x (%d bytes)\n", ringvrfOutput, ringSignature, len(ringSignature))

	// ringRecoveredVRFOutput, err := VRFSignedOutput(ringSignature)
	// if err != nil {
	// 	t.Fatalf("VRFSignedOutput (Ring): %v", err)
	// }
	// fmt.Printf("TestVRFOperations Ring Recovered VRFOutput: %x\n", ringRecoveredVRFOutput)

	// // Verify the Ring VRF signature
	// ringVrfOutput, err := RingVrfVerify(ringSet, ringSignature, vrfInputData, auxData)
	// if err != nil {
	// 	t.Fatalf("RingVrfVerify failed: %v", err)
	// }
	// fmt.Printf("TestVRFOperations Ring VRF Output: %x\n", ringVrfOutput)

	// // Check that ringvrfOutput, ringRecoveredVRFOutput, and ringVrfOutput are identical
	// if !bytes.Equal(ringRecoveredVRFOutput, ringvrfOutput) {
	// 	t.Fatalf("ringRecoveredVRFOutput and ringvrfOutput are not identical")
	// }
	// fmt.Printf("Ring VRF Outputs MATCH!\n")
}

func introduceError(data []byte) []byte {
	errorData := make([]byte, len(data))
	copy(errorData, data)
	for i := range errorData {
		errorData[i] ^= 0xFF // Flip all bytes to introduce significant errors
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

	// Perform N iterations
	iterationCnt := 100
	for i := 0; i < iterationCnt; i++ {
		seeds := make([][]byte, 6)
		pubKeys := make([]BanderSnatchKey, 6)
		privateKeys := make([]BanderSnatchSecret, 6)
		for i := 0; i < 6; i++ {
			seeds[i] = generateRandomSeed()
			fmt.Printf("TestVRFOperations seed %d: %s\n", i, hex.EncodeToString(seeds[i]))
			banderSnatch_pub, banderSnatch_priv, err := InitBanderSnatchKey(seeds[i])
			if err != nil {
				t.Fatalf("InitBanderSnatchKey failed: %v", err)
			}
			pubKeys[i] = banderSnatch_pub
			privateKeys[i] = banderSnatch_priv
			fmt.Printf("TestVRFOperations Public Key %d: %s\n", i, banderSnatch_pub.String())
			fmt.Printf("TestVRFOperations Private Key %d: %s\n", i, banderSnatch_priv.String())
		}

		// Create a ring set by concatenating all public keys
		var ringSet []byte
		for _, pubKey := range pubKeys {
			ringSet = append(ringSet, pubKey.Bytes()...)
		}

		// Example data to be signed
		vrfInputData := []byte("example input data")
		auxData := []byte("example aux data")
		// auxData := []byte("")

		fmt.Printf("Iteration %d:\n", i+1)

		// Use the third seed to sign the data with IETF VRF
		proverIdx := 5
		IETFsignature, ietfvrfOutput, err := IetfVrfSign(privateKeys[proverIdx], vrfInputData, auxData)
		if err != nil {
			t.Fatalf("IetfVrfSign: %v", err)
		}
		fmt.Printf("IetfVrfSign -- VRFOutput: %x Signature: %x (%d bytes)\n", ietfvrfOutput, IETFsignature, len(IETFsignature))
		// Sign the data with Ring VRF
		ringSignature, ringvrfOutput, err := RingVrfSign(privateKeys[proverIdx], ringSet, vrfInputData, auxData)
		if err != nil {
			fmt.Printf("RingVrfSign failed: %v\n", err)
			continue
		}
		fmt.Printf("RingVrfSign -- VRFOutput: %x Signature: %x (%d bytes)\n", ringvrfOutput, ringSignature, len(ringSignature))
		// Randomly introduce errors for the last 90 iterations
		isExpectingError := false
		randomCase := 0
		if i >= 10 {
			randomCase = getRandomInt(9)
			// randomCase = 6
			//randomCase = 1
			switch randomCase {
			case 0:
				// No error
				fmt.Println("No error introduced.")
			case 1:
				// Signature error
				isExpectingError = true
				IETFsignature = introduceError(IETFsignature)
				fmt.Println("Introduced error in IETFsignature.")
			case 2:
				// ringSignature error
				isExpectingError = true
				ringSignature = introduceError(ringSignature)
				fmt.Println("Introduced error in ringSignature.")
			case 3:
				// PubKey error
				isExpectingError = true
				errPub := BanderSnatchKey(introduceError(pubKeys[proverIdx].Bytes()))
				pubKeys[proverIdx] = errPub
				fmt.Println("Introduced error in pubKey.")
			case 4:
				// vrfInputData error
				// resign on IETF VRF
				isExpectingError = true
				IETFsignature, ietfvrfOutput, err = IetfVrfSign(privateKeys[proverIdx], introduceError(vrfInputData), auxData)
				if err != nil {
					t.Fatalf("IetfVrfSign failed: %v", err)
				}
				fmt.Printf("Introduce error in vrfInputData for IetfSign\n")
			case 5:
				// vrfInputData error
				// resign on Ring VRF
				isExpectingError = true
				ringSignature, ringvrfOutput, err = RingVrfSign(privateKeys[proverIdx], ringSet, introduceError(vrfInputData), auxData)
				if err != nil {
					t.Fatalf("RingVrfSign failed: %v", err)
				}
				fmt.Printf("Introduce error in vrfInputData for RingSign\n")
			case 6:
				// Aux data error
				isExpectingError = true
				auxData = introduceError(auxData)
				fmt.Println("Introduced error in auxData.")

			case 7:
				// RingSet error
				isExpectingError = true
				ringSet = introduceError(ringSet)
				fmt.Println("Introduced error in ringSet.")
			case 8:
				// compare IETF vrfoutput if we change the aux data
				ramdomAuxData := []byte("random aux data")
				_, vrfIETF, err := IetfVrfSign(privateKeys[proverIdx], vrfInputData, ramdomAuxData)
				if err != nil {
					t.Fatalf("IetfVrfSign failed: %v", err)
				}
				if !bytes.Equal(ietfvrfOutput, vrfIETF) {
					fmt.Printf("IETF vrfoutput is not equal when we change the aux data\n")
				}
				_, vrfRing, err := RingVrfSign(privateKeys[proverIdx], ringSet, vrfInputData, ramdomAuxData)
				if err != nil {
					t.Fatalf("RingVrfSign failed: %v", err)
				}
				if !bytes.Equal(ringvrfOutput, vrfRing) {
					fmt.Printf("Ring vrfoutput is not equal when we change the aux data\n")
				}

			}

			// Randomly introduce length errors ---> irrelevant test
			/*
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
					errPub := BanderSnatchKey(introduceError(pubKeys[proverIdx].Bytes()))
					pubKeys[proverIdx] = errPub
					fmt.Println("Introduced length error in pubKey.")
				case 3:
					// PrivateKey length error
					errPriv := BanderSnatchSecret(introduceError(privateKeys[proverIdx].Bytes()))
					privateKeys[proverIdx] = errPriv
					fmt.Println("Introduced length error in privateKey.")
				case 4:
					// RingSet length error
					ringSet = introduceLengthError(ringSet)
					fmt.Println("Introduced length error in ringSet.")
				}
			*/
		}

		ietfRecoveredVRFOutput, err := VRFSignedOutput(IETFsignature)
		if err == nil && randomCase == 1 {
			t.Fatalf("VRFSignedOutput (IETF) is expecting error on case#%v. But no error returned=%v\n", randomCase, err)
		} else if err != nil && !isExpectingError {
			t.Fatalf("VRFSignedOutput (IETF) failed with unexpected error=%v\n", err)
		}
		fmt.Printf("Ietf Recovered VRFOutput: %x\n", ietfRecoveredVRFOutput)

		// Verify the IETF VRF signature
		IetfVrfOutput, ErrItefVerify := IetfVrfVerify(pubKeys[proverIdx], IETFsignature, vrfInputData, auxData)
		fmt.Printf("IETF VRF Output: %x (%d bytes) [VERIFIED signature]\n", IetfVrfOutput, len(IetfVrfOutput))

		ringRecoveredVRFOutput, err := VRFSignedOutput(ringSignature)
		if err != nil && !isExpectingError {
			t.Fatalf("VRFSignedOutput (Ring): %v", err)
		}
		fmt.Printf("Ring Recovered VRFOutput: %x\n", ringRecoveredVRFOutput)

		// Verify the Ring VRF signature
		ringVrfOutput, ErrRingVerify := RingVrfVerify(ringSet, ringSignature, vrfInputData, auxData)
		fmt.Printf("Ring VRF Output: %x\n", ringVrfOutput)
		var ErrVRFoutput error
		if !bytes.Equal(ietfvrfOutput, ringVrfOutput) {
			ErrVRFoutput = fmt.Errorf("ietfvrfOutput and ringVrfOutput are not identical")
		} else if !bytes.Equal(ietfRecoveredVRFOutput, ietfvrfOutput) {
			ErrVRFoutput = fmt.Errorf("ietfRecoveredVRFOutput and ietfvrfOutput are not identical")
		} else if !bytes.Equal(ietfRecoveredVRFOutput, IetfVrfOutput) {
			ErrVRFoutput = fmt.Errorf("ietfRecoveredVRFOutput and IetfVrfOutput are not identical")
		} else if !bytes.Equal(ietfRecoveredVRFOutput, ringRecoveredVRFOutput) {
			ErrVRFoutput = fmt.Errorf("ietfRecoveredVRFOutput and ringRecoveredVRFOutput are not identical")
		} else {
			ErrVRFoutput = nil
		}
		if isExpectingError {
			if ErrItefVerify == nil && ErrRingVerify == nil && ErrVRFoutput == nil {
				t.Fatalf("Case#%v: Expected error but no error returned\n", randomCase)
			}
		} else {
			if ErrItefVerify != nil || ErrRingVerify != nil || ErrVRFoutput != nil {
				t.Fatalf("Case#%v: Unexpected error returned\n", randomCase)
			}
		}

	}
}

func TestRingCommitment(t *testing.T) {
	// Generate 6 different random seeds
	fmt.Println("TestRingCommitment: Generating 6 random seeds")
	seeds := make([][]byte, 6)
	pubKeys := make([]BanderSnatchKey, 6)
	privateKeys := make([]BanderSnatchSecret, 6)
	for i := 0; i < 6; i++ {
		seeds[i] = generateRandomSeed()
		fmt.Printf("TestRingCommitment seed %d: %s\n", i, hex.EncodeToString(seeds[i]))
		pubKey, err := getBanderSnatchPublicKey(seeds[i])
		if err != nil {
			t.Fatalf("getBanderSnatchPublicKey failed: %v", err)
		}
		privateKey, err := getBanderSnatchPrivateKey(seeds[i])
		if err != nil {
			t.Fatalf("getBanderSnatchPrivateKey failed: %v", err)
		}
		pubKeys[i] = pubKey
		privateKeys[i] = privateKey
	}

	// Create a ring set by concatenating all public keys
	extraByte := []byte{0x11, 0x12, 0x13}
	ringSet := []byte{}
	for _, pubKey := range pubKeys {
		ringSet = append(ringSet, pubKey.Bytes()...)
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

func TestVRFSign(t *testing.T) {
	// Generate 6 different random seeds
	seeds := make([][]byte, 6)
	pubKeys := make([]BanderSnatchKey, 6)
	privateKeys := make([]BanderSnatchSecret, 6)
	for i := 0; i < 6; i++ {
		seeds[i] = generateRandomSeed()
		fmt.Printf("TestVRFOperations seed %d: %s\n", i, hex.EncodeToString(seeds[i]))
		banderSnatch_pub, banderSnatch_priv, err := InitBanderSnatchKey(seeds[i])
		if err != nil {
			t.Fatalf("InitBanderSnatchKey failed: %v", err)
		}
		pubKeys[i] = banderSnatch_pub
		privateKeys[i] = banderSnatch_priv
		fmt.Printf("TestVRFOperations Public Key %d: %s\n", i, banderSnatch_pub.String())
		fmt.Printf("TestVRFOperations Private Key %d: %s\n", i, banderSnatch_priv.String())
	}

	// Create a ring set by concatenating all public keys
	var ringSet []byte
	for _, pubKey := range pubKeys {
		ringSet = append(ringSet, pubKey.Bytes()...)
	}

	// Example data to be signed
	vrfInputData := []byte("example input data")
	auxData := []byte("example aux data")
	// auxData := []byte("")
	// Use the third seed to sign the data with IETF VRF
	proverIdx := 5
	// Sign the data with Ring VRF
	for i := 0; i < 10; i++ {
		// Sign the data with Ring VRF using the third private key
		ringSignature, ringvrfOutput, err := RingVrfSign(privateKeys[proverIdx], ringSet, vrfInputData, auxData)
		if err != nil {
			fmt.Printf("RingVrfSign failed: %v\n", err)
			continue
		}
		fmt.Printf("TestVRFOperations RingVrfSign -- VRFOutput: %x Signature: %x (%d bytes)\n", ringvrfOutput, ringSignature, len(ringSignature))
		fmt.Printf("first 32 bytes of ringSignature: %x\n", ringSignature[:64])
	}
}
