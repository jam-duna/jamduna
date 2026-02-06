package grandpa

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/log"
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

	if !g.tryStartRound(round) {
		return
	}

	grandpaRound := &GrandpaRound{
		Round:        round,
		Timer1Signal: make(chan bool, 1),
		Timer2Signal: make(chan bool, 1),
		Ticker:       time.NewTicker(100 * time.Millisecond),
	}
	go func() {
		defer g.finishRound(round)
		defer grandpaRound.Ticker.Stop()

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
		// Only send primary proposal if round > 0 (need round-1 to exist)
		if round > 0 && bytes.Equal(grandpa_authorities[int(primary)].Ed25519.Bytes(), g.selfkey.Ed25519Pub[:]) {
			_, best_candidate_m1, err := g.BestFinalCandidate(round - 1)
			if err != nil {
				log.Error(log.Grandpa, "BestFinalCandidate failed", "voter", g.GetSelfVoterIndex(round), "err", err)
				return
			}
			primary_commit_message, err := g.NewFinalCommitMessage(best_candidate_m1.Block, round-1)
			if err != nil {
				log.Error(log.Grandpa, "NewFinalCommitMessage failed", "voter", g.GetSelfVoterIndex(round), "err", err)
				return
			}
			g.Broadcast(primary_commit_message)
			if g.block_tree.ChildOrBrother(best_candidate_m1, g.block_tree.GetLastFinalizedBlock()) {
				primary_precommit_message := g.NewPrimaryVoteMessage(best_candidate_m1.Block, round)
				g.Broadcast(primary_precommit_message)
				err := g.ProcessPrimaryProposeMessage(primary_precommit_message)
				if err != nil {
					log.Warn(log.Grandpa, "ProcessPrimaryProposeMessage failed", "voter", g.GetSelfVoterIndex(round), "err", err)
				}
			} else {
				log.Debug(log.Grandpa, "Primary precommit message not sent", "voter", g.GetSelfVoterIndex(round))
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

		// Get L (best final candidate from previous round, or last finalized for round 0)
		var L_block, N_block = g.block_tree.GetLastFinalizedBlock(), g.block_tree.GetLastFinalizedBlock()
		var err error

		if round == 0 {
			L_block = g.block_tree.GetLastFinalizedBlock()
		} else {
			_, L_block, err = g.BestFinalCandidate(round - 1)
			if err != nil || L_block == nil {
				// Use last finalized block as fallback if we can't determine best final candidate
				L_block = g.block_tree.GetLastFinalizedBlock()
				if err != nil {
					log.Warn(log.Grandpa, "BestFinalCandidate failed, using fallback", "voter", g.GetSelfVoterIndex(round), "err", err)
				}
			}
		}

		_, N_block, err = g.BestPreVoteCandidate(round)
		if err != nil || N_block == nil {
			// Use L_block as fallback for prevote candidate
			N_block = L_block
			if err != nil {
				log.Warn(log.Grandpa, "BestPreVoteCandidate failed, using fallback", "voter", g.GetSelfVoterIndex(round), "err", err)
			}
		}
		prevote := g.NewPrevoteVoteMessage(N_block.Block, round)
		g.Broadcast(prevote)
		err = g.ProcessPreVoteMessage(prevote)
		if err != nil {
			log.Warn(log.Grandpa, "ProcessPreVoteMessage failed", "voter", g.GetSelfVoterIndex(round), "err", err)
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
		if err != nil || ghost == nil {
			// Use last finalized block as fallback
			ghost = g.block_tree.GetLastFinalizedBlock()
			if err != nil {
				log.Warn(log.Grandpa, "GrandpaGhost failed, using fallback", "voter", g.GetSelfVoterIndex(round), "err", err)
			}
		}
		precommitmessage := g.NewPrecommitVoteMessage(ghost.Block, round)
		g.Broadcast(precommitmessage)
		err = g.ProcessPreCommitMessage(precommitmessage)
		if err != nil {
			log.Warn(log.Grandpa, "ProcessPreCommitMessage failed", "voter", g.GetSelfVoterIndex(round), "err", err)
		}
	outerLoop3:
		for {
			select {
			case <-grandpaRound.Ticker.C:
				err := g.AttemptToFinalizeAtRound(round)
				if err != nil {
					log.Warn(log.Grandpa, "AttemptToFinalizeAtRound failed", "voter", g.GetSelfVoterIndex(round), "err", err)
				}
				childorbro := g.block_tree.ChildOrBrother(g.block_tree.GetLastFinalizedBlock(), L_block)
				finalizable, err := g.Finalizable(round)
				if err != nil {
					log.Warn(log.Grandpa, "Finalizable failed", "voter", g.GetSelfVoterIndex(round), "err", err)
				}
				if childorbro && finalizable {
					break outerLoop3
				}
			case <-ctx.Done():
				return
			}
		}
		// Send NewRoundEvent if EventChan is available
		// store the catch-up message for this round (if storage is available)
		if g.storage != nil {
			catchupResponse, err := g.GetCatchUpResponse(round)
			if err == nil {
				catchupBytes, err := catchupResponse.ToBytes()
				if err == nil {
					g.storage.StoreCatchupMassage(round, uint32(g.authority_set_id), catchupBytes)
				}
			}
		}
		if g.EventChan != nil {
			nextRound := round + 1
			event := GrandpaEvent{
				Kind:  NewRoundEvent,
				SetID: g.authority_set_id,
				Round: nextRound,
			}
			select {
			case g.EventChan <- event:
			default:
				// Channel full, skip sending event
			}
		} else {
			// No event channel, start next round directly
			g.PlayGrandpaRound(ctx, round+1)
		}

	outerLoop4:
		for {
			select {
			case <-grandpaRound.Ticker.C:
				err := g.AttemptToFinalizeAtRound(round)
				if err != nil {
					log.Warn(log.Grandpa, "AttemptToFinalizeAtRound failed", "voter", g.GetSelfVoterIndex(round), "err", err)
				}
				last_finalized_block := g.block_tree.GetLastFinalizedBlock()
				_, best_candidate, err := g.BestFinalCandidate(round)
				if err != nil {
					log.Warn(log.Grandpa, "BestFinalCandidate failed", "voter", g.GetSelfVoterIndex(round), "err", err)
					continue
				}
				if best_candidate == nil {
					log.Debug(log.Grandpa, "best_candidate is nil", "voter", g.GetSelfVoterIndex(round))
					continue
				}
				if g.block_tree.ChildOrBrother(last_finalized_block, best_candidate) {
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

func (g *Grandpa) GetAllResults(round uint64) (prevotes, precommits []SignedMessage) {
	round_state, err := g.GetRoundState(round)
	if err != nil || round_state == nil {
		return nil, nil
	}

	// Get prevotes from the PrevoteStage tracker
	prevoteTracker := round_state.GetVoteTracker(PrevoteStage)
	if prevoteTracker != nil {
		prevoteTracker.VoteMutex.RLock()
		votes := prevoteTracker.GetAllVotes()
		for _, voterVotes := range votes {
			for _, vote := range voterVotes {
				if vote != nil {
					prevotes = append(prevotes, *vote)
				}
			}
		}
		prevoteTracker.VoteMutex.RUnlock()
	}

	// Get precommits from the PrecommitStage tracker
	precommitTracker := round_state.GetVoteTracker(PrecommitStage)
	if precommitTracker != nil {
		precommitTracker.VoteMutex.RLock()
		votes := precommitTracker.GetAllVotes()
		for _, voterVotes := range votes {
			for _, vote := range voterVotes {
				if vote != nil {
					precommits = append(precommits, *vote)
				}
			}
		}
		precommitTracker.VoteMutex.RUnlock()
	}

	return prevotes, precommits
}

func GetBlockHashesFromVotes(votes []SignedMessage, precommits []SignedMessage) []common.Hash {
	hashMap := make(map[common.Hash]struct{})
	for _, vote := range votes {
		hash := vote.Message.Vote.HeaderHash
		hashMap[hash] = struct{}{}
	}
	for _, vote := range precommits {
		hash := vote.Message.Vote.HeaderHash
		hashMap[hash] = struct{}{}
	}
	var hashes []common.Hash
	for hash := range hashMap {
		hashes = append(hashes, hash)
	}
	return hashes
}

func (g *Grandpa) GetCatchUpResponse(round uint64) (CatchUpResponse, error) {
	catchUpResponse := CatchUpResponse{}
	catchUpResponse.Round = round
	round_state, err := g.GetRoundState(round)
	if err != nil || round_state == nil {
		return catchUpResponse, fmt.Errorf("GetCatchUpResponse: no grandpa round state found for round %d", round)
	}
	prevotes, precommits := g.GetAllResults(round)
	catchUpResponse.SignedPrevotes = prevotes
	catchUpResponse.SignedPrecommits = precommits
	blockHashes := GetBlockHashesFromVotes(prevotes, precommits)
	baseBlock, err := g.block_tree.GetCommonAncestorByGroups(blockHashes)
	if err != nil {
		return catchUpResponse, fmt.Errorf("GetCatchUpResponse: error getting common ancestor: %v", err)
	}
	catchUpResponse.BaseHash = baseBlock.Block.Header.Hash()
	catchUpResponse.BaseNumber = baseBlock.Block.Header.Slot
	return catchUpResponse, nil
}

// PlayRoundWithCatchUp processes a round using catch-up data when we're behind.
// Unlike PlayGrandpaRound which participates in voting, this replays votes from
// a completed round to reach the same finalized state.
func (g *Grandpa) PlayRoundWithCatchUp(ctx context.Context, round uint64, catchup CatchUpResponse) error {
	// Validate catch-up round matches
	if catchup.Round != round {
		return fmt.Errorf("PlayRoundWithCatchUp: round mismatch, expected %d got %d", round, catchup.Round)
	}

	// Initialize round state if needed
	round_state, _ := g.GetRoundState(round)
	if round_state == nil {
		g.InitRoundState(round, g.block_tree.Copy())
		g.InitTrackers(round)
	}

	// Process all prevotes from catch-up response
	for _, prevote := range catchup.SignedPrevotes {
		vote := GrandpaVote{
			Round:         round,
			SetId:         g.authority_set_id,
			SignedMessage: prevote,
		}
		if err := g.ProcessPreVoteMessage(vote); err != nil {
			// Log but continue - some votes may be duplicates or invalid
			log.Debug(log.Grandpa, "PlayRoundWithCatchUp: failed to process prevote", "round", round, "err", err)
		}
	}

	// Process all precommits from catch-up response
	for _, precommit := range catchup.SignedPrecommits {
		vote := GrandpaVote{
			Round:         round,
			SetId:         g.authority_set_id,
			SignedMessage: precommit,
		}
		if err := g.ProcessPreCommitMessage(vote); err != nil {
			// Log but continue - some votes may be duplicates or invalid
			log.Debug(log.Grandpa, "PlayRoundWithCatchUp: failed to process precommit", "round", round, "err", err)
		}
	}

	// Attempt finalization with the imported votes
	if err := g.AttemptToFinalizeAtRound(round); err != nil {
		return fmt.Errorf("PlayRoundWithCatchUp: finalization failed: %w", err)
	}

	// Update last completed round
	if round > g.Last_Completed_Round {
		g.Last_Completed_Round = round
	}

	return nil
}
