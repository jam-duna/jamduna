package evmtypes

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// EthereumTransaction represents a parsed Ethereum transaction
type EthereumTransaction struct {
	Hash     common.Hash     `json:"hash"`
	From     common.Address  `json:"from"`
	To       *common.Address `json:"to"`
	Value    *big.Int        `json:"value"`
	Gas      uint64          `json:"gas"`
	GasPrice *big.Int        `json:"gasPrice"`
	Nonce    uint64          `json:"nonce"`
	Data     []byte          `json:"data"`
	V        *big.Int        `json:"v"`
	R        *big.Int        `json:"r"`
	S        *big.Int        `json:"s"`

	// JAM-specific fields
	ReceivedAt time.Time `json:"receivedAt"`
	Size       uint64    `json:"size"`

	// Store the original go-ethereum transaction for proper type handling
	Inner *ethereumTypes.Transaction `json:"-"`
}

// EthereumTransactionResponse represents a complete Ethereum transaction for JSON-RPC responses
type EthereumTransactionResponse struct {
	Hash             string  `json:"hash"`
	Nonce            string  `json:"nonce"`
	BlockHash        string  `json:"blockHash"`
	BlockNumber      string  `json:"blockNumber"`
	TransactionIndex string  `json:"transactionIndex"`
	From             string  `json:"from"`
	To               *string `json:"to"`
	Value            string  `json:"value"`
	GasPrice         string  `json:"gasPrice"`
	Gas              string  `json:"gas"`
	Input            string  `json:"input"`
	V                string  `json:"v"`
	R                string  `json:"r"`
	S                string  `json:"s"`
}

// ConvertPayloadToEthereumTransaction converts the transaction payload to Ethereum format
func ConvertPayloadToEthereumTransaction(receipt *TransactionReceipt) (*EthereumTransactionResponse, error) {
	// The payload contains the RLP-encoded Ethereum transaction (from main.rs line 499: extrinsic.clone())
	// This can be a legacy tx, EIP-2930, or EIP-1559 transaction

	payload := receipt.Payload
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	// Decode the RLP-encoded Ethereum transaction using go-ethereum
	var tx ethereumTypes.Transaction
	err := tx.UnmarshalBinary(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode RLP transaction: %v", err)
	}

	// Extract transaction fields
	var to *string
	if tx.To() != nil {
		toAddr := tx.To().Hex()
		to = &toAddr
	}

	// Recover sender address from signature
	signer := ethereumTypes.LatestSignerForChainID(tx.ChainId())
	from, err := signer.Sender(&tx)
	if err != nil {
		return nil, fmt.Errorf("failed to recover sender: %v", err)
	}

	input := "0x" + hex.EncodeToString(tx.Data())

	// Extract v, r, s from signature
	v, r, s := tx.RawSignatureValues()

	return &EthereumTransactionResponse{
		Hash:             receipt.TransactionHash.String(),
		Nonce:            fmt.Sprintf("0x%x", tx.Nonce()),
		BlockHash:        receipt.BlockHash.String(),
		BlockNumber:      fmt.Sprintf("0x%x", receipt.BlockNumber),
		TransactionIndex: fmt.Sprintf("%d", receipt.TransactionIndex),
		From:             from.Hex(),
		To:               to,
		Value:            fmt.Sprintf("0x%x", tx.Value()),
		GasPrice:         fmt.Sprintf("0x%x", tx.GasPrice()),
		Gas:              fmt.Sprintf("0x%x", tx.Gas()),
		Input:            input,
		V:                fmt.Sprintf("0x%x", v),
		R:                fmt.Sprintf("0x%x", r),
		S:                fmt.Sprintf("0x%x", s),
	}, nil
}

// ParseTransactionObject parses a JSON transaction object into an EthereumTransaction
func ParseTransactionObject(txObj map[string]interface{}) (*EthereumTransaction, error) {
	tx := &EthereumTransaction{}

	// Parse 'to' field
	if toStr, ok := txObj["to"].(string); ok && toStr != "" {
		to := common.HexToAddress(toStr)
		tx.To = &to
	}

	// Parse 'from' field
	if fromStr, ok := txObj["from"].(string); ok && fromStr != "" {
		tx.From = common.HexToAddress(fromStr)
	}

	// Parse 'data' field
	if dataStr, ok := txObj["data"].(string); ok && dataStr != "" {
		if len(dataStr) >= 2 && dataStr[:2] == "0x" {
			tx.Data = common.FromHex(dataStr)
		}
	}

	// Parse 'value' field
	if valueStr, ok := txObj["value"].(string); ok && valueStr != "" {
		if len(valueStr) >= 2 && valueStr[:2] == "0x" {
			if value, success := big.NewInt(0).SetString(valueStr[2:], 16); success {
				tx.Value = value
			}
		}
	}
	if tx.Value == nil {
		tx.Value = big.NewInt(0)
	}

	// Parse 'gas' field
	if gasStr, ok := txObj["gas"].(string); ok && gasStr != "" {
		if len(gasStr) >= 2 && gasStr[:2] == "0x" {
			if gas, err := strconv.ParseUint(gasStr[2:], 16, 64); err == nil {
				tx.Gas = gas
			}
		}
	}
	if tx.Gas == 0 {
		tx.Gas = 21000 // Default gas limit
	}

	// Parse 'gasPrice' field
	if gasPriceStr, ok := txObj["gasPrice"].(string); ok && gasPriceStr != "" {
		if len(gasPriceStr) >= 2 && gasPriceStr[:2] == "0x" {
			if gasPrice, success := big.NewInt(0).SetString(gasPriceStr[2:], 16); success {
				tx.GasPrice = gasPrice
			}
		}
	}
	if tx.GasPrice == nil {
		tx.GasPrice = big.NewInt(1) // 1 wei default
	}

	// Parse 'nonce' field
	if nonceStr, ok := txObj["nonce"].(string); ok && nonceStr != "" {
		if len(nonceStr) >= 2 && nonceStr[:2] == "0x" {
			if nonce, err := strconv.ParseUint(nonceStr[2:], 16, 64); err == nil {
				tx.Nonce = nonce
			}
		}
	}
	// Note: If nonce is 0, caller should check if it was explicitly provided or needs to be fetched

	return tx, nil
}

// ParseRawTransactionBytes parses a raw signed transaction from bytes
func ParseRawTransactionBytes(rawTxBytes []byte) (*EthereumTransaction, error) {
	// Decode transaction (handles both legacy RLP and typed transactions)
	var ethTx ethereumTypes.Transaction
	if err := ethTx.UnmarshalBinary(rawTxBytes); err != nil {
		return nil, fmt.Errorf("failed to decode transaction: %v", err)
	}

	// Extract signature values
	v, r, s := ethTx.RawSignatureValues()

	// Convert 'to' address
	var to *common.Address
	if ethTx.To() != nil {
		addr := common.BytesToAddress(ethTx.To().Bytes())
		to = &addr
	}

	// Create our transaction structure
	tx := &EthereumTransaction{
		Hash:       common.BytesToHash(ethTx.Hash().Bytes()),
		From:       common.Address{}, // Will be recovered from signature
		To:         to,
		Value:      ethTx.Value(),
		Gas:        ethTx.Gas(),
		GasPrice:   ethTx.GasPrice(),
		Nonce:      ethTx.Nonce(),
		Data:       ethTx.Data(),
		V:          v,
		R:          r,
		S:          s,
		ReceivedAt: time.Now(),
		Size:       uint64(len(rawTxBytes)),
		Inner:      &ethTx, // Store original transaction for type-aware operations
	}
	return tx, nil
}

// ParseRawTransaction parses a raw signed transaction from bytes
func ParseRawTransaction(rawTxBytes []byte) (*EthereumTransaction, error) {
	return ParseRawTransactionBytes(rawTxBytes)
}

// RecoverSender recovers the sender address from transaction signature
func (tx *EthereumTransaction) RecoverSender() (common.Address, error) {
	// Validate signature values
	if tx.V == nil || tx.R == nil || tx.S == nil {
		return common.Address{}, fmt.Errorf("missing signature components")
	}

	// If we have the inner transaction, use it directly with LatestSignerForChainID
	// which automatically selects the correct signer for typed transactions
	if tx.Inner != nil {
		// Use LatestSignerForChainID to support all transaction types (legacy, EIP-2930, EIP-1559)
		chainID := tx.Inner.ChainId()
		if chainID == nil {
			// For unprotected transactions, try Homestead signer
			from, err := ethereumTypes.Sender(ethereumTypes.HomesteadSigner{}, tx.Inner)
			if err != nil {
				return common.Address{}, fmt.Errorf("failed to recover sender: %v", err)
			}
			recoveredAddr := common.BytesToAddress(from.Bytes())
			log.Debug(log.Node, "RecoverSender (Homestead)", "hash", tx.Hash.String(), "from", recoveredAddr.String())
			return recoveredAddr, nil
		}

		signer := ethereumTypes.LatestSignerForChainID(chainID)
		from, err := ethereumTypes.Sender(signer, tx.Inner)
		if err != nil {
			return common.Address{}, fmt.Errorf("failed to recover sender: %v", err)
		}
		recoveredAddr := common.BytesToAddress(from.Bytes())
		log.Debug(log.Node, "RecoverSender", "hash", tx.Hash.String(), "from", recoveredAddr.String(), "type", tx.Inner.Type())
		return recoveredAddr, nil
	}

	// Fallback for legacy code path: reconstruct transaction
	// Create signer based on V value (EIP-155 or Homestead)
	var signer ethereumTypes.Signer
	if tx.V.Sign() != 0 && isProtectedV(tx.V) {
		// EIP-155 transaction with chain ID
		chainID := deriveChainId(tx.V)
		signer = ethereumTypes.NewEIP155Signer(chainID)
	} else {
		// Pre-EIP155 homestead transaction
		signer = ethereumTypes.HomesteadSigner{}
	}

	// Reconstruct the transaction for signature recovery
	var to *common.Address
	if tx.To != nil {
		ethAddr := (ethereumCommon.Address)(*tx.To)
		to = (*common.Address)(&ethAddr)
	}

	// Create go-ethereum transaction
	ethTx := ethereumTypes.NewTx(&ethereumTypes.LegacyTx{
		Nonce:    tx.Nonce,
		GasPrice: tx.GasPrice,
		Gas:      tx.Gas,
		To:       (*ethereumCommon.Address)(to),
		Value:    tx.Value,
		Data:     tx.Data,
		V:        tx.V,
		R:        tx.R,
		S:        tx.S,
	})

	// Recover sender address using the signer
	from, err := ethereumTypes.Sender(signer, ethTx)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to recover sender: %v", err)
	}

	recoveredAddr := common.BytesToAddress(from.Bytes())
	log.Debug(log.Node, "RecoverSender (legacy fallback)", "hash", tx.Hash.String(), "from", recoveredAddr.String())

	return recoveredAddr, nil
}

// isProtectedV checks if V value indicates an EIP-155 transaction
func isProtectedV(v *big.Int) bool {
	if v.BitLen() <= 8 {
		vInt := v.Uint64()
		return vInt != 27 && vInt != 28
	}
	// Anything larger than 8 bits must be protected (chain ID encoding)
	return true
}

// deriveChainId derives the chain ID from V value for EIP-155 transactions
func deriveChainId(v *big.Int) *big.Int {
	if v.BitLen() <= 64 {
		vInt := v.Uint64()
		if vInt == 27 || vInt == 28 {
			return new(big.Int)
		}
		return new(big.Int).SetUint64((vInt - 35) / 2)
	}
	// V = CHAIN_ID * 2 + 35 + {0, 1}
	// CHAIN_ID = (V - 35) / 2
	chainID := new(big.Int).Sub(v, big.NewInt(35))
	chainID.Div(chainID, big.NewInt(2))
	return chainID
}

// VerifySignature verifies the transaction signature against a public key
func (tx *EthereumTransaction) VerifySignature(pubkey *ecdsa.PublicKey) bool {
	// Recover the sender address from the signature
	sender, err := tx.RecoverSender()
	if err != nil {
		log.Warn(log.Node, "VerifySignature: failed to recover sender", "error", err)
		return false
	}

	// Derive address from public key using crypto.PubkeyToAddress
	expectedAddr := crypto.PubkeyToAddress(*pubkey)

	// Compare recovered sender with expected address
	isValid := sender == common.BytesToAddress(expectedAddr.Bytes())

	log.Debug(log.Node, "VerifySignature", "hash", tx.Hash.String(), "valid", isValid, "sender", sender.String())

	return isValid
}

// ToJSON returns a JSON representation of the transaction
func (tx *EthereumTransaction) ToJSON() string {
	// Create a JSON-compatible representation with proper hex encoding
	jsonTx := struct {
		Hash     string  `json:"hash"`
		From     string  `json:"from"`
		To       *string `json:"to"`
		Value    string  `json:"value"`
		Gas      string  `json:"gas"`
		GasPrice string  `json:"gasPrice"`
		Nonce    string  `json:"nonce"`
		Data     string  `json:"data"`
		V        string  `json:"v,omitempty"`
		R        string  `json:"r,omitempty"`
		S        string  `json:"s,omitempty"`
	}{
		Hash:     tx.Hash.String(),
		From:     tx.From.String(),
		Value:    "0x" + tx.Value.Text(16),
		Gas:      fmt.Sprintf("0x%x", tx.Gas),
		GasPrice: "0x" + tx.GasPrice.Text(16),
		Nonce:    fmt.Sprintf("0x%x", tx.Nonce),
		Data:     "0x" + hex.EncodeToString(tx.Data),
	}

	// Set 'to' field (null for contract creation)
	if tx.To != nil {
		toStr := tx.To.String()
		jsonTx.To = &toStr
	}

	// Include signature values if present
	if tx.V != nil && tx.V.Sign() != 0 {
		jsonTx.V = "0x" + tx.V.Text(16)
	}
	if tx.R != nil && tx.R.Sign() != 0 {
		jsonTx.R = "0x" + tx.R.Text(16)
	}
	if tx.S != nil && tx.S.Sign() != 0 {
		jsonTx.S = "0x" + tx.S.Text(16)
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(jsonTx)
	if err != nil {
		log.Warn(log.Node, "ToJSON: failed to marshal transaction", "error", err)
		return "{}"
	}

	return string(jsonBytes)
}

// CreateSignedUSDMTransfer creates a signed transaction that transfers USDM tokens
// Returns the parsed EthereumTransaction, RLP-encoded bytes, transaction hash, and error
func CreateSignedUSDMTransfer(usdmAddress common.Address, privateKeyHex string, nonce uint64, to common.Address, amount *big.Int, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*EthereumTransaction, []byte, common.Hash, error) {
	if chainID == 0 {
		panic(222)
	}
	// Parse private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	// USDM transfer function signature: transfer(address,uint256)
	// Function selector: 0xa9059cbb
	calldata := make([]byte, 68)
	copy(calldata[0:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) // transfer(address,uint256) selector

	// Encode recipient address (32 bytes, left-padded with zeros)
	// Address is 20 bytes, so it goes in bytes 16-35 of the 32-byte word (bytes 4-35 of calldata)
	toBytes := to.Bytes()
	copy(calldata[16:36], toBytes)

	// Debug: verify calldata encoding
	fmt.Printf("DEBUG CreateSignedUSDMTransfer: to=%s, to.Bytes()=%x (len=%d)\n", to.Hex(), toBytes, len(toBytes))
	fmt.Printf("DEBUG calldata[4:36]=%x\n", calldata[4:36])

	// Encode amount (32 bytes)
	amountBytes := amount.FillBytes(make([]byte, 32))
	copy(calldata[36:68], amountBytes)

	// Create transaction to USDM contract
	ethTx := ethereumTypes.NewTransaction(
		nonce,
		ethereumCommon.Address(usdmAddress),
		big.NewInt(0), // value = 0 for token transfer
		gasLimit,
		gasPrice,
		calldata,
	)

	// Sign transaction
	signer := ethereumTypes.NewEIP155Signer(big.NewInt(int64(chainID)))
	signedTx, err := ethereumTypes.SignTx(ethTx, signer, privateKey)
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	// Encode to RLP
	rlpBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	// Calculate transaction hash (Ethereum uses Keccak256)
	txHash := common.Keccak256(rlpBytes)

	// Parse into EthereumTransaction
	tx, err := ParseRawTransactionBytes(rlpBytes)
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	// Recover sender from signature
	sender, err := tx.RecoverSender()
	if err != nil {
		return nil, nil, common.Hash{}, err
	}
	tx.From = sender

	return tx, rlpBytes, txHash, nil
}

// GetFunctionSelector computes the 4-byte function selector and event topic hash map
// Example: GetFunctionSelector("fibonacci(uint256)", ["FibCache(uint256)", "FibComputed(uint256)"])
// returns ([4]byte{0x61, 0x04, 0x7f, 0xf4}, map[hash]"FibCache(uint256)", map[hash]"FibComputed(uint256)")
func GetFunctionSelector(funcSig string, eventSigs []string) ([4]byte, map[common.Hash]string) {
	// Compute function selector
	funcHash := crypto.Keccak256([]byte(funcSig))
	var selector [4]byte
	copy(selector[:], funcHash[:4])

	// Compute event topic hashes and map to signatures
	topics := make(map[common.Hash]string, len(eventSigs))
	for _, eventSig := range eventSigs {
		eventHash := crypto.Keccak256([]byte(eventSig))
		topics[common.BytesToHash(eventHash[:32])] = eventSig
	}

	return selector, topics
}
