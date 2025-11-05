// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/access/AccessControl.sol";

/**
 * @title IdentityRegistry
 * @notice Manages investor identities and KYC/AML verification
 * @dev Implements T-REX identity registry pattern
 */
contract IdentityRegistry is AccessControl {
    bytes32 public constant AGENT_ROLE = keccak256("AGENT_ROLE");
    
    struct Identity {
        bool verified;
        uint16 country; // ISO 3166-1 numeric country code
        bool accredited; // Accredited/qualified investor status
        uint256 verificationExpiry;
        bytes32 identityHash; // Hash of off-chain identity data
    }
    
    mapping(address => Identity) public identities;
    mapping(uint16 => bool) public allowedCountries;
    
    event IdentityRegistered(address indexed user, uint16 country);
    event IdentityUpdated(address indexed user);
    event IdentityRemoved(address indexed user);
    event CountryAllowed(uint16 country);
    event CountryBlocked(uint16 country);
    
    constructor() {
        _grantRole(DEFAULT_ADMIN_ROLE, msg.sender);
        _grantRole(AGENT_ROLE, msg.sender);
        
        // Allow US by default
        allowedCountries[840] = true; // USA
    }
    
    function registerIdentity(
        address _user,
        uint16 _country,
        bool _accredited,
        uint256 _expiryTimestamp,
        bytes32 _identityHash
    ) external onlyRole(AGENT_ROLE) {
        require(allowedCountries[_country], "Country not allowed");
        
        identities[_user] = Identity({
            verified: true,
            country: _country,
            accredited: _accredited,
            verificationExpiry: _expiryTimestamp,
            identityHash: _identityHash
        });
        
        emit IdentityRegistered(_user, _country);
    }
    
    function updateIdentity(
        address _user,
        uint16 _country,
        bool _accredited,
        uint256 _expiryTimestamp
    ) external onlyRole(AGENT_ROLE) {
        require(identities[_user].verified, "Identity not registered");
        require(allowedCountries[_country], "Country not allowed");
        
        identities[_user].country = _country;
        identities[_user].accredited = _accredited;
        identities[_user].verificationExpiry = _expiryTimestamp;
        
        emit IdentityUpdated(_user);
    }
    
    function removeIdentity(address _user) external onlyRole(AGENT_ROLE) {
        delete identities[_user];
        emit IdentityRemoved(_user);
    }
    
    function allowCountry(uint16 _country) external onlyRole(DEFAULT_ADMIN_ROLE) {
        allowedCountries[_country] = true;
        emit CountryAllowed(_country);
    }
    
    function blockCountry(uint16 _country) external onlyRole(DEFAULT_ADMIN_ROLE) {
        allowedCountries[_country] = false;
        emit CountryBlocked(_country);
    }
    
    // View functions
    
    function isVerified(address _user) external view returns (bool) {
        Identity memory identity = identities[_user];
        return identity.verified && 
               block.timestamp <= identity.verificationExpiry &&
               allowedCountries[identity.country];
    }
    
    function investorCountry(address _user) external view returns (uint16) {
        return identities[_user].country;
    }
    
    function isAccredited(address _user) external view returns (bool) {
        return identities[_user].accredited;
    }
    
    function getIdentity(address _user) external view returns (
        bool verified,
        uint16 country,
        bool accredited,
        uint256 verificationExpiry,
        bytes32 identityHash
    ) {
        Identity memory identity = identities[_user];
        return (
            identity.verified,
            identity.country,
            identity.accredited,
            identity.verificationExpiry,
            identity.identityHash
        );
    }
}

/**
 * @title ComplianceModule
 * @notice Implements transfer compliance rules
 * @dev Checks various restrictions before allowing transfers
 */
contract ComplianceModule is AccessControl {
    bytes32 public constant COMPLIANCE_ROLE = keccak256("COMPLIANCE_ROLE");
    
    IdentityRegistry public identityRegistry;
    
    // Transfer limits
    struct TransferLimits {
        uint256 dailyLimit;
        uint256 monthlyLimit;
        uint256 dailyTransferred;
        uint256 monthlyTransferred;
        uint256 lastDayTimestamp;
        uint256 lastMonthTimestamp;
    }
    
    mapping(address => TransferLimits) public transferLimits;
    
    // Global limits
    uint256 public maxHolders = 2000; // Reg D / Reg S limit
    uint256 public currentHolders;
    mapping(address => bool) public isHolder;
    
    uint256 public maxTokensPerInvestor;
    bool public transfersEnabled = true;
    
    // Country-specific limits
    mapping(uint16 => uint256) public countryInvestorCount;
    mapping(uint16 => uint256) public countryInvestorLimit;
    
    event TransferApproved(address indexed from, address indexed to, uint256 value);
    event TransferBlocked(address indexed from, address indexed to, uint256 value, string reason);
    event TransferLimitSet(address indexed user, uint256 dailyLimit, uint256 monthlyLimit);
    event MaxHoldersUpdated(uint256 newMax);
    
    constructor(address _identityRegistry) {
        _grantRole(DEFAULT_ADMIN_ROLE, msg.sender);
        _grantRole(COMPLIANCE_ROLE, msg.sender);
        identityRegistry = IdentityRegistry(_identityRegistry);
        
        maxTokensPerInvestor = type(uint256).max; // No limit by default
    }
    
    function canTransfer(
        address _from,
        address _to,
        uint256 _value
    ) external returns (bool) {
        // Check if transfers are enabled
        if (!transfersEnabled) {
            emit TransferBlocked(_from, _to, _value, "Transfers disabled");
            return false;
        }
        
        // Check recipient verification
        if (!identityRegistry.isVerified(_to)) {
            emit TransferBlocked(_from, _to, _value, "Recipient not verified");
            return false;
        }
        
        // Check holder limit (for new holders)
        if (!isHolder[_to] && currentHolders >= maxHolders) {
            emit TransferBlocked(_from, _to, _value, "Max holders reached");
            return false;
        }
        
        // Check country limits
        uint16 toCountry = identityRegistry.investorCountry(_to);
        if (countryInvestorLimit[toCountry] > 0 && 
            !isHolder[_to] && 
            countryInvestorCount[toCountry] >= countryInvestorLimit[toCountry]) {
            emit TransferBlocked(_from, _to, _value, "Country limit reached");
            return false;
        }
        
        // Check transfer limits
        if (!checkTransferLimits(_from, _value)) {
            emit TransferBlocked(_from, _to, _value, "Transfer limit exceeded");
            return false;
        }
        
        // Update state
        _updateTransferLimits(_from, _value);
        
        if (!isHolder[_to]) {
            isHolder[_to] = true;
            currentHolders++;
            countryInvestorCount[toCountry]++;
        }
        
        emit TransferApproved(_from, _to, _value);
        return true;
    }
    
    function checkTransferLimits(address _user, uint256 _value) 
        public 
        view 
        returns (bool) 
    {
        TransferLimits memory limits = transferLimits[_user];
        
        // No limits set
        if (limits.dailyLimit == 0 && limits.monthlyLimit == 0) {
            return true;
        }
        
        // Check daily limit
        if (limits.dailyLimit > 0) {
            uint256 dailyTransferred = limits.dailyTransferred;
            if (block.timestamp >= limits.lastDayTimestamp + 1 days) {
                dailyTransferred = 0;
            }
            if (dailyTransferred + _value > limits.dailyLimit) {
                return false;
            }
        }
        
        // Check monthly limit
        if (limits.monthlyLimit > 0) {
            uint256 monthlyTransferred = limits.monthlyTransferred;
            if (block.timestamp >= limits.lastMonthTimestamp + 30 days) {
                monthlyTransferred = 0;
            }
            if (monthlyTransferred + _value > limits.monthlyLimit) {
                return false;
            }
        }
        
        return true;
    }
    
    function _updateTransferLimits(address _user, uint256 _value) internal {
        TransferLimits storage limits = transferLimits[_user];
        
        // Reset daily counter if needed
        if (block.timestamp >= limits.lastDayTimestamp + 1 days) {
            limits.dailyTransferred = 0;
            limits.lastDayTimestamp = block.timestamp;
        }
        
        // Reset monthly counter if needed
        if (block.timestamp >= limits.lastMonthTimestamp + 30 days) {
            limits.monthlyTransferred = 0;
            limits.lastMonthTimestamp = block.timestamp;
        }
        
        limits.dailyTransferred += _value;
        limits.monthlyTransferred += _value;
    }
    
    // Admin functions
    
    function setTransferLimit(
        address _user,
        uint256 _dailyLimit,
        uint256 _monthlyLimit
    ) external onlyRole(COMPLIANCE_ROLE) {
        transferLimits[_user].dailyLimit = _dailyLimit;
        transferLimits[_user].monthlyLimit = _monthlyLimit;
        emit TransferLimitSet(_user, _dailyLimit, _monthlyLimit);
    }
    
    function setMaxHolders(uint256 _maxHolders) 
        external 
        onlyRole(COMPLIANCE_ROLE) 
    {
        maxHolders = _maxHolders;
        emit MaxHoldersUpdated(_maxHolders);
    }
    
    function setCountryLimit(uint16 _country, uint256 _limit) 
        external 
        onlyRole(COMPLIANCE_ROLE) 
    {
        countryInvestorLimit[_country] = _limit;
    }
    
    function setMaxTokensPerInvestor(uint256 _max) 
        external 
        onlyRole(COMPLIANCE_ROLE) 
    {
        maxTokensPerInvestor = _max;
    }
    
    function setTransfersEnabled(bool _enabled) 
        external 
        onlyRole(COMPLIANCE_ROLE) 
    {
        transfersEnabled = _enabled;
    }
    
    function updateHolderStatus(address _holder, bool _isHolder) 
        external 
        onlyRole(COMPLIANCE_ROLE) 
    {
        if (_isHolder && !isHolder[_holder]) {
            isHolder[_holder] = true;
            currentHolders++;
            uint16 country = identityRegistry.investorCountry(_holder);
            countryInvestorCount[country]++;
        } else if (!_isHolder && isHolder[_holder]) {
            isHolder[_holder] = false;
            currentHolders--;
            uint16 country = identityRegistry.investorCountry(_holder);
            countryInvestorCount[country]--;
        }
    }
}

/**
 * @title NAVOracle
 * @notice Provides NAV pricing for the vault
 * @dev In production, would integrate with Chainlink or custom oracle
 */
contract NAVOracle is AccessControl {
    bytes32 public constant ORACLE_ROLE = keccak256("ORACLE_ROLE");
    
    struct NAVData {
        uint256 nav;
        uint256 timestamp;
        uint256 totalAssetValue;
        uint256 totalShares;
    }
    
    NAVData public latestNAV;
    
    mapping(uint256 => NAVData) public historicalNAV; // timestamp => NAV
    uint256[] public navTimestamps;
    
    event NAVUpdated(uint256 nav, uint256 totalAssetValue, uint256 totalShares, uint256 timestamp);
    
    constructor() {
        _grantRole(DEFAULT_ADMIN_ROLE, msg.sender);
        _grantRole(ORACLE_ROLE, msg.sender);
        
        // Initialize with 1.0 NAV
        latestNAV = NAVData({
            nav: 1e18,
            timestamp: block.timestamp,
            totalAssetValue: 0,
            totalShares: 0
        });
    }
    
    /**
     * @notice Update NAV with total asset value and shares
     * @param _totalAssetValue Total value of underlying S&P 500 assets in USD (18 decimals)
     * @param _totalShares Total shares outstanding
     */
    function updateNAV(uint256 _totalAssetValue, uint256 _totalShares) 
        external 
        onlyRole(ORACLE_ROLE) 
    {
        uint256 nav;
        if (_totalShares == 0) {
            nav = 1e18; // 1.0 NAV if no shares
        } else {
            nav = (_totalAssetValue * 1e18) / _totalShares;
        }
        
        latestNAV = NAVData({
            nav: nav,
            timestamp: block.timestamp,
            totalAssetValue: _totalAssetValue,
            totalShares: _totalShares
        });
        
        // Store historical data
        historicalNAV[block.timestamp] = latestNAV;
        navTimestamps.push(block.timestamp);
        
        emit NAVUpdated(nav, _totalAssetValue, _totalShares, block.timestamp);
    }
    
    /**
     * @notice Get latest NAV
     */
    function getLatestNAV() external view returns (uint256 nav, uint256 timestamp) {
        return (latestNAV.nav, latestNAV.timestamp);
    }
    
    /**
     * @notice Get asset value
     */
    function getAssetValue() external view returns (uint256) {
        return latestNAV.totalAssetValue;
    }
    
    /**
     * @notice Get historical NAV
     */
    function getHistoricalNAV(uint256 _timestamp) 
        external 
        view 
        returns (NAVData memory) 
    {
        return historicalNAV[_timestamp];
    }
    
    /**
     * @notice Get NAV change over period
     */
    function getNAVChange(uint256 _startTimestamp, uint256 _endTimestamp) 
        external 
        view 
        returns (int256 changePercent) 
    {
        NAVData memory startNAV = historicalNAV[_startTimestamp];
        NAVData memory endNAV = historicalNAV[_endTimestamp];
        
        require(startNAV.timestamp > 0 && endNAV.timestamp > 0, "NAV data not found");
        
        int256 change = int256(endNAV.nav) - int256(startNAV.nav);
        changePercent = (change * 10000) / int256(startNAV.nav); // Basis points
    }
}
