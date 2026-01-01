# CC-AUTH: Credit Card Model and Spend Authorization (JAM-backed)

This document specifies a **card-like** spending model for a JAM-based system that plays the “clearing + risk + conversion” role of a CEX (e.g., Gemini) while leaving **bank/card rails** to a regulated partner. It now includes a **PoC blueprint** that maps concepts to concrete artifacts so the system can be prototyped quickly.

The model is designed to support **small retail purchases** (e.g., $20), **batched settlement**, **privacy-preserving authorization**, and **bounded-loss compromise handling**. The PoC section focuses on the smallest viable feature slice that exercises risk controls, capability proofs, and batch settlement without needing production-grade issuer integrations.

---

## 1. Goals and Non-Goals

### Goals
- **Card-network compatible** authorization and settlement lifecycle (authorize → clear → settle).
- **Batched settlement** (no on-chain transfer per swipe).
- **Non-custodial / protocol-enforced** collateral accounting (no discretionary seizure).
- **Private spend authorization**: avoid linking on-chain identity to card events.
- **Bounded risk**: per-tx, per-day, and global limits; safe under volatility.
- **Refund/chargeback support** consistent with card rails.
- **Implementable**: clear interfaces + invariants that can be prototyped quickly.

### Non-Goals
- JAM does **not** issue cards or directly integrate with Visa/Mastercard. That is done by a **regulated issuer/processor**.
- This doc does not define KYC/AML policies (handled by issuer/program manager).
- This doc does not require real-time BTC on-chain movement at purchase time.

---

## 2. Actors

- **Alice (Cardholder)**: user with collateral locked/encumbered in JAM.
- **JAM Service (Clearing House Service)**: protocol state machine that:
  - maintains collateral claims and encumbrances,
  - mints/revokes spend capabilities,
  - authorizes spends within limits,
  - tracks outstanding authorizations and settlement obligations.
- **Card Program / Processor**: provides card app, tokenization, authorization plumbing.
- **Issuer Bank**: legally issues the card and interfaces with the card network.
- **Card Network**: routes authorizations and settlement files.
- **Merchant + Acquirer**: typical retail acceptance stack.
- **Relayers/Notifiers**: optional broadcast infra; **must not** know Alice’s view key.

---

## 3. Core Concepts

### 3.1 Collateral vs Settlement Obligation
- **Collateral**: value encumbered in JAM (e.g., BTC/ETH notes, stablecoin notes, or other locked assets).
- **Settlement Obligation (USD)**: amount JAM promises to deliver to the issuer/processor during batch settlement.

**Key point:** At swipe time, JAM creates/updates an internal **USD obligation**; it does not move fiat or on-chain BTC.

### 3.2 Spend Capability (Bearer Authorization)
A **Spend Capability** is an unlinkable bearer instrument granting limited spending power.

Properties:
- **Bounded**: max per transaction, max per day, max outstanding.
- **Short-lived**: expires (minutes to 24h, depending on product).
- **Revocable**: user can revoke instantly; issuer can freeze card independently.
- **Unlinkable**: no wallet address or note id is revealed during authorization.

### 3.3 Reservation (Auth Hold)
An authorization creates a **reservation**:
- reduces Alice’s available spend limit,
- increases JAM’s outstanding settlement liability,
- persists until cleared/expired/reversed.

---

## 4. State Machines

### 4.1 Capability State
- `ACTIVE`
- `REVOKED`
- `EXPIRED`

### 4.2 Authorization State (per transaction)
- `AUTHORIZED` (hold placed)
- `CLEARED` (final amount known)
- `SETTLED` (included in batch settlement)
- `REVERSED` (auth canceled before clearing)
- `REFUNDED` (post-settlement adjustment)
- `CHARGEBACK` (dispute outcome applied)

---

## 5. Data Objects (Logical)

### 5.1 SpendCapability
- `cap_id`: opaque identifier (hash)
- `cap_commitment`: commitment to cap parameters + secret nonce
- `limits`:
  - `max_tx_usd`
  - `max_day_usd`
  - `max_outstanding_usd`
- `expiry_ts`
- `revocation_key` (optional mechanism)
- `status`

### 5.2 SpendAuthRequest (from processor → JAM)
- `cap_id` (or capability proof)
- `tx_ref` (unique per authorization attempt)
- `amount_usd`
- `merchant_category` (optional; used only for risk limits)
- `timestamp`
- `sig_or_proof` (derived from capability)

### 5.3 SpendAuthResponse (JAM → processor)
- `approved` bool
- `auth_code` (opaque)
- `reserved_usd` (amount placed on hold)
- `expiry_ts` (hold expiry)
- `reason` (decline reason code)

### 5.4 ClearingRecord (issuer/processor → JAM, batched)
- `tx_ref`
- `final_amount_usd`
- `clearing_ts`

### 5.5 SettlementBatch (JAM → issuer/processor)
- `batch_id`
- `net_usd_owed`
- `tx_refs[]` (optional; can be hashed)
- `proof_of_reserves` / accounting attestation (optional)

---

## 6. Authorization Flow (Retail $20 Purchase)

### 6.1 Precondition: Capability Provisioning
Alice provisions a capability (via wallet/app):

1. Alice requests:
   - daily limit (e.g., $500/day),
   - per-tx limit (e.g., $50),
   - max outstanding (e.g., $200),
   - expiry (e.g., 24h).
2. JAM checks Alice’s collateral health and sets:
   - **spendable allowance** <= conservative fraction of collateral.
3. JAM returns a signed capability token or commitment.

**No issuer identity is required at this stage** unless your business model chooses it.

### 6.2 Authorization (T=0)
When Alice taps for $20:

1. Processor sends `SpendAuthRequest` to JAM.
2. JAM verifies:
   - capability is `ACTIVE` and not expired,
   - `amount_usd <= max_tx_usd`,
   - `spent_today + amount <= max_day_usd`,
   - `outstanding + amount <= max_outstanding_usd`,
   - collateral health after reservation remains above thresholds.
3. If approved:
   - JAM creates/updates reservation:
     - `outstanding_usd += 20`
     - `spent_today_usd += 20`
     - internally encumbers collateral value equivalent to $20 (or increases USD liability).
   - JAM returns `SpendAuthResponse(approved=true, auth_code=...)`.
4. Issuer/network returns approval to merchant.

**Privacy:** Request uses **capability proof**, not wallet address.

### 6.3 Authorization Hold Expiry
If the merchant never clears:
- After `hold_expiry` (e.g., 7 days), JAM transitions `AUTHORIZED → REVERSED` automatically:
  - `outstanding_usd -= reserved`
  - restore spend limit

---

## 7. Clearing and Settlement (Batch, T+1/T+2)

### 7.1 Clearing
The processor delivers `ClearingRecord`:

- If `final_amount_usd == reserved_usd`: mark `CLEARED`.
- If less: reduce reservation difference and restore available limit.
- If more: treat as incremental auth (must be within capability rules) or decline partial uplift.

### 7.2 Settlement Batch
At settlement time (T+1/T+2):
1. JAM computes `net_usd_owed = sum(cleared_amounts) - sum(net refunds/credits)` for the period.
2. JAM transfers value to issuer/processor:
   - via stablecoin rails to a bank-owned wallet, or
   - via pre-funded cash buffer, or
   - via netting across inbound/outbound flows.
3. JAM marks involved txs as `SETTLED`.

**Important:** Settlement is batched to avoid per-swipe on-chain transfers.

### 7.3 Minimal Settlement Loop (PoC-friendly)
1. Collect `ClearingRecord`s for a time window (e.g., 1h) in an append-only log.
2. Recompute `net_usd_owed` deterministically from that log and previously settled markers.
3. Produce a signed `SettlementBatch` with:
   - `batch_id` = hash(time window || clearing log root),
   - `net_usd_owed`,
   - Merkle root of included `tx_ref`s,
   - attestation over collateral sufficiency at batch close.
4. Mark all included auths as `SETTLED` and advance the watermark.
5. Persist the batch artifact (JSON) plus signature to allow replay / audit.

---

## 8. Collateral Conversion (BTC-backed Spending)

When Alice authorizes USD spend but collateral is BTC/ETH:
- JAM prices collateral using a **mark** (slow TWAP or scheduled snapshot).
- JAM increases USD obligation and correspondingly decreases Alice’s collateral claim **by USD notional**, using the current mark.

Example:
- BTC price = $40,000
- $20 spend → 0.0005 BTC of collateral claim reduction

**PoC simplification:** fix a single collateral asset + oracle source; expose the mark and haircut as parameters in configuration to make behavior reproducible in tests.

### Slippage/Spread
To bound risk between auth and settlement:
- Apply a small **spread** at authorization (e.g., +25–100 bps) OR
- Maintain an issuer-funded buffer OR
- Require higher collateral haircut.

---

## 9. Refunds

Refunds are applied as **settlement adjustments**.

### Case A: Refund before settlement
- Charge and refund net to zero in clearing.
- JAM cancels reservation / reduces cleared amount accordingly.

### Case B: Refund after settlement
- Issuer/processor reports a credit.
- JAM restores Alice’s collateral claim by refunded USD notional at the then-current mark (or uses same-day settlement mark for consistency).

Refunds do **not** “rewind” prior BTC prices; they are **USD nominal**.

---

## 10. Chargebacks / Disputes

Disputes are adjudicated by issuer/network rules.

- If Alice wins:
  - issuer credits settlement; JAM restores collateral claim.
- If Alice loses:
  - original settled amount stands; no protocol change needed.

**Protocol stance:** JAM honors the issuer’s final settlement outcome.

---

## 11. Compromise / Emergency Controls

### 11.1 If Alice suspects card compromise
Alice performs **two independent actions**:

1. **Freeze card with issuer/program**:
   - blocks new authorizations at the card network level.
2. **Revoke spend capabilities in JAM wallet**:
   - invalidates capability proofs immediately.

### 11.2 Bounded Loss by Design
Loss is capped by:
- `max_tx_usd`
- `max_day_usd`
- `max_outstanding_usd`
- optional per-merchant / MCC limits

---

## 12. Privacy Model

### 12.1 What must not leak
- Alice’s wallet address / note ownership
- total collateral size
- which asset backs a spend

### 12.2 Capability-Based Privacy
- Processor presents **bearer capability proofs**.
- JAM validates proofs without learning identity (beyond what issuer already knows off-chain).
- No per-swipe on-chain events are required.

### 12.3 Optional Broadcast Notifiers
If you deliver “new activity” notifications:
- Broadcast generic “new batch available”
- Alice’s client matches locally
- **No notifier has Alice’s view key**

---

## 13. Security Requirements

- **Replay protection**: `tx_ref` uniqueness + nonce binding.
- **Double-spend prevention**: reservation accounting is atomic.
- **Time bounds**: capability expiry and hold expiry.
- **Oracle robustness**: slow mark prices, bounded deltas, multi-source aggregation.
- **Rate limiting**: processor and capability-level limits.

**PoC guardrails:**
- Keep all state transitions single-threaded (or guarded by a global mutex) to avoid races while prototyping.
- Emit structured logs for every state transition (`cap_id`, `tx_ref`, old/new state, timestamps).
- Enforce an upper bound on outstanding authorizations at the service level to fail fast if the processor misbehaves.
- Include a `panic_on_invariant_violation` flag for early-stage testing.

---

## 14. Parameters (Suggested Defaults)

- `max_tx_usd`: $50–$200 (product dependent)
- `max_day_usd`: $500–$5,000
- `max_outstanding_usd`: $200–$2,000
- `cap_expiry`: 24h (refresh daily)
- `hold_expiry`: 7 days (typical card auth window)
- `collateral_haircut`: 10–30% depending on asset volatility
- `auth_spread`: 25–100 bps to cover drift + fees

---

## 15. Failure Modes and Handling

### 15.1 Clearing uplift > reserved
- Policy A: require incremental authorization (preferred)
- Policy B: decline uplift beyond reserved
- Policy C: accept up to a small tolerance, charge buffer fund

### 15.2 Settlement partner outage
- Pause new authorizations or reduce limits.
- Continue accepting refunds/clearing records for later settlement.
- Maintain a pre-funded buffer to bridge short outages.

### 15.3 Oracle outage
- Freeze collateral pricing updates.
- Keep spending limits conservative; optionally pause authorizations.
- Resume with safe mark once oracle returns.

---

## 16. Minimal Interface Summary

### Wallet ↔ JAM
- `CreateCapability(limits, expiry) -> capability`
- `RevokeCapability(cap_id) -> ok`

### Processor ↔ JAM
- `AuthorizeSpend(cap_proof, tx_ref, amount) -> approved/auth_code`
- `ClearSpend(tx_ref, final_amount) -> ok`
- `ReportRefund(tx_ref, amount) -> ok`
- `ReportChargeback(tx_ref, outcome, amount) -> ok`

### JAM ↔ Issuer/Processor
- `SettleBatch(batch_id, net_usd_owed, proof/attestation)`

### Storage (internal PoC sketch)
- `capabilities`: map `cap_id -> CapabilityRecord` (limits, expiry, status, rolling counters).
- `auths`: map `tx_ref -> AuthRecord` (amounts, state, timestamps, cap linkage).
- `ledger`: append-only list of `ClearingRecord` and `Refund` events, plus pointers to batch watermarks.
- `oracle_marks`: time-indexed pricing feed with last-good value + staleness checks.

Each map/ledger entry should be verifiable from logs to allow deterministic replays during integration tests.

---

## 17. PoC Implementation Blueprint

The PoC should validate three behaviors: **capability proof verification**, **bounded authorization accounting**, and **batched settlement accounting**. Production-grade issuer connections, PCI concerns, and mobile app polish are explicitly out-of-scope.

### 17.1 PoC Roles and Mock Components
- **Processor shim**: a CLI or service that forwards `SpendAuthRequest` and `ClearingRecord` JSON to JAM over HTTP.
- **Issuer simulator**: script that accepts `SettlementBatch`, records balances, and emits refunds/chargebacks.
- **Price feed stub**: fixed or slowly moving mark price refreshed on a timer (no external oracle dependency).

### 17.2 Minimal Storage Model (JAM-side)
- Capability table keyed by `cap_id` with limits, expiry, status, and counters: `spent_today_usd`, `outstanding_usd`.
- Authorization table keyed by `tx_ref` with `cap_id`, `reserved_usd`, `state`, timestamps, and audit trail of transitions.
- Settlement ledger capturing batches: `batch_id`, `net_usd_owed`, constituent `tx_refs`, and reconciliation status.

### 17.3 Happy-Path Flow (End-to-End)
1. **Capability issuance**: wallet signs a request; JAM stores commitment and returns `cap_id` + proof material.
2. **Auth request**: processor shim POSTs `{cap_proof, tx_ref, amount}`; JAM validates limits and creates a reservation.
3. **Clearing**: processor shim POSTs `{tx_ref, final_amount}`; JAM updates reservation and marks `CLEARED`.
4. **Settlement**: operator triggers `SettleBatch`; JAM computes net USD owed, persists `SettlementBatch`, and sends JSON to issuer simulator.

### 17.4 Negative / Edge Tests
- Decline when `max_tx_usd` or `max_outstanding_usd` is exceeded.
- Expire capability then attempt auth → expect failure.
- Clearing uplift above tolerance → require incremental auth (simulate both accept and reject).
- Refund after settlement → ledger should show collateral claim restored by USD notional.

### 17.5 Observability for the PoC
- Append-only log of state transitions for authorizations and batches (human-readable for demo).
- Metrics counters: approvals/declines, outstanding liability, and per-capability spend.
- Deterministic seeds for capability IDs and auth codes to keep traces reproducible in demos.

### 17.5.1 PoC success checklist
- $20 coffee purchase end-to-end: issue capability → auth → clear → settle; ledger shows bounded counters.
- Auth latency <50ms, capability issuance <100ms; batch settle (100 txs) <500ms.
- Capability expiry + hold expiry exercised: expired capability is declined; expired hold auto-reverses reservation.
- Oracle mark change triggers collateral haircut recomputation without breaking deterministic replay.

### 17.6 Minimal API Sketch (HTTP/JSON)
- `POST /capabilities` → `{cap_id, cap_proof, limits, expiry}`
- `POST /authorize` → `{approved, auth_code, reserved_usd, reason}`
- `POST /clear` → `{tx_ref, state}`
- `POST /refund` → `{tx_ref, refunded_usd}`
- `POST /settle` → `{batch_id, net_usd_owed, tx_refs}`

Each endpoint should update the storage model atomically and emit the observability events above. Authentication can be mocked (e.g., static API key) for the PoC.

---

## 18. Notes on “Partial Liquidation” vs “Retail Spend”
A $20 retail spend is a **voluntary collateral reduction** performed under capability limits. It is **not liquidation**.

Liquidation is only invoked when:
- collateral health falls below maintenance thresholds,
- and user fails to restore health during margin-call/close-only phases.

---

## 19. Open Questions (for implementation)
- Choice of collateral valuation mark (daily TWAP vs 4x/day snapshots).
- Settlement rail (stablecoin to bank wallet vs netted cash buffer).
- Capability proof construction (signature vs ZK-capability).
- Offline spend support (likely out-of-scope; increases risk).
- Regulatory boundary for issuer-held data vs protocol privacy.

---

## 19. Proof-of-Concept Build Plan

**Goal:** a minimal, end-to-end prototype that proves reservation accounting, batch settlement, and capability privacy without full card network integration.

### Scope
- Single collateral asset with fixed oracle feed.
- One processor identity with static API key.
- In-memory state with periodic snapshotting to disk (JSON) for restartability.
- No ZK yet: capability = signed token using a blinded nonce; proof = signature check.

### Phases (7-day implementation)

**Days 1-2: Foundation**
1. **Data model + invariants**
   - Implement `CapabilityRecord`, `AuthRecord`, ledger entries, and watermarking.
   - Add invariant checks: outstanding ≤ limits, counters monotonic, no duplicate `tx_ref`.
   - Target: Core structures with in-memory persistence and JSON serialization

**Days 3-4: Core Authorization Logic**
2. **API surface** (REST/JSON for simplicity)
   - `POST /capabilities`: issue capability token with returned `cap_id` + expiry.
   - `POST /authz`: verify capability proof, place reservation, return `auth_code`.
   - `POST /clearing`: accept batch of `ClearingRecord`s, transition states, emit log root.
   - Target: <50ms response time for auth requests

**Days 5-6: Settlement and Risk**
3. **Pricing + risk controls**
   - Integrate oracle mock with configurable mark + haircut (fixed $40k BTC price for PoC).
   - Apply spreads at authorization and recompute at clearing to detect uplifts.
4. **Settlement flow**
   - `POST /refunds` & `POST /chargebacks`: adjust reservations/claims.
   - `POST /settle`: finalize batch and write settlement artifact.

**Day 7: Integration and Testing**
5. **Integration test harness**
   - Happy path: provision → auth → clear → settle.
   - Timeouts: auth expiry -> reverse.
   - Edge cases: uplift > reserved, oracle stale, capability revoked.
6. **Demo CLI**
   - End-to-end scenario simulating $20 coffee purchase with BTC collateral
   - Automated verification of settlement math

**Performance targets:**
- Authorization latency: <50ms per request
- Settlement processing: <500ms per batch (100 transactions)
- Memory usage: <64MB for 1000 active capabilities
- Capability generation: <100ms per issuance
- Deterministic replay: All state transitions reproducible from logs

### Stretch (optional)
- Add a simple front-end “simulator” to issue capabilities and send auths for manual testing.
- Add Merkle proofs for inclusion of `tx_ref` in settlement artifacts.
- Swap in disk-backed storage (e.g., Badger/SQLite) behind the same interface.
