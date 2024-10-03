# BLS
## Info
### Secret Key (32 bytes)
### In gray paper, bls pubkey (144 bytes)
- G1 : 48 bytes key
  - G1 is usually used for the message hash (i.e., the point derived from the message that will be signed). The message is mapped to a point on the elliptic curve in G1.
- G2 : 96 bytes key
  - G2 is typically used for the public key (i.e., the key corresponding to the private key). The public key is a point in G2.
### Signature (48 bytes)
- BLS12-381 curve : 
  - A signature is a point in G1, which typically has a size of 48 bytes (384 bits).
- Aggregated BLS12-381 signature : 
  - The size of an aggregated signature remains 48 bytes (384 bits), even if it aggregates hundreds or thousands of individual signatures.

## Use Package
* package name : w3f_bls
* package link : 
https://docs.rs/w3f-bls/0.1.4/w3f_bls/?search=rs
* github : https://github.com/w3f/bls/tree/master

## Paper : Efficient Aggregatable BLS Signatures with Chaum-Pedersen Proofs
* Link : https://eprint.iacr.org/2022/1611
### official usage
```rust
use sha2::Sha256;
use ark_bls12_377::Bls12_377;
use ark_ff::Zero;
use rand::thread_rng;

use w3f_bls::{
    single_pop_aggregator::SignatureAggregatorAssumingPoP, DoublePublicKeyScheme, EngineBLS, Keypair, Message, PublicKey, PublicKeyInSignatureGroup, Signed, TinyBLS, TinyBLS377,
};


let message = Message::new(b"ctx", b"I'd far rather be happy than right any day.");
let mut keypairs: Vec<_> = (0..3)
    .into_iter()
    .map(|_| Keypair::<TinyBLS<Bls12_377, ark_bls12_377::Config>>::generate(thread_rng()))
    .collect();
let pub_keys_in_sig_grp: Vec<PublicKeyInSignatureGroup<TinyBLS377>> = keypairs
    .iter()
    .map(|k| k.into_public_key_in_signature_group())
    .collect();

let mut prover_aggregator =
    SignatureAggregatorAssumingPoP::<TinyBLS377>::new(message.clone());
let mut aggregated_public_key =
    PublicKey::<TinyBLS377>(<TinyBLS377 as EngineBLS>::PublicKeyGroup::zero());

//sign and aggegate
let _ = keypairs
    .iter_mut()
    .map(|k| {
        prover_aggregator.add_signature(&k.sign(&message));
        aggregated_public_key.0 += k.public.0;
    })
    .count();

let mut verifier_aggregator = SignatureAggregatorAssumingPoP::<TinyBLS377>::new(message);

verifier_aggregator.add_signature(&(&prover_aggregator).signature());

//aggregate public keys in signature group
verifier_aggregator.add_publickey(&aggregated_public_key);

pub_keys_in_sig_grp.iter().for_each(|pk| {verifier_aggregator.add_auxiliary_public_key(pk);});

assert!(
    verifier_aggregator.verify_using_aggregated_auxiliary_public_keys::<Sha256>(),
    "verifying with honest auxilary public key should pass"
);
```
### BLS Signatures and their Limitations:
BLS signatures, introduced by Boneh, Lynn, and Shacham, are popular for their efficient aggregation. However, verifying individual signatures is slower compared to other signature schemes like Schnorr or ECDSA due to the use of pairings on elliptic curves, especially in large distributed systems like Ethereum.

### Proposed Optimizations:
- Public Keys in Both Groups: 
  - The authors propose having public keys in both G1 and G2 groups with a proof-of-possession (PoP) to guard against rogue-key attacks.

- Aggregated Public Keys in G2: 
  - For aggregate signatures, the public keys are stored in G2 to simplify aggregate verification, allowing more efficient checks in G1.
- Chaum-Pedersen Proofs for Individual Signatures: 
  - To speed up individual signature verification, Chaum-Pedersen Discrete Logarithm Equality (DLEQ) proofs are used, eliminating the need for pairings and significantly reducing verification time.

### Aggregation and Verification:

- The scheme allows aggregating public keys and signatures across multiple signers, reducing both computational and bandwidth costs.
- Verifying aggregate signatures involves fewer operations due to optimizations like using the faster G1 group for certain operations and only requiring a small number of scalar multiplications.