#![no_std]
#![no_main]

extern crate alloc;

use alloc::vec;
use polkavm_derive::min_stack_size;
use simplealloc::SimpleAlloc;

const STACK_SIZE: usize = 0x10000;
min_stack_size!(STACK_SIZE);

const HEAP_SIZE: usize = 0x40000;
#[global_allocator]
static ALLOCATOR: SimpleAlloc<HEAP_SIZE> = SimpleAlloc::new();

const DEFAULT_ITERS: u64 = 200_000;
const DEFAULT_SIZE: usize = 64 * 1024;
const DEFAULT_STRIDE: usize = 64;
const MIN_SIZE: usize = 1024;
const OUTPUT_LEN: u64 = 32;

static mut OUTPUT: [u8; 32] = [0; 32];

fn read_u64(input: &[u8], offset: usize) -> Option<u64> {
	if input.len() < offset + 8 {
		return None;
	}
	let mut buf = [0u8; 8];
	buf.copy_from_slice(&input[offset..offset + 8]);
	Some(u64::from_le_bytes(buf))
}

fn clamp_size(size: usize) -> usize {
	let max_size = HEAP_SIZE.saturating_sub(MIN_SIZE);
	let mut out = size;
	if out < MIN_SIZE {
		out = MIN_SIZE;
	}
	if out > max_size {
		out = max_size;
	}
	out
}

fn read_params(start_address: u64, length: u64) -> (u64, usize, usize) {
	if start_address == 0 || length == 0 {
		return (DEFAULT_ITERS, DEFAULT_SIZE, DEFAULT_STRIDE);
	}

	let len = core::cmp::min(length, usize::MAX as u64) as usize;
	if len == 0 {
		return (DEFAULT_ITERS, DEFAULT_SIZE, DEFAULT_STRIDE);
	}

	let input = unsafe { core::slice::from_raw_parts(start_address as *const u8, len) };
	let iters = read_u64(input, 0).unwrap_or(DEFAULT_ITERS);
	let size = read_u64(input, 8).unwrap_or(DEFAULT_SIZE as u64) as usize;
	let stride = read_u64(input, 16).unwrap_or(DEFAULT_STRIDE as u64) as usize;

	(iters, clamp_size(size), stride)
}

#[polkavm_derive::polkavm_export]
extern "C" fn main(start_address: u64, length: u64) -> (u64, u64) {
	let (iters, size, stride_raw) = read_params(start_address, length);
	let mut stride = stride_raw.max(1);
	if stride >= size {
		stride %= size;
		stride = stride.max(1);
	}

	let mut buffer = vec![0u8; size];
	for (i, byte) in buffer.iter_mut().enumerate() {
		*byte = (i as u8).wrapping_mul(31).wrapping_add((size as u8) ^ 0x5a);
	}

	let mut acc: u64 = 0;
	let mut idx: usize = (size ^ (iters as usize)) % size;
	let mut iter = 0u64;
	while iter < iters {
		idx = (idx + stride) % size;
		let base = idx;

		let mut word: u64 = 0;
		let mut i = 0usize;
		while i < 8 {
			let pos = (base + i) % size;
			word |= (buffer[pos] as u64) << (i * 8);
			i += 1;
		}
		word = word.wrapping_add(acc ^ iter);

		i = 0;
		while i < 8 {
			let pos = (base + i) % size;
			buffer[pos] = (word >> (i * 8)) as u8;
			i += 1;
		}

		let pos1 = (base + (stride >> 1) + 13) % size;
		let pos2 = (base + (stride << 1) + 29) % size;
		let v1 = buffer[pos1];
		let v2 = buffer[pos2];
		buffer[pos1] = v1.wrapping_add((acc as u8) ^ (iter as u8));
		buffer[pos2] = v2.wrapping_sub((acc as u8).wrapping_add(7));

		acc = acc.wrapping_add(word).wrapping_add(v1 as u64).wrapping_add(v2 as u64);
		iter = iter.wrapping_add(1);
	}

	let mut checksum: u64 = 0;
	let mut pos = 0usize;
	while pos < buffer.len() {
		checksum = checksum.wrapping_add(buffer[pos] as u64).rotate_left(5);
		pos += 64;
	}

	unsafe {
		let out0 = acc;
		let out1 = checksum ^ (size as u64);
		let out2 = iters;
		let out3 = acc ^ checksum ^ (stride as u64);

		OUTPUT[0..8].copy_from_slice(&out0.to_le_bytes());
		OUTPUT[8..16].copy_from_slice(&out1.to_le_bytes());
		OUTPUT[16..24].copy_from_slice(&out2.to_le_bytes());
		OUTPUT[24..32].copy_from_slice(&out3.to_le_bytes());

		let out_ptr = core::ptr::addr_of_mut!(OUTPUT) as *mut u8;
		(out_ptr as u64, OUTPUT_LEN)
	}
}

#[cfg(all(not(test), target_arch = "riscv32", target_feature = "e"))]
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
	unsafe {
		core::arch::asm!("unimp", options(noreturn));
	}
}

#[cfg(not(all(not(test), target_arch = "riscv32", target_feature = "e")))]
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
	loop {}
}
