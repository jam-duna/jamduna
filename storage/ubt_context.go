package storage

import (
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// UBTExecContext holds per-execution state for EVM/UBT operations.
type UBTExecContext struct {
	manager        *UBTTreeManager
	activeRoot     common.Hash
	pinnedRoot     *common.Hash
	pinnedTree     *UnifiedBinaryTree
	readLog        []types.UBTRead
	readLogEnabled bool
	readLogMutex   sync.Mutex
	mutex          sync.RWMutex
}

func NewUBTExecContext(manager *UBTTreeManager) *UBTExecContext {
	return &UBTExecContext{manager: manager, readLog: make([]types.UBTRead, 0), readLogEnabled: true}
}

func (ctx *UBTExecContext) GetActiveRoot() common.Hash {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return ctx.activeRoot
}

func (ctx *UBTExecContext) SetActiveRoot(root common.Hash) error {
	if root != (common.Hash{}) && !ctx.manager.HasTree(root) {
		return fmt.Errorf("cannot set active root to %s: tree not found", root.Hex())
	}
	ctx.mutex.Lock()
	ctx.activeRoot = root
	ctx.mutex.Unlock()
	return nil
}

func (ctx *UBTExecContext) ClearActiveRoot() {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	ctx.activeRoot = common.Hash{}
}

// GetActiveTree returns the UBT tree for reads. Priority: pinned > active > canonical.
func (ctx *UBTExecContext) GetActiveTree() interface{} {
	return ctx.GetActiveTreeTyped()
}

// GetActiveTreeTyped returns the typed UBT tree. Priority: pinned > active > canonical.
func (ctx *UBTExecContext) GetActiveTreeTyped() *UnifiedBinaryTree {
	ctx.mutex.RLock()
	if ctx.pinnedTree != nil {
		tree := ctx.pinnedTree
		pinnedRoot := ""
		if ctx.pinnedRoot != nil {
			pinnedRoot = ctx.pinnedRoot.Hex()[:16]
		}
		treeRootHash := tree.RootHash()
		ctx.mutex.RUnlock()
		log.Debug(log.EVM, "GetActiveTreeTyped: pinnedTree", "pinnedRoot", pinnedRoot, "treeRoot", fmt.Sprintf("%x", treeRootHash[:8]))
		return tree
	}
	activeRoot := ctx.activeRoot
	ctx.mutex.RUnlock()

	if activeRoot != (common.Hash{}) {
		if tree, exists := ctx.manager.GetTree(activeRoot); exists {
			log.Debug(log.EVM, "GetActiveTreeTyped: activeRoot", "root", activeRoot.Hex()[:16])
			return tree
		}
	}

	tree := ctx.manager.GetCanonicalTree()
	canonicalRoot := ctx.manager.GetCanonicalRoot()
	log.Debug(log.EVM, "GetActiveTreeTyped: canonical", "root", canonicalRoot.Hex()[:16])
	return tree
}

// GetActiveUBTRoot returns the root hash. Priority: pinned > active > canonical.
func (ctx *UBTExecContext) GetActiveUBTRoot() common.Hash {
	ctx.mutex.RLock()
	if ctx.pinnedRoot != nil {
		root := *ctx.pinnedRoot
		ctx.mutex.RUnlock()
		return root
	}
	activeRoot := ctx.activeRoot
	ctx.mutex.RUnlock()
	if activeRoot != (common.Hash{}) {
		return activeRoot
	}
	return ctx.manager.GetCanonicalRoot()
}

func (ctx *UBTExecContext) PinToStateRoot(root common.Hash) error {
	tree, found := ctx.manager.GetTree(root)
	if !found {
		availableRoots := ctx.manager.ListRoots()
		treeStoreRoots := make([]string, 0, len(availableRoots))
		for _, r := range availableRoots {
			treeStoreRoots = append(treeStoreRoots, r.Hex()[:10])
		}
		log.Warn(log.EVM, "PinToStateRoot: not found", "root", root.Hex(), "available", treeStoreRoots)
		return fmt.Errorf("state root %s not available for pinning", root.Hex())
	}
	ctx.mutex.Lock()
	ctx.pinnedRoot = &root
	ctx.pinnedTree = tree
	ctx.mutex.Unlock()
	log.Info(log.EVM, "PinToStateRoot", "root", root.Hex())
	return nil
}

func (ctx *UBTExecContext) UnpinState() {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()
	if ctx.pinnedRoot != nil {
		log.Info(log.EVM, "UnpinState", "was", ctx.pinnedRoot.Hex())
	}
	ctx.pinnedRoot = nil
	ctx.pinnedTree = nil
}

func (ctx *UBTExecContext) IsPinned() bool {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return ctx.pinnedTree != nil
}

func (ctx *UBTExecContext) GetPinnedRoot() *common.Hash {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	if ctx.pinnedRoot == nil {
		return nil
	}
	rootCopy := *ctx.pinnedRoot
	return &rootCopy
}

func (ctx *UBTExecContext) GetPinnedTree() *UnifiedBinaryTree {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()
	return ctx.pinnedTree
}

func (ctx *UBTExecContext) AppendRead(read types.UBTRead) {
	ctx.readLogMutex.Lock()
	defer ctx.readLogMutex.Unlock()
	if ctx.readLogEnabled {
		ctx.readLog = append(ctx.readLog, read)
	}
}

func (ctx *UBTExecContext) GetReadLog() []types.UBTRead {
	ctx.readLogMutex.Lock()
	defer ctx.readLogMutex.Unlock()
	logCopy := make([]types.UBTRead, len(ctx.readLog))
	copy(logCopy, ctx.readLog)
	return logCopy
}

func (ctx *UBTExecContext) ClearReadLog() {
	ctx.readLogMutex.Lock()
	defer ctx.readLogMutex.Unlock()
	ctx.readLog = nil
}

func (ctx *UBTExecContext) SetReadLogEnabled(enabled bool) {
	ctx.readLogMutex.Lock()
	defer ctx.readLogMutex.Unlock()
	ctx.readLogEnabled = enabled
}

func (ctx *UBTExecContext) IsReadLogEnabled() bool {
	ctx.readLogMutex.Lock()
	defer ctx.readLogMutex.Unlock()
	return ctx.readLogEnabled
}

// Clone creates a new context for NewSession (shares manager, inherits pinned state, fresh readLog).
func (ctx *UBTExecContext) Clone() *UBTExecContext {
	ctx.mutex.RLock()
	var copiedPinnedRoot *common.Hash
	if ctx.pinnedRoot != nil {
		rootCopy := *ctx.pinnedRoot
		copiedPinnedRoot = &rootCopy
	}
	pinnedTree := ctx.pinnedTree
	ctx.mutex.RUnlock()

	ctx.readLogMutex.Lock()
	readLogEnabled := ctx.readLogEnabled
	ctx.readLogMutex.Unlock()

	return &UBTExecContext{
		manager:        ctx.manager,
		activeRoot:     common.Hash{},
		pinnedRoot:     copiedPinnedRoot,
		pinnedTree:     pinnedTree,
		readLog:        make([]types.UBTRead, 0),
		readLogEnabled: readLogEnabled,
	}
}
