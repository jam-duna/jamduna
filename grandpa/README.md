# GRANDPA

This `grandpa` package implements [GRANDPA Finality](https://spec.polkadot.network/sect-finality) with the [voting process](https://spec.polkadot.network/sect-finality#id-voting-process-in-round-r) with [GRANDPA Messages](https://spec.polkadot.network/chap-networking#sect-msg-grandpa) following the [JAM-NP Spec](https://github.com/MattHalpinParity/jam-np/blob/257246ce58a828076992dcb5480a835c12677ce9/simple.md#ce-130-grandpa-justification-request):

The `node` package has each `Node` having a `Grandpa` object running a go routine receiving and responding to Grandpa Message CE149-CE153:

* [CE 149 Grandpa Vote](https://github.com/MattHalpinParity/jam-np/blob/257246ce58a828076992dcb5480a835c12677ce9/simple.md#ce-130-grandpa-justification-request)  - `grandpavote.go` : GrandpaVote message type (prevote, precommit, primary propose) and processing (ProcessPreVoteMessage, ProcessPreCommitMessage, etc.)
* [CE 150 Grandpa Commit](https://github.com/MattHalpinParity/jam-np/blob/257246ce58a828076992dcb5480a835c12677ce9/simple.md#ce-150-grandpa-commit)  -  `grandpacommit.go` : GrandpaCommit message type (final commit with justification)
* [CE 151 Grandpa State](https://github.com/MattHalpinParity/jam-np/blob/257246ce58a828076992dcb5480a835c12677ce9/simple.md#ce-151-grandpa-state) - `grandpastate.go` : GrandpaStateMessage type (state synchronization)
* [CE 152 Grandpa Catchup](https://github.com/MattHalpinParity/jam-np/blob/257246ce58a828076992dcb5480a835c12677ce9/simple.md#ce-152-grandpa-catchup) - `grandpacatchup.go` : GrandpaCatchUp and CatchUpResponse types
* [CE 153 Warp Sync Request](https://github.com/MattHalpinParity/jam-np/blob/257246ce58a828076992dcb5480a835c12677ce9/simple.md#ce-153-warp-sync-request) - `warpsync.go` : WarpSyncRequest and WarpSyncResponse types


The GHOST algorithm is implemented with VoteTracker, Round structures with a BlockTree integration in the following files:

* `grandpa.go` : Core GRANDPA implementation with finality algorithms and Broadcaster interface
* `round.go` : Round management and voting logic (PlayGrandpaRound)
* `round_state.go` : Round state tracking and vote graph management
* `vote_tracker.go` : Vote tracking and supermajority detection

Test:
* `vote_graph_test.go` : Fork resolution and finality tests, including block tree pruning

### Testing Grandpa:

The `Node` object implements a `Broadcaster` interface pattern, mocked in `MockGrandpaNode`

* `grandpa_test.go` : Mock network of 6 `MockGrandpaNode`s (each MockGrandpaNode  implementing Broadcaster interface for testing)  with `NewGrandpa` each running `RunGrandpa` 


Key test:
```
go test -run=TestGrandpa
```

## TODO:

- [ ] [SN] Complete CatchUp implementation, WarpSync implementation, backed by JAMStorage + BLS Finality
- [ ] [SC] PoC in TestAlgoBlocks with auditing => make algo_compiler runner with nomt [with MC]

