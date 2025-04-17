//go:build network_test
// +build network_test

package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"

	"github.com/colorfulnotion/jam/types"
	"golang.org/x/exp/rand"

	_ "net/http/pprof"
	"time"
)

func transfer(n1 JNode, testServices map[string]*types.TestService, transferNum int) {
	fmt.Printf("\n=========================Start Transfer=========================\n\n")
	service0 := testServices["transfer_0"]
	service1 := testServices["transfer_1"]
	service_authcopy := testServices["auth_copy"]
	auth_copy_item := types.WorkItem{
		Service:            service_authcopy.ServiceCode,
		CodeHash:           service_authcopy.CodeHash,
		Payload:            []byte{},
		RefineGasLimit:     5678,
		AccumulateGasLimit: 9876,
		ImportedSegments:   make([]types.ImportSegment, 0),
		ExportCount:        0,
	}
	core := 0

	sa0, _, _ := n1.GetService(service0.ServiceCode)
	fmt.Printf("\033[38;5;208mtransfer_0\033[0m initial balance: \033[32m%v\033[0m\n", sa0.Balance)
	sa1, _, _ := n1.GetService(service1.ServiceCode)
	fmt.Printf("\033[38;5;208mtransfer_1\033[0m initial balance: \033[32m%v\033[0m\n", sa1.Balance)
	fmt.Printf("\n")
	prevBalance0 := sa0.Balance
	prevBalance1 := sa1.Balance

	TransferNum := transferNum
	Transfer_WorkPackages := make([]types.WorkPackage, 0, TransferNum)
	for n := 1; n <= TransferNum; n++ {
		var workPackage types.WorkPackage
		if n%2 == 0 {
			payload := make([]byte, 8)
			reciver := make([]byte, 4)
			binary.LittleEndian.PutUint32(reciver, uint32(service1.ServiceCode))
			amount := make([]byte, 4)
			binary.LittleEndian.PutUint32(amount, uint32(n*10))
			payload = append(reciver, amount...)

			workPackage = types.WorkPackage{
				Authorization:         []byte("0x"),
				AuthCodeHost:          0,
				AuthorizationCodeHash: bootstrap_auth_codehash,
				ParameterizationBlob:  []byte{},
				WorkItems: []types.WorkItem{
					{
						Service:            service0.ServiceCode,
						CodeHash:           service0.CodeHash,
						Payload:            payload,
						RefineGasLimit:     5678,
						AccumulateGasLimit: 9876,
						ImportedSegments:   make([]types.ImportSegment, 0),
						ExportCount:        1,
					},
					auth_copy_item,
				},
			}
		} else {
			payload := make([]byte, 8)
			reciver := make([]byte, 4)
			binary.LittleEndian.PutUint32(reciver, uint32(service0.ServiceCode))
			amount := make([]byte, 4)
			binary.LittleEndian.PutUint32(amount, uint32(n*10))
			payload = append(reciver, amount...)

			workPackage = types.WorkPackage{
				Authorization:         []byte("0x"),
				AuthCodeHost:          0,
				AuthorizationCodeHash: bootstrap_auth_codehash,
				ParameterizationBlob:  []byte{},
				WorkItems: []types.WorkItem{
					{
						Service:            service1.ServiceCode,
						CodeHash:           service1.CodeHash,
						Payload:            payload,
						RefineGasLimit:     5678,
						AccumulateGasLimit: 9876,
						ImportedSegments:   make([]types.ImportSegment, 0),
						ExportCount:        1,
					},
					auth_copy_item,
				},
			}
		}
		Transfer_WorkPackages = append(Transfer_WorkPackages, workPackage)
		fmt.Printf("** \033[36mTRANSFER WorkPackage #%d\033[0m: %v **\n", n, workPackage.Hash())
	}

	transferChan := make(chan types.WorkPackage, 1)
	transferSuccessful := make(chan string)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	transferCounter := 0
	transferReady := true

	for {
		if transferCounter >= TransferNum && transferReady {
			fmt.Printf("All transfer work packages processed\n")
			break
		}
		time.Sleep(10 * time.Millisecond)
		select {
		case <-ticker.C:
			if transferCounter < TransferNum && transferReady {
				wp := Transfer_WorkPackages[transferCounter]
				transferChan <- wp
				transferCounter++
				transferReady = false

				fmt.Printf("\n** \033[36m TRANSFER=%v \033[0m workPackage: %v **\n", transferCounter, wp.Hash().String_short())
			}
		case wp := <-transferChan:
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			defer cancel()
			n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
				WorkPackage:     wp,
				CoreIndex:       uint16(core),
				ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
			})
		case success := <-transferSuccessful:
			if success != "ok" {
				fmt.Println("Transfer work package failed")
			}

			sa0, _, _ := n1.GetService(service0.ServiceCode)
			sa1, _, _ := n1.GetService(service1.ServiceCode)
			fmt.Printf("\033[38;5;208mtransfer_0_balance\033[0m: \033[32m%v\033[0m -> \033[32m%v\033[0m\n", prevBalance0, sa0.Balance)
			fmt.Printf("\033[38;5;208mtransfer_1_balance\033[0m: \033[32m%v\033[0m -> \033[32m%v\033[0m\n", prevBalance1, sa1.Balance)
			prevBalance0 = sa0.Balance
			prevBalance1 = sa1.Balance

			if transferCounter%2 == 0 {
				fmt.Printf("\n\033[38;5;208mtransfer_0\033[0m sent \033[32m%v\033[0m tokens to \033[38;5;208mtransfer_1\033[0m\n", transferCounter*10)
			} else {
				fmt.Printf("\n\033[38;5;208mtransfer_1\033[0m sent \033[32m%v\033[0m tokens to \033[38;5;208mtransfer_0\033[0m\n", transferCounter*10)
			}

			transferReady = true
		}
	}
}

func scaled_transfer(n1 JNode, testServices map[string]*types.TestService, transferNum int, splitTransferNum int) {
	fmt.Printf("\n=========================Start Scaled Transfer=========================\n\n")
	service0 := testServices["transfer_0"]
	service1 := testServices["transfer_1"]
	service_authcopy := testServices["auth_copy"]
	auth_copy_item := types.WorkItem{
		Service:            service_authcopy.ServiceCode,
		CodeHash:           service_authcopy.CodeHash,
		Payload:            []byte{},
		RefineGasLimit:     5678,
		AccumulateGasLimit: 9876,
		ImportedSegments:   make([]types.ImportSegment, 0),
		ExportCount:        0,
	}

	core := 0

	sa0, _, _ := n1.GetService(service0.ServiceCode)
	fmt.Printf("\033[38;5;208mtransfer_0\033[0m initial balance: \033[32m%v\033[0m\n", sa0.Balance)
	sa1, _, _ := n1.GetService(service1.ServiceCode)
	fmt.Printf("\033[38;5;208mtransfer_1\033[0m initial balance: \033[32m%v\033[0m\n", sa1.Balance)
	fmt.Printf("\n")
	prevBalance0 := sa0.Balance
	prevBalance1 := sa1.Balance

	TransferNum := transferNum           // 10
	SplitTransferNum := splitTransferNum // 600. 10000 for 0.62 MB
	Transfer_WorkPackages := make([]types.WorkPackage, 0, TransferNum)

	for n := 1; n <= TransferNum; n++ {
		var workPackage types.WorkPackage
		var Transfer_WorkItems []types.WorkItem
		if n%2 == 0 {
			for i := 0; i < SplitTransferNum; i++ {
				payload := make([]byte, 8)
				reciver := make([]byte, 4)
				binary.LittleEndian.PutUint32(reciver, uint32(service1.ServiceCode))
				amount := make([]byte, 4)
				binary.LittleEndian.PutUint32(amount, uint32(n))
				payload = append(reciver, amount...)

				Transfer_WorkItems = append(Transfer_WorkItems, types.WorkItem{
					Service:            service0.ServiceCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        1,
				})
				Transfer_WorkItems = append(Transfer_WorkItems, auth_copy_item)
			}
			workPackage = types.WorkPackage{
				Authorization:         []byte("0x"),
				AuthCodeHost:          0,
				AuthorizationCodeHash: bootstrap_auth_codehash,
				ParameterizationBlob:  []byte{},
				WorkItems:             Transfer_WorkItems,
			}
		} else {
			for i := 0; i < SplitTransferNum; i++ {
				payload := make([]byte, 8)
				reciver := make([]byte, 4)
				binary.LittleEndian.PutUint32(reciver, uint32(service0.ServiceCode))
				amount := make([]byte, 4)
				binary.LittleEndian.PutUint32(amount, uint32(n))
				payload = append(reciver, amount...)

				Transfer_WorkItems = append(Transfer_WorkItems, types.WorkItem{
					Service:            service1.ServiceCode,
					CodeHash:           service1.CodeHash,
					Payload:            payload,
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        1,
				})
				Transfer_WorkItems = append(Transfer_WorkItems, auth_copy_item)
			}
			workPackage = types.WorkPackage{
				Authorization:         []byte("0x"),
				AuthCodeHost:          0,
				AuthorizationCodeHash: bootstrap_auth_codehash,
				ParameterizationBlob:  []byte{},
				WorkItems:             Transfer_WorkItems,
			}
		}
		Transfer_WorkPackages = append(Transfer_WorkPackages, workPackage)
		fmt.Printf("** \033[36mTRANSFER WorkPackage #%d\033[0m: %v **\n", n, workPackage.Hash())
		totalWPSizeInMB := calaulateTotalWPSize(workPackage, types.ExtrinsicsBlobs{})
		fmt.Printf("Total Work Package Size: %v MB\n", totalWPSizeInMB)
	}

	transferChan := make(chan types.WorkPackage, 1)
	transferSuccessful := make(chan string)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	transferCounter := 0
	transferReady := true

	for {
		if transferCounter >= TransferNum && transferReady {
			fmt.Printf("All transfer work packages processed\n")
			break
		}
		time.Sleep(10 * time.Millisecond)
		select {
		case <-ticker.C:
			if transferCounter < TransferNum && transferReady {
				wp := Transfer_WorkPackages[transferCounter]
				transferChan <- wp
				transferCounter++
				transferReady = false

				fmt.Printf("\n** \033[36m TRANSFER=%v \033[0m workPackage: %v **\n", transferCounter, wp.Hash().String_short())
			}
		case wp := <-transferChan:
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			defer cancel()
			n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
				CoreIndex:       uint16(core),
				WorkPackage:     wp,
				ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
			})
		case success := <-transferSuccessful:
			if success != "ok" {
				fmt.Println("Transfer work package failed")
			}

			sa0, _, _ := n1.GetService(service0.ServiceCode)
			sa1, _, _ := n1.GetService(service1.ServiceCode)
			fmt.Printf("\033[38;5;208mtransfer_0_balance\033[0m: \033[32m%v\033[0m -> \033[32m%v\033[0m\n", prevBalance0, sa0.Balance)
			fmt.Printf("\033[38;5;208mtransfer_1_balance\033[0m: \033[32m%v\033[0m -> \033[32m%v\033[0m\n", prevBalance1, sa1.Balance)
			prevBalance0 = sa0.Balance
			prevBalance1 = sa1.Balance

			if transferCounter%2 == 0 {
				fmt.Printf("\n\033[38;5;208mtransfer_0\033[0m sent \033[32m%v\033[0m tokens to \033[38;5;208mtransfer_1\033[0m\n", transferCounter*SplitTransferNum)
			} else {
				fmt.Printf("\n\033[38;5;208mtransfer_1\033[0m sent \033[32m%v\033[0m tokens to \033[38;5;208mtransfer_0\033[0m\n", transferCounter*SplitTransferNum)
			}

			transferReady = true
		}
	}
}

// some useful struct, function for balances test case
const AssetSize = 89 // 8 + 32 + 8 + 32 + 8 + 1

type Asset struct {
	AssetID     uint64
	Issuer      [32]byte // 32 bytes
	MinBalance  uint64
	Symbol      [32]byte // 32 bytes
	TotalSupply uint64
	Decimals    uint8
}

func (a *Asset) Bytes() []byte {
	result := make([]byte, AssetSize)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.AssetID)
	buffer.Write(a.Issuer[:])
	binary.Write(buffer, binary.LittleEndian, a.MinBalance)
	buffer.Write(a.Symbol[:])
	binary.Write(buffer, binary.LittleEndian, a.TotalSupply)
	buffer.WriteByte(a.Decimals)

	return buffer.Bytes()
}

func (a *Asset) FromBytes(data []byte) (Asset, error) {
	if len(data) != AssetSize {
		return Asset{}, fmt.Errorf("invalid data size: expected %d, got %d", AssetSize, len(data))
	}
	buffer := bytes.NewReader(data)
	asset := Asset{}
	binary.Read(buffer, binary.LittleEndian, &asset.AssetID)
	buffer.Read(asset.Issuer[:])
	binary.Read(buffer, binary.LittleEndian, &asset.MinBalance)
	buffer.Read(asset.Symbol[:])
	binary.Read(buffer, binary.LittleEndian, &asset.TotalSupply)
	dec, _ := buffer.ReadByte()
	asset.Decimals = dec
	return asset, nil
}

const AccountSize = 24 // 8 (nonce) + 8 (free) + 8 (reserved)
type Account struct {
	Nonce    uint64
	Free     uint64
	Reserved uint64
}

func (a *Account) Bytes() []byte {
	result := make([]byte, AccountSize)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.Nonce)
	binary.Write(buffer, binary.LittleEndian, a.Free)
	binary.Write(buffer, binary.LittleEndian, a.Reserved)
	return buffer.Bytes()
}

func (a *Account) FromBytes(data []byte) (Account, error) {
	if len(data) != AccountSize {
		return Account{}, fmt.Errorf("invalid data size: expected %d, got %d", AccountSize, len(data))
	}
	buffer := bytes.NewReader(data)
	account := Account{}
	binary.Read(buffer, binary.LittleEndian, &account.Nonce)
	binary.Read(buffer, binary.LittleEndian, &account.Free)
	binary.Read(buffer, binary.LittleEndian, &account.Reserved)
	return account, nil
}

type CreateAssetExtrinsic struct {
	method_id uint32
	asset     Asset
}

func (a *CreateAssetExtrinsic) Bytes() []byte {
	result := make([]byte, 4+AssetSize)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	buffer.Write(a.asset.Bytes())
	return buffer.Bytes()
}

type MintExtrinsic struct {
	method_id   uint32
	asset_id    uint64
	account_key [32]byte
	amount      uint64
}

func (a *MintExtrinsic) Bytes() []byte {
	result := make([]byte, 4+8+32+8)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	binary.Write(buffer, binary.LittleEndian, a.asset_id)
	buffer.Write(a.account_key[:])
	binary.Write(buffer, binary.LittleEndian, a.amount)
	return buffer.Bytes()
}

type BurnExtrinsic struct {
	method_id   uint32
	asset_id    uint64
	account_key [32]byte
	amount      uint64
}

func (a *BurnExtrinsic) Bytes() []byte {
	result := make([]byte, 4+8+32+8)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	binary.Write(buffer, binary.LittleEndian, a.asset_id)
	buffer.Write(a.account_key[:])
	binary.Write(buffer, binary.LittleEndian, a.amount)
	return buffer.Bytes()
}

type BondExtrinsic struct {
	method_id   uint32
	asset_id    uint64
	account_key [32]byte
	amount      uint64
}

func (a *BondExtrinsic) Bytes() []byte {
	result := make([]byte, 4+8+32+8)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	binary.Write(buffer, binary.LittleEndian, a.asset_id)
	buffer.Write(a.account_key[:])
	binary.Write(buffer, binary.LittleEndian, a.amount)
	return buffer.Bytes()
}

type UnbondExtrinsic struct {
	method_id   uint32
	asset_id    uint64
	account_key [32]byte
	amount      uint64
}

func (a *UnbondExtrinsic) Bytes() []byte {
	result := make([]byte, 4+8+32+8)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	binary.Write(buffer, binary.LittleEndian, a.asset_id)
	buffer.Write(a.account_key[:])
	binary.Write(buffer, binary.LittleEndian, a.amount)
	return buffer.Bytes()
}

type TransferExtrinsic struct {
	method_id uint32
	asset_id  uint64
	from      [32]byte
	to        [32]byte
	amount    uint64
}

func (a *TransferExtrinsic) Bytes() []byte {
	result := make([]byte, 4+8+32+32+8)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	binary.Write(buffer, binary.LittleEndian, a.asset_id)
	buffer.Write(a.from[:])
	buffer.Write(a.to[:])
	binary.Write(buffer, binary.LittleEndian, a.amount)
	return buffer.Bytes()
}

func balances(n1 JNode, testServices map[string]*types.TestService) {

	// !!!! targetN is NOT USED - need to wire this up
	fmt.Printf("\n=========================Balances Test=========================\n")

	// General setup
	BalanceService := testServices["balances"]
	service_authcopy := testServices["auth_copy"]
	auth_copy_item := types.WorkItem{
		Service:            service_authcopy.ServiceCode,
		CodeHash:           service_authcopy.CodeHash,
		Payload:            []byte{},
		RefineGasLimit:     5678,
		AccumulateGasLimit: 9876,
		ImportedSegments:   make([]types.ImportSegment, 0),
		ExportCount:        0,
	}
	balancesServiceIndex := BalanceService.ServiceCode
	balancesServiceCodeHash := BalanceService.CodeHash

	// Method ID bytes
	create_asset_id := uint32(0)
	mint_id := uint32(1)
	burn_id := uint32(2)
	bond_id := uint32(3)
	unbond_id := uint32(4)
	transfer_id := uint32(5)

	_ = create_asset_id
	_ = mint_id
	_ = burn_id
	_ = bond_id
	_ = unbond_id
	_ = transfer_id

	v0_bytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	v1_bytes := []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	v2_bytes := []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
	v3_bytes := []byte{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3}
	v4_bytes := []byte{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4}
	v5_bytes := []byte{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5}
	_ = v0_bytes
	_ = v1_bytes
	_ = v2_bytes
	_ = v3_bytes
	_ = v4_bytes
	_ = v5_bytes

	asset := Asset{
		AssetID:     1984,
		Issuer:      [32]byte{},
		MinBalance:  100,
		Symbol:      [32]byte{},
		TotalSupply: 100,
		Decimals:    8,
	}
	copy(asset.Issuer[:], v1_bytes)
	copy(asset.Symbol[:], []byte("USDT"))

	// Create Asset test
	fmt.Printf("\n\033[38;5;208mCreating Asset (\033[38;5;46m1984 USDT\033[38;5;208m)...\033[0m\n")

	create_asset_workPackage := types.WorkPackage{}

	// Generate the extrinsic
	extrinsicsBytes := types.ExtrinsicsBlobs{}
	extrinsic := CreateAssetExtrinsic{
		method_id: create_asset_id,
		asset:     asset,
	}
	extrinsicBytes_signed := AddEd25519Sign(extrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash := common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len := uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	create_asset_workPackage = types.WorkPackage{
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     5678,
				AccumulateGasLimit: 9876,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
	defer cancel()
	_, err := n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
		CoreIndex:       0,
		WorkPackage:     create_asset_workPackage,
		ExtrinsicsBlobs: extrinsicsBytes,
	})

	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}

	ShowAssetDetail(n1, balancesServiceIndex, 1984)

	// Mint test
	// mint 1234 for v1, v2, v3

	validators := [][]byte{v1_bytes, v2_bytes, v3_bytes}
	fmt.Printf("\n\033[38;5;208mMinting \033[38;5;46m1234 USDT\033[38;5;208m for \033[38;5;46mV1, V2, V3\033[38;5;208m...\033[0m\n")

	for _, v_key_bytes := range validators {
		var account_key [32]byte
		copy(account_key[:], v_key_bytes)

		// Generate the extrinsic
		extrinsicsBytes := types.ExtrinsicsBlobs{}
		extrinsic := MintExtrinsic{
			method_id:   mint_id,
			asset_id:    1984,
			account_key: account_key,
			amount:      1234,
		}
		extrinsicBytes_signed := AddEd25519Sign(extrinsic.Bytes())
		extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

		extrinsics_hash := common.Blake2Hash(extrinsicBytes_signed)
		extrinsic_len := uint32(len(extrinsicBytes_signed))

		// Put the extrinsic hash and length into the work item extrinsic
		work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
		work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
			Hash: extrinsics_hash,
			Len:  extrinsic_len,
		})

		payload_bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

		mint_workPackage := types.WorkPackage{
			Authorization:         []byte("0x"),
			AuthCodeHost:          0,
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			WorkItems: []types.WorkItem{
				{
					Service:            balancesServiceIndex,
					CodeHash:           balancesServiceCodeHash,
					Payload:            payload_bytes,
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   make([]types.ImportSegment, 0),
					Extrinsics:         work_item_extrinsic,
					ExportCount:        1,
				},
				auth_copy_item,
			},
		}

		_, err := n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
			CoreIndex:       0,
			WorkPackage:     mint_workPackage,
			ExtrinsicsBlobs: extrinsicsBytes,
		})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		time.Sleep(6 * time.Second)
	}

	ShowAssetDetail(n1, balancesServiceIndex, 1984)
	for _, v_key_bytes := range validators {
		ShowAccountDetail(n1, balancesServiceIndex, 1984, [32]byte(v_key_bytes))
	}

	// Burn test
	// burn 100 for v3
	fmt.Printf("\n\033[38;5;208mBurning \033[38;5;46m234 USDT\033[38;5;208m from \033[38;5;46mV3\033[38;5;208m...\033[0m\n")

	var account_key [32]byte
	copy(account_key[:], v3_bytes)

	extrinsicsBytes = types.ExtrinsicsBlobs{}
	burnextrinsic := BurnExtrinsic{
		method_id:   burn_id,
		asset_id:    1984,
		account_key: account_key,
		amount:      234,
	}
	extrinsicBytes_signed = AddEd25519Sign(burnextrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash = common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len = uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic = make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes = make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	mint_workPackage := types.WorkPackage{
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     5678,
				AccumulateGasLimit: 9876,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
		},
	}

	_, err = n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
		CoreIndex:       0,
		WorkPackage:     mint_workPackage,
		ExtrinsicsBlobs: extrinsicsBytes,
	})
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}

	ShowAssetDetail(n1, balancesServiceIndex, 1984)
	ShowAccountDetail(n1, balancesServiceIndex, 1984, account_key)

	// Bond test
	// bond 666 for v3
	fmt.Printf("\n\033[38;5;208mBonding \033[38;5;46m666 USDT\033[38;5;208m from \033[38;5;46mV3\033[38;5;208m...\033[0m\n")

	account_key = [32]byte{}
	copy(account_key[:], v3_bytes)

	extrinsicsBytes = types.ExtrinsicsBlobs{}
	bondextrinsic := BondExtrinsic{
		method_id:   bond_id,
		asset_id:    1984,
		account_key: account_key,
		amount:      666,
	}
	extrinsicBytes_signed = AddEd25519Sign(bondextrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash = common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len = uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic = make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes = make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	bond_workPackage := types.WorkPackage{
		Authorization:         []byte("0x"),
		AuthCodeHost:          balancesServiceIndex,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     5678,
				AccumulateGasLimit: 9876,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
		},
	}

	_, err = n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
		CoreIndex:       0,
		WorkPackage:     bond_workPackage,
		ExtrinsicsBlobs: extrinsicsBytes,
	})
	ShowAccountDetail(n1, balancesServiceIndex, 1984, account_key)

	// Unbond test
	// unbond 333 for v3
	fmt.Printf("\n\033[38;5;208mUnbonding \033[38;5;46m333 USDT\033[38;5;208m from \033[38;5;46mV3\033[38;5;208m...\033[0m\n")

	account_key = [32]byte{}
	copy(account_key[:], v3_bytes)

	extrinsicsBytes = types.ExtrinsicsBlobs{}
	unbondextrinsic := UnbondExtrinsic{
		method_id:   unbond_id,
		asset_id:    1984,
		account_key: account_key,
		amount:      333,
	}
	extrinsicBytes_signed = AddEd25519Sign(unbondextrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash = common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len = uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic = make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes = make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	unbond_workPackage := types.WorkPackage{
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     5678,
				AccumulateGasLimit: 9876,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
		},
	}

	_, err = n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
		CoreIndex:       0,
		WorkPackage:     unbond_workPackage,
		ExtrinsicsBlobs: extrinsicsBytes,
	})
	ShowAccountDetail(n1, balancesServiceIndex, 1984, account_key)

	// Transfer test
	// transfer 100 from v3 to v2
	fmt.Printf("\n\033[38;5;208mTransferring \033[38;5;46m167 USDT\033[38;5;208m from \033[38;5;46mV3\033[38;5;208m to \033[38;5;46mV2\033[38;5;208m...\033[0m\n")

	from_account := [32]byte{}
	copy(from_account[:], v3_bytes)

	to_account := [32]byte{}
	copy(to_account[:], v2_bytes)

	extrinsicsBytes = types.ExtrinsicsBlobs{}
	transferextrinsic := TransferExtrinsic{
		method_id: transfer_id,
		asset_id:  1984,
		from:      from_account,
		to:        to_account,
		amount:    167,
	}

	extrinsicBytes_signed = AddEd25519Sign(transferextrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash = common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len = uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic = make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes = make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	transfer_workPackage := types.WorkPackage{
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     5678,
				AccumulateGasLimit: 9876,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
		},
	}

	_, err = n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
		CoreIndex:       0,
		WorkPackage:     transfer_workPackage,
		ExtrinsicsBlobs: extrinsicsBytes,
	})

	ShowAccountDetail(n1, balancesServiceIndex, 1984, from_account)
	ShowAccountDetail(n1, balancesServiceIndex, 1984, to_account)
}

func scaled_balances(n1 JNode, testServices map[string]*types.TestService, targetN_mint int, targetN_transfer int) {
	fmt.Printf("\n=========================Balances Scale Test=========================\n")
	// General setup
	BalanceService := testServices["balances"]
	balancesServiceIndex := BalanceService.ServiceCode
	balancesServiceCodeHash := BalanceService.CodeHash
	service_authcopy := testServices["auth_copy"]
	auth_copy_item := types.WorkItem{
		Service:            service_authcopy.ServiceCode,
		CodeHash:           service_authcopy.CodeHash,
		Payload:            []byte{},
		RefineGasLimit:     5678,
		AccumulateGasLimit: 9876,
		ImportedSegments:   make([]types.ImportSegment, 0),
		ExportCount:        0,
	}

	// Total Work Package Size in MB
	var totalWPSizeInMB float64

	// Method ID bytes
	create_asset_id := uint32(0)
	mint_id := uint32(1)
	transfer_id := uint32(5)

	_ = create_asset_id
	_ = mint_id
	_ = transfer_id

	issder_bytes := [32]byte{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5}

	asset := Asset{
		AssetID:     1984,
		Issuer:      issder_bytes,
		MinBalance:  100,
		Symbol:      [32]byte{},
		TotalSupply: 0,
		Decimals:    8,
	}
	copy(asset.Symbol[:], []byte("USDT"))

	// Create Asset test
	fmt.Printf("\n\033[38;5;208mCreating Asset (\033[38;5;46m1984 USDT\033[38;5;208m)...\033[0m\n")

	create_asset_workPackage := types.WorkPackage{}

	// Generate the extrinsic
	var extrinsicsBytes types.ExtrinsicsBlobs
	extrinsicsBytes = types.ExtrinsicsBlobs{}
	extrinsic := CreateAssetExtrinsic{
		method_id: create_asset_id,
		asset:     asset,
	}
	extrinsicBytes_signed := AddEd25519Sign(extrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash := common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len := uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	create_asset_workPackage = types.WorkPackage{
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     5678,
				AccumulateGasLimit: 9876,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
		},
	}

	totalWPSizeInMB = calaulateTotalWPSize(create_asset_workPackage, extrinsicsBytes)
	fmt.Printf("\nTotal Work Package Size: %v MB\n\n", totalWPSizeInMB)

	ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
	defer cancel()
	_, err := n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
		CoreIndex:       0,
		WorkPackage:     create_asset_workPackage,
		ExtrinsicsBlobs: extrinsicsBytes,
	})
	ShowAssetDetail(n1, balancesServiceIndex, 1984)

	// Scaled mint test
	num_of_mint := targetN_mint // 110
	fmt.Printf("\n\033[38;5;208mMinting \033[38;5;46m90000 USDT\033[38;5;208m for \033[38;5;46m%v\033[38;5;208m accounts...\033[0m\n", num_of_mint)

	var mint_workPackage types.WorkPackage
	var mint_workItems []types.WorkItem
	extrinsicsBytes = types.ExtrinsicsBlobs{}
	for i := 0; i < num_of_mint; i++ {
		account_key := generateVBytes(uint64(i))

		// Generate the extrinsic
		extrinsic := MintExtrinsic{
			method_id:   mint_id,
			asset_id:    1984,
			account_key: account_key,
			amount:      90000,
		}
		extrinsicBytes_signed := AddEd25519Sign(extrinsic.Bytes())
		extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

		extrinsics_hash := common.Blake2Hash(extrinsicBytes_signed)
		extrinsic_len := uint32(len(extrinsicBytes_signed))

		// Put the extrinsic hash and length into the work item extrinsic
		work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
		work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
			Hash: extrinsics_hash,
			Len:  extrinsic_len,
		})

		payload_bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(payload_bytes, uint64(i)) // extrinsic index

		workItem := types.WorkItem{
			Service:            balancesServiceIndex,
			CodeHash:           balancesServiceCodeHash,
			Payload:            payload_bytes,
			RefineGasLimit:     5678,
			AccumulateGasLimit: 9876,
			ImportedSegments:   make([]types.ImportSegment, 0),
			Extrinsics:         work_item_extrinsic,
			ExportCount:        1,
		}

		mint_workItems = append(mint_workItems, workItem)
		mint_workItems = append(mint_workItems, auth_copy_item)
	}

	mint_workPackage = types.WorkPackage{
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		WorkItems:             mint_workItems,
	}

	totalWPSizeInMB = calaulateTotalWPSize(mint_workPackage, extrinsicsBytes)
	fmt.Printf("\nTotal Work Package Size: %v MB\n\n", totalWPSizeInMB)

	ctx, cancel = context.WithTimeout(context.Background(), RefineTimeout)
	defer cancel()
	_, err = n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
		CoreIndex:       0,
		WorkPackage:     mint_workPackage,
		ExtrinsicsBlobs: extrinsicsBytes,
	})
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}
	ShowAssetDetail(n1, balancesServiceIndex, 1984)

	// Scaled Transfer test
	num_of_transfer := targetN_transfer // 100
	fmt.Printf("\n\033[38;5;208mTransferring \033[38;5;46m1 USDT\033[38;5;208m from \033[38;5;46mV6\033[38;5;208m to \033[38;5;46m%v\033[38;5;208m accounts...\033[0m\n", num_of_transfer)

	var transfer_workPackage types.WorkPackage
	var transfer_workItems []types.WorkItem
	extrinsicsBytes = types.ExtrinsicsBlobs{}

	sender_id := uint64(6)
	sender := generateVBytes(sender_id)
	var receiver [32]byte
	for i := 0; i < num_of_transfer; i++ {
		receiver = generateVBytes(uint64(i))
		if uint64(i) == sender_id {
			receiver = generateVBytes(0)
		}

		// Generate the extrinsic
		extrinsic := TransferExtrinsic{
			method_id: transfer_id,
			asset_id:  1984,
			from:      sender,
			to:        receiver,
			amount:    1,
		}
		extrinsicBytes_signed := AddEd25519Sign(extrinsic.Bytes())
		extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

		extrinsics_hash := common.Blake2Hash(extrinsicBytes_signed)
		extrinsic_len := uint32(len(extrinsicBytes_signed))

		// Put the extrinsic hash and length into the work item extrinsic
		work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
		work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
			Hash: extrinsics_hash,
			Len:  extrinsic_len,
		})

		payload_bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(payload_bytes, uint64(i)) // extrinsic index

		workItem := types.WorkItem{
			Service:            balancesServiceIndex,
			CodeHash:           balancesServiceCodeHash,
			Payload:            payload_bytes,
			RefineGasLimit:     5678,
			AccumulateGasLimit: 9876,
			ImportedSegments:   make([]types.ImportSegment, 0),
			Extrinsics:         work_item_extrinsic,
			ExportCount:        1,
		}

		transfer_workItems = append(transfer_workItems, workItem)
		transfer_workItems = append(transfer_workItems, auth_copy_item)

	}

	transfer_workPackage = types.WorkPackage{
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		WorkItems:             transfer_workItems,
	}

	totalWPSizeInMB = calaulateTotalWPSize(transfer_workPackage, extrinsicsBytes)
	fmt.Printf("\nTotal Work Package Size: %v MB\n\n", totalWPSizeInMB)

	ctx, cancel = context.WithTimeout(context.Background(), RefineTimeout)
	defer cancel()

	_, err = n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
		CoreIndex:       0,
		WorkPackage:     transfer_workPackage,
		ExtrinsicsBlobs: extrinsicsBytes,
	})
	if err != nil {
		fmt.Printf("SubmitAndWaitForWorkPackage ERR %v\n", err)
	}
	ShowAccountDetail(n1, balancesServiceIndex, 1984, sender)
}

func AddEd25519Sign(data []byte) []byte {
	// Generate a new key pair
	seed := make([]byte, 32)
	rand.Read(seed)
	pubKey, privKey, err := types.InitEd25519Key(seed)
	if err != nil {
		return nil
	}

	signature := types.Ed25519Sign(types.Ed25519Priv(privKey), data)
	combinedBytes := append(pubKey.Bytes(), data...)
	data = append(combinedBytes, signature.Bytes()...)
	return data
}

func generateVBytes(n uint64) [32]byte {
	var arr [32]byte
	binary.LittleEndian.PutUint64(arr[:8], n)
	return arr
}

func calaulateTotalWPSize(create_asset_workPackage types.WorkPackage, extrinsicsBytes types.ExtrinsicsBlobs) float64 {
	WPsizeInBytes := len(create_asset_workPackage.Bytes())
	WPsizeInMB := float64(WPsizeInBytes) / 1024 / 1024

	ETsizeInBytes := len(extrinsicsBytes.Bytes())
	ETsizeInMB := float64(ETsizeInBytes) / 1024 / 1024

	totalWPsizeInMB := WPsizeInMB + ETsizeInMB
	return totalWPsizeInMB
}

func ShowAssetDetail(n JNode, balance_service_index uint32, asset_id uint64) {
	var asset Asset
	key_byte := make([]byte, 8)
	binary.LittleEndian.PutUint64(key_byte, asset_id)
	ka := common.ServiceStorageKey(balance_service_index, key_byte)
	service_account_byte, _, _ := n.GetServiceStorage(balance_service_index, ka)
	fetched_asset, _ := asset.FromBytes(service_account_byte)
	fmt.Printf("\n\033[38;5;13mAsset ID\033[0m: \033[32m%v\033[0m\n", fetched_asset.AssetID)
	fmt.Printf("\033[38;5;13mAsset Issuer\033[0m: \033[32m%x\033[0m\n", fetched_asset.Issuer)
	fmt.Printf("\033[38;5;13mAsset MinBalance\033[0m: \033[32m%v\033[0m\n", fetched_asset.MinBalance)
	fmt.Printf("\033[38;5;13mAsset Symbol\033[0m: \033[32m%s\033[0m\n", fetched_asset.Symbol)
	fmt.Printf("\033[38;5;13mAsset TotalSupply\033[0m: \033[32m%v\033[0m\n", fetched_asset.TotalSupply)
	fmt.Printf("\033[38;5;13mAsset Decimals\033[0m: \033[32m%v\033[0m\n", fetched_asset.Decimals)
}

func ShowAccountDetail(n JNode, balance_service_index uint32, asset_id uint64, account_key [32]byte) {
	var account Account
	key_byte := make([]byte, 8+32)
	binary.LittleEndian.PutUint64(key_byte, asset_id)
	copy(key_byte[8:], account_key[:])
	ka := common.ServiceStorageKey(balance_service_index, key_byte)
	service_account_byte, _, _ := n.GetServiceStorage(balance_service_index, ka)

	fetched_account, _ := account.FromBytes(service_account_byte)
	fmt.Printf("\n\033[38;5;13mAccount Key\033[0m: \033[32m%x\033[0m\n", account_key)
	fmt.Printf("\033[38;5;13mAsset ID\033[0m: \033[32m%v\033[0m\n", asset_id)
	fmt.Printf("\033[38;5;13mAccount Nonce\033[0m: \033[32m%v\033[0m\n", fetched_account.Nonce)
	fmt.Printf("\033[38;5;13mAccount Free\033[0m: \033[32m%v\033[0m\n", fetched_account.Free)
	fmt.Printf("\033[38;5;13mAccount Reserved\033[0m: \033[32m%v\033[0m\n", fetched_account.Reserved)
}

func generateJobID() string {
	seed := uint64(time.Now().UnixNano()) // nano seconds. but still not unique
	source := rand.NewSource(seed)
	r := rand.New(source)
	var out [8]byte
	binary.LittleEndian.PutUint64(out[:], r.Uint64())
	jobID := fmt.Sprintf("%x", out)
	return jobID
}

func GenerateNMBFilledBytes(n int) []byte {
	size := n * 1024 * 1024
	return bytes.Repeat([]byte{1}, size)
}

func blake2b(n1 JNode, testServices map[string]*types.TestService) {
	fmt.Printf("Start Blake2b Test\n")

	service0 := testServices["blake2b"]

	payload := make([]byte, 0)
	input := []byte("Hello, Blake2b!")
	input_length := uint32(len(input))
	input_length_bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(input_length_bytes, input_length)
	payload = append(payload, input_length_bytes...)
	payload = append(payload, input...)

	workPackage := types.WorkPackage{
		Authorization:         []byte("0x"), // TODO: set up null-authorizer
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,

		WorkItems: []types.WorkItem{
			{
				Service:            service0.ServiceCode,
				CodeHash:           service0.CodeHash,
				Payload:            payload,
				RefineGasLimit:     5678,
				AccumulateGasLimit: 9876,
				ImportedSegments:   []types.ImportSegment{},
				ExportCount:        1,
			},
		},
	}
	workPackageHash := workPackage.Hash()

	fmt.Printf("\n** \033[36m Blake2b=%v \033[0m workPackage: %v **\n", 1, common.Str(workPackageHash))
	ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
	defer cancel()
	_, err := n1.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
		CoreIndex:       0,
		WorkPackage:     workPackage,
		ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
	})
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}
	fmt.Printf("\nBlake2b Expected Result: %x\n", common.Blake2Hash(input).Bytes())
	k := common.ServiceStorageKey(service0.ServiceCode, []byte{0})
	hash_result, _, _ := n1.GetServiceStorage(service0.ServiceCode, k)
	fmt.Printf("Blake2b Actual Result:   %x\n", hash_result)
}
