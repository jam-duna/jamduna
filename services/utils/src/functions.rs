/*
 * Table of Contents:
 * 1. Parsing Functions
 *    - parse_refine_args and RefineArgs struct
 *    - parse_accumulate_args and AccumulateArgs struct
 *    - parse_standard_program_initialization_args and StandardProgramInitializationArgs struct
 *
 * 2. Child VM Related Functions
 *    - standard_program_initialization_for_child
 *    - setup_page
 *    - get_page
 *    - serialize_gas_and_registers
 *    - deserialize_gas_and_registers
 *    - initialize_pvm_registers
 *
 * 3. Logging Functions
 *    - write_result
 *    - call_log
 *
 * 4. Helper Functions
 *    - extract_discriminator
 *    - power_of_two
 *    - decode_e_l
 *    - decode_e
 *    - ceiling_divide
 *    - p_func
 *    - z_func
 */

extern crate alloc;
use alloc::format;
use alloc::string::String;
use alloc::vec;
use alloc::vec::Vec;

use crate::constants::*;
use crate::host_functions::*;

// Import ObjectId type (assuming it's available in the global scope)
pub type Hash32 = [u8; 32];
pub type ObjectId = Hash32;

/// BlockHeader struct matching the Go BlockHeader for deserialization
#[derive(Debug, Clone)]
pub struct BlockHeader {
    pub parent_header_hash: Hash32,
    pub parent_state_root: Hash32,
    pub extrinsic_hash: Hash32,
    pub slot: u32,
    // Note: The following fields are complex types that would need full definitions:
    // pub epoch_mark: Option<EpochMark>,
    // pub tickets_mark: Option<Vec<TicketBody>>,
    // pub offenders_mark: Vec<Ed25519Key>,
    // pub author_index: u16,
    // pub entropy_source: BandersnatchVrfSignature,
    // pub seal: BandersnatchVrfSignature,
    // For now, we'll include the basic fields that are most commonly needed
}

impl BlockHeader {
    /// Attempts to deserialize a BlockHeader from encoded bytes
    /// This is a simplified version that handles the basic fields
    pub fn deserialize(data: &[u8]) -> Option<Self> {
        // This is a simplified deserialization - the actual implementation
        // would need to handle the full JAM encoding format
        if data.len() < 100 {  // Minimum size for basic fields
            return None;
        }

        let mut offset = 0;

        // Extract parent_header_hash (32 bytes)
        let mut parent_header_hash = [0u8; 32];
        parent_header_hash.copy_from_slice(&data[offset..offset + 32]);
        offset += 32;

        // Extract parent_state_root (32 bytes)
        let mut parent_state_root = [0u8; 32];
        parent_state_root.copy_from_slice(&data[offset..offset + 32]);
        offset += 32;

        // Extract extrinsic_hash (32 bytes)
        let mut extrinsic_hash = [0u8; 32];
        extrinsic_hash.copy_from_slice(&data[offset..offset + 32]);
        offset += 32;

        // Extract slot (4 bytes, little-endian)
        if data.len() < offset + 4 {
            return None;
        }
        let slot = u32::from_le_bytes([
            data[offset],
            data[offset + 1],
            data[offset + 2],
            data[offset + 3],
        ]);

        Some(BlockHeader {
            parent_header_hash,
            parent_state_root,
            extrinsic_hash,
            slot,
        })
    }
}

// Parse refine args
#[repr(C)]
#[derive(Debug, Clone)]
pub struct RefineArgs {
    pub core: u16, // NEW
    pub wi_index: u16,
    pub wi_service_index: u32,
    pub wi_payload_start_address: u64,
    pub wi_payload_length: u64,
    pub wphash: [u8; 32],
}

impl Default for RefineArgs {
    fn default() -> Self {
        Self {
            core: 0,
            wi_index: 0,
            wi_service_index: 0,
            wi_payload_start_address: 0,
            wi_payload_length: 0,
            wphash: [0u8; 32],
        }
    }
}

pub fn parse_refine_args(start_address: u64, mut remaining_length: u64) -> Option<RefineArgs> {
    let mut current_address = start_address;
    let mut args = RefineArgs::default();
    if remaining_length < 4 {
        call_log(
            0,
            None,
            &format!(
                "**** parse_refine_args: returning None **** remaining_length={}",
                remaining_length
            ),
        );
        return None;
    }
    // core
    let core_full_slice = unsafe {
        core::slice::from_raw_parts(start_address as *const u8, remaining_length as usize)
    };
    let core_len = extract_discriminator(core_full_slice) as u64;
    if core_len == 0 || remaining_length < core_len {
        call_log(
            1,
            None,
            &format!(
                "**** parse_refine_args: returning None - invalid core_len={} (remaining_length={})",
                core_len, remaining_length
            ),
        );
        return None;
    }
    let core_slice = &core_full_slice[..core_len as usize];
    args.core = decode_e(core_slice) as u16;
    current_address += core_len;
    remaining_length -= core_len;

    // wi_index
    let i_full_slice = unsafe {
        core::slice::from_raw_parts(current_address as *const u8, remaining_length as usize)
    };
    let i_len = extract_discriminator(i_full_slice) as u64;
    if i_len == 0 || remaining_length < i_len {
        call_log(
            1,
            None,
            &format!(
                "**** parse_refine_args: returning None - invalid i_len={} (remaining_length={})",
                i_len, remaining_length
            ),
        );
        return None;
    }
    let i_slice = &i_full_slice[..i_len as usize];
    args.wi_index = decode_e(i_slice) as u16;

    current_address += i_len;
    remaining_length -= i_len;

    // wi_service_index
    let s_full_slice = unsafe {
        core::slice::from_raw_parts(current_address as *const u8, remaining_length as usize)
    };
    let s_len = extract_discriminator(s_full_slice) as u64;
    if s_len == 0 || remaining_length < s_len {
        call_log(
            1,
            None,
            &format!(
                "**** parse_refine_args: returning None - invalid t_len={} (remaining_length={})",
                s_len, remaining_length
            ),
        );
        return None;
    }
    let s_slice = &s_full_slice[..s_len as usize];
    args.wi_service_index = decode_e(s_slice) as u32;

    current_address += s_len;
    remaining_length -= s_len;

    // args.wi_payload_{start_address,length}
    let payload_slice = unsafe {
        core::slice::from_raw_parts(current_address as *const u8, remaining_length as usize)
    };
    let discriminator_len = extract_discriminator(payload_slice);
    let payload_len = if discriminator_len > 0 {
        decode_e(&payload_slice[..discriminator_len as usize])
    } else {
        0
    };
    current_address += discriminator_len as u64;
    remaining_length = remaining_length.saturating_sub(discriminator_len as u64);

    if remaining_length < payload_len {
        call_log(
            1,
            None,
            &format!(
                "**** parse_refine_args: returning None - insufficient length for payload (need {}, have {})",
                payload_len, remaining_length
            ),
        );
        return None;
    }
    args.wi_payload_start_address = current_address;
    args.wi_payload_length = payload_len;

    current_address += payload_len;
    remaining_length = remaining_length.saturating_sub(payload_len);
    if remaining_length < 32 {
        call_log(
            1,
            None,
            &format!(
                "**** parse_refine_args: returning None - insufficient length for wphash (need 32, have {})",
                remaining_length
            ),
        );
        return None;
    }

    // args.wphash
    let hash_slice = unsafe { core::slice::from_raw_parts(current_address as *const u8, 32) };
    args.wphash.copy_from_slice(hash_slice);

    return Some(args);
}

// Parse accumulate args
#[repr(C)]
#[derive(Debug, Clone)]
pub struct AccumulateArgs {
    pub t: u32,
    pub s: u32,
    pub num_accumulate_inputs: u32,
}

impl Default for AccumulateArgs {
    fn default() -> Self {
        Self {
            t: 0,
            s: 0,
            num_accumulate_inputs: 0,
        }
    }
}

// Parse accumulate args
#[repr(C)]
#[derive(Debug, Clone)]
pub struct AccumulateOperandArgs {
    // TODO: H, E, A, Y
    pub gas: u64,
    pub result_code: u32,
    pub output_ptr: u64,
    pub output_len: u64,
}

impl Default for AccumulateOperandArgs {
    fn default() -> Self {
        Self {
            gas: 0,
            result_code: 0,
            output_ptr: 0,
            output_len: 0,
        }
    }
}
pub fn parse_accumulate_operand_args(
    start_address: u64,
    length: u64,
) -> Option<AccumulateOperandArgs> {
    if length == 0 {
        return None;
    }
    let mut _current_address = start_address;
    let mut _remaining_length = length;

    let mut args = AccumulateOperandArgs::default();

    let mut current_address = start_address + 128; // skip over 4 hashes (H E A Y) of 128 bytes
    let mut remaining_length = length - 128;

    // Create a slice of the available data to parse gas
    let g_full_slice = unsafe {
        core::slice::from_raw_parts(current_address as *const u8, remaining_length as usize)
    };
    let g_len = extract_discriminator(g_full_slice) as u64;
    if g_len == 0 || remaining_length < g_len {
        return None;
    }
    let g_slice = &g_full_slice[..g_len as usize];
    args.gas = decode_e(g_slice) as u64;
    current_address += g_len;
    remaining_length -= g_len;

    // TODO: args.result_code = current_address[0]
    current_address += 1;
    remaining_length -= 1;

    if args.result_code == 0 {
        let o_full_slice = unsafe {
            core::slice::from_raw_parts(current_address as *const u8, remaining_length as usize)
        };
        let o_len = extract_discriminator(o_full_slice) as u64;
        if o_len == 0 || remaining_length < o_len {
            return None;
        }
        let o_slice = &o_full_slice[..o_len as usize];
        current_address += o_len;
        _remaining_length -= o_len;
        args.output_ptr = current_address;
        args.output_len = decode_e(o_slice) as u64;
        let _output_slice = unsafe {
            core::slice::from_raw_parts(args.output_ptr as *const u8, args.output_len as usize)
        };
    }
    // TODO: auth bytes
    return Some(args);
}

pub fn parse_accumulate_args(start_address: u64, length: u64) -> Option<AccumulateArgs> {
    if length == 0 {
        return None;
    }
    let mut current_address = start_address;
    let mut remaining_length = length;

    let mut args = AccumulateArgs::default();

    // Create a slice of the available data to parse t
    let t_full_slice = unsafe {
        core::slice::from_raw_parts(current_address as *const u8, remaining_length as usize)
    };
    let t_len = extract_discriminator(t_full_slice) as u64;
    if t_len == 0 || remaining_length < t_len {
        return None;
    }

    // Decode t and update pointers
    let t_slice = &t_full_slice[..t_len as usize];
    args.t = decode_e(t_slice) as u32;
    current_address += t_len;
    remaining_length -= t_len;

    // Create a new slice for the remaining data to parse s
    let s_full_slice = unsafe {
        core::slice::from_raw_parts(current_address as *const u8, remaining_length as usize)
    };
    let s_len = extract_discriminator(s_full_slice) as u64;
    if s_len == 0 || remaining_length < s_len {
        return None;
    }

    // Decode s and update pointers
    let s_slice = &s_full_slice[..s_len as usize];
    args.s = decode_e(s_slice) as u32;
    current_address += s_len;
    remaining_length -= s_len;

    let full_slice = unsafe {
        core::slice::from_raw_parts(current_address as *const u8, remaining_length as usize)
    };
    let discriminator_len = extract_discriminator(full_slice);
    if discriminator_len as usize > full_slice.len() {
        return None;
    }
    args.num_accumulate_inputs = decode_e(&full_slice[..discriminator_len as usize]) as u32;

    return Some(args);
}

// Parse standard program initialization args
#[repr(C)]
#[derive(Debug, Clone)]
pub struct StandardProgramInitializationArgs {
    pub o_len_bytes: [u8; 3],
    pub w_len_bytes: [u8; 3],
    pub z_bytes: [u8; 2],
    pub s_bytes: [u8; 3],
    pub z: u64,
    pub s: u64,
    pub o_bytes_address: u64,
    pub o_bytes_length: u64,
    pub w_bytes_address: u64,
    pub w_bytes_length: u64,
    pub c_len_bytes: [u8; 4],
    pub c_bytes_address: u64,
    pub c_bytes_length: u64,
}

impl Default for StandardProgramInitializationArgs {
    fn default() -> Self {
        Self {
            o_len_bytes: [0u8; 3],
            w_len_bytes: [0u8; 3],
            z_bytes: [0u8; 2],
            s_bytes: [0u8; 3],
            z: 0,
            s: 0,
            o_bytes_address: 0,
            o_bytes_length: 0,
            w_bytes_address: 0,
            w_bytes_length: 0,
            c_len_bytes: [0u8; 4],
            c_bytes_address: 0,
            c_bytes_length: 0,
        }
    }
}
/*
func EncodeProgram(coreCode []byte, z_val uint32, s_val uint32, o_byte []byte, w_byte []byte) []byte {
    var result []byte

    o_size := uint32(len(o_byte))
    w_size := uint32(len(w_byte))
    c_size := uint64(len(coreCode))
    result = append(result, types.E_l(uint64(o_size), 3)...)
    result = append(result, types.E_l(uint64(w_size), 3)...)
    result = append(result, types.E_l(uint64(z_val), 2)...)
    result = append(result, types.E_l(uint64(s_val), 3)...)
    result = append(result, o_byte...)
    result = append(result, w_byte...)
    result = append(result, types.E_l(c_size, 4)...)
    result = append(result, coreCode...)

    return result
}
*/
pub fn parse_standard_program_initialization_args(
    start_address: u64,
    length: u64,
) -> Option<StandardProgramInitializationArgs> {
    if length == 0 {
        call_log(
            1,
            None,
            "parse_standard_program_initialization_args: returning None - length is 0",
        );
        return None;
    }

    let mut args = StandardProgramInitializationArgs::default();

    let mut current_address = start_address;
    let mut remaining_length = length;

    if remaining_length < 3 {
        call_log(
            1,
            None,
            &format!(
                "parse_standard_program_initialization_args: returning None - insufficient length for o_len (need 3, have {})",
                remaining_length
            ),
        );
        return None;
    }
    let o_len_slice = unsafe { core::slice::from_raw_parts(current_address as *const u8, 3) };
    args.o_len_bytes.copy_from_slice(o_len_slice);
    let o_len = decode_e_l(&args.o_len_bytes);

    current_address += 3;
    remaining_length -= 3;

    if remaining_length < 3 {
        call_log(
            1,
            None,
            &format!(
                "parse_standard_program_initialization_args: returning None - insufficient length for w_len (need 3, have {})",
                remaining_length
            ),
        );
        return None;
    }
    let w_len_slice = unsafe { core::slice::from_raw_parts(current_address as *const u8, 3) };
    args.w_len_bytes.copy_from_slice(w_len_slice);
    let w_len = decode_e_l(&args.w_len_bytes);

    current_address += 3;
    remaining_length -= 3;

    if remaining_length < 2 {
        call_log(
            1,
            None,
            &format!(
                "parse_standard_program_initialization_args: returning None - insufficient length for z (need 2, have {})",
                remaining_length
            ),
        );
        return None;
    }
    let z_slice = unsafe { core::slice::from_raw_parts(current_address as *const u8, 2) };
    args.z = decode_e_l(z_slice);
    args.z_bytes.copy_from_slice(z_slice);

    current_address += 2;
    remaining_length -= 2;

    if remaining_length < 3 {
        call_log(
            1,
            None,
            &format!(
                "parse_standard_program_initialization_args: returning None - insufficient length for s (need 3, have {})",
                remaining_length
            ),
        );
        return None;
    }
    let s_slice = unsafe { core::slice::from_raw_parts(current_address as *const u8, 3) };
    args.s = decode_e_l(s_slice);
    args.s_bytes.copy_from_slice(s_slice);

    current_address += 3;
    remaining_length -= 3;

    if remaining_length < o_len {
        call_log(
            1,
            None,
            &format!(
                "parse_standard_program_initialization_args: returning None - insufficient length for o_bytes (need {}, have {})",
                o_len, remaining_length
            ),
        );
        return None;
    }
    args.o_bytes_address = current_address;
    args.o_bytes_length = o_len;

    current_address += o_len;
    remaining_length -= o_len;

    if remaining_length < w_len {
        call_log(
            1,
            None,
            &format!(
                "parse_standard_program_initialization_args: returning None - insufficient length for w_bytes (need {}, have {})",
                w_len, remaining_length
            ),
        );
        return None;
    }
    args.w_bytes_address = current_address;
    args.w_bytes_length = w_len;
    call_log(
        2,
        None,
        &format!(
            "parse_standard_program_initialization_args: set w_bytes_address=0x{:x}, w_bytes_length={}",
            args.w_bytes_address, args.w_bytes_length
        ),
    );
    current_address += w_len;
    remaining_length -= w_len;

    if remaining_length < 4 {
        call_log(
            1,
            None,
            &format!(
                "parse_standard_program_initialization_args: returning None - insufficient length for c_len (need 4, have {})",
                remaining_length
            ),
        );
        return None;
    }
    let c_len_slice = unsafe { core::slice::from_raw_parts(current_address as *const u8, 4) };
    args.c_len_bytes.copy_from_slice(c_len_slice);
    let c_len = decode_e_l(&args.c_len_bytes);

    current_address += 4;
    remaining_length -= 4;

    if remaining_length < c_len {
        call_log(
            1,
            None,
            &format!(
                "parse_standard_program_initialization_args: returning None - insufficient length for c_bytes (need {}, have {})",
                c_len, remaining_length
            ),
        );
        return None;
    }
    args.c_bytes_address = current_address;
    args.c_bytes_length = c_len;

    Some(args)
}

pub fn log_gas_checkpoint(checkpoint: u32) {
    let gas = unsafe { gas() };
    call_log(
        2,
        None,
        &format!("Gas checkpoint {}: remaining gas {}", checkpoint, gas),
    );
}

pub fn standard_program_initialization_for_child(
    z: u64,
    s: u64,
    o_bytes_address: u64,
    o_bytes_length: u64,
    w_bytes_address: u64,
    w_bytes_length: u64,
    machine_index: u32,
) {
    let o_bytes_page_len = ceiling_divide(o_bytes_length, PAGE_SIZE);
    let w_bytes_page_len = ceiling_divide(w_bytes_length, PAGE_SIZE) + z;
    let stack_page_len = ceiling_divide(s, PAGE_SIZE);

    let o_start_addreess = Z_Z;
    let o_start_page = Z_Z / PAGE_SIZE;

    let zero_result = unsafe { pages(machine_index as u64, o_start_page, o_bytes_page_len, 2) };

    if zero_result != OK {
        return call_log(
            1,
            None,
            "StandardProgramInitializationForChild: zero failed for o_bytes",
        );
    }

    let w_start_address = 2 * Z_Z + z_func(o_bytes_length);
    let w_start_page = w_start_address / PAGE_SIZE;

    let zero_result = unsafe { pages(machine_index as u64, w_start_page, w_bytes_page_len, 2) };

    if zero_result != OK {
        call_log(
            1,
            None,
            &format!(
                "StandardProgramInitializationForChild: zero failed for w_bytes, result={}",
                zero_result
            ),
        );
        return;
    }

    let s_start_address = (1u64 << 32) - 2 * Z_Z - Z_I - p_func(s);
    let s_start_page = s_start_address / PAGE_SIZE;

    let zero_result = unsafe { pages(machine_index as u64, s_start_page, stack_page_len, 2) };

    if zero_result != OK {
        return call_log(
            1,
            None,
            "StandardProgramInitializationForChild: zero failed for stack",
        );
    }
    let poke_result = unsafe {
        poke(
            machine_index as u64,
            o_bytes_address,
            o_start_addreess,
            o_bytes_length,
        )
    };
    if poke_result != OK {
        return call_log(
            1,
            None,
            "StandardProgramInitializationForChild: poke failed for o_bytes",
        );
    }
    let poke_result = unsafe {
        poke(
            machine_index as u64,
            w_bytes_address,
            w_start_address,
            w_bytes_length,
        )
    };
    if poke_result != OK {
        return call_log(
            1,
            None,
            "StandardProgramInitializationForChild: poke failed for w_bytes",
        );
    }
}

pub fn read_machine_page(
    machine_index: u32,
    start_page_id: u32,
    pages_length: u64,
    result_address: u64,
) -> core::result::Result<(), &'static str> {
    let length = pages_length * PAGE_SIZE;
    if length == 0 {
        return Err("read_machine_page: length is zero");
    }
    let page_address = start_page_id as u64 * PAGE_SIZE;

    let peek_result = unsafe {
        peek(
            machine_index as u64,
            result_address as u64,
            page_address,
            length,
        )
    };
    if peek_result != OK {
        return Err("read_machine_page: peek failed");
    }
    Ok(())
}

pub fn write_machine_page(
    machine_index: u32,
    start_page_id: u32,
    pages_length: u64,
    data_address: u64,
    data_length: u64,
) -> core::result::Result<(), &'static str> {
    let page_address = start_page_id as u64 * PAGE_SIZE;
    let _length = pages_length * PAGE_SIZE;
    // page the machine
    let page_result = unsafe {
        pages(
            machine_index as u64,
            start_page_id as u64,
            pages_length as u64,
            2,
        )
    };
    if page_result != OK {
        call_log(
            1,
            None,
            &format!("write_machine_page: pages failed, result={}", page_result),
        );
        return Err("write_machine_page: pages failed");
    }
    let poke_result = unsafe {
        poke(
            machine_index as u64,
            data_address,
            page_address,
            data_length,
        )
    };
    if poke_result != OK {
        call_log(
            1,
            None,
            &format!("write_machine_page: poke failed, result={}", poke_result),
        );
        return Err("write_machine_page: poke failed");
    }
    Ok(())
}
pub struct ExtractedMemory {
    pub o_pages_bytes: Vec<u8>,
    pub w_pages_bytes: Vec<u8>,
    pub stack_bytes: Vec<u8>,
    pub o_start_page: u64,
    pub w_start_page: u64,
    pub s_start_page: u64,
    pub o_bytes_page_len: u64,
    pub w_bytes_page_len: u64,
    pub stack_page_len: u64,
    pub o_bytes_length: u64,
    pub w_bytes_length: u64,
    pub s: u64,
}

pub fn extract_memory_from_machine(
    z: u64,
    s: u64,
    _o_bytes_address: u64,
    o_bytes_length: u64,
    _w_bytes_address: u64,
    w_bytes_length: u64,
    machine_index: u32,
) -> Result<ExtractedMemory, &'static str> {
    let o_bytes_page_len = ceiling_divide(o_bytes_length, PAGE_SIZE);
    let w_bytes_page_len = ceiling_divide(w_bytes_length, PAGE_SIZE) + z;
    let stack_page_len = ceiling_divide(s, PAGE_SIZE);

    let _o_start_addreess = Z_Z;
    let o_start_page = Z_Z / PAGE_SIZE;
    let _o_start_page_address = o_start_page * PAGE_SIZE;
    let w_start_address = 2 * Z_Z + z_func(o_bytes_length);
    let w_start_page = w_start_address / PAGE_SIZE;
    let _w_start_page_address = w_start_page * PAGE_SIZE;
    let s_start_address = (1u64 << 32) - 2 * Z_Z - Z_I - p_func(s);
    let s_start_page = s_start_address / PAGE_SIZE;
    let _s_start_page_address = s_start_page * PAGE_SIZE;

    call_log(
        2,
        None,
        &format!(
            "extract_memory_from_machine: o_start_page={}, o_bytes_page_len={}, w_start_page={}, w_bytes_page_len={}, s_start_page={}, stack_page_len={}",
            o_start_page,
            o_bytes_page_len,
            w_start_page,
            w_bytes_page_len,
            s_start_page,
            stack_page_len
        ),
    );

    let mut o_pages_bytes = vec![0u8; (PAGE_SIZE * o_bytes_page_len) as usize];
    let mut w_pages_bytes = vec![0u8; (PAGE_SIZE * w_bytes_page_len) as usize];
    let mut stack_bytes = vec![0u8; (PAGE_SIZE * stack_page_len) as usize];

    if read_machine_page(
        machine_index,
        o_start_page as u32,
        o_bytes_page_len,
        o_pages_bytes.as_mut_ptr() as u64,
    )
    .is_err()
    {
        return Err("extract_memory_from_machine: read o_bytes failed");
    }

    if read_machine_page(
        machine_index,
        w_start_page as u32,
        w_bytes_page_len,
        w_pages_bytes.as_mut_ptr() as u64,
    )
    .is_err()
    {
        return Err("extract_memory_from_machine: read w_bytes failed");
    }

    if read_machine_page(
        machine_index,
        s_start_page as u32,
        stack_page_len,
        stack_bytes.as_mut_ptr() as u64,
    )
    .is_err()
    {
        return Err("extract_memory_from_machine: read stack failed");
    }

    call_log(
        2,
        None,
        &format!(
            "extract_memory_from_machine: success, extracted o_bytes_length={}, w_bytes_length={}, s={}",
            o_bytes_length, w_bytes_length, s
        ),
    );

    Ok(ExtractedMemory {
        o_pages_bytes,
        w_pages_bytes,
        stack_bytes,
        o_start_page,
        w_start_page,
        s_start_page,
        o_bytes_page_len,
        w_bytes_page_len,
        stack_page_len,
        o_bytes_length,
        w_bytes_length,
        s,
    })
}

pub fn write_memory_to_machine(
    extracted_memory: &ExtractedMemory,
    target_machine_index: u32,
) -> core::result::Result<(), &'static str> {
    call_log(
        2,
        None,
        &format!(
            "write_memory_to_machine: writing to target machine {}, o_start_page={}, w_start_page={}, s_start_page={}",
            target_machine_index,
            extracted_memory.o_start_page,
            extracted_memory.w_start_page,
            extracted_memory.s_start_page
        ),
    );

    if let Err(_err) = write_machine_page(
        target_machine_index,
        extracted_memory.o_start_page as u32,
        extracted_memory.o_bytes_page_len,
        extracted_memory.o_pages_bytes.as_ptr() as u64,
        extracted_memory.o_bytes_length,
    ) {
        return Err("write_memory_to_machine: write o_bytes failed");
    }

    if let Err(_err) = write_machine_page(
        target_machine_index,
        extracted_memory.w_start_page as u32,
        extracted_memory.w_bytes_page_len,
        extracted_memory.w_pages_bytes.as_ptr() as u64,
        extracted_memory.w_bytes_length,
    ) {
        return Err("write_memory_to_machine: write w_bytes failed");
    }

    if let Err(_err) = write_machine_page(
        target_machine_index,
        extracted_memory.s_start_page as u32,
        extracted_memory.stack_page_len,
        extracted_memory.stack_bytes.as_ptr() as u64,
        extracted_memory.s,
    ) {
        return Err("write_memory_to_machine: write stack failed");
    }

    call_log(
        2,
        None,
        &format!(
            "write_memory_to_machine: success, wrote o_bytes_length={}, w_bytes_length={}, s={}",
            extracted_memory.o_bytes_length, extracted_memory.w_bytes_length, extracted_memory.s
        ),
    );

    Ok(())
}

pub fn copy_memory_to_another_machine(
    z: u64,
    s: u64,
    _o_bytes_address: u64,
    o_bytes_length: u64,
    _w_bytes_address: u64,
    w_bytes_length: u64,
    machine_index: u32,
    target_machine_index: u32,
) -> core::result::Result<(), &'static str> {
    let o_bytes_page_len = ceiling_divide(o_bytes_length, PAGE_SIZE);
    let w_bytes_page_len = ceiling_divide(w_bytes_length, PAGE_SIZE) + z;
    let stack_page_len = ceiling_divide(s, PAGE_SIZE);
    let _o_start_addreess = Z_Z;
    let o_start_page = Z_Z / PAGE_SIZE;
    let _o_start_page_address = o_start_page * PAGE_SIZE;
    let w_start_address = 2 * Z_Z + z_func(o_bytes_length);
    let w_start_page = w_start_address / PAGE_SIZE;
    let _w_start_page_address = w_start_page * PAGE_SIZE;
    let s_start_address = (1u64 << 32) - 2 * Z_Z - Z_I - p_func(s);
    let s_start_page = s_start_address / PAGE_SIZE;
    let _s_start_page_address = s_start_page * PAGE_SIZE;
    call_log(
        2,
        None,
        &format!(
            "copy_memory_to_another_machine: o_start_page={}, o_bytes_page_len={}, w_start_page={}, w_bytes_page_len={}, s_start_page={}, stack_page_len={}",
            o_start_page,
            o_bytes_page_len,
            w_start_page,
            w_bytes_page_len,
            s_start_page,
            stack_page_len
        ),
    );
    let mut o_pages_bytes = vec![0u8; (PAGE_SIZE * o_bytes_page_len) as usize];
    let mut w_pages_bytes = vec![0u8; (PAGE_SIZE * w_bytes_page_len) as usize];
    let mut stack_bytes = vec![0u8; (PAGE_SIZE * stack_page_len) as usize];
    // get the address of the pages
    if read_machine_page(
        machine_index,
        o_start_page as u32,
        o_bytes_page_len,
        o_pages_bytes.as_mut_ptr() as u64,
    )
    .is_err()
    {
        return Err("copy_memory_to_another_machine: read o_bytes failed");
    }

    if read_machine_page(
        machine_index,
        w_start_page as u32,
        w_bytes_page_len,
        w_pages_bytes.as_mut_ptr() as u64,
    )
    .is_err()
    {
        return Err("copy_memory_to_another_machine: read w_bytes failed");
    }

    if read_machine_page(
        machine_index,
        s_start_page as u32,
        stack_page_len,
        stack_bytes.as_mut_ptr() as u64,
    )
    .is_err()
    {
        return Err("copy_memory_to_another_machine: read stack failed");
    }

    // write the pages to the target machine
    if write_machine_page(
        target_machine_index,
        o_start_page as u32,
        o_bytes_page_len,
        o_pages_bytes.as_ptr() as u64,
        o_bytes_length,
    )
    .is_err()
    {
        return Err("copy_memory_to_another_machine: write o_bytes failed");
    }
    if write_machine_page(
        target_machine_index,
        w_start_page as u32,
        w_bytes_page_len,
        w_pages_bytes.as_ptr() as u64,
        w_bytes_length,
    )
    .is_err()
    {
        return Err("copy_memory_to_another_machine: write w_bytes failed");
    }
    if write_machine_page(
        target_machine_index,
        s_start_page as u32,
        stack_page_len,
        stack_bytes.as_ptr() as u64,
        s,
    )
    .is_err()
    {
        return Err("copy_memory_to_another_machine: write stack failed");
    }
    call_log(
        2,
        None,
        &format!(
            "copy_memory_to_another_machine: success, o_bytes_length={}, w_bytes_length={}, s={}",
            o_bytes_length, w_bytes_length, s
        ),
    );
    Ok(())
}

pub fn copy_image_to_another_machine(
    _source_machine_index: u32,
    _target_machine_index: u32,
    _start_page_id: u32,
    pages_length: u64,
) -> core::result::Result<(), &'static str> {
    if pages_length == 0 {
        return Err("copy_image_to_another_machine: pages_length is zero");
    }

    Ok(())
}
// Child VM related functions
pub fn setup_page(segment: &[u8]) {
    if segment.len() < 8 {
        return call_log(0, None, "setup_page: buffer too small");
    }

    let (m, page_id) = (
        u32::from_le_bytes(segment[0..4].try_into().unwrap()) as u64,
        u32::from_le_bytes(segment[4..8].try_into().unwrap()) as u64,
    );

    let page_address = page_id * PAGE_SIZE;
    let data = &segment[8..];

    if unsafe { pages(m, page_id, 1, 2) } != OK {
        return call_log(0, None, "setup_page: pages failed");
    }

    if unsafe { poke(m, data.as_ptr() as u64, page_address, PAGE_SIZE) } != OK {
        call_log(0, None, "setup_page: poke failed");
    }
}

pub fn get_page(vm_id: u32, page_id: u32) -> [u8; SEGMENT_SIZE as usize] {
    let mut result = [0u8; SEGMENT_SIZE as usize];

    result[0..4].copy_from_slice(&vm_id.to_le_bytes());
    result[4..8].copy_from_slice(&page_id.to_le_bytes());

    let page_address = (page_id as u64) * PAGE_SIZE;
    let result_address = unsafe { result.as_mut_ptr().add(8) };
    let result_length = (SEGMENT_SIZE - 8) as u64;

    let peek_result = unsafe {
        peek(
            vm_id as u64,
            result_address as u64,
            page_address as u64,
            result_length,
        )
    };
    if peek_result != OK {
        call_log(0, None, "get_page: peek failed");
    }
    result
}

pub fn serialize_gas_and_registers(gas: u64, child_vm_registers: &[u64; 13]) -> [u8; 112] {
    let mut result = [0u8; 112];

    result[0..8].copy_from_slice(&gas.to_le_bytes());

    for (i, &reg) in child_vm_registers.iter().enumerate() {
        let start = 8 + i * 8;
        result[start..start + 8].copy_from_slice(&reg.to_le_bytes());
    }
    result
}

pub fn deserialize_gas_and_registers(data: &[u8; 112]) -> (u64, [u64; 13]) {
    let gas = u64::from_le_bytes(data[0..8].try_into().unwrap());

    let mut child_vm_registers = [0u64; 13];
    for i in 0..13 {
        let start = 8 + i * 8;
        child_vm_registers[i] = u64::from_le_bytes(data[start..start + 8].try_into().unwrap());
    }

    (gas, child_vm_registers)
}

pub fn initialize_pvm_registers() -> [u64; 13] {
    let mut registers = [0u64; 13];
    registers[0] = INIT_RA;
    registers[1] = (1u64 << 32) - 2 * Z_Z - Z_I;
    registers[7] = (1u64 << 32) - Z_Z - Z_I;
    registers[8] = 0;
    return registers;
}

// Logging related functions
pub fn write_result(result: u64, key: u8) {
    let key_bytes = key.to_le_bytes();
    let result_bytes = result.to_le_bytes();
    unsafe {
        write(
            key_bytes.as_ptr() as u64,
            key_bytes.len() as u64,
            result_bytes.as_ptr() as u64,
            result_bytes.len() as u64,
        );
    }
    call_log(
        2,
        None,
        &format!("write_result key={:x?}, result={:x?}", key_bytes, result),
    );
}

pub fn call_log(level: u64, target: Option<&str>, msg: &str) {
    let (target_address, target_length) = if let Some(target_str) = target {
        let target_bytes = target_str.as_bytes();
        (target_bytes.as_ptr() as u64, target_bytes.len() as u64)
    } else {
        (0, 0)
    };
    let msg_bytes = msg.as_bytes();
    let msg_address = msg_bytes.as_ptr() as u64;
    let msg_length = msg_bytes.len() as u64;
    unsafe {
        log(
            level,
            target_address,
            target_length,
            msg_address,
            msg_length,
        )
    };
}

// Log level convenience wrappers
#[inline(always)]
pub fn log_crit(message: &str) {
    call_log(0, None, message);
}

#[inline(always)]
pub fn log_error(message: &str) {
    call_log(1, None, message);
}

#[inline(always)]
pub fn log_info(message: &str) {
    call_log(2, None, message);
}

#[inline(always)]
pub fn log_debug(message: &str) {
    call_log(3, None, message);
}

#[inline(always)]
pub fn log_trace(message: &str) {
    call_log(4, None, message);
}

// Helper functions
pub fn extract_discriminator(input: &[u8]) -> u8 {
    if input.is_empty() {
        return 0;
    }

    let first_byte = input[0];
    match first_byte {
        0..=127 => 1,
        128..=191 => 2,
        192..=223 => 3,
        224..=239 => 4,
        240..=247 => 5,
        248..=251 => 6,
        252..=253 => 7,
        254..=u8::MAX => 8,
    }
}

pub fn power_of_two(exp: u32) -> u64 {
    1 << exp
}

pub fn decode_e_l(encoded: &[u8]) -> u64 {
    let mut x: u64 = 0;
    for &byte in encoded.iter().rev() {
        x = x.wrapping_mul(256).wrapping_add(byte as u64);
    }
    x
}

pub fn decode_e(encoded: &[u8]) -> u64 {
    let first_byte = encoded[0];
    if first_byte == 0 {
        return 0;
    }
    if first_byte == 255 {
        return decode_e_l(&encoded[1..9]);
    }
    for l in 0..8 {
        let left_bound = 256 - power_of_two(8 - l);
        let right_bound = 256 - power_of_two(8 - (l + 1));

        if (first_byte as u64) >= left_bound && (first_byte as u64) < right_bound {
            let x1 = (first_byte as u64) - left_bound;
            let x2 = decode_e_l(&encoded[1..(1 + l as usize)]);
            let x = x1 * power_of_two(8 * l) + x2;
            return x;
        }
    }
    0
}

fn ceiling_divide(a: u64, b: u64) -> u64 {
    if b == 0 {
        0
    } else {
        (a + b - 1) / b
    }
}

fn p_func(x: u64) -> u64 {
    Z_P * ceiling_divide(x, Z_P)
}

pub fn z_func(x: u64) -> u64 {
    Z_Z * ceiling_divide(x, Z_Z)
}

// AccumulateOperandElements and related functions

/// Result represents the execution result - either Ok with data or Err with error code
#[derive(Debug, Clone)]
pub struct WorkResult {
    /// Ok contains the result data if execution succeeded
    pub ok: Option<Vec<u8>>,
    /// Err contains the error code if execution failed
    pub err: Option<u8>,
}

/// JAM accumulate operand elements structure
#[derive(Debug, Clone)]
pub struct AccumulateOperandElements {
    /// p = (w_s)_p WorkPackageHash
    pub work_package_hash: [u8; 32],
    /// e = (w_s)_e ExportedSegmentRoot  
    pub exported_segment_root: [u8; 32],
    /// a = w_a AuthorizerHash
    pub authorizer_hash: [u8; 32],
    /// y = r_y PayloadHash
    pub payload_hash: [u8; 32],
    /// g = r_g Gas
    pub gas: u64,
    /// l = r_l Result
    pub result: WorkResult,
    /// t = w_t Trace
    pub trace: Vec<u8>,
}

// deferred transfer
#[derive(Debug, Clone)]
pub struct DeferredTransfer {
    pub sender_index: u32,
    pub receiver_index: u32,
    pub amount: u64,
    pub memo: [u8; 128],
    pub gas_limit: u64,
}

/// Accumulate input enum representing either accumulate operand elements or a deferred transfer
#[derive(Debug, Clone)]
pub enum AccumulateInput {
    /// Standard accumulate operand elements from work package execution
    OperandElements(AccumulateOperandElements),
    /// Deferred transfer between services
    DeferredTransfer(DeferredTransfer),
}

/// Error types for harness operations
#[derive(Debug, Clone)]
pub enum HarnessError {
    TruncatedInput,
    HostFetchFailed,
    HostReadFailed,
    HostWriteFailed,
    HostExportFailed,
    CommitmentMismatch,
    EffectMismatch,
    SegmentTooLarge,
    SharedValueTooLarge,
    ExtrinsicTooLarge,
    VersionOverflow,
    ChecksumMismatch,
    VMExecutionFailed,
    ValidationFailed,
    UnsupportedVersion,
    SegmentNotFound,
    ParseError,
    BufferOverflow,
    InvalidResponse,
}

/// Result type for harness operations
pub type HarnessResult<T> = core::result::Result<T, HarnessError>;

/// Fetches data from the host environment using specified parameters
pub fn fetch_data(
    buffer: &mut [u8],
    datatype: u64,
    index: u64,
    offset: u64,
    max_length: u64,
) -> HarnessResult<Option<Vec<u8>>> {
    unsafe {
        let result_len = fetch(
            buffer.as_mut_ptr() as u64,
            offset,
            max_length,
            datatype,
            index,
            0,
        );

        if result_len == 0 {
            call_log(2, None, &format!("fetch_data: zero"));
            return Ok(None);
        }

        if result_len > max_length {
            call_log(2, None, &format!("fetch_data: FAILED"));
            return Err(HarnessError::HostFetchFailed);
        }

        let result = buffer[..result_len as usize].to_vec();

        Ok(Some(result))
    }
}

/// Fetches an imported segment by index from the host environment
pub fn fetch_imported_segment(work_item_index: u16, index: u32) -> HarnessResult<Vec<u8>> {
    const FETCH_DATATYPE_IMPORTED_SEGMENT: u64 = 5;

    let mut buffer = vec![0u8; SEGMENT_SIZE as usize];

    unsafe {
        let result_len = fetch(
            buffer.as_mut_ptr() as u64,
            0,
            SEGMENT_SIZE,
            FETCH_DATATYPE_IMPORTED_SEGMENT,
            work_item_index as u64,
            index as u64,
        );

        if result_len == 0 {
            return Ok(Vec::new());
        }

        if result_len > SEGMENT_SIZE {
            return Err(HarnessError::HostFetchFailed);
        }

        buffer.truncate(result_len as usize);
        Ok(buffer)
    }
}

/// Fetches an extrinsic by index from the host environment
pub fn fetch_extrinsic(work_item_index: u16, index: u32) -> HarnessResult<Vec<u8>> {
    const FETCH_DATATYPE_EXTRINSIC: u64 = 3;
    let mut buffer = vec![0u8; 256 * 1024];
    match fetch_data(
        &mut buffer,
        FETCH_DATATYPE_EXTRINSIC,
        work_item_index as u64,
        index as u64,
        256 * 1024,
    )? {
        Some(buffer) => Ok(buffer),
        None => Ok(Vec::new()),
    }
}

/// Fetches the current block header from the host environment
pub fn fetch_header() -> Option<BlockHeader> {
    const FETCH_DATATYPE_HEADER: u64 = 16;
    // Allocate a very large buffer size for the encoded header to handle any JAM header
    // JAM headers can be very large due to complex types (tickets, signatures, epoch marks, etc.)
    let mut buffer = vec![0u8; 32768];
    match fetch_data(
        &mut buffer,
        FETCH_DATATYPE_HEADER,
        0,
        0,
        32768,
    ) {
        Ok(Some(data)) => {
            log_info(&format!("✅ fetch_header: Got {} bytes of header data", data.len()));
            // Attempt to deserialize the header from the fetched data
            match BlockHeader::deserialize(&data) {
                Some(header) => {
                    log_info(&format!("✅ fetch_header: Successfully deserialized header"));
                    Some(header)
                },
                None => {
                    log_error(&format!("❌ fetch_header: Failed to deserialize {} bytes of header data", data.len()));
                    None
                }
            }
        },
        Ok(None) => {
            log_info(&format!("⚠️ fetch_header: No header available from host"));
            None
        },
        Err(e) => {
            log_error(&format!("❌ fetch_header: Error occurred during fetch: {:?}", e));
            None
        }
    }
}

/// Fetches extrinsics by index from the host environment
pub fn fetch_extrinsics(work_item_index: u16) -> HarnessResult<Vec<Vec<u8>>> {
    const FETCH_DATATYPE_EXTRINSICS: u64 = 4;

    // Get a slice from the static buffer safely
    let buffer_slice = unsafe {
        let ptr = core::ptr::addr_of_mut!(TEST_BUFFER);
        core::slice::from_raw_parts_mut((*ptr).as_mut_ptr(), (*ptr).len())
    };

    match fetch_data(
        buffer_slice,
        FETCH_DATATYPE_EXTRINSICS,
        work_item_index as u64,
        0,
        256 * 1024,
    )? {
        Some(_buffer) => {
            let mut cursor = 0;
            let buffer_len = 256 * 1024;

            // Get number of extrinsics using variable-length encoding
            if cursor >= buffer_len {
                return Err(HarnessError::TruncatedInput);
            }
            let count_full_slice = &buffer_slice[cursor..];
            let count_len_len = extract_discriminator(count_full_slice) as u64;

            if count_len_len == 0 || buffer_len - cursor < count_len_len as usize {
                return Err(HarnessError::TruncatedInput);
            }
            let count_slice = &count_full_slice[..count_len_len as usize];

            let extrinsics_count = decode_e(count_slice) as usize;

            cursor += count_len_len as usize;

            let mut extrinsics = Vec::with_capacity(extrinsics_count);

            // For each extrinsic, extract length first using variable-length encoding, then extract that many bytes
            for _ in 0..extrinsics_count {
                if cursor >= buffer_len {
                    return Err(HarnessError::TruncatedInput);
                }

                // Get extrinsic length using variable-length encoding
                let len_full_slice = &buffer_slice[cursor..];
                let len_len_len = extract_discriminator(len_full_slice) as u64;

                if len_len_len == 0 || buffer_len - cursor < len_len_len as usize {
                    return Err(HarnessError::TruncatedInput);
                }
                let len_slice = &len_full_slice[..len_len_len as usize];
                let extrinsic_len = decode_e(len_slice) as usize;
                cursor += len_len_len as usize;

                // Extract the extrinsic bytes
                if cursor + extrinsic_len > buffer_len {
                    return Err(HarnessError::TruncatedInput);
                }
                extrinsics.push(buffer_slice[cursor..cursor + extrinsic_len].to_vec());
                cursor += extrinsic_len;
            }
            Ok(extrinsics)
        }
        None => Ok(Vec::new()),
    }
}

/// Fetches the work item payload from the host environment
pub fn fetch_payload(index: u16) -> HarnessResult<Option<Vec<u8>>> {
    call_log(2, None, &format!("fetch_payload 1: {}", index));
    pub const FETCH_DATATYPE_WORKITEM_PAYLOAD: u64 = 13;
    call_log(2, None, &format!("fetch_payload 2: {}", index));
    let mut buffer = vec![0u8; 256 * 1024];
    fetch_data(
        &mut buffer,
        FETCH_DATATYPE_WORKITEM_PAYLOAD,
        index as u64,
        0,
        256 * 1024,
    )
}

/// Import segment reference for work items
#[derive(Debug, Clone)]
pub struct RefineContext {
    pub anchor: [u8; 32],
    pub state_root: [u8; 32],
    pub beefy_root: [u8; 32],
    pub lookup_anchor: [u8; 32],
    pub lookup_anchor_slot: u32,
    pub prerequisites: Vec<[u8; 32]>,
}

/// Import segment reference for work items
#[derive(Debug, Clone)]
pub struct ImportSegment {
    pub work_package_hash: [u8; 32],
    pub index: u16,
}

/// Individual work item
#[derive(Debug, Clone)]
pub struct WorkItem {
    pub service: u32,
    pub code_hash: Vec<u8>,
    pub refine_gas_limit: u64,
    pub accumulate_gas_limit: u64,
    pub export_count: u16,
    pub payload: Vec<u8>,
    pub imported_segments: Vec<ImportSegment>,
    pub work_item_extrinsics: Vec<WorkItemExtrinsic>,
}

impl WorkItem {
    /// Returns the range (start_index, end_index) within imported_segments that match
    /// the work_package_hash and index range from the given ObjectRef.
    /// Returns None if no matching segments are found.
    pub fn get_imported_segments_range(&self, object_ref: &crate::objects::ObjectRef) -> Option<(usize, usize)> {
        let target_hash = &object_ref.work_package_hash;
        let index_start = object_ref.index_start;
        let index_end = object_ref.index_end;

        log_debug(&format!(
            "get_imported_segments_range: looking for work_package_hash={:?}, index_start={}, index_end={}, total imported_segments={}",
            target_hash,
            index_start,
            index_end,
            self.imported_segments.len()
        ));

        let mut start_pos: Option<usize> = None;
        let mut end_pos: Option<usize> = None;

        for (i, segment) in self.imported_segments.iter().enumerate() {
            if segment.work_package_hash == *target_hash {
                log_debug(&format!(
                    "get_imported_segments_range: segment[{}] matches hash, segment.index={}",
                    i, segment.index
                ));
                if segment.index >= index_start && segment.index < index_end {
                    if start_pos.is_none() {
                        start_pos = Some(i);
                        log_debug(&format!(
                            "get_imported_segments_range: set start_pos={}",
                            i
                        ));
                    }
                    end_pos = Some(i + 1);
                    log_debug(&format!(
                        "get_imported_segments_range: updated end_pos={}",
                        i + 1
                    ));
                }
            }
        }

        match (start_pos, end_pos) {
            (Some(start), Some(end)) => {
                log_debug(&format!(
                    "get_imported_segments_range: success, returning range ({}, {})",
                    start, end
                ));
                Some((start, end))
            }
            _ => {
                log_error(&format!(
                    "get_imported_segments_range: no matching segments found for work_package_hash={:?}, index_start={}, index_end={}",
                    target_hash, index_start, index_end
                ));
                None
            }
        }
    }
}

#[derive(Debug, Clone)]
pub struct WorkItemExtrinsic {
    pub hash: [u8; 32],
    pub len: u32,
}

static mut BUFFER: [u8; 4104] = [0u8; 4104];
static mut TEST_BUFFER: [u8; 4096 * 16] = [0u8; 4096 * 16];

/// Fetches and constructs a WorkItem from the JAM runtime
pub fn fetch_work_item(wi_index: u16) -> Option<WorkItem> {
    pub const FETCH_DATATYPE_WORK_ITEM: u64 = 12;

    // Try to fetch a structured work item blob from the host.
    let buffer_slice = unsafe {
        let ptr = core::ptr::addr_of_mut!(BUFFER);
        core::slice::from_raw_parts_mut((*ptr).as_mut_ptr(), (*ptr).len())
    };

    let data = match fetch_data(
        buffer_slice,
        FETCH_DATATYPE_WORK_ITEM,
        wi_index as u64,
        0,
        4104,
    ) {
        Ok(Some(d)) => d,
        Ok(None) => {
            call_log(1, None, "fetch_work_item: no data returned from host");
            return None;
        }
        Err(_) => {
            call_log(1, None, "fetch_work_item: host fetch failed");
            return None;
        }
    };

    // Parsing layout (little-endian):
    // [0..4)   service: u32
    // [8..8+L) code_hash bytes
    // next 8   refine_gas_limit: u64
    // next 8   accumulate_gas_limit: u64
    // next 2   export_count: u16
    // payload_len (variable-length encoded using discriminator + decode_e)
    // next P   payload bytes (if present)
    // imported_segments_count (variable-length)
    // repeats  imported_segment: [32 bytes work_package_hash] + [2 bytes index]
    // extrinsics_count (variable-length)
    // repeats  extrinsic: [32 bytes hash] + [4 bytes len]
    let mut cursor = 0usize;
    let need = |c: usize, n: usize, label: &str| -> bool {
        if data.len().saturating_sub(c) < n {
            call_log(
                1,
                None,
                &format!("fetch_work_item: truncated input at {}", label),
            );
            return false;
        }
        true
    };

    if !need(cursor, 4, "service") {
        return None;
    }
    let service = u32::from_le_bytes(data[cursor..cursor + 4].try_into().unwrap());
    cursor += 4;

    let ch_len = 32;

    if !need(cursor, ch_len, "code_hash") {
        return None;
    }
    let code_hash = data[cursor..cursor + ch_len].to_vec();
    cursor += ch_len;

    if !need(cursor, 8, "refine_gas_limit") {
        return None;
    }
    let refine_gas_limit = u64::from_le_bytes(data[cursor..cursor + 8].try_into().unwrap());
    cursor += 8;

    if !need(cursor, 8, "accumulate_gas_limit") {
        return None;
    }
    let accumulate_gas_limit = u64::from_le_bytes(data[cursor..cursor + 8].try_into().unwrap());
    cursor += 8;

    if !need(cursor, 2, "export_count") {
        return None;
    }
    let export_count = u16::from_le_bytes(data[cursor..cursor + 2].try_into().unwrap());
    cursor += 2;

    // payload_len (variable-length encoded using discriminator + decode_e)
    let p_full_slice = &data[cursor..];
    let p_len_len = extract_discriminator(p_full_slice) as u64;

    if p_len_len == 0 || data.len() - cursor < p_len_len as usize {
        return None;
    }
    let p_slice = &p_full_slice[..p_len_len as usize];
    let payload_len = decode_e(p_slice) as usize;

    cursor += p_len_len as usize;

    let payload = if data.len() - cursor >= payload_len {
        let p = data[cursor..cursor + payload_len].to_vec();
        cursor += payload_len;
        p
    } else {
        Vec::new()
    };

    // Imported segments count (variable-length)
    let iseg_full_slice = &data[cursor..];
    let iseg_len = extract_discriminator(iseg_full_slice) as u64;
    if iseg_len == 0 || data.len() - cursor < iseg_len as usize {
        return None;
    }
    let iseg_slice = &iseg_full_slice[..iseg_len as usize];
    let imported_count = decode_e(iseg_slice) as usize;
    cursor += iseg_len as usize;

    let mut imported_segments = Vec::with_capacity(imported_count);
    for i in 0..imported_count {
        if !need(cursor, 34, &format!("imported_segment[{}]", i)) {
            return None;
        }
        let mut wph = [0u8; 32];
        wph.copy_from_slice(&data[cursor..cursor + 32]);
        cursor += 32;
        let index = u16::from_le_bytes(data[cursor..cursor + 2].try_into().unwrap());
        cursor += 2;
        imported_segments.push(ImportSegment {
            work_package_hash: wph,
            index,
        });
    }

    // Extrinsics count (variable-length)
    let ext_full_slice = &data[cursor..];
    let ext_len = extract_discriminator(ext_full_slice) as u64;
    if ext_len == 0 || data.len() - cursor < ext_len as usize {
        return None;
    }
    let ext_slice = &ext_full_slice[..ext_len as usize];
    let extrinsics_count = decode_e(ext_slice) as usize;
    cursor += ext_len as usize;

    let mut work_item_extrinsics = Vec::with_capacity(extrinsics_count);
    for i in 0..extrinsics_count {
        if !need(cursor, 36, &format!("extrinsic[{}]", i)) {
            return None;
        }
        let mut h = [0u8; 32];
        h.copy_from_slice(&data[cursor..cursor + 32]);
        cursor += 32;
        let len = u32::from_le_bytes(data[cursor..cursor + 4].try_into().unwrap());
        cursor += 4;
        work_item_extrinsics.push(WorkItemExtrinsic { hash: h, len });
    }

    Some(WorkItem {
        service,
        code_hash,
        refine_gas_limit,
        accumulate_gas_limit,
        export_count,
        payload,
        imported_segments,
        work_item_extrinsics,
    })
}

pub fn fetch_refine_context() -> Option<RefineContext> {
    pub const FETCH_DATATYPE_REFINE_CONTEXT: u64 = 10;
    let mut buffer = vec![0u8; 256 * 1024];
    let data = match fetch_data(&mut buffer, FETCH_DATATYPE_REFINE_CONTEXT, 0, 0, 256 * 1024) {
        Ok(Some(buffer)) => buffer,
        Ok(None) | Err(_) => {
            call_log(1, None, "fetch_refine_context: host fetch failed");
            return None;
        }
    };

    let need_min = 32 + 32 + 32 + 32 + 4 + 4; // fixed portion
    if data.len() < need_min {
        call_log(
            1,
            None,
            "fetch_refine_context: truncated input (fixed header)",
        );
        return None;
    }

    let mut cursor = 0usize;

    // anchor
    let mut anchor = [0u8; 32];
    anchor.copy_from_slice(&data[cursor..cursor + 32]);
    cursor += 32;

    // state_root
    let mut state_root = [0u8; 32];
    state_root.copy_from_slice(&data[cursor..cursor + 32]);
    cursor += 32;

    // beefy_root
    let mut beefy_root = [0u8; 32];
    beefy_root.copy_from_slice(&data[cursor..cursor + 32]);
    cursor += 32;

    // lookup_anchor
    let mut lookup_anchor = [0u8; 32];
    lookup_anchor.copy_from_slice(&data[cursor..cursor + 32]);
    cursor += 32;

    // lookup_anchor_slot (u32, LE)
    let slot_bytes: [u8; 4] = match data.get(cursor..cursor + 4) {
        Some(s) => s.try_into().unwrap(),
        None => {
            call_log(
                1,
                None,
                "fetch_refine_context: truncated input (lookup_anchor_slot)",
            );
            return None;
        }
    };
    let lookup_anchor_slot = u32::from_le_bytes(slot_bytes);
    cursor += 4;

    // prereq_count (u32, LE)
    let count_bytes: [u8; 4] = match data.get(cursor..cursor + 4) {
        Some(s) => s.try_into().unwrap(),
        None => {
            call_log(
                1,
                None,
                "fetch_refine_context: truncated input (prereq_count)",
            );
            return None;
        }
    };
    let prereq_count = u32::from_le_bytes(count_bytes) as usize;
    cursor += 4;

    // prerequisites: prereq_count * 32 bytes
    let need_total = need_min + prereq_count * 32;
    if data.len() < need_total {
        call_log(
            1,
            None,
            "fetch_refine_context: truncated input (prerequisites)",
        );
        return None;
    }
    let mut prerequisites = Vec::with_capacity(prereq_count);
    for _ in 0..prereq_count {
        let mut h = [0u8; 32];
        h.copy_from_slice(&data[cursor..cursor + 32]);
        cursor += 32;
        prerequisites.push(h);
    }

    Some(RefineContext {
        anchor,
        state_root,
        beefy_root,
        lookup_anchor,
        lookup_anchor_slot,
        prerequisites,
    })
}

/// Parses accumulate operand element from raw bytes
pub fn parse_accumulate_operand_element(data: &[u8]) -> Option<AccumulateOperandElements> {
    if data.len() < 128 {
        // 4 * 32 bytes + 8 bytes for gas + 1 byte for result type
        return None;
    }

    let mut cursor = 0;

    // Parse work_package_hash (32 bytes)
    let mut work_package_hash = [0u8; 32];
    work_package_hash.copy_from_slice(&data[cursor..cursor + 32]);
    cursor += 32;

    // Parse exported_segment_root (32 bytes)
    let mut exported_segment_root = [0u8; 32];
    exported_segment_root.copy_from_slice(&data[cursor..cursor + 32]);
    cursor += 32;

    // Parse authorizer_hash (32 bytes)
    let mut authorizer_hash = [0u8; 32];
    authorizer_hash.copy_from_slice(&data[cursor..cursor + 32]);
    cursor += 32;

    // Parse payload_hash (32 bytes)
    let mut payload_hash = [0u8; 32];
    payload_hash.copy_from_slice(&data[cursor..cursor + 32]);
    cursor += 32;

    // Parse gas
    let gas_slice = &data[cursor..];
    let discriminator_len = extract_discriminator(gas_slice) as usize;
    let gas = if discriminator_len > 0 {
        decode_e(&gas_slice[..discriminator_len]) as u64
    } else {
        0
    };
    cursor += discriminator_len;

    // Parse result (Result struct)
    // Format: 1 byte type flag (0 = Ok, 1 = Err) + variable data
    let result_type = data[cursor];
    cursor += 1;

    let result = if result_type == 0 {
        // Ok result - use discriminator to get length, then data
        let ok_slice = &data[cursor..];
        let discriminator_len = extract_discriminator(ok_slice) as usize;
        let ok_len = if discriminator_len > 0 {
            decode_e(&ok_slice[..discriminator_len]) as usize
        } else {
            0
        };
        cursor += discriminator_len;

        if cursor + ok_len > data.len() {
            call_log(1, None, &format!("parse_accumulate_operand_element: insufficient data for ok_data (need {}, have {})", ok_len, data.len() - cursor));
            return None;
        }
        let ok_data = data[cursor..cursor + ok_len].to_vec();
        cursor += ok_len;

        WorkResult {
            ok: Some(ok_data),
            err: None,
        }
    } else {
        // Err result - next byte is error code
        if cursor + 1 > data.len() {
            call_log(
                1,
                None,
                "parse_accumulate_operand_element: insufficient data for err_code",
            );
            return None;
        }
        let err_code = data[cursor];
        cursor += 1;
        call_log(
            2,
            None,
            &format!(
                "parse_accumulate_operand_element: Err result, err_code={}",
                err_code
            ),
        );

        WorkResult {
            ok: None,
            err: Some(err_code),
        }
    };

    // Parse trace (remaining bytes)
    let trace = data[cursor..].to_vec();

    Some(AccumulateOperandElements {
        work_package_hash,
        exported_segment_root,
        authorizer_hash,
        payload_hash,
        gas,
        result,
        trace,
    })
}

/// Parses deferred transfer from raw bytes
pub fn parse_deferred_transfer(data: &[u8]) -> Option<DeferredTransfer> {
    // Expected size: 4 + 4 + 8 + 128 + 8 = 152 bytes
    if data.len() < 152 {
        return None;
    }

    let mut cursor = 0;

    // Parse sender_index (4 bytes, little-endian u32)
    let sender_index = u32::from_le_bytes(data[cursor..cursor + 4].try_into().ok()?);
    cursor += 4;

    // Parse receiver_index (4 bytes, little-endian u32)
    let receiver_index = u32::from_le_bytes(data[cursor..cursor + 4].try_into().ok()?);
    cursor += 4;

    // Parse amount (8 bytes, little-endian u64)
    let amount = u64::from_le_bytes(data[cursor..cursor + 8].try_into().ok()?);
    cursor += 8;

    // Parse memo (128 bytes)
    let mut memo = [0u8; 128];
    memo.copy_from_slice(&data[cursor..cursor + 128]);
    cursor += 128;

    // Parse gas_limit (8 bytes, little-endian u64)
    let gas_limit = u64::from_le_bytes(data[cursor..cursor + 8].try_into().ok()?);

    Some(DeferredTransfer {
        sender_index,
        receiver_index,
        amount,
        memo,
        gas_limit,
    })
}

/// Fetches an accumulate input by index from the host environment
pub fn fetch_accumulate_input(index: u32) -> HarnessResult<AccumulateInput> {
    const FETCH_DATATYPE_ACCUMULATE_INPUT: u64 = 15;

    let buffer_slice = unsafe {
        let ptr = core::ptr::addr_of_mut!(TEST_BUFFER);
        core::slice::from_raw_parts_mut((*ptr).as_mut_ptr(), (*ptr).len())
    };

    let data = match fetch_data(
        buffer_slice,
        FETCH_DATATYPE_ACCUMULATE_INPUT,
        index as u64,
        0,
        buffer_slice.len() as u64,
    )? {
        Some(buffer) => buffer,
        None => return Err(HarnessError::HostFetchFailed),
    };

    if data.is_empty() {
        return Err(HarnessError::TruncatedInput);
    }

    // First byte indicates type: 0 = AccumulateOperandElements, 1 = DeferredTransfer
    let input_type = data[0];
    let remaining_data = &data[1..];

    match input_type {
        0 => {
            // Parse as AccumulateOperandElements
            match parse_accumulate_operand_element(remaining_data) {
                Some(operand) => Ok(AccumulateInput::OperandElements(operand)),
                None => {
                    call_log(
                        1,
                        None,
                        "fetch_accumulate_input: failed to parse AccumulateOperandElements",
                    );
                    Err(HarnessError::ParseError)
                }
            }
        }
        1 => {
            // Parse as DeferredTransfer
            call_log(1, None, "fetch_accumulate_input: parsing DeferredTransfer");
            match parse_deferred_transfer(remaining_data) {
                Some(transfer) => {
                    call_log(
                        3,
                        None,
                        "fetch_accumulate_input: successfully parsed DeferredTransfer",
                    );
                    Ok(AccumulateInput::DeferredTransfer(transfer))
                }
                None => {
                    call_log(
                        1,
                        None,
                        "fetch_accumulate_input: failed to parse DeferredTransfer",
                    );
                    Err(HarnessError::ParseError)
                }
            }
        }
        _ => {
            call_log(
                1,
                None,
                &format!("accumulate: invalid input_type={}", input_type),
            );
            Err(HarnessError::ParseError)
        }
    }
}

/// Fetch all accumulate inputs for the given number of operands
pub fn fetch_accumulate_inputs(num_inputs: u64) -> HarnessResult<Vec<AccumulateInput>> {
    // Fetch all accumulate inputs
    let mut accumulate_inputs = Vec::new();
    for i in 0..num_inputs {
        match fetch_accumulate_input(i as u32) {
            Ok(input) => {
                accumulate_inputs.push(input);
            }
            Err(e) => {
                return Err(e);
            }
        }
    }
    Ok(accumulate_inputs)
}

/// Fetches entropy from JAM host using datatype 1
pub fn fetch_consensus_seed() -> HarnessResult<[u8; 32]> {
    const FETCH_DATATYPE_ENTROPY: u64 = 1;
    // Request exactly 32 bytes of entropy from the host.
    let mut buffer = vec![0u8; 32];
    let data = match fetch_data(&mut buffer, FETCH_DATATYPE_ENTROPY, 0, 0, 32)? {
        Some(buffer) => buffer,
        None => {
            call_log(1, None, "fetch_consensus_seed: host returned no data");
            return Err(HarnessError::HostFetchFailed);
        }
    };

    if data.len() != 32 {
        return Err(HarnessError::TruncatedInput);
    }

    let mut seed = [0u8; 32];
    seed.copy_from_slice(&data[..32]);
    Ok(seed)
}

/// Generates deterministic ObjectIds for new objects using consensus seed
pub fn generate_consensus_object_id(seed: &[u8; 32], object_index: u64) -> ObjectId {
    // Combine consensus seed with object index for deterministic ID generation
    let mut hash_input = Vec::new();
    hash_input.extend_from_slice(seed);
    hash_input.extend_from_slice(&object_index.to_le_bytes());

    // Simple hash-based ID generation (in production, use proper cryptographic hash)
    let mut id_bytes = [0u8; 32];
    for (i, chunk) in hash_input.chunks(32).enumerate() {
        for (j, &byte) in chunk.iter().enumerate() {
            id_bytes[j] ^= byte.wrapping_add(i as u8);
        }
    }

    id_bytes
}

/// Format ObjectId for logging
pub fn format_object_id(object_id: ObjectId) -> String {
    // Format all 32 bytes of the ObjectId as hex
    let mut result = String::from("0x");
    for byte in object_id.iter() {
        result.push_str(&format!("{:02x}", byte));
    }
    result
}

/// Format segment data (first 256 bytes) for logging
pub fn format_segment(data: &[u8]) -> String {
    let len = core::cmp::min(data.len(), 256);
    let mut result = String::from("0x");
    for byte in data[..len].iter() {
        result.push_str(&format!("{:02x}", byte));
    }
    result
}
