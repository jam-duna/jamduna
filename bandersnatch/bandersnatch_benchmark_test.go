package bandersnatch

import (
	"crypto/rand"
	"testing"
)

// 10 bytes
// 100 bytes
// 1MB
func BenchmarkRingVRFSign_10B(b *testing.B)  { benchmarkSingleSignRingVRF(b, 10, 10) }
func BenchmarkRingVRFSign_100B(b *testing.B) { benchmarkSingleSignRingVRF(b, 100, 100) }
func BenchmarkRingVRFSign_1MB(b *testing.B)  { benchmarkSingleSignRingVRF(b, 1024, 1024) }
func BenchmarkIETFVRFSign_10B(b *testing.B)  { benchmarkSingleSignIETFVRF(b, 10, 10) }
func BenchmarkIETFVRFSign_100B(b *testing.B) { benchmarkSingleSignIETFVRF(b, 100, 100) }
func BenchmarkIETFVRFSign_1MB(b *testing.B)  { benchmarkSingleSignIETFVRF(b, 1024, 1024) }

func benchmarkSingleSignRingVRF(b *testing.B, aux_data_len int, vrfinput_len int) {
	ringSize := 6
	seeds := make([][]byte, ringSize)
	pubKeys := make([]BanderSnatchKey, ringSize)
	privateKeys := make([]BanderSnatchSecret, ringSize)
	for i := 0; i < ringSize; i++ {
		seeds[i] = generateRandomSeed()
		banderSnatch_pub, banderSnatch_priv, err := InitBanderSnatchKey(seeds[i])
		if err != nil {
			b.Fatalf("InitBanderSnatchKey failed: %v", err)
		}
		pubKeys[i] = banderSnatch_pub
		privateKeys[i] = banderSnatch_priv
	}

	// Create a ring set by concatenating all public keys
	var ringSet []byte
	for _, pubKey := range pubKeys {
		ringSet = append(ringSet, pubKey.Bytes()...)
	}
	// ramdom data aux_data
	aux_data := make([]byte, aux_data_len)
	//use rand read to generate random data
	_, err := rand.Read(aux_data)
	if err != nil {
		b.Fatalf("rand.Read failed: %v", err)
	}
	// vrfinput
	vrfinput := make([]byte, vrfinput_len)
	//use rand read to generate random data
	_, err = rand.Read(vrfinput)
	if err != nil {
		b.Fatalf("rand.Read failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := RingVrfSign(ringSize, privateKeys[0], ringSet, vrfinput, aux_data)
		if err != nil {
			b.Fatalf("RingVrfSign failed: %v", err)
		}
	}
}
func benchmarkSingleSignIETFVRF(b *testing.B, aux_data_len int, vrfinput_len int) {
	seed := generateRandomSeed()
	privateKey, err := getBanderSnatchPrivateKey(seed)
	if err != nil {
		b.Fatalf("getBanderSnatchPrivateKey failed: %v", err)
	}
	signer_private_key := privateKey
	// ramdom data aux_data
	aux_data := make([]byte, aux_data_len)
	//use rand read to generate random data
	_, err = rand.Read(aux_data)
	if err != nil {
		b.Fatalf("rand.Read failed: %v", err)
	}
	// vrfinput
	vrfinput := make([]byte, vrfinput_len)
	//use rand read to generate random data
	_, err = rand.Read(vrfinput)
	if err != nil {
		b.Fatalf("rand.Read failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err = IetfVrfSign(signer_private_key, vrfinput, aux_data)
		if err != nil {
			b.Fatalf("IetfVrfSign failed: %v", err)
		}
	}
}
func BenchmarkGetRingCommitment(b *testing.B) {
	ringSize := 6
	seeds := make([][]byte, ringSize)
	pubKeys := make([]BanderSnatchKey, ringSize)
	privateKeys := make([]BanderSnatchSecret, ringSize)
	for i := 0; i < ringSize; i++ {
		seeds[i] = generateRandomSeed()
		pubKey, err := getBanderSnatchPublicKey(seeds[i])
		if err != nil {
			b.Fatalf("getBanderSnatchPublicKey failed: %v", err)
		}
		privateKey, err := getBanderSnatchPrivateKey(seeds[i])
		if err != nil {
			b.Fatalf("getBanderSnatchPrivateKey failed: %v", err)
		}
		pubKeys[i] = pubKey
		privateKeys[i] = privateKey
	}
	ringset := make([]byte, 0)
	for _, pubKey := range pubKeys {
		ringset = append(ringset, pubKey.Bytes()...)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetRingCommitment(ringSize, ringset)
		if err != nil {
			b.Fatalf("GetRingCommitment failed: %v", err)
		}
	}
}

func getRingSignature(ringSize int) (signature, aux_data, vrfinput, ringset []byte, pubkey BanderSnatchKey, err error) {
	seeds := make([][]byte, ringSize)
	pubKeys := make([]BanderSnatchKey, ringSize)
	privateKeys := make([]BanderSnatchSecret, ringSize)
	for i := 0; i < ringSize; i++ {
		seeds[i] = generateRandomSeed()
		banderSnatch_pub, banderSnatch_priv, err := InitBanderSnatchKey(seeds[i])
		if err != nil {
			return nil, nil, nil, nil, pubkey, err
		}
		pubKeys[i] = banderSnatch_pub
		privateKeys[i] = banderSnatch_priv
	}

	// Create a ring set by concatenating all public keys
	var ringSet []byte
	for _, pubKey := range pubKeys {
		ringSet = append(ringSet, pubKey.Bytes()...)
	}
	signer_private_key := privateKeys[0]
	signer_public_key := pubKeys[0]
	// ramdom data aux_data
	aux_data = make([]byte, 1024)
	//use rand read to generate random data
	_, err = rand.Read(aux_data)
	if err != nil {
		return nil, nil, nil, nil, pubkey, err
	}
	// vrfinput
	vrfinput = make([]byte, 1024)
	//use rand read to generate random data
	_, err = rand.Read(vrfinput)
	if err != nil {
		return nil, nil, nil, nil, pubkey, err
	}

	signature, _, err = RingVrfSign(ringSize, signer_private_key, ringSet, vrfinput, aux_data)
	if err != nil {
		return nil, nil, nil, nil, pubkey, err
	}
	return signature, aux_data, vrfinput, ringSet, signer_public_key, nil
}

func BenchmarkVRFSignedOutput_ring(b *testing.B) {
	ringSize := 6
	signature, _, _, _, _, err := getRingSignature(ringSize)
	if err != nil {
		b.Fatalf("getRingSignature failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = VRFSignedOutput(signature)
	}
}

func BenchmarkRingVRFVerify(b *testing.B) {
	ringSize := 6
	signature, aux_data, vrfinput, ringset, _, err := getRingSignature(ringSize)
	if err != nil {
		b.Fatalf("getRingSignature failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := RingVrfVerify(ringSize, ringset, signature, vrfinput, aux_data)
		if err != nil {
			b.Fatalf("Verify failed: %v", err)
		}
	}
}

func getIETFSignature() (signature, aux_data, vrfinput []byte, pubkey BanderSnatchKey, err error) {
	seed := generateRandomSeed()
	pubKey, err := getBanderSnatchPublicKey(seed)
	if err != nil {
		return nil, nil, nil, pubkey, err
	}
	privateKey, err := getBanderSnatchPrivateKey(seed)
	if err != nil {
		return nil, nil, nil, pubkey, err
	}
	signer_private_key := privateKey
	signer_public_key := pubKey
	// ramdom data aux_data
	aux_data = make([]byte, 1024)
	//use rand read to generate random data
	_, err = rand.Read(aux_data)
	if err != nil {
		return nil, nil, nil, pubkey, err
	}
	// vrfinput
	vrfinput = make([]byte, 1024)
	//use rand read to generate random data
	_, err = rand.Read(vrfinput)
	if err != nil {
		return nil, nil, nil, pubkey, err
	}

	signature, _, err = IetfVrfSign(signer_private_key, vrfinput, aux_data)
	if err != nil {
		return nil, nil, nil, pubkey, err
	}
	return signature, aux_data, vrfinput, signer_public_key, nil
}

func BenchmarkVRFSignedOutput_ietf(b *testing.B) {
	signature, _, _, _, err := getIETFSignature()
	if err != nil {
		b.Fatalf("getIETFSignature failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = VRFSignedOutput(signature)
	}
}

func BenchmarkIETFVRFVerify(b *testing.B) {
	signature, aux_data, vrfinput, pub, err := getIETFSignature()
	if err != nil {
		b.Fatalf("getIETFSignature failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := IetfVrfVerify(pub, signature, vrfinput, aux_data)
		if err != nil {
			b.Fatalf("Verify failed: %v", err)
		}
	}
}
