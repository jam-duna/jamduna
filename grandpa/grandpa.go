package grandpa

import (
	"fmt"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

const (
	DefaultChannelSize = 200
	TimeOut            = 1
)

type Broadcaster interface {
	Broadcast(msg interface{}, evID ...uint64)
	FinalizedBlockHeader(headerHash common.Hash)
	FinalizedEpoch(epoch uint32, beefyHash common.Hash, aggregatedSignature bls.Signature)
}

type Grandpa struct {
	block_tree           *types.BlockTree // hold the same block tree as the node itself
	RoundStateMutex      sync.RWMutex
	RoundState           map[uint64]*RoundState
	selfkey              types.ValidatorSecret
	Last_Completed_Round uint64

	ScheduledAuthoritySet map[uint32][]types.Validator
	authority_set_id      uint32

	Voter_Staked map[types.Ed25519Key]uint64
	ErrorChan    chan error

	broadcaster Broadcaster
	storage     types.JAMStorage

	// CE149
	PreVoteMessageCh   chan GrandpaVote
	PreCommitMessageCh chan GrandpaVote
	PrimaryMessageCh   chan GrandpaVote

	// CE150
	CommitMessageCh chan GrandpaCommit

	// CE151
	StateMessageCh chan GrandpaStateMessage

	// CE152
	CatchUpMessageCh chan GrandpaCatchUp

	// CE153
	WarpSyncRequestCh chan uint32

	NewValidatorsCh chan []types.Validator

	// BLS signature tracking: map[epoch_beefyHash]map[validatorIndex]signature
	singleBLSSignatures      map[string]map[uint16]bls.Signature
	singleBLSSignaturesMutex sync.RWMutex

	// Finalized aggregated signatures: map[epoch_beefyHash]aggregatedSignature
	finalizedSignatures      map[string]bls.Signature
	finalizedSignaturesMutex sync.RWMutex
}

// this function will be used in receiving the grandpa message and be the bridge between grandpa and network
func (g *Grandpa) RunGrandpa() {
	for {
		select {

		case vote := <-g.PreVoteMessageCh: // CE149
			err := g.ProcessPreVoteMessage(vote)
			if err != nil {
				fmt.Println(err)
			}
		case vote := <-g.PreCommitMessageCh: // CE149
			err := g.ProcessPreCommitMessage(vote)
			if err != nil {
				fmt.Println(err)
			}
		case vote := <-g.PrimaryMessageCh: // CE149
			err := g.ProcessPrimaryProposeMessage(vote)
			if err != nil {
				fmt.Println(err)
			}
		case commit := <-g.CommitMessageCh: // C150
			g.ProcessCommitMessage(commit)
		case state := <-g.StateMessageCh: // CE151
			err := g.ProcessStateMessage(state)
			if err != nil {
				fmt.Println(err)
			}
		case catchup := <-g.CatchUpMessageCh: // CE152
			response, err := g.ProcessCatchUpMessage(catchup)
			if err != nil {
				fmt.Println(err)
			}
			g.ProcessCatchUpResponse(response)
		case warpReq := <-g.WarpSyncRequestCh: // CE153
			response, err := g.ProcessWarpSyncRequest(warpReq)
			if err != nil {
				fmt.Println(err)
			}
			g.ProcessWarpSyncResponse(response)
		case validators := <-g.NewValidatorsCh:
			g.ProcessNewValidators(validators)

		case <-g.ErrorChan:
			// fmt.Println("Grandpa error:", err)
		}
	}
}

type GrandpaRound struct {
	Round        uint64
	Timer1Signal chan bool
	Timer2Signal chan bool
	Ticker       *time.Ticker
}

func (g *Grandpa) InitRoundState(round uint64, block_tree *types.BlockTree) {
	g.RoundStateMutex.Lock()
	defer g.RoundStateMutex.Unlock()
	// also cleanup the old round state
	if round-10 >= 0 {
		delete(g.RoundState, round-10)
	}

	if _, exists := g.RoundState[round]; !exists {
		g.RoundState[round] = &RoundState{
			GrandpaState:   NewGrandpaState(g.GetCurrentScheduledAuthoritySet(round), round),
			PreVoteGraph:   block_tree.Copy(),
			PreCommitGraph: block_tree.Copy(),
			GrandpaCJF_Map: NewGrandpaCJF(len(g.GetCurrentScheduledAuthoritySet(round))),
			VoteTracker:    &sync.Map{},
		}
	}
}

func (g *Grandpa) GetRoundState(round uint64) (*RoundState, error) {
	g.RoundStateMutex.RLock()
	defer g.RoundStateMutex.RUnlock()
	if _, exists := g.RoundState[round]; !exists {
		return nil, fmt.Errorf("round %d state not found, last round=%d", round, g.Last_Completed_Round)
	}
	return g.RoundState[round], nil
}

func (g *Grandpa) GetRoundGrandpaState(round uint64) *GrandpaState {
	g.RoundStateMutex.RLock()
	defer g.RoundStateMutex.RUnlock()
	if _, exists := g.RoundState[round]; !exists {
		return nil
	} else {
		return g.RoundState[round].GrandpaState
	}
}

func (g *Grandpa) GetCJF(round uint64) (*GrandpaCJF, bool) {
	g.RoundStateMutex.RLock()
	defer g.RoundStateMutex.RUnlock()
	if _, exists := g.RoundState[round]; !exists {
		return nil, false
	} else {
		return g.RoundState[round].GrandpaCJF_Map, true
	}
}

func NewGrandpa(block_tree *types.BlockTree, selfkey types.ValidatorSecret, authSet0 []types.Validator, genesis_blk *types.Block, broadcaster Broadcaster, stor types.JAMStorage) *Grandpa {

	staked := make(map[types.Ed25519Key]uint64)
	for _, validator := range authSet0 {
		staked[validator.Ed25519] = 100
	}
	g := &Grandpa{
		block_tree:            block_tree,
		RoundState:            make(map[uint64]*RoundState),
		selfkey:               selfkey,
		Last_Completed_Round:  0,
		ScheduledAuthoritySet: make(map[uint32][]types.Validator),
		Voter_Staked:          staked,
		broadcaster:           broadcaster,
		storage:               stor,
		authority_set_id:      0,
		singleBLSSignatures:   make(map[string]map[uint16]bls.Signature),
		finalizedSignatures:   make(map[string]bls.Signature),
	}
	g.ScheduledAuthoritySet[0] = authSet0
	g.InitRoundState(0, block_tree.Copy())
	g.InitTrackers(0)

	// Initialize input channels
	g.PreVoteMessageCh = make(chan GrandpaVote, DefaultChannelSize)
	g.PreCommitMessageCh = make(chan GrandpaVote, DefaultChannelSize)
	g.PrimaryMessageCh = make(chan GrandpaVote, DefaultChannelSize)
	g.CommitMessageCh = make(chan GrandpaCommit, DefaultChannelSize)
	g.StateMessageCh = make(chan GrandpaStateMessage, DefaultChannelSize)
	g.CatchUpMessageCh = make(chan GrandpaCatchUp, DefaultChannelSize)
	g.WarpSyncRequestCh = make(chan uint32, DefaultChannelSize)
	g.NewValidatorsCh = make(chan []types.Validator, DefaultChannelSize)
	g.ErrorChan = make(chan error, DefaultChannelSize)

	// This is sufficient to get everyone to broadcast finalization of genesis
	g.Broadcast(g.NewPrecommitVoteMessage(genesis_blk, 0))
	// commit, _ := g.NewFinalCommitMessage(genesis_blk, 0)
	// g.Broadcast(g.NewPrevoteVoteMessage(genesis_blk, 0))
	// g.Broadcast(commit)
	return g
}

func (g *Grandpa) Broadcast(msg interface{}) {
	g.broadcaster.Broadcast(msg)
}

func (g *Grandpa) GetCurrentScheduledAuthoritySet(round uint64) []types.Validator {
	return g.ScheduledAuthoritySet[0] // TODO: need to change to round
}

func (g *Grandpa) GetCurrentAuthoritySetIndex(round uint64, ed25519 types.Ed25519Key) int {
	authorities := g.GetCurrentScheduledAuthoritySet(round)
	for i, v := range authorities {
		if v.Ed25519 == ed25519 {
			return i
		}
	}
	return -1
}

func (g *Grandpa) GetVoteTracker(round uint64, stage GrandpaStage) *VoteTracker {
	round_state, err := g.GetRoundState(round)
	if err != nil {
		return nil
	}
	if round_state.VoteTracker == nil {
		round_state.VoteTracker = &sync.Map{}
	}
	if _, exists := round_state.VoteTracker.Load(stage); !exists {
		g.InitTrackers(round)
	}
	value, ok := round_state.VoteTracker.Load(stage)
	if !ok {
		return nil
	}
	return value.(*VoteTracker)
}

func (g *Grandpa) GetSupermajorityThreshold(round uint64) int {
	g.RoundStateMutex.RLock()
	defer g.RoundStateMutex.RUnlock()
	total_vote_weight := uint64(0)
	for _, staked := range g.Voter_Staked {
		total_vote_weight += staked
	}
	weight := int(total_vote_weight)
	return weight*2/3 + 1
}

func (g *Grandpa) SetScheduledAuthoritySet(setID uint32, authorities []types.Validator) {
	g.ScheduledAuthoritySet[setID] = authorities
}

func (g *Grandpa) InitTrackers(round uint64) {
	authorities := g.GetCurrentScheduledAuthoritySet(round)
	round_state, _ := g.GetRoundState(round)
	if round_state == nil {
		g.InitRoundState(round, g.block_tree.Copy())
		round_state, _ = g.GetRoundState(round)
	}
	tracker := round_state.VoteTracker
	if _, exists := tracker.Load(PrevoteStage); !exists {
		tracker.Store(PrevoteStage, NewVoteTracker(PrevoteStage, round, g.Voter_Staked))
	}
	if _, exists := tracker.Load(PrecommitStage); !exists {
		tracker.Store(PrecommitStage, NewVoteTracker(PrecommitStage, round, g.Voter_Staked))
	}
	if _, exists := tracker.Load(PrimaryProposeStage); !exists {
		tracker.Store(PrimaryProposeStage, NewVoteTracker(PrimaryProposeStage, round, g.Voter_Staked))
	}
	// Only initialize CJF if it doesn't exist yet
	if round_state.GrandpaCJF_Map == nil {
		round_state.GrandpaCJF_Map = NewGrandpaCJF(len(authorities))
	}
}

func NewGrandpaCJF(len int) *GrandpaCJF {
	return &GrandpaCJF{
		Votes:          make([]SignedPrecommit, len),
		Justifications: make([]JustificationElement, len),
	}
}

type Authority struct {
	Authorities []types.Validator
}

// Algorithm 15. Derive Primary
func DerivePrimary(r uint64, V []types.Validator) uint64 {
	return r % uint64(len(V))
}

// Algorithm 16. Best Final Candidate
func (g *Grandpa) BestFinalCandidate(round uint64) (common.Hash, *types.BT_Node, error) {
	ghost_hash, ghost, err := g.GrandpaGhost(round)
	if err != nil {
		return common.Hash{}, nil, err
	}
	if round == 0 {
		return ghost_hash, ghost, nil
	} else {
		// get the young blocks from the tree
		round_state, err := g.GetRoundState(round)
		//if can't get the round state, return the ghost block
		if err != nil {
			return ghost_hash, ghost, nil
		}
		round_state.UpdatePreCommitGraph(true)
		precommit_graph := round_state.PreCommitGraph
		// root_hash := precommit_graph.Root.Block.Header.Hash()。
		old_ghost, err := precommit_graph.FindGhost(ghost_hash, uint64(g.GetSupermajorityThreshold(round)))
		if err != nil {
			return ghost_hash, ghost, nil
		}
		old_ghost, ok := g.block_tree.GetBlockNode(old_ghost.Block.Header.Hash())
		older := g.block_tree.ChildOrBrother(ghost, old_ghost)
		if !older {
			return ghost_hash, ghost, nil
		}
		if err != nil || !ok {
			return ghost_hash, ghost, nil
		}
		if old_ghost.Block.GetParentHeaderHash() == ghost.Block.GetParentHeaderHash() {
			return ghost_hash, ghost, nil
		} else {
			return old_ghost.Block.Header.Hash(), old_ghost, nil
		}

	}
}

// Algorithm 17. GRANDPA GHOST
func (g *Grandpa) GrandpaGhost(round uint64) (common.Hash, *types.BT_Node, error) {
	if round == 0 {
		return g.block_tree.GetLastFinalizedBlock().Block.Header.Hash(), g.block_tree.GetLastFinalizedBlock(), nil
	} else {
		best_candidate, best_candidate_block, err := g.BestFinalCandidate(round - 1)
		if err != nil {
			return common.Hash{}, nil, err
		}
		ghost_blocks, _, err := g.block_tree.GetDescendingBlocks(best_candidate)
		if err != nil {
			return common.Hash{}, nil, err
		}
		if len(ghost_blocks) == 0 {
			return best_candidate, best_candidate_block, nil
		}
		latest_block := best_candidate_block
		round_state, err := g.GetRoundState(round)
		if err != nil {
			return latest_block.Block.Header.Hash(), latest_block, nil
		}
		round_state.UpdatePreVoteGraph(false)
		prevote_graph := round_state.PreVoteGraph
		ghost, err := prevote_graph.FindGhost(best_candidate, uint64(g.GetSupermajorityThreshold(round)))
		if err != nil {
			return latest_block.Block.Header.Hash(), latest_block, nil
		} else {
			ghost_hash := ghost.Block.Header.Hash()
			ghost_block, find := g.block_tree.GetBlockNode(ghost_hash)
			if !find {
				return latest_block.Block.Header.Hash(), latest_block, nil
			}
			if ghost_block.IsAncestor(best_candidate_block) {
				return ghost_hash, ghost_block, nil
			} else {
				return latest_block.Block.Header.Hash(), latest_block, nil
			}
		}
	}
}

// Algorithm 18. Best PreVote Candidate
func (g *Grandpa) BestPreVoteCandidate(round uint64) (common.Hash, *types.BT_Node, error) {
	round_state, err := g.GetRoundState(round)
	if err != nil {
		return common.Hash{}, nil, err
	}
	// get the ghost block
	L_Hash, L_block, err := g.BestFinalCandidate(round - 1)
	if err != nil {
		return common.Hash{}, nil, err
	}
	err = round_state.UpdatePreVoteGraph(true)
	if err != nil {
		return common.Hash{}, nil, err
	}
	prevote_graph := round_state.PreVoteGraph
	ghost, err := prevote_graph.FindGhost(L_Hash, uint64(g.GetSupermajorityThreshold(round)))
	if err != nil {
		return common.Hash{}, L_block, err
	}
	ghost_hash := ghost.Block.Header.Hash()
	// use real block tree
	ghost_block, find := g.block_tree.GetBlockNode(ghost_hash)
	if !find {
		return common.Hash{}, L_block, fmt.Errorf("ghost block not found")
	}
	// if receive the primary
	primary_id := DerivePrimary(round, g.GetCurrentScheduledAuthoritySet(round))
	primary_validator := g.GetCurrentScheduledAuthoritySet(round)[primary_id]
	tracker := g.GetVoteTracker(round, PrimaryProposeStage)
	proposed_hash, err_proposed := tracker.GetPrimaryPropose(primary_validator.Ed25519)
	if err_proposed == nil {
		proposed_block, err := prevote_graph.FindGhost(proposed_hash, uint64(g.GetSupermajorityThreshold(round)))
		if err != nil {

			return ghost_hash, ghost_block, nil
		} else {
			proposed_hash = proposed_block.Block.Header.Hash()
			proposed_block, ok := g.block_tree.GetBlockNode(proposed_hash)
			if !ok {
				return ghost_hash, ghost_block, nil
			} else {
				return proposed_hash, proposed_block, nil
			}
		}
	} else {
		return ghost_hash, ghost_block, nil
	}
}

// Algorithm 19. Attempt To Finalize At Round
func (g *Grandpa) AttemptToFinalizeAtRound(round uint64) error {

	//L
	last_finalized_block := g.block_tree.GetLastFinalizedBlock()
	// last_finalized_block_hash := last_finalized_block.Block.Header.Hash()()

	//E
	best_final_candidate_hash, best_final_candidate, err := g.BestFinalCandidate(round)
	if err != nil {
		return err
	}
	e_bigger_than_l := g.block_tree.ChildOrBrother(best_final_candidate, last_finalized_block)
	round_state, err := g.GetRoundState(round)
	if err != nil {
		return err
	}

	round_state.UpdatePreCommitGraph(false)
	precommit_graph := round_state.PreCommitGraph
	E_node, ok := precommit_graph.GetBlockNode(best_final_candidate_hash)
	if !ok {
		return fmt.Errorf("best final candidate not found")
	}
	e_enough_votes := E_node.GetCumulativeVotesWeight() > uint64(g.GetSupermajorityThreshold(round))

	if e_bigger_than_l && e_enough_votes {
		g.block_tree.FinalizeBlock(best_final_candidate_hash)
		commit, err := g.NewFinalCommitMessage(best_final_candidate.Block, round)
		if err != nil {
			return err
		}
		g.Broadcast(commit)
	}
	return nil
}

// Algorithm 20. Finalizable
func (g *Grandpa) Finalizable(round uint64) (bool, error) {
	completable, err := g.Completable(round)
	if err != nil {
		return false, err
	}
	if !completable {
		return false, nil
	}
	_, ghost_block, err := g.GrandpaGhost(round)
	if err != nil {
		return false, err
	}
	_, best_candidate_block, err := g.BestFinalCandidate(round)
	if err != nil {
		return false, err
	}
	_, best_candidate_m1_block, err := g.BestFinalCandidate(round - 1)
	if err != nil {
		return false, err
	}
	first := g.block_tree.ChildOrBrother(best_candidate_block, best_candidate_m1_block)
	second := g.block_tree.ChildOrBrother(ghost_block, best_candidate_block)
	finalize := first && second
	if best_candidate_block != nil && finalize {
		return true, nil
	} else {
		return false, nil
	}

}

// Completable
func (g *Grandpa) Completable(round uint64) (bool, error) {
	ghost, _, err := g.GrandpaGhost(round)
	if err != nil {
		return false, err
	}
	round_state, err := g.GetRoundState(round)
	if err != nil {
		return false, err
	}
	round_state.UpdatePreCommitGraph(false)
	precommit_graph := round_state.PreCommitGraph
	ghost_block, ok := precommit_graph.GetBlockNode(ghost)
	if !ok {
		return false, fmt.Errorf("ghost block not found")
	}
	descendants, _, err := precommit_graph.GetDescendingBlocks(ghost)
	if err != nil {
		return false, err
	}
	for _, descendant := range descendants {
		if descendant.IsAncestor(ghost_block) && descendant.Cumulative_VotseWeight > 0 {
			return true, nil
		}
	}

	return true, nil
}

// get the index of the voter in the grandpa_authorities
func (g *Grandpa) GetSelfVoterIndex(round uint64) uint64 {
	grandpa_state := g.GetRoundGrandpaState(round)
	if grandpa_state == nil {
		g.InitRoundState(round, g.block_tree.Copy()) // initialize the round state if it doesn't exist
		grandpa_state = g.GetRoundGrandpaState(round)
	}
	for i, v := range grandpa_state.grandpa_authorities {
		if v.GetEd25519Key() == g.selfkey.Ed25519Pub {
			return uint64(i)
		}
	}
	return 0
}

func (g *Grandpa) ProcessNewValidators(validators []types.Validator) {
	g.ScheduledAuthoritySet[g.authority_set_id] = validators
}

func (g *Grandpa) GetAuthoritySet(setID uint32) ([]types.Validator, bool) {
	authSet, exists := g.ScheduledAuthoritySet[setID]
	if !exists {
		return nil, false
	}
	return authSet, true
}

func (g *Grandpa) GetQuorumSize(round uint64) int {
	return g.GetSupermajorityThreshold(round)
}

func (g *Grandpa) ProcessAggregateBLSSignature(epoch uint32, beefyHash common.Hash, aggregateSignature bls.Signature, validatorIndex uint16) error {
	// Process aggregate BLS signature (group signature from multiple validators)
	key := fmt.Sprintf("%d_%s", epoch, beefyHash.Hex())
	g.finalizedSignaturesMutex.Lock()
	defer g.finalizedSignaturesMutex.Unlock()

	// Check if we aleady have processed this finalized signature to avoid duplicates
	if _, exists := g.finalizedSignatures[key]; exists {
		return nil // Already processed
	}

	// Get the current authority set for this epoch
	authSet := g.GetCurrentScheduledAuthoritySet(uint64(epoch))
	if authSet == nil {
		return fmt.Errorf("no authority set found for epoch %d", epoch)
	}

	// Collect all DoublePublicKeys from the authority set
	pubkeys := make([]bls.DoublePublicKey, len(authSet))
	for i, validator := range authSet {
		var dpk bls.DoublePublicKey
		copy(dpk[:], validator.Bls[:])
		pubkeys[i] = dpk
	}

	// Verify the aggregate signature against the authority set
	// Message format: X_B ++ beefyHash (matching how it was signed in statedb/finalize.go)
	beefyBytes := append([]byte(types.X_B), beefyHash.Bytes()...)
	if !bls.AggregateVerify(pubkeys, aggregateSignature, beefyBytes) {
		return fmt.Errorf("aggregate BLS signature verification failed for epoch %d", epoch)
	}

	// Store the verified aggregate signature
	g.finalizedSignatures[key] = aggregateSignature

	return nil
}

func (g *Grandpa) ProcessBLSSignature(epoch uint32, beefyHash common.Hash, signature bls.Signature, validatorIndex uint16) error {
	// Create key from epoch and beefyHash
	key := fmt.Sprintf("%d_%s", epoch, beefyHash.Hex())

	g.singleBLSSignaturesMutex.Lock()
	defer g.singleBLSSignaturesMutex.Unlock()

	// Get the current authority set for this epoch
	authSet := g.GetCurrentScheduledAuthoritySet(uint64(epoch))
	if authSet == nil {
		return fmt.Errorf("no authority set found for epoch %d", epoch)
	}

	// Validate validator index
	if int(validatorIndex) >= len(authSet) {
		return fmt.Errorf("validator index %d out of range for authority set size %d", validatorIndex, len(authSet))
	}

	// Get validator BLS public key (DoublePublicKey = G1 (48 bytes) + G2 (96 bytes))
	validator := authSet[validatorIndex]
	doublePublicKey := validator.Bls

	// Extract G2 public key from the double public key (last 96 bytes)
	var g2PublicKey bls.G2PublicKey
	copy(g2PublicKey[:], doublePublicKey[bls.G1Len:]) // Skip first 48 bytes (G1), take next 96 bytes (G2)

	// Verify BLS signature against beefyHash
	// Message format: X_B ++ beefyHash (matching how it was signed in statedb/finalize.go)
	beefyBytes := append([]byte(types.X_B), beefyHash.Bytes()...)
	if !g2PublicKey.Verify(beefyBytes, signature) {
		return fmt.Errorf("BLS signature verification failed for validator %d at epoch %d", validatorIndex, epoch)
	}

	// Initialize map for this epoch+beefyHash if it doesn't exist
	if g.singleBLSSignatures[key] == nil {
		g.singleBLSSignatures[key] = make(map[uint16]bls.Signature)
	}

	// Store the verified signature
	g.singleBLSSignatures[key][validatorIndex] = signature

	// Check if we have enough signatures to aggregate and finalize
	threshold := g.GetSupermajorityThreshold(uint64(epoch))
	if len(g.singleBLSSignatures[key]) >= threshold {
		// Check if not already finalized
		g.finalizedSignaturesMutex.RLock()
		_, alreadyFinalized := g.finalizedSignatures[key]
		g.finalizedSignaturesMutex.RUnlock()

		if !alreadyFinalized {
			// Collect signatures into a slice
			signatures := make([]bls.Signature, 0, len(g.singleBLSSignatures[key]))
			for _, sig := range g.singleBLSSignatures[key] {
				signatures = append(signatures, sig)
			}

			// Aggregate signatures
			aggregatedSignature, err := bls.AggregateSignatures(signatures, beefyBytes)
			if err != nil {
				return fmt.Errorf("failed to aggregate BLS signatures for epoch %d: %w", epoch, err)
			}

			// Store the finalized aggregated signature
			g.finalizedSignaturesMutex.Lock()
			g.finalizedSignatures[key] = aggregatedSignature
			g.finalizedSignaturesMutex.Unlock()

			// Broadcast the finalized aggregated signature or trigger finalization
			g.broadcaster.FinalizedEpoch(epoch, beefyHash, aggregatedSignature)
		}
	}

	return nil
}
