#![no_std]
#![no_main]

extern crate alloc;
use alloc::string::String;
use simplealloc::SimpleAlloc;

#[global_allocator]
static ALLOCATOR: SimpleAlloc<4096> = SimpleAlloc::new();

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    unsafe {
        core::arch::asm!("unimp", options(noreturn));
    }
}

use miniserde::json;

#[derive(Debug, miniserde::Deserialize)]
struct Person {
    name: String,
    age: u8,
}

#[polkavm_derive::polkavm_export]
extern "C" fn refine() -> u64 {
    let json_str = r#"{"name": "Alice", "age": 25}"#;
    let person: Person = json::from_str(json_str).unwrap();
    let person_name = person.name;
    let name_bytes = person_name.as_bytes();
    let name_ptr = name_bytes.as_ptr() as u64;
    let name_len = name_bytes.len() as u64;
    unsafe {
        core::arch::asm!(
            "mv a1, {0}",
            in(reg) name_len,
        );
    }
    name_ptr
}


#[polkavm_derive::polkavm_export]
extern "C" fn accumulate() -> u64 {
    0
}

#[polkavm_derive::polkavm_export]
extern "C" fn on_transfer() -> u32 {
    0
}
