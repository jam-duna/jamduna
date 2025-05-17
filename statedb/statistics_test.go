//go:build testing
// +build testing

package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/types"
)

func testStaticsSTF(t *testing.T, jsonFile string) {
	jsonPath := filepath.Join("../jamtestvectors/statistics/", jsonFile)
	expectedJson, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("failed to read JSON file: %v", err)
	}

	statisticsSTF := validator_statistics_test{}
	err = json.Unmarshal(expectedJson, &statisticsSTF)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON data: %v", err)
	}

	input := statisticsSTF.Input
	slot := input.Slot
	authorIndex := input.AuthorIndex
	extrinsic := input.Extrinsic
	et := extrinsic.Tickets
	eg := extrinsic.Guarantees
	ea := extrinsic.Assurances
	ep := extrinsic.Preimages

	prestate := statisticsSTF.Prestate
	pi := prestate

	tau := prestate.Tau
	kappa_prime := prestate.Kappa_prime
	jam_state := NewJamState()
	jam_state.SafroleState.CurrValidators = kappa_prime
	jam_state.SafroleState.Timeslot = tau
	jam_state.ValidatorStatistics.Current = pi.Current
	jam_state.ValidatorStatistics.Last = pi.Last
	jam_state.ValidatorStatistics.CoreStatistics = pi.CoreStatistics
	jam_state.ValidatorStatistics.ServiceStatistics = map[uint32]types.ServiceStatistics{}
	for _, service_state := range pi.ServiceStatics {
		jam_state.ValidatorStatistics.ServiceStatistics[uint32(service_state.ServiceIndex)] = service_state.ServiceStatistics
	}

	epoch_change := (slot / types.EpochLength) > (tau / types.EpochLength)

	if epoch_change {
		jam_state.ResetTallyStatistics()
	}
	JamState := jam_state.Copy()
	JamState.tallyStatistics(uint32(authorIndex), "tickets", uint32(len(et)))
	numReports := map[uint16]uint32{}
	for _, guarantee := range eg {
		for _, g := range guarantee.Signatures {
			_, ok := numReports[g.ValidatorIndex]
			if !ok {
				numReports[g.ValidatorIndex] = 1
			} else {
				numReports[g.ValidatorIndex]++
			}
		}
	}
	_, num_assurances := JamState.CountAvailableWR(ea)
	for validatorIndex, nassurances := range num_assurances {
		JamState.tallyStatistics(uint32(validatorIndex), "assurances", uint32(nassurances))
	}
	for validatorIndex, nreports := range numReports {
		JamState.tallyStatistics(uint32(validatorIndex), "reports", uint32(nreports))
	}

	num_preimage := uint32(len(ep))
	num_octets := uint32(0)
	for _, l := range ep {
		num_octets += l.BlobLength()
	}

	JamState.tallyStatistics(uint32(authorIndex), "preimages", num_preimage)
	JamState.tallyStatistics(uint32(authorIndex), "octets", num_octets)
	JamState.tallyStatistics(uint32(authorIndex), "blocks", 1)
	post_jam := state{}
	post_jam.Tau = prestate.Tau // should be: input.Slot
	post_jam.Current = JamState.ValidatorStatistics.Current
	post_jam.Last = JamState.ValidatorStatistics.Last
	post_jam.CoreStatistics = JamState.ValidatorStatistics.CoreStatistics
	post_jam.ServiceStatics = make(types.ServiceStatisticsKeyPairs, 0)
	post_jam.Kappa_prime = JamState.SafroleState.CurrValidators
	for k, v := range JamState.ValidatorStatistics.ServiceStatistics {
		post_jam.ServiceStatics = append(post_jam.ServiceStatics, types.ServiceStatisticsKeyPair{ServiceIndex: uint(k), ServiceStatistics: v})
	}

	poststate := statisticsSTF.Poststate // what we *should* get, of type "state"

	// check types equality
	if !reflect.DeepEqual(poststate, post_jam) {
		//original_diff := CompareJSON(prestate, poststate)
		//fmt.Printf("original changes:\n%v\n", original_diff)

		diff := CompareJSON(poststate, post_jam)
		if diff == "JSONs are identical" {

		} else {
			fmt.Printf("=====\nDIFFERENCE %s\n", jsonFile)
			fmt.Printf("post:\n%v\n", poststate.String())
			fmt.Printf("post_jam:\n%v\n", post_jam.String())
			t.Fatalf("poststate != post_jam: %v", diff)
		}
	}
}

func TestStaticsSTFVerify(t *testing.T) {
	network_args := *network
	fmt.Printf("Report Test Case (Guraantee Verification), Network=%s\n", network_args)
	testcase := []string{
		fmt.Sprintf("%s/stats_with_empty_extrinsic-1.json", network_args),
		fmt.Sprintf("%s/stats_with_epoch_change-1.json", network_args),
		fmt.Sprintf("%s/stats_with_some_extrinsic-1.json", network_args),
	}

	for _, tc := range testcase {
		t.Run(tc, func(t *testing.T) {
			testStaticsSTF(t, tc)
		})
	}

}
