#![no_std]
#![no_main]

extern crate alloc;

// Railgun service modules
#[path = "refiner.rs"]
mod refiner;
#[path = "accumulator.rs"]
mod accumulator;
#[path = "state.rs"]
mod state;
#[path = "crypto.rs"]
mod crypto;
#[path = "errors.rs"]
mod errors;

use alloc::{vec::Vec, string::String};
#[cfg(any(
    all(
        any(target_arch = "riscv32", target_arch = "riscv64"),
        target_feature = "e"
    ),
    doc
))]
use polkavm_derive::min_stack_size;

#[cfg(not(any(
    all(
        any(target_arch = "riscv32", target_arch = "riscv64"),
        target_feature = "e"
    ),
    doc
)))]
macro_rules! min_stack_size {
    ($size:expr) => {};
}

#[cfg(any(
    all(
        any(target_arch = "riscv32", target_arch = "riscv64"),
        target_feature = "e"
    ),
    doc
))]
use polkavm_derive::sbrk as polkavm_sbrk;

#[cfg(not(any(
    all(
        any(target_arch = "riscv32", target_arch = "riscv64"),
        target_feature = "e"
    ),
    doc
)))]
fn polkavm_sbrk(_size: usize) {}

use simplealloc::SimpleAlloc;
use utils::{
    constants::FIRST_READABLE_ADDRESS,
    functions::{
        fetch_accumulate_inputs, fetch_extrinsics, fetch_refine_context, fetch_work_item,
        log_error, log_info, parse_accumulate_args, parse_refine_args,
    },
};

// Import log_crit only for RISC-V target (used in panic handler)
#[cfg(all(not(test), target_arch = "riscv32", target_feature = "e"))]
use utils::functions::log_crit;

const SIZE0: usize = 0x200000; // 2 MB stack
min_stack_size!(SIZE0);

#[global_allocator]
static ALLOCATOR: SimpleAlloc<FIRST_READABLE_ADDRESS> = SimpleAlloc::<FIRST_READABLE_ADDRESS>::new();

/// Refine entry point for JAM Railgun service
///
/// Called by JAM validators during work package processing.
/// Verifies ZK proofs, checks state witnesses, and outputs write intents.
///
/// # Arguments
/// - `start_address`: Memory address containing refine arguments
/// - `length`: Length of refine arguments in bytes
///
/// # Returns
/// - `(output_ptr, output_len)`: Pointer and length of serialized execution effects
#[polkavm_derive::polkavm_export]
pub extern "C" fn refine(start_address: u32, length: u32) -> (u32, u32) {
    log_info("🔫 Railgun refine started");

    // Parse refine arguments from PVM memory
    let refine_args = match parse_refine_args(start_address, length) {
        Ok(args) => args,
        Err(e) => {
            log_error(&alloc::format!("Failed to parse refine args: {:?}", e));
            return (0, 0);
        }
    };

    // Fetch refine context (state root, timeslot, etc.)
    let refine_context = match fetch_refine_context() {
        Ok(ctx) => ctx,
        Err(e) => {
            log_error(&alloc::format!("Failed to fetch refine context: {:?}", e));
            return (0, 0);
        }
    };

    log_info(&alloc::format!(
        "Processing work item {} for service {}",
        refine_args.work_item_index, refine_args.service_id
    ));

    // Fetch work item
    let _work_item = match fetch_work_item(refine_args.work_item_index) {
        Ok(wi) => wi,
        Err(e) => {
            log_error(&alloc::format!("Failed to fetch work item: {:?}", e));
            return (0, 0);
        }
    };

    // Fetch extrinsics for this work item
    let extrinsics = match fetch_extrinsics(refine_args.work_item_index) {
        Ok(exts) => exts,
        Err(e) => {
            log_error(&alloc::format!("Failed to fetch extrinsics: {:?}", e));
            return (0, 0);
        }
    };

    log_info(&alloc::format!("Processing {} extrinsics", extrinsics.len()));

    // Process extrinsics and generate write intents
    let execution_effects = match refiner::process_extrinsics(
        refine_args.service_id,
        &refine_context,
        &extrinsics,
    ) {
        Ok(effects) => effects,
        Err(e) => {
            log_error(&alloc::format!("Refine processing failed: {:?}", e));
            return (0, 0);
        }
    };

    log_info(&alloc::format!(
        "Generated {} write intents",
        execution_effects.len()
    ));

    // Serialize execution effects for accumulate
    let serialized = match refiner::serialize_effects(&execution_effects) {
        Ok(data) => data,
        Err(e) => {
            log_error(&alloc::format!("Failed to serialize effects: {:?}", e));
            return (0, 0);
        }
    };

    log_info("🔫 Railgun refine completed successfully");

    // Return pointer and length to serialized output
    (serialized.as_ptr() as u32, serialized.len() as u32)
}

/// Accumulate entry point for JAM Railgun service
///
/// Called after refine to apply verified state changes and process deferred transfers.
///
/// # Arguments
/// - `start_address`: Memory address containing accumulate arguments
/// - `length`: Length of accumulate arguments in bytes
///
/// # Returns
/// - `(output_ptr, output_len)`: Pointer and length of result (currently unused)
#[polkavm_derive::polkavm_export]
pub extern "C" fn accumulate(start_address: u32, length: u32) -> (u32, u32) {
    log_info("🔫 Railgun accumulate started");

    // Parse accumulate arguments
    let accumulate_args = match parse_accumulate_args(start_address, length) {
        Ok(args) => args,
        Err(e) => {
            log_error(&alloc::format!("Failed to parse accumulate args: {:?}", e));
            return (0, 0);
        }
    };

    // Fetch accumulate inputs (includes deferred transfers)
    let inputs = match fetch_accumulate_inputs(accumulate_args.service_id) {
        Ok(inp) => inp,
        Err(e) => {
            log_error(&alloc::format!("Failed to fetch accumulate inputs: {:?}", e));
            return (0, 0);
        }
    };

    log_info(&alloc::format!(
        "Processing {} accumulate inputs",
        inputs.len()
    ));

    // Process accumulate inputs (apply writes, handle transfers)
    match accumulator::process_accumulate(
        accumulate_args.service_id,
        &accumulate_args,
        &inputs.extrinsics,
    ) {
        Ok(_) => {
            log_info("🔫 Railgun accumulate completed successfully");
            (0, 0)
        }
        Err(e) => {
            log_error(&alloc::format!("Accumulate processing failed: {:?}", e));
            (0, 0)
        }
    }
}

/// Panic handler for no_std environment
#[cfg(all(not(test), target_arch = "riscv32", target_feature = "e"))]
#[panic_handler]
fn panic(info: &core::panic::PanicInfo) -> ! {
    log_crit(&alloc::format!("🔫 PANIC: {}", info));
    core::arch::asm!("unimp", options(noreturn));
}

/// Test panic handler for non-RISC-V targets
#[cfg(not(all(not(test), target_arch = "riscv32", target_feature = "e")))]
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
