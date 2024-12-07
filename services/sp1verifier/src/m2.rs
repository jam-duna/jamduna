#![no_std]
#![no_main]

extern crate alloc;

use alloc::vec::Vec;
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
    pub fn gas() -> i64;
    #[polkavm_import(index = 1)]
    pub fn lookup(service: u32, hash_ptr: *const u8, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 2)]
    pub fn read(service: u32, key_ptr: *const u8, key_len: u32, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 3)]
    pub fn write(key_ptr: *const u8, key_len: u32, value: *const u8, value_len: u32) -> u32;
    #[polkavm_import(index = 4)]
    pub fn info(service: u32, out: *mut u8) -> u32;
    #[polkavm_import(index = 5)]
    pub fn empower(m: u32, a: u32, v: u32, o: u32, n: u32) -> u32;
    #[polkavm_import(index = 6)]
    pub fn assign(c: u32, out: *mut u8) -> u32;
    #[polkavm_import(index = 7)]
    pub fn designate(out: *mut u8) -> u32;
    #[polkavm_import(index = 8)]
    pub fn checkpoint() -> u64;
    #[polkavm_import(index = 9)]
    pub fn new(service: u32, hash_ptr: *const u8, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 10)]
    pub fn upgrade(out: *const u8, g: u64, m: u64) -> u32;
    #[polkavm_import(index = 11)]
    pub fn transfer(d: u32, a: u64, g: u64, out: *mut u8) -> u32;
    #[polkavm_import(index = 12)]
    pub fn quit(d: u32, a: u64, g: u64, out: *mut u8) -> u32;
    #[polkavm_import(index = 13)]
    pub fn solicit(hash_ptr: *const u8, z: u32) -> u32;
    #[polkavm_import(index = 14)]
    pub fn forget(hash_ptr: *const u8, z: u32) -> u32;
    #[polkavm_import(index = 15)]
    pub fn historical_lookup(service: u32, hash_ptr: *const u8, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 16)]
    pub fn import(import_index: u32, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 17)]
    pub fn export(out: *const u8, out_len: u32) -> u32;
    #[polkavm_import(index = 18)]
    pub fn machine(out: *const u8, out_len: u32) -> u32;
    #[polkavm_import(index = 19)]
    pub fn peek(out: *const u8, out_len: u32, i: u32) -> u32;
    #[polkavm_import(index = 20)]
    pub fn poke(n: u32, a: u32, b: u32, l: u32) -> u32;
    #[polkavm_import(index = 21)]
    pub fn invoke(n: u32, out: *mut u8) -> u32;
    #[polkavm_import(index = 22)]
    pub fn expunge(n: u32) -> u32;
    #[polkavm_import(index = 99)]
    pub fn sp1verify(vk: *const u8, vk_len: u32, proof: *const u8, proof_len: u32, out: *mut u8) -> u32;
}

#[polkavm_derive::polkavm_export]
extern "C" fn is_authorized() -> u32 {
    0
}

#[polkavm_derive::polkavm_export]
extern "C" fn refine() -> u32 {
    // get payload y
    let buffer_addr = buffer.as_ptr() as u32;
    let buffer_len = buffer.len() as u32;
    unsafe {
        core::arch::asm!(
            "mv a7, {0}",
            "mv a8, {1}",
            in(reg) buffer_addr,
            in(reg) buffer_len,
        );
    }
    // TODO:
    // 1. get payload y into vk
    // 2. get a set of extrinsics (chain_id, blocknumber, blockhash, proof_type, proof_len, proof)
    unsafe {
        sp1verify(vk, 256, proof, proof_len, result_addr);
        // for each extrinsic, output the result
        result_addr[0..4] = chain_id
        result_addr[4..8] = blocknumber
        result_addr[8..40] = blockhash
        result_addr[40] = proof_type
        result_addr[41] = result
    }
    return result_addr
}

#[polkavm_derive::polkavm_export]
extern "C" fn accumulate() -> u32 {
    let buffer = [0u8; 12];
    let key = [0u8; 1];

    // for each proof verification in result,  make a key out of (chain_id, blocknumber) + value out of (blockhash, proof_type, value)
    result_addr[0..4] = chain_id
    result_addr[4..8] = blocknumber
    result_addr[8..40] = blockhash
    result_addr[40] = proof_type
    result_addr[41] = result
    unsafe {
        // write the key-value to the service
        write(key.as_ptr(), 1, buffer.as_ptr(), buffer.len() as u32);
    }
    // solicit the new block / header
    // forget the old block / header

    // if chain_id == 1 then finalize all the dependent chains that have a storage proof of L2
    // generate accumation output
    let buffer_addr = buffer.as_ptr() as u32;
    let buffer_len = buffer.len() as u32;
    unsafe {
        core::arch::asm!(
            "mv a3, {0}",
            "mv a4, {1}",
            in(reg) buffer_addr,
            in(reg) buffer_len,
        );
    }
    0
}

#[polkavm_derive::polkavm_export]
extern "C" fn on_transfer() -> u32 {
    0
}
