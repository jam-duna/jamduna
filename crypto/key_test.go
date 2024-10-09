package key

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/statedb"
	"testing"
)

/*
   type ValidatorSecret struct {
	BandersnatchPub    BandersnatchKey           `json:"bandersnatch"`
	Ed25519Pub         Ed25519Key                `json:"ed25519"`
	BlsPub             [BlsPubInBytes]byte       `json:"bls"`
	Metadata           [MetadataSizeInBytes]byte `json:"metadata"`
	BandersnatchSecret []byte                    `json:"bandersnatch_priv"`
	Ed25519Secret      [Ed25519PrivInBytes]byte  `json:"ed25519_priv"`
	BlsSecret          [BlsPrivInBytes]byte      `json:"bls_priv"`
}
*/

func TestCrypto(t *testing.T) {
	seed := make([]byte, 32)
	v, err := statedb.InitValidatorSecret(seed, seed, seed, "")
	if err != nil {
		t.Fatalf("%v", err)
	}
	jsonData, err := json.MarshalIndent(v, "", "  ") // Marshal with indentation for pretty-print
	if err != nil {
		t.Fatalf("Failed to marshal ValidatorSecret: %v", err)
	}
	fmt.Println(string(jsonData)) // Print the marshalled JSON

}
