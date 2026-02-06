package types

import (
	"testing"

	"github.com/jam-duna/jamduna/common"
)

func TestUP1AnnouncementSerialization(t *testing.T) {
	configBlob := []byte("test_config_blob")
	authCodeHash := common.Blake2Hash([]byte("test_auth_code"))
	authHash := common.Blake2Hash(append(authCodeHash[:], configBlob...))

	ann := UP1Announcement{
		ServiceID:             42,
		CoreIndex:             7,
		AuthorizationCodeHash: authCodeHash,
		AuthorizerHash:        authHash,
		ConfigurationBlob:     configBlob,
		RollupBlock:           []byte("rollup_block"),
		Checkpoint:            1,
	}

	bytes, err := ann.ToBytes()
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}

	var decoded UP1Announcement
	err = decoded.FromBytes(bytes)
	if err != nil {
		t.Fatalf("Failed to deserialize: %v", err)
	}

	if decoded.ServiceID != ann.ServiceID {
		t.Errorf("ServiceID mismatch: got %d, want %d", decoded.ServiceID, ann.ServiceID)
	}
	if decoded.CoreIndex != ann.CoreIndex {
		t.Errorf("CoreIndex mismatch: got %d, want %d", decoded.CoreIndex, ann.CoreIndex)
	}
	if decoded.AuthorizerHash != ann.AuthorizerHash {
		t.Errorf("AuthorizerHash mismatch")
	}
	if decoded.AuthorizationCodeHash != ann.AuthorizationCodeHash {
		t.Errorf("AuthorizationCodeHash mismatch")
	}
	if string(decoded.ConfigurationBlob) != string(ann.ConfigurationBlob) {
		t.Errorf("ConfigurationBlob mismatch")
	}
	if string(decoded.RollupBlock) != string(ann.RollupBlock) {
		t.Errorf("RollupBlock mismatch")
	}
	if decoded.Checkpoint != ann.Checkpoint {
		t.Errorf("Checkpoint mismatch: got %d, want %d", decoded.Checkpoint, ann.Checkpoint)
	}
}

func TestUP1AnnouncementAuthorizerVerification(t *testing.T) {
	configBlob := []byte("test_config_blob")
	authCodeHash := common.Blake2Hash([]byte("test_auth_code"))
	authHash := common.Blake2Hash(append(authCodeHash[:], configBlob...))

	ann := UP1Announcement{
		ServiceID:             42,
		CoreIndex:             7,
		AuthorizationCodeHash: authCodeHash,
		AuthorizerHash:        authHash,
		ConfigurationBlob:     configBlob,
		RollupBlock:           []byte("rollup_block"),
		Checkpoint:            0,
	}

	if !ann.VerifyAuthorizerPreimage() {
		t.Error("Valid authorizer preimage should verify successfully")
	}

	ann.AuthorizerHash = common.Hash{}
	if ann.VerifyAuthorizerPreimage() {
		t.Error("Invalid authorizer preimage should fail verification")
	}
}

func TestUP1AnnouncementEmptyRollupBlock(t *testing.T) {
	configBlob := []byte("test_config")
	authCodeHash := common.Blake2Hash([]byte("test_auth_code"))
	authHash := common.Blake2Hash(append(authCodeHash[:], configBlob...))

	ann := UP1Announcement{
		ServiceID:             1,
		CoreIndex:             0,
		AuthorizationCodeHash: authCodeHash,
		AuthorizerHash:        authHash,
		ConfigurationBlob:     configBlob,
		RollupBlock:           []byte{},
		Checkpoint:            0,
	}

	bytes, err := ann.ToBytes()
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}

	var decoded UP1Announcement
	err = decoded.FromBytes(bytes)
	if err != nil {
		t.Fatalf("Failed to deserialize: %v", err)
	}

	if len(decoded.RollupBlock) != 0 {
		t.Errorf("Expected empty rollup block, got %d bytes", len(decoded.RollupBlock))
	}
}
