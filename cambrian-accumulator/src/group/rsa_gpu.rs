//! GPU-accelerated RSA group using CUDA for batch modular exponentiation.
use super::{ElemFrom, Group, UnknownOrderGroup};
use crate::util::{int, TypeRep};
use rug::Integer;
use std::os::raw::c_void;

#[allow(clippy::module_name_repetitions)]
#[derive(Clone, Debug, PartialEq, Eq, Hash)]
/// GPU-accelerated RSA-2048 group implementation that delegates batch operations to CUDA.
pub enum RsaGpu {}

#[allow(clippy::module_name_repetitions)]
#[derive(Clone, Debug, PartialEq, Eq, Hash)]
/// A GPU-accelerated RSA 2048 group element, wrapping a GMP integer from the `rug` crate.
pub struct RsaGpuElem(Integer);

// FFI declarations for CUDA batch modular exponentiation
#[cfg(feature = "gpu")]
#[link(name = "rsa_cuda")]
extern "C" {
    fn rsa_batch_modexp(
        bases: *const c_void,
        exps: *const c_void,
        results: *mut c_void,
        n: *const c_void,
        count: i32,
    ) -> i32;
}

impl TypeRep for RsaGpu {
    type Rep = Integer;
    fn rep() -> &'static Self::Rep {
        &crate::group::rsa::RSA2048_MODULUS
    }
}

impl Group for RsaGpu {
    type Elem = RsaGpuElem;

    fn op_(modulus: &Integer, a: &RsaGpuElem, b: &RsaGpuElem) -> RsaGpuElem {
        // Single modular multiplication (fallback to CPU)
        Self::elem(int(&a.0 * &b.0) % modulus)
    }

    fn id_(_: &Integer) -> RsaGpuElem {
        Self::elem(1)
    }

    fn inv_(modulus: &Integer, x: &RsaGpuElem) -> RsaGpuElem {
        Self::elem(x.0.invert_ref(modulus).unwrap())
    }

    fn exp_(modulus: &Integer, x: &RsaGpuElem, n: &Integer) -> RsaGpuElem {
        // Single modexp - fallback to CPU for now
        // Could alternatively use GPU batch of 1 for consistency
        Self::elem(x.0.pow_mod_ref(n, modulus).unwrap())
    }

    // Override batch_exp to use GPU implementation
    fn batch_exp(bases: &[Self::Elem], exps: &[Integer]) -> Vec<Self::Elem> {
        RsaGpu::batch_exp(bases, exps)
    }
}

impl<T> ElemFrom<T> for RsaGpu
where
    Integer: From<T>,
{
    fn elem(t: T) -> RsaGpuElem {
        use crate::group::rsa::HALF_MODULUS;
        let modulus = Self::rep();
        let val = int(t) % modulus;
        if val > *HALF_MODULUS {
            RsaGpuElem(<(Integer, Integer)>::from((-val).div_rem_euc_ref(&modulus)).1)
        } else {
            RsaGpuElem(val)
        }
    }
}

impl UnknownOrderGroup for RsaGpu {
    fn unknown_order_elem_(_: &Integer) -> RsaGpuElem {
        Self::elem(2)
    }
}

impl RsaGpu {
    /// GPU-accelerated batch modular exponentiation.
    ///
    /// Computes `bases[i] ^ exps[i] mod n` for all i, using CUDA for parallel computation.
    /// This is the primary advantage of the GPU implementation.
    #[cfg(feature = "gpu")]
    pub fn batch_exp(bases: &[RsaGpuElem], exps: &[Integer]) -> Vec<RsaGpuElem> {
        assert_eq!(bases.len(), exps.len());

        if bases.is_empty() {
            return Vec::new();
        }

        // Convert bases and exponents to byte arrays for FFI
        let mut bases_bytes: Vec<[u32; 64]> = bases
            .iter()
            .map(|elem| integer_to_limbs(&elem.0))
            .collect();

        let mut exps_bytes: Vec<[u32; 64]> = exps
            .iter()
            .map(integer_to_limbs)
            .collect();

        let modulus_bytes = integer_to_limbs(Self::rep());
        let mut results_bytes: Vec<[u32; 64]> = vec![[0u32; 64]; bases.len()];

        // Call CUDA kernel via FFI
        unsafe {
            let ret = rsa_batch_modexp(
                bases_bytes.as_ptr() as *const c_void,
                exps_bytes.as_ptr() as *const c_void,
                results_bytes.as_mut_ptr() as *mut c_void,
                &modulus_bytes as *const _ as *const c_void,
                bases.len() as i32,
            );

            if ret != 0 {
                panic!("CUDA batch modexp failed with error code: {}", ret);
            }
        }

        // Convert results back to RsaGpuElem
        results_bytes
            .iter()
            .map(|limbs| Self::elem(limbs_to_integer(limbs)))
            .collect()
    }

    /// CPU fallback batch modular exponentiation (when GPU feature is disabled).
    #[cfg(not(feature = "gpu"))]
    pub fn batch_exp(bases: &[RsaGpuElem], exps: &[Integer]) -> Vec<RsaGpuElem> {
        assert_eq!(bases.len(), exps.len());

        bases
            .iter()
            .zip(exps.iter())
            .map(|(base, exp)| Self::exp(base, exp))
            .collect()
    }
}

/// Convert a rug::Integer to a 2048-bit limb array (64 x 32-bit limbs).
fn integer_to_limbs(n: &Integer) -> [u32; 64] {
    let mut limbs = [0u32; 64];
    let bytes = n.to_digits::<u8>(rug::integer::Order::Lsf);

    for (i, chunk) in bytes.chunks(4).enumerate() {
        if i >= 64 {
            break;
        }
        let mut limb = 0u32;
        for (j, &byte) in chunk.iter().enumerate() {
            limb |= (byte as u32) << (j * 8);
        }
        limbs[i] = limb;
    }

    limbs
}

/// Convert a 2048-bit limb array back to a rug::Integer.
fn limbs_to_integer(limbs: &[u32; 64]) -> Integer {
    let mut bytes = Vec::with_capacity(256); // 64 * 4 = 256 bytes

    for &limb in limbs.iter() {
        bytes.push((limb & 0xFF) as u8);
        bytes.push(((limb >> 8) & 0xFF) as u8);
        bytes.push(((limb >> 16) & 0xFF) as u8);
        bytes.push(((limb >> 24) & 0xFF) as u8);
    }

    Integer::from_digits(&bytes, rug::integer::Order::Lsf)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_init() {
        let _x = &RsaGpu::rep();
    }

    #[test]
    fn test_op() {
        let a = RsaGpu::op(&RsaGpu::elem(2), &RsaGpu::elem(3));
        assert!(a == RsaGpu::elem(6));
        let b = RsaGpu::op(&RsaGpu::elem(-2), &RsaGpu::elem(-3));
        assert!(b == RsaGpu::elem(6));
    }

    #[test]
    fn test_exp() {
        let a = RsaGpu::exp(&RsaGpu::elem(2), &int(3));
        assert!(a == RsaGpu::elem(8));
    }

    #[test]
    fn test_inv() {
        let x = RsaGpu::elem(2);
        let inv = RsaGpu::inv(&x);
        assert!(RsaGpu::op(&x, &inv) == RsaGpu::id());
    }

    #[test]
    fn test_batch_exp() {
        let bases = vec![RsaGpu::elem(2), RsaGpu::elem(3), RsaGpu::elem(5)];
        let exps = vec![int(3), int(2), int(4)];
        let results = RsaGpu::batch_exp(&bases, &exps);

        assert_eq!(results.len(), 3);
        assert_eq!(results[0], RsaGpu::elem(8));
        assert_eq!(results[1], RsaGpu::elem(9));
        assert_eq!(results[2], RsaGpu::elem(625));
    }
}
