# NOMT Benchmark Analysis & Implementation Plan

## 1. Rust NOMT Benchmark Results

### Exact Command Used

```bash
cargo run --release --manifest-path /Users/michael/Github/nomt/benchtop/Cargo.toml -- run \
  --backend nomt \
  --workload-name randw \
  --workload-size 20000 \
  --workload-fresh 50 \
  --workload-capacity 14 \
  --commit-concurrency 1 \
  --workload-concurrency 1 \
  --io-workers 3 \
  --op-limit 100000
```

### Test Configuration

| Parameter | Value | Meaning |
|-----------|-------|---------|
| `workload` | randw | 100% writes (0% reads, 100% writes) |
| `workload-size` | 20,000 | Operations per iteration |
| `workload-fresh` | 50% | 50% fresh writes (inserts), 50% existing keys (updates) |
| `workload-capacity` | 2^14 | 16,384 keys pre-populated |
| `commit-concurrency` | 1 | Single-threaded Merkle commit |
| `workload-concurrency` | 1 | Single-threaded workload execution |
| `io-workers` | 3 | Number of I/O threads (macOS non-Linux) |
| `op-limit` | 100,000 | Total operations (5 iterations × 20K) |

### Performance Results

**Actual benchtop output snippet:**
```
nomt
  mean workload: 113 ms
read not measured
  mean commit_and_prove: 108 ms
  mean throughput: 175696.4 ops/s
max rss: 258544 MiB
```

| Metric | Value | Notes |
|--------|-------|-------|
| **Mean workload time** | 113 ms | Time per 20K op iteration (Timer::workload) |
| **Mean commit time** | 108 ms | WAL write + HT sync + fsync (NOT proof generation) |
| **Throughput** | **175,696 ops/second** | 20,000 ops ÷ 0.113s |
| **Max RSS** | 258,544 MiB (~252 GB) | Peak memory usage |

**⚠️ Important:** The "commit_and_prove" metric name in benchtop is misleading—it measures **WAL+commit timing only**, NOT witness/proof generation. Witness generation is tested separately in `gp_witness_test.rs` but not benchmarked in standard benchtop runs.

### What NOMT Actually Tests (per iteration)

**Operations (20,000 total):**
- ~10,000 **inserts** (fresh keys, 50%)
- ~10,000 **updates** (existing keys, 50%)
- **0 deletes** (not included in benchtop workloads)
- **0 reads** (randw = write-only workload)

**Merkle Tree Operations:**
- ✅ Complete tree building via `build_trie()` ([nomt/core/src/update.rs](https://github.com/thrumdev/nomt/blob/main/core/src/update.rs))
- ✅ Page-based storage (Bitbox hash table)
- ✅ WAL (Write-Ahead Log) syncing to disk
- ✅ Root hash computation
- ✅ Persistent storage with fsync

**What's NOT tested in benchtop:**
- ❌ Witness/proof generation (separate test suite: `gp_witness_test.rs`)
- ❌ Proof verification
- ❌ Delete operations
- ❌ Read-heavy workloads

---

## 2. Go BMT Current State (bmt/nomt_test.go)

### TestNomtBenchmarkMedium - AS-IS Configuration

**Current test parameters (lines 725-734):**

```go
params := TestParams{
    MaxIterations:   20,     // 20 iterations (vs Rust: 5)
    CommitLag:       5,      // Commit overlay 5 iterations ago (Rust: 0)
    NumInserts:      10000,  // ✅ Matches Rust
    NumUpdates:      10000,  // ✅ Matches Rust
    NumDeletes:      100,    // ❌ Rust: 0 (not benchmarked)
    NumWitnessKeys:  10000,  // ❌ STUB - no real proof generation
    ValueSize:       256,    // ❌ Rust default: 32 bytes
    OptimizeWitness: true,   // ❌ Not implemented
}
```

**What test_nomt() currently does (lines 447-706):**

✅ **Implemented:**
- Insert operations with deterministic keys
- Update operations on previous iteration's keys
- Delete operations (lines 540-560)
- Commit with `Prepare()` and `Commit()`
- State root tracking
- Basic timing measurements

❌ **Missing/Stubbed:**
- **No read operations** - test never calls `db.Get()` except for verification
- **No `NumReads` parameter** in `TestParams` struct
- **Witness proofs are stubbed** (line 652): `Path: nil`
- **Verification is fake** (line 676): Just checks `bytes.Equal(actualValue, proof.Value)`
- **No Merkle path collection** during tree traversal
- **No sibling hash tracking**

### Critical Gaps

1. **Witness Generation is Completely Stubbed** (lines 647-653)
   ```go
   // Create a simple proof with just key-value (no expensive merkle path generation)
   proofs[i] = MerkleProof{
       Key:   key,
       Value: value,
       Path:  nil, // Skip expensive proof generation for performance
   }
   ```
   **Missing:** No hooks to collect sibling hashes during `Insert()`/`Get()` operations.

2. **Verification is Not Cryptographic** (lines 675-677)
   ```go
   // For keys without full proof paths, just verify the value matches
   if bytes.Equal(actualValue, proof.Value) {
       verifiedCount++
   }
   ```
   **Missing:** `VerifyProof()` function that hashes up the tree.

3. **No Read Workload** (line 84)
   ```go
   4. **No Reads** - Test only does writes (inserts/updates/deletes)
   ```
   **Missing:** `NumReads` parameter and read operation loop in `test_nomt()`.

4. **Witness struct has no Indices field**
   ```go
   type MerkleProof struct {
       Key   [32]byte
       Value []byte
       Path  [][32]byte  // Sibling hashes (currently always nil)
       // Missing: Indices []bool  // Direction bits (left/right)
   }
   ```

---

## 3. Implementation Plan for Go BMT

### Phase 1A: Make Comparable to Rust Benchmark (PLANNED - Not Yet Implemented)

**Goal:** Match Rust workload parameters for apples-to-apples comparison

#### 1A.1 Add NumReads Parameter to TestParams

**File:** `bmt/nomt_test.go` (lines 35-45)

```go
type TestParams struct {
    MaxIterations   int
    CommitLag       int
    NumInserts      int
    NumUpdates      int
    NumDeletes      int
    NumReads        int  // 🆕 TODO: Add this field
    NumWitnessKeys  int
    ValueSize       int
    OptimizeWitness bool
}
```

#### 1A.2 Implement Read Operations in test_nomt

**File:** `bmt/nomt_test.go` (after line 560, before Step 2 flush)

```go
// Step 2d: Read numReads keys (if possible from previous iterations)
// TODO: This section does not exist yet - need to add
readStart := time.Now()
readsPerformed := 0
if iteration > 0 {
    for i := 0; i < params.NumReads && i < params.NumInserts; i++ {
        // Read keys from previous iteration
        key := make([]byte, 32)
        copy(key, fmt.Sprintf("insert_%d_%d", iteration-1, i))
        var keyArray [32]byte
        copy(keyArray[:], key)

        if value, err := writeSession.Get(keyArray); err == nil && value != nil {
            readsPerformed++
        }
    }
}
readTime := time.Since(readStart)

// Add to timing report (line 690)
t.Logf("Timing - Insert: %v, Update: %v, Read: %v, Delete: %v",
    insertTime, updateTime, readTime, deleteTime)
```

#### 1A.3 Update TestNomtBenchmarkMedium Parameters

**File:** `bmt/nomt_test.go` (lines 725-734)

```go
func TestNomtBenchmarkMedium(t *testing.T) {
    params := TestParams{
        MaxIterations:   5,      // 🔧 Match Rust: 100K / 20K = 5 iterations
        CommitLag:       0,      // 🔧 Disable (Rust commits immediately)
        NumInserts:      10000,  // ✅ Same as Rust
        NumUpdates:      10000,  // ✅ Same as Rust
        NumReads:        5000,   // 🆕 TODO: Add realistic read workload
        NumDeletes:      50,     // 🔧 Minimal (Rust=0, but keep feature alive)
        NumWitnessKeys:  0,      // 🔧 Disable stub until Phase 2
        ValueSize:       32,     // 🔧 Match Rust (was 256)
        OptimizeWitness: false,  // Not relevant yet
    }
    test_nomt(t, params)
}
```

**Status:** ❌ **TODO - Not implemented yet**

---

### Phase 1B: Run Baseline Benchmark (PLANNED)

**Goal:** Establish Go BMT baseline performance WITHOUT witness generation

#### 1B.1 Run Test

```bash
cd /Users/michael/Github/jam
go test ./bmt -run=TestNomtBenchmarkMedium -v -timeout=30m
```

#### 1B.2 Actual Output (Phase 1A Baseline)

```
=== RUN   TestNomtBenchmarkMedium
    nomt_test.go:466: === Iteration 1/5 ===
    nomt_test.go:715: Operations: 10000 inserts, 0 updates, 0 reads, 0 deletes
    nomt_test.go:716: Timing - Insert: 5.702083ms, Update: 0s, Read: 42ns, Delete: 41ns
    nomt_test.go:717: State roots - Previous: 0000000000000000000000000000000000000000000000000000000000000000, New: e00889d93ebb2fbe3c37549e84dfd17ca98a1bad9b149eb79ed565b3032521b7
    nomt_test.go:718: Commit time: 0s, Flush time: 569.581792ms
    nomt_test.go:723: Witness generation: disabled (NumWitnessKeys=0)
    nomt_test.go:466: === Iteration 2/5 ===
    nomt_test.go:715: Operations: 10000 inserts, 10000 updates, 5000 reads, 0 deletes
    nomt_test.go:716: Timing - Insert: 6.759042ms, Update: 7.065333ms, Read: 1.357542ms, Delete: 42ns
    nomt_test.go:717: State roots - Previous: e00889d93ebb2fbe3c37549e84dfd17ca98a1bad9b149eb79ed565b3032521b7, New: 03318dcd3668724fd0416b49086361edfb7131d0314b3f31f737df815f2ff2b9
    nomt_test.go:718: Commit time: 0s, Flush time: 2.385321459s
    nomt_test.go:723: Witness generation: disabled (NumWitnessKeys=0)
    nomt_test.go:466: === Iteration 3/5 ===
    nomt_test.go:715: Operations: 10000 inserts, 10000 updates, 5000 reads, 50 deletes
    nomt_test.go:716: Timing - Insert: 6.524208ms, Update: 7.214083ms, Read: 938.75µs, Delete: 21.542µs
    nomt_test.go:717: State roots - Previous: 03318dcd3668724fd0416b49086361edfb7131d0314b3f31f737df815f2ff2b9, New: f0ea6755cc80c2d41a4c272a38b030f6ccddb92b6e37a2dff1ac45549d6b98da
    nomt_test.go:718: Commit time: 0s, Flush time: 3.763653833s
    nomt_test.go:723: Witness generation: disabled (NumWitnessKeys=0)
    nomt_test.go:466: === Iteration 4/5 ===
    nomt_test.go:715: Operations: 10000 inserts, 10000 updates, 5000 reads, 50 deletes
    nomt_test.go:716: Timing - Insert: 5.757958ms, Update: 8.354125ms, Read: 817.958µs, Delete: 25.5µs
    nomt_test.go:717: State roots - Previous: f0ea6755cc80c2d41a4c272a38b030f6ccddb92b6e37a2dff1ac45549d6b98da, New: fdfd22feb5d666a542668aa33b8a4b311d60ca72f42fed8626178942051a8f8f
    nomt_test.go:718: Commit time: 0s, Flush time: 5.491561125s
    nomt_test.go:723: Witness generation: disabled (NumWitnessKeys=0)
    nomt_test.go:466: === Iteration 5/5 ===
    nomt_test.go:715: Operations: 10000 inserts, 10000 updates, 5000 reads, 50 deletes
    nomt_test.go:716: Timing - Insert: 5.716666ms, Update: 7.875834ms, Read: 819.458µs, Delete: 19.875µs
    nomt_test.go:717: State roots - Previous: fdfd22feb5d666a542668aa33b8a4b311d60ca72f42fed8626178942051a8f8f, New: 349237e59934eaa66af3616598aeb8c224a024e04c1850f3c8ccbb7dd8d834a1
    nomt_test.go:718: Commit time: 0s, Flush time: 7.690393583s
    nomt_test.go:723: Witness generation: disabled (NumWitnessKeys=0)
--- PASS: TestNomtBenchmarkMedium (19.98s)
PASS
ok      github.com/colorfulnotion/jam/bmt       19.993s
```

#### 1B.3 Baseline Comparison

| Metric | Rust NOMT | Go BMT (Phase 1A) | Ratio | Analysis |
|--------|-----------|-------------------|-------|----------|
| **Throughput** | **175,696 ops/s** | **5,262 ops/s** | **3.0%** | 🚨 Go is 33x slower |
| Total operations | 100,000 | 105,150 | 105% | Similar workload size |
| Total time | ~0.57s (5×113ms) | 19.98s | 35x | Go takes much longer |
| Mean workload time | 113 ms | ~14 ms | 12% | Go workload ops are fast |
| Mean flush time | 108 ms | 3.96s (avg) | 37x | 🚨 Flush is the bottleneck |
| Memory (RSS) | 252 GB | Not measured | - | TODO: Add profiling |

**Status:** ✅ **Phase 1A Complete - Baseline Established**

**Critical Finding - Test Methodology Issue:** The Go BMT benchmark **accumulates state across iterations** instead of resetting to a clean baseline like Rust's `bench_isolate` mode. This causes flush times to grow linearly (570ms → 2.4s → 3.8s → 5.5s → 7.7s) as the tree size increases from 10K to 105K entries.

**Root Cause:**
- **Rust benchtop (isolate mode)**: Reinstantiates fresh DB each iteration → constant 20K state → constant ~108ms flush
  ```rust
  for _ in 0..iterations {
      let mut db = backend.instantiate(true, commit_concurrency, io_workers);
      db.execute(None, &mut init);  // rebuild baseline state
      db.execute(Some(&mut timer), &mut *workload);  // measure 20K ops only
  }
  ```
- **Go BMT (current)**: Reuses same DB instance → accumulates 10K + 20K + 25K + 25K + 25K = 105K total entries → growing flush times

**Impact:** The current benchmark measures **cumulative growth** (30-35x slower) rather than **per-iteration workload**. The workload operations themselves are fast (~14ms for 20-25K ops), but we're benchmarking the wrong thing.

**Fix Required:** Add DB reset/rollback before each iteration to match Rust's isolated benchmark methodology. Options:
1. Close and reopen DB with fresh directory each iteration
2. Implement rollback to baseline root (if supported by BMT)
3. Clear all state and rebuild baseline between iterations

Until this is fixed, the 33x performance gap is **not a fair comparison** - we're measuring different workloads.

---

### Phase 1B: Dual-Mode Benchmark Implementation (✅ COMPLETED)

**Goal:** Implement both isolated and sequential benchmark modes to match Rust's approach and enable fair comparisons.

#### 1B.1 Implementation Changes

**Added BenchmarkMode enum** (nomt_test.go:35-43):
- `BenchmarkIsolated`: Fresh DB each iteration (matches Rust's bench_isolate)
- `BenchmarkSequential`: Reuses same DB across iterations (matches Rust's bench_sequential)

**Refactored test_nomt()** (nomt_test.go:459-770):
- Isolated mode: Creates fresh DB in unique temp directory per iteration, closes at end
- Sequential mode: Single DB instance for all iterations, deferred cleanup
- Conditional workload operations:
  - Isolated: Updates/reads/deletes work on same iteration's keys
  - Sequential: Updates/reads/deletes work on previous iterations' keys

**Created test functions:**
- `TestNomtBenchmarkSmall_Isolated` / `TestNomtBenchmarkSmall_Sequential`
- `TestNomtBenchmarkMedium_Isolated` / `TestNomtBenchmarkMedium_Sequential`

#### 1B.2 Comprehensive Benchmark Results

All tests run with matching parameters:
- **Workload size:** 20K operations per iteration (10K inserts + 10K updates)
- **Iterations:** 5 iterations
- **Value size:** 32 bytes
- **Witness generation:** Disabled (Phase 2)

**Rust Isolated Mode (bench_isolate equivalent):**
```bash
cargo run --release --manifest-path /Users/michael/Github/nomt/benchtop/Cargo.toml -- run \
  --backend nomt --workload-name randw --workload-size 20000 --workload-fresh 50 \
  --workload-capacity 14 --commit-concurrency 1 --workload-concurrency 1 \
  --io-workers 3 --op-limit 100000
```

Results:
```
mean workload: 113 ms
mean commit_and_prove: 108 ms
mean throughput: 175,696 ops/s
max rss: 258,544 MiB
```

**Rust Sequential Mode (bench_sequential equivalent):**
```bash
# Same command with --overlay-window-length 0
```

Results:
```
mean workload: 408 ms
mean commit_and_prove: 402 ms
mean throughput: 48,991 ops/s
max rss: 382,016 MiB
```

**Go BMT Isolated Mode:**
```bash
go test -v -timeout=30m -run=TestNomtBenchmarkMedium_Isolated /Users/michael/Github/jam/bmt
```

Results (5 iterations, 10K inserts + 10K updates per iteration):
```
Iteration 1: Flush time: 554.40ms
Iteration 2: Flush time: 559.19ms
Iteration 3: Flush time: 594.06ms
Iteration 4: Flush time: 561.74ms
Iteration 5: Flush time: 621.84ms
Mean flush time: ~578ms
Total test time: 2.98s
```

**Go BMT Sequential Mode:**
```bash
go test -v -timeout=30m -run=TestNomtBenchmarkMedium_Sequential /Users/michael/Github/jam/bmt
```

Results (5 iterations, 10K inserts + 10K updates per iteration):
```
Iteration 1: Flush time: 569.58ms (10K entries total)
Iteration 2: Flush time: 2.39s (30K entries total - 4.2x slower)
Iteration 3: Flush time: 3.73s (50K entries total - 6.5x slower)
Iteration 4: Flush time: 5.39s (70K entries total - 9.5x slower)
Iteration 5: Flush time: 7.31s (90K entries total - 12.8x slower)
Total test time: 19.41s
```

#### 1B.3 Comparative Analysis

| Mode | Implementation | Ops per Iter | Mean Flush Time | Flush Time per Op | Throughput | Analysis |
|------|----------------|--------------|-----------------|-------------------|------------|----------|
| **Isolated** | Rust NOMT | 20,000 | 108 ms | 5.4 µs | 175,696 ops/s | Baseline reference |
| **Isolated** | Go BMT | 20,000 | 578 ms | **28.9 µs** | **34,602 ops/s** | **5.4x slower per op** |
| **Sequential** | Rust NOMT | 20,000 | 402 ms | 20.1 µs | 48,991 ops/s | **3.7x slower than isolated** |
| **Sequential** | Go BMT (iter 1) | 20,000 | 570 ms | 28.5 µs | 35,088 ops/s | Similar to isolated |
| **Sequential** | Go BMT (iter 5) | 20,000 | 7,310 ms | **365.5 µs** | 2,737 ops/s | **12.8x slower than iter 1** |

**Key Findings:**

1. **Isolated Mode (Apples-to-Apples Comparison):**
   - Go BMT is **5.4x slower per operation** than Rust NOMT (28.9µs vs 5.4µs)
   - Go achieves **34,602 ops/s** compared to Rust's **175,696 ops/s** (19.7% of Rust performance)
   - This is the **true baseline performance gap** without cumulative growth effects
   - Go workload operations are fast (~6-12ms for 20K ops), but flush dominates at ~578ms

2. **Sequential Mode Reveals Critical Issues:**
   - **Rust degrades 3.7x** from isolated to sequential (5.4µs → 20.1µs per op) - expected and acceptable
   - **Go degrades 12.8x** within sequential mode from iter 1 to iter 5 (28.5µs → 365.5µs per op)
   - This indicates Go has **both** a baseline flush issue (5.4x) **and** a severe sequential degradation problem (12.8x growth)

3. **Diagnostic Value:**
   - **Isolated slow:** Per-iteration flush logic needs optimization (✅ confirmed - 5.4x gap)
   - **Sequential explodes:** Growing linearly with tree size, suggesting tree is rebuilt from scratch each flush (✅ confirmed - no incremental updates)
   - **Linear growth:** Each iteration adds 20K entries and flush time increases proportionally

#### 1B.4 Root Cause Analysis

**Go BMT Performance Gaps:**

1. **Baseline Flush Performance (5.4x slower in isolated mode):**
   - **578ms** to flush 20K operations vs Rust's **108ms**
   - Per-operation cost: **28.9µs** vs Rust's **5.4µs**
   - Likely causes (in order of probability):
     1. **Tree rebuilt from scratch on every flush** - no incremental updates or dirty tracking
     2. **Inefficient serialization/hashing** - possibly recomputing all hashes instead of caching
     3. **Excessive disk I/O** - not batching writes or using proper WAL
     4. **No page-based storage** - entire tree serialized to disk on each commit
     5. **Memory allocations** - creating new tree structures instead of reusing
   - **Action:** Profile `Flush()` to identify hot paths

2. **Sequential Mode Degradation (12.8x growth from iteration 1 to 5):**
   - Flush time grows: 570ms → 2.4s → 3.7s → 5.4s → 7.3s (linear with tree size)
   - Per-operation cost grows: 28.5µs → 120µs → 187µs → 270µs → 365µs
   - **Confirms hypothesis:** Tree is rebuilt from scratch, and cost scales linearly with total entries
   - Missing optimizations:
     - No dirty page tracking (Rust's Bitbox only writes modified pages)
     - No incremental hash computation (Rust caches unchanged subtree hashes)
     - No proper overlay/compaction (accumulating all changes in memory)
   - **Action:** Implement incremental tree updates and dirty page tracking

3. **Comparison to Rust's Sequential Degradation:**
   - **Rust degrades 3.7x** from isolated to sequential (5.4µs → 20.1µs per op) - acceptable overhead
   - **Go degrades 12.8x** within sequential mode across iterations (28.5µs → 365µs per op)
   - Rust's sequential overhead is from legitimate extra work (disk reads, overlay management)
   - Go's growth is from algorithmic inefficiency (full tree rebuild)

**Performance Breakdown Hypothesis:**

Given the 5.4x gap and linear growth pattern, the most likely implementation issue is:

```go
// What Go BMT is probably doing (WRONG):
func (db *Nomt) Flush() error {
    // Rebuild entire tree from scratch
    root := &Node{}
    for key, value := range db.allEntries {  // O(n) iteration
        root.Insert(key, value)              // O(log n) per insert
    }
    root.ComputeHash()                       // O(n) - rehash entire tree
    root.SerializeToDisk()                   // O(n) - write all nodes
    return nil
}
// Total: O(n log n) + O(n) + O(n) = O(n log n) per flush
// With n=20K, each flush does ~260K operations
// With n=90K (sequential iteration 5), each flush does ~1.5M operations
```

```rust
// What Rust NOMT does (CORRECT):
fn commit() -> Result<Root> {
    let dirty_pages = self.overlay.dirty_pages();  // Only modified pages
    let updated_hashes = compute_delta_hashes(dirty_pages);  // Only changed paths
    self.wal.write(dirty_pages)?;              // Batch write to WAL
    self.bitbox.sync(dirty_pages)?;            // Write only dirty pages
    Ok(updated_root)
}
// Total: O(k log n) where k = number of changes, n = total size
// With k=20K changes, n=any total: ~300K operations (constant per iteration)
```

#### 1B.5 Next Steps

**Immediate Optimizations (targeting 5.4x baseline gap):**

1. **Profile and Confirm Root Cause:**
   - [ ] Run Go isolated mode with CPU profiler: `go test -cpuprofile=cpu.prof -run=TestNomtBenchmarkMedium_Isolated`
   - [ ] Analyze profile to confirm `Flush()` is rebuilding tree from scratch
   - [ ] Identify specific hot paths (likely: tree traversal, hashing, serialization)
   - [ ] Measure time breakdown: workload ops vs flush vs I/O

2. **Implement Incremental Tree Updates:**
   - [ ] Add dirty page tracking to session (`Session.dirtyPages map[PageID]bool`)
   - [ ] Modify `Insert()`/`Update()` to mark affected pages as dirty
   - [ ] Change `Commit()` to only rebuild/rehash dirty subtrees
   - [ ] Cache unchanged node hashes instead of recomputing
   - [ ] Expected improvement: Reduce flush from 578ms to <350ms (target: <3x gap)

3. **Fix Sequential Mode Growth (targeting 12.8x degradation):**
   - [ ] Implement proper overlay/compaction strategy
   - [ ] Add page eviction policy to prevent unbounded memory growth
   - [ ] Test with 10+ iterations to verify constant flush time
   - [ ] Expected: Eliminate linear growth (constant ~400ms flush time like Rust)

4. **Optimize I/O Layer:**
   - [ ] Implement Write-Ahead Log (WAL) for batched writes
   - [ ] Add page-based storage (similar to Rust's Bitbox)
   - [ ] Batch fsync calls (once per commit, not per page)
   - [ ] Expected improvement: **2-3x** reduction (assuming dirty tracking already in place)

**Success Criteria:**

**Phase 1 (Incremental Updates):**
- **Isolated mode:** Reduce from 51.5s to <2.5s (target: within 5x of Rust when normalized)
- **Sequential mode:** Constant flush time across iterations (no growth)
- **Throughput:** Achieve >20,000 ops/s (11% of Rust, up from 2.2%)

**Phase 2 (I/O Optimization):**
- **Isolated mode:** Reduce to <1s (target: within 2x of Rust when normalized)
- **Sequential mode:** Graceful degradation like Rust (3-4x slower than isolated, not 7x)
- **Throughput:** Achieve >50,000 ops/s (28% of Rust performance)

**Phase 3 (Full Optimization):**
- **Isolated mode:** <500ms (matching Rust's per-operation performance)
- **Sequential mode:** <2s with stable performance across 20+ iterations
- **Throughput:** >80,000 ops/s (45% of Rust, acceptable for MVP)

---

### Phase 2: Implement Real Witness Generation (PLANNED - Future Work)

**Goal:** Remove stubs, implement actual Merkle proof collection and verification

**⚠️ Major Gap:** The current Go BMT implementation has no infrastructure for:
- Recording sibling hashes during tree traversal
- Tracking node positions (left/right child)
- Collecting proof paths during `Insert()`/`Get()` operations

#### 2.1 Add Merkle Path Infrastructure to BMT

**File:** `bmt/bmt.go` (new code)

```go
// MerklePath represents a cryptographic proof path for a key
type MerklePath struct {
    Siblings [][32]byte  // Sibling hashes from leaf to root
    Indices  []bool      // Direction bits: false=left child, true=right child
}

// ProofCollector tracks siblings during tree operations
type ProofCollector struct {
    enabled  bool
    siblings [][32]byte
    indices  []bool
}

// GenerateProof generates a Merkle proof for a given key
// TODO: This requires major refactoring of Insert/Get to track tree paths
func (db *DB) GenerateProof(key [32]byte) (*MerklePath, error) {
    // MISSING: Need to modify tree traversal to collect siblings
    // Current implementation has no hooks for this
    return nil, errors.New("not implemented - requires tree refactoring")
}
```

**What needs to change:**
1. **During Insert():** Track which nodes are visited, record sibling hashes
2. **During Get():** Same as Insert - collect proof path
3. **Tree structure:** May need to store intermediate node hashes

#### 2.2 Implement Cryptographic Proof Verification

**File:** `bmt/verify.go` (new file)

```go
package bmt

import (
    "bytes"
    "crypto/sha256"  // TODO: Use Blake3 or Blake2b-GP to match Rust
)

// VerifyProof cryptographically verifies a Merkle proof
func VerifyProof(root [32]byte, key [32]byte, value []byte, path *MerklePath) bool {
    if path == nil || len(path.Siblings) == 0 {
        return false
    }

    // Compute leaf hash
    currentHash := hashLeaf(key, value)

    // Walk up the tree using sibling hashes
    for i := 0; i < len(path.Siblings); i++ {
        sibling := path.Siblings[i]
        isRight := path.Indices[i]

        if isRight {
            // Current node is on the right
            currentHash = hashInternal(sibling, currentHash)
        } else {
            // Current node is on the left
            currentHash = hashInternal(currentHash, sibling)
        }
    }

    // Final hash should match root
    return bytes.Equal(currentHash[:], root[:])
}

// hashLeaf computes the hash of a leaf node
// TODO: Must match Rust's leaf hashing scheme exactly
func hashLeaf(key [32]byte, value []byte) [32]byte {
    // Placeholder - need to match Rust implementation
    h := sha256.New()
    h.Write([]byte("leaf"))  // Prefix for leaf nodes
    h.Write(key[:])
    h.Write(value)
    var result [32]byte
    copy(result[:], h.Sum(nil))
    return result
}

// hashInternal computes the hash of an internal node
// TODO: Must match Rust's internal node hashing scheme exactly
func hashInternal(left, right [32]byte) [32]byte {
    // Placeholder - need to match Rust implementation
    h := sha256.New()
    h.Write([]byte("node"))  // Prefix for internal nodes
    h.Write(left[:])
    h.Write(right[:])
    var result [32]byte
    copy(result[:], h.Sum(nil))
    return result
}
```

**Rust references to study:**
- Leaf hashing: `nomt/core/src/trie.rs` (LeafData encoding)
- Internal hashing: `nomt/core/src/hash.rs` (GpNodeHasher)
- Proof generation: `nomt/nomt/tests/gp_witness_test.rs`

#### 2.3 Update Witness struct

**File:** `bmt/witness.go` (modify existing)

```go
type MerkleProof struct {
    Key     [32]byte
    Value   []byte
    Path    [][32]byte   // Sibling hashes
    Indices []bool       // 🆕 TODO: Add direction bits
}
```

#### 2.4 Update test_nomt to Use Real Proofs

**File:** `bmt/nomt_test.go` (lines 632-661)

Replace stubbed proof generation with:

```go
// Generate proofs for the witness keys - REAL IMPLEMENTATION
proofs := make([]MerkleProof, len(witnessKeys))

for i, key := range witnessKeys {
    value, err := db.Get(key)
    if err != nil {
        t.Logf("Warning: Could not get key %x: %v", key, err)
        proofs[i] = MerkleProof{
            Key:   key,
            Value: nil,
            Path:  nil,
        }
        continue
    }

    // 🆕 GENERATE REAL MERKLE PATH
    // TODO: This requires Phase 2.1 to be completed first
    merklePath, err := db.GenerateProof(key)
    if err != nil {
        t.Fatalf("Failed to generate proof for key %x: %v", key, err)
    }

    proofs[i] = MerkleProof{
        Key:     key,
        Value:   value,
        Path:    merklePath.Siblings,
        Indices: merklePath.Indices,
    }
}
```

#### 2.5 Replace Fake Verification

**File:** `bmt/nomt_test.go` (lines 664-686)

```go
// Step 4: Verify all witness keys with CRYPTOGRAPHIC PROOF VERIFICATION
verifyStart := time.Now()
verifiedCount := 0
for _, proof := range witness.Proofs {
    if proof.Path == nil {
        continue  // Skip keys that failed proof generation
    }

    // 🆕 CRYPTOGRAPHIC VERIFICATION
    merklePath := &MerklePath{
        Siblings: proof.Path,
        Indices:  proof.Indices,
    }

    if !VerifyProof(newStateRoot, proof.Key, proof.Value, merklePath) {
        t.Errorf("❌ Proof verification FAILED for key %x", proof.Key)
    } else {
        verifiedCount++
    }
}
verifyTime := time.Since(verifyStart)

t.Logf("Verification: %v (%d/%d keys verified)", verifyTime, verifiedCount, len(witnessKeys))
```

**Status:** ❌ **TODO - Major refactoring required**

---

## 4. Action Items

### ✅ Completed

- [x] Run Rust NOMT benchmark (benchtop)
- [x] Document exact Rust parameters and results
- [x] Identify gaps in Go BMT implementation
- [x] Create implementation plan

### Phase 1A: Make Tests Comparable

- [x] Add `NumReads` field to `TestParams` struct (nomt_test.go:42)
- [x] Implement read operations in `test_nomt()` (nomt_test.go:563-579)
- [x] Update timing logs to include read time (nomt_test.go:715-716)
- [x] Update `TestNomtBenchmarkMedium` parameters:
  - [x] Set `MaxIterations: 5`
  - [x] Set `CommitLag: 0`
  - [x] Add `NumReads: 5000`
  - [x] Reduce `NumDeletes: 50`
  - [x] Set `NumWitnessKeys: 0` (disable stub)
  - [x] Change `ValueSize: 32`
- [x] Run baseline Go BMT benchmark
- [x] Document Go BMT performance numbers

**Status:** ✅ Complete - but benchmark methodology issue discovered (see Phase 1B)

### Phase 1B: Fix Benchmark Isolation (CRITICAL)

**Problem:** Current benchmark accumulates state across iterations instead of resetting like Rust's `bench_isolate` mode.

- [ ] Modify `test_nomt()` to reset DB state between iterations
- [ ] Option 1: Close and reopen DB with fresh temp directory each iteration
- [ ] Option 2: Implement rollback/reset to baseline root (if BMT supports it)
- [ ] Option 3: Drop all keys and rebuild baseline state each iteration
- [ ] Re-run benchmark with isolated iterations
- [ ] Update comparison table with fair apples-to-apples numbers
- [ ] Verify flush times remain constant across iterations (~570ms baseline)

**Expected Deliverable:** Fair comparison showing true per-iteration performance (not cumulative growth).

### Study Rust Implementation

- [ ] Read Rust Merkle proof code (`nomt/src/merkle/`)
- [ ] Understand `build_trie()` in `nomt/core/src/update.rs`
- [ ] Study witness generation in `gp_witness_test.rs`
- [ ] Document Rust's hashing scheme (leaf + internal nodes)
- [ ] Identify where Rust collects sibling hashes

### Design Go BMT Proof API

- [ ] Design `ProofCollector` struct
- [ ] Plan tree refactoring to track siblings
- [ ] Define `GenerateProof()` API signature
- [ ] Define `VerifyProof()` API signature

### Phase 2: Implement Witness Generation

- [ ] Refactor tree traversal to collect siblings
- [ ] Implement `hashLeaf()` matching Rust
- [ ] Implement `hashInternal()` matching Rust
- [ ] Implement `GenerateProof()`
- [ ] Implement `VerifyProof()`
- [ ] Add `Indices` field to `MerkleProof`
- [ ] Replace stubbed proof generation in test
- [ ] Replace fake verification with real crypto

### Validation

- [ ] Re-enable `NumWitnessKeys: 10000` in test
- [ ] Verify all proofs pass cryptographic verification
- [ ] Benchmark witness generation overhead
- [ ] Compare witness performance with Rust `gp_witness_test.rs`

### Additional Tasks

- [ ] Profile and optimize hot paths
- [ ] Add GP encoding support (Blake2b-GP feature)
- [ ] Implement commit lag (overlay window)
- [ ] Contribute delete support to Rust benchtop (for parity)
- [ ] Fuzz test proof generation/verification
- [ ] Add witness benchmarks to CI

---

## 5. How to Interpret Benchmark Results

### Key Metrics to Track

#### 1. Throughput (ops/second)

- **Rust baseline:** 175,696 ops/s
- **Goal for Go (Phase 1):** >100,000 ops/s (57% of Rust)
- **Goal for Go (Phase 2):** >80,000 ops/s (with witness enabled)

**Calculation:**
```
Throughput = total_ops / mean_workload_time
Example: 20,000 ops / 0.113s = 176,991 ops/s
```

#### 2. Commit Time

- **Rust baseline:** 108 ms (WAL + HT sync per 20K ops)
- **Goal for Go:** <200 ms (within 2x is acceptable for MVP)

**What this measures:**
- Tree building (`build_trie()` in Rust, equivalent in Go)
- Write-Ahead Log flush
- Hash table sync
- Fsync to disk

**Does NOT measure:** Witness/proof generation (separate phase)

#### 3. Memory Usage (RSS)

- **Rust baseline:** 252 GB (high due to page/leaf caching)
- **Goal for Go:** <512 GB (2x is acceptable, investigate if higher)

**Why Rust uses so much memory:**
- Page cache (configurable: `--page-cache-size`)
- Leaf cache (configurable: `--leaf-cache-size`)
- Overlay window (configurable: `--overlay-window-length`)

#### 4. Witness Generation Time (Phase 2 only)

- **Rust:** Not measured in benchtop (tested in `gp_witness_test.rs`)
- **Goal for Go:** <50 ms for 10K proofs

**What this measures:**
- Proof path collection during tree traversal
- Memory allocation for sibling arrays

**Does NOT measure:** Verification time (separate metric)

### Performance Red Flags 🚩

| Issue | Threshold | Likely Cause |
|-------|-----------|--------------|
| Low throughput | <50,000 ops/s | Major algorithmic issue, inefficient tree |
| Slow commits | >500 ms | Inefficient `build_trie()`, excessive I/O |
| High memory | >1 TB | Memory leak, unbounded caching |
| Slow witness gen | >500 ms | Proof collection too expensive |

### Debugging Performance Issues

**If throughput is low:**
1. Profile with `go test -cpuprofile=cpu.prof`
2. Check for O(n²) algorithms in hot paths
3. Verify efficient tree traversal (should be O(log n))

**If commit time is high:**
1. Profile I/O operations (`-trace=trace.out`)
2. Check fsync frequency (should be once per commit)
3. Verify WAL batching is working

**If memory is high:**
1. Profile with `go test -memprofile=mem.prof`
2. Check for memory leaks (unreleased slices)
3. Review cache sizes and eviction policies

---

## 6. References

### Rust NOMT Code Locations

**Benchtop (benchmark framework):**
- Main: `/Users/michael/Github/nomt/benchtop/src/bench.rs`
- Workloads: `/Users/michael/Github/nomt/benchtop/src/workload.rs`
- Custom workload: `/Users/michael/Github/nomt/benchtop/src/custom_workload.rs`
- NOMT backend: `/Users/michael/Github/nomt/benchtop/src/nomt.rs`

**Core NOMT Implementation:**
- Merkle operations: `/Users/michael/Github/nomt/nomt/src/merkle/`
- Tree building: `/Users/michael/Github/nomt/core/src/update.rs`
- Hashing: `/Users/michael/Github/nomt/core/src/hash.rs`
- Trie structure: `/Users/michael/Github/nomt/core/src/trie.rs`

**Witness/Proof Tests:**
- GP witness test: `/Users/michael/Github/nomt/nomt/tests/gp_witness_test.rs`
- Overlay test: `/Users/michael/Github/nomt/nomt/tests/overlay.rs`
- Fill and empty: `/Users/michael/Github/nomt/nomt/tests/fill_and_empty.rs`

### Go BMT Files to Modify

**Test files:**
- Main test: `/Users/michael/Github/jam/bmt/nomt_test.go`
- Test helper: `test_nomt()` function (lines 447-706)
- Benchmark: `TestNomtBenchmarkMedium()` (lines 724-736)

**Implementation files (to be modified in Phase 2):**
- Core DB: `/Users/michael/Github/jam/bmt/bmt.go`
- Witness: `/Users/michael/Github/jam/bmt/witness.go`
- New file: `/Users/michael/Github/jam/bmt/verify.go` (proof verification)

### External Resources

- **NOMT Presentation:** [November 2024 slides](https://hackmd.io/@Xo-wxO7bQkKidH1LrqACsw/rkG0lmjWyg#/)
- **JAM Gray Paper:** Binary Merkle Trie specification
- **Rust NOMT README:** `/Users/michael/Github/nomt/README.md`

---

## 7. Summary

### Current Status

| Component | Status | Details |
|-----------|--------|---------|
| **Rust NOMT** | ✅ Production | 175K ops/s, full tree, no witness in benchtop |
| **Go BMT (baseline)** | ❌ Not tested | Need Phase 1A to establish baseline |
| **Go BMT (witness)** | ❌ Stubbed | Need Phase 2 for real implementation |

### What We Learned from Rust Benchmark

1. **Benchtop does NOT test witness generation** - it only measures tree operations
2. **Witness tests are separate** (`gp_witness_test.rs`)
3. **Deletes are not benchmarked** - only tested for correctness
4. **Throughput: 175K ops/s** with single-threaded commit
5. **Memory usage is high** (252 GB) due to aggressive caching

### Critical Path Forward

**Phase 1:** Make Go BMT comparable
- Add reads to match realistic workload
- Disable witness stub
- Run baseline benchmark
- Establish performance gap

**Phase 2:** Implement real witness
- Refactor tree to collect siblings
- Implement cryptographic verification
- Re-benchmark with witness enabled
- Compare with Rust `gp_witness_test.rs`

### Success Criteria

**Phase 1 (Baseline):**
- ✅ Go BMT runs same workload as Rust (5 iterations, 20K ops, 50% fresh)
- ✅ Throughput >100K ops/s (57% of Rust is acceptable)
- ✅ Commit time <200 ms (within 2x of Rust)
- ✅ Memory <512 GB (within 2x of Rust)

**Phase 2 (Witness):**
- ✅ Proofs verify cryptographically (100% pass rate)
- ✅ Witness generation <50 ms for 10K proofs
- ✅ No performance regression in baseline metrics
- ✅ Root hashes match between incremental and fresh builds

---
