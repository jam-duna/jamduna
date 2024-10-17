package node

import (
	"fmt"
	"sort"
	"sync"
	"time"

	//"encoding/binary"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// NOTE: the GuaranteeReport is the incomplete version of the Guarantee
// The GuaranteeReport is the report that is signed by the validator

// PutGuaranteeBucket puts the guarantee report into the guarantee bucket
// The guarantee bucket is a map of work package hash to a list of guarantee reports
// will be used to form the extrinsic guarantee
func (n *Node) PutGuaranteeBucket(workR types.GuaranteeReport) error {
	workPackageHash := workR.Report.GetWorkPackageHash()
	if debug {
		fmt.Printf("%s PutGuaranteeBucket (workPackageHash:%v) WR From [V%d]\n", n.String(), workPackageHash, workR.GuaranteeCredential.ValidatorIndex)
	}
	n.guaranteeMutex.Lock()
	defer n.guaranteeMutex.Unlock()
	if n.guaranteeBucket == nil {
		// initialize the guarantee bucket
		n.guaranteeBucket = make(map[common.Hash][]types.GuaranteeReport)
	}
	// check if the guarantee is already in the bucket
	for _, work := range n.guaranteeBucket[workPackageHash] {
		if work.GuaranteeCredential.ValidatorIndex == workR.GuaranteeCredential.ValidatorIndex {
			return fmt.Errorf("Guarantee already in bucket")
		}
	}

	n.guaranteeBucket[workPackageHash] = append(n.guaranteeBucket[workPackageHash], workR)
	return nil
}

func (n *Node) PutGuaranteeBucketWithoutReport(workR types.GuaranteeReport, workPackageHash common.Hash, work_report_hash common.Hash) error {
	fmt.Printf("%s PutGuaranteeBucketWithoutReport (workPackageHash:%v) From [N%d]\n", n.String(), workPackageHash, workR.GuaranteeCredential.ValidatorIndex)
	n.guaranteeMutex.Lock()
	defer n.guaranteeMutex.Unlock()
	if n.guaranteeBucket == nil {
		// initialize the guarantee bucket
		n.guaranteeBucket = make(map[common.Hash][]types.GuaranteeReport)
	}
	// check if the guarantee is already in the bucket
	for _, work := range n.guaranteeBucket[workPackageHash] {
		if work.GuaranteeCredential.ValidatorIndex == workR.GuaranteeCredential.ValidatorIndex {
			return fmt.Errorf("Guarantee already in bucket")
		}
		if work.Report.Hash() == work_report_hash {
			workR.Report = work.Report
		}
	}

	n.guaranteeBucket[workPackageHash] = append(n.guaranteeBucket[workPackageHash], workR)
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
			// TODO: Michael + Shawn
			peerID := uint16(0)
			workpackagehashes := []common.Hash{}
			segmentRoots := []common.Hash{}
			bundle := []byte{}
			go n.peersInfo[peerID].ShareWorkPackage(core, workpackagehashes, segmentRoots, bundle)

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
		work, _, root, err := n.ProcessWorkPackage(workPackage)
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
func (n *Node) ProcessWorkPackage(workPackage types.WorkPackage) (work types.GuaranteeReport, spec *types.AvailabilitySpecifier, treeRoot common.Hash, err error) {
	workReport, spec, treeRoot, err := n.executeWorkPackage(workPackage)
	if err != nil {
		return types.GuaranteeReport{}, spec, common.Hash{}, err
	}

	work = types.GuaranteeReport{
		Report: workReport,
		GuaranteeCredential: types.GuaranteeCredential{
			ValidatorIndex: uint16(n.GetCurrValidatorIndex()),
		},
	}
	work.Sign(n.GetEd25519Secret())
	fmt.Printf("%s [ProcessWorkPackage:executeWorkPackage] Resulting Work Report Hash: %v\n", n.String(), workReport.Hash())
	return work, spec, treeRoot, nil
}

// helper function
// work report -> guarantee report

func (p *Peer) MakeGuaranteeReport(signature types.Ed25519Signature, validatoridx uint16) types.GuaranteeReport {
	var g = types.GuaranteeReport{
		Report: types.WorkReport{},
		GuaranteeCredential: types.GuaranteeCredential{
			ValidatorIndex: validatoridx,
			Signature:      signature,
		},
	}
	return g
}
