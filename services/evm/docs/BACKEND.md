# EVM Backend (Overlay-First Design)

This tracks how Majik's overlay-first backend surfaces the vendor EVM runtime traits. The runtime now interacts with `MajikOverlay`, which embeds the upstream `OverlayedBackend` together with Majik's dependency tracker. `MajikBackend` keeps its role as the DA-facing store for code and shards, but it is no longer exposed directly to the interpreter during execution.

## State Persistence Model

### Architecture Overview

The execution stack is reduced to two Majik-owned components plus the vendor overlay internals:

- **`MajikOverlay`** – Primary runtime entry point. Implements every runtime trait (`RuntimeBaseBackend`, `RuntimeBackend`, `RuntimeEnvironment`, and `TransactionalBackend`) while owning the vendor `OverlayedBackend`. All reads and writes are recorded inline through the embedded tracker. Substate management is delegated to the vendor overlay but coordinated through `MajikOverlay` so tracker frames stay in sync with CALL/CREATE depth.

- **`MajikBackend`** – DA helper reachable only from `MajikOverlay`. Handles cache misses, resolves SSR metadata, and applies the final overlay diff once a block finishes. It no longer masquerades as a runtime backend; mutators remain stubbed because real writes happen when the overlay diff is flushed.

**Lifecycle:** Block execution constructs a single `MajikOverlay` backed by a fresh `MajikBackend`. Each transaction pushes a vendor substate via `MajikOverlay::begin_transaction`, executes, then commits or reverts through the wrapper. After the block completes, `MajikOverlay::deconstruct()` returns the backend, the vendor change-set, and the tracker output so the orchestrator can persist to DA and emit dependency artefacts.

```
┌──────────────────────────────────────────────────────────────┐
│ EVM Interpreter                                               │
│  ├─ Calls RuntimeBaseBackend / RuntimeBackend traits          │
│  └─ Calls RuntimeEnvironment for block metadata               │
└──────────────────────────────┬────────────────────────────────┘
                               │ all traffic hits MajikOverlay
                               ↓
┌──────────────────────────────────────────────────────────────┐
│ MajikOverlay                                                   │
│  • Owns vendor OverlayedBackend + substate stack               │
│  • Records read/write dependencies inline                      │
│  • Exposes transactional helpers (begin/commit/revert)         │
└──────────────────────────────┬────────────────────────────────┘
                               │ cache misses / block flush
                               ↓
┌──────────────────────────────────────────────────────────────┐
│ MajikBackend                                                  │
│  • Imports shards/code on demand from JAM DA                   │
│  • Applies OverlayedChangeSet at block finalisation            │
│  • RefCell<BTreeMap> caches for interior mutability           │
└──────────────────────────────┬────────────────────────────────┘
                               │ data availability
                               ↓
┌──────────────────────────────────────────────────────────────┐
│ JAM DA                                                        │
│  • Storage shards (SSR-indexed 4KB segments)                   │
│  • Code objects + precompile 0x01 balance/nonce storage        │
└──────────────────────────────────────────────────────────────┘
```

**Trait-to-Implementation Mapping:**

| Trait | Implemented by | Notes |
|-------|----------------|-------|
| `RuntimeBaseBackend` | `MajikOverlay` | Reads delegate to vendor overlay, then always record a dependency to the base DA object (shard/code) regardless of cache hit. Helper methods on `MajikBackend` resolve ObjectIDs and versions. |
| `RuntimeBackend` | `MajikOverlay` | Mutations update the vendor overlay first, then record a `WriteRecord` with transaction index and instance counter for deterministic ordering. |
| `RuntimeEnvironment` | `MajikOverlay` | All methods delegate directly to the vendor overlay, which retrieves metadata from the wrapped `MajikBackend`. |
| `TransactionalBackend` | `MajikOverlay` | `push_substate`/`pop_substate` hooks coordinate vendor overlay substates with tracker frame reads. Commit merges child reads into parent; revert discards them. |

---

## RefCell Pattern for Interior Mutability

> Interior mutability is a core ingredient of the `MajikBackend` runtime. This section captures why we lean on `RefCell<BTreeMap<...>>`, where it lives, and the guard rails to keep it safe.

### Why `RefCell` + `BTreeMap`?

- Runtime traits such as `RuntimeBaseBackend` expose only `&self`, but reads still need to mutate caches and shard state.
- Execution is single-threaded inside the JAM VM, so `RefCell` gives us fast interior mutability without `Mutex` or `RwLock` overhead.
- `BTreeMap` keeps iteration deterministic (critical for reproducible state export) and copying small values such as `U256` is cheap.

Taken together the type simply means **"a cache keyed by `H160` that we can mutate through `&self`."**

### Where We Use It (`services/evm/src/state.rs`)

```rust
pub struct MajikBackend {
    pub code_storage: RefCell<BTreeMap<H160, Vec<u8>>>,   // address -> bytecode
    pub code_hashes: RefCell<BTreeMap<H160, H256>>,       // address -> keccak(code)
    pub storage_shards: RefCell<BTreeMap<H160, ContractStorage>>, // address -> SSR + shards
    pub balances: RefCell<BTreeMap<H160, U256>>,          // address -> balance
    pub nonces: RefCell<BTreeMap<H160, U256>>,            // address -> nonce
    pub imported_objects: RefCell<BTreeMap<ObjectId, (ObjectRef, Vec<u8>)>>, // object_id cache
    pub execution_mode: ExecutionMode,                    // Builder vs Guarantor mode
    // ...
}
```

| Field | Backing Data | Why `RefCell`? | Notes |
|-------|--------------|----------------|-------|
| `code_storage` | Bytecode blobs | Lazy-import bytecode, then cache for future calls | Populated by UBT fetch or DA import |
| `code_hashes` | Keccak of bytecode | Provide `EXTCODEHASH` without recomputing | Updated alongside bytecode |
| `storage_shards` | SSR header + shard map | Need to import or mutate shards inside read paths | Wraps `ContractStorage` |
| `balances` / `nonces` | System-contract slots | `balance()` or `nonce()` run with `&self` | Cache zeroes to avoid TRAPs |
| `imported_objects` | `(ObjectRef, payload)` | Reuse DA payloads across shard or code calls | Keyed by 32-byte object id |
| `execution_mode` | Builder vs Guarantor | N/A (not `RefCell`) | Set at construction, immutable during execution |

### Read / Write Flow in Practice

1. Borrow immutably and try the cache.
2. On a miss, import from DA (or the overlay) and collect dependency metadata.
3. Borrow mutably in a new scope and insert the freshly fetched value.
4. Keep borrows short -- drop them before logging or calling into other helpers.

#### Example: `MajikBackend::balance`

```rust
fn balance(&self, address: H160) -> U256 {
    // Check cache first
    {
        let balances = self.balances.borrow();
        if let Some(balance) = balances.get(&address) {
            return *balance;
        }
    }

    // Cache miss - fetch based on execution mode
    match self.execution_mode {
        ExecutionMode::Builder => {
            // Builder mode: Use UBT host function
            let balance = crate::ubt_host::fetch_balance_ubt(address, 0);
            self.balances.borrow_mut().insert(address, balance);
            balance
        }
        ExecutionMode::Guarantor => {
            // Guarantor mode: Cache miss is fatal
            panic!("GUARANTOR: Balance cache miss for address={:?}", address);
        }
    }
}
```

That final `borrow_mut` is why the pattern exists -- without `RefCell` this method could not cache the value while only holding `&self`.

#### The TRAP That Motivated the Cache

Before the fix we only cached non-zero balances. A zero result (for example `0x44`) triggered a DA fetch for every opcode that peeked at the balance, eventually exhausting VM resources and halting execution. Caching zero balances through the `RefCell` map closes the loop and keeps the VM stable.

### Safety Guidelines

- **Borrow discipline**: one mutable borrow or many immutable borrows -- violations panic at runtime.
- **Scope carefully**: wrap reads in braces so the immutable borrow drops before calling `borrow_mut()`.
- **Clone tiny values**: copying `U256` or `H256` is cheaper than holding borrowed references longer than needed.
- **Log after dropping**: if you need to call `call_log`, do it after the immutable borrow scope so logging cannot extend the borrow.
- **Tests live nearby**: `services/evm/src/tests.rs` hits the caching paths -- extend them when you touch borrow-heavy logic.

### When *Not* to Use `RefCell`

- Shared across threads -> use `Arc<RwLock<_>>` or a mutex.
- Async contexts -> use `tokio::sync::RwLock` so `.await` does not hold a `Ref`.
- Pure `&mut self` call paths -> prefer a plain `BTreeMap` and skip the runtime borrow checks entirely.

### Checklist for Adding a New Cache

1. Choose the key (`H160`, `ShardId`, and so on) and ensure deterministic ordering matters.
2. Add the `RefCell<BTreeMap<...>>` field with a comment explaining the mapping.
3. Initialize it in `MajikBackend::new` (or the appropriate constructor).
4. Implement the read path: borrow, check, clone or copy the value.
5. Implement the miss path: fetch, then call `borrow_mut().insert`.
6. Wire dependency tracking into the tracker (`record_read` or `record_write`).
7. Add logging or tests that fail without the cache to catch regressions.

Following this recipe keeps the JAM EVM runtime fast, deterministic, and safe even while mutating state through immutable handles.

---

## UBT Witness Integration

The EVM service now uses UBT multiproofs (JAMProfile hashing).
See `services/evm/docs/UBT-CODEX.md` for the architecture, witness format, and execution flow.

### Cache-First Pattern

All `RuntimeBaseBackend` methods follow this pattern:

```rust
fn state_read(&self, address: H160) -> Value {
    // 1. Check cache
    {
        let cache = self.cache.borrow();
        if let Some(value) = cache.get(&address) {
            return *value;
        }
    }

    // 2. Handle cache miss based on mode
    match self.execution_mode {
        ExecutionMode::Builder => {
            // Fetch from UBT tree via host function
            let value = crate::ubt_host::fetch_balance_ubt(address, 0);
            self.cache.borrow_mut().insert(address, value);
            value
        }
        ExecutionMode::Guarantor => {
            // Cache miss is fatal - witness incomplete
            panic!("GUARANTOR: Cache miss for {:?}", address);
        }
    }
}
```

**Design Rationale**:
- **Builder**: UBT reads logged to `StateDBStorage.ubtReadLog` (storage layer maintains authoritative log)
- **Guarantor**: All accessed state must be in witness (deterministic verification)
- **Two-step API**: Query size first (`output_max_len=0`), then fetch data
- **Witness ownership**: Storage package owns witness construction and tree operations
- **Clean separation**: statedb = EVM operations, storage = UBT tree + witness

---

## `RuntimeBaseBackend`

### `balance`
**Interface:** `fn balance(&self, address: H160) -> U256`
**Description:** Gets the balance associated with an address.
**JAM DA/State Handling:**
- **Builder Mode**: Uses UBT host function `host_fetch_ubt(FETCH_BALANCE)` to read from UBT tree
  - Go reads BasicData key (suffix 0) from UBT tree
  - Extracts balance from bytes [16:31] of BasicData (16 bytes, big-endian uint128)
  - Logs read to `StateDBStorage.ubtReadLog` for witness construction
- **Guarantor Mode**: Reads from pre-populated cache (populated from UBTWitness)
  - Cache miss causes panic (witness must contain all accessed state)
- UBT tree structure per EIP-6800: balance stored at BasicData offset 16-31

**Call Sites:**
1. **BALANCE opcode** ([eval/system.rs:53](../services/vendor/evm-interpreter/src/eval/system.rs#L53)) – Pops an address off the stack and pushes the result of `handler.balance`.
2. **SELFBALANCE opcode** ([eval/system.rs:64](../services/vendor/evm-interpreter/src/eval/system.rs#L64)) – Reads the executing account's balance via `handler.balance`.
3. **SELFDESTRUCT opcode** ([eval/system.rs:379](../services/vendor/evm-interpreter/src/eval/system.rs#L379)) – Retrieves the contract's balance before transferring it to the beneficiary.
4. **Gas cost – CALL** ([invoker/mod.rs:655](../services/vendor/evm/src/standard/invoker/mod.rs#L655)) – Ensures the source account has sufficient funds when value is supplied with a call.
5. **Gas cost – CREATE** ([invoker/mod.rs:733](../services/vendor/evm/src/standard/invoker/mod.rs#L733)) – Verifies the caller can fund the value transfer during contract creation.
6. **Gas cost – SUICIDE** ([gasometer/mod.rs:488](../services/vendor/evm/src/standard/gasometer/mod.rs#L488)) – Uses the current balance to compute refunds and payouts.
7. **Overlay deposit** ([backend/overlayed.rs:348](../services/vendor/evm/src/backend/overlayed.rs#L348)) – Reads the existing balance before crediting value.
8. **Overlay withdrawal** ([backend/overlayed.rs:359](../services/vendor/evm/src/backend/overlayed.rs#L359)) – Checks the current balance to guard against overdrafts.
9. **Overlay exists check** ([backend/overlayed.rs:198](../services/vendor/evm/src/backend/overlayed.rs#L198)) – Treats zero balance as part of the empty-account predicate under EIP-161.

### `code`
**Interface:** `fn code(&self, address: H160) -> Vec<u8>`
**Description:** Returns the bytecode stored at an address.
**JAM DA/State Handling:**
- Code ObjectId = `[20B contract][1B kind=0x00][11B zero]` stored in DA as 4KB segments.
- Refine imports code via `Read(code_object_id) -> Option<ObjectRef>`, then fetches DA segments.
- Bytecode cached in-memory during transaction; subsequent calls are free (no DA re-import).
- Code access adds dependency: work package depends on current code object version.

**Call Sites:**
1. **EXTCODECOPY opcode** ([eval/system.rs:178](../services/vendor/evm-interpreter/src/eval/system.rs#L178)) – Copies external contract code into memory via `handler.code`.
2. **Call trap setup** ([trap.rs:101](../services/vendor/evm-interpreter/src/trap.rs#L101)) – Loads the callee's bytecode when preparing a call trap.
3. **Resolver call entry** ([invoker/resolver.rs:138](../services/vendor/evm/src/standard/invoker/resolver.rs#L138)) – Fetches contract code to initialise a new interpreter.
4. **Overlay fallback** ([backend/overlayed.rs:168](../services/vendor/evm/src/backend/overlayed.rs#L168)) – Delegates to the base backend when overlay caches lack bytecode.
5. **Default `code_hash`** ([runtime.rs:224](../services/vendor/evm-interpreter/src/runtime.rs#L224)) – Hashes the bytecode returned by `code()`.
6. **Default `code_size`** ([runtime.rs:217](../services/vendor/evm-interpreter/src/runtime.rs#L217)) – Counts the bytes provided by `code()` to compute size.

### `code_size`
**Interface:** `fn code_size(&self, address: H160) -> U256`
**Description:** Reports the length of the bytecode at an address.
**JAM DA/State Handling:**
- Code size stored in ObjectRef.payload_length field after initial code import from DA.
- Can be read without importing full bytecode if ObjectRef is cached in JAM State.
- Default implementation calls `code()` which triggers full DA import; optimized backends cache size separately.
- Size=0 check is critical for CREATE collision detection per EIP-7610.

**Call Sites:**
1. **EXTCODESIZE opcode** ([eval/system.rs:153](../services/vendor/evm-interpreter/src/eval/system.rs#L153)) – Pushes `handler.code_size` for an external account onto the stack.
2. **Transaction validation** ([invoker/mod.rs:339](../services/vendor/evm/src/standard/invoker/mod.rs#L339)) – Rejects top-level transactions whose caller already has code.
3. **In-memory backend `can_create`** ([backend/in_memory.rs:224](../services/vendor/evm/src/backend/in_memory.rs#L224)) – Ensures CREATE targets have no code.
4. **Overlay empty-account check** ([backend/overlayed.rs:199](../services/vendor/evm/src/backend/overlayed.rs#L199)) – Combines code size with balance and nonce for EIP-161 semantics.
5. **Overlay collision check** ([backend/overlayed.rs:237](../services/vendor/evm/src/backend/overlayed.rs#L237)) – Requires zero code size before allowing CREATE.
6. **Default `code_size`** ([runtime.rs:217](../services/vendor/evm-interpreter/src/runtime.rs#L217)) – Computes size by measuring the bytes from `code()`.
7. **Default `can_create`** ([runtime.rs:243](../services/vendor/evm-interpreter/src/runtime.rs#L243)) – Uses `code_size` to detect create collisions.

### `code_hash`
**Interface:** `fn code_hash(&self, address: H160) -> H256`
**Description:** Returns the keccak256 hash of an account's bytecode.
**JAM DA/State Handling:**
- Code hash can be precomputed and stored in JAM State account metadata to avoid DA imports.
- Default implementation imports full bytecode via `code()` then hashes, triggering DA segment fetch.
- Returns H256::zero() for non-existent accounts per EIP-1052; empty code hashes to `keccak256([])`.
- Optimized backends cache hash alongside code to serve EXTCODEHASH without re-importing.

**Call Sites:**
1. **EXTCODEHASH opcode** ([eval/system.rs:164](../services/vendor/evm-interpreter/src/eval/system.rs#L164)) – Pushes the keccak of the target's bytecode via `handler.code_hash`.
2. **Overlay implementation** ([backend/overlayed.rs:172](../services/vendor/evm/src/backend/overlayed.rs#L172)) – Returns zero for empty accounts before hashing the cached bytecode.
3. **Default `code_hash`** ([runtime.rs:223](../services/vendor/evm-interpreter/src/runtime.rs#L223)) – Checks existence and then hashes the bytes from `code()`.

### `storage`
**Interface:** `fn storage(&self, address: H160, index: H256) -> H256`
**Description:** Fetches the storage value at a specific key for an address.
**JAM DA/State Handling:**
- Resolve shard via SSR: hash key with keccak256, lookup ShardId from SSR entries based on prefix bits.
- Import shard from DA: `shard_object_id(contract, ShardId)` → fetch 4KB segment with up to 63 sorted entries.
- Binary search within shard for key_h; cache shard in-memory for subsequent reads (~200-300 gas).
- Track read dependency: work package depends on shard's ObjectRef version for parallelism.

**Call Sites:**
1. **SLOAD opcode** ([eval/system.rs:286](../services/vendor/evm-interpreter/src/eval/system.rs#L286)) – Loads contract storage through `handler.storage`.
2. **Gas cost – SSTORE** ([gasometer/mod.rs:446](../services/vendor/evm/src/standard/gasometer/mod.rs#L446)) – Compares current and new values when pricing a store.
3. **Overlay read** ([backend/overlayed.rs:184](../services/vendor/evm/src/backend/overlayed.rs#L184)) – Delegates to the base backend if the overlay lacks a cached slot.
4. **Overlay original tracking** ([backend/overlayed.rs:247](../services/vendor/evm/src/backend/overlayed.rs#L247)) – Uses `storage` to answer `original_storage` queries.

### `nonce`
**Interface:** `fn nonce(&self, address: H160) -> U256`
**Description:** Returns the current nonce used for replay protection and CREATE address derivation.
**JAM DA/State Handling:**
- **Builder Mode**: Uses UBT host function `host_fetch_ubt(FETCH_NONCE)` to read from UBT tree
  - Go reads BasicData key (suffix 0) from UBT tree
  - Extracts nonce from bytes [8:15] of BasicData (8 bytes, big-endian uint64)
  - Logs read to `StateDBStorage.ubtReadLog` for witness construction
- **Guarantor Mode**: Reads from pre-populated cache (populated from UBTWitness)
  - Cache miss causes panic (witness must contain all accessed state)
- UBT tree structure per EIP-6800: nonce stored at BasicData offset 8-15
- Used for CREATE address derivation: `keccak256(rlp([sender, nonce]))[12:]`; nonce=0 required for `can_create`.

**Call Sites:**
1. **CREATE legacy address derivation** ([trap.rs:401](../services/vendor/evm-interpreter/src/trap.rs#L401)) – Reads the caller's nonce to compute the legacy create address.
2. **In-memory backend `can_create`** ([backend/in_memory.rs:223](../services/vendor/evm/src/backend/in_memory.rs#L223)) – Requires zero nonce before allowing contract creation.
3. **Overlay retrieval** ([backend/overlayed.rs:213](../services/vendor/evm/src/backend/overlayed.rs#L213)) – Falls back to the base backend when the overlay lacks nonce data.
4. **Overlay exists check** ([backend/overlayed.rs:200](../services/vendor/evm/src/backend/overlayed.rs#L200)) – Treats a zero nonce as part of the empty-account predicate.
5. **Overlay collision guard** ([backend/overlayed.rs:237](../services/vendor/evm/src/backend/overlayed.rs#L237)) – Ensures CREATE targets start with nonce zero.
6. **Overlay `inc_nonce`** ([backend/overlayed.rs:369](../services/vendor/evm/src/backend/overlayed.rs#L369)) – Reads the prior nonce before incrementing it.
7. **Default `can_create`** ([runtime.rs:243](../services/vendor/evm-interpreter/src/runtime.rs#L243)) – Includes nonce in the create collision test.

### `transient_storage`
**Interface:** `fn transient_storage(&self, address: H160, index: H256) -> H256`
**Description:** Reads a transient (EIP-1153) storage slot that resets each transaction.
**JAM DA/State Handling:**
- Transient storage is NOT persisted to DA or JAM State; exists only in-memory during transaction.
- Implemented as in-memory hashmap cleared at transaction boundaries (TSTORE/TLOAD opcodes).
- No DA imports, no object versioning, no rent charges; purely ephemeral per EIP-1153.
- Gas cost ~100 for TLOAD/TSTORE since no persistence overhead.

**Call Sites:**
1. **TLOAD opcode** ([eval/system.rs:318](../services/vendor/evm-interpreter/src/eval/system.rs#L318)) – Fetches transient storage through `handler.transient_storage`.
2. **Overlay retrieval** ([backend/overlayed.rs:192](../services/vendor/evm/src/backend/overlayed.rs#L192)) – Delegates to the base backend when the overlay lacks a value.

### `exists`
**Interface:** `fn exists(&self, address: H160) -> bool`
**Description:** Determines whether an address is considered existing under the active protocol rules.
**JAM DA/State Handling:**
- Pre-EIP-161: account exists if any of its DA objects exist (code, balance in 0x01, nonce in 0x01).
- Post-EIP-161: account exists if (balance > 0) OR (nonce > 0) OR (code_size > 0).
- Check requires reading balance/nonce from 0x01's DA shards and code_size from code ObjectRef.
- Empty accounts (0/0/0) are deleted per EIP-161 by removing entries from 0x01 and code objects.

**Call Sites:**
1. **Gas cost – CALLCODE** ([gasometer/mod.rs:347](../services/vendor/evm/src/standard/gasometer/mod.rs#L347)) – Checks whether the target exists when pricing a `CALLCODE`.
2. **Gas cost – STATICCALL** ([gasometer/mod.rs:363](../services/vendor/evm/src/standard/gasometer/mod.rs#L363)) – Applies the same existence test for `STATICCALL`.
3. **Gas cost – DELEGATECALL** ([gasometer/mod.rs:414](../services/vendor/evm/src/standard/gasometer/mod.rs#L414)) – Queries `handler.exists` while metering `DELEGATECALL`.
4. **Gas cost – SUICIDE** ([gasometer/mod.rs:478](../services/vendor/evm/src/standard/gasometer/mod.rs#L478)) – Determines whether the beneficiary account already exists.
5. **Gas cost – CALL** ([gasometer/mod.rs:494](../services/vendor/evm/src/standard/gasometer/mod.rs#L494)) – Evaluates callee existence when pricing a value-bearing call.
6. **Overlay empty-account shortcut** ([backend/overlayed.rs:78](../services/vendor/evm/src/backend/overlayed.rs#L78)) – Uses existence when applying EIP-161 deletions.
7. **Overlay touch tracking** ([backend/overlayed.rs:84](../services/vendor/evm/src/backend/overlayed.rs#L84)) – Checks existence before marking accounts as touched.
8. **Overlay implementation** ([backend/overlayed.rs:196](../services/vendor/evm/src/backend/overlayed.rs#L196)) – Applies EIP-161 rules before deferring to the base backend.
9. **Overlay fallback** ([backend/overlayed.rs:205](../services/vendor/evm/src/backend/overlayed.rs#L205)) – Delegates to the underlying backend when overlay knowledge is insufficient.
10. **Default `code_hash`** ([runtime.rs:223](../services/vendor/evm-interpreter/src/runtime.rs#L223)) – Uses `exists` to decide whether to hash code bytes.

### `can_create`
**Interface:** `fn can_create(&self, address: H160) -> bool`
**Description:** Detects potential create collisions per EIP-7610 and related rules.
**JAM DA/State Handling:**
- Returns true only if (code_size == 0) AND (nonce == 0) per EIP-7610 collision rules.
- Requires reading nonce from 0x01's DA shards and code_size from code ObjectRef metadata.
- Prevents CREATE/CREATE2 from overwriting existing contracts or EOAs with nonce > 0.
- EIP-7610 also checks storage emptiness; check target contract's SSR header total_keys field for non-zero entries.

**Call Sites:**
1. **Create routine** ([invoker/routines.rs:100](../services/vendor/evm/src/standard/invoker/routines.rs#L100)) – Aborts contract creation if a collision is detected.
2. **In-memory backend** ([backend/in_memory.rs:223](../services/vendor/evm/src/backend/in_memory.rs#L223)) – Implements the zero-code/zero-nonce requirement.
3. **Overlay backend** ([backend/overlayed.rs:235](../services/vendor/evm/src/backend/overlayed.rs#L235)) – Applies EIP-7610 storage checks before deferring.
4. **Default implementation** ([runtime.rs:243](../services/vendor/evm-interpreter/src/runtime.rs#L243)) – Provides the canonical collision detection logic.

## `RuntimeBackend`

### `original_storage`
**Interface:** `fn original_storage(&self, address: H160, index: H256) -> H256`
**Description:** Retrieves the storage slot value from the start of the current transaction.
**JAM DA/State Handling:**
- Original storage must be cached at transaction start before any SSTORE modifications occur.
- Overlay tracks original values when first slot is accessed; base backend reads from DA shards.
- EIP-2200/EIP-1283 refund calculations depend on comparing original → current → new values.
- Backend imports shard once per transaction; original_storage reads from initial cached state.

**Call Sites:**
1. **Gas cost – SSTORE** ([gasometer/mod.rs:445](../services/vendor/evm/src/standard/gasometer/mod.rs#L445)) – Reads the original slot value for refund calculations.
2. **Overlay fallback** ([backend/overlayed.rs:247](../services/vendor/evm/src/backend/overlayed.rs#L247)) – Delegates to the base backend when the overlay lacks an original value.

### `deleted`
**Interface:** `fn deleted(&self, address: H160) -> bool`
**Description:** Indicates whether an account has been marked for deletion in this transaction.
**JAM DA/State Handling:**
- Deletion tracked in-memory overlay; not persisted to DA/State until transaction completes.
- At accumulate, deleted accounts have their DA objects removed: code, storage SSR, all shards.
- Balance/nonce entries in 0x01 precompile shards are also cleared for deleted accounts.
- Refund gas (24000) credited once per deletion; `deleted()` prevents double refunds.

**Call Sites:**
1. **Gas cost – SUICIDE** ([gasometer/mod.rs:491](../services/vendor/evm/src/standard/gasometer/mod.rs#L491)) – Uses `handler.deleted` to avoid double-counting refunds.
2. **Overlay lookup** ([backend/overlayed.rs:256](../services/vendor/evm/src/backend/overlayed.rs#L256)) – Consults the overlay substate for accounts marked deleted.
3. **Overlay merge** ([backend/overlayed.rs:581](../services/vendor/evm/src/backend/overlayed.rs#L581)) – Propagates deleted flags when substates merge.

### `created`
**Interface:** `fn created(&self, address: H160) -> bool`
**Description:** Reports whether an account was created during the current transaction.
**JAM DA/State Handling:**
- Creation tracked in-memory overlay during transaction; persisted at accumulate.
- EIP-6780 allows SELFDESTRUCT only for accounts created in same transaction.
- At accumulate, new code objects exported to DA; nonce set to 1 in 0x01 precompile shards.
- Initial balance written to 0x01 shards if creation includes value transfer.

**Call Sites:**
1. **Overlay tracking** ([backend/overlayed.rs:252](../services/vendor/evm/src/backend/overlayed.rs#L252)) – Records addresses created in the current transaction.
2. **Overlay delete guard** ([backend/overlayed.rs:315](../services/vendor/evm/src/backend/overlayed.rs#L315)) – Checks whether an account was just created before honoring EIP-6780 deletions.
3. **Overlay merge** ([backend/overlayed.rs:591](../services/vendor/evm/src/backend/overlayed.rs#L591)) – Carries the created flag up the substate stack.

### `is_cold`
**Interface:** `fn is_cold(&self, address: H160, index: Option<H256>) -> bool`
**Description:** Tests whether an address or storage slot remains cold in the current transaction.
**JAM DA/State Handling:**
- Cold/warm tracking maintained in-memory per transaction; not persisted to DA/State.
- First access to address/slot incurs cold surcharge (+2600 gas); subsequent accesses are warm.
- Access list prewarming avoids DA import cost penalties by marking addresses/slots warm upfront.
- Tracking is purely for EIP-2929 gas accounting; no impact on DA object versions or dependencies.

**Call Sites:**
1. **Gas cost – EXTCODESIZE** ([gasometer/mod.rs:320](../services/vendor/evm/src/standard/gasometer/mod.rs#L320)) – Determines whether the target address is cold.
2. **Gas cost – BALANCE** ([gasometer/mod.rs:329](../services/vendor/evm/src/standard/gasometer/mod.rs#L329)) – Applies the cold-access surcharge to balance lookups.
3. **Gas cost – EXTCODEHASH** ([gasometer/mod.rs:340](../services/vendor/evm/src/standard/gasometer/mod.rs#L340)) – Checks warmth before hashing code.
4. **Gas cost – CALLCODE** ([gasometer/mod.rs:352](../services/vendor/evm/src/standard/gasometer/mod.rs#L352)) – Uses `is_cold` for callcode accesses.
5. **Gas cost – STATICCALL** ([gasometer/mod.rs:368](../services/vendor/evm/src/standard/gasometer/mod.rs#L368)) – Warmth check for static calls.
6. **Gas cost – EXTCODECOPY** ([gasometer/mod.rs:386](../services/vendor/evm/src/standard/gasometer/mod.rs#L386)) – Applies cold-access pricing when copying code.
7. **Gas cost – SLOAD** ([gasometer/mod.rs:407](../services/vendor/evm/src/standard/gasometer/mod.rs#L407)) – Tests whether the slot has been touched.
8. **Gas cost – DELEGATECALL** ([gasometer/mod.rs:419](../services/vendor/evm/src/standard/gasometer/mod.rs#L419)) – Checks if the delegate target is cold.
9. **Gas cost – SSTORE** ([gasometer/mod.rs:441](../services/vendor/evm/src/standard/gasometer/mod.rs#L441)) – Warmth test prior to storing.
10. **Gas cost – SUICIDE** ([gasometer/mod.rs:483](../services/vendor/evm/src/standard/gasometer/mod.rs#L483)) – Determines whether the beneficiary is cold.
11. **Gas cost – CALL** ([gasometer/mod.rs:499](../services/vendor/evm/src/standard/gasometer/mod.rs#L499)) – Applies cold-access pricing for call targets.

### `mark_hot`
**Interface:** `fn mark_hot(&mut self, address: H160, kind: TouchKind)`
**Description:** Marks an address as warm for EIP-2929 access accounting.
**JAM DA/State Handling:**
- Warmth tracked in-memory hashset; purely for EIP-2929 gas accounting, no DA/State persistence.
- Access list prewarming happens at transaction start; marked addresses skip cold surcharge.
- `TouchKind::StateChange` indicates account state modification; triggers dependency tracking for parallelism.
- No impact on DA object versions; warmth resets at transaction boundaries.

**Call Sites:**
1. **Transaction initiation – coinbase** ([invoker/mod.rs:337](../services/vendor/evm/src/standard/invoker/mod.rs#L337)) – Warms the block coinbase with `TouchKind::Coinbase`.
2. **Transaction initiation – access list** ([invoker/mod.rs:406](../services/vendor/evm/src/standard/invoker/mod.rs#L406)) – Marks listed addresses as warm with `TouchKind::Access`.
3. **Transaction initiation – caller access** ([invoker/mod.rs:412](../services/vendor/evm/src/standard/invoker/mod.rs#L412)) – Warms the transaction caller.
4. **Transaction initiation – caller state change** ([invoker/mod.rs:413](../services/vendor/evm/src/standard/invoker/mod.rs#L413)) – Marks the caller with `TouchKind::StateChange` for nonce updates.
5. **Transaction initiation – target** ([invoker/mod.rs:414](../services/vendor/evm/src/standard/invoker/mod.rs#L414)) – Warms the top-level call target.
6. **Access list storage keys** ([invoker/mod.rs:408](../services/vendor/evm/src/standard/invoker/mod.rs#L408)) – Warms storage keys listed in the access list (paired with `mark_storage_hot`).
7. **CALL substack entry** ([invoker/mod.rs:674](../services/vendor/evm/src/standard/invoker/mod.rs#L674)) – Warms the callee when entering a call trap.
8. **CREATE substack entry** ([invoker/mod.rs:769](../services/vendor/evm/src/standard/invoker/mod.rs#L769)) – Warms the soon-to-be-created address.
9. **Call routine** ([invoker/routines.rs:41](../services/vendor/evm/src/standard/invoker/routines.rs#L41)) – Uses `TouchKind::StateChange` when entering a subcall.
10. **Create routine** ([invoker/routines.rs:84](../services/vendor/evm/src/standard/invoker/routines.rs#L84)) – Marks the current context as a state-changing touch.
11. **Gas cost – EXTCODESIZE** ([gasometer/mod.rs:321](../services/vendor/evm/src/standard/gasometer/mod.rs#L321)) – Warms the external address accessed by EXTCODESIZE.
12. **Gas cost – BALANCE** ([gasometer/mod.rs:330](../services/vendor/evm/src/standard/gasometer/mod.rs#L330)) – Warms the address inspected by BALANCE.
13. **Gas cost – EXTCODEHASH** ([gasometer/mod.rs:341](../services/vendor/evm/src/standard/gasometer/mod.rs#L341)) – Warms the address whose code hash is requested.
14. **Gas cost – CALLCODE** ([gasometer/mod.rs:353](../services/vendor/evm/src/standard/gasometer/mod.rs#L353)) – Warms the callcode target.
15. **Gas cost – STATICCALL** ([gasometer/mod.rs:369](../services/vendor/evm/src/standard/gasometer/mod.rs#L369)) – Warms the static call target.
16. **Gas cost – EXTCODECOPY** ([gasometer/mod.rs:387](../services/vendor/evm/src/standard/gasometer/mod.rs#L387)) – Warms the address whose code is copied.
17. **Gas cost – DELEGATECALL** ([gasometer/mod.rs:420](../services/vendor/evm/src/standard/gasometer/mod.rs#L420)) – Warms the delegate call target.
18. **Gas cost – SUICIDE (access)** ([gasometer/mod.rs:484](../services/vendor/evm/src/standard/gasometer/mod.rs#L484)) – Warms the beneficiary address.
19. **Gas cost – SUICIDE (state change)** ([gasometer/mod.rs:485](../services/vendor/evm/src/standard/gasometer/mod.rs#L485)) – Records a state-changing touch on the beneficiary.
20. **Gas cost – CALL** ([gasometer/mod.rs:500](../services/vendor/evm/src/standard/gasometer/mod.rs#L500)) – Warms the callee when issuing a call.

### `mark_storage_hot`
**Interface:** `fn mark_storage_hot(&mut self, address: H160, index: H256)`
**Description:** Marks an `(address, storage slot)` pair as warm for subsequent access.
**JAM DA/State Handling:**
- Warmth tracked in-memory hashmap per (address, key) pair; no DA/State persistence.
- First SLOAD/SSTORE incurs 2100 gas cold access; subsequent access ~100 gas.
- Access list prewarming imports DA shards upfront, marking slots warm before execution.
- Warmth tracking independent of shard versioning; purely EIP-2929 gas accounting.

**Call Sites:**
1. **Gas cost – SLOAD** ([gasometer/mod.rs:408](../services/vendor/evm/src/standard/gasometer/mod.rs#L408)) – Warms the accessed storage slot.
2. **Gas cost – SSTORE** ([gasometer/mod.rs:442](../services/vendor/evm/src/standard/gasometer/mod.rs#L442)) – Marks the slot as warm after storing.
3. **Access list prewarm** ([invoker/mod.rs:408](../services/vendor/evm/src/standard/invoker/mod.rs#L408)) – Applies to storage keys included in the access list.

### `set_storage`
**Interface:** `fn set_storage(&mut self, address: H160, index: H256, value: H256) -> Result<(), ExitError>`
**Description:** Writes a persistent storage value at the specified slot.
**JAM DA/State Handling:**
- Writes cached in-memory overlay during transaction; not exported to DA until accumulate.
- At accumulate, resolve shard via SSR, apply copy-on-write to create new shard version.
- New shard exported to DA with updated entry; ObjectRef in JAM State points to new version.
- Work package declares ObjectDependency on old version; enables parallelism via version tracking.

**Call Sites:**
1. **SSTORE opcode** ([eval/system.rs:298](../services/vendor/evm-interpreter/src/eval/system.rs#L298)) – Mutates contract storage using `handler.set_storage`.

### `set_transient_storage`
**Interface:** `fn set_transient_storage(&mut self, address: H160, index: H256, value: H256) -> Result<(), ExitError>`
**Description:** Writes a transient storage value (cleared after the transaction).
**JAM DA/State Handling:**
- Transient storage purely in-memory; never exported to DA or JAM State.
- Cleared at transaction boundaries; no persistence across transactions per EIP-1153.
- No ObjectRefs, no versioning, no dependencies; purely ephemeral scratch space.
- Gas cost ~100 for TSTORE/TLOAD; no DA import or export overhead.

**Call Sites:**
1. **TSTORE opcode** ([eval/system.rs:329](../services/vendor/evm-interpreter/src/eval/system.rs#L329)) – Updates transient storage via `handler.set_transient_storage`.

### `log`
**Interface:** `fn log(&mut self, log: Log) -> Result<(), ExitError>`
**Description:** Emits an EVM log with the provided topics and data.
**JAM DA/State Handling:**
- Logs accumulated in-memory during transaction; exported to DA receipt at accumulate.
- Receipts stored as separate DA objects indexed by transaction hash; not in JAM State directly.
- JAM State may hold receipt root hash for Merkle proofs; full logs in DA for availability.
- No impact on contract state versioning; logs are append-only output artifacts.

**Call Sites:**
1. **LOG0–LOG4 opcodes** ([eval/system.rs:362](../services/vendor/evm-interpreter/src/eval/system.rs#L362)) – Emit events by invoking `handler.log`.

### `mark_delete_reset`
**Interface:** `fn mark_delete_reset(&mut self, address: H160)`
**Description:** Flags an account for deletion and balance reset (SELFDESTRUCT semantics).
**JAM DA/State Handling:**
- Deletion tracked in-memory overlay; account balance transferred to beneficiary before marking.
- EIP-6780 restricts deletion to accounts created in same transaction; otherwise balance transfer only.
- At accumulate, deleted accounts have code object, storage SSR, all shards removed from DA.
- Balance/nonce entries in 0x01 precompile shards also cleared; 24000 gas refund credited once.

**Call Sites:**
1. **SELFDESTRUCT opcode** ([eval/system.rs:387](../services/vendor/evm-interpreter/src/eval/system.rs#L387)) – Marks the account for deletion via `handler.mark_delete_reset`.

### `mark_create`
**Interface:** `fn mark_create(&mut self, address: H160)`
**Description:** Records that an account was created in this transaction.
**JAM DA/State Handling:**
- Creation tracked in-memory overlay; address added to created set for EIP-6780 SELFDESTRUCT checks.
- At accumulate, new code object exported to DA; nonce initialized to 1 in 0x01 precompile shards.
- Empty SSR header created for new contract storage; initial balance written to 0x01 if value transferred.
- Created flag enables same-transaction deletion via SELFDESTRUCT per EIP-6780.

**Call Sites:**
1. **Create routine** ([invoker/routines.rs:122](../services/vendor/evm/src/standard/invoker/routines.rs#L122)) – Records the newly created account in the overlay.

### `reset_storage`
**Interface:** `fn reset_storage(&mut self, address: H160)`
**Description:** Clears all persistent storage for an account.
**JAM DA/State Handling:**
- Called before contract deployment to clear any pre-existing storage at CREATE address.
- Removes SSR header and all shard ObjectRefs from JAM State for the target contract.
- DA objects remain available for historical queries; new SSR created for fresh contract.
- EIP-7610 collision rules prevent resetting non-empty accounts; should only occur for empty targets.

**Call Sites:**
1. **Create routine** ([invoker/routines.rs:121](../services/vendor/evm/src/standard/invoker/routines.rs#L121)) – Clears the account's storage before deployment.

### `set_code`
**Interface:** `fn set_code(&mut self, address: H160, code: Vec<u8>, origin: SetCodeOrigin) -> Result<(), ExitError>`
**Description:** Installs new bytecode on an account.
**JAM DA/State Handling:**
- Bytecode cached in-memory overlay during transaction; exported to DA at accumulate.
- Code ObjectId = `[20B contract][1B kind=0x00][11B zero]`; exported as 4KB DA segments.
- JAM State stores ObjectRef with code hash and size; full bytecode in DA for availability.
- Code immutable after deployment; subsequent writes rejected (no code replacement in EVM).

**Call Sites:**
1. **Create deployment** ([invoker/routines.rs:248](../services/vendor/evm/src/standard/invoker/routines.rs#L248)) – Installs the returned bytecode via `handler.set_code`.

### `deposit`
**Interface:** `fn deposit(&mut self, target: H160, value: U256)`
**Description:** Credits value to the specified account.
**JAM DA/State Handling:**
- Balance updates cached in-memory overlay; written to 0x01 precompile shards at accumulate.
- Resolve shard via SSR: storage key = `keccak256(target_address)`, lookup ShardId from 0x01's SSR.
- Import shard, apply copy-on-write: update balance entry, export new shard version to DA.
- JAM State ObjectRef updated to point to new shard version; old version remains for dependencies.

**Call Sites:**
1. **Transaction refund** ([invoker/mod.rs:541](../services/vendor/evm/src/standard/invoker/mod.rs#L541)) – Deposits refunded gas into the caller's account.
2. **Coinbase reward** ([invoker/mod.rs:545](../services/vendor/evm/src/standard/invoker/mod.rs#L545)) – Credits the block producer with priority fees.
3. **Default `transfer`** ([runtime.rs:312](../services/vendor/evm-interpreter/src/runtime.rs#L312)) – Credits the recipient after withdrawing from the source.

### `withdrawal`
**Interface:** `fn withdrawal(&mut self, source: H160, value: U256) -> Result<(), ExitError>`
**Description:** Debits value from the specified account.
**JAM DA/State Handling:**
- Balance read from 0x01 precompile shards to verify sufficient funds before debit.
- Resolve shard via SSR: storage key = `keccak256(source_address)`, import shard from DA.
- Apply copy-on-write: deduct amount from balance entry, export updated shard version to DA.
- Insufficient balance causes transaction revert; balance check prevents overdraft.

**Call Sites:**
1. **Transaction initiation** ([invoker/mod.rs:366](../services/vendor/evm/src/standard/invoker/mod.rs#L366)) – Deducts the up-front gas fee from the caller.
2. **Default `transfer`** ([runtime.rs:311](../services/vendor/evm-interpreter/src/runtime.rs#L311)) – Withdraws value from the source before depositing to the target.

### `transfer`
**Interface:** `fn transfer(&mut self, transfer: Transfer) -> Result<(), ExitError>`
**Description:** Composite helper that withdraws from the source then deposits into the target.
**JAM DA/State Handling:**
- Atomically combines withdrawal from source and deposit to target via 0x01 precompile shards.
- May touch two separate shards if source and target map to different ShardIds in 0x01's SSR.
- Both shards updated via copy-on-write; work package depends on both old shard versions.
- Transfer failures (insufficient balance) revert transaction; no partial state updates.

**Call Sites:**
1. **Call routine** ([invoker/routines.rs:44](../services/vendor/evm/src/standard/invoker/routines.rs#L44)) – Executes any call-value transfer before entering the callee.
2. **Create routine** ([invoker/routines.rs:89](../services/vendor/evm/src/standard/invoker/routines.rs#L89)) – Moves the creation value from caller to new contract.
3. **SELFDESTRUCT opcode** ([eval/system.rs:381](../services/vendor/evm-interpreter/src/eval/system.rs#L381)) – Transfers the entire balance to the beneficiary.
4. **Default implementation** ([runtime.rs:311](../services/vendor/evm-interpreter/src/runtime.rs#L311)) – Provides the withdrawal-then-deposit behaviour.

### `inc_nonce`
**Interface:** `fn inc_nonce(&mut self, address: H160) -> Result<(), ExitError>`
**Description:** Increments an account's nonce by one.
**JAM DA/State Handling:**
- Nonce read from 0x01 precompile shards, incremented, written back at accumulate.
- Resolve shard via SSR: storage key = `keccak256(address || "nonce")`, import from DA.
- Apply copy-on-write: increment nonce entry, export updated shard version to DA.
- Critical for CREATE address derivation and replay protection; nonce=1 set for new contracts per EIP-161.

**Call Sites:**
1. **Transaction initiation** ([invoker/mod.rs:351](../services/vendor/evm/src/standard/invoker/mod.rs#L351)) – Bumps the caller's nonce at the start of a transaction.
2. **Create substack entry** ([invoker/mod.rs:748](../services/vendor/evm/src/standard/invoker/mod.rs#L748)) – Increments the caller nonce before deploying via a call trap.
3. **Create routine** ([invoker/routines.rs:109](../services/vendor/evm/src/standard/invoker/routines.rs#L109)) – Optionally increments the new contract's nonce per EIP-161.

### `is_hot`
**Interface:** `fn is_hot(&self, address: H160, index: Option<H256>) -> bool`
**Description:** Convenience method that simply returns `!is_cold`, so it has no dedicated call sites.
**Call Sites:**
- None – the default trait implementation simply inverts `is_cold` for convenience.

## Block-Level Persistence

### Interface
`finish_block(overlay: &mut OverlayedBackend<'_, MajikBackend>) -> Result<(), FlushError>` — illustrative orchestrator entry point responsible for draining the merged overlay diff into DA.

### Description
The overlay stack acts as the sole source of truth while a block is executing. Every transaction pushes a new `Substate`, records its reads/writes there, and either merges or discards that snapshot when the call frame exits. Until the block orchestrator explicitly walks the accumulated diff, no mutations reach the canonical `MajikBackend` maps or DA.

### JAM DA/State Handling
- **Within a block:**
  - Each transaction pushes a fresh `Substate` via `OverlayedBackend::push_substate`, records its reads/writes there, and merges with `pop_substate` only if the call/transaction succeeds.
  - Once merged, the parent `Substate` now exposes the committed values to subsequent transactions, so Transaction *N + 1* reads Transaction *N*'s writes directly from memory (no DA round-trip).
  - All intermediate data lives inside the overlay's `Substate` structures (see `services/vendor/evm/src/backend/overlayed.rs::Substate`), not in `MajikBackend`.
- **After the block:**
  - The orchestrator gathers the merged diff from the overlay (e.g., via the `Substate::into_diff()` helper) and performs a deterministic flush:
    1. **Storage slots:** call `MajikBackend::set_storage_value` for every modified `(address, key)` recorded in the diff.
    2. **Balances & nonces:** batch-update the 0x01 "account table" shards using the recorded balance/nonces map.
    3. **Code objects:** export any newly-created bytecode blobs (or code hashes) to DA and update the code ObjectRefs.
    4. **Shard versions:** for each touched shard, emit a new DA segment via copy-on-write and update the JAM State ObjectRef.
    5. **Receipts/logs:** write accumulated logs/receipts to their DA collection if required by the execution environment.
  - Because the flush happens once per block, redundant writes are collapsed automatically (only the last value per key leaves memory).
- **Failure handling:**
  - If a transaction reverts, its child `Substate` is simply dropped before merge (`pop_substate(MergeStrategy::Revert)`), restoring the parent snapshot. If the entire block fails validation, the orchestrator discards the top-level overlay without touching `MajikBackend` or DA; no additional rollback mechanism is required.
- **Nested call isolation:**
  - Nested calls rely on the same `push_substate`/`pop_substate` mechanics, so a reverting frame never contaminates its parent. Only successful frames merge their diffs upward, maintaining transactional semantics across the call stack.

### Call Sites
- Not a direct API; the block orchestrator is responsible for invoking the flush entry point after block execution and for discarding the accumulated overlay if validation fails.
