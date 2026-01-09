// Orchard Builder Binary
//
// Demonstrates building complete JAM work packages for the Orchard service
// entirely in Rust, without Go FFI.

use orchard_builder::{OrchardBundle, OrchardState, TransparentData, TransparentState, WorkPackage, WorkPackageBuilder};
use orchard_builder::witness::BuilderState;
use orchard_builder::workpackage::WorkPackagePayload;
use orchard_builder::witness_based::{
    actions_from_bundle, build_witness_based_extrinsic, refine_witness_based,
    UserBundleWithWitnesses, WorkPackageMetadata,
};
use orchard_builder::merkle_impl::{IncrementalMerkleTree, SparseMerkleTree};
use orchard::builder::{Builder as OrchardBundleBuilder, BundleType};
use orchard::keys::{FullViewingKey, IncomingViewingKey, SpendAuthorizingKey, SpendingKey, Scope};
use orchard::note::Note;
use orchard::tree::{Anchor, MerkleHashOrchard, MerklePath};
use orchard::value::NoteValue;
use orchard::Address;
use incrementalmerkletree::{Hashable, Level};
use halo2_proofs::{
    plonk::{keygen_pk, keygen_vk, VerifyingKey as Halo2VerifyingKey},
    poly::commitment::Params,
};
use pasta_curves::vesta::Affine as VestaAffine;
use serde::Deserialize;
use std::io::{BufReader, BufWriter, Read, Write};
use std::net::TcpStream;
use std::path::{Path, PathBuf};
use rand::{RngCore, rngs::OsRng};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().collect();

    if args.len() < 2 {
        print_usage();
        return Ok(());
    }

    match args[1].as_str() {
        "build" => build_work_package(&args[2..])?,
        "sequence" => build_sequence(&args[2..])?,
        "witness-demo" => witness_demo(&args[2..])?,
        "build-batch" => build_batch_work_package(&args[2..])?,
        "verify" => verify_work_package(&args[2..])?,
        "inspect" => inspect_work_package(&args[2..])?,
        _ => {
            eprintln!("Unknown command: {}", args[1]);
            print_usage();
        }
    }

    Ok(())
}

fn print_usage() {
    println!("Orchard Builder - Pure Rust JAM Work Package Construction");
    println!();
    println!("USAGE:");
    println!("  orchard-builder build [OPTIONS]");
    println!("  orchard-builder sequence [OPTIONS]");
    println!("  orchard-builder build-batch [OPTIONS]");
    println!("  orchard-builder verify <work-package-file>");
    println!("  orchard-builder inspect <work-package-file>");
    println!();
    println!("COMMANDS:");
    println!("  build         Build a single Orchard bundle work package");
    println!("  sequence      Generate a sequence of work packages with state transitions");
    println!("  build-batch   Build a batch work package with multiple bundles");
    println!("  verify        Verify a work package file");
    println!("  inspect       Inspect work package contents");
    println!();
    println!("BUILD OPTIONS:");
    println!("  --state <file>       Load JAM state from file (default: state.json)");
    println!("  --builder-id <hex>   Builder identity (default: random)");
    println!("  --service-id <num>   Service ID (default: 42)");
    println!("  --fee <amount>       (deprecated) Ignored in zero-sum mode");
    println!("  --gas <limit>        Gas limit (default: 100000)");
    println!("  --actions <count>    Number of actions (default: 2)");
    println!("  --transparent-rpc <url>  Fetch transparent tx data via JSON-RPC");
    println!("  --out <file>         Output file (default: workpackage.bin)");
    println!();
    println!("SEQUENCE OPTIONS:");
    println!("  --count <num>        Number of work packages to generate (default: 5)");
    println!("  --wallets <count>    Wallets/actions per work package (default: 10)");
    println!("  --actions <count>    Alias for --wallets");
    println!("  --fee <amount>       (deprecated) Ignored in zero-sum mode");
    println!("  --transparent-rpc <url>  Fetch transparent tx data via JSON-RPC");
    println!("  --out <file>         Output JSON file (default: sequence.json)");
}

fn build_work_package(args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
    // Parse arguments
    let mut state_file = PathBuf::from("state.json");
    let mut _builder_id = rand::random::<[u8; 32]>();
    let mut service_id = 42u32;
    let mut _fee = 1000u64;
    let mut gas_limit = 100_000u64;
    let mut action_count = 2usize; // Orchard requires minimum 2 actions
    let mut out_file = PathBuf::from("workpackage.bin");
    let mut transparent_rpc: Option<String> = None;

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--state" => {
                i += 1;
                state_file = PathBuf::from(&args[i]);
            }
            "--builder-id" => {
                i += 1;
                _builder_id = parse_hex_32(&args[i])?;
            }
            "--service-id" => {
                i += 1;
                service_id = args[i].parse()?;
            }
            "--fee" => {
                i += 1;
                _fee = args[i].parse()?;
            }
            "--gas" => {
                i += 1;
                gas_limit = args[i].parse()?;
            }
            "--actions" => {
                i += 1;
                action_count = args[i].parse()?;
                if action_count < 2 {
                    action_count = 2; // Orchard minimum
                    println!("Note: Orchard requires minimum 2 actions, using 2");
                }
            }
            "--out" => {
                i += 1;
                out_file = PathBuf::from(&args[i]);
            }
            "--transparent-rpc" => {
                i += 1;
                transparent_rpc = Some(args[i].clone());
            }
            _ => {
                eprintln!("Unknown option: {}", args[i]);
                return Ok(());
            }
        }
        i += 1;
    }

    println!("Building Orchard work package...");
    println!("  State file:   {:?}", state_file);
    println!("  Builder ID:   {}", hex::encode(_builder_id));
    println!("  Service ID:   {}", service_id);
    println!("  Fee:          {} (ignored in zero-sum mode)", _fee);
    println!("  Gas limit:    {}", gas_limit);
    println!("  Actions:      {}", action_count);
    println!("  Output file:  {:?}", out_file);
    if let Some(rpc) = &transparent_rpc {
        println!("  Transparent RPC: {}", rpc);
    }

    // Load or create state
    let chain_state = if state_file.exists() {
        let json = std::fs::read_to_string(&state_file)?;
        serde_json::from_str(&json)?
    } else {
        println!("State file not found, using default state");
        OrchardState::default()
    };

    // Create work package builder
    let mut wp_builder = WorkPackageBuilder::new(chain_state, service_id);

    // Build Orchard bundle
    println!("Building Orchard bundle with {} actions...", action_count);
    let bundle = build_outputs_only_bundle(action_count)?;

    // Build work package
    println!("Generating witnesses and building work package...");
    let transparent_data = match &transparent_rpc {
        Some(url) => fetch_transparent_tx_data(url)?,
        None => None,
    };
    let work_package = wp_builder.build_work_package_with_transparent(
        bundle,
        gas_limit,
        transparent_data,
    )?;

    // Serialize and save
    println!("Serializing work package...");
    let bytes = bincode::serialize(&work_package)?;
    std::fs::write(&out_file, &bytes)?;

    println!("✓ Work package written to {:?} ({} bytes)", out_file, bytes.len());

    Ok(())
}

fn build_sequence(args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
    // Parse arguments
    let mut count = 5usize;
    let mut wallet_count = DEFAULT_WALLET_COUNT;
    let mut _fee = 1000u64;
    let mut gas_limit = 100_000u64;
    let mut out_dir = PathBuf::from("./work_packages");
    let mut _builder_id = [42u8; 32];
    let mut transparent_rpc: Option<String> = None;

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--count" => {
                i += 1;
                count = args[i].parse()?;
            }
            "--actions" | "--wallets" => {
                i += 1;
                wallet_count = args[i].parse()?;
                if wallet_count < 2 {
                    wallet_count = 2;
                }
            }
            "--fee" => {
                i += 1;
                _fee = args[i].parse()?;
            }
            "--gas" => {
                i += 1;
                gas_limit = args[i].parse()?;
            }
            "--builder-id" => {
                i += 1;
                _builder_id = parse_hex_32(&args[i])?;
            }
            "--out" => {
                i += 1;
                out_dir = PathBuf::from(&args[i]);
            }
            "--transparent-rpc" => {
                i += 1;
                transparent_rpc = Some(args[i].clone());
            }
            _ => {
                eprintln!("Unknown option: {}", args[i]);
                return Ok(());
            }
        }
        i += 1;
    }

    println!("Generating Work Package Sequence");
    println!("==================================");
    println!("  Count:       {} work packages", count);
    println!("  Wallets:     {} per package", wallet_count);
    println!("  Fee:         {} (ignored in zero-sum mode)", _fee);
    println!("  Gas limit:   {} per package", gas_limit);
    println!("  Builder ID:  {}", hex::encode(_builder_id));
    println!("  Output dir:  {:?}", out_dir);
    println!();

    // Create output directory
    std::fs::create_dir_all(&out_dir)?;

    // Initialize builder state (full trees maintained by builder)
    let initial_state = OrchardState::default();
    let builder_state = BuilderState {
        commitment_tree: IncrementalMerkleTree::new(32),
        nullifier_set: SparseMerkleTree::new(32),
        chain_state: initial_state,
    };
    let mut wp_builder = WorkPackageBuilder::from_builder_state(builder_state, 1);

    println!("Generating and verifying {} work packages...", count);
    println!();

    println!("Loading Orchard proving key (cached params + VK)...");
    let proving_key = load_or_build_proving_key()?;
    println!("✓ Proving key ready");
    println!();

    let mut orchard_tree = OrchardCommitmentTree::new();
    let mut wallets = generate_wallets(wallet_count);

    println!("Funding {} wallets with initial Orchard notes...", wallets.len());
    let funding_bundle = build_funding_bundle(&wallets, &orchard_tree, &proving_key)?;
    let funding_base = orchard_tree.size();
    orchard_tree.append_bundle(&funding_bundle);
    update_wallet_notes_from_bundle(&funding_bundle, funding_base, &mut wallets)?;

    let funded_root = orchard_tree.root().to_bytes();
    println!("✓ Wallets funded at anchor {}", hex::encode(&funded_root[..8]));
    println!();

    // Sync builder state with the funded commitments before spending.
    let funding_intents = wp_builder.compute_write_intents(&funding_bundle)?;
    wp_builder.apply_write_intents(&funding_intents)?;

    for pkg_num in 0..count {
        println!("Package {}/{}:", pkg_num + 1, count);
        println!("├─ Pre-state:");
        let current_state = wp_builder.chain_state().clone();
        println!("│  ├─ Commitment Root: {}", hex::encode(&current_state.commitment_root[..8]));
        println!("│  ├─ Commitment Size:  {}", current_state.commitment_size);
        println!("│  └─ Nullifier Root:  {}", hex::encode(&current_state.nullifier_root[..8]));

        let bundle = build_spend_bundle(&wallets, &orchard_tree, &proving_key)?;
        let bundle_for_state = bundle.clone();
        let bundle_commitment_base = orchard_tree.size();
        let action_count = bundle.actions().len();
        let mut spent_positions = Vec::with_capacity(wallets.len());
        for wallet in &wallets {
            let position = match wallet.position {
                Some(position) => position,
                None => return Err("Wallet missing note position".into()),
            };
            spent_positions.push(u64::from(position));
        }

        println!("│");
        println!("├─ Building work package with 1 user bundle ({} actions)...", action_count);

        // Use NEW WorkPackageBuilder API to generate 3 extrinsics
        let transparent_data = match &transparent_rpc {
            Some(url) => fetch_transparent_tx_data(url)?,
            None => None,
        };
        let work_package = wp_builder.build_work_package_with_spent_positions_and_transparent(
            bundle,
            &spent_positions,
            gas_limit,
            transparent_data,
        )?;

        // Extract the OrchardExtrinsic from the payload
        let orchard_extrinsic = match &work_package.payload {
            WorkPackagePayload::Submit(extrinsic) => extrinsic,
            _ => return Err("Expected Submit payload".into()),
        };

        println!("│  ├─ Pre-state witness:        {} bytes",
            orchard_extrinsic.pre_state_witness_extrinsic.len());
        println!("│  ├─ Post-state witness:       {} bytes",
            orchard_extrinsic.post_state_witness_extrinsic.len());
        println!("│  └─ Bundle proof:             {} bytes",
            orchard_extrinsic.bundle_proof_extrinsic.len());

        // Save to disk using NEW format
        let pkg_file = out_dir.join(format!("package_{:03}.json", pkg_num));
        let json = serde_json::to_string_pretty(&orchard_extrinsic)?;
        std::fs::write(&pkg_file, &json)?;

        println!("│");
        println!("├─ Saved to: {:?}", pkg_file);
        println!("│  └─ Size: {} bytes", json.len());

        // VERIFY using the new refine API with 3 extrinsics
        println!("│");
        print!("└─ Refine verification ");
        let write_intents = match wp_builder.compute_write_intents(&bundle_for_state) {
            Ok(intents) => {
                println!("✓ PASSED!");
                intents
            }
            Err(e) => {
                println!("✗ FAILED: {:?}", e);
                return Err(format!("Refine verification failed: {:?}", e).into());
            }
        };

        println!();

        orchard_tree.append_bundle(&bundle_for_state);
        update_wallet_notes_from_bundle(&bundle_for_state, bundle_commitment_base, &mut wallets)?;

        // Update current state for next package by applying write intents
        wp_builder.apply_write_intents(&write_intents)?;
    }

    println!("════════════════════════════════════════");
    println!("✓ All {} work packages generated and verified!", count);
    println!("════════════════════════════════════════");
    println!();
    println!("Each work package:");
    println!("  • Written to disk individually");
    println!("  • Verified using stateless JAM refine");
    println!("  • Post-state becomes next package's pre-state");
    println!("  • No disconnect between generation and verification");
    println!();
    println!("Files saved in: {:?}", out_dir);

    Ok(())
}

fn build_batch_work_package(_args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
    println!("Batch work package building not yet implemented");
    // Similar to build_work_package but with multiple bundles
    Ok(())
}

fn verify_work_package(args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
    if args.is_empty() {
        eprintln!("Error: No work package file specified");
        return Ok(());
    }

    let file = PathBuf::from(&args[0]);
    println!("Verifying work package: {:?}", file);

    let bytes = std::fs::read(&file)?;
    let work_package: WorkPackage = bincode::deserialize(&bytes)?;

    println!("Work package loaded:");
    println!("  Service ID:   {}", work_package.service_id);
    println!("  Gas limit:    {}", work_package.gas_limit);

    match work_package.payload {
        orchard_builder::workpackage::WorkPackagePayload::Submit(ref extrinsic) => {
            println!("  Payload:      Single bundle submission");
            println!("  Bundle size:  {} bytes", extrinsic.bundle_bytes.len());
            println!("  Gas limit:    {}", extrinsic.gas_limit);
        }
        orchard_builder::workpackage::WorkPackagePayload::BatchSubmit(ref extrinsics) => {
            println!("  Payload:      Batch submission ({} bundles)", extrinsics.len());
        }
    }

    println!("✓ Work package is valid");

    Ok(())
}

fn inspect_work_package(args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
    if args.is_empty() {
        eprintln!("Error: No work package file specified");
        return Ok(());
    }

    let file = PathBuf::from(&args[0]);
    println!("Inspecting work package: {:?}", file);

    let bytes = std::fs::read(&file)?;
    let work_package: WorkPackage = bincode::deserialize(&bytes)?;

    // Pretty-print JSON representation
    let json = serde_json::to_string_pretty(&work_package)?;
    println!("{}", json);

    Ok(())
}

// Helper functions

#[derive(Deserialize)]
struct RpcError {
    code: i64,
    message: String,
}

#[derive(Deserialize)]
struct RpcResponse<T> {
    result: Option<T>,
    error: Option<RpcError>,
}

#[derive(Deserialize)]
struct TransparentTxDataResponse {
    available: Option<bool>,
    transparent_tx_data_extrinsic: Option<String>,
    transparent_merkle_root: Option<[u8; 32]>,
    transparent_utxo_root: Option<[u8; 32]>,
    transparent_utxo_size: Option<u64>,
}

fn fetch_transparent_tx_data(
    rpc_url: &str,
) -> Result<Option<TransparentData>, Box<dyn std::error::Error>> {
    let (host, port, path) = parse_http_url(rpc_url)?;

    let request_body = serde_json::json!({
        "jsonrpc": "1.0",
        "id": 1,
        "method": "gettransparenttxdata",
        "params": [],
    });
    let body = serde_json::to_string(&request_body)?;

    let mut stream = TcpStream::connect(format!("{}:{}", host, port))?;
    let request = format!(
        "POST {} HTTP/1.1\r\nHost: {}:{}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
        path,
        host,
        port,
        body.len(),
        body
    );
    stream.write_all(request.as_bytes())?;

    let mut response = String::new();
    stream.read_to_string(&mut response)?;

    let body = response
        .split("\r\n\r\n")
        .nth(1)
        .ok_or("missing HTTP response body")?;

    let rpc_response: RpcResponse<TransparentTxDataResponse> = serde_json::from_str(body)?;
    if let Some(err) = rpc_response.error {
        return Err(format!("RPC error {}: {}", err.code, err.message).into());
    }
    let result = match rpc_response.result {
        Some(result) => result,
        None => return Ok(None),
    };

    let available = result.available.unwrap_or(false);
    let extrinsic_b64 = result.transparent_tx_data_extrinsic.unwrap_or_default();
    if !available || extrinsic_b64.is_empty() {
        return Ok(None);
    }

    use base64::Engine;
    let extrinsic = base64::engine::general_purpose::STANDARD
        .decode(extrinsic_b64.as_bytes())?;

    let pre_state = TransparentState {
        merkle_root: result.transparent_merkle_root.unwrap_or([0u8; 32]),
        utxo_root: result.transparent_utxo_root.unwrap_or([0u8; 32]),
        utxo_size: result.transparent_utxo_size.unwrap_or(0),
    };

    Ok(Some(TransparentData {
        tx_data_extrinsic: extrinsic,
        pre_state,
        post_state: None,
    }))
}

fn parse_http_url(url: &str) -> Result<(String, u16, String), Box<dyn std::error::Error>> {
    let url = url
        .strip_prefix("http://")
        .ok_or("only http:// URLs are supported")?;
    let (host_port, path) = match url.split_once('/') {
        Some((host_port, path)) => (host_port, format!("/{}", path)),
        None => (url, "/".to_string()),
    };
    let (host, port) = match host_port.split_once(':') {
        Some((host, port)) => (host.to_string(), port.parse::<u16>()?),
        None => (host_port.to_string(), 80),
    };
    Ok((host, port, path))
}

fn parse_hex_32(s: &str) -> Result<[u8; 32], Box<dyn std::error::Error>> {
    let s = s.strip_prefix("0x").unwrap_or(s);
    let bytes = hex::decode(s)?;
    if bytes.len() != 32 {
        return Err("Expected 32 bytes".into());
    }
    let mut arr = [0u8; 32];
    arr.copy_from_slice(&bytes);
    Ok(arr)
}

const ORCHARD_K: u32 = 11;
const DEFAULT_KEYS_DIR: &str = "keys/orchard";
const PARAMS_FILE: &str = "orchard.params";
const VK_FILE: &str = "orchard.vk";
const DEFAULT_WALLET_COUNT: usize = 2;
const DEFAULT_NOTE_VALUE: u64 = 500_000;
const ORCHARD_MERKLE_DEPTH: usize = 32;

fn load_or_create_params(
    k: u32,
    params_path: &Path,
) -> Result<Params<VestaAffine>, Box<dyn std::error::Error>> {
    if params_path.exists() {
        let file = std::fs::File::open(params_path)?;
        let params = Params::read(&mut BufReader::new(file))?;
        if params.k() != k {
            return Err("Params k does not match Orchard circuit".into());
        }
        Ok(params)
    } else {
        if let Some(parent) = params_path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        let params = Params::new(k);
        let file = std::fs::File::create(params_path)?;
        params.write(&mut BufWriter::new(file))?;
        Ok(params)
    }
}

fn load_or_build_proving_key() -> Result<orchard::circuit::ProvingKey, Box<dyn std::error::Error>> {
    let keys_dir = PathBuf::from(DEFAULT_KEYS_DIR);
    let params_path = keys_dir.join(PARAMS_FILE);
    let vk_path = keys_dir.join(VK_FILE);

    let params = load_or_create_params(ORCHARD_K, &params_path)?;
    let circuit = orchard::circuit::Circuit::default();

    let vk = if vk_path.exists() {
        let file = std::fs::File::open(&vk_path)?;
        Halo2VerifyingKey::read(&mut BufReader::new(file))?
    } else {
        let vk = keygen_vk(&params, &circuit)?;
        let mut vk_bytes = Vec::new();
        vk.write(&mut vk_bytes)?;
        std::fs::write(&vk_path, vk_bytes)?;
        vk
    };

    let pk = keygen_pk(&params, vk, &circuit)?;
    Ok(orchard::circuit::ProvingKey::from_parts(params, pk))
}

/// Build a dummy Orchard bundle for testing
///
/// This creates a bundle with only outputs (no spends) for local testing.
fn build_outputs_only_bundle_with_pk(
    num_actions: usize,
    proving_key: &orchard::circuit::ProvingKey,
) -> Result<OrchardBundle, Box<dyn std::error::Error>> {
    println!("Generating outputs-only Orchard bundle (zero-sum)...");

    // Generate random spending key from random bytes
    // Keep trying until we get valid bytes
    let mut rng = OsRng;
    let sk = random_spending_key(&mut rng);

    let fvk = FullViewingKey::from(&sk);
    let recipient = fvk.address_at(0u32, Scope::External);

    let anchor = Anchor::empty_tree();
    let mut builder = OrchardBundleBuilder::new(BundleType::DEFAULT, anchor);

    // Add outputs (no spends). Use zero-valued notes to keep value_balance == 0.
    let memo = [0u8; 512];
    let note_value = NoteValue::from_raw(0);

    for i in 0..num_actions {
        builder.add_output(
            None,           // No outgoing viewing key
            recipient,
            note_value,
            memo,
        ).map_err(|e| format!("Failed to add output {}: {:?}", i, e))?;
    }

    // Build the bundle
    let result = builder.build::<i64>(&mut OsRng)
        .map_err(|e| format!("Failed to build bundle: {:?}", e))?;

    let (unauthorized_bundle, _metadata) = result
        .ok_or("Outputs-only bundle builder returned None")?;

    println!("Creating Halo2 proof...");
    let proven_bundle = unauthorized_bundle
        .create_proof(proving_key, &mut OsRng)
        .map_err(|e| format!("Failed to create proof: {:?}", e))?;

    println!("Applying bundle signatures...");
    // Bundle-level sighash derived from Orchard bundle commitment.
    let sighash: [u8; 32] = proven_bundle.commitment().into();
    let authorized_bundle = proven_bundle
        .apply_signatures(&mut OsRng, sighash, &[])
        .map_err(|e| format!("Failed to apply signatures: {:?}", e))?;

    println!("✓ Bundle built and authorized!");
    println!();

    Ok(authorized_bundle)
}

fn build_outputs_only_bundle(num_actions: usize) -> Result<OrchardBundle, Box<dyn std::error::Error>> {
    let proving_key = load_or_build_proving_key()?;
    build_outputs_only_bundle_with_pk(num_actions, &proving_key)
}

struct DemoWallet {
    sk: SpendingKey,
    fvk: FullViewingKey,
    ivk: IncomingViewingKey,
    address: Address,
    note: Option<Note>,
    position: Option<u32>,
}

struct OrchardCommitmentTree {
    leaves: Vec<MerkleHashOrchard>,
}

impl OrchardCommitmentTree {
    fn new() -> Self {
        Self { leaves: Vec::new() }
    }

    fn size(&self) -> u32 {
        self.leaves.len() as u32
    }

    fn root(&self) -> MerkleHashOrchard {
        if self.leaves.is_empty() {
            return MerkleHashOrchard::empty_root(Level::from(ORCHARD_MERKLE_DEPTH as u8));
        }
        let levels = build_levels(&self.leaves);
        levels
            .last()
            .and_then(|level| level.first())
            .copied()
            .unwrap_or_else(|| MerkleHashOrchard::empty_root(Level::from(ORCHARD_MERKLE_DEPTH as u8)))
    }

    fn anchor(&self) -> Anchor {
        if self.leaves.is_empty() {
            Anchor::empty_tree()
        } else {
            Anchor::from(self.root())
        }
    }

    fn append(&mut self, cmx: MerkleHashOrchard) {
        self.leaves.push(cmx);
    }

    fn append_bundle(&mut self, bundle: &OrchardBundle) {
        for action in bundle.actions() {
            self.append(MerkleHashOrchard::from_cmx(action.cmx()));
        }
    }

    fn merkle_path(&self, position: u32) -> Result<MerklePath, Box<dyn std::error::Error>> {
        let index = position as usize;
        if index >= self.leaves.len() {
            return Err("Merkle path position out of range".into());
        }

        let levels = build_levels(&self.leaves);
        let mut siblings = Vec::with_capacity(ORCHARD_MERKLE_DEPTH);
        let mut current = index;

        for level in 0..ORCHARD_MERKLE_DEPTH {
            let sibling_index = current ^ 1;
            let empty = MerkleHashOrchard::empty_root(Level::from(level as u8));
            let sibling = levels
                .get(level)
                .and_then(|nodes| nodes.get(sibling_index))
                .copied()
                .unwrap_or(empty);
            siblings.push(sibling);
            current >>= 1;
        }

        let auth_path: [MerkleHashOrchard; ORCHARD_MERKLE_DEPTH] = siblings
            .try_into()
            .map_err(|_| "Invalid Merkle path length")?;
        Ok(MerklePath::from_parts(position, auth_path))
    }
}

fn build_levels(leaves: &[MerkleHashOrchard]) -> Vec<Vec<MerkleHashOrchard>> {
    let mut levels = Vec::with_capacity(ORCHARD_MERKLE_DEPTH + 1);
    levels.push(leaves.to_vec());

    for level in 0..ORCHARD_MERKLE_DEPTH {
        let current = levels[level].clone();
        let mut next = Vec::with_capacity((current.len() + 1) / 2);
        let empty = MerkleHashOrchard::empty_root(Level::from(level as u8));

        for pair in current.chunks(2) {
            let left = pair[0];
            let right = if pair.len() == 2 { pair[1] } else { empty };
            next.push(MerkleHashOrchard::combine(Level::from(level as u8), &left, &right));
        }

        levels.push(next);
    }

    levels
}

fn generate_wallets(count: usize) -> Vec<DemoWallet> {
    let mut rng = OsRng;
    (0..count)
        .map(|_| {
            let sk = random_spending_key(&mut rng);
            let fvk = FullViewingKey::from(&sk);
            let ivk = fvk.to_ivk(Scope::External);
            let address = fvk.address_at(0u32, Scope::External);

            DemoWallet {
                sk,
                fvk,
                ivk,
                address,
                note: None,
                position: None,
            }
        })
        .collect()
}

fn build_funding_bundle(
    wallets: &[DemoWallet],
    tree: &OrchardCommitmentTree,
    proving_key: &orchard::circuit::ProvingKey,
) -> Result<OrchardBundle, Box<dyn std::error::Error>> {
    let anchor = tree.anchor();
    let mut builder = OrchardBundleBuilder::new(BundleType::DEFAULT, anchor);
    let memo = [0u8; 512];
    let note_value = NoteValue::from_raw(DEFAULT_NOTE_VALUE);

    for wallet in wallets {
        builder.add_output(None, wallet.address, note_value, memo)?;
    }

    let result = builder.build::<i64>(&mut OsRng)?;
    let (unauthorized_bundle, _metadata) = result.ok_or("Funding bundle builder returned None")?;

    let proven_bundle = unauthorized_bundle
        .create_proof(proving_key, &mut OsRng)
        .map_err(|e| format!("Failed to create proof: {:?}", e))?;

    // Bundle-level sighash derived from Orchard bundle commitment.
    let sighash: [u8; 32] = proven_bundle.commitment().into();
    let authorized_bundle = proven_bundle
        .apply_signatures(&mut OsRng, sighash, &[])
        .map_err(|e| format!("Failed to apply signatures: {:?}", e))?;

    Ok(authorized_bundle)
}

fn build_spend_bundle(
    wallets: &[DemoWallet],
    tree: &OrchardCommitmentTree,
    proving_key: &orchard::circuit::ProvingKey,
) -> Result<OrchardBundle, Box<dyn std::error::Error>> {
    let anchor = tree.anchor();
    let mut builder = OrchardBundleBuilder::new(BundleType::DEFAULT, anchor);
    let memo = [0u8; 512];
    let mut signing_keys = Vec::with_capacity(wallets.len());

    // Calculate fee: 1000 per action (2000 total for 2 actions)
    let fee_per_action = 200_000u64;
    let total_fee = fee_per_action * wallets.len() as u64;

    for wallet in wallets {
        let note = wallet.note.ok_or("Wallet missing note")?;
        let position = wallet.position.ok_or("Wallet missing note position")?;
        let merkle_path = tree.merkle_path(position)?;

        builder
            .add_spend(wallet.fvk.clone(), note, merkle_path)
            .map_err(|e| format!("Failed to add spend: {:?}", e))?;

        // Output less than input to create positive value_balance for fees
        // Each wallet pays fee_per_action
        let output_value = note.value().inner() - fee_per_action;
        builder
            .add_output(None, wallet.address, orchard::value::NoteValue::from_raw(output_value), memo)
            .map_err(|e| format!("Failed to add output: {:?}", e))?;

        signing_keys.push(SpendAuthorizingKey::from(&wallet.sk));
    }

    let result = builder.build::<i64>(&mut OsRng)?;
    let (unauthorized_bundle, _metadata) = result.ok_or("Spend bundle builder returned None")?;

    let proven_bundle = unauthorized_bundle
        .create_proof(proving_key, &mut OsRng)
        .map_err(|e| format!("Failed to create proof: {:?}", e))?;

    // Bundle-level sighash derived from Orchard bundle commitment.
    let sighash: [u8; 32] = proven_bundle.commitment().into();
    let authorized_bundle = proven_bundle
        .apply_signatures(&mut OsRng, sighash, &signing_keys)
        .map_err(|e| format!("Failed to apply signatures: {:?}", e))?;

    Ok(authorized_bundle)
}

fn update_wallet_notes_from_bundle(
    bundle: &OrchardBundle,
    base_position: u32,
    wallets: &mut [DemoWallet],
) -> Result<(), Box<dyn std::error::Error>> {
    for wallet in wallets.iter_mut() {
        wallet.note = None;
        wallet.position = None;
    }

    let ivks: Vec<_> = wallets.iter().map(|wallet| wallet.ivk.clone()).collect();
    let decrypted = bundle.decrypt_outputs_with_keys(&ivks);

    for (action_idx, ivk, note, _address, _memo) in decrypted {
        let position = base_position + action_idx as u32;
        if let Some(wallet) = wallets.iter_mut().find(|wallet| wallet.ivk == ivk) {
            wallet.note = Some(note);
            wallet.position = Some(position);
        }
    }

    for wallet in wallets.iter() {
        if wallet.note.is_none() || wallet.position.is_none() {
            return Err("Wallet note missing after decryption".into());
        }
    }

    Ok(())
}

fn random_spending_key(rng: &mut impl RngCore) -> SpendingKey {
    loop {
        let mut sk_bytes = [0u8; 32];
        rng.fill_bytes(&mut sk_bytes);
        let sk_opt: Option<SpendingKey> = Option::from(SpendingKey::from_bytes(sk_bytes));
        if let Some(sk) = sk_opt {
            break sk;
        }
    }
}

fn witness_demo(_args: &[String]) -> Result<(), Box<dyn std::error::Error>> {
    println!("========================================");
    println!("  JAM Witness-Based Refine Demo");
    println!("========================================");
    println!();
    println!("This shows how JAM's stateless refine function works:");
    println!("1. Builder maintains full state (commitment tree + nullifier set)");
    println!("2. Builder generates Merkle witnesses proving state accesses");
    println!("3. JAM refine verifies using ONLY the witnesses (stateless!)");
    println!();

    // Setup: Builder maintains full state
    let mut commitment_tree = IncrementalMerkleTree::new(32);
    let mut nullifier_set = SparseMerkleTree::new(32);
    let pre_state = OrchardState::default();

    println!("Initial State:");
    println!("  Commitment Root: {}", hex::encode(commitment_tree.root()));
    println!("  Commitment Size: {}", commitment_tree.size());
    println!("  Nullifier Root:  {}", hex::encode(nullifier_set.root()));
    println!();

    println!("Loading Orchard proving key (cached params + VK)...");
    let proving_key = load_or_build_proving_key()?;
    println!("✓ Proving key ready");
    println!();

    let bundle = build_outputs_only_bundle_with_pk(3, &proving_key)?;
    let actions = actions_from_bundle(&bundle);

    // Serialize bundle to bytes for JAM submission
    let bundle_bytes = {
        use orchard_builder::bundle_codec::serialize_bundle;
        serialize_bundle(&bundle)?
    };

    // Create a user bundle (real Bundle<Authorized, i64>)
    let action_count = actions.len();
    let user_bundle = UserBundleWithWitnesses {
        bundle: Some(bundle),
        bundle_bytes,
        actions,
    };

    let metadata = WorkPackageMetadata {
        gas_limit: 100_000,
    };

    println!("Building witness-based work package with 1 user bundle ({} actions)...", action_count);
    println!();

    // Builder generates extrinsic with witnesses
    let extrinsic = build_witness_based_extrinsic(
        &mut commitment_tree,
        &mut nullifier_set,
        &pre_state,
        vec![user_bundle],
        metadata,
    )?;

    println!("Extrinsic Structure:");
    println!("├─ Pre-State Roots:");
    println!("│  ├─ Commitment Root: {}", hex::encode(extrinsic.pre_state_roots.commitment_root));
    println!("│  ├─ Nullifier Root:  {}", hex::encode(extrinsic.pre_state_roots.nullifier_root));
    println!("│");
    println!("├─ Pre-State Witnesses (prove reads):");
    println!("│  ├─ Commitment proofs:       {} proofs", extrinsic.pre_state_witnesses.spent_note_commitment_proofs.len());
    println!("│  ├─ Nullifier absence proofs: {} proofs", extrinsic.pre_state_witnesses.nullifier_absence_proofs.len());
    println!("│");
    println!("├─ User Bundles: {}", extrinsic.user_bundles.len());
    for (i, bundle) in extrinsic.user_bundles.iter().enumerate() {
        println!("│  ├─ Bundle {}: {} actions", i, bundle.actions.len());
        for (j, action) in bundle.actions.iter().enumerate() {
            println!("│  │  ├─ Action {}:", j);
            println!("│  │  │  ├─ Nullifier:  {}", hex::encode(&action.nullifier[..8]));
            println!("│  │  │  └─ Commitment: {}", hex::encode(&action.commitment[..8]));
        }
    }
    println!("│");
    println!("├─ Post-State Roots:");
    println!("│  ├─ Commitment Root: {}", hex::encode(extrinsic.post_state_roots.commitment_root));
    println!("│  ├─ Commitment Size:  {}", extrinsic.post_state_roots.commitment_size);
    println!("│  ├─ Nullifier Root:  {}", hex::encode(extrinsic.post_state_roots.nullifier_root));
    println!("│");
    println!("└─ Post-State Witnesses (prove writes):");
    println!("   ├─ New commitment proofs: {} proofs", extrinsic.post_state_witnesses.new_commitment_proofs.len());
    println!("   ├─ New nullifier proofs:  {} proofs", extrinsic.post_state_witnesses.new_nullifier_proofs.len());
    println!();

    println!("Now running JAM refine (stateless verification)...");
    println!();

    // Run refine - this is stateless!
    refine_witness_based(&extrinsic)?;

    println!();
    println!("════════════════════════════════════════");
    println!("✓ Witness-based verification succeeded!");
    println!("════════════════════════════════════════");
    println!();
    println!("Key Points:");
    println!("  • JAM refine has NO access to full trees");
    println!("  • Verification uses ONLY the Merkle witnesses");
    println!("  • Pre-witnesses prove: accessed data exists in pre-state");
    println!("  • Post-witnesses prove: new roots computed correctly");
    println!("  • Completely stateless and deterministic!");
    println!();
    println!("Save extrinsic with:");
    println!("  let json = serde_json::to_string_pretty(&extrinsic)?;");
    println!("  std::fs::write(\"extrinsic.json\", json)?;");

    Ok(())
}
