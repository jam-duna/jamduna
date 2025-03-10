package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	rand0 "math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

func ReadDisputableStateTransitions(baseDir, dir string) (stfs []*statedb.StateTransition, err error) {
	stfs = make([]*statedb.StateTransition, 0)
	state_transitions_dir := filepath.Join(baseDir, dir, "state_transitions")
	stFiles, err := os.ReadDir(state_transitions_dir)
	if err != nil {
		//panic(fmt.Sprintf("failed to read directory: %v\n", err))
		return stfs, fmt.Errorf("failed to read directory: %v", err)
	}
	fmt.Printf("Selected Dir: %v\n", dir)
	file_idx := 0
	for _, file := range stFiles {
		if strings.HasSuffix(file.Name(), ".bin") {
			file_idx++
			// Extract epoch and phase from filename `${epoch}_${phase}.bin`
			parts := strings.Split(strings.TrimSuffix(file.Name(), ".bin"), "_")
			if len(parts) != 2 {
				log.Printf("Invalid block filename format: %s\n", file.Name())
				continue
			}

			// Read the st file
			stPath := filepath.Join(state_transitions_dir, file.Name())
			stBytes, err := os.ReadFile(stPath)
			if err != nil {
				log.Printf("Error reading block file %s: %v\n", file.Name(), err)
				continue
			}

			// Decode st from stBytes
			b, _, err := types.Decode(stBytes, reflect.TypeOf(statedb.StateTransition{}))
			if err != nil {
				log.Printf("Error decoding block %s: %v\n", file.Name(), err)
				continue
			}
			// Store the state transition in the stateTransitions map
			stf := b.(statedb.StateTransition)
			stfs = append(stfs, &stf)
		}
	}
	fmt.Printf("Loaded %v state transitions\n", len(stfs))
	return stfs, nil
}

func MakeJudgement(workreport types.WorkReport, auditPass bool, validatorindex uint16, validator_secret types.ValidatorSecret) (judgement types.Judgement) {
	judgement = types.Judgement{
		Judge:          auditPass,
		Validator:      validatorindex,
		WorkReportHash: workreport.Hash(),
	}
	judgement.Sign(validator_secret.Ed25519Secret[:])
	return judgement
}

func MakeDisputes(store *storage.StateDBStorage, stf *statedb.StateTransition, validator_secrerts []types.ValidatorSecret, guarantees_history []types.Guarantee) (disputable bool, err error, dispute_stf *statedb.StateTransition) {
	disputable = false
	dispute_stf = nil
	// Check if the state transition is disputable
	block := stf.Block
	prestate, err := statedb.NewStateDBFromSnapshotRaw(store, &stf.PreState)
	if err != nil {
		return disputable, err, dispute_stf
	}
	poststate, err := statedb.NewStateDBFromSnapshotRaw(store, &stf.PostState)
	if err != nil {
		return disputable, err, dispute_stf
	}
	// Check if the block is disputable
	poststate.Block = block.Copy()
	prestatecopy := prestate.Copy()
	_, availableWorkReport := prestatecopy.JamState.ProcessAssurances(poststate.Block.Extrinsic.Assurances, poststate.GetTimeslot())
	// start making the dispute
	// random choose the number from 1 to availableWorkReport_len

	var ramdomNum int
	if len(availableWorkReport) > 0 {
		ramdomNum = rand0.Intn(len(availableWorkReport)) + 1
		fmt.Printf("num of reports will be dispute: %d\n", ramdomNum)
		disputable = true
	} else {
		ramdomNum = 0
	}
	// ramdom choose ramdom num of the availableWorkReport
	// Fisher-Yates shuffle to randomly choose the work report that will be disputed
	for i := len(availableWorkReport) - 1; i > 0; i-- {
		j := rand0.Intn(i + 1)
		availableWorkReport[i], availableWorkReport[j] = availableWorkReport[j], availableWorkReport[i]
	}
	disputeWorkReports := availableWorkReport[0:ramdomNum]
	judgement_bucket := types.JudgeBucket{}
	// use ramdom sign sequence to shuffle the validator_secrerts
	// ramdom permute the ramdom_sign_sequence
	dispute_set := make(map[common.Hash]string)
	for _, dispute_report := range disputeWorkReports {
		// ramdom select three mode from 0 to 2
		mode := rand0.Intn(3)
		validator_len := len(validator_secrerts)
		ramdom_sign_sequence := rand0.Perm(validator_len)
		var old_eg types.Guarantee
		for _, guarantee := range guarantees_history {
			if guarantee.Report.AvailabilitySpec.WorkPackageHash == dispute_report.GetWorkPackageHash() {
				old_eg = guarantee
				break
			}
		}

		switch mode {
		case 0:
			dispute_set[dispute_report.GetWorkPackageHash()] = "goodset"
			// goodset over 2/3+1 validators votes yes
			// ramdom selet 1/3 (can be less than 1/3) validators to vote no
			for i := 0; i < validator_len; i++ {
				choosen_validator := validator_secrerts[ramdom_sign_sequence[i]]
				if i < validator_len/3-1 {

					judgement := MakeJudgement(dispute_report, false, uint16(ramdom_sign_sequence[i]), choosen_validator)
					judgement_bucket.PutJudgement(judgement)

				} else {
					judgement := MakeJudgement(dispute_report, true, uint16(ramdom_sign_sequence[i]), choosen_validator)
					judgement_bucket.PutJudgement(judgement)
				}
			}
		case 1:
			dispute_set[dispute_report.GetWorkPackageHash()] = "badset"
			// badset over 2/3+1 validators votes no
			// ramdom selet 1/3 (can be less than 1/3) validators to vote yes
			for i := 0; i < validator_len; i++ {
				choosen_validator := validator_secrerts[ramdom_sign_sequence[i]]
				if i < validator_len/3-1 {

					judgement := MakeJudgement(dispute_report, true, uint16(ramdom_sign_sequence[i]), choosen_validator)
					judgement_bucket.PutJudgement(judgement)

				} else {
					judgement := MakeJudgement(dispute_report, false, uint16(ramdom_sign_sequence[i]), choosen_validator)
					judgement_bucket.PutJudgement(judgement)
				}
			}
		case 2:
			dispute_set[dispute_report.GetWorkPackageHash()] = "wonkeyset"
			// no one exceed 2/3+1 validators votes yes or no
			for i := 0; i < validator_len; i++ {
				choosen_validator := validator_secrerts[ramdom_sign_sequence[i]]
				if i%2 == 0 {
					judgement := MakeJudgement(dispute_report, false, uint16(ramdom_sign_sequence[i]), choosen_validator)
					judgement_bucket.PutJudgement(judgement)
				} else {
					judgement := MakeJudgement(dispute_report, true, uint16(ramdom_sign_sequence[i]), choosen_validator)
					judgement_bucket.PutJudgement(judgement)
				}
			}
		}
		disputeblock, err_dispute := poststate.AppendDisputes(&judgement_bucket, dispute_report.Hash(), old_eg)
		if disputeblock == nil {
			panic(fmt.Sprintf("Error appending disputes: %v", err_dispute))
		} else {
			poststate.Block = disputeblock.Copy()
		}
		if err_dispute == nil {
			poststate.Block.Extrinsic.Disputes.FormatDispute()
			// bloock done here
		} else {
			return disputable, err_dispute, nil
		}
	}
	for packagehash, set := range dispute_set {
		fmt.Printf("● packagehash %s => %s\n", packagehash.String_short(), set)
	}
	// resign the dispute block
	// use prestate as dispute state
	dispute_block := poststate.Block.Copy()
	author_index := dispute_block.Header.AuthorIndex
	validator_secret := validator_secrerts[int(author_index)]
	dispute_prestate := prestate.Copy()
	dispute_prestate.Block = dispute_block.Copy()
	// also drop the assurance
	disputed_report_map := make(map[common.Hash]struct{})
	dispute_results := statedb.VerdictsToResults(dispute_prestate.Block.Extrinsic.Disputes.Verdict)
	for _, dispute_result := range dispute_results {
		if dispute_result.PositveCount <= (len(validator_secrerts) / 2) {
			disputed_report_map[dispute_result.WorkReportHash] = struct{}{}
		}
	}
	new_assurances := make([]types.Assurance, len(dispute_prestate.Block.Extrinsic.Assurances))
	//copy
	copy(new_assurances, dispute_prestate.Block.Extrinsic.Assurances)
	for i, rho := range dispute_prestate.JamState.AvailabilityAssignments {
		// see if the report hash in the disputed report map
		if rho != nil {
			if _, ok := disputed_report_map[rho.WorkReport.Hash()]; ok {
				// resign the assurance
				for index, assurance := range new_assurances {
					assurance.SetBitFieldBit(uint16(i), false)
					assurance_index := assurance.ValidatorIndex
					assurance.Sign(validator_secrerts[assurance_index].Ed25519Secret[:])
					new_assurances[index] = assurance
				}
			}
		}
	}

	dispute_prestate.Block.Extrinsic.Assurances = []types.Assurance{}
	err = dispute_prestate.ReSignDisputeBlock(validator_secret, new_assurances)
	if err != nil {
		return false, err, nil
	}
	dispute_block = dispute_prestate.Block.Copy()
	dispute_poststate, err := statedb.ApplyStateTransitionFromBlock(dispute_prestate, context.Background(), dispute_block, "MakeDisputes")
	if err != nil {
		fmt.Println(dispute_block.String())
		return false, fmt.Errorf("ApplyStateTransitionFromBlock Error:%v", err), nil
	}
	dispute_stf = node.BuildStateTransitionStruct(dispute_prestate, dispute_block, dispute_poststate)
	return disputable, nil, dispute_stf
}

// validators, secrets, err := node.GenerateValidatorNetwork()
func main() {
	_, secrets, err := node.GenerateValidatorNetwork()
	if err != nil {
		log.Fatalf("Error generating validator network: %v", err)
	}
	// Load the state transitions
	baseDir := "../importblocks/rawdata"
	// /home/ntust/jam/cmd/importblocks/rawdata/assurances_with_two_reports
	var mode string
	flag.StringVar(&mode, "mode", "assurances", "Mode to test")
	flag.Parse()
	stfs, err := ReadDisputableStateTransitions(baseDir, mode)
	if err != nil {
		log.Fatalf("Error reading state transitions: %v", err)
	}
	// Load the guarantees
	guarantees_history := make([]types.Guarantee, 0)
	for _, stf := range stfs {
		for _, guarantee := range stf.Block.Extrinsic.Guarantees {
			guarantees_history = append(guarantees_history, guarantee)
		}
	}
	// Make disputes
	store, err := storage.NewStateDBStorage("/tmp/disputes")
	if err != nil {
		log.Fatalf("Error creating storage: %v", err)
	}
	for _, stf := range stfs {
		fmt.Printf("======================================================================\n")
		fmt.Printf("Processing stf %s, slot%d...\n", stf.Block.Header.Hash().String_short(), stf.Block.TimeSlot())
		disputable, err, dispute_stf := MakeDisputes(store, stf, secrets, guarantees_history)
		if err != nil {
			fmt.Printf("\033[1;31mError making disputes: %v\033[0m\n", err)
		}
		if dispute_stf != nil && disputable {
			err = node.WriteSTFLog(dispute_stf, dispute_stf.Block.TimeSlot(), "../importblocks/rawdata/disputes")
			if err != nil {
				fmt.Printf("\033[1;31mError writing dispute state transition: %v\033[0m\n", err)
			} else {
				fmt.Printf("\033[1;32mDispute state transition written for block %s, slot %d\033[0m\n", dispute_stf.Block.Header.Hash().String_short(), dispute_stf.Block.TimeSlot())
			}
		} else {
			fmt.Printf("\033[1;33mNo dispute state transition written for block %s, slot %d\033[0m\n", stf.Block.Header.Hash().String_short(), stf.Block.TimeSlot())
		}
	}
}
