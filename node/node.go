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

	//"github.com/colorfulnotion/jam/statedb"

	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/safrole"
	"github.com/quic-go/quic-go"
	"golang.org/x/crypto/blake2b"
	"io"
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

type QuicMessage struct {
	Id      int
	MsgType string `json:"msgType"`
	Payload string `json:"payload"`
}

type NodeInfo struct {
	PeerID     int               `json:"peer_id"`
	PeerAddr   string            `json:"peer_addr"`
	RemoteAddr string            `json:"remote_addr"`
	Validator  safrole.Validator `json:"validator"`
}

type Node struct {
	id         int
	credential safrole.ValidatorSecret
	server     quic.Listener
	peers      []string
	peersInfo  map[string]NodeInfo //<ed25519> -> NodeInfo
	//peersAddr  	 map[string]string
	tlsConfig    *tls.Config
	mutex        sync.Mutex
	VMs          map[uint32]*pvm.VM
	connections  map[string]quic.Connection
	streams      map[string]quic.Stream
	store        *storage.StateDBStorage /// where to put this?
	statedb      *statedb.StateDB
	connectionMu sync.Mutex
	messageChan  chan statedb.Message
}

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

func (n *Node) setValidatorCredential(credential safrole.ValidatorSecret) {
	n.credential = credential
	jsonData, err := json.Marshal(credential)
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Printf("[N%v] credential %s\n", n.id, jsonData)
}

func newNode(id int, credential safrole.ValidatorSecret, genesisConfig *safrole.GenesisConfig, peers []string, peerList map[string]NodeInfo) (*Node, error) {
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
	fmt.Printf("[N%v] OPENING %s\n", id, addr)
	listener, err := quic.ListenAddr(addr, tlsConfig, generateQuicConfig())

	if err != nil {
		fmt.Printf("ERR %v\n", err)
		return nil, err
	}

	messageChan := make(chan statedb.Message, 100)

	node := &Node{
		id: id,
		//credential:  credential,
		store:       store,
		server:      *listener,
		peers:       peers,
		peersInfo:   peerList,
		tlsConfig:   tlsConfig,
		connections: make(map[string]quic.Connection),
		streams:     make(map[string]quic.Stream),
		messageChan: messageChan,
	}
	_, _statedb, err := statedb.NewGenesisStateDB(node.store, genesisConfig)
	if err == nil {
		_statedb.SetID(id)
		_statedb.SetCredential(credential)
		_statedb.OpenMsgChannel(messageChan)
		node.statedb = _statedb
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

func (n *Node) getState() *statedb.StateDB {
	return n.statedb
}

func (n *Node) getPeerIndex(identifier string) (int, error) {
	peer, exist := n.peersInfo[identifier]
	if exist {
		return peer.PeerID, nil
	}
	return 0, fmt.Errorf("peer not found")
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

	fmt.Printf("[N%v] handleConnection. peerAddr=%v\n", n.id, peerAddr)

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
		fmt.Printf("[N%v] AcceptStream. peerAddr=%v\n", n.id, peerAddr)
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

	fmt.Printf(" -- [N%v] handleStream Read From N%v (msgType=%v)\n", n.id, msg.Id, msg.MsgType)
	response := []byte("1")
	ok := []byte("0")
	switch msg.MsgType {
	case "Ping":
		var ping Ping
		err := json.Unmarshal([]byte(msg.Payload), &ping)
		if err == nil {
			err = n.processPing(ping)
			if err == nil {
				response = ok
			}
		}
	case "Pong":
		var pong Pong
		err := json.Unmarshal([]byte(msg.Payload), &pong)
		if err == nil {
			err = n.processPong(pong)
			if err == nil {
				response = ok
			}
		}
	case "ImportDAQuery":
		var query ImportDAQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err == nil {
			r := ImportDAResponse{Data: [][]byte{[]byte("dummy data")}, Proof: BMTProof{}}
			serializedR, err := json.Marshal(r)
			if err == nil {
				response = serializedR
			}
		}
	case "AuditDAQuery":
		var query AuditDAQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err == nil {
			r := AuditDAResponse{Data: []byte("dummy data"), Proof: BMTProof{}}
			serializedR, err := json.Marshal(r)
			if err == nil {
				response = serializedR
			}
		}
	case "ImportDAReconstructQuery":
		var query ImportDAReconstructQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err == nil {
			r := ImportDAReconstructResponse{Data: []byte("dummy data")}
			serializedR, err := json.Marshal(r)
			if err == nil {
				response = serializedR
			}
		}
	case "Ticket":
		var ticket *safrole.Ticket
		err := json.Unmarshal([]byte(msg.Payload), &ticket)
		if err == nil {
			err = n.processTicket(ticket)
			if err == nil {
				response = ok
			}
		}
	case "Guarantee":
		var guarantee Guarantee
		err := json.Unmarshal([]byte(msg.Payload), &guarantee)
		if err == nil {
			err = n.processGuarantee(guarantee)
			if err == nil {
				response = ok
			}
		}
	case "Assurance":
		var assurance Assurance
		err := json.Unmarshal([]byte(msg.Payload), &assurance)
		if err == nil {
			err = n.processAssurance(assurance)
			if err == nil {
				response = ok
			}
		}
	case "Disputes":
		var disputes Disputes
		err := json.Unmarshal([]byte(msg.Payload), &disputes)
		if err == nil {
			err = n.processDisputes(disputes)
			if err == nil {
				response = ok
			}
		}
	case "PreimageLookup":
		var preimageLookup PreimageLookup
		err := json.Unmarshal([]byte(msg.Payload), &preimageLookup)
		if err == nil {
			err = n.processPreimageLookup(preimageLookup)
			if err == nil {
				response = ok
			}
		}
	case "Block":
		var block *statedb.Block
		err := json.Unmarshal([]byte(msg.Payload), &block)
		if err == nil {
			err = n.processBlock(block)
			if err == nil {
				response = ok
			}
		}
	case "Announcement":
		var announcement Announcement
		err := json.Unmarshal([]byte(msg.Payload), &announcement)
		if err == nil {
			err = n.processAnnouncement(announcement)
			if err == nil {
				response = ok
			}
		}
	case "WorkPackage":
		var workPackage WorkPackage
		err := json.Unmarshal([]byte(msg.Payload), &workPackage)
		if err == nil {
			err = n.processWorkPackage(workPackage)
			if err == nil {
				response = ok
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

func (n *Node) broadcast(obj interface{}) {
	for _, peer := range n.peersInfo {
		peerIdentifier := peer.Validator.Ed25519.String()
		if peer.PeerID == n.id {
			continue
		}
		//fmt.Printf("PeerID=%v, peerIdentifier=%v\n", peer.PeerID, peerIdentifier)
		_, err := n.makeRequest(peerIdentifier, obj)
		if err != nil {
			fmt.Printf("runClient request error: %v\n", err)
			continue
		}
	}
}

func (n *Node) makeRequest(peerIdentifier string, obj interface{}) ([]byte, error) {
	peerAddr, _ := n.getPeerAddr(peerIdentifier)
	peerID, _ := n.getPeerIndex(peerIdentifier)
	msgType := getMessageType(obj)
	if msgType == "unknown" {
		return nil, fmt.Errorf("unsupported type")
	}
	fmt.Printf("[N%v] makeRequest %v to [N%v]\n", n.id, msgType, peerID)
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

	fmt.Printf(" -- [N%v] makeRequest WRITE %s => N%v\n", n.id, msgType, peerID)

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
	fmt.Printf("-- [N%v] makeRequest READ RESPONSE %d bytes from N%v\n", n.id, buffer.Len(), peerID)
	fmt.Printf(" -- [N%v] makeRequest WRITE %s => N%v\n", n.id, msgType, peerID)
	stream.Close()
	return nil, nil
}

// Helper function to determine if the error is a timeout error
func isTimeoutError(err error) bool {
	// Add more specific error handling as needed
	return strings.Contains(err.Error(), "timeout")
}

// should process ticket being moved to safrole
func (n *Node) processTicket(ticket *safrole.Ticket) error {
	s := n.getState()
	s.ProcessIncomingTicket(ticket)
	return nil // Success
}

func (n *Node) processGuarantee(guarantee Guarantee) error {
	// TODO: Store the lookup in a E_G aggregator
	return nil // Success
}

func (n *Node) processAssurance(assurance Assurance) error {
	// TODO: Store the lookup in a E_A aggregator
	return nil // Success
}

func (n *Node) processDisputes(disputes Disputes) error {
	// TODO: Store the lookup in a E_D aggregator
	return nil // Success
}

func (n *Node) processPreimageLookup(preimageLookup PreimageLookup) error {
	// TODO: Store the lookup in a E_P aggregator
	return nil // Success
}

func (n *Node) processBlock(blk *statedb.Block) error {
	s := n.getState()
	s.ProcessIncomingBlock(blk)
	//fmt.Printf("[N%v] processBlock %v\n", n.id, blk)
	return nil // Success
}

func (n *Node) processPing(ping Ping) (err error) {
	peerIndex, _ := n.getPeerIndex(ping.Sender)
	fmt.Printf("[N%v] processPing N%v %s\n", n.id, peerIndex, ping.Message)
	rspMsg := fmt.Sprintf("N%v Received N%v Ping message", n.id, peerIndex)
	pong := Pong{
		Sender:   n.getIdentifier(),
		Response: []byte(rspMsg),
	}

	_, err = n.makeRequest(ping.Sender, pong)
	if err != nil {
		fmt.Printf("Error sending Pong: N%v\n", err)
	}
	return err // Success
}

func (n *Node) processPong(pong Pong) error {
	fmt.Printf("[N%v] processPong %s - acknowledge\n", n.id, pong.Response)
	return nil // Success
}

func (n *Node) processAnnouncement(announcement Announcement) error {
	// TODO: TBD
	return nil // Success
}

func (n *Node) processWorkPackage(workPackage WorkPackage) error {
	// TODO: Incorporate WP chunks
	return nil // Success
}

func getMessageType(obj interface{}) string {
	switch obj.(type) {
	case ImportDAQuery:
		return "ImportDAQuery"
	case AuditDAQuery:
		return "AuditDAQuery"
	case ImportDAReconstructQuery:
		return "ImportDAReconstructQuery"
	case Guarantee:
		return "Guarantee"
	case Assurance:
		return "Assurance"
	case Disputes:
		return "Disputes"
	case PreimageLookup:
		return "PreimageLookup"
	case Ping:
		return "Ping"
	case Pong:
		return "Pong"
	case safrole.Ticket:
		return "Ticket"
	case statedb.Block:
		return "Block"
	case Announcement:
		return "Announcement"
	case WorkPackage:
		return "WorkPackage"
	default:
		return "unknown"
	}
}

const TickTime = 2000

func (n *Node) runClient() {

	ticker_pulse := time.NewTicker(TickTime * time.Millisecond)
	defer ticker_pulse.Stop()

	ticker_outgoing := time.NewTicker(TickTime * time.Millisecond)
	defer ticker_outgoing.Stop()

	for {
		select {
		case <-ticker_pulse.C:
			n.statedb.ProcessState()
			/*
				currJCE := safrole.ComputeCurrentJCETime()
				fmt.Printf("[N%v] currJCE at %v\n", n.id, currJCE)

				// Generate a random message
				fmt.Printf("[N%v] lifecheck at %s\n", n.id, time.Now().Format(time.RFC3339))
				message := n.generateRandomObject()
				fmt.Printf("[N%v] Generated message: %v\n", n.id, message)

				// Loop through each peer and send the message using makeRequest
				for _, peer := range n.peersInfo {
					peerIdentifier := peer.Validator.Ed25519.String()
					if peer.PeerID == n.id {
						continue
					}
					//fmt.Printf("PeerID=%v, peerIdentifier=%v\n", peer.PeerID, peerIdentifier)
					_, err := n.makeRequest(peerIdentifier, message)
					if err != nil {
						fmt.Printf("runClient request error: %v\n", err)
						continue
					}
				}
			*/
		case <-ticker_outgoing.C:
			//messages := n.statedb.GetOutgoingMessages()
			/*
				for _, peer := range n.peersInfo {
					peerIdentifier := peer.Validator.Ed25519.String()
					if peer.PeerID == n.id {
						continue
					}
					//fmt.Printf("PeerID=%v, peerIdentifier=%v\n", peer.PeerID, peerIdentifier)
					_, err := n.makeRequest(peerIdentifier, message)
					if err != nil {
						fmt.Printf("runClient request error: %v\n", err)
						continue
					}
				}
			*/
		case msg := <-n.messageChan:
			n.processOutgoingMessage(msg)
		}
	}
}

func (n *Node) processOutgoingMessage(msg statedb.Message) {
	// Normalize message type to handle case insensitivity
	msgType := msg.MsgType

	// Unmarshal the payload to the appropriate type
	switch msgType {
	case "Ticket":
		var ticket safrole.Ticket
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
		fmt.Printf("[N%v] Outgoing Ticket: %+v\n", n.id, ticket.TicketID())
		n.broadcast(ticket)
	case "Block":
		var blk statedb.Block
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
		fmt.Printf("[N%v] Outgoing Block: %+v\n", n.id, blk.Hash())
		n.broadcast(blk)
	default:
		fmt.Printf("[N%v] Unhandled message type: %v\n", n.id, msg.MsgType)
	}
}

func generateObject() interface{} {
	return nil
}

func (n *Node) generateRandomObject() interface{} {
	types := []string{
		//"ImportDAQuery", "AuditDAQuery", "ImportDAReconstructQuery", "Ticket", "Guarantee", "Assurance", "Disputes", "PreimageLookup", "Block", "Announcement", "WorkPackage",
		"Ping", //"Pong",
	}

	randomIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(types))))
	selectedType := types[randomIndex.Int64()]

	switch selectedType {
	case "ImportDAQuery":
		return ImportDAQuery{
			SegmentRoot:    bhash([]byte("segment_root")),
			SegmentIndex:   0,
			ProofRequested: true,
		}
	case "AuditDAQuery":
		return AuditDAQuery{
			SegmentRoot:    bhash([]byte("segment_root")),
			Index:          0,
			ProofRequested: true,
		}
	case "ImportDAReconstructQuery":
		return ImportDAReconstructQuery{
			SegmentRoot: bhash([]byte("segment_root")),
		}
	case "Guarantee":
		return Guarantee{
			WorkReport:  WorkReport{},
			TimeSlot:    1,
			Credentials: []GuaranteeCredential{},
		}
	case "Assurance":
		return Assurance{
			ParentHash:     bhash([]byte("parent_hash")),
			Bitstring:      []byte("bitstring"),
			ValidatorIndex: 1,
			Signature:      Ed25519Signature{},
		}
	case "Disputes":
		return Disputes{
			Verdicts: []Verdict{
				{WorkReportHash: bhash([]byte("work_report_hash")), Epoch: 1},
			},
			Culprits: []Culprit{
				{R: PublicKey{}, K: PublicKey{}, Signature: Ed25519Signature{}},
			},
			Faults: []Fault{
				{R: PublicKey{}, K: PublicKey{}, V: PublicKey{}, Signature: Ed25519Signature{}},
			},
		}
	case "PreimageLookup":
		return PreimageLookup{
			ServiceIndex: 0,
			Data:         []byte("lookup"),
		}
	case "Announcement":
		return Announcement{
			Signature: Ed25519Signature{},
		}
	case "WorkPackage":
		return WorkPackage{
			AuthorizationToken: []byte("exampleToken"),
			ServiceIndex:       0,
			AuthorizationCode:  bhash([]byte("authorization_code")),
			ParamBlob:          []byte("param_blob"),
			Context:            []byte("context"),
			WorkItems:          []WorkItem{},
		}
	case "Ping":
		msg := fmt.Sprintf("PINGGGG @ %s!!!", time.Now().Format(time.RFC3339))
		return Ping{
			Sender:  n.getIdentifier(),
			Message: []byte(msg),
		}
	case "Pong":
		msg := fmt.Sprintf("PONGGGG @ %s!!!", time.Now().Format(time.RFC3339))
		return Pong{
			Sender:   n.getIdentifier(),
			Response: []byte(msg),
		}
	default:
		return nil
	}
}
