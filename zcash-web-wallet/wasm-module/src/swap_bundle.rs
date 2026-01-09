use serde::{Deserialize, Serialize};
use wasm_bindgen::prelude::*;

const ORCHARD_SIGHASH_INFO_V0: [u8; 1] = [0u8];
const ZSA_ENC_CIPHERTEXT_SIZE: usize = 612;
const OUT_CIPHERTEXT_SIZE: usize = 80;

#[derive(Debug, Deserialize)]
struct SwapActionInput {
    cv_net: String,
    nullifier: String,
    rk: String,
    cmx: String,
    epk: String,
    enc_ciphertext: String,
    out_ciphertext: String,
}

#[derive(Debug, Deserialize)]
struct SwapBurnInput {
    asset: String,
    amount: serde_json::Value,
}

#[derive(Debug, Deserialize)]
struct SwapActionGroupInput {
    actions: Vec<SwapActionInput>,
    flags: serde_json::Value,
    anchor: String,
    expiry_height: serde_json::Value,
    #[serde(default)]
    burns: Vec<SwapBurnInput>,
    proof: String,
    #[serde(default)]
    spend_auth_sigs: Vec<String>,
}

#[derive(Debug, Deserialize)]
struct SwapBundleRequest {
    action_groups: Vec<SwapActionGroupInput>,
    value_balance: serde_json::Value,
    binding_sig: String,
}

#[derive(Debug, Serialize)]
struct SwapBundleGroupSummary {
    action_count: usize,
    flags: u8,
    anchor: String,
    expiry_height: u32,
    burn_count: usize,
    proof_len: usize,
    spend_auth_sig_count: usize,
}

#[derive(Debug, Serialize)]
struct SwapBundleBuildResult {
    success: bool,
    swap_bundle_hex: Option<String>,
    value_balance: Option<i64>,
    binding_sig: Option<String>,
    action_groups: Option<Vec<SwapBundleGroupSummary>>,
    error: Option<String>,
}

#[wasm_bindgen]
pub fn build_swap_bundle(request_json: &str) -> String {
    let result = match build_swap_bundle_inner(request_json) {
        Ok(res) => res,
        Err(err) => SwapBundleBuildResult {
            success: false,
            swap_bundle_hex: None,
            value_balance: None,
            binding_sig: None,
            action_groups: None,
            error: Some(err),
        },
    };

    serde_json::to_string(&result).unwrap_or_else(|e| {
        serde_json::to_string(&SwapBundleBuildResult {
            success: false,
            swap_bundle_hex: None,
            value_balance: None,
            binding_sig: None,
            action_groups: None,
            error: Some(format!("Serialization error: {}", e)),
        })
        .unwrap()
    })
}

fn build_swap_bundle_inner(request_json: &str) -> Result<SwapBundleBuildResult, String> {
    let request: SwapBundleRequest =
        serde_json::from_str(request_json).map_err(|e| format!("Invalid request JSON: {}", e))?;

    if request.action_groups.is_empty() {
        return Err("SwapBundle requires at least one action group".to_string());
    }

    let mut bytes = Vec::new();
    write_compact_size(&mut bytes, request.action_groups.len() as u64);

    let mut summaries = Vec::with_capacity(request.action_groups.len());

    for group in request.action_groups {
        if group.actions.is_empty() {
            return Err("SwapBundle action group requires at least one action".to_string());
        }

        let flags = parse_u64_value(&group.flags)? as u64;
        if flags > u64::from(u8::MAX) {
            return Err("Action group flags must fit in u8".to_string());
        }
        let flags = flags as u8;

        let expiry_height_u64 = parse_u64_value(&group.expiry_height)?;
        if expiry_height_u64 > u64::from(u32::MAX) {
            return Err("Action group expiry_height must fit in u32".to_string());
        }
        let expiry_height = expiry_height_u64 as u32;
        if expiry_height != 0 {
            return Err("Action group expiry_height must be 0 for NU7".to_string());
        }

        let anchor = decode_hex_array::<32>(&group.anchor)
            .map_err(|e| format!("Invalid anchor: {}", e))?;

        write_compact_size(&mut bytes, group.actions.len() as u64);

        for action in &group.actions {
            let cv_net = decode_hex_array::<32>(&action.cv_net)
                .map_err(|e| format!("Invalid cv_net: {}", e))?;
            let nullifier = decode_hex_array::<32>(&action.nullifier)
                .map_err(|e| format!("Invalid nullifier: {}", e))?;
            let rk = decode_hex_array::<32>(&action.rk)
                .map_err(|e| format!("Invalid rk: {}", e))?;
            let cmx = decode_hex_array::<32>(&action.cmx)
                .map_err(|e| format!("Invalid cmx: {}", e))?;
            let epk = decode_hex_array::<32>(&action.epk)
                .map_err(|e| format!("Invalid epk: {}", e))?;
            let enc_ciphertext = decode_hex_vec(&action.enc_ciphertext)
                .map_err(|e| format!("Invalid enc_ciphertext: {}", e))?;
            if enc_ciphertext.len() != ZSA_ENC_CIPHERTEXT_SIZE {
                return Err(format!(
                    "enc_ciphertext must be {} bytes",
                    ZSA_ENC_CIPHERTEXT_SIZE
                ));
            }
            let out_ciphertext = decode_hex_vec(&action.out_ciphertext)
                .map_err(|e| format!("Invalid out_ciphertext: {}", e))?;
            if out_ciphertext.len() != OUT_CIPHERTEXT_SIZE {
                return Err(format!(
                    "out_ciphertext must be {} bytes",
                    OUT_CIPHERTEXT_SIZE
                ));
            }

            bytes.extend_from_slice(&cv_net);
            bytes.extend_from_slice(&nullifier);
            bytes.extend_from_slice(&rk);
            bytes.extend_from_slice(&cmx);
            bytes.extend_from_slice(&epk);
            bytes.extend_from_slice(&enc_ciphertext);
            bytes.extend_from_slice(&out_ciphertext);
        }

        bytes.push(flags);
        bytes.extend_from_slice(&anchor);
        bytes.extend_from_slice(&expiry_height.to_le_bytes());

        write_compact_size(&mut bytes, group.burns.len() as u64);
        for burn in &group.burns {
            let asset = decode_hex_array::<32>(&burn.asset)
                .map_err(|e| format!("Invalid burn asset: {}", e))?;
            let amount = parse_u64_value(&burn.amount)
                .map_err(|e| format!("Invalid burn amount: {}", e))?;
            bytes.extend_from_slice(&asset);
            bytes.extend_from_slice(&amount.to_le_bytes());
        }

        let proof = decode_hex_vec(&group.proof).map_err(|e| format!("Invalid proof: {}", e))?;
        write_compact_size(&mut bytes, proof.len() as u64);
        bytes.extend_from_slice(&proof);

        if group.spend_auth_sigs.len() != group.actions.len() {
            return Err("Spend auth signature count must match action count".to_string());
        }

        for sig_hex in &group.spend_auth_sigs {
            let sig = decode_hex_array::<64>(sig_hex)
                .map_err(|e| format!("Invalid spend auth signature: {}", e))?;
            write_versioned_signature(&mut bytes, &sig);
        }

        summaries.push(SwapBundleGroupSummary {
            action_count: group.actions.len(),
            flags,
            anchor: hex::encode(anchor),
            expiry_height,
            burn_count: group.burns.len(),
            proof_len: proof.len(),
            spend_auth_sig_count: group.spend_auth_sigs.len(),
        });
    }

    let value_balance = parse_i64_value(&request.value_balance)?;
    bytes.extend_from_slice(&value_balance.to_le_bytes());

    let binding_sig = decode_hex_array::<64>(&request.binding_sig)
        .map_err(|e| format!("Invalid binding signature: {}", e))?;
    write_versioned_signature(&mut bytes, &binding_sig);

    Ok(SwapBundleBuildResult {
        success: true,
        swap_bundle_hex: Some(hex::encode(&bytes)),
        value_balance: Some(value_balance),
        binding_sig: Some(hex::encode(binding_sig)),
        action_groups: Some(summaries),
        error: None,
    })
}

fn write_versioned_signature(out: &mut Vec<u8>, sig: &[u8; 64]) {
    write_compact_size(out, ORCHARD_SIGHASH_INFO_V0.len() as u64);
    out.extend_from_slice(&ORCHARD_SIGHASH_INFO_V0);
    out.extend_from_slice(sig);
}

fn write_compact_size(out: &mut Vec<u8>, n: u64) {
    match n {
        0..=252 => out.push(n as u8),
        253..=0xFFFF => {
            out.push(253);
            out.extend_from_slice(&(n as u16).to_le_bytes());
        }
        0x10000..=0xFFFF_FFFF => {
            out.push(254);
            out.extend_from_slice(&(n as u32).to_le_bytes());
        }
        _ => {
            out.push(255);
            out.extend_from_slice(&n.to_le_bytes());
        }
    }
}

fn decode_hex_array<const N: usize>(hex_str: &str) -> Result<[u8; N], String> {
    let trimmed = hex_str.trim();
    if trimmed.is_empty() {
        return Err("hex string is empty".to_string());
    }
    let bytes = hex::decode(trimmed).map_err(|e| e.to_string())?;
    if bytes.len() != N {
        return Err(format!("expected {} bytes, got {}", N, bytes.len()));
    }
    let mut out = [0u8; N];
    out.copy_from_slice(&bytes);
    Ok(out)
}

fn decode_hex_vec(hex_str: &str) -> Result<Vec<u8>, String> {
    let trimmed = hex_str.trim();
    if trimmed.is_empty() {
        return Err("hex string is empty".to_string());
    }
    hex::decode(trimmed).map_err(|e| e.to_string())
}

fn parse_u64_value(value: &serde_json::Value) -> Result<u64, String> {
    match value {
        serde_json::Value::Number(num) => num
            .as_u64()
            .ok_or_else(|| "value must be a positive integer".to_string()),
        serde_json::Value::String(s) => s
            .parse::<u64>()
            .map_err(|_| "value must be a positive integer".to_string()),
        _ => Err("value must be a number or numeric string".to_string()),
    }
}

fn parse_i64_value(value: &serde_json::Value) -> Result<i64, String> {
    match value {
        serde_json::Value::Number(num) => num
            .as_i64()
            .ok_or_else(|| "value must be an integer".to_string()),
        serde_json::Value::String(s) => s
            .parse::<i64>()
            .map_err(|_| "value must be an integer".to_string()),
        _ => Err("value must be a number or numeric string".to_string()),
    }
}
