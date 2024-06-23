package erasurecoding

import (
    "fmt"
    "log"
    //"math/rand"
    //"time"
)


// Helper functions to convert between byte slices and int slices
func byteArrayToIntArray(data []byte) []int {
    result := make([]int, len(data))
    for i := range data {
        result[i] = int(data[i])
    }
    return result
}

func intArrayToByteArray(data []int) []byte {
    result := make([]byte, len(data))
    for i := range data {
        result[i] = byte(data[i])
    }
    return result
}

// Transpose function
func Transpose(matrix [][]byte) [][]byte {
    rowCount := len(matrix)
    colCount := len(matrix[0])
    transposed := make([][]byte, colCount)
    for i := range transposed {
        transposed[i] = make([]byte, rowCount)
    }
    for i := range matrix {
        for j := range matrix[i] {
            transposed[j][i] = matrix[i][j]
        }
    }
    return transposed
}

// Split function
func Split(data []byte, n int) [][]byte {
    chunks := make([][]byte, (len(data)+n-1)/n)
    for i := 0; i < len(chunks); i++ {
        end := (i + 1) * n
        if end > len(data) {
            end = len(data)
        }
        chunks[i] = data[i*n:end]
    }
    return chunks
}

// Join function
func Join(chunks [][]byte) []byte {
    var joined []byte
    for _, chunk := range chunks {
        joined = append(joined, chunk...)
    }
    return joined
}


// GF16 elements and their logarithms and antilogarithms for efficient computation
var (
    gf16Exp = []int{1, 2, 4, 8, 3, 6, 12, 11, 5, 10, 7, 14, 15, 13, 9, 1}
    gf16Log = map[int]int{
        1: 0, 2: 1, 4: 2, 8: 3, 3: 4, 6: 5, 12: 6, 11: 7,
        5: 8, 10: 9, 7: 10, 14: 11, 15: 12, 13: 13, 9: 14, 0: -1,
    }
)

// GF16Add performs addition in GF(16)
func GF16Add(x, y int) int {
    return x ^ y
}

// GF16Mul performs multiplication in GF(16)
func GF16Mul(x, y int) int {
    if x == 0 || y == 0 {
        return 0
    }
    return gf16Exp[(gf16Log[x]+gf16Log[y])%15]
}

// GF16Div performs division in GF(16)
func GF16Div(x, y int) int {
    if y == 0 {
        log.Fatal("division by zero")
    }
    if x == 0 {
        return 0
    }
    return gf16Exp[(gf16Log[x]-gf16Log[y]+15)%15]
}

// CalculateSyndromes calculates the syndromes for the given data
func CalculateSyndromes(data []int, n, k int) []int {
    syndromes := make([]int, n-k)
    for i := 0; i < n-k; i++ {
        for j := 0; j < n; j++ {
            syndromes[i] = GF16Add(syndromes[i], GF16Mul(data[j], gf16Exp[(i*j)%15]))
        }
    }
    return syndromes
}

// isAllZero checks if all elements in the array are zero
func isAllZero(syndromes []int) bool {
    for _, s := range syndromes {
        if s != 0 {
            return false
        }
    }
    return true
}

// Encode encodes the data using a (6, 3) Reed-Solomon code in GF(16)
func Encode(data []int) []int {
    n := len(data)
    k := n / 2
    parity := make([]int, n-k)

    for i := 0; i < n-k; i++ {
        parity[i] = data[0]
        for j := 1; j < k; j++ {
            parity[i] = GF16Add(parity[i], GF16Mul(data[j], gf16Exp[(i*j)%15]))
        }
    }

    return append(data, parity...)
}

// FindErrorLocatorPolynomial finds the error locator polynomial
func FindErrorLocatorPolynomial(syndromes []int) []int {
    errLoc := []int{1}
    for _, syndrome := range syndromes {
        if syndrome == 0 {
            continue
        }
        nextTerm := GF16Mul(syndrome, errLoc[len(errLoc)-1])
        errLoc = append(errLoc, nextTerm)
    }
    return errLoc
}

// ChienSearch finds the error positions
func ChienSearch(errLoc []int, n int) []int {
    errorPositions := []int{}
    for i := 0; i < n; i++ {
        eval := 0
        for j := 0; j < len(errLoc); j++ {
            eval = GF16Add(eval, GF16Mul(errLoc[j], gf16Exp[(i*j)%15]))
        }
        if eval == 0 {
            errorPositions = append(errorPositions, i)
        }
    }
    return errorPositions
}

// CorrectErrors corrects the errors in the data
func CorrectErrors(data []int, errorPositions []int, syndromes []int) []int {
    correctedData := make([]int, len(data))
    copy(correctedData, data)

    for _, pos := range errorPositions {
        errVal := 0
        for i := 0; i < len(syndromes); i++ {
            errVal = GF16Add(errVal, GF16Mul(syndromes[i], gf16Exp[(pos*i)%15]))
        }
        correctedData[pos] = GF16Add(correctedData[pos], errVal)
    }
    return correctedData
}

// Decode decodes the data using a (6, 3) Reed-Solomon code in GF(16)
func Decode(data []int, n, k int) ([]int, error) {
    syndromes := CalculateSyndromes(data, n, k)

    if isAllZero(syndromes) {
        return data[:k], nil // No errors
    }

    errLoc := FindErrorLocatorPolynomial(syndromes)
    errorPositions := ChienSearch(errLoc, n)

    if len(errorPositions) == 0 {
        return nil, fmt.Errorf("unable to correct errors")
    }

    correctedData := CorrectErrors(data, errorPositions, syndromes)

    return correctedData[:k], nil
}


