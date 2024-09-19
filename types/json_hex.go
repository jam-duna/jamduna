package types

import (
	"encoding/json"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

func (h Ed25519Key) MarshalJSON() ([]byte, error) {
	type Alias Ed25519Key
	fmt.Println("Ed25519Key:")
	common.PrintHex(h)
	return json.Marshal(Alias(h))
}

func (h BandersnatchKey) MarshalJSON() ([]byte, error) {
	type Alias BandersnatchKey
	fmt.Println("BandersnatchKey:")
	common.PrintHex(h)
	return json.Marshal(Alias(h))
}

func (h BandersnatchVrfSignature) MarshalJSON() ([]byte, error) {
	type Alias BandersnatchVrfSignature
	fmt.Println("BandersnatchVrfSignature:")
	common.PrintHex(h)
	return json.Marshal(Alias(h))
}
