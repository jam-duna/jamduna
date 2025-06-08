package x86_execute

import (
	"fmt"
	"testing"
)

func TestExecuteX86(t *testing.T) {
	// 程式碼：mov rax, 1; ret
	code := []byte{
		0x48, 0xC7, 0xC0, 0x01, 0x00, 0x00, 0x00, // mov rax, 1
		0xC3, // ret
	}
	status := ExecuteX86(code)
	switch status {
	case 0:
		fmt.Println("✅ Executed successfully")
	case -1:
		t.Fatal("❌ Caught SIGSEGV")
	default:
		t.Fatalf("❌ Failed with status: %d", status)
	}
}
