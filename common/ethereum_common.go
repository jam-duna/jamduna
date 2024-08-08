package common

import (
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	//"github.com/ethereum/go-ethereum/common/hexutil"
	//"encoding/hex"
	"encoding/json"
)

// Hash is a custom type based on Ethereum's common.Hash
type Hash ethereumCommon.Hash

// Bytes returns the byte representation of the hash.
func (h Hash) Bytes() []byte {
	return ethereumCommon.Hash(h).Bytes()
}

// String returns the string representation of the hash.
func (h Hash) String() string {
	return ethereumCommon.Hash(h).String()
}

// Hex returns the hexadecimal string representation of the hash.
func (h Hash) Hex() string {
	return ethereumCommon.Hash(h).Hex()
}

// BytesToHash converts a byte slice to a Hash.
func BytesToHash(b []byte) Hash {
	return Hash(ethereumCommon.BytesToHash(b))
}

// HexToHash converts a hexadecimal string to a Hash.
func HexToHash(s string) Hash {
	return Hash(ethereumCommon.HexToHash(s))
}

// Bytes2Hex converts a byte slice to a hexadecimal string.
func Bytes2Hex(d []byte) string {
	return ethereumCommon.Bytes2Hex(d)
}

// MarshalJSON custom marshaler to convert Hash to hex string.
func (h Hash) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.Hex())
}

// UnmarshalJSON custom unmarshaler to handle hex strings for Hash.
func (h *Hash) UnmarshalJSON(data []byte) error {
	var hexStr string
	if err := json.Unmarshal(data, &hexStr); err != nil {
		return err
	}
	*h = HexToHash(hexStr)
	return nil
}
