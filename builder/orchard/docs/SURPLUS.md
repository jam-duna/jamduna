# ZSA Intent Swap Order Protocol v1 (Surplus Capture)

**Status**: Draft Specification
**Audience**: Wallet and builder implementers; external reviewers
**Constraint**: No modifications to NU7 circuits

---

## Abstract

This document specifies an **application-layer protocol** for decentralized order-based swaps on Zcash Shielded Assets (ZSA), enabling users to specify **surplus routing** terms when creating swap orders. The protocol operates entirely at the transaction construction layer, requiring no consensus or circuit changes.

**Key properties**:
- Users sign orders specifying minimum outputs and surplus split (builder/treasury percentages)
- Builders aggregate orders into V6 swap transactions
- Wallets enforce order terms before signing spend authorizations
- Reputation and optional bonding discourage builder misbehavior

---


# ZSA Surplus Capture Overview
## Quick Summary

**Constraint**: No modifications to NU7 circuits

**Key Concepts**:
- Users sign orders with surplus routing terms (builder/treasury split)
- Builders aggregate orders into V6 transactions
- Wallets verify outputs before signing
- Enforcement via signatures + reputation + optional bonds

**Architecture**:
```
┌─────────────────────────────────────────┐
│  ORDER PROTOCOL (Application Layer)    │
│  • Signed orders with surplus_split     │
│  • Treasury address derivation          │
│  • Builder matching + transaction build │
└─────────────────────────────────────────┘
                   ↓
┌─────────────────────────────────────────┐
│  EXISTING NU7 CIRCUITS (Unchanged)      │
│  • Validate action groups               │
│  • Verify ZK proofs                     │
│  • Check value balance                  │
└─────────────────────────────────────────┘
```

**Enforcement Model**:
- **Consent**: Orders signed with surplus terms
- **Execution**: Wallets verify final transaction matches order
- **Reputation**: Public verification of builder behavior
- **Optional**: Bonding/slashing for stronger guarantees

See [SURPLUS.md](SURPLUS.md) for:
- Complete order format specification
- Message encoding (246 bytes)
- Signature scheme
- Wallet verification algorithm
- Replay/cancellation mechanisms
- Partial fill rules
- Threat model & mitigations

---

### Key Insights

1. **No Circuit Changes for Surplus**: The surplus routing logic is entirely in transaction construction. The circuit just sees valid actions with balanced value.

2. **Two-Level Signatures**:
   - **Per-action**: Spend authorization signatures (Alice authorizes her spend, Bob authorizes his)
   - **Bundle-level**: Single binding signature over aggregated value balance (prevents inflation)

3. **Cryptographic Binding**: Binding signature ensures no value inflation. Commitment ensures no transaction malleability.

4. **Modular Design**: Each action group is independent. Alice's order generates one action group, Bob's another. Builder combines them into one SwapBundle.

5. **Burn Integration**: Burns are first-class participants in binding signature, preventing unauthorized asset destruction.



## Security Model

### Consent Enforcement (Signatures)

Orders are **signed off-chain** and cryptographically bind the user's stated constraints:
- Input amount and asset
- Minimum output amount and asset
- Surplus routing split (builder/treasury percentages)
- Recipient address
- Expiry and replay protection

**Property**: A builder cannot construct a valid swap transaction using an order signature unless the transaction matches the signed terms.

### Execution Enforcement (Wallets)

Wallets are the **critical trust boundary**. Before signing the final transaction spend authorization:

1. **Parse** the V6 transaction constructed by the builder
2. **Verify** outputs match signed order constraints:
   - User receives ≥ `min_output` of correct asset to correct address
   - Surplus (if any) is routed per signed `surplus_split`
3. **Reject** if any constraint is violated

**Property**: If wallets implement verification correctly, builders cannot steal surplus without being detected.

**Failure mode**: If wallets sign blindly, all surplus protection collapses.

### Economic/Social Enforcement (Reputation & Bonds)

- **Reputation**: Builders who violate order terms can be proven dishonest (if orders are public)
- **Optional bonding**: Builders stake collateral; slashed on provable violations
- **Competition**: Permissionless builder set; users switch to honest builders

**Property**: Rational builders choose honest behavior for long-term revenue.

**Requirement**: Orders or order hashes must be publicly observable for reputation to function.

---

## Terminology

| Term | Definition |
|------|------------|
| **Order** | Signed user intent specifying input, minimum output, and surplus routing |
| **Builder** | Service that aggregates orders and constructs swap transactions |
| **Treasury** | Protocol-defined address receiving remainder of surplus |
| **Surplus** | Excess output beyond `min_output` (e.g., from favorable clearing price) |
| **Fill** | Execution of an order (full or partial) in a swap transaction |
| **Clearing price** | Exchange rate determined by builder's matching algorithm |
| **Action group** | V6 transaction component containing circuit proof for one party's actions |
| **Swap bundle** | V6 transaction component aggregating multiple action groups |
| **Split notes** | Outputs with no corresponding spends (used for surplus routing) |

---

## Order Format

### SignedSwapOrder (NEW STRUCTURE - Application Layer)

**Note**: This structure does NOT exist in orchard or librustzcash. It is a new application-layer protocol structure for the surplus capture order protocol.

```rust
struct SignedSwapOrder {
    // Version
    version: u8,                         // Protocol version (0x01)

    // What I'm spending
    input_asset: AssetBase,              // 32 bytes
    input_amount: u64,                   // Smallest indivisible units
    input_note_nullifier: Nullifier,     // 32 bytes - binds to specific note

    // What I want
    output_asset: AssetBase,             // 32 bytes
    min_output: u64,                     // Minimum acceptable output
    my_address: OrchardAddress,          // 43 bytes

    // Liveness / fill policy
    network_id: u32,                     // Chain/network binding
    order_nonce: u64,                    // Optional replay/cancellation handle
    expiry_block: u32,                   // Reject after this height
    partial_fill: bool,                  // Allow partial fills?
    min_fill_amount: u64,                // Minimum fill (0 = no minimum)

    // Surplus routing
    surplus_split: SurplusSplit,

    // Authorization
    spend_auth_vk: SpendAuthVk,          // Verifier key (32 bytes) - REQUIRED for verification
    order_signature: Signature,          // 64 bytes - signs all above fields
}
```

### SurplusSplit

```rust
// NEW STRUCTURE: Application-layer protocol structure (not in orchard/librustzcash)
struct SurplusSplit {
    builder_percent: u8,  // Percentage to builder (0-100)
    // Treasury receives: 100 - builder_percent
}
```

**Invariants**:
- `builder_percent ≤ 100`
- Treasury share is implicit: `100 - builder_percent`
- Builder address is implicit via submission channel (not in order)
- Treasury address is protocol-derived (not in order)

**Surplus routing**:

| Portion | Recipient | Calculation |
|---------|-----------|-------------|
| Builder share | Builder address (implicit) | `floor(surplus × builder_percent / 100)` |
| Treasury share | Treasury address (derived) | `surplus - builder_share` |

**Example**:
```
Surplus: 10 tokens
builder_percent: 50

builder_share = floor(10 × 50 / 100) = 5
treasury_share = 10 - 5 = 5
```

---

## Message Encoding

### Canonical Byte Layout (V1)

All integers are **little-endian**. Points are **compressed**.

```
Offset  | Length | Field
--------|--------|---------------------------
0       | 1      | version (0x01)
1       | 32     | input_asset (AssetBase)
33      | 8      | input_amount (u64 LE)
41      | 32     | input_note_nullifier (Nullifier)
73      | 32     | output_asset (AssetBase)
105     | 8      | min_output (u64 LE)
113     | 43     | my_address (Orchard raw address)
156     | 4      | network_id (u32 LE)
160     | 8      | order_nonce (u64 LE)
168     | 4      | expiry_block (u32 LE)
172     | 1      | flags (bit 0 = partial_fill)
173     | 8      | min_fill_amount (u64 LE)
181     | 1      | builder_percent (u8)
182     | 32     | spend_auth_vk (SpendAuthVk)
214     | 64     | order_signature
--------|--------|---------------------------
Total:  | 278 bytes
```

**Flags byte** (offset 172):
```
Bit 0: partial_fill (1 = allow, 0 = reject)
Bits 1-7: Reserved (must be 0)
```

**Notes**:
- `treasury_address` is protocol-derived, not serialized
- `builder_address` is implicit via submission channel, not serialized
- `spend_auth_vk` is REQUIRED for signature verification (cannot be derived from signature)
- Signature covers all fields from offset 0-213 (excludes signature itself)

### Versioning

**V1 compatibility**:
- Version byte 0x01 identifies this format
- Future versions (0x02+) may change layout or add fields
- Unknown versions MUST be rejected by wallets and builders

---

## Signing

### Domain Separation

```rust
const ORDER_DOMAIN: &[u8] = b"ZSA_Order_v1";
```

### Signature Coverage

The order signature covers **all order terms** except the signature itself:

```rust
fn sign_order(order: &SwapOrder, sk: &SpendingKey) -> SignedSwapOrder {
    // Derive spend auth verifying key from spending key
    let spend_auth_vk = SpendAuthVk::from(sk);

    let order_hash = blake2b_256(
        personal: ORDER_DOMAIN,
        message: [
            order.version,                         // 1 byte
            order.input_asset,                     // 32 bytes
            order.input_amount.to_le_bytes(),      // 8 bytes
            order.input_note_nullifier,            // 32 bytes
            order.output_asset,                    // 32 bytes
            order.min_output.to_le_bytes(),        // 8 bytes
            order.my_address.to_bytes(),           // 43 bytes
            order.network_id.to_le_bytes(),        // 4 bytes
            order.order_nonce.to_le_bytes(),       // 8 bytes
            order.expiry_block.to_le_bytes(),      // 4 bytes
            order.flags,                           // 1 byte
            order.min_fill_amount.to_le_bytes(),   // 8 bytes
            order.surplus_split.builder_percent,   // 1 byte
            spend_auth_vk.to_bytes(),              // 32 bytes
        ]
    );

    let signature = sk.sign(order_hash);

    SignedSwapOrder {
        ..order,
        spend_auth_vk,
        signature
    }
}
```

**Verification**:
```rust
fn verify_order_signature(order: &SignedSwapOrder) -> bool {
    let order_hash = blake2b_256(/* same as above */);
    order.spend_auth_vk.verify(order_hash, order.order_signature)
}
```

### Signature Algorithm

- **Curve**: RedPallas (Pallas curve, SpendAuth signature type)
- **Hash**: BLAKE2b-256 with domain separation
- **Signature size**: 64 bytes (R point 32 bytes + s scalar 32 bytes)

### Replay Protection

**Network binding**:
- `network_id` prevents replay across chains (mainnet/testnet/devnet)
- Standard values: 0x00000000 (mainnet), 0x00000001 (testnet)

**Nullifier binding (V1)**:
- `input_note_nullifier` binds order to specific note
- Once nullifier is spent, order becomes invalid
- **Tradeoff**: Links off-chain order to on-chain spend (privacy leak)

**Optional nonce** (future):
- `order_nonce` can enable explicit cancellation
- Not enforced in V1 (nullifier binding is primary mechanism)

---

## Rounding Rules

All amounts are **integer units** (no fractional outputs).

### Deterministic Rounding

```rust
fn calculate_surplus_split(surplus: u64, builder_percent: u8) -> (u64, u64) {
    assert!(builder_percent <= 100);

    // Builder gets floor of percentage
    let builder_share = (surplus as u128 * builder_percent as u128 / 100) as u64;

    // Treasury gets remainder (includes rounding dust)
    let treasury_share = surplus - builder_share;

    (builder_share, treasury_share)
}
```

**Properties**:
- `builder_share + treasury_share == surplus` (exact, no loss)
- Rounding dust always goes to treasury
- Deterministic: same inputs → same outputs

**Example**:
```
Surplus: 7 tokens
builder_percent: 50

builder_share = floor(7 × 50 / 100) = 3
treasury_share = 7 - 3 = 4  (includes 1 token rounding dust)
```

---

## Wallet Verification Algorithm

Wallets MUST verify the final transaction before signing spend authorizations.

### Builder Output Disclosure (Required)

**Problem**: Shielded outputs cannot be identified by address without decryption keys.

**Solution**: Builders MUST provide **surplus output disclosures** alongside the transaction:

```rust
struct SurplusDisclosure {
    // For each order in the swap
    order_hash: [u8; 32],                    // Identifies which order

    // Fill information
    filled_input_amount: Option<u64>,        // None = full fill, Some = partial fill amount

    // User's output (wallet can verify via own keys)
    user_output_index: u32,                  // Index in action group
    user_value: u64,                         // Actual value received

    // Builder's surplus output (plaintext disclosure)
    builder_output_index: u32,               // Index in surplus action group
    builder_value: u64,                      // Builder's surplus share
    builder_note_plaintext: NotePlaintext,   // Proof of correct value

    // Treasury's surplus output (plaintext disclosure)
    treasury_output_index: u32,              // Index in surplus action group
    treasury_value: u64,                     // Treasury's surplus share
    treasury_note_plaintext: NotePlaintext,  // Proof of correct value
}
```

**Verification**:
1. Wallet decrypts `user_output` using own viewing key
2. Wallet verifies `builder_note_plaintext` and `treasury_note_plaintext` match actual transaction outputs
3. Wallet checks surplus split is correct

### Constraint Checks (Required)

```rust
fn verify_transaction_against_order(
    tx: &TransactionV6,
    order: &SignedSwapOrder,
    disclosure: &SurplusDisclosure,
) -> Result<(), VerificationError> {
    // 0. Verify disclosure matches order
    if disclosure.order_hash != hash_order(order) {
        return Err(WrongDisclosure);
    }

    // 1. Find and decrypt user's output
    let user_output = tx.get_output(disclosure.user_output_index)?;
    let user_plaintext = decrypt_with_ivk(user_output, order.my_ivk)?;

    if user_plaintext.value != disclosure.user_value {
        return Err(DisclosureMismatch);
    }

    // 2. Determine if this is a partial fill
    let filled_input = disclosure.filled_input_amount.unwrap_or(order.input_amount);
    let is_partial = filled_input < order.input_amount;

    // For partial fills, scale min_output proportionally
    let scaled_min_output = if is_partial {
        if !order.partial_fill {
            return Err(PartialFillNotAllowed);
        }
        if filled_input < order.min_fill_amount {
            return Err(BelowMinimumFill);
        }
        // Scale: min_output * filled_input / input_amount
        (order.min_output as u128 * filled_input as u128 / order.input_amount as u128) as u64
    } else {
        order.min_output
    };

    // 3. Verify minimum output
    if user_plaintext.asset != order.output_asset {
        return Err(WrongAsset);
    }
    if user_plaintext.value < scaled_min_output {
        return Err(InsufficientOutput);
    }

    // 4. Calculate expected surplus split
    let actual_surplus = user_plaintext.value - scaled_min_output;
    let (expected_builder, expected_treasury) =
        calculate_surplus_split(actual_surplus, order.surplus_split.builder_percent);

    // 4. Verify builder surplus disclosure
    if disclosure.builder_value != expected_builder {
        return Err(IncorrectBuilderSurplus);
    }
    let builder_output = tx.get_output(disclosure.builder_output_index)?;
    verify_note_plaintext(builder_output, &disclosure.builder_note_plaintext)?;

    // 5. Verify treasury surplus disclosure
    if disclosure.treasury_value != expected_treasury {
        return Err(IncorrectTreasurySurplus);
    }
    let treasury_output = tx.get_output(disclosure.treasury_output_index)?;
    verify_note_plaintext(treasury_output, &disclosure.treasury_note_plaintext)?;

    // Verify treasury address is correct
    let expected_treasury_addr = derive_treasury_address(order.network_id);
    if disclosure.treasury_note_plaintext.address != expected_treasury_addr {
        return Err(WrongTreasuryAddress);
    }

    // 6. Verify expiry
    if tx.expiry_height > order.expiry_block {
        return Err(OrderExpired);
    }

    Ok(())
}
```

### Exact Output Mode (Optional)

If a **canonical clearing policy** is published by the builder:

```rust
fn verify_exact_outputs(
    tx: &TransactionV6,
    order: &SignedSwapOrder,
    canonical_clearing_price: ClearingPrice,
) -> Result<(), VerificationError> {
    // Calculate expected output from clearing price
    let expected_output = calculate_output(
        order.input_amount,
        canonical_clearing_price
    );

    // Verify user received exact expected amount
    let user_output = find_output_for_address(tx, order.my_address)?;
    if user_output.value != expected_output {
        return Err(UnexpectedOutput);
    }

    // Constraint checks still apply
    verify_transaction_against_order(tx, order)?;

    Ok(())
}
```

**Note**: Exact output mode is **optional**. Constraint-based verification is sufficient for security.

---

## Replay & Cancellation

### V1: Nullifier Binding

**Primary mechanism**:
- Order is bound to `input_note_nullifier`
- Once the nullifier appears on-chain (note spent), order is invalidated
- Automatic cancellation when note is consumed

**Tradeoff**:
- **Privacy leak**: Links off-chain order to on-chain spend
- Enables orderflow surveillance, censorship, front-running

**Implications**:
- Builders can observe which orders correspond to which nullifiers
- Cross-order linkage possible
- Users should consider privacy cost vs. replay protection benefit

### Explicit Cancellation (V1)

Users can publish signed cancellation messages:

```rust
struct OrderCancellation {
    order_hash: [u8; 32],        // Hash of original order
    cancellation_signature: Signature,  // Signs order_hash with same key
}
```

**Distribution**:
- **Gossip mode**: Publish cancellation to same feed as orders
- **Direct submission**: Send cancellation to builder(s) who have the order
- **Best-effort**: Builders not required to honor cancellations (nullifier binding is authoritative)

### Future: Nonce-Based Replay Protection

**V2 design** (out of scope for V1):
- Remove nullifier binding
- Use `order_nonce` with explicit cancellation bulletin board
- Requires additional infrastructure (public bulletin, nonce registry)
- Better privacy but higher complexity

---

## Partial Fills

### Allowance

Partial fills are **only allowed** when `partial_fill = true` in order.

If `partial_fill = false`:
- Builder MUST fill entire `input_amount` or reject order
- Any partial execution invalidates the order

### Single-Transaction Constraint

Partial fills are **single-transaction only**:
- Builder fills portion of order in one transaction
- Returns unfilled amount to user as change output
- **No cross-transaction partial fills** (nullifier binding prevents this)

### Proportional Scaling

All order terms scale **proportionally** with filled amount:

```rust
fn calculate_partial_fill(
    order: &SignedSwapOrder,
    filled_input: u64,  // Amount actually filled (≤ input_amount)
) -> PartialFillTerms {
    assert!(filled_input <= order.input_amount);
    assert!(filled_input >= order.min_fill_amount);  // Check minimum

    // Scale minimum output proportionally
    let scaled_min_output = (order.min_output as u128
        * filled_input as u128
        / order.input_amount as u128) as u64;

    // Calculate change to user
    let change_amount = order.input_amount - filled_input;

    PartialFillTerms {
        filled_input,
        scaled_min_output,
        change_amount,
        // surplus_split applies to filled portion only
        surplus_split: order.surplus_split,
    }
}
```

### Minimum Fill Amount

```rust
if order.min_fill_amount > 0 && filled_input < order.min_fill_amount {
    return Err(BelowMinimumFill);
}
```

**Purpose**: Prevents dust fills that waste user's note

**Disable**: Set `min_fill_amount = 0`

### Example

```
Original order:
  input_amount: 100 AAA
  min_output: 50 BBB
  builder_percent: 50
  partial_fill: true
  min_fill_amount: 20 AAA

Builder fills 60 AAA:
  filled_input: 60 AAA (≥ 20, ✓)
  scaled_min_output: 50 × 60/100 = 30 BBB
  change_output: 40 AAA back to user

Builder clears at 33 BBB:
  user_receives: 30 BBB (min) + 3 BBB (surplus) = 33 BBB
  surplus: 3 BBB
  builder_share: floor(3 × 0.5) = 1 BBB
  treasury_share: 3 - 1 = 2 BBB
```

### Expiry Semantics

`expiry_block` is evaluated at transaction inclusion time:
- If `current_block_height > expiry_block`, transaction is invalid
- Applies to both full and partial fills
- **No multi-transaction fills**: Unfilled remainder requires new order

---

## Observability Modes

### Direct Builder Submission

**Flow**:
1. User sends signed order directly to chosen builder
2. Builder address is implicit (user knows builder's payout address)
3. Order may or may not be published publicly

**Advantages**:
- Simple
- Low latency
- No infrastructure required

**Disadvantages**:
- Limited public auditability
- Reputation enforcement requires post-facto order disclosure
- Builder censorship not observable

### Gossip / Bulletin Board

**Flow**:
1. User publishes signed order (or order hash) to public feed
2. Builders monitor feed and compete to fill orders
3. Anyone can verify builder behavior against published orders

**Advantages**:
- Public auditability
- Reputation enforcement works
- Observable censorship/selective execution

**Disadvantages**:
- Requires infrastructure (p2p gossip or bulletin board service)
- Privacy: All orders are public
- Higher complexity

### Hybrid Approach

**Recommended for V1**:
1. Users submit orders directly to builders (low latency)
2. Builders **optionally** publish order hashes when filling
3. Users can publish full orders post-facto to prove misbehavior

**Properties**:
- Efficient order flow
- Reputation still works (proof of violation possible)
- Privacy: Orders only public when user chooses or builder misbehaves

---

## Threat Model

### Cheating Builder

**Attack**: Builder constructs transaction violating surplus split

```
Alice's order: builder_percent = 50
Builder creates: builder_percent = 100 (keeps all surplus)
```

**Defense**:
1. **Order signature** doesn't match (builder cannot use Alice's signature)
2. If builder obtains Alice's spend auth sig through other means:
   - **Wallet verification** rejects transaction (surplus mismatch)
   - **Public order** allows anyone to prove builder cheated
   - **Reputation** destroyed, users flee to honest builders

**Mitigation**: Wallets MUST implement verification

### Censorship / Selective Execution

**Attack**: Builder refuses to fill certain orders (discrimination)

**Observability**:
- **Direct submission**: Not observable (user doesn't know why order unfilled)
- **Public orders**: Observable (everyone sees order published but not filled)

**Defense**:
- Competition: Users switch to non-censoring builders
- **Requires**: Public orderflow for observability

### Replay Attack

**Attack**: Builder fills same order multiple times

**Defense (V1)**:
- Nullifier binding: Once note spent, order invalid
- **On-chain enforcement**: Consensus rejects duplicate nullifiers

**Note**: Works because V1 binds to specific note nullifier

### Front-Running / MEV

**Attack**: Builder observes profitable order, front-runs with own transaction

**V1 stance**: **Not addressed** (see Non-Goals)

**Partial mitigations**:
- Multiple competing builders (reduces single-builder MEV)
- Threshold encryption (future work)
- Commit-reveal schemes (future work)

### Withheld Execution

**Attack**: Builder accepts order but never fills it

**Defense**:
- **Expiry**: Order expires at `expiry_block`, user can resubmit elsewhere
- **Competition**: Users switch to responsive builders
- **Reputation**: Pattern of withheld execution damages reputation

**Mitigation**: Short expiry windows, multiple builder submissions

---

## Non-Goals & Future Work

### Non-Goals (V1)

**Not addressed in this specification**:

1. **MEV protection**: No encryption, commit-reveal, or batch auctions
   - Builders can front-run profitable orders
   - Mitigation: Competition reduces single-builder MEV

2. **Privacy enhancements**: Nullifier binding leaks privacy
   - Links off-chain orders to on-chain spends
   - Future: Commitment-based binding, ephemeral notes

3. **Cross-transaction partial fills**: Only single-tx fills supported
   - Unfilled remainder requires new order
   - Future: Nonce-based replay protection enables multi-tx fills

4. **Consensus enforcement**: Surplus splits not verified on-chain
   - Enforcement via wallets + reputation + optional bonds
   - Future: ZK proofs of correct surplus routing (high complexity)

5. **Canonical clearing policy**: No standardized price discovery
   - Each builder determines clearing prices independently
   - Future: Uniform clearing price specifications

### Future Work (V2+)

**Privacy V2**:
- Remove nullifier binding
- Use commitment-based order identification
- Adapter signatures over sighash fragments
- Threshold encryption for orderflow

**Replay V2**:
- Explicit nonce registry (on-chain or bulletin board)
- Enables cross-transaction partial fills
- Better cancellation semantics

**MEV Protection**:
- Threshold encryption (orders decrypted at execution time)
- Commit-reveal schemes (adds latency)
- Batch auctions with uniform clearing price
- Time-weighted average price (TWAP) mechanisms

**On-Chain Enforcement**:
- ZK proofs of correct surplus routing
- Slashing conditions in consensus
- Builder registration and bonding in consensus

**Standardized Clearing**:
- Canonical price discovery algorithms
- Published clearing price commitments
- Verifiable execution (proofs of correct matching)

---

## Implementation Checklist

### For Wallet Developers

- [ ] Implement order creation UI (input/output selection, surplus split)
- [ ] Implement order signing (`sign_order` with proper domain separation)
- [ ] Implement transaction verification (`verify_transaction_against_order`)
- [ ] **Critical**: Reject transactions that violate order terms
- [ ] Display surplus split to user before signing order
- [ ] Handle partial fill scaling (if `partial_fill = true`)
- [ ] Implement expiry checking
- [ ] Support order cancellation (publish cancellation messages)
- [ ] Warn users about privacy tradeoff (nullifier binding)

### For Builder Developers

- [ ] Implement order parsing and signature verification
- [ ] Implement matching algorithm (price discovery)
- [ ] Calculate surplus splits correctly (deterministic rounding)
- [ ] Construct V6 swap transactions with correct outputs
- [ ] Support partial fills with proportional scaling
- [ ] Enforce `min_fill_amount` constraint
- [ ] Check expiry before including orders
- [ ] **Optional**: Publish order hashes for transparency
- [ ] **Optional**: Implement bonding/slashing for reputation
- [ ] **Optional**: Support exact output mode (canonical clearing)

### For Verifiers / Auditors

- [ ] Monitor public orderflow (if available)
- [ ] Compare transaction outputs vs. published orders
- [ ] Detect and publish proofs of misbehavior
- [ ] Track builder reputation scores
- [ ] Verify surplus split calculations
- [ ] Check for front-running patterns

---

## Appendix A: Treasury Address Derivation

```rust
const TREASURY_SEED: &[u8] = b"Zcash_ZSA_Treasury_v1";

fn derive_treasury_address(network_id: u32) -> OrchardAddress {
    let network = match network_id {
        0x00000000 => Network::Mainnet,
        0x00000001 => Network::Testnet,
        _ => panic!("Unknown network_id"),
    };

    // Deterministic derivation from seed
    let fvk = derive_fvk_from_seed(TREASURY_SEED, network);
    fvk.default_address()
}
```

**Properties**:
- Same treasury address for all users (deterministic)
- Network-specific (mainnet ≠ testnet)
- Anyone can verify correctness
- **Governance required**: Who controls treasury spending key?

---

## Appendix B: Test Vectors

### Example 1: Simple Swap Order

```
Input: 100 AAA
Output: ≥50 BBB
Builder: 50%
Treasury: 50%

Order signature: <64 bytes>
Network: Mainnet (0x00000000)
Expiry: Block 1000000
Partial fill: false
Min fill: 0

Encoded: <246 bytes>
```

### Example 2: Partial Fill Order

```
Input: 1000 USDC
Output: ≥800 DAI
Builder: 30%
Treasury: 70%

Order signature: <64 bytes>
Network: Testnet (0x00000001)
Expiry: Block 500000
Partial fill: true
Min fill: 100 USDC

Builder fills 600 USDC:
  Scaled min: 480 DAI
  Clears at: 500 DAI
  Surplus: 20 DAI
  Builder gets: floor(20 × 0.30) = 6 DAI
  Treasury gets: 20 - 6 = 14 DAI
  Change to user: 400 USDC
```

---

## Appendix C: Security Considerations

### Critical: Wallet Verification

**If wallets do not verify transactions, the entire protocol fails.**

Wallets must:
1. Parse V6 transaction structure
2. Extract all outputs (user, builder, treasury)
3. Verify constraints match signed order
4. **Reject** if any mismatch

**Test**: Wallets should reject transactions where:
- User receives less than `min_output`
- Surplus split is incorrect
- Wrong asset type
- Wrong recipient address
- Expired order

### Builder Registration

**V1**: Permissionless, no registration required

**Optional enhancement**: Off-chain registry with:
- Builder public keys
- Service URLs
- Published fee schedules
- Reputation scores

### Bond Amount Sizing

**Example bond**: 1,000 USDx per builder

**Rationale**:
- Large enough to deter misbehavior
- Small enough to allow new builders to enter
- Should exceed typical single-order surplus value

**Slashing**: 100% of bond for proven violation

### Governance Requirements

**Treasury key management**:
- Who holds treasury spending key?
- Multisig threshold?
- Distribution policy?
- Burn vs. redistribute?

**Builder bonds**:
- Who adjudicates slashing?
- Proof requirements?
- Appeal process?

**Must be specified before mainnet deployment.**

---

## References

- **ZIP-226**: Transfer and Burn of Zcash Shielded Assets
- **ZIP-227**: Issuance of Zcash Shielded Assets
- **ZIP-230**: Transaction V6 Format
- **ZIP-244**: Transaction Identifier Non-Malleability
- **ZIP-246**: Digests for the Version 6 Transaction Format
- **Orchard Book**: https://github.com/zcash/orchard/blob/main/book/src/SUMMARY.md
