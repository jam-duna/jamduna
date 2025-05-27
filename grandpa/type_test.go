package grandpa

import (
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/types"
)

func InitializeAuthoritySet(authority_num int) ([]types.Ed25519Priv, []types.Ed25519Key, error) {
	Pub := make([]types.Ed25519Key, authority_num)
	Priv := make([]types.Ed25519Priv, authority_num)
	for i := 0; i < authority_num; i++ {
		data := make([]byte, 32)
		data[0] = byte(i)
		pub, priv, err := types.InitEd25519Key(data)
		if err != nil {
			return nil, nil, err
		}
		Pub[i] = pub
		Priv[i] = priv
	}
	return Priv, Pub, nil
}

func InitializeBandersnatchSet(authority_num int) ([]bandersnatch.BanderSnatchSecret, []bandersnatch.BanderSnatchKey, error) {
	Pub := make([]bandersnatch.BanderSnatchKey, authority_num)
	Priv := make([]bandersnatch.BanderSnatchSecret, authority_num)
	for i := 0; i < authority_num; i++ {
		data := make([]byte, 32)
		data[0] = byte(i)
		pub, priv, err := bandersnatch.InitBanderSnatchKey(data)
		if err != nil {
			return nil, nil, err
		}
		Pub[i] = pub
		Priv[i] = priv
	}
	return Priv, Pub, nil
}

func TestGrandpaTypes(t *testing.T) {
	authority_num := 8
	Priv, Pub, err := InitializeAuthoritySet(authority_num)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < authority_num; i++ {
		fmt.Printf("Ed25519 Authority %d: %s\n", i, Pub[i].String())
		fmt.Printf("Ed25519 Authority %d: %x\n", i, Priv[i])
	}
	banderSnatchPriv, bandersnatchPub, err := InitializeBandersnatchSet(authority_num)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < authority_num; i++ {
		fmt.Printf("BanderSnatch Authority %d: %s\n", i, bandersnatchPub[i].String())
		fmt.Printf("BanderSnatch Authority %d: %x\n", i, banderSnatchPriv[i])
	}
}
