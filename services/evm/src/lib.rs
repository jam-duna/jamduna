//! EVM service library exports
//!
//! This module exports shared types that can be used by other services

#![no_std]
#![allow(dead_code)]

extern crate alloc;

// Precompile contracts: (address_byte, bytecode, name)
pub const PRECOMPILES: &[(u8, &[u8], &str)] = &[
    (
        0x01,
        include_bytes!("../contracts/usdm-runtime.bin"),
        "usdm-runtime.bin",
    ),
    (
        0x02,
        include_bytes!("../contracts/gov-runtime.bin"),
        "gov-runtime.bin",
    ),
    (
        0x03,
        include_bytes!("../contracts/math-runtime.bin"),
        "math-runtime.bin",
    ),
];

// Declare all modules (same as main.rs)
#[path = "accumulator.rs"]
pub mod accumulator;
#[path = "backend.rs"]
mod backend;
#[path = "block.rs"]
pub mod block;
#[path = "bmt.rs"]
mod bmt;
#[path = "da.rs"]
pub mod da;
#[path = "genesis.rs"]
mod genesis;
#[path = "meta_sharding.rs"]
pub mod meta_sharding;
#[path = "mmr.rs"]
pub mod mmr;
#[path = "receipt.rs"]
mod receipt;
#[path = "refiner.rs"]
mod refiner;
#[path = "contractsharding.rs"]
mod contractsharding;
#[path = "state.rs"]
mod state;
#[path = "tx.rs"]
mod tx;
#[path = "verkle.rs"]
pub mod verkle;
#[path = "verkle_constants.rs"]
pub mod verkle_constants;
#[path = "writes.rs"]
mod writes;
#[path = "witness_events.rs"]
mod witness_events;

// Re-export commonly used types
pub use block::EvmBlockPayload;
pub use contractsharding::format_object_id;
pub use writes::serialize_execution_effects;

// Provide stub for fetch_object host function during testing
#[cfg(test)]
#[unsafe(no_mangle)]
pub extern "C" fn fetch_object(_s: u64, _ko: u64, _kz: u64, _o: u64, _f: u64, _l: u64) -> u64 {
    0 // Return 0 (not found) - tests don't need actual DA operations
}

// Provide stub for host_fetch_verkle during testing
#[cfg(test)]
#[unsafe(no_mangle)]
pub extern "C" fn host_fetch_verkle(
    _fetch_type: u64,
    _address_ptr: u64,
    _key_ptr: u64,
    _output_ptr: u64,
    _output_max_len: u64,
) -> u64 {
    0 // Return 0 (not found) - tests don't need actual Verkle operations
}
