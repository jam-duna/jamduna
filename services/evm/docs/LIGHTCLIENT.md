# LIGHTCLIENT.md
## Light Client Tag Model (ZIP-307–Style Scanning) for Private Notes & Notifications

This document describes a **tag-based light client scanning model** inspired by Zcash’s ZIP-307-style approach. The goal is to let a user (e.g., **Alice**) efficiently detect **incoming encrypted notes** from a public stream **without revealing ownership**, and without requiring any notifier/relayer to hold Alice’s view key or perform per-user filtering.

### PoC orientation

This note is written so an engineer can quickly stand up a **minimal proof of concept**. It emphasizes the *interfaces* and *checks* a demo needs to exercise and includes a simple end-to-end data flow (sender → chain → relayer → light client).

**PoC quick facts**
- Tag = 16 bytes from `blake2s(ivk, r)`; ciphertext = 144 bytes (X25519 + ChaCha20-Poly1305).
- Batch scan target: <1s for 1000 outputs; tag check <1ms; decrypt <10ms on tag match.
- Demo success: one sender appends an output to JSON/HTTP feed; light client detects exactly one new note and none for wrong IVK.

This model is appropriate for:
- **Incoming transfers** to private notes (margin top-ups, payouts, deposits, gifts)
- **Private “inbox” style delivery** for note-based systems
- **Separation of concerns** where relayers broadcast everything and clients filter locally

It is **not required** for public “risk signals” (e.g., margin-call beacons) that can be matched via deterministic hashes without encryption.

---

## 1. Terminology

### Notes
A **note** is a private record of value/position/state owned by a user. A note typically includes:
- asset identifier
- amount
- recipient-specific secrets (e.g., spending key material)
- randomness/blinding factors
- metadata (optional)

Notes are usually committed into a **note commitment tree** (e.g., Merkle tree) so the system can later prove membership and prevent double-spends (via nullifiers).

### Ciphertext
The **ciphertext** is the encrypted payload carrying note contents, intended only for the recipient.

### Tag
A **tag** is a short, fixed-size value published alongside each ciphertext. It is computed as a **keyed pseudorandom value** under the recipient’s **incoming view key** (or equivalent).

**Security requirement:** to anyone without the recipient’s view key, tags are indistinguishable from random, and are not linkable to the recipient or to each other.

### Light Client
A **light client** downloads (or is served) a stream of compact data from the chain and performs local scanning to detect notes belonging to the user.

---

## 2. Design Objectives

1. **No per-user routing**: Notifiers/relayers do **not** target Alice specifically.
2. **No view key sharing**: Alice’s view key remains on her device (or her trusted scanner).
3. **Efficient scanning**: Avoid “decrypt everything” by adding a cheap pre-filter step (tag match).
4. **No ownership leakage**: Observers cannot determine which notes belong to Alice.
5. **Composable transport**: Works over direct chain sync, indexer APIs, WebSocket streams, or push wake-ups.

---

## 3. Data Published on the Public Stream

For each newly created note output, the chain publishes a bundle conceptually like:

```text
OutputRecord {
  tag: bytes[T],             // small, fixed-size
  ciphertext: bytes[C],       // fixed or bounded size
  commitment: bytes32,        // note commitment (for tree)
  aux: ...                    // proof-related or routing-neutral metadata
}
```

All `OutputRecord`s are publicly visible to everyone.

**Important:** The record does **not** contain the recipient identity (address) in plaintext.

### Minimal PoC payloads

For a prototype, keep the footprint small and deterministic:
- `T = 16` bytes for the tag (fits inside a single 128-bit limb)
- `C = 144` bytes for ciphertext (e.g., x25519 + ChaCha20-Poly1305 with 32-byte MAC)
- `commitment = 32` bytes (e.g., Poseidon hash)
- `aux` can be just `block_height` and `position_in_block` to support later proof construction

---

## 4. Keys & Roles

### Recipient (Alice) holds:
- **Spending key**: enables spending notes
- **Incoming view key (IVK)**: enables detection of incoming notes (without spending)

In some architectures, the IVK is derivable from the spending key, and can optionally be delegated to a “view-only” device.

### Sender (Bob) holds:
- Alice’s **public receiving material** (e.g., a transmission key / diversified address) sufficient to encrypt the note for Alice.
- Sender chooses fresh randomness per output.

### Notifier/Relayer holds:
- **No secret keys** related to Alice.
- Only subscribes to the public stream and re-broadcasts it.

---

## 5. Tag Construction (Abstract)

A ZIP-307–style tag can be modeled abstractly as:

```text
tag = PRF(IVK, r) truncated_to_T_bytes
```

Where:
- `PRF` is a cryptographic pseudorandom function (e.g., Poseidon-based PRF, Blake2s-based keyed PRF, etc.)
- `IVK` is Alice’s incoming view key (secret)
- `r` is sender-chosen fresh randomness (never reused)
- the output is truncated to `T` bytes (e.g., 16 or 32 bytes)

### Security properties required
1. **Indistinguishability**: Without `IVK`, `tag` looks random.
2. **Unlinkability**: Two tags for the same IVK with different `r` values are unlinkable.
3. **Non-invertibility**: Observers cannot recover `IVK` or `r` from `tag`.

**Do not** use tags like `H(IVK)` or reuse `r`, as these would leak linkable identity.

### PoC-friendly instantiation

One pragmatic PoC recipe that aligns with the above properties:
- `IVK`: 32-byte secret (sampled uniformly)
- `r`: 32-byte nonce per output (from CSPRNG)
- `PRF(ivk, r) = Blake2s(key=ivk, input=r)`
- `tag = truncate_16(PRF(ivk, r))`

This uses widely available libraries, keeps implementation complexity low, and is fast enough to benchmark on mobile hardware.

---

## 6. Ciphertext Construction (Abstract)

The ciphertext is produced with standard public-key encryption (or hybrid KEM-DEM), conceptually:

```text
ciphertext = Enc(pk_Alice, note_plaintext; r_enc)
```

Where:
- `pk_Alice` is a public receiving key/address component
- `r_enc` is encryption randomness (may be derived from `r` or independent, depending on scheme)
- `note_plaintext` includes amount, asset, and any data needed to later spend the note

The light client should only attempt `Dec(sk_or_view, ciphertext)` when the tag indicates a likely match.

### PoC recipe

Use X25519 for key agreement and ChaCha20-Poly1305 for authenticated encryption:

```text
ephemeral_sk, ephemeral_pk = generate_x25519_keypair()
shared_secret = x25519(ephemeral_sk, pk_Alice)
dk = KDF(shared_secret || r_enc)
ciphertext = AEAD_Encrypt(key=dk, nonce=0, plaintext=note_plaintext, ad=commitment)
```

Include `ephemeral_pk` in the ciphertext so Alice can derive the same `shared_secret`. Reuse `r` as `r_enc` only if the KDF domain-separates tag and encryption usage; otherwise sample independently.

---

## 7. Light Client Scanning Algorithm

### Inputs
- User secrets: `IVK`, and decryption material as needed
- Stream of `OutputRecord`s from chain/relayer/indexer

### Core loop
1. **Read output record**: `(tag, ciphertext, commitment, ...)`
2. **Tag match test** (cheap):
   - Use `IVK` to compute whether this `tag` is consistent with being intended for Alice.
   - In practice, the scheme specifies exactly how to test this efficiently.
3. If tag does **not** match: **skip**
4. If tag matches: **attempt decryption**
5. If decryption succeeds and internal checks pass:
   - record the note as owned by Alice
   - store the commitment position needed for future spends
6. Continue

### Pseudocode (conceptual)
```python
def scan_outputs(outputs, ivk, decrypt_key):
    for out in outputs:
        if not tag_matches(ivk, out.tag, out.aux):
            continue
        pt = try_decrypt(decrypt_key, out.ciphertext)
        if pt is None:
            continue
        if not plaintext_valid(pt, out.commitment):
            continue
        add_note_to_wallet(pt, out.commitment, out.aux)
```

**Key point:** the client does not broadcast whether a match occurred.

### PoC end-to-end flow

1. **Sender constructs output**: compute `tag`, encrypt `ciphertext`, and emit `(tag, ciphertext, commitment, aux)` into a mock block.
2. **Relayer demo**: expose `GET /outputs?from_height=...` returning the serialized records; no filtering.
3. **Light client**: iterate over records, run `tag_matches`, attempt decryption, and store matched notes in an in-memory wallet.
4. **Verification harness**: unit test that a wallet with the correct IVK recovers the note while a wallet with a different IVK never does.

---

## 8. Transport & Notifier Model

### 8.1 Broadcast, not targeted delivery
Relayers/notifiers:
- subscribe to blocks / message stream
- extract all `OutputRecord`s
- **broadcast them to everyone** via one or more channels:
  - WebSocket feed (recommended)
  - HTTP batch endpoint
  - pub/sub topic
  - gossip

They do **not**:
- store `wallet → device` maps
- hold view keys
- filter “for Alice”

### 8.2 Push notifications (wake-up only)
For mobile/web push:
- Notifier sends a generic “new batch available” ping
- App wakes up and fetches recent `OutputRecord`s
- App scans locally and decides whether to show a user-facing alert

Push payload should be semantically bland, e.g.:
```json
{ "type": "new_batch", "range": "block_123450-123480" }
```

---

## 9. Privacy Analysis

### What observers can see
- total number of outputs
- tags (random-looking)
- ciphertexts (opaque)
- commitments (random-looking)

### What observers cannot learn
- which outputs belong to Alice
- how many notes Alice received
- Alice’s balances
- which ciphertexts decrypt successfully

### Why tags do not leak ownership
- tags are computed with a secret key (`IVK`) via a PRF
- without `IVK`, tags are indistinguishable from random
- fresh randomness per note prevents linkability

### What can still leak (honest limits)
- volume metadata: total outputs per block/epoch
- network-layer metadata if Alice fetches from a unique endpoint (mitigate via common endpoints, batching, caching, Tor/VPN if needed)

---

## 10. Performance Considerations

### Scanning cost drivers
- number of outputs per unit time
- tag matching cost (very cheap)
- decryption attempts (rare, only on tag matches)

### Practical optimizations
- batch downloads (range queries)
- parallel scan on device (bounded)
- store last scanned height
- keep a compact set of known commitments / positions

**Key win vs “trial decrypt everything”:** the client typically decrypts *near-zero* outputs that aren’t intended for it.

---

## 11. Security Considerations & Gotchas

1. **Never reuse randomness `r`** across outputs.
2. **Constant-time comparisons** for tag matches to reduce side-channel risk on-device.
3. **Avoid per-user acknowledgements** (no “read receipts” on-chain).
4. **Key rotation**: support migrating to new receiving keys without breaking old note recovery.
5. **DoS resistance**: if output volume spikes, wallet may degrade; mitigate with batching and rate limits at relayer endpoints.
6. **Replay tolerance**: clients should deduplicate on `(commitment, block_height, position_in_block)` to avoid re-processing.
7. **Domain separation**: derive tag PRF keys and encryption keys with distinct labels to avoid cross-protocol leakage.
8. **Telemetry discipline**: avoid shipping metrics that correlate scan timing with specific heights; batch uploads if enabled.

---

## 12. Relationship to Perp System Notifications

### Incoming transfers (value)
Use the tag+ciphertext note model described here.

### Risk notifications (margin call, close-only, forced reduction)
Do **not** require tag+ciphertext. A more private and cheaper approach is:
- protocol emits anonymous **risk beacons** identified by opaque hashes
- client matches by deterministic local computation (no decryption)

This keeps “risk state” and “value transfer” pathways distinct and easier to reason about.

---

## 13. Minimal Interfaces

### Chain/API provides
- `get_outputs(start_height, end_height) -> [OutputRecord]`
- optional: `get_commitment_tree_state(...)`

### Wallet provides
- `scan(range) -> notes`
- `store_note(note)`
- `spend(note, ...) -> tx`

### Minimal PoC types (example)

```go
type OutputRecord struct {
    Tag        [16]byte
    Ciphertext [144]byte
    Commitment [32]byte
    Height     uint64
    Index      uint16
}

type Wallet interface {
    Scan(outputs []OutputRecord) []OwnedNote
    Store(note OwnedNote)
}
```

Even a simple CLI wallet can exercise these interfaces by reading JSON batches, running the scan loop, and printing matched note IDs.

### Relayer provides (optional)
- `ws://.../outputs` stream
- `GET /outputs?from=...&to=...`

---

## 14. Summary

- The network publishes **(tag, ciphertext)** pairs for *all* recipients.
- Notifiers **broadcast everything**; they do not filter for Alice.
- Alice’s light client uses her **incoming view key** to **recognize tags** and decrypt only relevant ciphertexts.
- Tags are safe because they are **PRF outputs**: indistinguishable from random and unlinkable without the view key.

---

## 15. Proof-of-Concept (PoC) Implementation Outline

The PoC goal is to demonstrate end-to-end detection of a tagged ciphertext by a light client without revealing ownership. The steps below are intentionally minimal so the prototype can be built quickly and iterated on.

### 15.1 Data Model (minimal JSON)

Use a single JSON list to represent a batch of `OutputRecord`s. Each entry keeps the values that the scanner actually consumes:

```json
[
  {
    "tag": "hexstring",           // T-byte hex, PRF(ivk, r)
    "ciphertext": "hexstring",    // encrypted note payload
    "commitment": "0x...",        // note commitment (32 bytes)
    "aux": { "height": 12345 }    // optional, but include block height for checkpointing
  }
]
```

### 15.2 Minimal Tag/Decrypt Flow

1. **Tag generation (sender side, demo-only):**
   ```python
   tag = prf(ivk, r)[:T]
   ciphertext = enc(pk_alice, note_plaintext, r_enc)
   ```
   - For the PoC, pick `T = 16` bytes and use a simple keyed BLAKE2s PRF (e.g., `blake2s(key=ivk, data=r)`), acknowledging this is not final cryptography.
2. **Transport:** dump the JSON batch to disk or serve it via a single HTTP endpoint (`GET /outputs?from=H1&to=H2`).
3. **Client scan:** iterate over the JSON array, compute `candidate = blake2s(key=ivk, data=r_guess)[:16]` based on the scheme’s match rule, and attempt decryption only on matches.
4. **Verification:** successful decrypts should include a self-check (e.g., MAC tag or embedded `commitment`) so the client can discard false positives.

### 15.3 Components to Stand Up

- **Sender stub:** CLI script that accepts an IVK, synthesizes `r`, computes `tag`, encrypts a dummy note, and appends an `OutputRecord` to the JSON file.
- **Relayer stub:** trivial HTTP server that serves the JSON batch unchanged. No per-user filtering.
- **Light client stub:** CLI or small service that:
  - Loads IVK + decrypt key from local config
  - Fetches the batch
  - Runs the tag match + decrypt loop
  - Prints the commitments of notes it owns

### 15.4 Success Criteria for the PoC

- Running the sender once causes the light client to detect exactly one new note after fetching the batch.
- Running the sender multiple times with fresh `r` values creates additional notes that remain unlinkable by observers.
- Re-running the scanner without new outputs produces no spurious decrypts.

### 15.5 7-Day Implementation Plan

**Days 1-2: Core Primitives**
1. **Tag generation and verification**
   - Implement BLAKE2s keyed PRF for tag computation: `tag = blake2s(key=ivk, data=r)[:16]`
   - Create tag matching function with constant-time comparison
   - Target: <1ms per tag operation

**Days 3-4: Encryption and Transport**
2. **Note encryption/decryption**
   - X25519 + ChaCha20-Poly1305 implementation for note payloads
   - JSON-based transport format for OutputRecord batches
   - Simple HTTP server for batch serving (GET /outputs?from=H&to=H)

**Days 5-6: Light Client Core**
3. **Scanner implementation**
   - Core scanning loop: tag_match → attempt_decrypt → validate → store
   - IVK management and note storage (in-memory with JSON persistence)
   - CLI tools for key generation, scanning, and note enumeration

**Day 7: Integration and Demo**
4. **End-to-end demonstration**
   - Sender CLI that creates tagged encrypted notes
   - Relayer that serves note batches unchanged (no filtering)
   - Light client that scans and detects owned notes
   - Integration test: multiple senders, one client detects only intended notes

**Performance targets:**
- Tag verification: <1ms per tag check
- Note decryption: <10ms per attempt (only on tag matches)
- Batch scanning: <1s for 1000 notes
- Memory usage: <32MB for light client state
- False positive rate: <0.01% (cryptographically bounded)

### 15.6 Next Steps After the PoC

- Swap the demo PRF/encryption for production-grade primitives (e.g., the ZIP-307 construction or Poseidon-based PRF + hybrid KEM-DEM).
- Attach the output stream to real block production instead of a static JSON file.
- Add incremental checkpointing (last scanned height) and bounded cache for commitments.
- Integrate push "wake-up" notifications that only convey a new block range, not user-specific data.
