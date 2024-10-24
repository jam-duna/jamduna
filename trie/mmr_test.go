package trie

import (
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"testing"
)

// Recent History Test
func TestMMRAppend(t *testing.T) {
	mmr := MMR{}
	expected := []MMR{
		MMR{
			peaks: [][]byte{
				common.Hex2Bytes("0x8720b97ddd6acc0f6eb66e095524038675a4e4067adc10ec39939eaefc47d842"),
			},
		},
		MMR{
			peaks: [][]byte{
				nil,
				common.Hex2Bytes("0x7076c31882a5953e097aef8378969945e72807c4705e53a0c5aacc9176f0d56b"),
			},
		},
		MMR{
			peaks: [][]byte{
				nil,
				nil,
				nil,
				common.Hex2Bytes("0x658b919f734bd39262c10589aa1afc657471d902a6a361c044f78de17d660bc6"),
			},
		},
		MMR{
			peaks: [][]byte{
				common.Hex2Bytes("0xa983417440b618f29ed0b7fa65212fce2d363cb2b2c18871a05c4f67217290b0"),
				nil,
				nil,
				common.Hex2Bytes("0x658b919f734bd39262c10589aa1afc657471d902a6a361c044f78de17d660bc6"),
			},
		},
	}

	// test 1
	mmr.Append(common.Hex2Bytes("0x8720b97ddd6acc0f6eb66e095524038675a4e4067adc10ec39939eaefc47d842"))
	if mmr.ComparePeaks(expected[0]) == false {
		t.Fatalf("Test1 FAIL")
	}

	// test 2
	mmr.Append(common.Hex2Bytes("0x7507515a48439dc58bc318c48a120b656136699f42bfd2bd45473becba53462d"))
	if mmr.ComparePeaks(expected[1]) == false {
		t.Fatalf("Test2 FAIL")
	}

	// test 3
	mmr = MMR{}
	mmr.peaks = [][]byte{
		common.Hex2Bytes("0xf986bfeff7411437ca6a23163a96b5582e6739f261e697dc6f3c05a1ada1ed0c"),
		common.Hex2Bytes("0xca29f72b6d40cfdb5814569cf906b3d369ae5f56b63d06f2b6bb47be191182a6"),
		common.Hex2Bytes("0xe17766e385ad36f22ff2357053ab8af6a6335331b90de2aa9c12ec9f397fa414"),
	}
	mmr.Append(common.Hex2Bytes("0x8223d5eaa57ccef85993b7180a593577fd38a65fb41e4bcea2933d8b202905f0"))
	if mmr.ComparePeaks(expected[2]) == false {
		t.Fatalf("Test3 FAIL")
	}

	// test 4
	mmr = MMR{}
	mmr.peaks = [][]byte{
		nil,
		nil,
		nil,
		common.Hex2Bytes("0x658b919f734bd39262c10589aa1afc657471d902a6a361c044f78de17d660bc6"),
	}
	mmr.Append(common.Hex2Bytes("0xa983417440b618f29ed0b7fa65212fce2d363cb2b2c18871a05c4f67217290b0"))
	if mmr.ComparePeaks(expected[3]) == false {
		t.Fatalf("Test4 FAIL")
	}

	fmt.Printf("SUCCESS\n")
}
