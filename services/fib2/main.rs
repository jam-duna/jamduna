#![no_std]
#![no_main]

extern crate alloc;

use simplealloc::SimpleAlloc;

#[global_allocator]
static ALLOCATOR: SimpleAlloc<4096> = SimpleAlloc::new();

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe {
        core::arch::asm!("unimp", options(noreturn));
    }
}

#[polkavm_derive::polkavm_import]
extern "C" {
    #[polkavm_import(index = 0)]
    pub fn gas() -> u64;
    // accumulate
    #[polkavm_import(index = 1)]
    pub fn lookup(s: u64, h: u64, o: u64, f: u64, l: u64) -> u64;
    #[polkavm_import(index = 2)]
    pub fn read(s: u64, ko: u64, kz: u64, o: u64, f: u64, l: u64) -> u64;
    #[polkavm_import(index = 3)]
    pub fn write(ko: u64, kz: u64, bo: u64, bz: u64) -> u64;
    #[polkavm_import(index = 4)]
    pub fn info(s: u64, o: u64) -> u64;
    #[polkavm_import(index = 5)]
    pub fn bless(m: u64, a: u64, v: u64, o: u64, n: u64) -> u64;
    #[polkavm_import(index = 6)]
    pub fn assign(c: u64, o: u64) -> u64;

    #[polkavm_import(index = 9)]
    pub fn new(o: u64, l: u64, g: u64, m: u64) -> u64;
    #[polkavm_import(index = 10)]
    pub fn upgrade(o: u64, g: u64, m: u64) -> u64;

    #[polkavm_import(index = 12)]
    pub fn eject(d: u64, o: u64) -> u64;
    #[polkavm_import(index = 13)]
    pub fn query(o: u64, z: u64) -> u64;
    #[polkavm_import(index = 14)]
    pub fn solicit(o: u64, z: u64) -> u64;
    #[polkavm_import(index = 15)]
    pub fn forget(o: u64, z: u64) -> u64;
    #[polkavm_import(index = 16)]
    pub fn oyield(o: u64) -> u64;

    // refine
    #[polkavm_import(index = 17)]
    pub fn historical_lookup(service: u64, h: u64, o: u64) -> u64;
    #[polkavm_import(index = 18)]
    pub fn fetch(start_address: u64, offset: u64, maxlen: u64, omega_10: u64, omega_11: u64, omega_12: u64) -> u64;
    #[polkavm_import(index = 19)]
    pub fn export(out: u64, out_len: u64) -> u64;
    #[polkavm_import(index = 20)]
    pub fn machine(po: u64, pz: u64, i: u64) -> u64;
    #[polkavm_import(index = 21)]
    pub fn peek(n: u64, o: u64, s: u64, z: u64) -> u64;
    #[polkavm_import(index = 22)]
    pub fn poke(n: u64, s: u64, o: u64, z: u64) -> u64;
    #[polkavm_import(index = 23)]
    pub fn zero(n: u64, p: u64, c: u64) -> u64;
    #[polkavm_import(index = 24)]
    pub fn void(n: u64, p: u64, c: u64) -> u64;
    #[polkavm_import(index = 25)]
    pub fn invoke(n: u64, o: u64) -> u64;
    #[polkavm_import(index = 25)]
    pub fn expunge(n: u64) -> u64;

    #[polkavm_import(index = 100)]
    pub fn log(level: u64, target: u64, target_len: u64, message: u64, message_len: u64) -> u64; //https://hackmd.io/@polkadot/jip1
}

pub const NONE: u64 = u64::MAX;
pub const PAGE_START: u64 = 4096;
pub const PAGE_SIZE: u64 = 4096;
pub const W_G: u64 = 4104;


fn setup_page(buffer: [u8; 4104]) {
    let m = buffer[0..4].try_into().unwrap();
    let pagenumber = buffer[4..8].try_into().unwrap();
    if m == 0 {
        memcpy(buffer[8:]);  /// TODO: implement this
    } else {
        poke(m, buffer[8:], pagenumber, 4096);
    }
    info("setup_page");
}

pub fn info(message: string) {
    log(2, 0, 0, message.as_ptr() as u64, message.len() as u64);
}

#[polkavm_derive::polkavm_export]
extern "C" fn refine() -> u64 {
    let mut buffer = [0u8; 4104];
    // TODO: extract from "a" https://graypaper.fluffylabs.dev/#/5f542d7/2dee002dee00
    // - service index
    // - payload y
    //   this will be 8 byte packets like
    //   [child payload 1] [child payload 2] ... [parent payload]
    //   8 byte packet:  [N 4 bytes] [F 4 bytes]
    // - extrinsics x
    let mut numextrinsics: u64 = 0; // TODO: get this from "a" 
    let mut child_machine: u64 = [0u64; 16]; // support up to 16 child machines
    let mut num_child_machines: u64 = 0;

    // (1) for any children in the payload y, create child VM with MACHINE using payload -- need "deblob"
    let num_payloaditems = payload.len() / 8;
    let mut i = 0;
    for (i=0; i<num_payloaditems-1; i++) { // the first items are the children, the last is the parent
        let child_payload = payload[i*8..(i+1)*8];
        let child_N = child_payload[0:4];  // work for the child
        let child_F = child_payload[4:8]; // what the work is (fib (1), trib (2), factorial (3), ...)
        let child_m = unsafe {
            let po = 0u64;  // TODO
            let pz = 0u64;  // TODO
            let j = 0u64;  // TODO
            machine(
                po, 
                pz, 
                j
            )
        };
        if child_m < 16 {
            child_machine[num_child_machines] = child_m;
            num_child_machines += 1;
            info("machine");  
        } else {
            info("machine HUH");  
        }
    }
    let mut offset: u64 = 0;
    let i = num_payloaditems-1
    let parent_payload = payload[i*12..(i+1)*12];
    let parent_m = parent_payload[0:4];
    let parent_N = parent_payload[4:8];   
    let parent_F = parent_payload[8:12];  // sum, prod 

    // (1) for 4104 byte segments ( [ 4 byte child machine (0=parent) ] [ 4 byte page number ] [ 4096 byte of data ] )
    // (1a) FETCH all the segments we can  and setup the page (with poke) to write the page
    let offset = 0; 
    do { 
        let result = unsafe {
            fetch(
                buffer.as_mut_ptr() as u64,
                offset,
                W_G,
                5,
                0,
                0,
            )
        };
        info("fetch");

        setup_page(buffer);
        offset += 1;       
    } while result != NONE;

    // (1b) HISTORICAL_LOOKUP of all preimages from provided extrinsics and do a setup_page
    for ( offset = 0; offset < numextrinsics; offset ++ ) {
        let m = buffer[0..4].try_into().unwrap();
        let pagenumber = buffer[4..8].try_into().unwrap();
        let result = unsafe {
            historical_lookup(service, buffer.as_mut_ptr() as u64, buffer.len());
        };
        info("historical lookup");
        setup_page(buffer);
    }

    let buffer_addr = buffer.as_ptr() as u64;
    let buffer_len = buffer.len() as u64;

    for (i=0; i<num_child_machines; i++) {
        let child_m = child_machine[i];
        let child_N = child_payload[0:4];
        let child_F = child_payload[4:8];

        // (3a) do a VOID (or ZERO) of page 4096 (this probably doesn't do anything but is a good practice)
        let result = unsafe {
            let p = 4096u64; // TODO
            let c = 0u64; // TODO
            void(child_m, p, c);
        };
        info("voided memory");

        // (3b) POKE the value of F (some number like F=Fib, Trib, ...) + N in page 4096  using the payload -- F can differ but basically result of 2 or 3 previous pages
        let poke_result = unsafe {
            let n = 0u64; // TODO
            let s = 0u64; // TODO
            let o = 0u64; // TODO
            let z = 0u64; // TODO
            poke(n, s, o, z);
        }
        info("poke child");

        // ******  TODO: SEPARATE THIS OUT INTO A FUNCTION WITHIN INVOKE ******
        // (4) INVOKE child code which will  add a NEW page with  F(N) 
        let invoke_result = unsafe {
            let n = 0u64; // TODO
            let s = 0u64; // TODO
            let o = 0u64; // TODO
            let z = 0u64; // TODO
            invoke(n, s, o, z);
        }
        // the result of invoking the code basically do this:
        if child_F == 1 {
            // FIB
            // get result_prev_1 and result_prev_2
            let result_prev_1 = 1u64; // TODO: read from prev page
            let result_prev_2 = 1u64; // TODO: read from prevx2 page
            let result = result_prev_1 + result_prev_2;
            // put result into the new page
        } else if child_F == 2 {
            // Trib
            // get result_prev_1 and result_prev_2
            let result_prev_1 = 1u64; // TODO: read from prev page
            let result_prev_2 = 1u64; // TODO: read from prev page
            let result_prev_3 = 1u64; // TODO: read from prev page
            let result = result_prev_1 + result_prev_2 + result_prev_3;
            // put result into the new page
        } else if child_F == 3 {
            // Factorial
            let result = child_N * result_prev_1;
            // put result into the new page
        }
        info("invoked child");

        // (5) for all N pages of EACH child, do a PEEK to read the page out and then EXPORT N+1 pages; Although all pages are written exactly once, we will write all N pages.  
        for (page = 0; page<child_N; page++) {
            let s = PAGE_START + PAGE_SIZE * page; // TODO
            let result = unsafe {
                peek(child_m, buffer_addr, s, W_G);
            };
            info("peek page");
    
            let result = unsafe {
                export(buffer_addr, W_G);
            }
            info("exported page");
        }

        // (5) PEEK in the child VM in new page for F(N) and output [ N, F(N), F(N-1), F(N-2) ] (16 bytes) as before, unless your parent says to combine the result of the child eg FT(N) = Fib(N)+Trib(N)
        let peek_result = unsafe {
            let s = PAGE_START + PAGE_SIZE * N; // TODO
            let o = buffer_addr; 
            let z = 4u64; 
            peek(child_m, s, o, z);
        }
        child_result[child_M] = result;
        info("peeked N and got invoked result");

        // expunge the child machine
        let result = unsafe {
            expunge(child_m);
        };
        info("expunge"); 
    }

    // aggregate the results
    for (i=0; i < num_child_machines; i++) {
        let mut parent_result = 0u64; // TODO
        if parent_F == 16 {
            // SUM 
            if i == 0 {
                parent_result = child_result[i];
            } else {
                parent_result += child_result[i];   
            }            
        } else if parent_F == 17 {
            // PROD 
            if i == 0 {
                parent_result = child_result[i];
            } else {
                parent_result *= child_result[i];   
            }            
        }
    }

    // the output should be the same 12 byte output of before
    info("parent_result"); 
    unsafe {
        core::arch::asm!(
            "mv a1, {0}",
            in(reg) buffer_len,
        );
    }
    // this equals to a0 = buffer_addr
    buffer_addr
}

#[polkavm_derive::polkavm_export]
extern "C" fn accumulate() -> u64 {
    // read the input start address and length from register a0 and a1
    let omega_7: u64; // accumulate input start address
    let omega_8: u64; // accumulate input length

    unsafe {
        core::arch::asm!(
            "mv {0}, a0",
            "mv {1}, a1",
            out(reg) omega_7,
            out(reg) omega_8,
        );
    }
    // fetch service index
    let service_index_address = omega_7 + 4; // skip 4 bytes time slot
    let SERVICE_INDEX: u64   = unsafe { ( *(service_index_address as *const u32)).into() }; // 4 bytes service index

    // fetch all_accumulation_o
    let mut start_address = omega_7 + 4 + 4; // 4 bytes time slot + 4 bytes service index
    let mut remaining_length = omega_8 - 4 - 4; // 4 bytes time slot + 4 bytes service index
    let all_accumulation_o = unsafe { core::slice::from_raw_parts(start_address as *const u8, remaining_length as usize) };

    // fetch the number of accumulation_o
    let all_accumulation_o_discriminator_length = extract_discriminator(all_accumulation_o);
    let num_of_accumulation_o = decode_e(&all_accumulation_o[..all_accumulation_o_discriminator_length as usize]);

    // update the address pointer and remaining length
    start_address += all_accumulation_o_discriminator_length as u64;
    remaining_length -= all_accumulation_o_discriminator_length as u64;

    // set variables for storing work result address and length
    let mut work_result_address: u64 = 0;
    let mut work_result_length: u64 = 0;

    // set variables for storing auth output address and length
    let mut auth_output_address: u64 = 0;
    let mut auth_output_length: u64 = 0;

    for n in 0.. num_of_accumulation_o {
        // we only use the 0th accumulation_o
        if n > 0 {
            break;
        }
        // fetch work result prefix
        let accumulation_o = unsafe { core::slice::from_raw_parts(start_address as *const u8, remaining_length as usize) };
        let work_result_prefix = &accumulation_o[..1];

        start_address += 1;
        remaining_length -= 1;

        // fetch work result
        if work_result_prefix[0] == 0 {
            let accumulation_o = unsafe { core::slice::from_raw_parts(start_address as *const u8, remaining_length as usize) };
            let work_result_discriminator_length = extract_discriminator(accumulation_o);
            work_result_length = if work_result_discriminator_length > 0 {
                decode_e(&accumulation_o[..work_result_discriminator_length as usize])
            } else {
                0
            };

            start_address += work_result_discriminator_length as u64;
            remaining_length -= work_result_discriminator_length as u64;

            // store the work result address
            work_result_address = start_address;

            // update the address pointer and remaining length
            start_address += work_result_length as u64;
            remaining_length -= work_result_length as u64;
        }

        // skip l, k which are two 32 bytes hashes
        start_address += 32 + 32;
        remaining_length -= 32 + 32;

        // fetch auth output prefix
        let accumulation_o = unsafe { core::slice::from_raw_parts(start_address as *const u8, remaining_length as usize) };
        let auth_output_discriminator_length = extract_discriminator(accumulation_o);
        auth_output_length = if auth_output_discriminator_length > 0 {
            decode_e(&accumulation_o[..auth_output_discriminator_length as usize])
        } else {
            0
        };

        start_address += auth_output_discriminator_length as u64;
        remaining_length -= auth_output_discriminator_length as u64;

        // store the auth output address
        auth_output_address = start_address;

        // update the address pointer and remaining length
        start_address += auth_output_length as u64;
        remaining_length -= auth_output_length as u64;
    }

    // write FIB result to storage
    let key = [0u8; 1];
    let n: u64 = unsafe { ( *(work_result_address as *const u32)).into() };
    unsafe {
        write(key.as_ptr() as u64, key.len() as u64, work_result_address, work_result_length);
    }


    // Prepare some keys and hashes.
    let jam_key: [u8; 3] = [b'j', b'a', b'm'];
    let dot_val: [u8; 3] = [b'D', b'O', b'T'];
    
    // blake2b("jam") = 0x6a0d4a19d199505713fc65f531038e73f1d885645632c8ae503c4f0c4d5e19a7
    let jam_key_hash: [u8; 32] = [
        0x6a, 0x0d, 0x4a, 0x19, 0xd1, 0x99, 0x50, 0x57,
        0x13, 0xfc, 0x65, 0xf5, 0x31, 0x03, 0x8e, 0x73,
        0xf1, 0xd8, 0x85, 0x64, 0x56, 0x32, 0xc8, 0xae,
        0x50, 0x3c, 0x4f, 0x0c, 0x4d, 0x5e, 0x19, 0xa7
    ];

    // blake2b("dot") = 0xbfa9bb0fa4968747e63d3cf1e74a49ddc4a6eca89a6a6f339da3337fd2eb5507
    let dot_val_hash: [u8; 32] = [
        0xbf, 0xa9, 0xbb, 0x0f, 0xa4, 0x96, 0x87, 0x47,
        0xe6, 0x3d, 0x3c, 0xf1, 0xe7, 0x4a, 0x49, 0xdd,
        0xc4, 0xa6, 0xec, 0xa8, 0x9a, 0x6a, 0x6f, 0x33,
        0x9d, 0xa3, 0x33, 0x7f, 0xd2, 0xeb, 0x55, 0x07
    ];

    let JAM_KEY_ADDRESS: u64 = jam_key.as_ptr() as u64;
    let JAM_KEY_LENGTH: u64 = jam_key.len() as u64;

    let DOT_VAL_ADDRESS: u64 = dot_val.as_ptr() as u64;
    let DOT_VAL_LENGTH: u64 = dot_val.len() as u64;

    
    let JAM_KEY_HASH_ADDRESS: u64 = jam_key_hash.as_ptr() as u64;
    let JAM_KEY_HASH_LENGTH: u64 = jam_key_hash.len() as u64;

    let DOT_VAL_HASH_ADDRESS: u64 = dot_val_hash.as_ptr() as u64;
    let DOT_VAL_HASH_LENGTH: u64 = dot_val_hash.len() as u64;
    
    let info_bytes = [0u8; 100];
    let INFO_ADDRESS: u64 = info_bytes.as_ptr() as u64;
    let INFO_LENGTH: u64 = info_bytes.len() as u64;

    let mut buffer = [0u8; 256];
    let buffer_address = buffer.as_ptr() as u64;
    let buffer_length = buffer.len() as u64;

    // Depending on what "n" is, test different host functions
    if n == 1 {
        // do nothing
    } else if n == 2 {
        let read_none_result = unsafe { read(SERVICE_INDEX, JAM_KEY_ADDRESS, JAM_KEY_LENGTH, buffer_address, 0, buffer_length) }; // NONE
        write_result(read_none_result, 1);

        let write_result1 = unsafe { write(JAM_KEY_ADDRESS, JAM_KEY_LENGTH, DOT_VAL_ADDRESS, DOT_VAL_LENGTH) }; // OK
        write_result(write_result1, 2);

        let read_ok_result = unsafe { read(SERVICE_INDEX, JAM_KEY_ADDRESS, JAM_KEY_LENGTH, buffer_address, 0, buffer_length) }; // OK
        write_result(read_ok_result, 5);

        let forget_result = unsafe { forget(JAM_KEY_ADDRESS, 0) }; // HUH: not any lookup meet the condition
        write_result(forget_result, 6);
    } else if n == 3 {
        let solicit_result = unsafe { solicit(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: initialize empty timeslot
        write_result(solicit_result, 1);

        let query_jamhash_result = unsafe { query(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: 0
        write_result(query_jamhash_result, 2);

        let query_none_result = unsafe { query(DOT_VAL_HASH_ADDRESS, DOT_VAL_LENGTH) }; // NONE: no such key
        write_result(query_none_result, 5);
    } else if n == 4 {
        let forget_result = unsafe { forget(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: insert one timeslot
        write_result(forget_result, 1);

        let query_jamhash_result = unsafe { query(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: 2
        write_result(query_jamhash_result, 2);

        let lookup_none_result = unsafe { lookup(SERVICE_INDEX, DOT_VAL_HASH_ADDRESS, buffer_address, 0, DOT_VAL_LENGTH) }; // NONE: never written
        write_result(lookup_none_result, 5);

        let assign_result = unsafe { assign(1000, JAM_KEY_ADDRESS) }; // CORE: invalid core number
        write_result(assign_result, 6);
    } else if n == 5 {
        let lookup_result = unsafe { lookup(SERVICE_INDEX, JAM_KEY_HASH_ADDRESS, buffer_address, 0, JAM_KEY_LENGTH) }; // OK: 3
        write_result(lookup_result, 1);

        let read_ok_result = unsafe { query(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: 2
        write_result(read_ok_result, 2);

        let eject_who_result = unsafe { eject(SERVICE_INDEX, JAM_KEY_HASH_ADDRESS) }; // WHO: invalid service index
        write_result(eject_who_result, 5);

        let overflow_s = 0xFFFFFFFFFFFFu64;
        let bless_who_result = unsafe { bless(overflow_s, 0, 0, JAM_KEY_HASH_ADDRESS, 0) }; // WHO: invalid service index
        write_result(bless_who_result, 6);
    } else if n == 6 {
        let solicit_result = unsafe { solicit(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: insert one timeslot
        write_result(solicit_result, 1);

        let query_jamhash_result = unsafe { query(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: 3
        write_result(query_jamhash_result, 2);

        let core_index = 0;

        let mut auth_hashes = [0; 2560];
        let mut i = 0;
        while i < 80 {
            let offset = i * 32;
            auth_hashes[offset..offset + 32].copy_from_slice(&jam_key_hash);
            i += 1;
        }

        let assign_ok_result = unsafe { assign(core_index, auth_hashes.as_ptr() as u64) }; // OK
        write_result(assign_ok_result, 5);
    } else if n == 7 {
        let forget_result = unsafe { forget(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: adjust 3 timeslots to 2 timeslots
        write_result(forget_result, 1);

        let query_jamhash_result = unsafe { query(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: 2
        write_result(query_jamhash_result, 2);
    } else if n == 8 {
        let lookup_result = unsafe { lookup(SERVICE_INDEX, JAM_KEY_HASH_ADDRESS, buffer_address, 0, JAM_KEY_LENGTH) }; // OK: |v| = 3
        write_result(lookup_result, 1);

        let query_jamhash_result = unsafe { query(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: 2
        write_result(query_jamhash_result, 2);
    } else if n == 9 {
        let read_result = unsafe { read(SERVICE_INDEX, JAM_KEY_ADDRESS, JAM_KEY_LENGTH, buffer_address, 0, buffer_length) }; // OK |v| = 3
        write_result(read_result, 1);

        let write_result1 = unsafe { write(JAM_KEY_ADDRESS, JAM_KEY_LENGTH, 0, 0) }; // delete OK
        write_result(write_result1, 2);

        let read_ok_result = unsafe { read(SERVICE_INDEX, JAM_KEY_ADDRESS, JAM_KEY_LENGTH, buffer_address, 0, buffer_length) }; // NONE: deleted
        write_result(read_ok_result, 5);

        let solicit_result = unsafe { solicit(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH) }; // OK: insert one timeslot
        write_result(solicit_result, 6);
    } else if n == 1024 {
        let g: u64 = 911911; // this will trigger error: ServiceItemTooLow: Accumulated gas is below the service minimum.
        let m: u64 = 911911;
        let new_result = unsafe { new(JAM_KEY_HASH_ADDRESS, JAM_KEY_LENGTH, g, m) }; // OK
        write_result(new_result, 1);
        
        // this will trigger error: BadCodeHash: Work result code hash doesn't match the one expected for the service.
        let upgrade_result = unsafe { upgrade(JAM_KEY_HASH_ADDRESS, g, m) };
        write_result(upgrade_result, 2);

        let s:u32 = 911;
        let s_bytes = s.to_le_bytes();
        let gas_bytes = g.to_le_bytes();
        let mut bless_input = [0u8; 12];
        bless_input[..4].copy_from_slice(&s_bytes);
        bless_input[4..12].copy_from_slice(&gas_bytes);
        let bless_input_address = bless_input.as_ptr() as u64;
    
        let bless_ok_result = unsafe { bless(0, 1, 1, bless_input_address, 1) };
        write_result(bless_ok_result, 5);
    }

    // write info to 8
    let info_result = unsafe { info(SERVICE_INDEX, buffer_address) };
    write_result(info_result, 8);

    // write gas to 9
    let gas_result = unsafe { gas() };
    write_result(gas_result, 9);

    // Prepare an output buffer (pad result to 32 bytes).
    // pad result to 32 bytes
    let mut output_bytes_32 = [0u8; 32];
    output_bytes_32[..work_result_length as usize].copy_from_slice(&unsafe { core::slice::from_raw_parts(work_result_address as *const u8, work_result_length as usize) });
    let omega_7 = output_bytes_32.as_ptr() as u64;
    let omega_8 = output_bytes_32.len() as u64;

    // write yield
    if n % 3 == 0 {
        unsafe {
            core::arch::asm!(
                "li a0, 0",
                "li a1, 1",
                "jalr x0, a0, 0", // djump(0+0) causes panic
            );
        }
    }
    unsafe { oyield(omega_7); }
    // set the result length to register a1
    unsafe {
        core::arch::asm!(
            "mv a1, {0}",
            in(reg) omega_8,
        );
    }
    // this equals to a0 = omega_7
    omega_7
}

#[polkavm_derive::polkavm_export]
extern "C" fn on_transfer() -> u64 {
    0
}

// some helpful functions
fn extract_discriminator(input: &[u8]) -> u8 {
    if input.is_empty() {
        return 0;
    }

    let first_byte = input[0];
    match first_byte {
        1..=127 => 1,
        128..=191 => 2,
        192..=223 => 3,
        224..=239 => 4,
        240..=247 => 5,
        248..=251 => 6,
        252..=253 => 7,
        254..=u8::MAX => 8,
        _ => 0,
    }
}

fn power_of_two(exp: u32) -> u64 {
    1 << exp
}

fn decode_e_l(encoded: &[u8]) -> u64 {
    let mut x: u64 = 0;
    for &byte in encoded.iter().rev() {
        x = x.wrapping_mul(256).wrapping_add(byte as u64);
    }
    x
}

fn decode_e(encoded: &[u8]) -> u64 {
    let first_byte = encoded[0];
    if first_byte == 0 {
        return 0;
    }
    if first_byte == 255 {
        return decode_e_l(&encoded[1..9]);
    }
    for l in 0..8 {
        let left_bound  = 256 - power_of_two(8 - l);
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

fn write_result(result: u64, key: u8) {
    let key_bytes = key.to_le_bytes();
    let result_bytes = result.to_le_bytes();
    unsafe {
        write(key_bytes.as_ptr() as u64, key_bytes.len() as u64, result_bytes.as_ptr() as u64, result_bytes.len() as u64);
    }
}
