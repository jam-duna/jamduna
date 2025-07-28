package vm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie/utils"
	"github.com/holiman/uint256"
)

type StateDB interface {
	GetNonce(common.Address) uint64
	SetNonce(common.Address, uint64, tracing.BalanceChangeReason)
	GetCodeHash(common.Address) common.Hash
	GetStorageRoot(common.Address) common.Hash
	GetCode(common.Address) []byte
	CreateAccount(common.Address)
	CreateContract(common.Address)
	Exist(common.Address) bool
	AddBalance(common.Address, *uint256.Int, tracing.BalanceChangeReason)
	Snapshot() int
	RevertToSnapshot(int)
	AddAddressToAccessList(common.Address)
	AccessEvents() *state.AccessEvents
	GetBalance(common.Address) *uint256.Int
	GetCodeSize(common.Address) int
	GetState(common.Address, common.Hash) common.Hash
	SetState(common.Address, common.Hash, common.Hash)
	SelfDestruct(common.Address)
	SelfDestruct6780(common.Address)
	PointCache() *utils.PointCache
	AddRefund(uint64)
	SubRefund(uint64)
	HasSelfDestructed(common.Address) bool
	GetStateAndCommittedState(common.Address, common.Hash) (common.Hash, common.Hash)
	Witness() error
	Empty(common.Address) bool
	SubBalance(common.Address, *uint256.Int, tracing.BalanceChangeReason)
	AddLog(*types.Log)
	AddPreimage(common.Hash, []byte)
	SetCode(common.Address, []byte)
	GetRefund() uint64
	AddressInAccessList(common.Address) bool
	SlotInAccessList(common.Address, common.Hash) (common.Hash, bool)
	AddSlotToAccessList(common.Address, common.Hash) bool
	GetTransientState(common.Address, common.Hash) common.Hash
	SetTransientState(common.Address, common.Hash, common.Hash)
}

type MockStateDB struct {
	nonces        map[common.Address]uint64
	balances      map[common.Address]*uint256.Int
	codes         map[common.Address][]byte
	storage       map[common.Address]map[common.Hash]common.Hash
	refund        uint64
	accessList    map[common.Address]map[common.Hash]bool
	selfdestructs map[common.Address]bool
	logs          []*types.Log
}

func NewMockStateDB() *MockStateDB {
	return &MockStateDB{
		nonces:        make(map[common.Address]uint64),
		balances:      make(map[common.Address]*uint256.Int),
		codes:         make(map[common.Address][]byte),
		storage:       make(map[common.Address]map[common.Hash]common.Hash),
		accessList:    make(map[common.Address]map[common.Hash]bool),
		selfdestructs: make(map[common.Address]bool),
	}
}

func (m *MockStateDB) GetNonce(addr common.Address) uint64 {
	return m.nonces[addr]
}

func (m *MockStateDB) SetNonce(addr common.Address, nonce uint64, _ tracing.BalanceChangeReason) {
	m.nonces[addr] = nonce
}

func (m *MockStateDB) GetCodeHash(addr common.Address) common.Hash {
	return common.BytesToHash(m.codes[addr])
}

func (m *MockStateDB) GetStorageRoot(common.Address) common.Hash {
	return common.Hash{} // dummy
}

func (m *MockStateDB) GetCode(addr common.Address) []byte {
	return m.codes[addr]
}

func (m *MockStateDB) CreateAccount(addr common.Address) {
	m.nonces[addr] = 0
	m.balances[addr] = uint256.NewInt(0)
	m.storage[addr] = make(map[common.Hash]common.Hash)
}

func (m *MockStateDB) CreateContract(addr common.Address) {
	m.CreateAccount(addr)
}

func (m *MockStateDB) Exist(addr common.Address) bool {
	_, ok := m.nonces[addr]
	return ok
}

func (m *MockStateDB) AddBalance(addr common.Address, amount *uint256.Int, _ tracing.BalanceChangeReason) {
	if m.balances[addr] == nil {
		m.balances[addr] = uint256.NewInt(0)
	}
	m.balances[addr].Add(m.balances[addr], amount)
}

func (m *MockStateDB) Snapshot() int          { return 0 }
func (m *MockStateDB) RevertToSnapshot(_ int) {}
func (m *MockStateDB) AddAddressToAccessList(a common.Address) {
	if _, ok := m.accessList[a]; !ok {
		m.accessList[a] = make(map[common.Hash]bool)
	}
}

func (m *MockStateDB) AccessEvents() *state.AccessEvents {
	return &state.AccessEvents{}
}

func (m *MockStateDB) GetBalance(addr common.Address) *uint256.Int {
	return m.balances[addr]
}

func (m *MockStateDB) GetCodeSize(addr common.Address) int {
	return len(m.codes[addr])
}

func (m *MockStateDB) GetState(addr common.Address, key common.Hash) common.Hash {
	return m.storage[addr][key]
}

func (m *MockStateDB) SetState(addr common.Address, key, value common.Hash) {
	if m.storage[addr] == nil {
		m.storage[addr] = make(map[common.Hash]common.Hash)
	}
	m.storage[addr][key] = value
}

func (m *MockStateDB) SelfDestruct(addr common.Address) {
	m.selfdestructs[addr] = true
}

func (m *MockStateDB) SelfDestruct6780(addr common.Address) {
	m.SelfDestruct(addr)
}

func (m *MockStateDB) PointCache() *utils.PointCache {
	return nil
}

func (m *MockStateDB) AddRefund(v uint64) {
	m.refund += v
}

func (m *MockStateDB) SubRefund(v uint64) {
	if m.refund > v {
		m.refund -= v
	} else {
		m.refund = 0
	}
}

func (m *MockStateDB) HasSelfDestructed(addr common.Address) bool {
	return m.selfdestructs[addr]
}

func (m *MockStateDB) GetStateAndCommittedState(addr common.Address, key common.Hash) (common.Hash, common.Hash) {
	return m.storage[addr][key], m.storage[addr][key]
}

func (m *MockStateDB) Witness() error {
	return nil
}

func (m *MockStateDB) Empty(addr common.Address) bool {
	return len(m.codes[addr]) == 0 && m.GetBalance(addr).IsZero()
}

func (m *MockStateDB) SubBalance(addr common.Address, amount *uint256.Int, _ tracing.BalanceChangeReason) {
	if m.balances[addr] == nil {
		m.balances[addr] = uint256.NewInt(0)
	}
	m.balances[addr].Sub(m.balances[addr], amount)
}

func (m *MockStateDB) AddLog(log *types.Log) {
	m.logs = append(m.logs, log)
}

func (m *MockStateDB) AddPreimage(common.Hash, []byte) {}

func (m *MockStateDB) SetCode(addr common.Address, code []byte) {
	m.codes[addr] = code
}

func (m *MockStateDB) GetRefund() uint64 {
	return m.refund
}

func (m *MockStateDB) AddressInAccessList(addr common.Address) bool {
	_, ok := m.accessList[addr]
	return ok
}

func (m *MockStateDB) SlotInAccessList(addr common.Address, slot common.Hash) (common.Hash, bool) {
	if slots, ok := m.accessList[addr]; ok {
		if _, exists := slots[slot]; exists {
			return slot, true
		}
	}
	return common.Hash{}, false
}

func (m *MockStateDB) AddSlotToAccessList(addr common.Address, slot common.Hash) bool {
	if m.accessList[addr] == nil {
		m.accessList[addr] = make(map[common.Hash]bool)
	}
	_, existed := m.accessList[addr][slot]
	m.accessList[addr][slot] = true
	return !existed
}

func (m *MockStateDB) GetTransientState(addr common.Address, key common.Hash) common.Hash {
	return common.Hash{} // not implemented
}

func (m *MockStateDB) SetTransientState(addr common.Address, key, value common.Hash) {
	// not implemented
}
