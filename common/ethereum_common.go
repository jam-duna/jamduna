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

// Address is a custom type based on Ethereum's common.Address
type Address ethereumCommon.Address

// Bytes returns the byte representation of the hash.
func (h Hash) Bytes() []byte {
	return ethereumCommon.Hash(h).Bytes()
}

// String returns the string representation of the hash.
func (h Hash) String() string {
	return ethereumCommon.Hash(h).String()
}

func (h Hash) String_short() string {
	return fmt.Sprintf("%s..%s", h.Hex()[2:6], h.Hex()[62:66])
}

func (h Hash) String_shortLen(length int) string {
	return h.Hex()[2 : 2+length]
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
	return "0x" + ethereumCommon.Bytes2Hex(d)
}

func Bytes2String(d []byte) string {
	return ethereumCommon.Bytes2Hex(d)
}

// Hex2Bytes converts a byte slice to a Hash.
func Hex2Bytes(b string) []byte {
	return ethereumCommon.FromHex(b)
}

func Hex2BLS(b string) [144]byte {
	x := ethereumCommon.FromHex(b)
	var result [144]byte
	copy(result[:], x)
	return result
}

func Hex2Metadata(b string) [128]byte {
	x := ethereumCommon.FromHex(b)
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

// Skips "0x" and prints the next 8 characters
func Str(hash Hash) string {
	return fmt.Sprintf("%s..%s", hash.Hex()[2:6], hash.Hex()[len(hash.Hex())-4:len(hash.Hex())])
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

func HexString(h interface{}) string {
	var result string
	v := reflect.ValueOf(h)
	if (v.Kind() == reflect.Slice || v.Kind() == reflect.Array) && (v.Type().Elem().Kind() == reflect.Uint8) {
		result = "0x"
		for i := 0; i < v.Len(); i++ {
			result += fmt.Sprintf("%02x", v.Index(i).Interface())
		}
	} else if v.Kind() == reflect.String {
		result = fmt.Sprintf("%s", v.Interface())
	} else {
		result = "Unsupported type"
	}
	return result
}

// Address methods

// Bytes returns the byte representation of the address.
func (a Address) Bytes() []byte {
	return ethereumCommon.Address(a).Bytes()
}

// String returns the string representation of the address.
func (a Address) String() string {
	return ethereumCommon.Address(a).String()
}

// Hex returns the hexadecimal string representation of the address.
func (a Address) Hex() string {
	return ethereumCommon.Address(a).Hex()
}

// HexToAddress converts a hexadecimal string to an Address.
func HexToAddress(s string) Address {
	return Address(ethereumCommon.HexToAddress(s))
}

// BytesToAddress converts a byte slice to an Address.
func BytesToAddress(b []byte) Address {
	return Address(ethereumCommon.BytesToAddress(b))
}

// MarshalJSON custom marshaler to convert Address to hex string.
func (a Address) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.Hex())
}

// UnmarshalJSON custom unmarshaler to handle hex strings for Address.
func (a *Address) UnmarshalJSON(data []byte) error {
	var hexStr string
	if err := json.Unmarshal(data, &hexStr); err != nil {
		return err
	}
	*a = HexToAddress(hexStr)
	return nil
}

// GetEVMDevAccount returns a standard Hardhat/Anvil test account by index
// These are derived from the mnemonic: "test test test test test test test test test test test junk"
// Returns the address and private key (without 0x prefix) for the account at index % 10
func GetEVMDevAccount(index int) (Address, string) {
	addresses := []Address{
		HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"), // Account #0
		HexToAddress("0x70997970C51812dc3A010C7d01b50e0d17dc79C8"), // Account #1
		HexToAddress("0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC"), // Account #2
		HexToAddress("0x90F79bf6EB2c4f870365E785982E1f101E93b906"), // Account #3
		HexToAddress("0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65"), // Account #4
		HexToAddress("0x9965507D1a55bcC2695C58ba16FB37d819B0A4dc"), // Account #5
		HexToAddress("0x976EA74026E726554dB657fA54763abd0C3a0aa9"), // Account #6
		HexToAddress("0x14dC79964da2C08b23698B3D3cc7Ca32193d9955"), // Account #7
		HexToAddress("0x23618e81E3f5cdF7f54C3d65f7FBc0aBf5B21E8f"), // Account #8
		HexToAddress("0xa0Ee7A142d267C1f36714E4a8F75612F20a79720"), // Account #9
	}

	privateKeys := []string{
		"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", // Account #0
		"59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d", // Account #1
		"5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a", // Account #2
		"7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6", // Account #3
		"47e179ec197488593b187f80a00eb0da91f1b9d0b13f8733639f19c30a34926a", // Account #4
		"8b3a350cf5c34c9194ca85829a2df0ec3153be0318b5e2d3348e872092edffba", // Account #5
		"92db14e403b83dfe3df233f83dfa3a0d7096f21ca9b0d6d6b8d88b2b4ec1564e", // Account #6
		"4bbbf85ce3377467afe5d46f804f221813b2bb87f24d81f60f1fcdbf7cbf4356", // Account #7
		"dbda1821b80551c9d65939329250298aa3472ba22feea921c0cf5d620ea67b97", // Account #8
		"2a871d0798f97d79848a013d4936a73bf4cc922c825d33c1cf7073dff6d409c6", // Account #9
	}
	return addresses[index%10], privateKeys[index%10]
}
