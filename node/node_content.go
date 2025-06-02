package node

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func (n *NodeContent) GetBlockTree() *types.BlockTree {
	return n.block_tree
}

func (n *NodeContent) SetServiceDir(dir string) {
	n.loaded_services_dir = dir
}

func (n *NodeContent) LoadService(service_name string) ([]byte, error) {
	// read the .pvm from the service directory
	service_path := fmt.Sprintf("%s/%s.pvm", n.loaded_services_dir, service_name)
	return types.ReadCodeWithMetadata(service_path, service_name)
}

func IsWorkPackageInHistory(latestdb *statedb.StateDB, workPackageHash common.Hash) bool {
	for _, block := range latestdb.JamState.RecentBlocks {
		if len(block.Reported) != 0 {
			for _, segmentRootLookup := range block.Reported {
				if segmentRootLookup.WorkPackageHash == workPackageHash {
					return true
				}
			}
		}
	}

	prereqSetFromAccumulationHistory := make(map[common.Hash]struct{})
	for i := 0; i < types.EpochLength; i++ {
		for _, hash := range latestdb.JamState.AccumulationHistory[i].WorkPackageHash {
			prereqSetFromAccumulationHistory[hash] = struct{}{}
		}
	}
	_, exists := prereqSetFromAccumulationHistory[workPackageHash]
	return exists
}
