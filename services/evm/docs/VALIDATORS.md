# Validator Adjustment Design

**Status**: Design Specification → adds PoC guidance
**Scope**: Epoch-level adjustment of JAM cores (`C`) and validators (`V = 3C`) using utilization, validator performance, and stake requirements.

---

## 1. Goals

* Dynamically scale execution capacity (cores) with real demand
* Maintain safety invariant `V = 3C`
* Remove non-live or low-performance validators deterministically
* Admit new validators from a queue in a fair, stake-backed manner
* Avoid oscillation and excessive churn
* Deliver a minimal, testable PoC that proves end-to-end adjustment behavior

---

## 1.1 PoC Objectives

* Demonstrate a full epoch transition with synthetic metrics (utilization, queue depth, validator scores) and reproducible outcomes
* Provide API-level hooks for instrumentation so metrics can be swapped for live data without rewriting the algorithm
* Record every decision (scale up/down, removals, admissions) into a trace log for deterministic replay during testing
* Ship an executable harness that runs multiple epochs from a fixture, producing the next-epoch validator set as a manifest

## 2. Observed Inputs (Per Epoch)

### 2.1 Core / System Utilization

For each core `c`:

* `U_c`: utilization (fraction of timeslots with guaranteed work packages or gas usage)
* `Q_c`: contention / backlog indicator (e.g. rejected WPs, authorization clearing price)

Aggregates:

* `U_med`, `U_p90`
* `Q_med`, `Q_p90`

### 2.2 Validator Metrics

For each validator `i`:

* `L_i`: liveness (missed duties rate)
* `P_i`: performance (verification latency, throughput, success rate)
* `B_i`: bad events (timeouts, invalid guarantees, equivocation)
* `stake_i`: JAM stake

### 2.3 PoC Metric Fixtures

For the initial PoC, consume metrics from a static fixture file (e.g. JSON or YAML) with the shape:

```json
{
  "epoch": 42,
  "cores": 64,
  "utilization": {"p90": 0.71, "median": 0.63},
  "queue": {"p90": 0.18, "median": 0.12},
  "validators": [
    {"id": "val-001", "stake": "500000000000000000000", "miss_rate": 0.02, "performance": 0.91, "bad": 0.0},
    {"id": "val-002", "stake": "400000000000000000000", "miss_rate": 0.11, "performance": 0.72, "bad": 0.0}
  ],
  "queue_candidates": [
    {"id": "cand-101", "stake": "450000000000000000000", "availability": 0.95}
  ]
}
```

Use deterministic seeds to generate synthetic sequences and enable replay testing.

---

## 3. Core Count Adjustment (C)

### 3.1 Targets and Hysteresis

Parameters:

* `U_target` (e.g. 0.60)
* `U_high = U_target + h` (e.g. 0.75)
* `U_low  = U_target - h` (e.g. 0.45)
* `h`: hysteresis band

### 3.2 Scaling Rules

At each epoch boundary:

* **Scale Up** if:

  * `U_p90 > U_high` **OR** `Q_p90 > Q_high`
  * condition holds for `K_up` consecutive epochs

* **Scale Down** if:

  * `U_p90 < U_low` **AND** `Q_p90 < Q_low`
  * condition holds for `K_down` consecutive epochs

### 3.3 Safety Constraints

* `C_new = clamp(C_old ± ΔC, C_min, C_max)`
* `|ΔC| ≤ C_old × r` (rate limit, e.g. 5–10% per epoch)
* Cooldown: after any change, suppress further changes for `N` epochs unless emergency

### 3.4 Validator Count

* `V_new = 3 × C_new`

### 3.5 PoC Controls

* Expose `U_high`, `U_low`, `Q_high`, `Q_low`, `K_up`, `K_down`, and `r` as CLI flags or environment variables for rapid tuning
* Emit a decision record:
  * `epoch`, previous `C`, proposed `C_new`, `reason` (e.g. `util_p90_high`, `queue_high`), `persistence_counter`
  * Guardrail decisions (`rate_limit`, `cooldown_active`) should be explicit booleans for debugging
* Include a dry-run mode that prints the computed `C_new`/`V_new` without mutating any persistent state

---

## 4. Validator Quality and Removal

### 4.1 Quality Score

For ranking and soft removal:

```
score_i = w_L(1 - miss_rate_i)
        + w_P performance_i
        - w_B bad_event_rate_i
```

Weights `w_L, w_P, w_B` are protocol constants.

### 4.2 Hard Removal (Immediate)

A validator is **ineligible** for the next epoch if:

* `stake_i < MIN_STAKE`
* `miss_rate_i > X_hard` (e.g. >20%)
* Slashable offense (equivocation, invalid guarantees)

These validators are removed before any size adjustment.

### 4.3 Soft Removal (Probation)

* `miss_rate_i > X_soft` (e.g. >5%) **OR** `performance_i < P_min`
* If condition persists for `M` consecutive epochs → remove

### 4.4 Churn Limits

* Max removals per epoch capped at `max_churn_ratio × V`
* Hard failures bypass churn cap

### 4.5 PoC Scoring Module

* Implement scoring as a pure function that accepts the fixture objects from Section 2.3 and returns `(eligible, score, reason)` tuples
* Track provenance: when a validator is excluded, log whether it was due to hard removal, soft removal, or churn pressure
* Provide a toggle to switch between stake-weighted scores and flat weights so performance sensitivity can be evaluated quickly

---

## 5. Validator Admission Queue

### 5.1 Eligibility

Candidates must satisfy:

* `stake ≥ MIN_STAKE`
* Valid keys and identity
* Optional pre-validator availability checks

### 5.2 Selection

When adding validators:

* Prefer higher stake
* Prefer candidates with demonstrated availability
* Optionally enforce diversity constraints

### 5.3 Warmup

New validators:

* Enter a warmup state for 1 epoch
* May be assigned reduced or non-critical duties

---

## 6. Epoch Transition Algorithm

At each epoch boundary:

1. Compute `U_p90`, `Q_p90`
2. Determine `C_new` using Section 3
3. Set `V_new = 3 × C_new`
4. Remove hard-fail validators (`stake`, slashing)
5. Rank remaining validators by `score_i`
6. If `|active| > V_new`: remove lowest-ranked (respect churn cap)
7. If `|active| < V_new`: promote from queue
8. Apply warmup status to new validators
9. Finalize assignments for next epoch

### 6.1 PoC Algorithm Skeleton

```pseudo
input: fixture_epoch

util = fixture_epoch.utilization
queue = fixture_epoch.queue
validators = fixture_epoch.validators
candidate_queue = fixture_epoch.queue_candidates

decision = adjust_core_count(util, queue)
v_target = decision.cores * 3

hard_failures = filter(validators, fails_hard_rules)
eligible = validators - hard_failures

scored = map(eligible, compute_score)
sorted = sort_by_score(scored)

removals = max(0, len(sorted) - v_target)
kept = drop_lowest(sorted, removals, churn_cap)

promotions = pop_queue(candidate_queue, v_target - len(kept))

output_manifest = {
  "epoch": fixture_epoch.epoch + 1,
  "cores": decision.cores,
  "validators": kept + promotions,
  "trace": {
    "core_decision": decision,
    "hard_failures": hard_failures,
    "removed_for_churn": removals,
    "promoted": promotions
  }
}

write_json(output_manifest, stdout | file)
```

The PoC harness can run `N` epochs by chaining the manifest as the next fixture's input, letting testers observe steady-state behavior.

---

## 7. Minimum Stake Enforcement

* Stake is checked **at epoch boundary**
* Validators below `MIN_STAKE` are not eligible for the next epoch
* Optional short grace period may be allowed for non-slashing stake dips

---

## 8. Stability Guarantees

This design ensures:

* No oscillation via hysteresis and persistence requirements
* Bounded validator churn
* Demand-driven scaling of execution capacity
* Continuous removal of non-live or low-quality validators
* Deterministic, stake-backed validator admission

### 8.1 PoC Verification and Tooling

**7-day implementation schedule:**

**Days 1-2: Core Algorithm**
- Implement core adjustment logic: utilization analysis, validator scoring, and epoch transition algorithm
- JSON fixture parsing and deterministic seed handling
- Target: Core algorithm processes 1000 validators in <100ms

**Days 3-4: CLI and Simulation**
- CLI command: `validators-sim --fixture fixtures/epoch.json --epochs 5`
- Deterministic multi-epoch simulation with reproducible outputs
- JSON Lines trace format for automated analysis

**Days 5-6: Test Scenarios**
- Golden fixtures for key scenarios:
  * Scale-up path (high utilization/backlog)
  * Scale-down path (idle cores)
  * Churn cap enforcement with mixed hard/soft failures
- Automated verification against expected outcomes

**Day 7: Integration and Validation**
- Human-readable output formatting and summary statistics
- Performance profiling and optimization for large validator sets
- End-to-end validation: deterministic replay produces identical results

**Performance targets:**
- Algorithm execution: <100ms for 1000 validators per epoch
- Memory usage: <64MB for full simulation state
- Simulation speed: 100 epochs in <10s
- Deterministic guarantee: Identical outputs for same fixtures

**Golden fixtures + CLI expectations**
- Provide sample fixtures under `services/evm/docs/fixtures/validators/*.json` (scale-up, scale-down, churn-cap, slashing).
- `validators-sim --fixture fixtures/scale_up.json --epochs 3` should emit:
  - `manifest_epoch_*.json`: final validator set per epoch (golden-checked in CI)
  - `trace_epoch_*.jsonl`: decision logs (core changes, removals, promotions)
  - `summary.txt`: human-readable recap with timing metrics
- CI should diff manifests against goldens and fail on any drift to ensure algorithm stability.

**CLI outputs include:**
* Final validator manifest (JSON)
* Per-epoch trace logs in JSON Lines format for machine inspection
* Human-readable summary (scale decisions, removals, promotions)
* Performance metrics (processing time, memory usage)

---

## 9. Tunable Parameters (Protocol Constants)

* `U_target`, `U_high`, `U_low`
* `Q_high`, `Q_low`
* `K_up`, `K_down`
* `r` (max core change rate)
* `MIN_STAKE`
* `X_soft`, `X_hard`, `M`
* `max_churn_ratio`

These may be adjusted via governance as the network matures.

---

## 10. Summary

DESIGNATE defines a conservative, load-responsive mechanism for adjusting JAM cores and validators while maintaining safety, performance, and economic accountability. It cleanly separates **capacity scaling**, **validator quality enforcement**, and **admission**, making the system predictable and robust under growth or contraction.
