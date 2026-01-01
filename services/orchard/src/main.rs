#![allow(dead_code)]
#![cfg_attr(any(target_arch = "riscv32", target_arch = "riscv64"), no_std)]
#![cfg_attr(any(target_arch = "riscv32", target_arch = "riscv64"), no_main)]

#[macro_use]
extern crate alloc;

// Orchard service modules
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
#[path = "objects.rs"]
mod objects;
#[path = "vk_registry.rs"]
mod vk_registry;
#[path = "witness.rs"]
mod witness;
#[path = "bundle_codec.rs"]
mod bundle_codec;

// Service constants removed - service ID now sourced from work item

// no std collections directly used here
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

use simplealloc::SimpleAlloc;
use utils::{
    constants::FIRST_READABLE_ADDRESS,
    functions::{
        fetch_accumulate_inputs, fetch_extrinsics, fetch_refine_context, fetch_work_item,
        log_error, log_info, parse_accumulate_args, parse_refine_args,
    },
};

#[cfg(not(any(target_arch = "riscv32", target_arch = "riscv64")))]
fn main() {}

// Import log_crit only for RISC-V target (used in panic handler)
#[cfg(all(not(test), target_arch = "riscv32", target_feature = "e"))]
use utils::functions::log_crit;

const SIZE0: usize = 0x200000; // 2 MB stack
min_stack_size!(SIZE0);

const SIZE1: usize = 0x200000;
#[global_allocator]
static ALLOCATOR: SimpleAlloc<SIZE1> = SimpleAlloc::new();

/// Refine entry point for JAM Orchard service
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
pub extern "C" fn refine(start_address: u64, length: u64) -> (u64, u64) {
    log_info("🔫 Orchard refine started");

    // Parse refine arguments from PVM memory
    let Some(refine_args) = parse_refine_args(start_address, length) else {
        log_error("Failed to parse refine args");
        return (FIRST_READABLE_ADDRESS as u64, 0);
    };

    // Fetch refine context (state root, timeslot, etc.)
    let Some(refine_context) = fetch_refine_context() else {
        log_error("Failed to fetch refine context");
        return (FIRST_READABLE_ADDRESS as u64, 0);
    };

    log_info(&alloc::format!(
        "Processing work item {} for service {}",
        refine_args.wi_index,
        refine_args.wi_service_index
    ));

    // Fetch work item
    let Some(work_item) = fetch_work_item(refine_args.wi_index) else {
        log_error("Failed to fetch work item");
        return (FIRST_READABLE_ADDRESS as u64, 0);
    };

    // Fetch extrinsics for this work item
    let extrinsics = match fetch_extrinsics(refine_args.wi_index) {
        Ok(exts) => exts,
        Err(_) => {
            log_error("Failed to fetch extrinsics");
            return (FIRST_READABLE_ADDRESS as u64, 0);
        }
    };

    log_info(&alloc::format!("Processing {} extrinsics", extrinsics.len()));

    // Parse raw extrinsics into OrchardExtrinsic enum
    let parsed_extrinsics = match parse_orchard_extrinsics(&extrinsics) {
        Ok(parsed) => parsed,
        Err(e) => {
            log_error(&alloc::format!("Failed to parse extrinsics: {:?}", e));
            return (FIRST_READABLE_ADDRESS as u64, 0);
        }
    };

    // Process extrinsics (witnesses are now inline with each extrinsic)
    let refine_output = match refiner::process_extrinsics_witness_aware(
        work_item.service,
        &refine_context,
        &parsed_extrinsics,
        None,  // Witnesses now bundled with extrinsics
    ) {
        Ok(output) => output,
        Err(e) => {
            log_error(&alloc::format!("Witness-aware refine processing failed: {:?}", e));
            return (FIRST_READABLE_ADDRESS as u64, 0);
        }
    };

    // Extract WriteIntents directly - no conversion needed
    let execution_effects = extract_intents_from_refine_output(refine_output);

    log_info(&alloc::format!(
        "Generated {} write intents",
        execution_effects.len()
    ));

    // Serialize execution effects for accumulate
    let mut buffer = match refiner::serialize_effects(&execution_effects) {
        Ok(data) => data,
        Err(e) => {
            log_error(&alloc::format!("Failed to serialize effects: {:?}", e));
            return (FIRST_READABLE_ADDRESS as u64, 0);
        }
    };

    log_info(&alloc::format!(
        "📝 Refine: ExecutionEffects serialized to {} bytes",
        buffer.len()
    ));

    // No accumulate instructions needed in current implementation

    log_info(&alloc::format!(
        "📝 Refine: Total output buffer size: {} bytes",
        buffer.len()
    ));

    log_info("🔫 Orchard refine completed successfully");

    // Leak output (same pattern as EVM)
    let ptr = buffer.as_mut_ptr() as u64;
    let len = buffer.len() as u64;
    log_info(&alloc::format!("leak_output ptr=0x{:x} len={}", ptr, len));
    core::mem::forget(buffer);
    (ptr, len)
}

/// Accumulate entry point for JAM Orchard service
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
pub extern "C" fn accumulate(start_address: u64, length: u64) -> (u64, u64) {
    log_info("🔫 Orchard accumulate started");

    // Parse accumulate arguments
    let Some(accumulate_args) = parse_accumulate_args(start_address, length) else {
        log_error("Failed to parse accumulate args");
        return (FIRST_READABLE_ADDRESS as u64, 0);
    };

    // Fetch accumulate inputs (includes deferred transfers)
    let Ok(inputs) = fetch_accumulate_inputs(accumulate_args.num_accumulate_inputs as u64) else {
        log_error("Failed to fetch accumulate inputs");
        return (FIRST_READABLE_ADDRESS as u64, 0);
    };

    log_info(&alloc::format!(
        "Processing {} accumulate inputs",
        inputs.len()
    ));

    // Process accumulate inputs (apply writes, handle transfers)
    match accumulator::process_accumulate(
        accumulate_args.s,
        &accumulate_args,
        &inputs,
    ) {
        Ok(_) => {
            log_info("🔫 Orchard accumulate completed successfully");
            (FIRST_READABLE_ADDRESS as u64, 0)
        }
        Err(e) => {
            log_error(&alloc::format!("Accumulate processing failed: {:?}", e));
            (FIRST_READABLE_ADDRESS as u64, 0)
        }
    }
}

// Helper functions for witness-aware execution

/// Parse witness bundle from JAM work item payload
fn parse_witness_bundle_from_work_item(work_item: &utils::functions::WorkItem) -> Result<Option<witness::WitnessBundle>, errors::OrchardError> {
    // Check if this is a builder or guarantor work item by looking at payload structure
    // JAM work item payload format (simplified):
    // [extrinsic_count:4][extrinsics...][witness_present:1][witness_bundle...]

    if work_item.payload.len() < 4 {
        // Too small to have proper structure - default to builder mode
        return Ok(None);
    }

    // Parse extrinsic count
    let extrinsic_count = u32::from_le_bytes([
        work_item.payload[0],
        work_item.payload[1],
        work_item.payload[2],
        work_item.payload[3]
    ]);

    // Calculate expected extrinsics size (simplified - assumes fixed size for now)
    // In reality, this would parse variable-length extrinsics
    let mut cursor = 4;

    // Skip past extrinsics using proper variable-length parsing
    for i in 0..extrinsic_count {
        if cursor >= work_item.payload.len() {
            return Ok(None); // Builder mode - no witness section
        }

        // Parse extrinsic length (each extrinsic starts with u32 length prefix)
        if cursor + 4 > work_item.payload.len() {
            return Ok(None); // Builder mode - incomplete length prefix
        }

        let extrinsic_length = u32::from_le_bytes([
            work_item.payload[cursor],
            work_item.payload[cursor + 1],
            work_item.payload[cursor + 2],
            work_item.payload[cursor + 3]
        ]) as usize;
        cursor += 4;

        // Skip past extrinsic data
        if cursor + extrinsic_length > work_item.payload.len() {
            return Ok(None); // Builder mode - incomplete extrinsic data
        }
        cursor += extrinsic_length;

        log_info(&alloc::format!("Skipped extrinsic {}: {} bytes", i, extrinsic_length));
    }

    // Check for witness bundle presence marker
    if cursor >= work_item.payload.len() {
        return Ok(None); // Builder mode - no witness section
    }

    let witness_present = work_item.payload[cursor];
    cursor += 1;

    if witness_present == 0 {
        return Ok(None); // Builder mode - no witness
    }

    // Parse witness bundle
    if cursor >= work_item.payload.len() {
        return Err(errors::OrchardError::ParseError(
            "Witness present flag set but no witness data".into()
        ));
    }

    let witness_data = &work_item.payload[cursor..];
    let witness_bundle = witness::WitnessBundle::deserialize(witness_data)?;

    // CRITICAL: Validate witness bundle for security attacks
    witness::validate_witness_bundle(work_item.service, &witness_bundle)?;
    witness::detect_witness_attacks(&witness_bundle)?;

    log_info(&alloc::format!(
        "Parsed and validated witness bundle: {} reads, {} writes",
        witness_bundle.reads.len(),
        witness_bundle.writes.len()
    ));

    Ok(Some(witness_bundle))
}

/// Parse raw extrinsic bytes into OrchardExtrinsic enum
fn parse_orchard_extrinsics(raw_extrinsics: &[alloc::vec::Vec<u8>]) -> Result<alloc::vec::Vec<state::OrchardExtrinsic>, errors::OrchardError> {
    let mut parsed = alloc::vec::Vec::new();

    for (idx, raw) in raw_extrinsics.iter().enumerate() {
        if raw.is_empty() {
            return Err(errors::OrchardError::ParseError(
                alloc::format!("Empty extrinsic at index {}", idx)
            ));
        }

        // Parse based on discriminant byte
        // Use OrchardExtrinsic::deserialize for proper parsing
        let extrinsic = match state::OrchardExtrinsic::deserialize(raw) {
            Ok(ext) => ext,
            Err(e) => {
                return Err(errors::OrchardError::ParseError(
                    alloc::format!("Failed to parse extrinsic at index {}: {:?}", idx, e)
                ));
            }
        };
        parsed.push(extrinsic);
    }

    Ok(parsed)
}


// Removed old placeholder parsing functions - now using OrchardExtrinsic::deserialize()

/// Extract WriteIntents from RefineOutput - no conversion needed since we use utils::effects::WriteIntent directly
fn extract_intents_from_refine_output(
    refine_output: refiner::RefineOutput
) -> alloc::vec::Vec<utils::effects::WriteIntent> {
    match refine_output {
        refiner::RefineOutput::Builder { intents } => intents,
        refiner::RefineOutput::Guarantor { intents } => intents,
    }
}

// Panic handler is provided by utils crate
