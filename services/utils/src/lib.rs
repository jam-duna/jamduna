#![no_std]

extern crate alloc;

pub mod constants;
pub mod effects;
pub mod functions;
pub mod hash_functions;
pub mod helpers;
pub mod host_functions;
pub mod objects;
pub mod tracking;

#[cfg(test)]
mod tests;

#[cfg(target_os = "none")]
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    #[cfg(any(target_arch = "riscv64", target_arch = "riscv32"))]
    unsafe {
        core::arch::asm!("unimp", options(noreturn));
    }

    #[cfg(not(any(target_arch = "riscv64", target_arch = "riscv32")))]
    {
        loop {
            core::hint::spin_loop();
        }
    }
}
