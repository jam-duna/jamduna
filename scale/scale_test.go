package scale

import (
	"reflect"
	"testing"
)

func TestEncodeEmpty(t *testing.T) {
	expected := []byte{}
	result := EncodeEmpty()
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("EncodeEmpty() = %v, want %v", result, expected)
	}
}

func TestEncodeOctetSequence(t *testing.T) {
	input := []byte{0x01, 0x02, 0x03}
	expected := input
	result := EncodeOctetSequence(input)
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("EncodeOctetSequence(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncodeTuple(t *testing.T) {
	input1 := []byte{0x01}
	input2 := []byte{0x02}
	expected := []byte{0x01, 0x02}
	result := EncodeTuple(input1, input2)
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("EncodeTuple(%v, %v) = %v, want %v", input1, input2, result, expected)
	}
}

func TestEncodeInteger(t *testing.T) {
	input := uint64(258)
	expected := []byte{0x02, 0x01}
	result := EncodeInteger(input)
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("EncodeInteger(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncodeGeneralInteger(t *testing.T) {
	input := uint64(300)
	expected := []byte{0xAC, 0x01}
	result, err := EncodeGeneralInteger(input)
	if err != nil {
		t.Fatalf("EncodeGeneralInteger(%v) returned error: %v", input, err)
	}
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("EncodeGeneralInteger(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncodeSequence(t *testing.T) {
	input := [][]byte{
		{0x01},
		{0x02, 0x03},
	}
	expected := []byte{0x01, 0x02, 0x03}
	result := EncodeSequence(input)
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("EncodeSequence(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncodeDiscriminated(t *testing.T) {
	input := []byte{0x01, 0x02}
	expected := []byte{0x02, 0x01, 0x02}
	result := EncodeDiscriminated(input)
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("EncodeDiscriminated(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncodeBitSequence(t *testing.T) {
	input := []bool{true, false, true, true}
	expected := []byte{0x04, 0xB0}
	result := EncodeBitSequence(input)
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("EncodeBitSequence(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncodeDictionary(t *testing.T) {
	input := map[uint64][]byte{
		1: {0x01},
		2: {0x02, 0x03},
	}
	expected := []byte{0x01, 0x01, 0x02, 0x02, 0x03}
	result := EncodeDictionary(input)
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("EncodeDictionary(%v) = %v, want %v", input, result, expected)
	}
}

func TestEncodeBlock(t *testing.T) {
	h := []byte{0x01, 0x02}
	etr := []byte{0x03, 0x04}
	er := []byte{0x05, 0x06}
	t0 := []uint64{7, 8}
	lv := []uint64{9, 10}
	tu := [][]byte{
		{0x0A},
		{0x0B, 0x0C},
	}
	cu := [][]byte{
		{0x0D},
		{0x0E, 0x0F},
	}
	c := map[uint64][]byte{
		11: {0x10},
		12: {0x11, 0x12},
	}

	expected := EncodeTuple(
		EncodeTuple(h),
		EncodeTuple(etr),
		EncodeTuple(er),
		EncodeTuple(EncodeSequence(tu)),
		EncodeDictionary(c),
	)

	result := EncodeBlock(h, etr, er, t0, lv, tu, cu, c)
	if !reflect.DeepEqual(expected, result) {
		t.Errorf("EncodeBlock() = %v, want %v", result, expected)
	}
}
