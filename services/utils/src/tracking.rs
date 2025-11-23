//! JAM DA dependency tracking for parallel execution and work package consensus
//!
//! This module provides generic structures for tracking read/write dependencies
//! across JAM services. Used by EVM backend to enable:
//! - Parallel transaction execution (via tx_reads conflict detection)
//! - Work package validation (via block_reads ObjectId list)
//! - Deterministic write ordering (via WriteRecord instance counters)

use alloc::{
    collections::{BTreeMap, BTreeSet},
    vec::Vec,
};
use primitive_types::{H160, H256};
use crate::objects::ObjectId;

// ObjectDependency replaced with ObjectId

/// Kind of write recorded by the overlay tracker
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord)]
pub enum WriteKind {
    Balance,
    Code,
    Nonce,
    Storage(H256),
    StorageReset,
    TransientStorage(H256),
    Log,
    Delete,
    Create,
}

/// Unique identifier for a write event
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord)]
pub struct WriteKey {
    pub address: H160,
    pub kind: WriteKind,
}

/// Ordered record describing a write emitted during execution
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct WriteRecord {
    pub tx_index: usize,
    pub key: WriteKey,
    pub instance: u32,
}

/// Snapshot emitted when flushing the tracker at the end of the block
#[derive(Debug, Clone, Default)]
pub struct TrackerOutput {
    pub block_reads: BTreeSet<ObjectId>,
    pub tx_reads: Vec<BTreeSet<ObjectId>>,
    pub writes: Vec<WriteRecord>,
}

pub struct OverlayTracker {
    block_reads: BTreeSet<ObjectId>,
    frame_reads: Vec<BTreeSet<ObjectId>>,
    tx_reads: Vec<BTreeSet<ObjectId>>,
    pending_tx_reads: BTreeSet<ObjectId>,
    writes: Vec<WriteRecord>,
    write_counters: BTreeMap<WriteKey, u32>,
    current_tx: Option<usize>,
}

impl WriteKey {
    pub fn storage(address: H160, key: H256) -> Self {
        Self {
            address,
            kind: WriteKind::Storage(key),
        }
    }

    pub fn transient_storage(address: H160, key: H256) -> Self {
        Self {
            address,
            kind: WriteKind::TransientStorage(key),
        }
    }

    pub fn balance(address: H160) -> Self {
        Self {
            address,
            kind: WriteKind::Balance,
        }
    }

    pub fn nonce(address: H160) -> Self {
        Self {
            address,
            kind: WriteKind::Nonce,
        }
    }

    pub fn code(address: H160) -> Self {
        Self {
            address,
            kind: WriteKind::Code,
        }
    }

    pub fn storage_reset(address: H160) -> Self {
        Self {
            address,
            kind: WriteKind::StorageReset,
        }
    }

    pub fn delete(address: H160) -> Self {
        Self {
            address,
            kind: WriteKind::Delete,
        }
    }

    pub fn create(address: H160) -> Self {
        Self {
            address,
            kind: WriteKind::Create,
        }
    }

    pub fn log(address: H160) -> Self {
        Self {
            address,
            kind: WriteKind::Log,
        }
    }
}

impl OverlayTracker {
    /// Initialize empty tracker for new block
    /// Used by: MajikOverlay::new() at block start
    pub fn new() -> Self {
        Self {
            block_reads: BTreeSet::new(),
            frame_reads: Vec::new(),
            tx_reads: Vec::new(),
            pending_tx_reads: BTreeSet::new(),
            writes: Vec::new(),
            write_counters: BTreeMap::new(),
            current_tx: None,
        }
    }

    /// Start new transaction with isolated read tracking
    /// JAM DA: Clears pending_tx_reads for fresh transaction-level dependency set
    /// Frame management: Pushes root frame for transaction-level call depth
    pub fn begin_transaction(&mut self, tx_index: usize) {
        self.current_tx = Some(tx_index);
        self.pending_tx_reads.clear();
        self.frame_reads.clear();
        self.frame_reads.push(BTreeSet::new());
        if self.tx_reads.len() <= tx_index {
            self.tx_reads.resize(tx_index + 1, BTreeSet::new());
        }
        self.tx_reads[tx_index].clear();
    }

    /// Commit transaction, snapshot reads for parallelism analysis
    /// JAM DA: Saves pending_tx_reads → tx_reads[tx_index] for work package
    /// Clears frame_reads and pending_tx_reads for next transaction
    pub fn commit_transaction(&mut self) {
        if let Some(tx_index) = self.current_tx.take() {
            if self.tx_reads.len() <= tx_index {
                self.tx_reads.resize(tx_index + 1, BTreeSet::new());
            }
            self.tx_reads[tx_index] = self.pending_tx_reads.clone();
        }
        self.frame_reads.clear();
        self.pending_tx_reads.clear();
    }

    /// Revert transaction, discard all reads/writes
    /// JAM DA: Pending reads not saved, frame reads cleared
    /// Reverted transactions don't pollute dependency graph
    pub fn revert_transaction(&mut self) {
        self.current_tx = None;
        self.frame_reads.clear();
        self.pending_tx_reads.clear();
    }

    /// Push new frame for nested CALL/CREATE
    /// Frame isolation: Each nested call gets separate read set
    /// Mirrors: vendor OverlayedBackend's push_substate
    pub fn enter_frame(&mut self) {
        self.frame_reads.push(BTreeSet::new());
    }

    /// Commit frame, merge child reads into parent
    /// JAM DA: Successful nested calls propagate reads upward
    /// Used for: Preserving dependencies through call stack
    pub fn commit_frame(&mut self) {
        if let Some(child) = self.frame_reads.pop() {
            if let Some(parent) = self.frame_reads.last_mut() {
                for dependency in child {
                    parent.insert(dependency);
                }
            }
        }
    }

    /// Revert frame, discard child reads
    /// JAM DA: Failed nested calls don't pollute parent's dependency set
    /// Mirrors: vendor OverlayedBackend's pop_substate(MergeStrategy::Revert)
    pub fn revert_frame(&mut self) {
        self.frame_reads.pop();
    }

    /// Record DA object read for dependency tracking
    /// JAM DA: Inserts into block_reads, frame_reads, pending_tx_reads
    /// Used for: Work package ObjectId list, parallelism analysis
    pub fn record_read(&mut self, dependency: ObjectId) {
        self.block_reads.insert(dependency);
        if let Some(frame) = self.frame_reads.last_mut() {
            frame.insert(dependency);
        }
        self.pending_tx_reads.insert(dependency);
    }

    /// Record write with transaction index and instance counter
    /// JAM DA: Enables deterministic write ordering for accumulate phase
    /// Instance counter: Distinguishes multiple writes to same slot
    pub fn record_write(&mut self, key: WriteKey) {
        if let Some(tx_index) = self.current_tx {
            let counter = self.write_counters.entry(key.clone()).or_insert(0);
            *counter += 1;
            self.writes.push(WriteRecord {
                tx_index,
                key,
                instance: *counter,
            });
        }
    }

    /// Convert tracker to output for orchestrator
    /// JAM DA: Returns block_reads for work package, tx_reads for parallelism, writes for ordering
    /// Used by: MajikOverlay::deconstruct() at block finalization
    pub fn into_output(self) -> TrackerOutput {
        TrackerOutput {
            block_reads: self.block_reads,
            tx_reads: self.tx_reads,
            writes: self.writes,
        }
    }
}
