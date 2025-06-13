package node

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"github.com/gorilla/websocket"
)

type WorkPackageRequest struct {
	WorkPackage     types.WorkPackage     `json:"work_package"`
	ExtrinsicsBlobs types.ExtrinsicsBlobs `json:"extrinsic_blobs"`
	Prerequisites   []string              `json:"prerequisites,omitempty"`
	Identifier      string                `json:"_"`
}

func (wpReq *WorkPackageRequest) String() string {
	return types.ToJSON(wpReq)
}

func UpdateJCESignalUniversal(nodes []*Node, initialValue uint32) *ManualJCEManager {
	m := NewManualJCEManager(nodes, initialValue)
	m.Start(context.Background())
	return m
}

const (
	UpperJCEBuffer = 3
)

type ManualJCEManager struct {
	Nodes           []*Node
	PollJCE         uint32
	UniversalJCE    uint32
	JCEBuffer       uint32
	wpSignalChan    chan common.Hash
	PendingIndices  []int
	CompleteIndices []int

	refineUpdatesChan       chan []byte
	accumulationUpdatesChan chan []byte

	stateMu                  sync.RWMutex
	WPQueue                  []common.Hash
	RefinedWP                []common.Hash
	AccumulatedWP            []common.Hash
	currentRefineState       statedb.RecentBlocks
	currentAccumulationState [types.EpochLength]types.AccumulationHistory
}

func NewManualJCEManager(nodes []*Node, initialValue uint32) *ManualJCEManager {
	pending := make([]int, len(nodes))
	for i := range nodes {
		pending[i] = i
	}

	return &ManualJCEManager{
		Nodes:                    nodes,
		UniversalJCE:             initialValue,
		PollJCE:                  0,
		JCEBuffer:                UpperJCEBuffer,
		wpSignalChan:             make(chan common.Hash, 10),
		WPQueue:                  []common.Hash{},
		RefinedWP:                []common.Hash{},
		AccumulatedWP:            []common.Hash{},
		PendingIndices:           pending,
		CompleteIndices:          []int{},
		refineUpdatesChan:        make(chan []byte, 1),
		accumulationUpdatesChan:  make(chan []byte, 1),
		currentRefineState:       nil,
		currentAccumulationState: [types.EpochLength]types.AccumulationHistory{},
	}
}

func (m *ManualJCEManager) Replenish() {
	pollTicker := time.NewTicker(100 * time.Millisecond)
	defer pollTicker.Stop()

	for range pollTicker.C {
		//fmt.Printf("Replenish: Replenishing JCEBuffer to %d.\n", UpperJCEBuffer)
		m.JCEBuffer = UpperJCEBuffer
	}
}

func (m *ManualJCEManager) SendWP(wp common.Hash) {
	select {
	case m.wpSignalChan <- wp:
		m.JCEBuffer = UpperJCEBuffer
		//fmt.Printf("SendWP: Signal received for WP %s. JCEBuffer reset.\n", wp.Hex())
	default:
		fmt.Printf("Warning: WP signal channel is full, dropping WP %s\n", wp.Hex())
	}
}

func (m *ManualJCEManager) UpdateRefineState(c3Bytes []byte) {
	select {
	case m.refineUpdatesChan <- c3Bytes:

	default:
		fmt.Printf("Warning: Beta Requirement updates channel full. Dropping refine update\n")
	}
}

func (m *ManualJCEManager) UpdateAccumulationState(c15Bytes []byte) {
	select {
	case m.accumulationUpdatesChan <- c15Bytes:

	default:
		fmt.Printf("Warning: Rho Accumulation updates channel full. Dropping accu update.\n")
	}
}

func (m *ManualJCEManager) Poll() {
	newPending := m.PendingIndices[:0]
	m.CompleteIndices = m.CompleteIndices[:0]
	lastCompletedJCE := m.PollJCE

	allCompleteThisCycle := true
	for _, idx := range m.PendingIndices {
		completedJCE := m.Nodes[idx].GetCompletedJCE()
		if completedJCE < m.UniversalJCE {
			newPending = append(newPending, idx)
			allCompleteThisCycle = false
		} else {
			m.CompleteIndices = append(m.CompleteIndices, idx)
			lastCompletedJCE = completedJCE
		}
	}
	m.PendingIndices = newPending

	if allCompleteThisCycle && len(m.Nodes) > 0 {
		m.PollJCE = lastCompletedJCE
	}
}

func (m *ManualJCEManager) Advance() {
	m.UniversalJCE++

	m.stateMu.Lock()
	wpQueueLenBeforeClear := len(m.WPQueue)
	wpQueueWasNonEmpty := wpQueueLenBeforeClear > 0

	if wpQueueWasNonEmpty {
		//fmt.Printf("Clearing WPQueue and resetting JCEBuffer to %d.\n", UpperJCEBuffer)
		m.JCEBuffer = UpperJCEBuffer
		m.WPQueue = m.WPQueue[:0]
	}

	m.stateMu.Unlock()

	//fmt.Printf("Advancing to universal JCE: %d (WPQueue had %d items before potential clear)\n", m.UniversalJCE, wpQueueLenBeforeClear)

	for _, node := range m.Nodes {
		node.SendNewJCE(m.UniversalJCE)
	}

	m.PendingIndices = make([]int, len(m.Nodes))
	for i := range m.Nodes {
		m.PendingIndices[i] = i
	}
	m.CompleteIndices = m.CompleteIndices[:0]

}

func removeWPsFromSlice(slicePtr *[]common.Hash, wpsToRemove map[common.Hash]struct{}) {
	if len(wpsToRemove) == 0 {
		return
	}
	sourceSlice := *slicePtr
	newSlice := sourceSlice[:0]
	for _, wp := range sourceSlice {
		if _, found := wpsToRemove[wp]; !found {
			newSlice = append(newSlice, wp)
		}
	}
	*slicePtr = newSlice
}

func (m *ManualJCEManager) CheckReq() bool {

	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	wpQueueLen := len(m.WPQueue)

	if wpQueueLen == 0 {
		if m.JCEBuffer > 0 {
			m.JCEBuffer--
			//fmt.Printf("CheckReq: WPQueue empty. Using buffer. JCEBuffer=%d. Allow advance.\n", m.JCEBuffer)
			return true
		} else {
			//fmt.Printf("CheckReq: WPQueue empty. Buffer empty. JCEBuffer=%d. PAUSE advance.\n", m.JCEBuffer)
			return false
		}
	}

	if len(m.currentRefineState) == 0 {

		//fmt.Println("CheckReq Error: WPQueue non-empty, but currentRefineState is nil/empty. Cannot check requirements.")

		if m.JCEBuffer > 0 {
			m.JCEBuffer--
			//fmt.Printf("CheckReq: Using buffer due to missing Beta state. JCEBuffer=%d. Allow advance.\n", m.JCEBuffer)
			return true
		} else {
			//fmt.Printf("CheckReq: Buffer empty and missing Beta state. JCEBuffer=%d. PAUSE advance.\n", m.JCEBuffer)
			return false
		}
	}

	refine_pending := make(map[common.Hash]struct{}, wpQueueLen)
	for _, wp := range m.WPQueue {
		refine_pending[wp] = struct{}{}
	}

	accumalated_done := make(map[common.Hash]struct{}, 0)
	for i := len(m.currentAccumulationState) - 1; i >= 0; i-- {
		accumulationHistory := m.currentAccumulationState[i]
		for _, accumulatedWPHash := range accumulationHistory.WorkPackageHash {
			accumalated_done[accumulatedWPHash] = struct{}{}
		}
	}

	if len(refine_pending) == 0 {
		fmt.Println("CheckReq Warning: WPQueue non-empty, but refine_pending map is empty? Allowing advance.")
		return true
	}
	initialRequiredCount := len(refine_pending)
	//fmt.Printf("CheckReq: Need to find %d unique WP(s) from WPQueue: %v\n", initialRequiredCount, m.WPQueue)

	refine_found_in_beta := make(map[common.Hash]struct{})
	for i := len(m.currentRefineState) - 1; i >= 0; i-- {
		beta := m.currentRefineState[i]
		for coreIdx, segmentRootInfo := range beta.Reported {
			wpHash := segmentRootInfo.WorkPackageHash
			if _, needed := refine_pending[wpHash]; needed {
				if _, alreadyFound := refine_found_in_beta[wpHash]; !alreadyFound {
					log.Trace(log.Node, "CheckReq: Found required WP in Beta state", "wp", wpHash.Hex(), "i", i, "coreIdx", coreIdx)
					refine_found_in_beta[wpHash] = struct{}{}
				}
			}
		}
	}

	if len(refine_found_in_beta) > 0 {
		refinedCompleteWP := make([]common.Hash, 0, len(refine_found_in_beta))
		for wp := range refine_found_in_beta {
			refinedCompleteWP = append(refinedCompleteWP, wp)
		}
		//fmt.Printf("CheckReq: Moving %d found WPs from WPQueue to RefinedWP: %v\n", len(refine_found_in_beta), refinedCompleteWP)

		m.RefinedWP = append(m.RefinedWP, refinedCompleteWP...)

		removeWPsFromSlice(&m.WPQueue, refine_found_in_beta)
		//fmt.Printf("CheckReq: WPQueue size after moving found WPs: %d\n", len(m.WPQueue))
	}

	accmulated_found_in_c15 := make(map[common.Hash]struct{})

	for _, wp := range m.WPQueue {
		if _, found := accumalated_done[wp]; found {
			if _, alreadyMarked := accmulated_found_in_c15[wp]; !alreadyMarked {
				//fmt.Printf("CheckReq: Found WP %s in Accumulation state. Moving WPQueue => AccumulatedWP.\n", wp.Hex())
				accmulated_found_in_c15[wp] = struct{}{}
			}
		}
	}

	for _, wp := range m.RefinedWP {
		if _, found := accumalated_done[wp]; found {
			if _, alreadyMarked := accmulated_found_in_c15[wp]; !alreadyMarked {
				//fmt.Printf("CheckReq: Found WP %s in Accumulation state. Moving RefinedWP => AccumulatedWP.\n", wp.Hex())
				accmulated_found_in_c15[wp] = struct{}{}
			}
		}
	}

	if len(accmulated_found_in_c15) > 0 {
		accumCompleteWP := make([]common.Hash, 0, len(accmulated_found_in_c15))
		for wp := range accmulated_found_in_c15 {
			accumCompleteWP = append(accumCompleteWP, wp)
		}
		//fmt.Printf("CheckReq: Moving %d found WPs to AccumulatedWP: %v\n", len(accmulated_found_in_c15), accumCompleteWP)

		m.AccumulatedWP = append(m.AccumulatedWP, accumCompleteWP...)

		removeWPsFromSlice(&m.WPQueue, accmulated_found_in_c15)
		removeWPsFromSlice(&m.RefinedWP, accmulated_found_in_c15)
	}

	//fmt.Printf("After CheckReq: WPQueue sz=%d %v | RefinedWP sz=%d %v | AccumWP sz=%d\n", len(m.WPQueue), m.WPQueue, len(m.RefinedWP), m.RefinedWP, len(m.AccumulatedWP))

	allFound := len(m.WPQueue) == 0

	if allFound {
		//fmt.Printf("CheckReq: All %d required unique WP(s) processed (moved to Refined or Accumulated). Requirement met. Allow advance.\n", initialRequiredCount)

		return true
	} else {

		numFound := initialRequiredCount - len(m.WPQueue)
		log.Trace(log.Node, "CheckReq", "numFound", numFound, "irc", initialRequiredCount, "lenQ", len(m.WPQueue))

		if m.JCEBuffer > 0 {
			m.JCEBuffer--
			//fmt.Printf("CheckReq: Using buffer because not all WPs processed. JCEBuffer=%d. Allow advance.\n", m.JCEBuffer)
			return true
		} else {
			//fmt.Printf("CheckReq: Buffer empty and not all WPs processed. JCEBuffer=%d. PAUSE advance.\n", m.JCEBuffer)
			return false
		}
	}

}

func (m *ManualJCEManager) Start(ctx context.Context) {
	fmt.Println("ManualJCEManager starting...")
	pollTicker := time.NewTicker(300 * time.Millisecond)
	reqTicker := time.NewTicker(500 * time.Millisecond)
	bufferTicker := time.NewTicker(5000 * time.Millisecond)
	accumCheckTicker := time.NewTicker(2000 * time.Millisecond)

	defer pollTicker.Stop()
	defer reqTicker.Stop()
	defer bufferTicker.Stop()
	defer accumCheckTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("ManualJCEManager stopping...")
			return

		case <-pollTicker.C:
			m.Poll()
			if len(m.PendingIndices) == 0 {

				if m.CheckReq() {
					m.Advance()
				}
			}

		case <-accumCheckTicker.C:

		case <-bufferTicker.C:

			if m.JCEBuffer == 0 {
				//fmt.Printf("Refreshing JCEBuffer to %d.\n", UpperJCEBuffer)
				m.JCEBuffer = UpperJCEBuffer
			}

		case <-reqTicker.C:
		drainLoop:
			for {
				select {
				case wp := <-m.wpSignalChan:

					m.stateMu.Lock()
					m.WPQueue = append(m.WPQueue, wp)
					queueLen := len(m.WPQueue)
					m.stateMu.Unlock()

					log.Trace(log.Node, "Received WP signal", "wp", wp.Hex(), "ql", queueLen)
				default:
					break drainLoop
				}
			}

		case c3Bytes := <-m.refineUpdatesChan:

			c3, _, err := types.Decode(c3Bytes, reflect.TypeOf(statedb.RecentBlocks{}))
			if err != nil {
				//fmt.Printf("Error decoding Beta state update: %v\n", err)
				continue
			}
			recentBlocks, ok := c3.(statedb.RecentBlocks)
			if !ok {
				//fmt.Printf("Error type asserting Beta state update\n")
				continue
			}

			m.stateMu.Lock()
			m.currentRefineState = recentBlocks
			m.stateMu.Unlock()

		case c15Bytes := <-m.accumulationUpdatesChan:

			c15, _, err := types.Decode(c15Bytes, reflect.TypeOf([types.EpochLength]types.AccumulationHistory{}))
			if err != nil {
				//fmt.Printf("Error decoding Accumulation state update: %v\n", err)
				continue
			}
			accumulationHistories, ok := c15.([types.EpochLength]types.AccumulationHistory)
			if !ok {
				//fmt.Printf("Error type asserting Accumulation state update\n")
				continue
			}

			m.stateMu.Lock()
			m.currentAccumulationState = accumulationHistories
			m.stateMu.Unlock()

		}
	}
}

func UpdateJCESignalSimple(nodes []*Node, initialValue uint32) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	universalJCE := initialValue
	updateInterval := 6 * time.Second
	lastUpdate := time.Now()
	for {
		<-ticker.C
		if time.Since(lastUpdate) >= updateInterval {
			universalJCE++
			for _, node := range nodes {
				node.SendNewJCE(universalJCE)
			}
			lastUpdate = time.Now()
			//fmt.Printf("Simple JCE updated to: %d\n", universalJCE)
		}
	}
}

func UpdateJCESignalUniversal2(nodes []*Node, initialValue uint32, addtionalReq chan string) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	universalJCE := initialValue
	pendingNodes := make([]*Node, len(nodes))
	copy(pendingNodes, nodes)
	for {
		<-ticker.C
		newPending := pendingNodes[:0]
		nodesReady := 0
		for _, node := range pendingNodes {
			nodeJCE := node.GetCompletedJCE()
			if nodeJCE < universalJCE {

				newPending = append(newPending, node)
			} else {

				nodesReady++
			}
		}
		pendingNodes = newPending
		numPending := len(pendingNodes)
		if numPending > 0 {
			//fmt.Printf("Waiting for %d node(s) (%d ready) to complete JCE %d\n", numPending, nodesReady, universalJCE)
		} else {
			universalJCE++
			//fmt.Printf("Deploying new universal JCE: %d\n", universalJCE)
			for _, node := range nodes {
				node.SendNewJCE(universalJCE)
			}

			pendingNodes = make([]*Node, len(nodes))
			copy(pendingNodes, nodes)
		}
	}
}

func StartGameOfLifeServer(addr string, path string) func(data []byte) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	var wsConn *websocket.Conn
	var connMu sync.Mutex

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, path)
		})
		mux.HandleFunc("/wsgof_rpc", func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				fmt.Println("upgrade error:", err)
				return
			}
			fmt.Println("Client connected")

			connMu.Lock()

			if wsConn != nil {
				fmt.Println("Closing previous WebSocket connection.")
				wsConn.Close()
			}
			wsConn = conn
			connMu.Unlock()

			conn.SetCloseHandler(func(code int, text string) error {
				fmt.Printf("WebSocket closed: %d %s\n", code, text)
				connMu.Lock()

				if wsConn == conn {
					wsConn = nil
				}
				connMu.Unlock()
				return nil
			})

		})

		fmt.Printf("Starting Game of Life server on %s (serving %s)\n", addr, path)
		err := http.ListenAndServe(addr, mux)
		if err != nil && err != http.ErrServerClosed {
			fmt.Printf("ListenAndServe error: %v\n", err)

		}
		fmt.Println("Game of Life server shut down.")
	}()

	return func(data []byte) {
		connMu.Lock()
		conn := wsConn
		connMu.Unlock()

		if conn != nil {

			err := conn.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				fmt.Printf("Error writing to WebSocket: %v\n", err)

				connMu.Lock()
				if wsConn == conn {
					wsConn.Close()
					wsConn = nil
				}
				connMu.Unlock()
			}
		} else {

		}
	}
}

func SetupJceManager(nodes []*Node, initialJCE uint32, jceMode string) (string, *ManualJCEManager, context.CancelFunc, error) {
	var manager *ManualJCEManager
	var cancelFunc context.CancelFunc
	var setupErr error

	if len(nodes) == 0 {
		return jceMode, nil, nil,
			errors.New("SetupJceManager: cannot setup JCE Manager with zero nodes")
	}

	//fmt.Printf("SetupJceManager: Detected JCE Mode '%v'\n", jceMode)

	switch jceMode {
	case JCEManual:
		fmt.Println("SetupJceManager: Setting up Manual JCE Manager...")
		// Create a cancellable context and return its cancelFunc
		ctx, cf := context.WithCancel(context.Background())
		cancelFunc = cf

		managerChan := make(chan *ManualJCEManager, 1)
		go func(innerCtx context.Context) {
			defer close(managerChan)

			fmt.Println("Goroutine: Creating ManualJCEManager...")
			m := NewManualJCEManager(nodes, initialJCE)

			fmt.Println("Goroutine: Sending manager instance back...")
			select {
			case managerChan <- m:
				fmt.Println("Goroutine: Manager instance sent.")
			case <-innerCtx.Done():
				fmt.Println("Goroutine: Context cancelled before sending instance.")
				return
			}

			fmt.Println("Goroutine: Starting ManualJCEManager loop...")
			m.Start(innerCtx)
			fmt.Println("Goroutine: ManualJCEManager loop finished.")
		}(ctx)

		fmt.Println("SetupJceManager: Waiting for Manual JCE Manager instance...")
		select {
		case receivedManager, ok := <-managerChan:
			if !ok || receivedManager == nil {
				fmt.Println("SetupJceManager: Failed to receive manager instance.")
				setupErr = errors.New("SetupJceManager: failed to setup ManualJCEManager")
				// Cancel the context since setup failed
				cancelFunc()
			} else {
				manager = receivedManager
				//fmt.Printf("SetupJceManager: Manual JCE Manager (%p) setup complete.\n", manager)
			}
		case <-time.After(10 * time.Second):
			fmt.Println("SetupJceManager: Timeout waiting for manager instance.")
			setupErr = errors.New("SetupJceManager: timeout waiting for ManualJCEManager")
			cancelFunc()
		}

	case JCESimple:
		fmt.Println("SetupJceManager: Setting up Simple JCE (background task)...")
		go UpdateJCESignalSimple(nodes, initialJCE)

	default:
		//fmt.Printf("SetupJceManager: Unknown JCE Mode '%s'. Using default delay...\n", jceMode)
		time.Sleep(types.SecondsPerSlot * time.Second)
	}

	if setupErr != nil {
		return jceMode, nil, nil, setupErr
	}
	return jceMode, manager, cancelFunc, nil
}
