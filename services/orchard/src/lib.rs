#![cfg_attr(any(target_arch = "riscv32", target_arch = "riscv64"), no_std)]
#![allow(dead_code)]
#![allow(static_mut_refs)]

#[macro_use]
extern crate alloc;

#[cfg(any(target_arch = "riscv32", target_arch = "riscv64"))]
use simplealloc::SimpleAlloc;

const SIZE1: usize = 0x4000000; // 64 MB heap for no_std allocator
#[cfg(any(target_arch = "riscv32", target_arch = "riscv64"))]
#[global_allocator]
static ALLOCATOR: SimpleAlloc<SIZE1> = SimpleAlloc::new();

#[cfg(not(any(target_arch = "riscv32", target_arch = "riscv64")))]
extern crate std;

// Global allocator is defined here for no_std targets.

// Conditional logging - stubs when not in polkavm mode
#[cfg(feature = "polkavm")]
pub use utils::functions::{log_info, log_error};

#[cfg(not(feature = "polkavm"))]
#[inline(always)]
pub fn log_info(_msg: &str) {
    // No-op in FFI mode
}

#[cfg(not(feature = "polkavm"))]
#[inline(always)]
pub fn log_error(_msg: &str) {
    // No-op in FFI mode
}

pub mod state;
pub mod crypto;
pub mod errors;
pub mod vk_registry;
pub mod witness;
pub mod compactblock;
pub mod compacttx;
pub mod bundle_codec;
pub mod bundle_parser;
pub mod signature_verifier;
pub mod nu7_types;

pub mod sinsemilla_nostd;

#[cfg(feature = "ffi")]
pub mod ffi;

// Transparent modules are available in polkavm mode or when the transparent feature is enabled.
#[cfg(any(feature = "polkavm", feature = "transparent", feature = "ffi"))]
pub mod transparent_parser;
#[cfg(any(feature = "polkavm", feature = "transparent", feature = "ffi"))]
pub mod transparent_utxo;
#[cfg(any(feature = "polkavm", feature = "transparent", feature = "ffi"))]
pub mod transparent_script;
#[cfg(any(feature = "polkavm", feature = "transparent", feature = "ffi"))]
pub mod transparent_verify;
#[cfg(any(feature = "polkavm", feature = "ffi"))]
pub mod objects;
#[cfg(any(feature = "polkavm", feature = "ffi"))]
pub mod refiner;
#[cfg(any(feature = "polkavm", feature = "ffi"))]
pub mod accumulator;

#[cfg(test)]
pub mod test_utils;

// Orchard service configuration constants removed - service ID now sourced from work item
