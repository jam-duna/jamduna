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

/*
	"target": "0x376fd230cc3710d737382e6e7cf4eaa46f3bf89589e061492e8ffdf6bb5f90d4",
    "key": "0x22351e22105a19aabb42589162ad7f1ea0df1c25cebf0e4a9fcd261301274862",
    "signature": "0x0391803965e847d3781f8ede650068acbafefe79b57172b2ed382eabd11594af9cce3955cb24a926ca805984945a59e1269efe041e7fedb00d920b3946443101"
*/

func TestVerify(t *testing.T) {
	ReportHash := common.FromHex("1182df70471c8623d4b057e64d73c9f428524c5d9844a9ecccadb079132f9717")
	fmt.Printf("ReportHash: %x\n", ReportHash)
	key := common.FromHex("837ce344bc9defceb0d7de7e9e9925096768b7adb4dad932e532eb6551e0ea02")
	fmt.Printf("key: %x\n", key)
	signature := common.FromHex("f8eda0f28e7a6e6119b34e84e2a545a63d2cfb9e655c11a59b402d19935dbc686f6a1630300fec51598e2451280d3942e87f7926887d64ea93958f4dd8c82f00")
	fmt.Printf("signature: %x\n", signature)
	signtext := append([]byte(X_G), ReportHash...)
	verify := Ed25519Verify(Ed25519Key(key), signtext, Ed25519Signature(signature))
	if !verify {
		t.Errorf("Verify failed")
	}

}
