package grandpa

import (
	"fmt"
	"testing"

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

func TestGrandpaTypes(t *testing.T) {
	authority_num := 8
	Priv, Pub, err := InitializeAuthoritySet(authority_num)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < authority_num; i++ {
		fmt.Printf("Authority %d: %s\n", i, Pub[i].String())
		fmt.Printf("Authority %d: %x\n", i, Priv[i])
	}
}
