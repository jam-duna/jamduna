//! Structured Reference String (SRS) generation matching go-ipa/go-verkle.
//!
//! Algorithm:
//! 1) hash = SHA256(seed || counter_be)
//! 2) x = Fq::from_bytes_le(hash)   // reduction mod p
//! 3) x_bytes = x.to_bytes_le()
//! 4) point = BandersnatchPoint::from_bytes(x_bytes) // includes subgroup + curve checks
//! 5) if step 4 fails, increment counter and retry

use alloc::vec::Vec;
use sha2::{Digest, Sha256};
use ark_ff::{PrimeField, BigInteger};

use super::curve::BandersnatchPoint;
use super::field::Fq;

const SEED: &[u8] = b"eth_verkle_oct_2021";

/// Generate the canonical 256-point SRS.
pub fn generate_srs() -> [BandersnatchPoint; 256] {
    let points = generate_srs_points(256);
    points
        .try_into()
        .expect("generate_srs_points(256) must return exactly 256 points")
}

/// Generate the first `num_points` SRS elements following go-ipa.
pub fn generate_srs_points(num_points: usize) -> Vec<BandersnatchPoint> {
    let mut points = Vec::with_capacity(num_points);
    let mut counter: u64 = 0;

    while points.len() < num_points {
        // Hash seed || counter (big-endian)
        let mut hasher = Sha256::new();
        hasher.update(SEED);
        hasher.update(&counter.to_be_bytes());
        let hash: [u8; 32] = hasher.finalize().into();

        // Reduce hash modulo q to get field element (matches go-ipa)
        let x = Fq::from_be_bytes_mod_order(&hash);

        // Serialize back to big-endian bytes for decompression
        let mut x_bytes = [0u8; 32];
        let x_le = x.into_bigint().to_bytes_le();
        x_bytes[..x_le.len()].copy_from_slice(&x_le);
        x_bytes.reverse(); // Convert to big-endian

        // Attempt decompression (handles curve + subgroup checks)
        if let Ok(point) = BandersnatchPoint::from_bytes(&x_bytes) {
            points.push(point);
        }

        counter += 1;
    }

    points
}
