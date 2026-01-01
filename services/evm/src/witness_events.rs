//! Verkle witness access/write event tracking for EIP-4762
//!
//! This module tracks all Verkle tree access and write events during transaction execution
//! to enable witness-based gas charging and proof construction.

extern crate alloc;

use alloc::collections::BTreeSet;
use primitive_types::{H160, U256};

use crate::verkle_constants::*;

// Verkle tree key: (address, tree_key, sub_key)
// tree_key = pos / 256 (U256 to avoid truncation for high slots)
// sub_key = pos % 256
type VerkleKey = (H160, U256, u8);

// ===== Phase D: Precompile/System Contract Filtering =====

/// Check if address is a JAM precompile (0x01-0x03, 0x10)
/// Per Phase D requirement: JAM precompiles are 0x01 (usdm), 0x02 (gov), 0x03 (math), 0x10 (shield)
#[inline]
pub fn is_precompile(addr: H160) -> bool {
    addr == USDM_ADDRESS || addr == GOVERNOR_ADDRESS || addr == MATH_PRECOMPILE_ADDRESS || addr == SHIELD_ADDRESS
}

/// Check if address is a system contract
#[inline]
pub fn is_system_contract(addr: H160) -> bool {
    // Placeholder until a concrete system-contract list is defined.
    // Avoid over-filtering user addresses by default.
    let _ = addr;
    false
}

/// Tracks all Verkle tree access and write events during execution
pub struct VerkleEventTracker {
    // Access tracking (reads)
    accessed_branches: BTreeSet<(H160, U256)>, // (address, tree_key)
    accessed_leaves: BTreeSet<VerkleKey>,      // (address, tree_key, sub_key)

    // Write tracking
    edited_subtrees: BTreeSet<(H160, U256)>, // First edit to subtree
    edited_leaves: BTreeSet<VerkleKey>,      // First edit to leaf
    leaf_fills: BTreeSet<VerkleKey>,         // None→value writes (CHUNK_FILL_COST)
}

impl VerkleEventTracker {
    /// Create a new empty event tracker
    pub fn new() -> Self {
        Self {
            accessed_branches: BTreeSet::new(),
            accessed_leaves: BTreeSet::new(),
            edited_subtrees: BTreeSet::new(),
            edited_leaves: BTreeSet::new(),
            leaf_fills: BTreeSet::new(),
        }
    }

    /// Reset all recorded events so a new transaction starts with a clean slate
    pub fn clear(&mut self) {
        self.accessed_branches.clear();
        self.accessed_leaves.clear();
        self.edited_subtrees.clear();
        self.edited_leaves.clear();
        self.leaf_fills.clear();
    }

    /// Snapshot current event counts for logging
    pub fn stats(&self) -> (usize, usize, usize, usize, usize) {
        (
            self.accessed_branches.len(),
            self.accessed_leaves.len(),
            self.edited_subtrees.len(),
            self.edited_leaves.len(),
            self.leaf_fills.len(),
        )
    }

    /// Calculate Verkle tree keys from storage slot
    /// Implements get_storage_slot_tree_keys from EIP-4762
    ///
    /// Handles arbitrary U256 storage slots correctly by using proper modulo arithmetic
    fn get_storage_slot_tree_keys(storage_key: U256) -> (U256, u8) {
        // For storage keys, calculate position using U256 arithmetic
        // pos = HEADER_STORAGE_OFFSET + key (if key < CODE_OFFSET - HEADER_STORAGE_OFFSET)
        //    or MAIN_STORAGE_OFFSET + key (otherwise)

        let threshold = U256::from(CODE_OFFSET - HEADER_STORAGE_OFFSET);
        let pos_u256 = if storage_key < threshold {
            U256::from(HEADER_STORAGE_OFFSET) + storage_key
        } else {
            MAIN_STORAGE_OFFSET + storage_key
        };

        // tree_key = pos / 256 (can be very large for large storage slots)
        let tree_key = pos_u256 / U256::from(VERKLE_NODE_WIDTH);

        // sub_key = pos % 256 (always fits in u8)
        let sub_key = (pos_u256 % U256::from(VERKLE_NODE_WIDTH)).low_u64() as u8;

        (tree_key, sub_key)
    }

    /// Record storage slot access (SLOAD)
    pub fn record_storage_access(&mut self, address: H160, slot: U256) {
        let (tree_key, sub_key) = Self::get_storage_slot_tree_keys(slot);

        // Track branch access
        self.accessed_branches.insert((address, tree_key));

        // Track leaf access
        self.accessed_leaves.insert((address, tree_key, sub_key));
    }

    /// Record account header access (BALANCE, CALL, etc.)
    /// Uses BASIC_DATA_LEAF_KEY position
    ///
    /// Phase D: Filters precompiles/system contracts per EIP-4762
    pub fn record_account_header_access(&mut self, address: H160) {
        // Phase D: Skip precompile/system contract code/header accesses from witness
        // per EIP-4762 (balance writes still tracked via record_account_write)
        if is_precompile(address) || is_system_contract(address) {
            return;
        }

        // Track branch access (tree_key = 0 for account headers)
        self.accessed_branches.insert((address, U256::zero()));

        // Track leaf access
        self.accessed_leaves
            .insert((address, U256::zero(), BASIC_DATA_LEAF_KEY));
    }

    /// Record code hash access (EXTCODEHASH)
    /// Uses CODEHASH_LEAF_KEY position
    ///
    /// Phase D: Filters precompiles/system contracts per EIP-4762
    pub fn record_codehash_access(&mut self, address: H160) {
        // Phase D: Skip precompile/system contract code hash accesses from witness
        if is_precompile(address) || is_system_contract(address) {
            return;
        }

        const CODEHASH_LEAF_KEY: u8 = 1; // Code hash at position 1

        // Track branch access (tree_key = 0 for code hash)
        self.accessed_branches.insert((address, U256::zero()));

        // Track leaf access
        self.accessed_leaves
            .insert((address, U256::zero(), CODEHASH_LEAF_KEY));
    }

    /// Record code chunk access (CODECOPY, EXTCODECOPY, execution)
    ///
    /// Phase D: Filters precompiles/system contracts per EIP-4762
    pub fn record_code_chunk_access(&mut self, address: H160, chunk_index: usize) {
        // Phase D: Skip precompile/system contract code chunks from witness
        if is_precompile(address) || is_system_contract(address) {
            return;
        }

        // Code chunks start at CODE_OFFSET
        let pos = CODE_OFFSET + chunk_index;
        let tree_key = U256::from(pos / VERKLE_NODE_WIDTH);
        let sub_key = (pos % VERKLE_NODE_WIDTH) as u8;

        // Track branch access
        self.accessed_branches.insert((address, tree_key));

        // Track leaf access
        self.accessed_leaves.insert((address, tree_key, sub_key));
    }

    /// Record storage write (SSTORE)
    ///
    /// # Arguments
    /// * `address` - Contract address
    /// * `slot` - Storage slot
    /// * `original_value` - Value before transaction (from state)
    /// * `new_value` - Value being written
    pub fn record_storage_write(
        &mut self,
        address: H160,
        slot: U256,
        original_value: Option<U256>,
        new_value: U256,
    ) {
        let (tree_key, sub_key) = Self::get_storage_slot_tree_keys(slot);

        // First record the read (SSTORE reads before writing)
        self.accessed_branches.insert((address, tree_key));
        self.accessed_leaves.insert((address, tree_key, sub_key));

        // Track subtree edit
        self.edited_subtrees.insert((address, tree_key));

        // Track leaf edit
        let key = (address, tree_key, sub_key);
        self.edited_leaves.insert(key);

        // Track fill (None→value write incurs CHUNK_FILL_COST)
        if original_value.is_none() && new_value != U256::zero() {
            self.leaf_fills.insert(key);
        }
    }

    /// Record account write (balance change, creation)
    pub fn record_account_write(&mut self, address: H160) {
        // First record the read
        self.accessed_branches.insert((address, U256::zero()));
        self.accessed_leaves
            .insert((address, U256::zero(), BASIC_DATA_LEAF_KEY));

        // Track edit
        self.edited_subtrees.insert((address, U256::zero()));
        self.edited_leaves
            .insert((address, U256::zero(), BASIC_DATA_LEAF_KEY));
    }

    /// Record code write (CREATE, CREATE2)
    /// Writes code chunks covering the entire code
    pub fn record_code_write(&mut self, address: H160, code_length: usize) {
        // Code is stored in 31-byte chunks
        let num_chunks = (code_length + 30) / 31;

        for chunk_index in 0..num_chunks {
            let pos = CODE_OFFSET + chunk_index;
            let tree_key = U256::from(pos / VERKLE_NODE_WIDTH);
            let sub_key = (pos % VERKLE_NODE_WIDTH) as u8;

            // Track branch/leaf access
            self.accessed_branches.insert((address, tree_key));
            let key = (address, tree_key, sub_key);
            self.accessed_leaves.insert(key);

            // Track edit
            self.edited_subtrees.insert((address, tree_key));
            self.edited_leaves.insert(key);
            // Note: code writes are always fills (new account)
            self.leaf_fills.insert(key);
        }

        // Also track code hash write (as fill for new account)
        const CODEHASH_LEAF_KEY: u8 = 1;
        self.accessed_branches.insert((address, U256::zero()));
        let codehash_key = (address, U256::zero(), CODEHASH_LEAF_KEY);
        self.accessed_leaves.insert(codehash_key);
        self.edited_subtrees.insert((address, U256::zero()));
        self.edited_leaves.insert(codehash_key);
        self.leaf_fills.insert(codehash_key); // Codehash is also a fill (new account)
    }

    /// Record transaction-level origin access
    /// Per EIP-4762: access both BASIC_DATA_LEAF_KEY and CODEHASH_LEAF_KEY
    pub fn record_tx_origin_access(&mut self, address: H160) {
        // Skip precompile/system contract headers from witness tracking
        if is_precompile(address) || is_system_contract(address) {
            return;
        }
        // Track branch access
        self.accessed_branches.insert((address, U256::zero()));

        // Track both leaves
        self.accessed_leaves
            .insert((address, U256::zero(), BASIC_DATA_LEAF_KEY));
        self.accessed_leaves
            .insert((address, U256::zero(), CODE_HASH_LEAF_KEY));
    }

    /// Record transaction-level target access (if value > 0)
    /// Per EIP-4762: access both BASIC_DATA_LEAF_KEY and CODEHASH_LEAF_KEY
    pub fn record_tx_target_access(&mut self, address: H160) {
        // Skip precompile/system contract headers from witness tracking
        if is_precompile(address) || is_system_contract(address) {
            return;
        }
        // Track branch access
        self.accessed_branches.insert((address, U256::zero()));

        // Track both leaves
        self.accessed_leaves
            .insert((address, U256::zero(), BASIC_DATA_LEAF_KEY));
        self.accessed_leaves
            .insert((address, U256::zero(), CODE_HASH_LEAF_KEY));
    }

    /// Record transaction-level origin write (always happens - gas payment)
    /// Per EIP-4762: tx-level events don't charge edit/fill costs
    pub fn record_tx_origin_write(&mut self, address: H160) {
        // Skip precompile/system contract headers from witness tracking
        if is_precompile(address) || is_system_contract(address) {
            return;
        }
        // Track branch/leaf access (already done in record_tx_origin_access)
        self.accessed_branches.insert((address, U256::zero()));
        self.accessed_leaves
            .insert((address, U256::zero(), BASIC_DATA_LEAF_KEY));

        // DO NOT track in edited_subtrees/edited_leaves
        // Per EIP-4762: tx-level writes skip edit/fill charges
    }

    /// Record transaction-level target write (if value > 0)
    /// Per EIP-4762: tx-level events don't charge edit/fill costs
    pub fn record_tx_target_write(&mut self, address: H160) {
        // Skip precompile/system contract headers from witness tracking
        if is_precompile(address) || is_system_contract(address) {
            return;
        }
        // Track branch/leaf access (already done in record_tx_target_access)
        self.accessed_branches.insert((address, U256::zero()));
        self.accessed_leaves
            .insert((address, U256::zero(), BASIC_DATA_LEAF_KEY));

        // DO NOT track in edited_subtrees/edited_leaves
        // Per EIP-4762: tx-level writes skip edit/fill charges
    }

    /// Get the set of accessed branches for witness construction
    /// NOTE: Reserved for future witness proof generation (Phase C)
    #[allow(dead_code)]
    pub fn accessed_branches(&self) -> &BTreeSet<(H160, U256)> {
        &self.accessed_branches
    }

    /// Get the set of accessed leaves for witness construction
    /// NOTE: Reserved for future witness proof generation (Phase C)
    #[allow(dead_code)]
    pub fn accessed_leaves(&self) -> &BTreeSet<VerkleKey> {
        &self.accessed_leaves
    }

    /// Calculate total witness gas cost from all tracked events (Phase B Option C)
    ///
    /// Returns the total gas cost for the witness based on first-access tracking.
    /// Call this after transaction execution completes to get the witness gas charge.
    ///
    /// Gas breakdown:
    /// - WITNESS_BRANCH_COST (1900) per unique (address, tree_key) branch access
    /// - WITNESS_CHUNK_COST (200) per unique (address, tree_key, sub_key) leaf access
    /// - SUBTREE_EDIT_COST (3000) per unique (address, tree_key) subtree edit
    /// - CHUNK_EDIT_COST (500) per unique (address, tree_key, sub_key) leaf edit
    /// - CHUNK_FILL_COST (6200) per unique (address, tree_key, sub_key) fill
    pub fn calculate_total_witness_gas(&self) -> u64 {
        let mut total = 0u64;

        // Charge for all accessed branches
        total = total.saturating_add(
            (self.accessed_branches.len() as u64).saturating_mul(WITNESS_BRANCH_COST),
        );

        // Charge for all accessed leaves
        total = total
            .saturating_add((self.accessed_leaves.len() as u64).saturating_mul(WITNESS_CHUNK_COST));

        // Charge for all edited subtrees
        total = total
            .saturating_add((self.edited_subtrees.len() as u64).saturating_mul(SUBTREE_EDIT_COST));

        // Charge for all edited leaves
        total =
            total.saturating_add((self.edited_leaves.len() as u64).saturating_mul(CHUNK_EDIT_COST));

        // Charge for all fills
        total =
            total.saturating_add((self.leaf_fills.len() as u64).saturating_mul(CHUNK_FILL_COST));

        total
    }
}

impl Default for VerkleEventTracker {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn addr(val: u64) -> H160 {
        let mut bytes = [0u8; 20];
        bytes[12..].copy_from_slice(&val.to_be_bytes());
        H160::from(bytes)
    }

    #[test]
    fn test_storage_slot_tree_keys() {
        // Storage slot 0 should map to header storage area
        let (tree_key, sub_key) = VerkleEventTracker::get_storage_slot_tree_keys(U256::zero());
        assert_eq!(tree_key, U256::zero()); // 64 / 256 = 0
        assert_eq!(sub_key, 64); // 64 % 256 = 64

        // Storage slot 1
        let (tree_key, sub_key) = VerkleEventTracker::get_storage_slot_tree_keys(U256::one());
        assert_eq!(tree_key, U256::zero()); // 65 / 256 = 0
        assert_eq!(sub_key, 65); // 65 % 256 = 65
    }

    #[test]
    fn test_record_storage_access() {
        let mut tracker = VerkleEventTracker::new();
        let addr = addr(1);

        tracker.record_storage_access(addr, U256::zero());

        assert_eq!(tracker.accessed_branches.len(), 1);
        assert_eq!(tracker.accessed_leaves.len(), 1);
        assert_eq!(tracker.edited_leaves.len(), 0);
    }

    #[test]
    fn test_record_storage_write_fill() {
        let mut tracker = VerkleEventTracker::new();
        let addr = addr(1);

        // None→value write should be tracked as fill
        tracker.record_storage_write(addr, U256::zero(), None, U256::from(42));

        assert_eq!(tracker.accessed_branches.len(), 1);
        assert_eq!(tracker.accessed_leaves.len(), 1);
        assert_eq!(tracker.edited_subtrees.len(), 1);
        assert_eq!(tracker.edited_leaves.len(), 1);
        assert_eq!(tracker.leaf_fills.len(), 1);
    }

    #[test]
    fn test_record_storage_write_no_fill() {
        let mut tracker = VerkleEventTracker::new();
        let addr = addr(1);

        // Some→value write should NOT be tracked as fill
        tracker.record_storage_write(addr, U256::zero(), Some(U256::from(1)), U256::from(42));

        assert_eq!(tracker.leaf_fills.len(), 0);
        assert_eq!(tracker.edited_leaves.len(), 1);
    }

}
