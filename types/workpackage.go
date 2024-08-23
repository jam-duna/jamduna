package types

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

/*
14.3. Packages and Items.  A work-package includes: (See Equation 174):
* ${\bf j}$ - a simple blob acting as an authorization token
* $h$ - the index of the service which hosts the authorization code h
* $c$ - an authorization code hash
* ${\bf p}$ - a parameterization blob
* $x$ - context
* ${\bf i}$ - a sequence of work items
*/

// WorkPackage represents a work package.
type WorkPackage struct {
	// $j$ - a simple blob acting as an authorization token
	AuthorizationToken []byte `json:"authorization_token"`
	// $h$ - the index of the service which hosts the authorization code
	ServiceIndex int `json:"service_index"`
	// $c$ - an authorization code hash
	AuthorizationCode common.Hash `json:"authorization_code"`
	// $p$ - a parameterization blob
	ParamBlob []byte `json:"param_blob"`
	// $x$ - context
	Context []byte `json:"context"`
	// $i$ - a sequence of work items

	WorkItems []WorkItem `json:"work_items"`
}

// Bytes returns the bytes of the Assurance
func (a *WorkPackage) Bytes() []byte {
	enc, err := json.Marshal(a)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (a *WorkPackage) Hash() common.Hash {
	data := a.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}
