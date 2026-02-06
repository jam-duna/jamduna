


package types

import (
	"encoding/binary"
	"fmt"

	"github.com/jam-duna/jamduna/common"
)


type UBTMultiproof struct {
	Proofs map[common.Hash][]common.Hash
}


type UBTStateDelta struct {
	NumEntries uint32
	Entries    []byte
}


func (d *UBTStateDelta) Serialize() []byte {
	if d == nil {
		return nil
	}
	buffer := make([]byte, 4+len(d.Entries))
	binary.LittleEndian.PutUint32(buffer[0:4], d.NumEntries)
	copy(buffer[4:], d.Entries)
	return buffer
}


func DeserializeUBTStateDelta(data []byte) (*UBTStateDelta, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("delta too short: got %d bytes, need at least 4", len(data))
	}
	numEntries := binary.LittleEndian.Uint32(data[0:4])
	entries := make([]byte, len(data)-4)
	copy(entries, data[4:])
	return &UBTStateDelta{
		NumEntries: numEntries,
		Entries:    entries,
	}, nil
}


type EthereumTransaction struct{}


type ObjectKind byte

const (
	ObjectKindCode         ObjectKind = 0x00
	ObjectKindStorageShard ObjectKind = 0x01
	ObjectKindBalance      ObjectKind = 0x02
	ObjectKindReceipt      ObjectKind = 0x03
	ObjectKindMetaShard    ObjectKind = 0x04
)


type ShardID struct {
	Ld       uint8
	Prefix56 [7]byte
}


func (s *ShardID) ToBytes() []byte {
	result := make([]byte, 8)
	result[0] = s.Ld
	copy(result[1:], s.Prefix56[:])
	return result
}


type ContractShard struct {
	ShardID ShardID
	Entries []EvmEntry
}


func (cs *ContractShard) Serialize() []byte {
	result := make([]byte, 2+len(cs.Entries)*64)
	binary.LittleEndian.PutUint16(result[0:2], uint16(len(cs.Entries)))

	offset := 2
	for _, entry := range cs.Entries {
		copy(result[offset:offset+32], entry.KeyH[:])
		copy(result[offset+32:offset+64], entry.Value[:])
		offset += 64
	}
	return result
}


type EvmEntry struct {
	KeyH  common.Hash
	Value common.Hash
}


type ContractStorage struct {
	Shard ContractShard
}


type TransactionReceipt struct {
	TransactionHash  common.Hash
	Type             uint8
	Version          uint8
	Success          bool
	UsedGas          uint64
	Payload          []byte
	LogsData         []byte
	CumulativeGas    uint64
	LogIndexStart    uint64
	Logs             []Log
	TransactionIndex uint32
	BlockHash        common.Hash
	BlockNumber      uint32
	Timestamp        uint32
}


type Log struct {
	Address  common.Address
	Topics   []common.Hash
	Data     []byte
	LogIndex uint64
}


type EvmBlockPayload struct {

	Number          uint32
	WorkPackageHash common.Hash
	SegmentRoot     common.Hash


	PayloadLength       uint32
	NumTransactions     uint32
	Timestamp           uint32
	GasUsed             uint64
	UBTRoot             common.Hash
	TransactionsRoot    common.Hash
	ReceiptRoot         common.Hash
	BlockAccessListHash common.Hash


	TxHashes      []common.Hash
	ReceiptHashes []common.Hash
	Transactions  []TransactionReceipt
	UBTStateDelta *UBTStateDelta
}


func GetBlockNumberKey() common.Hash {
	var key common.Hash
	for i := range key {
		key[i] = 0xFF
	}
	return key
}


func SerializeBlockNumber(blockNumber uint32) []byte {
	value := make([]byte, 4)
	binary.LittleEndian.PutUint32(value, blockNumber)
	return value
}


func ParseShardPayload(shardPayload []byte) ([]EvmEntry, error) {
	if len(shardPayload) < 2 {
		return nil, fmt.Errorf("shard payload too small: %d bytes", len(shardPayload))
	}

	entryCount := binary.LittleEndian.Uint16(shardPayload[0:2])
	expectedLen := 2 + (int(entryCount) * 64)

	if len(shardPayload) < expectedLen {
		return nil, fmt.Errorf("shard payload incomplete: got %d bytes, expected %d", len(shardPayload), expectedLen)
	}

	entries := make([]EvmEntry, entryCount)
	offset := 2
	for i := 0; i < int(entryCount); i++ {
		copy(entries[i].KeyH[:], shardPayload[offset:offset+32])
		copy(entries[i].Value[:], shardPayload[offset+32:offset+64])
		offset += 64
	}

	return entries, nil
}


func CodeToObjectID(address common.Address) common.Hash {
	var objectID common.Hash
	copy(objectID[:20], address[:])
	objectID[31] = byte(ObjectKindCode)
	return objectID
}


func ShardToObjectID(address common.Address, shardID []byte) common.Hash {
	var objectID common.Hash
	copy(objectID[:20], address[:])
	objectID[20] = 0xFF
	if len(shardID) >= 8 {
		objectID[21] = shardID[0]
		copy(objectID[22:29], shardID[1:8])
	}
	objectID[31] = byte(ObjectKindStorageShard)
	return objectID
}


type PayloadType byte

const (
	PayloadTypeBuilder      PayloadType = 0x00
	PayloadTypeTransactions PayloadType = 0x01
	PayloadTypeGenesis      PayloadType = 0x02
	PayloadTypeCall         PayloadType = 0x03
)


func BuildPayload(payloadType PayloadType, count int, globalDepth uint8, numWitnesses int, blockAccessListHash common.Hash) []byte {
	payload := make([]byte, 40)
	payload[0] = byte(payloadType)
	binary.LittleEndian.PutUint32(payload[1:5], uint32(count))
	payload[5] = globalDepth
	binary.LittleEndian.PutUint16(payload[6:8], uint16(numWitnesses))
	copy(payload[8:40], blockAccessListHash[:])
	return payload
}


func DeserializeContractShard(data []byte) (*ContractShard, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("ContractShard payload too small: %d bytes", len(data))
	}

	cs := &ContractShard{
		Entries: []EvmEntry{},
	}

	entryCount := binary.LittleEndian.Uint16(data[0:2])
	expectedLen := 2 + (int(entryCount) * 64)

	if len(data) < expectedLen {
		return nil, fmt.Errorf("ContractShard incomplete: got %d bytes, expected %d", len(data), expectedLen)
	}

	offset := 2
	for i := 0; i < int(entryCount); i++ {
		var entry EvmEntry
		copy(entry.KeyH[:], data[offset:offset+32])
		copy(entry.Value[:], data[offset+32:offset+64])
		cs.Entries = append(cs.Entries, entry)
		offset += 64
	}

	return cs, nil
}


func ParseBlockReceiptsBlob(data []byte) ([]*TransactionReceipt, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("block receipts blob too short: %d bytes", len(data))
	}

	receiptCount := binary.LittleEndian.Uint32(data[0:4])
	receipts := make([]*TransactionReceipt, receiptCount)


	for i := range receipts {
		receipts[i] = &TransactionReceipt{}
	}

	return receipts, nil
}


func SerializeEvmBlockPayload(payload *EvmBlockPayload) []byte {
	baseSize := 148 + len(payload.TxHashes)*32 + len(payload.ReceiptHashes)*32

	deltaSize := 0
	if payload.UBTStateDelta != nil {
		deltaBytes := payload.UBTStateDelta.Serialize()
		deltaSize = len(deltaBytes)
	}

	result := make([]byte, baseSize+deltaSize)


	binary.LittleEndian.PutUint32(result[0:4], payload.PayloadLength)
	binary.LittleEndian.PutUint32(result[4:8], payload.NumTransactions)
	binary.LittleEndian.PutUint32(result[8:12], payload.Timestamp)
	binary.LittleEndian.PutUint64(result[12:20], payload.GasUsed)
	copy(result[20:52], payload.UBTRoot[:])
	copy(result[52:84], payload.TransactionsRoot[:])
	copy(result[84:116], payload.ReceiptRoot[:])
	copy(result[116:148], payload.BlockAccessListHash[:])

	offset := 148
	for _, hash := range payload.TxHashes {
		copy(result[offset:offset+32], hash[:])
		offset += 32
	}
	for _, hash := range payload.ReceiptHashes {
		copy(result[offset:offset+32], hash[:])
		offset += 32
	}

	if payload.UBTStateDelta != nil {
		deltaBytes := payload.UBTStateDelta.Serialize()
		copy(result[offset:], deltaBytes)
	}

	return result
}


func DeserializeEvmBlockPayload(data []byte, headerOnly bool) (*EvmBlockPayload, error) {
	if len(data) < 148 {
		return nil, fmt.Errorf("block payload too short: got %d bytes, need at least 148", len(data))
	}

	payload := &EvmBlockPayload{
		PayloadLength:       binary.LittleEndian.Uint32(data[0:4]),
		NumTransactions:     binary.LittleEndian.Uint32(data[4:8]),
		Timestamp:           binary.LittleEndian.Uint32(data[8:12]),
		GasUsed:             binary.LittleEndian.Uint64(data[12:20]),
	}

	copy(payload.UBTRoot[:], data[20:52])
	copy(payload.TransactionsRoot[:], data[52:84])
	copy(payload.ReceiptRoot[:], data[84:116])
	copy(payload.BlockAccessListHash[:], data[116:148])

	if headerOnly {
		return payload, nil
	}

	offset := 148
	payload.TxHashes = make([]common.Hash, payload.NumTransactions)
	for i := uint32(0); i < payload.NumTransactions && offset+32 <= len(data); i++ {
		copy(payload.TxHashes[i][:], data[offset:offset+32])
		offset += 32
	}

	payload.ReceiptHashes = make([]common.Hash, payload.NumTransactions)
	for i := uint32(0); i < payload.NumTransactions && offset+32 <= len(data); i++ {
		copy(payload.ReceiptHashes[i][:], data[offset:offset+32])
		offset += 32
	}

	if offset < len(data) {
		delta, err := DeserializeUBTStateDelta(data[offset:])
		if err == nil {
			payload.UBTStateDelta = delta
		}
	}

	return payload, nil
}
