package types

import (
	"encoding/json"
	//"errors"
	"fmt"
	//"reflect"

	"github.com/colorfulnotion/jam/common"
)

type BandersnatchKey common.Hash
type BandersnatchVrfSignature [IETFSignatureLen]byte
type BandersnatchRingSignature [ExtrinsicSignatureInBytes]byte

func (b BandersnatchKey) Hash() common.Hash {
	return common.Hash(b)
}

func HexToBandersnatchKey(hexStr string) BandersnatchKey {
	b := common.Hex2Bytes(hexStr)
	var pubkey BandersnatchKey
	copy(pubkey[:], b)
	return pubkey
}

func (k BandersnatchKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(common.Hash(k).Hex())
}

func (k *BandersnatchKey) UnmarshalJSON(data []byte) error {
	var hexStr string
	if err := json.Unmarshal(data, &hexStr); err != nil {
		return err
	}
	*k = BandersnatchKey(common.HexToHash(hexStr))
	return nil
}

func (s BandersnatchVrfSignature) MarshalJSON() ([]byte, error) {
	return json.Marshal(common.Bytes2Hex(s[:]))
}

func (s *BandersnatchVrfSignature) UnmarshalJSON(data []byte) error {
	var hexStr string
	if err := json.Unmarshal(data, &hexStr); err != nil {
		return err
	}
	bytes := common.FromHex(hexStr)
	if len(bytes) != len(s) {
		return fmt.Errorf("invalid length for BandersnatchVrfSignature: expected %d bytes, got %d bytes", len(s), len(bytes))
	}
	copy(s[:], bytes)
	return nil
}
