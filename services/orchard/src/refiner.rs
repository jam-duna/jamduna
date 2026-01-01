/// Orchard refine implementation - Real Zcash Orchard bundle verification
///
/// Verifies production Orchard bundles using native Halo2 proof verification.
/// Enforces JAM-specific requirements: stateless verification with sparse Merkle proofs.

use alloc::{vec::Vec, format};
use crate::bundle_codec::{decode_bundle, DecodedBundle};
use crate::errors::{OrchardError, Result};
use crate::objects::ObjectKind;
use crate::state::{OrchardExtrinsic, MerkleProof};
use crate::crypto::merkle_root_from_leaves;
use utils::effects::WriteIntent;
use utils::functions::{log_info, log_error, RefineContext};

struct OrchardBatchVerifier {
    #[cfg(all(feature = "orchard", feature = "std"))]
    validator: orchard::bundle::BatchValidator,
    #[cfg(all(feature = "orchard", feature = "std"))]
    count: usize,
}

impl OrchardBatchVerifier {
    fn new() -> Self {
        Self {
            #[cfg(all(feature = "orchard", feature = "std"))]
            validator: orchard::bundle::BatchValidator::new(),
            #[cfg(all(feature = "orchard", feature = "std"))]
            count: 0,
        }
    }

    fn add_decoded_bundle(&mut self, decoded: &DecodedBundle) -> Result<()> {
        #[cfg(all(feature = "orchard", feature = "std"))]
        {
            let bundle = crate::bundle_codec::to_orchard_bundle(decoded)?;
            let sighash: [u8; 32] = bundle.commitment().into();
            self.validator.add_bundle(&bundle, sighash);
            self.count += 1;
            return Ok(());
        }

        #[cfg(not(all(feature = "orchard", feature = "std")))]
        {
            let _ = decoded;
            log_error("Orchard verification unavailable (enable features: orchard,std)");
            Err(OrchardError::InvalidProof)
        }
    }

    fn verify(self) -> Result<()> {
        #[cfg(all(feature = "orchard", feature = "std"))]
        {
            if self.count == 0 {
                return Ok(());
            }

            let vk = cached_orchard_vk();
            let ok = self.validator.validate(vk, rand::rngs::OsRng);
            if ok {
                Ok(())
            } else {
                Err(OrchardError::InvalidProof)
            }
        }

        #[cfg(not(all(feature = "orchard", feature = "std")))]
        {
            Err(OrchardError::InvalidProof)
        }
    }
}

#[cfg(all(feature = "orchard", feature = "std"))]
fn cached_orchard_vk() -> &'static orchard::circuit::VerifyingKey {
    use std::sync::OnceLock;

    static VK: OnceLock<orchard::circuit::VerifyingKey> = OnceLock::new();
    VK.get_or_init(|| {
        let maybe_vk = load_orchard_vk_from_disk();
        maybe_vk.unwrap_or_else(orchard::circuit::VerifyingKey::build)
    })
}

#[cfg(all(feature = "orchard", feature = "std"))]
fn load_orchard_vk_from_disk() -> Option<orchard::circuit::VerifyingKey> {
    use std::fs::File;
    use std::io::BufReader;
    use pasta_curves::vesta::Affine as VestaAffine;
    use halo2_proofs::poly::commitment::Params;

    let keys_dir = std::env::var("ORCHARD_KEYS_DIR").ok()
        .map(std::path::PathBuf::from)
        .unwrap_or_else(|| {
            std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
                .join("../../keys/orchard")
        });
    let params_path = keys_dir.join("orchard.params");
    let vk_path = keys_dir.join("orchard.vk");

    let params_file = File::open(params_path).ok()?;
    let params = Params::<VestaAffine>::read(&mut BufReader::new(params_file)).ok()?;
    let vk_file = File::open(vk_path).ok()?;
    orchard::circuit::VerifyingKey::read(params, BufReader::new(vk_file)).ok()
}

/// Output from witness-aware refiner execution
#[derive(Debug, Clone)]
#[allow(dead_code)]
pub enum RefineOutput {
    Builder {
        intents: Vec<WriteIntent>,
    },
    Guarantor {
        intents: Vec<WriteIntent>,
    },
}

/// Process Orchard bundle extrinsics
pub fn process_extrinsics_witness_aware(
    service_id: u32,
    refine_context: &RefineContext,
    extrinsics: &[OrchardExtrinsic],
    _witness: Option<&()>,  // Simplified - witnesses now inline with extrinsics
) -> Result<RefineOutput> {
    let mut all_intents = Vec::new();
    let mut batch = OrchardBatchVerifier::new();

    for extrinsic in extrinsics {
        match extrinsic {
            OrchardExtrinsic::BundleSubmit {
                bundle_bytes,
                nullifier_absence_proofs,
            } => {
                let intents = verify_bundle_and_generate_intents(
                    service_id,
                    refine_context,
                    bundle_bytes,
                    nullifier_absence_proofs,
                    &mut batch,
                )?;
                all_intents.extend(intents);
            }
        }
    }

    batch.verify()?;

    // For now, treat all executions as guarantor mode
    Ok(RefineOutput::Guarantor {
        intents: all_intents,
    })
}

/// Verify Orchard bundle and generate write intents
///
/// Steps:
/// 1. Deserialize Bundle<Authorized, i64> from bundle_bytes
/// 2. Extract anchor, nullifiers, commitments from bundle actions
/// 3. Verify anchor matches current state.commitment_root
/// 4. Verify nullifiers are absent via sparse Merkle proofs
/// 5. Verify Halo2 proof using Orchard's native verification
/// 6. Verify zero-sum: value_balance == 0 (fully shielded)
/// 7. Generate WriteIntents for nullifier insertions and commitment tree updates
fn verify_bundle_and_generate_intents(
    service_id: u32,
    refine_context: &RefineContext,
    bundle_bytes: &[u8],
    nullifier_absence_proofs: &[MerkleProof],
    batch: &mut OrchardBatchVerifier,
) -> Result<Vec<WriteIntent>> {
    log_info("🌳 Verifying Orchard bundle");

    let decoded = decode_bundle(bundle_bytes)?;
    let (anchor, nullifiers, commitments, value_balance) = extract_bundle_fields(&decoded);

    log_info(&format!(
        "Bundle: anchor={:?}, nullifiers={}, commitments={}, value_balance={}",
        &anchor[..8], nullifiers.len(), commitments.len(), value_balance
    ));

    // Step 2: Verify anchor (must match current commitment_root)
    let commitment_root = fetch_state_root(service_id)?;
    if anchor != commitment_root {
        return Err(OrchardError::AnchorMismatch {
            expected: commitment_root,
            got: anchor,
        });
    }

    // Step 3: Verify nullifier absence
    if nullifiers.len() != nullifier_absence_proofs.len() {
        return Err(OrchardError::ParseError(
            format!("Nullifier count mismatch: {} actions vs {} proofs",
                nullifiers.len(), nullifier_absence_proofs.len())
        ));
    }

    for (nullifier, proof) in nullifiers.iter().zip(nullifier_absence_proofs.iter()) {
        // Verify proof shows leaf == 0 (nullifier absent)
        if proof.leaf != [0u8; 32] {
            return Err(OrchardError::NullifierAlreadySpent { nullifier: *nullifier });
        }

        if !proof.verify() {
            return Err(OrchardError::InvalidProof);
        }
    }

    log_info("✅ All nullifiers verified absent");

    // Step 4: Add bundle to batch verifier
    batch.add_decoded_bundle(&decoded)?;

    // Step 5: Verify zero-sum
    if value_balance != 0 {
        return Err(OrchardError::ParseError(
            format!("Value balance must be zero, got {}", value_balance)
        ));
    }

    log_info("✅ Zero-sum verified");

    // Step 6: Generate WriteIntents
    let mut intents = Vec::new();

    // Mark nullifiers as spent
    for nullifier in &nullifiers {
        let key = crate::state::format_nullifier_key(nullifier);
        let object_id = compute_storage_opaque_key(service_id, key.as_bytes());

        let mut data = Vec::new();
        data.extend_from_slice(&(key.len() as u16).to_le_bytes());
        data.extend_from_slice(key.as_bytes());
        data.extend_from_slice(&[1u8; 32]); // Mark as spent

        intents.push(WriteIntent {
            effect: utils::effects::WriteEffectEntry {
                object_id,
                ref_info: utils::effects::ObjectRef {
                    work_package_hash: refine_context.anchor,
                    index_start: 0,
                    payload_length: data.len() as u32,
                object_kind: ObjectKind::Nullifier as u8,
                },
                payload: data,
                tx_index: 0,
            },
        });
    }

    // Update commitment tree (append new commitments)
    let commitment_size = fetch_commitment_size(service_id)?;
    const TREE_CAPACITY: u64 = 1u64 << 32;
    let requested_additions = commitments.len() as u64;
    if commitment_size.saturating_add(requested_additions) > TREE_CAPACITY {
        return Err(OrchardError::TreeCapacityExceeded {
            current_size: commitment_size,
            requested_additions,
            capacity: TREE_CAPACITY,
        });
    }
    let mut existing_commitments = collect_commitments(service_id, commitment_size)?;
    let base_root = merkle_root_from_leaves(&existing_commitments)?;
    if base_root != commitment_root {
        return Err(OrchardError::AnchorMismatch {
            expected: commitment_root,
            got: base_root,
        });
    }
    let base_len = existing_commitments.len();
    existing_commitments.extend_from_slice(&commitments);
    let new_root = merkle_root_from_leaves(&existing_commitments)?;
    let new_size = base_len as u64 + commitments.len() as u64;

    let mut root_data = Vec::new();
    root_data.extend_from_slice(&(b"commitment_root".len() as u16).to_le_bytes());
    root_data.extend_from_slice(b"commitment_root");
    root_data.extend_from_slice(&new_root);

    let root_object_id = compute_storage_opaque_key(service_id, b"commitment_root");
    intents.push(WriteIntent {
        effect: utils::effects::WriteEffectEntry {
            object_id: root_object_id,
            ref_info: utils::effects::ObjectRef {
                work_package_hash: refine_context.anchor,
                index_start: 0,
                payload_length: root_data.len() as u32,
                object_kind: ObjectKind::StateWrite as u8,
            },
            payload: root_data,
            tx_index: 0,
        },
    });

    // Update commitment_size
    let mut size_data = Vec::new();
    size_data.extend_from_slice(&(b"commitment_size".len() as u16).to_le_bytes());
    size_data.extend_from_slice(b"commitment_size");
    size_data.extend_from_slice(&new_size.to_le_bytes());
    let size_object_id = compute_storage_opaque_key(service_id, b"commitment_size");
    intents.push(WriteIntent {
        effect: utils::effects::WriteEffectEntry {
            object_id: size_object_id,
            ref_info: utils::effects::ObjectRef {
                work_package_hash: refine_context.anchor,
                index_start: 0,
                payload_length: size_data.len() as u32,
                object_kind: ObjectKind::StateWrite as u8,
            },
            payload: size_data,
            tx_index: 0,
        },
    });

    // Write commitments into state
    for (idx, commitment) in commitments.iter().enumerate() {
        let commitment_index = commitment_size + idx as u64;
        let key = format!("commitment_{}", commitment_index);
        let object_id = compute_storage_opaque_key(service_id, key.as_bytes());

        let mut data = Vec::new();
        data.extend_from_slice(&(key.len() as u16).to_le_bytes());
        data.extend_from_slice(key.as_bytes());
        data.extend_from_slice(commitment);

        intents.push(WriteIntent {
            effect: utils::effects::WriteEffectEntry {
                object_id,
                ref_info: utils::effects::ObjectRef {
                    work_package_hash: refine_context.anchor,
                    index_start: 0,
                    payload_length: data.len() as u32,
                    object_kind: ObjectKind::Commitment as u8,
                },
                payload: data,
                tx_index: 0,
            },
        });
    }

    log_info(&format!("✅ Generated {} write intents", intents.len()));

    Ok(intents)
}

fn extract_bundle_fields(decoded: &DecodedBundle) -> (
    [u8; 32],
    Vec<[u8; 32]>,
    Vec<[u8; 32]>,
    i64,
) {
    let nullifiers = decoded.actions.iter().map(|a| a.nullifier).collect();
    let commitments = decoded.actions.iter().map(|a| a.cmx).collect();
    (decoded.anchor, nullifiers, commitments, decoded.value_balance)
}

/// Compute JAM storage opaque key
fn compute_storage_opaque_key(service_id: u32, raw_key: &[u8]) -> [u8; 32] {
    use utils::hash_functions::blake2b_hash;

    // Step 1: Compute_storageKey_internal: k -> E4(2^32-1)++k
    let mut as_internal_key = Vec::with_capacity(4 + raw_key.len());
    let prefix = u32::MAX;
    as_internal_key.extend_from_slice(&prefix.to_le_bytes());
    as_internal_key.extend_from_slice(raw_key);

    // Step 2: Hash the internal key
    let hash = blake2b_hash(&as_internal_key);

    // Step 3: Interleave service_id with hash
    let n = service_id.to_le_bytes();
    let mut state_key = [0u8; 32];
    for i in 0..4 {
        state_key[2 * i] = n[i];
        state_key[2 * i + 1] = hash[i];
    }
    state_key[8..31].copy_from_slice(&hash[4..27]);
    // Last byte stays 0

    state_key
}

fn fetch_state_value(service_id: u32, key: &[u8]) -> Result<Vec<u8>> {
    use alloc::vec;
    let mut buffer = vec![0u8; 128];

    let ko = key.as_ptr() as u64;
    let kz = key.len() as u64;
    let bo = buffer.as_mut_ptr() as u64;
    let l = buffer.len() as u64;

    let result = unsafe {
        utils::host_functions::read(
            u64::MAX,
            ko,
            kz,
            bo,
            0,
            l,
        )
    };

    if result == u64::MAX {
        log_error(&format!("State read missing for service {} key {:?}", service_id, key));
        return Err(OrchardError::StateError("Missing state value".into()));
    }

    buffer.truncate(result as usize);
    log_info(&format!(
        "State read: service={}, key={:?}, value_len={}",
        service_id,
        key,
        buffer.len()
    ));
    Ok(buffer)
}

fn fetch_state_root(service_id: u32) -> Result<[u8; 32]> {
    let value = fetch_state_value(service_id, b"commitment_root")?;
    to_array32(&value)
}

fn fetch_commitment_size(service_id: u32) -> Result<u64> {
    let value = fetch_state_value(service_id, b"commitment_size")?;
    if value.len() < 8 {
        return Err(OrchardError::StateError("commitment_size too short".into()));
    }
    let mut bytes = [0u8; 8];
    bytes.copy_from_slice(&value[..8]);
    Ok(u64::from_le_bytes(bytes))
}

fn collect_commitments(service_id: u32, size: u64) -> Result<Vec<[u8; 32]>> {
    let mut out = Vec::with_capacity(size as usize);
    for idx in 0..size {
        let key = format!("commitment_{}", idx);
        let value = fetch_state_value(service_id, key.as_bytes())?;
        out.push(to_array32(&value)?);
    }
    Ok(out)
}

fn to_array32(bytes: &[u8]) -> Result<[u8; 32]> {
    if bytes.len() < 32 {
        return Err(OrchardError::ParseError("buffer too short for 32 bytes".into()));
    }
    let mut out = [0u8; 32];
    out.copy_from_slice(&bytes[..32]);
    Ok(out)
}

/// Serialize execution effects for accumulate
pub fn serialize_effects(effects: &[WriteIntent]) -> Result<Vec<u8>> {
    let mut output = Vec::new();
    output.extend_from_slice(&(effects.len() as u32).to_le_bytes());

    for (idx, effect) in effects.iter().enumerate() {
        log_info(&format!(
            "  Write intent {}: object_id={:?}, kind={}, data_len={}",
            idx, &effect.effect.object_id[..8],
            effect.effect.ref_info.object_kind,
            effect.effect.payload.len()
        ));
        output.extend_from_slice(&effect.effect.object_id);
        output.push(effect.effect.ref_info.object_kind);
        output.extend_from_slice(&(effect.effect.payload.len() as u32).to_le_bytes());
        output.extend_from_slice(&effect.effect.payload);
    }

    Ok(output)
}
