package pvm

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"syscall"
	"testing"
	"unsafe"
)

func TestX86MemAddStore(t *testing.T) {
	memSize := 4096
	mem, err := syscall.Mmap(
		-1, 0, memSize,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		t.Fatalf("failed to mmap memory: %v", err)
	}

	binary.LittleEndian.PutUint64(mem[0:], 1)
	binary.LittleEndian.PutUint64(mem[8:], 5)

	// x86code:
	// mov rbx, memAddr
	// mov rax, [rbx]
	// add rax, [rbx+8]
	// mov [rbx+16], rax
	// ret
	x86code := []byte{
		0x48, 0xbb, /* addr 8 bytes */ // mov rbx, memAddr
		0x48, 0x8b, 0x03, // mov rax, [rbx]
		0x48, 0x03, 0x43, 0x08, // add rax, [rbx+8]
		0x48, 0x89, 0x43, 0x10, // mov [rbx+16], rax
		0xc3, // ret
	}

	memAddr := uintptr(unsafe.Pointer(&mem[0]))
	addrBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(addrBuf, uint64(memAddr))
	x86code = append(x86code[:2], append(addrBuf, x86code[2:]...)...)

	codeAddr, err := syscall.Mmap(
		-1, 0, len(x86code),
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
	if err != nil {
		t.Fatalf("failed to mmap exec code: %v", err)
	}
	copy(codeAddr, x86code)

	type execFunc func() int
	fnPtr := (uintptr)(unsafe.Pointer(&codeAddr))
	fn := *(*execFunc)(unsafe.Pointer(&fnPtr))
	result := fn()

	sum := binary.LittleEndian.Uint64(mem[16:])

	fmt.Printf("Result = %d (should be 6)\n", result)
	fmt.Printf("Memory[16] = %d (should be 6)\n", sum)
}

func TestCode(t *testing.T) {

	PvmLogging = true
	filePath := "../jamtestvectors/pvm/programs/inst_add_64.json"
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}

	var testCase TestCase
	err = json.Unmarshal(data, &testCase)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
	}
	_, err = pvm_test(testCase)
	if err != nil {
		t.Fatalf("%v", err)
	}

}
