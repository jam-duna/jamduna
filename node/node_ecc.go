package node

import (
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
)

func (n *Node) GetStorage() (*storage.StateDBStorage, error) {
	if n == nil {
		return nil, fmt.Errorf("Node Not initiated")
	}
	if n.store == nil {
		return nil, fmt.Errorf("Node Store Not initiated")
	}
	return n.store, nil
}

func (n *Node) ReadKV(key common.Hash) ([]byte, error) {
	store, err := n.GetStorage()
	if err != nil {
		return []byte{}, err
	}
	val, err := store.ReadKV(key)
	if err != nil {
		return []byte{}, fmt.Errorf("ReadKV K=%v not found\n", key)
	}
	return val, nil
}

func (n *Node) WriteKV(key common.Hash, val []byte) error {
	store, err := n.GetStorage()
	if err != nil {
		return err
	}
	err = store.WriteKV(key, val)
	if err != nil {
		return fmt.Errorf("WriteKV K=%v|V=%x err:%v\n", key, val, err)
	}
	return nil
}

func (n *Node) WriteRawKV(key string, val []byte) error {
	store, err := n.GetStorage()
	if err != nil {
		return err
	}

	err = store.WriteRawKV([]byte(key), val)
	if err != nil {
		return fmt.Errorf("WriteRawKV K=%v|V=%x Err:%v\n", key, val, err)
	}
	return nil
}

func (n *Node) WriteKVByte(key []byte, val []byte) error {
	store, err := n.GetStorage()
	if err != nil {
		return err
	}
	err = store.WriteRawKV(key, val)
	if err != nil {
		return fmt.Errorf("WriteKVByte K=%v|V=%x Err:%v\n", key, val, err)
	}
	return nil
}

func (n *Node) ReadRawKV(key []byte) ([]byte, bool, error) {
	store, err := n.GetStorage()
	if err != nil {
		return []byte{}, false, err
	}
	val, ok, err := store.ReadRawKV([]byte(key))
	if err != nil {
		return []byte{}, false, fmt.Errorf("ReadRawKV Err:%v\n", err)
	} else if !ok {
		fmt.Printf("ReadRawKV K=%x not found\n", string(key))
		return []byte{}, false, fmt.Errorf("ReadRawKV K=%v not found\n", string(key))
	}
	return val, true, nil
}

func (n *Node) ReadKVByte(key []byte) ([]byte, bool, error) {
	store, err := n.GetStorage()
	if err != nil {
		return []byte{}, false, err
	}
	val, ok, err := store.ReadRawKV(key)
	if err != nil {
		return []byte{}, false, fmt.Errorf("ReadKVByte Err:%v\n", err)
	} else if !ok {
		return []byte{}, false, fmt.Errorf("ReadKVByte K=%v not found\n", key)
	}
	return val, true, nil
}
