package gosafe

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Worker struct {
	ID        string
	Label     string
	StartTime time.Time
	Done      chan struct{}
}

type WorkerManager struct {
	mu      sync.Mutex
	workers map[string]*Worker // key: ID
}

func NewWorkerManager() *WorkerManager {
	return &WorkerManager{
		workers: make(map[string]*Worker),
	}
}

func (wm *WorkerManager) StartWorker(label string, task func()) string {
	id := uuid.New().String()

	worker := &Worker{
		ID:        id,
		Label:     label,
		StartTime: time.Now(),
		Done:      make(chan struct{}),
	}

	wm.mu.Lock()
	wm.workers[id] = worker
	wm.mu.Unlock()

	go func() {
		defer func() {
			close(worker.Done)
			wm.mu.Lock()
			delete(wm.workers, id)
			wm.mu.Unlock()
			// fmt.Printf("Worker %s (%s) finished\n", label, id)
		}()
		task()
	}()

	return id
}

func (wm *WorkerManager) ListWorkers() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	fmt.Println("Active Workers:")
	for id, worker := range wm.workers {
		duration := time.Since(worker.StartTime)
		fmt.Printf("- [%s] %s (running for %s)\n", id, worker.Label, duration.Round(time.Second))
	}
}

func (wm *WorkerManager) CountWorkers() int {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	return len(wm.workers)
}

func (wm *WorkerManager) StopWorker(id string) {
	wm.mu.Lock()
	worker, ok := wm.workers[id]
	wm.mu.Unlock()

	if ok {
		fmt.Println("Cannot force stop goroutine:", id, worker.Label)
	}
}

func (wm *WorkerManager) WaitWorker(id string) {
	wm.mu.Lock()
	worker, ok := wm.workers[id]
	wm.mu.Unlock()

	if ok {
		<-worker.Done
	}
}
