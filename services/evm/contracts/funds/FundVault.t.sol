// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "../FundVault.sol";
import "../ComplianceModules.sol";

/**
 * @title FundVaultTest
 * @notice Comprehensive test suite for the compliant S&P 500 vault system
 */
contract FundVaultTest is Test {
    // Contracts
    FundVault public vault;
    CompliantShareToken public shareToken;
    IdentityRegistry public identityRegistry;
    ComplianceModule public compliance;
    NAVOracle public navOracle;
    MockUSDC public usdc;
    
    // Test actors
    address public admin = address(1);
    address public fundManager = address(2);
    address public complianceOfficer = address(3);
    address public kycAgent = address(4);
    address public oracleUpdater = address(5);
    
    address public investorA = address(100);
    address public investorB = address(101);
    address public investorC = address(102);
    address public unverifiedInvestor = address(200);
    
    // Constants
    uint256 constant MINIMUM_INVESTMENT = 100_000 * 1e6; // $100k USDC
    uint256 constant INITIAL_NAV = 1e18; // $1.00
    
    function setUp() public {
        vm.startPrank(admin);
        
        // Deploy USDC mock
        usdc = new MockUSDC();
        
        // Deploy compliance infrastructure
        identityRegistry = new IdentityRegistry();
        compliance = new ComplianceModule(address(identityRegistry));
        navOracle = new NAVOracle();
        
        // Deploy vault (automatically deploys share token)
        vault = new FundVault(
            IERC20(address(usdc)),
            "SP500 Institutional Fund",
            "SP500",
            address(identityRegistry),
            address(compliance),
            address(navOracle),
            MINIMUM_INVESTMENT
        );
        
        shareToken = vault.shareToken();
        
        // Grant roles
        identityRegistry.grantRole(identityRegistry.AGENT_ROLE(), kycAgent);
        compliance.grantRole(compliance.COMPLIANCE_ROLE(), complianceOfficer);
        navOracle.grantRole(navOracle.ORACLE_ROLE(), oracleUpdater);
        vault.grantRole(vault.FUND_MANAGER_ROLE(), fundManager);
        
        // Configure compliance
        compliance.setMaxHolders(2000);
        compliance.setCountryLimit(840, 500); // USA limit
        
        // Mint USDC to investors
        usdc.mint(investorA, 1_000_000 * 1e6);
        usdc.mint(investorB, 1_000_000 * 1e6);
        usdc.mint(investorC, 1_000_000 * 1e6);
        
        vm.stopPrank();
    }
    
    // ============ IDENTITY REGISTRY TESTS ============
    
    function test_RegisterIdentity() public {
        vm.startPrank(kycAgent);
        
        identityRegistry.registerIdentity(
            investorA,
            840, // USA
            true, // Accredited
            block.timestamp + 365 days,
            keccak256("investor-A-data")
        );
        
        assertTrue(identityRegistry.isVerified(investorA));
        assertEq(identityRegistry.investorCountry(investorA), 840);
        assertTrue(identityRegistry.isAccredited(investorA));
        
        vm.stopPrank();
    }
    
    function test_CannotRegisterInDisallowedCountry() public {
        vm.startPrank(kycAgent);
        
        vm.expectRevert("Country not allowed");
        identityRegistry.registerIdentity(
            investorA,
            643, // Russia (not allowed)
            true,
            block.timestamp + 365 days,
            keccak256("investor-A-data")
        );
        
        vm.stopPrank();
    }
    
    function test_IdentityExpiry() public {
        vm.startPrank(kycAgent);
        
        uint256 expiryTime = block.timestamp + 100 days;
        identityRegistry.registerIdentity(
            investorA,
            840,
            true,
            expiryTime,
            keccak256("investor-A-data")
        );
        
        assertTrue(identityRegistry.isVerified(investorA));
        
        // Fast forward past expiry
        vm.warp(expiryTime + 1);
        
        assertFalse(identityRegistry.isVerified(investorA));
        
        vm.stopPrank();
    }
    
    // ============ COMPLIANCE MODULE TESTS ============
    
    function test_MaxHoldersLimit() public {
        // Set low holder limit for testing
        vm.prank(complianceOfficer);
        compliance.setMaxHolders(2);
        
        // Register 2 investors
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        registerInvestor(investorB, true);
        vm.stopPrank();
        
        // Mark as holders
        vm.startPrank(complianceOfficer);
        compliance.updateHolderStatus(investorA, true);
        compliance.updateHolderStatus(investorB, true);
        vm.stopPrank();
        
        // Try to add third investor
        vm.startPrank(kycAgent);
        registerInvestor(investorC, true);
        vm.stopPrank();
        
        // Should fail compliance check
        assertFalse(compliance.canTransfer(address(vault), investorC, 100));
    }
    
    function test_CountryInvestorLimit() public {
        vm.startPrank(complianceOfficer);
        compliance.setCountryLimit(840, 1); // USA: max 1 investor
        vm.stopPrank();
        
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        registerInvestor(investorB, true);
        vm.stopPrank();
        
        // First investor should pass
        assertTrue(compliance.canTransfer(address(vault), investorA, 100));
        
        vm.prank(complianceOfficer);
        compliance.updateHolderStatus(investorA, true);
        
        // Second investor should fail
        assertFalse(compliance.canTransfer(address(vault), investorB, 100));
    }
    
    function test_TransferLimits() public {
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        vm.stopPrank();
        
        vm.startPrank(complianceOfficer);
        compliance.setTransferLimit(
            investorA,
            1000 * 1e18, // Daily: 1000
            5000 * 1e18  // Monthly: 5000
        );
        vm.stopPrank();
        
        // First transfer should pass
        assertTrue(compliance.checkTransferLimits(investorA, 500 * 1e18));
        
        // Simulate transfer
        vm.prank(address(shareToken));
        assertTrue(compliance.canTransfer(investorA, investorB, 500 * 1e18));
        
        // Second transfer should pass
        assertTrue(compliance.checkTransferLimits(investorA, 400 * 1e18));
        
        // Third transfer exceeding daily limit should fail
        assertFalse(compliance.checkTransferLimits(investorA, 200 * 1e18));
    }
    
    // ============ NAV ORACLE TESTS ============
    
    function test_UpdateNAV() public {
        uint256 totalAssetValue = 1_000_000 * 1e18;
        uint256 totalShares = 1_000_000 * 1e18;
        
        vm.prank(oracleUpdater);
        navOracle.updateNAV(totalAssetValue, totalShares);
        
        (uint256 nav, uint256 timestamp) = navOracle.getLatestNAV();
        assertEq(nav, 1e18); // $1.00 NAV
        assertEq(timestamp, block.timestamp);
    }
    
    function test_NAVAppreciation() public {
        // S&P 500 goes up 10%
        uint256 totalAssetValue = 1_100_000 * 1e18;
        uint256 totalShares = 1_000_000 * 1e18;
        
        vm.prank(oracleUpdater);
        navOracle.updateNAV(totalAssetValue, totalShares);
        
        (uint256 nav, ) = navOracle.getLatestNAV();
        assertEq(nav, 1.1e18); // $1.10 NAV (10% gain)
    }
    
    function test_HistoricalNAV() public {
        vm.startPrank(oracleUpdater);
        
        uint256 timestamp1 = block.timestamp;
        navOracle.updateNAV(1_000_000 * 1e18, 1_000_000 * 1e18);
        
        vm.warp(block.timestamp + 1 days);
        
        uint256 timestamp2 = block.timestamp;
        navOracle.updateNAV(1_050_000 * 1e18, 1_000_000 * 1e18);
        
        vm.stopPrank();
        
        NAVOracle.NAVData memory day1 = navOracle.getHistoricalNAV(timestamp1);
        NAVOracle.NAVData memory day2 = navOracle.getHistoricalNAV(timestamp2);
        
        assertEq(day1.nav, 1e18);
        assertEq(day2.nav, 1.05e18);
    }
    
    // ============ DEPOSIT TESTS ============
    
    function test_FirstDeposit() public {
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        vm.stopPrank();
        
        uint256 depositAmount = 100_000 * 1e6;
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), depositAmount);
        uint256 shares = vault.deposit(depositAmount, investorA);
        vm.stopPrank();
        
        assertEq(shares, depositAmount); // 1:1 for first deposit
        assertEq(shareToken.balanceOf(investorA), depositAmount);
        assertEq(usdc.balanceOf(address(vault)), depositAmount);
    }
    
    function test_CannotDepositBelowMinimum() public {
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        vm.stopPrank();
        
        uint256 depositAmount = 50_000 * 1e6; // Below $100k minimum
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), depositAmount);
        
        vm.expectRevert("Below minimum investment");
        vault.deposit(depositAmount, investorA);
        
        vm.stopPrank();
    }
    
    function test_CannotDepositIfNotVerified() public {
        uint256 depositAmount = 100_000 * 1e6;
        
        vm.startPrank(unverifiedInvestor);
        usdc.mint(unverifiedInvestor, depositAmount);
        usdc.approve(address(vault), depositAmount);
        
        vm.expectRevert("Receiver not verified");
        vault.deposit(depositAmount, unverifiedInvestor);
        
        vm.stopPrank();
    }
    
    function test_CannotDepositIfNotAccredited() public {
        vm.startPrank(kycAgent);
        registerInvestor(investorA, false); // Not accredited
        vm.stopPrank();
        
        uint256 depositAmount = 100_000 * 1e6;
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), depositAmount);
        
        vm.expectRevert("Must be accredited investor");
        vault.deposit(depositAmount, investorA);
        
        vm.stopPrank();
    }
    
    function test_DepositWithNAVAppreciation() public {
        // First investor deposits
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        registerInvestor(investorB, true);
        vm.stopPrank();
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), 100_000 * 1e6);
        vault.deposit(100_000 * 1e6, investorA);
        vm.stopPrank();
        
        // NAV appreciates to $1.10
        vm.prank(oracleUpdater);
        navOracle.updateNAV(110_000 * 1e6, shareToken.totalSupply());
        
        // Update vault NAV
        vault.updateNAV();
        
        // Second investor deposits same amount
        vm.startPrank(investorB);
        usdc.approve(address(vault), 110_000 * 1e6);
        uint256 shares = vault.deposit(110_000 * 1e6, investorB);
        vm.stopPrank();
        
        // Should receive fewer shares due to higher NAV
        // shares = 110_000 / 1.10 = 100_000
        assertEq(shares, 100_000 * 1e6);
    }
    
    function test_LockupPeriod() public {
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        registerInvestor(investorB, true);
        vm.stopPrank();
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), 100_000 * 1e6);
        vault.deposit(100_000 * 1e6, investorA);
        vm.stopPrank();
        
        // Try to transfer immediately - should fail
        vm.startPrank(investorA);
        vm.expectRevert("Tokens are locked");
        shareToken.transfer(investorB, 1000);
        vm.stopPrank();
        
        // Fast forward 90 days
        vm.warp(block.timestamp + 91 days);
        
        // Should now succeed
        vm.startPrank(investorA);
        shareToken.transfer(investorB, 1000);
        vm.stopPrank();
        
        assertEq(shareToken.balanceOf(investorB), 1000);
    }
    
    // ============ REDEMPTION TESTS ============
    
    function test_RedemptionRequest() public {
        // Setup: Investor deposits
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        vm.stopPrank();
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), 100_000 * 1e6);
        uint256 shares = vault.deposit(100_000 * 1e6, investorA);
        vm.stopPrank();
        
        // Fast forward past lockup
        vm.warp(block.timestamp + 91 days);
        
        // Request redemption
        vm.startPrank(investorA);
        uint256 requestId = vault.requestRedemption(shares / 2);
        vm.stopPrank();
        
        // Check redemption request
        (address investor, uint256 redemptionShares, uint256 requestTime, bool processed) = 
            vault.redemptionQueue(requestId);
        
        assertEq(investor, investorA);
        assertEq(redemptionShares, shares / 2);
        assertFalse(processed);
        
        // Shares should be burned immediately
        assertEq(shareToken.balanceOf(investorA), shares / 2);
    }
    
    function test_ProcessRedemption() public {
        // Setup: Investor deposits
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        vm.stopPrank();
        
        uint256 depositAmount = 100_000 * 1e6;
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), depositAmount);
        uint256 shares = vault.deposit(depositAmount, investorA);
        vm.stopPrank();
        
        // Fast forward past lockup
        vm.warp(block.timestamp + 91 days);
        
        // Request redemption
        vm.prank(investorA);
        uint256 requestId = vault.requestRedemption(shares);
        
        uint256 initialBalance = usdc.balanceOf(investorA);
        
        // Try to process immediately - should fail
        vm.prank(fundManager);
        vm.expectRevert("Settlement period not elapsed");
        vault.processRedemption(requestId);
        
        // Fast forward T+2
        vm.warp(block.timestamp + 3 days);
        
        // Process redemption
        vm.prank(fundManager);
        vault.processRedemption(requestId);
        
        // Check USDC received
        assertEq(usdc.balanceOf(investorA), initialBalance + depositAmount);
    }
    
    function test_RedemptionWithNAVChange() public {
        // Setup: Investor deposits
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        vm.stopPrank();
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), 100_000 * 1e6);
        uint256 shares = vault.deposit(100_000 * 1e6, investorA);
        vm.stopPrank();
        
        // NAV appreciates 20%
        vm.prank(oracleUpdater);
        navOracle.updateNAV(120_000 * 1e6, shares);
        
        // Fast forward past lockup
        vm.warp(block.timestamp + 91 days);
        
        // Request redemption
        vm.prank(investorA);
        uint256 requestId = vault.requestRedemption(shares);
        
        // Fast forward T+2
        vm.warp(block.timestamp + 3 days);
        
        uint256 initialBalance = usdc.balanceOf(investorA);
        
        // Process redemption
        vm.prank(fundManager);
        vault.processRedemption(requestId);
        
        // Should receive $120k (20% gain)
        assertEq(usdc.balanceOf(investorA), initialBalance + 120_000 * 1e6);
    }
    
    function test_BatchRedemptionProcessing() public {
        // Setup: Multiple investors
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        registerInvestor(investorB, true);
        registerInvestor(investorC, true);
        vm.stopPrank();
        
        // All deposit
        address[] memory investors = new address[](3);
        investors[0] = investorA;
        investors[1] = investorB;
        investors[2] = investorC;
        
        uint256[] memory requestIds = new uint256[](3);
        
        for (uint i = 0; i < investors.length; i++) {
            vm.startPrank(investors[i]);
            usdc.approve(address(vault), 100_000 * 1e6);
            vault.deposit(100_000 * 1e6, investors[i]);
            vm.stopPrank();
        }
        
        // Fast forward past lockup
        vm.warp(block.timestamp + 91 days);
        
        // All request redemption
        for (uint i = 0; i < investors.length; i++) {
            vm.prank(investors[i]);
            requestIds[i] = vault.requestRedemption(50_000 * 1e6);
        }
        
        // Fast forward T+2
        vm.warp(block.timestamp + 3 days);
        
        // Batch process
        vm.prank(fundManager);
        vault.batchProcessRedemptions(requestIds);
        
        // Verify all received funds
        for (uint i = 0; i < investors.length; i++) {
            assertTrue(usdc.balanceOf(investors[i]) > 0);
        }
    }
    
    // ============ MANAGEMENT FEE TESTS ============
    
    function test_ManagementFeeCollection() public {
        // Setup: Investor deposits
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        vm.stopPrank();
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), 100_000 * 1e6);
        vault.deposit(100_000 * 1e6, investorA);
        vm.stopPrank();
        
        uint256 initialSupply = shareToken.totalSupply();
        
        // Fast forward 1 year
        vm.warp(block.timestamp + 365 days);
        
        // Collect fees
        vault.collectManagementFee();
        
        uint256 newSupply = shareToken.totalSupply();
        
        // Should be approximately 2% more shares
        uint256 expectedFeeShares = (initialSupply * 2) / 100;
        assertApproxEqRel(newSupply - initialSupply, expectedFeeShares, 0.01e18); // 1% tolerance
    }
    
    // ============ COMPLIANCE ENFORCEMENT TESTS ============
    
    function test_FrozenAccountCannotTransfer() public {
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        registerInvestor(investorB, true);
        vm.stopPrank();
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), 100_000 * 1e6);
        vault.deposit(100_000 * 1e6, investorA);
        vm.stopPrank();
        
        // Freeze account
        vm.prank(admin);
        shareToken.freezeAccount(investorA);
        
        // Fast forward past lockup
        vm.warp(block.timestamp + 91 days);
        
        // Try to transfer
        vm.startPrank(investorA);
        vm.expectRevert("Sender account is frozen");
        shareToken.transfer(investorB, 1000);
        vm.stopPrank();
    }
    
    function test_PausedVaultBlocksDeposits() public {
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        vm.stopPrank();
        
        // Pause vault
        vm.prank(complianceOfficer);
        vault.pause();
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), 100_000 * 1e6);
        
        vm.expectRevert();
        vault.deposit(100_000 * 1e6, investorA);
        
        vm.stopPrank();
    }
    
    function test_UnverifiedRecipientBlocksTransfer() public {
        vm.startPrank(kycAgent);
        registerInvestor(investorA, true);
        vm.stopPrank();
        
        vm.startPrank(investorA);
        usdc.approve(address(vault), 100_000 * 1e6);
        vault.deposit(100_000 * 1e6, investorA);
        vm.stopPrank();
        
        // Fast forward past lockup
        vm.warp(block.timestamp + 91 days);
        
        // Try to transfer to unverified investor
        vm.startPrank(investorA);
        vm.expectRevert("Recipient not verified");
        shareToken.transfer(unverifiedInvestor, 1000);
        vm.stopPrank();
    }
    
    // ============ HELPER FUNCTIONS ============
    
    function registerInvestor(address investor, bool accredited) internal {
        identityRegistry.registerIdentity(
            investor,
            840, // USA
            accredited,
            block.timestamp + 365 days,
            keccak256(abi.encodePacked("investor-", investor))
        );
    }
}

/**
 * @title MockUSDC
 * @notice Mock USDC token for testing
 */
contract MockUSDC is ERC20 {
    constructor() ERC20("USD Coin", "USDC") {}
    
    function decimals() public pure override returns (uint8) {
        return 6;
    }
    
    function mint(address to, uint256 amount) external {
        _mint(to, amount);
    }
}
