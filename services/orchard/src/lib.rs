#![cfg_attr(any(target_arch = "riscv32", target_arch = "riscv64"), no_std)]
#![allow(dead_code)]
#![allow(static_mut_refs)]

#[macro_use]
extern crate alloc;

#[cfg(any(target_arch = "riscv32", target_arch = "riscv64"))]
extern crate simplealloc;

#[cfg(any(target_arch = "riscv32", target_arch = "riscv64"))]
use simplealloc::SimpleAlloc;

#[cfg(not(any(target_arch = "riscv32", target_arch = "riscv64")))]
extern crate std;

#[cfg(not(any(target_arch = "riscv32", target_arch = "riscv64")))]
#[global_allocator]
static ALLOC: std::alloc::System = std::alloc::System;

#[cfg(any(target_arch = "riscv32", target_arch = "riscv64"))]
#[global_allocator]
static ALLOC: SimpleAlloc<{ 1024 * 1024 }> = SimpleAlloc::new(); // 1MB allocator

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

#[cfg(feature = "ffi")]
pub mod ffi;

// PVM-specific modules only available in polkavm mode
#[cfg(feature = "polkavm")]
pub mod refiner;
#[cfg(feature = "polkavm")]
pub mod accumulator;

#[cfg(test)]
pub mod test_utils;

// Orchard service configuration constants removed - service ID now sourced from work item
