package node

import (
	"fmt"

	"github.com/jam-duna/jamduna/types"
)

func (n *NodeContent) GetStorage() (types.JAMStorage, error) {
	if n == nil {
		return nil, fmt.Errorf("Node Not initiated")
	}
	if n.store == nil {
		return nil, fmt.Errorf("Node Store Not initiated")
	}
	return n.store, nil
}

