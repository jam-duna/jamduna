package merkle

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/jam-duna/jamduna/bmt/core"
)

// WitnessBuilder builds merkle witnesses (proofs) for tree updates.
type WitnessBuilder struct {
	PageId   core.PageId
	Updates  []PageUpdate
	Hasher   *core.Blake2bBinaryHasher
	Siblings [][]byte
	Entries  []WitnessEntry
}

// WitnessEntry represents a single entry in a witness proof.
type WitnessEntry struct {
	Key           []byte
	OldValue      []byte
	NewValue      []byte
	IsDelete      bool
	SiblingHashes [][]byte
}

// Witness represents a complete merkle proof for a set of updates.
type Witness struct {
	PageId  core.PageId
	Entries []WitnessEntry
	RootHash []byte
}

// Serialize converts the witness to a byte representation.
func (wb *WitnessBuilder) Serialize() ([]byte, error) {
	buf := bytes.NewBuffer(nil)

	// Write header
	if err := wb.writeHeader(buf); err != nil {
		return nil, fmt.Errorf("failed to write header: %w", err)
	}

	// Write entries
	if err := wb.writeEntries(buf); err != nil {
		return nil, fmt.Errorf("failed to write entries: %w", err)
	}

	return buf.Bytes(), nil
}

// writeHeader writes the witness header.
func (wb *WitnessBuilder) writeHeader(buf *bytes.Buffer) error {
	// Write magic number for witness format
	if err := binary.Write(buf, binary.LittleEndian, uint32(0x57495453)); err != nil { // "WITS"
		return err
	}

	// Write version
	if err := binary.Write(buf, binary.LittleEndian, uint32(1)); err != nil {
		return err
	}

	// Write page ID
	pageIdBytes := wb.PageId.Encode()
	if _, err := buf.Write(pageIdBytes[:]); err != nil {
		return err
	}

	// Write number of entries
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(wb.Entries))); err != nil {
		return err
	}

	return nil
}

// writeEntries writes all witness entries.
func (wb *WitnessBuilder) writeEntries(buf *bytes.Buffer) error {
	for i, entry := range wb.Entries {
		if err := wb.writeEntry(buf, &entry); err != nil {
			return fmt.Errorf("failed to write entry %d: %w", i, err)
		}
	}
	return nil
}

// writeEntry writes a single witness entry.
func (wb *WitnessBuilder) writeEntry(buf *bytes.Buffer, entry *WitnessEntry) error {
	// Write key length and key
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(entry.Key))); err != nil {
		return err
	}
	if _, err := buf.Write(entry.Key); err != nil {
		return err
	}

	// Write old value length and old value (0 if nil)
	oldValueLen := uint32(0)
	if entry.OldValue != nil {
		oldValueLen = uint32(len(entry.OldValue))
	}
	if err := binary.Write(buf, binary.LittleEndian, oldValueLen); err != nil {
		return err
	}
	if entry.OldValue != nil {
		if _, err := buf.Write(entry.OldValue); err != nil {
			return err
		}
	}

	// Write new value length and new value (0 if delete or nil)
	newValueLen := uint32(0)
	if entry.NewValue != nil && !entry.IsDelete {
		newValueLen = uint32(len(entry.NewValue))
	}
	if err := binary.Write(buf, binary.LittleEndian, newValueLen); err != nil {
		return err
	}
	if entry.NewValue != nil && !entry.IsDelete {
		if _, err := buf.Write(entry.NewValue); err != nil {
			return err
		}
	}

	// Write delete flag
	deleteFlag := uint8(0)
	if entry.IsDelete {
		deleteFlag = 1
	}
	if err := binary.Write(buf, binary.LittleEndian, deleteFlag); err != nil {
		return err
	}

	// Write number of sibling hashes
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(entry.SiblingHashes))); err != nil {
		return err
	}

	// Write sibling hashes
	for _, siblingHash := range entry.SiblingHashes {
		if len(siblingHash) != 32 {
			return fmt.Errorf("invalid sibling hash length: %d (expected 32)", len(siblingHash))
		}
		if _, err := buf.Write(siblingHash); err != nil {
			return err
		}
	}

	return nil
}

// DeserializeWitness deserializes a witness from bytes.
func DeserializeWitness(data []byte) (*Witness, error) {
	buf := bytes.NewReader(data)

	// Read header
	witness, err := readWitnessHeader(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Read entries
	if err := readWitnessEntries(buf, witness); err != nil {
		return nil, fmt.Errorf("failed to read entries: %w", err)
	}

	return witness, nil
}

// readWitnessHeader reads the witness header.
func readWitnessHeader(buf *bytes.Reader) (*Witness, error) {
	// Read magic number
	var magic uint32
	if err := binary.Read(buf, binary.LittleEndian, &magic); err != nil {
		return nil, err
	}
	if magic != 0x57495453 { // "WITS"
		return nil, fmt.Errorf("invalid magic number: 0x%x", magic)
	}

	// Read version
	var version uint32
	if err := binary.Read(buf, binary.LittleEndian, &version); err != nil {
		return nil, err
	}
	if version != 1 {
		return nil, fmt.Errorf("unsupported version: %d", version)
	}

	// Read page ID
	var pageIdBytes [32]byte
	if _, err := buf.Read(pageIdBytes[:]); err != nil {
		return nil, err
	}
	pageId, err := core.DecodePageId(pageIdBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode page ID: %w", err)
	}

	// Read number of entries
	var numEntries uint32
	if err := binary.Read(buf, binary.LittleEndian, &numEntries); err != nil {
		return nil, err
	}

	return &Witness{
		PageId:  *pageId,
		Entries: make([]WitnessEntry, 0, numEntries),
	}, nil
}

// readWitnessEntries reads all witness entries.
func readWitnessEntries(buf *bytes.Reader, witness *Witness) error {
	expectedEntries := cap(witness.Entries)

	for i := 0; i < expectedEntries; i++ {
		entry, err := readWitnessEntry(buf)
		if err != nil {
			return fmt.Errorf("failed to read entry %d: %w", i, err)
		}
		witness.Entries = append(witness.Entries, *entry)
	}

	return nil
}

// readWitnessEntry reads a single witness entry.
func readWitnessEntry(buf *bytes.Reader) (*WitnessEntry, error) {
	entry := &WitnessEntry{}

	// Read key
	var keyLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &keyLen); err != nil {
		return nil, err
	}
	entry.Key = make([]byte, keyLen)
	if _, err := buf.Read(entry.Key); err != nil {
		return nil, err
	}

	// Read old value
	var oldValueLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &oldValueLen); err != nil {
		return nil, err
	}
	if oldValueLen > 0 {
		entry.OldValue = make([]byte, oldValueLen)
		if _, err := buf.Read(entry.OldValue); err != nil {
			return nil, err
		}
	}

	// Read new value
	var newValueLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &newValueLen); err != nil {
		return nil, err
	}
	if newValueLen > 0 {
		entry.NewValue = make([]byte, newValueLen)
		if _, err := buf.Read(entry.NewValue); err != nil {
			return nil, err
		}
	}

	// Read delete flag
	var deleteFlag uint8
	if err := binary.Read(buf, binary.LittleEndian, &deleteFlag); err != nil {
		return nil, err
	}
	entry.IsDelete = deleteFlag != 0

	// Read sibling hashes
	var numSiblings uint32
	if err := binary.Read(buf, binary.LittleEndian, &numSiblings); err != nil {
		return nil, err
	}

	entry.SiblingHashes = make([][]byte, numSiblings)
	for i := uint32(0); i < numSiblings; i++ {
		siblingHash := make([]byte, 32)
		if _, err := buf.Read(siblingHash); err != nil {
			return nil, err
		}
		entry.SiblingHashes[i] = siblingHash
	}

	return entry, nil
}

// VerifyWitness verifies a witness against the given root hash.
func (w *Witness) VerifyWitness(expectedRootHash []byte, hasher *core.Blake2bBinaryHasher) error {
	// For each entry, verify the merkle path leads to the root
	for i, entry := range w.Entries {
		if err := w.verifyEntry(&entry, expectedRootHash, hasher); err != nil {
			return fmt.Errorf("entry %d verification failed: %w", i, err)
		}
	}
	return nil
}

// verifyEntry verifies a single witness entry.
func (w *Witness) verifyEntry(entry *WitnessEntry, expectedRootHash []byte, hasher *core.Blake2bBinaryHasher) error {
	// Start with the leaf hash
	var leafHash []byte
	if entry.IsDelete {
		// For deletes, hash the old value
		if entry.OldValue != nil {
			leafData := append(entry.Key, entry.OldValue...)
			hash := hasher.Hash(leafData)
			leafHash = hash[:]
		} else {
			return fmt.Errorf("delete operation missing old value")
		}
	} else {
		// For inserts/updates, hash the new value
		if entry.NewValue != nil {
			leafData := append(entry.Key, entry.NewValue...)
			hash := hasher.Hash(leafData)
			leafHash = hash[:]
		} else {
			return fmt.Errorf("insert/update operation missing new value")
		}
	}

	// Walk up the tree using sibling hashes
	currentHash := leafHash
	for _, siblingHash := range entry.SiblingHashes {
		// Combine current hash with sibling hash
		// The order depends on the position in the tree (left or right)
		// For simplicity, we'll always put current hash first
		combinedData := append(currentHash, siblingHash...)
		hash := hasher.Hash(combinedData)
		currentHash = hash[:]
	}

	// Verify we reached the expected root
	if !bytes.Equal(currentHash, expectedRootHash) {
		return fmt.Errorf("witness verification failed: computed root %x != expected root %x",
			currentHash, expectedRootHash)
	}

	return nil
}