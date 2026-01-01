package rpc

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/colorfulnotion/jam/common"
)

type OrchardWallet interface {
	NewAddress() (string, error)
	ListAddresses() []string
	ValidateAddress(address string) bool
	Notes() []WalletNote
	UnspentNotes() []WalletNote
	NotesByAddress(address string) []WalletNote
}

type WalletNote struct {
	Address    string
	Value      uint64
	Commitment common.Hash
	Nullifier  common.Hash
}

type InMemoryWallet struct {
	mu        sync.RWMutex
	rollup    *OrchardRollup
	addresses map[string][32]byte
	pubKeys   map[[32]byte]string
}

func NewInMemoryWallet(rollup *OrchardRollup) *InMemoryWallet {
	return &InMemoryWallet{
		rollup:    rollup,
		addresses: make(map[string][32]byte),
		pubKeys:   make(map[[32]byte]string),
	}
}

func (w *InMemoryWallet) NewAddress() (string, error) {
	var pk [32]byte
	if _, err := rand.Read(pk[:]); err != nil {
		return "", fmt.Errorf("failed to generate address key: %v", err)
	}

	address := addressFromPubKey(pk)
	w.mu.Lock()
	w.addresses[address] = pk
	w.pubKeys[pk] = address
	w.mu.Unlock()

	return address, nil
}

func (w *InMemoryWallet) ListAddresses() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	addresses := make([]string, 0, len(w.addresses))
	for address := range w.addresses {
		addresses = append(addresses, address)
	}
	return addresses
}

func (w *InMemoryWallet) ValidateAddress(address string) bool {
	if !strings.HasPrefix(address, "u1") {
		return false
	}
	if len(address) != 42 {
		return false
	}
	_, err := hex.DecodeString(address[2:])
	return err == nil
}

func (w *InMemoryWallet) Notes() []WalletNote {
	return w.notesForAddresses(nil)
}

func (w *InMemoryWallet) UnspentNotes() []WalletNote {
	return w.notesForAddresses(nil)
}

func (w *InMemoryWallet) NotesByAddress(address string) []WalletNote {
	return w.notesForAddresses(map[string]struct{}{address: {}})
}

func (w *InMemoryWallet) notesForAddresses(filter map[string]struct{}) []WalletNote {
	if w == nil || w.rollup == nil {
		return nil
	}

	notes := w.rollup.ActiveNotes()

	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make([]WalletNote, 0, len(notes))
	for _, note := range notes {
		address, ok := w.pubKeys[note.OwnerPubKey]
		if !ok {
			continue
		}
		if filter != nil {
			if _, allowed := filter[address]; !allowed {
				continue
			}
		}

		result = append(result, WalletNote{
			Address:    address,
			Value:      note.Value,
			Commitment: note.Commitment,
			Nullifier:  note.Nullifier,
		})
	}

	return result
}

func addressFromPubKey(pubKey [32]byte) string {
	return "u1" + hex.EncodeToString(pubKey[:20])
}
