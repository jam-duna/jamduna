package fuzz

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
)

func testFullMessageEncoding(t *testing.T, msg *Message, expectedFullMessage []byte) {
	t.Helper()

	encodedBody, err := encode(msg)
	if err != nil {
		t.Fatalf("encode() failed: %v", err)
	}
	byteStr, _ := json.Marshal(msg) // For debugging purposes, to see the JSON representation
	fmt.Printf("Expected message string: %v\n", string(byteStr))
	fmt.Printf("Expected full message: %x\n", expectedFullMessage)

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, uint32(len(encodedBody))); err != nil {
		t.Fatalf("binary.Write for length failed: %v", err)
	}
	buf.Write(encodedBody)
	fullMessage := buf.Bytes()

	if !bytes.Equal(fullMessage, expectedFullMessage) {
		t.Errorf("Full message encoding mismatch:\n got: %x\nwant: %x", fullMessage, expectedFullMessage)
	}
}

func TestEncodeDecodePeerInfo(t *testing.T) {
	peerInfo := PeerInfo{
		//FuzzVersion: 1,
		Name:       "fuzzer",
		AppVersion: ParseVersion("0.1.23"),
		JamVersion: ParseVersion("0.7.0"),
		//Features:    FeatureBundleRefinement,
	}
	msg := &Message{PeerInfo: &peerInfo}

	//0x0e000000 0x000666757a7a6572000117000606
	//^ length   ^ encoded-message
	expectedFullMessage := common.FromHex("0e000000000666757a7a6572000117000606")

	testFullMessageEncoding(t, msg, expectedFullMessage)

	encodedBody := expectedFullMessage[4:]
	decodedMsg, err := decode(encodedBody)
	if err != nil {
		t.Fatalf("decode() failed: %v", err)
	}

	if decodedMsg.PeerInfo == nil {
		t.Fatal("decode() result has nil PeerInfo field")
	}

	if !reflect.DeepEqual(*decodedMsg.PeerInfo, peerInfo) {
		t.Errorf("decode() output mismatch for PeerInfo:\n got: %+v\nwant: %+v", *decodedMsg.PeerInfo, peerInfo)
	}
}

func TestEncodeDecodeStateRoot(t *testing.T) {
	stateRootHash := common.HexToHash("0x4559342d3a32a8cbc3c46399a80753abff8bf785aa9d6f623e0de045ba6701fe")
	msg := &Message{StateRoot: &stateRootHash}

	//0x21000000 0x054559342d3a32a8cbc3c46399a80753abff8bf785aa9d6f623e0de045ba6701fe
	//^ length   ^ encoded-message
	expectedFullMessage := common.FromHex("21000000054559342d3a32a8cbc3c46399a80753abff8bf785aa9d6f623e0de045ba6701fe")
	testFullMessageEncoding(t, msg, expectedFullMessage)

	encodedBody := expectedFullMessage[4:]
	decodedMsg, err := decode(encodedBody)
	if err != nil {
		t.Fatalf("decode() failed: %v", err)
	}

	if decodedMsg.StateRoot == nil {
		t.Fatal("decode() result has nil StateRoot field")
	}

	if !reflect.DeepEqual(*decodedMsg.StateRoot, stateRootHash) {
		t.Errorf("decode() output mismatch for StateRoot:\n got: %+v\nwant: %+v", *decodedMsg.StateRoot, stateRootHash)
	}
}
