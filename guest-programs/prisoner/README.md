# Actor-Based Game Simulation with Deterministic Entropy

This Rust program simulates a game of interactions among 8,192 actors,
each stored as a 4,096‑byte page. Each actor begins with a $1000.00
balance, one of 13 possible strategies, a circular history of up to
100 interactions, and a 32‑byte public key. Instead of using a
conventional random number generator, all randomness is derived from a
custom entropy source that uses Keccak256 hashing seeded by the
current simulation step, ensuring deterministic behavior.

### Actor State:
Each actor is represented by a 4,096‑byte page containing:

* A balance (initially $1000)
* A strategy chosen from 13 variants (e.g., Always Cooperate, Always Defect, Alternate, various Tit‑for‑Tat types, Random, Grim Trigger, Pavlov, Generous Tit‑for‑Tat, Tit‑for‑Two‑Tats, Reciprocal Ratio, Forgiving Pavlov, and Contrarian)
* A circular record of the last 100 interactions
* A 32‑byte public key generated deterministically via the entropy source
* Deterministic Entropy: A custom Entropy struct generates pseudo‑random numbers using a 32‑byte state updated by Keccak256 hashing. At each simulation step, the entropy source is re‑seeded with the step number, replacing calls to traditional randomness libraries.

### Simulation Mechanics:
At each simulation step:

* Each actor selects an opponent (with an 80% chance to choose one from its history and 20% a new actor).
* Both actors decide whether to cooperate or defect based on their strategy and past interactions.
* Balances are updated: mutual cooperation yields a bonus T (initially 0), one-sided cooperation results in a transfer of 2 units, and mutual defection leaves balances unchanged.
* The interaction is recorded in both actors’ histories.
* Output: Every 1,000 steps, the program prints a summary for the first 5 actors, including:
  - Balance, strategy, and recent interactions
  - The Keccak256 hash (hex‑encoded) of the actor’s 4,096‑byte state page

### Building and Running

```
cargo build --release
cargo run --release
```

The simulation runs for 10,000 steps, printing summary statistics every 1,000 steps.  

## Notes

The simulation uses deterministic entropy; running it with the same
parameters will yield identical results.  You can adjust the number of
actors, history capacity, simulation steps, or strategies by modifying
the constants and code in the source.  This project demonstrates a
complex agent-based simulation with a custom entropy source and state
serialization into fixed-size pages.