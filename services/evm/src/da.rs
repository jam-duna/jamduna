//! Shared DA (Data Availability) Utilities
//!
//! # Overview
//!
//! This module provides **shared utilities** used by BOTH contract sharding and meta-sharding:
//! - `ShardId` - shard identifier with local depth and routing prefix
//! - `SplitPoint` - split point metadata for SSR routing
//! - Helper functions: `take_prefix56()`, `mask_prefix56()`, `matches_prefix()`
//!
//! # Architecture
//!
//! All structures and functions in this module are **USED BY BOTH** contract sharding and meta-sharding.
//!
//! **Contract-specific code** is in `contractsharding.rs`:
//! - Contract `SSRData` with `ContractSSR` type alias
//! - Contract shard resolution and serialization
//!
//! **Meta-sharding specific code** is in `meta_sharding.rs`:
//! - Meta `MetaSSR` struct
//! - Meta-shard resolution and splitting

use core::cmp::Ordering;

// ===== Error Types =====

#[derive(Debug)]
pub enum SerializeError {
    InvalidSize,
    InvalidObjectRef,
}

// ===== Shared SSR Data Structures (Used by BOTH Contract and Meta-Sharding) =====

/// SplitPoint - tracks where a shard was split in the SSR routing tree (9 bytes)
///
/// **USED BY BOTH**: Contract sharding AND meta-sharding
///
/// A split point records: "All keys whose first `d` bits match `prefix56`
/// are routed to depth `ld` (instead of global_depth)."
///
/// Format: [1B d][1B ld][7B prefix56]
#[derive(Clone, Copy, PartialEq, Eq)]
pub struct SplitPoint {
    pub d: u8,             // Split depth (parent depth where split occurred)
    pub ld: u8,            // Local depth after split
    pub prefix56: [u8; 7], // 56-bit routing prefix (first d bits significant)
}

impl PartialOrd for SplitPoint {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for SplitPoint {
    fn cmp(&self, other: &Self) -> Ordering {
        self.d
            .cmp(&other.d)
            .then_with(|| self.prefix56.cmp(&other.prefix56))
    }
}

/// Shard ID - identifies a storage shard (8 bytes)
///
/// **USED BY BOTH**: Contract sharding AND meta-sharding
///
/// Identifies a specific shard by its local depth and routing prefix.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct ShardId {
    pub ld: u8,            // Local depth (0-56)
    pub prefix56: [u8; 7], // 56-bit routing prefix
}

impl PartialOrd for ShardId {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for ShardId {
    fn cmp(&self, other: &Self) -> Ordering {
        self.ld
            .cmp(&other.ld)
            .then_with(|| self.prefix56.cmp(&other.prefix56))
    }
}

impl ShardId {
    /// Create root shard ID (ld=0, all zeros prefix)
    /// Post-SSR: All contracts use this single root shard
    pub fn root() -> Self {
        Self {
            ld: 0,
            prefix56: [0u8; 7],
        }
    }

    /// Encode ShardId as u32 for compact routing (top 20 bits)
    ///
    /// Used in tests for ShardId serialization roundtrip validation.
    /// Format: [8 bits ld][12 bits from prefix56[0..2]]
    #[cfg(test)]
    pub fn to_u32(&self) -> u32 {
        let b0 = self.ld as u32;
        let b1 = self.prefix56[0] as u32;
        let b2 = (self.prefix56[1] >> 4) as u32; // Top 4 bits of second byte
        (b0 << 12) | (b1 << 4) | b2
    }

    /// Decode ShardId from u32 routing key (top 20 bits)
    ///
    /// Used in tests for ShardId serialization roundtrip validation.
    /// Reconstructs ld and first 12 bits of prefix56.
    #[cfg(test)]
    pub fn from_u32(val: u32) -> Self {
        let ld = ((val >> 12) & 0xFF) as u8;
        let mut prefix56 = [0u8; 7];
        prefix56[0] = ((val >> 4) & 0xFF) as u8;
        prefix56[1] = ((val & 0x0F) << 4) as u8;
        Self { ld, prefix56 }
    }
}

// ===== Shared Prefix Manipulation Utilities (Used by BOTH) =====

/// Extract first 56 bits (7 bytes) from 32-byte hash as routing prefix
///
/// **USED BY BOTH**: Contract sharding AND meta-sharding
pub fn take_prefix56(key_hash: &[u8; 32]) -> [u8; 7] {
    let mut prefix = [0u8; 7];
    prefix.copy_from_slice(&key_hash[0..7]);
    prefix
}

/// Check if key_prefix matches entry_prefix for first 'bits' bits
///
/// **USED BY BOTH**: Contract sharding AND meta-sharding
pub fn matches_prefix(key_prefix: [u8; 7], entry_prefix: [u8; 7], bits: u8) -> bool {
    let bytes_to_compare = (bits / 8) as usize;
    let remaining_bits = bits % 8;

    // Check full bytes
    if bytes_to_compare > 0 && key_prefix[0..bytes_to_compare] != entry_prefix[0..bytes_to_compare]
    {
        return false;
    }

    // Check remaining bits
    if remaining_bits > 0 && bytes_to_compare < 7 {
        let mask = 0xFF << (8 - remaining_bits);
        if (key_prefix[bytes_to_compare] & mask) != (entry_prefix[bytes_to_compare] & mask) {
            return false;
        }
    }

    true
}

/// Mask prefix to first 'bits' bits, zero out the rest
///
/// **USED BY BOTH**: Contract sharding AND meta-sharding
pub fn mask_prefix56(prefix: [u8; 7], bits: u8) -> [u8; 7] {
    if bits > 56 {
        // Clamp to 56 bits max
        return prefix;
    }

    let mut masked = [0u8; 7];
    let full_bytes = (bits / 8) as usize;
    let remaining_bits = bits % 8;

    // Copy full bytes
    if full_bytes > 0 {
        masked[0..full_bytes].copy_from_slice(&prefix[0..full_bytes]);
    }

    // Mask remaining bits
    if remaining_bits > 0 && full_bytes < 7 {
        let mask = 0xFF << (8 - remaining_bits);
        masked[full_bytes] = prefix[full_bytes] & mask;
    }

    masked
}

// Note: Meta-shard resolution and splitting functions have been moved to meta_sharding.rs
// This module now only contains shared helper functions and data structures.

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_shard_id_u32_roundtrip() {
        let shard = ShardId {
            ld: 5,
            prefix56: [0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE],
        };
        let encoded = shard.to_u32();
        let decoded = ShardId::from_u32(encoded);

        // First 20 bits should match
        assert_eq!(decoded.ld, shard.ld);
        assert_eq!(decoded.prefix56[0], shard.prefix56[0]);
        // Top 4 bits of second byte preserved
        assert_eq!(decoded.prefix56[1] & 0xF0, shard.prefix56[1] & 0xF0);
    }
}
