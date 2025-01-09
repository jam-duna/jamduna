## Files
* `const.go` : constants for grandpa
* `type.go` : grandpa type like messages
* `grandpa.go` : basic Algorithms for play grandpa ground
* `play_ground.go` : main process for grandpa update the finality
* `message.go` : functions for the process and produce messages
* `round_state.go` : handle round state and round process

## useful link
[Finality](https://spec.polkadot.network/sect-finality)
[voting process](https://spec.polkadot.network/sect-finality#id-voting-process-in-round-r)
[GRANDPA Messages](https://spec.polkadot.network/chap-networking#sect-msg-grandpa)
[parity implemetaion](https://github.com/paritytech/finality-grandpa/tree/master)


## grandpa TODO

- [x] block_tree
- [x] block_tree_test
- [x] grandpa basic setup
- [x] Algorithms
- [x] Network
- [x] Apply in system
- [x] Pruned Block Tree
- [ ] optimize the system
- [ ] setup fuzz test
