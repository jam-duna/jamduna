package common

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"
)

func TestJustificationEncodeDecode(t *testing.T) {
	// make [][]byte with 10 elements
	justifications := make([][]byte, 3)
	//ed7d142e65b94ad9ffc8f69fb687fcb738a16fcc396dcbe51530b28ff33735cd0000000000000000000000000000000000000000000000000000000000000000 e24331aa21bd1e175a8b5946f94004e41ba7348c9fb4f88c4fff9217a28581b10000000000000000000000000000000000000000000000000000000000000000 108de6e76e42899ca13192704d481f960c72618d6b698a70899d6e6752b6c12f
	justifications_text := []string{
		"ed7d142e65b94ad9ffc8f69fb687fcb738a16fcc396dcbe51530b28ff33735cd0000000000000000000000000000000000000000000000000000000000000000",
		"4aa7c9a7991d13c34e85fb12352b85e2d643fc69b885db49f0d2918ba91091000000000000000000000000000000000000000000000000000000000000000000",
		"108de6e76e42899ca13192704d481f960c72618d6b698a70899d6e6752b6c12f",
	}
	for i, text := range justifications_text {
		justifications[i], _ = hex.DecodeString(text)
	}
	// encode the justification
	encoded, err := EncodeJustification(justifications, 1026)
	if err != nil {
		t.Fatalf("EncodeJustification error: %v", err)
	}

	// decode the justification
	decoded, err := DecodeJustification(encoded, 1026)
	if err != nil {
		t.Fatalf("DecodeJustification error: %v", err)
	}

	// compare the original justification with the decoded justification
	if len(justifications) != len(decoded) {
		t.Fatalf("len(justifications) != len(decoded)")
	}
	// use reflect.DeepEqual to compare the two slices
	for i := 0; i < len(justifications); i++ {
		if !reflect.DeepEqual(justifications[i], decoded[i]) {
			t.Fatalf("justifications[%d] != decoded[%d]", i, i)
		}
	}

	for _, decoded_text := range decoded {
		fmt.Printf("decoded_text: %x\n", decoded_text)
	}
}

func TestMetadataUsage(t *testing.T) {
	addrs := []string{
		"127.0.0.1:9900",         // IPv4
		"[::1]:9900",             // loopback IPv6
		"[2001:db8::dead]:30333", // normal IPv6
	}

	for _, addr := range addrs {
		addr := addr
		t.Run("encode_"+addr, func(t *testing.T) {
			t.Parallel()

			meta, err := AddressToMetadata(addr)
			if err != nil {
				t.Fatalf("❌ Failed to encode %s: %v", addr, err)
			}
			fmt.Printf("✅ Encoded %s → %x\n", addr, meta)

			decodedAddr, port, err := MetadataToAddress(meta)
			if err != nil {
				t.Fatalf("❌ Failed to decode metadata %x: %v", meta, err)
			}
			fmt.Printf("✅ Decoded %x → %s:%d\n", meta, decodedAddr, port)
		})
	}
}
