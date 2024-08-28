package node

import (
	"fmt"
	"time"

	//"sync"
	"context"
	"testing"

	"github.com/colorfulnotion/jam/common"

	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"

	"github.com/colorfulnotion/jam/types"
)

func TestNodePOAAccumulatePVM(t *testing.T) {

	genesisConfig, peers, peerList, validatorSecrets, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error Seeting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint32(i), validatorSecrets[i], &genesisConfig, peers, peerList, DAFlag)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		nodes[i] = node
	}
	// Wait for nodes to be ready
	fmt.Println("Waiting for nodes to be ready...")
	time.Sleep(1 * time.Second)

	senderNode := nodes[0]
	extrinsics := []string{"abcdef", "abcd", "cat", "dog"}
    blob_arr := make([][]byte, len(extrinsics))
	for data_i, s := range extrinsics {
		data := []byte(s)
        blob_arr[data_i] = data
		fmt.Println(data)
		blob_hash, err := senderNode.EncodeAndDistributeData(data)
		if err != nil {
			t.Fatalf("Failed to encode and distribute data: %v", err)
		}
		fmt.Printf("successfully distributed blob_hash : %v\n", blob_hash)
	}

	// Accumulate function performs the accumulation of a single service.
	serviceIndex := 49
	//solict_program_code := pvm.LoadPVMCode("../pvm/hostfunctions/solicit.pvm")

	// solict_program_code := []byte{0, 0, 0, 1, 2, 3, 4, 5}
	solict_program_code := []byte{
		0,
		0,
		74, // size of c
		4, 0, 0, 64,
		4, 1, 6,
		38, 2, 0, 64, 2, 140, 91, 117,
		38, 2, 4, 64, 26, 184, 246, 11,
		38, 2, 8, 64, 158, 6, 29, 62,
		38, 2, 12, 64, 227, 25, 126, 23,
		38, 2, 16, 64, 101, 37, 44, 137,
		38, 2, 20, 64, 77, 169, 193, 149,
		38, 2, 24, 64, 43, 144, 90, 219,
		38, 2, 28, 64, 18, 98, 60, 158,
		78, 13,
		0,
		145, 128, 128, 128, 128, 128, 128, 128, 128, 254, // bitmask
		0}

	// solicit code executes causes SDB writes for a_l USING: ReadServiceBytes, ReadServicePreimageLookup, WriteServicePreimageLookup
	for i := 0; i < 1; i++ {
		//you can potentially call NewForceCreateVM() and initialize your vm, make sure you handle the reg properly
		poa_node := nodes[i]
		target_statedb := poa_node.getPVMStateDB()
		vm := pvm.NewVMFromCode(solict_program_code, 0, target_statedb)
		// NEW IDEA: hostSolicit will fill this array
		// lookups = vm.Solicits
		err := vm.Execute(types.EntryPointAccumulate)
		lookups := vm.Solicits
		if err != nil {
			fmt.Printf("VM Execute Err:%v/n", err)
		}

		ctx := context.Background()
		//stateDB.NewStateDB(nodes[0].storage)
		//s := nodes[i].statedb.NewStateDB(nodes[0].storage)

		s := poa_node.statedb
		targetJCE := statedb.ComputeCurrentJCETime() + 120
		b0, s2, err0 := s.MakeBlock(poa_node.credential, targetJCE)
		if err0 != nil {
			t.Fatalf("MakeBlock err %v\n", err0)
		} else {
			fmt.Printf("S2 StateRoot:%v Block:%v\n", b0.String(), s2.StateRoot)
		}
		// use lookups to do Fetch
		poa_node.statedb.ApplyStateTransitionFromBlock(ctx, b0)
		for _, l := range lookups {
			reconstructData, err := senderNode.FetchAndReconstructData(l.BlobHash)
			//reconstructData, err := senderNode.FetchAndReconstructData(l.BlobHash, l.Length)
			if err != nil {
				t.Fatalf("Failed to fetch and reconstruct data: %v", err)
			}
			// now you have Preimage AND blob
			lookup := types.PreimageLookup{
				ServiceIndex: uint32(serviceIndex),
				Data:         reconstructData[0:l.Length],
			}

			// ADD TO Queue  which is used in the NEXT MakeBlock to fill the E_P
			//stateDB need to add lookup
			nodes[i].processLookup(lookup)
		}

		b1, s3, err0 := s.MakeBlock(poa_node.credential, targetJCE+1)
		if err0 != nil {
			t.Fatalf("MakeBlock err %v\n", err0)
		} else {
			fmt.Printf("S3 StateRoot:%v Block:%v\n", b1.String(), s3.StateRoot)
		}

		err = s.ProcessIncomingBlock(b1) // now s is getting updated as if we are applying the block
		if err != nil {
			t.Fatalf("ProcessIncomingBlock err %v\n", err0)
		}
		// THIS update a_p
		// nodes[i].statedb.ApplyStateTransitionFromBlock(ctx, b1)

		tr := s.GetTrie()
		tr = s.CopyTrieState(s.StateRoot)
        e_p := b1.Extrinsic.PreimageLookups[0]
		preimageBlob, _ := tr.GetPreImageBlob(e_p.Service_Index(), e_p.BlobHash().Bytes())
		anchor_timeslot, _ := tr.GetPreImageLookup(e_p.Service_Index(), e_p.BlobHash(), e_p.BlobLength())

        if (!common.CompareBytes(preimageBlob, blob_arr[0])){
            t.Fatalf("Mismatch: originalBlob=%x, retrievedBlob=%x\n", preimageBlob, blob_arr[0])
        }

        if (anchor_timeslot[0] != b1.TimeSlot()){
            t.Fatalf("Anchor_slot mimatch original=%v, retrieved=%v\n", preimageBlob, blob_arr[0])
        }
        fmt.Printf("EP Succ\n")
	}
}

func TestNodePOASolicitLookup(t *testing.T) {
    // testing a_l & a_p
    //-- after the Above EP. use pvm host lookup=1 to retrive result
}

func TestNodePOAHistoricalLookup(t *testing.T) {
    //-- after the Above EP. use pvm host historical_lookup=15 to anchor_timeslot
}


func TestNodePOAReadWrite(t *testing.T) {
     // testing a_s via read=2, write=3
     //-- reads 4 numbers, squares them and writes the sum
}

func TestNodePOAExport(t *testing.T) {
    //-- squares an extrinsic input and exports it
}
