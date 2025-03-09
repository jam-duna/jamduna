use tiny_keccak::{Hasher, Keccak};
use hex;
use std::convert::TryInto;

// --- Configuration Constants ---
const NUM_ACTORS: usize = 8192;         // total number of actors
const PAGE_SIZE: usize = 4096;            // each actor's state in a 4096-byte page
const HISTORY_CAPACITY: usize = 100;      // max interaction records per actor

// Global variable T (bonus for mutual cooperation), initially 0.
static mut T: i64 = 0;

// --- Keccak256 helper ---
fn keccak256_hash(data: &[u8]) -> [u8; 32] {
    let mut hasher = Keccak::v256();
    hasher.update(data);
    let mut res = [0u8; 32];
    hasher.finalize(&mut res);
    res
}

// --- Entropy Source ---
// This struct holds a 32-byte state. Each call to next_bool or next_u32 returns a value
// drawn from the state and then updates the state by hashing it.
struct Entropy {
    state: [u8; 32],
}

impl Entropy {
    // Create a new entropy source seeded by a u64 value (e.g. the step number).
    fn new(seed: u64) -> Self {
        // Convert the seed to bytes.
        let seed_bytes = seed.to_le_bytes(); // 8 bytes
        // Use keccak256 to expand to 32 bytes.
        let state = keccak256_hash(&seed_bytes);
        Entropy { state }
    }

    // Update the state: new_state = keccak256(state)
    fn update(&mut self) {
        self.state = keccak256_hash(&self.state);
    }

    // Return a random u32 in 0..max.
    fn next_u32(&mut self, max: u32) -> u32 {
        // Use the first 4 bytes of state.
        let num = u32::from_le_bytes(self.state[0..4].try_into().unwrap());
        self.update();
        num % max
    }

    // Return a random bool with probability 'prob' to be true.
    fn next_bool(&mut self, prob: f64) -> bool {
        // Use the first byte from state.
        let threshold = (prob * 256.0).round() as u8;
        let val = self.state[0];
        self.update();
        val < threshold
    }
}

// --- Strategies ---
#[derive(Debug, Clone, Copy)]
enum Strategy {
    AlwaysCooperate,
    AlwaysDefect,
    Alternate,
    TitForTatCooperate,
    TitForTatDefect,
    Random,
    GrimTrigger,
    Pavlov,
    GenerousTitForTat,
    TitForTwoTats,
    ReciprocalRatio,
    ForgivingPavlov,
    Contrarian,
}

impl Strategy {
    // Return a random strategy among all 13 variants, using the provided entropy source.
    fn random(entropy: &mut Entropy) -> Self {
        match entropy.next_u32(13) {
            0 => Strategy::AlwaysCooperate,
            1 => Strategy::AlwaysDefect,
            2 => Strategy::Alternate,
            3 => Strategy::TitForTatCooperate,
            4 => Strategy::TitForTatDefect,
            5 => Strategy::Random,
            6 => Strategy::GrimTrigger,
            7 => Strategy::Pavlov,
            8 => Strategy::GenerousTitForTat,
            9 => Strategy::TitForTwoTats,
            10 => Strategy::ReciprocalRatio,
            11 => Strategy::ForgivingPavlov,
            _ => Strategy::Contrarian,
        }
    }
}

// --- Interaction Record ---
#[derive(Debug, Clone, Copy)]
struct Interaction {
    opponent_id: usize,  // which actor was involved
    opponent_move: bool, // true = cooperate, false = defect
}

// --- Actor State ---
#[derive(Debug, Clone)]
struct Actor {
    balance: i64,
    strategy: Strategy,
    last_move: Option<bool>,
    interactions: [Option<Interaction>; HISTORY_CAPACITY],
    next_int_idx: usize,
    public_key: [u8; 32],
}

impl Actor {
    // Create a new actor. Use the provided entropy source to choose a strategy and public key.
    fn new(entropy: &mut Entropy) -> Self {
        let mut pubkey = [0u8; 32];
        // Fill public key deterministically from entropy.
        for i in 0..32 {
            pubkey[i] = entropy.state[i];
        }
        // Update entropy after using it.
        entropy.update();
        Actor {
            balance: 1000,
            strategy: Strategy::random(entropy),
            last_move: None,
            interactions: [None; HISTORY_CAPACITY],
            next_int_idx: 0,
            public_key: pubkey,
        }
    }

    // Record an interaction.
    fn record_interaction(&mut self, opponent_id: usize, opponent_move: bool) {
        self.interactions[self.next_int_idx] = Some(Interaction {
            opponent_id,
            opponent_move,
        });
        self.next_int_idx = (self.next_int_idx + 1) % HISTORY_CAPACITY;
    }

    // Look up the last interaction with a given opponent.
    fn last_interaction(&self, opponent_id: usize) -> Option<bool> {
        self.interactions.iter().rev().find_map(|&entry| {
            if let Some(inter) = entry {
                if inter.opponent_id == opponent_id {
                    return Some(inter.opponent_move);
                }
            }
            None
        })
    }

    // Decide move (true = cooperate, false = defect) using the given entropy source.
    fn decide_move(&mut self, opponent_id: usize, entropy: &mut Entropy) -> bool {
        match self.strategy {
            Strategy::AlwaysCooperate => true,
            Strategy::AlwaysDefect => false,
            Strategy::Alternate => {
                let next = self.last_move.map_or(true, |m| !m);
                self.last_move = Some(next);
                next
            }
            Strategy::TitForTatCooperate => self.last_interaction(opponent_id).unwrap_or(true),
            Strategy::TitForTatDefect => self.last_interaction(opponent_id).unwrap_or(false),
            Strategy::Random => entropy.next_bool(0.5),
            Strategy::GrimTrigger => {
                let ever_defected = self.interactions.iter().any(|&record| {
                    if let Some(inter) = record {
                        inter.opponent_id == opponent_id && !inter.opponent_move
                    } else {
                        false
                    }
                });
                if ever_defected { false } else { true }
            }
            Strategy::Pavlov => {
                let op_last = self.last_interaction(opponent_id).unwrap_or(true);
                match self.last_move {
                    Some(my_last) => {
                        if my_last == op_last { my_last } else { !my_last }
                    },
                    None => { self.last_move = Some(true); true }
                }
            }
            Strategy::GenerousTitForTat => {
                if let Some(last) = self.last_interaction(opponent_id) {
                    if !last {
                        entropy.next_bool(0.3) // forgive with 30% chance
                    } else {
                        true
                    }
                } else {
                    true
                }
            }
            Strategy::TitForTwoTats => {
                let history: Vec<_> = self.interactions.iter()
                    .filter_map(|&record| record)
                    .filter(|rec| rec.opponent_id == opponent_id)
                    .collect();
                if history.len() >= 2 {
                    let last_two = &history[history.len()-2..];
                    if !last_two[0].opponent_move && !last_two[1].opponent_move {
                        false
                    } else { true }
                } else { true }
            }
            Strategy::ReciprocalRatio => {
                let mut total = 0;
                let mut coop = 0;
                for record in self.interactions.iter().flatten().filter(|r| r.opponent_id == opponent_id) {
                    total += 1;
                    if record.opponent_move { coop += 1; }
                }
                if total == 0 { true } else { (coop as f64 / total as f64) >= 0.6 }
            }
            Strategy::ForgivingPavlov => {
                let op_last = self.last_interaction(opponent_id).unwrap_or(true);
                if let Some(my_last) = self.last_move {
                    if my_last == op_last {
                        my_last
                    } else {
                        if entropy.next_bool(0.2) { my_last } else { !my_last }
                    }
                } else {
                    self.last_move = Some(true);
                    true
                }
            }
            Strategy::Contrarian => {
                let mut total = 0;
                let mut coop = 0;
                for record in self.interactions.iter().flatten().filter(|r| r.opponent_id == opponent_id) {
                    total += 1;
                    if record.opponent_move { coop += 1; }
                }
                if total == 0 { true } else { if (coop as f64 / total as f64) > 0.5 { false } else { true } }
            }
        }
    }

    // Serialize the actor's state into a 4096-byte page.
    fn to_page(&self) -> [u8; PAGE_SIZE] {
        let mut page = [0u8; PAGE_SIZE];
        page[0..8].copy_from_slice(&self.balance.to_le_bytes());
        page[8] = match self.strategy {
            Strategy::AlwaysCooperate => 0,
            Strategy::AlwaysDefect => 1,
            Strategy::Alternate => 2,
            Strategy::TitForTatCooperate => 3,
            Strategy::TitForTatDefect => 4,
            Strategy::Random => 5,
            Strategy::GrimTrigger => 6,
            Strategy::Pavlov => 7,
            Strategy::GenerousTitForTat => 8,
            Strategy::TitForTwoTats => 9,
            Strategy::ReciprocalRatio => 10,
            Strategy::ForgivingPavlov => 11,
            Strategy::Contrarian => 12,
        };
        page[9..11].copy_from_slice(&(self.next_int_idx as u16).to_le_bytes());
        page[11..43].copy_from_slice(&self.public_key);
        let mut offset = 43;
        for entry in &self.interactions {
            if let Some(inter) = entry {
                let id = inter.opponent_id as u16;
                page[offset..offset+2].copy_from_slice(&id.to_le_bytes());
                page[offset+2] = if inter.opponent_move { 1 } else { 0 };
            } else {
                page[offset] = 0;
                page[offset+1] = 0;
                page[offset+2] = 0;
            }
            offset += 3;
            if offset + 3 > PAGE_SIZE { break; }
        }
        page
    }
}

// --- Simulation ---
fn main() {
    // Instead of using rand, we use our entropy source.
    // We seed the entropy source at each simulation step with the step number.
    // At initialization, we use a fixed seed (e.g. 0).
    let mut entropy = Entropy::new(0);

    // Create a vector of actors using our entropy source.
    let mut actors: Vec<Actor> = (0..NUM_ACTORS)
        .map(|_| Actor::new(&mut entropy))
        .collect();

    unsafe { T = 0; } // Initially 0
    let mut step: u64 = 0;
    // Simulation loop
    loop {
        step += 1;
        // Re-seed the entropy source with the current step number for determinism.
        entropy = Entropy::new(step);

        // Each actor picks an opponent and interacts.
        for i in 0..actors.len() {
            // 80% chance to pick from history if available.
            let pick_from_history = if actors[i].interactions.iter().any(|x| x.is_some()) {
                entropy.next_bool(0.8)
            } else {
                false
            };

            let opponent_id = if pick_from_history {
                // Use entropy to pick a random actor.
                let op = entropy.next_u32(NUM_ACTORS as u32) as usize;
                // (Optionally ensure not self; here we allow it.)
                op
            } else {
                // Pick a new opponent (ensure not self).
                let mut op;
                loop {
                    op = entropy.next_u32(NUM_ACTORS as u32) as usize;
                    if op != i { break; }
                }
                op
            };

            // Let actor i decide its move (pass entropy).
            let move_i = actors[i].decide_move(opponent_id, &mut entropy);
            let move_op = actors[opponent_id].decide_move(i, &mut entropy);

            // Update balances.
            if move_i && move_op {
                unsafe {
                    actors[i].balance += T;
                    actors[opponent_id].balance += T;
                }
            } else if move_i && !move_op {
                actors[i].balance -= 2;
                actors[opponent_id].balance += 2;
            } else if !move_i && move_op {
                actors[i].balance += 2;
                actors[opponent_id].balance -= 2;
            }
            // Record interactions.
            actors[i].record_interaction(opponent_id, move_op);
            actors[opponent_id].record_interaction(i, move_i);
        }

        // (Optional) Update T here if desired.
        // unsafe { T = ...; }

        // For demonstration, every 1000 steps print summaries for first 5 actors.
        if step % 1000 == 0 {
            println!("After {} steps:", step);
            for i in 0..5 {
                let actor = &actors[i];
                println!("Actor {}: balance = {}, strategy = {:?}, first 3 interactions: {:?}",
                         i, actor.balance, actor.strategy, &actor.interactions[0..3]);
                let page = actor.to_page();
                let hash = keccak256_hash(&page);
                println!("  Page hash: {}", hex::encode(hash));
            }
        }

        if step >= 10000 { break; }
    }
}
