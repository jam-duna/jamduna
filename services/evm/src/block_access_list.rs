//! Block Access List (BAL) construction and hashing
//!
//! Based on EIP-7928 adapted for JAM's stateless verification model.
//! Constructs BAL from verified UBT witnesses for deterministic block_access_list_hash.

use alloc::collections::BTreeMap;
use alloc::vec::Vec;
use primitive_types::{H160, H256};
use crate::ubt::{UBTKeyType, UBTWitnessEntry};

/// Block Access List - all state accesses in a block
#[derive(Debug, Clone)]
pub struct BlockAccessList {
    pub accounts: Vec<AccountChanges>,  // Sorted by address
}

/// Account-level changes
#[derive(Debug, Clone)]
pub struct AccountChanges {
    pub address: H160,
    pub storage_reads: Vec<StorageRead>,      // Read-only slots (sorted by key)
    pub storage_changes: Vec<SlotChanges>,    // Modified slots (sorted by key)
    pub balance_changes: Vec<BalanceChange>,  // Ordered by tx_index
    pub nonce_changes: Vec<NonceChange>,      // Ordered by tx_index
    pub code_changes: Vec<CodeChange>,        // Ordered by tx_index
}

/// Storage slot that was read but not modified
#[derive(Debug, Clone)]
pub struct StorageRead {
    pub key: H256,  // Full 32-byte storage key
}

/// Storage slot that was modified (may have multiple writes)
#[derive(Debug, Clone)]
pub struct SlotChanges {
    pub key: H256,
    pub writes: Vec<SlotWrite>,  // Ordered by tx_index
}

/// Single write to a storage slot
#[derive(Debug, Clone)]
pub struct SlotWrite {
    pub block_access_index: u32,  // Transaction index
    pub pre_value: H256,
    pub post_value: H256,
}

/// Balance modification
#[derive(Debug, Clone)]
pub struct BalanceChange {
    pub block_access_index: u32,
    pub pre_balance: H256,   // Big-endian uint128 in last 16 bytes (BasicData format)
    pub post_balance: H256,
}

/// Nonce modification
#[derive(Debug, Clone)]
pub struct NonceChange {
    pub block_access_index: u32,
    pub pre_nonce: [u8; 8],   // Little-endian uint64 (BasicData format)
    pub post_nonce: [u8; 8],
}

/// Code deployment
#[derive(Debug, Clone)]
pub struct CodeChange {
    pub block_access_index: u32,
    pub code_hash: H256,   // keccak256 of code
    pub code_size: u32,
}

impl BlockAccessList {
    /// Build BAL from verified UBT witness entries.
    pub fn from_ubt_entries(
        read_entries: &[UBTWitnessEntry],
        write_entries: &[UBTWitnessEntry],
    ) -> Self {
        let mut account_map: BTreeMap<H160, AccountChanges> = BTreeMap::new();

        let write_keys = collect_write_keys_ubt(write_entries);

        for read in read_entries {
            if is_modified_ubt(read, &write_keys) {
                continue;
            }

            let account = account_map
                .entry(read.metadata.address)
                .or_insert_with(|| AccountChanges::new(read.metadata.address));

            if read.metadata.key_type == UBTKeyType::Storage {
                account.storage_reads.push(StorageRead {
                    key: read.metadata.storage_key,
                });
            }
        }

        let slot_changes = build_slot_changes_ubt(write_entries);
        let balance_changes = build_balance_changes_ubt(write_entries);
        let nonce_changes = build_nonce_changes_ubt(write_entries);
        let code_changes = build_code_changes_ubt(write_entries);

        for (addr, slots) in slot_changes {
            account_map
                .entry(addr)
                .or_insert_with(|| AccountChanges::new(addr))
                .storage_changes = slots;
        }

        for (addr, changes) in balance_changes {
            account_map
                .entry(addr)
                .or_insert_with(|| AccountChanges::new(addr))
                .balance_changes = changes;
        }

        for (addr, changes) in nonce_changes {
            account_map
                .entry(addr)
                .or_insert_with(|| AccountChanges::new(addr))
                .nonce_changes = changes;
        }

        for (addr, changes) in code_changes {
            account_map
                .entry(addr)
                .or_insert_with(|| AccountChanges::new(addr))
                .code_changes = changes;
        }

        let mut accounts: Vec<AccountChanges> = account_map.into_values().collect();

        for account in &mut accounts {
            account.storage_reads.sort_by_key(|r| r.key);
            account.storage_changes.sort_by_key(|c| c.key);
        }

        BlockAccessList { accounts }
    }

    /// Compute Blake2b hash of RLP-encoded BAL
    pub fn hash(&self) -> [u8; 32] {
        let encoded = self.rlp_encode();
        utils::hash_functions::blake2b_hash(&encoded)
    }

    /// RLP encode the BAL structure
    fn rlp_encode(&self) -> Vec<u8> {
        use rlp::RlpStream;

        // BlockAccessList is a struct with one field: Accounts
        // RLP encoding should wrap the accounts list in an outer list
        let mut stream = RlpStream::new_list(1); // Outer struct wrapper
        stream.begin_list(self.accounts.len());  // Accounts field

        for account in &self.accounts {
            // Encode AccountChanges as RLP list:
            // [address, storage_reads, storage_changes, balance_changes, nonce_changes, code_changes]
            stream.begin_list(6);
            stream.append(&account.address);

            // StorageReads
            stream.begin_list(account.storage_reads.len());
            for read in &account.storage_reads {
                stream.append(&read.key);
            }

            // StorageChanges
            stream.begin_list(account.storage_changes.len());
            for slot in &account.storage_changes {
                stream.begin_list(2);  // [key, writes]
                stream.append(&slot.key);

                stream.begin_list(slot.writes.len());
                for write in &slot.writes {
                    stream.begin_list(3);  // [index, pre, post]
                    stream.append(&write.block_access_index);
                    stream.append(&write.pre_value);
                    stream.append(&write.post_value);
                }
            }

            // BalanceChanges
            stream.begin_list(account.balance_changes.len());
            for change in &account.balance_changes {
                stream.begin_list(3);
                stream.append(&change.block_access_index);
                stream.append(&change.pre_balance);
                stream.append(&change.post_balance);
            }

            // NonceChanges
            stream.begin_list(account.nonce_changes.len());
            for change in &account.nonce_changes {
                stream.begin_list(3);
                stream.append(&change.block_access_index);
                stream.append(&change.pre_nonce.as_ref());
                stream.append(&change.post_nonce.as_ref());
            }

            // CodeChanges
            stream.begin_list(account.code_changes.len());
            for change in &account.code_changes {
                stream.begin_list(3);
                stream.append(&change.block_access_index);
                stream.append(&change.code_hash);
                stream.append(&change.code_size);
            }
        }

        stream.out().to_vec()
    }
}

impl AccountChanges {
    fn new(address: H160) -> Self {
        Self {
            address,
            storage_reads: Vec::new(),
            storage_changes: Vec::new(),
            balance_changes: Vec::new(),
            nonce_changes: Vec::new(),
            code_changes: Vec::new(),
        }
    }
}

fn is_modified_ubt(read: &UBTWitnessEntry, write_keys: &BTreeMap<(H160, H256), ()>) -> bool {
    if read.metadata.key_type == UBTKeyType::Storage {
        write_keys.contains_key(&(read.metadata.address, read.metadata.storage_key))
    } else {
        false
    }
}

fn collect_write_keys_ubt(writes: &[UBTWitnessEntry]) -> BTreeMap<(H160, H256), ()> {
    let mut keys = BTreeMap::new();
    for write in writes {
        if write.metadata.key_type == UBTKeyType::Storage {
            keys.insert((write.metadata.address, write.metadata.storage_key), ());
        }
    }
    keys
}

fn build_slot_changes_ubt(writes: &[UBTWitnessEntry]) -> BTreeMap<H160, Vec<SlotChanges>> {
    let mut slot_map: BTreeMap<H160, BTreeMap<H256, Vec<SlotWrite>>> = BTreeMap::new();

    for write in writes {
        if write.metadata.key_type != UBTKeyType::Storage {
            continue;
        }

        slot_map
            .entry(write.metadata.address)
            .or_insert_with(BTreeMap::new)
            .entry(write.metadata.storage_key)
            .or_insert_with(Vec::new)
            .push(SlotWrite {
                block_access_index: write.metadata.tx_index,
                pre_value: H256::from(write.pre_value),
                post_value: H256::from(write.post_value),
            });
    }

    slot_map
        .into_iter()
        .map(|(addr, slots)| {
            let changes = slots
                .into_iter()
                .map(|(key, writes)| SlotChanges { key, writes })
                .collect();
            (addr, changes)
        })
        .collect()
}

fn build_balance_changes_ubt(writes: &[UBTWitnessEntry]) -> BTreeMap<H160, Vec<BalanceChange>> {
    let mut changes: BTreeMap<H160, Vec<BalanceChange>> = BTreeMap::new();

    for write in writes {
        if write.metadata.key_type != UBTKeyType::BasicData {
            continue;
        }

        let mut pre_balance = [0u8; 32];
        let mut post_balance = [0u8; 32];
        pre_balance[16..32].copy_from_slice(&write.pre_value[16..32]);
        post_balance[16..32].copy_from_slice(&write.post_value[16..32]);

        changes
            .entry(write.metadata.address)
            .or_insert_with(Vec::new)
            .push(BalanceChange {
                block_access_index: write.metadata.tx_index,
                pre_balance: H256::from(pre_balance),
                post_balance: H256::from(post_balance),
            });
    }

    changes
}

fn build_nonce_changes_ubt(writes: &[UBTWitnessEntry]) -> BTreeMap<H160, Vec<NonceChange>> {
    let mut changes: BTreeMap<H160, Vec<NonceChange>> = BTreeMap::new();

    for write in writes {
        if write.metadata.key_type != UBTKeyType::BasicData {
            continue;
        }

        let mut pre_nonce = [0u8; 8];
        let mut post_nonce = [0u8; 8];
        pre_nonce.copy_from_slice(&write.pre_value[8..16]);
        post_nonce.copy_from_slice(&write.post_value[8..16]);

        changes
            .entry(write.metadata.address)
            .or_insert_with(Vec::new)
            .push(NonceChange {
                block_access_index: write.metadata.tx_index,
                pre_nonce,
                post_nonce,
            });
    }

    changes
}

fn build_code_changes_ubt(writes: &[UBTWitnessEntry]) -> BTreeMap<H160, Vec<CodeChange>> {
    let mut code_sizes: BTreeMap<H160, (u32, u32)> = BTreeMap::new();

    for write in writes {
        if write.metadata.key_type == UBTKeyType::BasicData {
            let code_size = ((write.post_value[5] as u32) << 16)
                | ((write.post_value[6] as u32) << 8)
                | (write.post_value[7] as u32);

            if code_size > 0 {
                code_sizes.insert(write.metadata.address, (write.metadata.tx_index, code_size));
            }
        }
    }

    let mut changes: BTreeMap<H160, Vec<CodeChange>> = BTreeMap::new();

    for write in writes {
        if write.metadata.key_type == UBTKeyType::CodeHash {
            let code_hash = H256::from(write.post_value);
            let (tx_index, code_size) = code_sizes
                .get(&write.metadata.address)
                .copied()
                .unwrap_or((write.metadata.tx_index, 0));

            changes
                .entry(write.metadata.address)
                .or_insert_with(Vec::new)
                .push(CodeChange {
                    block_access_index: tx_index,
                    code_hash,
                    code_size,
                });
        }
    }

    changes
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_empty_bal() {
        let bal = BlockAccessList::from_ubt_entries(&[], &[]);
        assert_eq!(bal.accounts.len(), 0);

        // Empty BAL should have deterministic hash
        let hash = bal.hash();
        assert_eq!(hash.len(), 32);
    }

    #[test]
    fn test_bal_hash_determinism() {
        let bal1 = BlockAccessList::from_ubt_entries(&[], &[]);
        let bal2 = BlockAccessList::from_ubt_entries(&[], &[]);

        assert_eq!(bal1.hash(), bal2.hash());
    }
}
