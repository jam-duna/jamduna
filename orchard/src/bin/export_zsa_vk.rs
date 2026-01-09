use std::env;
use std::fs::{self, File};
use std::io::BufWriter;
use std::path::PathBuf;

use orchard::circuit::VerifyingKey;
use orchard::orchard_flavor::OrchardZSA;

fn main() {
    let out_dir = env::args()
        .nth(1)
        .unwrap_or_else(|| "../services/orchard/keys".to_string());

    let mut vk_path = PathBuf::from(&out_dir);
    vk_path.push("orchard_zsa.vk");

    let mut params_path = PathBuf::from(&out_dir);
    params_path.push("orchard_zsa.params");

    fs::create_dir_all(&out_dir).expect("create output dir");

    let vk = VerifyingKey::build::<OrchardZSA>();

    let vk_file = File::create(&vk_path).expect("create orchard_zsa.vk");
    let mut vk_writer = BufWriter::new(vk_file);
    vk.write_vk(&mut vk_writer).expect("write vk");

    let params_file = File::create(&params_path).expect("create orchard_zsa.params");
    let mut params_writer = BufWriter::new(params_file);
    vk.write_params(&mut params_writer).expect("write params");

    println!(
        "Wrote ZSA VK to {} and params to {}",
        vk_path.display(),
        params_path.display()
    );
}
