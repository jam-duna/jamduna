// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import "@openzeppelin/contracts/token/ERC20/extensions/ERC4626.sol";
import "@openzeppelin/contracts/access/AccessControl.sol";
import "@openzeppelin/contracts/security/ReentrancyGuard.sol";
import "@openzeppelin/contracts/security/Pausable.sol";

/**
 * @title FundVault
 * @notice ERC-4626 vault with ERC-3643/T-REX compliance for institutional S&P 500 product
 * @dev Combines vault accounting (ERC-4626) with transfer restrictions (ERC-3643 pattern)
 * 
 * Key Features:
 * - NAV-based pricing with oracle integration
 * - Transfer restrictions via compliance module
 * - Institutional investor verification (KYC/AML)
 * - Country & accreditation checks
 * - Transfer limits and lock-up periods
 * - Pausable for regulatory requirements
 */

interface IIdentityRegistry {
    function isVerified(address _userAddress) external view returns (bool);
    function investorCountry(address _userAddress) external view returns (uint16);
    function isAccredited(address _userAddress) external view returns (bool);
}

interface ICompliance {
    function canTransfer(
        address _from,
        address _to,
        uint256 _value
    ) external view returns (bool);
}

interface INAVOracle {
    function getLatestNAV() external view returns (uint256 nav, uint256 timestamp);
    function getAssetValue() external view returns (uint256);
}

/**
 * @title CompliantShareToken
 * @notice ERC-20 share token with T-REX compliance restrictions
 */
contract CompliantShareToken is ERC20, AccessControl {
    bytes32 public constant VAULT_ROLE = keccak256("VAULT_ROLE");
    
    IIdentityRegistry public identityRegistry;
    ICompliance public compliance;
    
    mapping(address => uint256) public lockupExpiry;
    mapping(address => bool) public frozenAccounts;
    
    event IdentityRegistrySet(address indexed registry);
    event ComplianceSet(address indexed compliance);
    event AccountFrozen(address indexed account);
    event AccountUnfrozen(address indexed account);
    event TokensLocked(address indexed account, uint256 expiry);
    
    constructor(
        string memory name,
        string memory symbol,
        address _identityRegistry,
        address _compliance
    ) ERC20(name, symbol) {
        _grantRole(DEFAULT_ADMIN_ROLE, msg.sender);
        identityRegistry = IIdentityRegistry(_identityRegistry);
        compliance = ICompliance(_compliance);
    }
    
    function setIdentityRegistry(address _identityRegistry) 
        external 
        onlyRole(DEFAULT_ADMIN_ROLE) 
    {
        identityRegistry = IIdentityRegistry(_identityRegistry);
        emit IdentityRegistrySet(_identityRegistry);
    }
    
    function setCompliance(address _compliance) 
        external 
        onlyRole(DEFAULT_ADMIN_ROLE) 
    {
        compliance = ICompliance(_compliance);
        emit ComplianceSet(_compliance);
    }
    
    function freezeAccount(address account) 
        external 
        onlyRole(DEFAULT_ADMIN_ROLE) 
    {
        frozenAccounts[account] = true;
        emit AccountFrozen(account);
    }
    
    function unfreezeAccount(address account) 
        external 
        onlyRole(DEFAULT_ADMIN_ROLE) 
    {
        frozenAccounts[account] = false;
        emit AccountUnfrozen(account);
    }
    
    function setLockup(address account, uint256 expiry) 
        external 
        onlyRole(VAULT_ROLE) 
    {
        lockupExpiry[account] = expiry;
        emit TokensLocked(account, expiry);
    }
    
    function mint(address to, uint256 amount) 
        external 
        onlyRole(VAULT_ROLE) 
    {
        _mint(to, amount);
    }
    
    function burn(address from, uint256 amount) 
        external 
        onlyRole(VAULT_ROLE) 
    {
        _burn(from, amount);
    }
    
    function _beforeTokenTransfer(
        address from,
        address to,
        uint256 amount
    ) internal virtual override {
        super._beforeTokenTransfer(from, to, amount);
        
        // Skip checks for minting/burning
        if (from == address(0) || to == address(0)) {
            return;
        }
        
        // Compliance checks
        require(!frozenAccounts[from], "Sender account is frozen");
        require(!frozenAccounts[to], "Recipient account is frozen");
        require(block.timestamp >= lockupExpiry[from], "Tokens are locked");
        require(identityRegistry.isVerified(to), "Recipient not verified");
        require(compliance.canTransfer(from, to, amount), "Transfer not compliant");
    }
}

/**
 * @title FundVault
 * @notice Main vault contract combining ERC-4626 with compliance
 */
contract FundVault is ERC4626, AccessControl, ReentrancyGuard, Pausable {
    bytes32 public constant FUND_MANAGER_ROLE = keccak256("FUND_MANAGER_ROLE");
    bytes32 public constant ORACLE_MANAGER_ROLE = keccak256("ORACLE_MANAGER_ROLE");
    bytes32 public constant COMPLIANCE_OFFICER_ROLE = keccak256("COMPLIANCE_OFFICER_ROLE");
    
    CompliantShareToken public immutable shareToken;
    INAVOracle public navOracle;
    
    // Subscription/Redemption parameters
    uint256 public minimumInvestment;
    uint256 public minimumRedemption;
    uint256 public subscriptionLockupPeriod; // e.g., 90 days
    uint256 public managementFeePerSecond; // Annual fee / 365.25 days / 86400 seconds
    uint256 public lastFeeCollection;
    
    // NAV management
    uint256 public lastNAVUpdate;
    uint256 public cachedNAV;
    uint256 public constant NAV_DECIMALS = 18;
    uint256 public constant MAX_NAV_STALENESS = 1 days;
    
    // Redemption queue for T+n settlement
    struct RedemptionRequest {
        address investor;
        uint256 shares;
        uint256 requestTime;
        bool processed;
    }
    
    mapping(uint256 => RedemptionRequest) public redemptionQueue;
    uint256 public redemptionRequestCount;
    uint256 public redemptionSettlementPeriod; // e.g., 2 days for T+2
    
    event NAVUpdated(uint256 newNAV, uint256 timestamp);
    event ManagementFeeCollected(uint256 feeAmount);
    event RedemptionRequested(uint256 indexed requestId, address indexed investor, uint256 shares);
    event RedemptionProcessed(uint256 indexed requestId, address indexed investor, uint256 assets);
    event MinimumInvestmentUpdated(uint256 newMinimum);
    event OracleUpdated(address indexed newOracle);
    
    constructor(
        IERC20 _asset,
        string memory _name,
        string memory _symbol,
        address _identityRegistry,
        address _compliance,
        address _navOracle,
        uint256 _minimumInvestment
    ) ERC4626(_asset) ERC20(_name, _symbol) {
        // Deploy compliant share token
        shareToken = new CompliantShareToken(
            string(abi.encodePacked(_name, " Shares")),
            string(abi.encodePacked(_symbol, "-S")),
            _identityRegistry,
            _compliance
        );
        
        // Grant vault role to this contract
        shareToken.grantRole(shareToken.VAULT_ROLE(), address(this));
        
        // Setup roles
        _grantRole(DEFAULT_ADMIN_ROLE, msg.sender);
        _grantRole(FUND_MANAGER_ROLE, msg.sender);
        _grantRole(ORACLE_MANAGER_ROLE, msg.sender);
        _grantRole(COMPLIANCE_OFFICER_ROLE, msg.sender);
        
        navOracle = INAVOracle(_navOracle);
        minimumInvestment = _minimumInvestment;
        minimumRedemption = _minimumInvestment / 10; // 10% of minimum investment
        subscriptionLockupPeriod = 90 days;
        redemptionSettlementPeriod = 2 days;
        
        // 2% annual management fee
        managementFeePerSecond = (2 * 1e18) / (365.25 days) / 100;
        lastFeeCollection = block.timestamp;
        
        // Initialize NAV
        cachedNAV = 1e18; // Start at 1.0
        lastNAVUpdate = block.timestamp;
    }
    
    /**
     * @notice Update NAV from oracle
     */
    function updateNAV() public {
        (uint256 newNAV, uint256 timestamp) = navOracle.getLatestNAV();
        require(timestamp >= lastNAVUpdate, "NAV is stale");
        
        cachedNAV = newNAV;
        lastNAVUpdate = timestamp;
        
        emit NAVUpdated(newNAV, timestamp);
    }
    
    /**
     * @notice Get current NAV per share
     */
    function getCurrentNAV() public view returns (uint256) {
        require(block.timestamp - lastNAVUpdate <= MAX_NAV_STALENESS, "NAV too stale");
        return cachedNAV;
    }
    
    /**
     * @notice Collect management fees
     */
    function collectManagementFee() public {
        uint256 timeSinceLastCollection = block.timestamp - lastFeeCollection;
        uint256 totalShares = shareToken.totalSupply();
        
        if (totalShares > 0) {
            uint256 feeShares = (totalShares * managementFeePerSecond * timeSinceLastCollection) / 1e18;
            
            if (feeShares > 0) {
                shareToken.mint(address(this), feeShares);
                emit ManagementFeeCollected(feeShares);
            }
        }
        
        lastFeeCollection = block.timestamp;
    }
    
    /**
     * @notice Override totalAssets to use NAV
     */
    function totalAssets() public view virtual override returns (uint256) {
        uint256 totalShares = shareToken.totalSupply();
        if (totalShares == 0) return 0;
        
        return (totalShares * cachedNAV) / 1e18;
    }
    
    /**
     * @notice Convert assets to shares using NAV
     */
    function convertToShares(uint256 assets) 
        public 
        view 
        virtual 
        override 
        returns (uint256) 
    {
        uint256 supply = shareToken.totalSupply();
        if (supply == 0) {
            return assets; // 1:1 for first deposit
        }
        
        return (assets * 1e18) / cachedNAV;
    }
    
    /**
     * @notice Convert shares to assets using NAV
     */
    function convertToAssets(uint256 shares) 
        public 
        view 
        virtual 
        override 
        returns (uint256) 
    {
        return (shares * cachedNAV) / 1e18;
    }
    
    /**
     * @notice Deposit with compliance checks
     */
    function deposit(uint256 assets, address receiver) 
        public 
        virtual 
        override 
        nonReentrant 
        whenNotPaused 
        returns (uint256) 
    {
        require(assets >= minimumInvestment, "Below minimum investment");
        require(shareToken.identityRegistry().isVerified(receiver), "Receiver not verified");
        require(shareToken.identityRegistry().isAccredited(receiver), "Must be accredited investor");
        
        // Update NAV before deposit
        updateNAV();
        collectManagementFee();
        
        uint256 shares = previewDeposit(assets);
        
        // Standard ERC-4626 deposit
        SafeERC20.safeTransferFrom(IERC20(asset()), msg.sender, address(this), assets);
        shareToken.mint(receiver, shares);
        
        // Apply lockup period
        shareToken.setLockup(receiver, block.timestamp + subscriptionLockupPeriod);
        
        emit Deposit(msg.sender, receiver, assets, shares);
        
        return shares;
    }
    
    /**
     * @notice Request redemption (T+n settlement)
     */
    function requestRedemption(uint256 shares) 
        external 
        nonReentrant 
        whenNotPaused 
        returns (uint256 requestId) 
    {
        require(shares >= convertToShares(minimumRedemption), "Below minimum redemption");
        require(shareToken.balanceOf(msg.sender) >= shares, "Insufficient shares");
        
        // Transfer shares to vault for escrow
        shareToken.burn(msg.sender, shares);
        
        requestId = redemptionRequestCount++;
        redemptionQueue[requestId] = RedemptionRequest({
            investor: msg.sender,
            shares: shares,
            requestTime: block.timestamp,
            processed: false
        });
        
        emit RedemptionRequested(requestId, msg.sender, shares);
    }
    
    /**
     * @notice Process redemption after settlement period
     */
    function processRedemption(uint256 requestId) 
        external 
        nonReentrant 
        onlyRole(FUND_MANAGER_ROLE) 
    {
        RedemptionRequest storage request = redemptionQueue[requestId];
        require(!request.processed, "Already processed");
        require(
            block.timestamp >= request.requestTime + redemptionSettlementPeriod,
            "Settlement period not elapsed"
        );
        
        // Update NAV before redemption
        updateNAV();
        collectManagementFee();
        
        uint256 assets = convertToAssets(request.shares);
        
        request.processed = true;
        
        SafeERC20.safeTransfer(IERC20(asset()), request.investor, assets);
        
        emit RedemptionProcessed(requestId, request.investor, assets);
    }
    
    /**
     * @notice Batch process multiple redemptions
     */
    function batchProcessRedemptions(uint256[] calldata requestIds) 
        external 
        onlyRole(FUND_MANAGER_ROLE) 
    {
        updateNAV();
        collectManagementFee();
        
        for (uint256 i = 0; i < requestIds.length; i++) {
            uint256 requestId = requestIds[i];
            RedemptionRequest storage request = redemptionQueue[requestId];
            
            if (!request.processed && 
                block.timestamp >= request.requestTime + redemptionSettlementPeriod) {
                
                uint256 assets = convertToAssets(request.shares);
                request.processed = true;
                
                SafeERC20.safeTransfer(IERC20(asset()), request.investor, assets);
                emit RedemptionProcessed(requestId, request.investor, assets);
            }
        }
    }
    
    /**
     * @notice Override standard redeem to use request-based flow
     */
    function redeem(uint256, address, address) 
        public 
        virtual 
        override 
        returns (uint256) 
    {
        revert("Use requestRedemption instead");
    }
    
    /**
     * @notice Override standard withdraw to use request-based flow
     */
    function withdraw(uint256, address, address) 
        public 
        virtual 
        override 
        returns (uint256) 
    {
        revert("Use requestRedemption instead");
    }
    
    // Admin functions
    
    function setMinimumInvestment(uint256 _minimum) 
        external 
        onlyRole(FUND_MANAGER_ROLE) 
    {
        minimumInvestment = _minimum;
        emit MinimumInvestmentUpdated(_minimum);
    }
    
    function setNAVOracle(address _oracle) 
        external 
        onlyRole(ORACLE_MANAGER_ROLE) 
    {
        navOracle = INAVOracle(_oracle);
        emit OracleUpdated(_oracle);
    }
    
    function setManagementFee(uint256 annualFeePercentage) 
        external 
        onlyRole(FUND_MANAGER_ROLE) 
    {
        collectManagementFee(); // Collect fees at old rate first
        managementFeePerSecond = (annualFeePercentage * 1e18) / (365.25 days) / 100;
    }
    
    function pause() external onlyRole(COMPLIANCE_OFFICER_ROLE) {
        _pause();
    }
    
    function unpause() external onlyRole(COMPLIANCE_OFFICER_ROLE) {
        _unpause();
    }
    
    function emergencyWithdraw(address token, uint256 amount) 
        external 
        onlyRole(DEFAULT_ADMIN_ROLE) 
    {
        SafeERC20.safeTransfer(IERC20(token), msg.sender, amount);
    }
}
