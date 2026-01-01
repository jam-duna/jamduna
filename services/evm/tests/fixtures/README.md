# Verkle Test Fixtures

This directory contains test fixtures for Verkle proof verification, generated using `go-verkle@v0.2.2`.

## Generating Fixtures

To regenerate all fixtures:

```bash
cd services/evm/tests/fixtures
go run generate_fixtures.go
```

This will create 4 test fixture files covering different scenarios.

## Fixture Format

All fixtures follow this JSON structure:

```json
{
  "metadata": {
    "name": "fixture_name",
    "description": "Human-readable description",
    "go_verkle_version": "v0.2.2",
    "generated_at": "2025-12-23T...",
    "scenario": "scenario_type"
  },
  "pre_state_root": "hex_encoded_root_commitment",
  "post_state_root": "hex_encoded_root_commitment",
  "verkle_proof": {
    "otherStems": [],
    "depthExtensionPresent": "0x...",
    "commitmentsByPath": ["0x...", "0x..."],
    "d": "0x...",
    "ipaProof": {
      "cl": ["0x...", ...],
      "cr": ["0x...", ...],
      "finalEvaluation": "0x..."
    }
  },
  "state_diff": [
    {
      "stem": "0x...",
      "suffixDiffs": [
        {
          "suffix": 0,
          "currentValue": "0x...",
          "newValue": null
        }
      ]
    }
  ]
}
```

### Field Descriptions

#### `metadata`
- **name**: Short identifier for the fixture
- **description**: Human-readable description of the test scenario
- **go_verkle_version**: Version of go-verkle used to generate the fixture
- **generated_at**: ISO 8601 timestamp of generation
- **scenario**: Scenario type (see below)

#### `pre_state_root` / `post_state_root`
- Hex-encoded 32-byte root commitment
- `pre_state_root`: Tree state before any updates
- `post_state_root`: Tree state after updates (same as pre for read-only proofs)

#### `verkle_proof`
go-verkle's serialized proof format:

- **otherStems**: Stems for proof-of-absence (empty if all keys present)
- **depthExtensionPresent**: Bitfield indicating which tree levels have commitments
- **commitmentsByPath**: Array of commitments along the proof path
  - **Note**: go-verkle v0.2.2+ uses **pruned format** - only includes commitments that cannot be reconstructed from state diff
  - Typically: `[root, leaf]` (C1/C2 child commitments omitted)
- **d**: D point (quotient polynomial commitment) for IPA multiproof
- **ipaProof**: IPA (Inner Product Argument) proof structure
  - **cl**: Left commitments for each folding round (8 rounds for 256-element domain)
  - **cr**: Right commitments for each folding round
  - **finalEvaluation**: Final scalar evaluation after all folding rounds

#### `state_diff`
Array of stem-level state changes:

- **stem**: 31-byte stem (common prefix of related keys)
- **suffixDiffs**: Array of suffix-level changes
  - **suffix**: Suffix byte (0-255)
  - **currentValue**: Current 32-byte value (null for absent keys)
  - **newValue**: New value after update (null for read-only proofs)

**Important**: `state_diff` includes **both present and absent keys** for proof-of-absence scenarios, but IPA evaluations only cover **present keys**.

## Test Scenarios

### 1. Proof of Absence (`verkle_fixture_proof_of_absence.json`)
**Purpose**: Test proof-of-absence handling with mixed present/absent keys

- **Stems**: 1 (stem: `0x42000...`)
- **Present keys**: 3 (suffixes 0, 10, 20)
- **Absent keys**: 2 (suffixes 5, 15 - implicitly proven absent)
- **IPA evaluations**: 10 (root + leaf base + leaf pointers + C1 suffix pairs)

**What it tests**:
- Absent keys don't appear in IPA multiproof evaluations
- StateDiff correctly marks absent keys with `currentValue: null`
- Proof verification succeeds with missing keys

**Used by**: `test_fresh_fixture_proof_of_absence()`

---

### 2. Multi-Stem (`verkle_fixture_multi_stem.json`)
**Purpose**: Test proofs spanning multiple stems (different tree branches)

- **Stems**: 3 (stems: `0x10000...`, `0x20000...`, `0x30000...`)
- **Present keys**: 5 total
  - Stem1: suffixes 0, 5
  - Stem2: suffixes 10, 15
  - Stem3: suffix 20
- **IPA evaluations**: Includes root + 3 leaves + C1/C2 suffix data

**What it tests**:
- Multi-stem proof generation and verification
- Correct leaf commitment extraction per stem
- Cross-stem proof aggregation

---

### 3. Deep Tree (`verkle_fixture_deep_tree.json`)
**Purpose**: Test extension nodes and deep path traversal

- **Stems**: 3 (with extreme values to force extension nodes)
  - `0x00000...01` (low)
  - `0x00000...02` (low)
  - `0xfffff...01` (high)
- **Present keys**: 3 (one per stem, all suffix 0)

**What it tests**:
- Extension node handling (long common prefixes)
- Deep path commitment verification
- Sparse tree structures

---

### 4. Mixed Multi-Stem (`verkle_fixture_mixed_multi_stem.json`)
**Purpose**: Combine multi-stem with proof-of-absence

- **Stems**: 2 (stems: `0x50000...`, `0x60000...`)
- **Present keys**: 4
  - Stem1: suffixes 0, 10 (implicitly proves 5 absent)
  - Stem2: suffixes 0, 20 (implicitly proves 10 absent)

**What it tests**:
- Multi-stem proof-of-absence
- Per-stem absence handling
- Complex state diff structures

## Version Compatibility

### go-verkle v0.2.2+ Format Changes

Starting with go-verkle v0.2.2, the proof format uses **pruned commitments**:

- **Old format**: `commitmentsByPath = [root, leaf, C1, C2, ...]`
- **New format**: `commitmentsByPath = [root, leaf]` (C1/C2 omitted)

**Rationale**: Child commitments (C1/C2) can be reconstructed from the state diff values, so including them is redundant.

**Rust Compatibility**:
- ✅ IPA verification works with both formats
- ⚠️ `verify_prestate_commitment()` currently assumes old format (TODO: fix)
- Use `build_multiproof_inputs_from_state_diff_with_root()` to reconstruct full evaluation set

### Breaking Changes
If regenerating fixtures with a newer go-verkle version:
1. Update `go_verkle_version` in metadata
2. Test against Rust verifier to ensure compatibility
3. Update this README with any format changes

## Regenerating Specific Scenarios

The `generate_fixtures.go` script has separate functions for each scenario:

```go
generateProofOfAbsence()      // Scenario 1
generateMultiStem()            // Scenario 2
generateDeepTree()             // Scenario 3
generateMixedMultiStem()       // Scenario 4
```

To generate only specific scenarios, modify `main()` to call only the needed functions.

## Adding New Scenarios

To add a new test scenario:

1. **Add generator function** in `generate_fixtures.go`:
   ```go
   func generateMyScenario() {
       tree := verkle.New()
       // ... build tree ...
       proof, _, _, _, _ := verkle.MakeVerkleMultiProof(tree, nil, keys, nil)
       vp, stateDiff, _ := verkle.SerializeProof(proof)

       fixture := VerkleFixture{
           Metadata: FixtureMetadata{
               Name: "my_scenario",
               Description: "What this tests",
               GoVerkleVersion: "v0.2.2",
               GeneratedAt: time.Now().UTC().Format(time.RFC3339),
               Scenario: "my_scenario",
           },
           // ... rest of fixture ...
       }
       writeFixture("verkle_fixture_my_scenario.json", fixture)
   }
   ```

2. **Call from main()**:
   ```go
   func main() {
       // ... existing calls ...
       generateMyScenario()
   }
   ```

3. **Document in this README**:
   - Add scenario description
   - Explain what it tests
   - Note any special considerations

4. **Add Rust test** in `services/evm/tests/`:
   ```rust
   #[test]
   fn test_my_scenario() {
       // Load fixture
       // Parse proof
       // Verify
   }
   ```

## Troubleshooting

### Fixture verification fails in Rust

**Check**:
1. go-verkle version matches (`metadata.go_verkle_version`)
2. Rust code handles pruned commitment format
3. IPA evaluation count matches go-verkle's GetProofItems output

**Debug steps**:
```rust
// Log evaluation count
println!("IPA evals: {}", multiproof_inputs.indices.len());

// Compare with Go
// In generate_fixtures.go, add after MakeVerkleMultiProof:
proof, cis, zis, yis, _ := verkle.MakeVerkleMultiProof(...)
fmt.Printf("Go evals: %d\n", len(cis))
```

### "PathMismatch" error

The proof's `commitmentsByPath` length doesn't match `depthExtensionPresent` bits.

**Cause**: Pruned commitment format (v0.2.2+) omits reconstructable commitments

**Fix**: Skip `verify_prestate_commitment()` or update it to handle pruned format (see `IPA.md` Step 4)

### Absent keys appearing in IPA evaluations

**Check**: `build_multiproof_inputs_from_state_diff_with_root()` filters absent keys:
```rust
.filter(|sd| sd.current_value.is_some())
```

This must exclude keys where `current_value == null`.

## References

- [go-verkle](https://github.com/ethereum/go-verkle) - Ethereum's Verkle tree implementation
- [go-ipa](https://github.com/crate-crypto/go-ipa) - IPA polynomial commitment scheme
- [IPA.md](../../IPA.md) - Rust IPA verification debugging guide
- [EIP-6800](https://eips.ethereum.org/EIPS/eip-6800) - Verkle tree specification
