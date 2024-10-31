package types

import (
	"github.com/colorfulnotion/jam/common"
	"reflect"
)

type MMR struct {
	Peaks Peaks `json:"peaks"`
}

type Peaks []*common.Hash

// C3 RecentBlocks
func (T Peaks) Decode(data []byte) (interface{}, uint32) {
	if len(data) == 0 {
		return Peaks{}, 0
	}
	peaks_len, length, err := Decode(data, reflect.TypeOf(uint(0)))
	if err != nil {
		return Peaks{}, 0
	}
	if peaks_len.(uint) == 0 {
		return Peaks{}, length
	}
	peaks := make([]*common.Hash, peaks_len.(uint))
	for i := 0; i < int(peaks_len.(uint)); i++ {
		if data[length] == 0 {
			peaks[i] = nil
			length++
		} else if data[length] == 1 {
			length++
			decoded, l, err := Decode(data[length:], reflect.TypeOf(common.Hash{}))
			if err != nil {
				return Peaks{}, 0
			}
			peak := decoded.(common.Hash)
			peaks[i] = &peak
			length += l
		}
	}
	return peaks, length
}

// C3
func (T Peaks) Encode() []byte {
	if len(T) == 0 {
		return []byte{0}
	}
	encoded, err := Encode(uint(len(T)))
	if err != nil {
		return []byte{}
	}
	for i := 0; i < len(T); i++ {
		if T[i] == nil {
			encoded = append(encoded, []byte{0}...)
		} else {
			encoded = append(encoded, []byte{1}...)
			encodedTi, err := Encode(T[i])
			if err != nil {
				return []byte{}
			}
			encoded = append(encoded, encodedTi...)
		}
	}
	return encoded
}
