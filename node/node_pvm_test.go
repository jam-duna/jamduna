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
	"github.com/colorfulnotion/jam/trie"
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
	time.Sleep(2 * time.Second)

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
	//0x755b8c020bf6b81a3e1d069e177e19e3892c256595c1a94ddb5a902b9e3c6212
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
		//target_statedb := poa_node.getPVMStateDB()
		target_statedb := poa_node.statedb.Copy()
		fmt.Printf("Starting StateRoot: %v\n", target_statedb.GetStateRoot())
		vm := pvm.NewVMFromCode(serviceIndex, solict_program_code, 0, target_statedb)
		// NEW IDEA: hostSolicit will fill this array
		// lookups = vm.Solicits
		vm_err := vm.Execute(types.EntryPointAccumulate)
		lookups := vm.Solicits
		if vm_err != nil {
			fmt.Printf("VM Execute Err:%v\n", vm_err)
		}
		fmt.Printf("Tentative StateRoot: %v\n", target_statedb.GetTentativeStateRoot())
		// IMPORTANT - manually trigger the stateRoot update here. But What should be the process to actually update this?
		target_statedb.StateRoot = target_statedb.GetTentativeStateRoot()
		//poa_node.statedb = target_statedb.Copy()

		fmt.Printf("--------BBBB-------\n")
		target_statedb_tr := target_statedb.GetTrie()
		target_statedb_tr.PrintTree(target_statedb_tr.Root, 0)

		v, err2 := target_statedb_tr.GetPreImageLookup(49, common.Blake2Hash(blob_arr[0]), 6)
		if err2 != nil {
			t.Fatalf("ROOT2 err %v\n",  err2)
		}
		fmt.Printf("GetPreImageLookup2 right after %v\n", v)

		validation_tr, _ := trie.InitMerkleTreeFromHash(target_statedb.GetTentativeStateRoot().Bytes(), target_statedb.GetStorage())
		fmt.Printf("--------AAAA-------\n")
		validation_tr.PrintTree(validation_tr.Root, 0)

		trie.CompareTrees(target_statedb_tr.Root, validation_tr.Root)

		//validation_tr :=  target_statedb.GetTrie()
		validation_anchor_timeslot, v_err := validation_tr.GetPreImageLookup(49, common.Blake2Hash(blob_arr[0]), 6)
		if (v_err != nil){
			//t.Fatalf("ROOT=%v. NOT FOUND! v_err %v\n", validation_tr.GetRoot(), v_err)
			//panic(0)
		}
		fmt.Printf("validation_anchor_timeslot=%v\n", validation_anchor_timeslot)

		ctx := context.Background()

		s0 := poa_node.statedb
		targetJCE := statedb.ComputeCurrentJCETime() + 120
		b1, b1_err := s0.MakeBlock(poa_node.credential, targetJCE)
		if b1_err != nil {
			t.Fatalf("MakeBlock err %v\n", b1_err)
		}
		fmt.Printf("B1 Block:%v\n", b1.String())

		// σ' ≡ Υ(σ,B)
		//s1  ≡ Υ(s0,B1)
		s1, s1_err := statedb.ApplyStateTransitionFromBlock(s0, ctx, b1)
		if (s1_err != nil){
			t.Fatalf("S0->S1 Transition Err: %v\n", s1_err)
		}
		fmt.Printf("S0->S1 Transition success!\n")
		poa_node.addStateDB(s1)

		// use lookups to do Fetch
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

		// this block should include E_P
		b2, b2_err := s1.MakeBlock(poa_node.credential, targetJCE+1)
		if b2_err != nil {
			t.Fatalf("MakeBlock err %v\n", b2_err)
		}
		fmt.Printf("B2 Block:%v\n", b2.String())
		fmt.Printf("B2 EP:%v\n", b2.PreimageLookups())

		//s2  ≡ Υ(s1,B2)
		s2, s2_err := statedb.ApplyStateTransitionFromBlock(s1, ctx, b2) // now s is getting updated as if we are applying the block
		if s2_err != nil {
			t.Fatalf("S1->S2 Transition Err %v\n", s2_err)
		}

		poa_node.addStateDB(s2)

		//s2_tr := s2.GetTrie()
		s2_tr := s2.CopyTrieState(s2.StateRoot)
		s2_preimages := b2.PreimageLookups()
        e_p := s2_preimages[0]
		preimageBlob, _ := s2_tr.GetPreImageBlob(e_p.Service_Index(), e_p.BlobHash().Bytes())
		anchor_timeslot, _ := s2_tr.GetPreImageLookup(e_p.Service_Index(), e_p.BlobHash(), e_p.BlobLength())

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
