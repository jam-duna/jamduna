package statedb

import (
	"github.com/colorfulnotion/jam/types"
)

type AuthorizeCode struct {
	PackageMetaData   []byte
	AuthorizationCode []byte
}

func (a *AuthorizeCode) Encode() ([]byte, error) {
	metadata := string(a.PackageMetaData)
	encoded_data := types.CombineMetadataAndCode(metadata, a.AuthorizationCode)
	return encoded_data, nil
}

func (a *AuthorizeCode) Decode(data []byte) error {
	metadata, authcode := types.SplitMetadataAndCode(data)
	a.PackageMetaData = []byte(metadata)
	a.AuthorizationCode = authcode
	return nil
}

// func (a *AuthorizeCode) Encode() ([]byte, error) {
// 	//len uint32 bytes data (4 bytes)
// 	lenMetaData := uint32(len(a.PackageMetaData))
// 	//len uint32 bytes data (4 bytes)
// 	lenMetaDataBytes := make([]byte, 4)
// 	binary.LittleEndian.PutUint32(lenMetaDataBytes, lenMetaData)
// 	encoded_data := make([]byte, 0)
// 	encoded_data = append(encoded_data, lenMetaDataBytes...)
// 	encoded_data = append(encoded_data, a.PackageMetaData...)
// 	encoded_data = append(encoded_data, a.AuthorizationCode...)
// 	return encoded_data, nil
// }

// func (a *AuthorizeCode) Decode(data []byte) error {
// 	lenMetaData := binary.LittleEndian.Uint32(data[:4])
// 	a.PackageMetaData = data[4 : 4+lenMetaData]
// 	a.AuthorizationCode = data[4+lenMetaData:]
// 	return nil
// }
