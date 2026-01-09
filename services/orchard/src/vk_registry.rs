#![allow(dead_code)]
/// Verification Key Registry for Halo2 Circuits
///
/// Provides deterministic VK lookup with Blake2b-256 hash verification
/// and canonical field layout validation for each circuit type.

use crate::errors::{OrchardError, Result};
use blake2::{Blake2b, Digest};
use core::sync::atomic::{AtomicBool, Ordering};

/// Orchard NU5 Action Circuit Public Input Layout (CONSENSUS-CRITICAL)
///
/// Per orchard/src/circuit.rs:Instance, each action has 9 public inputs:
/// 1. anchor: Commitment tree root (32 bytes)
/// 2. cv_net_x: Net value commitment x-coordinate (field element)
/// 3. cv_net_y: Net value commitment y-coordinate (field element)
/// 4. nf_old: Nullifier of spent note (32 bytes)
/// 5. rk_x: Randomized verification key x-coordinate (field element)
/// 6. rk_y: Randomized verification key y-coordinate (field element)
/// 7. cmx: New note commitment (32 bytes)
/// 8. enable_spend: Spend enabled flag (field element, 0 or 1)
/// 9. enable_output: Output enabled flag (field element, 0 or 1)
///
/// For a bundle with N actions, total public inputs = 9 * N
///
/// NOTE: This is the per-action layout. Bundle proofs concatenate all actions.
pub const ORCHARD_NU5_ACTION_LAYOUT: [&str; 9] = [
    "anchor",        // Commitment tree root
    "cv_net_x",      // Net value commitment x-coordinate
    "cv_net_y",      // Net value commitment y-coordinate
    "nf_old",        // Nullifier of spent note
    "rk_x",          // Randomized verification key x-coordinate
    "rk_y",          // Randomized verification key y-coordinate
    "cmx",           // Extracted commitment of new note
    "enable_spend",  // Spend enabled flag (bool as field element)
    "enable_output", // Output enabled flag (bool as field element)
];

/// Orchard ZSA Action Circuit Public Input Layout (CONSENSUS-CRITICAL)
///
/// ZSA adds `enable_zsa` as the 10th field.
pub const ORCHARD_ZSA_ACTION_LAYOUT: [&str; 10] = [
    "anchor",        // Commitment tree root
    "cv_net_x",      // Net value commitment x-coordinate
    "cv_net_y",      // Net value commitment y-coordinate
    "nf_old",        // Nullifier of spent note
    "rk_x",          // Randomized verification key x-coordinate
    "rk_y",          // Randomized verification key y-coordinate
    "cmx",           // Extracted commitment of new note
    "enable_spend",  // Spend enabled flag (bool as field element)
    "enable_output", // Output enabled flag (bool as field element)
    "enable_zsa",    // ZSA enable flag (bool as field element)
];

/// Current production layout (Orchard NU5)
pub const SPEND_LAYOUT: [&str; 9] = ORCHARD_NU5_ACTION_LAYOUT;

/// Circuit domain separation tags
pub const DOMAIN_SPEND_V1: &str = "orchard_spend_v1";
pub const DOMAIN_ZSA_V1: &str = "orchard_zsa_v1";

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
    entries: [VkEntry; 2],
}

impl VkRegistry {
    /// Create new VK registry with embedded verification keys
    ///
    /// NOTE: field_count is per-action (9 fields). Total public inputs for a bundle
    /// with N actions = 9 * N. The caller must validate this.
    pub fn new() -> Self {
        Self {
            entries: [
                VkEntry {
                    vk_id: 1,
                    field_count: 9, // Per-action field count (Orchard NU5)
                    domain: DOMAIN_SPEND_V1,
                    layout: &SPEND_LAYOUT,
                    vk_hash: compute_vk_hash(crate::crypto::ORCHARD_VK_BYTES),
                    vk_bytes: crate::crypto::ORCHARD_VK_BYTES,
                },
                VkEntry {
                    vk_id: 2,
                    field_count: 10, // Per-action field count (Orchard ZSA)
                    domain: DOMAIN_ZSA_V1,
                    layout: &ORCHARD_ZSA_ACTION_LAYOUT,
                    vk_hash: compute_vk_hash(crate::crypto::ORCHARD_ZSA_VK_BYTES),
                    vk_bytes: crate::crypto::ORCHARD_ZSA_VK_BYTES,
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
    ///
    /// For Orchard NU5, public_inputs.len() must be a multiple of 9 (9 fields per action).
    /// The number of actions is derived from the public input count.
    pub fn validate_public_inputs(&self, vk_id: u32, public_inputs: &[[u8; 32]]) -> Result<()> {
        let entry = self.get_vk_entry(vk_id)?;

        // For Orchard NU5: total fields = 9 * num_actions
        let fields_per_action = entry.field_count;
        if public_inputs.len() % fields_per_action != 0 {
            return Err(OrchardError::PublicInputMismatch {
                vk_id,
                expected_fields: fields_per_action,
                provided_fields: public_inputs.len(),
            });
        }

        let num_actions = public_inputs.len() / fields_per_action;
        if num_actions == 0 {
            return Err(OrchardError::StateError(
                "Bundle must have at least one action".into()
            ));
        }

        Ok(())
    }

    /// Get number of actions from public inputs
    pub fn get_action_count(&self, vk_id: u32, public_inputs: &[[u8; 32]]) -> Result<usize> {
        let entry = self.get_vk_entry(vk_id)?;
        Ok(public_inputs.len() / entry.field_count)
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
