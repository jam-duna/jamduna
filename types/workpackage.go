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

// C.28 - 14.2 WorkPackage represents a work package.
type WorkPackage struct {
	// h, u, c, |j, |f, |w
	// $h$ - the index of the service which hosts the authorization code
	AuthCodeHost uint32 `json:"auth_code_host"`
	// $u$ - an authorization code hash
	AuthorizationCodeHash common.Hash `json:"auth_code_hash"`
	// $c$ - context
	RefineContext RefineContext `json:"context"`
	// $j$ - a simple blob acting as an authorization token
	AuthorizationToken []byte `json:"authorization"`
	// $f$ - configuration blob
	ConfigurationBlob []byte `json:"authorizer_config"` //configuration_blob
	// $w$ - a sequence of work items
	WorkItems []WorkItem `json:"items"`
}

type WPQueueItem struct {
	WorkPackage        WorkPackage
	CoreIndex          uint16
	Extrinsics         ExtrinsicsBlobs
	AddTS              int64
	NextAttemptAfterTS int64
	NumFailures        int
	Slot               uint32 // the slot for which this work package is intended
	EventID            uint64
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
		AuthCodeHost          uint32        `json:"auth_code_host"`
		AuthorizationCodeHash common.Hash   `json:"auth_code_hash"`
		RefineContext         RefineContext `json:"context"`
		AuthorizationToken    string        `json:"authorization"`
		ConfigurationBlob     string        `json:"authorizer_config"`
		WorkItems             []WorkItem    `json:"items"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	a.AuthCodeHost = s.AuthCodeHost
	a.AuthorizationCodeHash = s.AuthorizationCodeHash
	a.RefineContext = s.RefineContext
	a.AuthorizationToken = common.FromHex(s.AuthorizationToken)
	a.ConfigurationBlob = common.FromHex(s.ConfigurationBlob)
	a.WorkItems = s.WorkItems

	return nil
}

func (a WorkPackage) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		AuthCodeHost          uint32        `json:"auth_code_host"`
		AuthorizationCodeHash common.Hash   `json:"auth_code_hash"`
		RefineContext         RefineContext `json:"context"`
		Authorization         string        `json:"authorization"`
		ConfigurationBlob     string        `json:"authorizer_config"`
		WorkItems             []WorkItem    `json:"items"`
	}{
		AuthCodeHost:          a.AuthCodeHost,
		AuthorizationCodeHash: a.AuthorizationCodeHash,
		RefineContext:         a.RefineContext,
		Authorization:         common.HexString(a.AuthorizationToken),
		ConfigurationBlob:     common.HexString(a.ConfigurationBlob),
		WorkItems:             a.WorkItems,
	})
}

// Clone creates a deep copy of the WorkPackage
func (a *WorkPackage) Clone() WorkPackage {
	clone := WorkPackage{
		AuthCodeHost:          a.AuthCodeHost,
		AuthorizationCodeHash: a.AuthorizationCodeHash,
		RefineContext:         a.RefineContext,
	}

	// Deep copy byte slices
	if a.AuthorizationToken != nil {
		clone.AuthorizationToken = make([]byte, len(a.AuthorizationToken))
		copy(clone.AuthorizationToken, a.AuthorizationToken)
	}

	if a.ConfigurationBlob != nil {
		clone.ConfigurationBlob = make([]byte, len(a.ConfigurationBlob))
		copy(clone.ConfigurationBlob, a.ConfigurationBlob)
	}

	// Deep copy WorkItems slice
	if a.WorkItems != nil {
		clone.WorkItems = make([]WorkItem, len(a.WorkItems))
		for i, item := range a.WorkItems {
			clone.WorkItems[i] = item.Clone()
		}
	}

	return clone
}
