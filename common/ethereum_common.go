package common

import (
	"fmt"
	"reflect"

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

func Bytes2Hex(d []byte) string {
	return ethereumCommon.Bytes2Hex(d)
}

// Hex2Bytes converts a byte slice to a Hash.
func Hex2Bytes(b string) []byte {
	return ethereumCommon.Hex2Bytes(b)
}

func Hex2BLS(b string) [144]byte {
	x := ethereumCommon.Hex2Bytes(b)
	var result [144]byte
	copy(result[:], x)
	return result
}

func Hex2Metadata(b string) [128]byte {
	x := ethereumCommon.Hex2Bytes(b)
	var result [128]byte
	copy(result[:], x)
	return result
}

func FromHex(b string) []byte {
	return ethereumCommon.FromHex(b)
}

// HexToHash converts a hexadecimal string to a Hash.
func HexToHash(s string) Hash {
	return Hash(ethereumCommon.HexToHash(s))
}

func Hex2Hash(s string) Hash {
	return HexToHash(s)
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

func PrintHex(h interface{}) {
	v := reflect.ValueOf(h)
	if (v.Kind() == reflect.Slice || v.Kind() == reflect.Array) && (v.Type().Elem().Kind() == reflect.Uint8) {
		fmt.Printf("0x")
		for i := 0; i < v.Len(); i++ {
			fmt.Printf("%x", v.Index(i).Interface())
		}
		fmt.Printf("\n")
	} else if v.Kind() == reflect.String {
		fmt.Printf("%s", v.Interface())
		fmt.Printf("\n")
	} else {
		fmt.Println("Unsupported type")
	}
}