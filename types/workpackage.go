package types

import (
	"encoding/json"

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
	Authorization []byte `json:"authorization"`
	// $h$ - the index of the service which hosts the authorization code
	AuthCodeHost uint32 `json:"auth_code_host"`
	// $u$ - an authorization code hash
	AuthorizationCodeHash common.Hash `json:"authorization_code_hash"`
	// $p$ - a parameterization blob
	ParameterizationBlob []byte `json:"parameterization_blob"`
	// $x$ - context
	RefineContext RefineContext `json:"context"`
	// $w$ - a sequence of work items
	WorkItems []WorkItem `json:"items"`
}

func (a *WorkPackage) String() string {
	return ToJSON(a)
}

// Bytes returns the bytes of the Assurance
func (a *WorkPackage) Bytes() []byte {
	encode, err := Encode(a)
	if err != nil {
		return nil
	}
	return encode
}

func (a *WorkPackage) Hash() common.Hash {
	data := a.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.Blake2Hash(data)
}

func (a *WorkPackage) UnmarshalJSON(data []byte) error {
	var s struct {
		Authorization string        `json:"authorization"`
		AuthCodeHost  uint32        `json:"auth_code_host"`
		Authorizer    Authorizer    `json:"authorizer"`
		RefineContext RefineContext `json:"context"`
		WorkItems     []WorkItem    `json:"items"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	a.Authorization = common.FromHex(s.Authorization)
	a.AuthCodeHost = s.AuthCodeHost
	a.AuthorizationCodeHash = s.Authorizer.CodeHash
	a.ParameterizationBlob = s.Authorizer.Params
	a.RefineContext = s.RefineContext
	a.WorkItems = s.WorkItems

	return nil
}

func (a WorkPackage) MarshalJSON() ([]byte, error) {
	// Convert Authorization from []byte to hex string
	authorization := common.HexString(a.Authorization)

	return json.Marshal(&struct {
		Authorization string        `json:"authorization"`
		AuthCodeHost  uint32        `json:"auth_code_host"`
		Authorizer    Authorizer    `json:"authorizer"`
		RefineContext RefineContext `json:"context"`
		WorkItems     []WorkItem    `json:"items"`
	}{
		Authorization: authorization,
		AuthCodeHost:  a.AuthCodeHost,
		Authorizer: Authorizer{
			CodeHash: a.AuthorizationCodeHash,
			Params:   a.ParameterizationBlob,
		},
		RefineContext: a.RefineContext,
		WorkItems:     a.WorkItems,
	})
}
