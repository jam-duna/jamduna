package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"html/template"
	"log"
	"net/http"
	"path"
)

type SerializedBlock struct {
	Header    types.BlockHeader   `json:"header"`
	Extrinsic types.ExtrinsicData `json:"extrinsic"`
	BlockHash common.Hash         `json:"block_hash"`
}

// Get Recent Block up to limit
func (n *Node) getRecentBlocks(limit int) []SerializedBlock {
	blocks := make([]SerializedBlock, 0, limit)

	if n.statedb == nil {
		return blocks // Return an empty slice if statedb is not initialized
	}

	// Start from the current block in the statedb
	currentState := n.statedb
	currentHash := currentState.BlockHash

	for i := 0; i < limit; i++ {
		block, exists := n.blocks[currentHash]
		if !exists {
			break // Stop if the block is not found
		}

		blocks = append(blocks, SerializedBlock{
			Header:    block.Header,
			Extrinsic: block.Extrinsic,
			BlockHash: block.Hash(),
		})

		// Move to the parent block using the ParentHash
		parentHash := currentState.ParentHash
		nextState, exists := n.statedbMap[parentHash]
		if !exists {
			break // Stop if the parent block is not found in the statedbMap
		}

		currentHash = parentHash
		currentState = nextState
	}

	return blocks
}

func (n *Node) getBlock(blockHash string) (types.Block, error) {
	hash := common.HexToHash(blockHash)
	block, exists := n.blocks[hash]
	if !exists {
		return types.Block{}, errors.New("Not found")
	}
	return *block, nil
}

// Get JamState by its blockHash
func (n *Node) getJamStateByBlockHash(blockHash string) (*statedb.JamState, error) {
	hash := common.HexToHash(blockHash)
	stateDB, exists := n.statedbMap[hash]
	if !exists {
		return nil, errors.New("Not found")
	}
	jamState := stateDB.GetJamState()
	return jamState, nil
}

func setCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func (n *Node) runWebService(i uint32) {
	port := 8080
	mux := http.NewServeMux()

	mux.HandleFunc("/recentblocks", func(w http.ResponseWriter, r *http.Request) {
		setCorsHeaders(w)
		blocks := n.getRecentBlocks(20)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(blocks)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fp := path.Join(".", "home.html")
		tmpl, err := template.ParseFiles(fp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
	})
	// Handler for fetching a block by blockHash
	mux.HandleFunc("/block/", func(w http.ResponseWriter, r *http.Request) {
		setCorsHeaders(w)
		blockHash := path.Base(r.URL.Path)
		block, err := n.getBlock(blockHash)
		if err != nil {
			http.Error(w, "Block not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(block)
	})

	// Handler for fetching JamState by blockHash
	mux.HandleFunc("/jamstate/", func(w http.ResponseWriter, r *http.Request) {
		setCorsHeaders(w)
		blockHash := path.Base(r.URL.Path)
		jamState, err := n.getJamStateByBlockHash(blockHash)
		if err != nil {
			http.Error(w, "JamState not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jamState)
	})

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	fmt.Printf("Starting server on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
