package types

import (
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/common"
)

/* "target": "0x99841f584606ab31badcdf38e8122874a699e0cb3989d8ddc7c0874b8f5f76bf",
   "key": "0x3b6a27bcceb6a42d62a3a8d02a6f0d73653215771de243a63ac048a18b59da29",
   "signature": "0x99841f584606ab31badcdf38e8122874a699e0cb3989d8ddc7c0874b8f5f76bfc87c17f29dfbde33cfe599f3fb71d0fc211140801080ec105fbf435a977f784a"
*/

const (
	debugSig = false
)

func TestVerify(t *testing.T) {
	ReportHash := common.FromHex("376fd230cc3710d737382e6e7cf4eaa46f3bf89589e061492e8ffdf6bb5f90d4")
	key := common.FromHex("22351e22105a19aabb42589162ad7f1ea0df1c25cebf0e4a9fcd261301274862")
	signature := common.FromHex("0391803965e847d3781f8ede650068acbafefe79b57172b2ed382eabd11594af9cce3955cb24a926ca805984945a59e1269efe041e7fedb00d920b3946443101")
	if debugSig {
		fmt.Printf("ReportHash: %x\n", ReportHash)
		fmt.Printf("key: %x\n", key)
		fmt.Printf("signature: %x\n", signature)
	}
	signtext := append([]byte(X_G), ReportHash...)
	verify := Ed25519Verify(Ed25519Key(key), signtext, Ed25519Signature(signature))
	if !verify {
		t.Errorf("Verify failed")
	}

}
