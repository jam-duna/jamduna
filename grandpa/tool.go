package grandpa

// get the index of the voter in the grandpa_authorities
func (g *Grandpa) GetSelfVoterIndex(round uint64) uint64 {
	grandpa_state := g.GetRoundGrandpaState(round)
	if grandpa_state == nil {
		g.InitRoundState(round, g.block_tree.Copy()) // initialize the round state if it doesn't exist
		grandpa_state = g.GetRoundGrandpaState(round)
	}
	for i, key := range grandpa_state.grandpa_authorities {
		if key == g.selfkey.PublicKey() {
			return uint64(i)
		}
	}
	return 0
}
