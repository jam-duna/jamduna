package node

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	rand_math "math/rand"
	"os"
	"sort"
	"sync"
	"time"

	"container/list"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

type ArchiveNode struct {
	NodeContent
	selected_peer      *Peer // we should only create a connection to one peer, but right now our broadcast is not really wisper with the neighbors
	latest_block_mutex sync.Mutex
	latest_block       *types.BlockHeader
	is_sync            bool
	block_forest       *list.List
}

const (
	blk_sync = "block_sync"
	blk      = "block"
	rpc_mod  = "rpc"
)

func (n *ArchiveNode) GetSelectedPeer() *Peer {
	return n.selected_peer
}

func (n *ArchiveNode) IsSync() bool {
	return n.is_sync
}

func NewArchiveNode(id uint16, seed []byte, genesisStateFile string, genesisBlockFile string, epoch0Timestamp uint32, startPeerList map[uint16]*Peer, dataDir string, port int, web_port int) (*ArchiveNode, error) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	//log.Info(module, fmt.Sprintf("[N%v]", id), "addr", addr, "dataDir", dataDir)

	//REQUIRED FOR CAPTURING JOBID. DO NOT DELETE THIS LINE!!
	fmt.Printf("NewArchiveNode addr=%v, dataDir=%v\n", addr, dataDir)
	levelDBPath := fmt.Sprintf("%v/leveldb/%d/", dataDir, port)
	store, err := storage.NewStateDBStorage(levelDBPath)
	if err != nil {
		return nil, err
	}
	node_content := NewNodeContent(9999, store)
	ArchiveNode := &ArchiveNode{
		NodeContent: node_content,
	}

	Ed25519PrivKey := ed25519.NewKeyFromSeed(seed)
	Ed25519PubKey := Ed25519PrivKey.Public().(ed25519.PublicKey)
	cert, err := generateSelfSignedCert(Ed25519PubKey, Ed25519PrivKey)
	if err != nil {
		return nil, fmt.Errorf("Error generating self-signed certificate: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		ClientAuth:         tls.RequireAnyClientCert,
		InsecureSkipVerify: true,
		GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
			return &tls.Config{
				Certificates:       []tls.Certificate{cert},
				ClientAuth:         tls.RequireAnyClientCert,
				InsecureSkipVerify: true,
				VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
					if len(rawCerts) == 0 {
						return fmt.Errorf("no client certificate provided")
					}
					cert, err := x509.ParseCertificate(rawCerts[0])
					if err != nil {
						return fmt.Errorf("failed to parse client certificate: %v", err)
					}
					_, ok := cert.PublicKey.(ed25519.PublicKey)
					if !ok {
						return fmt.Errorf("client public key is not Ed25519")
					}
					return nil
				},
				NextProtos: []string{"h3", "http/1.1", "ping/1.1"},
			}, nil
		},
		NextProtos: []string{"h3", "http/1.1", "ping/1.1"},
	}
	ArchiveNode.node_name = fmt.Sprintf("arc")
	ArchiveNode.tlsConfig = tlsConfig
	for validatorIndex, p := range startPeerList {
		ArchiveNode.peersInfo[validatorIndex] = p
		ArchiveNode.peersInfo[validatorIndex].node = &ArchiveNode.NodeContent
	}
	// ramdomly select a peer
	peers_len := len(ArchiveNode.peersInfo)
	if peers_len == 0 {
		return nil, fmt.Errorf("No peers provided")
	}
	ramdom_index := rand_math.Intn(peers_len)
	ArchiveNode.selected_peer = ArchiveNode.peersInfo[uint16(ramdom_index)]

	block := statedb.NewBlockFromFile(genesisBlockFile)
	_statedb, err := statedb.NewStateDBFromSnapshotRawFile(ArchiveNode.store, genesisStateFile)
	_statedb.Block = block
	_statedb.HeaderHash = block.Header.Hash()
	ArchiveNode.statedb = _statedb
	ArchiveNode.block_forest = list.New()
	ArchiveNode.is_sync = false
	genesis_block_tree := types.NewBlockTree(
		&types.BT_Node{
			Block:    block,
			Children: make([]*types.BT_Node, 0),
		},
	)
	ArchiveNode.block_tree = genesis_block_tree
	if err == nil {
		_statedb.SetID(uint16(id))
		ArchiveNode.addStateDB(_statedb)
	} else {
		fmt.Printf("NewGenesisStateDB ERR %v\n", err)
		return nil, err
	}
	ArchiveNode.epoch0Timestamp = epoch0Timestamp

	validators, _, err := generateValidatorNetwork()
	if err != nil {
		return nil, err
	}
	ArchiveNode.statedb.GetSafrole().NextValidators = validators
	ArchiveNode.statedb.GetSafrole().CurrValidators = validators
	// go ArchiveNode.runJamWeb(uint16(port+1000) + id)
	for _, peer := range ArchiveNode.peersInfo {
		peer.GetOrInitBlockAnnouncementStream()
	}

	listener, err := quic.ListenAddr(addr, tlsConfig, GenerateQuicConfig())
	if err != nil {
		fmt.Printf("ERR %v\n", err)
		return nil, err
	}
	ArchiveNode.server = *listener
	go ArchiveNode.runReceiveBlock()
	go ArchiveNode.runServer()
	go ArchiveNode.SyncState()
	go ArchiveNode.runJamWeb(uint16(web_port))
	go ArchiveNode.StartRPCServer()
	return ArchiveNode, nil
}

func (n *NodeContent) GetBlockTree() *types.BlockTree {
	return n.block_tree
}

func (n *ArchiveNode) InsertBlock(block *types.Block, put_into_tree_directly bool) error {
	// see the length of the forest
	length := n.block_forest.Len()
	_, already_exsist := n.block_tree.GetBlockNode(block.Header.Hash())
	if already_exsist {
		return fmt.Errorf("Block already in the block tree, block hash: %v", block.Header.Hash())
	}
	if length == 0 && put_into_tree_directly {
		// see the orinal block tree's nodes is the same as the block's parent
		_, is_here := n.block_tree.GetBlockNode(block.Header.ParentHeaderHash)
		if is_here {
			err := n.block_tree.AddBlock(block)
			if err != nil {
				return err
			}
		} else {
			err := n.PutBlockInTheForest(block)
			if err != nil {
				return err
			}

		}

	} else {
		err := n.PutBlockInTheForest(block)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *ArchiveNode) PutBlockInTheForest(block *types.Block) error {
	l := n.block_forest
	for e := l.Front(); e != nil; e = e.Next() {
		block_tree := e.Value.(*types.BlockTree)
		block_tree_root := block_tree.GetRoot()
		if block_tree_root.Block.Header.Hash() == block.Header.Hash() {
			return fmt.Errorf("Block already in the forest, block hash: %v", block.Header.Hash())
		}
		_, is_here := block_tree.GetBlockNode(block.Header.ParentHeaderHash)
		if is_here {
			err := block_tree.AddBlock(block)
			if err != nil {
				return err
			} else {
				return nil
			}
		}
	}

	// create another block tree
	block_tree := types.NewBlockTree(
		&types.BT_Node{
			Block:    block,
			Children: make([]*types.BT_Node, 0),
		},
	)
	n.block_forest.PushBack(block_tree)
	return nil
}

func (n *ArchiveNode) runReceiveBlock() {
	// ticker here to avoid high CPU usage
	pulseTicker := time.NewTicker(100 * time.Millisecond)
	defer pulseTicker.Stop()
	for {
		select {
		case <-pulseTicker.C:
			if n.block_forest.Len() == 0 {
				continue
			} else {
				n.is_sync = false
				// see if the forest sync with the block tree
				var err error
				// sync the forest
				// sync every tree in the forest with the block tree genesis block as root
				for e := n.block_forest.Front(); e != nil; e = e.Next() {
					n.block_tree, err = types.MergeTwoBlockTree(n.block_tree, e.Value.(*types.BlockTree))
					if err != nil {
						log.Warn(blk_sync, "Try to MergeTwoBlockTree", "err", err)
					} else {
						n.block_forest.Remove(e)
					}
				}
				if n.block_forest.Len() == 0 {
					n.is_sync = true
					log.Info(blk_sync, "runReceiveBlock", "block_tree", "syncornized to latest block")
				} else {
					block_trees := make([]*types.BlockTree, 0)
					block_trees = append(block_trees, n.block_tree)
					for e := n.block_forest.Front(); e != nil; e = e.Next() {
						block_tree := e.Value.(*types.BlockTree)
						block_trees = append(block_trees, block_tree)
					}
					// sort the block_header_roots by slot
					sort.Slice(block_trees, func(i, j int) bool {
						return block_trees[i].GetRoot().Block.Header.Slot < block_trees[j].GetRoot().Block.Header.Slot
					})
					for i, tree := range block_trees {
						if i < len(block_trees)-1 {
							var leaf *types.BT_Node
							for _, l := range tree.GetLeafs() {
								leaf = l
							}
							slot1 := leaf.Block.Header.Slot
							slot2 := block_trees[i+1].GetRoot().Block.Header.Slot
							diffs := slot2 - slot1
							if diffs >= 1 {
								// send the request to the selected peer
								// get the blocks from the selected peer
								blocksRaw, err := n.selected_peer.SendBlockRequest(block_trees[i+1].GetRoot().Block.Header.ParentHeaderHash, 1, diffs)
								if err != nil {
									log.Error(blk, "SendBlockRequest", "err", err)
								}
								for i := len(blocksRaw) - 1; i >= 0; i-- {
									n.InsertBlock(&blocksRaw[i], false)
									err = n.StoreBlock(&blocksRaw[i], uint16(9999), false)
									if err != nil {
										log.Error(blk, "StoreBlock", "err", err)
									}
								}
							}
						}
					}
				}
			}
		case blockAnnouncement := <-n.blockAnnouncementsCh:
			headerHash := blockAnnouncement.Header.HeaderHash()
			header := blockAnnouncement.Header
			var blocksRaw []types.Block
			var err error
			n.latest_block_mutex.Lock()
			if n.latest_block == nil {
				n.latest_block = &header
			} else if n.latest_block.Slot < header.Slot {
				n.latest_block = &header
			}
			n.latest_block_mutex.Unlock()
			time.Sleep(500 * time.Millisecond)
			author_index := header.AuthorIndex
			for attempt := 1; attempt <= 3; attempt++ {
				blocksRaw, err = n.peersInfo[author_index].SendBlockRequest(headerHash, 1, 1)
				if err == nil {
					if attempt > 1 {
						log.Info(blk, "processBlockAnnouncement", "SendBlockRequest", fmt.Sprintf("attempt %d succeeded", attempt))
					}
					break // exit loop if request succeeds
				}
				//time.Sleep(100)
				if attempt == 3 {
					log.Error(blk, "processBlockAnnouncement", "SendBlockRequest", fmt.Sprintf("attempt %d failed", attempt))
					continue
				}
			}

			block := &blocksRaw[0]
			receivedHeaderHash := block.Header.Hash()
			if receivedHeaderHash != headerHash {
				panic(6665)
			}
			err = n.InsertBlock(block, true)
			if err != nil {
				log.Info(blk_sync, "block tree is sync yet", "block sync", "failed")
			}
			err = n.StoreBlock(block, uint16(9999), false)
			if err != nil {
				log.Error(blk, "StoreBlock", "err", err)
			}

		}
	}
}

func (n *ArchiveNode) SyncState() {
	ticket := time.NewTicker(50 * time.Millisecond)
	for {
		select {
		case <-ticket.C:
			// check the latest statedb
			n.statedbMutex.Lock()
			latest_statedb := n.statedb
			if latest_statedb.Block == nil {
				n.statedbMutex.Unlock()
				continue
			} else {
				if n.latest_block != nil {
					if latest_statedb.HeaderHash == n.latest_block.Hash() {
						// the state is sync
						n.statedbMutex.Unlock()
						continue
					}
				}

			}

			current_block_hash := latest_statedb.Block.Header.Hash()
			curr_node, _ := n.block_tree.GetBlockNode(current_block_hash)
			n.statedbMutex.Unlock()
			var applyChildren func(node *types.BT_Node) error
			applyChildren = func(node *types.BT_Node) error {
				for _, child := range node.Children {
					if child.Applied {
						continue
					}
					err := n.ApplyBlock(child)
					if err != nil {
						log.Error("SyncState", "ApplyBlock", err)
						return err
					} else {
						log.Info("SyncState", "ApplyBlock", "success apply block", fmt.Sprintf("[%s]", child.Block.Header.Hash().String()))
						log.Info("SyncState", "ApplyBlock", "block info", fmt.Sprintf("%s", child.Block.Str()))
					}
					err = applyChildren(child)
					if err != nil {
						return err
					}
				}
				return nil
			}

			err := applyChildren(curr_node)
			if err != nil {
				log.Error("SyncState", "applyChildren", err)
			}

		}

	}
}

func (n *ArchiveNode) ApplyBlock(block_node *types.BT_Node) error {
	old_statedb := n.statedb
	block := block_node.Block.Copy()
	newstatedb, err := statedb.ApplyStateTransitionFromBlock(old_statedb, context.Background(), block, "SyncState")
	if err != nil {
		block_node.Applied = false
		return err
	}
	n.statedb = newstatedb
	n.statedb.Block = block
	n.addStateDB(newstatedb)
	announcement := fmt.Sprintf("{\"method\":\"BlockAnnouncement\",\"result\":{\"blockHash\":\"%s\",\"headerHash\":\"%s\"}}", block.Hash(), block.Header.Hash())
	if n.hub != nil {
		n.hub.broadcast <- []byte(announcement)
	}
	block_node.Applied = true
	return nil
}

func (n *NodeContent) BroadcastPreimageAnnouncement(serviceID uint32, preimageHash common.Hash, preimageLen uint32, preimage []byte) (err error) {
	pa := types.PreimageAnnouncement{
		ServiceIndex: serviceID,
		PreimageHash: preimageHash,
		PreimageLen:  preimageLen,
	}

	n.StoreImage(preimageHash, preimage)

	for _, peer := range n.peersInfo {
		go func(peer *Peer) {
			err := peer.SendPreimageAnnouncement(&pa)
			if err != nil {
				log.Error(rpc_mod, "SendPreimageAnnouncement", err)
			}
		}(peer)
	}

	return nil
}

func (n *ArchiveNode) runServer() {
	for {
		conn, err := n.server.Accept(context.Background())
		if err != nil {
			fmt.Printf("runServer: Server accept error: %v\n", err)
			continue
		}
		go n.handleConnection(conn)
	}
}

func (n *ArchiveNode) handleConnection(conn quic.Connection) {
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			log.Error(debugDA, "AcceptStream", "n", n.id, "err", err)
			break
		}
		go func() {
			n.DispatchIncomingQUICStream(stream)
		}()
	}
}

// jamsnp_dispatch reads from QUIC and dispatches based on message type
func (n *ArchiveNode) DispatchIncomingQUICStream(stream quic.Stream) error {
	var msgType byte

	msgTypeBytes := make([]byte, 1) // code

	msgLenBytes := make([]byte, 4)
	_, err := io.ReadFull(stream, msgTypeBytes)
	if err != nil {
		fmt.Printf("DispatchIncomingQUICStream1 ERR %v\n", err)
		return err
	}
	msgType = msgTypeBytes[0]

	_, err = io.ReadFull(stream, msgLenBytes)
	if err != nil {
		fmt.Printf("DispatchIncomingQUICStream2 ERR %v\n", err)
		return err
	}
	msgLen := binary.BigEndian.Uint32(msgLenBytes)
	msg := make([]byte, msgLen)
	_, err = io.ReadFull(stream, msg)
	if err != nil {
		fmt.Printf("DispatchIncomingQUICStream3 ERR %v\n", err)
		return err
	}
	// Dispatch based on msgType
	switch msgType {
	case CE128_BlockRequest:
		err := n.onBlockRequest(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE128_BlockRequest", "n", n.id, "err", err)
		}
	case CE129_StateRequest:
		err := n.onStateRequest(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE129_StateRequest", "n", n.id, "err", err)
		}
	case CE136_WorkReportRequest:
		err := n.onWorkReportRequest(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE136_WorkReportRequest", "n", n.id, "err", err)
		}
	case CE143_PreimageRequest:
		err := n.onPreimageRequest(stream, msg)
		if err != nil {
			log.Warn(debugStream, "CE143_PreimageRequest", "n", n.id, "err", err)
		}
	default:
		return errors.New("unknown message type")
	}
	return nil
}

func (n *ArchiveNode) SetServiceDir(dir string) {
	n.loaded_services_dir = dir
}

func (n *NodeContent) LoadService(service_name string) ([]byte, error) {
	// read the .pvm from the service directory
	service_path := fmt.Sprintf("%s/%s.pvm", n.loaded_services_dir, service_name)
	service, err := os.ReadFile(service_path)
	return service, err
}
