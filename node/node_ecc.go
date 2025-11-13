package node

import (
	"fmt"

	storage "github.com/colorfulnotion/jam/storage"
)

func (n *NodeContent) GetStorage() (storage.JAMStorage, error) {
	if n == nil {
		return nil, fmt.Errorf("Node Not initiated")
	}
	if n.store == nil {
		return nil, fmt.Errorf("Node Store Not initiated")
	}
	return n.store, nil
}

