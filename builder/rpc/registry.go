// Package rpc provides unified RPC namespace registration for JAM builders
package rpc

import (
	"net/rpc"

	evmrpc "github.com/colorfulnotion/jam/builder/evm/rpc"
	evmwitness "github.com/colorfulnotion/jam/builder/evm/witness"
)

// BuilderRPCRegistry manages all builder RPC namespace registrations
// Provides clean separation between JAM network RPC and user-facing RPC
type BuilderRPCRegistry struct {
	server *rpc.Server
}

// NewBuilderRPCRegistry creates a new builder RPC registry
func NewBuilderRPCRegistry(server *rpc.Server) *BuilderRPCRegistry {
	return &BuilderRPCRegistry{
		server: server,
	}
}

// RegisterEVMOnly registers only EVM RPC methods
func (r *BuilderRPCRegistry) RegisterEVMOnly(evmBuilder *evmwitness.EVMBuilder) error {
	return evmrpc.RegisterEthereumRPC(r.server, evmBuilder)
}
