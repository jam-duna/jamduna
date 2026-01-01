use halo2_proofs::{
    pasta::vesta::Affine as VestaAffine,
    plonk::{keygen_pk, keygen_vk, VerifyingKey as Halo2VerifyingKey},
    poly::commitment::Params,
};
use orchard::circuit::Circuit;
use std::{
    env,
    fs::File,
    io::{self, BufReader, BufWriter, Write},
    path::PathBuf,
};

const ORCHARD_K: u32 = 11;
const DEFAULT_OUT_DIR: &str = "keys/orchard";
const PARAMS_FILE: &str = "orchard.params";

fn main() -> io::Result<()> {
    let args: Vec<String> = env::args().collect();
    let mut k = ORCHARD_K;
    let mut out_dir = PathBuf::from(DEFAULT_OUT_DIR);

    let mut i = 1;
    while i < args.len() {
        match args[i].as_str() {
            "--k" => {
                i += 1;
                if i >= args.len() {
                    usage();
                    std::process::exit(1);
                }
                k = args[i].parse::<u32>().unwrap_or(ORCHARD_K);
            }
            "--out" => {
                i += 1;
                if i >= args.len() {
                    usage();
                    std::process::exit(1);
                }
                out_dir = PathBuf::from(&args[i]);
            }
            "-h" | "--help" => {
                usage();
                return Ok(());
            }
            _ => {
                usage();
                std::process::exit(1);
            }
        }
        i += 1;
    }

    if k != ORCHARD_K {
        eprintln!("error: Orchard circuit size is fixed at k={}", ORCHARD_K);
        std::process::exit(1);
    }

    let params_path = out_dir.join(PARAMS_FILE);
    let params: Params<VestaAffine> = if params_path.exists() {
        let f = File::open(&params_path)?;
        Params::read(&mut BufReader::new(f))?
    } else {
        let params = Params::new(k);
        std::fs::create_dir_all(&out_dir)?;
        let f = File::create(&params_path)?;
        params.write(&mut BufWriter::new(f))?;
        params
    };
    if params.k() != k {
        return Err(io::Error::new(
            io::ErrorKind::Other,
            "params k does not match requested k",
        ));
    }
    let circuit = Circuit::default();

    let vk_path = out_dir.join("orchard.vk");
    let vk: Halo2VerifyingKey<VestaAffine> = if vk_path.exists() {
        let f = File::open(&vk_path)?;
        Halo2VerifyingKey::read(&mut BufReader::new(f))?
    } else {
        keygen_vk(&params, &circuit).expect("keygen_vk failed")
    };
    let pk = keygen_pk(&params, vk.clone(), &circuit).expect("keygen_pk failed");

    let pk_path = out_dir.join("orchard.pk");

    let mut vk_bytes = Vec::new();
    vk.write(&mut vk_bytes)
        .expect("verifying key serialization failed");
    std::fs::write(&vk_path, vk_bytes)?;

    let mut pk_file = File::create(&pk_path)?;
    writeln!(pk_file, "# orchard keygen (debug format)")?;
    writeln!(pk_file, "# k: {}", k)?;
    writeln!(pk_file, "{:#?}", pk)?;

    println!(
        "✅ Generated orchard.params, orchard.vk, and orchard.pk in {}",
        out_dir.display()
    );
    Ok(())
}

fn usage() {
    eprintln!("Usage: orchard-keygen [--k <log2>] [--out <dir>]");
    eprintln!("  --k <n>   Circuit size log2 (must be {})", ORCHARD_K);
    eprintln!("  --out <dir> Output directory (default: {})", DEFAULT_OUT_DIR);
}
