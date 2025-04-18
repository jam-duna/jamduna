package node

import (
	"sync"

	"github.com/colorfulnotion/jam/common"
)

type BlockAnnouncementChecker struct {
	sync.Mutex
	IsSent map[common.Hash]map[uint16]bool
}

func InitBlockAnnouncementChecker() *BlockAnnouncementChecker {
	return &BlockAnnouncementChecker{
		IsSent: make(map[common.Hash]map[uint16]bool),
	}
}

func (bac *BlockAnnouncementChecker) CheckAndSet(hash common.Hash, nodeID uint16) bool {
	bac.Lock()
	defer bac.Unlock()

	if _, ok := bac.IsSent[hash]; !ok {
		bac.IsSent[hash] = make(map[uint16]bool)
	}
	if _, ok := bac.IsSent[hash][nodeID]; !ok {
		bac.IsSent[hash][nodeID] = true
		return false
	}
	return true
}

func (bac *BlockAnnouncementChecker) Set(hash common.Hash, id uint16) {
	bac.Lock()
	defer bac.Unlock()
	if _, ok := bac.IsSent[hash]; !ok {
		bac.IsSent[hash] = make(map[uint16]bool)
	}
	bac.IsSent[hash][id] = true
}

func (bac *BlockAnnouncementChecker) Clear(hash common.Hash) {
	bac.Lock()
	defer bac.Unlock()
	delete(bac.IsSent, hash)
}
