//! Merkle Mountain Range (MMR) implementation
//!
//! This module implements the MMR data structure as described in the Graypaper
//! Equations E.8 and E.10 for JAM blockchain
//!
//! NOTE: Uses keccak256 for hashing to match receipt hash computation in accumulator

use alloc::{vec, vec::Vec, format};
use utils::hash_functions::keccak256;
use crate::sharding::format_object_id;

/// MMR structure holding peaks
#[derive(Debug, Clone)]
pub struct MMR {
    /// Peaks of the MMR tree
    pub peaks: Vec<Option<[u8; 32]>>,
}

impl MMR {
    /// Create a new empty MMR
    pub fn new() -> Self {
        Self {
            peaks: Vec::new(),
        }
    }

    /// Hash concatenation of left and right nodes using keccak256
    fn hash_concat(left: Option<&[u8; 32]>, right: Option<&[u8; 32]>) -> Option<[u8; 32]> {
        match (left, right) {
            (None, None) => None,
            (Some(l), Some(r)) => {
                let mut combined = Vec::with_capacity(64);
                combined.extend_from_slice(l);
                combined.extend_from_slice(r);
                Some(keccak256(&combined).into())
            }
            (Some(l), None) => {
                let mut combined = Vec::with_capacity(64);
                combined.extend_from_slice(l);
                combined.extend_from_slice(&[0u8; 32]);
                Some(keccak256(&combined).into())
            }
            (None, Some(r)) => {
                let mut combined = Vec::with_capacity(64);
                combined.extend_from_slice(&[0u8; 32]);
                combined.extend_from_slice(r);
                Some(keccak256(&combined).into())
            }
        }
    }

    /// Equation(E.8) A in GP 0.6.2
    /// Append function for the MMR
    pub fn append(&mut self, data: [u8; 32]) {
        self.peaks = Self::append_to_mmr(self.peaks.clone(), Some(data));
    }

    /// Append to MMR peaks
    fn append_to_mmr(peaks: Vec<Option<[u8; 32]>>, l: Option<[u8; 32]>) -> Vec<Option<[u8; 32]>> {
        Self::p(peaks, l, 0)
    }

    /// Equation(E.8) P in GP 0.6.2
    /// Recursive function P, combining roots
    fn p(r: Vec<Option<[u8; 32]>>, l: Option<[u8; 32]>, n: usize) -> Vec<Option<[u8; 32]>> {
        if n >= r.len() {
            let mut result = r;
            result.push(l);
            return result;
        }
        if n < r.len() && r[n].is_none() {
            return Self::r(r, n, l);
        }
        let hash = Self::hash_concat(r[n].as_ref(), l.as_ref());
        Self::p(Self::r(r, n, None), hash, n + 1)
    }

    /// Equation(E.8) R in GP 0.6.2
    /// Function R for updating Peaks
    fn r(r: Vec<Option<[u8; 32]>>, i: usize, t: Option<[u8; 32]>) -> Vec<Option<[u8; 32]>> {
        let mut s = vec![None; r.len()];
        for j in 0..r.len() {
            if i == j {
                s[j] = t;
            } else {
                s[j] = r[j];
            }
        }
        s
    }

    /// Equation(E.10) A in GP 0.6.2
    /// Compute the SuperPeak of the MMR
    pub fn super_peak(&self) -> [u8; 32] {
        Self::compute_super_peak(&self.peaks)
    }

    /// Helper function to compute SuperPeak recursively
    fn compute_super_peak(hashes: &[Option<[u8; 32]>]) -> [u8; 32] {
        // Remove nil values from hashes
        let non_nil_hashes: Vec<[u8; 32]> = hashes
            .iter()
            .filter_map(|h| *h)
            .collect();

        // Base cases
        if non_nil_hashes.is_empty() {
            // Return empty hash (e.g., zero hash)
            return keccak256(&[]).into();
        }
        if non_nil_hashes.len() == 1 {
            // Return the single hash
            return non_nil_hashes[0];
        }

        // Recursive computation
        let left = Self::compute_super_peak(
            &hashes[..hashes.len() - 1]
                .iter()
                .copied()
                .collect::<Vec<_>>()
        );
        let right = non_nil_hashes[non_nil_hashes.len() - 1];

        let mut combined = Vec::with_capacity(4 + 32 + 32);
        combined.extend_from_slice(b"peak");
        combined.extend_from_slice(&left);
        combined.extend_from_slice(&right);

        keccak256(&combined).into()
    }

    /// Compare peaks of two MMRs for equality
    #[cfg(test)]
    pub fn compare_peaks(&self, other: &MMR) -> bool {
        if self.peaks.len() != other.peaks.len() {
            return false;
        }

        for (a, b) in self.peaks.iter().zip(other.peaks.iter()) {
            match (a, b) {
                (Some(hash_a), Some(hash_b)) => {
                    if hash_a != hash_b {
                        return false;
                    }
                }
                (None, None) => continue,
                _ => return false,
            }
        }

        true
    }

    /// Serialize MMR to bytes
    /// Format: peak_count (4 bytes) || peaks (N * 33 bytes each: 1 byte flag + 32 bytes hash)
    pub fn serialize(&self) -> Vec<u8> {
        let mut buffer = Vec::with_capacity(4 + self.peaks.len() * 33);

        // Write peak count
        buffer.extend_from_slice(&(self.peaks.len() as u32).to_le_bytes());

        // Write each peak (1 byte flag + optional 32 byte hash)
        for peak in &self.peaks {
            match peak {
                Some(hash) => {
                    buffer.push(1); // Flag: Some
                    buffer.extend_from_slice(hash);
                }
                None => {
                    buffer.push(0); // Flag: None
                    buffer.extend_from_slice(&[0u8; 32]); // Padding
                }
            }
        }

        buffer
    }

    /// Deserialize MMR from bytes
    /// Expected format: peak_count (4 bytes) || peaks (N * 33 bytes each)
    pub fn deserialize(data: &[u8]) -> Option<Self> {
        if data.len() < 4 {
            return None;
        }

        let peak_count = u32::from_le_bytes(data[0..4].try_into().ok()?) as usize;
        let expected_len = 4 + peak_count * 33;

        if data.len() != expected_len {
            return None;
        }

        let mut peaks = Vec::with_capacity(peak_count);
        let mut offset = 4;

        for _ in 0..peak_count {
            let flag = data[offset];
            offset += 1;

            let mut hash = [0u8; 32];
            hash.copy_from_slice(&data[offset..offset + 32]);
            offset += 32;

            if flag == 1 {
                peaks.push(Some(hash));
            } else {
                peaks.push(None);
            }
        }

        Some(Self { peaks })
    }

    /// Read MMR from JAM State using MMR_KEY
    pub fn read_mmr(service_id: u32) -> Option<MMR> {
        use utils::{
            constants::{MMR_KEY, WHAT, NONE},
            functions::{log_error, log_debug},
            host_functions::read as host_read,
        };

        // Allocate buffer for reading MMR data
        let mut buffer = alloc::vec![0u8; 10000]; // Large enough for typical MMR

        let result = unsafe {
            host_read(
                service_id as u64,
                MMR_KEY.as_ptr() as u64,
                MMR_KEY.len() as u64,
                buffer.as_mut_ptr() as u64,
                0 as u64,
                buffer.len() as u64,
            )
        };

        // Check for failure codes
        if result == WHAT {
            log_error(&format!("❌ read_mmr: WHAT error (wrong mode), service_id={}", service_id));
            return None;
        }

        // NONE means key not found, initialize with empty MMR
        if result == NONE {
            log_debug("read_mmr: MMR_KEY not found (NONE), returning empty MMR");
            return Some(MMR::new());
        }

        // Result is the length of data read
        let len = result as usize;
        buffer.truncate(len);

        // Try to deserialize, fail if deserialization fails
        match MMR::deserialize(&buffer) {
            Some(mmr) => {
                log_debug(&format!("read_mmr: Successfully read MMR with {} peaks", mmr.peaks.len()));
                Some(mmr)
            }
            None => {
                log_error("read_mmr: Failed to deserialize MMR data");
                None
            }
        }
    }

    /// Write MMR to JAM State using MMR_KEY and return the MMR root
    pub fn write_mmr(&self) -> Option<[u8; 32]> {
        use utils::{
            constants::{MMR_KEY, WHAT, FULL},
            functions::{log_error, log_info},
            host_functions::write as host_write,
        };

        log_info(&format!("🌲 Writing MMR with {} peaks to storage", self.peaks.len()));

        let buffer = self.serialize();
        let result = unsafe {
            host_write(
                MMR_KEY.as_ptr() as u64,
                MMR_KEY.len() as u64,
                buffer.as_ptr() as u64,
                buffer.len() as u64,
            )
        };

        // Check for failure codes
        if result == WHAT || result == FULL {
            log_error(&format!("❌ Failed to write MMR to storage, error code: {}", result));
            return None;
        }

        // Compute and return MMR root
        let mmr_root = self.super_peak();
        log_info(&format!("🔗 Returning MMR root: {}", format_object_id(&mmr_root)));
        Some(mmr_root)
    }
}

impl Default for MMR {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_new_mmr() {
        let mmr = MMR::new();
        assert_eq!(mmr.peaks.len(), 0);
    }

    #[test]
    fn test_append() {
        let mut mmr = MMR::new();
        let hash1 = [1u8; 32];
        mmr.append(hash1);
        assert_eq!(mmr.peaks.len(), 1);
    }

    #[test]
    fn test_super_peak_empty() {
        let mmr = MMR::new();
        let super_peak = mmr.super_peak();
        // Should return hash of empty bytes
        assert_eq!(super_peak, blake2b(&[]));
    }


    #[test]
    fn test_serialize_empty() {
        let mmr = MMR::new();
        let serialized = mmr.serialize();
        // Should be 4 bytes (peak_count = 0)
        assert_eq!(serialized.len(), 4);
        assert_eq!(&serialized[0..4], &[0, 0, 0, 0]);
    }

    #[test]
    fn test_serialize_deserialize() {
        let mut mmr = MMR::new();
        let hash1 = [1u8; 32];
        let hash2 = [2u8; 32];

        mmr.append(hash1);
        mmr.append(hash2);

        let serialized = mmr.serialize();
        let deserialized = MMR::deserialize(&serialized).expect("Failed to deserialize");

        assert!(mmr.compare_peaks(&deserialized));
    }

    #[test]
    fn test_serialize_with_none() {
        let mut mmr = MMR::new();
        mmr.peaks.push(Some([1u8; 32]));
        mmr.peaks.push(None);
        mmr.peaks.push(Some([2u8; 32]));

        let serialized = mmr.serialize();
        let deserialized = MMR::deserialize(&serialized).expect("Failed to deserialize");

        assert_eq!(mmr.peaks.len(), deserialized.peaks.len());
        assert!(mmr.compare_peaks(&deserialized));
    }

    #[test]
    fn test_deserialize_invalid_length() {
        let invalid_data = vec![1, 0, 0, 0]; // peak_count = 1, but no peak data
        assert!(MMR::deserialize(&invalid_data).is_none());
    }
}
