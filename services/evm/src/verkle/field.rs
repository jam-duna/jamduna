//! Finite field operations for Bandersnatch / Banderwagon (BLS12-381 scalar field).
//!
//! This module re-exports arkworks' Bandersnatch field and curve types directly.

pub use ark_ed_on_bls12_381_bandersnatch::{Fq, Fr, EdwardsAffine, EdwardsProjective, EdwardsConfig};
pub use primitive_types::U256;

// Expose the modulus constant for compatibility
pub const FQ_MODULUS: U256 = U256([
    0xffffffff00000001,
    0x53bda402fffe5bfe,
    0x3339d80809a1d805,
    0x73eda753299d7d48,
]);
