package storage

import (
	"testing"

	"github.com/colorfulnotion/jam/common"
)

func TestDeriveBinaryTreeKeyJAMDeterministic(t *testing.T) {
	var addr1 common.Address
	var addr2 common.Address
	addr1[0] = 0x01
	addr2[0] = 0x02

	var input [32]byte
	input[31] = 0x05

	k1 := DeriveBinaryTreeKeyJAM(addr1, input)
	k2 := DeriveBinaryTreeKeyJAM(addr1, input)
	if k1 != k2 {
		t.Fatalf("determinism failure: %v != %v", k1, k2)
	}
	k3 := DeriveBinaryTreeKeyJAM(addr2, input)
	if k1.Stem == k3.Stem {
		t.Fatalf("expected different stems for different addresses")
	}
}

func TestDeriveBinaryTreeKeyEIPDeterministic(t *testing.T) {
	var addr1 common.Address
	var addr2 common.Address
	addr1[0] = 0x01
	addr2[0] = 0x02

	var input [32]byte
	input[31] = 0x05

	k1 := DeriveBinaryTreeKeyEIP(addr1, input)
	k2 := DeriveBinaryTreeKeyEIP(addr1, input)
	if k1 != k2 {
		t.Fatalf("determinism failure: %v != %v", k1, k2)
	}
	k3 := DeriveBinaryTreeKeyEIP(addr2, input)
	if k1.Stem == k3.Stem {
		t.Fatalf("expected different stems for different addresses")
	}
}

func TestBasicDataLeafEncodeDecode(t *testing.T) {
	var balance [16]byte
	balance[15] = 0x7f
	leaf := NewBasicDataLeaf(42, balance, 0x010203)
	encoded := leaf.Encode()
	decoded := DecodeBasicDataLeaf(encoded)

	if decoded.Version != 0 {
		t.Fatalf("version mismatch: got %d", decoded.Version)
	}
	if decoded.CodeSize != 0x010203 {
		t.Fatalf("code size mismatch: got %d", decoded.CodeSize)
	}
	if decoded.Nonce != 42 {
		t.Fatalf("nonce mismatch: got %d", decoded.Nonce)
	}
	if decoded.Balance != balance {
		t.Fatalf("balance mismatch: got %x want %x", decoded.Balance, balance)
	}
}

func TestGetStorageSlotKeyMainStorageOffset(t *testing.T) {
	var addr common.Address
	addr[0] = 0x22
	addr[19] = 0x99

	var slot [32]byte
	slot[0] = 0x10
	slot[5] = 0xAA
	slot[30] = 0xBB
	slot[31] = 0x7F

	got := GetStorageSlotKey(EIPProfile, addr, slot)

	var input [32]byte
	copy(input[1:31], slot[1:31])
	input[0] = slot[0] + 1
	input[31] = slot[31]

	want := GetBinaryTreeKey(EIPProfile, addr, input)
	if got != want {
		t.Fatalf("unexpected storage slot key: got %x want %x", got, want)
	}
}

func TestGetStorageSlotKeyMainStorageWraps(t *testing.T) {
	var addr common.Address
	addr[0] = 0x33

	var slot [32]byte
	slot[0] = 0xFF
	slot[1] = 0x10
	slot[30] = 0x20
	slot[31] = 0x01

	got := GetStorageSlotKey(EIPProfile, addr, slot)

	var input [32]byte
	copy(input[1:31], slot[1:31])
	input[0] = 0x00
	input[31] = slot[31]

	want := GetBinaryTreeKey(EIPProfile, addr, input)
	if got != want {
		t.Fatalf("unexpected storage slot key wrap: got %x want %x", got, want)
	}
}
