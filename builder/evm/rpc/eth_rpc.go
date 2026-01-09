// Package rpc provides EVM user-facing RPC server functionality
package rpc

import (
	"fmt"
	"net/rpc"

	log "github.com/colorfulnotion/jam/log"
)

// EVMRPCServer provides Ethereum JSON-RPC API implementation.
// This is a thin wrapper around EVMRPCHandler for net/rpc registration.
type EVMRPCServer struct {
	handler *EVMRPCHandler
}

// NewEVMRPCServer creates a new EVM RPC server instance.
func NewEVMRPCServer(rollup *Rollup, txPool *TxPool) *EVMRPCServer {
	return &EVMRPCServer{
		handler: NewEVMRPCHandler(rollup, txPool),
	}
}

// ===== Network Metadata =====

func (s *EVMRPCServer) ChainId(req []string, res *string) error {
	return s.handler.ChainId(req, res)
}

func (s *EVMRPCServer) Accounts(req []string, res *string) error {
	return s.handler.Accounts(req, res)
}

func (s *EVMRPCServer) GasPrice(req []string, res *string) error {
	return s.handler.GasPrice(req, res)
}

// ===== State Queries =====

func (s *EVMRPCServer) GetBalance(req []string, res *string) error {
	return s.handler.GetBalance(req, res)
}

func (s *EVMRPCServer) GetStorageAt(req []string, res *string) error {
	return s.handler.GetStorageAt(req, res)
}

func (s *EVMRPCServer) GetTransactionCount(req []string, res *string) error {
	return s.handler.GetTransactionCount(req, res)
}

func (s *EVMRPCServer) GetCode(req []string, res *string) error {
	return s.handler.GetCode(req, res)
}

func (s *EVMRPCServer) BlockNumber(req []string, res *string) error {
	return s.handler.BlockNumber(req, res)
}

// ===== Transaction Operations =====

func (s *EVMRPCServer) SendRawTransaction(req []string, res *string) error {
	return s.handler.SendRawTransaction(req, res)
}

func (s *EVMRPCServer) EstimateGas(req []string, res *string) error {
	return s.handler.EstimateGas(req, res)
}

func (s *EVMRPCServer) Call(req []string, res *string) error {
	return s.handler.Call(req, res)
}

// ===== Transaction Queries =====

func (s *EVMRPCServer) GetTransactionReceipt(req []string, res *string) error {
	return s.handler.GetTransactionReceipt(req, res)
}

func (s *EVMRPCServer) GetTransactionByHash(req []string, res *string) error {
	return s.handler.GetTransactionByHash(req, res)
}

func (s *EVMRPCServer) GetBlockByNumber(req []string, res *string) error {
	return s.handler.GetBlockByNumber(req, res)
}

func (s *EVMRPCServer) GetBlockByHash(req []string, res *string) error {
	return s.handler.GetBlockByHash(req, res)
}

func (s *EVMRPCServer) GetLogs(req []string, res *string) error {
	return s.handler.GetLogs(req, res)
}

// RegisterEthereumRPC registers all Ethereum RPC methods with the given RPC server.
// This provides the 'eth' namespace for EVM operations.
func RegisterEthereumRPC(server *rpc.Server, rollup *Rollup, txPool *TxPool) error {
	evmRPC := NewEVMRPCServer(rollup, txPool)

	// Register with 'eth' namespace.
	err := server.RegisterName("eth", evmRPC)
	if err != nil {
		return fmt.Errorf("failed to register eth RPC namespace: %v", err)
	}

	log.Info(log.Node, "Registered Ethereum RPC namespace", "namespace", "eth")
	return nil
}
