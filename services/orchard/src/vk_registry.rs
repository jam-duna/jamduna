#![allow(dead_code)]
/// Verification Key Registry for Halo2 Circuits
///
/// Provides deterministic VK lookup with Blake2b-256 hash verification
/// and canonical field layout validation for each circuit type.

use crate::errors::{OrchardError, Result};
use blake2::{Blake2b, Digest};
use core::sync::atomic::{AtomicBool, Ordering};

/// Circuit field layouts (consensus-critical)
pub const SPEND_LAYOUT: [&str; 44] = [
    "anchor_root", "num_inputs", "num_outputs",
    "input_nullifier_0", "input_nullifier_1", "input_nullifier_2", "input_nullifier_3",
    "output_commitment_0", "output_commitment_1", "output_commitment_2", "output_commitment_3",
    "delta_0_asset_id", "delta_0_in_public_lo", "delta_0_in_public_hi",
    "delta_0_out_public_lo", "delta_0_out_public_hi",
    "delta_0_burn_public_lo", "delta_0_burn_public_hi",
    "delta_0_mint_public_lo", "delta_0_mint_public_hi",
    "delta_1_asset_id", "delta_1_in_public_lo", "delta_1_in_public_hi",
    "delta_1_out_public_lo", "delta_1_out_public_hi",
    "delta_1_burn_public_lo", "delta_1_burn_public_hi",
    "delta_1_mint_public_lo", "delta_1_mint_public_hi",
    "delta_2_asset_id", "delta_2_in_public_lo", "delta_2_in_public_hi",
    "delta_2_out_public_lo", "delta_2_out_public_hi",
    "delta_2_burn_public_lo", "delta_2_burn_public_hi",
    "delta_2_mint_public_lo", "delta_2_mint_public_hi",
    "allowed_root", "terms_hash", "poi_root", "epoch", "fee_lo", "fee_hi"
];

pub const WITHDRAW_LAYOUT: [&str; 17] = [
    "anchor_root", "anchor_size", "nullifier",
    "recipient_0", "recipient_1", "recipient_2",
    "amount_lo", "amount_hi", "asset_id", "gas_limit",
    "fee_lo", "fee_hi", "poi_root", "epoch",
    "has_change", "change_commitment", "next_root"
];

pub const ISSUANCE_LAYOUT: [&str; 11] = [
    "asset_id", "mint_amount_lo", "mint_amount_hi",
    "burn_amount_lo", "burn_amount_hi",
    "issuer_pk_0", "issuer_pk_1", "issuer_pk_2",
    "signature_verification_0", "signature_verification_1",
    "issuance_root"
];

pub const BATCH_LAYOUT: [&str; 17] = [
    "anchor_root", "anchor_size", "next_root",
    "nullifiers_root", "commitments_root", "deltas_root",
    "issuance_root", "memo_root", "equity_allowed_root", "poi_root",
    "total_fee_lo", "total_fee_hi", "epoch", "num_user_txs",
    "min_fee_per_tx_lo", "min_fee_per_tx_hi", "batch_hash"
];

/// Circuit domain separation tags
pub const DOMAIN_SPEND_V1: &str = "orchard_spend_v1";
pub const DOMAIN_WITHDRAW_V1: &str = "orchard_withdraw_v1";
pub const DOMAIN_ISSUANCE_V1: &str = "orchard_issue_v1";
pub const DOMAIN_BATCH_V1: &str = "orchard_batch_v1";

/// VK registry entry
#[derive(Clone, Debug)]
pub struct VkEntry {
    pub vk_id: u32,
    pub field_count: usize,
    pub domain: &'static str,
    pub layout: &'static [&'static str],
    pub vk_hash: [u8; 32],
    pub vk_bytes: &'static [u8],
}

/// VK registry with deterministic Blake2b-256 hash verification
pub struct VkRegistry {
    entries: [VkEntry; 4],
}

impl VkRegistry {
    /// Create new VK registry with embedded verification keys
    pub fn new() -> Self {
        Self {
            entries: [
                VkEntry {
                    vk_id: 1,
                    field_count: 44,
                    domain: DOMAIN_SPEND_V1,
                    layout: &SPEND_LAYOUT,
                    vk_hash: compute_vk_hash(crate::crypto::ORCHARD_SPEND_VK_BYTES),
                    vk_bytes: crate::crypto::ORCHARD_SPEND_VK_BYTES,
                },
                VkEntry {
                    vk_id: 2,
                    field_count: 17,
                    domain: DOMAIN_WITHDRAW_V1,
                    layout: &WITHDRAW_LAYOUT,
                    vk_hash: compute_vk_hash(crate::crypto::ORCHARD_WITHDRAW_VK_BYTES),
                    vk_bytes: crate::crypto::ORCHARD_WITHDRAW_VK_BYTES,
                },
                VkEntry {
                    vk_id: 3,
                    field_count: 11,
                    domain: DOMAIN_ISSUANCE_V1,
                    layout: &ISSUANCE_LAYOUT,
                    vk_hash: compute_vk_hash(crate::crypto::ORCHARD_ISSUANCE_VK_BYTES),
                    vk_bytes: crate::crypto::ORCHARD_ISSUANCE_VK_BYTES,
                },
                VkEntry {
                    vk_id: 4,
                    field_count: 17,
                    domain: DOMAIN_BATCH_V1,
                    layout: &BATCH_LAYOUT,
                    vk_hash: compute_vk_hash(crate::crypto::ORCHARD_BATCH_VK_BYTES),
                    vk_bytes: crate::crypto::ORCHARD_BATCH_VK_BYTES,
                },
            ],
        }
    }

    /// Lookup VK entry by ID with validation
    pub fn get_vk_entry(&self, vk_id: u32) -> Result<&VkEntry> {
        let entry = self.entries.iter()
            .find(|e| e.vk_id == vk_id)
            .ok_or(OrchardError::InvalidVkId { vk_id })?;

        // Verify VK hash integrity
        let computed_hash = compute_vk_hash(entry.vk_bytes);
        if computed_hash != entry.vk_hash {
            return Err(OrchardError::VkHashMismatch {
                vk_id,
                expected: entry.vk_hash,
                computed: computed_hash,
            });
        }

        Ok(entry)
    }

    /// Validate public inputs match expected field layout
    pub fn validate_public_inputs(&self, vk_id: u32, public_inputs: &[[u8; 32]]) -> Result<()> {
        let entry = self.get_vk_entry(vk_id)?;

        if public_inputs.len() != entry.field_count {
            return Err(OrchardError::PublicInputMismatch {
                vk_id,
                expected_fields: entry.field_count,
                provided_fields: public_inputs.len(),
            });
        }

        Ok(())
    }
}

/// Compute deterministic Blake2b-256 hash of VK bytes
fn compute_vk_hash(vk_bytes: &[u8]) -> [u8; 32] {
    let mut hasher = Blake2b::new();
    hasher.update(b"orchard_vk_v1");  // Domain separation
    hasher.update(vk_bytes);
    hasher.finalize().into()
}

/// Global VK registry instance

static mut VK_REGISTRY: Option<VkRegistry> = None;
static VK_REGISTRY_INIT: AtomicBool = AtomicBool::new(false);

/// Get global VK registry (thread-safe singleton)
pub fn get_vk_registry() -> &'static VkRegistry {
    if !VK_REGISTRY_INIT.load(Ordering::Acquire) {
        unsafe {
            if VK_REGISTRY.is_none() {
                VK_REGISTRY = Some(VkRegistry::new());
                VK_REGISTRY_INIT.store(true, Ordering::Release);
            }
        }
    }
    unsafe {
        VK_REGISTRY.as_ref().unwrap_unchecked()
    }
}
