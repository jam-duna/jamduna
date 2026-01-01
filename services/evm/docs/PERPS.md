# JAM Slow Perpetuals (PERPS)

This document specifies the **Slow Perpetuals (PERPS) service** for JAM.

Slow perps are **margin-settled perpetual derivatives** designed for **days-long risk management**, strong privacy, and minimal reflexive liquidation. They intentionally resemble **TradFi margin finance**, not high-frequency crypto perps.

---

## 1. Design Goals

* Days-long margin and liquidation timelines
* Low leverage, high robustness
* Privacy-preserving positions (no public sizes or liquidation prices)
* Deterministic, slashable enforcement
* Oracle-driven reference prices (not tick-level trading prices)
* JAM-native state machines (epoch-based)

Non-goals:

* High-frequency trading
* 20–100× leverage
* Tick-level mark prices
* Public orderbooks

---

## 2. Supported Markets

PERPS targets **macro, low-volatility assets** only.

Examples:

* Equity indices (SP500, Nasdaq-100, MSCI World, Euro Stoxx 50)
* Rates (US Treasuries, Bunds, Gilts)
* FX majors (EUR/USD, USD/JPY, GBP/USD)
* Commodities (Gold, Oil, Copper)
* Crypto majors (BTC, ETH)

Each market defines:

* Max leverage (typically 2–5×)
* Margin bands
* Oracle cadence
* Volatility limits

---

## 3. Position Model (Private)

Positions are **commitment-based**.

A user position is represented on-chain only by:

```
PositionCommitment = H(q, entry_price, margin, nonce)
```

Publicly visible:

* Commitment hash
* Margin balance
* Market ID
* Position state

Hidden until required:

* Position size
* Entry price
* Liquidation price

Reveals occur **only** during settlement, reduction, or liquidation, and only to the protocol.

---

## 4. Margin Bands & States

Each position moves through deterministic states:

| State       | Condition                        | User permissions         |
| ----------- | -------------------------------- | ------------------------ |
| HEALTHY     | Equity ≥ Initial Margin (IM)     | Full                     |
| MARGIN_CALL | Equity < IM                      | Full                     |
| CLOSE_ONLY  | Equity < Maintenance Margin (MM) | Reduce / add margin only |
| REDUCING    | Equity < Buffer Margin (BM)      | Protocol reduces size    |
| LIQUIDATED  | Insolvent after reductions       | Closed                   |

Typical defaults (SP500):

* IM: 20%
* MM: 10%
* BM: 6%

---

## 5. Timeline (Days-Long Enforcement)

Slow perps intentionally stretch enforcement over **days**.

Example configuration:

* Margin call window: 48 hours
* Close-only window: 48–72 hours
* Forced reduction phase: multi-day
* Final liquidation: rare, last resort

Liquidation is **never instantaneous** under normal conditions.

---

## 6. Forced Reduction (Key Primitive)

Instead of full liquidation, PERPS uses **forced reduction**.

During `REDUCING`:

* Position size is gradually decreased (e.g. 10–20% per epoch)
* Goal: restore equity ≥ MM
* User retains remaining position

This mirrors broker sell-downs and dramatically reduces cascades.

---

## 7. Mark Price & Funding

### Mark Price

* Derived from **reference oracle prices**
* Uses long TWAPs or official settlement prices
* Clamped vs index to prevent manipulation

For indices: session close or 1-day TWAP.

### Funding

Funding aligns perp price to index price over time.

Per funding interval:

```
premium = (mark - index) / index
rate = clamp(k * premium, -cap, +cap)
funding_index += mark * rate
```

Each position settles funding lazily using a cumulative funding index.

Funding continues during margin calls and close-only phases, slowly bleeding risk.

---

## 8. Liquidation Windows & Auctions

When forced reduction fails:

1. A **liquidation window** opens
2. User is warned and given final time to act
3. Liquidation occurs via **sealed-bid auction**
4. Threshold executor selects best bid

This prevents:

* Liquidation hunting
* MEV races
* Front-running

---

## 9. Oracle Model

PERPS relies on **reference-price oracles**, not spot feeds.

Oracle design:

* Threshold oracle committee
* Each signer fetches data independently
* JAM verifies quorum signatures

Sources:

* Equity indices: official index publishers
* FX: WM/Refinitiv fixes
* Commodities: ICE / LBMA settlements
* Crypto: multi-exchange medians

Cadence:

* Indices / FX: daily or hourly
* Rates / commodities: daily
* Crypto: 1–4h

Oracle signers are slashable for:

* Missed updates
* Equivocation
* Out-of-bounds prices

---

## 10. Emergency Mode

If markets move beyond modeled risk:

Emergency mode may:

* Shorten windows
* Accelerate reductions
* Temporarily halt new positions

Emergency activation is explicit and auditable.

---

## 11. Risk Controls

Global controls:

* Max open interest per market
* Max leverage caps
* Insurance fund
* Volatility circuit breakers

PERPS prioritizes **system solvency over speed**.

---

## 12. Privacy Properties

PERPS provides:

* Hidden position sizes
* Hidden liquidation prices
* Delayed revelation only when necessary
* No public liquidation sniping

What remains public:

* Aggregate market health
* Funding direction
* State transitions

---

## 13. Mental Model

> Slow perps are margin finance with crypto-native enforcement.

They are designed for:

* Investors
* Hedgers
* Long-horizon exposure

Not for:

* Scalpers
* High-frequency traders
* Reflexive leverage

---

## 14. Summary

The JAM PERPS service:

* Trades speed for robustness
* Trades leverage for safety
* Trades transparency for privacy

It brings **clearing-house-style discipline** into a decentralized, programmable system.

---

## 15. PoC Implementation Outline

The goal of the initial proof of concept is to **validate the state machine, margin math, and oracle plumbing** with minimal surface area. The PoC should prioritize determinism, auditability, and testability over feature breadth.

### Scope & Non-Goals

**Included in PoC:**

* Single market (e.g., SP500 synthetic) with fixed leverage and margin bands
* Simplified oracle feed (threshold-signed price once per epoch)
* Position lifecycle: open → margin call → close-only → reducing → liquidation
* Funding accrual using cumulative index
* Off-chain sealed-bid liquidation interface mocked as deterministic input

**PoC success criteria (single-market, mock oracle)**
- Deterministic 7-day script that replays oracle prices and position actions with identical outputs on rerun.
- Margin math validated with a fixed SP500-style config: IM 20%, MM 10%, BM 6%, reduction rate 15%/epoch.
- Mock oracle update accepted/rejected paths both exercised (good signature vs stale timestamp).
- State replay proves no negative equity escapes liquidation; per-epoch processing remains <1s for 100 positions.

**Explicitly excluded:**

* Multi-market routing and cross-margining
* On-chain auctions or order books
* Advanced privacy tooling (zk proofs); use commitment reveals only when needed
* Dynamic parameter governance

### Minimal State Machine

Each position is tracked as a row keyed by commitment hash:

| Field               | Description                                        |
| ------------------- | -------------------------------------------------- |
| `commitment`        | `H(q, entry_price, margin, nonce)`                 |
| `market_id`         | Fixed to the single PoC market                     |
| `margin`            | Current margin balance (public)                    |
| `state`             | One of HEALTHY / MARGIN_CALL / CLOSE_ONLY / REDUCING / LIQUIDATED |
| `funding_checkpoint`| Funding index at last settle                       |
| `nonce`             | Prevents replay when reopening with same params    |

State transitions are event-driven:

* **Open:** user commits hash, posts margin, receives `commitment` handle
* **Oracle update:** recompute equity vs IM/MM/BM to determine state
* **User actions:** add margin, reduce position (reveals partial size/proof)
* **Protocol actions:** stepwise reductions when below BM
* **Liquidation:** triggered after reduction window expires and equity < 0

### Price & Funding Loop

1. **Epoch start:** ingest oracle price (threshold signature) and store `P_epoch`
2. **Mark calc:** derive mark price via TWAP clamp vs previous `P_epoch`
3. **Funding:** update `funding_index` with capped premium formula
4. **Health checks:** recompute states for all active commitments

### Liquidation & Reduction PoC Flow

* **Reduction step:** deterministic percentage trim (e.g., 15%) each epoch in `REDUCING`
* **Auction placeholder:** sealed-bid winner provided as signed message; settlement burns position and pays out
* **Payout order:** auction proceeds → insurance fund top-up → residual to user

### Storage Layout (EVM Service)

* **Positions:** mapping `commitment → PositionRecord`
* **Funding:** global `funding_index` per market + per-position checkpoints
* **Oracle:** latest signed price blob + signer set ID
* **Risk params:** IM/MM/BM, reduction rate, windows stored in immutable config slot for PoC

### Test Plan

* **Unit tests:**
  * Margin math across state transitions
  * Funding accrual and settlement
  * Oracle signature verification and staleness rejection
  * Reduction ladder converging to solvency
* **Integration harness:** scripted timeline over multiple epochs simulating user margin calls, reductions, and final liquidation
* **Fuzz cases:** random price paths ensuring no negative equity escapes liquidation

### Milestones (7-day sprint plan)

**Days 1-2: Foundation**
1. **Scaffold contracts/storage:** define structs, state machine enums, and parameter constants
   - Target: Core data structures with in-memory storage for PoC
   - Single market (SP500 synthetic) with fixed IM/MM/BM ratios

**Days 3-4: Core Logic**
2. **Oracle ingestion:** implement threshold signature verification and price storage
   - Mock oracle with deterministic price feeds (can use simple linear progression)
   - Focus on signature verification plumbing, not complex aggregation
3. **Health engine:** per-epoch update that drives state transitions
   - Implement margin band calculations and state transitions
   - Target: <1s processing for 100 positions per epoch

**Days 5-6: User Interface**
4. **Position lifecycle:** open/close, add margin, optional reveal for reduction
   - CLI tools for position management and margin operations
   - Commitment-reveal flow for position parameters

**Day 7: Integration**
5. **Reduction/liquidation:** deterministic reduction loop plus auction placeholder
   - Simple percentage-based reduction (15% per epoch in REDUCING state)
   - Mock sealed-bid auction that accepts deterministic winner
6. **Test harness:** deterministic epoch scripts to exercise all paths
   - End-to-end scenario: healthy → margin call → close-only → reducing → liquidated

**Performance targets:**
- Position state updates: <10ms per position
- Oracle price ingestion: <100ms per update
- Memory usage: <256MB for 1000 positions
- Deterministic replay: exact same outputs for same inputs

The PoC should ship with deterministic fixtures (fixed signer keys, fixed market parameters) so it can be run in CI and replayed exactly, making it easy to audit the PERPS logic before expanding scope.
