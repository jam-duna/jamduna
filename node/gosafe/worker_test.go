package gosafe

import (
	"fmt"
	"testing"
	"time"
)

func simpleTask() {
	fmt.Println("Simple task start")
	time.Sleep(2 * time.Second)
	fmt.Println("Simple task done")
}

func customTask(msg string, delay time.Duration) func() {
	return func() {
		fmt.Printf("Custom task [%s] start\n", msg)
		time.Sleep(delay)
		fmt.Printf("Custom task [%s] done\n", msg)
	}
}

func TestWorkerManager(t *testing.T) {
	manager := NewWorkerManager()

	id1 := manager.StartWorker("simple", simpleTask)
	id2 := manager.StartWorker("custom", customTask("hello", 3*time.Second))
	id3 := manager.StartWorker("lambda", func() {
		fmt.Println("Lambda worker running")
		time.Sleep(1 * time.Second)
		fmt.Println("Lambda worker finished")
	})

	time.Sleep(1 * time.Second)
	fmt.Println("=== After 1s ===")
	manager.ListWorkers()

	manager.WaitWorker(id1)
	manager.WaitWorker(id2)
	manager.WaitWorker(id3)

	fmt.Println("=== Final state ===")
	manager.ListWorkers()
}
