//! Point-to-bytes hashing compatible with go-verkle/go-ipa.
//!
//! Algorithm (mapToScalarField):
//!   z = X / Y in base field (Fq)
//!   reinterpret z bytes as scalar field (Fr) element
//!   serialize Fr little-endian (32 bytes)

use ark_ff::{Field, PrimeField, BigInteger};
use super::{curve::BandersnatchPoint, field::Fr};

/// Hash point to 32-byte scalar: LE_bytes(X / Y mod q).
pub fn hash_point_to_bytes(point: &BandersnatchPoint) -> [u8; 32] {
    assert!(!point.is_identity(), "identity not allowed");

    let (x, y) = point.to_affine();
    let y_inv = y.inverse().expect("Y is zero");
    let z = x * y_inv;

    // Map base-field element into scalar field by reinterpreting little-endian bytes,
    // matching go-ipa's MapToScalarField (fp -> fr).
    let z_le = z.into_bigint().to_bytes_le();
    let scalar = Fr::from_le_bytes_mod_order(&z_le);

    // Serialize scalar to little-endian
    let mut bytes = [0u8; 32];
    let scalar_le = scalar.into_bigint().to_bytes_le();
    bytes.copy_from_slice(&scalar_le);
    bytes
}
