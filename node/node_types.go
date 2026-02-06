package node

import (
	"sync"

	"github.com/jam-duna/jamduna/common"
)

type BlockAnnouncementChecker struct {
	sync.Mutex
	IsSent map[common.Hash]map[string]bool // <blockHash> -> <peerKey> -> seen
}

func InitBlockAnnouncementChecker() *BlockAnnouncementChecker {
	return &BlockAnnouncementChecker{
		IsSent: make(map[common.Hash]map[string]bool),
	}
}

func (bac *BlockAnnouncementChecker) CheckAndSet(hash common.Hash, peerKey string) bool {
	bac.Lock()
	defer bac.Unlock()

	if _, ok := bac.IsSent[hash]; !ok {
		bac.IsSent[hash] = make(map[string]bool)
	}
	if _, ok := bac.IsSent[hash][peerKey]; !ok {
		bac.IsSent[hash][peerKey] = true
		return false
	}
	return true
}

func (bac *BlockAnnouncementChecker) Set(hash common.Hash, peerKey string) {
	bac.Lock()
	defer bac.Unlock()
	if _, ok := bac.IsSent[hash]; !ok {
		bac.IsSent[hash] = make(map[string]bool)
	}
	bac.IsSent[hash][peerKey] = true
}

func (bac *BlockAnnouncementChecker) Clear(hash common.Hash) {
	bac.Lock()
	defer bac.Unlock()
	delete(bac.IsSent, hash)
}
