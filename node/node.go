package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	//"crypto/elliptic"
	//"crypto/ecdsa"
	//"crypto/x509"
	//"crypto/x509/pkix"
	//"encoding/pem"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/quic-go/quic-go"
	"golang.org/x/crypto/blake2b"
	"github.com/colorfulnotion/jam/pvm"
	"log"
	"math/big"
	"sync"
	"time"
)

// Hash function using Blake2b
func bhash(data []byte) common.Hash {
	hash := blake2b.Sum256(data)
	return common.BytesToHash(hash[:])
}

const (
	numNodes   = 6
	numKeys    = 100
	quicAddr   = "localhost:%d"
	nodePrefix = "node"
)

type Node struct {
	id     int
	store  *StateDBStorage
	server quic.Listener
	peers  []string
	mutex  sync.Mutex
	VMs map[uint32]*pvm.VM
}

func newNode(id int, peers []string) (*Node, error) {
	path := fmt.Sprintf("path/to/testdb%d", id)
	store, err := NewStateDBStorage(path)
	if err != nil {
		return nil, err
	}

	for i := 1; i <= numKeys; i++ {
		key := common.BytesToHash([]byte(fmt.Sprintf("%d", i)))
		value := []byte(fmt.Sprintf("%s%d", nodePrefix, id))
		err := store.WriteKV(key, value)
		if err != nil {
			return nil, err
		}
	}

	addr := fmt.Sprintf(quicAddr, 9000+id)
	listener, err := quic.ListenAddr(addr, generateTLSConfig(), generateQuicConfig())

	if err != nil {
		return nil, err
	}

	node := &Node{
		id:     id,
		store:  store,
		server: *listener,
		peers:  peers,
	}
	go node.runServer()
	return node, nil
}

func generateTLSConfig() *tls.Config {
	// Use a dummy TLS configuration with InsecureSkipVerify set to true
	return &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
}

func (n *Node) runServer() {
	for {
		conn, err := n.server.Accept(context.Background())
		if err != nil {
			log.Println("Server accept error:", err)
			continue
		}
		go n.handleConnection(conn)
	}
}

func (n *Node) handleConnection(conn quic.Connection) {
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			log.Println("Accept stream error:", err)
			return
		}
		go n.handleStream(stream)
	}
}

func (n *Node) handleStream(stream quic.Stream) {
	defer stream.Close()

	var key common.Hash
	err := binary.Read(stream, binary.LittleEndian, &key)
	if err != nil {
		log.Println("Failed to read key:", err)
		return
	}

	value, err := n.store.ReadKV(key)
	if err != nil {
		log.Println("Failed to read value:", err)
		return
	}

	_, err = stream.Write(value)
	if err != nil {
		log.Println("Failed to write value:", err)
		return
	}
}

func (n *Node) makeRequest(peerAddr string, key common.Hash) ([]byte, error) {
	conn, err := quic.DialAddr(context.Background(), peerAddr, generateTLSConfig(), nil)
	if err != nil {
		return nil, err
	}
	defer conn.CloseWithError(0, "client closing connection")

	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	err = binary.Write(stream, binary.LittleEndian, key)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(stream)
	return buf.Bytes(), nil
}

func (n *Node) runClient() {
	for {
		time.Sleep(1 * time.Second)
		peerIndex, _ := rand.Int(rand.Reader, big.NewInt(numNodes))
		if int(peerIndex.Int64()) == n.id {
			continue
		}

		keyIndex, _ := rand.Int(rand.Reader, big.NewInt(numKeys))
		key := common.BytesToHash([]byte(fmt.Sprintf("%d", keyIndex.Int64()+1)))

		value, err := n.makeRequest(n.peers[peerIndex.Int64()], key)
		if err != nil {
			log.Println("Client request error:", err)
			continue
		}

		n.mutex.Lock()
		err = n.store.WriteKV(key, value)
		n.mutex.Unlock()
		if err != nil {
			log.Println("Failed to write key-value pair:", err)
		}
	}
}

func generateQuicConfig() *quic.Config {
	return &quic.Config{}
}
