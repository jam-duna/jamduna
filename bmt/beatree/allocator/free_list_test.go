package allocator

import (
	"testing"
)

func TestFreeListAlloc(t *testing.T) {
	fl := NewFreeList()

	if !fl.IsEmpty() {
		t.Error("new free list should be empty")
	}

	if fl.Len() != 0 {
		t.Errorf("length = %d, want 0", fl.Len())
	}
}

func TestFreeListPushPop(t *testing.T) {
	fl := NewFreeList()

	// Push some pages
	fl.Push(PageNumber(1))
	fl.Push(PageNumber(2))
	fl.Push(PageNumber(3))

	if fl.Len() != 3 {
		t.Errorf("length = %d, want 3", fl.Len())
	}

	// Pop in LIFO order
	if page := fl.Pop(); page != PageNumber(3) {
		t.Errorf("pop = %v, want 3", page)
	}

	if page := fl.Pop(); page != PageNumber(2) {
		t.Errorf("pop = %v, want 2", page)
	}

	if page := fl.Pop(); page != PageNumber(1) {
		t.Errorf("pop = %v, want 1", page)
	}

	if !fl.IsEmpty() {
		t.Error("free list should be empty after popping all pages")
	}
}

func TestFreeListPopEmpty(t *testing.T) {
	fl := NewFreeList()

	page := fl.Pop()
	if page != InvalidPageNumber {
		t.Errorf("pop from empty list = %v, want InvalidPageNumber", page)
	}
}

func TestFreeListNoLeaks(t *testing.T) {
	fl := NewFreeList()

	// Allocate and free 1000 pages
	for i := 0; i < 1000; i++ {
		fl.Push(PageNumber(i))
	}

	if fl.Len() != 1000 {
		t.Errorf("length = %d, want 1000", fl.Len())
	}

	// Pop all pages
	for i := 0; i < 1000; i++ {
		page := fl.Pop()
		if !page.IsValid() {
			t.Errorf("pop %d returned invalid page", i)
		}
	}

	if !fl.IsEmpty() {
		t.Error("free list should be empty after popping all pages")
	}
}

func TestFreeListClear(t *testing.T) {
	fl := NewFreeList()

	fl.Push(PageNumber(1))
	fl.Push(PageNumber(2))
	fl.Push(PageNumber(3))

	fl.Clear()

	if !fl.IsEmpty() {
		t.Error("free list should be empty after clear")
	}
}
