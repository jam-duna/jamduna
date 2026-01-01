# JAM Builder Separation via Authorizers

**Version**: 3.2 | **Status**: Design Specification | **Updated**: 2025-12-19

## Executive Summary

JAM's builder separation uses **authorizers and commit-reveal auctions**, enforced by guarantors executing PVM code—eliminating trusted relays.

**Core mechanism**:
1. Builders submit bid commitments to auction contract (commit-reveal with 32-byte salt)
2. Top 80 revealed bids populate `AuthorizationQueue[c][0..79]` (refreshed every 8 minutes)
3. At timeslot `t`, slot `t % 80` migrates to `AuthorizationsPool[c]`
4. Builders create work packages with explicit `AuthorizerHash` field matching pool entry
5. Guarantors verify `AuthorizerHash` exists in pool, then execute authorizer to validate ECDSA signature
6. Valid work packages consume authorization; payment already settled during auction finalization

**Key properties**: No relay, trustless (consensus-level enforcement), core-sharded (341 parallel auctions), upfront payment with automatic refunds.

---

## 1. Core Concept Mapping

| Ethereum PBS | JAM Equivalent |
|--------------|----------------|
| Block | Work package for specific core |
| Builder | Per-core work package builder |
| Relay | ❌ None—guarantors enforce via authorizer |
| Proposer | ❌ None—builders submit directly to guarantor mempool |
| Bid | Commitment in `AuthorizationQueue[c]` |
| Bid Reveal | `configuration_blob` in work package |
| Auction Enforcement | Authorizer PVM executed by guarantors |

---

## 2. BAL → Core Routing

```
core_index = Blake2b(BAL_hash || epoch_entropy) mod NumCores
```

- `epoch_entropy`: From JAM `EpochMark.Entropy` (η₁'), unpredictable until epoch boundary
- Reshuffles each epoch; prevents grinding for "easy" cores
- Grace period (2 timeslots) at epoch transition for builders to migrate orderflow

---

## 3. Auction Contract (EVM)

**Complete Implementation**: See [`services/evm/contracts/auction.sol`](../contracts/auction.sol)

The production contract (569 lines) implements a secure commit-reveal auction with the following features:

### 3.1 Core Mechanism

**Timeline per 8-minute cycle**:
- **T-8min to T-2min (Commit)**: `submitBid{value: payment}(core, cycle, commitment)`
- **T-2min to T=0 (Reveal)**: `revealBid(core, cycle, index, builder, declaredValue, salt)`
- **T=0 (Finalize)**: `finalizeQueue()` during accumulate—ranks bids, settles payments, calls ASSIGN

**Configuration Blob** (74 bytes, all big-endian):
```
builder[20] || declared_value[16] || salt[32] || core[2] || cycle[4]
```

**Commitment**: `Blake2b(AUTHORIZER_CODE_HASH || configuration_blob)`

**Delegation Model**: `submitter` (who commits and pays) may differ from `builder` (who signs work packages). This enables bidding proxies and funding accounts.

### 3.2 Security Features

**Fund Safety**:
- ✅ Refunds to **submitter** (not builder) to prevent fund loss
- ✅ Pull payment pattern with `withdrawRefund()` (prevents DoS)
- ✅ Force-refund unrevealed bids during `cleanupCycle()` (prevents burn)
- ✅ Uses `.call()` not `.transfer()` (handles contract recipients)

**Re-entrancy Protection**:
- ✅ `nonReentrant` modifier on all withdrawal functions
- ✅ CEI (Checks-Effects-Interactions) pattern throughout
- ✅ Re-finalization prevention with `finalized` mapping

**Gas DoS Protection**:
- ✅ Max 500 bids per core/cycle (`MAX_BIDS_PER_CORE_CYCLE`)
- ✅ Only current + next cycle allowed (prevents far-future storage bloat)
- ✅ Alternative `selectTop80()` with O(n×80) predictable gas cost

**Timing & Lifecycle**:
- ✅ Deterministic genesis-based timing (no manual `startCycle()` needed)
- ✅ Cycle-ended check before finalization (prevents premature execution)
- ✅ Bounds checking with custom errors (no generic panics)
- ✅ 1-day grace period before `cleanupCycle()` deletion

**Deployment Safety**:
- ✅ Constructor validates treasury and accumulate caller addresses
- ✅ Blake2b reverts with `Blake2bNotImplemented()` until precompile integrated

### 3.3 Key Functions

| Function | Purpose | Access Control |
|----------|---------|----------------|
| `submitBid(core, cycle, commitment)` | Commit phase bid submission | Anyone with payment ≥ BASE_FEE |
| `revealBid(core, cycle, idx, builder, value, salt)` | Reveal phase commitment opening | Only original submitter |
| `finalizeQueue(core, cycle)` | Rank bids, settle payments, call ASSIGN | Only accumulate caller |
| `withdrawRefund()` | Pull refunds for losers/overpayments | Anyone with pending refund |
| `withdrawUnrevealedBid(core, cycle, idx)` | Recover unrevealed bid payment | Only original submitter |
| `cleanupCycle(core, cycle)` | Delete old data after 1 day | Anyone (saves storage) |

### 3.4 Auction Economics

**Clearing Price**: `max(BASE_FEE, 80th_highest_bid)`

**Settlement**:
- Winners pay clearing price (uniform price auction)
- Losers get full refund
- Overpayments refunded to submitter via pull pattern
- Treasury receives `numWinners × clearingPrice`

**NULL Slots**:
- If <80 bids revealed, remaining slots = `bytes32(0)` (permissionless)
- NULL authorizations follow same queue→pool lifecycle
- Any builder can use NULL slots without payment

**Edge Cases**:
- No bids: All 80 slots = `bytes32(0)`, fully permissionless cycle
- <80 bids: All win at `max(BASE_FEE, lowest_bid)`, rest NULL
- ≥80 bids: Top 80 win at 80th price, losers refunded

---

## 4. Configuration Blob

```rust
struct ConfigurationBlob {
    builder: [u8; 20],        // EVM address, big-endian (ABI-native)
    declared_value: u128,     // Big-endian (ABI-native, 16 bytes)
    salt: [u8; 32],           // Random bytes
    core: u16,                // Big-endian (ABI-native)
    cycle: u32,               // Big-endian (ABI-native)
}  // Total: 74 bytes
```

**Serialization**: `abi.encodePacked(builder, declaredValue, salt, core, cycle)` — all integers are **big-endian** per Solidity ABI encoding. Rust serialization must match.

**Privacy**: Commitment fully hides bid until reveal (salt prevents grinding). No privacy after reveal phase.

---

## 5. Work Package Structure

```go
type WorkPackage struct {
    // Authorization fields
    AuthCodeHost          uint32
    AuthorizationCodeHash Hash      // Must equal AUTHORIZER_CODE_HASH for non-NULL
    AuthorizerHash        Hash      // Explicit 32-byte hash; must exist in pool
    ConfigurationBlob     []byte    // 74 bytes for non-NULL; empty for NULL
    AuthorizationToken    []byte    // ECDSA signature (65 bytes); empty for NULL
    
    // Standard fields
    RefineContext         RefineContext
    WorkItems             []WorkItem
}
```

**Authorization hash relationship** (conditional):
- **Non-NULL**: `AuthorizerHash = Blake2b(AuthorizationCodeHash || ConfigurationBlob)`
- **NULL**: `AuthorizerHash = ZeroHash`; `AuthorizationCodeHash` and `ConfigurationBlob` are ignored

**Work package hash** (for signing):
```
WORK_PACKAGE_HASH = Blake2b(Encode(WorkPackage) with AuthorizationToken = empty)
```
The canonical encoding excludes the signature field (or treats it as empty bytes). Builders sign this hash; the host function `fetch(WORK_PACKAGE_HASH)` returns the same value.

---

## 6. Authorization Queue & Pool

### 6.1 Data Structures

```rust
AuthorizationQueue: [341 cores][80 slots]Hash  // ~873KB total
AuthorizationsPool: [341 cores]Vec<Hash>       // Dynamic, bounded by MaxAuthorizationPoolItems
```

### 6.2 Queue Dynamics

1. **ASSIGN** (accumulate): Populates `AuthorizationQueue[c][0..79]` with 80 authorization hashes
2. **Migration** (every timeslot): `AuthorizationQueue[c][t % 80]` → `AuthorizationsPool[c]`
3. **Consumption** (guarantee): Remove matching hash from pool when work package guaranteed
4. **Rotation**: 80 slots × 6s = 8 minutes per cycle

```go
// Queue → Pool migration (authorizations.go)
func (s *StateDB) AddNewPoolFromQueue() {
    jamState := s.GetJamState()
    modSlot := s.Block.TimeSlot() % 80
    
    for core := range jamState.AuthorizationsPool {
        // Get authorization hash from queue slot
        newAuth := jamState.AuthorizationQueue[core][modSlot]
        
        // Append to pool
        pool := jamState.AuthorizationsPool[core]
        pool = append(pool, newAuth)
        
        // Enforce pool size limit (FIFO eviction)
        if len(pool) > MaxAuthorizationPoolItems {
            pool = pool[len(pool)-MaxAuthorizationPoolItems:]
        }
        
        // Write back
        jamState.AuthorizationsPool[core] = pool
    }
}

// Pool consumption (authorizations.go)
func (s *StateDB) RemoveAuthorizationFromPool() {
    jamState := s.GetJamState()
    
    for _, eg := range s.Block.Extrinsic.Guarantees {
        core := eg.Report.CoreIndex
        pool := jamState.AuthorizationsPool[core]
        
        // Remove first matching hash (handles duplicates including ZeroHash)
        for i, h := range pool {
            if h == eg.Report.AuthorizerHash {
                pool = append(pool[:i], pool[i+1:]...)
                break
            }
        }
        
        jamState.AuthorizationsPool[core] = pool
    }
}
```

**ZeroHash duplicates**: The pool may contain multiple `ZeroHash` entries (one per unfilled queue slot that migrated). Each guarantee consumes exactly one instance via first-match removal.

---

## 7. Guarantor Validation

```go
var (
    ZeroHash              = common.Hash{}
    AUTHORIZER_CODE_HASH  = common.HexToHash("0x...")  // Deployed authorizer code hash
    
    ErrAuthorizationNotInPool = errors.New("authorization hash not in pool")
    ErrWrongAuthorizer        = errors.New("wrong authorizer code hash")
    ErrAuthorizationFailed    = errors.New("authorizer execution failed")
    ErrHashMismatch           = errors.New("authorizer hash does not match computed value")
)

func validateWorkPackage(wp WorkPackage, core uint16) error {
    // 1. Check AuthorizerHash exists in pool (using explicit WP field)
    if !AuthorizationsPool[core].contains(wp.AuthorizerHash) {
        return ErrAuthorizationNotInPool
    }
    
    // 2. NULL_AUTHORIZER: zero hash in pool means permissionless slot
    if wp.AuthorizerHash == ZeroHash {
        // Skip authorizer execution and signature check
        // Any builder can use this slot without payment
        return nil
    }
    
    // 3. Verify correct authorizer code hash (prevents pointing to different authorizer)
    if wp.AuthorizationCodeHash != AUTHORIZER_CODE_HASH {
        return ErrWrongAuthorizer
    }
    
    // 4. Sanity check: verify AuthorizerHash matches computed value
    computedHash := Blake2b(wp.AuthorizationCodeHash, wp.ConfigurationBlob)
    if wp.AuthorizerHash != computedHash {
        return ErrHashMismatch
    }
    
    // 5. Execute authorizer PVM (verifies ECDSA signature)
    result := ExecuteAuthorization(wp, core)
    if result != Ok {
        return ErrAuthorizationFailed
    }
    
    return nil
}
```

**Key points**:
- Pool lookup uses `wp.AuthorizerHash` directly (explicit field)
- NULL check is `wp.AuthorizerHash == ZeroHash` (not computed Blake2b)
- Guard ensures `wp.AuthorizationCodeHash == AUTHORIZER_CODE_HASH` for non-NULL
- Sanity check verifies hash consistency for non-NULL only

---

## 8. Authorizer PVM Implementation

**Complete Implementation**: See [`services/auth_copy/src/main.rs`](../../auth_copy/src/main.rs)

The authorizer PVM validates ECDSA signatures over work package hashes. Key features:

**Signature Verification Flow**:
1. Parse core from JAM codec input: `Encode(u16)` (exactly 2 bytes LE)
2. Fetch 74-byte configuration blob: `builder[20] || declared_value[16] || salt[32] || core[2] || cycle[4]`
3. Fetch 65-byte authorization token (ECDSA signature: `r[32] || s[32] || v[1]`)
4. Fetch 32-byte work package hash (Blake2b of encoded WP with `AuthorizationToken=empty`)
5. Recover signer from signature using secp256k1 (Ethereum ECDSA)
6. Verify recovered address matches builder address in config blob (first 20 bytes)
7. Verify core in config blob matches core argument (defense-in-depth)

**Entry Point** (matches JAM ABI): `is_authorized(start_address, length) -> (u64, u64)`
- **Symbol**: `is_authorized` (PVM export, called by guarantor during work package validation)
- **Input Format**: JAM codec `u16` core index (LE, exactly 2 bytes for cores 0..340)
  - JAM passes: `types.Encode(uint16(core))` → `E_l(core, 2)` → 2 bytes LE
  - Authorizer validates: `length == 2` then parses `u16::from_le_bytes([lo, hi])`
  - Examples: core=0 → `[0x00, 0x00]`, core=256 → `[0x00, 0x01]`, core=340 → `[0x54, 0x01]`
- **Returns**: `(FIRST_READABLE_ADDRESS, 0)` on success, `(FIRST_READABLE_ADDRESS, 1)` on failure

**Host Function Usage** (actual JAM ABI, not simplified spec):
- Actual signature: `fetch(o: u64, f: u64, l: u64, d_type: u64, index_0: u64, index_1: u64) -> u64`
- Usage with exact length validation:
  - `fetch(ptr, 0, 32, 0, 0, 0)` → WORK_PACKAGE_HASH (Blake2b of WP with AuthorizationToken=empty)
  - `fetch(ptr, 0, 74, 8, 0, 0)` → CONFIG_BLOB (74 bytes)
  - `fetch(ptr, 0, 65, 9, 0, 0)` → AUTH_TOKEN (65 bytes: r[32] || s[32] || v[1])

**Signature Format** (strict Ethereum ECDSA):
- **v value**: Accepts 0, 1 (raw) or 27, 28 (Ethereum convention), normalized to 0/1
- **Address derivation**: `keccak256(pubkey[1..65])[12..32]` (Ethereum standard, not Blake2b)
- **Defense-in-depth**:
  - Validates core field in config blob (bytes 68-69 BE) matches core argument
  - Parses cycle field (bytes 70-73 BE) for format validation
  - TODO: Add cycle validation when guarantor provides expected cycle

**Implementation Notes**:
- Spec shows simplified pseudocode - actual PVM ABI uses `(start_address, length) -> (u64, u64)` for all exports
- Heap allocation: 64KB (sufficient for signature verification)

**Gas Budget**:
- **Allocation**: 50,000,000 gas
- **Estimated usage**: 52,000–101,000 gas (secp256k1 recovery dominates at 50k–100k)
- **Optimization**: Propose secp256k1 as JAM host function (~3,000 gas)

**Build**: `make auth_copy` produces `auth_copy.pvm` and `auth_copy_blob.pvm` (1.1M each)

---

## 9. NULL_AUTHORIZER Specification

**Value**: `bytes32(0)` / `ZeroHash` (all zeros)

**Queue population**: When auction has <80 bids, remaining slots filled with `bytes32(0)`

**Lifecycle**: NULL slots follow the same lifecycle as normal authorizations—permissionless capacity is available only after the zero sentinel has migrated from queue to pool (one per timeslot).

**Canonical NULL work package encoding**:
```go
wp := WorkPackage{
    AuthorizationCodeHash: ZeroHash,       // Canonical: zero
    AuthorizerHash:        ZeroHash,       // Must match pool entry
    ConfigurationBlob:     []byte{},       // Canonical: empty
    AuthorizationToken:    []byte{},       // Canonical: empty (no signature)
    // ... work items
}
```

All NULL work packages MUST use this canonical encoding for consistency across builders, indexers, and tooling.

**Guarantor handling**: When `wp.AuthorizerHash == ZeroHash` and pool contains zero hash:
- Skip authorizer execution
- Skip signature verification  
- Allow work package from any builder
- No payment required

---

## 10. Builder Flow

### 10.1 Auction Winners (Non-NULL Slots)

1. **Commit phase**: Submit `commitment = Blake2b(AUTHORIZER_CODE_HASH || config_blob)`
2. **Reveal phase**: Reveal `(builder, declaredValue, salt)`
3. **Monitor pool**: Wait for `commitment` to appear in `AuthorizationsPool[core]`
4. **Construct work package**:
   ```go
   configBlob := serializeBigEndian(builder, declaredValue, salt, core, cycle)
   authHash := Blake2b(AUTHORIZER_CODE_HASH, configBlob)
   
   wp := WorkPackage{
       AuthorizationCodeHash: AUTHORIZER_CODE_HASH,
       AuthorizerHash:        authHash,
       ConfigurationBlob:     configBlob,
       AuthorizationToken:    []byte{},  // Placeholder
       // ... work items
   }
   
   // Sign: WORK_PACKAGE_HASH is Blake2b(Encode(wp) with AuthorizationToken = empty)
   wpHash := Blake2b(Encode(wp))
   wp.AuthorizationToken = secp256k1_sign(wpHash, builderPrivkey)
   ```
5. **Submit**: Send to guarantor mempool

### 10.2 Opportunistic Builders (NULL Slots)

1. **Monitor pool**: Watch for `ZeroHash` entries in `AuthorizationsPool[core]`
2. **Construct work package** (canonical encoding):
   ```go
   wp := WorkPackage{
       AuthorizationCodeHash: ZeroHash,
       AuthorizerHash:        ZeroHash,
       ConfigurationBlob:     []byte{},
       AuthorizationToken:    []byte{},
       // ... work items
   }
   ```
3. **Submit**: Send to guarantor mempool (no signature needed)

---

## 11. Security Considerations

| Attack | Mitigation |
|--------|------------|
| Pool exhaustion | Future policy: slash authorization service if consistently unused (mechanism TBD) |
| Payment evasion | Upfront payment, held in escrow |
| Guarantor collusion | Slashing for invalid guarantees |
| Queue griefing | Authorization marketplace enables service replacement |
| Authorization theft | ECDSA signature over wp_hash proves builder ownership |
| Wrong authorizer | Guard: `wp.AuthorizationCodeHash == AUTHORIZER_CODE_HASH` |
| Time-bandit | JAM finality prevents deep reorgs; 1-slot residual risk |
| Cross-core sandwich | Deterministic BAL routing limits; atomic cross-core MEV not yet supported |
| Builder collusion | BASE_FEE floor, transparent auction, permissionless entry |
| Early reveal by adversary | Reveal bound to `msg.sender == bid.submitter` |

---

## 12. Open Questions

### Authorization Marketplace

**Problem**: `ASSIGN` requires `AuthQueueServiceID[c]` authorization. How to enable trustless handoff?

**Options**:
- **A) Manager override**: Manager service can reassign `AuthQueueServiceID[c]` (recommended)
- **B) Governance auction**: Manager runs queue auction logic
- **C) Voluntary handoff**: Current controller cooperates (no enforcement)

### Cross-Core Dependencies

**Current**: No cross-core authorization sharing.
**Future**: Enable for atomic cross-core MEV.

### Multi-Authorizer Support

**Current**: Single `AUTHORIZER_CODE_HASH` per auction contract.
**Future**: Auction contract could commit to different authorizer code hashes per slot, enabling heterogeneous authorization schemes.

---

## 13. Comparison with Ethereum PBS

| Aspect | Ethereum PBS | JAM PBS |
|--------|-------------|---------|
| Scope | 1 block/slot | 341 parallel work packages/timeslot |
| Enforcement | Relay reputation | Authorizer PVM (consensus-level) |
| Trust | Semi-trusted relay | Trustless |
| Payment | In-block transfer | Upfront + refunds |
| Privacy | Relay sees bids early | Hidden until reveal |
| Latency | 12s | 6s |
| Slots/cycle | 1 | 27,280 |

---

## Appendix A: Type Definitions

```go
type WorkPackage struct {
    AuthCodeHost          uint32
    AuthorizationCodeHash Hash
    AuthorizerHash        Hash      // Explicit; must match pool entry
    ConfigurationBlob     []byte
    AuthorizationToken    []byte
    RefineContext         RefineContext
    WorkItems             []WorkItem
}

type RefineContext struct {
    CoreIndex      uint16
    TimeSlot       uint32
    ServiceAccount ServiceAccount
    PriorContext   []byte
}

type RefineResult struct {
    WorkOutput       []byte
    GasUsed          uint64
    NewContext       []byte
    ServiceTransfers []Transfer
}

var (
    ZeroHash                  = common.Hash{}
    ErrAuthorizationNotInPool = errors.New("authorization hash not in pool")
    ErrWrongAuthorizer        = errors.New("wrong authorizer code hash")
    ErrAuthorizationFailed    = errors.New("authorizer execution failed")
    ErrHashMismatch           = errors.New("authorizer hash does not match computed value")
)
```

**Note**: The pseudocode snippets throughout this document are simplified for clarity. For production implementations with complete security features, see:
- **Auction Contract**: [`services/evm/contracts/auction.sol`](../contracts/auction.sol) (569 lines)
- **Authorizer PVM**: [`services/auth_copy/src/main.rs`](../../auth_copy/src/main.rs) (156 lines)

---

## Appendix B: Implementation Status

| Component | Location | Status |
|-----------|----------|--------|
| Queue→Pool migration | `authorizations.go:AddNewPoolFromQueue()` | ✅ |
| Pool consumption | `authorizations.go:RemoveAuthorizationFromPool()` | ✅ |
| ASSIGN host function | `hostfunctions.go:hostAssign()` | ✅ |
| Authorizer execution | `authorization.go:ExecuteAuthorization()` | ✅ |
| fetch(0,8,9) | `hostfunctions.go:hostFetch()` | ⚠️ Verify |
| Auction contract | `services/evm/contracts/auction.sol` | ✅ |
| Authorizer PVM | `services/auth_copy/src/main.rs` | ✅ |
| NULL_AUTHORIZER handling | `guarantee.go` | ❌ |
| AuthorizerHash field in WP | `types/workpackage.go` | ⚠️ Verify exists |

---

## Appendix C: System Invariants

1. **Authorization Uniqueness**: At most one work package consumes each authorization per timeslot
2. **Payment Guarantee**: Every non-zero queued authorization has prepaid ≥ clearingPrice
3. **Queue Progression**: `AuthorizationQueue[c][t % 80]` migrates to pool at timeslot `t`
4. **Pool Boundedness**: `|AuthorizationsPool[c]| ≤ MaxAuthorizationPoolItems`
5. **Consumption Finality**: Consumed authorizations removed from pool, cannot be reused
6. **Clearing Price Floor**: `clearingPrice ≥ BASE_FEE` always
7. **Hash Consistency** (non-NULL only): `wp.AuthorizerHash == Blake2b(wp.AuthorizationCodeHash || wp.ConfigurationBlob)`
8. **NULL Canonical Form**: NULL work packages use `AuthorizerHash = ZeroHash`, `AuthorizationCodeHash = ZeroHash`, `ConfigurationBlob = empty`, `AuthorizationToken = empty`

---

## Appendix D: Serialization Reference

**Configuration blob** (74 bytes, all big-endian ABI-native):
| Field | Offset | Size | Encoding |
|-------|--------|------|----------|
| builder | 0 | 20 | bytes (address) |
| declared_value | 20 | 16 | uint128 big-endian |
| salt | 36 | 32 | bytes |
| core | 68 | 2 | uint16 big-endian |
| cycle | 70 | 4 | uint32 big-endian |

**Solidity**: `abi.encodePacked(builder, declaredValue, salt, core, cycle)`

**Rust** (must match):
```rust
fn serialize_config_blob(builder: [u8; 20], declared_value: u128, 
                         salt: [u8; 32], core: u16, cycle: u32) -> [u8; 74] {
    let mut buf = [0u8; 74];
    buf[0..20].copy_from_slice(&builder);
    buf[20..36].copy_from_slice(&declared_value.to_be_bytes());  // big-endian
    buf[36..68].copy_from_slice(&salt);
    buf[68..70].copy_from_slice(&core.to_be_bytes());            // big-endian
    buf[70..74].copy_from_slice(&cycle.to_be_bytes());           // big-endian
    buf
}
```

**Work package hash** (for signing):
```
WORK_PACKAGE_HASH = Blake2b(Encode(WorkPackage) with AuthorizationToken treated as empty)
```
Both builder (when signing) uses host function `fetch` compute this identically.

---

## Next Steps

1. ✅ ~~Write authorizer PVM code in auth_copy~~ - **COMPLETED** ([services/auth_copy/src/main.rs](../../auth_copy/src/main.rs))
2. ✅ ~~Implement auction contract with commit-reveal (bound to submitter)~~ - **COMPLETED** ([services/evm/contracts/auction.sol](../contracts/auction.sol))
3. Verify NULL_AUTHORIZER handling in guarantee.go
4. Verify AuthorizerHash field exists in types/workpackage.go
5. Verify fetch(0,8,9) implementation in hostfunctions.go
