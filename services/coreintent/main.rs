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
    #[polkavm_import(index = 2)]
    pub fn read(service: u32, key_ptr: *const u8, key_len: u32, out: *mut u8, out_len: u32) -> u32;
    #[polkavm_import(index = 3)]
    pub fn write(ko: u32, kz: u32, bo: u32, bz: u32) -> u32;
    #[polkavm_import(index = 30)]
    pub fn fetch(o: u32, l_off: u64, dataid: u64) -> u32;

    #[polkavm_import(index = 64)]
    pub fn sp1verify(proof: u32, l: u32, publicvalues: u32, publicvalues_len: u32, verifierkey: u32) -> u32;
    #[polkavm_import(index = 65)]
    pub fn ed25519verify(sig: u32, msg: u32, l: u32, pubkey: u32) -> u32;
    #[polkavm_import(index = 66)]
    pub fn ietfvrfverify(pubKey: u32, signature: u32, vrfInputData: u32, auxData: u32) -> u32;
}


pub const SIGNATURE_LEN: usize = 96;
pub type AccountId = [u8; 32];
pub type ProgramId = [u8; 32];
pub type AssetId = u64;

// Structures
#[repr(C)]
#[derive(Default, Debug, Clone)]
pub struct ProofRecord {
    pub user: AccountId,
    pub deadline_contest: u64,
    pub deadline_fulfillment: u64,
    pub cycle_count: u64,
    pub fee: u64,
    pub proof_type: u16,
    pub prover: AccountId,
    pub ticket_id: ProgramId,
    pub verifier_key: ProgramId,
    pub proof_data: Vec<u8>,
    pub public_values: Vec<u8>,
}

#[repr(C)]
#[derive(Default, Debug, Clone)]
pub struct Asset {
    pub issuer: AccountId,
    pub min_balance: u64,
    pub decimals: u8,
    pub symbol: [u8; 32],
    pub total_supply: u64,
}

#[repr(C)]
#[derive(Default, Debug, Clone)]
pub struct Account {
    pub free: u64,
    pub reserved: u64,
}
#[polkavm_derive::polkavm_export]
extern "C" fn is_authorized() -> u32 {
    0
}

#[polkavm_derive::polkavm_export]
extern "C" fn refine() -> u32 {
    // TODO: use fetch to iterate over all extrinsics https://github.com/gavofyork/graypaper/issues/186
    // for each extrinsic, call ed25519verify

    // Separate extrinsic data and signature
    if extrinsic.len() < SIGNATURE_LEN {
        return; // Invalid extrinsic
    }
    let (data, signature) = extrinsic.split_at(extrinsic.len() - SIGNATURE_LEN);

    // Extract method ID and payload
    let method_id = &data[..10];
    let payload = &data[10..];

    // Extract public key (assume public key is part of the payload for this example)
    let public_key = Self::extract_account(&payload[..32]);

    // Verify signature
    let is_valid = unsafe {
        verify_signature(
            public_key.as_ptr(),
            data.as_ptr(),
            data.len() as u32,
            signature.as_ptr(),
            SIGNATURE_LEN as u32,
        )
    };
    if is_valid == 1 {
        // TODO: add to work result
        return; // Invalid signature
    }
}

#[polkavm_derive::polkavm_export]
extern "C" fn accumulate() -> u32 {

    // Call appropriate service directly, assuming verification and validation are done
    let method_id = &extrinsic[..10];
    let payload = &extrinsic[10..];

    match method_id {
        b"submituserintent" => self.submit_user_intent(payload),
        b"entercontest" => self.enter_contest(payload),
        b"fulfillcontent" => self.fulfill_intent(payload),
        _ => {} // Unrecognized method ID
        _ => {} // Unrecognized method ID
    }
}

#[polkavm_derive::polkavm_export]
extern "C" fn on_transfer() -> u32 {
    0
}

pub fn quit_service(&mut self, payload: &[u8]) {
    let asset_id = Self::extract_u64(&payload[0..8]);
    let amount = Self::extract_u64(&payload[8..16]);
    let service_id = Self::extract_u32(&payload[16..20]);
    let sender = Self::extract_account(&payload[20..52]);

    let mut sender_account = self.read_account(asset_id, &sender);
    if sender_account.free >= amount {
        sender_account.free -= amount;
        self.write_account(asset_id, &sender, &sender_account);

        let service_key = Self::generate_service_key(service_id);
        let mut service_account = self.read_account(asset_id, &service_key);
        service_account.free += amount;
        self.write_account(asset_id, &service_key, &service_account);
    }
}

fn read_account(&self, asset_id: AssetId, account_id: &AccountId) -> Account {
    let mut buf = [0u8; core::mem::size_of::<Account>()];
    let key = Self::generate_account_key(asset_id, account_id);
    unsafe {
        read(1, key.as_ptr(), key.len() as u32, buf.as_mut_ptr(), buf.len() as u32);
    }
    unsafe { core::ptr::read(buf.as_ptr() as *const _) }
}

fn write_account(&self, asset_id: AssetId, account_id: &AccountId, account: &Account) {
    let key = Self::generate_account_key(asset_id, account_id);
    let account_bytes = unsafe {
        core::slice::from_raw_parts(
            account as *const _ as *const u8,
            core::mem::size_of::<Account>(),
        )
    };
    unsafe {
        write(key.as_ptr() as u32, key.len() as u32, account_bytes.as_ptr() as u32, account_bytes.len() as u32);
    }
}

// Helper functions for key generation
fn generate_account_key(asset_id: AssetId, account_id: &AccountId) -> Vec<u8> {
    let mut key = asset_id.to_be_bytes().to_vec();
    key.extend_from_slice(account_id);
    key
}

fn generate_service_key(service_id: u32) -> AccountId {
    let mut key = [0u8; 32];
    key[0..4].copy_from_slice(&service_id.to_be_bytes());
    key
}

// Extraction helper functions
fn extract_account(bytes: &[u8]) -> AccountId {
    let mut account = [0u8; 32];
    account.copy_from_slice(bytes);
    account
}

fn extract_program_id(bytes: &[u8]) -> ProgramId {
    let mut id = [0u8; 32];
    id.copy_from_slice(bytes);
    id
}

fn extract_symbol(bytes: &[u8]) -> [u8; 32] {
    let mut symbol = [0u8; 32];
    symbol.copy_from_slice(bytes);
    symbol
}

fn extract_u64(bytes: &[u8]) -> u64 {
    u64::from_be_bytes(bytes.try_into().unwrap())
}

fn extract_u32(bytes: &[u8]) -> u32 {
    u32::from_be_bytes(bytes.try_into().unwrap())
}

fn extract_u16(bytes: &[u8]) -> u16 {
    u16::from_be_bytes(bytes.try_into().unwrap())
}

pub fn submit_user_intent(&mut self, payload: &[u8]) {
    let user = Self::extract_account(&payload[0..32]);
    let program_id = Self::extract_program_id(&payload[32..64]);
    let deadline_contest = Self::extract_u64(&payload[64..72]);
    let deadline_fulfillment = Self::extract_u64(&payload[72..80]);
    let cycle_count = Self::extract_u64(&payload[80..88]);
    let fee = Self::extract_u64(&payload[88..96]);
    let proof_type = Self::extract_u16(&payload[96..98]);

    let proof_record = ProofRecord {
        user,
        deadline_contest,
        deadline_fulfillment,
        cycle_count,
        fee,
        proof_type,
        ..Default::default()
    };

    self.write_intent(&program_id, &proof_record);
}

pub fn enter_contest(&mut self, payload: &[u8]) {
    let prover = Self::extract_account(&payload[0..32]);
    let program_id = Self::extract_program_id(&payload[32..64]);
    // TODO: get this right between refine (which should verify sig) and accumulate
    let ticket_id = Self::ietfvrfverify(&payload[64..96]);

    let mut proof_record = self.read_proof_record(&program_id);
    proof_record.prover = prover;
    proof_record.ticket_id = ticket_id;
    self.write_intent(&program_id, &proof_record);
}

pub fn fulfill_intent(&mut self, payload: &[u8]) {
    let prover = Self::extract_account(&payload[0..32]);
    let program_id = Self::extract_program_id(&payload[32..64]);
    let verifier_key = Self::extract_program_id(&payload[64..96]);
    let proof_data = payload[96..payload.len() / 2].to_vec();
    let public_values = payload[payload.len() / 2..].to_vec();

    let mut proof_record = self.read_intent(&program_id);

    // TODO: call sp1verify(proof: u32, l: u32, publicvalues: u32, publicvalues_len: u32, verifierkey: u32) -> u32;

    proof_record.verifier_key = verifier_key;
    proof_record.proof_data = proof_data;
    proof_record.public_values = public_values;

    self.write_intent(&program_id, &proof_record);
}

// Helper functions to read and write to service storage
fn read_intent(&self, program_id: &ProgramId) -> ProofRecord {
    let mut buf = vec![0u8; 1024]; // Adjust buffer size as needed for maximum expected size
    unsafe {
        read(128, program_id.as_ptr(), program_id.len() as u32, buf.as_mut_ptr(), buf.len() as u32);
    }
    unsafe { core::ptr::read(buf.as_ptr() as *const _) }
}

fn write_intent(&self, program_id: &ProgramId, record: &ProofRecord) {
    let record_bytes = unsafe {
        core::slice::from_raw_parts(
            record as *const _ as *const u8,
            core::mem::size_of::<ProofRecord>(),
        )
    };
    unsafe {
        write(program_id.as_ptr() as u32, program_id.len() as u32, record_bytes.as_ptr() as u32, record_bytes.len() as u32);
    }
}

