use std::error::Error;
use zcash_primitives::consensus::Network;
use zcash_wallet_core::restore_wallet;

const DEV_SEED: &str = "abandon abandon abandon abandon abandon abandon abandon abandon \
abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon \
abandon abandon abandon abandon abandon art";

fn main() -> Result<(), Box<dyn Error>> {
    let network = Network::TestNetwork; // switch to Network::MainNetwork if needed

    for service_id in [1u32, 2u32] {
        let wallet = restore_wallet(DEV_SEED, network, service_id, 0)?;
        let ua = &wallet.unified_address;
        println!("service_id={} ua={}", service_id, ua);
    }

    Ok(())
}
