// Compute Sinsemilla root for test commitments

use orchard_builder::sinsemilla_merkle;
use hex;

fn main() {
    // Commitments from package_000.json
    let commitment1 = hex::decode("f17c865507b04ec0248ae58627808cac776cf229a0b66f9599de505afafa0e1c").unwrap();
    let commitment2 = hex::decode("dd8086ce90a88e3f11372db57a868f1911c369095580a25649ae74b3b750e43e").unwrap();

    let mut leaf1 = [0u8; 32];
    let mut leaf2 = [0u8; 32];
    leaf1.copy_from_slice(&commitment1);
    leaf2.copy_from_slice(&commitment2);

    let leaves = vec![leaf1, leaf2];

    let root = sinsemilla_merkle::compute_merkle_root(&leaves).expect("Failed to compute root");

    println!("Commitment 1: {}", hex::encode(leaf1));
    println!("Commitment 2: {}", hex::encode(leaf2));
    println!("Sinsemilla Root: {}", hex::encode(root));
    println!("First 8 bytes: {:?}", &root[0..8]);
}
