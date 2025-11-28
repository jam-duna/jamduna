package grandpa

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
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

type Syncer interface {
	CatchUp(round uint64, setId uint32) (CatchUpResponse, error)
	WarpSync(fromSetID uint32) (types.WarpSyncResponse, error)
}

type GrandpaEventKind string

const (
	NewEpochEvent GrandpaEventKind = "NewEpoch"
	NewRoundEvent GrandpaEventKind = "NewRound"
)

type GrandpaEvent struct {
	Kind  GrandpaEventKind
	SetID uint32
	Round uint64
}

type GrandpaManager struct {
	Id            uint32
	blockTree     *types.BlockTree
	selfKey       types.ValidatorSecret
	AuthoritySet  []types.Validator
	Broadcaster   Broadcaster
	WhoRoundReady map[uint32]map[uint64]uint16
	WhoMutex      sync.Mutex
	Syncer        Syncer
	SyncChecker   map[uint32]map[uint64]struct{}
	Sync          bool
	SyncMutex     sync.Mutex
	FarBehind     bool

	Storage types.JAMStorage

	Grandpa      map[uint32]*Grandpa
	mutex        sync.Mutex
	EventChan    chan GrandpaEvent
	CurrentSetID uint32
	CurrentRound uint64
	ctx          context.Context
	cancel       context.CancelFunc

	// CE149
	PreVoteMessageCh   chan GrandpaVote
	PreCommitMessageCh chan GrandpaVote
	PrimaryMessageCh   chan GrandpaVote

	// CE150
	CommitMessageCh chan GrandpaCommitMessage

	// CE151
	StateMessageCh chan GrandpaStateMessage

	// CE152
	CatchUpMessageCh chan GrandpaCatchUp

	// CE153
	WarpSyncRequestCh chan uint32
}

func (gm *GrandpaManager) SetWhoRoundReady(setID uint32, round uint64, peerID uint16) {
	gm.WhoMutex.Lock()
	defer gm.WhoMutex.Unlock()
	if _, ok := gm.WhoRoundReady[setID]; !ok {
		gm.WhoRoundReady[setID] = make(map[uint64]uint16)
	}
	if _, ok := gm.WhoRoundReady[setID][round]; ok {
		return
	}
	gm.WhoRoundReady[setID][round-1] = peerID
}

func (gm *GrandpaManager) GetWhoRoundReady(setID uint32, round uint64) (uint16, bool) {
	gm.WhoMutex.Lock()
	defer gm.WhoMutex.Unlock()
	if rounds, ok := gm.WhoRoundReady[setID]; ok {
		if peerID, ok := rounds[round]; ok {
			return peerID, true
		}
	}
	return 0, false
}

func (gm *GrandpaManager) IsSyncing() bool {
	gm.SyncMutex.Lock()
	defer gm.SyncMutex.Unlock()
	return gm.Sync
}

func (gm *GrandpaManager) SetSyncing(syncing bool) {
	gm.SyncMutex.Lock()
	defer gm.SyncMutex.Unlock()
	gm.Sync = syncing
}

func NewGrandpaManager(blockTree *types.BlockTree, selfKey types.ValidatorSecret) *GrandpaManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &GrandpaManager{
		blockTree:          blockTree,
		selfKey:            selfKey,
		Grandpa:            make(map[uint32]*Grandpa),
		EventChan:          make(chan GrandpaEvent, DefaultChannelSize),
		ctx:                ctx,
		cancel:             cancel,
		PreVoteMessageCh:   make(chan GrandpaVote, DefaultChannelSize),
		PreCommitMessageCh: make(chan GrandpaVote, DefaultChannelSize),
		PrimaryMessageCh:   make(chan GrandpaVote, DefaultChannelSize),
		CommitMessageCh:    make(chan GrandpaCommitMessage, DefaultChannelSize),
		StateMessageCh:     make(chan GrandpaStateMessage, DefaultChannelSize),
		CatchUpMessageCh:   make(chan GrandpaCatchUp, DefaultChannelSize),
		WarpSyncRequestCh:  make(chan uint32, DefaultChannelSize),
		SyncChecker:        make(map[uint32]map[uint64]struct{}),
		WhoRoundReady:      make(map[uint32]map[uint64]uint16),
	}
}

func (gm *GrandpaManager) SetGrandpa(setID uint32, grandpa *Grandpa) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	grandpa.EventChan = gm.EventChan
	gm.Grandpa[setID] = grandpa
}

func (gm *GrandpaManager) GetGrandpa(setID uint32) (*Grandpa, bool) {
	gm.mutex.Lock()
	defer gm.mutex.Unlock()
	grandpa, exists := gm.Grandpa[setID]
	return grandpa, exists
}

func (gm *GrandpaManager) GetOrInitializeGrandpa(setID uint32) *Grandpa {
	gm.mutex.Lock()
	grandpa, exists := gm.Grandpa[setID]
	gm.mutex.Unlock()
	if !exists {
		// Share the live block tree so the new authority set keeps seeing imported blocks.
		grandpa = NewGrandpa(gm.blockTree, gm.selfKey, gm.AuthoritySet, nil, gm.Broadcaster, gm.Storage, setID)
		gm.SetGrandpa(setID, grandpa)
	}
	return grandpa
}

// isBehind checks if we are behind the given set ID and round
func (gm *GrandpaManager) isBehind(targetSetID uint32, targetRound uint64) bool {
	if targetSetID > gm.CurrentSetID {
		return true
	}
	if targetSetID == gm.CurrentSetID && targetRound > gm.CurrentRound+1 {
		return true
	}
	return false
}

// isNextRound checks if the state message represents the next expected round
func (gm *GrandpaManager) isNextRound(targetSetID uint32, targetRound uint64) bool {
	return targetSetID == gm.CurrentSetID && targetRound == gm.CurrentRound+1
}

// handleStateMessage processes incoming state messages and triggers catch-up if needed
func (gm *GrandpaManager) handleStateMessage(state GrandpaStateMessage) {
	targetSetID := state.SetId
	targetRound := state.Round

	// Dedup: check if we've already processed this state
	if _, ok := gm.SyncChecker[targetSetID]; !ok {
		gm.SyncChecker[targetSetID] = make(map[uint64]struct{})
	}
	if _, ok := gm.SyncChecker[targetSetID][targetRound]; ok {
		return // already processing
	}
	gm.SyncChecker[targetSetID][targetRound] = struct{}{}

	log.Trace(log.Grandpa, "StateMessage received",
		"node", gm.Id, "targetSet", targetSetID, "targetRound", targetRound,
		"currentSet", gm.CurrentSetID, "currentRound", gm.CurrentRound, "syncing", gm.IsSyncing())

	// Case 1: We're synced and this is the next round - advance normally
	if gm.IsSyncing() && gm.isNextRound(targetSetID, targetRound) {
		gm.EventChan <- GrandpaEvent{
			Kind:  NewRoundEvent,
			SetID: targetSetID,
			Round: targetRound,
		}
		return
	}

	// Case 2: We're behind - need to catch up
	if gm.isBehind(targetSetID, targetRound) {
		gm.SetSyncing(false)
		go gm.catchUpTo(targetSetID, targetRound)
		return
	}

	// Case 3: We're syncing but received a future state - set a timeout to detect if we fall behind
	if gm.IsSyncing() && (targetSetID != gm.CurrentSetID || targetRound > gm.CurrentRound+1) {
		go gm.monitorSyncProgress(targetSetID, targetRound)
	}
}

// catchUpTo catches up from current position to target set ID and round
func (gm *GrandpaManager) catchUpTo(targetSetID uint32, targetRound uint64) {
	log.Info(log.Grandpa, "Starting catch-up",
		"node", gm.Id, "fromSet", gm.CurrentSetID, "fromRound", gm.CurrentRound,
		"toSet", targetSetID, "toRound", targetRound)

	// If we're behind on authority sets, use warp sync first to quickly update
	if targetSetID > gm.CurrentSetID {
		if err := gm.warpSyncToSet(targetSetID); err != nil {
			log.Warn(log.Grandpa, "Warp sync failed, falling back to round-by-round catch-up",
				"node", gm.Id, "err", err)
			// Fall through to round-by-round catch-up
		}
	}

	// After warp sync (or if it failed), catch up remaining rounds in target set
	gm.catchUpRoundsInSet(targetSetID, targetRound)

	// Now we should be caught up - join the target round live
	gm.SetSyncing(true)
	gm.EventChan <- GrandpaEvent{
		Kind:  NewRoundEvent,
		SetID: targetSetID,
		Round: targetRound,
	}
}

// warpSyncToSet uses warp sync to quickly update authority sets from current to target
func (gm *GrandpaManager) warpSyncToSet(targetSetID uint32) error {
	if gm.Syncer == nil {
		return fmt.Errorf("syncer not configured")
	}

	log.Info(log.Grandpa, "Starting warp sync for authority sets",
		"node", gm.Id, "fromSet", gm.CurrentSetID, "toSet", targetSetID)

	// Request warp sync fragments from peers
	response, err := gm.Syncer.WarpSync(gm.CurrentSetID)
	if err != nil {
		return fmt.Errorf("warp sync request failed: %w", err)
	}

	if len(response.Fragments) == 0 {
		return fmt.Errorf("no warp sync fragments received")
	}

	log.Info(log.Grandpa, "Received warp sync fragments",
		"node", gm.Id, "count", len(response.Fragments))

	// Process each fragment to update authority sets
	for _, fragment := range response.Fragments {
		setID := fragment.Justification.SetId

		// Skip fragments we've already processed
		if setID < gm.CurrentSetID {
			continue
		}

		// Stop if we've reached target
		if setID >= targetSetID {
			break
		}

		// Initialize grandpa for this set if needed
		grandpa := gm.GetOrInitializeGrandpa(setID)

		// Verify and process the warp sync fragment
		if err := grandpa.ProcessWarpSyncFragment(fragment); err != nil {
			log.Error(log.Grandpa, "Failed to process warp sync fragment",
				"node", gm.Id, "setID", setID, "err", err)
			return err
		}

		// Update current set ID after successful processing
		gm.CurrentSetID = setID + 1
		gm.CurrentRound = 0

		log.Info(log.Grandpa, "Warp sync: updated authority set",
			"node", gm.Id, "setID", setID, "newCurrentSet", gm.CurrentSetID)
	}

	return nil
}

// catchUpRoundsInSet catches up rounds within a single authority set
func (gm *GrandpaManager) catchUpRoundsInSet(setID uint32, targetRound uint64) {
	grandpa := gm.GetOrInitializeGrandpa(setID)

	// Determine starting round
	startRound := uint64(0)
	if setID == gm.CurrentSetID {
		startRound = gm.CurrentRound + 1
	}

	// Target round - catch up to one before target so we can join live
	var endRound uint64
	if targetRound > 0 {
		endRound = targetRound - 1
	} else {
		endRound = 0
	}

	if startRound > endRound {
		return // Nothing to catch up
	}

	log.Debug(log.Grandpa, "Catching up rounds in set",
		"node", gm.Id, "setID", setID, "startRound", startRound, "endRound", endRound)

	for round := startRound; round <= endRound; round++ {
		select {
		case <-gm.ctx.Done():
			return
		default:
		}

		catchUpRes, err := gm.Syncer.CatchUp(round, setID)
		if err != nil {
			log.Error(log.Grandpa, "CatchUp failed", "node", gm.Id, "setID", setID, "round", round, "err", err)
			continue
		}

		err = grandpa.PlayRoundWithCatchUp(gm.ctx, round, catchUpRes)
		if err != nil {
			log.Error(log.Grandpa, "PlayRoundWithCatchUp failed", "node", gm.Id, "setID", setID, "round", round, "err", err)
			continue
		}

		gm.CurrentSetID = setID
		gm.CurrentRound = round
		log.Info(log.Grandpa, "Caught up", "node", gm.Id, "setID", setID, "round", round)
	}
}

// getLastRoundForSet returns the last completed round for a given set ID
// This queries storage for stored catch-up messages to find the highest round
func (gm *GrandpaManager) getLastRoundForSet(setID uint32) uint64 {
	if gm.Storage == nil {
		return 0
	}
	// Search for the highest round that has a stored catch-up message
	// Start from a reasonable upper bound and work backwards
	for round := uint64(1000); round > 0; round-- {
		_, exists, err := gm.Storage.GetCatchupMassage(round, setID)
		if err == nil && exists {
			return round
		}
	}
	return 0
}

// monitorSyncProgress monitors if we're making progress and triggers catch-up if not
func (gm *GrandpaManager) monitorSyncProgress(expectedSetID uint32, expectedRound uint64) {
	select {
	case <-time.After(6 * time.Second):
		// Check if we've caught up
		if gm.isBehind(expectedSetID, expectedRound) {
			log.Warn(log.Grandpa, "Still behind after timeout",
				"node", gm.Id, "expectedSet", expectedSetID, "expectedRound", expectedRound,
				"currentSet", gm.CurrentSetID, "currentRound", gm.CurrentRound)
			gm.SetSyncing(false)
			go gm.catchUpTo(expectedSetID, expectedRound)
		}
	case <-gm.ctx.Done():
		return
	}
}

func (gm *GrandpaManager) RunManager() {
	gm.SetSyncing(true)
	for {
		select {
		case state := <-gm.StateMessageCh:
			gm.handleStateMessage(state)
		case event := <-gm.EventChan:
			switch event.Kind {
			case NewEpochEvent:
				if event.SetID <= gm.CurrentSetID {
					// Ignore stale epoch events
					continue
				}
				log.Info(log.Grandpa, "Manager event", "node", gm.Id, "kind", event.Kind, "setID", event.SetID, "round", event.Round)
				gm.CurrentSetID = event.SetID
				gm.CurrentRound = event.Round // typically 0 for a new epoch
				grandpa := gm.GetOrInitializeGrandpa(gm.CurrentSetID)
				grandpaState := GrandpaStateMessage{
					Round: gm.CurrentRound,
					SetId: gm.CurrentSetID,
					Slot:  uint32(gm.blockTree.GetLastFinalizedBlock().Block.Header.Slot),
				}
				grandpa.broadcaster.Broadcast(grandpaState)
				go func() {
					grandpa.PlayGrandpaRound(gm.ctx, gm.CurrentRound)
					// after play grandpa round, we should store the catup massage

				}()

			case NewRoundEvent:
				if event.SetID != gm.CurrentSetID {
					// Stale event from a previous authority set
					continue
				}
				if event.Round <= gm.CurrentRound {
					// Already running or completed this round
					continue
				}
				if event.Round == 4 && gm.Id == 5 {
					continue
				}
				log.Info(log.Grandpa, "Manager event", "node", gm.Id, "kind", event.Kind, "setID", event.SetID, "round", event.Round)
				gm.CurrentRound = event.Round
				if grandpa, exists := gm.GetGrandpa(gm.CurrentSetID); exists {
					grandpaState := GrandpaStateMessage{
						Round: gm.CurrentRound,
						SetId: gm.CurrentSetID,
						Slot:  uint32(gm.blockTree.GetLastFinalizedBlock().Block.Header.Slot),
					}
					grandpa.broadcaster.Broadcast(grandpaState)
					go func() {
						grandpa.PlayGrandpaRound(gm.ctx, gm.CurrentRound)
					}()
				}
			}
		case <-gm.ctx.Done():
			return
		}
	}
}

func (gm *GrandpaManager) Stop() {
	if gm.cancel != nil {
		gm.cancel()
	}
}

type Grandpa struct {
	block_tree           *types.BlockTree // hold the same block tree as the node itself
	RoundStateMutex      sync.RWMutex
	RoundState           map[uint64]*RoundState
	selfkey              types.ValidatorSecret
	Last_Completed_Round uint64

	LastFinalizedSlot     uint32
	ScheduledAuthoritySet []types.Validator
	UpcomingAuthoritySet  []types.Validator
	authority_set_id      uint32

	Voter_Staked map[types.Ed25519Key]uint64

	broadcaster Broadcaster
	storage     types.JAMStorage

	// event channel
	EventChan chan GrandpaEvent

	roundMu      sync.Mutex
	activeRounds map[uint64]struct{}

	// BLS signature tracking: map[epoch_beefyHash]map[validatorIndex]signature
	singleBLSSignatures      map[string]map[uint16]bls.Signature
	singleBLSSignaturesMutex sync.RWMutex

	// Finalized aggregated signatures: map[epoch_beefyHash]aggregatedSignature
	finalizedSignatures      map[string]bls.Signature
	finalizedSignaturesMutex sync.RWMutex
}

// RunGrandpa receives grandpa messages and routes them to the appropriate Grandpa instance
func (gm *GrandpaManager) RunGrandpa() {
	for {
		select {
		case vote := <-gm.PreVoteMessageCh: // CE149
			auth_set_id := vote.SetId
			grandpa := gm.GetOrInitializeGrandpa(auth_set_id)
			err := grandpa.ProcessPreVoteMessage(vote)
			if err != nil {
				fmt.Println(err)
			}
		case vote := <-gm.PreCommitMessageCh: // CE149
			auth_set_id := vote.SetId
			grandpa := gm.GetOrInitializeGrandpa(auth_set_id)
			err := grandpa.ProcessPreCommitMessage(vote)
			if err != nil {
				fmt.Println(err)
			}

		case vote := <-gm.PrimaryMessageCh: // CE149
			auth_set_id := vote.SetId
			grandpa := gm.GetOrInitializeGrandpa(auth_set_id)
			err := grandpa.ProcessPrimaryProposeMessage(vote)
			if err != nil {
				fmt.Println(err)
			}

		case commit := <-gm.CommitMessageCh: // CE150
			auth_set_id := commit.SetId
			grandpa := gm.GetOrInitializeGrandpa(auth_set_id)
			grandpa.ProcessCommitMessage(commit)

		case catchup := <-gm.CatchUpMessageCh: // CE152
			auth_set_id := catchup.SetId
			grandpa := gm.GetOrInitializeGrandpa(auth_set_id)
			response, err := grandpa.ProcessCatchUpMessage(catchup)
			if err != nil {
				fmt.Println(err)
			}
			grandpa.ProcessCatchUpResponse(response, auth_set_id)
		case warpReq := <-gm.WarpSyncRequestCh: // CE153
			// Warp sync requests don't have a SetId, use current set
			grandpa, exist := gm.GetGrandpa(gm.CurrentSetID)
			if exist {
				response, err := grandpa.ProcessWarpSyncRequest(warpReq)
				if err != nil {
					fmt.Println(err)
				}
				grandpa.ProcessWarpSyncResponse(response)
			}
		case <-gm.ctx.Done():
			return
		}
	}
}

type GrandpaRound struct {
	Round        uint64
	Timer1Signal chan bool
	Timer2Signal chan bool
	Ticker       *time.Ticker
}

func (g *Grandpa) tryStartRound(round uint64) bool {
	g.roundMu.Lock()
	defer g.roundMu.Unlock()
	if g.activeRounds == nil {
		g.activeRounds = make(map[uint64]struct{})
	}
	if _, exists := g.activeRounds[round]; exists {
		return false
	}
	g.activeRounds[round] = struct{}{}
	return true
}

func (g *Grandpa) finishRound(round uint64) {
	g.roundMu.Lock()
	delete(g.activeRounds, round)
	g.roundMu.Unlock()
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

func NewGrandpa(block_tree *types.BlockTree, selfkey types.ValidatorSecret, authSet0 []types.Validator, genesis_blk *types.Block, broadcaster Broadcaster, stor types.JAMStorage, authority_set_id uint32) *Grandpa {

	staked := make(map[types.Ed25519Key]uint64)
	for _, validator := range authSet0 {
		staked[validator.Ed25519] = 100
	}
	g := &Grandpa{
		block_tree:           block_tree,
		RoundState:           make(map[uint64]*RoundState),
		selfkey:              selfkey,
		Last_Completed_Round: 0,
		Voter_Staked:         staked,
		broadcaster:          broadcaster,
		storage:              stor,
		authority_set_id:     authority_set_id,
		singleBLSSignatures:  make(map[string]map[uint16]bls.Signature),
		finalizedSignatures:  make(map[string]bls.Signature),
		activeRounds:         make(map[uint64]struct{}),
	}
	g.ScheduledAuthoritySet = authSet0
	g.UpcomingAuthoritySet = authSet0
	g.InitRoundState(0, block_tree.Copy())
	g.InitTrackers(0)

	// This is sufficient to get everyone to broadcast finalization of genesis
	if genesis_blk != nil {
		g.Broadcast(g.NewPrecommitVoteMessage(genesis_blk, 0))
	}
	// commit, _ := g.NewFinalCommitMessage(genesis_blk, 0)
	// g.Broadcast(g.NewPrevoteVoteMessage(genesis_blk, 0))
	// g.Broadcast(commit)
	return g
}

func (g *Grandpa) Broadcast(msg interface{}) {
	g.broadcaster.Broadcast(msg)
}

func (g *Grandpa) GetCurrentScheduledAuthoritySet(round uint64) []types.Validator {
	return g.ScheduledAuthoritySet // TODO: need to change to round
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

func (g *Grandpa) SetScheduledAuthoritySet(authorities []types.Validator) {
	g.ScheduledAuthoritySet = authorities
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
	// For round 0, return the last finalized block
	if round == 0 {
		last_finalized := g.block_tree.GetLastFinalizedBlock()
		return last_finalized.Block.Header.Hash(), last_finalized, nil
	}

	// Get GHOST for current round
	ghost_hash, ghost, err := g.GrandpaGhost(round)
	if err != nil {
		return common.Hash{}, nil, err
	}

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
	old_ghost_block, ok := g.block_tree.GetBlockNode(old_ghost.Block.Header.Hash())
	if !ok {
		return ghost_hash, ghost, nil
	}
	older := g.block_tree.ChildOrBrother(ghost, old_ghost_block)
	if !older {
		return ghost_hash, ghost, nil
	}
	if old_ghost_block.Block.GetParentHeaderHash() == ghost.Block.GetParentHeaderHash() {
		return ghost_hash, ghost, nil
	} else {
		return old_ghost_block.Block.Header.Hash(), old_ghost_block, nil
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
	// For round 0, return last finalized block
	if round == 0 {
		last_finalized := g.block_tree.GetLastFinalizedBlock()
		return last_finalized.Block.Header.Hash(), last_finalized, nil
	}

	round_state, err := g.GetRoundState(round)
	if err != nil {
		return common.Hash{}, nil, err
	}
	// get the ghost block
	var (
		L_Hash  common.Hash
		L_block *types.BT_Node
	)
	if round == 0 {
		L_block = g.block_tree.GetLastFinalizedBlock()
		if L_block == nil {
			return common.Hash{}, nil, fmt.Errorf("last finalized block not found")
		}
		L_Hash = L_block.Block.Header.Hash()
	} else {
		L_Hash, L_block, err = g.BestFinalCandidate(round - 1)
		if err != nil {
			return common.Hash{}, nil, err
		}
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
		EpochChanged := false
		if best_final_candidate.Block.Header.Slot/types.EpochLength > last_finalized_block.Block.Header.Slot/types.EpochLength {
			EpochChanged = true
		}
		g.LastFinalizedSlot = best_final_candidate.Block.Header.Slot
		if EpochChanged && g.EventChan != nil {
			newSetEvent := GrandpaEvent{
				Kind:  NewEpochEvent,
				SetID: g.authority_set_id + 1,
				Round: 0,
			}
			g.EventChan <- newSetEvent
		}
		commit, err := g.NewFinalCommitMessage(best_final_candidate.Block, round)
		if err != nil {
			return err
		}
		g.Broadcast(commit)
		if EpochChanged {
			// store the grandpa warp sync info
			var warpInfo types.WarpSyncFragment
			warpInfo.Header = best_final_candidate.Block.Header
			warpInfo.Justification.Commit = commit.Commit
			warpInfo.Justification.Round = round
			warpInfo.Justification.SetId = g.authority_set_id
			warpInfo.Justification.VotesAncestries = g.GetVotesAncestries(commit.Commit, best_final_candidate.Block.Header.Hash())
			err = g.storage.StoreWarpSyncFragment(g.authority_set_id, warpInfo)
			if err != nil {
				return err
			}
		}

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
	var best_candidate_m1_block *types.BT_Node
	if round == 0 {
		best_candidate_m1_block = g.block_tree.GetLastFinalizedBlock()
		if best_candidate_m1_block == nil {
			return false, fmt.Errorf("last finalized block not found")
		}
	} else {
		_, best_candidate_m1_block, err = g.BestFinalCandidate(round - 1)
		if err != nil {
			return false, err
		}
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
	g.ScheduledAuthoritySet = validators
}

func (g *Grandpa) GetAuthoritySet() []types.Validator {
	return g.ScheduledAuthoritySet
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

func (g *Grandpa) GetVotesAncestries(commit types.GrandpaCommit, excludeHeader common.Hash) []types.BlockHeader {
	blockHeaders := make(map[common.Hash]types.BlockHeader)
	for _, precommit := range commit.Precommits {
		blockHash := precommit.Vote.HeaderHash
		if blockHash == excludeHeader {
			continue
		}
		if _, exists := blockHeaders[blockHash]; !exists {
			blockNode, ok := g.block_tree.GetBlockNode(blockHash)
			if ok {
				blockHeaders[blockHash] = blockNode.Block.Header
			}
		}
	}
	headers := make([]types.BlockHeader, 0, len(blockHeaders))
	for _, header := range blockHeaders {
		headers = append(headers, header)
	}
	return headers
}
