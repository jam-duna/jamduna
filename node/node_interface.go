package node

import (
	"github.com/colorfulnotion/jam/types"
)

// JNode is an alias for types.JNode for backward compatibility
// The actual interface definition is in types package to avoid circular dependencies
type JNode = types.JNode

// Node-specific extension methods that are not part of the core JNode interface
type JNodeExtended interface {
	types.JNode
	SetJCEManager(jceManager *ManualJCEManager) (err error)
	GetJCEManager() (jceManager *ManualJCEManager, err error)
}
