/// Railgun refine implementation (RAILGUN.md lines 171-379)
///
/// Stateless verification of ZK proofs and state transitions.
/// Outputs write intents for accumulate to apply.

use alloc::{vec, vec::Vec, string::ToString, format};
use crate::errors::{RailgunError, Result};
use crate::state::{RailgunExtrinsic, WriteIntent};
use crate::crypto::{verify_groth16, merkle_append};
use utils::functions::{log_info, log_error, RefineContext};

/// Process all extrinsics and generate write intents
///
/// RAILGUN.md lines 263-378
pub fn process_extrinsics(
    service_id: u32,
    refine_context: &RefineContext,
    extrinsics: &[Vec<u8>],
) -> Result<Vec<WriteIntent>> {
    let mut write_effects = Vec::new();

    // Calculate current epoch from timeslot
    const TIMESLOTS_PER_EPOCH: u64 = 600;
    let current_epoch = refine_context.timeslot / TIMESLOTS_PER_EPOCH;

    // Fetch pre-state witnesses (once per work item)
    log_info("Fetching state witnesses...");

    // Fetch commitment_root
    let (commitment_root, _proof) = fetch_state_value(service_id, b"commitment_root")?;

    // Fetch commitment_size
    let (commitment_size_bytes, _proof) = fetch_state_value(service_id, b"commitment_size")?;
    let mut commitment_size = u64::from_le_bytes(
        commitment_size_bytes[..8].try_into()
            .map_err(|_| RailgunError::ParseError("Invalid commitment_size".to_string()))?
    );

    // Fetch fee tally for current epoch
    let fee_tally_key = format!("fee_tally_{}", current_epoch);
    let (fee_tally_bytes, _proof) = fetch_state_value(service_id, fee_tally_key.as_bytes())?;
    let fee_tally_epoch = u128::from_le_bytes(
        fee_tally_bytes[..16].try_into()
            .map_err(|_| RailgunError::ParseError("Invalid fee_tally".to_string()))?
    );
    let mut fee_tally_accum = fee_tally_epoch;

    // Fetch consensus parameters
    let (min_fee_bytes, _proof) = fetch_state_value(service_id, b"min_fee")?;
    let min_fee = u64::from_le_bytes(
        min_fee_bytes[..8].try_into()
            .map_err(|_| RailgunError::ParseError("Invalid min_fee".to_string()))?
    );

    let (gas_min_bytes, _proof) = fetch_state_value(service_id, b"gas_min")?;
    let gas_min = u64::from_le_bytes(
        gas_min_bytes[..8].try_into()
            .map_err(|_| RailgunError::ParseError("Invalid gas_min".to_string()))?
    );

    let (gas_max_bytes, _proof) = fetch_state_value(service_id, b"gas_max")?;
    let gas_max = u64::from_le_bytes(
        gas_max_bytes[..8].try_into()
            .map_err(|_| RailgunError::ParseError("Invalid gas_max".to_string()))?
    );

    log_info(&format!(
        "State: root={:?}, size={}, min_fee={}, gas=[{}, {}]",
        &commitment_root[..8], commitment_size, min_fee, gas_min, gas_max
    ));

    // Track seen nullifiers within this work item
    let mut seen_nullifiers = Vec::new();
    let mut local_commitment_root = commitment_root;

    // Process each extrinsic
    for (idx, ext_bytes) in extrinsics.iter().enumerate() {
        log_info(&format!("Processing extrinsic {}", idx));

        // Decode extrinsic (simplified - needs proper SCALE decoding)
        let ext = decode_extrinsic(ext_bytes)?;

        match ext {
            RailgunExtrinsic::SubmitPrivate {
                proof,
                anchor_root,
                anchor_size,
                next_root,
                input_nullifiers,
                output_commitments,
                fee,
                encrypted_payload,
            } => {
                // RAILGUN.md line 269-274: Enforce encrypted_payload is zero-length in MVP
                if !encrypted_payload.is_empty() {
                    return Err(RailgunError::EncryptedPayloadNotAllowed {
                        length: encrypted_payload.len(),
                    });
                }

                // RAILGUN.md line 276-282: Enforce minimum fee
                if fee < min_fee {
                    return Err(RailgunError::FeeUnderflow {
                        provided: fee,
                        minimum: min_fee,
                    });
                }

                // RAILGUN.md line 284-291: Verify ZK proof (stub for now)
                let public_inputs = build_spend_public_inputs(
                    &anchor_root,
                    anchor_size,
                    &next_root,
                    &input_nullifiers,
                    &output_commitments,
                    fee,
                );

                if !verify_groth16(&proof, &public_inputs, &[]) {
                    log_error("ZK proof verification failed");
                    return Err(RailgunError::InvalidProof);
                }

                // RAILGUN.md line 293-296: Check anchor matches pre-state
                if anchor_root != local_commitment_root {
                    return Err(RailgunError::AnchorMismatch {
                        expected: local_commitment_root,
                        got: anchor_root,
                    });
                }

                if anchor_size != commitment_size {
                    return Err(RailgunError::AnchorMismatch {
                        expected: commitment_size.to_le_bytes().try_into().unwrap(),
                        got: anchor_size.to_le_bytes().try_into().unwrap(),
                    });
                }

                // RAILGUN.md line 291-300: Check tree capacity
                let tree_capacity: u64 = 1u64 << 32;
                if commitment_size
                    .checked_add(output_commitments.len() as u64)
                    .map_or(true, |new_size| new_size > tree_capacity)
                {
                    return Err(RailgunError::TreeCapacityExceeded {
                        current_size: commitment_size,
                        requested_additions: output_commitments.len() as u64,
                        capacity: tree_capacity,
                    });
                }

                // RAILGUN.md line 303-309: Verify deterministic next_root
                let next_root_expected = merkle_append(
                    &local_commitment_root,
                    commitment_size,
                    &output_commitments,
                );
                if next_root != next_root_expected {
                    return Err(RailgunError::ParseError(
                        "next_root mismatch".to_string()
                    ));
                }

                // RAILGUN.md line 311-335: Check nullifiers are fresh
                for nf in &input_nullifiers {
                    // Prevent intra-work-item reuse
                    if seen_nullifiers.contains(nf) {
                        return Err(RailgunError::NullifierReusedInWorkItem { nullifier: *nf });
                    }
                    seen_nullifiers.push(*nf);

                    // Fetch nullifier from state (stub - needs actual Verkle proof)
                    // TODO: verify_verkle_proof would go here
                }

                // RAILGUN.md line 337-366: Emit write intents
                write_effects.push(WriteIntent {
                    object_id: hash_key(b"commitment_root"),
                    data: next_root_expected.to_vec(),
                });

                write_effects.push(WriteIntent {
                    object_id: hash_key(b"commitment_size"),
                    data: (commitment_size + output_commitments.len() as u64)
                        .to_le_bytes()
                        .to_vec(),
                });

                for nf in &input_nullifiers {
                    write_effects.push(WriteIntent {
                        object_id: hash_key(&[b"nullifier_", &nf[..]].concat()),
                        data: vec![1u8; 32], // Mark as spent
                    });
                }

                for (i, cm) in output_commitments.iter().enumerate() {
                    let key = format!("commitment_{}", commitment_size + i as u64);
                    write_effects.push(WriteIntent {
                        object_id: hash_key(key.as_bytes()),
                        data: cm.to_vec(),
                    });
                }

                fee_tally_accum += fee as u128;

                // Update local tracking
                local_commitment_root = next_root_expected;
                commitment_size += output_commitments.len() as u64;
            }

            RailgunExtrinsic::WithdrawPublic { .. } => {
                // TODO: Implement withdraw processing (RAILGUN.md lines 456-585)
                log_info("Withdraw extrinsic (not yet implemented)");
            }

            RailgunExtrinsic::DepositPublic { .. } => {
                // Deposits are processed in accumulate, not refine
                log_error("DepositPublic should not appear in refine");
                return Err(RailgunError::ParseError(
                    "Deposit in refine".to_string()
                ));
            }
        }
    }

    // RAILGUN.md line 369-375: Emit fee tally DELTA write
    let fee_delta = fee_tally_accum - fee_tally_epoch;
    let delta_key = format!("fee_tally_delta_{}_{}", current_epoch, refine_context.timeslot);
    write_effects.push(WriteIntent {
        object_id: hash_key(delta_key.as_bytes()),
        data: fee_delta.to_le_bytes().to_vec(),
    });

    log_info(&format!("Generated {} write intents", write_effects.len()));
    Ok(write_effects)
}

/// Serialize execution effects for accumulate
pub fn serialize_effects(effects: &[WriteIntent]) -> Result<Vec<u8>> {
    // Simple serialization: [num_writes:4][object_id:32][data_len:4][data:N]...
    let mut output = Vec::new();
    output.extend_from_slice(&(effects.len() as u32).to_le_bytes());

    for effect in effects {
        output.extend_from_slice(&effect.object_id);
        output.extend_from_slice(&(effect.data.len() as u32).to_le_bytes());
        output.extend_from_slice(&effect.data);
    }

    Ok(output)
}

// Helper functions

fn fetch_state_value(_service_id: u32, _key: &[u8]) -> Result<(Vec<u8>, Vec<u8>)> {
    // TODO: Implement using actual host call
    // let object_id = hash_key(key);
    // fetch_object(service_id, &object_id)
    //     .map_err(|e| RailgunError::StateError(format!("fetch_object failed: {:?}", e)))
    Ok((vec![0u8; 32], vec![]))
}

fn hash_key(key: &[u8]) -> [u8; 32] {
    // TODO: Proper object_id = C(service_id, hash(key))
    let mut result = [0u8; 32];
    let len = core::cmp::min(key.len(), 32);
    result[..len].copy_from_slice(&key[..len]);
    result
}

fn decode_extrinsic(_bytes: &[u8]) -> Result<RailgunExtrinsic> {
    // TODO: Proper SCALE decoding
    // For now, return stub
    Err(RailgunError::ParseError("decode_extrinsic not implemented".to_string()))
}

fn build_spend_public_inputs(
    anchor_root: &[u8; 32],
    anchor_size: u64,
    next_root: &[u8; 32],
    nullifiers: &[[u8; 32]],
    commitments: &[[u8; 32]],
    fee: u64,
) -> Vec<[u8; 32]> {
    let mut inputs = Vec::new();
    inputs.push(*anchor_root);

    let mut size_bytes = [0u8; 32];
    size_bytes[..8].copy_from_slice(&anchor_size.to_le_bytes());
    inputs.push(size_bytes);

    inputs.push(*next_root);
    inputs.extend_from_slice(nullifiers);
    inputs.extend_from_slice(commitments);

    let mut fee_bytes = [0u8; 32];
    fee_bytes[..8].copy_from_slice(&fee.to_le_bytes());
    inputs.push(fee_bytes);

    inputs
}
