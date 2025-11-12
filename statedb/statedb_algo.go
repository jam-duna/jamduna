package statedb

import (
	"fmt"
	"math/rand"
)

func GenerateAlgoPayload(sz int, isSimple bool) []byte {
	// Create a deterministic random generator based on sz for reproducible payloads
	seed := int64(1107 + sz)
	rng := rand.New(rand.NewSource(seed))

	if isSimple {
		algo_payload := make([]byte, 2)
		for i := 0; i < 1; i++ {
			algo_payload[i*2] = byte(sz)
			algo_payload[i*2+1] = byte(5)
		}
		fmt.Printf("GenerateAlgoPayload SIMPLE: sz=%d, payload: %x:\n", sz, algo_payload)
		return algo_payload
	}
	algo_payload := make([]byte, sz*2)
	useCase := "top5"
	var algos []uint8
	top5 := []uint8{7, 8, 9, 11, 53}
	top10 := []uint8{0, 4, 7, 8, 9, 11, 23, 27, 52, 53}
	top20 := []uint8{0, 1, 3, 4, 7, 8, 9, 11, 12, 13, 23, 26, 27, 28, 35, 36, 50, 51, 52, 53}
	top30 := []uint8{0, 1, 2, 3, 4, 5, 7, 8, 9, 11, 12, 13, 21, 22, 23, 26, 27, 28, 32, 34, 35, 36, 45, 47, 48, 49, 50, 51, 52, 53}
	top40 := []uint8{0, 1, 2, 3, 4, 5, 7, 8, 9, 11, 12, 13, 18, 19, 20, 21, 22, 23, 24, 26, 27, 28, 29, 30, 31, 32, 34, 35, 36, 39, 40, 42, 45, 47, 48, 49, 50, 51, 52, 53}
	all := []uint8{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 11, 12, 13, 14, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 42, 44, 45, 47, 48, 49, 50, 51, 52, 53} // exclude 10, 15, 16, 41, 43, 46
	switch useCase {
	case "top5":
		algos = top5
	case "top10":
		algos = top10
	case "top20":
		algos = top20
	case "top30":
		algos = top30
	case "top40":
		algos = top40
	default:
		algos = all
	}
	algo_payload_str := make([]string, sz)
	for j := 0; j < sz; j++ {
		// pick a random element from algos
		p := algos[rng.Intn(len(algos))]

		// pick a random number from c_min, c_min+c_rand
		c_rand := rng.Intn(8)
		c_min := 1

		c_n := c_min + c_rand
		algo_payload[j*2] = byte(p)
		algo_payload[j*2+1] = byte(c_n)
		algo_payload_str[j] = fmt.Sprintf("algo_%03d\t n=%d^3\t Iter=%d\n", p, c_n, c_n*c_n*c_n)
	}
	fmt.Printf("GenerateAlgoPayload: sz=%d, payload: %x:\n%v\n", sz, algo_payload, algo_payload_str)
	return algo_payload
}
