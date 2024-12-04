package main

import (
	"github.com/colorfulnotion/jam/statedb"
	"testing"
)

func readStateTransitions(dir string) (stfs []*statedb.StateTransition) {
	stfs = make([]*statedb.StateTransition, 0)
	// TODO: read a StateTransition
	return stfs
}

func testFuzz(t *testing.T, stfs []*statedb.StateTransition, errors []error) {
	// TODO: hit all the errors with the stfs
}

func testFuzzSafrole(t *testing.T) {
	stfs := readStateTransitions("safrole")
	testFuzz(t, stfs, ErrorMap["safrole"])
}

func testFuzzReports(t *testing.T) {
	stfs := readStateTransitions("assurances")
	testFuzz(t, stfs, ErrorMap["reports"])
}

func testFuzzAssurances(t *testing.T) {
	stfs := readStateTransitions("assurances")
	testFuzz(t, stfs, ErrorMap["assurances"])
}

func testFuzzDisputes(t *testing.T) {
	stfs := readStateTransitions("disputes")
	testFuzz(t, stfs, ErrorMap["disputes"])
}

func testFuzzPreimages(t *testing.T) {
	stfs := readStateTransitions("disputes")
	testFuzz(t, stfs, ErrorMap["disputes"])
}

func TestFuzz(t *testing.T) {
	testFuzzSafrole(t)
	testFuzzReports(t)
	testFuzzAssurances(t)
	//testFuzzDisputes(t)
	//testFuzzPreimages(t)
}
