# Utils Package

Shared utilities for JAM services, including state witness verification, Merkle proof validation, and cryptographic functions.

## Running Tests

### Run All Tests

```bash
# From project root
cargo test --package utils --lib -- --nocapture

# From services/utils directory
cd services/utils
cargo test -- --nocapture
```

### Run Individual Tests

```bash
# Test state witness verification with real exported data
cargo test --package utils test_verify_exported_witnesses -- --nocapture

# Test basic verification logic with real witness files
cargo test --package utils verify_test -- --nocapture

# Test opaque key computation
cargo test --package utils test_compute_storage_opaque_key -- --nocapture
```

## State Witness Verification

The `test_verify_exported_witnesses` test verifies real witness files exported from Go tests.

### Generate Witness Files

Run the Go EVM tests to generate witness files:

```bash
make evm_jamtest
```

This creates `.bin` files in the `node/` directory with the format:
```
<objectid>-<version>-<stateroot>.bin
```

Example:
```
0000000000000000000000000000000000000001000000000000000000000000-1-b1c7e52f8638ad944425874da47c59747fc42f0230d3bb55a8b1acc57b53e061.bin
```

### Copy Witness Files for Testing

```bash
# Copy witness files to services/utils for testing
cp node/*.bin services/utils/
```

### What the Test Does

1. Finds all `.bin` files matching the witness pattern
2. Deserializes each witness using `StateWitness::deserialize()`
3. Parses the state root from the filename
4. Verifies the Merkle proof against the state root using `verify_merkle_proof()`
5. Reports success/failure for each witness

### Expected Output

```
Found 3 witness files
✓ 0000...0002-1-b1c7...e061.bin: VERIFIED (service_id=35, path_len=12)
✓ 0000...ff01-1-b1c7...e061.bin: VERIFIED (service_id=35, path_len=10)
✓ 0000...0000-1-b1c7...e061.bin: VERIFIED (service_id=35, path_len=12)

Results:
  Successful verifications: 3
  Failed verifications: 0
  Failed deserializations: 0
  Failed filename parsing: 0
```

## Witness Binary Format

Witnesses use a fixed binary layout (not SCALE encoded):

```
[object_id: 32 bytes]
[object_ref: 64 bytes]
[proof_hashes: 32 bytes each (zero or more entries)]
```

Legacy exports may include a 4-byte little-endian `proof_count` before the proofs. The Rust
deserializer accepts both legacy and current layouts.

### ObjectRef Format (64 bytes)

```
[service_id: 4 bytes LE]
[work_package_hash: 32 bytes]
[index_start: 2 bytes LE]
[index_end: 2 bytes LE]
[version: 4 bytes LE]
[timeslot: 4 bytes LE]
[payload_length: 4 bytes LE]
[reserved: 12 bytes]
```

## Merkle Proof Verification

The verification process:

1. Computes the opaque storage key from `service_id` and `object_id`
2. Creates a leaf node with the value (serialized ObjectRef)
3. Walks up the Merkle tree using the proof hashes
4. Verifies the computed root matches the expected state root

See `verify_merkle_proof()` in `effects.rs` for implementation details.

## Dependencies

- `polkavm` - PolkaVM runtime for service execution
- `blake2` - Blake2b hashing for Merkle trees
- `primitive_types` - H256 type for hashes
- `sha3` - Keccak256 hashing

## Module Structure

- `effects.rs` - State witness verification and Merkle proof validation
- `functions.rs` - Utility functions and host function wrappers
- `hash_functions.rs` - Cryptographic hash functions
- `host_functions.rs` - Host function definitions for PolkaVM
- `objects.rs` - Object reference types and serialization
- `tests.rs` - Test suite for witness verification
- `tracking.rs` - State tracking utilities
