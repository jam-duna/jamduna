//go:build linux && amd64
// +build linux,amd64

package recompiler

import (
	"fmt"
	"testing"
)

func TestExecuteX86(t *testing.T) {
	code := []byte{
		0x48, 0xC7, 0xC0, 0x01, 0x00, 0x00, 0x00, // mov rax, 1
		0xC3, // ret
	}
	status, _, _ := ExecuteX86(code, make([]byte, 14*8))
	switch status {
	case 0:
		fmt.Println("✅ Executed successfully")
	case -1:
		t.Fatal("❌ Caught SIGSEGV")
	default:
		t.Fatalf("❌ Failed with status: %d", status)
	}
}
