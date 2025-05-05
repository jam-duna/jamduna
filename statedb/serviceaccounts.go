package statedb

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// Solves Missing service representation in state.json -- https://github.com/jam-duna/jamtestnet/issues/51

type SAccount struct {
	ID   uint32       `json:"id"`
	Data SServiceData `json:"data"`
}

type SServiceData struct {
	Service    SService          `json:"service"`
	Preimages  []SPreimage       `json:"preimages"`
	LookupMeta []SLookup         `json:"lookup_meta"`
	Storage    map[string]string `json:"storage"`
}

type SService struct {
	CodeHash   string `json:"code_hash"`
	Balance    uint64 `json:"balance"`
	MinItemGas uint64 `json:"min_item_gas"`
	MinMemoGas uint64 `json:"min_memo_gas"`
	Bytes      uint64 `json:"bytes"`
	Items      uint32 `json:"items"`
}

type SPreimage struct {
	Hash string `json:"hash"`
	Blob string `json:"blob"`
}

type SLookup struct {
	Key   SLookupKey `json:"key"`
	Value []uint32   `json:"value"`
}

type SLookupKey struct {
	Hash   string `json:"hash"`
	Length uint32 `json:"length"`
}

// parseMetadata is a helper that converts a metadata string into a map.
func parseMetadata(md string) map[string]string {
	m := make(map[string]string)
	// Split on "|" to separate groups.
	groups := strings.Split(md, "|")
	for _, group := range groups {
		tokens := strings.Fields(strings.TrimSpace(group))
		var currentKey string
		for _, token := range tokens {
			if strings.Contains(token, "=") {
				// If token contains "=", start a new key-value pair.
				parts := strings.SplitN(token, "=", 2)
				currentKey = parts[0]
				m[currentKey] = parts[1]
			} else if currentKey != "" {
				// Append tokens without "=" to the previous key's value.
				m[currentKey] += " " + token
			}
		}
	}
	return m
}

// parse_SLookup parses metadata for "account_lookup" entries.
// For example: "s=1608995021|h=0x... l=210 t=[22]|tlen=1".
// It returns the service id and a constructed SLookup.
func parse_SLookup(md string) (uint32, SLookup) {
	m := parseMetadata(md)
	sStr := m["s"]
	sVal, _ := strconv.ParseUint(sStr, 10, 32)

	lookupKey := SLookupKey{
		Hash: m["h"],
	}
	if lStr, ok := m["l"]; ok {
		lVal, _ := strconv.ParseUint(lStr, 10, 32)
		lookupKey.Length = uint32(lVal)
	}

	var values []uint32
	if t, ok := m["t"]; ok {
		// Remove surrounding brackets and split by comma.
		t = strings.Trim(t, "[]")
		if t != "" {
			parts := strings.Split(t, " ") // fix for https://github.com/jam-duna/jamtestnet/issues/121
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if num, err := strconv.ParseUint(part, 10, 32); err == nil {
					values = append(values, uint32(num))
				}
			}
		}
	}

	lookup := SLookup{
		Key:   lookupKey,
		Value: values,
	}

	return uint32(sVal), lookup
}

func (saa SAccount) Encode() []byte {
	var buffer bytes.Buffer
	// service account ID
	id_bytes := types.E_l(uint64(saa.ID), 4)
	buffer.Write(id_bytes)

	// service account data
	codeHashBytes := common.Hex2Bytes(saa.Data.Service.CodeHash)
	buffer.Write(codeHashBytes)
	binary.Write(&buffer, binary.LittleEndian, saa.Data.Service.Balance)
	binary.Write(&buffer, binary.LittleEndian, saa.Data.Service.MinItemGas)
	binary.Write(&buffer, binary.LittleEndian, saa.Data.Service.MinMemoGas)
	binary.Write(&buffer, binary.LittleEndian, saa.Data.Service.Bytes)
	binary.Write(&buffer, binary.LittleEndian, saa.Data.Service.Items)

	// service account preimages
	preimage_length_bytes := types.E(uint64(len(saa.Data.Preimages)))
	buffer.Write(preimage_length_bytes)

	for _, preimage := range saa.Data.Preimages {
		preimage_hash := common.Hex2Bytes(preimage.Hash)
		preimage_blob := common.Hex2Bytes(preimage.Blob)
		encoded_preimage_blob, _ := types.Encode(preimage_blob)

		buffer.Write(preimage_hash)
		buffer.Write(encoded_preimage_blob)
	}

	// service account lookup metadata
	lookup_length_bytes := types.E(uint64(len(saa.Data.LookupMeta)))
	buffer.Write(lookup_length_bytes)

	for _, lookup := range saa.Data.LookupMeta {
		lookup_key_hash := common.Hex2Bytes(lookup.Key.Hash)
		lookup_key_length_bytes := types.E_l(uint64(lookup.Key.Length), 4)
		encoded_lookup_value, _ := types.Encode(lookup.Value)

		buffer.Write(lookup_key_hash)
		buffer.Write(lookup_key_length_bytes)
		buffer.Write(encoded_lookup_value)
	}

	// service account storage
	storage_length_bytes := types.E(uint64(len(saa.Data.Storage)))
	buffer.Write(storage_length_bytes)

	for key, value := range saa.Data.Storage {
		key_bytes := common.Hex2Bytes(key)
		value_bytes := common.Hex2Bytes(value)
		encoded_key_bytes, _ := types.Encode(key_bytes)
		encoded_value_bytes, _ := types.Encode(value_bytes)

		buffer.Write(encoded_key_bytes)
		buffer.Write(encoded_value_bytes)
	}

	return buffer.Bytes()
}
