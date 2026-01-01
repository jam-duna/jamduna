# Verkle Tree Test Vectors

This directory contains definitive test vectors for verifying Rust verkle tree implementation compatibility with go-verkle and go-ipa.

## Files

- **generate_verkle_vectors.go** - Go program that generates test vectors from actual go-ipa execution
- **verkle_test_vectors.txt** - Generated test vectors (reference data)

## Quick Start

### Generate Test Vectors

```bash
cd /Users/michael/Desktop/jam
go run test_vectors/generate_verkle_vectors.go > test_vectors/verkle_test_vectors.txt
```

### Use in Rust Tests

Test vectors are embedded in `services/evm/tests/verkle_compat_test.rs` as constants:

```rust
const GENERATOR_COMPRESSED: [u8; 32] = hex!("4a2c74...");
const SRS_VECTORS: [[u8; 32]; 10] = [ /* ... */ ];
const GENERATOR_HASH: [u8; 32] = hex!("d1e7de...");
```

## Test Vectors Included

### Generator Point

- **Compressed bytes** (32 bytes): `4a2c7486fd924882bf02c6908de395122843e3e05264d7991e18e7985dad51e9`
- **Uncompressed X**: `29c132cc2c0b34c5743711777bbe42f32b79c022ad998465e1e71866a252ae18`
- **Uncompressed Y**: `2a6c669eda123e0f157d8b50badcd586358cad81eee464605e3167b6cc974166`

### SRS Points (First 10 of 256)

Seed: `"eth_verkle_oct_2021"`

```
Point 0: 01587ad1336675eb912550ec2a28eb8923b824b490dd2ba82e48f14590a298a0
Point 1: 6c6e607df0723edfff382fa914bfc38136f3300ab2e06fb97007b559fd323b82
Point 2: 326be3bebfd97ed9d0d4ca1b8bc47e036a24b129f1488110b71c2cae1463db8f
Point 3: 6bd241cc12dc9b2c0ad6fc85e016605c49c1a92939c7faeea0a555d2a1c3ddf8
Point 4: 00d4bb940478cca48a5b822533d2b3215857ae7c6643c5954c96a0084ebffb2c
Point 5: 1c817b76e1c869c4a74f9ce5b8bc04dc810dae7a61ee05616a29eca128e60d3b
Point 6: 03ef64cbed1a63b043942bd0b114537227a116ffadd92a47749460b8facc7af9
Point 7: 1436bda962957699c4d084acd6964db917c46b6c9a42465f9f656c58def17e84
Point 8: 02fccde8e9b11a8d34bc1cddb50aca2d9158d8d3a8ced807020f934931ac6095
Point 9: 45097b0216b48412d811c2c0e7c5f58aba24abdda30b6aa54eae160e10943df0
```

Full 256 points available in `verkle_test_vectors.txt`.

### Hash to Bytes

`hash_to_bytes(generator)`: `d1e7de2aaea9603d5bc6c208d319596376556ecd8336671ba7670c2139772d14`

Algorithm: `LE_bytes(X / Y mod q)`

## Verification

Test vectors are generated from:
- **go-verkle**: v0.2.2
- **go-ipa**: v0.0.0-20240223125850-b1e8a79f509c

To verify versions:

```bash
# Check go-ipa version
go list -m github.com/crate-crypto/go-ipa
# Expected: github.com/crate-crypto/go-ipa v0.0.0-20240223125850-b1e8a79f509c

# Check go-verkle version
go list -m github.com/ethereum/go-verkle
# Expected: github.com/ethereum/go-verkle v0.2.2
```

## Usage in Tests

See `services/evm/tests/verkle_compat_test.rs`:

```rust
#[test]
fn test_generator_compression() {
    let gen = BandersnatchPoint::generator();
    let compressed = gen.to_bytes();

    assert_eq!(compressed, GENERATOR_COMPRESSED);
}

#[test]
fn test_srs_generation_first_10() {
    let srs = generate_srs_points(10);

    for (i, expected) in SRS_VECTORS.iter().enumerate() {
        assert_eq!(srs[i].to_bytes(), *expected);
    }
}
```

## Regeneration

Regenerate test vectors after:
- go-ipa version change
- go-verkle version change
- Algorithm clarification

Always commit both:
- `generate_verkle_vectors.go` (source)
- `verkle_test_vectors.txt` (output)

## Reference

See `services/evm/docs/VERKLE_IMPLEMENTATION_GUIDE.md` for complete implementation guide.
