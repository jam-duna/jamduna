package grandpa

import (
	"context"
	"fmt"
	"time"
)

// wait_timeout is used to determine whether to wait for the timeout, currently set to true because it's too easy to hit the complete condition
const wait_timeout = true

// Play-Grandpa-Round
func (g *Grandpa) PlayGrandpaRound(ctx context.Context, round uint64) {
	round_state, _ := g.GetRoundState(round)
	if round_state == nil {
		g.InitRoundState(round, g.block_tree.Copy())
		g.InitTrackers(round)
	}
	grandpaRound := &GrandpaRound{
		Round:        round,
		Timer1Signal: make(chan bool, 1),
		Timer2Signal: make(chan bool, 1),
		Ticker:       time.NewTicker(100 * time.Millisecond),
	}
	grandpa_string := fmt.Sprintf("[v%d] Playing grandpa round %d | last finalized block %v\n", g.GetSelfVoterIndex(round), round, g.block_tree.GetLastFinalizedBlock().Block.Header.Hash())
	g.GrandpaStatusChan <- grandpa_string
	go func() {
		//create a timer for the round
		if round == 1 {
			select {
			case <-time.After(8 * time.Second):
			case <-ctx.Done():
				return
			}
		}
		defer grandpaRound.Ticker.Stop()
		// fmt.Printf("[v%d] Playing grandpa round %d | last finalized block %v\n", g.GetSelfVoterIndex(round), round, g.block_tree.GetLastFinalizedBlock().Block.Header.Hash())
		// use another goroutine to calculate the timer
		go func() {
			timer1 := time.NewTimer(2 * TimeOut * time.Second)
			timer2 := time.NewTimer(4 * TimeOut * time.Second)

			defer timer1.Stop()
			defer timer2.Stop()

			select {
			case <-timer1.C:
				grandpaRound.Timer1Signal <- true
			case <-ctx.Done():
				return
			}

			select {
			case <-timer2.C:
				grandpaRound.Timer2Signal <- true
			case <-ctx.Done():
				return
			}
		}()
		grandpa_authorities := g.GetRoundGrandpaState(round).grandpa_authorities
		primary := DerivePrimary(round, grandpa_authorities)
		if grandpa_authorities[int(primary)] == g.selfkey.PublicKey() {
			_, best_candidate_m1, err := g.BestFinalCandidate(round - 1)
			// fmt.Printf("[v%d] primary best_candidate_m1: %v\n", g.GetSelfVoterIndex(round), best_candidate_m1.Block.Header.Hash())
			if err != nil {
				g.ErrorChan <- fmt.Errorf("[v%d] error in BestFinalCandidate: %v", g.GetSelfVoterIndex(round), err)
				return
			}
			primary_commit_message, err := g.NewFinalCommitMessage(best_candidate_m1.Block, round-1)
			if err != nil {
				g.ErrorChan <- fmt.Errorf("[v%d] error primary_commit_message in NewFinalCommitMessage: %v", g.GetSelfVoterIndex(round), err)
				return
			}
			g.BroadcastCommitChan <- primary_commit_message
			if g.block_tree.ChildOrBrother(best_candidate_m1, g.block_tree.GetLastFinalizedBlock()) {
				primary_precommit_message := g.NewPrimaryVoteMessage(best_candidate_m1.Block, round)
				g.GrandpaStatusChan <- fmt.Sprintf("[v%d] primary precommit message %v\n", g.GetSelfVoterIndex(round), primary_precommit_message.SignMessage.Message.Vote.BlockHash.String_short())
				g.BroadcastVoteChan <- primary_precommit_message
				err := g.ProcessPrimaryProposeMessage(primary_precommit_message)
				if err != nil {
					g.ErrorChan <- fmt.Errorf("[v%d] error in ProcessPrimaryProposeMessage: %v", g.GetSelfVoterIndex(round), err)
				}
			} else {
				fmt.Printf("[v%d] primary precommit message not sent\n", g.GetSelfVoterIndex(round))
			}
		}
		// if the timer1 is not expired, wait for the timer1 to expire
	outerLoop1:
		for {
			select {
			case <-grandpaRound.Ticker.C:
				if !wait_timeout {
					complete, _ := g.Completable(round)
					if complete {
						break outerLoop1
					}
				}
			case <-grandpaRound.Timer1Signal:
				break outerLoop1

			case <-ctx.Done():
				return

			}
		}
		_, L_block, err := g.BestFinalCandidate(round - 1)
		if err != nil {
			g.ErrorChan <- fmt.Errorf("[v%d] error in BestFinalCandidate: %v", g.GetSelfVoterIndex(round), err)
		}
		_, N_block, err := g.BestPreVoteCandidate(round)
		if err != nil {
			g.ErrorChan <- fmt.Errorf("[v%d] error in BestPreVoteCandidate: %v", g.GetSelfVoterIndex(round), err)
		}
		if N_block == nil {
			panic(fmt.Sprintf("N_block is nil, err: %v", err))
		}
		prevote := g.NewPrevoteVoteMessage(N_block.Block, round)
		g.BroadcastVoteChan <- prevote
		err = g.ProcessPreVoteMessage(prevote)
		if err != nil {
			g.ErrorChan <- fmt.Errorf("[v%d] error in ProcessPreVoteMessage: %v", g.GetSelfVoterIndex(round), err)
		}
		timerExpired := false
	outerLoop2:
		for {
			select {
			case <-grandpaRound.Ticker.C:
				_, ghost, err := g.GrandpaGhost(round)
				if err != nil {
					continue
				}
				complete, err := g.Completable(round)
				if err != nil {
					continue
				}
				if wait_timeout {
					complete = false
				}
				timeout := complete || timerExpired
				younger := g.block_tree.ChildOrBrother(ghost, L_block)
				if younger && timeout {
					break outerLoop2
				}
			case <-grandpaRound.Timer2Signal:
				timerExpired = true

			case <-ctx.Done():
				return
			}

		}
		_, ghost, err := g.GrandpaGhost(round)
		if ghost == nil {
			panic(fmt.Sprintf("ghost is nil, err: %v", err))
		}
		if err != nil {
			g.ErrorChan <- fmt.Errorf("[v%d] error in GrandpaGhost: %v", g.GetSelfVoterIndex(round), err)
		}
		precommitmessage := g.NewPrecommitVoteMessage(ghost.Block, round)
		g.BroadcastVoteChan <- precommitmessage
		err = g.ProcessPreCommitMessage(precommitmessage)
		if err != nil {
			g.ErrorChan <- fmt.Errorf("[v%d] error in ProcessPreCommitMessage: %v", g.GetSelfVoterIndex(round), err)
		}
	outerLoop3:
		for {
			select {
			case <-grandpaRound.Ticker.C:
				err := g.AttemptToFinalizeAtRound(round)
				if err != nil {
					g.ErrorChan <- fmt.Errorf("[v%d] error in AttemptToFinalizeAtRound: %v", g.GetSelfVoterIndex(round), err)
				}
				// fmt.Printf("last finalized %v, L_block %v\n", g.block_tree.GetLastFinalizedBlock().Block.Header.Hash(), L_block.Block.Header.Hash())
				childorbro := g.block_tree.ChildOrBrother(g.block_tree.GetLastFinalizedBlock(), L_block)
				finalizable, err := g.Finalizable(round)
				if err != nil {
					g.ErrorChan <- fmt.Errorf("[v%d] error in Finalizable: %v", g.GetSelfVoterIndex(round), err)
				}
				// fmt.Printf("[v%d] childorbro %v, finalizable %v\n", g.GetSelfVoterIndex(round), childorbro, finalizable)
				// if !childorbro {
				// 	fmt.Printf("[v%d] last finalized %v, L_block %v\n", g.GetSelfVoterIndex(round), g.block_tree.GetLastFinalizedBlock().Block.Header.Hash(), L_block.Block.Header.Hash())
				// }
				if childorbro && finalizable {
					break outerLoop3
				}
			case <-ctx.Done():
				return
			}
		}
		g.PlayGrandpaRound(ctx, round+1)
	outerLoop4:
		for {
			select {
			case <-grandpaRound.Ticker.C:
				err := g.AttemptToFinalizeAtRound(round)
				if err != nil {
					g.ErrorChan <- fmt.Errorf("[v%d] error in BestFinalCandidate: %v", g.GetSelfVoterIndex(round), err)
				}
				last_finalized_block := g.block_tree.GetLastFinalizedBlock()
				_, best_candidate, err := g.BestFinalCandidate(round)
				if err != nil {
					g.ErrorChan <- fmt.Errorf("[v%d] error in BestFinalCandidate: %v", g.GetSelfVoterIndex(round), err)
				}
				if best_candidate == nil {
					panic(fmt.Sprintf("best_candidate is nil, err: %v", err))
				}
				if g.block_tree.ChildOrBrother(last_finalized_block, best_candidate) {
					// fmt.Printf("last finalized %v, best candidate %v\n", last_finalized_block.Block.Header.Hash(), best_candidate.Block.Header.Hash())
					break outerLoop4
				}

			case <-ctx.Done():
				return
			}
		}
		if round > g.Last_Completed_Round {
			g.Last_Completed_Round = round
		}

	}()
}
