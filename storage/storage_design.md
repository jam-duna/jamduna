# StorageHub Architecture (Original vs Current)

This document is a **current-state** description with explicit contrast to the
original design, plus a wiring map from old calls to new ones.

## Original Architecture (Wrong)

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         StateDBStorage (GOD OBJECT)                             │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  ┌──────────────────────┐   ┌───────────────────────┐   ┌─────────────────────┐ │
│  │   JAM TRIE STUFF     │   │    EVM UBT STUFF      │   │   INFRASTRUCTURE    │ │
│  │   (C1-C16 state)     │   │  (EVM contract state) │   │                     │ │
│  ├──────────────────────┤   ├───────────────────────┤   ├─────────────────────┤ │
│  │ trieDB *MerkleTree   │   │ ubtState.treeStore    │   │ db *leveldb.DB      │ │
│  │ Root common.Hash     │   │ ubtState.canonicalRoot│   │ jamda types.JAMDA   │ │
│  │ stagedInserts map    │   │ activeRoot            │   │ telemetryClient     │ │
│  │ stagedDeletes map    │   │ pinnedStateRoot       │   │ nodeID              │ │
│  │ keys map             │   │ pinnedTree            │   │ logChan             │ │
│  │ stagedMutex          │   │ ubtReadLog            │   │ contexts...         │ │
│  └──────────────────────┘   │ ubtReadLogEnabled     │   └─────────────────────┘ │
│           │                 │ ubtReadLogMutex       │             │             │
│           │                 └───────────────────────┘             │             │
│           │                          │                            │             │
│  ┌────────┴──────────────────────────┴────────────────────────────┴──────────┐  │
│  │                                                                           │  │
│  │   ┌──────────────────────┐   ┌──────────────────────┐                     │  │
│  │   │   WITNESS CACHE      │   │  SERVICE STATE       │                     │  │
│  │   │   (Phase 4+ EVM)     │   │  (Multi-rollup)      │                     │  │
│  │   ├──────────────────────┤   ├──────────────────────┤                     │  │
│  │   │ Storageshard map     │   │ serviceUBTRoots map  │                     │  │
│  │   │ Code map             │   │ serviceJAMStateRoots │                     │  │
│  │   │ UBTProofs map        │   │ serviceBlockHashIndex│                     │  │
│  │   │ witnessMutex         │   │ latestRollupBlock    │                     │  │
│  │   └──────────────────────┘   │ mutex                │                     │  │
│  │                              └──────────────────────┘                     │  │
│  │                                                                           │  │
│  │   ┌──────────────────────┐                                                │  │
│  │   │  CHECKPOINT MGR      │   + isClone bool                               │  │
│  │   │  (UBT snapshots)     │   + mutex sync.RWMutex                         │  │
│  │   └──────────────────────┘   + memMap (unused?)                           │  │
│  │                                                                           │  │
│  └───────────────────────────────────────────────────────────────────────────┘  │
│                                                                                 │
│  PROBLEMS:                                                                      │
│  • 5+ different mutexes with unclear locking order                              │
│  • JAM trie root is INSTANCE state but should be SESSION state                  │
│  • UBT has "shared" pointer vs "per-instance" fields - confusing                │
│  • CloneTrieView() does magic pointer sharing                                   │
│  • 1800+ lines in storage.go alone                                              │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘

```

Condensed view:

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                         StateDBStorage (GOD OBJECT)                          │
├──────────────────────────────────────────────────────────────────────────────┤
│  trieDB *MerkleTree                                                          │
│  Root common.Hash                                                            │
│  stagedInserts / stagedDeletes / keys                                        │
│                                                                              │
│  UBT state (treeStore, canonicalRoot, activeRoot, pinnedTree, readLog, ...)  │
│  Service state (service roots, block index, latest rollup block)             │
│  Witness cache                                                               │
│  Persistence (LevelDB) + JAMDA + telemetry + misc                            │
└──────────────────────────────────────────────────────────────────────────────┘
```

Why it was wrong:
- Root lived at the **storage** level, not the **session/execution** level.
- Shared vs per-execution state were interleaved in one struct.
- Copy/clone semantics were unclear; isolation relied on pointer sharing.
- Locking order and ownership were hard to reason about.

## Root Placement: Before vs After

```
ORIGINAL (WRONG):
┌─────────────────────────────────────┐
│       StateDBStorage (old)          │
│  ┌─────────────────────────────┐    │
│  │  Root common.Hash  ◄────────│────│── ROOT IS AT STORAGE LEVEL
│  │  trieDB *MerkleTree         │    │   (persists across all operations)
│  │  stagedInserts              │    │
│  └─────────────────────────────┘    │
└─────────────────────────────────────┘

MODIFIED (CORRECT):
┌─────────────────────────────────────┐     ┌──────────────────────────────────┐
│      StorageHub (refactored)        │     │        JAMTrieSession            │
│  ┌─────────────────────────────┐    │     │  ┌─────────────────────────┐     │
│  │  jamBackend (shared infra)  │    │     │  │  root common.Hash       │     │
│  │  - no root here!            │    │     │  │  stagedInserts          │     │
│  └─────────────────────────────┘    │     │  │  trie (isolated view)   │     │
│                                     │     │  └─────────────────────────┘     │
│  jamSession ────────────────────────┼────►│  (session owns root)             │
│  (points to session)                │     │  Methods:                        │
└─────────────────────────────────────┘     │  Finish() → commits, returns root│
                                            │  Rollback() → discards changes   │
                                            └──────────────────────────────────┘
```

This mirrors Rust's Session pattern where Session::finish() returns the new root.
The storage layer never holds mutable root state — sessions do.

## Visual: CloneTrieView Confusion (Original Problem)

```
                    ORIGINAL StateDBStorage
                    ┌──────────────────────────────┐
                    │ trieDB ───────────────────┐  │
                    │ Root                      │  │
                    │ stagedInserts             │  │
                    │                           │  │
                    │ ubtState ──────────────┐  │  │
                    │ activeRoot             │  │  │
                    │ pinnedTree             │  │  │
                    │ ubtReadLog             │  │  │
                    │                        │  │  │
                    │ serviceState ───────┐  │  │  │
                    │ db ─────────────┐   │  │  │  │
                    └─────────────────│───│──│──│──┘
                                      │   │  │  │
                           CloneTrieView()│  │  │
                              │           │  │  │
                    ┌─────────▼───────────│──│──│──┐
                    │        CLONE        │  │  │  │
                    │                     │  │  │  │
                    │ trieDB.CloneView()──┘  │  │  │  ← NEW (isolated trie)
                    │ Root (copied)          │  │  │
                    │ stagedInserts (new)    │  │  │
                    │                        │  │  │
                    │ ubtState ──────────────┘  │  │  ← SAME POINTER (shared)
                    │ activeRoot (new empty)    │  │  ← NEW (per-instance)
                    │ pinnedTree (inherited)────┘  │  ← COPIED (confusing)
                    │ ubtReadLog (new empty)       │  ← NEW (per-instance)
                    │                              │
                    │ serviceState ────────────────┘  ← SAME POINTER (shared)
                    │ db ──────────────────────────┘  ← SAME POINTER (shared)
                    │                              │
                    │ isSessionCopy = true         │
                    └──────────────────────────────┘

    WHAT'S SHARED:              WHAT'S ISOLATED:          WHAT'S INHERITED:
    • ubtState.treeStore        • trieDB view             • pinnedStateRoot (copy)
    • ubtState.mutex            • trieDB.Root             • pinnedTree (pointer)
    • serviceState.*            • stagedInserts/deletes
    • db (LevelDB)              • activeRoot
                                • ubtReadLog
```

## Modified Architecture (Current)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ LAYER 1: PERSISTENCE                                                         │
│  PersistenceStore                                                            │
│  db *leveldb.DB                                                              │
│  Get/Put/Delete/NewBatch                                                     │
└──────────────────────────────────────────────────────────────────────────────┘
                     │
                     ▼
┌───────────────────────────────┐   ┌───────────────────────────────┐   ┌───────────────────────────────┐
│ JAMTrieBackend                │   │ UBTTreeManager                │   │ ServiceStateManager           │
│ - writeBatch (in-memory)      │   │ - treeStore map               │   │ - service block indices       │
│ - batchMutex                  │   │ - canonicalRoot               │   │ - persisted blocks in KV      │
│ - db *PersistenceStore        │   │ - mutex                       │   │ - mutex                       │
└───────────────────────────────┘   └───────────────────────────────┘   └───────────────────────────────┘
                     │
                     ▼
┌───────────────────────────────┐   ┌───────────────────────────────┐
│ JAMTrieSession                │   │ UBTExecContext                │
│ - root                        │   │ - activeRoot / pinnedRoot     │
│ - staged inserts/deletes      │   │ - readLog + readLogEnabled    │
│ - uses JAMTrieBackend         │   │ - manager: UBTTreeManager     │
│ - CopySession()               │   │ - Clone()                     │
└───────────────────────────────┘   └───────────────────────────────┘

┌───────────────────────────────────────────────────────────────────────────────┐
│ StorageHub (implements JAMStorage + EVMJAMStorage)                            │
│  Persist: PersistenceStore                                                    │
│  Shared: {UBTTreeManager, ServiceStateManager}                                │
│  Session: {JAMTrieSession, UBTExecContext}                                    │
│  Aux: WitnessCache, CheckpointManager                                         │
│  Infra: JAMDA, Telemetry, NodeID                                              │
│  Root/keys/context flags                                                      │
│                                                                               │
│ NewSession() returns a StorageSession backed by a new StorageHub session view │
└───────────────────────────────────────────────────────────────────────────────┘
```

Notes:
- `JAMTrieBackend` currently stores trie nodes in-memory (`writeBatch`).
  `PersistenceStore` is wired but node persistence is not yet enabled.
- `UBTTreeManager` and `ServiceStateManager` are shared across sessions.
- `JAMTrieSession` and `UBTExecContext` are per-session/per-execution.

## Visual: NewSession Isolation (Current)

```
                  ORIGINAL StorageHub (JAMStorage)
                  ┌──────────────────────────────┐
                  │ Persist: PersistenceStore    │
                  │ Shared: {UBT, Service}       │
                  │ Session: {JAM, UBT}          │
                  │ Aux: Witness/Checkpoint      │
                  │ Infra: JAMDA/Telemetry       │
                  └───────────────┬──────────────┘
                                  │
                              NewSession()
                                  │
                  ┌───────────────▼──────────────┐
                  │     StorageSession view      │
                  │  (backed by StorageHub)      │
                  ├──────────────────────────────┤
                  │ Persist: SAME pointer        │
                  │ Shared:  SAME pointers       │
                  │ Session: NEW JAM session     │
                  │          NEW UBT context     │
                  │ Aux: new Witness cache       │
                  │ Checkpoint: shared           │
                  └──────────────────────────────┘

    SHARED (stable):     SESSION-LOCAL:            INHERITED:
    • PersistenceStore   • JAMTrieSession root     • UBTExecContext pinnedRoot
    • UBTTreeManager     • JAM staged ops          • UBTExecContext readLogEnabled
    • ServiceStateMgr    • UBTExecContext readLog
```

## Wiring: Old → New

### Naming and API Mapping

| Old | New | Notes |
|-----|-----|------|
| `StateDBStorage` | `StorageHub` | Concrete implementation remains the same role |
| `NewStateDBStorage()` | `NewStorageHub()` | Constructor rename |
| `CloneTrieView()` | `NewSession()` | Session creation (now returns `StorageSession`) |
| `MerkleTree.CloneView()` | `JAMTrieSession.CopySession()` | Internal session copy |

### Methods Extracted from StorageHub

| Current Method | New Location | Notes |
|----------------|--------------|-------|
| `ReadRawKV(key []byte)` | `PersistenceStore.Get(key)` | Returns (value, found, error) |
| `WriteRawKV(key, value []byte)` | `PersistenceStore.Put(key, value)` | Direct delegation |
| `DeleteRawK(key []byte)` | `PersistenceStore.Delete(key)` | Direct delegation |
| `ReadRawKVWithPrefix(prefix)` | `PersistenceStore.GetWithPrefix(prefix)` | Iterator-based scan |
| `ReadKV(key common.Hash)` | `PersistenceStore.GetHash(key)` | Convenience wrapper |
| `WriteKV(key common.Hash, value)` | `PersistenceStore.PutHash(key)` | Convenience wrapper |
| `DeleteK(key common.Hash)` | `PersistenceStore.DeleteHash(key)` | Convenience wrapper |
| `CloseDB()` | `PersistenceStore.Close()` | Direct delegation |

### Session Wiring (Old → New)

- **StateDB.NewStateDBFromStateRoot**
  - Old: `sdb.CloneTrieView()`
  - New: `sdb.NewSession()`
- **StateDB.Copy / CopyForPhase2**
  - Old: `sdb.CloneTrieView()`
  - New: `sdb.NewSession()`
- **Phase 2 isolation**
  - Old: session copy + shared read log
  - New: `NewSession()` creates isolated `UBTExecContext` (read log per session)

## Current Interfaces

**StorageSession**
- Core trie ops: `Insert`, `Get`, `Delete`, `Trace`, `Flush`
- Root management: `GetRoot`, `SetRoot`, `OverlayRoot`, `ClearStagedOps`
- Service ops: C1–C16 storage + preimage access
- KV ops: `ReadRawKV`, `WriteRawKV`, `ReadKV`
- DA fetch + telemetry: `FetchJAMDASegments`, `GetTelemetryClient`
- Session creation + lifecycle: `NewSession`, `Close`

**JAMStorage**
- Embeds `StorageSession`
- Adds block storage and extended KV helpers (`ReadRawKVWithPrefix`, block indexes)

**EVMJAMStorage**
- Extends `JAMStorage` with UBT + EVM helpers (roots, witnesses, block cache)

**StorageHub**

- Concrete implementation of `JAMStorage` and `EVMJAMStorage`
- `NewSession()` returns a `StorageSession` (the underlying `*StorageHub` supports
  type assertion to `JAMStorage` or `EVMJAMStorage` when full interface access is needed)

## Current Call-Site Usage

**node**
- Creates the concrete `StorageHub` via `NewStorageHub(...)` and exposes it as
  `JAMStorage` (e.g., `n.GetStorage()`), used for JAMDA and telemetry access.

**StateDB**
- Stores `StorageSession` internally (`sdb` field).
- `NewStateDBFromStateRoot` and `Copy` call `NewSession()` to isolate trie root.
- `CopyForPhase2` uses `NewSession()` to isolate UBT read logs.
- `GetStorage()` returns `JAMStorage` for compatibility, while
  `GetStorageSession()` is the preferred accessor for session-only use.

**evm-builder**
- Uses `GetStorage()` and type-asserts to `EVMJAMStorage` (guarded) for
  EVM-specific operations (UBT roots, witnesses, block cache).

**duna_target / duna_fuzzer / duna_bench / duna_replay**
- No direct `StorageHub` usage. They exercise state transitions through `StateDB`.

**TestFuzzTraceSequential / TestTracesInterpreter**
- Use `StateDB` which now operates on `StorageSession` and `NewSession()` for
  isolation.

### Code Snippets

**1. node – Returns `JAMStorage`** (`node/node_ecc.go:9-17`)

```go
func (n *NodeContent) GetStorage() (types.JAMStorage, error) {
    if n == nil {
        return nil, fmt.Errorf("Node Not initiated")
    }
    if n.store == nil {
        return nil, fmt.Errorf("Node Store Not initiated")
    }
    return n.store, nil
}
```

**2. StateDB – Stores `StorageSession`, uses `NewSession()`**

Storage field (`statedb/statedb.go:50`):
```go
type StateDB struct {
    // ...
    sdb types.StorageSession  // Can hold JAMStorage or session-only mock
    // ...
}
```

Accessor methods (`statedb/statedb.go:308-321`):
```go
// GetStorage returns sdb as JAMStorage (panics if not supported)
func (s *StateDB) GetStorage() types.JAMStorage {
    jam, ok := s.sdb.(types.JAMStorage)
    if !ok {
        panic("StateDB.sdb does not implement JAMStorage")
    }
    return jam
}

// TryGetStorage returns (storage, true) if sdb implements JAMStorage
func (s *StateDB) TryGetStorage() (types.JAMStorage, bool) {
    jam, ok := s.sdb.(types.JAMStorage)
    return jam, ok
}
```

`NewSession()` usage for isolation (`statedb/statedb.go:628, 710, 767`):
```go
// NewStateDBFromStateRoot - creates isolated session for concurrent safety
func NewStateDBFromStateRoot(stateRoot common.Hash, sdb types.JAMStorage) (*StateDB, error) {
    // Create an isolated session for this StateDB.
    isolatedSdb := sdb.NewSession()
    recoveredStateDB = newEmptyStateDB(isolatedSdb)
    // ...
}

// StateDB.Copy() - isolated copy for fork branches
func (s *StateDB) Copy() (newStateDB *StateDB) {
    // Create an isolated session for the copy.
    isolatedSdb := s.sdb.NewSession()
    newStateDB = &StateDB{
        sdb: isolatedSdb,
        // ...
    }
}

// CopyForPhase2() - lightweight copy for Phase 2 witness builds
func (s *StateDB) CopyForPhase2() *StateDB {
    isolatedSdb := s.sdb.NewSession()
    return &StateDB{
        sdb: isolatedSdb,
        // ...
    }
}
```

**3. evm-builder – Type asserts to `EVMJAMStorage`** (`builder/evm/rpc/rollup.go:570-578`)

```go
// Get storage for state management
jamStorage := r.GetStateDB().GetStorage()
evmStorage, ok := jamStorage.(types.EVMJAMStorage)
if !ok {
    return fmt.Errorf("storage does not implement EVMJAMStorage")
}

// === ROOT-FIRST STATE MODEL ===
preRoot := evmStorage.GetCanonicalRoot()
```

Also in `statedb/pvm_hostfunctions.go:53-58`:
```go
if env, ok := hostenv.(interface {
    GetStorage() types.JAMStorage
}); ok {
    if evmStorage, ok := env.GetStorage().(types.EVMJAMStorage); ok {
        return evmStorage, true
    }
}
```

**4. duna_* tools** – No direct storage access

These tools operate through StateDB and do not access storage directly:
- Use `StateDB.SetRoot()`, `StateDB.Copy()`, etc.
- StateDB handles session creation internally via `NewSession()`

**5. TestFuzzTraceSequential / TestTracesInterpreter** (`statedb/importblock_test.go`)

```go
func TestFuzzTraceSequential(t *testing.T) {
    // Initialize storage
    testDir := t.TempDir()
    test_storage, err := initStorage(testDir)  // Returns *StorageHub
    defer test_storage.Close()

    // Create StateDB from storage (uses JAMStorage interface)
    stateDB, err := NewStateDBFromStateKeyVals(test_storage, &StateKeyVals{...})
    // All subsequent operations go through StateDB's session isolation
}
```

### Summary Table

| Call-site | Interface Used | Session Handling |
|-----------|----------------|------------------|
| **node** | `JAMStorage` (returned) | Owns the StorageHub |
| **StateDB** | `StorageSession` (stored) | Calls `NewSession()` for isolation |
| **evm-builder** | `EVMJAMStorage` (asserted) | Safe assertion with `ok` check |
| **duna_\*** | None directly | Access via StateDB |
| **Tests** | `JAMStorage` (init) | Via StateDB session isolation |
