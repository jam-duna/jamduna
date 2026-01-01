#![no_std]
#![no_main]

extern crate alloc;

// Backend modules
#[path = "accumulator.rs"]
mod accumulator;
#[path = "backend.rs"]
mod backend;
#[path = "block.rs"]
mod block;
#[path = "bmt.rs"]
mod bmt;
#[path = "da.rs"]
mod da;
#[path = "genesis.rs"]
mod genesis;
#[path = "meta_sharding.rs"]
mod meta_sharding;
#[path = "mmr.rs"]
mod mmr;
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
mod verkle;
#[path = "verkle_constants.rs"]
mod verkle_constants;
#[path = "verkle_proof.rs"]
mod verkle_proof;
#[path = "block_access_list.rs"]
mod block_access_list;
#[path = "writes.rs"]
mod writes;
#[path = "witness_events.rs"]
mod witness_events;

use alloc::{format, vec::Vec};
#[cfg(any(
    all(
        any(target_arch = "riscv32", target_arch = "riscv64"),
        target_feature = "e"
    ),
    doc
))]
use polkavm_derive::min_stack_size;
use refiner::BlockRefiner;
use contractsharding::format_object_id;
use writes::serialize_execution_effects;
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

const SIZE0: usize = 0x200000;
min_stack_size!(SIZE0);

const SIZE1: usize = 0x200000;
#[global_allocator]
static ALLOCATOR: SimpleAlloc<SIZE1> = SimpleAlloc::new();

// Precompile contracts: (address_byte, bytecode, name) - matches lib.rs
const PRECOMPILES: &[(u8, &[u8], &str)] = &[
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
    (
        0x10,
        include_bytes!("../contracts/shield-runtime.bin"),
        "shield-runtime.bin",
    ),
];


#[polkavm_derive::polkavm_export]
extern "C" fn refine(start_address: u64, length: u64) -> (u64, u64) {
    polkavm_sbrk(4096 * 4096);
    let Some(refine_args) = parse_refine_args(start_address, length) else {
        log_error("Refine: parse_refine_args failed");
        return empty_output();
    };

    // Fetch refine context once for the entire function
    let refine_context = match fetch_refine_context() {
        Some(context) => {
            log_info(&format!(
                "📍 Refine context state_root: {}",
                format_object_id(&context.state_root)
            ));
            context
        }
        None => {
            log_error("Failed to fetch refine context");
            return empty_output();
        }
    };

    let Some(work_item) = fetch_work_item(refine_args.wi_index) else {
        log_error(&format!(
            "Refine: fetch_work_item failed for wi_index={}",
            refine_args.wi_index
        ));
        return empty_output();
    };

    // Fetch extrinsics for this work item
    let extrinsics = match fetch_extrinsics(refine_args.wi_index) {
        Ok(exts) => exts,
        Err(e) => {
            log_error(&format!("Refine: fetch_extrinsics failed: {:?}", e));
            return empty_output();
        }
    };

    log_info(&format!(
        "📦 Refine: Fetched {} extrinsics",
        extrinsics.len()
    ));

    // Process work item and execute transactions
    let (execution_effects, accumulate_instructions, contract_witness_index_start, contract_witness_payload_length) = match BlockRefiner::from_work_item(
        refine_args.wi_index,
        &work_item,
        &extrinsics,
        &refine_context,
        &refine_args,
    ) {
        Some((effects, instructions, idx_start, payload_len)) => (effects, instructions, idx_start, payload_len),
        None => {
            log_error("Refine: from_work_item failed");
            return empty_output();
        }
    };

    // Serialize ExecutionEffects (includes conversion to ObjectCandidateWrite and logging)
    let mut buffer = serialize_execution_effects(&execution_effects, contract_witness_index_start, contract_witness_payload_length);

    log_info(&format!(
        "📝 Refine: ExecutionEffects serialized to {} bytes",
        buffer.len()
    ));

    // DEBUG: Show first few bytes of execution effects buffer
    if !buffer.is_empty() {
        let preview_len = core::cmp::min(20, buffer.len());
        let preview_bytes = &buffer[0..preview_len];
        log_info(&format!(
            "📝 ExecutionEffects buffer preview: {:02x?}",
            preview_bytes
        ));
    }

    // Append serialized accumulate instructions to the output
    if !accumulate_instructions.is_empty() {
        use utils::host_functions::AccumulateInstruction;
        let accumulate_bytes = AccumulateInstruction::serialize_all(&accumulate_instructions);
        log_info(&format!(
            "📝 Appending {} accumulate instructions ({} bytes) to refine output",
            accumulate_instructions.len(),
            accumulate_bytes.len()
        ));

        // DEBUG: Show first few bytes of accumulate instructions
        if !accumulate_bytes.is_empty() {
            let preview_len = core::cmp::min(20, accumulate_bytes.len());
            let preview_bytes = &accumulate_bytes[0..preview_len];
            log_info(&format!(
                "📝 AccumulateInstructions buffer preview: {:02x?}",
                preview_bytes
            ));
        }

        buffer.extend_from_slice(&accumulate_bytes);
    }

    log_info(&format!(
        "📝 Refine: Total output buffer size: {} bytes",
        buffer.len()
    ));

    leak_output(buffer)
}

fn empty_output() -> (u64, u64) {
    (FIRST_READABLE_ADDRESS as u64, 0)
}

fn leak_output(mut buffer: Vec<u8>) -> (u64, u64) {
    let ptr = buffer.as_mut_ptr() as u64;
    let len = buffer.len() as u64;
    log_info(&format!("leak_output ptr=0x{:x} len={}", ptr, len));
    core::mem::forget(buffer);
    (ptr, len)
}

/// Accumulate orders all ExecutionEffects from refine calls and produces a final commitment across objects
#[polkavm_derive::polkavm_export]
pub extern "C" fn accumulate(start_address: u64, length: u64) -> (u64, u64) {

    let Some(args) = parse_accumulate_args(start_address, length) else {
        log_error("Accumulate: parse_accumulate_args failed");
        return empty_output();
    };


    if args.num_accumulate_inputs == 0 {
        log_error("Accumulate: num_accumulate_inputs is zero, returning empty");
        return empty_output();
    }
    let accumulate_inputs = match fetch_accumulate_inputs(args.num_accumulate_inputs as u64) {
        Ok(inputs) => {
            inputs
        }
        Err(e) => {
            log_error(&format!(
                "Accumulate: fetch_accumulate_inputs failed: {:?}",
                e
            ));
            return empty_output();
        }
    };

    // Accumulate execution effects from any payloads (Transactions, Blocks)
    let Some(accumulate_root) =
        accumulator::BlockAccumulator::accumulate(args.s, args.t, &accumulate_inputs)
    else {
        log_error("❌ Accumulate: BlockAccumulator::accumulate returned None");
        return empty_output();
    };

    leak_output(accumulate_root.to_vec())
}

// Panic handler is only needed for targets that don't have std
// When compiling for development/testing, std provides its own panic handler
#[cfg(all(not(test), target_arch = "riscv32", target_feature = "e"))]
#[panic_handler]
fn panic(info: &core::panic::PanicInfo) -> ! {
    let message = match info.location() {
        Some(location) => format!(
            "panic at {}:{}:{} — {}",
            location.file(),
            location.line(),
            location.column(),
            info
        ),
        None => format!("panic: {}", info),
    };

    log_crit(&message);

    unsafe {
        core::arch::asm!("unimp", options(noreturn));
    }
}
