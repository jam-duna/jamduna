//! Bandersnatch / Banderwagon group operations and compression.
//!
//! Uses arkworks Bandersnatch field types for proper Montgomery arithmetic.

use core::fmt;
use ark_ff::{Field, PrimeField, BigInteger, Zero, One};
use primitive_types::U256;

use super::field::{Fq, Fr, FQ_MODULUS};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum VerkleError {
    InvalidLength,
    NonCanonicalFieldElement,
    NotOnCurve,
    NotInSubgroup,
    NotASquare,
}

/// Twisted Edwards parameter a = -5 mod p.
fn curve_a() -> Fq {
    -Fq::from(5u64)
}

/// Twisted Edwards parameter d for Bandersnatch.
fn curve_d() -> Fq {
    // 0x6389c12633c267cbc66e3bf86be3b6d8cb66677177e54f92b369f2f5188d58e7
    let bytes = [
        0x63, 0x89, 0xc1, 0x26, 0x33, 0xc2, 0x67, 0xcb,
        0xc6, 0x6e, 0x3b, 0xf8, 0x6b, 0xe3, 0xb6, 0xd8,
        0xcb, 0x66, 0x67, 0x71, 0x77, 0xe5, 0x4f, 0x92,
        0xb3, 0x69, 0xf2, 0xf5, 0x18, 0x8d, 0x58, 0xe7,
    ];
    let mut le_bytes = bytes;
    le_bytes.reverse();
    Fq::from_le_bytes_mod_order(&le_bytes)
}

/// Banderwagon point in affine coordinates.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct BandersnatchPoint {
    pub x: Fq,
    pub y: Fq,
}

impl BandersnatchPoint {
    /// Identity (0,1).
    pub fn identity() -> Self {
        Self {
            x: Fq::zero(),
            y: Fq::one(),
        }
    }

    /// Generator point (coordinates from go-ipa test vectors, little-endian).
    pub fn generator() -> Self {
        const X_BYTES: [u8; 32] = [
            0x29, 0xc1, 0x32, 0xcc, 0x2c, 0x0b, 0x34, 0xc5,
            0x74, 0x37, 0x11, 0x77, 0x7b, 0xbe, 0x42, 0xf3,
            0x2b, 0x79, 0xc0, 0x22, 0xad, 0x99, 0x84, 0x65,
            0xe1, 0xe7, 0x18, 0x66, 0xa2, 0x52, 0xae, 0x18,
        ];
        const Y_BYTES: [u8; 32] = [
            0x2a, 0x6c, 0x66, 0x9e, 0xda, 0x12, 0x3e, 0x0f,
            0x15, 0x7d, 0x8b, 0x50, 0xba, 0xdc, 0xd5, 0x86,
            0x35, 0x8c, 0xad, 0x81, 0xee, 0xe4, 0x64, 0x60,
            0x5e, 0x31, 0x67, 0xb6, 0xcc, 0x97, 0x41, 0x66,
        ];

        // Hex strings from test vectors are big-endian, reverse to little-endian for arkworks
        let mut x_le = X_BYTES;
        x_le.reverse();
        let mut y_le = Y_BYTES;
        y_le.reverse();

        Self {
            x: Fq::from_le_bytes_mod_order(&x_le),
            y: Fq::from_le_bytes_mod_order(&y_le),
        }
    }

    pub fn to_affine(&self) -> (Fq, Fq) {
        (self.x, self.y)
    }

    pub fn is_identity(&self) -> bool {
        self.x.is_zero() && self.y.is_one()
    }

    /// Equality up to the quotient group {(x,y), (-x,-y)}.
    pub fn equivalent(&self, other: &Self) -> bool {
        // Check if (x1, y1) == (x2, y2) or (x1, y1) == (-x2, -y2)
        (self.x == other.x && self.y == other.y) ||
        (self.x == -other.x && self.y == -other.y)
    }

    /// Compression: return x (negated when y is lexicographically smaller).
    pub fn to_bytes(&self) -> [u8; 32] {
        let mut out_x = self.x;
        if !is_lex_largest(self.y) {
            out_x = -out_x;
        }
        field_to_bytes_be(out_x)
    }

    /// Map a point to the scalar field using x/y (matches go-ipa Banderwagon mapping).
    pub fn map_to_scalar_field(&self) -> Fr {
        let y_inv = self
            .y
            .inverse()
            .expect("bandersnatch points always have non-zero y except identity");
        let ratio = self.x * y_inv;
        let mut le_bytes = [0u8; 32];
        let ratio_bytes = ratio.into_bigint().to_bytes_le();
        le_bytes[..ratio_bytes.len()].copy_from_slice(&ratio_bytes);
        Fr::from_le_bytes_mod_order(&le_bytes)
    }

    /// Decompression from 32-byte compressed form.
    pub fn from_bytes(bytes: &[u8; 32]) -> Result<Self, VerkleError> {
        // Reject non-canonical encodings (>= modulus).
        let x = canonical_fq_from_be(bytes)?;

        // Special case: X=0 decodes to identity (0, 1)
        if x.is_zero() {
            return Ok(Self::identity());
        }

        let (x, y) = Self::recover_y(x)?;
        Ok(Self { x, y })
    }

    /// Edwards point addition using arkworks' optimized projective coordinates.
    pub fn add(&self, other: &Self) -> Self {
        use ark_ec::CurveGroup;
        use super::field::{EdwardsAffine, EdwardsProjective};

        // Convert to affine, convert to projective, add, convert back
        let p1_affine = EdwardsAffine::new_unchecked(self.x, self.y);
        let p2_affine = EdwardsAffine::new_unchecked(other.x, other.y);
        let p1: EdwardsProjective = p1_affine.into();
        let p2: EdwardsProjective = p2_affine.into();
        let p3 = (p1 + p2).into_affine();

        Self { x: p3.x, y: p3.y }
    }

    /// Scalar multiplication using arkworks' optimized implementation.
    pub fn mul(&self, scalar: &Fr) -> Self {
        use ark_ec::CurveGroup;
        use super::field::{EdwardsAffine, EdwardsProjective};

        // Convert to affine, then projective, do scalar mul, convert back
        let p_affine = EdwardsAffine::new_unchecked(self.x, self.y);
        let p: EdwardsProjective = p_affine.into();
        let result = (p * scalar).into_affine();

        Self { x: result.x, y: result.y }
    }

    /// Multi-scalar multiplication using arkworks' optimized MSM.
    ///
    /// Complexity: O(n/log n) using Pippenger's algorithm
    /// For 256-element MSMs: ~10-20x speedup vs naive
    pub fn msm(points: &[Self], scalars: &[Fr]) -> Self {
        use ark_ec::{CurveGroup, VariableBaseMSM};
        use super::field::EdwardsProjective;

        assert_eq!(points.len(), scalars.len());

        if points.is_empty() {
            return Self::identity();
        }

        // Convert our affine points to arkworks affine points
        use super::field::EdwardsAffine;
        let affine_points: alloc::vec::Vec<EdwardsAffine> = points
            .iter()
            .map(|p| EdwardsAffine::new_unchecked(p.x, p.y))
            .collect();

        // Use arkworks' optimized MSM implementation
        let result = EdwardsProjective::msm(&affine_points, scalars)
            .expect("MSM with equal length inputs should not fail")
            .into_affine();

        Self { x: result.x, y: result.y }
    }

    /// Compute y from x using the curve equation, choosing lexicographically largest y.
    fn recover_y(x: Fq) -> Result<(Fq, Fq), VerkleError> {
        let a = curve_a();
        let d = curve_d();

        // Twisted Edwards: a*x^2 + y^2 = 1 + d*x^2*y^2
        // Solve for y^2: y^2 = (a*x^2 - 1) / (d*x^2 - 1)
        let x2 = x.square();

        // Subgroup check: 1 - a*x^2 must be a square
        let subgroup_check = Fq::one() - a * x2;
        if !matches!(subgroup_check.legendre(), ark_ff::LegendreSymbol::QuadraticResidue) {
            return Err(VerkleError::NotInSubgroup);
        }

        let num = a * x2 - Fq::one();
        let den = d * x2 - Fq::one();

        let den_inv = den.inverse().ok_or(VerkleError::NotOnCurve)?;
        let y2 = num * den_inv;

        let y = y2.sqrt().ok_or(VerkleError::NotASquare)?;

        // Choose lexicographically largest y
        let y_final = if is_lex_largest(y) { y } else { -y };
        Ok((x, y_final))
    }
}

/// Check if field element is lexicographically largest (> (q-1)/2).
fn is_lex_largest(f: Fq) -> bool {
    let bytes = field_to_bytes_be(f);

    // (q-1)/2 in big-endian hex
    const HALF_MODULUS: [u8; 32] = [
        0x39, 0xf6, 0xd3, 0xa9, 0x94, 0xce, 0xbe, 0xa4,
        0x19, 0x9c, 0xec, 0x04, 0x04, 0xd0, 0xec, 0x02,
        0xa9, 0xde, 0xd2, 0x01, 0x7f, 0xff, 0x2d, 0xff,
        0xff, 0xff, 0xff, 0xff, 0x80, 0x00, 0x00, 0x00,
    ];

    bytes > HALF_MODULUS
}

/// Convert field element to big-endian bytes.
fn field_to_bytes_be(f: Fq) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    let le_bytes = f.into_bigint().to_bytes_le();
    bytes.copy_from_slice(&le_bytes);
    bytes.reverse();
    bytes
}

/// Convert big-endian bytes to field element.
fn field_from_bytes_be(bytes: &[u8; 32]) -> Fq {
    let mut le_bytes = *bytes;
    le_bytes.reverse();
    Fq::from_le_bytes_mod_order(&le_bytes)
}

/// Parse canonical big-endian field element; error if not canonical (< modulus).
pub(crate) fn canonical_fq_from_be(bytes: &[u8; 32]) -> Result<Fq, VerkleError> {
    let raw = U256::from_big_endian(bytes);
    if raw >= FQ_MODULUS {
        return Err(VerkleError::NonCanonicalFieldElement);
    }
    Ok(field_from_bytes_be(bytes))
}

impl fmt::Display for VerkleError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{:?}", self)
    }
}
