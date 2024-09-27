package node

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// NOTE: the GuaranteeReport is the incomplete version of the Guarantee
// The GuaranteeReport is the report that is signed by the validator

// PutGuaranteeBucket puts the guarantee report into the guarantee bucket
// The guarantee bucket is a map of work package hash to a list of guarantee reports
// will be used to form the extrinsic guarantee
func (n *Node) PutGuaranteeBucket(workR types.GuaranteeReport) error {
	fmt.Printf("[V%d]Put Guarantee Bucket (W_Hash:%v) From [V%d]\n", n.GetCurrValidatorIndex(), workR.Report.GetWorkPackageHash(), workR.GuaranteeCredential.ValidatorIndex)
	n.guaranteeMutex.Lock()
	defer n.guaranteeMutex.Unlock()
	if n.guaranteeBucket == nil {
		// initialize the guarantee bucket
		n.guaranteeBucket = make(map[common.Hash][]types.GuaranteeReport)
	}
	// check if the guarantee is already in the bucket
	for _, work := range n.guaranteeBucket[workR.Report.GetWorkPackageHash()] {
		if work.GuaranteeCredential.ValidatorIndex == workR.GuaranteeCredential.ValidatorIndex {
			return fmt.Errorf("Guarantee already in bucket")
		}
	}

	n.guaranteeBucket[workR.Report.GetWorkPackageHash()] = append(n.guaranteeBucket[workR.Report.GetWorkPackageHash()], workR)
	return nil
}

// verify the guarantee report and put it into the guarantee bucket
func (n *Node) processGuaranteeReport(report types.GuaranteeReport) error {
	// Check if the guarantee is valid
	core, err := n.GetSelfCoreIndex()
	if err != nil {
		return err
	}
	if report.Report.CoreIndex != core {
		return fmt.Errorf("Invalid core index in guarantee report")
	}
	coWorkers := n.GetCoreCoWorkers(core)

	isInCore := false
	// Check if the guarantee is signed by the validator
	if uint16(n.GetCurrValidatorIndex()) == report.GuaranteeCredential.ValidatorIndex {
		isInCore = true
	}
	for _, coWorker := range coWorkers {
		key := coWorker.GetEd25519Key()
		verify := report.Verify(key)
		if verify {
			isInCore = true
			break
		}
	}

	if !isInCore {
		return fmt.Errorf("Guarantee report not signed by this core's validator")
	}

	err = n.PutGuaranteeBucket(report)
	if err != nil {
		return err
	}

	return nil
}

// when n receives a work package, it will send the work package to the co-workers
// and then process the work package
// then send the guarantee report to the co-workers
// then see if other co-workers have the same report
// try to make the extrinsic guarantee

func (n *Node) GenerateGuarantee(workPackage types.WorkPackage) (types.Guarantee, common.Hash, types.AvailabilitySpecifier, error) {
	core, err := n.GetSelfCoreIndex()
	if err != nil {
		return types.Guarantee{}, common.Hash{}, types.AvailabilitySpecifier{}, err
	}
	coWorkers := n.GetCoreCoWorkers(core)
	if len(coWorkers) == 0 {
		return types.Guarantee{}, common.Hash{}, types.AvailabilitySpecifier{}, fmt.Errorf("No co-workers from core %d", core)
	}

	// Send the work package to the co-worker
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = n.coreBroadcast(workPackage)
	}()
	wg.Wait()

	workChan := make(chan types.GuaranteeReport)
	rootChan := make(chan common.Hash)

	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		work, _, root, err := n.processWorkPackage(workPackage)
		if err != nil {
			fmt.Println("Error processing work package:", err)
			return
		}
		workChan <- work
		rootChan <- root
	}()

	// Move the Wait to after receiving from channels
	work := <-workChan
	root := <-rootChan
	wg.Wait() // This ensures that the first goroutine has completed
	_ = n.coreBroadcast(work)
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := n.processGuaranteeReport(work)
		if err != nil {
			fmt.Println("Error processing guarantee report:", err)
		}
	}()
	wg.Wait() // This ensures that the second goroutine has completed
	if err != nil {
		return types.Guarantee{}, common.Hash{}, types.AvailabilitySpecifier{}, err
	}
	//delay a while to wait for the guarantee report to be processed
	time.Sleep(3 * time.Second)
	n.guaranteeMutex.Lock()
	if len(n.guaranteeBucket[workPackage.Hash()]) >= 2 {
		//try to make the extrinsic guarantee
		guantorcredentials := make([]types.GuaranteeCredential, 0)
		// make a map workreporthash to count the number of guarantees
		workReportCounter := make(map[common.Hash]int)
		// count if the work report are the same
		// if the same reports are more than 2, then we can make an extrinsic guarantee
		for _, eg := range n.guaranteeBucket[workPackage.Hash()] {
			if _, exists := workReportCounter[eg.Report.Hash()]; !exists {
				workReportCounter[eg.Report.Hash()] = 1
				fmt.Println("Core Index is:", eg.Report.CoreIndex)
			} else {
				workReportCounter[eg.Report.Hash()]++
			}
		}
		for hash, cnt := range workReportCounter {
			fmt.Printf("WorkReportHash: %v, Count: %v\n", hash, cnt)
		}
		var goodWorkReports types.WorkReport
		for _, eg := range n.guaranteeBucket[workPackage.Hash()] {
			if workReportCounter[eg.Report.Hash()] >= 2 {
				guantorcredentials = append(guantorcredentials, eg.GuaranteeCredential)
				goodWorkReports = eg.Report
			}
		}
		// Sort the guarantor credentials by validator index
		sort.Slice(guantorcredentials, func(i, j int) bool {
			return guantorcredentials[i].ValidatorIndex < guantorcredentials[j].ValidatorIndex
		})
		if len(guantorcredentials) >= 2 {
			extrinsicGuarantee := types.Guarantee{
				Report:     goodWorkReports,
				Slot:       n.statedb.GetTimeslot(),
				Signatures: guantorcredentials,
			}
			n.guaranteeMutex.Unlock()
			return extrinsicGuarantee, root, goodWorkReports.AvailabilitySpec, nil
		}
	}
	n.guaranteeMutex.Unlock()
	return types.Guarantee{}, common.Hash{}, types.AvailabilitySpecifier{}, fmt.Errorf("Not enough guarantees to make an extrinsic guarantee")
}

// co-worker also have rights to form the extrinsic guarantee
// if the work package sender somehow didn't form the extrinsic guarantee
// the co-worker can form the extrinsic guarantee
func (n *Node) FormGuarantee(PackageHash common.Hash) (types.Guarantee, error) {
	n.guaranteeMutex.Lock()
	if len(n.guaranteeBucket[PackageHash]) >= 2 {
		//try to make the extrinsic guarantee
		guantorcredentials := make([]types.GuaranteeCredential, 0)
		// make a map workreporthash to count the number of guarantees
		workReportCounter := make(map[common.Hash]int)
		// count if the work report are the same
		// if the same reports are more than 2, then we can make an extrinsic guarantee
		for _, eg := range n.guaranteeBucket[PackageHash] {
			if _, exists := workReportCounter[eg.Report.Hash()]; !exists {
				workReportCounter[eg.Report.Hash()] = 1
			} else {
				workReportCounter[eg.Report.Hash()]++
			}
		}
		var goodWorkReports types.WorkReport
		for _, eg := range n.guaranteeBucket[PackageHash] {
			if workReportCounter[eg.Report.Hash()] >= 2 {
				guantorcredentials = append(guantorcredentials, eg.GuaranteeCredential)
				goodWorkReports = eg.Report
			}
		}
		// Sort the guarantor credentials by validator index
		sort.Slice(guantorcredentials, func(i, j int) bool {
			return guantorcredentials[i].ValidatorIndex < guantorcredentials[j].ValidatorIndex
		})
		if len(guantorcredentials) >= 2 {
			extrinsicGuarantee := types.Guarantee{
				Report:     goodWorkReports,
				Slot:       n.statedb.GetTimeslot(),
				Signatures: guantorcredentials,
			}
			n.guaranteeMutex.Unlock()
			return extrinsicGuarantee, nil
		}
	}
	n.guaranteeMutex.Unlock()
	var a string
	for _, eg := range n.guaranteeBucket[PackageHash] {
		a += fmt.Sprintf("We got [V%d], ", eg.GuaranteeCredential.ValidatorIndex)
	}
	return types.Guarantee{}, fmt.Errorf("Not enough guarantees to make an extrinsic guarantee: %s", a)
}

// work package -> work report -> guarantee report
func (n *Node) processWorkPackage(workPackage types.WorkPackage) (work types.GuaranteeReport, spec *types.AvailabilitySpecifier, treeRoot common.Hash, err error) {

	// Create a new PVM instance with mock code and execute it
	results := []types.WorkResult{}
	targetStateDB := n.getPVMStateDB()
	service_index := uint32(workPackage.AuthCodeHost)
	packageHash := workPackage.Hash()
	fmt.Printf("[V%d]Processing Work Package: %v, Key: %v\n", n.GetCurrValidatorIndex(), packageHash, n.GetEd25519Key())
	// set up audit friendly work WorkPackage
	asworkPackage := types.WorkPackage{
		WorkItems: make([]types.WorkItem, 0),
	}
	segments := make([][]byte, 0)
	for _, workItem := range workPackage.WorkItems {
		// recover code from the bpt. NOT from DA
		code := targetStateDB.ReadServicePreimageBlob(service_index, workItem.CodeHash)
		if len(code) == 0 {
			err = fmt.Errorf("code not found in bpt. C(%v, %v)", service_index, workItem.CodeHash)
			fmt.Println(err)
			return types.GuaranteeReport{}, spec, common.Hash{}, err
		}
		if common.Blake2Hash(code) != workItem.CodeHash {
			fmt.Printf("Code and CodeHash Mismatch\n")
			panic(0)
		}

		vm := pvm.NewVMFromCode(service_index, code, 0, targetStateDB)
		imports, err := n.getImportSegments(workItem.ImportedSegments)
		fmt.Printf("[V%d]Imported Segments: %v\n", n.GetCurrValidatorIndex(), imports)
		if err != nil {
			// return spec, common.Hash{}, err
			imports = make([][]byte, 0)
		}
		vm.SetImports(imports)
		vm.SetExtrinsicsPayload(workItem.ExtrinsicsBlobs, workItem.Payload)
		err = vm.Execute(types.EntryPointRefine)
		if err != nil {
			return types.GuaranteeReport{}, spec, common.Hash{}, err
		}
		output, _ := vm.GetArgumentOutputs()

		// The workitem is an ordered collection of segments
		asWorkItem := types.ASWorkItem{
			Segments:   make([]types.Segment, 0),
			Extrinsics: make([]types.WorkItemExtrinsic, 0),
		}
		for _, i := range vm.Imports {
			asWorkItem.Segments = append(asWorkItem.Segments, types.Segment{Data: i})
		}
		for _, extrinsicblob := range workItem.ExtrinsicsBlobs {
			asWorkItem.Extrinsics = append(asWorkItem.Extrinsics, types.WorkItemExtrinsic{Hash: common.BytesToHash(extrinsicblob), Len: uint32(len(extrinsicblob))})
		}

		// 1. NOTE: We do NOT need to erasure code import data
		// 2. TODO: We DO need to erasure encode extrinsics into "Audit DA"
		// ******TODO******

		// 3. We DO need to erasure code exports from refine execution into "Import DA"
		fmt.Printf("VM Exports: %v\n", vm.Exports)
		for _, e := range vm.Exports {
			s := e
			segments = append(segments, s) // this is used in NewAvailabilitySpecifier
		}
		pageProofs, _ := trie.GeneratePageProof(segments)
		combinedSegmentAndPageProofs := append(segments, pageProofs...)

		var wg sync.WaitGroup
		wg.Add(1)

		// EncodeAndDistributeSegmentData
		go func() {
			treeRoot, err = n.EncodeAndDistributeSegmentData(combinedSegmentAndPageProofs, &wg)
			if err != nil {
				fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
			}
		}()

		// Wait for the task to complete
		wg.Wait()
		if err != nil {
			return types.GuaranteeReport{}, spec, common.Hash{}, err
		}
		fmt.Printf("Combined Segment and Page Proofs: %v\n", combinedSegmentAndPageProofs)
		// setup work results
		// 11.1.4. Work Result. Equation 121. We finally come to define a work result, L, which is the data conduit by which services’ states may be altered through the computation done within a work-package.
		result := types.WorkResult{
			Service:     workItem.Service,
			CodeHash:    workItem.CodeHash,
			PayloadHash: common.Blake2Hash(workItem.Payload),
			GasRatio:    0,
			Result:      output,
		}
		results = append(results, result)
	}

	// Step 2:  Now create a WorkReport with AvailabilitySpecification and RefinementContext
	spec = n.NewAvailabilitySpecifier(packageHash, asworkPackage, segments)
	prerequisite_hash := common.HexToHash("0x")
	refinementContext := types.RefineContext{
		Anchor:           n.statedb.ParentHash,                      // TODO
		StateRoot:        n.statedb.Block.Header.ParentStateRoot,    // TODO, common.HexToHash("0x")
		BeefyRoot:        common.HexToHash("0x"),                    // SKIP
		LookupAnchor:     n.statedb.ParentHash,                      // TODO
		LookupAnchorSlot: n.statedb.Block.Header.Slot,               //TODO: uint32(0)
		Prerequisite:     (*types.Prerequisite)(&prerequisite_hash), //common.HexToHash("0x"), // SKIP
	}
	core, err := n.GetSelfCoreIndex()
	if err != nil {
		return types.GuaranteeReport{}, spec, common.Hash{}, err
	}
	workReport := types.WorkReport{
		AvailabilitySpec: *spec,
		AuthorizerHash:   common.HexToHash("0x"), // SKIP
		CoreIndex:        core,
		//	Output:               result.Output,
		RefineContext: refinementContext,
		Results:       results,
	}

	workReport.Print()
	work = n.MakeGuaranteeReport(workReport)
	work.Sign(n.GetEd25519Secret())
	return work, spec, treeRoot, nil
}

// helper function
// work report -> guarantee report
func (n *Node) MakeGuaranteeReport(report types.WorkReport) types.GuaranteeReport {
	var g = types.GuaranteeReport{
		Report: report,
		GuaranteeCredential: types.GuaranteeCredential{
			ValidatorIndex: uint16(n.GetCurrValidatorIndex()),
		},
	}
	return g
}
