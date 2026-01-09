use crate::bundle_codec::{deserialize_bundle, serialize_bundle};
use crate::OrchardBundle;
use blake2b_simd::Params as Blake2bParams;
use halo2_proofs::{
    plonk::{keygen_pk, VerifyingKey as Halo2VerifyingKey},
    poly::commitment::Params,
};
use orchard::builder::{Builder as OrchardBundleBuilder, BundleType};
use orchard::keys::{FullViewingKey, SpendingKey, Scope};
use orchard::tree::Anchor;
use orchard::value::NoteValue;
use pasta_curves::arithmetic::CurveAffine;
use pasta_curves::pallas;
use pasta_curves::group::{ff::PrimeField, Curve, GroupEncoding};
use pasta_curves::vesta::Affine as VestaAffine;
use rand::{rngs::StdRng, RngCore, SeedableRng};
use std::env;
use std::fs;
use std::slice;
use std::sync::OnceLock;

const ACTION_COUNT: usize = 2;
const INPUTS_PER_ACTION: usize = 9;
const MAX_INPUTS: usize = ACTION_COUNT * INPUTS_PER_ACTION;
const PROOF_SIZE: usize = 7264;

#[repr(C)]
pub enum FFIResult {
    Success = 0,
    InvalidInput = 1,
    ProofGenerationFailed = 2,
    VerificationFailed = 3,
    SerializationError = 4,
    InternalError = 5,
}

static PROVING_KEY: OnceLock<orchard::circuit::ProvingKey> = OnceLock::new();

fn load_proving_key() -> Result<orchard::circuit::ProvingKey, String> {
    let keys_dir = resolve_keys_dir();
    let params_path = keys_dir.join("orchard.params");
    let vk_path = keys_dir.join("orchard.vk");

    let params_bytes = fs::read(&params_path)
        .map_err(|e| format!("Failed to read params: {e}"))?;
    let params = Params::<VestaAffine>::read(&mut &params_bytes[..])
        .map_err(|e| format!("Failed to decode params: {e}"))?;

    let vk_bytes = fs::read(&vk_path)
        .map_err(|e| format!("Failed to read vk: {e}"))?;
    let vk = Halo2VerifyingKey::read(&mut &vk_bytes[..])
        .map_err(|e| format!("Failed to decode vk: {e}"))?;

    let circuit = orchard::circuit::Circuit::default();
    let pk = keygen_pk(&params, vk, &circuit)
        .map_err(|e| format!("Failed to create proving key: {e:?}"))?;

    Ok(orchard::circuit::ProvingKey::from_parts(params, pk))
}

fn resolve_keys_dir() -> std::path::PathBuf {
    let manifest_dir = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let manifest_candidate = manifest_dir
        .join("..")
        .join("..")
        .join("services")
        .join("orchard")
        .join("keys");
    if manifest_candidate.exists() {
        return manifest_candidate;
    }

    if let Ok(path) = env::var("ORCHARD_KEYS_DIR") {
        let candidate = std::path::PathBuf::from(path);
        if candidate.exists() {
            return candidate;
        }
    }

    if let Ok(cwd) = env::current_dir() {
        for ancestor in cwd.ancestors() {
            let candidate = ancestor.join("services").join("orchard").join("keys");
            if candidate.exists() {
                return candidate;
            }
            let candidate = ancestor.join("builder").join("orchard").join("keys");
            if candidate.exists() {
                return candidate;
            }
            let candidate = ancestor.join("keys");
            if candidate.exists() {
                return candidate;
            }
        }
    }

    std::path::PathBuf::from("keys")
}

fn get_proving_key() -> Result<&'static orchard::circuit::ProvingKey, String> {
    if let Some(pk) = PROVING_KEY.get() {
        return Ok(pk);
    }
    let pk = load_proving_key()?;
    Ok(PROVING_KEY.get_or_init(|| pk))
}

fn seed_from_bytes(seed: &[u8]) -> [u8; 32] {
    let mut out = [0u8; 32];
    let mut hasher = Blake2bParams::new()
        .hash_length(32)
        .to_state();
    hasher.update(seed);
    out.copy_from_slice(hasher.finalize().as_bytes());
    out
}

fn random_spending_key(rng: &mut impl RngCore) -> SpendingKey {
    loop {
        let mut bytes = [0u8; 32];
        rng.fill_bytes(&mut bytes);
        let sk = SpendingKey::from_bytes(bytes);
        if sk.is_some().into() {
            break sk.unwrap();
        }
    }
}

fn build_outputs_only_bundle(
    proving_key: &orchard::circuit::ProvingKey,
    seed: &[u8],
    anchor: Anchor,
) -> Result<OrchardBundle, String> {
    let seed_bytes = seed_from_bytes(seed);
    let mut rng = StdRng::from_seed(seed_bytes);

    let sk = random_spending_key(&mut rng);
    let fvk = FullViewingKey::from(&sk);
    let recipient = fvk.address_at(0u32, Scope::External);
    let mut builder = OrchardBundleBuilder::new(BundleType::DEFAULT, anchor);

    let memo = [0u8; 512];
    let note_value = NoteValue::from_raw(0);

    for _ in 0..ACTION_COUNT {
        builder
            .add_output(None, recipient, note_value, memo)
            .map_err(|e| format!("Failed to add output: {e:?}"))?;
    }

    let result = builder
        .build::<i64>(&mut rng)
        .map_err(|e| format!("Failed to build bundle: {e:?}"))?;
    let (unauthorized_bundle, _) = result.ok_or("Bundle builder returned None")?;

    let proven_bundle = unauthorized_bundle
        .create_proof(proving_key, &mut rng)
        .map_err(|e| format!("Failed to create proof: {e:?}"))?;

    let sighash: [u8; 32] = proven_bundle.commitment().into();
    let authorized_bundle = proven_bundle
        .apply_signatures(&mut rng, sighash, &[])
        .map_err(|e| format!("Failed to apply signatures: {e:?}"))?;

    Ok(authorized_bundle)
}

/// Generate a deterministic Orchard bundle and return serialized bundle bytes.
///
/// # Safety
/// All pointers must be valid for the specified lengths.
#[no_mangle]
pub extern "C" fn orchard_builder_generate_bundle(
    seed_ptr: *const u8,
    seed_len: u32,
    anchor_ptr: *const u8,
    anchor_len: u32,
    bundle_buf: *mut u8,
    bundle_cap: u32,
    bundle_len_out: *mut u32,
) -> u32 {
    if bundle_buf.is_null() || bundle_len_out.is_null() {
        return FFIResult::InvalidInput as u32;
    }
    if seed_len > 0 && seed_ptr.is_null() {
        return FFIResult::InvalidInput as u32;
    }
    if anchor_ptr.is_null() || anchor_len != 32 {
        return FFIResult::InvalidInput as u32;
    }

    let seed = unsafe { slice::from_raw_parts(seed_ptr, seed_len as usize) };
    let anchor_slice = unsafe { slice::from_raw_parts(anchor_ptr, anchor_len as usize) };
    let mut anchor_bytes = [0u8; 32];
    anchor_bytes.copy_from_slice(anchor_slice);

    let anchor = match Anchor::from_bytes(anchor_bytes).into_option() {
        Some(anchor) => anchor,
        None => {
            eprintln!("orchard_builder_generate_bundle: invalid anchor bytes");
            return FFIResult::InvalidInput as u32;
        }
    };

    let proving_key = match get_proving_key() {
        Ok(pk) => pk,
        Err(err) => {
            eprintln!("orchard_builder_generate_bundle: {err}");
            return FFIResult::InternalError as u32;
        }
    };

    let bundle = match build_outputs_only_bundle(proving_key, seed, anchor) {
        Ok(bundle) => bundle,
        Err(err) => {
            eprintln!("orchard_builder_generate_bundle: {err}");
            return FFIResult::ProofGenerationFailed as u32;
        }
    };

    let bundle_bytes = match serialize_bundle(&bundle) {
        Ok(bytes) => bytes,
        Err(err) => {
            eprintln!("orchard_builder_generate_bundle: {err:?}");
            return FFIResult::SerializationError as u32;
        }
    };

    if bundle_bytes.len() > bundle_cap as usize {
        return FFIResult::InvalidInput as u32;
    }

    unsafe {
        let bundle_out = slice::from_raw_parts_mut(bundle_buf, bundle_bytes.len());
        bundle_out.copy_from_slice(&bundle_bytes);
        *bundle_len_out = bundle_bytes.len() as u32;
    }

    FFIResult::Success as u32
}

/// Decode an Orchard bundle and return nullifiers, commitments, proof bytes, and public inputs.
///
/// # Safety
/// All pointers must be valid for the specified lengths.
#[no_mangle]
pub extern "C" fn orchard_builder_decode_bundle(
    bundle_ptr: *const u8,
    bundle_len: u32,
    nullifiers_buf: *mut u8,
    nullifiers_cap: u32,
    commitments_buf: *mut u8,
    commitments_cap: u32,
    proof_buf: *mut u8,
    proof_cap: u32,
    inputs_buf: *mut u8,
    inputs_cap: u32,
    action_count_out: *mut u32,
    inputs_len_out: *mut u32,
    proof_len_out: *mut u32,
) -> u32 {
    if bundle_ptr.is_null()
        || nullifiers_buf.is_null()
        || commitments_buf.is_null()
        || proof_buf.is_null()
        || inputs_buf.is_null()
        || action_count_out.is_null()
        || inputs_len_out.is_null()
        || proof_len_out.is_null()
    {
        return FFIResult::InvalidInput as u32;
    }
    if bundle_len == 0 {
        return FFIResult::InvalidInput as u32;
    }

    let bundle_bytes = unsafe { slice::from_raw_parts(bundle_ptr, bundle_len as usize) };
    let bundle = match deserialize_bundle(bundle_bytes) {
        Ok(bundle) => bundle,
        Err(err) => {
            eprintln!("orchard_builder_decode_bundle: {err}");
            return FFIResult::SerializationError as u32;
        }
    };

    let action_count = bundle.actions().len();
    let nullifier_len = action_count * 32;
    let commitment_len = action_count * 32;

    if nullifier_len > nullifiers_cap as usize || commitment_len > commitments_cap as usize {
        return FFIResult::InvalidInput as u32;
    }

    let anchor = bundle.anchor().to_bytes();
    let public_inputs = match encode_orchard_nu5_public_inputs(&anchor, &bundle) {
        Ok(inputs) => inputs,
        Err(err) => {
            eprintln!("orchard_builder_decode_bundle: {err}");
            return FFIResult::SerializationError as u32;
        }
    };

    let input_len = public_inputs.len() * 32;
    if input_len > inputs_cap as usize {
        return FFIResult::InvalidInput as u32;
    }

    let proof_bytes = bundle.authorization().proof().as_ref();
    if proof_bytes.len() > proof_cap as usize {
        return FFIResult::InvalidInput as u32;
    }

    unsafe {
        let nullifiers_out = slice::from_raw_parts_mut(nullifiers_buf, nullifier_len);
        let commitments_out = slice::from_raw_parts_mut(commitments_buf, commitment_len);

        for (idx, action) in bundle.actions().iter().enumerate() {
            let start = idx * 32;
            nullifiers_out[start..start + 32].copy_from_slice(&action.nullifier().to_bytes());
            commitments_out[start..start + 32].copy_from_slice(&action.cmx().to_bytes());
        }

        let proof_out = slice::from_raw_parts_mut(proof_buf, proof_bytes.len());
        proof_out.copy_from_slice(proof_bytes);

        let inputs_out = slice::from_raw_parts_mut(inputs_buf, input_len);
        for (idx, input) in public_inputs.iter().enumerate() {
            let start = idx * 32;
            inputs_out[start..start + 32].copy_from_slice(input);
        }

        *action_count_out = action_count as u32;
        *inputs_len_out = public_inputs.len() as u32;
        *proof_len_out = proof_bytes.len() as u32;
    }

    FFIResult::Success as u32
}

fn encode_orchard_nu5_public_inputs(
    anchor: &[u8; 32],
    bundle: &OrchardBundle,
) -> Result<Vec<[u8; 32]>, String> {
    let mut inputs = Vec::with_capacity(bundle.actions().len() * INPUTS_PER_ACTION);

    for action in bundle.actions() {
        inputs.push(*anchor);

        let cv_net_bytes = action.cv_net().to_bytes();
        let (cv_net_x, cv_net_y) = point_bytes_to_xy(&cv_net_bytes, true, "cv_net")?;
        inputs.push(cv_net_x);
        inputs.push(cv_net_y);

        inputs.push(action.nullifier().to_bytes());

        let rk_bytes: [u8; 32] = action.rk().into();
        let (rk_x, rk_y) = point_bytes_to_xy(&rk_bytes, false, "rk")?;
        inputs.push(rk_x);
        inputs.push(rk_y);

        inputs.push(action.cmx().to_bytes());
        inputs.push(u32_to_field_bytes(1));
        inputs.push(u32_to_field_bytes(1));
    }

    Ok(inputs)
}

fn u32_to_field_bytes(value: u32) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[0..4].copy_from_slice(&value.to_le_bytes());
    bytes
}

fn point_bytes_to_xy(
    bytes: &[u8; 32],
    allow_identity: bool,
    label: &str,
) -> Result<([u8; 32], [u8; 32]), String> {
    let point = pallas::Point::from_bytes(bytes)
        .into_option()
        .ok_or_else(|| format!("Invalid {label} point bytes"))?;
    let coords = point.to_affine().coordinates().into_option();

    if let Some(coords) = coords {
        Ok((coords.x().to_repr(), coords.y().to_repr()))
    } else if allow_identity {
        let zero = pallas::Base::from(0u64).to_repr();
        Ok((zero, zero))
    } else {
        Err(format!("{label} point is identity"))
    }
}

/// Generate a real Orchard proof and public inputs for a deterministic outputs-only bundle.
///
/// # Safety
/// All pointers must be valid for the specified lengths.
#[no_mangle]
pub extern "C" fn orchard_builder_generate_proof(
    seed_ptr: *const u8,
    seed_len: u32,
    proof_buf: *mut u8,
    proof_cap: u32,
    inputs_buf: *mut u8,
    inputs_cap: u32,
    inputs_len_out: *mut u32,
) -> u32 {
    if proof_buf.is_null() || inputs_buf.is_null() || inputs_len_out.is_null() {
        return FFIResult::InvalidInput as u32;
    }
    if seed_len > 0 && seed_ptr.is_null() {
        return FFIResult::InvalidInput as u32;
    }
    if proof_cap < PROOF_SIZE as u32 {
        return FFIResult::InvalidInput as u32;
    }
    if inputs_cap < (MAX_INPUTS * 32) as u32 {
        return FFIResult::InvalidInput as u32;
    }

    let seed = unsafe { slice::from_raw_parts(seed_ptr, seed_len as usize) };
    let proving_key = match get_proving_key() {
        Ok(pk) => pk,
        Err(err) => {
            eprintln!("orchard_builder_generate_proof: {err}");
            return FFIResult::InternalError as u32;
        }
    };

    let bundle = match build_outputs_only_bundle(proving_key, seed, Anchor::empty_tree()) {
        Ok(bundle) => bundle,
        Err(err) => {
            eprintln!("orchard_builder_generate_proof: {err}");
            return FFIResult::ProofGenerationFailed as u32;
        }
    };

    let anchor = Anchor::empty_tree().to_bytes();
    let public_inputs = match encode_orchard_nu5_public_inputs(&anchor, &bundle) {
        Ok(inputs) => inputs,
        Err(err) => {
            eprintln!("orchard_builder_generate_proof: {err}");
            return FFIResult::SerializationError as u32;
        }
    };
    if public_inputs.len() > MAX_INPUTS {
        return FFIResult::InvalidInput as u32;
    }

    let proof_bytes = bundle.authorization().proof().as_ref();
    if proof_bytes.len() != PROOF_SIZE {
        eprintln!(
            "orchard_builder_generate_proof: unexpected proof size {}",
            proof_bytes.len()
        );
        return FFIResult::ProofGenerationFailed as u32;
    }

    unsafe {
        let proof_out = slice::from_raw_parts_mut(proof_buf, PROOF_SIZE);
        proof_out.copy_from_slice(proof_bytes);

        let inputs_out = slice::from_raw_parts_mut(inputs_buf, public_inputs.len() * 32);
        for (idx, input) in public_inputs.iter().enumerate() {
            let start = idx * 32;
            inputs_out[start..start + 32].copy_from_slice(input);
        }
        *inputs_len_out = public_inputs.len() as u32;
    }

    FFIResult::Success as u32
}
