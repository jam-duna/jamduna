# Ethereum vs. JAM

Ethereum’s EVM was never designed to scale linearly with multicore hardware or exploding state. As usage grows, you hit the same ceiling every team does:

- **Serial execution of hot state.** One storage slot (e.g. `totalSupply`) forces every mint/burn worldwide to queue behind a single core.
- **Global fee auctions.** Gas price reflects whole-network contention, not the actual shard you touch. Costs spike 10–100× when traffic surges.
- **Hidden infrastructure burden.** You pay to read a 32-byte slot once, but node operators carry the storage forever—until they start charging or rate-limiting you.

JAM keeps the EVM programming model intact, but redesigns the runtime and storage layers so engineers can keep shipping without rewriting contracts. Three core changes make the difference:

1. **Automatic hot key isolation.** Recency-aware SSR splits any storage key that becomes contentious. After a handful of writes, hot slots (like `totalSupply`) live in their own shard with deterministic routing. Result: unavoidable serialization stays local while independent keys run in parallel across every core.

2. **Parallel execution as the default.** The overlay/accumulate architecture routes independent transactions to different lanes, merges successful diffs, and refunds losers. With 300+ executor cores, throughput hits 1.5–4.5K token transfers per second *without* touching your Solidity.

3. **Explicit storage economics.** Instead of a one-time `SSTORE` fee and an implicit promise that someone else will host your data forever, JAM charges rent (~$0.20/GB/week) tied to actual shard size. Reads are 10× cheaper (200 gas for cold `SLOAD`), writes are 40× cheaper (500 gas for new `SSTORE`), and you can predictably budget both user fees and operator costs.

In practice, this means:

- **100–300× higher throughput** for ERC-20 style workloads, because only the truly contended keys serialize.
- **10–40× lower per-transaction fees** during normal conditions and no “gas meltdown” during volatility.
- **Audit-friendly state.** Every write produces an ObjectRef with an explicit version. You get 28-day receipts and stable IDs for provenance without bloating canonical state.

You get these wins without migrating off Solidity, rewriting your minting logic, or retooling wallets. Point existing infrastructure at JAM’s fully-compatible JSON-RPC and see immediate gains; later you can opt into Move or custom VMs if you want richer concurrency semantics.

### Hot-Key Example: Stablecoin `totalSupply`

```solidity
function _mint(address account, uint256 amount) internal {
    totalSupply += amount;   // Serial bottleneck on Ethereum
    balances[account] += amount;
}
```

On Ethereum, **everything** is serial (~38 TPS max for any ERC20 operations due to block gas limits) -- the block gas limit inherently limits how much can happen.  Moreoever, on **any parallel execution system** (including optimistic L2s attempting parallelism like Base), state like `totalSupply` becomes the critical bottleneck, forcing serialization. 

On JAM:

| Phase | What Happens | Latency Impact |
|-------|--------------|----------------|
| First writes | `totalSupply` shares a shard with other storage | Cold miss, normal latency |
| Hot detection | SSR split after a few writes | Later reads/writes hit isolated shard |
| Promotion | ≥50 writes triggers hot lane routing | Every update hits dedicated core, other balances run fully parallel |

Balances still run in parallel, so transfers scale with hardware. No contract changes required.

### Cost Comparison (1B transfers/year, 50M holders)

| Metric | Ethereum | JAM |
|--------|----------|-----|
| Typical transfer fee | ~$0.30 (40k gas) | ~$0.01–0.03 (1–2k gas + rent) |
| Annual user cost | ~$300M | ~$20M |
| Storage burden | Hidden: 3–4GB state bloat externalized to RPC providers | Explicit: ~8GB shards + $87/year rent |

You cut user costs by ~93% while gaining headroom for volatility events.

Importantly, any intelligent person will ignore the fees business model and focus directly on the top stablecoin issuers Tether + Circle who earns $10-15MM/day on interest: no chain even comes close to that without poor UX from high gas fees.   Currently, Tether and Circle have competition from Stripe, Paypal, Brale, and many others, who are each aiming to launch their own chain, in an attempt to get network effects.  

### Operational Wins for Engineering Teams

- **Predictable performance.** Parallelism is automatic; no bespoke sharding or contract rewrites.
- **Easier compliance.** ObjectRefs and deterministic versions make audits trivial. Receipts expire after 28 days unless you pin them, so storage doesn’t explode.
- **Clean migrations.** Deploy the same bytecode, run the same ABIs, keep the same toolchain. Opt-in upgrades (e.g. Move contracts sharing the same object store) are available when you want them.

In short, JAM addresses the exact pain points engineers battle on Ethereum—throughput collapses, gas volatility, and opaque storage liabilities—while letting you keep the EVM mental model and existing codebase. If you’re responsible for keeping a high-volume system alive during peak demand, that combination is what makes JAM the pragmatic upgrade path.

### 🔧 Migration Path (Weeks, Not Months)

**Step 1**: Redeploy existing ERC20 bytecode to JAM (unchanged)
**Step 2**: Mint initial supply or migrate balances via merkle proof
**Step 3**: Update frontend to JAM RPC endpoints (same JSON-RPC interface)
**Step 4**: Monitor hot key detection (totalSupply auto-routes to hot lane)

**No changes needed**:
- ✅ Same Solidity contract
- ✅ Same ABI
- ✅ Same wallet addresses (H160 compatible)
- ✅ Same developer tools (Hardhat, Foundry, Remix)

**Integration partners**: Existing integrations (wallets, DEXes, bridges) work unchanged—JAM exposes standard EVM RPC.

### 📊 Stablecoin Issuer Comparison

| Metric | Ethereum Deployment | JAM Deployment |
|--------|---------------------|----------------|
| **Mint/burn TPS** | 10-50 (totalSupply bottleneck) | **1,500-4,500** (hot lane) |
| **Transfer TPS** | 10-50 (block gas limit) | **1,500-4,500** (parallel) |
| **Cost per transfer** | $0.30 typical, $3-10 peak | **$0.01-0.03** (stable) |
| **Annual user gas** | $300M (1B transfers) | **$20M** (93% savings) |
| **Storage cost** | Hidden (~$5-10M externalized) | **$87/year** (transparent) |
| **State bloat** | Unbounded (1+ TB Ethereum total) | **Bounded** (rent + expiry) |
| **Hot key handling** | Manual redesign required | **Automatic** (zero code changes) |
| **Upgrade complexity** | Proxy patterns (security risk) | **Immutable code** (deploy new version) |
| **Conflict handling** | Impossible (serial) | **Fee refunds** on version conflicts |

### 🏦 The Stablecoin Issuer's Dilemma

**Ethereum**:
- ✅ Battle-tested, largest ecosystem, regulatory clarity
- ❌ TPS ceiling kills UX during volatility
- ❌ Gas costs punish users ($300M/year at scale)
- ❌ totalSupply serialization unsolvable without breaking changes

**JAM**:
- ✅ 100-300× throughput (handle any demand spike)
- ✅ 93% cost reduction ($280M/year savings at USDC scale)
- ✅ Drop-in EVM compatibility (same Solidity, same tools)
- ✅ Automatic hot key routing (no manual optimizations)
- ✅ Transparent storage economics (no hidden infrastructure costs)
- ⚠️ Newer ecosystem (fewer integrations initially)

### 💡 Bottom Line for Stablecoin Issuers

**The question isn't "Why JAM?"**—it's **"Can you afford NOT to?"**

At 1B transactions/year, JAM saves your users **$280M annually** while delivering **100× higher throughput**. Your existing ERC20 code works unchanged. Migration takes weeks, not months.

**Ethereum** forces you to choose: Accept TPS limits, or rewrite your core contract and break integrations.

**JAM** removes the choice: Deploy the same Solidity, get 100× throughput and 93% cost savings automatically.

**The math**: USDC does ~$6T volume/year. Even 1% of volume moving to JAM = 100M transactions = **$29M in user savings**. Zero code changes. Zero integration breakage.

**Next step**: Deploy your stablecoin to JAM testnet. Same contract, same ABI, same tools. Measure the 100× throughput yourself.

---

## Quick Reference

| Aspect | Ethereum | JAM |
|--------|----------|-----|
| **Execution** | Serial | Parallel refine on ~341 cores; accumulate resolves |
| **State Model** | Flat key–value under contract | Sharded ObjectRefs (versioned pointers to DA) with owners & types |
| **Storage Cost** | One-time gas | Weekly rent ($0.20/GB/week) + 28-day DA refresh |
| **Conflicts** | N/A (serial) | Shard-level; losers refunded fees; no ObjectRef written |
| **Receipts** | Permanent in chain data | DA-only; auto-expire (~28d); winners get ObjectRef; losers don't |
| **Hot Keys** | Manual mitigation | Recency cache, proactive splits, hot lane, per-core fee collectors |
| **State Growth** | Unbounded | Bounded (rent + ED + purge + DA TTL) |
| **Archive** | Implicitly subsidized | Opt-in economics; explicit costs; optional subsidies possible |
| **Multi-VM** | EVM only | Multiple VMs (EVM, Move, …) on common ObjectRef/rent model |
| **Throughput** | ~tens TPS | 100–300× via parallelism & hot-key routing |
| **Upgrades** | Proxy patterns | Code immutable (exempt); storage versioned |

---

## A) Economic Model

### Rent & Refresh

**Ethereum**: Pay once to write; no recurring rent.

**JAM**: Rent charged weekly ($0.20/GB/week) at export time when due (every 168 epochs). DA refresh required ~every 28 days for active data (re-export new version).

**Impact**: JAM keeps state growth bounded through continuous economic pressure; Ethereum's state grows monotonically.

### Exemptions

**Ethereum**: N/A.

**JAM**: Receipts, system objects (e.g., per-core fee collectors), and contract code are rent-exempt.

**Impact**: Critical infrastructure and ephemeral data don't bloat rent costs; only long-lived user state pays.

### State Expiry vs. Rent

**Ethereum**: EIPs discussed (EIP-4444 for history expiry); not deployed broadly.

**JAM**: Enforced DA TTL + rent now. Receipts expire; storage must refresh; code stays.

**Impact**: JAM has a working solution to state growth; Ethereum still debating proposals.

### Archive Economics

**Ethereum**: Costs externalized to infra providers (Etherscan, Infura, Alchemy).

**JAM**: Explicit, opt-in. Archive nodes refresh only what they value; protocol can add targeted subsidies if desired.

**Impact**: JAM makes preservation costs transparent and market-driven; Ethereum hides costs in volunteer/commercial infrastructure.

### Practical Costs

**Ethereum**: 
- One-time: ~20,000 gas to write 32 bytes (~$0.50 at 25 gwei, $2500 ETH)
- Ongoing: $0 (externalized to node operators)

**JAM**:
- Weekly rent: $0.20/GB/week (~$10.43/GB/year)
- DA refresh: ~$0.01 per 4KB every 28 days
- Small contract (120 KB): ~$0.022/week, ~$1.14/year
- Medium contract (780 KB): ~$0.145/week, ~$7.54/year

**Impact**: JAM's explicit costs enable better capacity planning; Ethereum's hidden costs create surprises during state bloat.

---

## B) Execution Model

### Ownership & ObjectRefs

**Ethereum**: Contract-centric storage; no native object ownership model.

**JAM**: ObjectRef = versioned pointer to DA segments with owner and type, enabling rent and lifecycle policies per owner.

**Impact**: JAM can charge different accounts for different state; Ethereum can't distinguish who benefits from state growth.

### Parallelism & Conflicts

**Ethereum**: Strictly serial execution within a block; transactions processed sequentially.

**JAM**: Parallel refine across cores; accumulate picks winners per shard. Losers: no ObjectRef + fee refund.

**Impact**: JAM achieves 100-300× throughput via parallelism; Ethereum limited to single-threaded execution.

### Fee Semantics

**Ethereum**: Fees charged even if transaction reverts (gas consumed for computation).

**JAM**: 
- Optimistic charge during refine
- Automatic refund in accumulate if conflicted (version conflict = system fault, not user fault)
- Per-core fee collectors avoid system-level hot keys

**Impact**: JAM users never pay for version conflicts; only pay for their execution errors. Fair and predictable.

### Conflict Resolution

**Ethereum**: No conflicts (serial execution guarantees deterministic order).

**JAM**: 
- Conflicts resolved at shard granularity in accumulate
- Version-based: highest version wins
- Losers get full fee refund
- No ObjectRef written for conflicted transactions

**Impact**: JAM's explicit conflict model enables parallelism while maintaining fairness.

---

## C) Storage Architecture

### Shards & Density

**Ethereum**:
- 32-byte key → 32-byte value (64 bytes per entry)
- Sparse Merkle tree structure
- Each slot requires separate tree node
- High overhead for metadata

**JAM**:
- 4KB segments pack up to 63 entries (64 bytes per entry: 32B key hash + 32B value)
- Actual capacity: `(4096 - 32) / 64 = 63` entries per shard
- Split at 34 entries (~54% full) to maintain headroom
- SSR header (28 bytes) + entries (9 bytes each) track splits compactly
- Shards grow/split dynamically via SSR

**Impact**: JAM achieves dense packing (63 entries per 4KB segment) vs Ethereum's sparse tree. Typical contracts use 15-35 entries per shard, maximizing density while enabling splits.

### Hot Key Mitigation

**Ethereum**: Manual contract redesign required (e.g., separate mappings, sharding at application layer).

**JAM**: 
- **Recency cache**: 60-slot circular buffer with saturating counters
- **Proactive splits**: Isolate keys with ≥3 writes while in cache
- **Hot lane**: Dedicated shards for keys with ≥50 writes
- **Per-core fee collectors**: Eliminate universal hot key

**Impact**: JAM automatically detects and routes around hot keys; Ethereum requires developers to anticipate and code around bottlenecks.

### Storage Layout

**Ethereum**:
- Flat key-value under contract address
- All keys in same namespace
- Hot keys serialize entire contract
- Balance/nonce stored separately in account state

**JAM**:
- Sharded across multiple ObjectRefs via SSR (Sparse Split Registry)
- Each shard can be accessed independently
- Hot keys isolated in separate shards
- **Balance/nonce stored in 0x01 precompile**: All account balances and nonces are stored as regular storage entries in the system precompile at address 0x01, using the same SSR sharding mechanics as contracts
  - Storage key for balance: `keccak256(address)`
  - Storage key for nonce: `keccak256(address) + 1`
  - Same gas costs (~200-500) as contract storage operations
  - 0x01's storage can have many shards if network has many accounts

**Impact**: JAM's sharding enables parallel access to different parts of contract state; Ethereum's flat layout creates contention. JAM's unified storage model (0x01 precompile) treats balance/nonce operations uniformly with contract storage.

### Immutability & Upgrades

**Ethereum**: 
- Proxies/delegatecall patterns for upgrades
- Code can be indirectly mutable via proxy
- Storage and code tightly coupled

**JAM**: 
- Code immutable & rent-exempt (Type 0x05)
- Storage versioned independently
- Upgrades via new deployments and state migration
- Clean separation of code vs state lifecycle

**Impact**: JAM enforces immutability at the right layer; upgrades are explicit deployments rather than hidden proxy complexity.

---

## D) Data Lifecycle

### Receipts & Queryability

**Ethereum**: 
- Lookup via `(blockHash, txIndex)`
- Requires knowing which block contained transaction
- All receipts permanent in chain data

**JAM**: 
- `txHash` IS ObjectID → direct lookup
- If no ObjectRef: transaction conflicted (and fee refunded)
- Receipts auto-expire after 28 days in DA

**Impact**: JAM's direct lookup is simpler; conflict status is explicit. Ephemeral receipts reduce long-term storage burden.

### Versioning

**Ethereum**: 
- Implicit versioning via block order
- No per-object version numbers
- State transitions opaque

**JAM**: 
- Every ObjectRef has explicit `Version` field
- Increments on refresh/update
- Conflicts resolved at ObjectRef granularity using versions

**Impact**: JAM's explicit versioning enables fine-grained conflict detection; Ethereum relies on global ordering.

### Account Lifecycle

**Ethereum**: 
- Dust accounts persist forever
- No cleanup mechanism
- Empty accounts still consume state

**JAM**: 
- **Existential deposit** (1 token minimum)
- **Purge rules**: Delete rentable ObjectRefs when can't pay
- **Graceful degradation**: Caches deleted first, then storage; code never deleted
- **Resurrection**: Accounts can come back by re-funding

**Impact**: JAM keeps state tidy through economic pressure; Ethereum accumulates dust forever.

### Touch vs Refresh

**Ethereum**: No distinction (writes are writes).

**JAM**:
- **Touch**: Update metadata only (extend expiration), no DA write
- **Refresh**: Re-export to new work package, increment version, account pays

**Impact**: JAM distinguishes between keeping ObjectRef alive (cheap) vs keeping DA data alive (expensive).

---

## E) Multi-Stakeholder & Multi-VM

### Fee Collectors

**Ethereum**: 
- Single recipient per transaction (block proposer or base fee burn)
- Fee distribution handled at protocol level
- No storage-level hot key consideration

**JAM**: 
- Per-core fee collectors (341 separate addresses)
- System-exempt (Type 0x04, no rent)
- Eliminates universal hot key
- Periodic consolidation to master treasury

**Impact**: JAM's per-core collectors enable parallel fee updates without conflicts; Ethereum's single destination would be bottleneck in parallel execution.

### Sponsorship

**Ethereum**: 
- No native rent, no sponsorship model
- Meta-transactions possible but not protocol-level

**JAM**: 
- Sponsors can fund ObjectRefs (max 8 sponsors per ObjectRef)
- Pro-rated refunds on unsponsor
- Weight-based cost sharing

**Impact**: JAM enables flexible cost-sharing for dapps, DAOs, and users; Ethereum requires application-layer solutions.

### Multi-VM Support

**Ethereum**: 
- EVM only (with some L2 diversity)
- Single execution model
- Tight coupling of VM and state model

**JAM**: 
- Multiple VMs coexist (EVM, Move, custom VMs)
- Same ObjectRef/rent/DA machinery underneath
- Different storage semantics per VM:
  - EVM: Flat key-value, keccak256-based sharding
  - Move: Resource-oriented, module-based sharding
- Unified economics across all VMs

**Impact**: JAM's abstraction layer enables VM diversity without fragmenting the economic model; Ethereum requires separate chains for different VMs.

---

## JAM Terminology (Glossary)

### ObjectRef
Versioned pointer in JAM State to one or more DA segments. Structure from JAM spec:

```rust
pub struct ObjectRef {
    pub service_id: u32,
    pub work_package_hash: [u8; 32],
    pub index_start: u16,
    pub index_end: u16,
    pub version: u32,          // Increments on each update/refresh
    pub payload_length: u32,
    pub timeslot: Timeslot,    // Expiry (~28 days)
}
```

Key concepts:
- **ObjectID** (32 bytes): Identifies what (e.g., contract code, shard, SSR)
  - Code: `[20B address][1B 0x00][11B zero]`
  - SSR: `[20B address][1B 0x01][11B zero]`
  - Shard: `[20B address][1B 0x02][1B ld][7B prefix56][3B zero]`
- **Version**: Used for conflict detection during accumulate
- **Timeslot**: DA segments auto-purge after ~28 days unless refreshed

### Refine
Parallel execution phase where ~341 cores process transactions simultaneously. Produces:
- Tentative state updates
- Transaction receipts (all transactions, winners and losers)
- Fee charges (optimistic)

### Accumulate
Conflict resolution & commitment phase. Actions:
- Resolve version conflicts (pick winners)
- Write ObjectRefs for winners only
- Refund fees to losers
- Collect rent from accounts

### DA (Data Availability)
Off-chain blob storage with ~28-day TTL. Characteristics:
- Segments automatically purged after expiry
- Must refresh to persist (re-export to new work package)
- ObjectRefs in State point to DA segments
- Separation of hot (State) from cold (DA) data

### Shard
Storage partition containing up to 63 entries in dense 4KB segments. Properties:
- **Capacity**: `(4096 - 32) / 64 = 63` entries (32-byte key hash + 32-byte value)
- **Split threshold**: 34 entries (triggers split to avoid overflow)
- **ShardId**: `{ld: u8, prefix56: [u8; 7]}` where `ld` is local depth and `prefix56` is routing prefix
- **SSR (Sparse Split Registry)**: Tracks which shards have split beyond global depth
- **Versioned**: Conflicts detected at shard granularity
- **Dynamically split**: When exceeding 34 entries, split using next bit of key hash
- **Independent access**: Different shards can be modified in parallel

### Recency Cache
Lightweight hot-key detector using:
- 60-slot circular buffer (last 60 written keys)
- Saturating counters per slot (0-255)
- Thresholds: 3 = warm, 10 = hot, 50 = extreme

### Existential Deposit (ED)
Minimum balance (1 token) required to maintain rentable state. Below threshold:
- 0.1-1.0 tokens: At-risk (warning)
- <0.1 tokens: Purge (delete rentable ObjectRefs)

### Hot Lane
Dedicated single-key shards for extreme hot keys (≥50 writes). Characteristics:
- One key per shard (intentionally space-inefficient)
- Zero inter-key conflicts
- 100% intra-key conflict rate (expected for hot keys)
- Forwarding map (max 256 entries) routes lookups

---

## Detailed Comparison by Category

### Storage Operations

| Operation | Ethereum | JAM |
|-----------|----------|-----|
| **Create storage** | SSTORE (20K gas) | SSTORE_NEW (500 gas) + weekly rent |
| **Read storage (cold)** | SLOAD (2100 gas) | SLOAD_COLD (200 gas) + DA import if needed |
| **Read storage (warm)** | SLOAD (100 gas) | SLOAD_WARM (100 gas) |
| **Update storage** | SSTORE (5K gas) | SSTORE_UPDATE (200 gas) + weekly rent |
| **Delete storage** | SSTORE to 0 (5K gas + refund) | SSTORE_DELETE (200 gas) + stop rent |
| **Refresh storage** | N/A | Re-export to DA + increment version (~$0.01/4KB every 28 days) |

### State Growth Scenarios

**Ethereum (10M accounts, 50 slots each)**:
```
Storage: 500M slots × 64 bytes = 32 GB (plus tree overhead)
Growth: Unbounded (no cleanup mechanism)
Cost: One-time gas only (~20K gas per slot)
Long-term: State grows monotonically
```

**JAM (10M accounts, 50 storage entries each via contracts + balance/nonce in 0x01)**:
```
Account metadata (0x01 precompile):
  - 10M accounts × 2 entries (balance + nonce) = 20M entries
  - At 63 entries/shard: ~317K shards for 0x01
  - SSR for 0x01: varies with split depth

Contract storage (regular contracts):
  - 500M entries across all contracts
  - At ~30 entries/shard average: ~16.7M shards
  - Each shard: 4KB in DA
  - SSR overhead: 28B header + 9B per split entry

Total DA: ~67 GB in segments (0x01 + contracts + SSRs + code)
Weekly rent: $0.20/GB × 67 GB = $13.40/week
Growth: Bounded (rent pressure + 28-day DA refresh)
Inactive data: Auto-expires if not refreshed
```

### Hot Key Performance

**Ethereum (ERC20 totalSupply)**:
```
Access pattern: Every mint/burn touches totalSupply
Bottleneck: Serializes all mints/burns
Mitigation: None (application must redesign)
```

**JAM (ERC20 totalSupply)**:
```
Detection: Recency cache identifies after 3 writes
Isolation: Proactive split moves to separate shard
Routing: Affinity routing sends all mints/burns to same core
Hot lane: If ≥50 writes, dedicated single-key shard
Result: Serialized within hot key, parallel for all other operations
```

### Transaction Lifecycle Comparison

**Ethereum**:
```
1. Submit transaction
2. Included in block (serial execution)
3. Success → state updated, fee paid
4. Failure → state unchanged, gas fee paid
5. Receipt stored permanently
```

**JAM**:
```
1. Submit transaction
2. Routed to core (affinity-based)
3. Refine: Execute, charge fee optimistically, emit receipt
4. Export receipt to DA (all transactions)
5. Accumulate:
   - Winner → ObjectRef written, fee kept
   - Loser → No ObjectRef, fee refunded
6. Receipt auto-expires after 28 days (unless explicitly preserved)
```

---

## Migration Considerations

### For Ethereum Developers

**What stays the same**:
- Smart contract languages (Solidity, Vyper work in EVM JAM service)
- Core opcodes (SLOAD, SSTORE, LOG, etc.)
- ABI encoding
- Event emission

**What changes**:
- **Storage costs**: One-time → recurring (rent + refresh)
- **Hot key handling**: Manual → automatic (but benefits from understanding)
- **Receipt lifetime**: Permanent → 28 days
- **Conflict handling**: Impossible → possible (losers refunded)
- **State size planning**: Critical (rent pressure requires optimization)

### For dApp Operators

**Cost model shift**:
```
Ethereum: High upfront gas, zero ongoing
JAM: Lower creation cost, ongoing rent

Break-even: ~1-2 years for typical contract
Long-term: JAM cheaper for frequently-updated state
           Ethereum cheaper for write-once-read-forever state
```

**Operational changes**:
- Monitor rent balance (avoid purges)
- Optimize storage (delete unused ObjectRefs)
- Consider sponsorship for shared infrastructure
- Plan for DA refresh cycles (28 days)

### For Users

**Transaction experience**:
- Faster confirmation (parallel execution)
- Fee refunds on conflicts (fairer)
- Same wallet interfaces (account model similar)

**State preservation**:
- Active usage keeps state alive (auto-refresh)
- Inactive accounts may be purged (below ED)
- Can resurrect by re-funding (no permanent loss)

---
# Historical Comparison: JAM DA/State vs Ethereum Scaling Ideas

## Executive Summary

This document compares JAM's Data Availability (DA) and State model with historical Ethereum scaling proposals, focusing on how each approach handles large contract storage. We analyze quadratic sharding, contract/object sharding, and various application-level patterns, evaluating their trade-offs against JAM's SSR-based approach.

---

## 1. JAM DA/State Model Overview

### Architecture

JAM uses a clean separation between:
- **JAM State**: Thin mapping `object_id → ObjectRef` (version, segment range)
- **JAM DA**: 4KB segments storing actual payload data
- **SSR (Sparse Split Registry)**: Adaptive sharding for contract storage

### Key Features

**Storage Model:**
```rust
// Segment format
First segment:  [ObjectID(32) || ObjectRef(64) || Payload(≤4000)]
Subsequent:     [Payload(4096)]

// SSR-based sharding
ShardId { ld: u8, prefix56: [u8;7] }  // 8 bytes total
SSREntry { d: u8, ld: u8, prefix56: [u8;7] }  // 9 bytes per split
```

**Adaptive Scaling:**
- Shards split at 34/63 entry threshold
- Only hot storage paths split deeper
- Cold storage stays shallow (minimal overhead)
- Global depth advances when ≥70% of shards split

**Economics:**
- Weekly rent: $0.20/GB/week based on footprint
- Ultra-low gas: SLOAD ~200 gas, SSTORE ~500 gas
- Rent covers persistence; gas covers computation
- No refund gaming or slot manipulation incentives

---

## 2. Historical Ethereum Scaling Ideas

### 2.1 Quadratic Sharding (2016-2018)

**Concept:**
- N×N grid of shards (row/column committees)
- Each cell verified by 2 overlapping committees
- Theoretical O(N²) capacity with O(N) verification overhead

**Architecture:**
```
      Col0  Col1  Col2  Col3
Row0 [ C00  C01   C02   C03 ]
Row1 [ C10  C11   C12   C13 ]
Row2 [ C20  C21   C22   C23 ]
Row3 [ C30  C31   C32   C33 ]

- Cell C12 verified by Row1 committee + Col2 committee
- Cross-shard tx touches multiple cells
```

**Strengths:**
- Elegant mathematical scaling properties
- Redundancy without full replication
- Intersection security (corruption requires 2 committees)

**Weaknesses:**
- Complex cross-shard communication
- Non-deterministic cell assignment
- High latency for multi-cell transactions
- Committee coordination overhead

**Comparison to JAM:**

| Aspect | Quadratic Sharding | JAM SSR |
|--------|-------------------|---------|
| **Shard assignment** | Grid-based (non-deterministic) | Hash-based (deterministic) |
| **Cross-shard calls** | Requires 2D committee coordination | Not needed (shards within contract) |
| **Scaling** | O(N²) theoretical capacity | O(depth) adaptive per-contract |
| **Verification** | 2 committees per cell | Single service validation |
| **Complexity** | Very high (2D coordination) | Low (local SSR lookup) |

**Verdict:** JAM's approach is simpler and more practical. Quadratic sharding optimized for global state sharding across all contracts; JAM optimizes for per-contract storage scaling with deterministic routing.

---

### 2.2 Contract/Object Sharding (2017-2020)

**Concept:**
- Each contract lives on one shard
- Cross-contract calls are asynchronous messages
- Contracts can migrate between shards

**Architecture:**
```
Shard 0: [Contract A, Contract D, ...]
Shard 1: [Contract B, Contract E, ...]
Shard 2: [Contract C, Contract F, ...]

A.callB() → async message to Shard 1 → receipt back to Shard 0
```

**Strengths:**
- Natural contract isolation
- Clear ownership boundaries
- Parallelizable execution

**Weaknesses:**
- **Composability broken:** No atomic cross-contract calls
- **Complex finality:** Multi-shard txs need coordination
- **Migration overhead:** Moving contracts between shards is expensive
- **Load balancing:** Popular contracts create hotspots
- **Large contracts:** Still need internal sharding for big storage

**Comparison to JAM:**

| Aspect | Contract Sharding | JAM SSR |
|--------|------------------|---------|
| **Granularity** | Per-contract (coarse) | Per-storage-shard (fine) |
| **Cross-shard** | Async messages (complex) | Not applicable (shards are internal) |
| **Composability** | Broken (async only) | Preserved (within contract) |
| **Hot contracts** | Shard bottleneck | Splits adaptively |
| **Large contracts** | Still needs solution | Built-in SSR sharding |
| **Migration** | Explicit, expensive | Automatic splits |

**Critical Issue for Large Contracts:**

Contract sharding **doesn't solve** the problem of a single contract with massive storage:

```solidity
// Popular DEX with millions of users
contract UniswapV2 {
    mapping(address => uint256) balances;  // 10M entries
    mapping(address => mapping(address => uint256)) allowances;
}

// Even if UniswapV2 lives on Shard 5 alone:
// - All 10M balances are on one shard
// - Shard 5 becomes a bottleneck
// - Other shards underutilized
```

**JAM's Solution:**

JAM shards **within** the contract:
```rust
Contract 0xUniswap lives at service_id X
Storage:
  - SSR (global_depth=8): tracks 256+ storage shards
  - Shard_00: balances[0x00...] (20 entries)
  - Shard_01: balances[0x01...] (18 entries)
  - Shard_FF: balances[0xFF...] (22 entries)

Hot prefix "AB" splits to depth 10:
  - Shard_AB00, Shard_AB01, ..., Shard_AB03 (4 sub-shards)
```

**Verdict:** Contract sharding is orthogonal to intra-contract storage sharding. JAM's model handles both:
- Contracts can be separate services (like contract sharding)
- **Plus** each service has adaptive internal sharding via SSR

---

### 2.3 Application-Level Sharding Patterns

Ethereum developers invented numerous patterns to work around L1 storage limitations:

#### Pattern 1: Paged Arrays/Maps

```solidity
mapping(uint256 page => mapping(uint256 index => bytes32)) pagedData;

function get(uint256 i) returns (bytes32) {
    uint256 page = i / PAGE_SIZE;
    uint256 idx = i % PAGE_SIZE;
    return pagedData[page][idx];
}
```

**Pros:** Reduces per-tx storage touches
**Cons:** Developer must manage paging logic; still expensive writes

**JAM Equivalent:**
```rust
// SSR does this automatically:
resolve_shard_id(storage_key) → ShardId
// No manual paging; automatic split at 34 entries
```

#### Pattern 2: Child Storage Contracts

```solidity
contract UserRegistry {
    mapping(bytes1 prefix => address storageContract) shards;

    function register(address user) {
        bytes1 p = bytes1(uint8(uint256(uint160(user))));
        IStorageShard(shards[p]).store(user, data);
    }
}
```

**Pros:** Spreads storage across multiple contracts
**Cons:** High deployment cost; complex management; fixed shard count

**JAM Equivalent:**
```rust
// SSR entries track splits dynamically:
SSREntry { d: 4, ld: 6, prefix56: [0xAB, ...] }
// New shards created on-demand; no deployment cost
```

#### Pattern 3: SSTORE2 (Code Storage)

```solidity
// Store immutable data in contract bytecode
contract Blob {
    constructor(bytes memory data) {
        // Deploy with data in bytecode
    }
}

// Read with EXTCODECOPY (cheap)
```

**Pros:** Very cheap reads (~100 gas); cheap writes (deploy once)
**Cons:** Immutable; update requires new deployment

**JAM Equivalent:**
```rust
// Code object (ObjectKind::Code):
code_object_id(contract) → segments
// Immutable by design; versioned via ObjectRef.version
```

**Note:** JAM also supports code segmentation (TODO in backend.rs:64-66):
```rust
// Current: exports full bytecode in one segment
// Planned: split into 4KB chunks with index_start/index_end
```

#### Pattern 4: Diamond Pattern (ERC-2535)

```solidity
contract Diamond {
    mapping(bytes4 selector => address facet) facets;
    mapping(bytes32 namespace => bytes32 slot) storage;
}

// Add facets over time without storage collisions
```

**Pros:** Upgradeable; modular logic
**Cons:** Complex routing; still needs sharding for large datasets

**JAM Equivalent:**
- Services can be modular (multiple refine/accumulate functions)
- SSR handles storage sharding automatically
- No need for namespace tricks

---

## 3. Comparative Analysis: Large Contract Storage

### Scenario: 10 Million User Balances

**Ethereum L1 (No Sharding):**
```solidity
mapping(address => uint256) balances;  // 10M slots
// Cost: 10M × 20,000 gas = 200B gas to initialize
// Storage: 10M × 32 bytes = 320 MB on-chain
// Problem: All in one contract; single-threaded execution
```

**Contract Sharding:**
```
Contract on Shard 5:
  - Still has 10M slots
  - Shard 5 bottleneck
  - Doesn't help scalability within contract
```

**Application-Level Paging:**
```solidity
mapping(uint8 page => mapping(address => uint256)) balances;
// Developer manually assigns users to pages
// Cost: Still expensive; manual balancing
```

**JAM SSR:**
```rust
// Start: global_depth=0, one root shard
// At 35 users: split to depth 1 (2 shards)
// At ~280 users: split hot shards to depth 2 (4+ shards)
// At ~2K users: global_depth=4 (16+ shards)
// At 10M users: global_depth=16 (~65K shards, adaptive)

Storage footprint:
- SSR: 28 bytes header + ~200 entries × 9 bytes = ~2 KB
- Shards: ~160K shards × ~63 entries × 64 bytes = ~640 MB
- Total: ~640 MB (comparable to Ethereum)

But:
- Automatic load balancing (hot prefixes split more)
- Parallel reads (different shards independent)
- Only touched shards imported during refine
- Rent based on actual usage (empty shards cleaned up)
```

---

## 4. Trade-Off Matrix

| Dimension | Ethereum L1 | Quadratic Sharding | Contract Sharding | JAM SSR |
|-----------|------------|-------------------|------------------|---------|
| **Intra-contract scaling** | ❌ Manual patterns | ❌ Not addressed | ❌ Not addressed | ✅ Automatic |
| **Cross-contract calls** | ✅ Atomic | ⚠️ Complex | ❌ Async only | ✅ Within service |
| **Deterministic routing** | ✅ Yes | ❌ Committee-based | ⚠️ Contract address | ✅ Hash-based |
| **Hot path handling** | ❌ Bottleneck | ⚠️ Cell hotspots | ❌ Shard hotspot | ✅ Adaptive split |
| **Storage overhead** | Low (flat) | High (committees) | Medium (migrations) | Low (9B/split) |
| **Developer complexity** | High (manual) | Very high | High (async) | Low (transparent) |
| **Economics** | Gas only | Gas + committee | Gas + migration | Rent + low gas |
| **Composability** | ✅ Full | ⚠️ Limited | ❌ Broken | ✅ Within service |

---

## 5. Code Segmentation: Current TODO

### Current State (backend.rs:64-66)

```rust
//! ❌ **TODO:**
//! - Code segmentation (4KB chunks) - currently exports full bytecode
```

**Current Implementation:**
```rust
// Export: single segment with full bytecode
write_intents.push(WriteIntent {
    effect: WriteEffectEntry {
        object_id: code_object_id(address),
        ref_info: ObjectRef {
            index_start: 0,
            index_end: 1,  // Only 1 segment
            payload_length: bytecode.len(),
        },
        payload: bytecode.clone(),  // Full bytecode
    },
});
```

**Planned Implementation:**
```rust
// Export: split into 4KB chunks
const FIRST_SEGMENT_PAYLOAD: usize = 4000;  // 4096 - 96 metadata
const SUBSEQUENT_SEGMENT: usize = 4096;

let mut segments = Vec::new();
let mut remaining = bytecode;

// First segment
let first_chunk = remaining[..FIRST_SEGMENT_PAYLOAD.min(remaining.len())];
segments.push(create_segment_with_metadata(object_id, ref_info, first_chunk));
remaining = &remaining[first_chunk.len()..];

// Subsequent segments
while !remaining.is_empty() {
    let chunk = &remaining[..SUBSEQUENT_SEGMENT.min(remaining.len())];
    segments.push(chunk.to_vec());
    remaining = &remaining[chunk.len()..];
}

// Export with proper index_start/index_end
ref_info.index_start = start_index;
ref_info.index_end = start_index + segments.len();
```

**Benefits of Segmentation:**
1. **Lazy loading:** Import only needed code segments
2. **Partial verification:** Verify specific functions without full bytecode
3. **DA efficiency:** Fits JAM's 4KB segment model
4. **Consistent:** Matches shard/SSR segment handling

**Comparison to SSTORE2:**

| Aspect | SSTORE2 (Ethereum) | JAM Code Segments |
|--------|-------------------|-------------------|
| **Granularity** | Per-blob contract | 4KB chunks |
| **Immutability** | ✅ Yes | ✅ Yes (versioned) |
| **Read cost** | ~100 gas (EXTCODECOPY) | Import from DA |
| **Write cost** | ~32K gas/KB (deploy) | Export to DA + rent |
| **Partial access** | ❌ Must deploy full blob | ✅ Can import specific segments |
| **Updates** | New deployment + pointer | New version (copy-on-write) |

---

## 6. Economic Comparison

### Storage Costs (10 MB contract storage)

**Ethereum L1:**
```
Initial write: 10 MB / 32 bytes = 312,500 slots
Cost: 312,500 × 20,000 gas = 6.25B gas ≈ $6,250 @ 100 gwei, $2000 ETH
Ongoing: Free (no rent)
```

**Ethereum L2 (Optimism/Arbitrum):**
```
Initial: ~10× cheaper ≈ $625
Ongoing: Free (L2 covers DA to L1)
```

**JAM DA:**
```
Initial: 10 MB / 64 bytes = 156,250 entries
Cost: 156,250 × 500 gas = 78M gas (low, comparable to L2)
Rent: 10 MB × $0.20/GB/week = $0.002/week = $0.10/year
```

**Trade-offs:**

| Aspect | Ethereum L1 | Ethereum L2 | JAM |
|--------|------------|------------|-----|
| **Upfront cost** | Very high | Medium | Medium |
| **Ongoing cost** | Free | Free | Small rent |
| **State bloat** | ⚠️ Permanent | ⚠️ L2 pays | ✅ Rent incentivizes cleanup |
| **DoS resistance** | ❌ Pay once, squat forever | ⚠️ L2 subsidy | ✅ Unpaid rent → expiry |
| **Scalability** | ❌ Limited | ⚠️ Per-L2 | ✅ Adaptive SSR |

---

## 7. Strengths and Weaknesses

### JAM DA/State Model

**✅ Strengths:**

1. **Automatic scaling:** SSR splits hot paths without developer intervention
2. **Deterministic:** Any node can independently route to same shard
3. **Low overhead:** 9 bytes per SSR entry; compact ShardId
4. **Adaptive:** Cold storage stays shallow; hot storage splits deep
5. **Economic balance:** Rent prevents state bloat; low gas encourages usage
6. **Lazy loading:** Import only touched shards during refine
7. **Composability:** Full atomic operations within a service
8. **Simple model:** Clean DA/State separation

**❌ Weaknesses:**

1. **Rent complexity:** Requires weekly rent collection mechanism
2. **Cascade lookups:** Deep shards need multiple SSR hops (max 8)
3. **Global depth management:** Advancing/reducing `g` requires coordination
4. **Cold start cost:** First access to new contract imports full SSR
5. **Code segmentation:** Not yet implemented (TODO)
6. **Cross-service calls:** Async only (like contract sharding)
7. **Learning curve:** SSR mechanics not widely understood yet

### Historical Ethereum Ideas

**Quadratic Sharding:**
- ✅ Elegant scaling theory
- ❌ Too complex to implement
- ❌ Doesn't solve intra-contract storage

**Contract Sharding:**
- ✅ Natural isolation
- ❌ Breaks composability
- ❌ Doesn't solve large contract problem

**Application Patterns:**
- ✅ Work within current EVM
- ❌ High developer burden
- ❌ Gas-inefficient
- ❌ Manual load balancing

---

## 8. Conclusions

### When JAM is Better

1. **Large contracts with many users** (DEXs, registries, token contracts)
   - SSR automatically shards storage
   - Hot users split deeper; cold users stay shallow
   - No manual paging or child contracts needed

2. **Predictable economics**
   - Rent discourages state bloat
   - Low gas for operations
   - No refund gaming

3. **Lazy loading scenarios**
   - Only import touched shards
   - Efficient for contracts with sparse access patterns

4. **Developer simplicity**
   - No manual sharding logic
   - Transparent to smart contract code
   - Automatic load balancing

### When Ethereum Patterns Might Be Better

1. **Immutable data** (on-chain art, game assets)
   - SSTORE2 is very cheap for reads
   - JAM rent might be higher long-term for static data
   - **Mitigation:** JAM could offer reduced rent for read-only objects

2. **Small contracts** (< 1000 storage slots)
   - Ethereum's flat model is simpler
   - No SSR overhead
   - **JAM:** Still works fine, just 1 shard at depth 0

3. **Cross-contract composability is critical**
   - Ethereum L1 has atomic cross-contract calls
   - JAM services are isolated (async only)
   - **Mitigation:** Design services to be self-contained

4. **No ongoing costs tolerance**
   - Some prefer "pay once, use forever" model
   - JAM rent requires continuous funding
   - **Mitigation:** Rent is very low ($10/GB/year)

### Hybrid Future

The ideal might combine approaches:

- **JAM for large, mutable state** (user balances, positions)
- **IPFS/Arweave for permanent data** (immutable blobs)
- **L2 rollups for high-frequency trading** (batched execution)
- **L1 Ethereum for global settlement** (cross-chain bridges)

JAM's SSR model represents a significant evolution beyond quadratic/contract sharding by:
1. Operating at the right granularity (storage shards, not contracts)
2. Being deterministic and automatic
3. Balancing economics (rent) with usability (low gas)

---

## 9. Danksharding and Data Availability Sampling (DAS)

### 9.1 What is Danksharding?

Danksharding is Ethereum's future DA scaling upgrade that uses:

1. **Single slot proposer:** One validator chooses all transactions + data per slot
2. **Merged blob fee market:** Unified pricing for DA across all rollups
3. **PBS (Proposer-Builder Separation):** Builders assemble blocks, proposers pick best bid
4. **DAS (Data Availability Sampling):** Nodes sample small pieces instead of downloading all data

### 9.2 Current State: Proto-Danksharding (EIP-4844)

**Launched:** March 13, 2024 (Dencun upgrade)

**What it does:**
- Adds "blob" transactions (cheap temporary data for rollups)
- ~125 KB per blob, ~3-6 blobs per block initially
- Blobs expire after ~18 days (not permanent storage)
- Separate gas market from execution (cheaper for L2s)

**Architecture:**
```
Ethereum Block:
  Execution layer: [Transactions (permanent state)]
  Consensus layer: [Blobs (temporary DA, 18-day TTL)]

Blob format:
  - 4096 field elements × 32 bytes = 131,072 bytes per blob
  - KZG commitments for erasure coding
  - Blobs attached to beacon blocks, not execution blocks
```

**Limitations (pre-DAS):**
- All nodes must download all blobs (bandwidth bottleneck)
- Limited to ~375-750 KB/block (3-6 blobs)
- Not sufficient for 100+ rollups at scale

### 9.3 Next Step: PeerDAS (EIP-7594)

**PeerDAS** is Ethereum's concrete DAS protocol, enabling:

1. **Erasure coding:** Blobs split into N columns (e.g., 128 columns)
2. **Sampling:** Each node samples a few random columns (e.g., 16/128)
3. **Probabilistic guarantee:** If 50%+ columns available, full data reconstructable
4. **P2P distribution:** Nodes share sampled columns over gossip network

**How it works:**
```
Builder creates blob:
  - Encode blob into 128 columns using Reed-Solomon erasure coding
  - Publish KZG commitments for each column

Validator/Node:
  - Download only 16/128 columns (12.5% of data)
  - Verify columns against commitments
  - Share columns with peers

Reconstruction:
  - If 64+/128 columns available → full blob recoverable
  - If <64/128 → withholding detected → block not finalized
```

**Scaling impact:**
- Can safely increase to 16-32 blobs/block (~2-4 MB)
- Nodes still download ~12.5% (same bandwidth)
- 10x+ DA capacity via "Blob-Parameter-Only (BPO)" forks

### 9.4 Comparison: JAM DA vs Ethereum Danksharding

| Aspect | Ethereum Danksharding | JAM DA |
|--------|----------------------|---------|
| **Permanence** | Temporary (18 days) | Persistent (until rent unpaid) |
| **Segment size** | 131 KB blobs | 4 KB segments |
| **Erasure coding** | KZG commitments (future) | Not specified (implementation detail) |
| **Sampling** | DAS (future: 12.5% download) | Import only touched objects |
| **State model** | Separate (blobs ≠ state) | Unified (DA stores state) |
| **Pricing** | Blob gas market | Weekly rent ($0.20/GB/week) |
| **Use case** | Rollup calldata | Contract storage + code |
| **Reconstruction** | Automatic (erasure coded) | Not needed (versioned objects) |
| **Access pattern** | Write-once, read-many (L2 posts) | Read-write (contract storage) |
| **Finality** | Dependent on sampling | Per-object versioning |

### 9.5 Detailed Architecture Comparison

#### Ethereum Danksharding Stack

```
┌─────────────────────────────────────┐
│   Rollups (Optimism, Arbitrum)      │  Application layer
├─────────────────────────────────────┤
│   Blob Transactions (EIP-4844)      │  DA layer (temporary)
│   - 3-6 blobs/block (375-750 KB)   │
│   - KZG commitments                 │
│   - 18-day TTL                      │
├─────────────────────────────────────┤
│   PeerDAS (EIP-7594) - Future       │  Sampling layer
│   - Erasure code to 128 columns    │
│   - Nodes sample 16/128 columns    │
│   - Gossip protocol for sharing    │
├─────────────────────────────────────┤
│   Beacon Chain (Consensus)          │  Consensus layer
│   - PBS: builders bid on blocks    │
│   - Single proposer per slot       │
├─────────────────────────────────────┤
│   Execution Layer (State)           │  Permanent state
│   - 256-bit Merkle Patricia Trie   │
│   - Full nodes download everything │
└─────────────────────────────────────┘
```

**Key insight:** Danksharding separates DA (blobs) from state (execution). Blobs are temporary; state is permanent.

#### JAM DA Stack

```
┌─────────────────────────────────────┐
│   Services (EVM, WASM, etc)         │  Application layer
├─────────────────────────────────────┤
│   JAM State (Thin Index)            │  State layer
│   - object_id → ObjectRef mapping  │
│   - Version tracking                │
│   - Copy-on-write                   │
├─────────────────────────────────────┤
│   JAM DA (Segment Storage)          │  DA layer (persistent)
│   - 4 KB segments                   │
│   - First: [ObjectID||ObjectRef||  │
│            payload(4000)]           │
│   - Rest: [payload(4096)]           │
├─────────────────────────────────────┤
│   SSR (Adaptive Sharding)           │  Sharding layer
│   - Per-service storage sharding   │
│   - Automatic split at 34 entries  │
│   - Lazy loading (import touched)  │
├─────────────────────────────────────┤
│   Refine/Accumulate                 │  Execution model
│   - Refine: stateless (imports DA) │
│   - Accumulate: updates State      │
└─────────────────────────────────────┘
```

**Key insight:** JAM unifies DA and state. DA stores actual state; JAM State is just an index.

### 9.6 Use Case Comparison

#### Scenario 1: Rollup Posts Transaction Batch

**Ethereum (Danksharding):**
```
1. Rollup collects 1000 txs on L2
2. Compresses to 100 KB batch
3. Posts as blob transaction to L1
4. Blob stored for 18 days (DAS: nodes sample 12.5%)
5. L2 nodes can reconstruct from any 50% of samples
6. After 18 days: blob pruned (L2 must archive)

Cost: ~$0.01-0.10 depending on blob gas price
Economics: Blob fee market (EIP-4844)
```

**JAM (Hypothetical Rollup Service):**
```
1. Service collects 1000 txs
2. Updates state shards (touched balances)
3. Exports dirty shards to DA (e.g., 10 shards × 4 KB)
4. DA persists until rent unpaid
5. Next service can import shards via ObjectRef

Cost: 10 shards × 4 KB = 40 KB → ~$0.0001/week rent
Economics: Rent-based (permanent until evicted)
```

**Trade-off:**
- Ethereum: Cheaper upfront, temporary, optimized for write-once
- JAM: Small ongoing cost, permanent, optimized for read-write state

#### Scenario 2: Large Contract Storage (1 GB)

**Ethereum (Current):**
```
Option A: Store on L1
  - 1 GB / 32 bytes = 31.25M slots
  - Cost: 31.25M × 20K gas = 625B gas (prohibitive)
  - Ongoing: Free

Option B: Store as blobs (post-Danksharding)
  - Not suitable: blobs are temporary (18 days)
  - Rollup must re-post every 18 days
  - Cost: 1 GB / 131 KB = 7,634 blobs every 18 days

Option C: L2 + blob commitments
  - Store on L2, post commitments/roots to L1 blobs
  - L2 covers storage; L1 provides DA for roots
  - Best current approach
```

**JAM:**
```
1. Deploy contract with SSR sharding
2. 1 GB → ~15.6M entries → ~250K shards (adaptive)
3. SSR: ~5000 entries × 9 bytes = 45 KB
4. Rent: 1 GB × $0.20/week = $0.20/week = $10.40/year
5. Import only touched shards during refine

Economics: $10/GB/year ongoing vs Ethereum's $millions upfront
```

**Trade-off:**
- Ethereum: Unsuitable for large persistent state at L1
- L2s: Viable but centralized storage risk
- JAM: Native support, automatic sharding, predictable rent

### 9.7 Sampling Model Differences

#### Ethereum DAS (PeerDAS)

**Goal:** Verify blob data availability without downloading all data

**Method:**
```
1. Builder erasure-codes blob into 128 columns
2. Publishes KZG commitment for each column
3. Validators randomly sample 16/128 columns
4. Verify samples against commitments
5. If 64+/128 columns available → data available
6. Gossip protocol shares columns among nodes
```

**Math:**
- Probability of detecting 50% withholding: >99.999% with 16 samples
- Reed-Solomon allows reconstruction from any 64/128 columns
- Bandwidth per node: 16/128 = 12.5% of total blob data

**Security model:**
- Assumes honest majority of validators
- Probabilistic guarantee (not deterministic)
- Relies on gossip network health

#### JAM Lazy Loading (Implicit Sampling)

**Goal:** Import only necessary state during refine

**Method:**
```
1. Service lists needed objects in dependencies
2. Refine imports only those objects from DA
3. SSR routes to specific shards (deterministic)
4. Only touched shards loaded into memory
```

**Example:**
```rust
// EVM transfer from Alice to Bob
dependencies = [
    balance_shard(Alice),  // Might import shard 0x4A
    balance_shard(Bob),    // Might import shard 0xB2
]

// Don't import:
// - Other 65K shards in 0x01 precompile
// - Code object (if already cached)
// - SSR (unless updating)
```

**Math:**
- For 10M user contract with 65K shards:
  - Transfer touches 2 shards → 2/65K = 0.003% imported
  - Batch of 100 transfers → ~200 shards → 0.3% imported
- No probabilistic sampling; deterministic based on execution

**Security model:**
- Deterministic (not probabilistic)
- Relies on ObjectRef versioning (copy-on-write)
- State root verification (not shown, but implicit)

### 9.8 Economic Model Comparison

#### Ethereum Blob Pricing (EIP-4844)

**Mechanism:**
- Separate blob gas market (independent of execution gas)
- Target: 3 blobs/block (375 KB)
- Max: 6 blobs/block (750 KB)
- Price adjusts dynamically (like EIP-1559)

**Formula:**
```
blob_gas_price = MIN_BLOB_GAS_PRICE × e^(excess_blob_gas / BLOB_GAS_UPDATE_FRACTION)

Where:
- MIN_BLOB_GAS_PRICE = 1 wei
- Target = 3 blobs/block
- Price doubles every ~2.7 blocks above target
```

**Cost examples (at different loads):**
```
Low demand (1 blob/block): ~$0.001 per blob (131 KB)
At target (3 blobs/block): ~$0.01 per blob
High demand (6 blobs/block): ~$0.10-1.00 per blob (surge pricing)
```

**Post-DAS (16-32 blobs/block target):**
- Same pricing mechanism, higher capacity
- More blobs → more stable pricing (less surge)

#### JAM Rent Model

**Mechanism:**
- Fixed rate: $0.20/GB/week
- Charged weekly during export (if due)
- Based on footprint: `SSR size + shard_count × shard_size`

**Formula:**
```rust
footprint = ssr_size + (max_shards × 4096)
max_shards = 2^global_depth  // Upper bound estimate
rent_due = (footprint / 1GB) × $0.20

Collected if: current_epoch - last_rent_paid ≥ 168 epochs
```

**Cost examples:**
```
100 KB contract: $0.00002/week = $0.001/year
1 MB contract:   $0.0002/week  = $0.01/year
100 MB contract: $0.02/week    = $1/year
1 GB contract:   $0.20/week    = $10.40/year
```

**Rent vs. Blob Comparison:**

| Size | Ethereum (one-time post) | Ethereum (18-day renewal) | JAM (annual rent) |
|------|-------------------------|---------------------------|-------------------|
| 100 KB | $0.01 | $20.28/year | $0.001/year |
| 1 MB | $0.08 | $162/year | $0.01/year |
| 100 MB | $7.63 | $16,200/year | $1/year |
| 1 GB | $76.34 | $162,000/year | $10.40/year |

**Key insight:** Ethereum blobs are vastly cheaper for one-time posts but become extremely expensive for persistent storage (must re-post every 18 days). JAM rent is optimized for persistent, mutable state.

### 9.9 Strengths and Weaknesses

#### Ethereum Danksharding

**✅ Strengths:**

1. **Optimized for rollups:** Perfect fit for L2 batch posts
2. **Temporary data:** No long-term state bloat
3. **Proven research:** KZG commitments, erasure coding well-understood
4. **Probabilistic guarantees:** DAS provides strong availability assurance
5. **Separate fee market:** Blob pricing independent of execution gas
6. **Rollup-centric roadmap:** Aligns with Ethereum's modular vision
7. **PBS integration:** Sophisticated builder/proposer separation

**❌ Weaknesses:**

1. **Not for persistent state:** 18-day TTL unsuitable for contract storage
2. **Reconstruction overhead:** Must erasure-code and reconstruct
3. **Sampling complexity:** DAS requires complex gossip protocols
4. **Limited to calldata:** Can't store mutable contract state in blobs
5. **Centralization risk:** L2s must archive blob data themselves
6. **Temporary availability:** After 18 days, data may be lost
7. **Fixed blob size:** 131 KB may not fit all use cases

#### JAM DA Model

**✅ Strengths:**

1. **Persistent state:** DA stores actual contract storage (not temporary)
2. **Unified model:** No separation between DA and state
3. **Deterministic loading:** Import exactly what you need (no sampling)
4. **Adaptive sharding:** SSR scales per-contract
5. **Simple pricing:** Fixed rent, predictable costs
6. **Fine granularity:** 4 KB segments vs 131 KB blobs
7. **Lazy loading:** Only touched shards imported
8. **Versioning:** Copy-on-write via ObjectRef

**❌ Weaknesses:**

1. **Rent collection:** Requires ongoing payment mechanism
2. **Not optimized for write-once:** Blob model cheaper for one-time posts
3. **No erasure coding (yet):** Less redundancy than DAS
4. **State bloat risk:** Rent must be enforced to prevent spam
5. **Learning curve:** SSR mechanics more complex than flat storage
6. **Cross-service calls:** Async only (like Ethereum L1↔L2)

### 9.10 Hybrid Use Cases

The two models serve different needs:

**Use Ethereum Danksharding for:**
- Rollup transaction batches (write-once, read-many)
- Temporary data feeds (price oracles, proofs)
- Cross-chain bridges (L1→L2 messages)
- Archival data with off-chain backups
- High-throughput, low-latency posts

**Use JAM DA for:**
- Contract storage (balances, positions, state)
- Persistent registries (ENS-like systems)
- Large mutable datasets (DEX order books)
- Code storage (bytecode, WASM modules)
- Long-lived application state

**Example: Hybrid DEX**
```
Ethereum L1:
  - Settlement layer (finality)
  - Blob posts from L2 (trade batches)

Ethereum L2 (Optimism/Arbitrum):
  - Execution layer (trade matching)
  - Posts proofs/batches to L1 blobs

JAM Service:
  - User balances (persistent SSR shards)
  - Order book (adaptive sharding)
  - Settlement to Ethereum via bridge
```

### 9.11 Future Convergence

Both models may converge on similar techniques:

**JAM could add:**
- Erasure coding for DA segments (redundancy)
- Probabilistic sampling (optional, for light clients)
- Blob-like temporary objects (lower rent tier)

**Ethereum could add:**
- Longer blob TTLs (but still not permanent)
- State rent (discussed, not yet implemented)
- Native sharding (abandoned in favor of rollups)

**Most likely outcome:**
- Ethereum remains rollup-centric (blobs for L2 DA)
- JAM provides native execution + persistent DA
- Cross-chain bridges connect the ecosystems
- Developers choose based on use case

---

## 10. Visual Comparison

```
Ethereum L1 (Flat Storage):
Contract A: [████████████████] 10M slots, bottleneck

Contract Sharding:
Shard 0: [Contract A: ████████████████] Still bottleneck
Shard 1: [Contract B: ██]
Shard 2: [Contract C: ████]

Quadratic Sharding:
     C0    C1    C2    C3
R0 [ ██    ██    ██    ██ ]
R1 [ ██    ██    ██    ██ ]
R2 [ ██    ██    ██    ██ ]
R3 [ ██    ██    ██    ██ ]
(Complex coordination)

JAM SSR (Adaptive):
Contract A (10M users):
  SSR (global_depth=16):
    - Entry 1: {d=8, ld=10, prefix=AB}  # Hot prefix
    - Entry 2: {d=12, ld=14, prefix=CD12}  # Very hot
    - ... 200 entries total

  Shards (automatic distribution):
    Shard_0000: ██ (22 users)
    Shard_0001: ██ (19 users)
    ...
    Shard_AB00: ████ (35 users, split from AB)
    Shard_AB01: ████ (31 users, split from AB)
    ...
    Shard_CD1200: ██████ (40 users, deep split)
    ...
    Shard_FFFF: ██ (18 users)

  Result: ~65K shards, average 153 users each
```

---

## 11. Recommendations

### For JAM Development

1. **✅ Implement code segmentation** (backend.rs TODO)
   - Align with DA segment model
   - Enable lazy code loading
   - Support partial verification

2. **Consider rent tiers**
   - Reduced rate for read-only objects
   - Higher rate for write-heavy objects
   - Grace period for low-balance contracts

3. **SSR compaction tools**
   - Automatic cleanup of obsolete entries
   - Global depth advancement triggers
   - Shard merge for under-utilized storage

4. **Developer tooling**
   - SSR visualization (shard tree viewer)
   - Gas/rent estimators
   - Migration guides from Ethereum patterns

### For Large Contract Developers

1. **Design for sharding:**
   - Hash-based key distribution (avoid sequential IDs)
   - Minimize cross-shard dependencies
   - Use events for analytics (not storage)

2. **Rent budgeting:**
   - Monitor storage growth
   - Budget ~$10/GB/year
   - Clean up unused data

3. **Migration from Ethereum:**
   - Paged maps → direct mappings (SSR handles it)
   - Child contracts → unified contract (SSR splits)
   - SSTORE2 blobs → code objects (segment when implemented)

---

## Appendix: Further Reading

### JAM Documentation
- [DA-STORAGE.md](DA-STORAGE.md) - Full JAM DA specification with SSR details
- [ETHEREUM.md](ETHEREUM.md) - EVM integration details

### Ethereum Scaling Research
- **Quadratic Sharding** (2016-2018) - Early 2D sharding proposals
- **Contract/Object Sharding** (2017-2020) - Per-contract execution sharding
- **EIP-4844: Proto-Danksharding** - Blob transactions (live March 2024)
- **EIP-7594: PeerDAS** - Data availability sampling protocol (testnet 2025)
- **Danksharding Roadmap** - ethereum.org scaling documentation

### Ethereum Development Patterns
- **ERC-2535: Diamond Standard** - Upgradeable contract architecture
- **SSTORE2 Pattern** - Storing data in contract bytecode
- **Application-Level Sharding** - Manual paging and child contracts

### Academic Papers
- **Data Availability Sampling** - KZG commitments and erasure coding
- **Reed-Solomon Codes** - Error correction for distributed systems
- **Merkle Trees vs Verkle Trees** - State commitment structures

---

**Document Status:** Draft
**Last Updated:** 2025-01-18
**Author:** Historical analysis based on JAM implementation and Ethereum research archive
