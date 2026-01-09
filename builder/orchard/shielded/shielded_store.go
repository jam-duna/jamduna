package shielded

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
)

// CommitmentEntry captures a stored commitment and its position.
type CommitmentEntry struct {
	Position   uint64
	Commitment [32]byte
}

// ShieldedStore persists shielded commitments and nullifiers.
type ShieldedStore struct {
	db *leveldb.DB
}

// NewShieldedStore opens or creates the shielded store.
func NewShieldedStore(path string) (*ShieldedStore, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, fmt.Errorf("open shielded store: %w", err)
	}
	return &ShieldedStore{db: db}, nil
}

// AddCommitment stores a commitment at the given position.
func (s *ShieldedStore) AddCommitment(position uint64, commitment [32]byte) error {
	key := commitmentKey(position)
	return s.db.Put(key, commitment[:], nil)
}

// DeleteCommitment removes a commitment entry (used for rollback).
func (s *ShieldedStore) DeleteCommitment(position uint64) error {
	return s.db.Delete(commitmentKey(position), nil)
}

// AddNullifier records a nullifier as spent.
func (s *ShieldedStore) AddNullifier(nullifier [32]byte, height uint32) error {
	key := nullifierKey(nullifier)
	var value [4]byte
	binary.LittleEndian.PutUint32(value[:], height)
	return s.db.Put(key, value[:], nil)
}

// DeleteNullifier removes a nullifier entry (used for rollback).
func (s *ShieldedStore) DeleteNullifier(nullifier [32]byte) error {
	return s.db.Delete(nullifierKey(nullifier), nil)
}

// IsNullifierSpent checks whether a nullifier has been recorded.
func (s *ShieldedStore) IsNullifierSpent(nullifier [32]byte) (bool, error) {
	_, err := s.db.Get(nullifierKey(nullifier), nil)
	if err == leveldb.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListCommitments returns all stored commitments with positions.
func (s *ShieldedStore) ListCommitments() ([]CommitmentEntry, error) {
	iter := s.db.NewIterator(nil, nil)
	defer iter.Release()

	entries := make([]CommitmentEntry, 0)
	for iter.Next() {
		key := string(iter.Key())
		if !strings.HasPrefix(key, "cm_") {
			continue
		}
		pos, err := parseCommitmentPosition(key)
		if err != nil {
			continue
		}
		var cm [32]byte
		copy(cm[:], iter.Value())
		entries = append(entries, CommitmentEntry{
			Position:   pos,
			Commitment: cm,
		})
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Position < entries[j].Position
	})

	return entries, nil
}

// ListNullifiers returns all recorded nullifiers.
func (s *ShieldedStore) ListNullifiers() ([][32]byte, error) {
	iter := s.db.NewIterator(nil, nil)
	defer iter.Release()

	var out [][32]byte
	for iter.Next() {
		key := string(iter.Key())
		if !strings.HasPrefix(key, "nf_") {
			continue
		}
		nullifier, err := parseNullifierKey(key)
		if err != nil {
			continue
		}
		out = append(out, nullifier)
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return out, nil
}

// Close closes the underlying database.
func (s *ShieldedStore) Close() error {
	return s.db.Close()
}

func commitmentKey(position uint64) []byte {
	return []byte(fmt.Sprintf("cm_%020d", position))
}

func nullifierKey(nullifier [32]byte) []byte {
	return []byte(fmt.Sprintf("nf_%s", hex.EncodeToString(nullifier[:])))
}

func parseCommitmentPosition(key string) (uint64, error) {
	if !strings.HasPrefix(key, "cm_") {
		return 0, fmt.Errorf("invalid commitment key")
	}
	raw := strings.TrimPrefix(key, "cm_")
	return strconv.ParseUint(raw, 10, 64)
}

func parseNullifierKey(key string) ([32]byte, error) {
	var out [32]byte
	if !strings.HasPrefix(key, "nf_") {
		return out, fmt.Errorf("invalid nullifier key")
	}
	raw := strings.TrimPrefix(key, "nf_")
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		return out, fmt.Errorf("invalid nullifier key")
	}
	copy(out[:], decoded)
	return out, nil
}
