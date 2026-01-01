// Generate Verkle Tree Test Vectors
//
// This program generates definitive test vectors from go-verkle and go-ipa
// for verifying Rust implementation compatibility.
//
// Usage:
//   go run generate_verkle_vectors.go > verkle_test_vectors.txt
//
// The output can be used to update:
//   - services/evm/tests/verkle_compat_test.rs

package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/crate-crypto/go-ipa/banderwagon"
	"github.com/crate-crypto/go-ipa/ipa"
)

func main() {
	fmt.Println("# Verkle Tree Test Vectors")
	fmt.Println("# Generated from go-verkle v0.2.2 and go-ipa v0.0.0-20240223125850-b1e8a79f509c")
	fmt.Println()

	// Generator point
	generateGeneratorVectors()
	fmt.Println()

	// SRS points
	generateSRSVectors()
	fmt.Println()

	// Hash to bytes
	generateHashToBytes()
	fmt.Println()

	// IPA proof format
	generateIPAProofFormat()
	fmt.Println()

	// Field parameters
	generateFieldParameters()
}

func generateGeneratorVectors() {
	fmt.Println("## Generator Point")
	fmt.Println()

	gen := banderwagon.Generator

	// Get affine coordinates
	affineX := gen.Bytes()
	fmt.Printf("Compressed (32 bytes): %s\n", hex.EncodeToString(affineX[:]))
	fmt.Println()

	// Get uncompressed coordinates for reference
	uncompressed := gen.BytesUncompressedTrusted()
	xBytes := uncompressed[0:32]
	yBytes := uncompressed[32:64]
	fmt.Printf("X (field element):     %s\n", hex.EncodeToString(xBytes))
	fmt.Printf("Y (field element):     %s\n", hex.EncodeToString(yBytes))
	fmt.Println()

	// Test round-trip
	var decompressed banderwagon.Element
	err := decompressed.SetBytes(affineX[:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to decompress generator: %v\n", err)
		os.Exit(1)
	}

	decompressedBytes := decompressed.Bytes()
	if hex.EncodeToString(decompressedBytes[:]) != hex.EncodeToString(affineX[:]) {
		fmt.Fprintf(os.Stderr, "ERROR: Generator round-trip failed\n")
		os.Exit(1)
	}

	fmt.Println("✓ Generator round-trip verified")
	fmt.Println()

	// Rust test constant
	fmt.Println("Rust constant:")
	fmt.Printf("const GENERATOR_COMPRESSED: [u8; 32] = hex!(\"%s\");\n", hex.EncodeToString(affineX[:]))
}

func generateSRSVectors() {
	fmt.Println("## SRS Points")
	fmt.Println()
	fmt.Println("Seed: \"eth_verkle_oct_2021\"")
	fmt.Println("Algorithm: SHA256(seed || counter) → Fq → bytes → decompress (with subgroup check)")
	fmt.Println()

	// Generate first 10 for quick tests
	fmt.Println("### First 10 points (for quick verification)")
	fmt.Println()
	srs10 := ipa.GenerateRandomPoints(10)
	for i, p := range srs10 {
		compressedBytes := p.Bytes()
		fmt.Printf("Point %d: %s\n", i, hex.EncodeToString(compressedBytes[:]))
	}
	fmt.Println()

	// Rust test constants
	fmt.Println("Rust constants:")
	fmt.Println("const SRS_VECTORS: [[u8; 32]; 10] = [")
	for i, p := range srs10 {
		compressedBytes := p.Bytes()
		fmt.Printf("    hex!(\"%s\"), // Point %d\n", hex.EncodeToString(compressedBytes[:]), i)
	}
	fmt.Println("];")
	fmt.Println()

	// Generate full 256 for complete verification
	fmt.Println("### All 256 points (for full SRS)")
	fmt.Println()
	srs256 := ipa.GenerateRandomPoints(256)
	fmt.Printf("Generated %d points\n", len(srs256))

	// Verify first 10 match
	for i := 0; i < 10; i++ {
		bytes256 := srs256[i].Bytes()
		bytes10 := srs10[i].Bytes()
		if hex.EncodeToString(bytes256[:]) != hex.EncodeToString(bytes10[:]) {
			fmt.Fprintf(os.Stderr, "ERROR: SRS point %d mismatch between 10 and 256 generation\n", i)
			os.Exit(1)
		}
	}
	fmt.Println("✓ First 10 points match between partial and full generation")
	fmt.Println()

	// Output all 256 for reference
	fmt.Println("All 256 points:")
	for i, p := range srs256 {
		pBytes := p.Bytes()
		fmt.Printf("%3d: %s\n", i, hex.EncodeToString(pBytes[:]))
	}
	fmt.Println()
}

func generateHashToBytes() {
	fmt.Println("## Hash to Bytes")
	fmt.Println()
	fmt.Println("Algorithm: LE_bytes(X / Y mod q)")
	fmt.Println("Source: go-verkle/ipa.go:HashPointToBytes")
	fmt.Println()

	// Hash generator point
	gen := banderwagon.Generator
	var hashedPoint banderwagon.Fr
	gen.MapToScalarField(&hashedPoint)
	hashBytes := hashedPoint.BytesLE()

	fmt.Printf("hash_to_bytes(generator): %s\n", hex.EncodeToString(hashBytes[:]))
	fmt.Println()

	// Rust test constant
	fmt.Println("Rust constant:")
	fmt.Printf("const GENERATOR_HASH: [u8; 32] = hex!(\"%s\");\n", hex.EncodeToString(hashBytes[:]))
	fmt.Println()

	// Hash first few SRS points for additional verification
	fmt.Println("hash_to_bytes(SRS points):")
	srs := ipa.GenerateRandomPoints(5)
	for i, p := range srs {
		var hashed banderwagon.Fr
		p.MapToScalarField(&hashed)
		hashedBytes := hashed.BytesLE()
		fmt.Printf("SRS[%d]: %s\n", i, hex.EncodeToString(hashedBytes[:]))
	}
	fmt.Println()
}

func generateIPAProofFormat() {
	fmt.Println("## IPA Proof Format")
	fmt.Println()
	fmt.Println("For VectorLength = 256:")
	fmt.Println("  - Rounds: log2(256) = 8")
	fmt.Println("  - L: 8 points × 32 bytes = 256 bytes")
	fmt.Println("  - R: 8 points × 32 bytes = 256 bytes")
	fmt.Println("  - A: 1 scalar × 32 bytes = 32 bytes")
	fmt.Println("  - Total: 544 bytes")
	fmt.Println()

	fmt.Println("Serialization order: L[0], L[1], ..., L[7], R[0], R[1], ..., R[7], A")
	fmt.Println()

	// Create sample proof with generator points
	fmt.Println("Sample proof (all points = generator, scalar = 1):")
	gen := banderwagon.Generator
	sampleProof := make([]byte, 544)

	genBytes := gen.Bytes()

	// L points (8 × 32 = 256 bytes)
	for i := 0; i < 8; i++ {
		copy(sampleProof[i*32:(i+1)*32], genBytes[:])
	}

	// R points (8 × 32 = 256 bytes)
	for i := 0; i < 8; i++ {
		copy(sampleProof[256+i*32:256+(i+1)*32], genBytes[:])
	}

	// A scalar (32 bytes) - use scalar field element "1"
	var one banderwagon.Fr
	one.SetOne()
	oneBytes := one.BytesLE()
	copy(sampleProof[512:544], oneBytes[:])

	fmt.Printf("Hex: %s\n", hex.EncodeToString(sampleProof))
	fmt.Println()
}

func generateFieldParameters() {
	fmt.Println("## Field Parameters")
	fmt.Println()

	// Base field Fq
	fmt.Println("Base field Fq (Bandersnatch curve):")
	fmt.Println("  q = 52435875175126190479447740508185965837690552500527637822603658699938581184513")
	fmt.Println("  hex = 0x73eda753299d7d483339d80809a1d80553bda402fffe5bfeffffffff00000001")
	fmt.Println()

	// Scalar field Fr
	fmt.Println("Scalar field Fr:")
	fmt.Println("  r = 13108968793781547619861935127046491459309155893440570251786403306729687672801")
	fmt.Println("  hex = 0x1cfb69d4ca675f520cce760202687600ff8f87007419047174fd06b52876e7e1")
	fmt.Println()

	// Curve parameters
	fmt.Println("Twisted Edwards curve: a*x² + y² = 1 + d*x²*y²")
	fmt.Println("  a = -5")
	fmt.Println("  d = 138827208126141220649022263972958607803 / 171449701953573178309673572579671231137")
	fmt.Println()

	// Lexicographic threshold
	fmt.Println("Lexicographic threshold (q-1)/2:")
	fmt.Println("  (q-1)/2 = 26217937587563095239723870254092982918845276250263818911301829349969290592256")
	fmt.Println("  hex = 0x39f6d3a994cebea4199cec0404d0ec02a9ded2017fff2dff7fffffff80000000")
	fmt.Println()
	fmt.Println("A field element x is lexicographically largest iff x > (q-1)/2")
	fmt.Println()
}
