use blake2b_simd::Params as Blake2bParams;
use getrandom::getrandom;
use k256::schnorr::signature::hazmat::PrehashSigner;
use k256::schnorr::{Signature as SchnorrSignature, SigningKey};
use pasta_curves::arithmetic::{CurveAffine, CurveExt};
use pasta_curves::group::ff::{Field, FromUniformBytes, PrimeField, PrimeFieldBits};
use pasta_curves::group::{Curve, Group, GroupEncoding};
use pasta_curves::pallas;
use serde::{Deserialize, Serialize};
use sinsemilla::HashDomain;
use wasm_bindgen::prelude::*;
use zcash_address::unified::{self, Container, Encoding};

const ISSUE_SIG_ALGO_BYTE: u8 = 0x00;
const ISSUE_SIGHASH_INFO_V0: [u8; 1] = [0u8];
const ZCASH_ORCHARD_ZSA_ISSUE_PERSONALIZATION: &[u8; 16] = b"ZTxIdSAIssueHash";
const ZCASH_ORCHARD_ZSA_ISSUE_ACTION_PERSONALIZATION: &[u8; 16] = b"ZTxIdIssuActHash";
const ZCASH_ORCHARD_ZSA_ISSUE_NOTE_PERSONALIZATION: &[u8; 16] = b"ZTxIdIAcNoteHash";
const ZSA_ASSET_DIGEST_PERSONALIZATION: &[u8; 16] = b"ZSA-Asset-Digest";
const ZSA_ASSET_BASE_PERSONALIZATION: &str = "z.cash:OrchardZSA";
const PRF_EXPAND_PERSONALIZATION: &[u8; 16] = b"Zcash_ExpandSeed";
const KEY_DIVERSIFICATION_PERSONALIZATION: &str = "z.cash:Orchard-gd";
const NOTE_COMMITMENT_PERSONALIZATION: &str = "z.cash:Orchard-NoteCommit";
const NOTE_ZSA_COMMITMENT_PERSONALIZATION: &str = "z.cash:ZSA-NoteCommit";
const L_ORCHARD_BASE: usize = 255;

#[derive(Debug, Deserialize)]
struct IssueNoteInput {
    recipient: String,
    value: serde_json::Value,
    rho: Option<String>,
    rseed: Option<String>,
}

#[derive(Debug, Deserialize)]
struct IssueActionInput {
    asset_desc_hash: String,
    notes: Vec<IssueNoteInput>,
    finalize: bool,
}

#[derive(Debug, Deserialize)]
struct IssueBundleRequest {
    issuer_secret_key: Option<String>,
    actions: Vec<IssueActionInput>,
}

#[derive(Debug, Serialize)]
struct IssueAuthKeyResult {
    success: bool,
    issuer_secret_key: Option<String>,
    issuer_key: Option<String>,
    error: Option<String>,
}

#[derive(Debug, Serialize)]
struct IssueBundleNoteSummary {
    recipient_raw: String,
    value: u64,
    rho: String,
    rseed: String,
    commitment: String,
}

#[derive(Debug, Serialize)]
struct IssueBundleActionSummary {
    asset_desc_hash: String,
    asset_base: String,
    finalize: bool,
    notes: Vec<IssueBundleNoteSummary>,
}

#[derive(Debug, Serialize)]
struct IssueBundleBuildResult {
    success: bool,
    issue_bundle_hex: Option<String>,
    issuer_secret_key: Option<String>,
    issuer_key: Option<String>,
    signature: Option<String>,
    commitment: Option<String>,
    actions: Option<Vec<IssueBundleActionSummary>>,
    error: Option<String>,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
struct AssetBase([u8; 32]);

impl AssetBase {
    fn from_bytes(bytes: [u8; 32]) -> Self {
        Self(bytes)
    }

    fn to_bytes(&self) -> [u8; 32] {
        self.0
    }
}

#[derive(Clone, Debug)]
struct IssueNote {
    recipient: [u8; 43],
    value: u64,
    rho: [u8; 32],
    rseed: [u8; 32],
}

#[derive(Clone, Debug)]
struct IssueAction {
    asset_desc_hash: [u8; 32],
    notes: Vec<IssueNote>,
    finalize: bool,
}

#[derive(Clone, Debug)]
struct IssueBundleData {
    issuer_key: Vec<u8>,
    actions: Vec<IssueAction>,
}

#[wasm_bindgen]
pub fn generate_issue_auth_key() -> String {
    let result = match generate_issue_auth_key_inner() {
        Ok((secret_hex, issuer_key_hex)) => IssueAuthKeyResult {
            success: true,
            issuer_secret_key: Some(secret_hex),
            issuer_key: Some(issuer_key_hex),
            error: None,
        },
        Err(err) => IssueAuthKeyResult {
            success: false,
            issuer_secret_key: None,
            issuer_key: None,
            error: Some(err),
        },
    };

    serde_json::to_string(&result).unwrap_or_else(|e| {
        serde_json::to_string(&IssueAuthKeyResult {
            success: false,
            issuer_secret_key: None,
            issuer_key: None,
            error: Some(format!("Serialization error: {}", e)),
        })
        .unwrap()
    })
}

#[wasm_bindgen]
pub fn build_issue_bundle(request_json: &str) -> String {
    let result = match build_issue_bundle_inner(request_json) {
        Ok(result) => result,
        Err(err) => IssueBundleBuildResult {
            success: false,
            issue_bundle_hex: None,
            issuer_secret_key: None,
            issuer_key: None,
            signature: None,
            commitment: None,
            actions: None,
            error: Some(err),
        },
    };

    serde_json::to_string(&result).unwrap_or_else(|e| {
        serde_json::to_string(&IssueBundleBuildResult {
            success: false,
            issue_bundle_hex: None,
            issuer_secret_key: None,
            issuer_key: None,
            signature: None,
            commitment: None,
            actions: None,
            error: Some(format!("Serialization error: {}", e)),
        })
        .unwrap()
    })
}

fn build_issue_bundle_inner(request_json: &str) -> Result<IssueBundleBuildResult, String> {
    let request: IssueBundleRequest =
        serde_json::from_str(request_json).map_err(|e| format!("Invalid request JSON: {}", e))?;

    if request.actions.is_empty() {
        return Err("IssueBundle requires at least one action".to_string());
    }

    let (signing_key, issuer_secret_key_hex, issuer_key_bytes, issuer_key_hex) =
        resolve_issuer_keys(request.issuer_secret_key.as_deref())?;

    let mut actions = Vec::with_capacity(request.actions.len());
    let mut action_summaries = Vec::with_capacity(request.actions.len());

    for action in request.actions {
        if action.notes.is_empty() {
            return Err("IssueBundle action requires at least one note".to_string());
        }

        let asset_desc_hash = decode_hex_array::<32>(&action.asset_desc_hash)
            .map_err(|e| format!("Invalid asset_desc_hash: {}", e))?;
        let asset_base = derive_asset_base(&issuer_key_bytes, &asset_desc_hash)?;

        let mut notes = Vec::with_capacity(action.notes.len());
        let mut note_summaries = Vec::with_capacity(action.notes.len());

        for note in action.notes {
            let recipient = decode_recipient(&note.recipient)?;
            let value = parse_u64_value(&note.value)
                .map_err(|e| format!("Invalid note value: {}", e))?;
            let rho = decode_or_random_base(note.rho.as_deref(), "rho")?;
            let rseed = decode_or_random_32(note.rseed.as_deref(), "rseed")?;

            let issue_note = IssueNote {
                recipient,
                value,
                rho,
                rseed,
            };

            let commitment = issue_note_commitment(&issue_note, &asset_base)?;

            note_summaries.push(IssueBundleNoteSummary {
                recipient_raw: hex::encode(recipient),
                value,
                rho: hex::encode(rho),
                rseed: hex::encode(rseed),
                commitment: hex::encode(commitment),
            });

            notes.push(issue_note);
        }

        action_summaries.push(IssueBundleActionSummary {
            asset_desc_hash: hex::encode(asset_desc_hash),
            asset_base: hex::encode(asset_base.to_bytes()),
            finalize: action.finalize,
            notes: note_summaries,
        });

        actions.push(IssueAction {
            asset_desc_hash,
            notes,
            finalize: action.finalize,
        });
    }

    let issue_bundle = IssueBundleData {
        issuer_key: issuer_key_bytes.clone(),
        actions: actions.clone(),
    };

    let commitment = compute_issue_bundle_commitment(&issue_bundle)?;
    let signature = signing_key
        .sign_prehash(&commitment)
        .map_err(|_| "IssueBundle signature failed".to_string())?;

    let signature_bytes = encode_issue_signature(&signature);

    let issue_bundle_bytes =
        encode_issue_bundle_bytes(&issuer_key_bytes, &actions, &signature_bytes);

    Ok(IssueBundleBuildResult {
        success: true,
        issue_bundle_hex: Some(hex::encode(issue_bundle_bytes)),
        issuer_secret_key: Some(issuer_secret_key_hex),
        issuer_key: Some(issuer_key_hex),
        signature: Some(hex::encode(signature_bytes)),
        commitment: Some(hex::encode(commitment)),
        actions: Some(action_summaries),
        error: None,
    })
}

fn resolve_issuer_keys(
    issuer_secret_key_hex: Option<&str>,
) -> Result<(SigningKey, String, Vec<u8>, String), String> {
    let (signing_key, secret_key_hex) = match issuer_secret_key_hex {
        Some(value) if !value.trim().is_empty() => {
            let secret_key = decode_hex_array::<32>(value)
                .map_err(|e| format!("Invalid issuer secret key: {}", e))?;
            let signing_key = SigningKey::from_bytes(&secret_key)
                .map_err(|_| "Invalid issuer secret key".to_string())?;
            (signing_key, hex::encode(secret_key))
        }
        _ => {
            let signing_key = generate_signing_key()?;
            let secret_key = signing_key.to_bytes();
            (signing_key, hex::encode(secret_key))
        }
    };

    let verifying_key = signing_key.verifying_key();
    let issuer_key_bytes = encode_issue_validating_key(verifying_key);
    let issuer_key_hex = hex::encode(&issuer_key_bytes);

    Ok((signing_key, secret_key_hex, issuer_key_bytes, issuer_key_hex))
}

fn generate_issue_auth_key_inner() -> Result<(String, String), String> {
    let signing_key = generate_signing_key()?;
    let secret_key = signing_key.to_bytes();
    let verifying_key = signing_key.verifying_key();
    let issuer_key_bytes = encode_issue_validating_key(verifying_key);

    Ok((hex::encode(secret_key), hex::encode(issuer_key_bytes)))
}

fn generate_signing_key() -> Result<SigningKey, String> {
    for _ in 0..16 {
        let mut bytes = [0u8; 32];
        getrandom(&mut bytes).map_err(|e| format!("Randomness error: {}", e))?;
        if let Ok(key) = SigningKey::from_bytes(&bytes) {
            return Ok(key);
        }
    }
    Err("Failed to generate issuer key".to_string())
}

fn decode_recipient(input: &str) -> Result<[u8; 43], String> {
    let trimmed = input.trim();
    if trimmed.is_empty() {
        return Err("Recipient address is required".to_string());
    }

    if let Ok(bytes) = decode_hex_array::<43>(trimmed) {
        return Ok(bytes);
    }

    if let Ok((_, address)) = unified::Address::decode(trimmed) {
        for item in address.items() {
            if let unified::Receiver::Orchard(data) = item {
                return Ok(data);
            }
        }
        return Err("Unified address has no Orchard receiver".to_string());
    }

    Err("Unsupported recipient address format".to_string())
}

fn decode_or_random_32(input: Option<&str>, label: &str) -> Result<[u8; 32], String> {
    if let Some(value) = input {
        if value.trim().is_empty() {
            return random_bytes_32();
        }
        return decode_hex_array::<32>(value)
            .map_err(|e| format!("Invalid {}: {}", label, e));
    }
    random_bytes_32()
}

fn decode_or_random_base(input: Option<&str>, label: &str) -> Result<[u8; 32], String> {
    if let Some(value) = input {
        if value.trim().is_empty() {
            return random_base_bytes();
        }
        return decode_hex_array::<32>(value)
            .map_err(|e| format!("Invalid {}: {}", label, e));
    }
    random_base_bytes()
}

fn random_bytes_32() -> Result<[u8; 32], String> {
    let mut bytes = [0u8; 32];
    getrandom(&mut bytes).map_err(|e| format!("Randomness error: {}", e))?;
    Ok(bytes)
}

fn random_base_bytes() -> Result<[u8; 32], String> {
    let mut bytes = [0u8; 64];
    getrandom(&mut bytes).map_err(|e| format!("Randomness error: {}", e))?;
    Ok(pallas::Base::from_uniform_bytes(&bytes).to_repr())
}

fn parse_u64_value(value: &serde_json::Value) -> Result<u64, String> {
    match value {
        serde_json::Value::Number(num) => num
            .as_u64()
            .ok_or_else(|| "Value must be an unsigned integer".to_string()),
        serde_json::Value::String(s) => s
            .parse::<u64>()
            .map_err(|_| "Value must be an unsigned integer string".to_string()),
        _ => Err("Value must be an unsigned integer".to_string()),
    }
}

fn encode_issue_validating_key(verifying_key: &k256::schnorr::VerifyingKey) -> Vec<u8> {
    let mut out = Vec::with_capacity(33);
    out.push(ISSUE_SIG_ALGO_BYTE);
    out.extend_from_slice(verifying_key.to_bytes().as_slice());
    out
}

fn encode_issue_signature(signature: &SchnorrSignature) -> Vec<u8> {
    let mut out = Vec::with_capacity(65);
    out.push(ISSUE_SIG_ALGO_BYTE);
    out.extend_from_slice(signature.to_bytes().as_slice());
    out
}

fn encode_issue_bundle_bytes(
    issuer_key: &[u8],
    actions: &[IssueAction],
    signature: &[u8],
) -> Vec<u8> {
    let mut out = Vec::new();
    write_compact_size(&mut out, issuer_key.len());
    out.extend_from_slice(issuer_key);

    write_compact_size(&mut out, actions.len());
    for action in actions {
        out.extend_from_slice(&action.asset_desc_hash);
        write_compact_size(&mut out, action.notes.len());
        for note in &action.notes {
            out.extend_from_slice(&note.recipient);
            out.extend_from_slice(&note.value.to_le_bytes());
            out.extend_from_slice(&note.rho);
            out.extend_from_slice(&note.rseed);
        }
        out.push(action.finalize as u8);
    }

    write_compact_size(&mut out, ISSUE_SIGHASH_INFO_V0.len());
    out.extend_from_slice(&ISSUE_SIGHASH_INFO_V0);

    write_compact_size(&mut out, signature.len());
    out.extend_from_slice(signature);

    out
}

fn write_compact_size(out: &mut Vec<u8>, n: usize) {
    if n < 253 {
        out.push(n as u8);
    } else if n <= 0xFFFF {
        out.push(253);
        out.extend_from_slice(&(n as u16).to_le_bytes());
    } else if n <= 0xFFFF_FFFF {
        out.push(254);
        out.extend_from_slice(&(n as u32).to_le_bytes());
    } else {
        out.push(255);
        out.extend_from_slice(&(n as u64).to_le_bytes());
    }
}

fn compute_issue_bundle_commitment(bundle: &IssueBundleData) -> Result<[u8; 32], String> {
    let mut h = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ZSA_ISSUE_PERSONALIZATION)
        .to_state();

    h.update(&compact_size_bytes(bundle.issuer_key.len()));
    h.update(&bundle.issuer_key);

    let mut action_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ZSA_ISSUE_ACTION_PERSONALIZATION)
        .to_state();

    for action in &bundle.actions {
        action_hasher.update(&action.asset_desc_hash);

        let mut note_hasher = Blake2bParams::new()
            .hash_length(32)
            .personal(ZCASH_ORCHARD_ZSA_ISSUE_NOTE_PERSONALIZATION)
            .to_state();
        for note in &action.notes {
            note_hasher.update(&note.recipient);
            note_hasher.update(&note.value.to_le_bytes());
            note_hasher.update(&note.rho);
            note_hasher.update(&note.rseed);
        }
        action_hasher.update(note_hasher.finalize().as_bytes());
        action_hasher.update(&[action.finalize as u8]);
    }

    h.update(action_hasher.finalize().as_bytes());
    let mut out = [0u8; 32];
    out.copy_from_slice(h.finalize().as_bytes());
    Ok(out)
}

fn compact_size_bytes(size: usize) -> Vec<u8> {
    match size {
        s if s < 253 => vec![s as u8],
        s if s <= 0xFFFF => [&[253_u8], &(s as u16).to_le_bytes()[..]].concat(),
        s if s <= 0xFFFF_FFFF => [&[254_u8], &(s as u32).to_le_bytes()[..]].concat(),
        s => [&[255_u8], &(s as u64).to_le_bytes()[..]].concat(),
    }
}

fn derive_asset_base(
    issuer_key_bytes: &[u8],
    asset_desc_hash: &[u8; 32],
) -> Result<AssetBase, String> {
    let mut asset_id = Vec::with_capacity(1 + issuer_key_bytes.len() + asset_desc_hash.len());
    asset_id.push(0x00);
    asset_id.extend_from_slice(issuer_key_bytes);
    asset_id.extend_from_slice(asset_desc_hash);

    let asset_digest = Blake2bParams::new()
        .hash_length(64)
        .personal(ZSA_ASSET_DIGEST_PERSONALIZATION)
        .to_state()
        .update(&asset_id)
        .finalize();

    let asset_base =
        pallas::Point::hash_to_curve(ZSA_ASSET_BASE_PERSONALIZATION)(asset_digest.as_bytes());
    if bool::from(asset_base.is_identity()) {
        return Err("Derived asset base is identity".to_string());
    }

    Ok(AssetBase::from_bytes(asset_base.to_bytes()))
}

fn issue_note_commitment(note: &IssueNote, asset: &AssetBase) -> Result<[u8; 32], String> {
    let diversifier: [u8; 11] = note.recipient[..11]
        .try_into()
        .map_err(|_| "Invalid recipient length".to_string())?;
    let pk_d_bytes: [u8; 32] = note.recipient[11..]
        .try_into()
        .map_err(|_| "Invalid recipient length".to_string())?;

    let pk_d = pallas::Point::from_bytes(&pk_d_bytes)
        .into_option()
        .ok_or_else(|| "Invalid recipient pk_d".to_string())?;
    if bool::from(pk_d.is_identity()) {
        return Err("Recipient pk_d is identity".to_string());
    }

    let g_d = diversify_hash(&diversifier)?;
    let g_d_bytes = g_d.to_bytes();
    let pk_d_bytes = pk_d.to_bytes();

    let rho = pallas::Base::from_repr(note.rho)
        .into_option()
        .ok_or_else(|| "Invalid rho".to_string())?;

    let esk = pallas::Scalar::from_uniform_bytes(&prf_expand_tag(&note.rseed, 0x04, &note.rho));
    if bool::from(esk.is_zero()) {
        return Err("Invalid rseed (esk is zero)".to_string());
    }

    let rcm = pallas::Scalar::from_uniform_bytes(&prf_expand_tag(&note.rseed, 0x05, &note.rho));
    let psi = pallas::Base::from_uniform_bytes(&prf_expand_tag(&note.rseed, 0x09, &note.rho));

    let mut message = Vec::new();
    message.extend(bytes_to_bits_le(&g_d_bytes));
    message.extend(bytes_to_bits_le(&pk_d_bytes));
    message.extend(u64_to_bits_le(note.value));
    message.extend(base_to_bits_le(&rho, L_ORCHARD_BASE));
    message.extend(base_to_bits_le(&psi, L_ORCHARD_BASE));
    message.extend(bytes_to_bits_le(&asset.to_bytes()));

    let m_prefix = format!("{}-M", NOTE_ZSA_COMMITMENT_PERSONALIZATION);
    let r_prefix = format!("{}-r", NOTE_COMMITMENT_PERSONALIZATION);
    let m_domain = HashDomain::new(&m_prefix);
    let r_base = pallas::Point::hash_to_curve(&r_prefix)(&[]);
    let cm = m_domain
        .hash_to_point(message.into_iter())
        .map(|p| p + (r_base * rcm))
        .into_option()
        .ok_or_else(|| "Issue note commitment failed".to_string())?;

    Ok(extract_p(&cm).to_repr())
}

fn diversify_hash(diversifier: &[u8; 11]) -> Result<pallas::Point, String> {
    let hasher = pallas::Point::hash_to_curve(KEY_DIVERSIFICATION_PERSONALIZATION);
    let mut g_d = hasher(diversifier);
    if bool::from(g_d.is_identity()) {
        g_d = hasher(&[]);
    }
    if bool::from(g_d.is_identity()) {
        return Err("Diversify hash returned identity".to_string());
    }
    Ok(g_d)
}

fn prf_expand_tag(seed: &[u8; 32], tag: u8, rho: &[u8; 32]) -> [u8; 64] {
    let mut t = [0u8; 33];
    t[0] = tag;
    t[1..].copy_from_slice(rho);
    prf_expand(seed, &t)
}

fn prf_expand(seed: &[u8; 32], t: &[u8]) -> [u8; 64] {
    let mut h = Blake2bParams::new()
        .hash_length(64)
        .personal(PRF_EXPAND_PERSONALIZATION)
        .to_state();
    h.update(seed);
    h.update(t);
    let mut out = [0u8; 64];
    out.copy_from_slice(h.finalize().as_bytes());
    out
}

fn bytes_to_bits_le(bytes: &[u8]) -> Vec<bool> {
    let mut bits = Vec::with_capacity(bytes.len() * 8);
    for byte in bytes {
        for i in 0..8 {
            bits.push(((byte >> i) & 1) == 1);
        }
    }
    bits
}

fn base_to_bits_le(value: &pallas::Base, limit: usize) -> Vec<bool> {
    value.to_le_bits().iter().by_vals().take(limit).collect()
}

fn u64_to_bits_le(value: u64) -> Vec<bool> {
    let mut bits = Vec::with_capacity(64);
    for i in 0..64 {
        bits.push(((value >> i) & 1) == 1);
    }
    bits
}

fn extract_p(point: &pallas::Point) -> pallas::Base {
    point
        .to_affine()
        .coordinates()
        .into_option()
        .map(|coords| *coords.x())
        .unwrap_or_else(pallas::Base::zero)
}

fn decode_hex_array<const N: usize>(value: &str) -> Result<[u8; N], String> {
    let trimmed = value.trim_start_matches("0x");
    let bytes = hex::decode(trimmed).map_err(|e| format!("Invalid hex: {}", e))?;
    if bytes.len() != N {
        return Err(format!("Expected {} bytes, got {}", N, bytes.len()));
    }
    let mut out = [0u8; N];
    out.copy_from_slice(&bytes);
    Ok(out)
}
