use base64::engine::general_purpose::STANDARD;
use base64::Engine;
use orchard_service::refiner::{process_extrinsics_witness_aware, PreState, SpentCommitmentProof};
use orchard_service::state::OrchardExtrinsic;
use serde::Deserialize;
use std::fs;
use std::path::{Path, PathBuf};
use utils::functions::RefineContext;

const WORK_PACKAGE_DIRS: &[&str] = &[
    "work_packages",
    "work_packages_small",
    "work_packages_small1",
    "builder/orchard/work_packages",
];

#[derive(Debug, Deserialize)]
struct PayloadFields {
    pre_state_witness_extrinsic: String,
    #[serde(default)]
    post_state_witness_extrinsic: Option<String>,
    bundle_proof_extrinsic: String,
    #[serde(default)]
    pre_state: Option<StateRootsJson>,
    #[serde(default)]
    pre_state_roots: Option<StateRootsJson>,
    #[serde(default)]
    spent_commitment_proofs: Vec<SpentCommitmentProofJson>,
    #[serde(default)]
    metadata: Option<MetadataJson>,
    #[serde(default)]
    gas_limit: Option<u64>,
}

#[derive(Debug, Deserialize)]
struct StateRootsJson {
    commitment_root: [u8; 32],
    commitment_size: u64,
    #[serde(default)]
    commitment_frontier: Vec<[u8; 32]>,
    nullifier_root: [u8; 32],
    #[serde(default)]
    nullifier_size: Option<u64>,
    #[serde(default)]
    transparent_merkle_root: Option<[u8; 32]>,
    #[serde(default)]
    transparent_utxo_root: Option<[u8; 32]>,
    #[serde(default)]
    transparent_utxo_size: Option<u64>,
}

#[derive(Debug, Deserialize)]
struct SpentCommitmentProofJson {
    nullifier: [u8; 32],
    commitment: [u8; 32],
    tree_position: u64,
    branch_siblings: Vec<[u8; 32]>,
}

#[derive(Debug, Deserialize)]
struct MetadataJson {
    #[serde(default)]
    gas_limit: Option<u64>,
    #[serde(default)]
    gas_max: Option<u64>,
}

#[test]
#[ignore]
fn refine_work_packages_offline() {
    let mut packages = collect_work_packages();
    packages.retain(|path| {
        path.file_name()
            .and_then(|name| name.to_str())
            .map(|name| !name.contains("_new"))
            .unwrap_or(true)
    });

    assert!(
        !packages.is_empty(),
        "no work package json files found"
    );

    let refine_context = RefineContext {
        anchor: [0u8; 32],
        state_root: [0u8; 32],
        beefy_root: [0u8; 32],
        lookup_anchor: [0u8; 32],
        lookup_anchor_slot: 0,
        prerequisites: Vec::new(),
    };

    let mut processed = 0usize;
    let mut skipped = 0usize;
    let mut failures = 0usize;
    for path in packages {
        let payload = match load_payload(&path) {
            Ok(payload) => payload,
            Err(err) => {
                println!("⚠️ skipping {}: {}", path.display(), err);
                skipped += 1;
                continue;
            }
        };
        match run_refine(&path, &payload, &refine_context) {
            Ok(()) => {
                println!("✅ refined {}", path.display());
                processed += 1;
            }
            Err(err) => {
                println!("⚠️ refine failed for {}: {}", path.display(), err);
                failures += 1;
            }
        }
    }

    assert!(processed > 0, "no work packages processed");
    if failures > 0 {
        println!(
            "⚠️ refine completed with {} failure(s) and {} skipped package(s)",
            failures, skipped
        );
    }
}

fn collect_work_packages() -> Vec<PathBuf> {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let repo_root = manifest_dir
        .parent()
        .and_then(|path| path.parent())
        .map(Path::to_path_buf)
        .unwrap_or(manifest_dir.clone());
    let mut paths = Vec::new();
    for dir in WORK_PACKAGE_DIRS {
        let dir_path = repo_root.join(dir);
        let entries = match fs::read_dir(&dir_path) {
            Ok(entries) => entries,
            Err(_) => continue,
        };
        for entry in entries.flatten() {
            let path = entry.path();
            if path.extension().and_then(|ext| ext.to_str()) != Some("json") {
                continue;
            }
            paths.push(path);
        }
    }
    paths.sort_by(|a, b| a.to_string_lossy().cmp(&b.to_string_lossy()));
    paths
}

fn load_payload(path: &Path) -> Result<PayloadFields, String> {
    let raw = fs::read_to_string(path).map_err(|err| format!("read failed: {err}"))?;
    let json = extract_json(&raw)?;
    let value: serde_json::Value =
        serde_json::from_str(&json).map_err(|err| format!("json parse failed: {err}"))?;
    let payload_value = value
        .get("payload")
        .and_then(|payload| payload.get("Submit"))
        .cloned()
        .unwrap_or(value);
    serde_json::from_value(payload_value)
        .map_err(|err| format!("payload parse failed: {err}"))
}

fn extract_json(raw: &str) -> Result<String, String> {
    let trimmed = raw.trim_start();
    if trimmed.starts_with('{') {
        return Ok(trimmed.to_string());
    }
    if let Some(pos) = raw.find('{') {
        return Ok(raw[pos..].to_string());
    }
    Err("no json object found in file".to_string())
}

fn run_refine(
    path: &Path,
    payload: &PayloadFields,
    refine_context: &RefineContext,
) -> Result<(), String> {
    let pre_state = build_pre_state(payload)?;

    let mut extrinsics = Vec::new();
    extrinsics.push(decode_extrinsic(
        "pre_state_witness_extrinsic",
        &payload.pre_state_witness_extrinsic,
    )?);
    if let Some(post_state_witness_extrinsic) = payload.post_state_witness_extrinsic.as_deref() {
        let extrinsic = decode_extrinsic(
            "post_state_witness_extrinsic",
            post_state_witness_extrinsic,
        )?;
        match &extrinsic {
            OrchardExtrinsic::PostStateWitness { witness_bytes } => {
                if witness_bytes.len() >= 144 {
                    extrinsics.push(extrinsic);
                } else {
                    println!(
                        "⚠️ skipping legacy post-state witness in {} (len={})",
                        path.display(),
                        witness_bytes.len()
                    );
                }
            }
            _ => extrinsics.push(extrinsic),
        }
    }
    extrinsics.push(decode_extrinsic(
        "bundle_proof_extrinsic",
        &payload.bundle_proof_extrinsic,
    )?);

    extrinsics.push(OrchardExtrinsic::TransparentTxData {
        data_bytes: 0u32.to_le_bytes().to_vec(),
    });

    let mut refine_gas_limit = 10_000_000u64;
    if let Some(gas_limit) = payload.gas_limit {
        refine_gas_limit = refine_gas_limit.max(gas_limit);
    }
    if let Some(metadata) = payload.metadata.as_ref() {
        if let Some(gas_limit) = metadata.gas_limit {
            refine_gas_limit = refine_gas_limit.max(gas_limit);
        }
        if let Some(gas_max) = metadata.gas_max {
            refine_gas_limit = refine_gas_limit.max(gas_max);
        }
    }

    process_extrinsics_witness_aware(
        42,
        refine_context,
        &[0u8; 32],
        refine_gas_limit,
        &pre_state,
        &extrinsics,
        None,
    )
    .map(|_| ())
    .map_err(|err| format!("refine failed for {}: {:?}", path.display(), err))
}

fn build_pre_state(payload: &PayloadFields) -> Result<PreState, String> {
    let roots = payload
        .pre_state
        .as_ref()
        .or(payload.pre_state_roots.as_ref())
        .ok_or_else(|| "missing pre_state/pre_state_roots".to_string())?;

    let spent_commitment_proofs = payload
        .spent_commitment_proofs
        .iter()
        .map(|proof| SpentCommitmentProof {
            nullifier: proof.nullifier,
            commitment: proof.commitment,
            tree_position: proof.tree_position,
            branch_siblings: proof.branch_siblings.clone(),
        })
        .collect();

    Ok(PreState {
        commitment_root: roots.commitment_root,
        commitment_size: roots.commitment_size,
        commitment_frontier: roots.commitment_frontier.clone(),
        nullifier_root: roots.nullifier_root,
        nullifier_size: roots.nullifier_size.unwrap_or(0),
        spent_commitment_proofs,
        transparent_merkle_root: roots.transparent_merkle_root.unwrap_or([0u8; 32]),
        transparent_utxo_root: roots.transparent_utxo_root.unwrap_or([0u8; 32]),
        transparent_utxo_size: roots.transparent_utxo_size.unwrap_or(0),
    })
}

fn decode_extrinsic(label: &str, encoded: &str) -> Result<OrchardExtrinsic, String> {
    let bytes = STANDARD
        .decode(encoded)
        .map_err(|err| format!("{label} base64 decode failed: {err}"))?;
    OrchardExtrinsic::deserialize(&bytes)
        .map_err(|err| format!("{label} deserialize failed: {err}"))
}
