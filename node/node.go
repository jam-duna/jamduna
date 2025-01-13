package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"log"

	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"os"

	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math"
	"net"
	"reflect"

	"math/big"
	rand0 "math/rand"
	"strings"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"

	"github.com/colorfulnotion/jam/trie"
	"github.com/quic-go/quic-go"
)

const (
	// immediate-term: bundle=WorkPackage.Bytes(); short-term: bundle=WorkPackageBundle.Bytes() without justification; medium-term= same with proofs; long-term: push method
	debugDA           = false // DA
	debugB            = false // Blocks, Announcment
	debugG            = false // Guaranteeing
	debugT            = false // Tickets/Safrole
	debugP            = false // Preimages
	debugA            = false // Assurances
	debugF            = false // Finality
	debugJ            = false // Audits + Judgements
	debug             = false // General Node Ops
	debugAudit        = false // Audit
	trace             = false
	debugE            = false // monitoring fn execution time
	debugTree         = false // trie
	debugSegments     = false // Fetch import segments
	debugBundle       = false // Fetch WorkPackage Bundle
	debugAncestor     = false // Check Ancestor
	debugSTF          = true  // State Transition Function
	debugPublishTrace = true  // Publish Trace -- such that each node should have full state_transition
	numNodes          = 6
	quicAddr          = "127.0.0.1:%d"
	basePort          = 9000
	godMode           = false
	Grandpa           = false
)

const (
	ValidatorFlag   = "VALIDATOR"
	ValidatorDAFlag = "VALIDATOR&DA"
)

type Node struct {
	id uint16

	AuditNodeType string
	credential    types.ValidatorSecret
	server        quic.Listener
	peers         []string
	peersInfo     map[uint16]*Peer //<validatorIndex> -> Peer

	tlsConfig *tls.Config

	store *storage.StateDBStorage

	extrinsic_pool *types.ExtrinsicPool

	block_tree *types.BlockTree
	grandpa    *grandpa.Grandpa
	// holds a map of epoch (use entropy to control it) to at most 2 tickets
	selfTickets  map[common.Hash][]types.TicketBucket
	ticketsMutex sync.Mutex
	sendTickets  bool // when mode=fallback this is false, otherwise is true

	// this is for audit
	// announcement [headerHash -> [wr_hash]]
	auditingMap     sync.Map
	announcementMap sync.Map // headerHash -> stateDB
	judgementMap    sync.Map // headerHash -> JudgeBucket
	//judgementBucket types.JudgeBucket
	judgementWRMap map[common.Hash]common.Hash // wr_hash -> headerHash. TODO: shawn to update this
	judgementWRMapMutex sync.Mutex

	clients      map[string]string
	clientsMutex sync.Mutex

	// assurances state: are this node assuring the work package bundle/segments?
	assurancesBucket map[common.Hash]bool
	assuranceMutex   sync.Mutex
	delaysend        map[common.Hash]int // delaysend is a map of workpackagehash to the number of times it has been delayed

	// holds a map of the parenthash to the block
	blocks      map[common.Hash]*types.Block
	blocksMutex sync.Mutex

	headers      map[common.Hash]*types.Block
	headersMutex sync.Mutex

	preimages      map[common.Hash][]byte // preimageLookup -> preimageBlob
	preimagesMutex sync.Mutex

	workReports      map[common.Hash]types.WorkReport
	workReportsMutex sync.Mutex

	blockAnnouncementsCh    chan types.BlockAnnouncement
	ticketsCh               chan types.Ticket
	workPackagesCh          chan types.WorkPackage
	workReportsCh           chan types.WorkReport
	guaranteesCh            chan types.Guarantee
	assurancesCh            chan types.Assurance
	preimageAnnouncementsCh chan types.PreimageAnnouncement
	announcementsCh         chan types.Announcement
	judgementsCh            chan types.Judgement
	auditingCh              chan *statedb.StateDB // use this to trigger auditing, block hash

	// grandpa input channels
	grandpaPreVoteMessageCh   chan grandpa.VoteMessage
	grandpaPreCommitMessageCh chan grandpa.VoteMessage
	grandpaPrimaryMessageCh   chan grandpa.VoteMessage
	grandpaCommitMessageCh    chan grandpa.CommitMessage

	waitingAnnouncements      []types.Announcement
	waitingAnnouncementsMutex sync.Mutex
	waitingJudgements         []types.Judgement
	waitingJudgementsMutex    sync.Mutex

	// holds a map of the hash to the stateDB
	statedbMap      map[common.Hash]*statedb.StateDB
	statedbMapMutex sync.Mutex

	// holds the tip
	statedb      *statedb.StateDB
	statedbMutex sync.Mutex

	nodeType        string
	dataDir         string
	epoch0Timestamp uint32

	// DA testing only
	chunkMap map[common.Hash][]byte
	chunkBox map[common.Hash][][]byte

	// JamBlocks testing only
	JAMBlocksEndpoint string
	JAMBlocksPort     uint16

	// god mode
	godCh        *chan uint32
	timeslotUsed map[uint32]bool
}

/*
A Tip StateDB is held in the node structure
When a block comes in, we validate whether the block identified by parenthash and its extrinsics gets to this block.
  if it does, we put it into map[blockHash]*StateDB
  if its the latest timeslot, we update the tip
When a block is authored, we take the latest block identified by some parenthash and its extrinsics and get a new block.
  we update the tip
*/

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
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
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

func (n *Node) String() string {
	return fmt.Sprintf("[N%d]", n.id)
}

func (n *Node) setValidatorCredential(credential types.ValidatorSecret) {
	n.credential = credential
	if false {
		jsonData, err := types.Encode(credential)
		if err != nil {
			fmt.Printf("setValidatorCredential: %v\n", err)
		}
		fmt.Printf("[N%v] credential %s\n", n.id, jsonData)
	}
}

func (n *Node) setGodCh(c *chan uint32) {
	n.godCh = c
}

func (n *Node) sendGodTimeslotUsed(timeslot uint32) {
	if n.godCh == nil {
		return
	}

	*n.godCh <- timeslot
}

func (n *Node) receiveGodTimeslotUsed(timeslot uint32) {
	if n.godCh == nil {
		return
	}
	n.timeslotUsed[timeslot] = true
}

func (n *Node) checkGodTimeslotUsed(timeslot uint32) bool {
	if n.godCh == nil {
		return false
	}

	_, ok := n.timeslotUsed[timeslot]
	return ok
}

func loadStateSnapshot(filePath string) (statedb.StateSnapshotRaw, error) {
	snapshotRawBytes, err := os.ReadFile(filePath)
	if err != nil {
		return statedb.StateSnapshotRaw{}, fmt.Errorf("error reading JSON file %s: %v", filePath, err)
	}

	var stateSnapshotRaw statedb.StateSnapshotRaw
	err = json.Unmarshal(snapshotRawBytes, &stateSnapshotRaw)
	if err != nil {
		return statedb.StateSnapshotRaw{}, fmt.Errorf("error unmarshaling JSON file %s: %v", filePath, err)
	}

	return stateSnapshotRaw, nil
}

func getGenesisFile(network string) string {
	return fmt.Sprintf("/chainspecs/traces/genesis-%s.json", network)
}

func createNode(id uint16, credential types.ValidatorSecret, genesisStateFile string, epoch0Timestamp uint32, peers []string, peerList map[uint16]*Peer, dataDir string, port int, flag string) (*Node, error) {
	return newNode(id, credential, genesisStateFile, epoch0Timestamp, peers, peerList, flag, dataDir, port)
}

func NewNode(id uint16, credential types.ValidatorSecret, genesisStateFile string, epoch0Timestamp uint32, peers []string, peerList map[uint16]*Peer, dataDir string, port int) (*Node, error) {
	return createNode(id, credential, genesisStateFile, epoch0Timestamp, peers, peerList, dataDir, port, ValidatorFlag)
}

func NewNodeDA(id uint16, credential types.ValidatorSecret, genesisStateFile string, epoch0Timestamp uint32, peers []string, peerList map[uint16]*Peer, dataDir string, port int) (*Node, error) {
	return createNode(id, credential, genesisStateFile, epoch0Timestamp, peers, peerList, dataDir, port, ValidatorDAFlag)
}

func newNode(id uint16, credential types.ValidatorSecret, genesisStateFile string, epoch0Timestamp uint32, peers []string, startPeerList map[uint16]*Peer, nodeType string, dataDir string, port int) (*Node, error) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	fmt.Printf("[N%v] newNode addr=%s dataDir=%v\n", id, addr, dataDir)

	levelDBPath := fmt.Sprintf("%v/leveldb/", dataDir)
	store, err := storage.NewStateDBStorage(levelDBPath)
	if err != nil {
		return nil, fmt.Errorf("NewStateDBStorage %v", err)
	}

	var cert tls.Certificate
	ed25519_priv := ed25519.PrivateKey(credential.Ed25519Secret[:])
	ed25519_pub := ed25519_priv.Public().(ed25519.PublicKey)

	cert, err = generateSelfSignedCert(ed25519_pub, ed25519_priv)
	if err != nil {
		return nil, fmt.Errorf("Error generating self-signed certificate: %v", err)
	}

	node := &Node{
		id:        id,
		store:     store,
		peers:     peers,
		peersInfo: make(map[uint16]*Peer),
		clients:   make(map[string]string),
		nodeType:  nodeType,

		extrinsic_pool: types.NewExtrinsicPool(),

		statedbMap:     make(map[common.Hash]*statedb.StateDB),
		judgementWRMap: make(map[common.Hash]common.Hash),

		blocks:    make(map[common.Hash]*types.Block),
		headers:   make(map[common.Hash]*types.Block),
		preimages: make(map[common.Hash][]byte),

		selfTickets:      make(map[common.Hash][]types.TicketBucket),
		assurancesBucket: make(map[common.Hash]bool),
		delaysend:        make(map[common.Hash]int),

		blockAnnouncementsCh:    make(chan types.BlockAnnouncement, 200),
		ticketsCh:               make(chan types.Ticket, 200),
		workPackagesCh:          make(chan types.WorkPackage, 200),
		workReportsCh:           make(chan types.WorkReport, 200),
		guaranteesCh:            make(chan types.Guarantee, 200),
		assurancesCh:            make(chan types.Assurance, 200),
		preimageAnnouncementsCh: make(chan types.PreimageAnnouncement, 200),
		announcementsCh:         make(chan types.Announcement, 200),
		judgementsCh:            make(chan types.Judgement, 200),
		auditingCh:              make(chan *statedb.StateDB, 200),

		grandpaPreVoteMessageCh:   make(chan grandpa.VoteMessage, 200),
		grandpaPreCommitMessageCh: make(chan grandpa.VoteMessage, 200),
		grandpaPrimaryMessageCh:   make(chan grandpa.VoteMessage, 200),
		grandpaCommitMessageCh:    make(chan grandpa.CommitMessage, 200),

		sendTickets: true,

		timeslotUsed: make(map[uint32]bool),
		godCh:        nil,

		dataDir: dataDir,
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
					pubKey, ok := cert.PublicKey.(ed25519.PublicKey)
					if !ok {
						return fmt.Errorf("client public key is not Ed25519")
					}
					node.clientsMutex.Lock()
					node.clients[info.Conn.RemoteAddr().String()] = hex.EncodeToString(pubKey)
					node.clientsMutex.Unlock()
					//fmt.Printf("Client Ed25519 Public Key: %x %s\n", pubKey, info.Conn.RemoteAddr())
					return nil
				},
				NextProtos: []string{"h3", "http/1.1", "ping/1.1"},
			}, nil
		},
		NextProtos: []string{"h3", "http/1.1", "ping/1.1"},
	}
	node.tlsConfig = tlsConfig

	//fmt.Printf("[N%v] OPENING %s\n", id, addr)
	listener, err := quic.ListenAddr(addr, tlsConfig, GenerateQuicConfig())
	if err != nil {
		fmt.Printf("ERR %v\n", err)
		return nil, err
	}
	node.server = *listener

	for validatorIndex, p := range startPeerList {
		node.peersInfo[validatorIndex] = NewPeer(node, validatorIndex, p.Validator, p.PeerAddr)
	}

	_statedb, err := statedb.NewStateDBFromSnapshotRawFile(node.store, genesisStateFile)
	if err == nil {
		_statedb.SetID(uint16(id))
		node.addStateDB(_statedb)
	} else {
		fmt.Printf("NewGenesisStateDB ERR %v\n", err)
		return nil, err
	}
	node.setValidatorCredential(credential)
	node.epoch0Timestamp = epoch0Timestamp
	var validators []types.Validator
	validators = node.statedb.GetSafrole().NextValidators
	if len(validators) == 0 {
		panic("No validators")
	}
	go node.runServer()
	go node.runClient()
	go node.runMain()
	go node.runPreimages()
	go node.runBlocksTickets()
	go node.runAudit() // disable this to pause FetchWorkPackageBundle
	return node, nil
}

func GenerateQuicConfig() *quic.Config {
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

// use ed25519 key to get peer info
func (n *Node) GetPeerInfoByEd25519(key types.Ed25519Key) (*Peer, error) {
	for _, peer := range n.peersInfo {
		if peer.Validator.Ed25519 == key {
			return peer, nil
		}
	}
	return nil, fmt.Errorf("peer not found")
}

func (n *Node) GetBandersnatchSecret() []byte {
	return n.credential.BandersnatchSecret
}

func (n *Node) GetSelfCoreIndex() (uint16, error) {
	if len(n.statedb.GuarantorAssignments) == 0 {
		return 0, fmt.Errorf("NO ASSIGNMENTS")
	}
	for _, assignment := range n.statedb.GuarantorAssignments {
		if assignment.Validator.GetEd25519Key() == n.GetEd25519Key() {
			return assignment.CoreIndex, nil
		}
	}
	return 0, fmt.Errorf("core index not found")
}

func (n *Node) GetCoreIndexFromEd25519Key(key types.Ed25519Key) (uint16, error) {
	assignments := n.statedb.GuarantorAssignments
	for _, assignment := range assignments {
		if assignment.Validator.GetEd25519Key() == key {
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

// func (n *Node) GetCoreWorkersPeers(core uint16)(workers []Peer) {
// 	workers = make([]Peer, 0)
// 	for _, assignment := range n.statedb.GuarantorAssignments {
// 		if assignment.CoreIndex == core {
// 			peer, err := n.GetPeerInfoByEd25519(assignment.Validator.Ed25519)
// 			if err == nil {
// 				workers = append(workers, *peer.Clone())
// 			}
// 		}
// 	}
// 	return workers
// }

// this function will return the core workers of that core
func (n *Node) GetCoreCoWorkersPeers(core uint16) (coWorkers []Peer) {
	coWorkers = make([]Peer, 0)
	for _, assignment := range n.statedb.GuarantorAssignments {
		if assignment.CoreIndex == core {
			peer, err := n.GetPeerInfoByEd25519(assignment.Validator.Ed25519)
			if err == nil {
				coWorkers = append(coWorkers, *peer.Clone())
			}
		}
	}
	return coWorkers
}

func (n *Node) GetCurrValidatorIndex() uint32 {
	return uint32(n.statedb.GetSafrole().GetCurrValidatorIndex(n.GetEd25519Key()))
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

func (n *Node) getPeerByIndex(peerIdx uint16) (*Peer, error) {
	p := n.peersInfo[peerIdx]
	// check if peer exists
	if p != nil {
		return p, nil
	}
	return nil, fmt.Errorf("peer %v not found", peerIdx)
}

func (n *Node) getPeerAddr(peerIdx uint16) (*Peer, error) {
	peer, exist := n.peersInfo[peerIdx]
	if exist {
		return peer, nil
	}
	fmt.Printf("getPeerAddr not found %v\n", peerIdx)
	return nil, fmt.Errorf("peer not found")
}

func (n *Node) addStateDB(_statedb *statedb.StateDB) error {
	n.statedbMutex.Lock()
	n.statedbMapMutex.Lock()
	defer n.statedbMutex.Unlock()
	defer n.statedbMapMutex.Unlock()

	if n.statedb == nil || n.statedb.GetBlock() == nil {
		var headerHash common.Hash
		if _statedb.GetBlock() != nil {
			headerHash = _statedb.GetHeaderHash()
		}
		if debug {
			fmt.Printf("[N%d] addStateDB [%v <- %v] (stateRoot: %v)\n", n.id, _statedb.ParentHeaderHash, _statedb.HeaderHash, _statedb.StateRoot)
		}
		n.statedb = _statedb
		n.statedbMap[headerHash] = _statedb
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
		if debug {
			fmt.Printf("[N%d] addStateDB TIP %v\n", n.id, _statedb.GetHeaderHash())
		}
		n.statedb = _statedb
		n.statedbMap[_statedb.GetHeaderHash()] = _statedb
	}
	return nil
}

func (n *Node) GetEd25519Key() types.Ed25519Key {
	return types.Ed25519Key(n.credential.Ed25519Pub)
}

func (n *Node) GetEd25519Secret() []byte {
	return n.credential.Ed25519Secret[:]
}

func (n *Node) ResetPeer(peerIdentifier string) {
	/*
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
	*/
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

func (n *Node) lookupPubKey(pubKey string) (uint16, bool) {
	hpubKey := fmt.Sprintf("0x%s", pubKey)
	for validatorIndex, p := range n.peersInfo {
		peerPubKey := p.Validator.Ed25519.String()
		if peerPubKey == hpubKey {
			return validatorIndex, true
		}
	}

	return 0, false
}

func (n *Node) handleConnection(conn quic.Connection) {

	remoteAddr := conn.RemoteAddr().String()
	n.clientsMutex.Lock()
	pubKey, ok := n.clients[remoteAddr]
	n.clientsMutex.Unlock()
	if !ok {
		fmt.Printf("handleConnection: UNKNOWN remoteAddr=%s unknown\n", remoteAddr)
		return
	}

	validatorIndex, ok := n.lookupPubKey(pubKey)
	if !ok {
		fmt.Printf("handleConnection: UNKNOWN pubkey %s from remoteAddr=%s\n", pubKey, remoteAddr)
		return
	}
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			if quicErr, ok := err.(*quic.ApplicationError); ok && quicErr.ErrorCode == 0 {
				continue
			}
			fmt.Printf("handleConnection: Accept stream error: %v\n", err)
		}
		go n.DispatchIncomingQUICStream(stream, validatorIndex)
	}
}

func (n *Node) broadcast(obj interface{}) []byte {
	result := []byte{}
	objType := reflect.TypeOf(obj)
	for id, p := range n.peersInfo {
		if id == n.id {
			if objType == reflect.TypeOf(types.Assurance{}) {
				a := obj.(types.Assurance)
				n.assurancesCh <- a
				continue
			} else if objType == reflect.TypeOf(types.PreimageAnnouncement{}) {
				continue
			}
		}
		//fmt.Printf("%s BROADCAST PeerID=%v\n", n.String(), p.PeerID)
		switch objType {
		case reflect.TypeOf(types.Ticket{}):
			t := obj.(types.Ticket)
			epoch := uint32(0) // TODO: Shawn
			err := p.SendTicketDistribution(epoch, t, false)
			if err != nil {
				fmt.Printf("SendTicketDistribution ERR %v\n", err)
			}
			break
		case reflect.TypeOf(types.Block{}):
			b := obj.(types.Block)
			slot := uint32(0) // TODO: Shawn
			//fmt.Printf("%s BROADCAST SendBlockAnnouncement to %d: %v\n", n.String(), id, b.Header.Hash())
			err := p.SendBlockAnnouncement(b, slot)
			if err != nil {
				fmt.Printf("SendBlockAnnouncement ERR %v\n", err)
			}
			break

		case reflect.TypeOf(types.Guarantee{}):
			g := obj.(types.Guarantee)
			if debugG {
				fmt.Printf("%s [broadcast:SendWorkReportDistribution] to %d\n", n.String(), id)
			}
			err := p.SendWorkReportDistribution(g.Report, g.Slot, g.Signatures)
			if err != nil {
				fmt.Printf("SendWorkReportDistribution ERR %v\n", err)
			}

			break
		case reflect.TypeOf(types.Assurance{}):
			a := obj.(types.Assurance)
			err := p.SendAssurance(&a)
			if err != nil {
				fmt.Printf("SendAssurance ERR %v\n", err)
			}
			break
		case reflect.TypeOf(JAMSNPAuditAnnouncementWithProof{}):
			a := obj.(JAMSNPAuditAnnouncementWithProof)
			err := p.SendAuditAnnouncement(&a)
			if err != nil {
				fmt.Printf("SendAuditAnnouncement ERR %v\n", err)
			}
			break
		case reflect.TypeOf(types.Judgement{}):
			j := obj.(types.Judgement)
			epoch := uint32(0) // TODO: Shawn
			err := p.SendJudgmentPublication(epoch, j)
			if err != nil {
				fmt.Printf("SendJudgmentPublication ERR %v\n", err)
			}
			break
		case reflect.TypeOf(types.PreimageAnnouncement{}):
			preimageAnnouncement := obj.(types.PreimageAnnouncement)
			err := p.SendPreimageAnnouncement(&preimageAnnouncement)
			if err != nil {
				fmt.Printf("SendPreimageAnnouncement ERR %v\n", err)
			}
			break

		case reflect.TypeOf(grandpa.VoteMessage{}):
			vote := obj.(grandpa.VoteMessage)
			err := p.SendVoteMessage(vote)
			if err != nil {
				fmt.Printf("SendVoteMessage ERR %v\n", err)
			}
			break
		case reflect.TypeOf(grandpa.CommitMessage{}):
			commit := obj.(grandpa.CommitMessage)
			err := p.SendCommitMessage(commit)
			if err != nil {
				fmt.Printf("SendCommitMessage ERR %v\n", err)
			}
			break
		}

	}
	return result
}

// Helper function to determine if the error is a timeout error
func isTimeoutError(err error) bool {
	// Add more specific error handling as needed
	return strings.Contains(err.Error(), "timeout")
}

func (n *Node) dumpstatedbmap() {
	n.statedbMapMutex.Lock()
	defer n.statedbMapMutex.Unlock()

	for hash, statedb := range n.statedbMap {
		fmt.Printf("dumpstatedbmap: statedbMap[%v] => statedb (%v<=parent=%v) StateRoot %v\n", hash, statedb.ParentHeaderHash, statedb.HeaderHash, statedb.StateRoot)
	}

	n.blocksMutex.Lock()
	defer n.blocksMutex.Unlock()
	for hash, blk := range n.blocks {
		fmt.Printf("dumpstatedbmap: blocks[%v] => %v\n", hash, blk.Hash())
	}
}

func randomKey(m map[string]*Peer) string {
	rand0.Seed(time.Now().UnixNano())
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys[rand0.Intn(len(keys))]
}

func (n *Node) fetchBlocks(headerHash common.Hash, direction uint8, maximumBlocks uint32) (*[]types.Block, error) {
	requests_original := make([]CE128_request, types.TotalValidators)
	for i := range types.TotalValidators {
		requests_original[i] = CE128_request{
			ShardIndex:    uint16(i),
			HeaderHash:    headerHash,
			Direction:     direction,
			MaximumBlocks: maximumBlocks,
		}
	}
	requests := make([]interface{}, types.TotalValidators)
	for i, req := range requests_original {
		requests[i] = req
	}

	resps, err := n.makeRequests(requests, 1, time.Duration(3*maximumBlocks)*time.Second, time.Duration(6*maximumBlocks)*time.Second)
	if err != nil {
		return nil, err
	}
	for _, resp := range resps {
		ce128_Resp, ok := resp.(CE128_response)
		if !ok {
			// ignore such response
			continue
		}
		if len(ce128_Resp.Blocks) >= 0 && ce128_Resp.HeaderHash == headerHash {
			return &ce128_Resp.Blocks, nil
		}
	}
	return nil, fmt.Errorf("fetchBlocks - No response")
}

func (n *Node) extendChain() error {
	parentheaderhash := n.statedb.HeaderHash
	for {

		ok := false
		for _, b := range n.blocks {

			if b.GetParentHeaderHash() == parentheaderhash {
				ok = true
				nextBlock := b
				// Measure time taken to apply state transition
				start := time.Now()
				// Apply the block to the tip
				recoveredStateDB := n.statedb.Copy()
				recoveredStateDB.RecoverJamState(nextBlock.Header.ParentStateRoot)
				newStateDB, err := statedb.ApplyStateTransitionFromBlock(recoveredStateDB, context.Background(), nextBlock)
				if err != nil {
					fmt.Printf("[N%d] extendChain FAIL %v\n", n.id, err)
					return err
				}

				newStateDB.SetAncestor(nextBlock.Header, recoveredStateDB)
				if debugAncestor {
					timeSlots1 := n.statedb.GetAncestorTimeSlot()
					timeSlots2 := newStateDB.GetAncestorTimeSlot()
					fmt.Printf("[N%d] Ancestor timeSlots1 %d\n", n.id, timeSlots1)
					fmt.Printf("[N%d] Ancestor timeSlots2 %d\n", n.id, timeSlots2)
				}
				if debugPublishTrace {
					st := buildStateTransitionStruct(recoveredStateDB, nextBlock, newStateDB)
					err = n.writeDebug(st, nextBlock.TimeSlot()) // StateTransition
					if err != nil {
						fmt.Printf("writeDebug StateTransition err: %v\n", err)
					}
					err := n.writeDebug(nextBlock, nextBlock.TimeSlot()) // Blocks
					if err != nil {
						fmt.Printf("writeDebug Block err: %v\n", err)
					}
					err = n.writeDebug(newStateDB.JamState.Snapshot(), nextBlock.TimeSlot()) // StateSnapshot
					if err != nil {
						fmt.Printf("writeDebug StateSnapshot err: %v\n", err)
					}
				}

				//new_xi := newStateDB.GetXContext().GetX_i()
				if debugTree {
					fmt.Printf("[N%d] Author newStateDB %v\n", n.id, newStateDB.StateRoot)
					newStateDB.GetTrie().PrintTree(newStateDB.GetTrie().Root, 0)
				}

				// Print the elapsed time in milliseconds
				elapsed := time.Since(start).Microseconds()
				if elapsed > 1000000 && trace {
					fmt.Printf("[N%d] extendChain %v <- %v \033[ApplyStateTransitionFromBlock\033[0m took %d ms\n", n.id, common.Str(parentheaderhash), common.Str(nextBlock.Hash()), elapsed/1000)
				}

				// Extend the tip of the chain
				n.addStateDB(newStateDB)

				// simulated finality
				//n.finalizeBlocks()
				parentheaderhash = nextBlock.Header.Hash()
				n.assureNewBlock(b)

				n.statedbMapMutex.Lock()
				n.auditingCh <- n.statedbMap[n.statedb.HeaderHash].Copy()
				n.statedbMapMutex.Unlock()

				IsTicketSubmissionClosed := n.statedb.GetSafrole().IsTicketSubmissionClosed(n.statedb.GetTimeslot())
				n.extrinsic_pool.RemoveUsedExtrinsicFromPool(b, n.statedb.GetSafrole().Entropy[2], IsTicketSubmissionClosed)
				break
			}

		}

		if !ok {
			// If there is no next block, we're done!
			if debug {
				fmt.Printf("[N%d] extendChain NO further next block %v\n", n.id, parentheaderhash)
			}
			return nil
		}
	}
}

func (n *Node) assureNewBlock(b *types.Block) error {
	if len(b.Extrinsic.Guarantees) > 0 {
		for _, g := range b.Extrinsic.Guarantees {
			n.assureData(g)
		}
	}
	a, numCores, err := n.generateAssurance(b.Header.Hash())
	if err != nil {
		return err
	}
	if numCores == 0 {
		return nil
	}
	if debugA {
		fmt.Printf("%s [assureNewBlock] Broadcasting assurance bitfield=%x\n", n.String(), a.Bitfield)
	}
	n.broadcast(a)
	return nil
}

// we arrive here when we receive a block from another node
func (n *Node) processBlock(blk *types.Block) error {
	// walk blk backwards, up to the tip, if possible -- but if encountering an unknown parenthash, immediately fetch the block.  Give up if we can't do anything
	b := blk
	n.StoreBlock(blk, n.id, false)
	n.cacheBlock(blk)
	n.cacheHeaders(b.Header.Hash(), blk)
	for {
		if b.GetParentHeaderHash() == (common.Hash{}) {
			//fmt.Printf("[N%d] processBlock: hit genesis (%v <- %v)\n", n.id, b.ParentHash(), b.Hash())
			break
		} else if n.statedb != nil && b.GetParentHeaderHash() == n.statedb.HeaderHash {
			//fmt.Printf("[N%d] processBlock: hit TIP (%v <- %v)\n", n.id, b.ParentHash(), b.Hash())
			break
		} else {
			parentBlock, ok := n.cacheBlockRead(b.GetParentHeaderHash())
			if !ok {
				parentBlocks, err := n.fetchBlocks(b.GetParentHeaderHash(), 1, 1)
				if err != nil || parentBlocks == nil {
					// have to give up right now (could try again though!)
					return err
				} else {
					parentBlock = &(*parentBlocks)[0]
				}
				// got the parent block, store it in the cache
				if parentBlock.GetParentHeaderHash() == blk.GetParentHeaderHash() {
					//fmt.Printf("[N%d] fetchBlocks (%v<-%v) Validated --- CACHING\n", n.id, blk.GetParentHeaderHash(), blk.Hash())
					n.StoreBlock(parentBlock, n.id, false)
					n.cacheBlock(parentBlock)
				} else {
					return nil
				}

			}
			b = parentBlock
		}
	}

	// we got to the tip, now extend the chain, moving the tip forward, applying blocks using blockcache
	n.extendChain()

	currJCE := common.ComputeCurrentJCETime()
	currEpoch, currPhase := n.statedb.GetSafrole().EpochAndPhase(currJCE)
	if blk.Header.EpochMark != nil || (currEpoch == 0 && currPhase == 0) {
		if debug {
			fmt.Printf("[N%d]GenerateTickets currEpoch=%v, currPhase=%v\n", n.id, currEpoch, currPhase)
		}
		n.GenerateTickets()
		n.BroadcastTickets()
	}

	return nil // Success
}

func setupSegmentsShards(segmentLen int) (segmentShards [][][]byte) {
	// setup proper arr for reconstruction
	segmentShards = make([][][]byte, segmentLen)
	for j := 0; j < segmentLen; j++ {
		for shardIdx := uint16(0); shardIdx < types.TotalValidators; shardIdx++ {
			segmentShards[j] = make([][]byte, types.TotalValidators)
		}
	}
	return segmentShards
}

// reconstructSegments uses CE139 and CAN use CE140 upon failure
// need to actually ECdecode
// We continuily use erasureRoot to ask the question
func (n *Node) reconstructSegments(erasureRoot common.Hash, segmentIndices []uint16) (segments [][]byte, err error) {
	requests_original := make([]CE139_request, types.TotalValidators)
	for i := range types.TotalValidators {
		requests_original[i] = CE139_request{
			ErasureRoot:    erasureRoot,
			SegmentIndices: segmentIndices,
			ShardIndex:     uint16(i),
		}
	}
	requests := make([]interface{}, types.TotalValidators)
	for i, req := range requests_original {
		requests[i] = req
	}

	responses, err := n.makeRequests(requests, types.W_E/2, time.Duration(3)*time.Second, time.Duration(10)*time.Second)
	if err != nil {
		fmt.Printf("Error in fetching import segments By ErasureRoot: %v\n", err)
		return nil, err
	}
	receiveSegments := make([][]byte, len(segmentIndices))
	receiveShard := make([][][]byte, len(segmentIndices))
	for i := range receiveShard {
		receiveShard[i] = make([][]byte, types.TotalValidators)
	}

	for _, resp := range responses {
		daResp, ok := resp.(CE139_response)
		if !ok {
			fmt.Printf("Error in convert import segments CE139_response: %v\n", err)
		}
		selected_segments, err := SplitToSegmentShards(daResp.SegmentShards)
		if err != nil {
			fmt.Printf("Error in fetching import segments SplitToSegmentShards: %v\n", err)
			continue
		}
		for idx, selected_segment := range selected_segments {
			// segmentIdx := segmentIndex[idx]
			receiveShard[idx][daResp.ShardIndex] = selected_segment
		}
	}
	for return_idx, segmentShard := range receiveShard {
		segmentShardRaw := make([][][]byte, 1)
		segmentShardRaw[0] = segmentShard
		exported_segment, err := n.decode(segmentShardRaw, true, types.FixedSegmentSizeG)
		if err != nil {
			fmt.Printf("Error in fetching import segments decode: %v\n", err)
		}
		receiveSegments[return_idx] = exported_segment
	}
	segments = receiveSegments

	return segments, nil
}

func (n *Node) reconstructPackageBundleSegments(erasureRoot common.Hash, blength uint32) (workPackageBundle types.WorkPackageBundle, err error) {
	requests_original := make([]CE138_request, types.TotalValidators)
	for i := range types.TotalValidators {
		requests_original[i] = CE138_request{
			ErasureRoot: erasureRoot,
			ShardIndex:  uint16(i),
		}
	}
	requests := make([]interface{}, types.TotalValidators)
	for i, req := range requests_original {
		requests[i] = req
	}

	responses, err := n.makeRequests(requests, types.W_E/2, time.Duration(3)*time.Second, time.Duration(10)*time.Second)
	if err != nil {
		fmt.Printf("Error in fetching bundle segments makeRequests: %v\n", err)
		return types.WorkPackageBundle{}, err
	}
	bundleShards := make([][]byte, types.TotalValidators)
	for _, resp := range responses {
		daResp, ok := resp.(CE138_response)
		if !ok {
			fmt.Printf("Error in convert bundle segments CE138_response: %v\n", err)
		}

		if debugBundle {
			fmt.Printf("[N%d] [%d] len(BundleShard) %d daResp.BundleShard %x\n", n.id, daResp.ShardIndex, len(daResp.BundleShard), daResp.BundleShard)
		}
		// segmentIdx := segmentIndex[idx]
		bundleShards[daResp.ShardIndex] = daResp.BundleShard
	}
	bundleShardsRaw := make([][][]byte, 1)
	bundleShardsRaw[0] = bundleShards
	//fmt.Printf("blength %d, bundleShardsRaw %x\n", blength, bundleShardsRaw)
	exported_segment, err := n.decode(bundleShardsRaw, false, int(blength))
	if err != nil {
		fmt.Printf("Error in fetching bundle segments decode: %v\n", err)
	}
	if debugBundle {
		fmt.Printf("[N%d] decode bundle exported_segment %x\n", n.id, exported_segment)
	}
	workPackageBundleRaw, _, err := types.Decode(exported_segment, reflect.TypeOf(types.WorkPackageBundle{}))
	if err != nil {
		fmt.Printf("[auditWorkReport] ERR %v\n", err)
		return
	}
	workPackageBundle = workPackageBundleRaw.(types.WorkPackageBundle)
	return workPackageBundle, nil
}

func (n *Node) getPVMStateDB() *statedb.StateDB {
	// refine's executeWorkPackage is a statelessish process
	target_statedb := n.statedb.Copy()
	return target_statedb
}

// -----Custom methods for tiny QUIC EC experiment-----

func getStructType(obj interface{}) string {
	v := reflect.TypeOf(obj)

	// If the type is a pointer, get the underlying element type
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// Get the type name (if available) and the package path
	structType := v.Name()
	pkgPath := v.PkgPath()

	// If there's no name, handle unexported types differently
	if structType == "" {
		structType = fmt.Sprintf("%v", v) // Fallback to full type description
	}

	// Check if there's a package path (means it's an exported type)
	if pkgPath != "" {
		parts := strings.Split(structType, ".")
		structType = strings.ToLower(parts[len(parts)-1])
	} else {
		structType = strings.ToLower(structType)
	}

	fmt.Printf("!!!!getStructType=%v\n", structType)
	return structType
}

func getMessageType(obj interface{}) string {
	switch obj.(type) {
	case types.BlockQuery:
		return "BlockQuery"
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
	case *types.Ticket:
		return "Ticket"
	case *types.Block:
		return "Block"
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
	case *statedb.StateDB:
		return "StateDB"
	case statedb.StateDB:
		return "StateDB"
	case *statedb.JamState:
		return "JamState"
	case statedb.JamState:
		return "JamState"
	case statedb.StateSnapshot:
		return "state_snapshot"
	case *statedb.StateSnapshot:
		return "state_snapshot"
	case statedb.StateSnapshotRaw:
		return "Trace"
	case *statedb.StateSnapshotRaw:
		return "Trace"
	case statedb.StateTransition:
		return "state_transition"
	case *statedb.StateTransition:
		return "state_transition"
	case DA_announcement:
		return "DA_announcement"
	case DA_request:
		return "DA_request"
	case DA_response:
		return "DA_response"
	case CE128_request:
		return "CE128_request"
	case CE128_response:
		return "CE128_response"
	case CE138_request:
		return "CE138_request"
	case CE138_response:
		return "CE138_response"
	case CE139_request:
		return "CE139_request"
	case CE139_response:
		return "CE139_response"
	default:
		return "unknown"
	}
}

const TickTime = 200

func (n *Node) PrintGuarantorAssignments() {
	for _, assign := range n.statedb.GuarantorAssignments {
		vid := n.statedb.GetSafrole().GetCurrValidatorIndex(assign.Validator.GetEd25519Key())
		fmt.Printf("v%d->c%v\n", vid, assign.CoreIndex)
	}
}
func (n *Node) SetSendTickets(sendTickets bool) {
	n.sendTickets = sendTickets
}

func (n *Node) writeDebug(obj interface{}, timeslot uint32) error {
	l := storage.LogMessage{
		Payload:  obj,
		Timeslot: timeslot,
	}
	return n.WriteLog(l)
}

func (n *Node) WriteLog(logMsg storage.LogMessage) error {
	//msgType := getStructType(obj)
	obj := logMsg.Payload
	timeSlot := logMsg.Timeslot
	msgType := getMessageType(obj)
	if msgType != "unknown" {
	}
	dataDir := fmt.Sprintf("%s/data", n.dataDir)
	structDir := fmt.Sprintf("%s/%vs", dataDir, msgType)

	// Check if the directories exist, if not create them
	if _, err := os.Stat(structDir); os.IsNotExist(err) {
		err := os.MkdirAll(structDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("Error creating %v directory: %v\n", msgType, err)
		}
	}

	if msgType != "unknown" {
		epoch, phase := statedb.ComputeEpochAndPhase(timeSlot, n.epoch0Timestamp)
		path := fmt.Sprintf("%s/%v_%03d", structDir, epoch, phase)
		if epoch == 0 && phase == 0 {
			path = fmt.Sprintf("%s/genesis", structDir)
		}
		types.SaveObject(path, obj)
	}
	return nil
}

func (n *Node) runClient() {

	ticker_pulse := time.NewTicker(TickTime * time.Millisecond)
	defer ticker_pulse.Stop()

	logChan := n.store.GetChan()
	n.statedb.GetSafrole().EpochFirstSlot = n.epoch0Timestamp / types.SecondsPerSlot
	for {
		select {

		case <-ticker_pulse.C:
			if n.GetNodeType() != ValidatorFlag && n.GetNodeType() != ValidatorDAFlag {
				return
			}
			ticketIDs, err := n.GetSelfTicketsIDs()
			if err != nil {
				fmt.Printf("runClient: GetSelfTicketsIDs error: %v\n", err)
			}
			// timeslot mark
			// currJCE := common.ComputeCurrentJCETime()
			currJCE := common.ComputeTimeUnit(types.TimeUnitMode)
			currEpoch, currPhase := n.statedb.GetSafrole().EpochAndPhase(currJCE)
			if currEpoch != -1 {
				if currPhase == 0 {
					n.GenerateTickets()
					n.BroadcastTickets()

				} else if currPhase == types.EpochLength-1 { // you had currPhase == types.EpochLength-1
					// nextEpochFirst-endPhase <= currJCE <= nextEpochFirst
					if debug {
						fmt.Printf("[N%d]GenerateTickets currEpoch=%v, currPhase=%v\n", n.id, currEpoch, currPhase)
					}
					n.GenerateTickets()
					n.BroadcastTickets()

				}
			}
			//if n.checkGodTimeslotUsed(currJCE) {
			//	return
			//}

			newBlock, newStateDB, err := n.statedb.ProcessState(n.credential, ticketIDs, n.extrinsic_pool)
			if err != nil {
				fmt.Printf("[N%d] ProcessState ERROR: %v\n", n.id, err)
				panic(0)
			}

			if newStateDB != nil && debugTree {
				fmt.Printf("[N%d] Author PrintTree \n", n.id)
				newStateDB.GetTrie().PrintTree(newStateDB.GetTrie().Root, 0)
			}
			if newStateDB != nil {
				if n.checkGodTimeslotUsed(currJCE) {
					fmt.Printf("%s could author but blocked by god\n", n.String())
					return
				}
				n.sendGodTimeslotUsed(currJCE)
				// we authored a block
				oldstate := n.statedb
				//TODO: see what happens
				//newStateDB.RotateGuarantors()
				newStateDB.SetAncestor(newBlock.Header, oldstate)
				if debugAncestor {
					timeSlots1 := n.statedb.GetAncestorTimeSlot()
					timeSlots2 := newStateDB.GetAncestorTimeSlot()
					fmt.Printf("[N%d] Ancestor timeSlots1 %d\n", n.id, timeSlots1)
					fmt.Printf("[N%d] Ancestor timeSlots2 %d\n", n.id, timeSlots2)
				}

				n.addStateDB(newStateDB)
				n.StoreBlock(newBlock, n.id, true)
				n.cacheBlock(newBlock)
				headerHash := newBlock.Header.Hash()
				n.cacheHeaders(headerHash, newBlock)
				//fmt.Printf("%s BLOCK BROADCASTED: headerHash: %v (%v <- %v)\n", n.String(), headerHash, newBlock.ParentHash(), newBlock.Hash())
				n.broadcast(*newBlock)
				if newBlock.GetParentHeaderHash() == (common.Hash{}) {
				} else {
					block := newBlock.Copy()
					n.block_tree.AddBlock(block)
					//fmt.Printf("block tree block added, block %v(p:%v)\n", block.Header.Hash().String_short(), block.GetParentHeaderHash().String_short())
				}

				if debug {
					for _, g := range newStateDB.GuarantorAssignments {
						fmt.Printf("[N%d] GUARANTOR ASSIGNMENTS: %v -> core %v \n", n.id, g.Validator.Ed25519.String(), g.CoreIndex)
					}
				}
				timeslot := newStateDB.GetSafrole().Timeslot
				err := n.writeDebug(newBlock, timeslot)
				if err != nil {
					fmt.Printf("writeDebug Block err: %v\n", err)
				}
				s := n.statedb
				allStates := s.GetAllKeyValues()
				ok, err := s.CompareStateRoot(allStates, newBlock.Header.ParentStateRoot)
				if !ok || err != nil {
					log.Fatalf("Error CompareStateRoot %v\n", err)
				}

				st := buildStateTransitionStruct(oldstate, newBlock, newStateDB)
				err = n.writeDebug(st, timeslot) // StateTransition
				if err != nil {
					fmt.Printf("writeDebug StateTransition err: %v\n", err)
				}

				if debugSTF {
					err = statedb.CheckStateTransition(n.store, st, s.AncestorSet)
					if err != nil {
						panic(fmt.Sprintf("ERROR validating state transition\n"))
					} else if debug {
						fmt.Printf("Validated state transition\n")
					}
				}

				// store StateSnapshot
				err = n.writeDebug(newStateDB.JamState.Snapshot(), timeslot) // StateSnapshot
				if err != nil {
					fmt.Printf("writeDebug StateSnapshot err: %v\n", err)
				}

				// Author is assuring the new block, resulting in a broadcast assurance with anchor = newBlock.Hash()
				n.assureNewBlock(newBlock)
				n.statedbMapMutex.Lock()
				n.auditingCh <- n.statedbMap[n.statedb.HeaderHash].Copy()
				n.statedbMapMutex.Unlock()
				IsTicketSubmissionClosed := n.statedb.GetSafrole().IsTicketSubmissionClosed(n.statedb.GetTimeslot())
				n.extrinsic_pool.RemoveUsedExtrinsicFromPool(newBlock, n.statedb.GetSafrole().Entropy[2], IsTicketSubmissionClosed)

			}

		case log := <-logChan:
			//fmt.Printf("IM here!!! %v\n", log)
			n.WriteLog(log)
		}
	}
}

func buildStateTransitionStruct(oldStateDB *statedb.StateDB, newBlock *types.Block, newStateDB *statedb.StateDB) *statedb.StateTransition {

	st := statedb.StateTransition{
		PreState: statedb.StateSnapshotRaw{
			KeyVals:   oldStateDB.GetAllKeyValues(),
			StateRoot: oldStateDB.StateRoot,
		},
		Block: *newBlock,
		PostState: statedb.StateSnapshotRaw{
			KeyVals:   newStateDB.GetAllKeyValues(),
			StateRoot: newStateDB.StateRoot,
		},
	}

	return &st
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

// SplitDataIntoSegmentAndPageProof splits the data into segment and page proof
func SplitDataIntoSegmentAndPageProof(data [][]byte) (segment [][]byte, pageProof [][]byte) {
	totalData := len(data)
	var totalPageProofs int
	var totalSegments int

	// Initial estimate of totalPageProofs
	totalPageProofs = (totalData + 63) / 65

	// Iteratively compute totalPageProofs and totalSegments
	for {
		totalSegments = totalData - totalPageProofs
		newTotalPageProofs := (totalSegments + 63) / 64
		if newTotalPageProofs == totalPageProofs {
			break
		}
		totalPageProofs = newTotalPageProofs
	}

	// Return the segment and its corresponding page proof
	return data[:totalSegments+1], data[totalSegments+1:]
}

// SplitDataIntoSegmentAndPageProof splits the data into segment and page proof
func SplitDataIntoSegmentAndPageProofByIndex(data [][]byte, segmentIndex uint32) (segment []byte, pageProof []byte) {
	totalData := len(data)
	var totalPageProofs int
	var totalSegments int

	// Initial estimate of totalPageProofs
	totalPageProofs = (totalData + 63) / 65

	// Iteratively compute totalPageProofs and totalSegments
	for {
		totalSegments = totalData - totalPageProofs
		newTotalPageProofs := (totalSegments + 63) / 64
		if newTotalPageProofs == totalPageProofs {
			break
		}
		totalPageProofs = newTotalPageProofs
	}

	// Compute the group index
	groupIndex := segmentIndex / 64

	// Compute the page proof position
	pageProofPosition := totalSegments + int(groupIndex)

	// Return the segment and its corresponding page proof
	return data[segmentIndex], data[pageProofPosition]
}
