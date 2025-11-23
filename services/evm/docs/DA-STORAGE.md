# DA Storage Specification

> **Implementation Status:** SSR structures and sharded storage are implemented in `services/evm/src/state.rs`.
>
> **Related Documentation:** See [SHARDING.md](SHARDING.md) for the meta-sharding architecture that manages ObjectRef entries at scale.

This specification defines a compact, scalable layout for **Ethereum-style contract storage** in JAM's Data Availability (DA) layer. It uses the generic SSR (Sparse Split Registry) infrastructure from `services/evm/src/da.rs` for adaptive sharding.

## Document Scope

This document covers **contract-level storage sharding** (Layer 2 in the two-level architecture):
- How individual contracts store key→value mappings in sharded DA segments
- SSR (Sparse Split Registry) routing for contract storage
- Storage shard ObjectID format and lifecycle

For **service-level ObjectRef sharding** (Layer 1), see [SHARDING.md](SHARDING.md), which describes:
- How the EVM service manages billions of ObjectRefs across all contracts
- Meta-sharding architecture (1M meta-shards grouping ~1,000 ObjectRefs each)
- Work report capacity analysis and throughput estimates

## Overview

The DA storage system combines:
- 4104-byte segments (with 96-byte metadata header in first segment),
- sharded storage with **8-byte ShardId (prefix56)**,
- a **compact Sparse Split Registry (SSR)** (no child IDs), and
- a **global-depth–based rent** system billed weekly and stored in **SSR header lease metadata**.

The key idea is that:
* JAM State holds a map from ObjectID to JAM DA via ObjectRefs, where the ObjectRefs are versioned.
* JAM DA holds data for the latest versions for every ObjectID.

When you want your contract code or storage, you look in JAM State for the ObjectID => ObjectRef, which is provable against some state root.  Then using that ObjectRef, you find the payload in JAM DA.  

Every Refine exports objects and *potential* ObjectID-ObjectRef writes for Accumulate to write in JAM State.  

Every Accumulate resolves conflicts between multiple refines and writes out the ObjectID-ObjectRef.

When Refine imports state (previous contract storage + code), it takes state witnesses  (proofs of inclusion against an state root) when importing objects. Because Refine verifies these state witnesses, all the data can be trusted and you don't have to trust the builder/guarantor/auditor/validators.  This process is what makes the EVM service secure. 

Summary:  **JAM State** maps `object_id → ObjectRef`. **JAM DA** stores object payload (segments).

```rust
pub struct ObjectRef {
    pub service_id: u32,
    pub work_package_hash: [u8; 32],
    pub index_start: u16,
    pub index_end: u16,
    pub version: u32,
    pub payload_length: u32,
    pub timeslot: Timeslot,
}
```

- **Refine:**
  - `Import(ObjectRef) -> Vec<Vec<u8>>`: Fetches segments from DA
  - `Export(Vec<Vec<u8>>) -> ObjectCandidateWrite`: Creates write intent for accumulate
  - Verifies state witnesses against RefineContext.state_root
- **Accumulate:**
  - `Read(object_id) -> Option<ObjectRef>`: Looks up current version in JAM State
  - `Write(object_id, ObjectRef)`: Writes new version (with conflict resolution)


The scalability of JAM+EVM rests on JAM State being an index of what is held in JAM DA.  The trustlessness of JAM+EVM rests on each service verifying the imports against the Refine Context state root.

## How it Works

Each contract (20 byte addresses) has DA objects referenced by 32-byte `ObjectId`s of different kinds:

```rust
pub enum ObjectKind {
    /// Code object - bytecode storage (kind=0x00)
    Code = 0x00,
    /// Storage shard object (kind=0x01)
    StorageShard = 0x01,
    /// SSR metadata object (kind=0x02)
    SsrMetadata = 0x02,
    /// Receipt object (kind=0x03) - contains full transaction data
    Receipt = 0x03,
    /// Meta-shard object (kind=0x04) - ObjectID→ObjectRef mappings (see SHARDING.md)
    MetaShard = 0x04,
    /// Block object (kind=0x05) - EVM block payload with tx/receipt hashes
    Block = 0x05,
    /// Block metadata object (kind=0x06) - transactions_root, receipts_root, mmr_root
    BlockMetadata = 0x06,
    /// Meta-SSR metadata object (kind=0x07) - routing metadata for meta-shards (see SHARDING.md)
    MetaSsrMetadata = 0x07,
}
pub fn code_object_id(contract: Bytes20) -> ObjectId;
pub fn ssr_object_id(contract: Bytes20) -> ObjectId;
pub fn shard_object_id(contract: Bytes20, shard: ShardId) -> ObjectId;

// Block-related object IDs
pub fn block_number_to_object_id(block_number: u32) -> ObjectId {
    // Pattern: 0xFF repeated 28 times + block_number (4 bytes LE)
    let mut id = [0xFF; 32];
    id[28..32].copy_from_slice(&block_number.to_le_bytes());
    id
}

pub fn block_metadata_object_id(block: &EvmBlockPayload) -> ObjectId {
    // blake2b hash of block header (first HEADER_SIZE=432 bytes)
    blake2b_hash(&block.serialize_header()[0..HEADER_SIZE])
}
```

**Block Witness Pattern**: Block payloads use the `0xFF...FF[block_number]` pattern, which allows automatic detection during refine. When a witness with this pattern is encountered in `PayloadType::Transactions`, the refine process automatically computes block metadata (transactions_root, receipts_root, mmr_root) and exports it as a separate object.

**ObjectKind Usage**:
- **Code (0x00)**: Bytecode for smart contracts
- **StorageShard (0x01)**: Sharded storage entries for contracts (including 0x01 precompile for balances/nonces)
- **SsrMetadata (0x02)**: Sparse Split Registry for contract storage routing
- **Receipt (0x03)**: Transaction receipts with logs
- **MetaShard (0x04)**: Meta-shard segments (ObjectID→ObjectRef mappings) - see [SHARDING.md](SHARDING.md)
- **Block (0x05)**: Complete EVM blocks with tx/receipt hashes (accumulated)
- **BlockMetadata (0x06)**: Block metadata roots (computed in refine from block witnesses)
- **MetaSsrMetadata (0x07)**: Meta-SSR routing metadata - see [SHARDING.md](SHARDING.md)

Every contract has a fixed object id for its code and storage (SSR). The SSR contains an index of shardIDs.

All the data of a contract lives in shard segments.

## Generic SSR Architecture

The storage system uses a **generic Sparse Split Registry (SSR)** implementation from `services/evm/src/da.rs` that works with any entry type:

```rust
// Generic entry trait - works for both storage and meta-sharding
pub trait SSREntry: Clone {
    fn key_hash(&self) -> [u8; 32];      // Routing key
    fn serialized_size() -> usize;        // Fixed entry size
    fn serialize(&self) -> Vec<u8>;
    fn deserialize(data: &[u8]) -> Result<Self, SerializeError>;
}

// Contract storage implements SSREntry with 64-byte entries
impl SSREntry for EvmEntry {
    fn key_hash(&self) -> [u8; 32] { *self.key_h.as_fixed_bytes() }
    fn serialized_size() -> usize { 64 }  // 32-byte key + 32-byte value
    // ...
}

pub type ContractSSR = SSRData<EvmEntry>;
pub type ContractShard = ShardData<EvmEntry>;
```

**Key benefits:**
- Same splitting logic works for contract storage (64-byte entries) and meta-sharding (96-byte entries)
- Type-safe parameterization ensures consistency
- Easy to add new shard types (e.g., account sharding, nonce sharding)

For the meta-sharding use case (ObjectID→ObjectRef mappings), see [SHARDING.md](SHARDING.md).

## Contract Storage Details

The SSR storage segments hold the shards like this:

- **Dense segments**: 63 KV entries per shard segment (64 B per entry + 2 B count + 96 B metadata in first segment).
- **Lazy import/export**: `refine` fetches only touched shards; `accumulate` exports only mutated shards.
- **Shard-based growth**: split only hot shards via local depth (`ld`). Compact SSR entries encode splits.
- **Deterministic addressing**: any node derives the same shard for `(contract, slot)`.
- **Versioned writes**: copy-on-write with `object_id → ObjectRef(version, …)` in JAM State.
- **Predictable economics**: **$0.20/GB/week**, billed every 7 days, charged **at export** if due.
- **Ultra-low gas** for storage ops: SLOAD/SSTORE in the 200–500 gas range.

**Special Case: 0x01 Precompile (Account Metadata)**

Balance and nonce for **all** accounts are stored as regular storage entries in the **0x01 precompile** contract:

| Field | Contract | Storage Key | ObjectKind |
|-------|----------|-------------|-----------|
| Balance | `0x0000...0001` | `keccak256(address)` | StorageShard (0x01) |
| Nonce | `0x0000...0001` | `keccak256(address) + 1` | StorageShard (0x01) |

- The 0x01 precompile uses the **same SSR-based sharded storage** as regular contracts
- Balance/nonce reads/writes are standard SLOAD/SSTORE operations (~200-500 gas)
- ObjectKind::SsrMetadata (0x02) for 0x01's SSR, ObjectKind::StorageShard (0x01) for 0x01's shards
- Rent applies to 0x01's storage just like any contract

> Any canonical layout is fine; pick one network-wide and keep it stable.


### DA Segment Format

Each object stored in JAM DA consists of one or more **4104-byte segments** (`SegmentSize = 4104`).

**Important**: The first segment of every object includes a 96-byte metadata header:
```
[ObjectID(32) || ObjectRef(64) || payload_part0]
```

- Segments 1..N contain the remaining payload data without headers
- Shard data structures (SSRData, ShardData) are serialized into the payload portion
- Total payload available in first segment: 4104 - 96 = 4008 bytes
- Subsequent segments: 4104 bytes each


Storage Entries (inside a shard) are like this:

```rust
struct EvmEntry {
    key_h: H256,    // keccak256(key) - 32-byte slot key hash
    value: H256,    // 32-byte EVM word
}

struct ShardData {
    entries: Vec<EvmEntry>,  // Sorted by key_h, max 63 entries
}
```

| Off  | Size | Field                    |
|-----:|-----:|--------------------------|
| 0x00 |  32  | `key_h`  = keccak256(key) |
| 0x20 |  32  | `value` = 32-byte EVM word |


When serialized into a DA segment payload:
```
[entry_count(2 bytes LE) || EvmEntry₁(64) || EvmEntry₂(64) || ... || EvmEntryₙ(64)]
```

- **First segment capacity:** `(4008 − 2) / 64 = 62` entries (accounting for 96-byte metadata header)
- **Practical capacity:** 63 entries max when considering multi-segment shards
- Entries are **sorted by `key_h`** for binary search
- Count field (u16 LE) enables quick validation and iteration


### ContractStorage

Contract storage works with an 8-byte `ShardId` (prefix56)

```rust
struct ContractStorage {
    ssr: SSRData,                              // Sparse Split Registry
    shards: BTreeMap<ShardId, ShardData>,      // ShardId -> KV entries
}

struct SSRData {
    header: SSRHeader,
    entries: Vec<SSREntry>,  // Sorted exceptions
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct ShardId {
    ld: u8,             // local depth 0..=56
    prefix56: [u8; 7],  // first ld bits of H (MSB-first), rest zero
}

impl PartialOrd for ShardId {
    fn partial_cmp(&self, other: &Self) -> Option<core::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for ShardId {
    fn cmp(&self, other: &Self) -> core::cmp::Ordering {
        self.ld.cmp(&other.ld)
            .then_with(|| self.prefix56.cmp(&other.prefix56))
    }
}

fn take_prefix56(key: &H256) -> [u8; 7] {
    let mut prefix = [0u8; 7];
    prefix.copy_from_slice(&key.as_bytes()[0..7]);
    prefix
}

fn mask_prefix56(prefix: [u8; 7], bits: u8) -> [u8; 7] {
    let mut masked = [0u8; 7];
    let full_bytes = (bits / 8) as usize;
    let remaining_bits = bits % 8;

    // Copy full bytes
    masked[0..full_bytes].copy_from_slice(&prefix[0..full_bytes]);

    // Mask remaining bits
    if remaining_bits > 0 && full_bytes < 7 {
        let mask = 0xFF << (8 - remaining_bits);
        masked[full_bytes] = prefix[full_bytes] & mask;
    }

    masked
}

fn matches_prefix(key_prefix: [u8; 7], entry_prefix: [u8; 7], bits: u8) -> bool {
    let bytes_to_compare = (bits / 8) as usize;
    let remaining_bits = bits % 8;

    // Check full bytes
    if key_prefix[0..bytes_to_compare] != entry_prefix[0..bytes_to_compare] {
        return false;
    }

    // Check remaining bits
    if remaining_bits > 0 {
        let mask = 0xFF << (8 - remaining_bits);
        if (key_prefix[bytes_to_compare] & mask) != (entry_prefix[bytes_to_compare] & mask) {
            return false;
        }
    }

    true
}
```


The SSR has Compact Entries + **Lease Metadata in Header**

```rust
struct SSRHeader {
    global_depth: u8,    // g in 0..=56
    entry_count: u8,     // number of SSR entries
    total_keys: u32,     // total KV pairs across all shards
    owner: [u8; 20],     // contract address
    version: u32,        // increments on update
}
```

**Note:** Current implementation uses a minimal 28-byte header. The full DA spec (64 bytes) adds:
- `last_rent_paid_epoch: u64` — epoch of last rent collection
- `rent_balance: u64` — optional prepaid credits (tokens)
- `reserved: [u8; 24]` — future use

Header is kept in a single SSR segment together with the SSR entries list (first segment includes 96-byte DA metadata).

Compact SSR Entry (9 bytes as implemented)

```rust
#[derive(Clone, Copy)]
struct SSREntry {
    d: u8,              // Split point (prefix bit position)
    ld: u8,             // Local depth at this prefix
    prefix56: [u8; 7],  // 56-bit routing prefix
}
```

| Off  | Size | Field                                            |
|-----:|-----:|--------------------------------------------------|
| 0x00 |  1   | `d`  — split point (≥ `g`, < 56)                 |
| 0x01 |  1   | `ld` — local depth after split (**ld > d**, ≤56) |
| 0x02 |  7   | `prefix56` — first `d` bits (zeros beyond `d`)   |

- **Sorted by** `(d, prefix56)` (memcmp).
- **Capacity (with minimal header):** `(4008 − 28) / 9 = 442` entries (28 B header + 442 × 9 B = 4006 B, fits in first segment after metadata).

> The SSR object uses the standard segment format with 96-byte metadata in the first segment. Remaining payload space (4008 bytes) holds the header and entries.

An entry `{ d, ld, prefix56 }` means: **“All keys whose first `d` bits equal `prefix56` are governed at depth `ld`.”** No child IDs are stored; children arise implicitly from `ld` and `H` during lookup.

Lookup with Compact SSR:

```rust
pub fn resolve_shard_id(
    H: [u8; 32],
    g: u8,
    ssr_lookup: impl Fn(u8, &[u8;7]) -> Option<(u8,u8,[u8;7])>,
) -> Result<ShardId, ResolveError> {
    const MAX_ITERS: usize = 8;
    let mut depth = g as usize;
    for _ in 0..MAX_ITERS {
        let pref = take_prefix56(H, depth);
        if let Some((_d, ld, _p)) = ssr_lookup(depth as u8, &pref) {
            if ld as usize <= depth || ld > 56 { return Err(ResolveError::InvalidSSREntry); }
            depth = ld as usize;
            continue;
        }
        return Ok(ShardId { ld: depth as u8, prefix56: take_prefix56(H, depth) });
    }
    Err(ResolveError::SSRLoopDetected)
}
```

## Detailed Exception Example

- **Global depth:** `g = 4`
- **SSR entry:** `{ d=4, ld=6, prefix56 = 1011 0000 … }`

This entry states: all keys with first 4 bits `1011` are now governed at depth 6. Thus that 4-bit parent range maps to four 6-bit shards:
`101100`, `101101`, `101110`, `101111`

During lookup, we start at `g=4`, match the exception, **bump depth to 6**, and the final shard is `ShardId{ ld=6, prefix56 = first 6 bits of H }`.

---

## Shard Growth & Split

The contract storage can grow and split with increases in g:
- **Advance `g → g+1`** when **≥ 70%** of `g` prefixes have split to `g+1` or deeper; require `2^g ≥ 4`.
- **Reduce `g → g−1`** when **< 30%** usage at depth `g` (post deletions).
- Changes are **metadata-only** (header update). Existing shards remain valid.

Key idea:

- **Soft split trigger:** insert that would push **live_count > 34** entries.
- **Action:** split `ld = d` into `ld = d + 1` (or more) by partitioning on bit `d`.



### SSR Example

The SSR tracks which storage shards exist and at what depth:

```
Example: Contract with global_depth=2

Default shards (no SSR entries needed):
- ShardId{ld=2, prefix=00...} → for keys starting with 00
- ShardId{ld=2, prefix=01...} → for keys starting with 01
- ShardId{ld=2, prefix=10...} → for keys starting with 10
- ShardId{ld=2, prefix=11...} → for keys starting with 11

If shard "10" splits to depth 4, add SSR entry:
  SSREntry{d=2, ld=4, prefix56=10000000...}

Now keys starting with "10" use depth 4:
- ShardId{ld=4, prefix=1000...}
- ShardId{ld=4, prefix=1001...}
- ShardId{ld=4, prefix=1010...}
- ShardId{ld=4, prefix=1011...}
```

**Key concepts:**
- `d` = split point (where the exception starts)
- `ld` = local depth (actual depth for this prefix)
- `prefix56` = 7-byte prefix for routing

### Benefits

1. **Scalable** - Storage automatically shards as contracts grow
2. **Efficient** - Only ~9 bytes overhead per split (SSREntry)
3. **DA-Compatible** - Matches JAM's actual storage layout
4. **Demonstrable** - Can visualize shard tree structure
5. **Educational** - Shows SSR mechanics in action


```rust
pub fn split_shard_compact(
    contract: [u8;20],
    parent_id: ShardId,                // typically ld == d
    entries: Vec<EvmEntry>,            // sorted
    target_ld: u8,                     // usually d+1 (≤ 56)
    ssr_upsert: impl Fn(u8,u8,[u8;7]) -> (), // write (d,ld,prefix56)
) -> Result<Vec<(ShardId, Vec<EvmEntry>)>, SplitError> {
    if target_ld <= parent_id.ld || target_ld > 56 { return Err(SplitError::InvalidDepth); }
    let d = parent_id.ld;

    use std::collections::BTreeMap;
    let mut buckets: BTreeMap<[u8;7], Vec<EvmEntry>> = BTreeMap::new();
    for e in entries {
        let pref = take_prefix56(e.key_h, target_ld as usize);
        buckets.entry(pref).or_default().push(e);
    }

    let mut out = Vec::with_capacity(buckets.len());
    for (pref, group) in buckets {
        let id = ShardId { ld: target_ld, prefix56: pref };
        export_shard(contract, id, &group)?;
        out.push((id, group));
    }

    // Insert one compact exception for the parent range
    let parent_pref = take_prefix56(parent_id.prefix56_to_hash(), d as usize);
    ssr_upsert(d, target_ld, parent_pref);
    Ok(out)
}
```

##  Weekly Storage Rent (Global-Depth–Based Pricing)

We have not implemented this yet, but we can use g to price contract storage like this:

- **Rate:** `$0.20 / GB / week` (≈ `$10.43 / GB / year`).
- **Billing:** every **7 days (168 epochs)**.
- **Scope:** code + storage (DA) + on-chain keys (SSR + entries).
- **Charge timing:** **at export** during refine if `elapsed ≥ 168 epochs` since `last_rent_paid_epoch`.
- **Metadata location:** **SSR header** (not per object).

The SSR Header (with Lease Fields) can use these fields:

- `total_keys`
- `last_rent_paid_epoch`
- `rent_balance` (optional prepay; can be ignored if native balance is used)
- `owner` (20-byte contract address)

Footprint & Rent Formulas:

```rust
pub const RENT_USD_PER_GB_PER_WEEK: f64 = 0.20;
pub const EPOCHS_PER_WEEK: u64 = 24 * 7;       // 168 epochs
pub const GB: u64 = 1_073_741_824;             // 1024^3 bytes

pub struct StorageFootprint {
    pub on_chain_bytes: u64,  // SSR (4KB) + total_keys * 64
    pub da_bytes: u64,        // DA segments (upper bound)
    pub total_bytes: u64,
}

pub fn calculate_storage_footprint(header: &SSRHeader, code_segments: usize) -> StorageFootprint {
    let g = header.global_depth.min(16);              // cap for upper-bound estimate
    let ssr_size_bytes = 4104u64;                     // 1 segment for header+entries
    let key_value_bytes = header.total_keys as u64 * 64u64;
    let on_chain_bytes = ssr_size_bytes + key_value_bytes;

    let max_shards = 1u64 << g;                       // at depth g
    let segment_size = 4104u64;
    let da_bytes = (max_shards + code_segments as u64) * segment_size;

    StorageFootprint { on_chain_bytes, da_bytes, total_bytes: on_chain_bytes + da_bytes }
}

pub fn calculate_weekly_rent(footprint: StorageFootprint) -> f64 {
    let gb_fraction = footprint.total_bytes as f64 / GB as f64;
    gb_fraction * RENT_USD_PER_GB_PER_WEEK   // tokens ≈ USD here
}

pub fn is_rent_due(header: &SSRHeader, current_epoch: u64) -> bool {
    current_epoch.saturating_sub(header.last_rent_paid_epoch) >= EPOCHS_PER_WEEK
}
```

We can charge rent on export:


```rust
pub fn charge_rent_if_due(
    contract: [u8;20],
    header: &mut SSRHeader,
    code_segments: usize,
    current_epoch: u64,
) -> Result<f64, RentError> {
    if !is_rent_due(header, current_epoch) {
        return Ok(0.0);
    }
    let fp = calculate_storage_footprint(header, code_segments);
    let due = calculate_weekly_rent(fp);

    let mut bal = balances::balance_of(contract);
    if bal < due {
        return Err(RentError::InsufficientBalance { required: due, available: bal });
    }

    bal -= due;
    balances::set_balance(contract, bal);
    balances::credit_rent_collector(due);
    header.last_rent_paid_epoch = current_epoch;
    Ok(due)
}
```

There could be a gas Schedule like this:

```rust
pub const GAS_SLOAD_COLD:   u64 = 200;
pub const GAS_SLOAD_WARM:   u64 = 100;
pub const GAS_SSTORE_NEW:   u64 = 500;
pub const GAS_SSTORE_UPDATE:u64 = 200;
pub const GAS_SSTORE_DELETE:u64 = 200;
```

**Rationale:** rent covers persistence; gas covers computation.



## DA Refresh (Independent of Rent)

Objects are periodically **refreshed** (e.g., every 28 days) to keep DA segments recent. Rent is **not** charged at refresh; only at weekly export when due.


Cost Examples (Order-of-Magnitude):

- Tiny (23.2 KB total): ~$0.004/week; ~$0.22/year
- Small (120 KB total): ~$0.022/week; ~$1.14/year
- Medium (780 KB total): ~$0.145/week; ~$7.54/year
- Large (4.7 MB total): ~$0.88/week; ~$45.8/year
- Huge (50 MB total): ~$9.31/week; ~$484/year
- 1 GB: ~$200/week; ~$10,400/year

> Assumes 1 token ≈ 1 USD and upper-bound storage estimate based on `g`.


- **DoS safety:** exports revert if rent unpaid; inactive data naturally expires via DA refresh horizon.
- **State growth:** deeper `g` ⇒ more shards ⇒ larger DA footprint ⇒ higher rent.
- **Fairness:** code & storage both pay rent; incentives to keep binaries & state lean.



##  Appendix -  Sparse Split Registry (SSR) Tutorial
# Comprehensive Guide to Adaptive Storage Sharding with SSR

This document explains the complete design, implementation, and mechanics of the recursive storage sharding system used by the Majik EVM service.

## Table of Contents

1. [Overview and Motivation](#overview-and-motivation)
2. [Core Concepts](#core-concepts)
3. [Data Structures](#data-structures)
4. [Split Algorithm Details](#split-algorithm-details)
5. [SSR Routing and Lookup](#ssr-routing-and-lookup)
6. [Complete Example Walkthrough](#complete-example-walkthrough)
7. [Implementation Architecture](#implementation-architecture)
8. [Edge Cases and Validations](#edge-cases-and-validations)

---

## Overview and Motivation

### The Problem

JAM blockchain uses **4104-byte Data Availability (DA) segments**. The first segment includes a 96-byte metadata header, leaving 4008 bytes for payload. Storage shards must serialize to fit within these constraints. Each entry is 64 bytes (32B key + 32B value), giving us:

```
First segment payload = 4008 bytes
With 2-byte count field: (4008 - 2) / 64 = 62 entries
Practical max with multi-segment support: 63 entries
```

However, waiting until 63 entries means any additional write pushes us over the limit. We need **headroom for growth**.

### The Solution

**Adaptive binary splitting** with a soft threshold:

```
SPLIT_THRESHOLD = 34 entries (~50% of max capacity)
```

When a shard exceeds 34 entries, we recursively split it into smaller shards using hash prefix bits as the split criterion.

### Key Design Goals

1. **Stay under 4KB**: Guarantee all shards ≤ 63 entries
2. **Allow growth**: Split at 34 to leave headroom
3. **Handle any distribution**: Recursively split until all shards are small enough
4. **Efficient routing**: Use sparse split registry (SSR) to track split points
5. **Parallel access**: Each shard is a separate DA object for concurrent fetching

---

## Core Concepts

### 1. Shard Identifiers

Each shard is identified by `(ld, prefix56)`:

```rust
pub struct ShardId {
    pub ld: u8,           // Local depth - how many hash bits are fixed
    pub prefix56: [u8; 7], // First 56 bits of the hash prefix
}
```

**Example**:
```
ShardId { ld: 0, prefix56: [0x00, 0x00, ...] }  // Root shard (no bits fixed)
ShardId { ld: 1, prefix56: [0x80, 0x00, ...] }  // Bit 0 = 1
ShardId { ld: 3, prefix56: [0xE0, 0x00, ...] }  // Bits 0-2 = 111
```

### 2. Binary Tree Structure

Sharding creates a **binary tree** where:
- **Internal nodes**: Represent split points
- **Leaf nodes**: Contain actual storage entries
- **Tree depth**: Determined by hash distribution (not fixed upfront)

```
                Root (ld=0)
               /           \
        ld=1, bit0=0    ld=1, bit0=1
        /      \         /        \
    ld=2   ld=2     ld=2      ld=2
   bit1=0 bit1=1   bit1=0    bit1=1
```

### 3. SSR (Sparse Split Registry)

SSR tracks **internal split points only**, not leaves. This is the critical invariant:

```
Binary tree with N leaves has N-1 internal nodes
Therefore: N leaf shards → N-1 SSR entries
```

**SSR Entry Format**:
```rust
pub struct SSREntry {
    pub d: u8,            // Parent depth (where split occurred)
    pub ld: u8,           // Child depth (ld = d + 1)
    pub prefix56: [u8; 7], // Routing prefix for this branch
}
```

---

## Data Structures

### ShardData

Contains the actual storage entries:

```rust
pub struct ShardData {
    pub entries: Vec<EvmEntry>,  // Key-value pairs
}

pub struct EvmEntry {
    pub key_h: H256,   // Keccak256 hash of storage key
    pub value: H256,   // 32-byte value
}
```

**Serialization format**:
```
[2 bytes: entry count (u16 little-endian)]
[64 bytes: entry 0 (32B key_h + 32B value)]
[64 bytes: entry 1]
...
```

**Size calculation**:
```
Size = 2 + (entries * 64)
Max size = 2 + (63 * 64) = 4034 bytes
First segment available: 4008 bytes (after 96-byte metadata)
Fits in first segment: 4034 > 4008 (requires 2 segments for max size)
Practical single-segment limit: (4008 - 2) / 64 = 62 entries
```

### SSRHeader

Global metadata for a contract's storage:

```rust
pub struct SSRHeader {
    pub depth: u8,         // Maximum depth in the tree
    pub entry_count: u16,  // Number of SSR entries
    pub total_keys: u32,   // Total storage keys across all shards
    pub version: u8,       // Incremented on each split
}
```

### ContractStorage

Complete storage state for one contract:

```rust
pub struct ContractStorage {
    pub address: H160,
    pub ssr: SSRData,                    // Split metadata
    pub shards: BTreeMap<ShardId, ShardData>,  // In-memory cache
}

pub struct SSRData {
    pub header: SSRHeader,
    pub entries: Vec<SSREntry>,  // Split points
}
```

---

## Split Algorithm Details

### High-Level Flow

```
Transaction updates → Merge with cached shard → Check size
                                                      ↓
                                    If > 34 entries: Recursive split
                                                      ↓
                                    Create N leaf shards (≤34 each)
                                    Create N-1 SSR entries
                                                      ↓
                                    Export as DA objects
```

### Work Queue Algorithm

**Function**: `maybe_split_shard_recursive(shard_id, shard_data)`

**Returns**: `Option<(Vec<(ShardId, ShardData)>, Vec<SSREntry>)>`

**Pseudocode**:
```rust
fn maybe_split_shard_recursive(shard_id, shard_data) {
    if shard_data.entries.len() <= SPLIT_THRESHOLD {
        return None;  // No split needed
    }

    let mut leaf_shards = [];
    let mut ssr_entries = [];
    let mut work_queue = [(shard_id, shard_data)];

    while let Some((current_id, current_data)) = work_queue.pop() {
        if current_data.entries.len() <= SPLIT_THRESHOLD {
            // Base case: small enough
            leaf_shards.push((current_id, current_data));
            continue;
        }

        // Split this shard
        let (left_id, left_data, right_id, right_data, ssr_entry)
            = split_once(current_id, current_data);

        // Record this internal split
        ssr_entries.push(ssr_entry);

        // Recursively process children
        if left_data.entries.len() > SPLIT_THRESHOLD {
            work_queue.push((left_id, left_data));
        } else {
            leaf_shards.push((left_id, left_data));
        }

        if right_data.entries.len() > SPLIT_THRESHOLD {
            work_queue.push((right_id, right_data));
        } else {
            leaf_shards.push((right_id, right_data));
        }
    }

    return Some((leaf_shards, ssr_entries));
}
```

### Binary Split Logic

**Function**: `split_once(shard_id, shard_data)`

**Algorithm**:
1. Use bit at position `shard_id.ld` to partition entries
2. Calculate new shard IDs with updated prefixes
3. Create SSR entry for this split point

**Detailed example**:

```rust
// Input: ShardId { ld: 0, prefix56: [0x00, ...] }, 258 entries

let split_bit = 0;  // Use bit 0 (MSB)
let new_depth = 1;

// Partition entries based on bit 0
for entry in entries {
    let hash = entry.key_h;  // e.g., 0x3A7F... = 0011 1010 0111 1111...
    if get_bit(hash, 0) == 0 {  // Bit 0 = 0
        left_entries.push(entry);
    } else {  // Bit 0 = 1
        right_entries.push(entry);
    }
}

// Calculate child IDs
left_id = ShardId {
    ld: 1,
    prefix56: [0x00, 0x00, ...]  // Bit 0 = 0
};

right_id = ShardId {
    ld: 1,
    prefix56: [0x80, 0x00, ...]  // Bit 0 = 1 (0x80 = 1000 0000)
};

// Create SSR entry
ssr_entry = SSREntry {
    d: 0,   // Split occurred at depth 0
    ld: 1,  // Children are at depth 1
    prefix56: [0x00, 0x00, ...]  // Routing prefix
};

return (left_id, left_data, right_id, right_data, ssr_entry);
```

### Prefix Calculation

When splitting at depth `d`, the right child's prefix has bit `d` set:

```rust
fn calculate_right_prefix(parent_prefix: [u8; 7], depth: u8) -> [u8; 7] {
    let mut result = parent_prefix;
    let byte_idx = (depth / 8) as usize;
    let bit_idx = depth % 8;
    let bit_mask = 0x80 >> bit_idx;  // 0x80 = 10000000

    if byte_idx < 7 {
        result[byte_idx] |= bit_mask;  // Set the bit
    }

    result
}
```

**Example**:
```
Depth 0, Bit 0: mask = 10000000 = 0x80 → prefix[0] |= 0x80
Depth 1, Bit 1: mask = 01000000 = 0x40 → prefix[0] |= 0x40
Depth 2, Bit 2: mask = 00100000 = 0x20 → prefix[0] |= 0x20
Depth 8, Bit 8: mask = 10000000 = 0x80 → prefix[1] |= 0x80
```

---

## SSR Routing and Lookup

### Lookup Algorithm

To find which shard contains a storage key with hash `key_h`:

```rust
fn resolve_shard_id(key_h: H256, ssr: &SSRData) -> ShardId {
    let mut current_ld = 0;
    let mut current_prefix = [0u8; 7];

    // Check each SSR entry to see if we need to descend deeper
    loop {
        let mut found_split = false;

        for entry in &ssr.entries {
            if entry.d == current_ld {
                // Check if key_h matches this split's prefix
                if matches_prefix(key_h, entry.prefix56, entry.d) {
                    // Descend to this child
                    current_ld = entry.ld;
                    current_prefix = entry.prefix56;
                    found_split = true;
                    break;
                }
            }
        }

        if !found_split {
            // No more splits - we're at a leaf
            break;
        }
    }

    ShardId { ld: current_ld, prefix56: current_prefix }
}
```

### Routing Example

**Given**:
- Key hash: `0x3A7F... = 0011 1010 0111 1111...`
- SSR entries from 258→11 split (10 entries)

**Step-by-step**:

```
Start: ld=0, prefix=0x00...

Bit 0 of 0x3A = 0 (0011...)
  ↓ SSR entry: (d=0, ld=1, prefix=0x00...) ✓ matches
  → Descend to ld=1, prefix=0x00...

Bit 1 of 0x3A = 0 (0011...)
  ↓ SSR entry: (d=1, ld=2, prefix=0x00...) ✓ matches
  → Descend to ld=2, prefix=0x00...

Bit 2 of 0x3A = 1 (0011...)
  ↓ SSR entry: (d=2, ld=3, prefix=0x20...) ✓ matches
  → Descend to ld=3, prefix=0x20...

Bit 3 of 0x3A = 1 (0011...)
  ↓ SSR entry: (d=3, ld=4, prefix=0x30...) ✓ matches
  → Descend to ld=4, prefix=0x30...

No more SSR entries at d=4
  → Final shard: ShardId { ld: 4, prefix: 0x30... }
```

---

## Complete Example Walkthrough

### Scenario: Fibonacci(256) Transaction

Transaction writes 258 storage entries (1 fibonacci result per index 0-255, plus 2 overhead entries).

### Initial State

```
Shard: ShardId { ld: 0, prefix56: [0x00, ...] }
Entries: 258 (exceeds SPLIT_THRESHOLD of 34)
```

### Iteration 1: Root Split

```
Split at bit 0 (ld=0 → ld=1)

Partition:
  Bit 0 = 0: 131 entries → Left shard
  Bit 0 = 1: 127 entries → Right shard

Results:
  Left:  ShardId { ld: 1, prefix: 0x00... }, 131 entries (still > 34!)
  Right: ShardId { ld: 1, prefix: 0x80... }, 127 entries (still > 34!)
  SSR entry: (d=0, ld=1, prefix=0x00...)

Work queue: [Left(131), Right(127)]
Leaf shards: []
SSR entries: [1]
```

### Iteration 2: Split Left Child

```
Pop: Left shard (131 entries)
Split at bit 1 (ld=1 → ld=2)

Partition:
  Bit 1 = 0: 63 entries → Left-Left
  Bit 1 = 1: 68 entries → Left-Right

Results:
  LL: ShardId { ld: 2, prefix: 0x00... }, 63 entries (still > 34!)
  LR: ShardId { ld: 2, prefix: 0x40... }, 68 entries (still > 34!)
  SSR entry: (d=1, ld=2, prefix=0x00...)

Work queue: [Right(127), LL(63), LR(68)]
Leaf shards: []
SSR entries: [2]
```

### Iteration 3: Split Right Child

```
Pop: Right shard (127 entries)
Split at bit 1 (ld=1 → ld=2)

Partition:
  Bit 1 = 0: 71 entries → Right-Left
  Bit 1 = 1: 56 entries → Right-Right

Results:
  RL: ShardId { ld: 2, prefix: 0x80... }, 71 entries (still > 34!)
  RR: ShardId { ld: 2, prefix: 0xC0... }, 56 entries (still > 34!)
  SSR entry: (d=1, ld=2, prefix=0x80...)

Work queue: [LL(63), LR(68), RL(71), RR(56)]
Leaf shards: []
SSR entries: [3]
```

### Iterations 4-10: Continue Splitting

After 10 total splits:

```
Final state:
  Leaf shards: 11 shards with sizes:
    [29, 27, 34, 20, 17, 27, 14, 27, 23, 18, 22] entries
  SSR entries: 10 entries (11-1 internal nodes)

All shards ≤ 34 entries ✓
All shards ≤ 63 entries ✓
```

### Binary Tree Diagram

```
                        Root(ld=0, 258)
                       /                \
              L(ld=1, 131)          R(ld=1, 127)
              /         \            /           \
        LL(ld=2,63) LR(ld=2,68) RL(ld=2,71) RR(ld=2,56)
         /    \       /    \      /    \       /    \
    [29] [23] [27] [41]  [37] [34]  [29] [27]
                   / \     / \
               [14][27] [20][17]

Legend:
  [...] = Leaf shard (final result)
  (...) = Internal node (gets split further)
```

### SSR Entries Generated

```rust
[
    SSREntry { d: 0, ld: 1, prefix: 0x00... },  // Root → Left
    SSREntry { d: 1, ld: 2, prefix: 0x00... },  // L → LL
    SSREntry { d: 1, ld: 2, prefix: 0x80... },  // R → RL
    SSREntry { d: 2, ld: 3, prefix: 0x00... },  // LL → LLL
    SSREntry { d: 2, ld: 3, prefix: 0x40... },  // LR → LRL
    SSREntry { d: 2, ld: 3, prefix: 0x80... },  // RL → RLL
    SSREntry { d: 2, ld: 3, prefix: 0xC0... },  // RR → RRL
    SSREntry { d: 3, ld: 4, prefix: 0x80... },  // RLL → RLLL
    SSREntry { d: 3, ld: 4, prefix: 0x00... },  // LLL → LLLL
    SSREntry { d: 3, ld: 4, prefix: 0x40... },  // LRL → LRLL
]
```

### DA Object Exports

**11 Shard Objects**:
```
Object ID: 0x...ff...03c0... (ld=3, prefix=0xC0), payload=1858 bytes (29 entries)
Object ID: 0x...ff...03e0... (ld=3, prefix=0xE0), payload=1730 bytes (27 entries)
Object ID: 0x...ff...03a0... (ld=3, prefix=0xA0), payload=2178 bytes (34 entries)
...
```

**1 SSR Object**:
```
Object ID: 0x...ff...0000000002 (SSR metadata)
Payload: 100 bytes (10 + 9×10)
  Header: 8 bytes (depth=4, entry_count=10, ...)
  Entries: 90 bytes (10 entries × 9 bytes each)
```

---

## Implementation Architecture

### File Organization

**services/evm/src/sharding.rs** (Split Logic)
- `split_once()`: Single binary split
- `maybe_split_shard_recursive()`: Work queue recursive splitter
- `serialize_shard()`: DA format conversion + validation
- `serialize_ssr()`: SSR metadata serialization
- `resolve_shard_id()`: Routing lookup

**services/evm/src/backend.rs** (DA Integration)
- `process_single_shard()`: Entry point for shard processing
- `handle_shard_split_recursive()`: Convert results to write intents
- `handle_normal_shard_write()`: Non-split shard export
- `to_execution_effects()`: Top-level DA export

### Call Flow

```
Transaction execution
    ↓
Overlay updates collected
    ↓
backend::to_execution_effects()
    ↓
For each contract with updates:
    ↓
process_single_shard()
    ↓
Merge overlay + cached shard
    ↓
if entries > 34:
    ↓
maybe_split_shard_recursive()
    → Returns (leaf_shards, ssr_entries)
    ↓
handle_shard_split_recursive()
    → Remove stale parent shard
    → Create write intent for each leaf
    → Update SSR with N-1 entries
    → Cache new shards locally
else:
    ↓
handle_normal_shard_write()
    → Serialize shard
    → Create write intent
    → Cache shard
    ↓
Export all write intents to DA
```

### Emergency Validations

**serialize_shard()** (Last-line defense):
```rust
const MAX_ENTRIES: usize = 63;
if shard.entries.len() > MAX_ENTRIES {
    call_log(0, "FATAL: Shard exceeds 63-entry limit!");
    #[cfg(debug_assertions)]
    panic!("DA segment limit violation");
}
```

**handle_shard_split_recursive()** (Verify results):
```rust
for (i, (_shard_id, shard_data)) in leaf_shards.iter().enumerate() {
    if shard_data.entries.len() > MAX_ENTRIES {
        call_log(0, "FATAL: Leaf shard {} exceeds limit!", i);
        panic!("Recursive split failed to produce valid shards");
    }
}
```

---

## Edge Cases and Validations

### 1. Highly Skewed Distribution

**Problem**: All 129 entries hash to prefixes starting with `0b0...`

**Example**:
```
First split (bit 0):
  Left (bit 0 = 0): 129 entries
  Right (bit 0 = 1): 0 entries
```

**Solution**: Recursive splitting continues on the left branch:
```
Split 1: 129 → [129, 0]
Split 2 (left): 129 → [64, 65]
Split 3 (left-left): 64 → [32, 32]
Split 4 (left-right): 65 → [33, 32]

Result: 4 leaf shards, all ≤ 34 ✓
```

### 2. Stale Parent Shard Removal

**Problem**: After splitting, the original oversized shard must be removed from cache.

**Solution** (backend.rs:813-819):
```rust
// Remove the original unsplit parent shard
{
    let mut storage_shards_mut = self.storage_shards.borrow_mut();
    if let Some(contract_storage) = storage_shards_mut.get_mut(&address) {
        contract_storage.shards.remove(&original_shard_id);
    }
}
```

**Why critical**: Prevents lookups from finding stale 258-entry shard instead of correct leaf shards.

### 3. SSR Invariant Verification

**Invariant**: `ssr_entries.len() == leaf_shards.len() - 1`

**Test validation**:
```rust
assert_eq!(
    ssr_entries.len(),
    leaf_shards.len() - 1,
    "SSR entries should be N-1 for N leaves"
);
```

**Example**:
```
11 leaf shards → 10 SSR entries (11-1) ✓
```

### 4. Empty Right Child

**Problem**: After split, right child may be empty.

**Example**:
```
Split 1 entries, all with bit 0 = 0:
  Left: 1 entry
  Right: 0 entries
```

**Solution**: Both children are added to results (even if empty). Empty shards are valid and serialize to 2 bytes (count=0).

### 5. Maximum Depth

**Theoretical limit**: 56 bits (7 bytes × 8 bits)

**Practical limit**: ~10-15 splits for typical workloads

**Pathological case**: If all 2^56 entries hash to same prefix, we'd hit depth 56 and still have collisions. This is astronomically unlikely with keccak256.

---

## Performance Characteristics

### Time Complexity

**Recursive splitting**:
- **Best case**: O(n) - uniform distribution, minimal splits
- **Average case**: O(n log n) - balanced tree
- **Worst case**: O(n²) - highly skewed, deep tree (very rare)

**Single split**: O(n) to partition entries

**SSR lookup**: O(d) where d = depth (typically 3-5)

### Space Complexity

- **Work queue**: O(d) - depth of tree
- **SSR entries**: O(s) where s = number of splits = N-1
- **Temporary storage**: O(n) for entry copying during split

### Real-World Performance

**From jamtest logs** (258 entries):
- Splits: 10
- Time: ~2ms (includes serialization)
- Final shards: 11
- Max shard size: 34 entries (2178 bytes)
- SSR overhead: 100 bytes

**Scalability**: Handles up to ~2000 entries per shard before splitting becomes expensive (>20 levels deep).

---

## Summary

The recursive sharding implementation provides:

✅ **Correctness**: All shards guaranteed ≤ 63 entries (4KB limit)
✅ **Headroom**: Split at 34 entries to allow growth
✅ **Flexibility**: Handles any hash distribution via recursive splitting
✅ **Efficiency**: O(n log n) typical case, minimal SSR overhead
✅ **Safety**: Emergency validations prevent silent corruption
✅ **Consistency**: Binary tree invariants maintained (N leaves → N-1 SSR entries)

**Key Takeaway**: The work queue pattern ensures we keep splitting until every shard meets the threshold, regardless of how skewed the hash distribution is. This transforms an unbounded-size storage problem into a provably bounded one compatible with JAM's DA constraints.
##  Appendix -  Sparse Split Registry (SSR) Tutorial

## Introduction

This tutorial walks through concrete examples of how the **Sparse Split Registry (SSR)** manages storage shards for Ethereum contracts in JAM. We'll see how shards start simple, grow through splits, and how the compact SSR encoding avoids storing explicit child shard IDs.


## Core Concepts

### What is a Shard?

A **shard** is a DA object containing up to 63 storage key-value pairs for a contract. Each shard serializes to one or two 4104-byte segments (with 96-byte metadata in first segment). Each shard is identified by:

```rust
pub struct ShardId {
    pub ld: u8,           // local depth (0-56)
    pub prefix56: [u8;7], // first ld bits of key hash
}
```

**Key insight**: The shard ID is derived from the first `ld` bits of the key's hash.

### What is the SSR?

The **Sparse Split Registry** is a compact data structure that tracks **exceptions** to the global depth rule. Instead of storing every shard explicitly, it only records where local depth differs from global depth.

---

## Example 1: Starting Simple (Global Depth = 0)

### Initial State

A new contract `0xABCD...` has no storage yet.

```
Global depth (g): 0
SSR entries: [] (empty)
Shards: none
```

### First Write

We write `SSTORE(slot_0, value_0)`:

```
slot_0 → H₀ = keccak256(slot_0) = 0x7A3B2C...
```

**Lookup process**:
1. Start at depth `g=0`
2. Take first 0 bits of H₀ → prefix = `[]` (empty)
3. No SSR entry found
4. **Result**: `ShardId { ld: 0, prefix56: [0,0,0,0,0,0,0] }`

**Shard created**:
```
Shard_0 (ld=0, prefix=[]):
  Entry 1: [H₀: 7A3B2C...] → [value_0]
  Count: 1/63
```

### More Writes

We add 33 more keys. All land in the same root shard:

```
Shard_0 (ld=0, prefix=[]):
  Entry 1:  [H₀] → [value_0]
  Entry 2:  [H₁] → [value_1]
  ...
  Entry 34: [H₃₃] → [value_33]
  Count: 34/63
```

**SSR remains empty** because all keys are in the root shard at depth 0.

---

## Example 2: First Split (Depth 0 → 1)

### Trigger

We insert the 35th entry, exceeding the split threshold (34 entries).

**Action**: Split `Shard_0` at depth 0 → depth 1

### How Split Works

1. **Read** all 35 entries from `Shard_0`
2. **Partition** by bit 0 of each key hash:
   - Bit 0 = 0 → left child (`prefix = 0`)
   - Bit 0 = 1 → right child (`prefix = 1`)

Suppose the distribution is:
- 18 keys have bit 0 = `0`
- 17 keys have bit 0 = `1`

3. **Create new shards**:

```
Shard_L (ld=1, prefix=0):
  18 entries
  
Shard_R (ld=1, prefix=1):
  17 entries
```

4. **Update SSR**:

Since we split from depth 0 to depth 1, we add an entry:

```
SSR Entry 1: { d: 0, ld: 1, prefix56: [0x00, 0, 0, 0, 0, 0, 0] }
```

**What this means**: "All keys that were at depth 0 are now governed at depth 1."

5. **Delete old shard**: `Shard_0` no longer exists

### Updated State

```
Global depth (g): 0 (not advanced yet)
SSR entries: [{ d: 0, ld: 1, prefix: 00000000... }]
Shards: 
  - Shard_L (ld=1, prefix=0): 18 entries
  - Shard_R (ld=1, prefix=1): 17 entries
```

### Lookup After Split

New key arrives: `H_new = 0x9F2A...` (bit 0 = `1`)

**Lookup process**:
1. Start at `g=0`, prefix = `[]`
2. SSR lookup: found entry `{ d=0, ld=1 }`
3. Bump to depth 1
4. Take first 1 bit of H_new = `1`
5. No further SSR entry
6. **Result**: `ShardId { ld: 1, prefix56: [0x80, 0, 0, 0, 0, 0, 0] }` (binary: `1000...`)

The key goes into `Shard_R`.

---

## Example 3: Asymmetric Split (One Child Grows)

### Current State

```
Global depth (g): 0
SSR: [{ d: 0, ld: 1, prefix: 0 }]
Shards:
  - Shard_L (ld=1, prefix=0): 18 entries
  - Shard_R (ld=1, prefix=1): 17 entries
```

### Many Writes to Shard_R

Over time, `Shard_R` receives 18 more writes, reaching 35 entries.

**Trigger**: Split `Shard_R` at depth 1 → depth 2

### Split Process

1. **Read** 35 entries from `Shard_R`
2. **Partition** by bit 1 (second bit) of each hash:
   - Bit pattern `10` → `Shard_R_L` 
   - Bit pattern `11` → `Shard_R_R`

Suppose:
- 19 keys → `10` prefix
- 16 keys → `11` prefix

3. **Create new shards**:

```
Shard_R_L (ld=2, prefix=10):
  19 entries
  
Shard_R_R (ld=2, prefix=11):
  16 entries
```

4. **Update SSR**:

Add entry for the split:

```
SSR Entry 2: { d: 1, ld: 2, prefix56: [0x80, 0, 0, 0, 0, 0, 0] }
```

Binary: `10000000...` (first bit = 1, next bits = 0)

5. **Delete**: `Shard_R` removed

### Updated State

```
Global depth (g): 0
SSR entries:
  1. { d: 0, ld: 1, prefix: 00000000... }
  2. { d: 1, ld: 2, prefix: 10000000... }
  
Shards:
  - Shard_L (ld=1, prefix=0): 18 entries
  - Shard_R_L (ld=2, prefix=10): 19 entries
  - Shard_R_R (ld=2, prefix=11): 16 entries
```

**Important**: `Shard_L` was NOT split. Only the hot shard grew.

### Lookup with Cascading

New key: `H_new = 0xC7...` (binary starts: `11000111...`)

**Lookup process**:
1. Start at `g=0`, prefix = `[]`
2. **SSR lookup at d=0**: found `{ d=0, ld=1 }`
3. Bump to depth 1, prefix = `1` (bit 0 of H_new)
4. **SSR lookup at d=1, prefix=1**: found `{ d=1, ld=2, prefix=10000000 }`
5. Check: does H_new match prefix `10000000`? 
   - First bit = `1` ✓
   - All remaining bits in prefix are 0 (wildcards) ✓
6. Bump to depth 2
7. Take first 2 bits of H_new = `11`
8. No further SSR entry
9. **Result**: `ShardId { ld: 2, prefix56: [0xC0, 0, 0, 0, 0, 0, 0] }` (binary: `11000000...`)

The key goes into `Shard_R_R`.

---

## Example 4: Global Depth Advance

### Current State

After many splits:

```
Global depth (g): 0
SSR entries:
  1. { d: 0, ld: 1, prefix: 00... }
  2. { d: 1, ld: 2, prefix: 10... }
  3. { d: 1, ld: 3, prefix: 00... } (Shard_L was split)
  ...
  
Active shards: 8 (all at ld ≥ 2)
```

### Advance Condition

**Rule**: If ≥70% of possible g-bit prefixes have split beyond g, advance g.

At `g=0`:
- Possible prefixes: `[]` (just one)
- Has it split? Yes (entry 1 shows ld=1)
- **100% ≥ 70%** → advance g

**Action**: Set `g = 1`

### Cleanup SSR

When g advances, we can **remove obsolete entries** that are now redundant:

```
Before:
  1. { d: 0, ld: 1, prefix: 00... } ← d < new g, can remove
  2. { d: 1, ld: 2, prefix: 10... }
  3. { d: 1, ld: 3, prefix: 00... }

After:
  2. { d: 1, ld: 2, prefix: 10... }
  3. { d: 1, ld: 3, prefix: 00... }
```

### Updated State

```
Global depth (g): 1
SSR entries:
  1. { d: 1, ld: 2, prefix: 10000000... }
  2. { d: 1, ld: 3, prefix: 00000000... }
  
Shards: 8 shards at various depths
```

**Benefit**: New lookups start at depth 1 instead of 0, skipping the first cascade step.

---

## Example 5: Deep Split (ld=4 → ld=6)

### Setup

```
Global depth (g): 2
SSR: [{ d: 2, ld: 4, prefix: 1011 0000... }]

Shard_X (ld=4, prefix=1011):
  34 entries (near capacity)
```

### Trigger

35th entry arrives, triggering a split.

**Decision**: Split directly to `ld=6` (skipping ld=5) to reduce future splits.

### Split Process

1. **Read** 35 entries from `Shard_X`
2. **Partition** by first 6 bits:

```
Entries by first 6 bits:
  101100: 9 entries  → Shard_A (ld=6, prefix=101100)
  101101: 8 entries  → Shard_B (ld=6, prefix=101101)
  101110: 10 entries → Shard_C (ld=6, prefix=101110)
  101111: 8 entries  → Shard_D (ld=6, prefix=101111)
```

3. **Update SSR**:

Replace the old entry with a new one:

```
Remove: { d: 2, ld: 4, prefix: 1011 0000... }
Add:    { d: 4, ld: 6, prefix: 1011 0000... }
```

**Why d=4?** Because we're overriding at the parent's depth (4 bits).

4. **Delete**: `Shard_X` removed

### Updated State

```
Global depth (g): 2
SSR: [{ d: 4, ld: 6, prefix: 10110000... }]

Shards:
  - Shard_A (ld=6, prefix=101100): 9 entries
  - Shard_B (ld=6, prefix=101101): 8 entries
  - Shard_C (ld=6, prefix=101110): 10 entries
  - Shard_D (ld=6, prefix=101111): 8 entries
```

### Lookup with Deep Cascade

New key: `H = 0xB7...` (binary: `10110111...`)

**Lookup process**:
1. Start at `g=2`, prefix = `10` (first 2 bits)
2. No SSR entry at d=2, prefix=10
3. Continue at depth 2
4. No SSR entry matches
5. Try depth 3: prefix = `101`
6. No SSR entry matches
7. Try depth 4: prefix = `1011`
8. **SSR lookup at d=4, prefix=1011**: found `{ d=4, ld=6 }`
9. Bump to depth 6
10. Take first 6 bits = `101101`
11. **Result**: `ShardId { ld: 6, prefix56: [0xB4, 0, 0, 0, 0, 0, 0] }` (binary: `10110100...`)

The key goes into `Shard_B`.

---

## Example 6: Understanding Prefix Matching

### SSR Entry Details

Consider: `{ d: 4, ld: 6, prefix56: [0xB0, 0, 0, 0, 0, 0, 0] }`

Binary breakdown:
```
0xB0 = 1011 0000
       ^^^^ ^^^^
       d=4  rest
```

**Meaning**: "If the first 4 bits of a hash are `1011`, use depth 6."

### Matching Examples

| Hash (first 8 bits) | First 4 bits | Matches? | Resulting ld |
|---------------------|--------------|----------|--------------|
| `10110101` | `1011` | ✓ | 6 |
| `10111010` | `1011` | ✓ | 6 |
| `10110000` | `1011` | ✓ | 6 |
| `10100111` | `1010` | ✗ | (continue lookup) |
| `11110101` | `1111` | ✗ | (continue lookup) |

**Key point**: Only the first `d` bits matter. Remaining bits in `prefix56` are zeroed and ignored.

---

## Example 7: Multi-Level Cascade

### Setup

Complex SSR with multiple levels:

```
Global depth (g): 2
SSR entries (sorted by d):
  1. { d: 2, ld: 4, prefix: 1100 0000... }
  2. { d: 4, ld: 5, prefix: 1100 1000... }
  3. { d: 5, ld: 7, prefix: 1100 1100 0000... }
```

### Lookup: H = 0xCF... (binary: `11001111...`)

**Step-by-step**:

1. **Start**: `g=2`, depth=2, prefix=`11`
2. **SSR at d=2, prefix=11**: Check entries...
   - Entry 1: `prefix=1100 0000...` → first 2 bits = `11` ✓
   - **Match!** Set depth=4

3. **Now at depth=4**, prefix=`1100`
4. **SSR at d=4, prefix=1100**: Check entries...
   - Entry 2: `prefix=1100 1000...` → first 4 bits = `1100` ✓
   - **Match!** Set depth=5

5. **Now at depth=5**, prefix=`11001`
6. **SSR at d=5, prefix=11001**: Check entries...
   - Entry 3: `prefix=1100 1100 0...` → first 5 bits = `11001` ✓
   - **Match!** Set depth=7

7. **Now at depth=7**, prefix=`1100111` (7 bits)
8. **SSR at d=7**: No entries found
9. **Result**: `ShardId { ld: 7, prefix56: [0xCE, 0, 0, 0, 0, 0, 0] }` (binary: `11001110...`)

**Three cascades**: 2→4→5→7

---

## Example 8: Why Compact Encoding Wins

### Scenario: 1 Million Storage Slots

After many splits, we have:
- Global depth: `g=8` (256 possible 8-bit prefixes)
- 200 prefixes have split beyond depth 8
- 56 prefixes remain at depth 8

### Storage Comparison

**Old Explicit Child Method**:
```
Each SSR entry: 32 bytes
  - 8 bytes: parent shard ID
  - 8 bytes: child_0 shard ID
  - 8 bytes: child_1 shard ID
  - 8 bytes: metadata

200 exceptions × 32 bytes = 6,400 bytes
```

**New Compact Method**:
```
Each SSR entry: 9 bytes
  - 1 byte: d (split point)
  - 1 byte: ld (local depth)
  - 7 bytes: prefix56

200 exceptions × 9 bytes = 1,800 bytes
```

**Savings**: 72% reduction in SSR size (1,800 bytes vs 6,400 bytes)

**Plus**: No need to store the 56 non-exception shards at all—they're implicit!

---

## Example 9: Split Threshold Policy

### Why 34 Entries?

Shard capacity: 63 entries  
Split threshold: 34 entries (54% full)

**Reasoning**:
1. **Early splits** prevent overflow when many writes arrive in a batch
2. **Even distribution**: After split, expect ~17 entries per child
3. **Growth headroom**: Each child can accept 46 more entries before next split

### Alternative: 50 Entry Threshold

If threshold = 50 (79% full):
- After split: ~25 entries per child
- Less headroom: only 38 more entries before next split
- More splits needed, less efficient

### Why Not Split at 63?

Splitting exactly at capacity causes problems:
- **No room for batch inserts**
- **Emergency splits** are expensive
- **Cascading splits** if multiple shards hit limit simultaneously

**Trade-off**: 54% threshold balances segment density with operational efficiency.

---

## Example 10: SLOAD/SSTORE Flow

### Contract State

```
Contract: 0x1234...ABCD
Global depth: 3
SSR: [{ d: 3, ld: 5, prefix: 11100 000... }]

Shards:
  - Shard_A (ld=5, prefix=11100): 20 entries
  - Shard_B (ld=3, prefix=010): 15 entries
  - ... 8 total shards
```

### Transaction: SSTORE(slot_7, value_new)

**Step 1: Hash the slot**
```
H = keccak256(abi.encode(slot_7, slot)) = 0xE4... (binary: 11100100...)
```

**Step 2: Resolve shard ID**
```
1. Start at g=3, prefix=111
2. SSR lookup at d=3, prefix=111: found { d=3, ld=5 }
3. Bump to depth 5
4. Take first 5 bits of H = 11100
5. No further SSR entry
6. Result: ShardId { ld: 5, prefix56: [0xE0, 0, 0, 0, 0, 0, 0] }
```

**Step 3: Load shard (if not cached)**
```
ObjectId = shard_object_id(0x1234...ABCD, ShardId{ld=5, prefix=11100})
ObjectRef = jamState.Read(ObjectId)
Segments = jamDA.ImportSegments(ObjectRef.work_package_hash, ObjectRef.index_start..=index_end)
// First segment: [ObjectID(32) || ObjectRef(64) || payload_part0]
// Skip 96-byte metadata, then deserialize payload across all segments
Entries = deserializeShardFromSegments(Segments)  // Handles metadata skip
```

**Step 4: Update in-memory**
```
Find entry with key_h = 0xE4...
If exists: update value
Else: insert new entry (H, value_new)
Count: 20 → 21 entries
Mark shard as dirty
```

**Step 5: Check split condition**
```
21 entries < 34 threshold → No split needed
```

**Step 6: At end of refine**
```
Serialize dirty shard (Shard_A)
Export to new segments in JAM DA
Write new ObjectRef(version+1) to JAM State
```

### If Split Were Needed (35+ entries)

```
Step 5 (alternative): Split triggered
  1. Read all 35 entries from Shard_A
  2. Partition by bit 5 (next bit after ld=5):
     - prefix=111000: 18 entries
     - prefix=111001: 17 entries
  3. Create two new shards at ld=6
  4. Update SSR: { d=5, ld=6, prefix=11100 000... }
  5. Export both new shards
  6. Delete old Shard_A ObjectRef
```

---

## Key Takeaways

1. **SSR is sparse**: Only stores exceptions where `ld > g` or `ld > d`
2. **Cascading lookups**: Follow depth bumps until no more SSR entries match
3. **Implicit children**: Child shard IDs are computed, not stored
4. **Asymmetric growth**: Only hot shards split; cold shards stay shallow
5. **Compact encoding**: 9 bytes per entry (d, ld, prefix56) vs 32+ bytes in explicit designs
6. **Global depth**: Advances when most shards have split, reducing common lookup depth
7. **Split threshold**: 34/63 entries balances density and efficiency
8. **Deterministic**: Any node can independently resolve the same shard ID from `(contract, slot)`

---

## Visual Summary

```
Evolution of a Contract's Storage:

Time 0: g=0, SSR=[]
┌─────────────────┐
│   Root (ld=0)   │ 1 entry
└─────────────────┘

Time 1: g=0, SSR=[{d=0,ld=1}]
┌────────┬────────┐
│ L(ld=1)│ R(ld=1)│ 18 + 17 entries
└────────┴────────┘

Time 2: g=0, SSR=[{d=0,ld=1}, {d=1,ld=2,p=1}]
┌────────┬────────┬────────┐
│ L(ld=1)│RL(ld=2)│RR(ld=2)│ 18 + 19 + 16 entries
└────────┴────────┴────────┘

Time 3: g=1, SSR=[{d=1,ld=2,p=1}]  (g advanced, first entry removed)
┌────────┬────────┬────────┐
│ L(ld=1)│RL(ld=2)│RR(ld=2)│ Same shards, cleaner SSR
└────────┴────────┴────────┘

Time 4: g=1, SSR=[{d=1,ld=2,p=1}, {d=1,ld=3,p=0}]
┌────────┬────────┬────────┬────────┐
│LL(ld=3)│LR(ld=3)│RL(ld=2)│RR(ld=2)│ L split into LL+LR
└────────┴────────┴────────┴────────┘

... and so on, growing only where needed
```

---

## Further Reading

- **ShardId encoding**: See `take_prefix56()` and `hbit()` helpers in `services/evm/src/state.rs`
- **SSR format**: 9-byte compact entry layout (d, ld, prefix56)
- **Lookup algorithm**: `resolve_shard_id()` with iteration limit
- **Split operation**: `split_shard_compact()` with bucket partitioning
- **Global depth advancement**: Policy rules for when to increment g

---

## Related Documentation

For related topics on the EVM service architecture, see:

- **[BLOCKS.md](BLOCKS.md)** - Block creation, accumulation, and metadata computation via witnesses
- **[CALLS.md](CALLS.md)** - eth_call and eth_estimateGas implementation with PayloadType::Call
- **[EXECUTIONEFFECTS.md](EXECUTIONEFFECTS.md)** - Serialization format for refine→accumulate communication
- **[PROOFS.md](PROOFS.md)** - State witnesses, Merkle proofs, and witness verification

**Implementation Files**:
- `services/evm/src/state.rs` - SSR data structures and sharding logic
- `services/evm/src/refiner.rs` - Refine phase with witness verification
- `services/evm/src/block.rs` - Block structures and witness detection helpers
- `statedb/evmtypes/` - Go-side object structures and serialization
