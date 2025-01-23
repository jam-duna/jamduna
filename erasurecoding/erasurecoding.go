package erasurecoding

import (
	"fmt"

	"github.com/colorfulnotion/jam/types"
)

// Get the coding rate, K and N
func GetCodingRate() (coding_rate_K int, coding_rate_N int) {
	coding_rate_K = types.W_E / 2
	coding_rate_N = types.TotalValidators
	return coding_rate_K, coding_rate_N
}

// Encode data into shards
func Encode(data []byte, shardPieces int) ([][][]byte, error) {
	// Get the coding rate, K and N
	dataShard, totalValidator := GetCodingRate()

	// Calculate the number of groups, which means the number of shards. we call it segments in the past
	shardSize := shardPieces * 2
	groupDataSize := dataShard * shardSize
	// Calculate the number of groups
	numGroups := (len(data) + groupDataSize - 1) / groupDataSize
	out := make([][][]byte, numGroups)

	// Encode the data into shards
	for g := 0; g < numGroups; g++ {
		start := g * groupDataSize
		end := start + groupDataSize
		if end > len(data) {
			end = len(data)
		}

		// Get the data of the group
		groupData := make([]byte, groupDataSize)
		copy(groupData, data[start:end])

		// Transform the data into GFPoints
		m := make([]GFPoint, dataShard*shardPieces)
		for i := 0; i < len(m); i++ {
			b0 := groupData[2*i]
			b1 := groupData[2*i+1]
			m[i] = GFPoint(uint16(b1)<<8 | uint16(b0))
		}

		// Initialize the group with N (= totalValidator) shards
		groupShards := make([][]byte, totalValidator)
		for s := 0; s < totalValidator; s++ {
			groupShards[s] = make([]byte, shardSize)
		}

		// Interpolate the data and generate the shards
		for row := 0; row < shardPieces; row++ {
			xs := make([]GFPoint, dataShard)
			ys := make([]GFPoint, dataShard)
			for i := 0; i < dataShard; i++ {
				idxCantor := ToCantorBasis(GFPoint(i))
				valCantor := ToCantorBasis(m[row*dataShard+i])
				xs[i] = FromCantorBasis(idxCantor)
				ys[i] = FromCantorBasis(valCantor)
			}

			p := Interpolate(xs, ys)
			// Evaluate polynomial and generate the shards
			for s := 0; s < totalValidator; s++ {
				code := Evaluate(p, GFPoint(s))
				low := byte(code & 0xFF)
				high := byte((code >> 8) & 0xFF)
				offset := row * 2
				groupShards[s][offset] = low
				groupShards[s][offset+1] = high
			}
		}
		out[g] = groupShards
	}
	return out, nil
}

// Decode shards into data
func Decode(data [][][]byte, shardPieces int) ([]byte, error) {
	dataShard, _ := GetCodingRate()
	// Each shard is a piece of the data
	shardSize := shardPieces * 2
	groupDataSize := dataShard * shardSize

	var result []byte
	for g := 0; g < len(data); g++ {
		groupShards := data[g]
		if len(groupShards) == 0 {
			continue
		}
		recovered := make([]byte, groupDataSize)

		// Recover the data from the shards
		for row := 0; row < shardPieces; row++ {
			var available []struct {
				Word  GFPoint
				Index int
			}
			// Collect available shards
			for idx := 0; idx < len(groupShards); idx++ {
				if len(groupShards[idx]) < (row+1)*2 {
					continue
				}
				low := groupShards[idx][row*2]
				high := groupShards[idx][row*2+1]
				val := GFPoint(uint16(high)<<8 | uint16(low))
				available = append(available, struct {
					Word  GFPoint
					Index int
				}{Word: val, Index: idx})
			}
			// If there are not enough shards to recover the data, return an error
			if len(available) < dataShard {
				return nil, fmt.Errorf("not enough shards to recover data")
			}

			// Lagrange interpolation
			xs := make([]GFPoint, dataShard)
			ys := make([]GFPoint, dataShard)
			for i := 0; i < dataShard; i++ {
				idx := available[i].Index
				w := available[i].Word
				xs[i] = FromCantorBasis(ToCantorBasis(GFPoint(idx)))
				ys[i] = FromCantorBasis(ToCantorBasis(w))
			}
			p := Interpolate(xs, ys)

			// Recover the data
			for i := 0; i < dataShard; i++ {
				code := Evaluate(p, GFPoint(i))
				low := byte(code & 0xFF)
				high := byte((code >> 8) & 0xFF)
				recovered[2*(row*dataShard+i)] = low
				recovered[2*(row*dataShard+i)+1] = high
			}
		}
		result = append(result, recovered...)
	}
	return result, nil
}
