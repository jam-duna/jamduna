use ff::PrimeField;
use incrementalmerkletree::Hashable;
use pasta_curves::pallas;
use poseidon::{P128Pow5T3, Spec};

use orchard::tree::MerkleHashOrchard;

mod poseidon_vectors {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../..",
        "/zcash-test-vectors/test-vectors/rust/orchard_poseidon.rs"
    ));

    pub(super) fn run() {
        for tv in TEST_VECTORS {
            let mut state = [
                super::base_from_repr(tv.initial_state[0]),
                super::base_from_repr(tv.initial_state[1]),
                super::base_from_repr(tv.initial_state[2]),
            ];
            super::poseidon_permute(&mut state);

            let expected = [
                super::base_from_repr(tv.final_state[0]),
                super::base_from_repr(tv.final_state[1]),
                super::base_from_repr(tv.final_state[2]),
            ];

            assert_eq!(state, expected);
        }
    }
}

mod poseidon_hash_vectors {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../..",
        "/zcash-test-vectors/test-vectors/rust/orchard_poseidon_hash.rs"
    ));

    pub(super) fn run() {
        use ff::PrimeField;

        for tv in TEST_VECTORS {
            let input = [
                super::base_from_repr(tv.input[0]),
                super::base_from_repr(tv.input[1]),
            ];
            let result = poseidon::Hash::<_, poseidon::P128Pow5T3, poseidon::ConstantLength<2>, 3, 2>::init()
                .hash(input);
            assert_eq!(result.to_repr(), tv.output);
        }
    }
}

mod sinsemilla_vectors {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/tests/fixtures/orchard_sinsemilla_vectors.rs"
    ));

    pub(super) fn run() {
        use ff::PrimeField;
        use group::GroupEncoding;
        use pasta_curves::pallas;

        for tv in TEST_VECTORS {
            let domain = core::str::from_utf8(tv.domain).expect("domain is utf-8");
            let hash_domain = sinsemilla::HashDomain::new(domain);

            let point = Option::<pallas::Point>::from(
                hash_domain.hash_to_point(tv.msg.iter().copied()),
            )
            .expect("valid point");
            assert_eq!(point.to_bytes(), tv.point);

            let hash = Option::<pallas::Base>::from(hash_domain.hash(tv.msg.iter().copied()))
                .expect("valid hash");
            assert_eq!(hash.to_repr(), tv.hash);
        }
    }
}

mod generators_vectors {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../..",
        "/zcash-test-vectors/test-vectors/rust/orchard_generators.rs"
    ));

    pub(super) fn run() {
        use group::GroupEncoding;
        use pasta_curves::arithmetic::CurveExt;
        use pasta_curves::pallas;

        let tv = &TEST_VECTOR;

        let orchard_hasher = pallas::Point::hash_to_curve("z.cash:Orchard");
        assert_eq!(orchard_hasher(b"G").to_bytes(), tv.skb);
        assert_eq!(orchard_hasher(b"K").to_bytes(), tv.nkb);

        let cv_hasher = pallas::Point::hash_to_curve("z.cash:Orchard-cv");
        assert_eq!(cv_hasher(b"v").to_bytes(), tv.vcvb);
        assert_eq!(cv_hasher(b"r").to_bytes(), tv.vcrb);

        let note_commit_r_hasher = pallas::Point::hash_to_curve("z.cash:Orchard-NoteCommit-r");
        assert_eq!(note_commit_r_hasher(b"").to_bytes(), tv.cmb);

        let ivk_commit_r_hasher = pallas::Point::hash_to_curve("z.cash:Orchard-CommitIvk-r");
        assert_eq!(ivk_commit_r_hasher(b"").to_bytes(), tv.ivkb);

        let sinsemilla_q_hasher = pallas::Point::hash_to_curve("z.cash:SinsemillaQ");
        assert_eq!(
            sinsemilla_q_hasher(b"z.cash:Orchard-NoteCommit-M").to_bytes(),
            tv.cmq
        );
        assert_eq!(
            sinsemilla_q_hasher(b"z.cash:Orchard-CommitIvk-M").to_bytes(),
            tv.ivkq
        );
        assert_eq!(
            sinsemilla_q_hasher(b"z.cash:Orchard-MerkleCRH").to_bytes(),
            tv.mcq
        );
    }
}

mod merkle_tree_vectors {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../..",
        "/zcash-test-vectors/test-vectors/rust/orchard_merkle_tree.rs"
    ));

    pub(super) fn run() {
        for tv in TEST_VECTORS {
            let root = super::merkle_root_from_leaves(&tv.leaves);
            assert_eq!(root.to_bytes(), tv.root);

            for (index, path) in tv.paths.iter().enumerate() {
                let root_from_path =
                    super::merkle_root_from_path(tv.leaves[index], path, index as u32);
                assert_eq!(root_from_path.to_bytes(), tv.root);
            }
        }
    }
}

fn base_from_repr(bytes: [u8; 32]) -> pallas::Base {
    Option::<pallas::Base>::from(pallas::Base::from_repr(bytes)).expect("valid base field element")
}

fn merkle_hash_from_bytes(bytes: &[u8; 32]) -> MerkleHashOrchard {
    Option::<MerkleHashOrchard>::from(MerkleHashOrchard::from_bytes(bytes))
        .expect("valid merkle hash")
}

fn apply_mds(state: &mut [pallas::Base; 3], mds: &poseidon::Mds<pallas::Base, 3>) {
    let mut next_state = [pallas::Base::zero(); 3];
    for i in 0..3 {
        for j in 0..3 {
            next_state[i] += mds[i][j] * state[j];
        }
    }
    *state = next_state;
}

fn poseidon_permute(state: &mut [pallas::Base; 3]) {
    let (round_constants, mds, _) = <P128Pow5T3 as Spec<pallas::Base, 3, 2>>::constants();
    let full_rounds = <P128Pow5T3 as Spec<pallas::Base, 3, 2>>::full_rounds() / 2;
    let partial_rounds = <P128Pow5T3 as Spec<pallas::Base, 3, 2>>::partial_rounds();
    let mut round_iter = round_constants.iter();

    for _ in 0..full_rounds {
        let rc = round_iter.next().expect("full round constants");
        for (word, round) in state.iter_mut().zip(rc.iter()) {
            *word = <P128Pow5T3 as Spec<pallas::Base, 3, 2>>::sbox(*word + *round);
        }
        apply_mds(state, &mds);
    }

    for _ in 0..partial_rounds {
        let rc = round_iter.next().expect("partial round constants");
        for (word, round) in state.iter_mut().zip(rc.iter()) {
            *word += *round;
        }
        state[0] = <P128Pow5T3 as Spec<pallas::Base, 3, 2>>::sbox(state[0]);
        apply_mds(state, &mds);
    }

    for _ in 0..full_rounds {
        let rc = round_iter.next().expect("full round constants");
        for (word, round) in state.iter_mut().zip(rc.iter()) {
            *word = <P128Pow5T3 as Spec<pallas::Base, 3, 2>>::sbox(*word + *round);
        }
        apply_mds(state, &mds);
    }
}

fn merkle_root_from_leaves(leaves: &[[u8; 32]; 16]) -> MerkleHashOrchard {
    let mut current: Vec<MerkleHashOrchard> =
        leaves.iter().map(merkle_hash_from_bytes).collect();
    let mut level = 0u8;

    while current.len() > 1 {
        let mut next = Vec::with_capacity(current.len() / 2);
        for pair in current.chunks(2) {
            let left = pair[0];
            let right = pair[1];
            next.push(MerkleHashOrchard::combine(level.into(), &left, &right));
        }
        current = next;
        level += 1;
    }

    current[0]
}

fn merkle_root_from_path(
    leaf: [u8; 32],
    path: &[[u8; 32]; 4],
    position: u32,
) -> MerkleHashOrchard {
    let mut node = merkle_hash_from_bytes(&leaf);
    for (level, sibling_bytes) in path.iter().enumerate() {
        let sibling = merkle_hash_from_bytes(sibling_bytes);
        let level = level as u8;
        if (position >> level) & 1 == 0 {
            node = MerkleHashOrchard::combine(level.into(), &node, &sibling);
        } else {
            node = MerkleHashOrchard::combine(level.into(), &sibling, &node);
        }
    }
    node
}

#[test]
fn orchard_poseidon_vectors() {
    poseidon_vectors::run();
}

#[test]
fn orchard_poseidon_hash_vectors() {
    poseidon_hash_vectors::run();
}

#[test]
fn orchard_sinsemilla_vectors() {
    sinsemilla_vectors::run();
}

#[test]
fn orchard_generator_vectors() {
    generators_vectors::run();
}

#[test]
fn orchard_merkle_tree_vectors() {
    merkle_tree_vectors::run();
}
