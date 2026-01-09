#![cfg_attr(any(target_arch = "riscv32", target_arch = "riscv64"), no_std)]

#[cfg(any(target_arch = "riscv32", target_arch = "riscv64"))]
fn main() {}

#[cfg(not(any(target_arch = "riscv32", target_arch = "riscv64")))]
mod host {
    use std::env;
    use std::fs;
    use std::process::exit;

    use orchard_service::crypto::verify_halo2_proof;
    use orchard_service::state::OrchardExtrinsic;

    pub fn run() {
        let args: Vec<String> = env::args().collect();
        if args.len() == 1 || args.iter().any(|arg| arg == "-h" || arg == "--help") {
            print_usage();
            return;
        }

        let mut vk_id: u32 = 1;
        let mut proof_hex: Option<String> = None;
        let mut proof_hex_file: Option<String> = None;
        let mut inputs_hex: Option<String> = None;
        let mut inputs_hex_file: Option<String> = None;
        let mut extrinsic_hex: Option<String> = None;
        let mut extrinsic_hex_file: Option<String> = None;

        let mut i = 1;
        while i < args.len() {
            match args[i].as_str() {
                "--vk-id" => {
                    i += 1;
                    vk_id = parse_u32(args.get(i), "--vk-id");
                }
                "--proof-hex" => {
                    i += 1;
                    proof_hex = args.get(i).cloned();
                }
                "--proof-hex-file" => {
                    i += 1;
                    proof_hex_file = args.get(i).cloned();
                }
                "--public-inputs-hex" => {
                    i += 1;
                    inputs_hex = args.get(i).cloned();
                }
                "--public-inputs-hex-file" => {
                    i += 1;
                    inputs_hex_file = args.get(i).cloned();
                }
                "--bundle-proof-extrinsic-hex" => {
                    i += 1;
                    extrinsic_hex = args.get(i).cloned();
                }
                "--bundle-proof-extrinsic-hex-file" => {
                    i += 1;
                    extrinsic_hex_file = args.get(i).cloned();
                }
                other => {
                    eprintln!("Unknown argument: {other}");
                    print_usage();
                    exit(2);
                }
            }
            i += 1;
        }

        let extrinsic_hex = load_optional_text(extrinsic_hex, extrinsic_hex_file.as_deref());
        let proof_hex = load_optional_text(proof_hex, proof_hex_file.as_deref());
        let inputs_hex = load_optional_text(inputs_hex, inputs_hex_file.as_deref());

        let (vk_id, public_inputs, proof_bytes) = if let Some(extrinsic_hex) = extrinsic_hex {
            if proof_hex.is_some() || inputs_hex.is_some() {
                eprintln!("Provide either a BundleProof extrinsic or proof/public inputs, not both.");
                exit(2);
            }
            parse_from_extrinsic(&extrinsic_hex)
        } else {
            let proof_hex = proof_hex.unwrap_or_else(|| {
                eprintln!("Missing --proof-hex or --proof-hex-file");
                print_usage();
                exit(2);
            });
            let inputs_hex = inputs_hex.unwrap_or_else(|| {
                eprintln!("Missing --public-inputs-hex or --public-inputs-hex-file");
                print_usage();
                exit(2);
            });
            let proof_bytes = parse_hex_blob("proof", &proof_hex);
            let public_inputs = parse_public_inputs(&inputs_hex);
            (vk_id, public_inputs, proof_bytes)
        };

        println!(
            "Host Halo2 verify: vk_id={}, proof_len={}, public_inputs_len={}",
            vk_id,
            proof_bytes.len(),
            public_inputs.len()
        );

        match verify_halo2_proof(&proof_bytes, &public_inputs, vk_id) {
            Ok(true) => {
                println!("Halo2 proof verified ✅");
                exit(0);
            }
            Ok(false) => {
                println!("Halo2 proof rejected ❌");
                exit(1);
            }
            Err(err) => {
                eprintln!("Halo2 verification error: {err:?}");
                exit(1);
            }
        }
    }

    fn print_usage() {
        println!(
            "Usage:\n  \
halo2-verify-host --vk-id 1 --proof-hex <hex> --public-inputs-hex <hex1,hex2,...>\n  \
halo2-verify-host --bundle-proof-extrinsic-hex <hex>\n\n\
Options:\n  \
--vk-id <u32>                              (default: 1)\n  \
--proof-hex <hex>                          proof bytes as hex\n  \
--proof-hex-file <path>                    proof bytes hex file\n  \
--public-inputs-hex <hex1,hex2,...>        comma/whitespace separated 32-byte hex values\n  \
--public-inputs-hex-file <path>            file containing comma/whitespace separated inputs\n  \
--bundle-proof-extrinsic-hex <hex>         raw BundleProof extrinsic hex (tag=3)\n  \
--bundle-proof-extrinsic-hex-file <path>   raw BundleProof extrinsic hex file\n"
        );
    }

    fn parse_u32(value: Option<&String>, flag: &str) -> u32 {
        let Some(value) = value else {
            eprintln!("Missing value for {flag}");
            print_usage();
            exit(2);
        };
        value.parse().unwrap_or_else(|_| {
            eprintln!("Invalid u32 for {flag}: {value}");
            exit(2);
        })
    }

    fn load_optional_text(inline: Option<String>, file: Option<&str>) -> Option<String> {
        if let Some(text) = inline {
            return Some(text);
        }
        let Some(path) = file else {
            return None;
        };
        let data = fs::read_to_string(path).unwrap_or_else(|err| {
            eprintln!("Failed to read {path}: {err}");
            exit(2);
        });
        Some(data)
    }

    fn parse_from_extrinsic(extrinsic_hex: &str) -> (u32, Vec<[u8; 32]>, Vec<u8>) {
        let bytes = parse_hex_blob("bundle-proof-extrinsic", extrinsic_hex);
        let extrinsic = OrchardExtrinsic::deserialize(&bytes).unwrap_or_else(|err| {
            eprintln!("Failed to deserialize BundleProof extrinsic: {err:?}");
            exit(2);
        });
        match extrinsic {
            OrchardExtrinsic::BundleProof { vk_id, public_inputs, proof_bytes, .. } => {
                (vk_id, public_inputs, proof_bytes)
            }
            other => {
                eprintln!("Expected BundleProof extrinsic, got: {other:?}");
                exit(2);
            }
        }
    }

    fn parse_hex_blob(label: &str, hex_blob: &str) -> Vec<u8> {
        let mut cleaned = hex_blob.trim();
        if let Some(stripped) = cleaned.strip_prefix("0x") {
            cleaned = stripped;
        }
        let cleaned: String = cleaned.chars().filter(|c| !c.is_whitespace()).collect();
        if cleaned.is_empty() {
            eprintln!("Empty hex input for {label}");
            exit(2);
        }
        hex::decode(cleaned).unwrap_or_else(|err| {
            eprintln!("Invalid hex for {label}: {err}");
            exit(2);
        })
    }

    fn parse_public_inputs(inputs_hex: &str) -> Vec<[u8; 32]> {
        let tokens = inputs_hex
            .split(|c: char| c == ',' || c.is_whitespace())
            .filter(|s| !s.is_empty());
        let mut out = Vec::new();
        for token in tokens {
            let mut token = token.trim();
            if let Some(stripped) = token.strip_prefix("0x") {
                token = stripped;
            }
            let bytes = hex::decode(token).unwrap_or_else(|err| {
                eprintln!("Invalid public input hex: {err}");
                exit(2);
            });
            if bytes.len() != 32 {
                eprintln!(
                    "Public input length mismatch (expected 32 bytes, got {})",
                    bytes.len()
                );
                exit(2);
            }
            let mut array = [0u8; 32];
            array.copy_from_slice(&bytes);
            out.push(array);
        }
        if out.is_empty() {
            eprintln!("No public inputs parsed");
            exit(2);
        }
        out
    }
}

#[cfg(not(any(target_arch = "riscv32", target_arch = "riscv64")))]
fn main() {
    host::run();
}
