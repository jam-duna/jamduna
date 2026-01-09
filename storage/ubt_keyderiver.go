package storage

import "github.com/colorfulnotion/jam/common"

// KeyDeriver derives a TreeKey from an address and input key.
type KeyDeriver interface {
	DeriveBinaryTreeKey(address common.Address, inputKey [32]byte) TreeKey
}

// KeyDeriverFunc adapts a function to the KeyDeriver interface.
type KeyDeriverFunc func(address common.Address, inputKey [32]byte) TreeKey

func (f KeyDeriverFunc) DeriveBinaryTreeKey(address common.Address, inputKey [32]byte) TreeKey {
	return f(address, inputKey)
}

func defaultKeyDeriver(profile Profile) KeyDeriver {
	if profile == EIPProfile {
		return KeyDeriverFunc(DeriveBinaryTreeKeyEIP)
	}
	return KeyDeriverFunc(DeriveBinaryTreeKeyJAM)
}
