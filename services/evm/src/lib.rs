//! EVM service library exports
//!
//! This module exports shared types that can be used by other services

#![no_std]
#![allow(dead_code)]

extern crate alloc;

// Precompile contracts: (address_byte, bytecode, name)
const PRECOMPILES: &[(u8, &[u8], &str)] = &[
    (
        0x01,
        include_bytes!("../contracts/usdm-runtime.bin"),
        "usdm-runtime.bin",
    ),
    (
        0xFF,
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
#[path = "jam_gas.rs"]
mod jam_gas;
#[path = "meta_sharding.rs"]
pub mod meta_sharding;
#[path = "mmr.rs"]
pub mod mmr;
#[path = "receipt.rs"]
mod receipt;
#[path = "refiner.rs"]
mod refiner;
#[path = "sharding.rs"]
mod sharding;
#[path = "state.rs"]
mod state;
#[path = "tx.rs"]
mod tx;
#[path = "writes.rs"]
mod writes;

// Re-export commonly used types
pub use block::EvmBlockPayload;
pub use sharding::format_object_id;
pub use writes::serialize_execution_effects;
