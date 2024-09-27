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
	"encoding/pem"
	"fmt"
	"math"
	"reflect"

	"github.com/colorfulnotion/jam/common"
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
)

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
	//Payload string `json:"payload"`
	Payload []byte `json:"payload"`
}

type NodeInfo struct {
	PeerID     uint32          `json:"peer_id"`
	PeerAddr   string          `json:"peer_addr"`
	RemoteAddr string          `json:"remote_addr"`
	Validator  types.Validator `json:"validator"`
}

type Node struct {
	id        uint32
	coreIndex uint16

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
	// this is for audit
	announcementBucket     types.AnnounceBucket
	prevAnnouncementBucket types.AnnounceBucket
	announcementMutex      sync.Mutex
	judgementBucket        types.JudgeBucket
	judgementMutex         sync.Mutex
	// use validator index to lookup the guarantee from the validator in their core
	guaranteeBucket map[common.Hash][]types.GuaranteeReport
	guaranteeMutex  sync.Mutex
	isBadGuarantor  bool
	// this is for assurances
	// use work package hash to lookup the availbility
	assurancesBucket map[common.Hash]types.IsPackageRecieved
	assuranceMutex   sync.Mutex
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

	entropy := common.Blake2Hash([]byte("42"))

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
		ringSet[i] = common.Blake2Hash(append(seed[:], byte(i))).Bytes()
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
		jsonData := types.Encode(credential)
		fmt.Printf("[N%v] credential %s\n", n.id, jsonData)
	}
}

func NewNode(nodeName string, credential types.ValidatorSecret, genesisConfig *statedb.GenesisConfig, peers []string, peerList map[string]NodeInfo) (*Node, error) {
	n, err := newNode(0, credential, genesisConfig, peers, peerList, ValidatorFlag)
	return n, err
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
		coreIndex:   uint16(id % 2), // TODO: NumCores
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
	go node.runWebService(id)

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

// use ed25519 key to get peer info
func (n *Node) GetPeerInfoByEd25519(key types.Ed25519Key) (NodeInfo, error) {
	for _, peer := range n.peersInfo {
		if peer.Validator.Ed25519 == key {
			return peer, nil
		}
	}
	return NodeInfo{}, fmt.Errorf("peer not found")
}

func (n *Node) GetBandersnatchSecret() []byte {
	return n.credential.BandersnatchSecret
}
func (n *Node) GetSelfCoreIndex() (uint16, error) {
	for _, assignment := range n.statedb.GuarantorAssignments {
		if assignment.Validator.GetEd25519Key() == n.GetEd25519Key() {
			return assignment.CoreIndex, nil
		}
	}
	return 0, fmt.Errorf("core index not found")
}
func (n *Node) GetCoreCoWorkers(coreIndex uint16) []types.Validator {
	coWorkers := make([]types.Validator, 0)
	for _, assignment := range n.statedb.GuarantorAssignments {
		if types.Ed25519Key(n.credential.Ed25519Pub) == assignment.Validator.Ed25519 {
			continue
		}
		if assignment.CoreIndex == coreIndex {
			coWorkers = append(coWorkers, assignment.Validator)
		}
	}
	return coWorkers
}
func (n *Node) GetCurrValidatorIndex() uint32 {
	return uint32(n.statedb.GetSafrole().GetCurrValidatorIndex(n.GetEd25519Key()))
}

func (n *Node) generateEpochTickets(epoch uint32, isNextEpoch bool) {
	sf := n.statedb.GetSafrole()
	auth_secret, _ := sf.ConvertBanderSnatchSecret(n.GetBandersnatchSecret())
	tickets := sf.GenerateTickets(auth_secret, isNextEpoch)
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
			return fmt.Sprintf("%x", peer.Validator.Ed25519), nil
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

func (n *Node) GetEd25519Key() types.Ed25519Key {
	return types.Ed25519Key(n.credential.Ed25519Pub)
}

func (n *Node) GetEd25519Secret() []byte {
	return n.credential.Ed25519Secret
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
	decoded, _ := types.Decode(buf, reflect.TypeOf(msg))
	msg = decoded.(QuicMessage)

	//fmt.Printf(" -- [N%v] handleStream Read From N%v (msgType=%v)\n", n.id, msg.Id, msg.MsgType)
	response := []byte("1")
	ok := []byte("0")
	switch msg.MsgType {
	case "BlockQuery":
		var query types.BlockQuery
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(query))
		query = decoded.(types.BlockQuery)
		blk, found := n.blocks[query.BlockHash]
		fmt.Printf("[N%d] Received BlockQuery %v found: %v\n", n.id, query.BlockHash, found)
		if found {
			serializedR := types.Encode(blk)
			//serializedR := types.Encode(blk)
			if err == nil {
				fmt.Printf("[N%d] Responded to BlockQuery %v with: %s\n", n.id, query.BlockHash, serializedR)
				response = serializedR
			}
		}

	// case "ImportDAQuery":
	// 	var query types.ImportDAQuery
	// 	decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(query))
	// 	query = decoded.(types.ImportDAQuery)
	// 	r := types.ImportDAResponse{Data: [][]byte{[]byte("dummy data")}}
	// 	serializedR := types.Encode(r)
	// 	response = serializedR
	// case "AuditDAQuery":
	// 	var query types.AuditDAQuery
	// 	decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(query))
	// 	query = decoded.(types.AuditDAQuery)
	// 	r := types.AuditDAResponse{Data: []byte("dummy data")}
	// 	serializedR := types.Encode(r)
	// 	response = serializedR
	// case "ImportDAReconstructQuery":
	// 	var query types.ImportDAReconstructQuery
	// 	decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(query))
	// 	query = decoded.(types.ImportDAReconstructQuery)
	// 	if err == nil {
	// 		r := types.ImportDAReconstructResponse{Data: []byte("dummy data")}
	// 		serializedR := types.Encode(r)
	// 		response = serializedR
	// 	}
	case "Ticket":
		var ticket *types.Ticket
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(ticket))
		ticket = decoded.(*types.Ticket)
		err = n.processTicket(*ticket)
		if err == nil {
			response = ok
		}
	case "AvailabilityJustification":
		var aj *types.AvailabilityJustification
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(aj))
		aj = decoded.(*types.AvailabilityJustification)
		if err == nil {
			err = n.processAvailabilityJustification(aj)
			if err == nil {
				response = ok
			}
		}
	case "Guarantee":
		var guarantee types.Guarantee
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(guarantee))
		guarantee = decoded.(types.Guarantee)
		if err == nil {
			err = n.processGuarantee(guarantee)
			if err == nil {
				response = ok
			}
		}
		fmt.Printf(" -- [N%d] received guarantee From N%d\n", n.id, msg.Id)
	case "Assurance":
		var assurance types.Assurance
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(assurance))
		assurance = decoded.(types.Assurance)
		if err == nil {
			err = n.processAssurance(assurance)
			if err == nil {
				response = ok
			}
		}
		fmt.Printf(" -- [N%d] received assurance From N%d\n", n.id, msg.Id)
	case "Judgement":
		var judgement types.Judgement
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(judgement))
		judgement = decoded.(types.Judgement)
		if err == nil {
			err = n.processJudgement(judgement)
			if err == nil {
				response = ok
			} else {
				fmt.Println(err.Error())
			}
			fmt.Printf(" -- [N%d] received judgement From N%d\n", n.id, msg.Id)
			fmt.Printf(" -- [N%d] received judgement From N%d (%v <- %v)\n", n.id, msg.Id, judgement.WorkReport.GetWorkPackageHash(), judgement.WorkReport.GetWorkPackageHash())

		}
	case "Announcement":
		var announcement types.Announcement
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(announcement))
		announcement = decoded.(types.Announcement)
		err = n.processAnnouncement(announcement)
		if err == nil {
			response = ok
		}
		fmt.Printf(" -- [N%d] received announcement From N%d\n", n.id, msg.Id)
		fmt.Printf(" -- [N%d] received announcement From N%d (%v <- %v)\n", n.id, msg.Id, announcement.WorkReport.GetWorkPackageHash(), announcement.WorkReport.GetWorkPackageHash())
	case "Preimages":
		var preimages types.Preimages
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(preimages))
		preimages = decoded.(types.Preimages)
		if err == nil {
			// err = n.processPreimageLookup(preimageLookup)
			err = n.processLookup(preimages)
			if err == nil {
				response = ok
			}
		}
	case "Block":
		block, err := types.BlockFromBytes(msg.Payload)
		//err := interface{}
		if err == nil {
			fmt.Printf(" -- [N%d] received block From N%d (%v <- %v)\n", n.id, msg.Id, block.ParentHash(), block.Hash())
			err = n.processBlock(block)
			if err == nil {
				response = ok
			}
		}

	case "WorkPackage":
		var workPackage types.WorkPackage
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(workPackage))
		workPackage = decoded.(types.WorkPackage)
		var work types.GuaranteeReport
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			work, _, _, err = n.processWorkPackage(workPackage)
			if err != nil {
				fmt.Printf(" -- [N%d] WorkPackage Error: %v\n", n.id, err)
			}
		}()
		wg.Wait()
		if n.isBadGuarantor {
			fmt.Printf(" -- [N%d] Is a Bad Guarantor\n", n.id)
			work.Report.Results[0].Result.Ok = []byte("I am Culprits><")
			work.Sign(n.GetEd25519Secret())
		}
		_ = n.processGuaranteeReport(work)
		if err == nil {
			response = ok
		}
		n.coreBroadcast(work)
		// }
		fmt.Printf(" -- [N%d] received WorkPackage From N%d\n", n.id, msg.Id)
	case "GuaranteeReport":
		var guaranteeReport types.GuaranteeReport
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(guaranteeReport))
		guaranteeReport = decoded.(types.GuaranteeReport)
		err = n.processGuaranteeReport(guaranteeReport)
		if err == nil {
			response = ok
		}
		fmt.Printf(" -- [N%d] received GuaranteeReport From N%d\n", n.id, msg.Id)

	// -----Custom messages for tiny QUIC experiment-----

	case "DistributeECChunk":
		var chunk types.DistributeECChunk
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(chunk))
		chunk = decoded.(types.DistributeECChunk)
		err = n.processDistributeECChunk(chunk)
		if err == nil {
			response = ok
		}
		fmt.Printf(" -- [N%d] received DistributeECChunk From N%d\n", n.id, msg.Id)

	case "ECChunkQuery":
		var query types.ECChunkQuery
		decoded, _ := types.Decode([]byte(msg.Payload), reflect.TypeOf(query))
		query = decoded.(types.ECChunkQuery)
		r, err := n.processECChunkQuery(query)
		if err == nil {
			serializedR := types.Encode(r)
			response = serializedR
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
		peerIdentifier := fmt.Sprintf("%x", peer.Validator.Ed25519)
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

func (n *Node) coreBroadcast(obj interface{}) []byte {
	result := []byte{}
	core, err := n.GetSelfCoreIndex()
	if err != nil {
		fmt.Printf("coreBroadcast Error: %v\n", err)
		return nil
	}
	coworker := n.GetCoreCoWorkers(core)
	for _, peer := range n.peersInfo {
		for _, worker := range coworker {
			if worker.Ed25519 == peer.Validator.Ed25519 {
				peerIdentifier := fmt.Sprintf("%x", peer.Validator.Ed25519)
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

	payload := types.Encode(obj)

	quicMessage := QuicMessage{
		Id:      n.id,
		MsgType: msgType,
		Payload: payload,
	}

	messageData := types.Encode(quicMessage)

	// Sanity check: Unmarshal messageData to verify it
	// var sanityCheckMsg QuicMessage
	// decoded, _ := types.Decode(messageData, reflect.TypeOf(sanityCheckMsg))
	// sanityCheckMsg = decoded.(QuicMessage)

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
	tmp := make([]byte, 4096)
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

func (n *Node) processLookup(preimageLookup types.Preimages) error {
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
	if len(assurance.Signature) == 0 {
		return fmt.Errorf("No assurance signature")
	}
	s := n.getState()
	s.ProcessIncomingAssurance(assurance)
	return nil // Success
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
	peerIdentifier := fmt.Sprintf("%x", peer.Validator.Ed25519)
	resp, err := n.makeRequest(peerIdentifier, obj)
	if err != nil {
		fmt.Printf("runClient request error: %v\n", err)
		return nil, nil
	}
	if len(resp) > 10 {
		//this need to be consistent with the receiving side
		//blk, err := types.BlockFromBytes(resp)
		var blk *types.Block
		decoded, _ := types.Decode(resp, reflect.TypeOf(blk))
		blk = decoded.(*types.Block)
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

func (n *Node) computeAssuranceBitfield() [1]byte {
	// TODO
	return [1]byte{3}
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
	//ed25519Key := n.GetEd25519Key()
	ed25519Priv := n.GetEd25519Secret()
	assurance := types.Assurance{
		Anchor:         n.statedb.ParentHash,
		Bitfield:       n.computeAssuranceBitfield(),
		ValidatorIndex: uint16(n.id),
		//	Signature: signature,
	}
	assurance.Sign(ed25519Priv)

	n.broadcast(assurance)
	return nil
}

func (n *Node) getImportSegment(treeRoot common.Hash, segmentIndex uint16) ([]byte, error) {
	// TODO: do you need segmentRoot or segmentsRoot here?
	fmt.Printf("treeRoot: %v, segmentIndex: %v \n", treeRoot, segmentIndex)
	segmentData, err := n.FetchAndReconstructSpecificSegmentData(treeRoot)
	if err != nil {
		return []byte{}, err
	}
	return segmentData, nil
}
func (n *Node) GetImportSegments(importsegments []types.ImportSegment) ([][]byte, error) {
	return n.getImportSegments(importsegments)
}

func (n *Node) getImportSegments(importsegments []types.ImportSegment) ([][]byte, error) {
	var imports [][]byte
	for _, s := range importsegments {
		importItem, err := n.getImportSegment(s.TreeRoot, s.Index)
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

// -----Custom methods for tiny QUIC EC experiment-----

func (n *Node) processDistributeECChunk(chunk types.DistributeECChunk) error {
	// Serialize the chunk
	serialized := types.Encode(chunk)
	// Save the chunk to the local storage
	key := fmt.Sprintf("DA-%d", chunk.SegmentRoot)
	err := n.store.WriteRawKV([]byte(key), serialized)
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
	decoded, _ := types.Decode(data, reflect.TypeOf(chunk))
	chunk = decoded.(types.ECChunkResponse)
	return chunk, nil
}

func (n *Node) encode(data []byte, isFixed bool, data_len int) ([][][]byte, error) {
	// Load the file and encode them into segments of chunks. (3D byte array)
	c_base := 6
	if !isFixed {
		// get the veriable c_base by computing roundup(|b|/Wc)
		c_base = types.ComputeC_Base(int(data_len))
	}
	// encode the data
	encoded_data, err := erasurecoding.Encode(data, c_base)
	if err != nil {
		return nil, err
	}
	// return the encoded data
	return encoded_data, nil
}

func (n *Node) decode(data [][][]byte, isFixed bool, data_len int) ([]byte, error) {
	// Load the file and encode them into segments of chunks. (3D byte array)
	c_base := 6
	if !isFixed {
		// get the veriable c_base by computing roundup(|b|/Wc)
		c_base = types.ComputeC_Base(int(data_len))
	}
	// encode the data
	encoded_data, err := erasurecoding.Decode(data, c_base)
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

func (n *Node) EncodeAndDistributeSegmentData(data [][]byte, wg *sync.WaitGroup) (treeRoot common.Hash, err error) {
	defer wg.Done()

	basicTree := trie.NewCDMerkleTree(data)
	treeRoot = common.Hash(basicTree.Root())
	var segmentsECRoots []byte
	for _, singleData := range data {
		// Encode the data into segments
		erasureCodingSegments, err := n.encode(singleData, true, len(singleData)) // Set to false for variable size segments
		if err != nil {
			return common.Hash{}, err
		}

		// Build segment roots
		segmentRoots := make([][]byte, 0)
		for i := range erasureCodingSegments {
			leaves := erasureCodingSegments[i]
			tree := trie.NewCDMerkleTree(leaves)
			segmentRoots = append(segmentRoots, tree.Root())
		}

		// Generate the blob hash by hashing the original data
		blobTree := trie.NewCDMerkleTree(segmentRoots)
		segmentsECRoot := blobTree.Root()

		// Append the segment root to the list of segment roots
		segmentsECRoots = append(segmentsECRoots, segmentsECRoot...)

		// Flatten the segment roots
		segmentRootsFlattened := make([]byte, 0)
		for i := range segmentRoots {
			segmentRootsFlattened = append(segmentRootsFlattened, segmentRoots[i]...)
		}

		// Store the blob meta data, and return an error if it fails
		segment_meta := encodeBlobMeta(segmentRootsFlattened, len(data))
		err = n.store.WriteKV(common.Hash(segmentsECRoot), segment_meta)
		if err != nil {
			return common.Hash{}, err
		}

		// Pack the chunks into DistributeECChunk objects
		ecChunks, err := n.packChunks(erasureCodingSegments, segmentRoots)
		if err != nil {
			return common.Hash{}, err
		}

		// Distribute the chunks to the random K peers (K = numNodes)
		for i, ecChunk := range ecChunks {

			peerIdx := uint32(i % numNodes)

			if peerIdx == n.id {
				n.processDistributeECChunk(ecChunk)
			}

			peerIdentifier, err := n.getPeerByIndex(peerIdx)
			if err != nil {
				return common.Hash{}, err
			}
			response, err := n.makeRequest(peerIdentifier, ecChunk)
			if err != nil {
				fmt.Printf("Failed to make request from node %d to %s: %v", 0, peerIdentifier, err)
				continue
			}
			_ = response
		}
	}
	n.store.WriteKV(treeRoot, segmentsECRoots)
	// Return the blob hash if the process is successful
	return treeRoot, nil
}

func (n *Node) EncodeAndDistributeArbitraryData(blob []byte, blobLen int, wg *sync.WaitGroup) (blobHash common.Hash, err error) {
	// Defer the completion of the wait group
	defer wg.Done()

	// Load the file and encode them into segments of chunks
	segments, err := n.encode(blob, false, blobLen)
	if err != nil {
		return common.Hash{}, err
	}
	// if len(segments) != 1 {
	// 	panic("Expected only one segment")
	// }
	// fmt.Printf("Number of segments: %d for data size %d\n", len(segments), blobLen)

	// Build segment roots
	segmentRoots := make([][]byte, 0)
	for i := range segments {
		leaves := segments[i]
		tree := trie.NewCDMerkleTree(leaves)
		segmentRoots = append(segmentRoots, tree.Root())
	}

	blobHash = common.Blake2Hash(blob)
	segmentRootsFlattened := make([]byte, 0)
	for i := range segmentRoots {
		segmentRootsFlattened = append(segmentRootsFlattened, segmentRoots[i]...)
	}
	/// storer needs to know the size of original byte in order to eliminate any segment padding
	blob_meta := encodeBlobMeta(segmentRootsFlattened, len(blob))
	n.store.WriteKV(blobHash, blob_meta)

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

	return blobHash, nil
}

func (n *Node) FetchAndReconstructSegmentData(treeRoot common.Hash, segmentIndex uint32) ([]byte, error) {
	K, N := erasurecoding.GetCodingRate()
	fmt.Printf("Using K=%d and N=%d\n", K, N)

	// Get all segment EC roots from the tree root
	segmentsECRoots, err := n.store.ReadKV(treeRoot)
	if err != nil {
		fmt.Printf("Failed to read KV: %v\n", err)
		return nil, err
	}
	allHash := SplitBytesIntoHash(segmentsECRoots, len(common.Hash{}))

	// Get the segment EC root for the segment index
	segmentsECRoot := allHash[segmentIndex]
	segment_meta, err := n.store.ReadKV(segmentsECRoot)
	if err != nil {
		return nil, err
	}
	originalLength, segmentRootsFlattened, _ := decodeBlobMeta(segment_meta)
	erasurCodingSegmentRoots := make([][]byte, 0)
	for i := 0; i < len(segmentRootsFlattened); i += 32 {
		erasurCodingSegmentRoots = append(erasurCodingSegmentRoots, segmentRootsFlattened[i:i+32])
	}

	// Fetch the chunks from peers
	decoderInputSegments := make([][][]byte, 0)
	for i, erasurCodingSegmentRoot := range erasurCodingSegmentRoots {
		_ = i
		ecChunkResponses := make([]types.ECChunkResponse, 0)
		fetchedChunks := 0
		for j := 0; j < numNodes; j++ {
			peerIdentifier, err := n.getPeerByIndex(uint32(j))
			if err != nil {
				return nil, err
			}
			ecChunkQuery := types.ECChunkQuery{
				SegmentRoot: common.Hash(erasurCodingSegmentRoot),
			}
			response, err := n.makeRequest(peerIdentifier, ecChunkQuery)
			// fmt.Printf("[DEBUG] Received response: %s\n", response)
			if err != nil {
				fmt.Printf("Failed to make request from node %d to %s: %v", 0, peerIdentifier, err)
				ecChunkResponses = append(ecChunkResponses, types.ECChunkResponse{})
			}
			var ecChunkResponse types.ECChunkResponse
			decoded, _ := types.Decode(response, reflect.TypeOf(ecChunkResponse))
			ecChunkResponse = decoded.(types.ECChunkResponse)
			ecChunkResponses = append(ecChunkResponses, ecChunkResponse)

			fetchedChunks++
			if fetchedChunks >= K {
				break
			}
		}

		// debug
		fmt.Printf("Fetched %d chunks\n", fetchedChunks)
		for j := 0; j < fetchedChunks; j++ {
			fmt.Printf("Chunk %d: %x\n", j, ecChunkResponses[j])
		}

		// build decoder input
		// _ = decoderInputSegments
		decoderInputSegment := make([][]byte, 0)
		for j := 0; j < N; j++ {
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
	reconstructedData, err := n.decode(decoderInputSegments, true, 24)
	if err != nil {
		return nil, err
	}

	// Trim any padding using the original length
	if len(reconstructedData) > int(originalLength) {
		reconstructedData = reconstructedData[:originalLength]
	}

	return reconstructedData, nil
}

func (n *Node) FetchAndReconstructAllSegmentsData(treeRoot common.Hash) ([][]byte, error) {
	var outputData [][]byte
	K, N := erasurecoding.GetCodingRate()
	fmt.Printf("Using K=%d and N=%d\n", K, N)

	segmentsECRoots, err := n.store.ReadKV(treeRoot)
	if err != nil {
		return nil, err
	}
	allHash := SplitBytesIntoHash(segmentsECRoots, len(common.Hash{}))
	// segmentRoots, pageProohRoots := splitHashes(allHash)
	segmentRoots, _ := splitHashes(allHash)

	for _, segmentRoot := range segmentRoots {
		segmentMeta, err := n.store.ReadKV(segmentRoot)
		if err != nil {
			return nil, err
		}
		originalLength, segmentRootsFlattened, _ := decodeBlobMeta(segmentMeta)
		erasurCodingSegmentRoots := make([][]byte, 0)
		fmt.Printf("len(segmentRootsFlattened): %d\n", len(segmentRootsFlattened))
		for i := 0; i < len(segmentRootsFlattened); i += 32 {
			erasurCodingSegmentRoots = append(erasurCodingSegmentRoots, segmentRootsFlattened[i:i+32])
		}
		// Fetch the chunks from peers
		decoderInputSegments := make([][][]byte, 0)
		for i, erasurCodingSegmentRoot := range erasurCodingSegmentRoots {
			_ = i
			ecChunkResponses := make([]types.ECChunkResponse, 0)
			fetchedChunks := 0
			for j := 0; j < numNodes; j++ {
				peerIdentifier, err := n.getPeerByIndex(uint32(j))
				if err != nil {
					return nil, err
				}
				ecChunkQuery := types.ECChunkQuery{
					SegmentRoot: common.Hash(erasurCodingSegmentRoot),
				}
				response, err := n.makeRequest(peerIdentifier, ecChunkQuery)
				// fmt.Printf("[DEBUG] Received response: %s\n", response)
				if err != nil {
					fmt.Printf("Failed to make request from node %d to %s: %v", 0, peerIdentifier, err)
					ecChunkResponses = append(ecChunkResponses, types.ECChunkResponse{})
				}
				var ecChunkResponse types.ECChunkResponse
				decoded, _ := types.Decode(response, reflect.TypeOf(ecChunkResponse))
				ecChunkResponse = decoded.(types.ECChunkResponse)
				ecChunkResponses = append(ecChunkResponses, ecChunkResponse)

				fetchedChunks++
				if fetchedChunks >= K {
					break
				}
			}

			// debug
			fmt.Printf("Fetched %d chunks\n", fetchedChunks)
			for j := 0; j < fetchedChunks; j++ {
				fmt.Printf("Chunk %d: %x\n", j, ecChunkResponses[j])
			}

			// build decoder input
			// _ = decoderInputSegments
			decoderInputSegment := make([][]byte, 0)
			for j := 0; j < N; j++ {
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
		reconstructedData, err := n.decode(decoderInputSegments, true, 24)
		if err != nil {
			return nil, err
		}

		// Trim any padding using the original length
		if len(reconstructedData) > int(originalLength) {
			reconstructedData = reconstructedData[:originalLength]
		}
		outputData = append(outputData, reconstructedData)
	}
	return outputData, nil
}

func (n *Node) FetchAndReconstructSpecificSegmentData(segmentRoot common.Hash) ([]byte, error) {
	K, N := erasurecoding.GetCodingRate()
	fmt.Printf("Using K=%d and N=%d\n", K, N)

	segmentMeta, err := n.store.ReadKV(segmentRoot)
	if err != nil {
		return nil, err
	}
	originalLength, segmentRootsFlattened, _ := decodeBlobMeta(segmentMeta)
	erasurCodingSegmentRoots := make([][]byte, 0)
	fmt.Printf("len(segmentRootsFlattened): %d\n", len(segmentRootsFlattened))
	for i := 0; i < len(segmentRootsFlattened); i += 32 {
		erasurCodingSegmentRoots = append(erasurCodingSegmentRoots, segmentRootsFlattened[i:i+32])
	}
	// Fetch the chunks from peers
	decoderInputSegments := make([][][]byte, 0)
	for i, erasurCodingSegmentRoot := range erasurCodingSegmentRoots {
		_ = i
		ecChunkResponses := make([]types.ECChunkResponse, 0)
		fetchedChunks := 0
		for j := 0; j < numNodes; j++ {
			peerIdentifier, err := n.getPeerByIndex(uint32(j))
			if err != nil {
				return nil, err
			}
			ecChunkQuery := types.ECChunkQuery{
				SegmentRoot: common.Hash(erasurCodingSegmentRoot),
			}
			response, err := n.makeRequest(peerIdentifier, ecChunkQuery)
			// fmt.Printf("[DEBUG] Received response: %s\n", response)
			if err != nil {
				fmt.Printf("Failed to make request from node %d to %s: %v", 0, peerIdentifier, err)
				ecChunkResponses = append(ecChunkResponses, types.ECChunkResponse{})
			}
			var ecChunkResponse types.ECChunkResponse
			decoded, _ := types.Decode(response, reflect.TypeOf(ecChunkResponse))
			ecChunkResponse = decoded.(types.ECChunkResponse)
			ecChunkResponses = append(ecChunkResponses, ecChunkResponse)

			fetchedChunks++
			if fetchedChunks >= K {
				break
			}
		}

		// debug
		fmt.Printf("Fetched %d chunks\n", fetchedChunks)
		for j := 0; j < fetchedChunks; j++ {
			fmt.Printf("Chunk %d: %x\n", j, ecChunkResponses[j])
		}

		// build decoder input
		// _ = decoderInputSegments
		decoderInputSegment := make([][]byte, 0)
		for j := 0; j < N; j++ {
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
	reconstructedData, err := n.decode(decoderInputSegments, true, 24)
	if err != nil {
		return nil, err
	}

	// Trim any padding using the original length
	// if len(reconstructedData) > int(originalLength) {
	// 	reconstructedData = reconstructedData[:originalLength]
	// }

	_ = originalLength

	return reconstructedData, nil
}

func (n *Node) FetchAndReconstructArbitraryData(blobHash common.Hash, blobLen int) ([]byte, error) {
	K, N := erasurecoding.GetCodingRate()
	fmt.Printf("Using K=%d and N=%d for data size %d\n", K, N, blobLen)

	blob_meta, err := n.store.ReadKV(blobHash)
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
				SegmentRoot: common.Hash(segmentRoot),
			}
			response, err := n.makeRequest(peerIdentifier, ecChunkQuery)
			if err != nil {
				fmt.Printf("Failed to make request from node %d to %s: %v", 0, peerIdentifier, err)
				ecChunkResponses = append(ecChunkResponses, types.ECChunkResponse{})
			}
			var ecChunkResponse types.ECChunkResponse
			decoded, _ := types.Decode(response, reflect.TypeOf(ecChunkResponse))
			ecChunkResponse = decoded.(types.ECChunkResponse)
			ecChunkResponses = append(ecChunkResponses, ecChunkResponse)

			fetchedChunks++
			if fetchedChunks >= K {
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
		for j := 0; j < N; j++ {
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
	reconstructedData, err := n.decode(decoderInputSegments, false, int(blobLen))
	if err != nil {
		return nil, err
	}

	// Trim any padding using the original length
	if len(reconstructedData) > int(originalLength) {
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
	case types.Judgement:
		return "Judgement"
	case types.Preimages:
		return "Preimages"
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
	case types.GuaranteeReport:
		return "GuaranteeReport"
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
				newStateDB.PreviousGuarantors()
				newStateDB.AssignGuarantors()
				n.addStateDB(newStateDB)
				n.blocks[newBlock.Hash()] = newBlock
				n.broadcast(*newBlock)
				fmt.Printf("[N%d] BLOCK BROADCASTED: %v <- %v\n", n.id, newBlock.ParentHash(), newBlock.Hash())
				for _, g := range newStateDB.GuarantorAssignments {
					fmt.Printf("[N%d] GUARANTOR ASSIGNMENTS: %x -> core %v \n", n.id, g.Validator.Ed25519, g.CoreIndex)
				}
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
		payloadBytes := types.Encode(msg.Payload)
		decoded, _ := types.Decode(payloadBytes, reflect.TypeOf(ticket))
		ticket = decoded.(types.Ticket)
		//fmt.Printf("[N%v] Outgoing Ticket: %+v\n", n.id, ticket.TicketID())
		n.broadcast(ticket)
	case "Block":
		var blk types.Block
		payloadBytes := types.Encode(msg.Payload)
		decoded, _ := types.Decode(payloadBytes, reflect.TypeOf(blk))
		blk = decoded.(types.Block)
		//fmt.Printf("[N%v] Outgoing Block: %+v\n", n.id, blk.Hash())
		n.broadcast(blk)
	default:
		fmt.Printf("[N%v] Unhandled message type: %v\n", n.id, msg.MsgType)
	}
}

func generateObject() interface{} {
	return nil
}

// Split the []byte into Hash of fixed size
func SplitBytesIntoHash(data []byte, hashSize int) []common.Hash {
	// Calculate the number of hashs
	numChunks := int(math.Ceil(float64(len(data)) / float64(hashSize)))
	// Make a slice of hashs
	hashs := make([]common.Hash, 0, numChunks)

	// Slice the data into hashs
	for i := 0; i < len(data); i += hashSize {
		end := i + hashSize
		if end > len(data) {
			end = len(data)
		}
		hashs = append(hashs, common.Hash(data[i:end]))
	}
	return hashs
}

func splitHashes(hashes []common.Hash) ([]common.Hash, []common.Hash) {
	// Compute the total number of segments
	totalSegments := len(hashes)
	// Compute the number of page proofs
	pfCount := int(math.Ceil(float64(totalSegments) / 64))

	// The part of the hashes that are segment hashes
	segmentHashs := hashes[:totalSegments-pfCount]

	// The remaining part of the hashes that are page proofs
	pfHashs := hashes[totalSegments-pfCount:]

	return segmentHashs, pfHashs
}

func (n *Node) GetSegmentTreeRoots(treeRoot common.Hash) ([]common.Hash, error) {
	segmentsECRoots, err := n.store.ReadKV(treeRoot)
	if err != nil {
		return nil, err
	}
	allHash := SplitBytesIntoHash(segmentsECRoots, len(common.Hash{}))
	segmentRoots, _ := splitHashes(allHash)
	return segmentRoots, nil
}
