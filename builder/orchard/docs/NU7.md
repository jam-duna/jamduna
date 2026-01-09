# Zcash Shielded Assets (ZSA) - NU7 Technical Summary

Our current implementation is NU5 (Transactionv5) but [NU7](https://github.com/zcash/zips/milestone/82) introduces 
TransactionV6 with two major capabilities: **asset issuance** and **atomic multi-party swaps**, developed in these repos:

- **[QED-it/orchard](https://github.com/QED-it/orchard/tree/zsa_swap)** (`zsa_swap`) - OrchardZSA circuit with asset_base, split notes, burn, swap bundles
- **[zcash/librustzcash PR #1931](https://github.com/zcash/librustzcash/pull/1931)** (`zsa-swap`) - TxV6 format, swap bundles, issuance bundle, fee calculation
- **[QED-it/librustzcash](https://github.com/QED-it/librustzcash/tree/zsa-swap)** (`zsa-swap`) - ZSA wallet infrastructure with swap support
- **[QED-it/halo2](https://github.com/QED-it/halo2/tree/zsa1)** (`zsa1`) - Halo2 proving system fork
- **[QED-it/zebra](https://github.com/QED-it/zebra)** (`zsa-integration`) - Consensus validation for ZSA
- **[QED-it/zcash_tx_tool](https://github.com/QED-it/zcash_tx_tool)** - E2E testing for ZSA issuance, transfer, burn

Key files in the above:

- **librustzcash/zsa-swap**: `zcash_primitives/src/transaction/`
  - `mod.rs` - TransactionData, OrchardBundle enum
  - `components/orchard.rs` - Swap bundle serialization
  - `components/issuance.rs` - Issue bundle serialization
- **orchard/zsa_swap**: `src/`
  - `swap_bundle.rs` - SwapBundle structure
  - `bundle.rs` - Bundle (action group) structure
  - `issuance.rs` - IssueBundle structure
  

```rust
struct TransactionV6 {
    version: u32,                    // 6
    version_group_id: u32,
    consensus_branch_id: u32,        // Determines decoder (NU7 vs Swap)
    lock_time: u32,
    expiry_height: u32,
    // zip233_amount: u64 (if ZIP-233 enabled)

    // Transparent (existing)
    tx_in: Vec<TxIn>,
    tx_out: Vec<TxOut>,

    // Sapling (existing)
    sapling_bundle: Option<SaplingBundle>,

    // Orchard (includes swap via OrchardBundle::OrchardSwap)
    orchard_bundle: Option<OrchardBundle>,

    // NEW: Issuance (NU7/ZSA) - Creates new asset IDs
    issue_bundle: Option<IssueBundle>,
}
```

### OrchardBundle Variants (V5/V6)

#### 1. Standard Orchard (V5 only)
```
orchard_bundle: Some(OrchardBundle::OrchardVanilla(...))
issue_bundle: None
```
Regular transfers using existing Orchard protocol.
V6 does not use `OrchardVanilla`; V6 decoding selects `OrchardZSA` (NU7) or
`OrchardSwap` (Swap) based on `consensus_branch_id`.

#### 2. ZSA Issuance (New Asset Creation)

**Purpose**: Create new asset IDs and mint initial supply.

```
orchard_bundle: Some(OrchardBundle::OrchardZSA(...))
issue_bundle: Some(IssueBundle { ... })
```


**How Asset IDs are Created**:
```rust
// From orchard/src/issuance.rs (lines 52-59):
pub struct IssueBundle<T: IssueAuth> {
    ik: IssueValidatingKey<ZSASchnorr>,      // Issuer's public key
    actions: NonEmpty<IssueAction>, // At least 1 action
    authorization: T,             // Issuer signature
}
// From orchard/src/issuance.rs (lines 65-72):
pub struct IssueAction {
    asset_desc_hash: [u8; 32],   // BLAKE2b hash of asset description
    notes: Vec<Note>,             // Newly issued notes
    finalize: bool,               // If true, prevents future issuance
}


// Asset ID derivation (deterministic):
asset_id = derive_asset_id(
    ik,                    // Issuer's public key
    asset_desc_hash        // Hash of asset metadata (name, symbol, etc.)
)
```

**Key Properties**:
- **Deterministic**: Same issuer key + description → same asset ID
- **Unique**: Different issuer OR different description → different asset ID
- **Finalize flag**: If set, issuer cannot create more of this asset
- **Multiple assets per tx**: One IssueBundle can create multiple distinct assets

**Example - Issuing Two Assets**:
```
IssueBundle {
    ik: IssuerKey_Alice,
    actions: [
        IssueAction {
            asset_desc_hash: BLAKE2b("Alice's Gold Token"),
            notes: [1000 tokens → Alice's address],
            finalize: false  // Can issue more later
        },
        IssueAction {
            asset_desc_hash: BLAKE2b("Alice's Silver Token"),
            notes: [500 tokens → Bob's address],
            finalize: true   // No more can be issued
        }
    ],
    authorization: Signature(IssuerKey_Alice)
}
```

This creates **two distinct asset IDs**:
- `asset_id_gold = derive_asset_id(IssuerKey_Alice, BLAKE2b("Alice's Gold Token"))`
- `asset_id_silver = derive_asset_id(IssuerKey_Alice, BLAKE2b("Alice's Silver Token"))`

#### 3. ZSA Atomic Swaps (Multi-Party Exchange) with SwapBundle: Circuit to Transaction Bridge

**Purpose**: Atomically exchange existing assets between multiple parties.

The `SwapBundle` struct (from `orchard/src/swap_bundle.rs`) is the critical bridge between ZSA circuit proofs and the V6 transaction format. It aggregates multiple action groups (each with its own OrchardZSA circuit proof) into a single transaction component.   SwapBundle is **not** a top-level transaction field. It is a variant inside the `OrchardBundle` enum (`OrchardBundle::OrchardSwap(SwapBundle)`), which itself is wrapped in `Option<OrchardBundle>` at the transaction level.


```
orchard_bundle: Some(OrchardBundle::OrchardSwap(SwapBundle { ... }))
issue_bundle: None
```

From orchard/src/swap_bundle.rs (lines 21-30):

```rust
pub struct SwapBundle<V> {
    action_groups: Vec<Bundle<ActionGroupAuthorized, V, OrchardZSA>>, // One per party
    value_balance: V, // Net native-asset (USDx) flow
    binding_signature: VerBindingSig, // Prevents inflation
}
```

- `action_groups`: Each is a `Bundle` with `ActionGroupAuthorized` authorization
- `value_balance`: Generic type `V` (instantiated as `ZatBalance` = `i64` in transactions). In upstream Zcash, `ZatBalance` represents ZEC in zatoshis; here it represents USDx in the same 1e-8 base unit.
- `binding_signature`: Versioned RedPallas binding signature



Each action group contains:
- One party's spend (input asset)
- One party's output (received asset)
- ZK proof of validity
- Spend authorization signatures



**For complete order protocol, see [SURPLUS.md](SURPLUS.md) which situates this with Builders doing the matching:**


```
┌─────────────────────────────────────────────────────────────────┐
│                    V6 TRANSACTION (ZIP-230)                     │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              SWAP BUNDLE (SwapBundle<V>)                │   │
│  │                                                         │   │
│  │  Action Groups: Vec<Bundle<ActionGroupAuthorized>>     │   │
│  │  Value Balance: V (scalar native balance, USDx)        │   │
│  │  Binding Signature: VerBindingSig (signs bundle hash)  │   │
│  │                                                         │   │
│  │  ┌───────────────────────────────────────────────────┐ │   │
│  │  │     ACTION GROUP 1 (Alice's spend/output)         │ │   │
│  │  │  ┌─────────────────────────────────────────────┐  │ │   │
│  │  │  │   CIRCUIT INSTANCE (OrchardZSA)             │  │ │   │
│  │  │  │  - Actions: [spend 100 AAA, output 50 BBB]  │  │ │   │
│  │  │  │  - Proof: Halo2 proof (ActionGroupAuthorized)│ │ │   │
│  │  │  │  - Anchor: Merkle root                       │  │ │   │
│  │  │  │  - Value balance: cv_net                     │  │ │   │
│  │  │  │  - Binding key: bsk₁                         │  │ │   │
│  │  │  └─────────────────────────────────────────────┘  │ │   │
│  │  └───────────────────────────────────────────────────┘ │   │
│  │                                                         │   │
│  │  ┌───────────────────────────────────────────────────┐ │   │
│  │  │     ACTION GROUP 2 (Bob's spend/output)           │ │   │
│  │  │  ┌─────────────────────────────────────────────┐  │ │   │
│  │  │  │   CIRCUIT INSTANCE (OrchardZSA)             │  │ │   │
│  │  │  │  - Actions: [spend 55 BBB, output 95 AAA]   │  │ │   │
│  │  │  │  - Proof: Halo2 proof                        │  │ │   │
│  │  │  │  - Anchor: Merkle root                       │  │ │   │
│  │  │  │  - Value balance: cv_net                     │  │ │   │
│  │  │  │  - Binding key: bsk₂                         │  │ │   │
│  │  │  └─────────────────────────────────────────────┘  │ │   │
│  │  └───────────────────────────────────────────────────┘ │   │
│  │                                                         │   │
│  │  ┌───────────────────────────────────────────────────┐ │   │
│  │  │  ACTION GROUP 3 (Builder + Treasury surplus)      │ │   │
│  │  │  ┌─────────────────────────────────────────────┐  │ │   │
│  │  │  │   CIRCUIT INSTANCE (OrchardZSA)             │  │ │   │
│  │  │  │  - Actions: [output 2 BBB, output 3 BBB,     │  │ │   │
│  │  │  │             output 2 AAA, output 3 AAA]      │  │ │   │
│  │  │  │  - Proof: Halo2 proof (split notes)         │  │ │   │
│  │  │  │  - No spends (dummy spends for circuit)     │  │ │   │
│  │  │  │  - Binding key: bsk₃                         │  │ │   │
│  │  │  └─────────────────────────────────────────────┘  │ │   │
│  │  └───────────────────────────────────────────────────┘ │   │
│  │                                                         │   │
│  │  Bundle-level computation:                             │   │
│  │  • value_balance = Σ(action_group.value_balance())     │   │
│  │  • bsk = Σ(bsk₁ + bsk₂ + bsk₃)                         │   │
│  │  • sighash = hash_swap_bundle(action_groups, balance)  │   │
│  │  • binding_sig = sign(bsk, sighash)                    │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  V6 Transaction Digest (ZIP-244 extension):                    │
│  • Includes swap_bundle.commitment()                           │
│  • commitment = hash_swap_bundle(action_groups, value_balance) │
└─────────────────────────────────────────────────────────────────┘
```


The ZSA circuit lives in `QED-it/orchard`, not librustzcash:

```
QED-it/orchard/
├── src/
│   ├── circuit.rs          # Main Action circuit
│   ├── circuit/            # Circuit gadgets
│   ├── issuance.rs         # Issuance bundle logic
│   ├── note.rs             # Note structure with asset_base
│   └── value.rs            # Value commitments per asset
```

Key circuit additions for ZSA:
- `asset_base: Point` in note structure
- Split note handling (dummy spends for asset validity). Invariant: a split-only action group (no spends) is valid only when its value commitments are balanced by spends in other action groups within the same SwapBundle; otherwise the binding signature fails.
- Value commitment: `cv = v * asset_base + rcv * R`
- Burn records (public asset destruction)

---

**Key Properties**:

1. **ActionGroupAuthorized**: Each wraps one Halo2 ZK proof from OrchardZSA circuit
2. **Value Balance**: Sum of all action group balances (native USDx only; custom assets balanced via commitments)
3. **Binding Signature**: Proves knowledge of all value commitment randomness; prevents inflation
4. **Transaction Commitment**: Cryptographically links swap bundle to transaction ID
5. **Split Notes**: Outputs with no spends (used for surplus routing to builder/treasury)





## Details: Core Structures


# TransactionV6 Wire Format Specification

**Source**: QED-it/librustzcash `zsa-swap` + QED-it/orchard `zsa_swap`
**References**: ZIP-230 (V6 Format), ZIP-228 (Asset Swaps), ZIP-226/227 (ZSA)
ZIP-228 defines the consensus-level swap capability; this document (and `SURPLUS.md`) describes an application-layer protocol that uses that capability.

This section provides implementation-ready wire format details for V6 transactions, including SwapBundle and IssueBundle serialization.


## Complete V6 Transaction Wire Format

```
TransactionV6 {
    // === Header ===
    version: u32 = 6 | (1 << 31)
    version_group_id: u32 = V6_VERSION_GROUP_ID
    consensus_branch_id: u32  // IMPORTANT: NU7 vs Swap decoding
    lock_time: u32
    expiry_height: u32
    zip233_amount: u64  // NEW for V6

    // === Transparent ===
    tx_in: CompactSize + Vec<TxIn>
    tx_out: CompactSize + Vec<TxOut>

    // === Sapling ===
    v_spends_sapling: CompactSize + Vec<SpendDescription>
    v_outputs_sapling: CompactSize + Vec<OutputDescription>
    value_balance_sapling: i64 (if sapling present)
    anchor_sapling: [u8; 32] (if spends present)
    v_spend_proofs_sapling: Vec<Proof>
    v_spend_auth_sigs_sapling: Vec<Signature>
    v_output_proofs_sapling: Vec<Proof>
    binding_sig_sapling: Signature (if sapling present)

    // === Orchard (V5 Vanilla / V6 ZSA / V6 Swap) ===
    // Serialization depends on consensus_branch_id:
    //   - Nu7: OrchardZSA (single action group)
    //   - Swap: OrchardSwap (multiple action groups)

    // === IssueBundle (V6 only) ===
    // See format below
}
```

### Orchard Swap Bundle Format

```
SwapBundle {
    // Number of action groups
    num_action_groups: CompactSize

    // For each action group (MUST have at least 1 action):
    action_group: {
        // Actions (NonEmpty - at least 1)
        num_actions: CompactSize (>= 1)
        actions: Vec<Action> (without spend auth sigs)

        // Metadata
        flags: u8
        anchor: [u8; 32]
        expiry_height: u32 LE (always 0 for ZSA)

        // Burns
        num_burns: CompactSize
        burns: Vec<(asset_base: [u8; 32], value: u64 LE)>

        // Proof
        proof_size: CompactSize
        proof: Vec<u8>

        // Spend auth signatures
        spend_auth_sigs: Vec<VerSpendAuthSig> (one per action)
    }

    // Bundle-level metadata
    value_balance: i64 LE
    binding_signature: VerBindingSig (versioned)
}
```

### IssueBundle Format

```
IssueBundle {
    // Option encoding:
    // - None: issuer_size=0, action_count=0
    // - Some: issuer_size>0, action_count>0

    // 1. Issuer key
    issuer_size: CompactSize
    issuer_bytes: Vec<u8> (IssueValidatingKey encoding)

    // 2. Actions (only if issuer_size > 0)
    action_count: CompactSize
    actions: Vec<IssueAction> {
        asset_desc_hash: [u8; 32]
        note_count: CompactSize
        notes: Vec<Note> {
            recipient: [u8; 43]
            value: u64 LE
            rho: [u8; 32]
            rseed: [u8; 32]
        }
        finalize: u8  // 0x00 or 0x01
    }

    // 3. Authorization (only if issuer_size > 0)
    sighash_info_size: CompactSize
    sighash_info_bytes: Vec<u8>
    signature_size: CompactSize
    signature_bytes: Vec<u8>
}
```

---


### Option Layering 

`SwapBundle` itself is NOT wrapped in `Option`. The optionality comes from `Option<OrchardBundle>` at the transaction level.  The `Option` wrapping happens at the **transaction level**, not inside the Orchard crate:

```
TransactionData {
    orchard_bundle: Option<OrchardBundle<A>>  // <-- Option here
}

OrchardBundle<A> = enum {
    OrchardVanilla(Bundle<A, ZatBalance, OrchardVanilla>), // V5 only
    OrchardZSA(Bundle<A, ZatBalance, OrchardZSA>),
    OrchardSwap(SwapBundle<ZatBalance>),  // <-- SwapBundle is NOT Option<SwapBundle>
}
```


### Consensus Branch Selection

**IMPORTANT**: The transaction reader selects between Swap vs ZSA decoding based on `consensus_branch_id`:

From librustzcash/zcash_primitives/src/transaction/mod.rs:
- **NU7 branch**: Decodes OrchardZSA (single action group)
- **Swap branch**: Decodes OrchardSwap (multiple action groups)

This means test vectors MUST specify the correct `consensus_branch_id` to trigger the right decoder path.

---


### TransactionData

From librustzcash/zcash_primitives/src/transaction/mod.rs (lines 431-446):

```rust
pub struct TransactionData<A: Authorization> {
    version: TxVersion,
    consensus_branch_id: BranchId,
    lock_time: u32,
    expiry_height: BlockHeight,
    #[cfg(all(zcash_unstable = "nu7", feature = "zip-233"))]
    zip233_amount: Zatoshis,  // NEW for V6
    transparent_bundle: Option<transparent::Bundle<A::TransparentAuth>>,
    sprout_bundle: Option<sprout::Bundle>,
    sapling_bundle: Option<sapling::Bundle<A::SaplingAuth, ZatBalance>>,
    orchard_bundle: Option<OrchardBundle<A::OrchardAuth>>,  // Can be Swap variant
    #[cfg(zcash_unstable = "nu7")]
    issue_bundle: Option<IssueBundle<A::IssueAuth>>,
}
```

### OrchardBundle Enum

From librustzcash/zcash_primitives/src/transaction/mod.rs (lines 343-348):

```rust
pub enum OrchardBundle<A: orchard::bundle::Authorization> {
    OrchardVanilla(Bundle<A, ZatBalance, OrchardVanilla>), // V5 only
    #[cfg(zcash_unstable = "nu7")]
    OrchardZSA(Bundle<A, ZatBalance, OrchardZSA>),
    #[cfg(zcash_unstable = "nu7")]
    OrchardSwap(SwapBundle<ZatBalance>),
}
```

**Note**: `ZatBalance` is the `i64`-backed value type used throughout librustzcash transaction code.

### ActionGroupAuthorized

From orchard/src/swap_bundle.rs (lines 86-98):

```rust
pub struct ActionGroupAuthorized {
    proof: Proof,  // Halo2 ZK proof
}

impl Authorization for ActionGroupAuthorized {
    type SpendAuth = VerSpendAuthSig;
    fn proof(&self) -> Option<&Proof> {
        Some(&self.proof)
    }
}
```

### Bundle (Action Group)

From orchard/src/bundle.rs (lines 235-255):

```rust
pub struct Bundle<A: Authorization, V, P: OrchardPrimitives> {
    actions: NonEmpty<Action<A::SpendAuth, P>>,  // <-- NonEmpty (at least 1)
    flags: Flags,
    value_balance: V,
    burn: Vec<(AssetBase, NoteValue)>,
    anchor: Anchor,
    expiry_height: u32,  // Reserved, always 0 for ZSA
    authorization: A,
}
```

**CRITICAL**: `actions` is `NonEmpty<Action>` - bundles MUST have at least one action.



#### read_orchard_swap_bundle

**Source**: librustzcash/zcash_primitives/src/transaction/components/orchard.rs:137-151

```rust
pub fn read_orchard_swap_bundle<R: Read>(mut reader: R)
    -> io::Result<Option<SwapBundle<ZatBalance>>>
{
    let action_groups = read_action_groups(&mut reader)?;

    if action_groups.is_empty() {
        return Ok(None);  // <-- Option returned here
    }

    let (value_balance, binding_signature) = read_bundle_balance_metadata(&mut reader)?;

    Ok(Some(SwapBundle::from_parts(
        action_groups,
        value_balance,
        binding_signature,
    )))
}
```

**Returns**: `Option<SwapBundle<ZatBalance>>`
**Logic**: Empty action groups → `None`, otherwise `Some(SwapBundle)`

#### write_orchard_swap_bundle

**Source**: librustzcash/zcash_primitives/src/transaction/components/orchard.rs:483-498

```rust
pub fn write_orchard_swap_bundle<W: Write>(
    mut writer: W,
    bundle: &SwapBundle<ZatBalance>,
) -> io::Result<()> {
    CompactSize::write(&mut writer, bundle.action_groups().len())?;

    for ag in bundle.action_groups() {
        write_action_group(&mut writer, ag)?;
    }

    write_bundle_balance_metadata(
        &mut writer,
        bundle.value_balance(),
        bundle.binding_signature(),
    )?;

    Ok(())
}
```

**Format**:
1. CompactSize: number of action groups
2. For each action group: `write_action_group()`
3. Bundle metadata: value_balance (i64) + binding_signature

#### write_action_group

**Source**: librustzcash/zcash_primitives/src/transaction/components/orchard.rs:501-528

```rust
fn write_action_group<W: Write, A: Authorization<SpendAuth = VerSpendAuthSig>>(
    mut writer: W,
    bundle: &orchard::Bundle<A, ZatBalance, OrchardZSA>,
) -> io::Result<()> {
    // 1. Actions (without spend auth signatures) - NonEmpty
    Vector::write_nonempty(&mut writer, bundle.actions(), |w, a| {
        write_action_without_auth(w, a)
    })?;

    // 2. Flags (1 byte)
    writer.write_all(&[bundle.flags().to_byte()])?;

    // 3. Anchor (32 bytes)
    writer.write_all(&bundle.anchor().to_bytes())?;

    // 4. Timelimit/expiry_height (u32 LE - must be 0 for NU7)
    writer.write_u32_le(bundle.expiry_height())?;

    // 5. Burn vector
    write_burn(&mut writer, bundle.burn())?;

    // 6. Proof (CompactSize + proof bytes)
    Vector::write(
        &mut writer,
        bundle.authorization().proof().unwrap().as_ref(),
        |w, b| w.write_u8(*b),
    )?;

    // 7. Spend auth signatures (one per action)
    Array::write(
        &mut writer,
        bundle.actions().iter().map(|a| a.authorization()),
        |w, auth| write_versioned_signature(w, auth),
    )?;

    Ok(())
}
```

**Format**:
1. CompactSize + actions (without signatures) - **must be NonEmpty**
2. 1 byte: flags
3. 32 bytes: anchor
4. 4 bytes LE: expiry_height (u32)
5. CompactSize + burn items
6. CompactSize + proof bytes
7. Versioned spend auth signatures (one per action)

#### write_burn

**Source**: librustzcash/zcash_primitives/src/transaction/components/orchard.rs:453-460

```rust
pub fn write_burn<W: Write>(writer: &mut W, burn: &[(AssetBase, NoteValue)]) -> io::Result<()> {
    Vector::write(writer, burn, |w, (asset, amount)| {
        w.write_all(&asset.to_bytes())?;  // 32 bytes
        w.write_all(&amount.to_bytes())?;  // 8 bytes LE
        Ok(())
    })?;
    Ok(())
}
```

**Format**:
- CompactSize: number of burn items
- For each: 32 bytes (asset_base) + 8 bytes LE (note_value as u64)

### IssueBundle Serialization

From librustzcash/zcash_primitives/src/transaction/components/issuance.rs:

#### write_bundle

**Source**: librustzcash/zcash_primitives/src/transaction/components/issuance.rs:163-190

```rust
pub fn write_bundle<W: Write>(
    bundle: Option<&IssueBundle<Signed>>,
    mut writer: W,
) -> io::Result<()> {
    if let Some(bundle) = bundle {
        // 1. Issuer key (CompactSize + bytes)
        Vector::write(&mut writer, &bundle.ik().encode(), |w, b| w.write_u8(*b))?;

        // 2. Actions (NonEmpty)
        Vector::write_nonempty(&mut writer, bundle.actions(), write_action)?;

        // 3. Authorization sighash_info (CompactSize + bytes)
        let sighash_info_bytes = ISSUE_SIGHASH_VERSION_TO_INFO_BYTES
            .get(bundle.authorization().signature().version())?;
        Vector::write(&mut writer, sighash_info_bytes, |w, b| w.write_u8(*b))?;

        // 4. Authorization signature (CompactSize + bytes)
        Vector::write(
            &mut writer,
            &bundle.authorization().signature().sig().encode(),
            |w, b| w.write_u8(*b),
        )?;
    } else {
        // Option::None encoding
        CompactSize::write(&mut writer, 0)?;  // Empty issuer
        CompactSize::write(&mut writer, 0)?;  // Empty actions
    }
    Ok(())
}
```

**Format for Some(IssueBundle)**:
1. CompactSize + issuer key bytes
2. CompactSize + actions (NonEmpty)
3. CompactSize + sighash_info bytes
4. CompactSize + signature bytes

**Format for None**:
- CompactSize(0) - empty issuer
- CompactSize(0) - empty actions

#### write_action

**Source**: librustzcash/zcash_primitives/src/transaction/components/issuance.rs:193-198

```rust
fn write_action<W: Write>(mut writer: &mut W, action: &IssueAction) -> io::Result<()> {
    writer.write_all(action.asset_desc_hash())?;  // 32 bytes
    Vector::write(&mut writer, action.notes(), write_note)?;
    writer.write_u8(action.is_finalized() as u8)?;  // 0x00 or 0x01
    Ok(())
}
```

**Format**:
- 32 bytes: asset_desc_hash
- CompactSize + notes
- 1 byte: finalize flag (0x00 = false, 0x01 = true)

#### write_note

**Source**: librustzcash/zcash_primitives/src/transaction/components/issuance.rs:201-207

```rust
pub fn write_note<W: Write>(writer: &mut W, note: &Note) -> io::Result<()> {
    writer.write_all(&note.recipient().to_raw_address_bytes())?;  // 43 bytes
    writer.write_all(&note.value().to_bytes())?;  // 8 bytes LE (u64)
    writer.write_all(&note.rho().to_bytes())?;  // 32 bytes
    writer.write_all(note.rseed().as_bytes())?;  // 32 bytes
    Ok(())
}
```

**Format (total 115 bytes per note)**:
- 43 bytes: recipient (Orchard address)
- 8 bytes LE: value (u64)
- 32 bytes: rho
- 32 bytes: rseed

---


## Key Implementation Notes

1. **Option is at transaction level**: `Option<OrchardBundle>`, not `Option<SwapBundle>` inside orchard crate
2. **OrchardBundle is an enum**: Can be Vanilla, ZSA, or Swap variant
3. **Empty action groups → None**: `read_orchard_swap_bundle` returns `None` if action_groups is empty
4. **ZatBalance = i64**: Used consistently throughout librustzcash transaction code
5. **Versioned signatures**: All signatures (spend auth, binding) include version metadata
6. **Expiry height always 0**: For ZSA bundles, `expiry_height` is reserved and set to 0
7. **NonEmpty constraint**: Action groups MUST have at least 1 action (Rust type enforces this)
8. **IssueBundle Option encoding**: Empty issuer (size=0) + empty actions (count=0) = None
9. **Spend auth from authorization()**: Use `action.authorization()`, NOT `action.rk()`
10. **consensus_branch_id matters**: Determines which decoder (Swap vs ZSA) is used

---

## References


- [ZIP 226](https://zips.z.cash/zip-0226) - Transfer and Burn
- [ZIP 227](https://zips.z.cash/zip-0227) - Issuance
- [ZIP 228](https://github.com/zcash/zips/issues/776) - Asset Swaps
- [ZIP 230](https://zips.z.cash/zip-0230) - TxV6 Format
