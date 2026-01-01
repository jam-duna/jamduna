# WOLK / JAM Duna Architecture

This document describes the **final, internally consistent entity and revenue architecture** for the WOLK / JAM ecosystem after removing bank dependency and adopting a stablecoin-native model with issuers (Brale, Bridge, ...).

The design intentionally separates **protocol neutrality**, **service risk**, and **user-facing settlement** so that each layer can scale, fail, or be regulated independently without collapsing the system.

---

## 1. Core Design Principles

1. **No single entity does everything**
2. **Risk lives where services are built**
3. **Protocol is neutral infrastructure, not a business**
4. **Stablecoins are settlement, not deposits**
5. **No user earns yield by holding balances**

---

## 2. Entities Overview

### A) JAM Duna (Wyoming)

**Role:** Protocol steward and neutral settlement layer

**What JAM Duna does:**

* Maintains the JAM protocol specification
* Coordinates validator rules and upgrades
* Defines work package formats and guarantees
* Operates governance (12 independent implementer teams)
* Holds protocol treasury (stablecoin-denominated)

**What JAM Duna does NOT do:**

* Does not run services
* Does not interact with users
* Does not issue stablecoins
* Does not operate perps or trading
* Does not promise returns or yield

**Key property:**

> JAM Duna is infrastructure, not a financial intermediary.

---

### B) Validators (Independent Operators)

**Role:** Security and finality

**Validator compensation:**

* Paid for guaranteeing work packages
* Compensation is protocol-defined and statistical
* Based on uptime, correctness, availability, and slashing history

**Validators do NOT:**

* Share in service profits
* Earn protocol yield
* Touch user funds
* Set economic parameters for services

Validators are compensated like infrastructure providers, not market participants.

---

### C) Wolk (Service Builder Network)

**Role:** Service execution and economic risk

Wolk operates a **portfolio of zk-based services** on top of JAM:

#### Service classes

1. **Spot / custody-like services**

   * BTC / ETH trading
   * 1:1 settlement
   * No leverage
   * ZK balance privacy

2. **Private securities services**

   * Private stock trading (Carta-like)
   * Permissioned participants only
   * ZK cap table and transfer logic

3. **Synthetic / derivatives services**

   * Perps and index exposure
   * Synthetic settlement
   * Strict geo-fencing
   * Highest regulatory risk

**Wolk characteristics:**

* Users pay Wolk directly for services
* Wolk pays validators to submit work packages
* Wolk bears regulatory and business risk
* Wolk does not issue stablecoins
* Wolk does not distribute protocol rewards

**Important:**

> The highest-risk service Wolk operates (e.g. perps) defines Wolk’s regulatory posture.

---

### D) Stablecoin Issuer

**Role:** Stablecoin mint / burn and settlement rails

Stablecoin Issuer provides:

* Regulated stablecoin issuance
* Minting and redemption APIs
* On-chain settlement
* Reserve-backed stablecoins

Stablecoin Issuer does NOT:

* Run services
* Operate markets
* Offer leverage
* Pass yield to users or Wolk

Stablecoins are the **unit of account and settlement**, not investment products.

---

## 3. Money Flow

### Service and security flow

```
Users
  ↓ (service fees, margin, settlement)
Wolk Services
  ↓ (work package fees)
Validators
```

### Stablecoin settlement

```
Stablecoin Issuer
 ↔ mint / burn
Users & Wolk
```

### Protocol treasury

```
Penalties / inefficiencies / idle balances
        ↓
   JAM Duna Treasury
```

---

## 4. Stablecoin Yield (Critical Clarification)

* Yield arises **only** from protocol-owned balances in the JAM Duna treasury
* Yield is indistinguishable from protocol revenue
* No entity has a claim on yield

**Explicitly excluded:**

* User yield
* Validator yield
* Wolk yield
* Stablecoin Issuer pass-through yield

If users did not exist, yield would still accrue. This is the defining test.

---

## 5. Rewards (Optional, Non-Entitlement)

* JAM Duna may make discretionary grants
* Grants may fund ecosystem growth or user rewards
* Rewards are promotional, not formulaic
* No APY, no entitlement, no balance-based accrual

Rewards are spend, not yield.

---

## 6. Governance

* JAM Duna governance is composed of **12 independent implementer teams**
* No single team controls:

  * protocol execution
  * treasury unilaterally
  * validator admission

Governance controls **rules**, not services.

---

## 7. What This Architecture Achieves

* Clear isolation of regulatory risk
* Validators remain neutral infrastructure
* Services can innovate or fail independently
* Stablecoins remain settlement-only
* No hidden deposits or yield products

This design favors **survivability over shortcuts**.

---

## 8. Non-Goals (Explicit)

This architecture does NOT attempt to:

* Avoid regulation entirely
* Hide derivatives activity
* Promise user returns
* Make stablecoins behave like deposits

Instead, it makes responsibility and risk attribution explicit.

---

## 9. One-Line Summary

**JAM Duna defines the rules, validators provide security, Wolk builds and operates services, Stablecoin Issuer provides stablecoin settlement, and no user earns yield simply by holding money.**
