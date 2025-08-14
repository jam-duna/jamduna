package fuzz

import (
	"github.com/colorfulnotion/jam/statedb"
)

const (
	JamDunaImplementationsRPCPort = ":8088" // Implementations RPC
	InternalRPCPort               = ":8089" // Internal for fuzzing & validation
)

type FuzzedResult struct {
	Mutated bool
	STF     *statedb.StateTransition
}

type ValidationResult struct {
	Valid bool                    `json:"valid"`
	Error string                  `json:"error,omitempty"`
	STF   statedb.StateTransition `json:"stf,omitempty"`
}
