package node

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"

	"io"
	"log"
	"math/big"
	rand0 "math/rand"
	"strings"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/erasurecoding"
	"github.com/colorfulnotion/jam/trie"
	"github.com/quic-go/quic-go"
	"golang.org/x/crypto/blake2b"
)

// Hash function using Blake2b
func bhash(data []byte) common.Hash {
	hash := blake2b.Sum256(data)
	return common.BytesToHash(hash[:])
}

const (
	numNodes = 6
	quicAddr = "localhost:%d" // karura-internal.polkaholic.io:%d
)

const (
	ValidatorFlag   = "VALIDATOR"
	DAFlag          = "DA"
	ValidatorDAFlag = "VALIDATOR&DA"
)

type QuicMessage struct {
	Id      uint32
	MsgType string `json:"msgType"`
	Payload string `json:"payload"`
}

type NodeInfo struct {
	PeerID     uint32          `json:"peer_id"`
	PeerAddr   string          `json:"peer_addr"`
	RemoteAddr string          `json:"remote_addr"`
	Validator  types.Validator `json:"validator"`
}

type Node struct {
	id        uint32
	coreIndex uint32

	credential types.ValidatorSecret
	server     quic.Listener
	peers      []string
	peersInfo  map[string]NodeInfo //<ed25519> -> NodeInfo
	//peersAddr  	 map[string]string
	tlsConfig *tls.Config
	mutex     sync.Mutex

	connections map[string]quic.Connection
	streams     map[string]quic.Stream
	store       *storage.StateDBStorage /// where to put this?

	// holds a map of epoch to at most 2 tickets
	selfTickets map[uint32][]types.Ticket

	// holds a map of the parenthash to the block
	blocks map[common.Hash]*types.Block
	// holds a map of the hash to the stateDB
	statedbMap map[common.Hash]*statedb.StateDB
	// holds the tip
	statedb      *statedb.StateDB
	connectionMu sync.Mutex
	messageChan  chan statedb.Message
	nodeType     string
}

/*
A Tip StateDB is held in the node structure
When a block comes in, we validate whether the block identified by parenthash and its extrinsics gets to this block.
  if it does, we put it into map[blockHash]*StateDB
  if its the latest timeslot, we update the tip
When a block is authored, we take the latest block identified by some parenthash and its extrinsics and get a new block.
  we update the tip
*/

func generateSeedSet(ringSize int) ([][]byte, error) {

	entropy := bhash([]byte("42"))

	// Generate the ring set with deterministic random seeds
	ringSet := make([][]byte, ringSize)
	for i := 0; i < ringSize; i++ {
		seed := make([]byte, 32)
		if _, err := rand.Read(entropy.Bytes()); err != nil {
			return nil, err
		}
		// XOR the deterministic seed with the random seed to make it deterministic
		for j := range seed {
			seed[j] ^= entropy[j%len(entropy)]
		}
		ringSet[i] = bhash(append(seed[:], byte(i))).Bytes()
	}
	return ringSet, nil
}

func generateSelfSignedCert(ed25519_pub ed25519.PublicKey, ed25519_priv ed25519.PrivateKey) (tls.Certificate, error) {
	// Create a self-signed certificate
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Example Org"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, ed25519_pub, ed25519_priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	// PEM encode the certificate
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	// Convert a generated ed25519 key into a PEM block
	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(ed25519_priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	// PEM encode the private key
	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privKeyBytes})

	// Load the certificate
	return tls.X509KeyPair(certPEM, privKeyPEM)
}

func (n *Node) setValidatorCredential(credential types.ValidatorSecret) {
	n.credential = credential
	if false {
		jsonData, err := json.Marshal(credential)
		if err != nil {
			fmt.Printf("Error marshaling JSON: %v\n", err)
			return
		}
		fmt.Printf("[N%v] credential %s\n", n.id, jsonData)
	}
}

func newNode(id uint32, credential types.ValidatorSecret, genesisConfig *statedb.GenesisConfig, peers []string, peerList map[string]NodeInfo, nodeType string) (*Node, error) {
	path := fmt.Sprintf("/tmp/log/testdb%d", id)
	store, err := storage.NewStateDBStorage(path)
	if err != nil {
		return nil, err
	}

	var cert tls.Certificate
	use_rsa := false
	if use_rsa {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			fmt.Println("Failed to generate private key:", err)
			return nil, err
		}

		template := x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject: pkix.Name{
				Organization: []string{"Org"},
			},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().AddDate(1, 0, 0), // Valid for 1 year
			KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			BasicConstraintsValid: true,
		}

		derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
		if err != nil {
			fmt.Println("Failed to create certificate:", err)
			return nil, err
		}

		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

		cert, err = tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			log.Fatal("Error loading certificate:", err)
		}
	} else {
		privKeyBytes := credential.Ed25519Secret
		//fmt.Printf("privKeyBytes=%x\n", privKeyBytes)
		ed25519_priv := ed25519.PrivateKey(privKeyBytes)
		ed25519_pub := ed25519_priv.Public().(ed25519.PublicKey)

		cert, err = generateSelfSignedCert(ed25519_pub, ed25519_priv)
		if err != nil {
			return nil, fmt.Errorf("Error generating self-signed certificate: %v", err)
		}
	}

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,                                   // For testing purposes only
		NextProtos:         []string{"h3", "http/1.1", "ping/1.1"}, // Enable QUIC and HTTP/3
	}

	addr := fmt.Sprintf(quicAddr, 9000+id)
	//fmt.Printf("[N%v] OPENING %s\n", id, addr)
	listener, err := quic.ListenAddr(addr, tlsConfig, generateQuicConfig())

	if err != nil {
		fmt.Printf("ERR %v\n", err)
		return nil, err
	}

	messageChan := make(chan statedb.Message, 100)

	node := &Node{
		id:          id,
		coreIndex:   id % 2, // TODO: NumCores
		store:       store,
		server:      *listener,
		peers:       peers,
		peersInfo:   peerList,
		tlsConfig:   tlsConfig,
		connections: make(map[string]quic.Connection),
		streams:     make(map[string]quic.Stream),
		messageChan: messageChan,
		nodeType:    nodeType,
		statedbMap:  make(map[common.Hash]*statedb.StateDB),
		blocks:      make(map[common.Hash]*types.Block),
		selfTickets: make(map[uint32][]types.Ticket),
	}

	_statedb, err := statedb.NewGenesisStateDB(node.store, genesisConfig)
	if err == nil {
		_statedb.SetID(id)
		node.addStateDB(_statedb)
	} else {
		fmt.Printf("NewGenesisStateDB ERR %v\n", err)
		return nil, err
	}
	node.setValidatorCredential(credential)

	go node.runServer()
	go node.runClient()
	return node, nil
}

func generateQuicConfig() *quic.Config {
	return &quic.Config{
		Allow0RTT:       true,
		KeepAlivePeriod: time.Minute,
	}
}

func getConnKey(identifier string, incoming bool) string {
	if incoming {
		return fmt.Sprintf("%v-in", identifier)
	} else {
		return fmt.Sprintf("%v-out", identifier)
	}
}

func (n *Node) generatedEpochTickets(epoch uint32) bool {
	_, ok := n.selfTickets[epoch]
	if !ok {
		return false
	}
	return true
}

func (n *Node) GetBandersnatchSecret() []byte {
	return n.credential.BandersnatchSecret
}

func (n *Node) generateEpochTickets(epoch uint32, isNextEpoch bool) {
	sf := n.statedb.GetSafrole()
	tickets := sf.GenerateTickets(n.GetBandersnatchSecret(), isNextEpoch)
	fmt.Printf("[N%v] Generating Tickets for (Epoch,isNextEpoch) = (%v,%v)\n", n.id, epoch, isNextEpoch)
	n.selfTickets[epoch] = tickets
	for _, t := range tickets {
		// send tickets to neighbors
		n.broadcast(t)
	}
}

func (n *Node) GenerateTickets(currJCE uint32) {
	sf := n.statedb.GetSafrole()
	currEpoch, currPhase := sf.EpochAndPhase(currJCE)
	if currPhase >= types.TicketSubmissionEndSlot {
		if n.generatedEpochTickets(uint32(currEpoch+1)) == false {
			n.generateEpochTickets(uint32(currEpoch+1), true)
		}
	} else if n.generatedEpochTickets(uint32(currEpoch)) == false {
		n.generateEpochTickets(uint32(currEpoch), false)
	}
}

func (n *Node) GetNodeType() string {
	return n.nodeType
}

func (n *Node) getState() *statedb.StateDB {
	return n.statedb
}

func (n *Node) getTrie() *trie.MerkleTree {
	s := n.getState()
	return s.GetTrie()
}

func (n *Node) getPeerIndex(identifier string) (uint32, error) {
	peer, exist := n.peersInfo[identifier]
	if exist {
		return peer.PeerID, nil
	}
	return 0, fmt.Errorf("peer not found")
}

func (n *Node) getPeerByIndex(peerIdx uint32) (string, error) {
	for _, peer := range n.peersInfo {
		if peer.PeerID == peerIdx {
			return peer.Validator.Ed25519.String(), nil
		}
	}
	return "", fmt.Errorf("peer not found")
}

func (n *Node) getPeerAddr(identifier string) (string, error) {
	peer, exist := n.peersInfo[identifier]
	if exist {
		//fmt.Printf("getPeerAddr[%v] found %v\n", identifier, peer)
		return peer.PeerAddr, nil

	}
	fmt.Printf("getPeerAddr not found %v\n", identifier)
	return "", fmt.Errorf("peer not found")
}

func (n *Node) addStateDB(_statedb *statedb.StateDB) error {
	if n.statedb == nil || n.statedb.GetBlock() == nil {
		var blkHash common.Hash
		if _statedb.GetBlock() != nil {
			blkHash = _statedb.GetBlock().Hash()
			fmt.Printf("[N%d] addStateDB1 [%v <- %v] (stateRoot: %v)\n", n.id, _statedb.ParentHash, _statedb.BlockHash, _statedb.StateRoot)
		} else {
			fmt.Printf("[N%d] addStateDB0 [%v <- %v] (stateRoot: %v)\n", n.id, _statedb.ParentHash, _statedb.BlockHash, _statedb.StateRoot)
		}
		n.statedb = _statedb
		n.statedbMap[blkHash] = _statedb
		return nil
	}
	if _statedb.GetBlock() == nil {
		fmt.Printf("addStateDB: NO BLOCK!!! %v\n", _statedb)
		panic(0)
	}
	if n.statedb.GetBlock() == nil {
		fmt.Printf("node statedb: NO BLOCK!!! %v\n", n.statedb)
		panic(0)
	}
	if _statedb.GetBlock().TimeSlot() > n.statedb.GetBlock().TimeSlot() {
		fmt.Printf("[N%d] addStateDB TIP %v\n", n.id, _statedb.GetBlock().Hash())
		n.statedb = _statedb
		n.statedbMap[_statedb.GetBlock().Hash()] = _statedb
	}
	return nil
}

func (n *Node) getIdentifier() string {
	return n.credential.Ed25519Pub.String()
}

func (n *Node) ResetPeer(peerIdentifier string) {

	inConnKey := getConnKey(peerIdentifier, false)
	outcomingConnKey := getConnKey(peerIdentifier, false)
	n.connectionMu.Lock()
	defer n.connectionMu.Unlock()
	delete(n.connections, outcomingConnKey)
	delete(n.streams, outcomingConnKey)
	delete(n.connections, inConnKey)
	delete(n.streams, inConnKey)
	peer, exist := n.peersInfo[peerIdentifier]
	if exist {
		peer.RemoteAddr = peer.PeerAddr
	}
}

func (n *Node) runServer() {
	for {
		conn, err := n.server.Accept(context.Background())
		if err != nil {
			fmt.Printf("runServer: Server accept error: %v\n", err)
			continue
		}
		go n.handleConnection(conn)
	}
}

func (n *Node) handleConnection(conn quic.Connection) {
	peerAddr := conn.RemoteAddr().String()

	//fmt.Printf("[N%v] handleConnection. peerAddr=%v\n", n.id, peerAddr)

	n.connectionMu.Lock()
	n.connections[peerAddr] = conn
	n.connectionMu.Unlock()

	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			if quicErr, ok := err.(*quic.ApplicationError); ok && quicErr.ErrorCode == 0 {
				continue
			}
			fmt.Printf("handleConnection: Accept stream error: %v\n", err)
		}
		//fmt.Printf("[N%v] AcceptStream. peerAddr=%v\n", n.id, peerAddr)
		n.handleStream(peerAddr, stream)
	}
}

func (n *Node) handleStream(peerAddr string, stream quic.Stream) {

	defer stream.Close()
	var lengthPrefix [4]byte
	_, err := io.ReadFull(stream, lengthPrefix[:])
	if err != nil {
		fmt.Printf("[N%v] handleStream: Read length prefix error: %v\n", n.id, err)
		return
	}
	messageLength := binary.BigEndian.Uint32(lengthPrefix[:])
	buf := make([]byte, messageLength)
	_, err = io.ReadFull(stream, buf)
	if err != nil {
		fmt.Printf("[N%v] handleStream: Read message error: %v\n", n.id, err)
		return
	}

	var msg QuicMessage
	err = json.Unmarshal(buf, &msg)
	if err != nil {
		fmt.Printf("Failed to unmarshal QuicMessage: %v\n", err)
		return
	}

	//fmt.Printf(" -- [N%v] handleStream Read From N%v (msgType=%v)\n", n.id, msg.Id, msg.MsgType)
	response := []byte("1")
	ok := []byte("0")
	switch msg.MsgType {
	case "BlockQuery":
		var query types.BlockQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err == nil {
			blk, found := n.blocks[query.BlockHash]
			fmt.Printf("[N%d] Received BlockQuery %v found: %v\n", n.id, query.BlockHash, found)
			if found {
				serializedR, err := json.Marshal(blk)
				if err == nil {
					fmt.Printf("[N%d] Responded to BlockQuery %v with: %s\n", n.id, query.BlockHash, serializedR)
					response = serializedR
				}
			}
		}

	case "ImportDAQuery":
		var query types.ImportDAQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err == nil {
			r := types.ImportDAResponse{Data: [][]byte{[]byte("dummy data")}}
			serializedR, err := json.Marshal(r)
			if err == nil {
				response = serializedR
			}
		}
	case "AuditDAQuery":
		var query types.AuditDAQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err == nil {
			r := types.AuditDAResponse{Data: []byte("dummy data")}
			serializedR, err := json.Marshal(r)
			if err == nil {
				response = serializedR
			}
		}
	case "ImportDAReconstructQuery":
		var query types.ImportDAReconstructQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err == nil {
			r := types.ImportDAReconstructResponse{Data: []byte("dummy data")}
			serializedR, err := json.Marshal(r)
			if err == nil {
				response = serializedR
			}
		}
	case "Ticket":
		var ticket *types.Ticket
		err := json.Unmarshal([]byte(msg.Payload), &ticket)
		if err == nil {
			err = n.processTicket(*ticket)
			if err == nil {
				response = ok
			}
		}
	case "AvailabilityJustification":
		var aj *types.AvailabilityJustification
		err := json.Unmarshal([]byte(msg.Payload), &aj)
		if err == nil {
			err = n.processAvailabilityJustification(aj)
			if err == nil {
				response = ok
			}
		}
	case "Guarantee":
		var guarantee types.Guarantee
		err := json.Unmarshal([]byte(msg.Payload), &guarantee)
		if err == nil {
			err = n.processGuarantee(guarantee)
			if err == nil {
				response = ok
			}
		}
	case "Assurance":
		var assurance types.Assurance
		err := json.Unmarshal([]byte(msg.Payload), &assurance)
		if err == nil {
			err = n.processAssurance(assurance)
			if err == nil {
				response = ok
			}
		}
	case "Dispute":
		var disputes types.Dispute
		err := json.Unmarshal([]byte(msg.Payload), &disputes)
		if err == nil {
			err = n.processDisputes(disputes)
			if err == nil {
				response = ok
			}
		}
	case "PreimageLookup":
		var preimageLookup types.PreimageLookup
		err := json.Unmarshal([]byte(msg.Payload), &preimageLookup)
		if err == nil {
			// err = n.processPreimageLookup(preimageLookup)
			err = n.processLookup(preimageLookup)
			if err == nil {
				response = ok
			}
		}
	case "Block":
		var block *types.Block
		err := json.Unmarshal([]byte(msg.Payload), &block)
		if err == nil {
			fmt.Printf(" -- [N%d] received block From N%d (%v <- %v)\n", n.id, msg.Id, block.ParentHash(), block.Hash())
			err = n.processBlock(block)
			if err == nil {
				response = ok
			}
		}
	case "Announcement":
		var announcement types.Announcement
		err := json.Unmarshal([]byte(msg.Payload), &announcement)
		if err == nil {
			err = n.processAnnouncement(announcement)
			if err == nil {
				response = ok
			}
		}
	case "WorkPackage":
		var workPackage types.WorkPackage
		err := json.Unmarshal([]byte(msg.Payload), &workPackage)
		if err == nil {
			err = n.processWorkPackage(workPackage)
			if err == nil {
				response = ok
			}
		}
	// -----Custom messages for tiny QUIC experiment-----

	case "DistributeECChunk":
		var chunk types.DistributeECChunk
		err := json.Unmarshal([]byte(msg.Payload), &chunk)
		if err == nil {
			err = n.processDistributeECChunk(chunk)
			if err == nil {
				response = ok
			}
		}

	// case "ECChunkResponse":
	// 	var chunk ECChunkResponse
	// 	err := json.Unmarshal([]byte(msg.Payload), &chunk)
	// 	if err == nil {
	// 		err = n.processECChunkResponse(chunk)
	// 		if err == nil {
	// 			response = ok
	// 		}
	// 	}

	case "ECChunkQuery":
		var query types.ECChunkQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err == nil {
			r, err := n.processECChunkQuery(query)
			if err == nil {
				serializedR, err := json.Marshal(r)
				if err == nil {
					response = serializedR
				}
			}
		}
	}

	_, err = stream.Write(response)
	if err != nil {
		log.Println(err)
	}
	//fmt.Printf("responded with: %s\n", string(response))
	//stream.Close()
}

func (n *Node) broadcast(obj interface{}) []byte {
	result := []byte{}
	for _, peer := range n.peersInfo {
		peerIdentifier := peer.Validator.Ed25519.String()
		if peer.PeerID == n.id {
			continue
		}
		//fmt.Printf("PeerID=%v, peerIdentifier=%v\n", peer.PeerID, peerIdentifier)
		resp, err := n.makeRequest(peerIdentifier, obj)
		if err != nil {
			fmt.Printf("runClient request error: %v\n", err)
			continue
		}
		if len(resp) > 0 {
			result = resp
		}
	}
	return result
}

func (n *Node) makeRequest(peerIdentifier string, obj interface{}) ([]byte, error) {
	peerAddr, _ := n.getPeerAddr(peerIdentifier)
	peerID, _ := n.getPeerIndex(peerIdentifier)
	msgType := getMessageType(obj)
	if msgType == "unknown" {
		return nil, fmt.Errorf("unsupported type")
	}
	//fmt.Printf("[N%v] makeRequest %v to [N%v]\n", n.id, msgType, peerID)
	n.connectionMu.Lock()
	conn, exists := n.connections[peerAddr]
	if !exists {
		var err error
		conn, err = quic.DialAddr(context.Background(), peerAddr, n.tlsConfig, generateQuicConfig())
		if err != nil {
			n.connectionMu.Unlock()
			fmt.Printf("-- [N%v] makeRequest ERR %v peerAddr=%s (N%v)\n", n.id, err, peerAddr, peerID)
			return nil, err
		}
		n.connections[peerAddr] = conn
		//fmt.Printf("NEWCONN %d => %s\n", n.id, peerAddr);
	}
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		n.connectionMu.Unlock()
		fmt.Printf("-- [N%v] openstreamsync ERR %v\n", n.id, err)
		return nil, err
	}

	n.connectionMu.Unlock()

	payload, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	quicMessage := QuicMessage{
		Id:      n.id,
		MsgType: msgType,
		Payload: string(payload),
	}

	messageData, err := json.Marshal(quicMessage)
	if err != nil {
		fmt.Printf("HMM2 %v\n", err)
		return nil, err
	}

	// Sanity check: Unmarshal messageData to verify it
	var sanityCheckMsg QuicMessage
	err = json.Unmarshal(messageData, &sanityCheckMsg)
	if err != nil {
		fmt.Printf("Sanity check failed: %v\n", err)
		return nil, err
	}

	// Length-prefix the message
	messageLength := uint32(len(messageData))
	lengthPrefix := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthPrefix, messageLength)

	_, err = stream.Write(lengthPrefix)
	if err != nil {
		fmt.Printf("-- [N%v] Write ERR %v\n", n.id, err)
		return nil, err
	}

	_, err = stream.Write(messageData)
	if err != nil {
		fmt.Printf("-- [N%v] Write ERR %v\n", n.id, err)
		return nil, err
	}

	//fmt.Printf(" -- [N%v] makeRequest WRITE %s => N%v\n", n.id, msgType, peerID)

	// Read the response from the stream with an expandable buffer
	var buffer bytes.Buffer
	tmp := make([]byte, 100000)
	for {
		nRead, err := stream.Read(tmp)
		buffer.Write(tmp[:nRead])
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			fmt.Printf("-- Read ERR %v\n", err)
			return nil, err
		}
	}
	//fmt.Printf("-- [N%v] makeRequest READ RESPONSE %d bytes from N%v\n", n.id, buffer.Len(), peerID)
	//fmt.Printf(" -- [N%v] makeRequest WRITE %s => N%v\n", n.id, msgType, peerID)
	stream.Close()
	return buffer.Bytes(), nil
}

// Helper function to determine if the error is a timeout error
func isTimeoutError(err error) bool {
	// Add more specific error handling as needed
	return strings.Contains(err.Error(), "timeout")
}

func (n *Node) processTicket(ticket types.Ticket) error {
	// Store the ticket in the tip's queued tickets
	s := n.getState()
	s.ProcessIncomingTicket(ticket)
	return nil // Success
}

func (n *Node) processLookup(preimageLookup types.PreimageLookup) error {
	// TODO: Store the lookup in a E_P aggregator
	s := n.getState()
	s.ProcessIncomingLookup(preimageLookup)
	return nil // Success
}

// Guarantees are sent by a validator working on a core receiving a work package and executing a refine operation
func (n *Node) processGuarantee(guarantee types.Guarantee) error {
	// Store the guarantee in the tip's queued guarantee
	s := n.getState()
	s.ProcessIncomingGuarantee(guarantee)
	return nil // Success
}

func (n *Node) processAssurance(assurance types.Assurance) error {
	// Store the assurance in the tip's queued assurance
	s := n.getState()
	s.ProcessIncomingAssurance(assurance)
	return nil // Success
}

func (n *Node) processDisputes(dispute types.Dispute) error {
	// Store the dispute in the tip's queued disputes
	s := n.getState()
	s.ProcessIncomingDispute(dispute)
	return nil
}

func (n *Node) dumpstatedbmap() {
	for hash, statedb := range n.statedbMap {
		fmt.Printf("dumpstatedbmap: statedbMap[%v] => statedb (%v<=parent=%v) StateRoot %v\n", hash, statedb.ParentHash, statedb.BlockHash, statedb.StateRoot)
	}
	for hash, blk := range n.blocks {
		fmt.Printf("dumpstatedbmap: blocks[%v] => %v\n", hash, blk.Hash())
	}
}

func randomKey(m map[string]NodeInfo) string {
	rand0.Seed(time.Now().UnixNano())
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys[rand0.Intn(len(keys))]
}

func (n *Node) fetchBlock(blockHash common.Hash) (*types.Block, error) {
	obj := types.BlockQuery{BlockHash: blockHash}

	randomPeer := randomKey(n.peersInfo)
	peer := n.peersInfo[randomPeer]
	peerIdentifier := peer.Validator.Ed25519.String()
	resp, err := n.makeRequest(peerIdentifier, obj)
	if err != nil {
		fmt.Printf("runClient request error: %v\n", err)
		return nil, nil
	}
	if len(resp) > 10 {
		blk, err := types.BlockFromBytes(resp)
		if err != nil {
			return nil, err
		}
		fmt.Printf("[N%d] fetchBlock(%v) %v<-%v [from %s]: %s\n", n.id, blockHash, blk.ParentHash(), blk.Hash(), randomPeer, blk.String())
		n.blocks[blk.Hash()] = blk
		return blk, nil
	}
	return nil, fmt.Errorf("fetchBlock - No response")
}

// given n.blocks, 1 or more of which can extend the tip, we advance the chain
func (n *Node) extendChain() error {
	parenthash := n.statedb.BlockHash
	for {

		ok := false
		for _, b := range n.blocks {
			if b.ParentHash() == parenthash {
				ok = true
				nextBlock := b
				fmt.Printf("[N%d] extendChain %v <- %v\n", n.id, parenthash, nextBlock.Hash())
				// now APPLY the block to the tip
				newStateDB, err := statedb.ApplyStateTransitionFromBlock(n.statedb, context.Background(), nextBlock)
				if err != nil {
					fmt.Printf("[N%d] extendChain FAIL %v\n", n.id, err)
					return err
				}
				// EXTEND the tip of the chain
				n.addStateDB(newStateDB)
				parenthash = nextBlock.Hash()
				fmt.Printf("[N%d] extendChain addstatedb TIP Now: s:%v<-%v\n",
					n.id, newStateDB.ParentHash, newStateDB.BlockHash)
				break
			}
		}

		if ok == false {
			// if there is no next block, we're done!
			fmt.Printf("[N%d] extendChain NO further next block %v\n", n.id, parenthash)
			return nil
		}
	}
}

// we arrive here when we receive a block from another node
func (n *Node) processBlock(blk *types.Block) error {

	// walk blk backwards, up to the tip, if possible -- but if encountering an unknown parenthash, immediately fetch the block.  Give up if we can't do anything
	b := blk
	n.blocks[b.Hash()] = blk
	for {
		if b.ParentHash() == (common.Hash{}) {
			//fmt.Printf("[N%d] processBlock: hit genesis (%v <- %v)\n", n.id, b.ParentHash(), b.Hash())
			break
		} else if n.statedb != nil && b.ParentHash() == n.statedb.BlockHash {
			fmt.Printf("[N%d] processBlock: hit TIP (%v <- %v)\n", n.id, b.ParentHash(), b.Hash())
			break
		} else {
			var err error
			parentBlock, ok := n.blocks[b.ParentHash()]
			if !ok {
				parentBlock, err = n.fetchBlock(b.ParentHash())
				if err != nil {
					// have to give up right now (could try again though!)
					return err
				}
				// got the parent block, store it in the cache
				if parentBlock.Hash() == blk.ParentHash() {
					fmt.Printf("[N%d] fetchBlock (%v<-%v) Validated --- CACHING\n", n.id, blk.ParentHash(), blk.Hash())
					n.blocks[parentBlock.Hash()] = parentBlock
				} else {
					return nil
				}

			}
			b = parentBlock
		}
	}

	// we got to the tip, now extend the chain, moving the tip forward, applying blocks using blockcache
	n.extendChain()
	return nil // Success
}

func (n *Node) processAnnouncement(announcement types.Announcement) error {
	// TODO: TBD
	return nil // Success
}

func (n *Node) computeAssuranceBitstring() []byte {
	// TODO
	return []byte{1, 1}
}

/*
M_B: WBT
M : CDT

C : Erasure-encoding fn

b : the E(p,x,i,j)

Data availability specifi- cation of the package (s)
s : A(H(p),E(p,x,i,j),Ìe)

H(p): packageHash
p: package
w: workItem
x = [X(w)∣w <−pw]  "import segment data"
i = [M(w)∣w <−pw]  "the extrinsic data"
j = Merkle paths for the justifications
s : all segments exported by all work-packages exporting a segment to be imported

Ìe: sequence of results for each of the work-items

WC: 684 (full) / (2) tiny
WS: 6 	(full) / (6) tiny

#: a function mapping over all items of a sequence (eq.11) [f(x0),f(x1),...] = f#([x0,x1,...])

A: {H, Y, [G]} → S

	(h, b, s) ↦ (h, l: |b|, u, e: M(s))

where u = M_B([x̂ | x ∈ T[𝓫•, s•]])

and 𝓫• = H#(C⟦|b|/W_c⟧(𝓟_wc(b)))  𝓟 here is Padding

and s• = M#_B(T⟦C#_6⟧(s ⌢ P(s)))  P here is Page-Proof
*/
func (n *Node) newAvailabilityJustification(guarantee types.Guarantee) types.AvailabilityJustification {
	return types.AvailabilityJustification{}
}

func (n *Node) processAvailabilityJustification(aj *types.AvailabilityJustification) error {
	// TODO: validate proof

	assurance := types.Assurance{
		ParentHash:     n.statedb.ParentHash,
		Bitstring:      n.computeAssuranceBitstring(),
		ValidatorIndex: n.id,
		//	Signature: signature,
	}
	assurance.Sign(n.credential.Ed25519Secret, n.statedb.ParentHash)

	n.broadcast(assurance)
	return nil
}

func (n *Node) getImportSegment(segmentRoot common.Hash, segmentIndex uint32) ([]byte, error) {
	// TODO
	return []byte{}, nil
}

func (n *Node) getImportSegments(importsegments []types.ImportSegment) ([][]byte, error) {
	var imports [][]byte
	for _, s := range importsegments {
		importItem, err := n.getImportSegment(s.SegmentRoot, s.SegmentIndex)
		if err != nil {
			return imports, err
		}
		imports = append(imports, importItem)
	}
	return imports, nil
}

func (n *Node) getPVMStateDB() *statedb.StateDB {
	// TODO: processWorkPackage should provide clear signal on which stateDB to work with. What is this signal?
	// I don't think it should be the tip. workPackage probably requires somekind of blkhash or stateRoot?
	target_statedb := n.statedb.Copy() // stub. need to figure out what to do here
	return target_statedb
}

func (n *Node) processWorkPackage(workPackage types.WorkPackage) error {

	// Create a new PVM instance with mock code and execute it
	results := []types.WorkResult{}
	targetStateDB := n.getPVMStateDB()

	for _, workItem := range workPackage.WorkItems {
		code, err := n.FetchAndReconstructData(workItem.CodeHash)
		if err != nil {
			return err
		}
		// TODO: statedb should follow HostEnv
		vm := pvm.NewVMFromCode(code, 0, targetStateDB)
		imports, err := n.getImportSegments(workItem.ImportedSegments)
		if err != nil {
			return err
		}
		vm.SetImports(imports)
		vm.SetExtrinsicsPayload(workItem.Extrinsics, workItem.PayloadBlob)
		err = vm.Execute(types.EntryPointRefine)
		if err != nil {
			return err
		}

		leaves := [][]byte{}
		for _, e := range vm.Exports {
			h, err := n.EncodeAndDistributeData(e)
			if err != nil {
				return err
			}
			leaves = append(leaves, h.Bytes()) // h is the H(e) .. assuming e is wc*ws
			// 11.1.4. Work Result. Equation 121. We finally come to define a work result, L, which is the data conduit by which services’ states may be altered through the computation done within a work-package.
			/*
				result := types.WorkResult{
					Service:                workItem.ServiceIdentifier,
					CodeHash:               workItem.CodeHash,
					PayloadHash:            common.Blake2Hash(workItem.PayloadBlob),
					GasPrioritizationRatio: 0,
					Output:                 []byte{}, // refinement_context
					Error:                  "",
				}
			*/
			//results = append(results, result)
			//fmt.Printf("[N%d] EXPORTED ITEM %d => encoded in DA %v", n.id, e, h)
		}

		tree := trie.NewCDMerkleTree(leaves)

		// Test justification for each leaf
		for i, leaf := range leaves {
			justification, err := tree.Justify(i)
			if err != nil {
				return fmt.Errorf("Error justifying leaf %d: %v\n", i, err)
			}
			leafHash := trie.ComputeLeaf(leaf)
			computedRoot := trie.VerifyJustification(leafHash, i, justification)
			if !common.CompareBytes(computedRoot, tree.Root()) {
				return fmt.Errorf("Justification failed for leaf %d: expected root %s, got %s\n", i, tree.RootHash(), common.Hash(computedRoot))
			} else {
				fmt.Printf("Justification verified for leaf %d\n", i)
			}
		}

	}

	// Now create a WorkReport with AvailabilitySpecification and RefinementContext
	availabilitySpec := types.AvailabilitySpecification{
		WorkPackageHash: workPackage.Hash(),
		BundleLength:    10,                           // ???
		ErasureRoot:     common.HexToHash("0xdef456"), // TODO
		SegmentRoot:     common.HexToHash("0x789abc"), // TODO
	}

	refinementContext := types.RefinementContext{
		Anchor:             common.HexToHash("0x123abc"),          // TODO
		PosteriorStateRoot: n.statedb.Block.Header.PriorStateRoot, // TODO
		PosteriorBeefyRoot: common.HexToHash("0x"),                // SKIP
		LookupAnchor:       n.statedb.ParentHash,                  // TODO
		HeaderHash:         n.statedb.ParentHash,                  // TODO
		Timeslot:           n.statedb.Block.Header.TimeSlot,
		Prerequisite:       common.HexToHash("0x"), // SKIP
	}

	workReport := types.WorkReport{
		AvailabilitySpec: availabilitySpec,
		AuthorizerHash:   common.HexToHash("0x"), // SKIP
		Core:             n.coreIndex,
		//	Output:               result.Output,
		RefinementContext:    refinementContext,
		PackageSpecification: "mock_package_specification",
		Results:              results,
	}

	// Create a GuaranteeCredential with the signature and a mock validator index
	credential := types.GuaranteeCredential{
		ValidatorIndex: n.id, // Mock validator index
		// Sign the serialized WorkReport
		Signature: workReport.Sign(n.credential.Ed25519Secret),
	}

	// Create a Guarantee with the WorkReport, TimeSlot, and Credentials
	guarantee := types.Guarantee{
		WorkReport:  workReport,
		TimeSlot:    n.statedb.Block.TimeSlot(),
		Credentials: []types.GuaranteeCredential{credential},
	}
	// This will be received by all validators
	n.broadcast(guarantee)

	// This will be received by all validators
	aj := n.newAvailabilityJustification(guarantee)
	n.broadcast(aj)

	return nil
}

// -----Custom methods for tiny QUIC EC experiment-----

func (n *Node) processDistributeECChunk(chunk types.DistributeECChunk) error {
	// Serialize the chunk
	serialized, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	// Save the chunk to the local storage
	key := fmt.Sprintf("DA-%d", chunk.SegmentRoot)
	err = n.store.WriteRawKV([]byte(key), serialized)
	if err != nil {
		return err
	}
	return nil
}

func (n *Node) processECChunkQuery(ecChunkQuery types.ECChunkQuery) (types.ECChunkResponse, error) {
	key := fmt.Sprintf("DA-%d", ecChunkQuery.SegmentRoot)
	// fmt.Printf("[N%v] processECChunkQuery key=%s\n", n.id, key)
	data, err := n.store.ReadRawK([]byte(key))
	if err != nil {
		return types.ECChunkResponse{}, err
	}
	// Deserialize the chunk
	var chunk types.ECChunkResponse
	err = json.Unmarshal(data, &chunk)
	if err != nil {
		return types.ECChunkResponse{}, err
	}
	return chunk, nil
}

func (n *Node) encode(data []byte) ([][][]byte, error) {
	// Load the file and encode them into segments of chunks. (3D byte array)

	// encode the data
	encoded_data, err := erasurecoding.Encode(data)
	if err != nil {
		return nil, err
	}

	// return the encoded data
	return encoded_data, nil
}

func (n *Node) packChunks(segments [][][]byte, segmentRoots [][]byte) ([]types.DistributeECChunk, error) {
	// TODO: Pack the chunks into DistributeECChunk objects
	chunks := make([]types.DistributeECChunk, 0)
	for i := range segments {
		for j := range segments[i] {
			// Pack the DistributeECChunk object
			// TODO: Modify this as needed.
			chunk := types.DistributeECChunk{
				SegmentRoot: segmentRoots[i],
				Data:        segments[i][j],
			}
			chunks = append(chunks, chunk)
		}
	}
	// Return the DistributeECChunk objects
	return chunks, nil
}

func encodeBlobMeta(segmentRootsFlattened []byte, originalLength int) []byte {
	// Serialize the original byte length as a 4-byte slice
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(originalLength))

	// Combine the length bytes with the segment roots
	encodedBytes := append(lengthBytes, segmentRootsFlattened...)
	return encodedBytes
}

func decodeBlobMeta(value []byte) (int, []byte, error) {
	if len(value) < 4 {
		return 0, nil, fmt.Errorf("value is too short to contain a valid length")
	}

	// Extract the first 4 bytes to get the original length
	originalLength := int(binary.BigEndian.Uint32(value[:4]))

	// The rest of the value is the segment roots
	segmentRootsFlattened := value[4:]

	return originalLength, segmentRootsFlattened, nil
}

func (n *Node) EncodeAndDistributeData(data []byte) (common.Hash, error) {
	// Load the file and encode them into segments of chunks
	segments, err := n.encode(data)
	if err != nil {
		return common.Hash{}, err
	}

	// Build segment roots
	segmentRoots := make([][]byte, 0)
	for i := range segments {
		leaves := segments[i]
		tree := trie.NewCDMerkleTree(leaves)
		segmentRoots = append(segmentRoots, tree.Root())
	}

	blob_hash := bhash(data)
	segmentRootsFlattened := make([]byte, 0)
	for i := range segmentRoots {
		segmentRootsFlattened = append(segmentRootsFlattened, segmentRoots[i]...)
	}
	/// storer needs to know the size of original byte in order to eliminate any segment padding
	blob_meta := encodeBlobMeta(segmentRootsFlattened, len(data))
	n.store.WriteKV(blob_hash, blob_meta)

	// Pack the chunks into DistributeECChunk objects
	ecChunks, err := n.packChunks(segments, segmentRoots)
	if err != nil {
		return common.Hash{}, err
	}

	// Send the DistributeECChunk object to a random K peers
	for i, ecChunk := range ecChunks {

		peerIdx := uint32(i % numNodes)

		if peerIdx == n.id {
			n.processDistributeECChunk(ecChunk)
		}

		peerIdentifier, err := n.getPeerByIndex(peerIdx)
		if err != nil {
			return common.Hash{}, err
		}
		// fmt.Println("making request from node", 0, "to", peerIdentifier)
		response, err := n.makeRequest(peerIdentifier, ecChunk)
		if err != nil {
			fmt.Printf("Failed to make request from node %d to %s: %v", 0, peerIdentifier, err)
			continue
		}
		_ = response

		// Wait for nodes to process the request
		// time.Sleep(100 * time.Millisecond)
	}

	return blob_hash, nil
}

func (n *Node) FetchAndReconstructData(blob_hash common.Hash) ([]byte, error) {
	blob_meta, err := n.store.ReadKV(blob_hash)
	if err != nil {
		return nil, err
	}
	originalLength, segmentRootsFlattened, _ := decodeBlobMeta(blob_meta)
	segmentRoots := make([][]byte, 0)
	for i := 0; i < len(segmentRootsFlattened); i += 32 {
		segmentRoots = append(segmentRoots, segmentRootsFlattened[i:i+32])
	}

	// Fetch the chunks from peers
	decoderInputSegments := make([][][]byte, 0)
	for i, segmentRoot := range segmentRoots {
		_ = i
		ecChunkResponses := make([]types.ECChunkResponse, 0)
		fetchedChunks := 0
		for j := 0; j < numNodes; j++ {
			peerIdentifier, err := n.getPeerByIndex(uint32(j))
			if err != nil {
				return nil, err
			}
			ecChunkQuery := types.ECChunkQuery{
				SegmentRoot: segmentRoot,
			}
			/*
				if peerIdentifier == n.getIdentifier() {
					// Fetch the chunk from local storage
					ecChunkResponse, err := n.processECChunkQuery(ecChunkQuery)
					if err != nil {
						return nil, err
					}

					ecChunkResponses = append(ecChunkResponses, ecChunkResponse)

					fetchedChunks++
					if fetchedChunks >= erasurecoding.K {
						break
					}
					continue
				}
			*/
			response, err := n.makeRequest(peerIdentifier, ecChunkQuery)
			if err != nil {
				fmt.Printf("Failed to make request from node %d to %s: %v", 0, peerIdentifier, err)
				ecChunkResponses = append(ecChunkResponses, types.ECChunkResponse{})
			}
			var ecChunkResponse types.ECChunkResponse
			err = json.Unmarshal(response, &ecChunkResponse)
			if err != nil {
				return nil, err
			}
			ecChunkResponses = append(ecChunkResponses, ecChunkResponse)

			fetchedChunks++
			if fetchedChunks >= erasurecoding.K {
				break
			}
		}

		// debug
		fmt.Printf("Fetched %d chunks\n", fetchedChunks)
		for j := 0; j < fetchedChunks; j++ {
			fmt.Printf("Chunk %d: %x\n", j, ecChunkResponses[j])
		}

		// build decoder input
		_ = decoderInputSegments
		decoderInputSegment := make([][]byte, 0)
		for j := 0; j < erasurecoding.N; j++ {
			if j >= len(ecChunkResponses) {
				decoderInputSegment = append(decoderInputSegment, nil)
				continue
			}
			if len(ecChunkResponses[j].Data) == 0 {
				decoderInputSegment = append(decoderInputSegment, nil)
				continue
			}
			decoderInputSegment = append(decoderInputSegment, ecChunkResponses[j].Data)
		}
		decoderInputSegments = append(decoderInputSegments, decoderInputSegment)
	}

	// Reconstruct the data
	reconstructedData, err := erasurecoding.Decode(decoderInputSegments)
	if err != nil {
		return nil, err
	}

	// Trim any padding using the original length
	if len(reconstructedData) > originalLength {
		reconstructedData = reconstructedData[:originalLength]
	}

	return reconstructedData, nil
}

func getMessageType(obj interface{}) string {

	switch obj.(type) {
	case types.AvailabilityJustification:
		return "AvailabilityJustification"
	case types.ImportDAQuery:
		return "ImportDAQuery"
	case types.BlockQuery:
		return "BlockQuery"
	case types.AuditDAQuery:
		return "AuditDAQuery"
	case types.ImportDAReconstructQuery:
		return "ImportDAReconstructQuery"
	case types.Guarantee:
		return "Guarantee"
	case types.Assurance:
		return "Assurance"
	case types.Dispute:
		return "Dispute"
	case types.PreimageLookup:
		return "PreimageLookup"
	case types.Ticket:
		return "Ticket"
	case types.Block:
		return "Block"
	case types.Announcement:
		return "Announcement"
	case types.WorkPackage:
		return "WorkPackage"
	case types.DistributeECChunk:
		return "DistributeECChunk"
	case types.ECChunkQuery:
		return "ECChunkQuery"
	default:
		return "unknown"
	}
}

const TickTime = 2000

func (n *Node) runClient() {

	ticker_pulse := time.NewTicker(TickTime * time.Millisecond)
	defer ticker_pulse.Stop()

	for {
		select {
		case <-ticker_pulse.C:
			if n.GetNodeType() != ValidatorFlag && n.GetNodeType() != ValidatorDAFlag {
				return
			}
			newBlock, newStateDB, err := n.statedb.ProcessState(n.credential)
			if err != nil {
				fmt.Printf("[N%d] ProcessState ERROR: %v\n", n.id, err)
				panic(0)
			}
			if newStateDB != nil {
				// we authored a block
				n.addStateDB(newStateDB)
				n.blocks[newBlock.Hash()] = newBlock
				n.broadcast(*newBlock)
				fmt.Printf("[N%d] BLOCK BROADCASTED: %v <- %v\n", n.id, newBlock.ParentHash(), newBlock.Hash())
			}
		case msg := <-n.messageChan:
			n.processOutgoingMessage(msg)
		}
	}
}

func (n *Node) processOutgoingMessage(msg statedb.Message) {
	msgType := msg.MsgType

	// Unmarshal the payload to the appropriate type
	switch msgType {
	case "Ticket":
		var ticket types.Ticket
		payloadBytes, err := json.Marshal(msg.Payload)
		if err != nil {
			fmt.Printf("Error marshaling payload to JSON: %v\n", err)
			return
		}
		err = json.Unmarshal(payloadBytes, &ticket)
		if err != nil {
			fmt.Printf("Error unmarshaling Ticket: %v\n", err)
			return
		}
		//fmt.Printf("[N%v] Outgoing Ticket: %+v\n", n.id, ticket.TicketID())
		n.broadcast(ticket)
	case "Block":
		var blk types.Block
		payloadBytes, err := json.Marshal(msg.Payload)
		if err != nil {
			fmt.Printf("Error marshaling payload to JSON: %v\n", err)
			return
		}
		err = json.Unmarshal(payloadBytes, &blk)
		if err != nil {
			fmt.Printf("Error unmarshaling Ticket: %v\n", err)
			return
		}
		//fmt.Printf("[N%v] Outgoing Block: %+v\n", n.id, blk.Hash())
		n.broadcast(blk)
	default:
		fmt.Printf("[N%v] Unhandled message type: %v\n", n.id, msg.MsgType)
	}
}

func generateObject() interface{} {
	return nil
}
