package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"

	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io/ioutil"
	"os"

	"encoding/json"
	"encoding/pem"
	"fmt"
	"math"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"math/big"
	rand0 "math/rand"
	"strings"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/trie"
	"github.com/quic-go/quic-go"
)

const (
	numNodes = 6
	quicAddr = "localhost:%d"
)

const (
	ValidatorFlag   = "VALIDATOR"
	DAFlag          = "DA"
	ValidatorDAFlag = "VALIDATOR&DA"
)

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

	// holds a map of epoch (use entropy to control it) to at most 2 tickets
	selfTickets  map[common.Hash][]types.TicketBucket
	ticketsMutex sync.Mutex
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
	dataDir      string

	epoch0Timestamp uint32
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

func NewNode(nodeName string, credential types.ValidatorSecret, genesisConfig *statedb.GenesisConfig, peers []string, peerList map[string]NodeInfo, dataDir string) (*Node, error) {
	n, err := newNode(0, credential, genesisConfig, peers, peerList, ValidatorFlag, dataDir)
	return n, err
}

func newNode(id uint32, credential types.ValidatorSecret, genesisConfig *statedb.GenesisConfig, peers []string, peerList map[string]NodeInfo, nodeType string, dataDir string) (*Node, error) {
	fmt.Printf("[N%v] Using levelDB path=%v\n", id, dataDir)
	levelDBPath := fmt.Sprintf("%v/leveldb/", dataDir)
	store, err := storage.NewStateDBStorage(levelDBPath)
	if err != nil {
		return nil, err
	}

	var cert tls.Certificate
	ed25519_priv := ed25519.PrivateKey(credential.Ed25519Secret[:])
	ed25519_pub := ed25519_priv.Public().(ed25519.PublicKey)

	cert, err = generateSelfSignedCert(ed25519_pub, ed25519_priv)
	if err != nil {
		return nil, fmt.Errorf("Error generating self-signed certificate: %v", err)
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
		selfTickets: make(map[common.Hash][]types.TicketBucket),
		dataDir:     dataDir,
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
	if genesisConfig != nil && genesisConfig.Epoch0Timestamp > 0 {
		node.epoch0Timestamp = uint32(genesisConfig.Epoch0Timestamp)
	}
	err = node.writeDebug(_statedb.JamState.Snapshot(), 0xFFFFFFFF)
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

func (n *Node) GetEd25519Key() types.Ed25519Key {
	return types.Ed25519Key(n.credential.Ed25519Pub)
}

func (n *Node) GetEd25519Secret() []byte {
	return n.credential.Ed25519Secret[:]
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

func (n *Node) broadcast(obj interface{}) []byte {
	result := []byte{}
	for _, peer := range n.peersInfo {
		peerIdentifier := peer.Validator.Ed25519.String()
		if peer.PeerID == n.id {
			continue
		}
		//fmt.Printf("PeerID=%v, peerIdentifier=%v\n", peer.PeerID, peerIdentifier)
		resp, err := n.makeRequest(peerIdentifier, obj, types.QuicIndividualTimeout)
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
				peerIdentifier := peer.Validator.Ed25519.String()
				if peer.PeerID == n.id {
					continue
				}
				//fmt.Printf("PeerID=%v, peerIdentifier=%v\n", peer.PeerID, peerIdentifier)
				resp, err := n.makeRequest(peerIdentifier, obj, types.QuicOverallTimeout)
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
	peerIdentifier := peer.Validator.Ed25519.String()
	resp, err := n.makeRequest(peerIdentifier, obj, types.QuicIndividualTimeout)
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
	case types.GuaranteeReport:
		return "GuaranteeReport"
	case *statedb.StateDB:
		return "StateDB"
	case statedb.StateDB:
		return "StateDB"
	case *statedb.JamState:
		return "JamState"
	case statedb.JamState:
		return "JamState"
	case statedb.StateSnapshot:
		return "StateSnapshot"
	case *statedb.StateSnapshot:
		return "StateSnapshot"
	case *statedb.GenesisConfig:
		return "GenesisConfig"
	default:
		return "unknown"
	}
}

const TickTime = 2000

func (n *Node) writeDebug(obj interface{}, Timeslot uint32) error {
	//msgType := getStructType(obj)
	msgType := getMessageType(obj)
	if msgType != "unknown" {
	}
	dataDir := fmt.Sprintf("%s/data", n.dataDir)
	structDir := fmt.Sprintf("%s/%vs", dataDir, msgType)
	//fmt.Printf("!!!! writeDebug msgType=%v, structDir=%v\n", msgType, structDir)
	if msgType != "unknown" {
		epoch, phase := statedb.ComputeEpochAndPhase(Timeslot, n.epoch0Timestamp)
		path := fmt.Sprintf("%s/%v_%v", structDir, epoch, phase)
		if epoch == 0xFFFFFFFF || phase == 0xFFFFFFFF {
			path = fmt.Sprintf("%s/genesis", structDir)
		}
		if msgType == "Ticket" {
			if ticket, ok := obj.(*types.Ticket); ok {
				// Cast successful, you can now access ticket's methods or fields
				identifier, _ := ticket.TicketID() // Assuming TicketID() is a method of types.Ticket
				path = fmt.Sprintf("%v_%v", path, identifier)
			} else {
				// Handle case where obj is not a *types.Ticket
				return fmt.Errorf("expected types.Ticket but got %T", obj)
			}
		}
		if msgType == "Block" {
			if block, ok := obj.(*types.Block); ok {
				// Cast successful, you can now access ticket's methods or fields
				identifier := block.Hash() // Assuming TicketID() is a method of types.Ticket
				path = fmt.Sprintf("%v_%v", path, identifier)
			} else {
				// Handle case where obj is not a *types.Ticket
				return fmt.Errorf("expected types.Block but got %T", obj)
			}
		}
		jsonPath := fmt.Sprintf("%s.json", path)
		codecPath := fmt.Sprintf("%s.codec", path)
		//fmt.Printf("jsonPath=%v, codecPath=%v\n", jsonPath, codecPath)

		// Check if the directories exist, if not create them
		if _, err := os.Stat(structDir); os.IsNotExist(err) {
			err := os.MkdirAll(structDir, os.ModePerm)
			if err != nil {
				return fmt.Errorf("Error creating %v directory: %v\n", msgType, err)
			}
		}

		switch v := obj.(type) {
		default:
			jsonEncode, _ := json.MarshalIndent(v, "", "    ")
			codecEncode := types.Encode(v)

			//fmt.Printf("jsonEncode=%s \n", string(jsonEncode))
			//fmt.Printf("codecEncode=%x \n", codecEncode)

			err := ioutil.WriteFile(jsonPath, jsonEncode, 0644)
			if err != nil {
				return fmt.Errorf("Error writing block file: %v\n", err)
			}
			err = ioutil.WriteFile(codecPath, codecEncode, 0644)
			if err != nil {
				return fmt.Errorf("Error writing state file: %v\n", err)
			}
		}
	}
	return nil
}

func (n *Node) writeDebugState(newBlock *types.Block, newStateDB *statedb.StateDB) error {
	// save the new block as json, put it in to dir /block&state
	jsonBlock, _ := json.MarshalIndent(newBlock, "", "    ")
	jsonState, _ := json.MarshalIndent(newStateDB.JamState, "", "    ")
	// use epoch and phase
	epoch, phase := newStateDB.GetSafrole().EpochAndPhase(newStateDB.GetSafrole().Timeslot)

	dataDir := fmt.Sprintf("%s/data/", n.dataDir)

	blockDir := fmt.Sprintf("%s/blocks", dataDir)
	stateDir := fmt.Sprintf("%s/state", dataDir)

	// Check if the directories exist, if not create them
	if _, err := os.Stat(blockDir); os.IsNotExist(err) {
		err := os.MkdirAll(blockDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("Error creating block directory: %v\n", err)
		}
	}

	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		err := os.MkdirAll(stateDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("Error creating state directory: %v\n", err)
		}
	}

	blockPath := fmt.Sprintf("%s/block_%v_%v.json", blockDir, epoch, phase)
	statePath := fmt.Sprintf("%s/state_%v_%v.json", stateDir, epoch, phase)
	err := ioutil.WriteFile(blockPath, jsonBlock, 0644)
	if err != nil {
		return fmt.Errorf("Error writing block file: %v\n", err)
	}
	err = ioutil.WriteFile(statePath, jsonState, 0644)
	if err != nil {
		return fmt.Errorf("Error writing state file: %v\n", err)
	}
	return nil
}

func (n *Node) runClient() {

	debug := true
	ticker_pulse := time.NewTicker(TickTime * time.Millisecond)
	defer ticker_pulse.Stop()

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
			newBlock, newStateDB, err := n.statedb.ProcessState(n.credential, ticketIDs)
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
					fmt.Printf("[N%d] GUARANTOR ASSIGNMENTS: %v -> core %v \n", n.id, g.Validator.Ed25519.String(), g.CoreIndex)
				}

				currSlot := n.statedb.GetSafrole().Timeslot
				_, currPhase := n.statedb.GetSafrole().EpochAndPhase(currSlot)
				if currPhase >= types.EpochLength-1 {
					nextEpochSlot := n.statedb.GetSafrole().GetNextEpochFirst() + 2
					n.GenerateTickets(nextEpochSlot)
					n.CheckSelfTicketsIsIncluded(*newBlock, nextEpochSlot)
					n.BroadcastTickets(nextEpochSlot)
				}

				if newStateDB.Block.Header.EpochMark != nil {
					fmt.Printf("[N%d] EPOCH MARK: %v\n", n.id, newStateDB.Block.Header.EpochMark)
					currJCE := statedb.ComputeCurrentJCETime()
					n.GenerateTickets(currJCE)
					n.CheckSelfTicketsIsIncluded(*newBlock, currJCE)
					n.BroadcastTickets(currJCE)
				}
				// TODO Shawn: DO NOT WRITE ANYTHING into JAM repo
				if debug {
					timeslot := newStateDB.GetSafrole().Timeslot
					err := n.writeDebug(newBlock, timeslot)
					if err != nil {
						fmt.Printf("writeDebug Block err: %v\n", err)
					}
					err = n.writeDebug(newStateDB.JamState.Snapshot(), timeslot)
					if err != nil {
						fmt.Printf("writeDebug JamState err: %v\n", err)
					}
					//n.writeDebugState(newBlock, newStateDB)
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

func (n *Node) GetSegmentTreeRoots(erasureRoot common.Hash) ([]common.Hash, error) {
	hashs, err := n.store.ReadKV(erasureRoot)
	if err != nil {
		fmt.Println("Error in FetchWorkPackageAndExportedSegments:", err)
	}
	// fmt.Printf("allHash: %x\n", hashs)
	treeRoot := hashs[32:]

	segmentsECRoots, err := n.store.ReadKV(common.Hash(treeRoot))
	if err != nil {
		return nil, err
	}
	allHash := SplitBytesIntoHash(segmentsECRoots, len(common.Hash{}))
	segmentRoots, _ := splitHashes(allHash)
	return segmentRoots, nil
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
