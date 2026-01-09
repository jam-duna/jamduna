package witness

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"

	evmrpc "github.com/colorfulnotion/jam/builder/evm/rpc"
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/chainspecs"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	jamtypes "github.com/colorfulnotion/jam/types"
)

const testChainID = 0x1107

func TestEVMBlocksTransfersLocal(t *testing.T) {
	jamPath := os.Getenv("JAM_PATH")
	configPath := filepath.Join(jamPath, "chainspecs", "dev-config.json")
	dataPath := "/tmp/builder" // TODO: make this random temp dir
	pvmBackend := statedb.BackendInterpreter

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Failed to read dev config %s: %v\n", configPath, err)
		os.Exit(1)
	}
	var devConfig chainspecs.DevConfig
	if err := json.Unmarshal(configBytes, &devConfig); err != nil {
		fmt.Printf("Failed to parse dev config %s: %v\n", configPath, err)
		os.Exit(1)
	}
	chainSpecData, err := chainspecs.GenSpec(devConfig)
	if err != nil {
		fmt.Printf("Failed to generate chainspec: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Chainspec generated\n")
	validatorIndex := 6
	_, secrets, err := grandpa.GenerateValidatorSecretSet(types.TotalValidators + 1)
	if err != nil {
		t.Fatalf("GenerateValidatorSecretSet failed: %v", err)
	}
	if int(validatorIndex) >= len(secrets) {
		t.Fatalf("validator index %d out of range for %d secrets", validatorIndex, len(secrets))
	}

	port := randomPortAbove(40000)
	t.Setenv("JAM_BIND_ADDR", "127.0.0.1")
	// Create node-specific data path
	nodePath := filepath.Join(dataPath, fmt.Sprintf("jam-%d", validatorIndex))

	n, err := node.NewNode(uint16(validatorIndex), secrets[validatorIndex], chainSpecData, pvmBackend, nil, nil, nodePath, port, types.RoleBuilder)
	if err != nil {
		fmt.Printf("Failed to create node: %v\n", err)
		os.Exit(1)
	}

	storage, err := n.GetStorage()
	if err != nil {
		fmt.Printf("Failed to get storage: %v\n", err)
		os.Exit(1)
	}
	defer storage.Close()
	evmstorage := storage.(types.EVMJAMStorage)
	fmt.Printf("✓ Builder node created\n")
	log.InitLogger("debug")
	serviceID := uint32(0)
	authFile, err := common.GetFilePath(statedb.BootStrapNullAuthFile)
	if err != nil {
		t.Fatalf("GetFilePath(%s) failed: %v", statedb.BootStrapNullAuthFile, err)
	}
	authBytes, err := os.ReadFile(authFile)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", authFile, err)
	}
	authCode := statedb.AuthorizeCode{
		PackageMetaData:   []byte("bootstrap"),
		AuthorizationCode: authBytes,
	}
	authEncoded, err := authCode.Encode()
	if err != nil {
		t.Fatalf("AuthorizeCode.Encode failed: %v", err)
	}
	authHash := common.Blake2Hash(authEncoded)
	n.GetStateDB().WriteServicePreimageBlob(serviceID, authEncoded)
	n.GetStateDB().WriteServicePreimageLookup(serviceID, authHash, uint32(len(authEncoded)), []uint32{0})

	// Create rollup instance
	rollup, err := evmrpc.NewRollup(evmstorage, serviceID, n, pvmBackend)
	if err != nil {
		os.Exit(1)
	}

	txPool := evmrpc.NewTxPool()
	rollup.SetTxPool(txPool)

	startBalance := int64(61_000_000)
	if err := rollup.PrimeGenesis(startBalance); err != nil {
		t.Fatalf("failed to prime genesis: %v", err)
	}

	// Create transactions
	fromAddr, fromPrivKey := common.GetEVMDevAccount(0)
	const transferCount = 51
	receivers := make([]common.Address, 0, transferCount)
	for i := 1; i <= transferCount; i++ {
		addr, _ := common.GetEVMDevAccount(i)
		receivers = append(receivers, addr)
	}
	pendingTxs := make([]*evmtypes.EthereumTransaction, 0, transferCount)
	for i := 0; i < transferCount; i++ {
		tx, _, _, err := evmtypes.CreateSignedNativeTransfer(
			fromPrivKey,
			uint64(i),
			receivers[i],
			big.NewInt(int64(1000*(i+1))),
			big.NewInt(1),
			21_000,
			testChainID,
		)
		if err != nil {
			t.Fatalf("CreateSignedNativeTransfer tx%d failed: %v", i+1, err)
		}
		if tx.From != fromAddr {
			t.Fatalf("unexpected sender recovered for tx%d: got %s, want %s", i+1, tx.From, fromAddr)
		}
		pendingTxs = append(pendingTxs, tx)
	}

	// Create RefineContext (minimal for testing)
	refineCtx := jamtypes.RefineContext{
		Anchor:           common.Hash{},
		StateRoot:        common.Hash{},
		BeefyRoot:        common.Hash{},
		LookupAnchor:     common.Hash{},
		LookupAnchorSlot: 0,
		Prerequisites:    []common.Hash{},
	}

	// Use Rollup.PrepareWorkPackage instead of EVMBuilder.BuildBundle
	workPackage, extrinsicsBlobs, err := rollup.PrepareWorkPackage(refineCtx, pendingTxs)
	if err != nil {
		t.Fatalf("PrepareWorkPackage failed: %v", err)
	}

	// Validate work package structure
	if len(workPackage.WorkItems) != 1 {
		t.Fatalf("expected 1 work item, got %d", len(workPackage.WorkItems))
	}

	workItem := workPackage.WorkItems[0]
	if len(workItem.Extrinsics) != transferCount {
		t.Fatalf("expected %d extrinsics, got %d", transferCount, len(workItem.Extrinsics))
	}

	blobCount := 0
	if len(extrinsicsBlobs) > 0 {
		blobCount = len(extrinsicsBlobs[0])
	}
	if len(extrinsicsBlobs) != 1 || blobCount != transferCount {
		t.Fatalf("expected 1 extrinsics blob with %d entries, got %d/%d", transferCount, len(extrinsicsBlobs), blobCount)
	}

	// Validate extrinsic sizes (RLP-encoded transactions)
	if len(extrinsicsBlobs[0][0]) == 0 || len(extrinsicsBlobs[0][transferCount-1]) == 0 {
		t.Fatalf("extrinsics should not be empty, got %d and %d", len(extrinsicsBlobs[0][0]), len(extrinsicsBlobs[0][transferCount-1]))
	}
	t.Logf("Extrinsic sizes: first=%d last=%d bytes", len(extrinsicsBlobs[0][0]), len(extrinsicsBlobs[0][transferCount-1]))

	payload := workItem.Payload
	if len(payload) != 40 {
		t.Fatalf("expected 40-byte payload, got %d", len(payload))
	}
	// Rollup uses PayloadTypeBuilder (0x00)
	if payload[0] != 0x00 {
		t.Fatalf("expected payload type 0x00 (PayloadTypeBuilder), got 0x%02x", payload[0])
	}
	// Check transaction count (little-endian uint32)
	txCount := uint32(payload[1]) | uint32(payload[2])<<8 | uint32(payload[3])<<16 | uint32(payload[4])<<24
	if txCount != transferCount {
		t.Fatalf("expected tx count %d in payload, got %d", transferCount, txCount)
	}

	// Now execute the bundle with StateDB.BuildBundle
	bundle, workReport, err := n.GetStateDB().BuildBundle(workPackage, extrinsicsBlobs, 0, nil, statedb.BackendInterpreter)

	if err != nil {
		t.Fatalf("StateDB.BuildBundle returned error: %v", err)
	}

	// Validate WorkReport has Results
	if workReport == nil {
		t.Fatalf("expected WorkReport, got nil")
	}

	if len(workReport.Results) != 1 {
		t.Fatalf("expected 1 WorkDigest result (one per work item), got %d", len(workReport.Results))
	}

	result := workReport.Results[0]
	if result.ServiceID != serviceID {
		t.Fatalf("expected WorkDigest.ServiceID=%d, got %d", serviceID, result.ServiceID)
	}

	// Validate bundle structure
	if len(bundle.WorkPackage.WorkItems) != 1 {
		t.Fatalf("expected 1 work item in bundle, got %d", len(bundle.WorkPackage.WorkItems))
	}

	fmt.Printf("✅ WorkReport validated: %d results, serviceID=%d, gas=%d, gasUsed=%d\n",
		len(workReport.Results), result.ServiceID, result.Gas, result.GasUsed)

	guarantorReport, err := n.GetStateDB().ExecuteWorkPackageBundle(
		0,
		*bundle,
		workReport.SegmentRootLookup,
		n.GetStateDB().GetTimeslot(),
		log.OtherGuarantor,
		0,
		statedb.BackendInterpreter,
		"SKIP",
	)
	if err != nil {
		t.Fatalf("ExecuteWorkPackageBundle (guarantor) failed: %v", err)
	}
	if len(guarantorReport.Results) != 1 {
		t.Fatalf("expected 1 guarantor WorkDigest result, got %d", len(guarantorReport.Results))
	}
	guarantorResult := guarantorReport.Results[0]
	fmt.Printf("✅ Guarantor WorkReport validated: %d results, serviceID=%d, gas=%d, gasUsed=%d\n",
		len(guarantorReport.Results), guarantorResult.ServiceID, guarantorResult.Gas, guarantorResult.GasUsed)

	senderBalance, err := rollup.GetBalance(fromAddr, "latest")
	if err != nil {
		t.Fatalf("GetBalance(sender) failed: %v", err)
	}
	senderNonce, err := rollup.GetTransactionCount(fromAddr, "latest")
	if err != nil {
		t.Fatalf("GetTransactionCount(sender) failed: %v", err)
	}
	receiverSum := big.NewInt(0)
	for i, receiver := range receivers {
		receiverBalance, err := rollup.GetBalance(receiver, "latest")
		if err != nil {
			t.Fatalf("GetBalance(receiver %d) failed: %v", i+1, err)
		}
		receiverSum.Add(receiverSum, new(big.Int).SetBytes(receiverBalance.Bytes()))
	}
	coinbaseAddr := common.HexToAddress("0xEaf3223589Ed19bcd171875AC1D0F99D31A5969c")
	coinbaseBalance, err := rollup.GetBalance(coinbaseAddr, "latest")
	if err != nil {
		t.Fatalf("GetBalance(coinbase) failed: %v", err)
	}

	sum := new(big.Int).SetBytes(senderBalance.Bytes())
	sum.Add(sum, receiverSum)
	sum.Add(sum, new(big.Int).SetBytes(coinbaseBalance.Bytes()))
	expectedTotal := new(big.Int).Mul(big.NewInt(startBalance), big.NewInt(1_000_000_000_000_000_000))

	t.Logf("Post-state balances: sender=%s nonce=%d receivers_total=%s coinbase=%s total=%s expected=%s",
		new(big.Int).SetBytes(senderBalance.Bytes()).String(),
		senderNonce,
		receiverSum.String(),
		new(big.Int).SetBytes(coinbaseBalance.Bytes()).String(),
		sum.String(),
		expectedTotal.String(),
	)
	if sum.Cmp(expectedTotal) != 0 {
		t.Fatalf("balance sum mismatch: got %s want %s", sum.String(), expectedTotal.String())
	}
}

func randomPortAbove(base int) int {
	for i := 0; i < 20; i++ {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			continue
		}
		port := conn.LocalAddr().(*net.UDPAddr).Port
		_ = conn.Close()
		if port > base {
			return port
		}
	}
	return base + 1000
}
