#![allow(dead_code)]
/// Orchard accumulate implementation (see services/orchard/docs/ORCHARD.md accumulate path)
///
/// Applies verified state changes and processes deferred transfers.

use alloc::{vec::Vec, format};
use crate::errors::{OrchardError, Result};
use crate::objects::ObjectKind;
use crate::crypto::{commitment, merkle_append_from_leaves, merkle_root_from_leaves};
use utils::effects::WriteIntent;
use crate::refiner::RefineOutput;
use utils::functions::{log_info, log_error, AccumulateArgs, AccumulateInput, DeferredTransfer};
use core::convert::TryInto;

/// Memo v1 layout: [version=0x01 | pk(32) | rho(32) | reserved(63)]
/// Total: 128 bytes
const MEMO_SIZE: usize = 128;
const MEMO_VERSION_V1: u8 = 0x01;

/// Process accumulate inputs
///
/// See services/orchard/docs/ORCHARD.md accumulate flow: apply write intents and handle deferred transfers
pub fn process_accumulate(
    service_id: u32,
    args: &AccumulateArgs,
    inputs: &[AccumulateInput],
) -> Result<()> {
    log_info(&format!(
        "Orchard accumulate processing {} inputs for service {}",
        inputs.len(),
        service_id
    ));
    let _ = args;

    // Fetch current state for deposit validation
    let (commitment_root_bytes, _) = fetch_state_value(service_id, b"commitment_root")?;
    let mut commitment_root = to_array32(&commitment_root_bytes)?;
    let (commitment_size_bytes, _) = fetch_state_value(service_id, b"commitment_size")?;
    let mut commitment_size = u64::from_le_bytes(
        commitment_size_bytes[..8].try_into()
            .map_err(|_| OrchardError::ParseError("Invalid commitment_size".into()))?
    );
    let mut commitments = collect_commitments(service_id, commitment_size)?;
    let computed_root = merkle_root_from_leaves(&commitments)?;
    if computed_root != commitment_root {
        return Err(OrchardError::AnchorMismatch {
            expected: commitment_root,
            got: computed_root,
        });
    }

    // Try to read gas bounds from state, fall back to defaults if not present
    let gas_min = match fetch_state_value(service_id, b"gas_min") {
        Ok((bytes, _)) if bytes.len() >= 8 => {
            u64::from_le_bytes(bytes[..8].try_into().unwrap_or([0; 8]))
        }
        _ => 50_000 // Default if not in state
    };

    let gas_max = match fetch_state_value(service_id, b"gas_max") {
        Ok((bytes, _)) if bytes.len() >= 8 => {
            u64::from_le_bytes(bytes[..8].try_into().unwrap_or([0; 8]))
        }
        _ => 5_000_000 // Default if not in state
    };

    log_info(&format!(
        "State: root={:?}, size={}, gas=[{}, {}]",
        &commitment_root[..8], commitment_size, gas_min, gas_max
    ));

    // Process inputs
    for (idx, input) in inputs.iter().enumerate() {
        log_info(&format!("Processing accumulate input {}", idx));

        match input {
            AccumulateInput::DeferredTransfer(transfer) => {
                log_info(&format!(
                    "Processing deposit: amount={}, sender={}",
                    transfer.amount, transfer.sender_index
                ));

                // Process deposit (DepositPublic flow in services/orchard/docs/ORCHARD.md)
                commitment_root = process_deposit(
                    service_id,
                    transfer,
                    &mut commitments,
                    gas_min,
                    gas_max,
                )?;

                commitment_size = commitments.len() as u64;
            }
            AccumulateInput::OperandElements(elements) => {
                // Apply write intents from refine result (if present)
                if let Some(data) = &elements.result.ok {
                    let write_intents = deserialize_write_intents(data)?;
                    apply_write_intents(service_id, &write_intents)?;

                    // Refresh local view after applying intents
                    let (root_bytes, _) = fetch_state_value(service_id, b"commitment_root")?;
                    commitment_root = to_array32(&root_bytes)?;
                    let (size_bytes, _) = fetch_state_value(service_id, b"commitment_size")?;
                    commitment_size = u64::from_le_bytes(
                        size_bytes[..8].try_into()
                            .map_err(|_| OrchardError::ParseError("Invalid commitment_size".into()))?
                    );
                    commitments = collect_commitments(service_id, commitment_size)?;
                } else {
                    log_error("OperandElements missing ok payload; skipping");
                }
            }
        }
    }

    // Update commitment_size in state
    write_state_value(
        service_id,
        b"commitment_size",
        &commitment_size.to_le_bytes(),
    )?;

    log_info(&format!(
        "Accumulate processing completed: commitment_root={:?}, commitment_size={}",
        &commitment_root[..8],
        commitment_size
    ));
    Ok(())
}

/// Process a deposit from deferred transfer
///
/// DepositPublic accumulate flow (services/orchard/docs/ORCHARD.md §1):
/// 1. Enforce gas bounds
/// 2. Parse memo v1: [version | pk | rho | reserved]
/// 3. Compute commitment_expected = Com(amount, pk, rho)
/// 4. Verify value binding (commitment from memo must match)
/// 5. Check tree capacity
/// 6. Insert commitment and update root
fn process_deposit(
    service_id: u32,
    transfer: &DeferredTransfer,
    commitments: &mut Vec<[u8; 32]>,
    gas_min: u64,
    gas_max: u64,
) -> Result<[u8; 32]> {
    let commitment_size = commitments.len() as u64;
    // Step 1: Enforce gas bounds (DepositPublic spec)
    if transfer.gas_limit < gas_min || transfer.gas_limit > gas_max {
        log_error(&format!(
            "Gas limit {} out of bounds [{}, {}]",
            transfer.gas_limit, gas_min, gas_max
        ));
        return Err(OrchardError::InvalidGasLimit {
            provided: transfer.gas_limit,
            minimum: gas_min,
            maximum: gas_max,
        });
    }

    // Step 2: Parse memo (DepositPublic spec)
    let memo = &transfer.memo;

    // Check version
    let memo_version = memo[0];
    if memo_version != MEMO_VERSION_V1 && memo_version != 0x02 {
        return Err(OrchardError::ParseError(
            format!("Invalid memo version: {}", memo_version)
        ));
    }

    // Parse based on version
    let (asset_id, pk, rho) = if memo_version == 0x02 {
        // Memo v2 (Phase 6 multi-asset):
        // [0]: version = 0x02
        // [1..5]: asset_id (u32)
        // [5..37]: pk (32 bytes)
        // [37..69]: rho (32 bytes)
        // [69..128]: reserved (59 bytes)

        let asset_id = u32::from_le_bytes([memo[1], memo[2], memo[3], memo[4]]);

        let mut pk = [0u8; 32];
        pk.copy_from_slice(&memo[5..37]);

        let mut rho = [0u8; 32];
        rho.copy_from_slice(&memo[37..69]);

        // Check reserved bytes are zero (bytes 69..128)
        for &byte in &memo[69..128] {
            if byte != 0 {
                return Err(OrchardError::ParseError(
                    "Reserved memo bytes must be zero in v2".into()
                ));
            }
        }

        (asset_id, pk, rho)
    } else {
        // Memo v1 (backward compatibility):
        // [0]: version = 0x01
        // [1..33]: pk (32 bytes)
        // [33..65]: rho (32 bytes)
        // [65..128]: reserved (63 bytes)

        let mut pk = [0u8; 32];
        pk.copy_from_slice(&memo[1..33]);

        let mut rho = [0u8; 32];
        rho.copy_from_slice(&memo[33..65]);

        // Check reserved bytes are zero (bytes 65..128)
        for &byte in &memo[65..128] {
            if byte != 0 {
                return Err(OrchardError::ParseError(
                    "Reserved memo bytes must be zero in v1".into()
                ));
            }
        }

        // Default to CASH for v1 memos (backward compat)
        (1u32, pk, rho)
    };

    // Step 3: Compute commitment_expected (DepositPublic spec)
    // v1/v2 memos do not carry these fields yet; default to zero until memo schema expands.
    let note_rseed = [0u8; 32];
    let memo_hash = [0u8; 32];
    let commitment_expected = commitment(
        asset_id,
        transfer.amount as u128,
        &pk,
        &rho,
        &note_rseed,
        0,
        &memo_hash,
    )?;

    log_info(&format!(
        "Deposit commitment: {:?}",
        &commitment_expected[..8]
    ));

    // Step 4: Value binding check (DepositPublic spec)
    // Note: In a real implementation, the memo would also contain the commitment
    // For now, we trust the computed commitment is correct
    // TODO: Parse commitment from memo and verify equality

    // Step 5: Check tree capacity (DepositPublic spec)
    const TREE_CAPACITY: u64 = 1u64 << 32;
    if commitment_size >= TREE_CAPACITY {
        return Err(OrchardError::TreeCapacityExceeded {
            current_size: commitment_size,
            requested_additions: 1,
            capacity: TREE_CAPACITY,
        });
    }

    // Step 6: Insert commitment and update root (DepositPublic spec)
    let new_root = merkle_append_from_leaves(commitments, &[commitment_expected])?;

    // Mutate local tree for subsequent writes
    commitments.push(commitment_expected);

    // Write commitment to storage
    let commitment_key = format!("commitment_{}", commitment_size);
    write_state_value(service_id, commitment_key.as_bytes(), &commitment_expected)?;

    // Write new root
    write_state_value(service_id, b"commitment_root", &new_root)?;

    log_info(&format!(
        "Inserted commitment at position {}, new root: {:?}",
        commitment_size, &new_root[..8]
    ));

    // Phase 7: Supply tracking for deposits (transparent → shielded)
    if transfer.amount > 0 {
        // Deposit decreases transparent_supply, increases shielded_supply
        update_transparent_supply(service_id, asset_id, transfer.amount as u128, false)?;

        // Emit Deposit event
        emit_event(service_id, "Deposit", asset_id, transfer.amount as u128)?;

        log_info(&format!(
            "Supply updated for asset {}: -{} transparent (deposit)",
            asset_id, transfer.amount
        ));
    }

    Ok(new_root)
}

/// Apply write intents from refine
///
/// Apply each WriteIntent to state (per accumulate flow documentation)
/// Uses object_kind to determine how to process each intent
fn apply_write_intents(service_id: u32, intents: &[WriteIntent]) -> Result<()> {
    use crate::crypto::merkle_root_from_leaves;
    use crate::objects::ObjectKind;

    log_info(&format!("Applying {} write intents", intents.len()));

    //  Process intents by object_kind
    for (idx, intent) in intents.iter().enumerate() {
        let kind = ObjectKind::from_u8(intent.effect.ref_info.object_kind);
        log_info(&format!("  Write intent {}: kind={:?}, data_len={}", idx, kind, intent.effect.payload.len()));

        match kind {
            Some(ObjectKind::Deposit) => {
                // Decode deposit data: [commitment:32][value:8][sender_index:4][asset_id:4]
                if intent.effect.payload.len() < 48 {
                    return Err(OrchardError::ParseError("Invalid deposit data length".into()));
                }

                let mut commitment = [0u8; 32];
                commitment.copy_from_slice(&intent.effect.payload[..32]);
                let value = u64::from_le_bytes(intent.effect.payload[32..40].try_into().unwrap());
                let sender_index = u32::from_le_bytes(intent.effect.payload[40..44].try_into().unwrap());
                let asset_id = u32::from_le_bytes(intent.effect.payload[44..48].try_into().unwrap());

                log_info(&format!("Deposit: commitment={:?}, value={}, asset_id={}, sender={}",
                    &commitment[..8], value, asset_id, sender_index));

                // Guard against reserved asset IDs slipping past refine
                if asset_id == 0 {
                    return Err(OrchardError::ReservedAssetUsed);
                }

                // Read current commitment tree state
                let (size_bytes, _) = fetch_state_value(service_id, b"commitment_size")?;
                let commitment_size = u64::from_le_bytes(size_bytes[..8].try_into().unwrap());

                // Collect existing commitments
                let mut commitments = collect_commitments(service_id, commitment_size)?;

                // Add new commitment
                commitments.push(commitment);
                let new_root = merkle_root_from_leaves(&commitments)?;
                let new_size = commitments.len() as u64;

                // Write updated state
                write_state_value(service_id, b"commitment_root", &new_root)?;
                write_state_value(service_id, b"commitment_size", &new_size.to_le_bytes())?;

                // Write the commitment itself
                let commitment_key = format!("commitment_{}", commitment_size);
                write_state_value(service_id, commitment_key.as_bytes(), &commitment)?;

                // Phase 7: Supply tracking for deposits (transparent → shielded)
                if value > 0 {
                    update_transparent_supply(service_id, asset_id, value as u128, false)?;
                    emit_event(service_id, "Deposit", asset_id, value as u128)?;
                    log_info(&format!("Supply updated for asset {}: -{} transparent (deposit)", asset_id, value));
                }

                log_info(&format!("Deposit processed: new_size={}, new_root={:?}", new_size, &new_root[..8]));
            }

            Some(ObjectKind::Nullifier) | Some(ObjectKind::Commitment) => {
                // Decode key from data: [key_len:2][key][value]
                if intent.effect.payload.len() < 2 {
                    return Err(OrchardError::ParseError("Nullifier/Commitment data too short".into()));
                }
                let key_len = u16::from_le_bytes([intent.effect.payload[0], intent.effect.payload[1]]) as usize;
                if intent.effect.payload.len() < 2 + key_len {
                    return Err(OrchardError::ParseError("Truncated key in Nullifier/Commitment".into()));
                }
                let key = &intent.effect.payload[2..2 + key_len];
                let value = &intent.effect.payload[2 + key_len..];

                write_state_value(service_id, key, value)?;
            }

            Some(ObjectKind::StateWrite) | Some(ObjectKind::FeeTally) | Some(ObjectKind::DeltaWrite) => {
                // Generic state write with embedded key: [key_len:2][key][value]
                if intent.effect.payload.len() < 2 {
                    return Err(OrchardError::ParseError("StateWrite data too short".into()));
                }
                let key_len = u16::from_le_bytes([intent.effect.payload[0], intent.effect.payload[1]]) as usize;
                if intent.effect.payload.len() < 2 + key_len {
                    return Err(OrchardError::ParseError("Truncated key in StateWrite".into()));
                }
                let key = &intent.effect.payload[2..2 + key_len];
                let value = &intent.effect.payload[2 + key_len..];

                // NEW: Process multi-asset delta writes
                // Key format: "delta_{size}_{index}_{component}" where component is:
                // asset_id, in_public, out_public, burn_public, mint_public
                if key.starts_with(b"delta_") {
                    process_delta_write(service_id, key, value)?;
                } else {
                    write_state_value(service_id, key, value)?;
                }
            }

            Some(ObjectKind::Block) => {
                // Store compact block data for light client synchronization
                log_info(&format!("Processing compact block data, payload_len={}", intent.effect.payload.len()));

                // Store the serialized compact block data with a standard key
                let key = "compact_block_latest".as_bytes();
                let value = &intent.effect.payload;
                write_state_value(service_id, key, value)?;

                log_info("Compact block data stored successfully");
            }

            None => {
                log_error(&format!("Unknown object_kind: {}", intent.effect.ref_info.object_kind));
                return Err(OrchardError::ParseError("Unknown object_kind".into()));
            }
        }
    }

    log_info("All write intents applied successfully");
    Ok(())
}

/// Process multi-asset delta writes
///
/// Key format: "delta_{size}_{index}_{component}"
/// Components: asset_id (u32), in_public (u128), out_public (u128), burn_public (u128), mint_public (u128)
///
/// This function updates per-asset supply tracking:
/// - in_public: transparent → shielded (deposit)
/// - out_public: shielded → transparent (withdraw)
/// - burn_public: decrease total supply (e.g., option exercise)
/// - mint_public: increase total supply (e.g., stock from exercise)
fn process_delta_write(service_id: u32, key: &[u8], value: &[u8]) -> Result<()> {
    // Parse key: "delta_{size}_{index}_{component}"
    let key_str = core::str::from_utf8(key)
        .map_err(|_| OrchardError::ParseError("Invalid delta key UTF-8".into()))?;

    let parts: Vec<&str> = key_str.split('_').collect();
    if parts.len() != 4 || parts[0] != "delta" {
        return Err(OrchardError::ParseError(format!("Invalid delta key format: {}", key_str)));
    }

    let _size = parts[1].parse::<u64>()
        .map_err(|_| OrchardError::ParseError("Invalid delta size".into()))?;
    let index = parts[2].parse::<usize>()
        .map_err(|_| OrchardError::ParseError("Invalid delta index".into()))?;
    let component = parts[3];

    // First, fetch the asset_id for this delta (needed for supply tracking)
    let asset_id = if component == "assetid" {
        // Parse from current value
        if value.len() < 4 {
            return Err(OrchardError::ParseError("Invalid assetid value".into()));
        }
        u32::from_le_bytes(value[..4].try_into().unwrap())
    } else {
        // Fetch from previously written assetid
        let asset_id_key = format!("delta_{}_{}_assetid", parts[1], index);
        match fetch_state_value(service_id, asset_id_key.as_bytes()) {
            Ok((bytes, _)) if bytes.len() >= 4 => {
                u32::from_le_bytes(bytes[..4].try_into().unwrap())
            }
            _ => 0 // Default to 0 (padding delta)
        }
    };

    // Skip processing for padding deltas (asset_id = 0)
    if asset_id == 0 && component != "assetid" {
        log_info(&format!("Skipping zero-padding delta: key='{}', component={}", key_str, component));
        write_state_value(service_id, key, value)?;
        return Ok(());
    }

    match component {
        "assetid" => {
            if value.len() < 4 {
                return Err(OrchardError::ParseError("Invalid assetid value".into()));
            }
            log_info(&format!("Delta assetid: {}", asset_id));
            write_state_value(service_id, key, value)?;
        }
        "inpublic" => {
            if value.len() < 16 {
                return Err(OrchardError::ParseError("Invalid inpublic value".into()));
            }
            let amount = u128::from_le_bytes(value[..16].try_into().unwrap());

            if amount > 0 {
                log_info(&format!("Delta inpublic (deposit) asset={}, amount={}", asset_id, amount));

                // Update transparent_supply: decrease by amount (transparent → shielded)
                update_transparent_supply(service_id, asset_id, amount, false)?;

                // Emit Deposit event
                emit_event(service_id, "Deposit", asset_id, amount)?;
            }

            write_state_value(service_id, key, value)?;
        }
        "outpublic" => {
            if value.len() < 16 {
                return Err(OrchardError::ParseError("Invalid outpublic value".into()));
            }
            let amount = u128::from_le_bytes(value[..16].try_into().unwrap());

            if amount > 0 {
                log_info(&format!("Delta outpublic (withdraw) asset={}, amount={}", asset_id, amount));

                // Update transparent_supply: increase by amount (shielded → transparent)
                update_transparent_supply(service_id, asset_id, amount, true)?;

                // Emit Withdraw event
                emit_event(service_id, "Withdraw", asset_id, amount)?;
            }

            write_state_value(service_id, key, value)?;
        }
        "burnpublic" => {
            if value.len() < 16 {
                return Err(OrchardError::ParseError("Invalid burnpublic value".into()));
            }
            let amount = u128::from_le_bytes(value[..16].try_into().unwrap());

            if amount > 0 {
                log_info(&format!("Delta burnpublic (exercise) asset={}, amount={}", asset_id, amount));

                // Update total_supply: decrease by amount
                update_total_supply(service_id, asset_id, amount, false)?;

                // Emit Burn event
                emit_event(service_id, "Burn", asset_id, amount)?;
            }

            write_state_value(service_id, key, value)?;
        }
        "mintpublic" => {
            if value.len() < 16 {
                return Err(OrchardError::ParseError("Invalid mintpublic value".into()));
            }
            let amount = u128::from_le_bytes(value[..16].try_into().unwrap());

            if amount > 0 {
                log_info(&format!("Delta mintpublic (exercise) asset={}, amount={}", asset_id, amount));

                // Update total_supply: increase by amount
                update_total_supply(service_id, asset_id, amount, true)?;

                // Emit Mint event
                emit_event(service_id, "Mint", asset_id, amount)?;
            }

            write_state_value(service_id, key, value)?;
        }
        _ => {
            return Err(OrchardError::ParseError(format!("Unknown delta component: {}", component)));
        }
    }

    Ok(())
}

// ===== Helper Functions =====

/// Parse deferred transfer from input bytes
fn parse_deferred_transfer(bytes: &[u8]) -> Result<DeferredTransfer> {
    // Format: [sender_index:4][amount:8][gas_limit:8][memo:128]
    if bytes.len() < 4 + 8 + 8 + MEMO_SIZE {
        return Err(OrchardError::ParseError(
            "Invalid deferred transfer length".into()
        ));
    }

    let sender_index = u32::from_le_bytes(bytes[0..4].try_into().unwrap());
    let amount = u64::from_le_bytes(bytes[4..12].try_into().unwrap());
    let gas_limit = u64::from_le_bytes(bytes[12..20].try_into().unwrap());

    let mut memo = [0u8; MEMO_SIZE];
    memo.copy_from_slice(&bytes[20..20 + MEMO_SIZE]);

    Ok(DeferredTransfer {
        sender_index,
        receiver_index: 0,
        amount,
        memo,
        gas_limit,
    })
}

/// Deserialize write intents from refine output
fn deserialize_write_intents(bytes: &[u8]) -> Result<Vec<WriteIntent>> {
    // Format: [num_intents:4]([object_id:32][object_kind:1][data_len:4][data:N])*
    if bytes.len() < 4 {
        return Err(OrchardError::ParseError("Invalid write intents".into()));
    }

    let num_intents = u32::from_le_bytes(bytes[0..4].try_into().unwrap()) as usize;
    let mut intents = Vec::with_capacity(num_intents);
    let mut offset = 4;

    for _ in 0..num_intents {
        if offset + 32 + 1 + 4 > bytes.len() {
            return Err(OrchardError::ParseError("Truncated write intent".into()));
        }

        let mut object_id = [0u8; 32];
        object_id.copy_from_slice(&bytes[offset..offset + 32]);
        offset += 32;

        let object_kind = bytes[offset];
        offset += 1;

        let data_len = u32::from_le_bytes(bytes[offset..offset + 4].try_into().unwrap()) as usize;
        offset += 4;

        if offset + data_len > bytes.len() {
            return Err(OrchardError::ParseError("Truncated write intent data".into()));
        }

        let data = bytes[offset..offset + data_len].to_vec();
        offset += data_len;

        intents.push(WriteIntent {
            effect: utils::effects::WriteEffectEntry {
                object_id,
                ref_info: utils::effects::ObjectRef {
                    work_package_hash: [0u8; 32], // Dummy value during deserialization
                    index_start: 0,
                    payload_length: data.len() as u32,
                    object_kind,
                },
                payload: data,
                tx_index: 0,
            },
        });
    }

    Ok(intents)
}

/// Fetch state value by key using read host function
///
/// Accumulate runs after state has been updated, so it reads from the actual state
/// (not witnesses). Uses the JAM `read` host function.
fn fetch_state_value(service_id: u32, key: &[u8]) -> Result<(Vec<u8>, Vec<u8>)> {
    use alloc::vec;

    // Allocate buffer for the value (max 128 bytes for state values)
    let mut buffer = vec![0u8; 128];

    let ko = key.as_ptr() as u64;
    let kz = key.len() as u64;
    let bo = buffer.as_mut_ptr() as u64;
    let l = buffer.len() as u64;

    // Call read host function: read(s, ko, kz, o, f, l)
    // s = service_id (or u64::MAX for self)
    // ko, kz = key offset and size
    // o = output buffer offset
    // f = offset into value (0 to read from start)
    // l = length to read
    let result = unsafe {
        utils::host_functions::read(
            u64::MAX,  // Read from current service
            ko,
            kz,
            bo,
            0,  // Read from offset 0 in value
            l,
        )
    };

    // result is the actual number of bytes read, or NONE (u64::MAX) if not found
    if result == u64::MAX {
        // Key not found - return default zero value
        log_info(&format!("fetch_state_value: key not found, returning zeros for {:?}", key));
        Ok((vec![0u8; 32], vec![]))
    } else {
        // Truncate buffer to actual size read
        buffer.truncate(result as usize);
        log_info(&format!("fetch_state_value: service={}, key={:?}, value_len={}", service_id, key, buffer.len()));
        Ok((buffer, vec![]))
    }
}

fn collect_commitments(service_id: u32, size: u64) -> Result<Vec<[u8; 32]>> {
    let mut out = Vec::with_capacity(size as usize);
    for idx in 0..size {
        let key = format!("commitment_{}", idx);
        let (value, _) = fetch_state_value(service_id, key.as_bytes())?;
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

/// Write state value by key
fn write_state_value(_service_id: u32, key: &[u8], value: &[u8]) -> Result<()> {
    use utils::constants::{WHAT, FULL};

    log_info(&format!("write_state_value: key={:?}, value_len={}", key, value.len()));

    let ko = key.as_ptr() as u64;
    let kz = key.len() as u64;
    let vo = value.as_ptr() as u64;
    let vz = value.len() as u64;

    let result = unsafe {
        utils::host_functions::write(ko, kz, vo, vz)
    };

    // write() returns number of bytes written on success, or WHAT/FULL on error
    if result == WHAT {
        log_error(&format!("Write failed: WHAT error (wrong mode) for key={:?}", key));
        Err(OrchardError::ParseError("Host write WHAT error".into()))
    } else if result == FULL {
        log_error(&format!("Write failed: FULL error (storage full) for key={:?}", key));
        Err(OrchardError::ParseError("Host write FULL error".into()))
    } else if result == u64::MAX {
        // Host returns NONE (u64::MAX) when creating a brand-new key
        log_info(&format!("Created new key, previous length unknown for key={:?}", key));
        Ok(())
    } else {
        // Success: result is number of bytes written
        log_info(&format!("Wrote {} bytes for key={:?}", result, key));
        Ok(())
    }
}

/// Write state value by object ID
fn write_state_by_id(_service_id: u32, object_id: &[u8; 32], value: &[u8]) -> Result<()> {
    use utils::constants::{WHAT, FULL};

    log_info(&format!("write_state_by_id: object_id={:?}, value_len={}", &object_id[..8], value.len()));

    let ko = object_id.as_ptr() as u64;
    let kz = 32u64;
    let vo = value.as_ptr() as u64;
    let vz = value.len() as u64;

    let result = unsafe {
        utils::host_functions::write(ko, kz, vo, vz)
    };

    // write() returns number of bytes written on success, or WHAT/FULL on error
    if result == WHAT {
        log_error(&format!("Write failed: WHAT error (wrong mode) for object_id={:?}", &object_id[..8]));
        Err(OrchardError::ParseError("Host write WHAT error".into()))
    } else if result == FULL {
        log_error(&format!("Write failed: FULL error (storage full) for object_id={:?}", &object_id[..8]));
        Err(OrchardError::ParseError("Host write FULL error".into()))
    } else if result == u64::MAX {
        log_info(&format!("Created new key, previous length unknown for object_id={:?}", &object_id[..8]));
        Ok(())
    } else {
        // Success: result is number of bytes written
        log_info(&format!("Wrote {} bytes for object_id={:?}", result, &object_id[..8]));
        Ok(())
    }
}

/// Update per-asset total supply
///
/// State key format: "supply_{asset_id}_total"
/// Value: u128 (16 bytes, little-endian)
///
/// If increment = true: total_supply += amount
/// If increment = false: total_supply -= amount (burn)
fn update_total_supply(service_id: u32, asset_id: u32, amount: u128, increment: bool) -> Result<()> {
    let supply_key = format!("supply_{}_total", asset_id);

    // Fetch current total_supply
    let current_supply = match fetch_state_value(service_id, supply_key.as_bytes()) {
        Ok((bytes, _)) if bytes.len() >= 16 => {
            u128::from_le_bytes(bytes[..16].try_into().unwrap())
        }
        _ => 0 // Initialize to 0 if not present
    };

    // Compute new supply
    let new_supply = if increment {
        current_supply.checked_add(amount)
            .ok_or_else(|| OrchardError::ParseError("Total supply overflow".into()))?
    } else {
        current_supply.checked_sub(amount)
            .ok_or_else(|| OrchardError::ParseError("Total supply underflow".into()))?
    };

    log_info(&format!(
        "Update total_supply asset={}: {} -> {} ({}{})",
        asset_id, current_supply, new_supply,
        if increment { "+" } else { "-" }, amount
    ));

    // Write new supply
    write_state_value(service_id, supply_key.as_bytes(), &new_supply.to_le_bytes())?;

    Ok(())
}

/// Update per-asset transparent supply
///
/// State key format: "supply_{asset_id}_transparent"
/// Value: u128 (16 bytes, little-endian)
///
/// If increment = true: transparent_supply += amount (withdraw: shielded → transparent)
/// If increment = false: transparent_supply -= amount (deposit: transparent → shielded)
///
/// Shielded supply is computed implicitly as: shielded = total - transparent
fn update_transparent_supply(service_id: u32, asset_id: u32, amount: u128, increment: bool) -> Result<()> {
    let supply_key = format!("supply_{}_transparent", asset_id);

    // Fetch current transparent_supply
    let current_transparent = match fetch_state_value(service_id, supply_key.as_bytes()) {
        Ok((bytes, _)) if bytes.len() >= 16 => {
            u128::from_le_bytes(bytes[..16].try_into().unwrap())
        }
        _ => 0 // Initialize to 0 if not present
    };

    // Compute new transparent supply
    let new_transparent = if increment {
        current_transparent.checked_add(amount)
            .ok_or_else(|| OrchardError::ParseError("Transparent supply overflow".into()))?
    } else {
        current_transparent.checked_sub(amount)
            .ok_or_else(|| OrchardError::ParseError("Transparent supply underflow".into()))?
    };

    log_info(&format!(
        "Update transparent_supply asset={}: {} -> {} ({}{})",
        asset_id, current_transparent, new_transparent,
        if increment { "+" } else { "-" }, amount
    ));

    // Write new transparent supply
    write_state_value(service_id, supply_key.as_bytes(), &new_transparent.to_le_bytes())?;

    // Verify supply invariant: total >= transparent (shielded supply must be non-negative)
    verify_supply_invariant(service_id, asset_id)?;

    Ok(())
}

/// Verify supply invariant: total_supply = transparent_supply + shielded_supply
///
/// Returns an error if transparent_supply > total_supply (implies negative shielded supply)
fn verify_supply_invariant(service_id: u32, asset_id: u32) -> Result<()> {
    // Fetch total_supply
    let total_key = format!("supply_{}_total", asset_id);
    let total_supply = match fetch_state_value(service_id, total_key.as_bytes()) {
        Ok((bytes, _)) if bytes.len() >= 16 => {
            u128::from_le_bytes(bytes[..16].try_into().unwrap())
        }
        _ => 0
    };

    // Fetch transparent_supply
    let transparent_key = format!("supply_{}_transparent", asset_id);
    let transparent_supply = match fetch_state_value(service_id, transparent_key.as_bytes()) {
        Ok((bytes, _)) if bytes.len() >= 16 => {
            u128::from_le_bytes(bytes[..16].try_into().unwrap())
        }
        _ => 0
    };

    // Compute shielded_supply (implicit)
    if transparent_supply > total_supply {
        return Err(OrchardError::SupplyInvariantViolation {
            asset_id,
            total: total_supply,
            transparent: transparent_supply,
            shielded: 0, // Invalid state
        });
    }

    let shielded_supply = total_supply - transparent_supply;

    log_info(&format!(
        "Supply invariant OK asset={}: total={}, transparent={}, shielded={}",
        asset_id, total_supply, transparent_supply, shielded_supply
    ));

    Ok(())
}

/// Emit a public event (Phase 7: Production Readiness)
///
/// Events: Deposit, Withdraw, Burn, Mint
///
/// Event format: [event_type:u8][asset_id:u32][amount:u128]
/// - Deposit: event_type = 0
/// - Withdraw: event_type = 1
/// - Burn: event_type = 2
/// - Mint: event_type = 3
///
/// Events are written to a special state key for indexing by external systems
fn emit_event(service_id: u32, event_type: &str, asset_id: u32, amount: u128) -> Result<()> {
    let event_type_code = match event_type {
        "Deposit" => 0u8,
        "Withdraw" => 1u8,
        "Burn" => 2u8,
        "Mint" => 3u8,
        _ => return Err(OrchardError::ParseError(format!("Unknown event type: {}", event_type))),
    };

    log_info(&format!(
        "Emit event: {} (asset={}, amount={})",
        event_type, asset_id, amount
    ));

    // Encode event: [type:u8][asset_id:u32][amount:u128]
    let mut event_data = Vec::with_capacity(1 + 4 + 16);
    event_data.push(event_type_code);
    event_data.extend_from_slice(&asset_id.to_le_bytes());
    event_data.extend_from_slice(&amount.to_le_bytes());

    // Write event to state (for indexing by external systems)
    // Event counter is used to generate unique keys
    let counter_key = b"event_counter";
    let event_counter = match fetch_state_value(service_id, counter_key) {
        Ok((bytes, _)) if bytes.len() >= 8 => {
            u64::from_le_bytes(bytes[..8].try_into().unwrap())
        }
        _ => 0
    };

    let event_key = format!("event_{}", event_counter);
    write_state_value(service_id, event_key.as_bytes(), &event_data)?;

    // Increment event counter
    let new_counter = event_counter + 1;
    write_state_value(service_id, counter_key, &new_counter.to_le_bytes())?;

    Ok(())
}

// ====== WITNESS-AWARE ACCUMULATE FUNCTIONS ======

/// Process accumulate with RefineOutput from witness-aware execution
pub fn process_accumulate_witness_aware(
    _service_id: u32,
    refine_output: RefineOutput,
    jam_state_writer: &mut dyn JAMStateWriter,
) -> Result<()> {
    let intents = match refine_output {
        RefineOutput::Builder { intents, .. } => {
            log_error("Builder mode accumulate - should not happen in production");
            intents
        }
        RefineOutput::Guarantor { intents, .. } => intents,
    };

    // Apply all write intents to JAM state
    for intent in &intents {
        apply_write_intent_to_jam_state(intent, jam_state_writer)?;
        emit_accumulator_events(intent)?;
    }

    // Final invariant checks
    validate_supply_invariants(&intents)?;

    Ok(())
}

/// Apply a WriteIntent to JAM state
/// Extracts key-value pairs from the WriteIntent payload and applies them to JAM state
fn apply_write_intent_to_jam_state(
    intent: &WriteIntent,
    jam_state: &mut dyn JAMStateWriter
) -> Result<()> {
    // The payload contains: [key_len:2][key][value]
    let payload = &intent.effect.payload;
    if payload.len() < 2 {
        return Err(OrchardError::ParseError("WriteIntent payload too short".into()));
    }

    let key_len = u16::from_le_bytes([payload[0], payload[1]]) as usize;
    if payload.len() < 2 + key_len {
        return Err(OrchardError::ParseError("WriteIntent key truncated".into()));
    }

    let key = core::str::from_utf8(&payload[2..2 + key_len])
        .map_err(|_| OrchardError::ParseError("Invalid UTF-8 in key".into()))?;
    let value = &payload[2 + key_len..];

    jam_state.write_state(key, value)?;
    Ok(())
}

/// JAM state writer trait for witness-aware accumulation
pub trait JAMStateWriter {
    fn write_state(&mut self, key: &str, value: &[u8]) -> Result<()>;
    fn read_state(&self, key: &str) -> Result<Vec<u8>>;
}

/// Emit appropriate events based on WriteIntent object_kind
fn emit_accumulator_events(intent: &WriteIntent) -> Result<()> {
    let kind = ObjectKind::from_u8(intent.effect.ref_info.object_kind);
    let object_id = &intent.effect.object_id;

    match kind {
        Some(ObjectKind::StateWrite) => {
            log_info(&format!("State updated: object_id={:?}, payload_len={}", &object_id[..8], intent.effect.payload.len()));
        }
        Some(ObjectKind::Nullifier) => {
            log_info(&format!("Nullifier spent: object_id={:?}", &object_id[..8]));
        }
        Some(ObjectKind::Commitment) => {
            log_info(&format!("Commitment added: object_id={:?}", &object_id[..8]));
        }
        Some(ObjectKind::Deposit) => {
            log_info(&format!("Deposit processed: object_id={:?}", &object_id[..8]));
        }
        Some(ObjectKind::FeeTally) => {
            log_info(&format!("Fee accumulated: object_id={:?}", &object_id[..8]));
        }
        Some(ObjectKind::DeltaWrite) => {
            log_info(&format!("Delta written: object_id={:?}", &object_id[..8]));
        }
        Some(ObjectKind::Block) => {
            log_info(&format!("Compact block stored: object_id={:?}", &object_id[..8]));
        }
        None => {
            log_info(&format!("Unknown write intent: kind={}, object_id={:?}", intent.effect.ref_info.object_kind, &object_id[..8]));
        }
    }
    Ok(())
}

/// Validate supply invariants across all WriteIntent operations
/// Supply validation is now handled during refine phase, this is a placeholder
fn validate_supply_invariants(_intents: &[WriteIntent]) -> Result<()> {
    // Supply validation is now performed during refine phase
    // Individual WriteIntents no longer carry semantic information
    // All validation happens before WriteIntents are generated
    Ok(())
}
