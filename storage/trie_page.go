package storage

import (
	"fmt"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/types"
)

// ============================================================================
// Page Serialization/Deserialization
// ============================================================================

// Serialize converts Page to 4096-byte representation
func (p *Page) Serialize() []byte {
	data := make([]byte, 4096)

	// Copy bitmaps
	copy(data[0:16], p.IsLeaf.data[:])
	copy(data[16:32], p.IsEmbedded.data[:])

	// Copy all 126 nodes
	for i := 0; i < 126; i++ {
		offset := 32 + i*32
		copy(data[offset:offset+32], p.Nodes[i][:])
	}

	// Reserved section (bytes 4064-4095) remains zero

	return data
}

// Deserialize populates Page from 4096-byte data
func (p *Page) Deserialize(data []byte) error {
	if len(data) != 4096 {
		return fmt.Errorf("invalid page size: %d", len(data))
	}

	// Load bitmaps
	copy(p.IsLeaf.data[:], data[0:16])
	copy(p.IsEmbedded.data[:], data[16:32])

	// Load all 126 nodes
	for i := 0; i < 126; i++ {
		offset := 32 + i*32
		copy(p.Nodes[i][:], data[offset:offset+32])
	}

	// Compute SubRoot as hash of entire page data (using Blake2b-256)
	p.SubRoot = common.BytesToHash(trieComputeHash(data))

	// Clear dirty tracking
	p.Dirty = false

	// SubPages remain nil until opened
	p.SubPages = [126]*Page{}

	return nil
}

// ============================================================================
// Page Methods: Get, Insert, Delete
// ============================================================================

// Get retrieves value by key from this page (recursive)
func (p *Page) Get(key [31]byte, bitOffset int, db types.JAMStorage) ([]byte, bool, error) {
	nodeIdx := 0

	// Traverse up to 6 layers within this page
	for layer := 0; layer < 6; layer++ {
		if bitOffset >= 248 {
			break
		}

		nodeData := p.Nodes[nodeIdx]

		// Check if this is a leaf node
		if p.IsLeaf.Get(nodeIdx) {
			if p.IsEmbedded.Get(nodeIdx) {
				// Embedded value - return directly
				return pageStripTrailingZeros(nodeData[:]), true, nil
			} else {
				// Large value - fetch from LevelDB
				encodingKey := append([]byte{0x01}, nodeData[:]...)
				encoding, found, err := db.Get(encodingKey)
				if err != nil || !found {
					return nil, false, err
				}

				valueHash := encoding[32:64]
				valueKey := append([]byte{0x07}, valueHash...)
				value, found, err := db.Get(valueKey)
				if err != nil || !found {
					return nil, false, err
				}

				return value, true, nil
			}
		}

		// Check if node is empty (not found)
		if pageIsAllZeros(nodeData[:]) {
			return nil, false, nil
		}

		// Get next bit from key
		bit := pageGetBit(key[:], bitOffset)
		bitOffset++

		// Calculate next node index
		nextLayer := layer + 1
		if nextLayer >= 6 {
			// Reached page boundary - descend to child page
			currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
			childIdx := currentPos*2 + bit

			// The child node in layer 5 contains the SubRoot hash of the child page
			childNodeIdx := pageNodeIndex(5, childIdx)
			childPageHash := p.Nodes[childNodeIdx]

			// Check if child page exists
			if pageIsAllZeros(childPageHash[:]) {
				return nil, false, nil // Not found
			}

			// Lazy load subpage if needed
			if p.SubPages[childIdx] == nil {
				// Load page by its SubRoot hash
				pageKey := append([]byte{0x06}, childPageHash[:]...)
				childPageData, found, err := db.Get(pageKey)
				if err != nil || !found {
					return nil, false, nil // Not found
				}

				childPage := &Page{}
				if err := childPage.Deserialize(childPageData); err != nil {
					return nil, false, err
				}

				p.SubPages[childIdx] = childPage
			}

			// Recursive call to child page
			return p.SubPages[childIdx].Get(key, bitOffset, db)
		}

		// Navigate within page
		currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
		childPos := currentPos*2 + bit
		nodeIdx = pageNodeIndex(nextLayer, childPos)
	}

	return nil, false, nil
}

// Insert adds or updates a key-value pair in this page (recursive)
func (p *Page) Insert(key [31]byte, value []byte, bitOffset int, db types.JAMStorage) error {
	nodeIdx := 0

	for layer := 0; layer < 6; layer++ {
		if bitOffset >= 248 {
			break
		}

		nodeData := p.Nodes[nodeIdx]

		// Check if current position is empty
		if pageIsAllZeros(nodeData[:]) {
			// Insert leaf here
			return p.insertLeafAt(nodeIdx, key, value, db)
		}

		// Check if this is a leaf (collision case)
		if p.IsLeaf.Get(nodeIdx) {
			// Need to split this leaf
			return p.splitLeaf(nodeIdx, layer, bitOffset, key, value, db)
		}

		// Branch node - navigate to child
		bit := pageGetBit(key[:], bitOffset)
		bitOffset++

		nextLayer := layer + 1
		if nextLayer >= 6 {
			// Descend to child page
			currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
			childIdx := currentPos*2 + bit

			// The child node in layer 5 contains the SubRoot hash of the child page
			childNodeIdx := pageNodeIndex(5, childIdx)
			childPageHash := p.Nodes[childNodeIdx]

			// Lazy load or create subpage
			if p.SubPages[childIdx] == nil {
				if pageIsAllZeros(childPageHash[:]) {
					// Child page doesn't exist - create new one
					p.SubPages[childIdx] = &Page{}
				} else {
					// Load existing child page by its SubRoot hash
					pageKey := append([]byte{0x06}, childPageHash[:]...)
					childPageData, found, err := db.Get(pageKey)
					if err != nil || !found {
						// Page referenced but not found - create new
						p.SubPages[childIdx] = &Page{}
					} else {
						childPage := &Page{}
						if err := childPage.Deserialize(childPageData); err != nil {
							return err
						}
						p.SubPages[childIdx] = childPage
					}
				}
			}

			// Recursive insert
			if err := p.SubPages[childIdx].Insert(key, value, bitOffset, db); err != nil {
				return err
			}

			// Mark page as dirty (child's SubRoot will be updated in Flush)
			p.markDirty()
			return nil
		}

		// Navigate within page
		currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
		childPos := currentPos*2 + bit
		nodeIdx = pageNodeIndex(nextLayer, childPos)
	}

	return nil
}

// Delete removes a key from this page (recursive)
func (p *Page) Delete(key [31]byte, bitOffset int, db types.JAMStorage) error {
	nodeIdx := 0

	for layer := 0; layer < 6; layer++ {
		if bitOffset >= 248 {
			break
		}

		nodeData := p.Nodes[nodeIdx]

		// Check if node is empty (key not found)
		if pageIsAllZeros(nodeData[:]) {
			return nil // Nothing to delete
		}

		// Check if this is a leaf
		if p.IsLeaf.Get(nodeIdx) {
			// Found the leaf - delete it
			p.IsLeaf.Set(nodeIdx, false)
			p.IsEmbedded.Set(nodeIdx, false)

			// Zero out the node
			for i := range p.Nodes[nodeIdx] {
				p.Nodes[nodeIdx][i] = 0
			}

			p.markDirty()
			return nil
		}

		// Branch node - navigate to child
		bit := pageGetBit(key[:], bitOffset)
		bitOffset++

		nextLayer := layer + 1
		if nextLayer >= 6 {
			// Descend to child page
			currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
			childIdx := currentPos*2 + bit

			// The child node in layer 5 contains the SubRoot hash of the child page
			childNodeIdx := pageNodeIndex(5, childIdx)
			childPageHash := p.Nodes[childNodeIdx]

			// Check if child page exists
			if pageIsAllZeros(childPageHash[:]) {
				return nil // Key not found
			}

			// Lazy load subpage if needed
			if p.SubPages[childIdx] == nil {
				pageKey := append([]byte{0x06}, childPageHash[:]...)
				childPageData, found, err := db.Get(pageKey)
				if err != nil || !found {
					return nil // Key not found
				}

				childPage := &Page{}
				if err := childPage.Deserialize(childPageData); err != nil {
					return err
				}

				p.SubPages[childIdx] = childPage
			}

			// Recursive delete
			if err := p.SubPages[childIdx].Delete(key, bitOffset, db); err != nil {
				return err
			}

			// Mark page as dirty (child's SubRoot will be updated in Flush)
			p.markDirty()
			return nil
		}

		// Navigate within page
		currentPos := nodeIdx - ((1 << (layer + 1)) - 2)
		childPos := currentPos*2 + bit
		nodeIdx = pageNodeIndex(nextLayer, childPos)
	}

	return nil
}

// ============================================================================
// Page Helper Methods
// ============================================================================

// insertLeafAt stores a leaf value at the given node index
func (p *Page) insertLeafAt(nodeIdx int, key [31]byte, value []byte, db types.JAMStorage) error {
	p.IsLeaf.Set(nodeIdx, true)

	if len(value) <= 32 {
		// Embedded value - store directly
		p.IsEmbedded.Set(nodeIdx, true)
		copy(p.Nodes[nodeIdx][:], value)
	} else {
		// Large value - store hash of 64-byte encoding
		p.IsEmbedded.Set(nodeIdx, false)

		encoding := make([]byte, 64)
		encoding[0] = 0xC0
		copy(encoding[1:32], key[:])

		// Hash the value (using Blake2b-256)
		valueHash := trieComputeHash(value)
		copy(encoding[32:64], valueHash[:])

		// Compute encoding hash (using Blake2b-256)
		encodingHash := trieComputeHash(encoding)
		copy(p.Nodes[nodeIdx][:], encodingHash[:])

		// Store in LevelDB
		encodingKey := append([]byte{0x01}, encodingHash[:]...)
		db.Insert(encodingKey, encoding)

		valueKey := append([]byte{0x07}, valueHash[:]...)
		db.Insert(valueKey, value)
	}

	p.markDirty()
	return nil
}

// splitLeaf handles the case where we need to insert at a leaf position
// TODO: Full implementation requires key extraction from existing leaf
func (p *Page) splitLeaf(nodeIdx, layer, bitOffset int, newKey [31]byte, newValue []byte, db types.JAMStorage) error {
	return fmt.Errorf("splitLeaf not implemented - requires key storage in leaves")
}

// markDirty marks the page as needing flush
func (p *Page) markDirty() {
	p.Dirty = true
}

// Flush writes dirty page and all dirty subpages to LevelDB
func (p *Page) Flush(db types.JAMStorage) error {
	// Flush subpages first (bottom-up)
	for i, subPage := range p.SubPages {
		if subPage != nil && subPage.Dirty {
			if err := subPage.Flush(db); err != nil {
				return err
			}

			// Update our node with child's SubRoot
			// Find which node in layer 5 corresponds to subpage index i
			nodeIdx := pageNodeIndex(5, i)
			copy(p.Nodes[nodeIdx][:], subPage.SubRoot[:])
			p.markDirty()
		}
	}

	// Recompute hashes if dirty
	if p.Dirty {
		p.recomputeHashes()

		// Serialize and compute SubRoot
		data := p.Serialize()
		p.SubRoot = common.BytesToHash(trieComputeHash(data))

		// Save page by its SubRoot hash
		pageKey := append([]byte{0x06}, p.SubRoot[:]...)
		db.Insert(pageKey, data)

		p.Dirty = false
	}

	return nil
}

// recomputeHashes recalculates branch hashes for modified nodes
func (p *Page) recomputeHashes() {
	// Process layers bottom-up
	for layer := 5; layer >= 0; layer-- {
		layerSize := 1 << (layer + 1)
		layerOffset := (1 << (layer + 1)) - 2

		for pos := 0; pos < layerSize; pos++ {
			nodeIdx := layerOffset + pos

			// Skip leaves
			if p.IsLeaf.Get(nodeIdx) {
				continue
			}

			// Skip if children are in subpages (layer 5)
			if layer == 5 {
				continue
			}

			// Get child hashes
			leftIdx := pageNodeIndex(layer+1, pos*2)
			rightIdx := pageNodeIndex(layer+1, pos*2+1)

			// Compute branch hash per GP 0.6.2 Equation D.3
			branchData := make([]byte, 64)
			branchData[0] = p.Nodes[leftIdx][0] & 0x7F // Clear top bit
			copy(branchData[1:32], p.Nodes[leftIdx][1:])
			copy(branchData[32:64], p.Nodes[rightIdx][:])

			// Compute branch hash (using Blake2b-256)
			branchHash := trieComputeHash(branchData)
			copy(p.Nodes[nodeIdx][:], branchHash[:])
		}
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

// pageNodeIndex calculates the index in Nodes array
func pageNodeIndex(layer, position int) int {
	if layer < 0 || layer >= 6 {
		return -1
	}
	return (1 << (layer + 1)) - 2 + position
}

// pageGetBit gets bit at offset from byte array
func pageGetBit(data []byte, offset int) int {
	byteIdx := offset / 8
	bitIdx := 7 - (offset % 8)
	if (data[byteIdx] >> bitIdx) & 1 == 1 {
		return 1
	}
	return 0
}

// pageIsAllZeros checks if all bytes are zero
func pageIsAllZeros(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

// pageStripTrailingZeros strips trailing zero bytes
func pageStripTrailingZeros(data []byte) []byte {
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] != 0 {
			return data[:i+1]
		}
	}
	return []byte{}
}
