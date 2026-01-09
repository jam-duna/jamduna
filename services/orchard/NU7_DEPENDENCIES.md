# NU7 Dependency Tracking

**Last Updated**: 2026-01-05
**Status**: Phase 1 - Wire Format Parsing (COMPLETE)

---

## Pinned Dependencies

### QED-it/orchard
- **Branch**: `zcash` (local fork with ZSA + SwapBundle)
- **Commit**: `fd9f14c9b5e7500e7c12bdf823d97a38f0637eb9`
- **Commit Message**: "ZCash ZSA + SwapBundle with surplus"
- **Path**: `/Users/michael/Desktop/jam/orchard`
- **Features Used**: `zsa`, `circuit`, `std`

**Key Capabilities**:
- OrchardZSA circuit with asset_base support
- SwapBundle with multiple action groups
- IssueBundle for asset creation
- Burn records in binding signature
- Split notes (outputs without spends)

### QED-it/librustzcash
- **Branch**: `zcash` (local fork with ZSA support)
- **Commit**: `fd9f14c9b5e7500e7c12bdf823d97a38f0637eb9`
- **Commit Message**: "ZCash ZSA + SwapBundle with surplus"
- **Path**: `/Users/michael/Desktop/jam/librustzcash`
- **Features Used**: `zsa-swap`, `nu7`

**Key Capabilities**:
- TransactionV6 wire format
- OrchardBundle enum (Vanilla/ZSA/Swap)
- SwapBundle serialization/deserialization
- IssueBundle serialization/deserialization
- `consensus_branch_id` routing (NU7 vs Swap)

---

## Wire Format Validation Checklist

### TransactionV6 Structure ✅
- [x] `version: u32` (6 | 1 << 31)
- [x] `consensus_branch_id: u32` (determines decoder)
- [x] `orchard_bundle: Option<OrchardBundle>`
- [x] `issue_bundle: Option<IssueBundle>`
- [x] `zip233_amount: u64` (fee payment for swaps)

**Source**: librustzcash/zcash_primitives/src/transaction/mod.rs:431-446

### OrchardBundle Enum ✅
- [x] `OrchardVanilla(Bundle)` - V5 compatibility
- [x] `OrchardZSA(Bundle)` - V6 single action group with assets
- [x] `OrchardSwap(SwapBundle)` - V6 multi-party swaps

**Source**: librustzcash/zcash_primitives/src/transaction/mod.rs:343-348

### SwapBundle Format ✅
```rust
pub struct SwapBundle<V> {
    action_groups: Vec<Bundle<ActionGroupAuthorized, V, OrchardZSA>>,
    value_balance: V,
    binding_signature: VerBindingSig,
}
```

**Wire Format**:
1. CompactSize: num_action_groups
2. For each action group:
   - CompactSize + actions (NonEmpty)
   - flags (1 byte)
   - anchor (32 bytes)
   - expiry_height (u32 LE, must be 0 for ZSA)
   - CompactSize + burns
   - CompactSize + proof
   - Spend auth signatures (versioned)
3. value_balance (i64 LE)
4. binding_signature (versioned)

**Source**:
- librustzcash/zcash_primitives/src/transaction/components/orchard.rs:483-528
- orchard/src/swap_bundle.rs:21-30

**Validated Against**: NU7.md TransactionV6 Wire Format Specification

### IssueBundle Format ✅
```rust
pub struct IssueBundle<T: IssueAuth> {
    ik: IssueValidatingKey,
    actions: NonEmpty<IssueAction>,
    authorization: T,
}

pub struct IssueAction {
    asset_desc_hash: [u8; 32],
    notes: Vec<Note>,
    finalize: bool,
}
```

**Wire Format**:
1. CompactSize + issuer key bytes
2. CompactSize + actions (NonEmpty)
3. CompactSize + sighash_info
4. CompactSize + signature

**None Encoding**: issuer_size=0 && action_count=0

**Source**:
- librustzcash/zcash_primitives/src/transaction/components/issuance.rs:163-190
- orchard/src/issuance.rs:52-72

**Validated Against**: NU7.md TransactionV6 Wire Format Specification

---

## consensus_branch_id Routing

### BranchId::Nu7
- **Decoder**: OrchardZSA (single action group)
- **Capabilities**: Assets, burns, finalization
- **Use Case**: Regular asset transfers and issuance

### BranchId::Swap
- **Decoder**: OrchardSwap (multiple action groups)
- **Capabilities**: Multi-party swaps, split notes, aggregated binding signature
- **Use Case**: Atomic asset exchanges between multiple parties

**Source**: librustzcash/zcash_primitives/src/transaction/mod.rs

---

## Asset ID Derivation

**Algorithm**:
```rust
asset_id = derive_asset_id(
    issuer_key: IssueValidatingKey,
    asset_desc_hash: [u8; 32]
)
```

**Properties**:
- Deterministic: Same issuer + description → same asset ID
- Unique: Different issuer OR description → different asset ID
- Finalize flag: Prevents future issuance if set

**Source**: orchard/src/issuance.rs (asset ID derivation logic)

---

## Key Verification Parameters

### OrchardZSA Circuit
- **Verifying Key**: TBD (requires generation from circuit)
- **Public Inputs**: actions, anchor, nullifiers, commitments, burns, asset_base
- **Proof System**: Halo2

### OrchardSwap Circuit
- **Verifying Key**: Same as OrchardZSA (per-action-group verification)
- **Public Inputs**: Same as OrchardZSA (per action group)
- **Bundle-level**: Aggregated value balance, binding signature

**Note**: Verification keys not yet generated. This is a Phase 2 dependency.

---

## Open Questions

### 1. Public Input Layout
- **Question**: Exact layout of public inputs for OrchardZSA circuit (with asset_base and burns)
- **Status**: OPEN - Need to verify against QED-it/orchard zsa_swap circuit
- **Action Required**: Inspect circuit definition in orchard/src/circuit.rs

### 2. IssueBundle Finalization Tracking
- **Question**: Is finalization enforced by circuit or service-level state?
- **Status**: OPEN - Need to verify enforcement mechanism
- **Action Required**: Review orchard/src/issuance.rs for finalization constraints

### 3. Extrinsic Payload Format
- **Question**: Embed full TransactionV6 or only OrchardBundle + IssueBundle fields?
- **Status**: OPEN - Design decision needed
- **Recommendation**: Only bundle fields (more efficient, less redundant)

---

## Version Compatibility Matrix

| Component | V5 (Vanilla) | V6 (ZSA) | V6 (Swap) |
|-----------|--------------|----------|-----------|
| OrchardBundle | ✅ Vanilla | ✅ ZSA | ✅ Swap |
| IssueBundle | ❌ | ✅ | ✅ |
| Multi-asset | ❌ | ✅ | ✅ |
| Swap bundles | ❌ | ❌ | ✅ |
| Split notes | ❌ | ✅ | ✅ |
| Burns | ❌ | ✅ | ✅ |

---

## Build Metadata

```toml
[package.metadata.nu7]
orchard_commit = "fd9f14c9b5e7500e7c12bdf823d97a38f0637eb9"
librustzcash_commit = "fd9f14c9b5e7500e7c12bdf823d97a38f0637eb9"
orchard_branch = "zcash"
librustzcash_branch = "zcash"
last_verified = "2026-01-05"
```

---

## Phase 1 Progress

- [x] Create OrchardExtrinsic enum with V6 variant
- [x] Implement bundle_codec.rs extensions for SwapBundle/IssueBundle
- [x] Add unit tests for wire format parsing
- [ ] Update services/orchard/Cargo.toml with pinned dependencies (pending QED-it fork confirmation)

### Phase 1 Deliverables

**Completed Files**:
- `services/orchard/src/nu7_types.rs` (507 lines) - V6 type definitions and OrchardExtrinsic enum
- `services/orchard/src/bundle_codec.rs` (+379 lines) - Wire format parsers for ZSA/Swap/Issue bundles

**New Type Definitions**:
1. `OrchardExtrinsic` enum - JAM extrinsic payload with V5/V6 variants
2. `OrchardBundleType` enum - Discriminator (Vanilla/ZSA/Swap)
3. `AssetBase` - 32-byte asset identifier with native USDx support
4. `BurnRecord` - Asset destruction record
5. `ParsedSwapBundle` - Multi-party swap structure
6. `ParsedIssueBundle` - Asset issuance structure

**New Wire Format Parsers**:
1. `decode_zsa_bundle()` - Parses V6 single action group with assets and burns
2. `decode_swap_bundle()` - Parses V6 multi-party swaps with multiple action groups
3. `decode_issue_bundle()` - Parses V6 asset issuance bundles (with None encoding support)

**Test Coverage** (7 tests, all passing):
- ✅ ZSA bundle minimal (no burns)
- ✅ ZSA bundle with burns (native + custom assets)
- ✅ SwapBundle with 2 action groups
- ✅ IssueBundle None encoding
- ✅ IssueBundle single action with finalize flag
- ✅ Reject empty SwapBundle (NonEmpty constraint)
- ✅ Reject empty ZSA bundle (NonEmpty constraint)

**Validation**: All tests pass in `cargo test bundle_codec::tests`

---

## Next Steps (Phase 2)

- [ ] Generate OrchardZSA verification key from QED-it/orchard circuit
- [ ] Update refiner.rs for multi-action-group verification
- [ ] Add IssueBundle signature verification
- [ ] Implement asset ID derivation helper
- [ ] Add state transition rules for burns and issuance
