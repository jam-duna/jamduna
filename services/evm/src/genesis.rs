use crate::sharding::{ObjectKind, code_object_id, format_object_id};
use crate::state::MajikBackend;
use alloc::format;
use primitive_types::H160;
use utils::functions::{log_info};
use utils::objects::ObjectRef;

#[allow(dead_code)]
pub const ISSUER_EOA_BYTES: [u8; 20] = [
    0xF3, 0x9F, 0xD6, 0xE5, 0x1A, 0xAD, 0x88, 0xF6, 0xF4, 0xCE, 0x6A, 0xB8, 0x82, 0x72, 0x79, 0xCF,
    0xFF, 0xB9, 0x22, 0x66,
];

/// Load a single precompile contract directly from embedded bytecode
fn load_precompile(backend: &mut MajikBackend, address: H160, bytecode: &[u8], name: &str) {
    let code_object_id = code_object_id(address);

    // log_debug(&format!(
    //     "EVM Service: Loading precompile {}: {} bytes",
    //     name,
    //     bytecode.len()
    // ));

    let object_ref = ObjectRef {
        work_package_hash: [0u8; 32],
        index_start: 0,
        payload_length: bytecode.len() as u32,
        object_kind: ObjectKind::Code as u8,
    };

    backend
        .imported_objects
        .borrow_mut()
        .insert(code_object_id, (object_ref, bytecode.to_vec()));

    log_info(&format!(
        "✅ Loaded precompile {} (object_id={}, {} bytes)",
        name,
        format_object_id(&code_object_id),
        bytecode.len()
    ));
}

/// Load precompile contracts (USDM, MATH) directly from embedded bytecode
pub fn load_precompiles(backend: &mut MajikBackend) {
    for &(addr_byte, bytecode, name) in crate::PRECOMPILES {
        let mut addr_bytes = [0u8; 20];
        addr_bytes[19] = addr_byte;
        let address = H160::from_slice(&addr_bytes);
        load_precompile(backend, address, bytecode, name);
    }
}
