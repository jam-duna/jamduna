package grandpa

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jam-duna/jamduna/types"
)

func TestGrandpa(t *testing.T) {
	limit := time.After(300 * time.Second) //run five minutes
	genesis_blk := fakeroot()
	nodes := SetupGrandpaNodes(6, genesis_blk)

	ctx := context.Background()

	for _, node := range nodes {
		grandpa, _ := node.grandpa.GetGrandpa(0)
		grandpa.PlayGrandpaRound(ctx, 1)
	}
	tmp_blk := genesis_blk
	basePort := uint16(10000)
	graph_server := types.NewGraphServer(basePort)
	go graph_server.StartServer()
	ticker := time.NewTicker(6 * time.Second)
	ticker2 := time.NewTicker(1 * time.Second)
	var tmpSlot = 0
loop:
	for {
		select {
		case <-ticker.C:
			new_block := fakeblock(tmp_blk)
			new_block.Header.Slot = uint32(tmpSlot)
			tmpSlot++
			addBlock(nodes, new_block)
			fmt.Printf("New Block : %v, slot %d\n", new_block.Header.Hash().String_short(), new_block.Header.Slot)
			tmp_blk = *new_block
		case <-ticker2.C:
			graph_server.Update(nodes[0].blockTree)
		case <-limit:
			break loop
		}

	}

}
