# Compliant S&P 500 Tokenized Fund
## ERC-4626 Vault + ERC-3643/T-REX Compliance

A production-ready smart contract system for issuing compliant, NAV-based, on-chain S&P 500-style tokenized securities to institutional investors.

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Investor Interface                        │
│  (Frontend / API for KYC, Deposits, Redemptions, Transfers) │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                  FundVault                         │
│                    (ERC-4626 + Access Control)               │
│  • NAV-based pricing                                         │
│  • T+n redemption settlement                                 │
│  • Management fee collection                                 │
│  • Minimum investment enforcement                            │
└─────────┬───────────────────────────┬───────────────────────┘
          │                           │
          ▼                           ▼
┌──────────────────────┐    ┌──────────────────────┐
│  CompliantShareToken │    │      NAVOracle       │
│    (ERC-20 + T-REX)  │    │  • Price updates     │
│  • Transfer hooks    │    │  • Historical NAV    │
│  • Lockup periods    │    │  • Asset valuation   │
│  • Account freezing  │    └──────────────────────┘
└──────────┬───────────┘
           │
           ▼
┌─────────────────────────────────────────────────────────────┐
│                    Compliance Layer                          │
├──────────────────────┬──────────────────────────────────────┤
│  IdentityRegistry    │         ComplianceModule             │
│  • KYC/AML status    │  • Transfer restrictions             │
│  • Country codes     │  • Holder limits (Reg D: 2000)       │
│  • Accreditation     │  • Country-specific caps             │
│  • Expiry tracking   │  • Daily/monthly limits              │
└──────────────────────┴──────────────────────────────────────┘
```

---

## ✨ Key Features

### 1. **ERC-4626 Vault Standard**
- Standard interface for tokenized vaults
- Predictable deposit/withdraw mechanics
- Compatible with DeFi integrations
- Composable with other protocols

### 2. **ERC-3643 / T-REX Compliance**
- On-chain identity verification
- Transfer restrictions based on KYC/AML
- Country-based restrictions
- Accredited investor requirements
- Automatic compliance enforcement

### 3. **NAV-Based Pricing**
- Oracle-driven fair value pricing
- Daily NAV updates
- Historical NAV tracking
- Transparent valuation methodology

### 4. **Institutional Features**
- Minimum investment thresholds ($100k default)
- T+2 redemption settlement
- Management fee automation (2% annual)
- Lockup periods (90 days default)
- Emergency pause functionality
- Account freezing capabilities

### 5. **Regulatory Compliance**
- **Reg D**: Max 2000 investors, accredited only
- **Reg S**: Offshore distribution controls
- **1940 Act Exemptions**: Configurable holder limits
- **AML/KYC**: Identity registry with expiry
- **Transfer Restrictions**: Enforced on-chain

---

## 📋 Contract Overview

### Core Contracts

| Contract | Purpose | Standards |
|----------|---------|-----------|
| `FundVault` | Main investment vault | ERC-4626 |
| `CompliantShareToken` | Share token with restrictions | ERC-20 + T-REX |
| `IdentityRegistry` | KYC/AML verification | ERC-3643 |
| `ComplianceModule` | Transfer compliance rules | T-REX |
| `NAVOracle` | Price feed and valuation | Custom |

---

## 🚀 Deployment

### Prerequisites

```bash
# Install dependencies
npm install @openzeppelin/contracts
```

### Deployment Sequence

```solidity
// 1. Deploy Identity Registry
IdentityRegistry identityRegistry = new IdentityRegistry();

// 2. Deploy Compliance Module
ComplianceModule compliance = new ComplianceModule(address(identityRegistry));
compliance.setMaxHolders(2000); // Reg D limit

// 3. Deploy NAV Oracle
NAVOracle navOracle = new NAVOracle();

// 4. Deploy Vault (automatically deploys share token)
FundVault vault = new FundVault(
    IERC20(usdc),                    // Asset (USDC)
    "SP500 Institutional Fund",      // Name
    "SP500",                         // Symbol
    address(identityRegistry),       // Identity registry
    address(compliance),             // Compliance module
    address(navOracle),              // NAV oracle
    100_000 * 1e6                    // $100k minimum
);
```

### Initial Configuration

```solidity
// Grant operational roles
identityRegistry.grantRole(AGENT_ROLE, kycProvider);
compliance.grantRole(COMPLIANCE_ROLE, complianceOfficer);
navOracle.grantRole(ORACLE_ROLE, priceUpdater);

// Configure compliance rules
compliance.setCountryLimit(840, 500);  // USA: max 500 investors
identityRegistry.allowCountry(826);    // Allow UK investors
```

---

## 💼 Operational Workflows

### 1. Investor Onboarding

```solidity
// Off-chain: Complete KYC/AML verification

// On-chain: Register verified investor
identityRegistry.registerIdentity(
    investorAddress,
    840,                           // Country code (USA)
    true,                          // Accredited investor
    block.timestamp + 365 days,    // Verification expiry
    keccak256(offChainDataHash)    // Identity hash
);
```

### 2. Investment (Subscription)

```solidity
// Investor approves USDC
usdc.approve(vaultAddress, investmentAmount);

// Investor deposits
uint256 shares = vault.deposit(investmentAmount, investorAddress);

// Vault automatically:
// ✓ Updates NAV
// ✓ Verifies KYC status
// ✓ Checks accreditation
// ✓ Calculates shares = amount / NAV
// ✓ Mints shares with 90-day lockup
```

### 3. NAV Update (Daily)

```solidity
// Calculate off-chain total portfolio value
uint256 totalAssetValue = calculatePortfolioValue();
uint256 totalShares = vault.shareToken().totalSupply();

// Update NAV on-chain
navOracle.updateNAV(totalAssetValue, totalShares);

// New NAV = totalAssetValue / totalShares
```

### 4. Redemption (T+2 Settlement)

```solidity
// Investor requests redemption
uint256 requestId = vault.requestRedemption(shareAmount);

// Shares are immediately burned and escrowed

// After T+2 settlement period
vault.processRedemption(requestId);

// Investor receives USDC = shares × NAV
```

### 5. Management Fee Collection

```solidity
// Automatic during deposits/redemptions, or manual:
vault.collectManagementFee();

// Calculates time-weighted fee
// Mints new shares to vault (dilutes existing holders)
// Default: 2% annual = 0.0055% daily
```

---

## 🔐 Compliance Controls

### Account Freezing

```solidity
// Freeze suspicious account
vault.shareToken().freezeAccount(investorAddress);

// Prevents all transfers
// Can be unfrozen when resolved
```

### Transfer Limits

```solidity
// Set daily and monthly limits
compliance.setTransferLimit(
    investorAddress,
    1_000_000 * 1e18,  // Daily: $1M
    5_000_000 * 1e18   // Monthly: $5M
);
```

### Emergency Pause

```solidity
// Halt all operations
vault.pause();

// Use cases:
// - Market circuit breakers
// - Regulatory investigation  
// - Smart contract upgrade
// - Force majeure
```

### Country Restrictions

```solidity
// Block specific countries
identityRegistry.blockCountry(643);   // Russia

// Allow new countries
identityRegistry.allowCountry(826);   // UK
```

---

## 📊 NAV Calculation Example

```javascript
// Daily NAV calculation (off-chain)
const sp500IndexValue = 5000.00;  // Current S&P 500 level
const portfolioShares = 200;       // S&P 500 index shares held
const cashReserves = 50000;        // USDC reserves
const expenses = 2000;             // Accrued expenses

const totalAssetValue = 
    (sp500IndexValue * portfolioShares) + 
    cashReserves - 
    expenses;

// totalAssetValue = $1,048,000

// Update on-chain
await navOracle.updateNAV(
    ethers.utils.parseUnits(totalAssetValue.toString(), 6),
    await shareToken.totalSupply()
);

// If 1,000,000 shares outstanding:
// NAV per share = $1,048,000 / 1,000,000 = $1.048
```

---

## 📈 Example Scenarios

### Scenario 1: Initial Investment

```
Investor A:
- Deposits: $100,000 USDC
- Current NAV: $1.00
- Shares received: 100,000
- Lockup: Until day 90
```

### Scenario 2: S&P 500 Gains 5%

```
Day 30:
- NAV updated to $1.05
- Investor A balance: 100,000 shares × $1.05 = $105,000
- Unrealized gain: $5,000 (5%)
```

### Scenario 3: New Investor B

```
Day 60:
- Investor B deposits: $210,000 USDC
- Current NAV: $1.05
- Shares received: 200,000
- Total shares outstanding: 300,000
```

### Scenario 4: Management Fees

```
Day 365:
- 2% annual fee = 6,000 new shares minted to vault
- Total shares: 306,000
- NAV dilution: ~2%
```

### Scenario 5: Redemption

```
Day 100 (after lockup):
- Investor A requests redemption of 50,000 shares
- Current NAV: $1.10
- T+2 settlement initiated

Day 102:
- Redemption processed
- USDC received: 50,000 × $1.10 = $55,000
- Profit: $5,000 on $50,000 invested
```

---

## 🔍 Compliance Matrix

| Requirement | Implementation | Status |
|-------------|----------------|--------|
| **KYC/AML** | Identity Registry with verification expiry | ✅ |
| **Accredited Investors** | Checked on deposit | ✅ |
| **Transfer Restrictions** | Enforced by Compliance Module | ✅ |
| **Holder Limits** | Configurable max holders (2000 default) | ✅ |
| **Country Restrictions** | Whitelist/blacklist by country code | ✅ |
| **Lockup Periods** | 90-day default, per-investor configurable | ✅ |
| **Securities Law** | Reg D / Reg S compatible | ✅ |
| **1940 Act Exemption** | 3(c)(1) or 3(c)(7) configurable | ✅ |
| **Account Freezing** | Admin function for compliance | ✅ |
| **Transfer Limits** | Daily and monthly caps | ✅ |
| **Emergency Stop** | Pausable by compliance officer | ✅ |

---

## 🛡️ Security Considerations

### Access Control

```solidity
Roles:
- DEFAULT_ADMIN_ROLE: Full system control
- FUND_MANAGER_ROLE: NAV updates, fee management
- ORACLE_MANAGER_ROLE: Oracle configuration
- COMPLIANCE_OFFICER_ROLE: Pause/unpause, freeze accounts
- AGENT_ROLE: KYC registration
```

### Reentrancy Protection

- All external calls use `nonReentrant` modifier
- State changes before external calls
- CEI pattern (Checks-Effects-Interactions)

### Oracle Manipulation

- NAV staleness checks (<24 hours)
- Multiple oracle sources recommended
- Time-weighted average price (TWAP) option

### Front-Running Mitigation

- T+n settlement for redemptions
- NAV updated before each transaction
- No immediate withdrawals

---

## 🧪 Testing

```bash
# Unit tests
forge test --match-contract FundVaultTest

# Integration tests
forge test --match-contract IntegrationTest

# Gas optimization
forge test --gas-report

# Coverage
forge coverage
```

### Test Checklist

- [ ] Identity registration and verification
- [ ] Compliance checks (holder limits, country limits)
- [ ] NAV calculation and updates
- [ ] Deposit with various amounts
- [ ] Redemption request and processing
- [ ] Management fee calculation
- [ ] Transfer restrictions and lockups
- [ ] Freeze/unfreeze accounts
- [ ] Pause/unpause vault
- [ ] Edge cases (zero amounts, expired verification)

---

## 📊 Gas Estimates

| Operation | Gas Cost |
|-----------|----------|
| Register Identity | ~150,000 |
| First Deposit | ~350,000 |
| Subsequent Deposit | ~250,000 |
| Request Redemption | ~180,000 |
| Process Redemption | ~120,000 |
| Update NAV | ~80,000 |
| Transfer (with compliance) | ~180,000 |
| Freeze Account | ~50,000 |

---

## 🔮 Future Enhancements

1. **Multi-asset support**: Beyond S&P 500 to other indices
2. **On-chain governance**: DAO for parameter changes
3. **Automated rebalancing**: Smart contract portfolio management
4. **Dividend distributions**: Automatic income distribution
5. **Cross-chain bridges**: Deploy on L2s for lower costs
6. **Upgradability**: UUPS proxy pattern
7. **Advanced analytics**: On-chain reporting dashboard
8. **Staking rewards**: Additional yield for long-term holders

## 📚 References

- [ERC-4626: Tokenized Vault Standard](https://eips.ethereum.org/EIPS/eip-4626)
- [ERC-3643: T-REX Token Standard](https://eips.ethereum.org/EIPS/eip-3643)
- [SEC Regulation D](https://www.sec.gov/education/smallbusiness/exemptofferings/rule506b)
- [Investment Company Act of 1940](https://www.sec.gov/investment/laws-and-regulations)

---

## 📞 Support

For questions, issues, or contributions:
- Open an issue on GitHub
- Review the deployment guide
- Check example integrations
- Read compliance documentation

---

## 📄 License

MIT License - See LICENSE file for details

---

**Built with ❤️ for institutional DeFi**
